var $ = (s) => document.querySelector(s);
var esc = (v) => String(v ?? "").replace(/[&<>\"]/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]));
let state;
function toast(message) { const node = $("#toast"); node.textContent = message; node.hidden = false; setTimeout(() => { node.hidden = true; }, 2600); }
function render(data) {
  state = data;
  $("#manifest-name").textContent = data.manifest.name;
  $("#manifest-base").textContent = `extends ${data.manifest.extends || "generic base"}`;
  $("#agent-count").textContent = data.manifest.agents.length;
  $("#service-count").textContent = data.manifest.services.length;
  $("#trusted-count").textContent = data.manifest.trusted_services.length;
  const running = Object.values(data.agents).filter((v) => v === "running").length;
  $("#stack-count").textContent = data.stack.includes("running") ? "UP" : "DOWN";
  $("#runtime-state").textContent = `${running}/${Object.keys(data.agents).length} agents running`;
  $("#runtime-state").className = `status ${running ? "live" : ""}`;
  $("#agents").innerHTML = data.manifest.agents.map((agent) => {
    const key = agent.name === "zpr-node" ? "zpr-local-linux-node" : agent.name === "visa-service" ? "zpr-local-linux-node" : "zpr-auth-sandbox";
    const status = data.agents[key] || "not started";
    return `<article class="agent-card"><div class="agent-icon">${agent.boot_order}</div><div><strong>${esc(agent.name)}</strong><small>${esc(agent.artifact)}</small></div><span class="pill ${status === "running" ? "on" : "off"}">${esc(status)}</span></article>`;
  }).join("");
  $("#services").innerHTML = data.manifest.services.map((service) => `<article class="service-row"><div><strong>${esc(service.name)}</strong><small>${esc(service.kind)} · ${esc(service.command)}</small></div><span>${esc(service.endpoint)}</span></article>`).join("");
  $("#components").innerHTML = (data.manifest.components || []).map((component) => {
    const state = data.components[component.name] || "stopped";
    const identities = (component.identities || []).map((identity) => `<option value="${esc(identity.name)}">${esc(identity.name)} · ${esc(identity.auth)}</option>`).join("");
    return `<article class="component-card"><div><span class="component-kind">${esc(component.kind)}</span><strong>${esc(component.name)}</strong><small>${esc(component.address || "address pending")}</small></div>${identities ? `<label class="identity-select">IDENTITY<select data-component-identity="${esc(component.name)}">${identities}</select></label>` : ""}<span class="pill ${state.startsWith("running") ? "on" : "off"}">${esc(state)}</span><div class="component-actions"><button data-component-action="start-component" data-component-name="${esc(component.name)}">Start</button><button class="quiet" data-component-action="stop-component" data-component-name="${esc(component.name)}">Stop</button><button class="quiet" data-component-action="exercise-component" data-component-name="${esc(component.name)}">Exercise</button></div></article>`;
  }).join("");
  document.querySelectorAll("[data-component-action]").forEach((button) => button.addEventListener("click", () => { const selector = document.querySelector(`[data-component-identity="${CSS.escape(button.dataset.componentName)}"]`); action(button.dataset.componentAction, button.dataset.componentName, selector?.value || ""); }));
  $("#log-grid").innerHTML = Object.entries(data.logs).map(([name, log]) => `<article><div class="log-title">${esc(name)}</div><pre>${esc(log || "No log yet")}</pre></article>`).join("");
}
async function simulatorRefresh() { try { const response = await fetch("/api/simulator/status"); if (!response.ok) throw new Error(`HTTP ${response.status}`); render(await response.json()); } catch (error) { const status = $("#runtime-state"); if (status) status.textContent = "Simulator API unavailable"; } }
async function action(name, component, identity = "") { toast(`${name} requested`); const query = component ? `?name=${encodeURIComponent(component)}${identity ? `&identity=${encodeURIComponent(identity)}` : ""}` : ""; const response = await fetch(`/api/simulator/action/${name}${query}`, { method: "POST" }); const data = await response.json(); toast(data.output || `${name} complete`); await simulatorRefresh(); }
document.querySelectorAll("[data-action]").forEach((button) => button.addEventListener("click", () => action(button.dataset.action)));
let simulatorRefreshTimer;
$("#refresh").addEventListener("click", simulatorRefresh);
document.addEventListener("simulator:activate", (event) => {
  if (event.detail.path !== "/" || simulatorRefreshTimer) return;
  simulatorRefresh();
  simulatorRefreshTimer = setInterval(simulatorRefresh, 4000);
});
document.addEventListener("simulator:deactivate", (event) => {
  if (event.detail.path !== "/") return;
  clearInterval(simulatorRefreshTimer);
  simulatorRefreshTimer = undefined;
});
