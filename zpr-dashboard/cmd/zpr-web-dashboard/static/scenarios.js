const scenarioById = new Map();
let scenarioRun;
let scenarioCatalogSignature = "";
let scenarioRunSignature = "";
let activeScenarioOrganization = "";
let scenarioLogContext = "";
let scenarioLogMachines = new Map();
let scenarioLogError = "";
let scenarioLogsLoaded = false;
let scenarioEditorArtifact;
let scenarioEditorOrganization = "";
let scenarioEditorDirty = false;
let scenarioEditorJsonDirty = false;
let scenarioEditorDraft = {};
let scenarioEditorSaved = null;
let scenarioEditorSummary = "";
let scenarioEditorViewing = 0;
const scenarioEditorPage = window.ZPREditorPage;
const scenarioRequest = (path, options) => scenarioEditorPage.requestJSON(
  (...args) => window.fetch(...args), path, options,
);
const scenarioEditorController = scenarioEditorPage.createController({
  source: document.getElementById("scenario-editor-source"),
  status: document.getElementById("scenario-editor-status"),
  isDirty: () => scenarioEditorDirty,
  identity: { title: document.getElementById("scenario-editor-title"), version: document.getElementById("scenario-editor-revision-label"), modified: document.getElementById("scenario-editor-modified") },
  menu: { root: document.getElementById("scenario-editor-actions"), toggle: document.getElementById("scenario-editor-files-toggle"), menu: document.getElementById("scenario-editor-file-menu") },
  history: { menu: document.getElementById("scenario-editor-history-menu"), list: document.getElementById("scenario-editor-history"), count: document.getElementById("scenario-editor-history-count"), isAvailable: () => Boolean(scenarioEditorArtifact) },
  adapters: { load: openExistingScenarioEditor, analyze: analyzeScenarioEditor, save: saveScenarioDraft, render: updateScenarioEditorActions },
  syncControls: updateScenarioEditorActions,
});
const scenarioFormAnalysisScope = scenarioEditorController.createScope("form", () => [
  JSON.stringify(readScenarioEditorForm()), scenarioEditorOrganization,
  scenarioEditorArtifact?.id, scenarioEditorArtifact?.revision, scenarioEditorViewing,
  document.getElementById("scenario-editor-dialog").open,
  document.getElementById("scenario-editor-advanced").open,
]);
const scenarioHistory = scenarioEditorController.history;

function scenarioEscape(value) {
  return window.ZPRSafeDisplay.escapeHTML(value);
}

function scenarioPreviewContext() {
  return JSON.stringify([activeScenarioOrganization, scenarioRun?.scenario_id, scenarioRun?.started_at]);
}

function renderScenarioLogPreviews() {
  const busy = ["running", "cleaning"].includes(scenarioRun?.state);
  for (const lane of document.querySelectorAll(".scenario-lane[data-scenario-machine]")) {
    const existing = lane.querySelector(".scenario-log-preview");
    if (!busy) {
      existing?.remove();
      continue;
    }
    const id = lane.dataset.scenarioMachine;
    const machine = scenarioLogMachines.get(id);
    if (machine && machine.state !== "running") {
      existing?.remove();
      continue;
    }
    const preview = existing || document.createElement("div");
    preview.className = "scenario-log-preview";
    preview.setAttribute("role", "region");
    preview.setAttribute("aria-label", `Log preview for ${id}`);
    const status = preview.querySelector("span") || document.createElement("span");
    status.className = "scenario-log-preview-status";
    const sources = machine?.sources || [];
    const sourceErrors = sources.filter(source => source.error);
    status.textContent = scenarioLogError || (sourceErrors.length ? sourceErrors.map(source => `${source.name}: ${source.error}`).join("; ")
      : !machine ? scenarioLogsLoaded ? "Machine logs unavailable" : "Loading logs…" : "Live logs");
    preview.classList.toggle("unavailable", Boolean(scenarioLogError || sourceErrors.length || scenarioLogsLoaded && !machine));
    const output = preview.querySelector("pre") || document.createElement("pre");
    output.textContent = sources.flatMap(source => (source.lines || []).slice(-8).map(line => `${source.name}: ${line}`))
      .slice(-12).join("\n").slice(-8192) || (machine ? "No log entries." : "");
    const link = preview.querySelector("a") || document.createElement("a");
    link.href = "/machine-logs.html";
    link.dataset.simulatorNav = "";
    link.textContent = "Workers";
    link.setAttribute("aria-label", `Open Workers logs for ${id}`);
    if (!existing) {
      preview.append(status, output, link);
      lane.prepend(preview);
    }
    output.scrollTop = output.scrollHeight;
  }
  updateScenarioTrackScroll();
}

async function refreshScenarioLogPreviews({ signal, isCurrent }) {
  if (!["running", "cleaning"].includes(scenarioRun?.state)) return;
  const context = scenarioPreviewContext();
  let data;
  try {
    data = await scenarioRequest("/api/simulator/machine-logs", { cache: "no-store", signal });
  } catch (failure) {
    if (!isCurrent() || context !== scenarioPreviewContext() || !["running", "cleaning"].includes(scenarioRun?.state)) return;
    scenarioLogError = failure.message || "Machine logs unavailable";
    renderScenarioLogPreviews();
    return;
  }
  if (!isCurrent() || context !== scenarioPreviewContext() || !["running", "cleaning"].includes(scenarioRun?.state)) return;
  if (!Array.isArray(data?.machines) || data.machines.some(entry => !entry?.machine?.id || !Array.isArray(entry.sources)
    || entry.sources.some(source => !Array.isArray(source?.lines) || source.lines.some(line => typeof line !== "string")))) {
    throw new Error("Invalid machine log response");
  }
  scenarioLogsLoaded = true;
  if (data.organization_id !== activeScenarioOrganization) {
    scenarioLogMachines.clear();
    scenarioLogError = "Logs unavailable for this organization";
  } else {
    scenarioLogMachines = new Map((data.machines || []).map(entry => [entry.machine.id, entry]));
    scenarioLogError = "";
  }
  renderScenarioLogPreviews();
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
  const previousProgress = JSON.stringify([scenarioRun?.scenario_id, scenarioRun?.state, scenarioRun?.current_step, scenarioRun?.results]);
  scenarioRun = run || { state: "idle", steps: [] };
  const logContext = scenarioPreviewContext();
  if (scenarioLogContext !== logContext) {
    scenarioLogContext = logContext;
    scenarioLogMachines.clear();
    scenarioLogError = "";
    scenarioLogsLoaded = false;
  }
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
  const failures = (scenarioRun.steps || []).filter(step => step.status === "failed");
  const firstFailure = failures.find(step => step.phase !== "cleanup") || failures[0];
  const failedNumber = firstFailure?.number;
  const showFailure = firstFailure && (scenarioRun.state === "failed" || scenarioRun.state === "cleaning");
  progress.textContent = showFailure
    ? `Failed step ${failedNumber} / ${scenarioRun.total_steps || 0}${scenarioRun.state === "cleaning" ? ` · Cleanup ${scenarioRun.current_step || 0} / ${scenarioRun.total_steps || 0}` : ""}`
    : `${scenarioRun.current_step || 0} / ${scenarioRun.total_steps || 0}`;
  progress.hidden = !scenarioRun.scenario_id || !scenarioRun.total_steps;
  state.replaceChildren(stateLabel, progress);
  state.className = `scenario-state ${scenarioEscape(scenarioRun.state || "idle")}`;
  if (busy && previousProgress !== JSON.stringify([scenarioRun.scenario_id, scenarioRun.state, scenarioRun.current_step, scenarioRun.results])
    && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    state.animate([
      { opacity: 0.8, transform: "scale(1)", boxShadow: "inset 0 0 0 1px #1251a0" },
      { opacity: 1, transform: "scale(1.05)", boxShadow: "0 0 0 3px #2473d866, inset 0 0 0 1px #1251a0", offset: 0.45 },
      { opacity: 1, transform: "scale(1)", boxShadow: "inset 0 0 0 1px #1251a0" },
    ], { duration: 750, easing: "ease-in-out" });
  }
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
    const failureDetail = showFailure
      ? `Step ${failedNumber}: ${String(firstFailure.action || "unknown action").replaceAll("_", " ")}${firstFailure.machine ? ` on ${firstFailure.machine}` : ""}${firstFailure.error ? ` — ${firstFailure.error}` : ""}`
      : "";
    summary.innerHTML = `${failureDetail ? `<p class="scenario-run-error">${scenarioEscape(failureDetail)}</p>` : ""}${error && error !== firstFailure?.error ? `<p class="scenario-run-error">${scenarioEscape(error)}</p>` : ""}`;
    summary.hidden = !error && !failureDetail;
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
    const time = result?.finished_at ? window.ZPRSafeDisplay.formatTime(result.finished_at) : "";
    const machine = step.machine || "Shared";
    const lane = lanes.get(machine) || [];
    lane.push(`<li class="scenario-step ${scenarioEscape(status)} ${phase === "cleanup" ? "cleanup" : ""}"><span class="scenario-step-mark" aria-hidden="true"></span><div class="scenario-step-copy"><div><span class="scenario-step-number">${index + 1}</span><strong>${scenarioEscape(step.action.replaceAll("_", " "))}</strong>${step.component ? `<span>${scenarioEscape(step.component)}</span>` : ""}${phase === "cleanup" ? `<span>Cleanup</span>` : ""}</div>${detail ? `<p>${scenarioEscape(detail)}</p>` : ""}</div><time>${scenarioEscape(time)}</time></li>`);
    lanes.set(machine, lane);
  });
  document.getElementById("scenario-steps").innerHTML = [...lanes].map(([machine, steps]) => `<section class="scenario-lane"${machine === "Shared" ? "" : ` data-scenario-machine="${scenarioEscape(machine)}"`}><h3>${scenarioEscape(machine)}</h3><ol class="scenario-steps">${steps.join("")}</ol></section>`).join("");
  renderScenarioLogPreviews();
  updateScenarioTrackScroll();
  for (const button of document.querySelectorAll("[data-run-scenario]")) {
    button.disabled = busy || button.dataset.runOrganization !== activeScenarioOrganization || button.dataset.runPublished !== "true";
  }
}

async function refreshScenarios({ signal, isCurrent }) {
  const data = await scenarioRequest("/api/simulator/scenarios", { cache: "no-store", signal });
  if (!isCurrent()) return;
  const error = document.getElementById("scenario-error");
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
}

const scenarioPoller = window.ZPRSimulatorNavigation.createPagePoller({
  path: "/scenarios.html",
  run: refreshScenarios,
  interval: 1000,
  onError: (failure) => {
    const error = document.getElementById("scenario-error");
    error.textContent = failure.message || "Scenario catalog unavailable";
    error.hidden = false;
    document.getElementById("scenario-connection").textContent = "Scenario service unavailable";
  },
});
const refreshScenariosNow = () => scenarioPoller.refresh();
window.ZPRSimulatorNavigation.createPagePoller({
  path: "/scenarios.html",
  run: refreshScenarioLogPreviews,
  interval: 3000,
  onError: failure => {
    if (!["running", "cleaning"].includes(scenarioRun?.state)) return;
    scenarioLogError = failure.message || "Machine logs unavailable";
    renderScenarioLogPreviews();
  },
});

function setScenarioEditorStatus(message, state = "") {
  scenarioEditorController.setStatus(message || "", state === "saved" ? "success" : state);
}

function renderScenarioIdentity() {
  const artifact = scenarioEditorArtifact;
  const name = document.getElementById("scenario-editor-name").value.trim();
  let label = "New · unsaved";
  if (artifact) {
    const published = artifact.published_revision === artifact.revision ? "published"
      : artifact.published_revision ? `published v${artifact.published_revision}` : "not published";
    label = scenarioEditorViewing ? `Viewing version ${scenarioEditorViewing}` : `Version ${artifact.revision} · ${published}`;
  }
  scenarioEditorController.renderIdentity({ name, label, tooltip: artifact?.id || "", dirty: scenarioEditorDirty });
}

function setScenarioAnalyzeState(state = "") {
  for (const button of document.querySelectorAll("#scenario-source-analyze")) {
    if (!button) continue;
    scenarioEditorController.setAnalysisState(state);
    button.classList.toggle("button-next-evaluate", !state);
  }
}

function updateScenarioEditorActions() {
  const publish = document.getElementById("scenario-editor-publish");
  const saved = scenarioEditorArtifact && !scenarioEditorDirty;
  publish.disabled = !saved || scenarioEditorArtifact.published_revision === scenarioEditorArtifact.revision;
  document.getElementById("scenario-editor-delete").hidden = !scenarioEditorArtifact;
  document.getElementById("scenario-editor-discard").disabled = !scenarioEditorDirty;
  renderScenarioIdentity();
}

const scenarioStepActions = [
  "start_machine", "wait_controller", "login", "select_workloads", "logout", "stop_machine",
  "start_workload", "stop_workload", "start_test_service", "stop_test_service",
  "request_test_service", "benchmark_test_service", "start_service_fleet",
  "stop_service_fleet", "stress_traffic", "traffic", "resolve_dns", "delay",
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
  scenarioFormAnalysisScope.invalidate();
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
  if (window.setScenarioSourceFromScenario) window.setScenarioSourceFromScenario(scenario);
  else window.renderScenarioSourceEditor?.();
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
  scenarioFormAnalysisScope.invalidate();
  setScenarioAnalyzeState("pending");
  if (!fromJSON) {
    scenarioEditorJsonDirty = false;
    document.getElementById("scenario-editor-source").value = JSON.stringify(readScenarioEditorForm(), null, 2);
  }
  updateScenarioEditorActions();
  window.renderScenarioSourceEditor?.();
}

async function analyzeScenarioEditor() {
  if (document.getElementById("scenario-editor-advanced").open && window.analyzeScenarioSourceEditor) {
    const result = await window.analyzeScenarioSourceEditor();
    return result?.scenario || null;
  }
  const isCurrent = scenarioEditorController.beginAnalysis("form");
  setScenarioEditorStatus("");
  try {
    const scenario = readScenarioEditorForm();
    const result = await scenarioRequest(`/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenario-check`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ format: "json", source: JSON.stringify(scenario), scenario_id: scenarioEditorArtifact?.id || "" }),
    });
    if (!isCurrent()) return null;
    if (result.valid !== true) throw new Error(result.error || "Scenario analysis failed.");
    setScenarioAnalyzeState("success");
    setScenarioEditorStatus(result.diagnostics || "Scenario definition valid. Nothing saved, published, or run.");
    return result.scenario || scenario;
  } catch (error) {
    if (!isCurrent()) return null;
    setScenarioAnalyzeState("error");
    setScenarioEditorStatus(error.message || "Scenario analysis failed.", "error");
    return null;
  } finally {
    isCurrent.finish();
  }
}

function addScenarioStep(cleanup = false) {
  const scenario = readScenarioEditorForm();
  const steps = cleanup ? scenario.cleanup : scenario.steps;
  steps.push(cleanup ? { action: "stop_machine" } : { action: "delay", timeout_seconds: 1 });
  renderScenarioStepGroup(cleanup ? "scenario-editor-cleanup" : "scenario-editor-steps", steps);
  markScenarioEditorDirty();
}

function showScenarioEditorPage() {
  const dialog = document.getElementById("scenario-editor-dialog");
  if (!dialog.open) dialog.show();
  window.scrollTo(0, 0);
}

function closeScenarioEditorPage() {
  if (!scenarioEditorPage.confirmDiscard(scenarioEditorDirty, "Discard unsaved scenario changes?")) return false;
  scenarioEditorDirty = false;
  document.getElementById("scenario-editor-dialog").close();
  return true;
}

function newScenarioTemplate() {
  return {
    id: "new-scenario",
    organization_id: scenarioEditorOrganization,
    folder: "",
    name: "",
    description: "Describe the behavior this scenario exercises.",
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  };
}

function openNewScenarioEditor() {
  scenarioEditorArtifact = null;
  scenarioEditorOrganization = activeScenarioOrganization;
  scenarioEditorDirty = false;
  scenarioEditorViewing = 0;
  scenarioEditorSummary = "Initial version";
  scenarioAssistant.reset();
  document.getElementById("scenario-editor-advanced").open = false;
  scenarioEditorSaved = newScenarioTemplate();
  renderScenarioEditor(scenarioEditorSaved);
  scenarioHistory.render([]);
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  showScenarioEditorPage();
}

async function openExistingScenarioEditor(scenarioID) {
  const organizationID = activeScenarioOrganization;
  const path = `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios/${encodeURIComponent(scenarioID)}`;
  const artifact = await scenarioRequest(path);
  scenarioEditorArtifact = artifact;
  scenarioEditorOrganization = organizationID;
  scenarioEditorDirty = false;
  scenarioEditorViewing = 0;
  scenarioEditorSummary = "";
  scenarioAssistant.reset();
  document.getElementById("scenario-editor-advanced").open = false;
  scenarioEditorSaved = artifact.content;
  renderScenarioEditor(artifact.content);
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  showScenarioEditorPage();
  await refreshScenarioRevisions();
}

async function refreshScenarioRevisions() {
  const artifact = scenarioEditorArtifact;
  if (!artifact) { scenarioHistory.render([]); return; }
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(artifact.id)}/revisions`;
  const revisions = await scenarioRequest(path);
  if (artifact !== scenarioEditorArtifact) return;
  scenarioHistory.render([...revisions].reverse().map((revision) => ({ ...revision, number: revision.revision })), {
    current: scenarioEditorViewing || artifact.revision,
    detail: (revision) => revision.summary || "",
    meta: (revision) => [revision.revision === artifact.published_revision ? "published" : "", revision.author || ""].filter(Boolean).join(" · "),
    onSelect: (revision) => loadScenarioRevision(revision.number).catch((error) => setScenarioEditorStatus(error.message || "Could not load version.", "error")),
  });
}

async function readScenarioEditorSource() {
  let scenario;
  try { scenario = await window.readScenarioSourceEditor(); }
  catch (error) {
    if (error.sourceDiagnostic) throw error;
    throw new Error(`Scenario must be valid JSON or YAML: ${error.message}`);
  }
  if (!scenario || typeof scenario !== "object" || Array.isArray(scenario)) throw new Error("Scenario JSON must be an object.");
  if (scenario.organization_id && scenario.organization_id !== scenarioEditorOrganization) throw new Error("Scenario organization_id must match the selected organization.");
  if (scenarioEditorArtifact && scenario.id !== scenarioEditorArtifact.id) throw new Error("The existing scenario ID cannot be changed.");
  scenario.organization_id = scenarioEditorOrganization;
  return scenario;
}

async function saveScenarioDraft() {
  const scenario = scenarioEditorJsonDirty ? await readScenarioEditorSource() : readScenarioEditorForm();
  if (!scenarioEditorPage.isNamed(scenario.name)) throw new Error("Enter a scenario name other than Untitled before saving.");
  const summary = scenarioEditorSummary || (scenarioEditorArtifact ? "Updated scenario draft" : "Initial version");
  const creating = !scenarioEditorArtifact;
  const path = creating
    ? `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios`
    : `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}`;
  const result = await scenarioRequest(path, {
    method: creating ? "POST" : "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ scenario, summary, expected_revision: scenarioEditorArtifact?.revision || 0 }),
  });
  if (creating) scenarioEditorArtifact = result;
  else scenarioEditorArtifact = { ...scenarioEditorArtifact, revision: result.revision, content: result.content, content_hash: result.content_hash, published_revision: scenarioEditorArtifact.published_revision || 0 };
  scenarioEditorDraft = scenario;
  scenarioEditorDirty = false;
  scenarioEditorSaved = scenario;
  scenarioEditorSummary = "";
  scenarioEditorViewing = 0;
  renderScenarioEditor(scenario);
  setScenarioEditorStatus(`Saved version ${scenarioEditorArtifact.revision}. Publish it to enable runs.`, "saved");
  await refreshScenarioRevisions();
  updateScenarioEditorActions();
  await refreshScenariosNow();
}

async function publishScenarioRevision() {
  if (!scenarioEditorArtifact || scenarioEditorDirty) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}/publish`;
  const artifact = await scenarioRequest(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_revision: scenarioEditorArtifact.revision }),
  });
  scenarioEditorArtifact = artifact;
  setScenarioEditorStatus(`Published version ${artifact.published_revision} for ${scenarioEditorOrganization}.`, "saved");
  updateScenarioEditorActions();
  await refreshScenarioRevisions();
  await refreshScenariosNow();
}

async function loadScenarioRevision(number) {
  if (!number || !scenarioEditorArtifact) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}/revisions/${encodeURIComponent(number)}`;
  const revision = await scenarioRequest(path);
  if (scenarioEditorDirty && !window.confirm("Replace unsaved scenario changes with this version?")) return;
  renderScenarioEditor(revision.content);
  scenarioEditorSummary = `Restore version ${revision.revision}`;
  scenarioEditorViewing = revision.revision === scenarioEditorArtifact.revision ? 0 : revision.revision;
  scenarioEditorDirty = Boolean(scenarioEditorViewing);
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  await refreshScenarioRevisions();
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
  if (scenarioEditorArtifact?.id === scenarioID) {
    scenarioEditorDirty = false;
    document.getElementById("scenario-editor-dialog").close();
  }
  await refreshScenariosNow();
}

const scenarioAssistant = window.mountSimulatorDesignAssistant("scenario-assistant-slot", {
  scope: "scenario",
  editorID: "scenario-editor-dialog",
  applyLabel: "Apply scenario draft",
  emptyMessage: "Ask for a scenario outline, step plan, or review of this draft.",
  getContext: () => ({
    organization_id: scenarioEditorOrganization,
    scenario: scenarioEditorJsonDirty ? window.peekScenarioSourceEditor() : readScenarioEditorForm(),
    source: document.getElementById("scenario-editor-source").value,
    revision: scenarioEditorArtifact?.revision,
    viewing: scenarioEditorViewing,
  }),
  onApply: (proposal) => {
    const scenario = proposal.scenario;
    if (!scenario) throw new Error("Claude did not return a scenario draft.");
    if (scenarioEditorArtifact && scenario.id !== scenarioEditorArtifact.id) throw new Error("The proposal changed the existing scenario ID.");
    const before = scenarioEditorJsonDirty ? window.peekScenarioSourceEditor() : readScenarioEditorForm();
    const previousSource = document.getElementById("scenario-editor-source").value;
    const previousJsonDirty = scenarioEditorJsonDirty;
    const previousDirty = scenarioEditorDirty;
    const previousSummary = scenarioEditorSummary;
    const organization = scenarioEditorOrganization;
    const artifact = scenarioEditorArtifact;
    const revision = artifact?.revision;
    const viewing = scenarioEditorViewing;
    const current = () => JSON.stringify({ form: readScenarioEditorForm(), source: document.getElementById("scenario-editor-source").value, rawDirty: scenarioEditorJsonDirty });
    let expected;
    const restore = (value, text, rawDirty, dirty, summary) => {
      if (scenarioEditorOrganization !== organization || scenarioEditorArtifact !== artifact || artifact?.revision !== revision || scenarioEditorViewing !== viewing || current() !== expected) {
        throw new Error("The scenario changed after the AI edit. Undo/redo is no longer available.");
      }
      renderScenarioEditor(value);
      document.getElementById("scenario-editor-source").value = text;
      scenarioEditorJsonDirty = rawDirty;
      window.renderScenarioSourceEditor?.();
      scenarioEditorDirty = dirty;
      scenarioEditorSummary = summary;
      scenarioFormAnalysisScope.invalidate();
      setScenarioAnalyzeState("pending");
      updateScenarioEditorActions();
      setScenarioEditorStatus("AI change restored as an unsaved editor change.");
      expected = current();
    };
    scenario.organization_id = scenarioEditorOrganization;
    renderScenarioEditor(scenario);
    scenarioEditorDirty = true;
    scenarioFormAnalysisScope.invalidate();
    setScenarioAnalyzeState("pending");
    scenarioEditorSummary = "Claude-assisted scenario draft";
    setScenarioEditorStatus("Claude proposal applied to the editor. Review and save it as a draft.");
    updateScenarioEditorActions();
    const appliedSource = document.getElementById("scenario-editor-source").value;
    expected = current();
    return {
      undo: () => restore(before, previousSource, previousJsonDirty, previousDirty, previousSummary),
      redo: () => restore(scenario, appliedSource, false, true, "Claude-assisted scenario draft"),
    };
  },
});

async function postScenarioAction(url) {
  await scenarioRequest(url, { method: "POST" });
  await refreshScenariosNow();
}


document.getElementById("scenario-refresh").addEventListener("click", () => void refreshScenariosNow());
document.getElementById("scenario-new").addEventListener("click", openNewScenarioEditor);
document.getElementById("scenario-editor-dialog").addEventListener("close", () => {
  scenarioFormAnalysisScope.invalidate();
});
document.getElementById("scenario-editor-advanced").addEventListener("toggle", () => scenarioFormAnalysisScope.invalidate());
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
  const advanced = event.currentTarget;
  if (advanced.open) return;
  (async () => {
  try {
    const scenario = await readScenarioEditorSource();
    const previous = readScenarioEditorForm();
    scenarioEditorDirty = scenarioEditorDirty || scenarioEditorJsonDirty || JSON.stringify(scenario) !== JSON.stringify(previous);
    renderScenarioEditor(scenario);
    if (scenarioEditorDirty) setScenarioEditorStatus("Raw source loaded into the form. Save to create a new draft revision.");
    updateScenarioEditorActions();
  } catch (error) {
    advanced.open = true;
    setScenarioEditorStatus(error.sourceDiagnostic ? "" : error.message || "Scenario source must be valid.", "error");
  }
  })();
});
scenarioEditorController.bind("analyze", { button: document.getElementById("scenario-source-analyze") });
document.getElementById("scenario-editor-mode-toggle").addEventListener("click", async () => {
  const advanced = document.getElementById("scenario-editor-advanced");
  advanced.open = !advanced.open;
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
scenarioEditorController.bind("save", {
  button: document.getElementById("scenario-editor-save"),
  shortcutRoot: document.getElementById("scenario-editor-dialog"),
  onError: error => setScenarioEditorStatus(error.sourceDiagnostic ? "" : error.message || "Could not save scenario.", "error"),
});
document.getElementById("scenario-editor-publish").addEventListener("click", async () => {
  try { await publishScenarioRevision(); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not publish scenario.", "error"); }
});
document.getElementById("scenario-editor-discard").addEventListener("click", () => {
  if (!scenarioEditorDirty || !scenarioEditorPage.confirmDiscard(true, "Discard unsaved scenario changes?")) return;
  scenarioEditorDirty = false;
  scenarioEditorViewing = 0;
  scenarioEditorSummary = "";
  renderScenarioEditor(scenarioEditorSaved || newScenarioTemplate());
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  refreshScenarioRevisions().catch(() => {});
});
document.getElementById("scenario-editor-close").addEventListener("click", closeScenarioEditorPage);
document.getElementById("scenario-editor-form").addEventListener("submit", (event) => {
  event.preventDefault();
  closeScenarioEditorPage();
});
document.getElementById("scenario-editor-form").noValidate = true;
document.getElementById("scenario-editor-delete").addEventListener("click", async () => {
  if (!scenarioEditorArtifact) return;
  try { await archiveScenario(scenarioEditorArtifact.id, scenarioEditorOrganization, scenarioEditorArtifact.revision); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not delete scenario.", "error"); }
});
document.getElementById("scenario-list").addEventListener("click", async (event) => {
  const editButton = event.target.closest("[data-edit-scenario]");
  if (editButton) {
    try { await scenarioEditorController.perform("load", editButton.dataset.editScenario); }
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
      await refreshScenariosNow();
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
    await refreshScenariosNow();
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
    await refreshScenariosNow();
  } catch (error) {
    const message = document.getElementById("scenario-error");
    message.textContent = error.message || "Could not clear scenario run";
    message.hidden = false;
  } finally {
    button.disabled = false;
  }
});