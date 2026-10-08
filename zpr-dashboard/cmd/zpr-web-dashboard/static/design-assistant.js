window.mountSimulatorDesignAssistant = function mountSimulatorDesignAssistant(containerID, options) {
  const container = document.getElementById(containerID);
  const pane = document.createElement("section");
  pane.className = "design-assistant";
  pane.setAttribute("aria-label", "AI Assistant");
  pane.innerHTML = `<header class="assistant-heading design-assistant-head"><h3>AI Assistant</h3><span class="assistant-state" data-assistant-state>Checking</span><button class="button" type="button" data-assistant-collapse aria-label="Collapse AI Assistant" aria-expanded="true" aria-controls="${containerID}-messages">AI Assistant</button></header>`;
  container.append(pane);
  const controller = window.ZPRAssistant.mount({
    ...options, pane, prefix: containerID,
    unavailableLabel: "Unavailable",
    disclosure: "Asking Claude sends the current organization/scenario context and conversation to Anthropic. Suggestions never save, publish, activate or run automatically.",
    request: (conversation) => {
      const context = options.getContext();
      return window.ZPRAssistant.jsonRequest("/api/simulator/design-assistant", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...conversation, scope: options.scope, organization_id: context.organization_id, scenario: context.scenario }),
      });
    },
    watch(invalidate) {
      const editor = document.getElementById(options.editorID);
      if (editor) {
        editor.addEventListener("input", (event) => { if (!event.target.closest(".design-assistant")) invalidate(); });
        editor.addEventListener("change", (event) => { if (!event.target.closest(".design-assistant")) invalidate(); });
      }
    },
    apply: options.onApply,
  });
  window.ZPRAssistant.bindCollapse({
    pane, toggle: pane.querySelector("[data-assistant-collapse]"),
    storageKey: `zpr-design-assistant-${options.scope}-collapsed`,
  });
  window.ZPRAssistant.jsonRequest("/api/simulator/assistant/status", { cache: "no-store" })
    .then(controller.setStatus).catch(controller.statusError);
  return { reset: controller.reset };
};
