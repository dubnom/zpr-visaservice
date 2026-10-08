(() => {
  const tokenOptions = [[300, "300 tokens"], [600, "600 tokens"], [1200, "1,200 tokens"], [2400, "2,400 tokens"]];
  const numberFormat = new Intl.NumberFormat();

  function insertIntoEditor(target, text) {
    if (!target || target.disabled || target.readOnly) return false;
    target.focus();
    // execCommand keeps the insertion on the textarea's native undo stack and fires input.
    if (!document.execCommand("insertText", false, text)) {
      const start = target.selectionStart;
      target.setRangeText(text, start, target.selectionEnd, "end");
      target.dispatchEvent(new Event("input", { bubbles: true }));
    }
    return true;
  }

  // Renders assistant text with fenced code blocks as code plus an Insert action.
  function assistantContent(text, getTarget) {
    const fragment = document.createDocumentFragment();
    const fence = /```[^\n`]*\n([\s\S]*?)```/g;
    let last = 0;
    const appendText = (value) => {
      if (!value.trim()) return;
      const pre = document.createElement("pre");
      pre.textContent = value.replace(/^\n+|\n+$/g, "");
      fragment.append(pre);
    };
    for (const match of String(text).matchAll(fence)) {
      appendText(text.slice(last, match.index));
      last = match.index + match[0].length;
      const block = document.createElement("div");
      block.className = "assistant-code-block";
      const code = document.createElement("pre");
      code.className = "assistant-code";
      code.textContent = match[1].replace(/\n$/, "");
      block.append(code);
      if (getTarget) {
        const insert = document.createElement("button");
        insert.type = "button";
        insert.className = "button assistant-insert";
        insert.textContent = "Insert";
        insert.title = "Insert at the cursor (replaces the selection). Undo with Ctrl+Z.";
        insert.addEventListener("click", () => {
          const target = getTarget();
          const inserted = insertIntoEditor(target, code.textContent);
          insert.textContent = inserted ? "Inserted" : "Editor is read-only";
          setTimeout(() => { insert.textContent = "Insert"; }, 1600);
        });
        block.append(insert);
      }
      fragment.append(block);
    }
    appendText(text.slice(last));
    if (!fragment.childNodes.length) appendText(String(text) || " ");
    return fragment;
  }

  window.editorAssistantContent = assistantContent;

  function normalizeStatus(data) {
    const ready = Boolean(data?.ready ?? data?.assistant_ready);
    const model = data?.model ?? data?.assistant_model ?? "";
    const models = data?.models ?? data?.assistant_models ?? [];
    return { ready, model, models: Array.isArray(models) ? models : [] };
  }

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
    pane.innerHTML = `<div class="assistant-heading"><h3>AI Assistant</h3><span class="assistant-state">Checking</span><button class="pane-collapse-toggle editor-assistant-toggle" type="button" aria-controls="${id}-messages" aria-expanded="true" aria-label="Collapse AI Assistant" title="Collapse AI Assistant"><span class="pane-toggle-icon" aria-hidden="true"></span><span class="pane-toggle-label" aria-hidden="true">AI Assistant</span></button></div>
      <div class="assistant-settings">
        <label class="assistant-enable" for="${id}-enabled"><input id="${id}-enabled" type="checkbox" disabled> Use assistant</label>
        <label for="${id}-model">Model<select id="${id}-model" disabled></select></label>
        <label for="${id}-max-tokens">Max output<select id="${id}-max-tokens" disabled>${tokenOptions.map(([value, label]) => `<option value="${value}"${value === 1200 ? " selected" : ""}>${label}</option>`).join("")}</select></label>
        <span class="assistant-usage" role="status">Session: 0 input · 0 output tokens</span>
      </div>
      <p class="assistant-disclosure">Checking whether Claude is configured…</p>
      <div id="${id}-messages" class="assistant-messages" role="log" aria-live="polite"></div>
      <form class="assistant-form"><label for="${id}-question">Message</label><textarea id="${id}-question" rows="3" maxlength="12000" disabled></textarea><button class="button button-refresh" type="submit" disabled>Send</button></form>`;
    layout.append(main, pane);

    const stateLabel = pane.querySelector(".assistant-state");
    const toggle = pane.querySelector(".editor-assistant-toggle");
    const enabledInput = pane.querySelector(`#${id}-enabled`);
    const modelSelect = pane.querySelector(`#${id}-model`);
    const tokenSelect = pane.querySelector(`#${id}-max-tokens`);
    const usageLabel = pane.querySelector(".assistant-usage");
    const disclosure = pane.querySelector(".assistant-disclosure");
    const list = pane.querySelector(".assistant-messages");
    const form = pane.querySelector(".assistant-form");
    const question = pane.querySelector(`#${id}-question`);
    const send = form.querySelector("button[type=submit]");
    question.placeholder = options.placeholder;
    const state = { ready: false, enabled: false, pending: false, error: "", messages: [], usage: { input: 0, output: 0 } };
    const storageKey = `zpr-editor-assistant-${options.editor}-collapsed`;

    function setCollapsed(collapsed) {
      layout.dataset.assistantCollapsed = String(collapsed);
      pane.dataset.collapsed = String(collapsed);
      toggle.setAttribute("aria-expanded", String(!collapsed));
      toggle.setAttribute("aria-label", `${collapsed ? "Expand" : "Collapse"} AI Assistant`);
      toggle.title = `${collapsed ? "Expand" : "Collapse"} AI Assistant`;
      try { localStorage.setItem(storageKey, String(collapsed)); }
      catch { /* The pane still works without browser storage. */ }
    }

    function updateControls() {
      const enabled = state.ready && state.enabled;
      const editable = !source.disabled;
      enabledInput.disabled = !state.ready;
      enabledInput.checked = enabled;
      modelSelect.disabled = !enabled;
      tokenSelect.disabled = !enabled;
      question.disabled = !enabled || !editable;
      send.disabled = !enabled || !editable || state.pending;
      usageLabel.textContent = `Session: ${numberFormat.format(state.usage.input)} input · ${numberFormat.format(state.usage.output)} output tokens`;
    }

    function render() {
      list.replaceChildren();
      if (!state.messages.length && !state.error) {
        const empty = document.createElement("p");
        empty.className = "assistant-empty";
        empty.textContent = "No conversation yet.";
        list.append(empty);
      }
      for (const message of state.messages) {
        const entry = document.createElement("article");
        entry.className = `assistant-message ${message.role}`;
        const label = document.createElement("strong");
        label.textContent = message.role === "user" ? "YOU" : "CLAUDE";
        entry.append(label, message.role === "assistant" ? assistantContent(message.content, () => source) : assistantContent(message.content, null));
        list.append(entry);
      }
      if (state.error) {
        const error = document.createElement("p");
        error.className = "assistant-error";
        error.textContent = state.error;
        list.append(error);
      }
      if (state.pending) {
        const pending = document.createElement("p");
        pending.className = "assistant-pending";
        pending.textContent = "Claude is responding…";
        list.append(pending);
      }
      list.scrollTop = list.scrollHeight;
    }

    async function ask(text) {
      state.messages = state.messages.slice(-18);
      state.messages.push({ role: "user", content: text });
      state.pending = true;
      state.error = "";
      updateControls();
      render();
      try {
        const response = await (window.zprOperatorFetch ?? fetch)(options.askURL, {
          method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: JSON.stringify({ editor: options.editor, source: source.value, messages: state.messages, model: modelSelect.value, max_tokens: Number(tokenSelect.value) }),
        });
        const result = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(result.error || `Claude request failed (${response.status})`);
        state.messages.push({ role: "assistant", content: result.answer });
        state.usage.input += Number(result.input_tokens) || 0;
        state.usage.output += Number(result.output_tokens) || 0;
      } catch (error) {
        state.error = error.message;
      } finally {
        state.pending = false;
        updateControls();
        render();
      }
    }

    async function loadStatus() {
      try {
        const response = await fetch(options.statusURL, { cache: "no-store", headers: { Accept: "application/json" } });
        const data = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
        const status = normalizeStatus(data);
        state.ready = status.ready;
        const selected = modelSelect.value || status.model;
        modelSelect.replaceChildren(...status.models.map((model) => new Option(model, model)));
        modelSelect.value = status.models.includes(selected) ? selected : status.model;
        stateLabel.textContent = state.ready ? "Ready" : "Not configured";
        disclosure.textContent = state.ready ? options.disclosure : "Claude is off. Set ANTHROPIC_API_KEY on the server to enable it.";
      } catch (error) {
        state.ready = false;
        stateLabel.textContent = "Unavailable";
        disclosure.textContent = `Assistant status unavailable: ${error.message}`;
      }
      stateLabel.classList.toggle("ready", state.ready);
      updateControls();
    }

    toggle.addEventListener("click", () => setCollapsed(toggle.getAttribute("aria-expanded") === "true"));
    enabledInput.addEventListener("change", () => { state.enabled = enabledInput.checked; updateControls(); });
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      const text = question.value.trim();
      if (!text || !state.ready || !state.enabled || state.pending || source.disabled) return;
      question.value = "";
      ask(text);
    });
    question.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        form.requestSubmit();
      }
    });
    new MutationObserver(updateControls).observe(source, { attributes: true, attributeFilter: ["disabled"] });
    let collapsed = false;
    try { collapsed = localStorage.getItem(storageKey) === "true"; }
    catch { collapsed = false; }
    setCollapsed(collapsed);
    render();
    updateControls();
    loadStatus();
    return {
      pane,
      reset() { state.messages = []; state.error = ""; state.usage = { input: 0, output: 0 }; render(); updateControls(); },
      refresh: loadStatus,
    };
  }

  window.mountEditorAssistant = mountEditorAssistant;

  const gatewaySource = document.getElementById("gateway-source");
  if (gatewaySource) {
    mountEditorAssistant({
      editor: "gateway", source: gatewaySource, anchor: gatewaySource.closest(".editor-page-main") || gatewaySource.closest(".config-source-editor"),
      statusURL: "/api/gateways/assistant", askURL: "/api/gateways/assistant",
      placeholder: "Ask about this gateway draft",
      disclosure: "Submitting sends the current gateway draft and chat history to Anthropic. Suggestions are never saved or activated automatically.",
    });
  }
  const configSource = document.getElementById("zpr-config-source");
  if (configSource) {
    mountEditorAssistant({
      editor: "zpr-config", source: configSource, anchor: configSource.closest(".editor-page-main") || configSource.closest(".config-source-editor"),
      statusURL: "/api/policy", askURL: "/api/policy/assistant",
      placeholder: "Ask about this configuration",
      disclosure: "Submitting sends the current configuration draft and chat history to Anthropic. Suggestions are never saved or applied automatically.",
    });
  }
  const directorySource = document.getElementById("directory-editor-source");
  if (directorySource) {
    const directoryAssistant = mountEditorAssistant({
      editor: "directory-ldif", source: directorySource, anchor: directorySource.closest(".editor-page-main") || directorySource.closest(".directory-editor-field"),
      statusURL: "/api/simulator/assistant/status", askURL: "/api/simulator/editor-assistant",
      placeholder: "Ask about this directory seed",
      disclosure: "Submitting sends the current LDIF draft and chat history to Anthropic. Suggestions are never saved or published automatically.",
    });
    directorySource.closest("dialog")?.addEventListener("close", () => directoryAssistant?.reset());
  }
})();
