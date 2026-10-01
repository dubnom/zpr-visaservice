const scenarioById = new Map();
let scenarioRun;
let scenarioRefreshTimer;
let scenarioRefreshPromise;
let scenarioCatalogSignature = "";
let scenarioRunSignature = "";

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
    return `<article class="scenario-card${selected ? " selected" : ""}"><div class="scenario-card-heading"><div><span class="scenario-id">${scenarioEscape(scenario.id)}</span><h3>${scenarioEscape(scenario.name)}</h3></div><span class="scenario-step-count">${machines}/${maxMachines} machines · ${scenario.steps.length} steps</span></div><p>${scenarioEscape(scenario.description)}</p><button type="button" data-run-scenario="${scenarioEscape(scenario.id)}" ${running ? "disabled" : ""}>Run scenario</button></article>`;
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
  document.getElementById("scenario-steps").innerHTML = results.map((step) => {
    const detail = step.error || step.output || "Completed";
    const time = step.finished_at ? new Date(step.finished_at).toLocaleTimeString() : "Running";
    return `<li class="scenario-step ${scenarioEscape(step.status)} ${step.phase === "cleanup" ? "cleanup" : ""}"><span class="scenario-step-mark" aria-hidden="true"></span><div class="scenario-step-copy"><div><strong>${scenarioEscape(step.action.replaceAll("_", " "))}</strong><span>${scenarioEscape(step.machine || step.component || step.phase)}</span></div><p>${scenarioEscape(detail)}</p></div><time>${scenarioEscape(time)}</time></li>`;
  }).join("");
  for (const button of document.querySelectorAll("[data-run-scenario]")) button.disabled = busy;
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
      const scenarios = data.scenarios || [];
      const catalogSignature = JSON.stringify(scenarios);
      if (catalogSignature !== scenarioCatalogSignature) {
        scenarioCatalogSignature = catalogSignature;
        renderScenarioList(scenarios, data.max_machines || 10);
      }
      const runSignature = JSON.stringify(data.run || {});
      if (runSignature !== scenarioRunSignature) {
        scenarioRunSignature = runSignature;
        renderScenarioRun(data.run);
      }
      document.getElementById("scenario-connection").textContent = `${scenarios.length} scenarios available`;
    } catch (failure) {
      error.textContent = failure.message || "Scenario catalog unavailable";
      error.hidden = false;
      document.getElementById("scenario-connection").textContent = "Scenario service unavailable";
    }
  })();
  try { await scenarioRefreshPromise; } finally { scenarioRefreshPromise = undefined; }
}

async function postScenarioAction(url) {
  const response = await fetch(url, { method: "POST" });
  const body = await response.json();
  if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`);
  await refreshScenarios();
}

document.getElementById("scenario-refresh").addEventListener("click", refreshScenarios);
document.getElementById("scenario-list").addEventListener("click", async (event) => {
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