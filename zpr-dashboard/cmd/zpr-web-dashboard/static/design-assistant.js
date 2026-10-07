window.mountSimulatorDesignAssistant = function mountSimulatorDesignAssistant(containerID, options) {
  const container = document.getElementById(containerID);
  container.innerHTML = `<section class="design-assistant"><header class="design-assistant-head"><div><p class="eyebrow">CLAUDE DESIGN ASSISTANT</p><p class="design-assistant-description">Suggestions stay as drafts until you apply and save them.</p></div><span data-assistant-state>Checking</span></header><div class="design-assistant-thread" data-assistant-thread role="log" aria-live="polite"></div><div data-assistant-form><label class="design-assistant-prompt">What are you designing?<textarea data-assistant-question rows="2" maxlength="12000" required placeholder="Describe the scenario or organization change you want."></textarea></label><div class="design-assistant-controls"><label>Model<select data-assistant-model></select></label><label>Reply size<select data-assistant-tokens><option value="600">Short</option><option value="1200" selected>Standard</option><option value="2400">Detailed</option></select></label><button data-assistant-submit type="button">Ask Claude</button><button data-assistant-apply class="quiet" type="button" hidden>${options.applyLabel}</button><button data-assistant-clear class="quiet" type="button">Clear</button></div></div><p class="design-assistant-usage" data-assistant-usage>Session: 0 input · 0 output tokens</p></section>`;
  const thread = container.querySelector("[data-assistant-thread]");
  const form = container.querySelector("[data-assistant-form]");
  const question = container.querySelector("[data-assistant-question]");
  const model = container.querySelector("[data-assistant-model]");
  const maxTokens = container.querySelector("[data-assistant-tokens]");
  const send = container.querySelector("[data-assistant-submit]");
  const apply = container.querySelector("[data-assistant-apply]");
  const clear = container.querySelector("[data-assistant-clear]");
  const status = container.querySelector("[data-assistant-state]");
  const usage = container.querySelector("[data-assistant-usage]");
  const state = { messages: [], proposal: null, ready: false, pending: false, inputTokens: 0, outputTokens: 0 };
  let generation = 0;
  let proposalContext = "";
  let statusError = "";
  const disclosure = document.createElement("p");
  disclosure.className = "design-assistant-description";
  disclosure.textContent = "Asking Claude sends the current organization/scenario context and conversation to Anthropic. Suggestions never save, publish, activate, or run automatically.";
  form.before(disclosure);

  function render() {
    thread.replaceChildren();
    for (const message of state.messages) {
      const entry = document.createElement("article");
      entry.className = `design-assistant-message ${message.role}`;
      const label = document.createElement("strong");
      label.textContent = message.role === "user" ? "YOU" : "CLAUDE";
      const content = document.createElement("p");
      content.textContent = message.content;
      entry.append(label, content);
      thread.append(entry);
    }
    if (!state.messages.length) {
      const empty = document.createElement("p");
      empty.className = "design-assistant-empty";
      empty.textContent = statusError || options.emptyMessage;
      thread.append(empty);
    }
    status.textContent = state.pending ? "Thinking" : state.ready ? "Ready" : statusError ? "Unavailable" : "Not configured";
    send.disabled = !state.ready || state.pending || !question.value.trim();
    question.disabled = !state.ready || state.pending;
    model.disabled = !state.ready || state.pending;
    maxTokens.disabled = !state.ready || state.pending;
    clear.disabled = state.pending || (!state.messages.length && !state.proposal);
    apply.hidden = !state.proposal || state.pending;
    usage.textContent = `Session: ${state.inputTokens.toLocaleString()} input · ${state.outputTokens.toLocaleString()} output tokens`;
  }

  function reset() {
    generation++;
    state.pending = false;
    proposalContext = "";
    state.messages = [];
    state.proposal = null;
    state.inputTokens = 0;
    state.outputTokens = 0;
    render();
  }

  fetch("/api/simulator/assistant/status", { cache: "no-store" })
    .then(async (response) => {
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
      state.ready = Boolean(data.ready);
      for (const name of data.models || []) model.add(new Option(name, name));
      if (data.model) model.value = data.model;
      status.title = state.ready ? `Using ${data.model}` : "Set ANTHROPIC_API_KEY on the simulator server to enable Claude.";
    })
    .catch((error) => {
      state.ready = false;
      status.title = `Claude status unavailable: ${error.message}`;
      statusError = `Assistant status unavailable: ${error.message}`;
    })
    .finally(render);

  send.addEventListener("click", async () => {
    const text = question.value.trim();
    if (!text || !state.ready || state.pending) return;
    state.messages.push({ role: "user", content: text });
    state.messages = state.messages.slice(-20);
    state.proposal = null;
    question.value = "";
    state.pending = true;
    const requestGeneration = generation;
    render();
    try {
      const context = options.getContext();
      const fingerprint = JSON.stringify(context);
      const response = await fetch("/api/simulator/design-assistant", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          scope: options.scope,
          organization_id: context.organization_id,
          scenario: context.scenario,
          messages: state.messages,
          model: model.value,
          max_tokens: Number(maxTokens.value),
        }),
      });
      const result = await response.json();
      if (requestGeneration !== generation) return;
      if (!response.ok) throw new Error(result.error || `Claude request failed (${response.status})`);
      state.messages.push({ role: "assistant", content: result.proposal_error ? `${result.answer}\n\nProposal not applied: ${result.proposal_error}` : result.answer });
      if (JSON.stringify(options.getContext()) !== fingerprint) {
        state.proposal = null;
        state.messages.push({ role: "assistant", content: "The editor changed while Claude was responding. This proposal cannot be applied; ask again using the current draft." });
      } else {
        state.proposal = result.proposal || null;
        proposalContext = fingerprint;
      }
      state.inputTokens += Number(result.input_tokens) || 0;
      state.outputTokens += Number(result.output_tokens) || 0;
    } catch (error) {
      if (requestGeneration !== generation) return;
      state.messages.push({ role: "assistant", content: error.message || "Claude could not complete the request." });
    } finally {
      if (requestGeneration === generation) { state.pending = false; render(); }
    }
  });

  apply.addEventListener("click", async () => {
    if (!state.proposal || state.pending) return;
    try {
      if (JSON.stringify(options.getContext()) !== proposalContext) {
        state.proposal = null;
        throw new Error("The editor changed after this proposal was generated. Ask again using the current draft.");
      }
      await options.onApply(state.proposal);
      state.proposal = null;
      state.messages.push({ role: "assistant", content: "Applied to the editor as an unsaved draft. Review it, then use the editor's Save draft action." });
    } catch (error) {
      state.messages.push({ role: "assistant", content: error.message || "Could not apply this proposal." });
    }
    render();
  });

  clear.addEventListener("click", reset);
  question.addEventListener("input", render);
  question.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") send.click();
  });
  return { reset };
};