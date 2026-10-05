const scenarioById = new Map();
let scenarioRun;
let scenarioRefreshTimer;
let scenarioRefreshPromise;
let scenarioCatalogSignature = "";
let scenarioRunSignature = "";
let activeScenarioOrganization = "";
let scenarioEditorArtifact;
let scenarioEditorOrganization = "";
let scenarioEditorDirty = false;
let scenarioEditorJsonDirty = false;
let scenarioEditorDraft = {};

function scenarioEscape(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[character]);
}

function renderScenarioList(scenarios, maxMachines) {
  scenarioById.clear();
  for (const scenario of scenarios) scenarioById.set(scenario.id, scenario);
  document.getElementById("scenario-machine-limit").textContent = `Maximum ${maxMachines} machines per run`;
  if (!scenarios.length) {
    document.getElementById("scenario-list").innerHTML = `<p class="scenario-empty">No scenarios in this organization. Create one to get started.</p>`;
    return;
  }
  const folders = new Map();
  for (const scenario of scenarios) {
    const folder = scenario.folder?.trim() || "Unfiled";
    const entries = folders.get(folder) || [];
    entries.push(scenario);
    folders.set(folder, entries);
  }
  document.getElementById("scenario-list").innerHTML = [...folders].sort(([left], [right]) => left.localeCompare(right)).map(([folder, entries]) => {
    const cards = entries.map((scenario) => {
      const running = scenarioRun?.state === "running" || scenarioRun?.state === "cleaning";
      const selected = scenarioRun?.scenario_id === scenario.id;
      const machines = new Set([...(scenario.steps || []), ...(scenario.cleanup || [])].map((step) => step.machine).filter(Boolean)).size;
      const topology = scenario.topology;
      const topologySummary = topology?.nodes?.length ? `${topology.nodes.length} nodes · ${(topology.links || []).length} links` : "base topology";
      const published = Number(scenario.published_revision || 0);
      const canRun = scenario.organization_id === activeScenarioOrganization && published > 0 && !running;
      return `<article class="scenario-card${selected ? " selected" : ""}"><div class="scenario-card-heading"><div><span class="scenario-id">${scenarioEscape(scenario.id)}</span><h3>${scenarioEscape(scenario.name)}</h3></div><span class="scenario-step-count">${topologySummary} · ${machines}/${maxMachines} machines · ${(scenario.steps || []).length} steps</span></div><p>${scenarioEscape(scenario.description)}</p><div class="scenario-card-actions"><span class="scenario-version${published ? " published" : ""}">${published ? `Version ${published}` : `Draft r${scenario.current_revision || 1}`}</span><button type="button" data-run-scenario="${scenarioEscape(scenario.id)}" data-run-organization="${scenarioEscape(scenario.organization_id)}" data-run-published="${published > 0}" ${canRun ? "" : "disabled"}>Run</button><button class="quiet" type="button" data-edit-scenario="${scenarioEscape(scenario.id)}">Edit</button><button class="quiet danger" type="button" data-delete-scenario="${scenarioEscape(scenario.id)}" data-delete-organization="${scenarioEscape(scenario.organization_id)}" data-delete-revision="${scenario.current_revision || 1}" ${running && selected ? "disabled" : ""}>Delete</button></div></article>`;
    }).join("");
    const items = `<div class="scenario-folder-items">${cards}</div>`;
    if (folder === "Unfiled") return `<div class="scenario-unfiled">${items}</div>`;
    return `<section class="scenario-folder"><header><h3>${scenarioEscape(folder)}</h3><span>${entries.length}</span></header>${items}</section>`;
  }).join("");
}

function updateScenarioTrackScroll() {
  const frame = document.querySelector(".scenario-track-frame");
  const steps = document.getElementById("scenario-steps");
  let scrollbar = frame.querySelector(".scenario-track-scrollbar");
  let viewport = frame.querySelector(".scenario-track-viewport");
  if (!viewport) {
    scrollbar = document.createElement("div");
    scrollbar.className = "scenario-track-scrollbar";
    scrollbar.setAttribute("role", "region");
    scrollbar.setAttribute("aria-label", "Scroll scenario tracks horizontally");
    scrollbar.tabIndex = 0;
    scrollbar.append(document.createElement("div"));

    viewport = document.createElement("div");
    viewport.className = "scenario-track-viewport";
    viewport.setAttribute("role", "region");
    viewport.setAttribute("aria-label", "Scenario machine tracks");
    viewport.tabIndex = 0;
    frame.removeAttribute("role");
    frame.removeAttribute("aria-label");
    frame.removeAttribute("tabindex");
    frame.insertBefore(scrollbar, steps);
    frame.insertBefore(viewport, steps);
    viewport.append(steps);

    let syncing = false;
    const syncScroll = (source, target) => {
      if (syncing) return;
      syncing = true;
      target.scrollLeft = source.scrollLeft;
      syncing = false;
    };
    scrollbar.addEventListener("scroll", () => syncScroll(scrollbar, viewport), { passive: true });
    viewport.addEventListener("scroll", () => syncScroll(viewport, scrollbar), { passive: true });
    window.addEventListener("resize", updateScenarioTrackScroll);
  }
  scrollbar.firstElementChild.style.width = `${steps.scrollWidth}px`;
  scrollbar.hidden = steps.scrollWidth <= viewport.clientWidth + 1;
  scrollbar.scrollLeft = viewport.scrollLeft;
}

function renderScenarioRun(run) {
  scenarioRun = run || { state: "idle", steps: [] };
  const busy = scenarioRun.state === "running" || scenarioRun.state === "cleaning";
  const listedScenario = scenarioById.get(scenarioRun.scenario_id);
  const scenarioName = scenarioRun.scenario_name || listedScenario?.name || "Scenario";
  const title = document.getElementById("scenario-run-title");
  const state = document.getElementById("scenario-run-state");
  const actions = document.querySelector(".scenario-run-actions");
  const heading = title.parentElement;
  if (actions.parentElement !== heading) heading.append(actions);
  title.textContent = scenarioRun.scenario_id ? scenarioName : "Scenario";
  const stateLabel = document.createElement("span");
  stateLabel.className = "scenario-state-label";
  stateLabel.textContent = String(scenarioRun.state || "idle");
  const progress = document.createElement("span");
  progress.className = "scenario-progress";
  progress.textContent = `${scenarioRun.current_step || 0} / ${scenarioRun.total_steps || 0}`;
  progress.hidden = !scenarioRun.scenario_id || !scenarioRun.total_steps;
  state.replaceChildren(stateLabel, progress);
  state.className = `scenario-state ${scenarioEscape(scenarioRun.state || "idle")}`;
  document.getElementById("scenario-cancel").hidden = !busy;
  document.getElementById("scenario-cancel").disabled = !busy;
  document.getElementById("scenario-clear").hidden = busy || !scenarioRun.scenario_id;
  const summary = document.getElementById("scenario-run-summary");
  if (!scenarioRun.scenario_id) {
    summary.hidden = false;
    summary.innerHTML = `<span>No run selected</span>`;
  } else {
    const redundantCancellation = scenarioRun.state === "cancelled" && /^context canceled\.?$/i.test(String(scenarioRun.error || "").trim());
    const error = redundantCancellation ? "" : scenarioRun.error;
    summary.innerHTML = error ? `<p class="scenario-run-error">${scenarioEscape(error)}</p>` : "";
    summary.hidden = !error;
  }
  const results = scenarioRun.steps || [];
  const scenario = scenarioRun.scenario || listedScenario?.published_scenario || listedScenario;
  const planned = scenario ? [
    ...(scenario.steps || []).map((step) => ({ ...step, phase: "run" })),
    ...(scenario.cleanup || []).map((step) => ({ ...step, phase: "cleanup" })),
  ] : results;
  const finished = new Map(results.map((step) => [step.number, step]));
  const activeSteps = new Set(scenarioRun.active_steps || []);
  const lanes = new Map();
  planned.forEach((step, index) => {
    const phase = step.phase === "cleanup" ? "cleanup" : "run";
    const result = scenario ? finished.get(index + 1) : step;
    const active = !result && activeSteps.has(index + 1);
    const status = result?.status || (active ? "running" : "pending");
    const waiting = phase === "run" ? scenarioRun.state === "running" : busy;
    let detail = result?.error || result?.output || (active ? "Running" : status === "pending" ? waiting ? "Waiting" : "Not run" : "Completed");
    if (scenarioRun.state === "cancelled" && /^context canceled\.?$/i.test(String(detail).trim())) detail = "";
    const time = result?.finished_at ? new Date(result.finished_at).toLocaleTimeString() : "";
    const machine = step.machine || "Shared";
    const lane = lanes.get(machine) || [];
    lane.push(`<li class="scenario-step ${scenarioEscape(status)} ${phase === "cleanup" ? "cleanup" : ""}"><span class="scenario-step-mark" aria-hidden="true"></span><div class="scenario-step-copy"><div><span class="scenario-step-number">${index + 1}</span><strong>${scenarioEscape(step.action.replaceAll("_", " "))}</strong>${step.component ? `<span>${scenarioEscape(step.component)}</span>` : ""}${phase === "cleanup" ? `<span>Cleanup</span>` : ""}</div>${detail ? `<p>${scenarioEscape(detail)}</p>` : ""}</div><time>${scenarioEscape(time)}</time></li>`);
    lanes.set(machine, lane);
  });
  document.getElementById("scenario-steps").innerHTML = [...lanes].map(([machine, steps]) => `<section class="scenario-lane"><h3>${scenarioEscape(machine)}</h3><ol class="scenario-steps">${steps.join("")}</ol></section>`).join("");
  updateScenarioTrackScroll();
  for (const button of document.querySelectorAll("[data-run-scenario]")) {
    button.disabled = busy || button.dataset.runOrganization !== activeScenarioOrganization || button.dataset.runPublished !== "true";
  }
}

async function refreshScenarios() {
  if (scenarioRefreshPromise) return scenarioRefreshPromise;
  scenarioRefreshPromise = (async () => {
    const error = document.getElementById("scenario-error");
    try {
      const response = await fetch("/api/simulator/scenarios", { cache: "no-store" });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
      error.hidden = true;
      activeScenarioOrganization = data.active_organization_id || activeScenarioOrganization;
      if (scenarioEditorOrganization && scenarioEditorOrganization !== activeScenarioOrganization && document.getElementById("scenario-editor-dialog").open) {
        document.getElementById("scenario-editor-dialog").close();
      }
      const scenarios = data.scenarios || [];
      const catalogSignature = JSON.stringify({ scenarios, active: activeScenarioOrganization });
      const catalogChanged = catalogSignature !== scenarioCatalogSignature;
      if (catalogChanged) {
        scenarioCatalogSignature = catalogSignature;
        renderScenarioList(scenarios, data.max_machines || 10);
      }
      const runSignature = JSON.stringify(data.run || {});
      if (runSignature !== scenarioRunSignature || catalogChanged) {
        scenarioRunSignature = runSignature;
        renderScenarioRun(data.run);
      }
      const organizationName = data.organization?.name || activeScenarioOrganization;
      document.getElementById("scenario-organization").textContent = organizationName;
      document.getElementById("scenario-connection").textContent = `${organizationName}: ${scenarios.length} scenarios`;
    } catch (failure) {
      error.textContent = failure.message || "Scenario catalog unavailable";
      error.hidden = false;
      document.getElementById("scenario-connection").textContent = "Scenario service unavailable";
    }
  })();
  try { await scenarioRefreshPromise; } finally { scenarioRefreshPromise = undefined; }
}

function setScenarioEditorStatus(message, state = "") {
  const status = document.getElementById("scenario-editor-status");
  status.textContent = message;
  status.dataset.state = state;
}

function updateScenarioEditorActions() {
  const publish = document.getElementById("scenario-editor-publish");
  const saved = scenarioEditorArtifact && !scenarioEditorDirty;
  publish.disabled = !saved || scenarioEditorArtifact.published_revision === scenarioEditorArtifact.revision;
  document.getElementById("scenario-editor-delete").hidden = !scenarioEditorArtifact;
}

const scenarioStepActions = [
  "start_machine", "wait_controller", "login", "select_workloads", "logout", "stop_machine",
  "start_workload", "stop_workload", "start_test_service", "stop_test_service",
  "request_test_service", "benchmark_test_service", "start_service_fleet",
  "stop_service_fleet", "stress_traffic", "traffic", "delay",
];

function scenarioStepMarkup(step, index) {
  const actions = step.action && !scenarioStepActions.includes(step.action) ? [...scenarioStepActions, step.action] : scenarioStepActions;
  const actionOptions = actions.map((action) => `<option value="${scenarioEscape(action)}" ${step.action === action ? "selected" : ""}>${scenarioEscape(action.replaceAll("_", " "))}</option>`).join("");
  const expectedOptions = ["", "allow", "deny"].map((value) => `<option value="${value}" ${step.expected === value ? "selected" : ""}>${value || "Not set"}</option>`).join("");
  const values = {
    id: step.id || "",
    machine: step.machine || "",
    user: step.user || "",
    component: step.component || "",
    target: step.target || "",
    timeout_seconds: step.timeout_seconds ?? "",
    workloads: Array.isArray(step.workloads) ? step.workloads.join(", ") : "",
    after: Array.isArray(step.after) ? step.after.join(", ") : "",
  };
  return `<fieldset class="scenario-step-editor-row" data-scenario-step data-original-step="${scenarioEscape(JSON.stringify(step))}"><legend>Step ${index + 1}<button class="quiet" type="button" data-remove-step aria-label="Remove step" title="Remove step">×</button></legend><div class="scenario-step-fields"><label data-parallel-field>Step ID<input data-step-key="id" value="${scenarioEscape(values.id)}" placeholder="step-id"></label><label>Action<select data-step-key="action">${actionOptions}</select></label><label>Machine<input data-step-key="machine" value="${scenarioEscape(values.machine)}" placeholder="machine-01"></label><label>User<input data-step-key="user" value="${scenarioEscape(values.user)}" placeholder="Optional user"></label><label>Workload component<input data-step-key="component" value="${scenarioEscape(values.component)}" placeholder="Optional component"></label><label>Target<input data-step-key="target" value="${scenarioEscape(values.target)}" placeholder="Target or service"></label><label>Expected result<select data-step-key="expected">${expectedOptions}</select></label><label>Timeout (seconds)<input data-step-key="timeout_seconds" type="number" min="0" max="180" value="${scenarioEscape(values.timeout_seconds)}"></label><label>Workloads<input data-step-key="workloads" value="${scenarioEscape(values.workloads)}" placeholder="client, service"></label><label data-parallel-field>After step IDs<input data-step-key="after" value="${scenarioEscape(values.after)}" placeholder="step-id, other-step"></label></div></fieldset>`;
}

function renderScenarioStepGroup(containerID, steps) {
  const container = document.getElementById(containerID);
  container.innerHTML = steps.length ? steps.map(scenarioStepMarkup).join("") : `<p class="scenario-step-empty">No steps yet.</p>`;
  updateScenarioParallelFields();
}

function updateScenarioParallelFields() {
  const parallel = document.getElementById("scenario-editor-parallel").checked;
  for (const field of document.querySelectorAll("[data-parallel-field]")) field.hidden = !parallel;
}

function renderScenarioEditor(scenario) {
  scenarioEditorDraft = { ...scenario };
  document.getElementById("scenario-editor-id").value = scenario.id || "";
  document.getElementById("scenario-editor-id").disabled = Boolean(scenarioEditorArtifact);
  document.getElementById("scenario-editor-name").value = scenario.name || "";
  document.getElementById("scenario-editor-folder").value = scenario.folder || "";
  document.getElementById("scenario-editor-description").value = scenario.description || "";
  document.getElementById("scenario-editor-parallel").checked = Boolean(scenario.parallel);
  renderScenarioStepGroup("scenario-editor-steps", scenario.steps || []);
  renderScenarioStepGroup("scenario-editor-cleanup", scenario.cleanup || []);
  document.getElementById("scenario-editor-source").value = JSON.stringify(scenario, null, 2);
  scenarioEditorJsonDirty = false;
}

function readScenarioSteps(containerID) {
  return [...document.querySelectorAll(`#${containerID} [data-scenario-step]`)].map((row) => {
    let step = {};
    try { step = JSON.parse(row.dataset.originalStep || "{}"); } catch { step = {}; }
    for (const input of row.querySelectorAll("[data-step-key]")) {
      const key = input.dataset.stepKey;
      const value = input.value.trim();
      if (!value) {
        delete step[key];
        continue;
      }
      if (key === "timeout_seconds") step[key] = Number(value);
      else if (key === "workloads" || key === "after") step[key] = value.split(",").map((entry) => entry.trim()).filter(Boolean);
      else step[key] = value;
    }
    return step;
  });
}

function readScenarioEditorForm() {
  const parallel = document.getElementById("scenario-editor-parallel").checked;
  const steps = readScenarioSteps("scenario-editor-steps");
  if (!parallel) {
    for (const step of steps) {
      delete step.id;
      delete step.after;
    }
  }
  const scenario = {
    ...scenarioEditorDraft,
    id: document.getElementById("scenario-editor-id").value.trim(),
    organization_id: scenarioEditorOrganization,
    name: document.getElementById("scenario-editor-name").value.trim(),
    description: document.getElementById("scenario-editor-description").value.trim(),
    steps,
    cleanup: readScenarioSteps("scenario-editor-cleanup"),
  };
  const folder = document.getElementById("scenario-editor-folder").value.trim();
  if (folder) scenario.folder = folder;
  else delete scenario.folder;
  if (parallel) scenario.parallel = true;
  else delete scenario.parallel;
  return scenario;
}

function markScenarioEditorDirty(fromJSON = false) {
  scenarioEditorDirty = true;
  if (!fromJSON) {
    scenarioEditorJsonDirty = false;
    document.getElementById("scenario-editor-source").value = JSON.stringify(readScenarioEditorForm(), null, 2);
  }
  updateScenarioEditorActions();
}

function addScenarioStep(cleanup = false) {
  const scenario = readScenarioEditorForm();
  const steps = cleanup ? scenario.cleanup : scenario.steps;
  steps.push(cleanup ? { action: "stop_machine" } : { action: "delay", timeout_seconds: 1 });
  renderScenarioStepGroup(cleanup ? "scenario-editor-cleanup" : "scenario-editor-steps", steps);
  markScenarioEditorDirty();
}

function openNewScenarioEditor() {
  scenarioEditorArtifact = null;
  scenarioEditorOrganization = activeScenarioOrganization;
  scenarioEditorDirty = false;
  scenarioAssistant.reset();
  document.getElementById("scenario-editor-title").textContent = "New scenario";
  document.getElementById("scenario-editor-advanced").open = false;
  renderScenarioEditor({
    id: "new-scenario",
    organization_id: scenarioEditorOrganization,
    folder: "",
    name: "New scenario",
    description: "Describe the behavior this scenario exercises.",
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  });
  document.getElementById("scenario-editor-summary").value = "Initial version";
  document.getElementById("scenario-editor-revisions").replaceChildren(new Option("Not saved yet", ""));
  document.getElementById("scenario-editor-revisions").disabled = true;
  document.getElementById("scenario-editor-save").textContent = "Create scenario";
  setScenarioEditorStatus("Save a draft, then publish it before running.");
  updateScenarioEditorActions();
  document.getElementById("scenario-editor-dialog").showModal();
}

async function openExistingScenarioEditor(scenarioID) {
  const organizationID = activeScenarioOrganization;
  const path = `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios/${encodeURIComponent(scenarioID)}`;
  const response = await fetch(path, { cache: "no-store" });
  const artifact = await response.json();
  if (!response.ok) throw new Error(artifact.error || `HTTP ${response.status}`);
  scenarioEditorArtifact = artifact;
  scenarioEditorOrganization = organizationID;
  scenarioEditorDirty = false;
  scenarioAssistant.reset();
  document.getElementById("scenario-editor-advanced").open = false;
  document.getElementById("scenario-editor-title").textContent = `Edit ${artifact.content.name}`;
  renderScenarioEditor(artifact.content);
  document.getElementById("scenario-editor-summary").value = "";
  document.getElementById("scenario-editor-save").textContent = "Save new version";
  document.getElementById("scenario-editor-dialog").showModal();
  await refreshScenarioRevisions();
  updateScenarioEditorActions();
}

async function refreshScenarioRevisions() {
  const selector = document.getElementById("scenario-editor-revisions");
  const artifact = scenarioEditorArtifact;
  if (!artifact) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(artifact.id)}/revisions`;
  const response = await fetch(path, { cache: "no-store" });
  const revisions = await response.json();
  if (!response.ok) throw new Error(revisions.error || `HTTP ${response.status}`);
  selector.replaceChildren(...revisions.map((revision) => new Option(`r${revision.revision} · ${revision.summary || revision.author}`, String(revision.revision))));
  selector.disabled = revisions.length === 0;
}

function readScenarioEditorSource() {
  let scenario;
  try { scenario = JSON.parse(document.getElementById("scenario-editor-source").value); }
  catch { throw new Error("Scenario must be valid JSON."); }
  if (!scenario || typeof scenario !== "object" || Array.isArray(scenario)) throw new Error("Scenario JSON must be an object.");
  if (scenario.organization_id && scenario.organization_id !== scenarioEditorOrganization) throw new Error("Scenario organization_id must match the selected organization.");
  scenario.organization_id = scenarioEditorOrganization;
  return scenario;
}

async function saveScenarioDraft() {
  const scenario = scenarioEditorJsonDirty ? readScenarioEditorSource() : readScenarioEditorForm();
  const summary = document.getElementById("scenario-editor-summary").value.trim();
  const creating = !scenarioEditorArtifact;
  const path = creating
    ? `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios`
    : `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}`;
  const response = await fetch(path, {
    method: creating ? "POST" : "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ scenario, summary, expected_revision: scenarioEditorArtifact?.revision || 0 }),
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
  if (creating) scenarioEditorArtifact = result;
  else scenarioEditorArtifact = { ...scenarioEditorArtifact, revision: result.revision, content: result.content, content_hash: result.content_hash, published_revision: scenarioEditorArtifact.published_revision || 0 };
  scenarioEditorDraft = scenario;
  scenarioEditorDirty = false;
  renderScenarioEditor(scenario);
  document.getElementById("scenario-editor-summary").value = "";
  document.getElementById("scenario-editor-title").textContent = `Edit ${scenario.name}`;
  document.getElementById("scenario-editor-save").textContent = "Save new version";
  setScenarioEditorStatus(`Saved draft revision ${scenarioEditorArtifact.revision}. Publish it to enable runs.`, "saved");
  await refreshScenarioRevisions();
  updateScenarioEditorActions();
  await refreshScenarios();
}

async function publishScenarioRevision() {
  if (!scenarioEditorArtifact || scenarioEditorDirty) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}/publish`;
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_revision: scenarioEditorArtifact.revision }),
  });
  const artifact = await response.json();
  if (!response.ok) throw new Error(artifact.error || `HTTP ${response.status}`);
  scenarioEditorArtifact = artifact;
  setScenarioEditorStatus(`Published revision ${artifact.published_revision} for ${scenarioEditorOrganization}.`, "saved");
  updateScenarioEditorActions();
  await refreshScenarios();
}

async function loadScenarioRevision(number) {
  if (!number || !scenarioEditorArtifact) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}/revisions/${encodeURIComponent(number)}`;
  const response = await fetch(path, { cache: "no-store" });
  const revision = await response.json();
  if (!response.ok) throw new Error(revision.error || `HTTP ${response.status}`);
  renderScenarioEditor(revision.content);
  document.getElementById("scenario-editor-summary").value = `Restore revision ${revision.revision}`;
  scenarioEditorDirty = true;
  setScenarioEditorStatus(`Viewing revision ${revision.revision}; save to create a new version.`);
  updateScenarioEditorActions();
}

async function archiveScenario(scenarioID, organizationID, revision) {
  const scenario = scenarioById.get(scenarioID);
  if (!scenario || !window.confirm(`Delete “${scenario.name}” from this folder? Its revision history will be retained.`)) return;
  const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios/${encodeURIComponent(scenarioID)}`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_revision: Number(revision) }),
  });
  if (!response.ok) {
    const result = await response.json();
    throw new Error(result.error || `HTTP ${response.status}`);
  }
  if (scenarioEditorArtifact?.id === scenarioID) document.getElementById("scenario-editor-dialog").close();
  await refreshScenarios();
}

const scenarioAssistantSlot = document.createElement("div");
scenarioAssistantSlot.id = "scenario-assistant-slot";
document.getElementById("scenario-editor-advanced").before(scenarioAssistantSlot);
const scenarioAssistant = window.mountSimulatorDesignAssistant("scenario-assistant-slot", {
  scope: "scenario",
  applyLabel: "Apply scenario draft",
  emptyMessage: "Ask for a scenario outline, step plan, or review of this draft.",
  getContext: () => ({
    organization_id: scenarioEditorOrganization,
    scenario: scenarioEditorJsonDirty ? readScenarioEditorSource() : readScenarioEditorForm(),
  }),
  onApply: (proposal) => {
    const scenario = proposal.scenario;
    if (!scenario) throw new Error("Claude did not return a scenario draft.");
    if (scenarioEditorArtifact && scenario.id !== scenarioEditorArtifact.id) throw new Error("The proposal changed the existing scenario ID.");
    scenario.organization_id = scenarioEditorOrganization;
    renderScenarioEditor(scenario);
    scenarioEditorDirty = true;
    document.getElementById("scenario-editor-summary").value = "Claude-assisted scenario draft";
    setScenarioEditorStatus("Claude proposal applied to the form. Review and save it as a draft.");
    updateScenarioEditorActions();
  },
});

async function postScenarioAction(url) {
  const response = await fetch(url, { method: "POST" });
  const body = await response.json();
  if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`);
  await refreshScenarios();
}


document.getElementById("scenario-refresh").addEventListener("click", refreshScenarios);
document.getElementById("scenario-new").addEventListener("click", openNewScenarioEditor);
document.getElementById("scenario-editor-dialog").addEventListener("input", (event) => {
  if (event.target.id === "scenario-editor-source") {
    scenarioEditorJsonDirty = true;
    markScenarioEditorDirty(true);
    return;
  }
  if (event.target.closest("[data-scenario-step]") || ["scenario-editor-id", "scenario-editor-name", "scenario-editor-folder", "scenario-editor-description"].includes(event.target.id)) {
    markScenarioEditorDirty();
  }
});
document.getElementById("scenario-editor-dialog").addEventListener("change", (event) => {
  if (event.target.id === "scenario-editor-parallel") {
    updateScenarioParallelFields();
    markScenarioEditorDirty();
  } else if (event.target.matches("[data-step-key]")) {
    markScenarioEditorDirty();
  }
});
document.getElementById("scenario-editor-advanced").addEventListener("toggle", (event) => {
  if (event.currentTarget.open || !scenarioEditorJsonDirty) return;
  try {
    const scenario = readScenarioEditorSource();
    scenarioEditorDirty = true;
    renderScenarioEditor(scenario);
    setScenarioEditorStatus("Advanced JSON loaded into the builder. Save to create a new draft revision.");
    updateScenarioEditorActions();
  } catch (error) {
    event.currentTarget.open = true;
    setScenarioEditorStatus(error.message || "Scenario must be valid JSON.", "error");
  }
});
document.getElementById("scenario-editor-dialog").addEventListener("click", (event) => {
  if (event.target.closest("#scenario-add-step")) {
    addScenarioStep();
    return;
  }
  if (event.target.closest("#scenario-add-cleanup")) {
    addScenarioStep(true);
    return;
  }
  const removeButton = event.target.closest("[data-remove-step]");
  if (!removeButton) return;
  const row = removeButton.closest("[data-scenario-step]");
  const container = row.parentElement;
  const isCleanup = container.id === "scenario-editor-cleanup";
  const scenario = readScenarioEditorForm();
  const steps = isCleanup ? scenario.cleanup : scenario.steps;
  steps.splice([...container.querySelectorAll("[data-scenario-step]")].indexOf(row), 1);
  renderScenarioStepGroup(isCleanup ? "scenario-editor-cleanup" : "scenario-editor-steps", steps);
  markScenarioEditorDirty();
});
document.getElementById("scenario-editor-save").addEventListener("click", async () => {
  try { await saveScenarioDraft(); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not save scenario.", "error"); }
});
document.getElementById("scenario-editor-publish").addEventListener("click", async () => {
  try { await publishScenarioRevision(); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not publish scenario.", "error"); }
});
document.getElementById("scenario-editor-revisions").addEventListener("change", async (event) => {
  try { await loadScenarioRevision(event.target.value); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not load revision.", "error"); }
});
document.getElementById("scenario-editor-delete").addEventListener("click", async () => {
  if (!scenarioEditorArtifact) return;
  try { await archiveScenario(scenarioEditorArtifact.id, scenarioEditorOrganization, scenarioEditorArtifact.revision); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not delete scenario.", "error"); }
});
document.getElementById("scenario-list").addEventListener("click", async (event) => {
  const editButton = event.target.closest("[data-edit-scenario]");
  if (editButton) {
    try { await openExistingScenarioEditor(editButton.dataset.editScenario); }
    catch (error) {
      const message = document.getElementById("scenario-error");
      message.textContent = error.message || "Could not open scenario";
      message.hidden = false;
    }
    return;
  }
  const deleteButton = event.target.closest("[data-delete-scenario]");
  if (deleteButton) {
    deleteButton.disabled = true;
    try {
      await archiveScenario(deleteButton.dataset.deleteScenario, deleteButton.dataset.deleteOrganization, deleteButton.dataset.deleteRevision);
    } catch (error) {
      const message = document.getElementById("scenario-error");
      message.textContent = error.message || "Could not delete scenario";
      message.hidden = false;
    } finally {
      await refreshScenarios();
    }
    return;
  }
  const button = event.target.closest("[data-run-scenario]");
  if (!button) return;
  button.disabled = true;
  try {
    await postScenarioAction(`/api/simulator/scenarios/${encodeURIComponent(button.dataset.runScenario)}/run`);
  } catch (error) {
    const message = document.getElementById("scenario-error");
    message.textContent = error.message || "Could not start scenario";
    message.hidden = false;
  } finally {
    await refreshScenarios();
  }
});
document.getElementById("scenario-cancel").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  try {
    await postScenarioAction("/api/simulator/scenarios/cancel");
  } catch (error) {
    const message = document.getElementById("scenario-error");
    message.textContent = error.message || "Could not cancel scenario";
    message.hidden = false;
  }
});
document.getElementById("scenario-clear").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  try {
    await postScenarioAction("/api/simulator/scenarios/clear");
    await refreshScenarios();
  } catch (error) {
    const message = document.getElementById("scenario-error");
    message.textContent = error.message || "Could not clear scenario run";
    message.hidden = false;
  } finally {
    button.disabled = false;
  }
});
document.addEventListener("simulator:activate", (event) => {
  if (event.detail.path !== "/scenarios.html" || scenarioRefreshTimer) return;
  refreshScenarios();
  scenarioRefreshTimer = setInterval(refreshScenarios, 1000);
});
document.addEventListener("simulator:deactivate", (event) => {
  if (event.detail.path !== "/scenarios.html") return;
  clearInterval(scenarioRefreshTimer);
  scenarioRefreshTimer = undefined;
});