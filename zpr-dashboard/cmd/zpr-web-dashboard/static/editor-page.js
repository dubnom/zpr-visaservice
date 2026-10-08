// Shared editor-page blueprint used by Control Room and Simulator editors. It has no service dependencies.
(() => {
  const tokenPatterns = {
    toml: {
      pattern: /("""(?:\\[\s\S]|(?!""")[^\\])*"""|'''[\s\S]*?'''|"(?:\\.|[^"\\\n])*"|'[^'\n]*'|#[^\n]*|^[ \t]*\[\[?[^\n\]]+\]\]?|\b(?:true|false|inf|nan)\b|[+-]?\b\d[\w.+:-]*\b|[A-Za-z0-9_-]+(?=\s*(?:\.|=)))/gm,
      classify: (text) => text.startsWith("#") ? "comment" : /^["']/.test(text) ? "string" : text.trimStart().startsWith("[") ? "class-definition" : /^(true|false|inf|nan)$/.test(text) ? "keyword" : /^[+\-\d]/.test(text) ? "value" : "attribute",
    },
    json: {
      pattern: /"(?:\\.|[^"\\\n])*"|\b(?:true|false|null)\b|-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b|[{}[\],:]/g,
      classify: (text, rest) => text.startsWith('"') ? (/^\s*:/.test(rest) ? "attribute" : "string") : /^(true|false|null)$/.test(text) ? "keyword" : /^[-\d]/.test(text) ? "value" : "punctuation",
    },
    yaml: {
      pattern: /#[^\n]*|"(?:\\.|[^"\\\n])*"|'[^'\n]*'|^[ \t]*[-?]?[ \t]*[A-Za-z0-9_.-]+(?=\s*:)|\b(?:true|false|null)\b|-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/gm,
      classify: (text) => text.trimStart().startsWith("#") ? "comment" : /^["']/.test(text) ? "string" : /^[ \t]*[-?]?[ \t]*[A-Za-z0-9_.-]+$/.test(text) ? "attribute" : /^(true|false|null)$/.test(text) ? "keyword" : "value",
    },
    ldif: {
      pattern: /^#[^\n]*|^(?:dn|changetype)(?=::?[ \t])|^[A-Za-z][\w;-]*(?=::?[ \t]|::?$)|\b(?:cn|ou|dc|uid|o)=/gm,
      classify: (text) => text.startsWith("#") ? "comment" : /^(dn|changetype)$/.test(text) ? "keyword" : text.endsWith("=") ? "value" : "attribute",
    },
  };

  function highlight(pre, text, language) {
    const spec = tokenPatterns[language];
    pre.replaceChildren();
    if (!spec) {
      pre.append(document.createTextNode(text + "\n"));
      return;
    }
    let offset = 0;
    for (const match of text.matchAll(spec.pattern)) {
      if (!match[0]) continue;
      pre.append(document.createTextNode(text.slice(offset, match.index)));
      const token = document.createElement("span");
      token.className = `zpl-${spec.classify(match[0], text.slice(match.index + match[0].length))}`;
      token.textContent = match[0];
      pre.append(token);
      offset = match.index + match[0].length;
    }
    pre.append(document.createTextNode(text.slice(offset) + "\n"));
  }

  function lineOffset(text, index) {
    return text.split("\n").slice(0, index).reduce((length, line) => length + line.length + 1, 0);
  }

  // Wires a textarea, highlight overlay and diagnostic gutter into one code surface.
  function createSourceSurface({ source, highlight: pre, gutter, language, label = "Source", onMarker }) {
    let diagnostic = null;
    const container = source.closest(".config-source-editor");
    const languageName = () => typeof language === "function" ? language() : language;
    function syncScroll() {
      gutter.scrollTop = source.scrollTop;
      pre.scrollTop = source.scrollTop;
      pre.scrollLeft = source.scrollLeft;
      if (container) container.dataset.horizontalOverflow = String(source.scrollWidth > source.clientWidth);
    }
    function selectLine(line) {
      const lines = source.value.split("\n");
      const index = Math.max(0, Math.min(lines.length - 1, line - 1));
      const start = lineOffset(source.value, index);
      source.focus();
      source.setSelectionRange(start, start + lines[index].length);
      source.scrollTop = index * (Number.parseFloat(getComputedStyle(source).lineHeight) || 18);
      syncScroll();
    }
    function render() {
      gutter.replaceChildren(...source.value.split("\n").map((_, index) => {
        const row = document.createElement("div");
        row.className = "config-gutter-line";
        row.dataset.line = String(index + 1);
        if (diagnostic?.line === index + 1) {
          const marker = document.createElement("button");
          marker.type = "button";
          marker.className = "config-error-marker";
          marker.title = diagnostic.message;
          marker.setAttribute("aria-label", `${typeof label === "function" ? label() : label} error on line ${index + 1}: ${diagnostic.message}`);
          marker.textContent = "!";
          marker.addEventListener("click", () => {
            selectLine(index + 1);
            onMarker?.(diagnostic);
          });
          row.append(marker);
        } else {
          const spacer = document.createElement("span");
          spacer.className = "config-gutter-marker-space";
          spacer.setAttribute("aria-hidden", "true");
          row.append(spacer);
        }
        return row;
      }));
      highlight(pre, source.value, languageName());
      syncScroll();
    }
    source.addEventListener("scroll", syncScroll);
    if (window.ResizeObserver) new ResizeObserver(syncScroll).observe(source);
    return {
      render,
      syncScroll,
      selectLine,
      get diagnostic() { return diagnostic; },
      setDiagnostic(next) {
        const line = Number(next?.line);
        diagnostic = Number.isInteger(line) && line > 0 && line <= source.value.split("\n").length
          ? { line, message: String(next.message || "") } : null;
      },
    };
  }

  // Policy-style button menu (File...): toggle button + role=menu popover, Escape and outside click close it.
  function createMenu({ root, toggle, menu }) {
    function setOpen(open, restoreFocus = false) {
      menu.hidden = !open;
      toggle.setAttribute("aria-expanded", String(open));
      if (open) menu.querySelector("button:not(:disabled)")?.focus();
      else if (restoreFocus) toggle.focus();
    }
    toggle.addEventListener("click", () => setOpen(menu.hidden));
    menu.addEventListener("click", (event) => { if (event.target.closest("button:not(:disabled)")) setOpen(false); });
    menu.addEventListener("keydown", (event) => {
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      const items = [...menu.querySelectorAll("button:not(:disabled)")];
      if (!items.length) return;
      event.preventDefault();
      const current = items.indexOf(document.activeElement);
      const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : (current + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
      items[next].focus();
    });
    document.addEventListener("pointerdown", (event) => { if (!menu.hidden && !root.contains(event.target)) setOpen(false); });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !menu.hidden) { event.preventDefault(); setOpen(false, true); }
    });
    return { setOpen, get open() { return !menu.hidden; } };
  }

  // Policy-style Browse... picker pane.
  function createPicker({ toggle, pane, close, focus }) {
    function setOpen(open, restoreFocus = false) {
      pane.hidden = !open;
      toggle.setAttribute("aria-expanded", String(open));
      if (open) (focus || pane).focus();
      else if (restoreFocus) toggle.focus();
    }
    toggle.addEventListener("click", () => setOpen(pane.hidden));
    close?.addEventListener("click", () => setOpen(false, true));
    document.addEventListener("pointerdown", (event) => {
      if (!pane.hidden && !pane.contains(event.target) && !toggle.contains(event.target)) setOpen(false);
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !pane.hidden && !document.querySelector("[role=menu]:not([hidden])")) setOpen(false, true);
    });
    return { setOpen, get open() { return !pane.hidden; } };
  }

  // Policy-style History <details> dropdown. History cannot open when nothing is selected.
  function placeHistory(menu) {
    const identity = menu.closest(".policy-page")?.querySelector(".policy-identity");
    if (identity) identity.querySelector("strong")?.after(menu);
  }

  function createHistory({ menu, list, count, isAvailable }) {
    placeHistory(menu);
    const summary = menu.querySelector("summary");
    summary.addEventListener("click", (event) => { if (!isAvailable()) event.preventDefault(); });
    document.addEventListener("pointerdown", (event) => { if (menu.open && !menu.contains(event.target)) menu.open = false; });
    document.addEventListener("keydown", (event) => {
      if (event.key !== "Escape" || !menu.open) return;
      event.preventDefault();
      menu.open = false;
      summary.focus();
    });
    function render(revisions, { current, onSelect, title, detail, meta } = {}) {
      count.textContent = isAvailable() ? `${revisions.length} version${revisions.length === 1 ? "" : "s"}` : "—";
      list.replaceChildren();
      if (!isAvailable()) return;
      if (!revisions.length) {
        const empty = document.createElement("p");
        empty.className = "catalog-empty";
        empty.textContent = "No saved versions.";
        list.append(empty);
        return;
      }
      for (const revision of revisions) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "history-item";
        button.dataset.revision = String(revision.number);
        button.setAttribute("aria-current", String(revision.number === current));
        const strong = document.createElement("strong");
        strong.textContent = title ? title(revision) : `Version ${revision.number}`;
        const span = document.createElement("span");
        span.textContent = detail ? detail(revision) : "";
        const small = document.createElement("small");
        small.textContent = meta ? meta(revision) : "";
        button.append(strong, span, small);
        button.addEventListener("click", () => { menu.open = false; onSelect?.(revision); });
        list.append(button);
      }
    }
    return { render, close() { menu.open = false; } };
  }

  function isNamed(name) {
    return Boolean(name?.trim()) && name.trim().toLowerCase() !== "untitled";
  }

  function fitSourceToViewport(container) {
    const frame = container.closest(".policy-page");
    let scheduled = false;
    function schedule() {
      if (scheduled) return;
      scheduled = true;
      requestAnimationFrame(() => {
        scheduled = false;
        if (!container.getClientRects().length) return;
        const top = container.getBoundingClientRect().top + window.scrollY;
        const main = container.closest(".main-content");
        const pane = container.closest(".policy-editor-pane, .editor-assistant-main") || container.closest(".editor-page-main");
        const padding = Number.parseFloat(getComputedStyle(main).paddingBottom) + Number.parseFloat(getComputedStyle(pane).paddingBottom);
        const inset = Math.min(window.innerHeight * .04, padding);
        const height = Math.max(320, window.innerHeight - inset - top);
        container.style.setProperty("--editor-source-height", `${height}px`);
      });
    }
    const observer = new ResizeObserver(schedule);
    for (const element of [frame, frame.querySelector(".policy-toolbar"), frame.querySelector(".policy-editor-tools"), document.querySelector(".topbar"), ...container.parentElement.children]) {
      if (element) observer.observe(element);
    }
    window.addEventListener("resize", schedule);
    window.addEventListener("hashchange", schedule);
    schedule();
  }

  // History owns version information; the identity row shows only name and dirty state.
  function renderIdentity({ title, version, modified }, { name = "", label = "", tooltip = "", dirty = false } = {}) {
    title.textContent = name.trim() || "Untitled";
    title.hidden = false;
    if (tooltip) title.title = tooltip; else title.removeAttribute("title");
    version.textContent = "";
    version.hidden = true;
    modified.hidden = !dirty;
  }

  // Status lines are hidden whenever they have nothing to say.
  function setStatus(element, text = "", kind = "") {
    element.textContent = text;
    element.hidden = !text;
    if (kind) element.dataset.state = kind; else delete element.dataset.state;
  }

  window.ZPREditorPage = { highlight, createSourceSurface, createMenu, createPicker, createHistory, placeHistory, isNamed, fitSourceToViewport, renderIdentity, setStatus };
})();
