(() => {
  const fallbackEdits = new WeakMap();

  function editText(source, text, replaceAll = false) {
    if (!source || source.disabled || source.readOnly) throw new Error("This editor is read-only.");
    const before = source.value;
    const start = replaceAll ? 0 : source.selectionStart;
    const end = replaceAll ? before.length : source.selectionEnd;
    const after = before.slice(0, start) + text + before.slice(end);
    if (source.maxLength >= 0 && after.length > source.maxLength) throw new Error("The suggestion exceeds the editor size limit.");
    source.focus();
    source.setSelectionRange(start, end);
    const native = document.activeElement === source && typeof document.execCommand === "function" && document.execCommand(text ? "insertText" : "delete", false, text);
    if (native && source.value === after) return;
    let history = fallbackEdits.get(source);
    if (!history) {
      history = { undo: [], redo: [], editing: false, value: before };
      fallbackEdits.set(source, history);
      source.addEventListener("input", (event) => {
        if (!history.editing) {
          if (/^(insert|delete)/.test(event.inputType || "") && history.value !== source.value) {
            history.undo.push({ before: history.value, after: source.value });
          } else {
            history.undo = [];
          }
          history.redo = [];
        }
        history.value = source.value;
      });
      source.addEventListener("keydown", (event) => {
        if (!(event.ctrlKey || event.metaKey) || event.altKey || event.isComposing || source.disabled || source.readOnly) return;
        const key = event.key.toLowerCase();
        const redo = key === "y" || (key === "z" && event.shiftKey);
        if (key !== "z" && key !== "y") return;
        const from = redo ? history.redo : history.undo;
        const to = redo ? history.undo : history.redo;
        const entry = from.at(-1);
        if (!entry || source.value !== (redo ? entry.before : entry.after)) return;
        event.preventDefault();
        history.editing = true;
        source.value = redo ? entry.after : entry.before;
        source.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: redo ? "historyRedo" : "historyUndo" }));
        history.editing = false;
        to.push(from.pop());
      });
    }
    if (history.value !== before) { history.undo = []; history.redo = []; }
    history.editing = true;
    source.value = after;
    source.setSelectionRange(start + text.length, start + text.length);
    source.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertReplacementText" }));
    history.editing = false;
    history.undo.push({ before, after });
    history.redo = [];
  }

  function content(text, insert) {
    const fragment = document.createDocumentFragment();
    const value = String(text);
    let last = 0;
    const append = (part) => {
      if (!part.trim()) return;
      const pre = document.createElement("pre");
      pre.textContent = part.replace(/^\n+|\n+$/g, "");
      fragment.append(pre);
    };
    for (const match of value.matchAll(/```[^\n`]*\n([\s\S]*?)```/g)) {
      append(value.slice(last, match.index));
      last = match.index + match[0].length;
      const block = document.createElement("div");
      block.className = "assistant-code-block";
      const code = document.createElement("pre");
      code.className = "assistant-code";
      code.textContent = match[1].replace(/\n$/, "");
      block.append(code);
      if (insert) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "button assistant-insert";
        button.textContent = "Insert";
        button.title = "Insert as an unsaved edit. Undo with Ctrl/Command+Z.";
        button.addEventListener("click", () => insert(code.textContent));
        block.append(button);
      }
      fragment.append(block);
    }
    append(value.slice(last));
    return fragment;
  }

  function bindCollapse({ pane, toggle, layout, storageKey, changed = () => {} }) {
    function setCollapsed(collapsed) {
      pane.dataset.collapsed = String(collapsed);
      if (layout) layout.dataset.assistantCollapsed = String(collapsed);
      toggle.setAttribute("aria-expanded", String(!collapsed));
      toggle.setAttribute("aria-label", `${collapsed ? "Expand" : "Collapse"} AI Assistant`);
      toggle.title = `${collapsed ? "Expand" : "Collapse"} AI Assistant`;
      changed();
      try { localStorage.setItem(storageKey, String(collapsed)); }
      catch { /* Collapse still works when storage is unavailable. */ }
    }
    let collapsed = false;
    try { collapsed = localStorage.getItem(storageKey) === "true"; }
    catch { /* Use the expanded default without storage. */ }
    setCollapsed(collapsed);
    toggle.addEventListener("click", () => setCollapsed(toggle.getAttribute("aria-expanded") === "true"));
    return setCollapsed;
  }

  function mount(options) {
    const pane = options.pane;
    const prefix = options.prefix;
    for (const child of [...pane.children]) {
      if (!child.classList.contains("assistant-heading")) child.remove();
    }
    const body = document.createElement("div");
    body.className = "assistant-body";
    body.innerHTML = `<details class="assistant-options"><summary>Model and max tokens</summary><div class="assistant-settings">
      <label for="${prefix}-model">Model<select id="${prefix}-model" data-assistant-model aria-label="Model" disabled></select></label>
      <label for="${prefix}-max-tokens">Max tokens<select id="${prefix}-max-tokens" data-assistant-tokens aria-label="Max tokens" disabled><option value="300">300 tokens</option><option value="600">600 tokens</option><option value="1200" selected>1,200 tokens</option><option value="2400">2,400 tokens</option></select></label>
      </div></details>
      <span id="${prefix}-usage" class="assistant-usage" data-assistant-usage role="status">Session: 0 input · 0 output tokens</span>
      <p id="${prefix}-disclosure" class="assistant-disclosure"></p>
      <div id="${prefix}-messages" class="assistant-messages" data-assistant-thread role="log" aria-live="polite"></div>
      <div id="${prefix}-form" class="assistant-form" data-assistant-form><label for="${prefix}-question">Message</label><textarea id="${prefix}-question" data-assistant-question rows="3" maxlength="12000" placeholder="How can I help?" disabled></textarea>
      <div class="assistant-actions"><button id="${prefix}-send" class="button button-refresh" data-assistant-submit type="button" disabled>Send</button>
      <button class="button" data-assistant-apply type="button" hidden></button><button class="button" data-assistant-undo type="button" hidden>Undo AI change</button><button class="button" data-assistant-redo type="button" hidden>Redo AI change</button><button class="button" data-assistant-clear type="button">Clear</button></div></div>`;
    pane.append(body);
    const model = body.querySelector("[data-assistant-model]");
    const tokens = body.querySelector("[data-assistant-tokens]");
    const question = body.querySelector("[data-assistant-question]");
    const send = body.querySelector("[data-assistant-submit]");
    const apply = body.querySelector("[data-assistant-apply]");
    const undo = body.querySelector("[data-assistant-undo]");
    const redo = body.querySelector("[data-assistant-redo]");
    const clear = body.querySelector("[data-assistant-clear]");
    const thread = body.querySelector("[data-assistant-thread]");
    const usage = body.querySelector("[data-assistant-usage]");
    const disclosure = body.querySelector(".assistant-disclosure");
    disclosure.textContent = options.disclosure || "Suggestions never save, publish, activate or run automatically.";
    const label = pane.querySelector(".assistant-state");
    const state = { ready: false, pending: false, error: "", messages: [], input: 0, output: 0, proposal: null };
    let generation = 0;
    let proposalContext = "";
    let proposalGeneration = 0;
    let resetVersion = 0;
    const undoHistory = [];
    const redoHistory = [];
    let statusFailure = "";
    const fingerprint = () => JSON.stringify(options.getContext());

    function render() {
      const editable = options.editable?.() ?? true;
      model.disabled = tokens.disabled = !state.ready || state.pending;
      question.disabled = !state.ready || !editable || state.pending;
      send.disabled = !state.ready || !editable || state.pending || !question.value.trim();
      clear.disabled = state.pending;
      apply.hidden = !state.proposal;
      apply.disabled = state.pending || !editable;
      apply.textContent = options.applyLabel || "Apply draft";
      undo.hidden = !undoHistory.length;
      redo.hidden = !redoHistory.length;
      undo.disabled = redo.disabled = state.pending || !editable;
      label.textContent = state.pending ? "Thinking" : state.ready ? "Ready" : statusFailure ? "Unavailable" : options.unavailableLabel || "Not configured";
      label.classList.toggle("ready", state.ready);
      usage.textContent = `Session: ${state.input.toLocaleString()} input · ${state.output.toLocaleString()} output tokens`;
      thread.replaceChildren();
      if (!state.messages.length && !state.error) {
        const empty = document.createElement("p");
        empty.className = "assistant-empty";
        empty.textContent = options.emptyMessage || "No conversation yet.";
        thread.append(empty);
      }
      for (const message of state.messages) {
        const entry = document.createElement("article");
        entry.className = `assistant-message design-assistant-message ${message.role}`;
        const author = document.createElement("strong");
        author.textContent = message.role === "user" ? "YOU" : "CLAUDE";
        const insert = message.role === "assistant" && options.getTarget ? (text) => {
          try {
            if (message.generation !== generation || message.context !== fingerprint()) throw new Error("The editor changed after this suggestion. Ask again using the current draft.");
            editText(options.getTarget(), text);
            state.error = "";
          } catch (error) { state.error = error.message; }
          render();
        } : null;
        entry.append(author, content(message.content, insert));
        thread.append(entry);
      }
      if (state.error || statusFailure || state.pending) {
        const notice = document.createElement("p");
        notice.className = state.pending ? "assistant-pending" : "assistant-error";
        notice.textContent = state.pending ? "Claude is responding…" : state.error || statusFailure;
        thread.append(notice);
      }
      thread.scrollTop = thread.scrollHeight;
    }

    function invalidate() {
      generation++;
      if (state.pending) state.error = "The editor changed while Claude was responding. Ask again using the current draft.";
      state.pending = false;
      render();
    }

    function reset() {
      resetVersion++;
      invalidate();
      state.messages = [];
      state.proposal = null;
      state.error = "";
      state.input = state.output = 0;
      undoHistory.length = redoHistory.length = 0;
      question.value = "";
      render();
    }

    async function ask() {
      if (!state.ready || state.pending || question.disabled || !question.value.trim()) return;
      state.messages = state.messages.slice(-18);
      state.messages.push({ role: "user", content: question.value.trim() });
      question.value = "";
      state.error = "";
      state.proposal = null;
      state.pending = true;
      const current = ++generation;
      let context;
      try {
        context = fingerprint();
        render();
        const result = await options.request({
          messages: state.messages.map(({ role, content }) => ({ role, content })),
          model: model.value, max_tokens: Number(tokens.value),
        });
        if (current !== generation) return;
        state.input += Number(result.input_tokens) || 0;
        state.output += Number(result.output_tokens) || 0;
        if (context !== fingerprint()) throw new Error("The editor changed while Claude was responding. Ask again using the current draft.");
        if (typeof result.answer !== "string" || !result.answer.trim()) throw new Error("The assistant returned an empty response.");
        state.messages.push({ role: "assistant", content: result.answer, context, generation: current });
        state.proposal = result.proposal_error ? null : result.proposal || null;
        proposalContext = context;
        proposalGeneration = current;
        if (result.proposal_error) state.error = `Proposal not applied: ${result.proposal_error}`;
      } catch (error) {
        if (current === generation) state.error = error.message;
      } finally {
        if (current === generation) { state.pending = false; render(); }
      }
    }

    apply.addEventListener("click", async () => {
      if (!state.proposal || state.pending) return;
      const proposal = state.proposal;
      const version = resetVersion;
      const current = generation;
      const context = proposalContext;
      state.pending = true;
      render();
      try {
        if (proposalGeneration !== generation || proposalContext !== fingerprint()) {
          state.proposal = null;
          throw new Error("The editor changed after this proposal. Ask again using the current draft.");
        }
        const result = await options.apply(proposal, () => current === generation && context === fingerprint());
        if (version !== resetVersion) return;
        if (!result || typeof result.undo !== "function" || typeof result.redo !== "function") throw new Error("The editor did not provide undo/redo for this AI change.");
        undoHistory.push(result);
        redoHistory.length = 0;
        state.proposal = null;
        state.error = "";
        state.messages.push({ role: "assistant", content: "Applied as an unsaved draft. Review before saving." });
      } catch (error) { if (version === resetVersion) state.error = error.message; }
      finally { if (version === resetVersion) { state.pending = false; render(); } }
    });
    for (const [button, action] of [[undo, "undo"], [redo, "redo"]]) {
      button.addEventListener("click", async () => {
        const from = action === "undo" ? undoHistory : redoHistory;
        const to = action === "undo" ? redoHistory : undoHistory;
        const history = from.at(-1);
        if (!history || state.pending) return;
        const version = resetVersion;
        state.pending = true;
        render();
        try {
          await history[action]();
          if (version === resetVersion) { to.push(from.pop()); state.error = ""; }
        } catch (error) { if (version === resetVersion) state.error = error.message; }
        finally { if (version === resetVersion) { state.pending = false; render(); } }
      });
    }
    send.addEventListener("click", ask);
    question.addEventListener("input", render);
    question.addEventListener("keydown", (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter") { event.preventDefault(); ask(); }
    });
    clear.addEventListener("click", reset);
    if (options.watch) options.watch(invalidate, reset);
    render();
    return {
      render, reset, invalidate,
      setStatus(data) {
        state.ready = Boolean(data.ready ?? data.assistant_ready);
        const names = data.models ?? data.assistant_models ?? [];
        const selected = model.value || data.model || data.assistant_model;
        model.replaceChildren(...names.map((name) => new Option(name, name)));
        model.value = names.includes(selected) ? selected : (data.model || data.assistant_model || "");
        statusFailure = "";
        disclosure.textContent = options.disclosure || "Suggestions never save, publish, activate or run automatically.";
        if (!state.ready) disclosure.textContent += ` ${data.message || "Run scripts/configure-assistant.sh, then refresh assistant status."}`;
        render();
      },
      statusError(error) { state.ready = false; statusFailure = `Assistant status unavailable: ${error.message}`; render(); },
    };
  }

  async function jsonRequest(url, init = {}) {
    return window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch ?? window.fetch.bind(window), url, init);
  }

  window.ZPRAssistant = { mount, bindCollapse, editText, content, jsonRequest };
})();
