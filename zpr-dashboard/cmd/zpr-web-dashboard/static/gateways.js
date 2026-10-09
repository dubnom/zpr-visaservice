(() => {
  const root = document.querySelector("[data-gateway-manager]");
  if (!root) return;

  const state = { contracts: [], configs: [], selected: null, record: null, saved: "", validDraft: "", revision: 0, diagnostic: null, busy: false, loaded: false, formMode: false };
  const allowedMethods = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"];
  const byId = (id) => document.getElementById(id);
  const source = byId("gateway-source");
  const gutter = byId("gateway-gutter");
  const highlight = byId("gateway-highlight");
  const contractList = byId("gateway-contracts");
  const pane = byId("gateway-catalog-pane");
  const pickerToggle = byId("gateway-picker-toggle");
  const fileMenu = byId("gateway-file-menu");
  const filesToggle = byId("gateway-files-toggle");
  const analyzeButton = byId("gateway-analyze");
  const formatButton = byId("gateway-format");
  const saveButton = byId("gateway-save");
  const page = window.ZPREditorPage;
  const message = byId("gateway-draft-message");
  const editor = page.createController({
    source, status: message, isDirty: () => isDirty(),
    readContext: () => [source.value, state.selected?.organization_id, state.selected?.instance_id,
      state.selected?.adapter_cn, state.selected?.service_name, state.revision, state.formMode],
    identity: { title: byId("gateway-record-title"), version: byId("gateway-revision-label"), modified: byId("gateway-modified-indicator") },
    menu: { root: byId("gateway-actions"), toggle: filesToggle, menu: fileMenu },
    history: { menu: byId("gateway-history-menu"), list: byId("gateway-history"), count: byId("gateway-history-count"), isAvailable: () => Boolean(state.selected) },
    adapters: { load: loadInventory, analyze: analyzeDraft, save: saveDraft, render: renderSource },
    syncControls: updateControls,
  });
  const analysisScope = editor.createScope("analyze");
  window.getGatewayAssistantContext = () => [state.selected?.organization_id, state.selected?.instance_id, state.selected?.adapter_cn, state.selected?.service_name, state.revision];
  const surface = page.createSourceSurface({
    source, highlight, gutter, language: "json", label: "Gateway draft",
    markerText: "ERR",
    onMarker: diagnostic => {
      byId("gateway-error-text").textContent = diagnostic.message;
      byId("gateway-error-dialog").showModal();
    },
  });
  const historyMenu = editor.history;
  const picker = page.createPicker({ toggle: pickerToggle, pane, close: byId("gateway-picker-close"), focus: contractList });
  const files = editor.files;

  const readJSON = (path, options = {}, responsePolicy) => page.requestJSON(
    (...args) => window.zprOperatorFetch(...args), path,
    { headers: { Accept: "application/json" }, ...options },
    responsePolicy,
  );

  function emptyConfig(contract) {
    return {
      schema_version: 1,
      organization_id: contract.organization_id,
      instance_id: contract.instance_id,
      adapter_cn: contract.adapter_cn,
      service_name: contract.service_name,
      destinations: [{ origin: "", path_prefixes: ["/"] }],
      methods: ["GET", "HEAD"],
      timeout_ms: 8000,
      max_response_bytes: 2097152,
    };
  }

  const pretty = (config) => `${JSON.stringify(config, null, 2)}\n`;
  const isDirty = () => Boolean(state.selected) && (source.value !== state.saved || hasIncompleteFormNumber());
  const hasIncompleteFormNumber = () => state.formMode &&
    ["gateway-form-timeout", "gateway-form-response-limit"].some(id => !Number.isFinite(byId(id).valueAsNumber));

  function setMessage(text, kind = "") {
    editor.setStatus(text, kind);
  }

  function renderIdentity() {
    const selected = state.selected;
    editor.renderIdentity({
        name: selected?.service_name || "",
        label: !selected ? "" : state.revision ? `Version ${state.revision}` : "New · unsaved",
        tooltip: selected ? `${selected.instance_id} · ${selected.adapter_cn}` : "",
        dirty: isDirty(),
    });
  }

  // Parses the draft locally so syntax errors get a gutter marker before the server is asked.
  function parseSource() {
    try {
      return { config: JSON.parse(source.value) };
    } catch (error) {
      const text = String(error.message || error);
      let line = Number(text.match(/line (\d+)/)?.[1] || 0);
      const position = text.match(/position (\d+)/)?.[1];
      if (!line && position !== undefined) line = source.value.slice(0, Number(position)).split("\n").length;
      return { error: `Invalid JSON: ${text}`, line: line || 1 };
    }
  }

  function clearAnalysis() {
    analysisScope.invalidate();
    state.validDraft = "";
    surface.setDiagnostic(null);
    const dialog = byId("gateway-error-dialog");
    if (dialog.open) dialog.close();
    byId("gateway-error-text").textContent = "";
    byId("gateway-form-error").textContent = "";
    byId("gateway-form-error").hidden = true;
    editor.setAnalysisState();
  }

  function updateControls() {
    const selected = Boolean(state.selected);
    const empty = !source.value.trim();
    source.disabled = !selected;
    analyzeButton.disabled = state.busy || !selected || empty || hasIncompleteFormNumber();
    formatButton.disabled = state.busy || !selected || empty || state.formMode;
    formatButton.classList.toggle("button-save-as-ready", !formatButton.disabled);
    analyzeButton.classList.toggle("button-next-evaluate", !analyzeButton.disabled && !analyzeButton.dataset.analysisState);
    const canSave = !state.busy && selected && !hasIncompleteFormNumber() && (isDirty() || !state.record) && !empty;
    saveButton.disabled = !canSave;
    saveButton.classList.toggle("button-save-next", !canSave);
    saveButton.classList.toggle("button-save-as-ready", canSave);
    byId("gateway-open").disabled = state.busy || !selected;
    byId("gateway-download").disabled = !selected || empty;
    byId("gateway-discard").disabled = state.busy || !isDirty();
    byId("gateway-refresh").disabled = state.busy || hasIncompleteFormNumber();
    byId("gateway-mode-toggle").disabled = state.busy || !selected;
    byId("gateway-form-fields").disabled = state.busy || !selected;
  }

  function renderSource(refreshForm = true) {
    surface.render();
    if (state.formMode && refreshForm) renderForm();
    renderIdentity();
    updateControls();
    if (state.formMode) {
      const parsed = parseSource();
      if (parsed.error || formConfigError(parsed.config)) byId("gateway-form-fields").disabled = true;
    }
  }

  function formConfigError(config) {
    if (!config || typeof config !== "object" || Array.isArray(config)) return "The draft must be a JSON object.";
    if (!Array.isArray(config.destinations) || !config.destinations.every(destination =>
      destination && typeof destination === "object" && !Array.isArray(destination) &&
      typeof destination.origin === "string" && Array.isArray(destination.path_prefixes) &&
      destination.path_prefixes.every(prefix => typeof prefix === "string" && !/[\r\n]/.test(prefix)))) {
      return "Destinations must contain origins and string path-prefix arrays.";
    }
    if (!Array.isArray(config.methods) || !config.methods.every(method => allowedMethods.includes(method)) ||
        new Set(config.methods).size !== config.methods.length ||
        !Number.isFinite(config.timeout_ms) || !Number.isFinite(config.max_response_bytes)) {
      return "The form requires unique GET, HEAD, POST, PUT, PATCH, DELETE or OPTIONS methods and numeric timeout/response limits.";
    }
    return "";
  }

  function setFormError(text) {
    byId("gateway-form-error").textContent = text;
    byId("gateway-form-error").hidden = !text;
  }

  function updateFormDraft(edit, refreshForm = false) {
    if (state.busy || !state.selected) return;
    if (refreshForm && hasIncompleteFormNumber()) {
      setFormError("Enter numeric timeout and response limits before adding or removing destinations.");
      return;
    }
    const parsed = parseSource();
    const error = parsed.error || formConfigError(parsed.config);
    if (error) {
      setFormError(`${error} Switch to the raw editor to correct the draft.`);
      return;
    }
    edit(parsed.config);
    source.value = pretty(parsed.config);
    clearAnalysis();
    setMessage("");
    renderSource(refreshForm);
    if (hasIncompleteFormNumber()) setFormError("Enter numeric timeout and response limits before analyzing or saving.");
  }

  function renderForm() {
    const parsed = parseSource();
    const error = parsed.error || formConfigError(parsed.config);
    if (error) {
      byId("gateway-form-fields").disabled = true;
      setFormError(`${error} Switch to the raw editor to correct the draft.`);
      return;
    }
    const config = parsed.config;
    const identity = byId("gateway-form-identity");
    identity.replaceChildren();
    for (const [field, label] of [
      ["schema_version", "Schema version"], ["organization_id", "Organization"],
      ["instance_id", "Instance"], ["adapter_cn", "Adapter"], ["service_name", "Service"],
    ]) {
      const term = document.createElement("dt");
      term.textContent = label;
      const value = document.createElement("dd");
      value.textContent = String(config[field] ?? "(missing)");
      identity.append(term, value);
    }
    const rows = byId("gateway-form-destinations");
    rows.replaceChildren();
    config.destinations.forEach((destination, index) => {
      const row = document.createElement("div");
      row.className = "gateway-form-destination";
      const actions = document.createElement("div");
      actions.className = "gateway-form-destination-actions";
      for (const [field, label, value] of [
        ["origin", "Base URL", destination.origin],
        ["path_prefixes", "Paths", destination.path_prefixes.join("\n")],
      ]) {
        const wrapper = document.createElement("label");
        wrapper.textContent = label;
        const input = document.createElement(field === "origin" ? "input" : "textarea");
        input.setAttribute("aria-label", `${label} for destination ${index + 1}`);
        input.value = value;
        if (field === "origin") {
          input.type = "text";
          input.placeholder = "https://api.example.com";
        } else {
          input.rows = 3;
          input.placeholder = "/health\n/api/";
          input.title = 'One path prefix per line. "/" allows every path.';
        }
        input.addEventListener("input", () => updateFormDraft(draft => {
          draft.destinations[index][field] = field === "origin" ? input.value :
            input.value.split("\n");
        }));
        wrapper.append(input);
        row.append(wrapper);
      }
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "button";
      remove.textContent = "Remove";
      remove.title = "Delete this destination and all its allowed path prefixes from the draft";
      remove.setAttribute("aria-label", `Remove destination ${index + 1}`);
      remove.addEventListener("click", () => updateFormDraft(draft => draft.destinations.splice(index, 1), true));
      actions.append(remove);
      row.append(actions);
      rows.append(row);
    });
    byId("gateway-form-add-destination").disabled = config.destinations.length >= 32;
    for (const method of allowedMethods) {
      byId(`gateway-form-${method.toLowerCase()}`).checked = config.methods.includes(method);
    }
    byId("gateway-form-timeout").value = config.timeout_ms;
    byId("gateway-form-response-limit").value = config.max_response_bytes;
  }

  function setMode(formMode) {
    state.formMode = formMode;
    root.dataset.gatewayMode = formMode ? "form" : "raw";
    clearAnalysis();
    setMessage("");
    byId("gateway-form").hidden = !formMode;
    source.closest(".config-source-editor").hidden = formMode;
    const toggle = byId("gateway-mode-toggle");
    toggle.textContent = formMode ? "Raw JSON editor" : "Form editor";
    toggle.setAttribute("aria-pressed", String(formMode));
    renderSource();
  }
  byId("gateway-mode-toggle").addEventListener("click", () => {
    if (state.formMode) {
      if (hasIncompleteFormNumber()) {
        setFormError("Enter numeric timeout and response limits before switching to the raw editor.");
        return;
      }
      setMode(false);
      source.focus();
      return;
    }
    const parsed = parseSource();
    const error = parsed.error || formConfigError(parsed.config);
    if (error) {
      setMessage(`Cannot open form editor: ${error} Correct the raw JSON first.`, "error");
      return;
    }
    setMode(true);
  });
  byId("gateway-form-add-destination").addEventListener("click", () =>
    updateFormDraft(config => config.destinations.push({ origin: "", path_prefixes: ["/"] }), true));
  for (const method of allowedMethods) {
    const id = `gateway-form-${method.toLowerCase()}`;
    byId(id).addEventListener("input", () => updateFormDraft(config => {
      config.methods = allowedMethods.filter(method => byId(`gateway-form-${method.toLowerCase()}`).checked);
    }));
  }
  for (const [id, field] of [["gateway-form-timeout", "timeout_ms"], ["gateway-form-response-limit", "max_response_bytes"]]) {
    byId(id).addEventListener("input", () => {
      const value = byId(id).valueAsNumber;
      if (!Number.isFinite(value)) {
        clearAnalysis();
        setFormError("Enter a numeric value before analyzing or saving.");
        renderIdentity();
        updateControls();
        return;
      }
      updateFormDraft(config => { config[field] = value; });
    });
  }
  byId("gateway-error-close").addEventListener("click", () => byId("gateway-error-dialog").close());
  byId("gateway-error-dismiss").addEventListener("click", () => byId("gateway-error-dialog").close());

  function renderContracts() {
    contractList.replaceChildren();
    byId("gateway-contract-count").textContent = String(state.contracts.length);
    if (!state.contracts.length) {
      const empty = document.createElement("p");
      empty.className = "catalog-empty";
      empty.textContent = state.loaded ? "No installed ZPL Gateway services were reported for this organization." : "Loading installed gateway services…";
      contractList.append(empty);
      return;
    }
    for (const contract of state.contracts) {
      const record = state.configs.find((item) => item.instance_id === contract.instance_id);
      const item = document.createElement("button");
      item.type = "button";
      item.className = "policy-record-item gateway-record-item";
      item.dataset.kind = "gateway";
      item.dataset.instanceId = contract.instance_id;
      item.setAttribute("role", "treeitem");
      item.setAttribute("aria-selected", String(state.selected?.instance_id === contract.instance_id));
      const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      icon.setAttribute("class", "policy-kind-icon");
      icon.setAttribute("viewBox", "0 0 16 16");
      icon.setAttribute("aria-hidden", "true");
      icon.innerHTML = '<path d="M4.2 13C2 13 1.3 10.3 3.1 9.4 3 6.8 6.3 5.8 7.4 7.8 9 6.1 12.2 7 12.1 9.3 14.6 9.6 14.4 13 12 13Z"/>';
      const title = document.createElement("strong");
      title.textContent = contract.service_name;
      const detail = document.createElement("small");
      detail.textContent = `${contract.adapter_cn} · ${record?.current_revision ? `draft r${record.current_revision}` : "no saved draft"}`;
      item.append(icon, title, detail);
      item.addEventListener("click", () => selectContract(contract));
      contractList.append(item);
    }
  }

  function renderHistory() {
    const revisions = [...(state.record?.revisions || [])].reverse().map((revision) => ({ ...revision, number: revision.revision }));
    historyMenu.render(revisions, {
      current: state.revision,
      onSelect: loadRevision,
      detail: (revision) => revision.revision === state.record.current_revision ? "Latest saved draft" : "Earlier draft",
      meta: (revision) => revision.saved_at ? window.ZPRSafeDisplay.formatDateTime(revision.saved_at) : "",
    });
  }

  function renderSelection() {
    renderContracts();
    renderHistory();
    renderSource();
  }

  function loadSelected() {
    const latest = state.record?.revisions?.at(-1);
    state.revision = latest?.revision || 0;
    source.value = state.saved = state.selected ? pretty(latest?.config || emptyConfig(state.selected)) : "";
    clearAnalysis();
    setMessage("");
    renderSelection();
  }

  function setPickerOpen(open, restoreFocus = false) {
    picker.setOpen(open, restoreFocus);
  }

  async function loadInventory() {
    if (state.busy) return;
    state.busy = true;
    updateControls();
    try {
      const [contractResponse, configResponse] = await Promise.all([
        readJSON("/api/gateways/contracts"),
        readJSON("/api/gateways/configs"),
      ]);
      state.contracts = contractResponse.contracts || [];
      state.configs = configResponse.configs || [];
      state.loaded = true;
      const previous = state.selected?.instance_id;
      state.selected = state.contracts.find((item) => item.instance_id === previous) || state.contracts[0] || null;
      state.record = state.configs.find((record) => record.instance_id === state.selected?.instance_id) || null;
      if (!previous || previous !== state.selected?.instance_id || !isDirty()) loadSelected();
      else renderSelection();
    } catch (error) {
      state.loaded = true;
      state.contracts = [];
      state.configs = [];
      state.selected = null;
      state.record = null;
      loadSelected();
      setMessage(`Gateway inventory unavailable: ${error.message}`, "error");
    } finally {
      state.busy = false;
      renderSource();
    }
  }

  function selectContract(contract) {
    if (state.selected?.instance_id === contract.instance_id) {
      setPickerOpen(false, true);
      return;
    }
    if (!page.confirmDiscard(isDirty(), "Discard unsaved gateway draft edits?")) return;
    state.selected = contract;
    state.record = state.configs.find((record) => record.instance_id === contract.instance_id) || null;
    loadSelected();
    setPickerOpen(false, true);
  }

  function loadRevision(revision) {
    if (!page.confirmDiscard((source.value !== pretty(revision.config) || hasIncompleteFormNumber()) && isDirty(), "Discard unsaved gateway draft edits?")) return;
    state.revision = revision.revision;
    source.value = pretty(revision.config);
    clearAnalysis();
    setMessage("");
    renderHistory();
    renderSource();
  }

  async function analyzeDraft() {
    if (!state.selected || state.busy || hasIncompleteFormNumber()) return;
    const parsed = parseSource();
    if (parsed.error) {
      surface.setDiagnostic({ line: parsed.line, message: parsed.error });
      editor.setAnalysisState("error");
      setMessage(surface.diagnostic ? "" : parsed.error, "error");
      renderSource();
      return;
    }
    const analyzed = source.value;
    const isCurrent = editor.beginAnalysis();
    state.busy = true;
    setMessage("");
    updateControls();
    try {
      const result = await readJSON("/api/gateways/config/check", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: `{"config":${analyzed}}`,
      }, { acceptError: (result) => result?.valid === false });
      if (!isCurrent()) return;
      if (!result.valid) {
        const diagnostic = result.diagnostics || "Gateway draft analysis failed.";
        if (Number.isInteger(result.source_line) && result.source_line > 0 &&
            result.source_line <= analyzed.split("\n").length) {
          surface.setDiagnostic({ line: result.source_line, message: diagnostic });
          if (state.formMode) setFormError(diagnostic);
          setMessage("");
          editor.setAnalysisState("error");
          state.validDraft = "";
          return;
        }
        throw new Error(diagnostic);
      }
      state.validDraft = analyzed;
      surface.setDiagnostic(null);
      editor.setAnalysisState("success");
      setMessage(`${result.diagnostics} No runtime changes were made.`, "success");
      return true;
    } catch (error) {
      if (!isCurrent()) return;
      state.validDraft = "";
      editor.setAnalysisState("error");
      setMessage(error.message, "error");
    } finally {
      state.busy = false;
      isCurrent.finish();
      renderSource();
    }
  }

  function formatDraft() {
    const parsed = parseSource();
    if (parsed.error) {
      surface.setDiagnostic({ line: parsed.line, message: parsed.error });
      editor.setAnalysisState("error");
      setMessage(surface.diagnostic ? "" : parsed.error, "error");
      renderSource();
      return;
    }
    const formatted = pretty(parsed.config);
    if (formatted === source.value) return;
    source.focus();
    source.select();
    if (!document.execCommand("insertText", false, formatted)) source.value = formatted;
    clearAnalysis();
    renderSource();
  }

  async function saveDraft() {
    if (saveButton.disabled) return;
    if (!page.isNamed(state.selected?.service_name)) {
      setMessage("The installed gateway needs a name other than Untitled before its draft can be saved.", "error");
      return;
    }
    if (!await analyzeDraft() || state.validDraft !== source.value || (!isDirty() && state.record)) return;
    const submittedSource = source.value;
    const parsed = parseSource();
    if (parsed.error) return;
    state.busy = true;
    updateControls();
    try {
      const record = await readJSON(`/api/gateways/configs/${encodeURIComponent(state.selected.instance_id)}/revisions`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ expected_revision: state.record?.current_revision || 0, config: parsed.config }),
      });
      state.record = record;
      state.configs = [...state.configs.filter((item) => item.instance_id !== record.instance_id), record];
      state.revision = record.current_revision;
      state.saved = submittedSource;
      setMessage(`Saved draft revision ${record.current_revision}. Runtime is unchanged.`, "success");
    } catch (error) {
      setMessage(error.message, "error");
    } finally {
      state.busy = false;
      renderSelection();
    }
  }

  source.addEventListener("input", () => {
    clearAnalysis();
    setMessage("");
    renderSource();
  });
  editor.bind("save", { button: saveButton, shortcutRoot: root });
  editor.bind("analyze", { button: analyzeButton });
  formatButton.addEventListener("click", formatDraft);
  editor.bind("load", { button: byId("gateway-refresh") });
  byId("gateway-discard").addEventListener("click", () => {
    if (!isDirty() || !page.confirmDiscard(true, "Discard unsaved gateway draft edits?")) return;
    source.value = state.saved;
    clearAnalysis();
    setMessage("");
    renderSource();
  });
  byId("gateway-download").addEventListener("click", () => {
    const link = document.createElement("a");
    link.href = URL.createObjectURL(new Blob([source.value], { type: "application/json" }));
    link.download = `${state.selected?.instance_id || "gateway"}.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(link.href), 0);
  });
  byId("gateway-open").addEventListener("click", () => byId("gateway-file-input").click());
  byId("gateway-file-input").addEventListener("change", async (event) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file || (isDirty() && !confirm("Replace unsaved gateway draft edits with this file?"))) return;
    source.value = await file.text();
    clearAnalysis();
    setMessage("");
    renderSource();
  });
  window.addEventListener("beforeunload", (event) => {
    if (isDirty()) event.preventDefault();
  });
  window.addEventListener("hashchange", () => {
    if (location.hash === "#gateways" && !state.loaded) void editor.perform("load");
  });
  renderSelection();
  if (location.hash === "#gateways") void editor.perform("load");
})();
