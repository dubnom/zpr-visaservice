var $ = (s) => document.querySelector(s);
var esc = (v) => String(v ?? "").replace(/[&<>\"]/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]));
async function activityRefresh() { try { const response = await fetch("/api/simulator/activity"); if (!response.ok) throw new Error(`HTTP ${response.status}`); const data = await response.json(); const stats = data.stats || {}; $("#requests").textContent = stats.visa_requests || "0"; $("#approved").textContent = stats.visa_requests_approved || "0"; $("#denied").textContent = stats.visa_requests_denied || "0"; $("#visa-count").textContent = data.visas.length; $("#activity-state").textContent = "LIVE"; $("#activity-time").textContent = new Date(data.generated_at).toLocaleTimeString(); $("#visas").innerHTML = data.visas.map((visa) => `<article><div class="log-title">${esc(visa.proto)} · ${esc(visa.direction)}</div><pre>${esc(visa.source_addr)} → ${esc(visa.dest_addr)}\nports ${esc(visa.source_port)} → ${esc(visa.dest_port)}\npolicy ${esc(visa.policy_id)}</pre></article>`).join("") || `<article><pre>No visa activity yet.</pre></article>`; $("#denies").innerHTML = data.denies.map((deny) => `<article><div class="log-title">${esc(deny.code)}</div><pre>${esc(deny.source_addr)} → ${esc(deny.dest_addr)}\nport ${esc(deny.dest_port)}\ncount ${esc(deny.count)}</pre></article>`).join("") || `<article><pre>No denied flows yet.</pre></article>`; } catch (error) { const status = $("#activity-state"); if (status) status.textContent = "UNAVAILABLE"; } }
let activityRefreshTimer;
$("#refresh").addEventListener("click", activityRefresh);
document.addEventListener("simulator:activate", (event) => {
	if (event.detail.path !== "/activity.html" || activityRefreshTimer) return;
	activityRefresh();
	activityRefreshTimer = setInterval(activityRefresh, 4000);
});
document.addEventListener("simulator:deactivate", (event) => {
	if (event.detail.path !== "/activity.html") return;
	clearInterval(activityRefreshTimer);
	activityRefreshTimer = undefined;
});
