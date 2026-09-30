var $ = (s) => document.querySelector(s);
var esc = (v) => String(v ?? "").replace(/[&<>\"]/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]));
function toast(message) { const node = $("#toast"); node.textContent = message; node.hidden = false; setTimeout(() => { node.hidden = true; }, 2600); }
let data;
function render(snapshot) {
  data = snapshot; const components = snapshot.manifest.components || []; const running = Object.values(snapshot.components).filter((value) => value.startsWith("running")).length;
  $("#manifest-name").textContent = snapshot.manifest.name; $("#manifest-base").textContent = `extends ${snapshot.manifest.extends || "generic base"}`; $("#runtime-state").textContent = `${running}/${components.length} agents running`; $("#agent-count").textContent = components.length; $("#running-count").textContent = running; $("#service-count").textContent = snapshot.manifest.services.length; $("#stack-count").textContent = snapshot.stack.includes("running") ? "UP" : "DOWN";
  $("#components").innerHTML = components.map((component) => { const state = snapshot.components[component.name] || "stopped"; const identities = (component.identities || []).map((identity) => `<option value="${esc(identity.name)}">${esc(identity.name)} · ${esc(identity.auth)}</option>`).join(""); return `<article class="component-card"><div><span class="component-kind">${esc(component.kind)}</span><strong>${esc(component.name)}</strong><small>${esc(component.agent || "agent pending")} · ${esc(component.address || "address pending")}</small></div>${identities ? `<label class="identity-select">IDENTITY<select data-identity="${esc(component.name)}">${identities}</select></label>` : ""}<span class="pill ${state.startsWith("running") ? "on" : "off"}">${esc(state)}</span><div class="component-actions"><button data-action="start-component" data-name="${esc(component.name)}">Start</button><button class="quiet" data-action="stop-component" data-name="${esc(component.name)}">Stop</button><button class="quiet" data-action="exercise-component" data-name="${esc(component.name)}">Exercise</button></div></article>`; }).join("");
  $("#log-grid").innerHTML = Object.entries(snapshot.logs).map(([name, log]) => `<article><div class="log-title">${esc(name)}</div><pre>${esc(log || "No log yet")}</pre></article>`).join("");
  document.querySelectorAll("[data-action]").forEach((button) => button.addEventListener("click", () => { const name = button.dataset.name; const select = name && document.querySelector(`[data-identity="${CSS.escape(name)}"]`); action(button.dataset.action, name, select?.value || ""); }));
}
async function agentsRefresh() { try { const response = await fetch("/api/simulator/status"); if (!response.ok) throw new Error(`HTTP ${response.status}`); render(await response.json()); } catch (error) { const status = $("#runtime-state"); if (status) status.textContent = "Simulator API unavailable"; } }
async function action(name, component = "", identity = "") { toast(`${name} requested`); const query = component ? `?name=${encodeURIComponent(component)}${identity ? `&identity=${encodeURIComponent(identity)}` : ""}` : ""; const response = await fetch(`/api/simulator/action/${name}${query}`, { method: "POST" }); const result = await response.json(); toast(result.output || `${name} complete`); await agentsRefresh(); }
let agentsRefreshTimer;
$("#refresh").addEventListener("click", agentsRefresh);
document.addEventListener("simulator:activate", (event) => {
  if (event.detail.path !== "/agents.html" || agentsRefreshTimer) return;
  agentsRefresh();
  agentsRefreshTimer = setInterval(agentsRefresh, 4000);
});
document.addEventListener("simulator:deactivate", (event) => {
  if (event.detail.path !== "/agents.html") return;
  clearInterval(agentsRefreshTimer);
  agentsRefreshTimer = undefined;
});
