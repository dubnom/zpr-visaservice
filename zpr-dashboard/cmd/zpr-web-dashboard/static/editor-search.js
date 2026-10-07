(() => {
  for (const [sourceID, historyID] of [["scenario-editor-source", "scenario-editor-revisions"], ["directory-editor-source", "directory-editor-revisions"]]) {
    const history = document.getElementById(historyID);
    if (!history) continue;
    const host = document.createElement("div");
    host.dataset.editorSearchTarget = sourceID;
    const label = history.closest("label");
    const group = document.createElement("div");
    group.className = "editor-history-tools";
    label.before(group);
    group.append(host, label);
  }
  for (const host of document.querySelectorAll("[data-editor-search-target]")) {
    const sources = host.dataset.editorSearchTarget.split(",").map((id) => document.getElementById(id)).filter(Boolean);
    host.classList.add("editor-search-tools");
    host.innerHTML = `<button class="button" type="button" data-search-open>Search</button><button class="button" type="button" data-replace-open>Replace</button><div class="editor-search-panel" role="search" aria-label="Source search and replace" hidden><label>Find<input data-search-query type="text" aria-label="Find text" autocomplete="off"></label><label><input data-search-case type="checkbox"> Match case</label><div class="editor-search-actions"><button class="button" type="button" data-search-previous>Previous</button><button class="button" type="button" data-search-next>Next</button><button class="button" type="button" data-search-close>Close</button></div><div class="editor-replace-row" hidden><label>Replace with<input data-search-replacement type="text" aria-label="Replacement text" autocomplete="off"></label><button class="button" type="button" data-search-replace>Replace match</button><button class="button" type="button" data-search-replace-all>Replace all</button></div><p data-search-status role="status" aria-live="polite"></p></div>`;
    const panel = host.querySelector(".editor-search-panel");
    const query = host.querySelector("[data-search-query]");
    const matchCase = host.querySelector("[data-search-case]");
    const replacement = host.querySelector("[data-search-replacement]");
    const replacementRow = host.querySelector(".editor-replace-row");
    const status = host.querySelector("[data-search-status]");
    const target = () => sources.find((source) => source.getClientRects().length) || sources[0];

    function matches(source) {
      if (!query.value) return [];
      const literal = query.value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      return [...source.value.matchAll(new RegExp(literal, matchCase.checked ? "g" : "gi"))];
    }

    function refresh() {
      if (panel.hidden) return;
      const source = target();
      const count = matches(source).length;
      status.textContent = query.value ? `${count} ${count === 1 ? "match" : "matches"}` : "Enter text to find. Searches are literal, not regular expressions.";
      for (const button of host.querySelectorAll("[data-search-previous], [data-search-next]")) button.disabled = !count || source.disabled;
      for (const button of host.querySelectorAll("[data-search-replace], [data-search-replace-all]")) button.disabled = !count || source.disabled || source.readOnly;
    }

    function open(replace) {
      const source = target();
      const details = source.closest("details");
      if (details) details.open = true;
      const selected = source.value.slice(source.selectionStart, source.selectionEnd);
      if (selected && !selected.includes("\n")) query.value = selected;
      panel.hidden = false;
      replacementRow.hidden = !replace;
      host.querySelector("[data-search-open]").setAttribute("aria-expanded", "true");
      host.querySelector("[data-replace-open]").setAttribute("aria-expanded", String(replace));
      refresh();
      query.focus();
      query.select();
    }

    function close() {
      panel.hidden = true;
      for (const button of host.querySelectorAll("[data-search-open], [data-replace-open]")) button.setAttribute("aria-expanded", "false");
      target().focus();
    }

    function find(previous = false) {
      const source = target();
      const found = matches(source);
      if (!found.length || source.disabled) { refresh(); return null; }
      const match = previous
        ? found.findLast((entry) => entry.index + entry[0].length <= source.selectionStart) || found.at(-1)
        : found.find((entry) => entry.index >= source.selectionEnd) || found[0];
      source.focus();
      source.setSelectionRange(match.index, match.index + match[0].length);
      const before = source.value.slice(0, match.index);
      const line = before.split("\n").length;
      const style = getComputedStyle(source);
      const lineHeight = Number.parseFloat(style.lineHeight) || Number.parseFloat(style.fontSize) * 1.6;
      const mirror = document.createElement("div");
      Object.assign(mirror.style, {
        position: "fixed", visibility: "hidden", width: `${source.clientWidth}px`,
        boxSizing: "border-box", font: style.font, padding: style.padding,
        letterSpacing: style.letterSpacing, tabSize: style.tabSize,
        whiteSpace: source.wrap === "off" ? "pre" : "pre-wrap", overflowWrap: "break-word",
      });
      mirror.textContent = before;
      const caret = document.createElement("span");
      caret.textContent = match[0][0];
      mirror.append(caret);
      document.body.append(mirror);
      const top = caret.offsetTop;
      const left = caret.offsetLeft;
      mirror.remove();
      if (top < source.scrollTop || top + lineHeight > source.scrollTop + source.clientHeight) source.scrollTop = top;
      if (source.wrap === "off" && (left < source.scrollLeft || left + 20 > source.scrollLeft + source.clientWidth)) source.scrollLeft = Math.max(0, left - source.clientWidth / 2);
      source.dispatchEvent(new Event("scroll"));
      status.textContent = `Match ${found.indexOf(match) + 1} of ${found.length} · line ${line}, column ${match.index - before.lastIndexOf("\n")}`;
      return match;
    }

    function replace(all) {
      const source = target();
      if (source.disabled || source.readOnly) { status.textContent = "This source is not editable."; return; }
      let found = matches(source);
      if (!found.length) { refresh(); return; }
      if (!all) {
        const selected = found.find((entry) => entry.index === source.selectionStart && entry.index + entry[0].length === source.selectionEnd);
        found = [selected || find()];
      }
      let cursor = 0;
      let next = "";
      for (const match of found) {
        next += source.value.slice(cursor, match.index) + replacement.value;
        cursor = match.index + match[0].length;
      }
      next += source.value.slice(cursor);
      if (source.maxLength >= 0 && next.length > source.maxLength) {
        status.textContent = `Replacement exceeds the ${source.maxLength.toLocaleString()} character editor limit. No changes made.`;
        return;
      }
      source.value = next;
      const caret = found[0].index + replacement.value.length;
      source.setSelectionRange(caret, caret);
      source.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertReplacementText" }));
      refresh();
      status.textContent = `Replaced ${found.length} ${found.length === 1 ? "match" : "matches"}. ${matches(source).length} remaining. Changes are unsaved.`;
    }

    host.querySelector("[data-search-open]").addEventListener("click", () => open(false));
    host.querySelector("[data-replace-open]").addEventListener("click", () => open(true));
    host.querySelector("[data-search-close]").addEventListener("click", close);
    host.querySelector("[data-search-previous]").addEventListener("click", () => find(true));
    host.querySelector("[data-search-next]").addEventListener("click", () => find());
    host.querySelector("[data-search-replace]").addEventListener("click", () => replace(false));
    host.querySelector("[data-search-replace-all]").addEventListener("click", () => replace(true));
    query.addEventListener("input", refresh);
    matchCase.addEventListener("change", refresh);
    panel.addEventListener("keydown", (event) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); }
      else if (event.key === "Enter") { event.preventDefault(); find(event.shiftKey); }
    });
    for (const source of sources) {
      source.addEventListener("input", refresh);
      source.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && !panel.hidden) { event.preventDefault(); event.stopPropagation(); close(); return; }
        if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
        const key = event.key.toLowerCase();
        if (key === "f" || key === "h") { event.preventDefault(); open(key === "h"); }
      });
    }
  }
})();
