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

  function sourceLine(value, lineCount) {
    if (typeof value !== "number" && typeof value !== "string") return null;
    const line = Number(value);
    return Number.isInteger(line) && line > 0 && line <= lineCount ? line : null;
  }

  function placeStatus(element) {
    element.closest(".policy-page").querySelector(".policy-editor-tools").after(element);
  }

  async function requestJSON(fetcher, url, options = {}, { acceptError = () => false } = {}) {
    const response = await fetcher(url, { cache: "no-store", ...options });
    let result;
    try {
      result = await response.json();
    } catch (error) {
      if (!(error instanceof SyntaxError)) throw error;
      throw new Error(`HTTP ${response.status}: invalid JSON response`, { cause: error });
    }
    if (!response.ok && !acceptError(result)) {
      const error = new Error(result?.error || result?.diagnostics || `HTTP ${response.status}`);
      error.line = result?.line || 0;
      error.details = result;
      throw error;
    }
    return result;
  }

  function bindSourceLayout({ source, highlight: pre, gutter, gutterContent, container, onScroll, onResize }) {
    let disposed = false;
    function syncScroll() {
      if (disposed) return;
      pre.scrollTop = source.scrollTop;
      pre.scrollLeft = source.scrollLeft;
      if (gutterContent) gutterContent.style.transform = `translateY(${-source.scrollTop}px)`;
      else gutter.scrollTop = source.scrollTop;
      if (container) container.dataset.horizontalOverflow = String(source.scrollWidth > source.clientWidth);
    }
    function scroll() {
      syncScroll();
      onScroll?.();
    }
    const observer = window.ResizeObserver ? new ResizeObserver(() => {
      if (disposed) return;
      syncScroll();
      onResize?.();
    }) : null;
    source.addEventListener("scroll", scroll);
    observer?.observe(source);
    return {
      syncScroll,
      dispose() {
        if (disposed) return;
        disposed = true;
        source.removeEventListener("scroll", scroll);
        observer?.disconnect();
      },
    };
  }

  // Wires a textarea, highlight overlay and diagnostic gutter into one code surface.
  function createSourceSurface({ source, highlight: pre, gutter, language, label = "Source", onMarker }) {
    let diagnostic = null;
    const container = source.closest(".config-source-editor");
    const languageName = () => typeof language === "function" ? language() : language;
    const layout = bindSourceLayout({ source, highlight: pre, gutter, container });
    const syncScroll = layout.syncScroll;
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
    return {
      render,
      syncScroll,
      selectLine,
      dispose: layout.dispose,
      get diagnostic() { return diagnostic; },
      setDiagnostic(next) {
        const line = sourceLine(next?.line, source.value.split("\n").length);
        diagnostic = line !== null
          ? { line, message: String(next.message || "") } : null;
      },
    };
  }

  // Policy-style button menu (File...): toggle button + role=menu popover, Escape and outside click close it.
  function createMenu({ root, toggle, menu }) {
    const availableItems = () => [...menu.querySelectorAll("button:not(:disabled)")]
      .filter((button) => button.getClientRects().length > 0);
    function setOpen(open, restoreFocus = false) {
      menu.hidden = !open;
      toggle.setAttribute("aria-expanded", String(open));
      if (open) availableItems()[0]?.focus();
      else if (restoreFocus) toggle.focus();
    }
    toggle.addEventListener("click", () => setOpen(menu.hidden));
    menu.addEventListener("click", (event) => { if (event.target.closest("button:not(:disabled)")) setOpen(false); });
    menu.addEventListener("keydown", (event) => {
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      const items = availableItems();
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

  function confirmDiscard(dirty, message) {
    return !dirty || window.confirm(message);
  }

  function setAnalysisState(button, state = "") {
    if (!["", "pending", "success", "error"].includes(state)) {
      throw new Error(`Unsupported editor analysis state: ${state}`);
    }
    if (state) button.dataset.analysisState = state;
    else delete button.dataset.analysisState;
  }

  // Context adapters return primitive identity fields, never mutable record objects.
  function createAnalysisScope(readContext) {
    let generation = 0;
    return {
      invalidate() { generation++; },
      begin() {
        const current = ++generation;
        const context = [...readContext()];
        return () => {
          const next = readContext();
          return current === generation && context.length === next.length &&
            context.every((value, index) => value === next[index]);
        };
      },
    };
  }

  function bindSaveShortcut({ root, button }) {
    const onKeydown = (event) => {
      if (event.defaultPrevented || event.isComposing || event.repeat ||
          !(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey ||
          event.key.toLowerCase() !== "s") return;
      event.preventDefault();
      if (!button.disabled) button.click();
    };
    root.addEventListener("keydown", onKeydown);
    return () => root.removeEventListener("keydown", onKeydown);
  }

  const viewportBindings = new WeakMap();

  function fitSourceToViewport(container) {
    const existing = viewportBindings.get(container);
    if (existing) return existing;
    const frame = container.closest(".policy-page");
    const originalHeight = container.style.getPropertyValue("--editor-source-height");
    const originalPriority = container.style.getPropertyPriority("--editor-source-height");
    let animationFrame = null;
    let disposed = false;
    function schedule() {
      if (disposed || animationFrame !== null) return;
      animationFrame = requestAnimationFrame(() => {
        animationFrame = null;
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
    const binding = {
      schedule,
      dispose() {
        if (disposed) return;
        disposed = true;
        if (animationFrame !== null) cancelAnimationFrame(animationFrame);
        animationFrame = null;
        observer.disconnect();
        window.removeEventListener("resize", schedule);
        window.removeEventListener("hashchange", schedule);
        if (originalHeight) container.style.setProperty("--editor-source-height", originalHeight, originalPriority);
        else container.style.removeProperty("--editor-source-height");
        viewportBindings.delete(container);
      },
    };
    viewportBindings.set(container, binding);
    schedule();
    return binding;
  }

  // History owns version information; the identity row shows only name and dirty state.
  function renderIdentity({ title, version, modified }, { name = "", label = "", tooltip = "", dirty = false } = {}) {
    title.textContent = name.trim() || "Untitled";
    title.hidden = false;
    if (tooltip) title.title = tooltip; else title.removeAttribute("title");
    version.textContent = "";
    version.hidden = true;
    version.removeAttribute("title");
    modified.hidden = !dirty;
  }

  // Status lines are hidden whenever they have nothing to say.
  function setStatus(element, text = "", kind = "") {
    element.textContent = text;
    element.hidden = !text;
    if (kind) element.dataset.state = kind; else delete element.dataset.state;
  }

  window.ZPREditorPage = { highlight, sourceLine, placeStatus, requestJSON, createAnalysisScope, bindSourceLayout, createSourceSurface, createMenu, createPicker, createHistory, placeHistory, isNamed, confirmDiscard, setAnalysisState, bindSaveShortcut, fitSourceToViewport, renderIdentity, setStatus };
})();
