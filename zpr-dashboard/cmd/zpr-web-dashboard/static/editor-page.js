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
  function createSourceSurface({ source, highlight: pre, gutter, language, label = "Source", onMarker, markerText = "!" }) {
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
          marker.className = markerText === "ERR" ? "policy-test-line-result" : "config-error-marker";
          if (markerText === "ERR") {
            marker.dataset.effect = "error";
          }
          marker.title = diagnostic.message;
          marker.setAttribute("aria-label", `${typeof label === "function" ? label() : label} error on line ${index + 1}: ${diagnostic.message}`);
          marker.textContent = markerText;
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
    const listeners = new AbortController();
    const options = { signal: listeners.signal };
    const availableItems = () => [...menu.querySelectorAll("button:not(:disabled)")]
      .filter((button) => button.getClientRects().length > 0);
    function setOpen(open, restoreFocus = false) {
      menu.hidden = !open;
      toggle.setAttribute("aria-expanded", String(open));
      if (open) availableItems()[0]?.focus();
      else if (restoreFocus) toggle.focus();
    }
    toggle.addEventListener("click", () => setOpen(menu.hidden), options);
    menu.addEventListener("click", (event) => { if (event.target.closest("button:not(:disabled)")) setOpen(false); }, options);
    menu.addEventListener("keydown", (event) => {
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      const items = availableItems();
      if (!items.length) return;
      event.preventDefault();
      const current = items.indexOf(document.activeElement);
      const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : (current + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
      items[next].focus();
    }, options);
    document.addEventListener("pointerdown", (event) => { if (!menu.hidden && !root.contains(event.target)) setOpen(false); }, options);
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !menu.hidden) { event.preventDefault(); setOpen(false, true); }
    }, options);
    return { setOpen, get open() { return !menu.hidden; }, dispose() { setOpen(false); listeners.abort(); } };
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
    const listeners = new AbortController();
    const options = { signal: listeners.signal };
    placeHistory(menu);
    const summary = menu.querySelector("summary");
    summary.addEventListener("click", (event) => { if (!isAvailable()) event.preventDefault(); }, options);
    document.addEventListener("pointerdown", (event) => { if (menu.open && !menu.contains(event.target)) menu.open = false; }, options);
    document.addEventListener("keydown", (event) => {
      if (event.key !== "Escape" || !menu.open) return;
      event.preventDefault();
      menu.open = false;
      summary.focus();
    }, options);
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
        button.addEventListener("click", () => { menu.open = false; onSelect?.(revision); }, options);
        list.append(button);
      }
    }
    return { render, close() { menu.open = false; }, dispose() { menu.open = false; listeners.abort(); } };
  }

  function isNamed(name) {
    return Boolean(name?.trim()) && name.trim().toLowerCase() !== "untitled";
  }

  function confirmDiscard(dirty, message) {
    return !dirty || window.confirm(message);
  }

  function setAnalysisState(button, state = "") {
    if (!["", "pending", "success", "warning", "error"].includes(state)) {
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

  const editorControllers = new WeakMap();

  function createController({ source, root = source.closest(".policy-page"), readContext = () => [source.value],
    isDirty = () => false, identity, status, statusInFrame = true, menu, history, viewport = source.closest(".config-source-editor"),
    analyzeButton = root.querySelector('[id$="-analyze"], #policy-check, #zpr-config-validate'),
    adapters = {}, syncControls = () => {} }) {
    if (editorControllers.has(source)) throw new Error(`Editor already registered: ${source.id}`);
    const scopes = new Map();
    const active = new Map();
    const cleanups = [];
    let disposed = false;
    if (status && statusInFrame) placeStatus(status);
    const files = menu ? createMenu(menu) : null;
    const versions = history ? createHistory(history) : null;
    if (files) cleanups.push(files.dispose);
    if (versions) cleanups.push(versions.dispose);
    const layout = viewport ? fitSourceToViewport(viewport) : null;
    if (layout) {
      root.dataset.editorViewport = "true";
      viewport.dataset.editorSourceViewport = "true";
      const observer = new MutationObserver(layout.schedule);
      observer.observe(root, { attributes: true, attributeFilter: ["open", "hidden", "class"], subtree: true });
      root.addEventListener("toggle", layout.schedule, true);
      cleanups.push(() => { observer.disconnect(); root.removeEventListener("toggle", layout.schedule, true); layout.dispose(); });
    }
    const controller = {
      files,
      history: versions,
      get dirty() { return isDirty(); },
      get pending() { return active.size > 0; },
      renderIdentity(value, elements = identity) {
        if (!elements) throw new Error(`Editor identity not configured: ${source.id}`);
        renderIdentity(elements, { ...value, dirty: value?.dirty ?? isDirty() });
      },
      setStatus(text = "", kind = "") {
        if (!status) throw new Error(`Editor status not configured: ${source.id}`);
        setStatus(status, text, kind);
      },
      setAnalysisState(state = "") {
        if (!analyzeButton) throw new Error(`Editor has no Analyze action: ${source.id}`);
        setAnalysisState(analyzeButton, state);
      },
      confirmDiscard(message, dirty = isDirty()) { return confirmDiscard(dirty, message); },
      createScope(name, context = readContext) {
        if (scopes.has(name)) return scopes.get(name);
        const scope = createAnalysisScope(context);
        const registered = {
          begin() {
            active.delete(name);
            return scope.begin();
          },
          invalidate() {
            scope.invalidate();
            if (active.delete(name) && !active.size && analyzeButton.dataset.analysisState === "pending") controller.setAnalysisState();
          },
        };
        scopes.set(name, registered);
        return registered;
      },
      beginAnalysis(name = "analyze") {
        if (disposed) throw new Error(`Editor is disposed: ${source.id}`);
        const isCurrent = controller.createScope(name).begin();
        active.set(name, isCurrent);
        controller.setAnalysisState("pending");
        isCurrent.finish = () => {
          if (active.get(name) !== isCurrent) return;
          active.delete(name);
          if (!active.size && analyzeButton.dataset.analysisState === "pending") controller.setAnalysisState();
          syncControls();
        };
        return isCurrent;
      },
      perform(command, ...args) {
        if (disposed) throw new Error(`Editor is disposed: ${source.id}`);
        if (typeof adapters[command] !== "function") throw new Error(`Editor command not configured: ${command}`);
        return adapters[command](...args);
      },
      bind(command, { button, shortcutRoot, onError = error => controller.setStatus(error.message, "error") }) {
        const click = () => {
          Promise.resolve().then(() => controller.perform(command)).catch(onError);
        };
        button.addEventListener("click", click);
        cleanups.push(() => button.removeEventListener("click", click));
        if (shortcutRoot) cleanups.push(bindSaveShortcut({ root: shortcutRoot, button }));
      },
      dispose() {
        if (disposed) return;
        disposed = true;
        for (const scope of scopes.values()) scope.invalidate();
        for (const cleanup of cleanups) cleanup();
        editorControllers.delete(source);
      },
    };
    editorControllers.set(source, controller);
    return controller;
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
        if (!container.getClientRects().length || frame.closest(".editor-page-maximized")) return;
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

  function bindEditorMaximize() {
    const registrations = [...document.querySelectorAll("[data-editor-maximize]")].map((button) => {
      const pane = document.getElementById(button.dataset.editorMaximize);
      if (!pane) throw new Error(`Editor maximize target not found: ${button.dataset.editorMaximize}`);
      return { button, pane };
    });
    for (const editor of document.querySelectorAll(".editor-page")) {
      const utilities = editor.querySelector(".policy-editor-utilities");
      if (!utilities || utilities.querySelector("[data-editor-maximize]")) continue;
      const pane = editor.closest(".page-view") || editor;
      if (!pane.id) throw new Error("Editor maximize pane requires an id.");
      const button = document.createElement("button");
      button.id = `editor-maximize-${pane.id}`;
      button.className = "button button-refresh";
      button.type = "button";
      button.dataset.editorMaximize = pane.id;
      button.setAttribute("aria-label", "Maximize editor");
      button.setAttribute("aria-pressed", "false");
      button.title = "Maximize editor";
      button.textContent = "Maximize";
      utilities.append(button);
      registrations.push({ button, pane });
    }

    for (const { button, pane } of registrations) {
      const isDialog = pane instanceof HTMLDialogElement;
      const originalRole = pane.getAttribute("role");
      const originalModal = pane.getAttribute("aria-modal");
      let modalDialog = false;
      let inertSiblings = [];
      let maximized = false;

      function setMaximized(next, restoreFocus = false) {
        if (maximized === next) {
          if (restoreFocus) button.focus();
          return;
        }
        maximized = next;
        pane.classList.toggle("editor-page-maximized", maximized);
        document.body.classList.toggle("editor-page-maximized", maximized);
        button.setAttribute("aria-pressed", String(maximized));
        button.textContent = maximized ? "Restore" : "Maximize";
        button.setAttribute("aria-label", maximized ? "Restore editor" : "Maximize editor");
        button.title = maximized ? "Restore editor" : "Maximize editor";

        if (maximized) {
          modalDialog = isDialog && pane.matches(":modal");
          if (!modalDialog) {
            pane.setAttribute("role", "dialog");
            pane.setAttribute("aria-modal", "true");
            inertSiblings = [];
            let current = pane;
            while (current !== document.body) {
              const parent = current.parentElement;
              for (const sibling of parent.children) {
                if (sibling === current) continue;
                inertSiblings.push([sibling, sibling.inert]);
                sibling.inert = true;
              }
              current = parent;
            }
          }
          button.focus();
          return;
        }

        if (!modalDialog) {
          if (originalRole === null) pane.removeAttribute("role");
          else pane.setAttribute("role", originalRole);
          if (originalModal === null) pane.removeAttribute("aria-modal");
          else pane.setAttribute("aria-modal", originalModal);
          for (const [element, wasInert] of inertSiblings) element.inert = wasInert;
          inertSiblings = [];
        }
        if (restoreFocus) button.focus();
      }

      button.addEventListener("click", () => setMaximized(!maximized));
      if (isDialog) {
        pane.addEventListener("cancel", (event) => {
          if (!maximized || !modalDialog) return;
          event.preventDefault();
          setMaximized(false, true);
        });
        pane.addEventListener("close", () => setMaximized(false));
      }
      document.addEventListener("keydown", (event) => {
        if (!maximized || event.defaultPrevented) return;
        if (event.key === "Escape" && !modalDialog &&
            !document.querySelector("dialog:modal") &&
            !pane.querySelector('[role="menu"]:not([hidden])') &&
            !pane.querySelector(".policy-picker-menu:not([hidden])") &&
            !pane.querySelector(".policy-catalog-pane:not([hidden])")) {
          event.preventDefault();
          setMaximized(false, true);
          return;
        }
        if (event.key !== "Tab") return;
        const focusable = [...pane.querySelectorAll(
          'a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), summary, [tabindex]:not([tabindex="-1"])',
        )].filter((element) => element.getClientRects().length && !element.closest("[hidden]"));
        const first = focusable[0];
        const last = focusable.at(-1);
        if (event.shiftKey && (document.activeElement === first || !pane.contains(document.activeElement))) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      });
    }
  }

  bindEditorMaximize();

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

  window.ZPREditorPage = { highlight, sourceLine, placeStatus, requestJSON, createAnalysisScope, createController, getController: source => editorControllers.get(source), bindSourceLayout, createSourceSurface, createMenu, createPicker, createHistory, placeHistory, isNamed, confirmDiscard, setAnalysisState, bindSaveShortcut, fitSourceToViewport, renderIdentity, setStatus };
})();
