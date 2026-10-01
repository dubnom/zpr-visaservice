var $ = (s) => document.querySelector(s);
var esc = (v) => String(v ?? "").replace(/[&<>\"]/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]));
function toast(message) { const node = $("#toast"); node.textContent = message; node.hidden = false; setTimeout(() => { node.hidden = true; }, 2600); }
let data;
let openWorkloadPickerMachine = null;
let pendingLoginMachine = "";
const workloadDrafts = new Map();
function render(snapshot) {
  data = snapshot; const components = snapshot.manifest.components || []; const running = Object.values(snapshot.components).filter((value) => value.startsWith("running")).length;
  renderMachines(snapshot.manifest.machines || [], components, snapshot.controllers || {}, snapshot.components || {}, snapshot.sessions || {}, snapshot.machine_containers || {});
  $("#manifest-name").textContent = snapshot.manifest.name; $("#manifest-base").textContent = `extends ${snapshot.manifest.extends || "generic base"}`; $("#runtime-state").textContent = `${running}/${components.length} agents running`; $("#agent-count").textContent = components.length; $("#running-count").textContent = running; $("#service-count").textContent = snapshot.manifest.services.length; $("#stack-count").textContent = snapshot.stack.includes("running") ? "UP" : "DOWN";
  $("#log-grid").innerHTML = Object.entries(snapshot.logs).map(([name, log]) => `<article><div class="log-title">${esc(name)}</div><pre>${esc(log || "No log yet")}</pre></article>`).join("");
}
function renderMachines(machines, components, controllers, componentStates, sessions, machineContainers) {
  const filter = $("#machine-type-filter").value;
  const visible = machines.filter((machine) => filter === "all" || machine.type === filter);
  const workloadChoices = [...components].sort((left, right) => left.name.localeCompare(right.name));
  $("#machine-count").textContent = visible.length;
  $("#machine-grid").innerHTML = visible.map((machine) => {
    const posture = machine.secure ? "Secure" : "Unsecured";
    const connected = controllers[machine.id]?.connected === true;
    const controllerLabel = connected ? "Connected" : "Offline";
    const containerState = machineContainers[machine.id] || "missing";
    const session = sessions[machine.id] || {};
    const selectedNames = workloadDrafts.get(machine.id) || (session.authenticated ? (session.workloads || []) : []);
    const selectedSet = new Set(selectedNames);
    const workloads = components.filter((component) => selectedSet.has(component.name));
    const machineRunning = containerState === "running";
    const sessionControl = session.authenticated
      ? `<div class="machine-session"><div><span class="session-label">SIMULATED USER</span><strong>${esc(session.user)}</strong></div><button type="button" class="quiet" data-session-action="logout" data-machine="${esc(machine.id)}" ${connected && machineRunning ? "" : "disabled"}>Log out</button></div>`
      : `<div class="machine-session"><span class="session-label">NO USER SESSION</span><button type="button" data-login-machine="${esc(machine.id)}" data-owner="${esc(machine.owner)}" ${connected && machineRunning ? "" : "disabled"}>Log in</button></div>`;
    const workloadPicker = session.authenticated
      ? `<details class="workload-picker" data-workload-picker="${esc(machine.id)}" ${openWorkloadPickerMachine === machine.id ? "open" : ""}><summary>Select workloads <span>${selectedNames.length} selected</span></summary><div class="workload-picker-menu"><fieldset><legend>Clients and services</legend>${workloadChoices.map((component) => `<label><input type="checkbox" data-workload-choice="${esc(machine.id)}" value="${esc(component.name)}" ${selectedSet.has(component.name) ? "checked" : ""}><span>${esc(component.name)}<small>${component.kind === "client" ? "Client" : "Service"}</small></span></label>`).join("")}</fieldset><button type="button" data-save-workloads="${esc(machine.id)}" ${connected ? "" : "disabled"}>Commit</button></div></details>`
      : "";
    const workloadRows = workloads.length ? workloads.map((component) => {
      const state = componentStates[component.name] || "stopped";
      const active = state === "running";
      const pending = state === "starting";
      const kind = component.kind === "client" ? "Client" : "Service";
      const label = `${active ? "Stop" : "Start"} ${component.name}`;
      return `<div class="machine-workload"><div class="machine-workload-label"><span class="component-kind">${kind}</span><strong>${esc(component.name)}</strong><small>${esc(component.agent || "agent pending")} · ${esc(component.address || "address pending")}</small></div><span class="pill ${active ? "on" : "off"}">${pending ? "starting" : active ? "running" : "stopped"}</span><button type="button" class="workload-toggle" role="switch" aria-checked="${active}" aria-label="${label}" data-workload-name="${esc(component.name)}" ${connected && !pending ? "" : "disabled"}></button></div>`;
    }).join("") : `<p class="empty-workloads">No workloads selected</p>`;
    const lifecycleAction = machineRunning ? "stop" : "start";
    const lifecycleLabel = machineRunning ? "Stop machine" : "Start machine";
    return `<article class="machine-card"><div class="machine-card-head"><div class="machine-card-identity"><span class="component-kind">${esc(machine.type)}</span><strong>${esc(machine.id)}</strong></div><div class="machine-card-status"><span class="controller-indicator ${connected ? "connected" : "offline"}"><i></i>${controllerLabel}</span><span class="pill ${machine.secure ? "on" : "off"}">${posture}</span></div></div><div class="machine-runtime-control"><span><small>CONTAINER</small><strong class="machine-container-state ${machineRunning ? "running" : "stopped"}">${esc(containerState)}</strong></span><button type="button" class="quiet" data-machine-action="${lifecycleAction}" data-machine="${esc(machine.id)}">${lifecycleLabel}</button></div><details class="machine-details"><summary>Machine details</summary><dl><div><dt>Model</dt><dd>${esc(machine.model)}</dd></div><div><dt>Location</dt><dd>${esc(machine.location)}</dd></div><div><dt>Owner</dt><dd>${esc(machine.owner)}</dd></div></dl></details>${sessionControl}${workloadPicker}<div class="machine-workloads"><span>WORKLOADS</span>${workloadRows}</div></article>`;
  }).join("");
}
let agentsRefreshPromise;
async function agentsRefresh() {
  if (agentsRefreshPromise) return agentsRefreshPromise;
  agentsRefreshPromise = (async () => {
    try {
      const response = await fetch("/api/simulator/status");
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      render(await response.json());
    } catch (error) {
      const status = $("#runtime-state");
      if (status) status.textContent = "Simulator API unavailable";
    }
  })();
  try { await agentsRefreshPromise; } finally { agentsRefreshPromise = undefined; }
}
async function action(name, component = "") { toast(`${name} requested`); const query = component ? `?name=${encodeURIComponent(component)}` : ""; const response = await fetch(`/api/simulator/action/${name}${query}`, { method: "POST" }); const body = await response.text(); let result; try { result = JSON.parse(body); } catch { result = { output: body }; } if (!response.ok) throw new Error(result.output || `${name} failed`); toast(result.output || `${name} complete`); await agentsRefresh(); }
let agentsRefreshTimer;
document.querySelectorAll("[data-action]").forEach((button) => button.addEventListener("click", async () => {
  button.disabled = true;
  try {
    await action(button.dataset.action);
  } catch (error) {
    toast(error.message || "Simulator action failed");
  } finally {
    button.disabled = false;
  }
}));
$("#refresh").addEventListener("click", agentsRefresh);
$("#machine-type-filter").addEventListener("change", () => renderMachines(data?.manifest.machines || [], data?.manifest.components || [], data?.controllers || {}, data?.components || {}, data?.sessions || {}, data?.machine_containers || {}));
$("#machine-grid").addEventListener("click", async (event) => {
  const lifecycleButton = event.target.closest("[data-machine-action]");
  const loginButton = event.target.closest("[data-login-machine]");
  const toggle = event.target.closest("[data-workload-name]");
  const sessionButton = event.target.closest("[data-session-action]");
  const saveWorkloads = event.target.closest("[data-save-workloads]");
  if (lifecycleButton) {
    const machineID = lifecycleButton.dataset.machine;
    const lifecycleAction = lifecycleButton.dataset.machineAction;
    if (lifecycleAction === "stop" && !window.confirm(`Stop ${machineID}? Running workloads in this container will stop.`)) return;
    lifecycleButton.disabled = true;
    try {
      const response = await fetch(`/api/simulator/machines/${encodeURIComponent(machineID)}/${lifecycleAction}`, { method: "POST" });
      const body = await response.text();
      let result;
      try { result = JSON.parse(body); } catch { result = { error: body }; }
      if (!response.ok) throw new Error(result.error || `Machine ${lifecycleAction} failed`);
      toast(`${machineID} ${lifecycleAction === "start" ? "started" : "stopped"}`);
      await agentsRefresh();
    } catch (error) {
      toast(error.message || "Machine lifecycle action failed");
      if (lifecycleButton.isConnected) lifecycleButton.disabled = false;
    }
  } else if (loginButton) {
    pendingLoginMachine = loginButton.dataset.loginMachine;
    const userSelect = $("#machine-login-user");
    const availableUsers = [...new Set((data.manifest.machines || []).map((machine) => machine.owner).filter((user) => user && user !== "it-pool"))].sort();
    userSelect.innerHTML = availableUsers.map((user) => `<option value="${esc(user)}">${esc(user)}</option>`).join("");
    userSelect.value = availableUsers.includes(loginButton.dataset.owner) ? loginButton.dataset.owner : availableUsers[0] || "";
    $("#machine-login-title").textContent = `Log in to ${pendingLoginMachine}`;
    $("#machine-login-dialog").showModal();
    userSelect.focus();
  } else if (toggle) {
    const actionName = toggle.getAttribute("aria-checked") === "true" ? "stop-component" : "start-component";
    toggle.disabled = true;
    try {
      await action(actionName, toggle.dataset.workloadName);
    } catch (error) {
      toast(error.message || "Workload action failed");
      if (toggle.isConnected) toggle.disabled = false;
    }
  } else if (sessionButton) {
    const machineID = sessionButton.dataset.machine;
    sessionButton.disabled = true;
    try {
      const response = await fetch(`/api/simulator/machines/${encodeURIComponent(machineID)}/${sessionButton.dataset.sessionAction}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "{}",
      });
      const body = await response.text();
      let result;
      try { result = JSON.parse(body); } catch { result = { error: body }; }
      if (!response.ok) throw new Error(result.error || "Simulated login action failed");
      toast(sessionButton.dataset.sessionAction === "login" ? `Simulated login: ${result.user}` : "Simulated user logged out");
      await agentsRefresh();
    } catch (error) {
      toast(error.message || "Simulated login action failed");
      if (sessionButton.isConnected) sessionButton.disabled = false;
    }
  } else if (saveWorkloads) {
    const machineID = saveWorkloads.dataset.saveWorkloads;
    const workloads = workloadDrafts.get(machineID) || [...document.querySelectorAll(`[data-workload-choice="${CSS.escape(machineID)}"]:checked`)].map((input) => input.value);
    saveWorkloads.disabled = true;
    try {
      const response = await fetch(`/api/simulator/machines/${encodeURIComponent(machineID)}/workloads`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ workloads }),
      });
      const body = await response.text();
      let result;
      try { result = JSON.parse(body); } catch { result = { error: body }; }
      if (!response.ok) throw new Error(result.error || "Workload selection failed");
      workloadDrafts.delete(machineID);
      openWorkloadPickerMachine = null;
      toast(`${workloads.length} workload${workloads.length === 1 ? "" : "s"} selected for ${machineID}`);
      await agentsRefresh();
    } catch (error) {
      toast(error.message || "Workload selection failed");
      if (saveWorkloads.isConnected) saveWorkloads.disabled = false;
    }
  }
});
$("#machine-login-dialog").addEventListener("submit", async (event) => {
  event.preventDefault();
  const submitButton = $("#machine-login-submit");
  const machineID = pendingLoginMachine;
  submitButton.disabled = true;
  try {
    const response = await fetch(`/api/simulator/machines/${encodeURIComponent(machineID)}/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ user: $("#machine-login-user").value }),
    });
    const body = await response.text();
    let result;
    try { result = JSON.parse(body); } catch { result = { error: body }; }
    if (!response.ok) throw new Error(result.error || "Simulated login failed");
    $("#machine-login-dialog").close();
    pendingLoginMachine = "";
    toast(`Simulated login: ${result.user}`);
    await agentsRefresh();
  } catch (error) {
    toast(error.message || "Simulated login failed");
  } finally {
    submitButton.disabled = false;
  }
});
$("#machine-login-cancel").addEventListener("click", () => $("#machine-login-dialog").close());
$("#machine-grid").addEventListener("toggle", (event) => {
  const picker = event.target.closest("[data-workload-picker]");
  if (!picker) return;
  openWorkloadPickerMachine = picker.open ? picker.dataset.workloadPicker : null;
}, true);
$("#machine-grid").addEventListener("change", (event) => {
  const choice = event.target.closest("[data-workload-choice]");
  if (!choice) return;
  const machineID = choice.dataset.workloadChoice;
  const selected = [...document.querySelectorAll(`[data-workload-choice="${CSS.escape(machineID)}"]:checked`)].map((input) => input.value);
  workloadDrafts.set(machineID, selected);
});
document.addEventListener("simulator:activate", (event) => {
  if (event.detail.path !== "/agents.html" || agentsRefreshTimer) return;
  agentsRefresh();
  agentsRefreshTimer = setInterval(agentsRefresh, 8000);
});
document.addEventListener("simulator:deactivate", (event) => {
  if (event.detail.path !== "/agents.html") return;
  clearInterval(agentsRefreshTimer);
  agentsRefreshTimer = undefined;
});
