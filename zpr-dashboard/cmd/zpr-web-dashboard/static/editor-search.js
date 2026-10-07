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
    host.innerHTML = `<button class="button editor-search-toggle" type="button" data-search-toggle aria-expanded="false" aria-haspopup="dialog" title="Find and replace (Ctrl/⌘-F, Ctrl/⌘-H)">Find &amp; Replace</button>`
      + `<div class="editor-search-panel" role="dialog" aria-label="Find and Replace" hidden>`
      + `<div class="find-dialog-title"><span>Find and Replace</span><button class="find-dialog-x" type="button" data-search-close aria-label="Close">×</button></div>`
      + `<div class="find-dialog-tabs" role="tablist" aria-label="Find and Replace mode"><button type="button" role="tab" data-search-tab="find" aria-selected="true">Find</button><button type="button" role="tab" data-search-tab="replace" aria-selected="false" tabindex="-1">Replace</button></div>`
      + `<div class="find-dialog-body" role="search">`
      + `<label class="find-dialog-field"><span>Find what:</span><input data-search-query type="text" aria-label="Find what" autocomplete="off" spellcheck="false"></label>`
      + `<label class="find-dialog-field" data-replace-only><span>Replace with:</span><input data-search-replacement type="text" aria-label="Replace with" autocomplete="off" spellcheck="false"></label>`
      + `<div class="find-dialog-buttons"><button class="button" type="button" data-search-more aria-expanded="false">More &gt;&gt;</button><span class="find-dialog-spacer"></span><button class="button" type="button" data-search-replace data-replace-only>Replace</button><button class="button" type="button" data-search-replace-all data-replace-only>Replace All</button><button class="button find-dialog-default" type="button" data-search-next>Find Next</button><button class="button" type="button" data-search-cancel>Cancel</button></div>`
      + `<fieldset class="find-dialog-options" data-search-options hidden><legend>Search Options</legend><label>Search: <select data-search-direction aria-label="Search direction"><option value="all">All</option><option value="down">Down</option><option value="up">Up</option></select></label><label><input data-search-case type="checkbox"> Match case</label><label><input data-search-regex type="checkbox"> Use regular expressions</label></fieldset>`
      + `<p data-search-status role="status" aria-live="polite"></p></div></div>`;
    const panel = host.querySelector(".editor-search-panel");
    const query = host.querySelector("[data-search-query]");
    const matchCase = host.querySelector("[data-search-case]");
    const regex = host.querySelector("[data-search-regex]");
    const replacement = host.querySelector("[data-search-replacement]");
    const toggle = host.querySelector("[data-search-toggle]");
    const direction = host.querySelector("[data-search-direction]");
    const more = host.querySelector("[data-search-more]");
    const options = host.querySelector("[data-search-options]");
    const tabs = [...host.querySelectorAll("[data-search-tab]")];
    let mode = "find";

    function setMode(next) {
      mode = next;
      for (const tab of tabs) {
        const selected = tab.dataset.searchTab === mode;
        tab.setAttribute("aria-selected", String(selected));
        tab.tabIndex = selected ? 0 : -1;
      }
      for (const element of host.querySelectorAll("[data-replace-only]")) element.hidden = mode !== "replace";
      panel.dataset.mode = mode;
    }
    const status = host.querySelector("[data-search-status]");
    const target = () => sources.find((source) => source.getClientRects().length) || sources[0];

    let patternError = "";

    function matches(source) {
      patternError = "";
      if (!query.value) return [];
      const pattern = regex.checked ? query.value : query.value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      let expression;
      try {
        expression = new RegExp(pattern, matchCase.checked ? "g" : "gi");
      } catch (error) {
        patternError = `Invalid regular expression: ${error.message.replace(/^Invalid regular expression: /, "")}`;
        return [];
      }
      return [...source.value.matchAll(expression)].filter((match) => match[0].length);
    }

    function expand(match) {
      if (!regex.checked) return replacement.value;
      return replacement.value.replace(/\$(\$|&|<([^>]+)>|(\d{1,2}))/g, (token, kind, name, digits) => {
        if (kind === "$") return "$";
        if (kind === "&") return match[0];
        if (name !== undefined) return match.groups?.[name] ?? token;
        let index = Number(digits);
        if (digits.length === 2 && index >= match.length) index = Number(digits[0]);
        if (index < 1 || index >= match.length) return token;
        return (match[index] ?? "") + (digits.length === 2 && index !== Number(digits) ? digits[1] : "");
      });
    }

    function refresh() {
      if (panel.hidden) return;
      const source = target();
      const count = matches(source).length;
      status.textContent = patternError || (query.value ? `${count} ${count === 1 ? "match" : "matches"}` : "");
      host.querySelector("[data-search-next]").disabled = !count || source.disabled;
      for (const button of host.querySelectorAll("[data-search-replace], [data-search-replace-all]")) button.disabled = !count || source.disabled || source.readOnly;
    }

    function open(nextMode = mode) {
      setMode(nextMode);
      const source = target();
      const details = source.closest("details");
      if (details) details.open = true;
      const selected = source.value.slice(source.selectionStart, source.selectionEnd);
      if (selected && !selected.includes("\n")) query.value = selected;
      panel.hidden = false;
      toggle.setAttribute("aria-expanded", "true");
      refresh();
      const field = mode === "replace" && query.value ? replacement : query;
      field.focus();
      field.select();
    }

    function close() {
      panel.hidden = true;
      toggle.setAttribute("aria-expanded", "false");
      target().focus();
    }

    function find(previous = false) {
      const source = target();
      const found = matches(source);
      if (!found.length || source.disabled) { refresh(); return null; }
      const backward = previous !== (direction.value === "up");
      const wrap = direction.value === "all";
      let match = backward
        ? found.findLast((entry) => entry.index + entry[0].length <= source.selectionStart)
        : found.find((entry) => entry.index >= source.selectionEnd);
      if (!match && !wrap) {
        status.textContent = backward ? "Reached the beginning of the source." : "Reached the end of the source.";
        return null;
      }
      match ||= backward ? found.at(-1) : found[0];
      source.focus();
      source.setSelectionRange(match.index, match.index + match[0].length);
      const before = source.value.slice(0, match.index);
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
      status.textContent = `Match ${found.indexOf(match) + 1} of ${found.length}`;
      return match;
    }

    function replace(all) {
      const source = target();
      if (source.disabled || source.readOnly) { status.textContent = "This source is not editable."; return; }
      let found = matches(source);
      if (!found.length) { refresh(); return; }
      if (!all) {
        const selected = found.find((entry) => entry.index === source.selectionStart && entry.index + entry[0].length === source.selectionEnd);
        const next = selected || find();
        if (!next) return;
        found = [next];
      }
      const start = found[0].index;
      const end = found.at(-1).index + found.at(-1)[0].length;
      let cursor = start;
      let middle = "";
      for (const match of found) {
        middle += source.value.slice(cursor, match.index) + expand(match);
        cursor = match.index + match[0].length;
      }
      const length = source.value.length - (end - start) + middle.length;
      if (source.maxLength >= 0 && length > source.maxLength) {
        status.textContent = `Replacement exceeds the ${source.maxLength.toLocaleString()} character editor limit. No changes made.`;
        return;
      }
      // Native editing commands keep replacements on the textarea's undo stack.
      const control = host.contains(document.activeElement) ? document.activeElement : null;
      source.focus();
      source.setSelectionRange(start, end);
      const edited = middle
        ? document.execCommand("insertText", false, middle)
        : document.execCommand("delete", false);
      if (!edited || source.value.length !== length) {
        source.setRangeText(middle, start, end, "end");
        source.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertReplacementText" }));
      }
      const caret = start + middle.length;
      source.setSelectionRange(caret, caret);
      control?.focus();
      refresh();
      status.textContent = `Replaced ${found.length} ${found.length === 1 ? "match" : "matches"}. ${matches(source).length} remaining. Changes are unsaved.`;
    }

    toggle.addEventListener("click", () => (panel.hidden ? open("replace") : close()));
    host.querySelector("[data-search-close]").addEventListener("click", close);
    host.querySelector("[data-search-cancel]").addEventListener("click", close);
    host.querySelector("[data-search-next]").addEventListener("click", () => find());
    more.addEventListener("click", () => {
      options.hidden = !options.hidden;
      more.setAttribute("aria-expanded", String(!options.hidden));
      more.innerHTML = options.hidden ? "More &gt;&gt;" : "&lt;&lt; Less";
    });
    for (const tab of tabs) {
      tab.addEventListener("click", () => { setMode(tab.dataset.searchTab); refresh(); (mode === "replace" && query.value ? replacement : query).focus(); });
      tab.addEventListener("keydown", (event) => {
        if (!["ArrowLeft", "ArrowRight"].includes(event.key)) return;
        event.preventDefault();
        const next = tabs[(tabs.indexOf(tab) + 1) % tabs.length];
        setMode(next.dataset.searchTab);
        next.focus();
      });
    }
    host.querySelector("[data-search-replace]").addEventListener("click", () => replace(false));
    host.querySelector("[data-search-replace-all]").addEventListener("click", () => replace(true));
    query.addEventListener("input", refresh);
    matchCase.addEventListener("change", refresh);
    regex.addEventListener("change", refresh);
    panel.addEventListener("keydown", (event) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); close(); }
      else if (event.key === "Enter" && event.target.matches("input")) { event.preventDefault(); find(event.shiftKey); }
    });
    for (const source of sources) {
      source.addEventListener("input", refresh);
      source.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && !panel.hidden) { event.preventDefault(); event.stopPropagation(); close(); return; }
        if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
        const key = event.key.toLowerCase();
        if (key === "f" || key === "h") { event.preventDefault(); open(key === "h" ? "replace" : "find"); }
      });
    }
  }
})();
