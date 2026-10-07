(() => {
  const root = document.querySelector("[data-gateway-manager]");
  if (!root) return;

  const state = { contracts: [], configs: [], selected: null, record: null, validDraft: "", busy: false, loaded: false };
  const byId = (id) => document.getElementById(id);
  const contractList = byId("gateway-contracts");
  const contractCount = byId("gateway-contract-count");
  const status = byId("gateway-status");
  const draftIdentity = byId("gateway-draft-identity");
  const revisionPicker = byId("gateway-revision");
  const draftEmpty = byId("gateway-draft-empty");
  const draftForm = byId("gateway-draft-form");
  const destinations = byId("gateway-destinations");
  const draftMessage = byId("gateway-draft-message");
  const validateButton = byId("gateway-validate");
  const saveButton = byId("gateway-save");

  async function readJSON(path, options = {}) {
    const response = await fetch(path, { cache: "no-store", headers: { Accept: "application/json" }, ...options });
    let result;
    try { result = await response.json(); } catch { result = {}; }
    if (!response.ok) throw new Error(result.error || result.diagnostics || `HTTP ${response.status}`);
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

  function latestConfig(record) {
    return record?.revisions?.at(-1)?.config || emptyConfig(state.selected);
  }

  function isDirty() {
    return draftForm.dataset.savedValue !== JSON.stringify(readConfig());
  }

  function clearValidation(message = "") {
    state.validDraft = "";
    saveButton.disabled = true;
    draftMessage.textContent = message;
    draftMessage.dataset.state = "";
  }

  function renderContracts() {
    contractList.replaceChildren();
    contractCount.textContent = String(state.contracts.length);
    if (!state.contracts.length) {
      const empty = document.createElement("p");
      empty.className = "gateway-empty";
      empty.textContent = "No installed ZPL Gateway services were reported for this organization.";
      contractList.append(empty);
      return;
    }
    for (const contract of state.contracts) {
      const row = document.createElement("article");
      row.className = "gateway-contract-row";
      const details = document.createElement("div");
      const title = document.createElement("strong");
      title.textContent = contract.service_name;
      const identity = document.createElement("small");
      identity.textContent = `${contract.adapter_cn} · ${contract.external_network || "network label unavailable"}`;
      details.append(title, identity);
      const action = document.createElement("button");
      action.type = "button";
      action.className = "button gateway-select";
      action.textContent = state.selected?.instance_id === contract.instance_id ? "Selected" : "Configure";
      action.setAttribute("aria-pressed", String(state.selected?.instance_id === contract.instance_id));
      action.addEventListener("click", () => selectContract(contract));
      row.append(details, action);
      contractList.append(row);
    }
  }

  function configRows(config) {
    destinations.replaceChildren();
    const entries = config?.destinations?.length ? config.destinations : [{ origin: "", path_prefixes: ["/"] }];
    for (const destination of entries) {
      const row = document.createElement("div");
      row.className = "gateway-destination-row";
      const originLabel = document.createElement("label");
      originLabel.textContent = "HTTPS origin";
      const origin = document.createElement("input");
      origin.type = "url";
      origin.placeholder = "https://api.example.com";
      origin.setAttribute("aria-label", "Gateway HTTPS origin");
      origin.value = destination.origin || "";
      originLabel.append(origin);
      const pathsLabel = document.createElement("label");
      pathsLabel.textContent = "Allowed path prefixes";
      const paths = document.createElement("textarea");
      paths.rows = 2;
      paths.placeholder = "/v1/\n/health";
      paths.setAttribute("aria-label", "Allowed gateway path prefixes");
      paths.value = (destination.path_prefixes || []).join("\n");
      pathsLabel.append(paths);
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "button gateway-remove-destination";
      remove.textContent = "Remove";
      remove.disabled = entries.length < 2;
      remove.addEventListener("click", () => {
        if (destinations.children.length <= 1) return;
        row.remove();
        clearValidation("Draft changed. Validate it again before saving.");
        updateControls();
      });
      row.append(originLabel, pathsLabel, remove);
      row.addEventListener("input", () => clearValidation("Draft changed. Validate it again before saving."));
      destinations.append(row);
    }
    byId("gateway-method-get").checked = (config?.methods || ["GET", "HEAD"]).includes("GET");
    byId("gateway-method-head").checked = (config?.methods || ["GET", "HEAD"]).includes("HEAD");
    byId("gateway-timeout").value = String(config?.timeout_ms ?? 8000);
    byId("gateway-response-limit").value = String(config?.max_response_bytes ?? 2097152);
    draftForm.dataset.savedValue = JSON.stringify(readConfig());
    for (const input of draftForm.querySelectorAll("input, textarea")) input.addEventListener("input", () => clearValidation("Draft changed. Validate it again before saving."));
    for (const input of draftForm.querySelectorAll("input[type=checkbox]")) input.addEventListener("change", () => clearValidation("Draft changed. Validate it again before saving."));
    updateControls();
  }

  function readConfig() {
    if (!state.selected) return {};
    return {
      schema_version: 1,
      organization_id: state.selected.organization_id,
      instance_id: state.selected.instance_id,
      adapter_cn: state.selected.adapter_cn,
      service_name: state.selected.service_name,
      destinations: [...destinations.querySelectorAll(".gateway-destination-row")].map((row) => ({
        origin: row.querySelector("input").value.trim(),
        path_prefixes: row.querySelector("textarea").value.split("\n").map((value) => value.trim()).filter(Boolean),
      })),
      methods: [byId("gateway-method-get").checked ? "GET" : "", byId("gateway-method-head").checked ? "HEAD" : ""].filter(Boolean),
      timeout_ms: Number(byId("gateway-timeout").value),
      max_response_bytes: Number(byId("gateway-response-limit").value),
    };
  }

  function updateControls() {
    const config = JSON.stringify(readConfig());
    validateButton.disabled = state.busy || !state.selected;
    saveButton.disabled = state.busy || !state.validDraft || state.validDraft !== config || !isDirty();
    byId("gateway-add-destination").disabled = state.busy || destinations.children.length >= 32;
    for (const remove of destinations.querySelectorAll(".gateway-remove-destination")) remove.disabled = state.busy || destinations.children.length < 2;
  }

  function renderRevisionPicker(record, selectedRevision) {
    revisionPicker.replaceChildren();
    if (!record?.revisions?.length) revisionPicker.add(new Option("New draft", ""));
    for (const revision of record?.revisions || []) {
      const option = new Option(`r${revision.revision}${revision.revision === record.current_revision ? " · current" : ""}`, String(revision.revision));
      revisionPicker.add(option);
    }
    revisionPicker.value = selectedRevision ? String(selectedRevision) : "";
    revisionPicker.disabled = !record?.revisions?.length || state.busy;
  }

  function renderEditor(config = null) {
    const selected = state.selected;
    draftForm.hidden = !selected;
    draftEmpty.hidden = Boolean(selected);
    if (!selected) {
      draftIdentity.textContent = "Select an installed gateway.";
      revisionPicker.replaceChildren(new Option("New draft", ""));
      revisionPicker.disabled = true;
      return;
    }
    draftIdentity.textContent = `${selected.instance_id} · ${selected.adapter_cn} · ${selected.service_name}`;
    configRows(config || latestConfig(state.record));
    renderRevisionPicker(state.record, state.record?.current_revision);
  }

  async function loadInventory() {
    if (state.busy) return;
    state.busy = true;
    byId("gateway-refresh").disabled = true;
    status.textContent = "Loading installed gateway services and saved drafts…";
    try {
      const [contractResponse, configResponse] = await Promise.all([
        readJSON("/api/gateways/contracts"),
        readJSON("/api/gateways/configs"),
      ]);
      state.contracts = contractResponse.contracts || [];
      state.configs = configResponse.configs || [];
      if (state.selected) state.selected = state.contracts.find((item) => item.instance_id === state.selected.instance_id) || null;
      if (!state.selected) state.selected = state.contracts[0] || null;
      state.record = state.configs.find((record) => record.instance_id === state.selected?.instance_id) || null;
      state.loaded = true;
      renderContracts();
      renderEditor();
      status.textContent = `${contractResponse.organization_id} · ${state.contracts.length} installed gateway service${state.contracts.length === 1 ? "" : "s"}`;
    } catch (error) {
      status.textContent = `Gateway inventory unavailable: ${error.message}`;
      state.contracts = [];
      state.configs = [];
      state.selected = null;
      state.record = null;
      renderContracts();
      renderEditor();
    } finally {
      state.busy = false;
      byId("gateway-refresh").disabled = false;
      updateControls();
    }
  }

  async function selectContract(contract) {
    if (state.selected?.instance_id === contract.instance_id) return;
    if (state.selected && isDirty() && !confirm("Discard unsaved gateway draft edits?")) return;
    state.selected = contract;
    state.record = state.configs.find((record) => record.instance_id === contract.instance_id) || null;
    state.validDraft = "";
    draftMessage.textContent = "";
    renderContracts();
    renderEditor();
  }

  async function validateDraft() {
    if (!state.selected || state.busy) return;
    state.busy = true;
    updateControls();
    clearValidation("Validating draft against the live Gateway service…");
    try {
      const config = readConfig();
      const result = await readJSON("/api/gateways/config/check", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ config }),
      });
      if (!result.valid) throw new Error(result.diagnostics || "Gateway draft validation failed.");
      state.validDraft = JSON.stringify(config);
      draftMessage.textContent = `${result.diagnostics} No runtime changes were made.`;
      draftMessage.dataset.state = "success";
    } catch (error) {
      clearValidation(error.message);
      draftMessage.dataset.state = "error";
    } finally {
      state.busy = false;
      updateControls();
    }
  }

  async function saveDraft() {
    if (!state.selected || state.busy || !state.validDraft || state.validDraft !== JSON.stringify(readConfig())) return;
    state.busy = true;
    updateControls();
    try {
      const expectedRevision = state.record?.current_revision || 0;
      const config = readConfig();
      const record = await readJSON(`/api/gateways/configs/${encodeURIComponent(state.selected.instance_id)}/revisions`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expected_revision: expectedRevision, config }),
      });
      state.record = record;
      state.configs = [...state.configs.filter((item) => item.instance_id !== record.instance_id), record];
      draftForm.dataset.savedValue = JSON.stringify(config);
      renderContracts();
      renderRevisionPicker(record, record.current_revision);
      draftMessage.textContent = `Saved draft revision ${record.current_revision}. Runtime is unchanged.`;
      draftMessage.dataset.state = "success";
      state.validDraft = JSON.stringify(config);
      status.textContent = `${state.selected.instance_id} · draft r${record.current_revision} saved`;
    } catch (error) {
      draftMessage.textContent = error.message;
      draftMessage.dataset.state = "error";
    } finally {
      state.busy = false;
      updateControls();
    }
  }

  byId("gateway-refresh").addEventListener("click", () => void loadInventory());
  byId("gateway-add-destination").addEventListener("click", () => {
    if (destinations.children.length >= 32) return;
    const config = readConfig();
    config.destinations.push({ origin: "", path_prefixes: ["/"] });
    configRows(config);
    clearValidation("Draft changed. Validate it again before saving.");
  });
  byId("gateway-validate").addEventListener("click", () => void validateDraft());
  byId("gateway-save").addEventListener("click", () => void saveDraft());
  revisionPicker.addEventListener("change", () => {
    if (!state.record || !revisionPicker.value) return;
    if (isDirty() && !confirm("Discard unsaved gateway draft edits?")) {
      revisionPicker.value = String(state.record.current_revision);
      return;
    }
    const revision = state.record.revisions.find((item) => item.revision === Number(revisionPicker.value));
    if (!revision) return;
    state.validDraft = "";
    clearValidation(`Loaded draft r${revision.revision}. Validate before saving a new revision.`);
    configRows(revision.config);
  });
  window.addEventListener("hashchange", () => {
    if (location.hash === "#gateways" && !state.loaded) void loadInventory();
  });
  if (location.hash === "#gateways") void loadInventory();
})();
