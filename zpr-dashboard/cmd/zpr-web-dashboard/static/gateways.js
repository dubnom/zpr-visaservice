(() => {
  const root = document.querySelector("[data-gateway-manager]");
  if (!root) return;

  const state = { contracts: [], configs: [], selected: null, record: null, saved: "", validDraft: "", revision: 0, diagnostic: null, busy: false, loaded: false };
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
  const surface = page.createSourceSurface({
    source, highlight, gutter, language: "json", label: "Gateway draft",
  });
  const historyMenu = page.createHistory({
    menu: byId("gateway-history-menu"), list: byId("gateway-history"), count: byId("gateway-history-count"),
    isAvailable: () => Boolean(state.selected),
  });
  const picker = page.createPicker({ toggle: pickerToggle, pane, close: byId("gateway-picker-close"), focus: contractList });
  const files = page.createMenu({ root: byId("gateway-actions"), toggle: filesToggle, menu: fileMenu });

  async function readJSON(path, options = {}) {
    const response = await window.zprOperatorFetch(path, { cache: "no-store", headers: { Accept: "application/json" }, ...options });
    let result;
    try { result = await response.json(); } catch { result = {}; }
    if (!response.ok && !(result && result.valid === false)) throw new Error(result.error || result.diagnostics || `HTTP ${response.status}`);
    return result;
  }

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
  const isDirty = () => Boolean(state.selected) && source.value !== state.saved;

  function setMessage(text, kind = "") {
    page.setStatus(message, text, kind);
  }

  function renderIdentity() {
    const selected = state.selected;
    page.renderIdentity(
      { title: byId("gateway-record-title"), version: byId("gateway-revision-label"), modified: byId("gateway-modified-indicator") },
      {
        name: selected?.service_name || "",
        label: !selected ? "" : state.revision ? `Version ${state.revision}` : "New · unsaved",
        tooltip: selected ? `${selected.instance_id} · ${selected.adapter_cn}` : "",
        dirty: isDirty(),
      },
    );
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
    state.validDraft = "";
    surface.setDiagnostic(null);
    delete analyzeButton.dataset.analysisState;
  }

  function updateControls() {
    const selected = Boolean(state.selected);
    const empty = !source.value.trim();
    source.disabled = !selected;
    analyzeButton.disabled = state.busy || !selected || empty;
    formatButton.disabled = state.busy || !selected || empty;
    formatButton.classList.toggle("button-save-as-ready", !formatButton.disabled);
    analyzeButton.classList.toggle("button-next-evaluate", !analyzeButton.disabled && !analyzeButton.dataset.analysisState);
    const canSave = !state.busy && selected && isDirty() && state.validDraft === source.value;
    saveButton.disabled = !canSave;
    saveButton.classList.toggle("button-save-next", !canSave);
    saveButton.classList.toggle("button-save-as-ready", canSave);
    byId("gateway-open").disabled = state.busy || !selected;
    byId("gateway-download").disabled = !selected || empty;
    byId("gateway-discard").disabled = state.busy || !isDirty();
    byId("gateway-refresh").disabled = state.busy;
  }

  function renderSource() {
    surface.render();
    renderIdentity();
    updateControls();
  }

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
      meta: (revision) => revision.saved_at ? new Date(revision.saved_at).toLocaleString() : "",
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
      updateControls();
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
    if (!page.confirmDiscard(source.value !== pretty(revision.config) && isDirty(), "Discard unsaved gateway draft edits?")) return;
    state.revision = revision.revision;
    source.value = pretty(revision.config);
    clearAnalysis();
    setMessage("");
    renderHistory();
    renderSource();
  }

  async function analyzeDraft() {
    if (!state.selected || state.busy) return;
    const parsed = parseSource();
    if (parsed.error) {
      surface.setDiagnostic({ line: parsed.line, message: parsed.error });
      analyzeButton.dataset.analysisState = "error";
      setMessage(surface.diagnostic ? "" : parsed.error, "error");
      renderSource();
      return;
    }
    const analyzed = source.value;
    state.busy = true;
    analyzeButton.dataset.analysisState = "pending";
    setMessage("");
    updateControls();
    try {
      const result = await readJSON("/api/gateways/config/check", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ config: parsed.config }),
      });
      if (source.value !== analyzed) return;
      if (!result.valid) throw new Error(result.diagnostics || "Gateway draft analysis failed.");
      state.validDraft = analyzed;
      surface.setDiagnostic(null);
      analyzeButton.dataset.analysisState = "success";
      setMessage(`${result.diagnostics} No runtime changes were made.`, "success");
    } catch (error) {
      if (source.value !== analyzed) return;
      state.validDraft = "";
      analyzeButton.dataset.analysisState = "error";
      setMessage(error.message, "error");
    } finally {
      state.busy = false;
      renderSource();
    }
  }

  function formatDraft() {
    const parsed = parseSource();
    if (parsed.error) {
      surface.setDiagnostic({ line: parsed.line, message: parsed.error });
      analyzeButton.dataset.analysisState = "error";
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
      state.saved = source.value;
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
  page.bindSaveShortcut({ root: source, button: saveButton });
  analyzeButton.addEventListener("click", () => void analyzeDraft());
  formatButton.addEventListener("click", formatDraft);
  saveButton.addEventListener("click", () => void saveDraft());
  byId("gateway-refresh").addEventListener("click", () => void loadInventory());
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
    if (location.hash === "#gateways" && !state.loaded) void loadInventory();
  });
  renderSelection();
  if (location.hash === "#gateways") void loadInventory();
})();
