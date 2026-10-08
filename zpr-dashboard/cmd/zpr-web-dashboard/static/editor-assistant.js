(() => {
  function mountEditorAssistant(options) {
    const source = options.source;
    const anchor = options.anchor;
    if (!source || !anchor || anchor.closest(".editor-assistant-layout")) return null;
    const id = `editor-assistant-${options.editor}`;
    const layout = document.createElement("div");
    layout.className = "editor-assistant-layout";
    const main = document.createElement("div");
    main.className = "editor-assistant-main";
    anchor.before(layout);
    main.append(anchor);
    const pane = document.createElement("aside");
    pane.className = "policy-assistant-pane editor-assistant-pane";
    pane.id = `${id}-pane`;
    pane.dataset.editorAssistant = options.editor;
    pane.setAttribute("aria-label", "AI Assistant");
    pane.innerHTML = `<div class="assistant-heading"><h3>AI Assistant</h3><span class="assistant-state">Checking</span><button class="pane-collapse-toggle editor-assistant-toggle" type="button" aria-controls="${id}-messages" aria-expanded="true" aria-label="Collapse AI Assistant"><span class="pane-toggle-icon" aria-hidden="true"></span><span class="pane-toggle-label" aria-hidden="true">AI Assistant</span></button></div>`;
    layout.append(main, pane);
    const context = () => ({
      source: source.value,
      record: options.readContext(),
      identity: document.getElementById(options.identityID)?.textContent || "",
      revision: document.getElementById(options.revisionID)?.textContent || "",
    });
    const controller = window.ZPRAssistant.mount({
      pane, prefix: id, getContext: context, getTarget: () => source,
      editable: () => !source.disabled && !source.readOnly,
      disclosure: options.disclosure,
      request: (conversation) => window.ZPRAssistant.jsonRequest(options.askURL, {
        method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ ...conversation, editor: options.editor, source: source.value }),
      }),
      watch(invalidate, reset) {
        let observed = JSON.stringify(context());
        const owner = () => JSON.stringify(options.readContext().slice(0, options.identityFields));
        let observedOwner = owner();
        const observeContext = (force = false) => {
          const current = JSON.stringify(context());
          const currentOwner = owner();
          if (currentOwner !== observedOwner) {
            observedOwner = currentOwner;
            observed = current;
            reset();
          } else if (force || current !== observed) {
            observed = current;
            invalidate();
          }
        };
        source.addEventListener("input", () => observeContext(true));
        new MutationObserver(() => { observeContext(); controller.render(); }).observe(source, { attributes: true, attributeFilter: ["disabled", "readonly"] });
        for (const identity of [options.identityID, options.revisionID]) {
          const element = document.getElementById(identity);
          if (element) new MutationObserver(() => observeContext()).observe(element, { childList: true, subtree: true, characterData: true });
        }
      },
    });
    window.ZPRAssistant.bindCollapse({
      pane, toggle: pane.querySelector(".editor-assistant-toggle"), layout,
      storageKey: `zpr-editor-assistant-${options.editor}-collapsed`,
    });
    async function loadStatus() {
      try { controller.setStatus(await window.ZPRAssistant.jsonRequest(options.statusURL, { cache: "no-store" })); }
      catch (error) { controller.statusError(error); }
    }
    loadStatus();
    return { pane, reset: controller.reset, refresh: loadStatus };
  }
  window.mountEditorAssistant = mountEditorAssistant;

  for (const options of [
    { editor: "gateway", sourceID: "gateway-source", identityID: "gateway-record-title", revisionID: "gateway-revision-label", statusURL: "/api/gateways/assistant", askURL: "/api/gateways/assistant",
      readContext: () => window.getGatewayAssistantContext(),
      identityFields: 4,
      disclosure: "Submitting sends the current gateway draft and chat history to Anthropic. Suggestions are never saved or activated automatically." },
    { editor: "zpr-config", sourceID: "zpr-config-source", identityID: "zpr-config-title", revisionID: "zpr-config-revision-label", statusURL: "/api/policy", askURL: "/api/policy/assistant",
      readContext: () => window.getConfigAssistantContext(),
      identityFields: 2,
      disclosure: "Submitting sends the current configuration draft and chat history to Anthropic. Suggestions are never saved or applied automatically." },
    { editor: "directory-ldif", sourceID: "directory-editor-source", identityID: "directory-editor-title", revisionID: "directory-editor-revision-label", statusURL: "/api/simulator/assistant/status", askURL: "/api/simulator/editor-assistant",
      readContext: () => window.getDirectoryAssistantContext(),
      identityFields: 2,
      disclosure: "Submitting sends the current LDIF draft and chat history to Anthropic. Suggestions are never saved or published automatically." },
  ]) {
    const source = document.getElementById(options.sourceID);
    if (!source) continue;
    const assistant = mountEditorAssistant({
      ...options, source, anchor: source.closest(".editor-page-main") || source.closest(".config-source-editor") || source.closest(".directory-editor-field"),
    });
    source.closest("dialog")?.addEventListener("close", () => assistant?.reset());
  }
})();
