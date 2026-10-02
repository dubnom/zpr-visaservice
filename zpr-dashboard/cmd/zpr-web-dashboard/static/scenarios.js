const scenarioById = new Map();
let scenarioRun;
let scenarioRefreshTimer;
let scenarioRefreshPromise;
let scenarioCatalogSignature = "";
let scenarioRunSignature = "";
let scenarioOrganizations = [];
let activeScenarioOrganization = "";
let scenarioEditorArtifact;
let scenarioEditorOrganization = "";
let scenarioEditorDirty = false;

function scenarioEscape(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[character]);
}

function renderScenarioList(scenarios, maxMachines) {
  scenarioById.clear();
  for (const scenario of scenarios) scenarioById.set(scenario.id, scenario);
  document.getElementById("scenario-count").textContent = `${scenarios.length} loaded`;
  document.getElementById("scenario-machine-limit").textContent = `Maximum ${maxMachines} machines per run`;
  document.getElementById("scenario-list").innerHTML = scenarios.length ? scenarios.map((scenario) => {
    const running = scenarioRun?.state === "running" || scenarioRun?.state === "cleaning";
    const selected = scenarioRun?.scenario_id === scenario.id;
    const machines = new Set([...(scenario.steps || []), ...(scenario.cleanup || [])].map((step) => step.machine).filter(Boolean)).size;
    const topology = scenario.topology;
    const topologySummary = topology?.nodes?.length ? `${topology.nodes.length} nodes · ${(topology.links || []).length} links` : "base topology";
    const published = Number(scenario.published_revision || 0);
    const canRun = scenario.organization_id === activeScenarioOrganization && published > 0 && !running;
    return `<article class="scenario-card${selected ? " selected" : ""}"><div class="scenario-card-heading"><div><span class="scenario-id">${scenarioEscape(scenario.id)}</span><h3>${scenarioEscape(scenario.name)}</h3></div><span class="scenario-step-count">${topologySummary} · ${machines}/${maxMachines} machines · ${(scenario.steps || []).length} steps</span></div><p>${scenarioEscape(scenario.description)}</p><div class="scenario-card-actions"><span class="scenario-version${published ? " published" : ""}">${published ? `Published r${published}` : `Draft r${scenario.current_revision || 1}`}</span><button class="quiet" type="button" data-edit-scenario="${scenarioEscape(scenario.id)}">Edit</button><button type="button" data-run-scenario="${scenarioEscape(scenario.id)}" data-run-organization="${scenarioEscape(scenario.organization_id)}" data-run-published="${published > 0}" ${canRun ? "" : "disabled"}>Run published</button></div></article>`;
  }).join("") : `<p class="scenario-empty">No JSON scenarios found.</p>`;
}

function renderScenarioRun(run) {
  scenarioRun = run || { state: "idle", steps: [] };
  const busy = scenarioRun.state === "running" || scenarioRun.state === "cleaning";
  document.getElementById("scenario-state-label").textContent = String(scenarioRun.state || "idle").toUpperCase();
  document.getElementById("scenario-cancel").disabled = !busy;
  const summary = document.getElementById("scenario-run-summary");
  if (!scenarioRun.scenario_id) {
    summary.innerHTML = `<span class="scenario-state idle">Idle</span><span>No scenario selected</span>`;
  } else {
    const current = `${scenarioRun.current_step || 0} / ${scenarioRun.total_steps || 0}`;
    summary.innerHTML = `<span class="scenario-state ${scenarioEscape(scenarioRun.state)}">${scenarioEscape(scenarioRun.state)}</span><strong>${scenarioEscape(scenarioRun.scenario_name)}</strong><span class="scenario-progress">${current}</span>${scenarioRun.error ? `<p class="scenario-run-error">${scenarioEscape(scenarioRun.error)}</p>` : ""}`;
  }
  const results = scenarioRun.steps || [];
  const listedScenario = scenarioById.get(scenarioRun.scenario_id);
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
    const detail = result?.error || result?.output || (active ? "Running" : status === "pending" ? waiting ? "Waiting" : "Not run" : "Completed");
    const time = result?.finished_at ? new Date(result.finished_at).toLocaleTimeString() : "";
    const machine = step.machine || "Shared";
    const lane = lanes.get(machine) || [];
    lane.push(`<li class="scenario-step ${scenarioEscape(status)} ${phase === "cleanup" ? "cleanup" : ""}"><span class="scenario-step-mark" aria-hidden="true"></span><div class="scenario-step-copy"><div><span class="scenario-step-number">${index + 1}</span><strong>${scenarioEscape(step.action.replaceAll("_", " "))}</strong>${step.component ? `<span>${scenarioEscape(step.component)}</span>` : ""}${phase === "cleanup" ? `<span>Cleanup</span>` : ""}</div><p>${scenarioEscape(detail)}</p></div><time>${scenarioEscape(time)}</time></li>`);
    lanes.set(machine, lane);
  });
  document.getElementById("scenario-steps").innerHTML = [...lanes].map(([machine, steps]) => `<section class="scenario-lane"><h3>${scenarioEscape(machine)}</h3><ol class="scenario-steps">${steps.join("")}</ol></section>`).join("");
  for (const button of document.querySelectorAll("[data-run-scenario]")) {
    button.disabled = busy || button.dataset.runOrganization !== activeScenarioOrganization || button.dataset.runPublished !== "true";
  }
}

async function refreshScenarios() {
  if (scenarioRefreshPromise) return scenarioRefreshPromise;
  scenarioRefreshPromise = (async () => {
    const error = document.getElementById("scenario-error");
    try {
      if (!scenarioOrganizations.length) {
        const organizationsResponse = await fetch("/api/simulator/organizations", { cache: "no-store" });
        const organizationsData = await organizationsResponse.json();
        if (!organizationsResponse.ok) throw new Error(organizationsData.error || `HTTP ${organizationsResponse.status}`);
        scenarioOrganizations = organizationsData.organizations || [];
        activeScenarioOrganization = organizationsData.active_id || "";
        const selector = document.getElementById("scenario-organization");
        selector.replaceChildren(...scenarioOrganizations.map((organization) => new Option(`${organization.name}${organization.id === activeScenarioOrganization ? " · ACTIVE" : ""}`, organization.id)));
        if (!selector.value) selector.value = activeScenarioOrganization;
      }
      const selector = document.getElementById("scenario-organization");
      const organizationID = selector.value || activeScenarioOrganization;
      const response = await fetch(`/api/simulator/scenarios?organization_id=${encodeURIComponent(organizationID)}`, { cache: "no-store" });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
      error.hidden = true;
      activeScenarioOrganization = data.active_organization_id || activeScenarioOrganization;
      const scenarios = data.scenarios || [];
      const catalogSignature = JSON.stringify(scenarios);
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
      const selectedOrganization = scenarioOrganizations.find((organization) => organization.id === organizationID);
      document.getElementById("scenario-connection").textContent = `${selectedOrganization?.name || organizationID}: ${scenarios.length} scenarios${organizationID === activeScenarioOrganization ? " · active" : " · edit only"}`;
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
}

function openNewScenarioEditor() {
  scenarioEditorArtifact = null;
  scenarioEditorOrganization = document.getElementById("scenario-organization").value || activeScenarioOrganization;
  scenarioEditorDirty = false;
  document.getElementById("scenario-editor-title").textContent = "New scenario";
  document.getElementById("scenario-editor-source").value = JSON.stringify({
    id: "new-scenario",
    organization_id: scenarioEditorOrganization,
    name: "New scenario",
    description: "Describe the behavior this scenario exercises.",
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  }, null, 2);
  document.getElementById("scenario-editor-summary").value = "Initial version";
  document.getElementById("scenario-editor-revisions").replaceChildren(new Option("Not saved yet", ""));
  document.getElementById("scenario-editor-revisions").disabled = true;
  document.getElementById("scenario-editor-save").textContent = "Create scenario";
  setScenarioEditorStatus("Save a draft, then publish it before running.");
  updateScenarioEditorActions();
  document.getElementById("scenario-editor-dialog").showModal();
}

async function openExistingScenarioEditor(scenarioID) {
  const organizationID = document.getElementById("scenario-organization").value || activeScenarioOrganization;
  const path = `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios/${encodeURIComponent(scenarioID)}`;
  const response = await fetch(path, { cache: "no-store" });
  const artifact = await response.json();
  if (!response.ok) throw new Error(artifact.error || `HTTP ${response.status}`);
  scenarioEditorArtifact = artifact;
  scenarioEditorOrganization = organizationID;
  scenarioEditorDirty = false;
  document.getElementById("scenario-editor-title").textContent = `Edit ${artifact.content.name}`;
  document.getElementById("scenario-editor-source").value = JSON.stringify(artifact.content, null, 2);
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
  if (scenario.organization_id !== scenarioEditorOrganization) throw new Error("Scenario organization_id must match the selected organization.");
  return scenario;
}

async function saveScenarioDraft() {
  const scenario = readScenarioEditorSource();
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
  scenarioEditorDirty = false;
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
  document.getElementById("scenario-editor-source").value = JSON.stringify(revision.content, null, 2);
  document.getElementById("scenario-editor-summary").value = `Restore revision ${revision.revision}`;
  scenarioEditorDirty = true;
  setScenarioEditorStatus(`Viewing revision ${revision.revision}; save to create a new version.`);
  updateScenarioEditorActions();
}

async function postScenarioAction(url) {
  const response = await fetch(url, { method: "POST" });
  const body = await response.json();
  if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`);
  await refreshScenarios();
}


document.getElementById("scenario-refresh").addEventListener("click", refreshScenarios);
document.getElementById("scenario-new").addEventListener("click", openNewScenarioEditor);
document.getElementById("scenario-organization").addEventListener("change", () => {
  scenarioCatalogSignature = "";
  scenarioRunSignature = "";
  if (document.getElementById("scenario-editor-dialog").open) document.getElementById("scenario-editor-dialog").close();
  refreshScenarios();
});
document.getElementById("scenario-editor-source").addEventListener("input", () => {
  scenarioEditorDirty = true;
  updateScenarioEditorActions();
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