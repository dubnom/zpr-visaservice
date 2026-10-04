const byId = (id) => document.getElementById(id);
const zplKeywords = new Set(["allow", "never", "define", "with", "to", "access", "and", "as", "aka", "tag", "tags", "on", "optional", "multiple", "signal", "over"]);
const GRAPH_ARRIVAL_DURATION = 2800;
const GRAPH_REMOVAL_DURATION = 900;
const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
const state = { snapshot: null, timer: null, paused: false, pending: false, graphCamera: null, graphAnimations: !reducedMotion, topologyComponents: null, topologyNodeColumns: null, topologyNodeSlots: new Map(), topologyNewComponents: new Map(), selection: null, sorts: {}, dnsPending: false, policy: { loaded: false, configured: false, categories: [], records: [], attributes: [], categoryID: "", collapsedCategories: new Set(), treeInitialized: false, record: null, source: "", savedSource: "", revision: 0, browsingRevision: 0, saveAs: false, compilerReady: false, testerReady: false, testMode: false, testPending: false, testAbort: null, testResult: null, testSource: "", testDimensions: [], assistantReady: false, assistantEnabled: false, assistantUsage: { input: 0, output: 0 }, validSource: null, errorOffsets: [], revisions: [], messages: [], assistantPending: false, assistantError: "" } };

let previousPolledValues = null;

state.policy.fileClipboard = null;
state.policy.showArchived = false;

const pages = {
  map: "MAP",
  connections: "CONNECTIONS",
  actors: "ACTORS",
  "adapter-logs": "ADAPTER LOGS",
  services: "SERVICES",
  dns: "DNS",
  sources: "TRUSTED SOURCES",
  policy: "POLICY",
  visas: "VISAS",
  denies: "DENIALS",
};

function currentPage() {
  const page = location.hash.replace(/^#/, "") || "map";
  return Object.hasOwn(pages, page) ? page : "map";
}

function mountPolicyAssertionEditor() {
  const assertionEditor = byId("policy-assertion-editor");
  const editorPane = byId("policy-editor-pane");
  const historyHeading = byId("policy-history-heading");
  if (assertionEditor && editorPane && assertionEditor.parentElement !== editorPane) {
    if (historyHeading?.parentElement === editorPane) editorPane.insertBefore(assertionEditor, historyHeading);
    else editorPane.append(assertionEditor);
  }
}

function showPage(page = currentPage()) {
  for (const view of document.querySelectorAll(".page-view")) {
    view.hidden = view.dataset.page !== page;
    view.classList.toggle("active", !view.hidden);
  }
  for (const link of document.querySelectorAll("[data-page-link]")) {
    const active = link.dataset.pageLink === page;
    link.classList.toggle("active", active);
    if (active) link.setAttribute("aria-current", "page");
    else link.removeAttribute("aria-current");
  }
  if (page === "policy") loadPolicyWorkspace();
  if (page === "dns") loadDNSStats();
}

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  })[char]);
}

function actorDisplayName(actor) {
  return dnsNameForAddress(actor?.zpr_addr) || actor?.cn || "—";
}

function isGatewayService(service) {
  return String(service?.service_kind || "").toLowerCase() === "gateway"
    || Boolean(service?.external_network_connection);
}

function num(value) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatNumber(value) {
  return num(value).toLocaleString();
}

function dnsNumber(counters, key) {
  return formatNumber(counters?.[key] || 0);
}

function renderDNSStats(status, server, zones) {
  const counters = server.nsstats || {};
  const requests = num(counters.Requestv4) + num(counters.Requestv6);
  byId("dns-stats-status").textContent = `BIND ${server.version || "9"} · Updated ${new Date(status["current-time"]).toLocaleTimeString()}`;
  byId("dns-stat-requests").textContent = formatNumber(requests);
  byId("dns-stat-success").textContent = dnsNumber(counters, "QrySuccess");
  byId("dns-stat-nxdomain").textContent = dnsNumber(counters, "QryNXDOMAIN");
  byId("dns-stat-servfail").textContent = dnsNumber(counters, "QrySERVFAIL");

  const counterRows = [
    ["IPv4 requests", "Requestv4"], ["IPv6 requests", "Requestv6"],
    ["UDP queries", "QryUDP"], ["TCP queries", "QryTCP"],
    ["Authoritative answers", "QryAuthAns"], ["Successful queries", "QrySuccess"],
    ["NXDOMAIN", "QryNXDOMAIN"], ["SERVFAIL", "QrySERVFAIL"],
    ["Updates completed", "UpdateDone"], ["Updates failed", "UpdateFail"],
  ];
  byId("dns-counter-rows").innerHTML = counterRows.map(([label, key]) =>
    `<tr><td>${label}</td><td class="mono">${dnsNumber(counters, key)}</td></tr>`
  ).join("");

  const zoneRows = Object.entries(zones.views || {}).flatMap(([viewName, view]) =>
    (view.zones || []).map((zone) => ({ ...zone, view: viewName }))
  );
  byId("dns-zone-rows").innerHTML = zoneRows.length ? zoneRows.map((zone) =>
    `<tr><td>${escapeHTML(zone.name || "—")}<small class="dns-zone-view">${escapeHTML(zone.view)}</small></td><td>${escapeHTML(zone.type || "—")}</td><td class="mono">${escapeHTML(zone.serial ?? "—")}</td><td class="mono">${dnsNumber(zone.rcodes, "QrySuccess")}</td><td class="mono">${dnsNumber(zone.rcodes, "QryNXDOMAIN")}</td><td class="mono">${dnsNumber(zone.qtypes, "AAAA")}</td></tr>`
  ).join("") : `<tr><td colspan="6" class="empty-row">No zone statistics returned</td></tr>`;
}

async function loadDNSStats() {
  if (state.dnsPending) return;
  state.dnsPending = true;
  if (byId("dns-stats-status").textContent.startsWith("Waiting")) byId("dns-stats-status").textContent = "Loading BIND statistics…";
  const recordsRequest = loadDNSRecords(true);
  try {
    const paths = ["status", "server", "zones"];
    const responses = await Promise.all(paths.map((path) => fetch(`/api/dns/stats/json/v1/${path}`, { cache: "no-store", headers: { Accept: "application/json" } })));
    const failed = responses.find((response) => !response.ok);
    if (failed) throw new Error(`HTTP ${failed.status}`);
    const [status, server, zones] = await Promise.all(responses.map((response) => response.json()));
    renderDNSStats(status, server, zones);
  } catch (error) {
    byId("dns-stats-status").textContent = `DNS statistics unavailable (${error.message})`;
    byId("dns-counter-rows").innerHTML = `<tr><td colspan="2" class="empty-row">Unable to load DNS counters</td></tr>`;
    byId("dns-zone-rows").innerHTML = `<tr><td colspan="6" class="empty-row">Unable to load zone statistics</td></tr>`;
  } finally {
    await recordsRequest;
    state.dnsPending = false;
  }
}

let dnsAddressNames = new Map();
let dnsRecordsPending = false;
let dnsRecordsNextRefresh = 0;

function dnsAddressKey(address) {
  const value = String(address || "").trim();
  try {
    return new URL(`http://${value.includes(":") && !value.startsWith("[") ? `[${value}]` : value}/`).hostname.toLowerCase();
  } catch {
    return value.toLowerCase();
  }
}

function dnsNameForAddress(address) {
  if (!address) return "";
  const names = dnsAddressNames.get(dnsAddressKey(address)) || [];
  const preferred = names.find((name) => !name.includes(".adapters.svc.zpr.")) || names[0];
  return preferred ? preferred.replace(/\.$/, "") : "";
}

function dnsAddressLabel(address, fallback = "No DNS name") {
  return address ? dnsNameForAddress(address) || fallback : "—";
}

function dnsAddressTitle(address, prefix = "IP address") {
  return address ? `${prefix}: ${address}` : "";
}

function indexDNSAddresses(records) {
  const byName = new Map();
  for (const record of records) {
    if (!["A", "AAAA", "CNAME"].includes(record.type)) continue;
    const name = record.name.toLowerCase().replace(/\.$/, "");
    const entries = byName.get(name) || [];
    entries.push(record);
    byName.set(name, entries);
  }
  const addressesForName = (name, visited = new Set()) => {
    if (visited.has(name)) return [];
    visited.add(name);
    return (byName.get(name) || []).flatMap((record) => record.type === "CNAME"
      ? addressesForName(record.value.toLowerCase().replace(/\.$/, ""), visited)
      : [dnsAddressKey(record.value)]);
  };
  const namesByAddress = new Map();
  for (const name of [...byName.keys()].sort()) {
    for (const address of new Set(addressesForName(name))) {
      const names = namesByAddress.get(address) || [];
      names.push(name);
      namesByAddress.set(address, names);
    }
  }
  return namesByAddress;
}

async function loadDNSRecords(force = false) {
  if (dnsRecordsPending || (!force && Date.now() < dnsRecordsNextRefresh)) return;
  dnsRecordsPending = true;
  dnsRecordsNextRefresh = Date.now() + 60000;
  const status = byId("dns-record-status");
  const rows = byId("dns-record-rows");
  if (status.textContent.startsWith("Waiting")) status.textContent = "Loading zone records…";
  try {
    const response = await fetch("/api/dns/records", { cache: "no-store", headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const result = await response.json();
    const records = Array.isArray(result.records) ? result.records : [];
    dnsAddressNames = indexDNSAddresses(records);
    status.textContent = `${escapeHTML(result.zone || "DNS zone")} · ${formatNumber(records.length)} records`;
    rows.innerHTML = records.length ? records.map((record) =>
      `<tr><td class="mono">${escapeHTML(record.name || "—")}</td><td class="mono">${escapeHTML(record.ttl ?? "—")}</td><td>${escapeHTML(record.type || "—")}</td><td class="mono dns-record-value">${escapeHTML(record.value || "—")}</td></tr>`
    ).join("") : `<tr><td colspan="4" class="empty-row">No records returned</td></tr>`;
    if (state.snapshot) {
      render(state.snapshot);
    }
  } catch (error) {
    status.textContent = `DNS records unavailable (${error.message})`;
    rows.innerHTML = `<tr><td colspan="4" class="empty-row">Unable to load zone records</td></tr>`;
  } finally {
    dnsRecordsPending = false;
  }
}

function visibleRows(page, rows, columns) {
  const query = byId(`${page === "actors" ? "actor-search" : `${page}-filter`}`).value.trim().toLowerCase();
  const filtered = rows.filter((row) => !query || Object.values(columns).some((value) => String(value(row) ?? "").toLowerCase().includes(query)));
  const sort = state.sorts[page];
  if (!sort) return filtered;
  return filtered.sort((left, right) => {
    const a = columns[sort.key](left) ?? "";
    const b = columns[sort.key](right) ?? "";
    const comparison = typeof a === "number" && typeof b === "number"
      ? a - b : String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: "base" });
    return comparison * sort.direction;
  });
}

function formatDuration(seconds) {
  let remaining = Math.max(0, Math.floor(num(seconds)));
  const days = Math.floor(remaining / 86400); remaining %= 86400;
  const hours = Math.floor(remaining / 3600); remaining %= 3600;
  const minutes = Math.floor(remaining / 60);
  if (days) return `${days}d ${hours}h`;
  if (hours) return `${hours}h ${minutes}m`;
  return `${minutes}m ${remaining % 60}s`;
}

function since(seconds) {
  if (!seconds) return "not reported";
  const age = Math.max(0, Math.floor(Date.now() / 1000 - Number(seconds)));
  if (age < 60) return `${age}s ago`;
  if (age < 3600) return `${Math.floor(age / 60)}m ago`;
  return `${Math.floor(age / 3600)}h ago`;
}

function endpoint(source, port, destination, destPort) {
  const left = port == null ? (source || "?") : `${source || "?"}:${port}`;
  const right = destPort == null ? (destination || "?") : `${destination || "?"}:${destPort}`;
  return `${left}  →  ${right}`;
}

function visaEndpoint(source, port, destination, destPort) {
  return endpoint(dnsAddressLabel(source), port, dnsAddressLabel(destination), destPort);
}

function visaEndpointTitle(source, destination) {
	return [dnsAddressTitle(source, "Source IP"), dnsAddressTitle(destination, "Destination IP")].filter(Boolean).join(" · ");
}

function updateConnection(snapshot) {
  const stateEl = byId("connection-state");
  const apiState = snapshot.api_status || "disconnected";
  stateEl.dataset.state = apiState === "connected" ? "connected" : apiState === "partial" ? "partial" : "disconnected";
  byId("api-state-text").textContent = apiState === "connected" ? "Visa Service connected" : apiState === "partial" ? "Partial API response" : apiState === "not configured" ? "Admin API not configured" : "Visa Service unavailable";
  byId("last-updated").textContent = snapshot.generated_at ? `Updated ${new Date(snapshot.generated_at).toLocaleTimeString()}` : "Waiting for first snapshot";

  const issues = [];
  if (snapshot.config_error) issues.push(snapshot.config_error);
  if (snapshot.errors?.length) issues.push(...snapshot.errors);
  const alert = byId("alert-strip");
  if (issues.length) {
    alert.hidden = false;
    alert.innerHTML = `<strong>Some live data is unavailable.</strong> ${issues.map(escapeHTML).join(" · ")}`;
  } else {
    alert.hidden = true;
    alert.textContent = "";
  }
}

function countUpLinks(network) {
  return (network || []).filter((item) => item.ctype === "UP").length;
}

function renderMetrics(data) {
  const stats = data.stats || {};
  const upLinks = countUpLinks(data.network);
  const nodes = data.actors.filter((item) => item.node);
  const inSync = nodes.filter((node) => node.node_details?.in_sync).length;
  const approved = num(stats.visa_requests_approved);
  const denied = num(stats.visa_requests_denied);

  byId("metric-visas").textContent = formatNumber(data.visa_count);
  byId("metric-nodes").textContent = `${inSync} / ${nodes.length}`;
  byId("metric-nodes-note").textContent = `${upLinks} inter-node links reported UP`;
  byId("metric-allowed").textContent = formatNumber(approved);
  byId("metric-denied").textContent = formatNumber(denied);
  byId("metric-actors").textContent = formatNumber(data.actors.length);
  byId("metric-adapters").textContent = formatNumber(data.actors.filter((actor) => !actor.node).length);
  byId("metric-services").textContent = formatNumber(data.services.length);
  byId("metric-uptime").textContent = stats.uptime == null ? "—" : formatDuration(stats.uptime);
  for (const id of ["metric-nodes-note"]) {
    byId(id).parentElement.title = byId(id).textContent;
  }
}

function detailField(label, value, className = "", title = "") {
  const titleAttribute = title ? ` title="${escapeHTML(title)}"` : "";
  return `<div class="detail-field"><dt>${escapeHTML(label)}</dt><dd class="${className}"${titleAttribute}>${escapeHTML(value == null || value === "" ? "—" : value)}</dd></div>`;
}

function detailDNSAddressField(label, address, fallback = "No DNS name", titlePrefix = "IP address") {
  return detailField(label, dnsAddressLabel(address, fallback), "mono", dnsAddressTitle(address, titlePrefix));
}
function detailHTMLField(label, markup) {
  return `<div class="detail-field"><dt>${escapeHTML(label)}</dt><dd>${markup || "—"}</dd></div>`;
}

function detailSection(title, fields) {
  return `<section class="detail-section"><h3>${escapeHTML(title)}</h3><dl>${fields.join("")}</dl></section>`;
}

let adapterVisaDetails;

function refreshAdapterVisas(actor) {
  if (adapterVisaDetails?.key === actor.cn && (adapterVisaDetails.pending || adapterVisaDetails.snapshot === state.snapshot)) return;
  const previous = adapterVisaDetails?.key === actor.cn ? adapterVisaDetails : null;
  adapterVisaDetails?.controller.abort();
  const request = { key: actor.cn, snapshot: state.snapshot, pending: true, loaded: previous?.loaded || false, items: previous?.items || [], error: "", controller: new AbortController() };
  adapterVisaDetails = request;
  fetch(`/api/actors/${encodeURIComponent(actor.cn)}/visas`, { cache: "no-store", signal: request.controller.signal }).then(async (response) => {
    if (!response.ok) throw new Error(`Current visas unavailable (HTTP ${response.status})`);
    const items = await response.json();
    if (!Array.isArray(items)) throw new Error("Invalid current visa response");
    if (adapterVisaDetails !== request) return;
    request.items = items;
    request.loaded = true;
    request.pending = false;
    request.snapshot = state.snapshot;
    renderInspector();
  }).catch((error) => {
    if (error.name === "AbortError" || adapterVisaDetails !== request) return;
    request.pending = false;
    request.error = error.message;
    request.snapshot = state.snapshot;
    renderInspector();
  });
}

function openInspector(kind, key) {
  if (state.selection?.kind !== kind || state.selection?.key !== key) {
    adapterVisaDetails?.controller.abort();
    adapterVisaDetails = null;
  }
  state.selection = { kind, key };
  renderInspector();
}

function closeInspector() {
  adapterVisaDetails?.controller.abort();
  adapterVisaDetails = null;
  state.selection = null;
  const panel = byId("component-inspector");
  panel.classList.remove("open");
  panel.setAttribute("aria-hidden", "true");
}

function renderInspector() {
  if (!state.selection || !state.snapshot) return;
  const data = state.snapshot;
  const panel = byId("component-inspector");
  const body = byId("inspector-body");
  let kindLabel = "COMPONENT";
  let title = state.selection.key;
  let sections = [];

  if (state.selection.kind === "actor") {
    const actor = data.actors.find((item) => item.cn === state.selection.key);
    if (!actor) return closeInspector();
    const services = data.services.filter((item) => item.actor_cn === actor.cn);
    const gatewayService = services.find(isGatewayService);
    const relatedVisas = data.recent_visas.filter((visa) => visa.source_addr === actor.zpr_addr || visa.dest_addr === actor.zpr_addr);
    kindLabel = actor.node ? "FORWARDING NODE" : gatewayService ? "ZPR GATEWAY" : services.some((item) => item.service_kind === "Visa") ? "VISA SERVICE ADAPTER" : "ADAPTER";
    title = actorDisplayName(actor);
    sections.push(detailSection("Actor", [
		 detailField("Role", actor.node ? "Node / forwarder" : gatewayService ? "External-network gateway" : kindLabel === "VISA SERVICE ADAPTER" ? "Visa Service adapter" : "Adapter"),
      detailField("Common name", actor.cn, "mono"),
      detailDNSAddressField("DNS name", actor.zpr_addr, actor.cn),
      detailField("Authentication expires", actor.auth_exp ? new Date(actor.auth_exp * 1000).toLocaleString() : "No expiry reported"),
    ]));
    if (gatewayService) {
      sections.push(detailSection("Gateway boundary", [
        detailField("External network", gatewayService.external_network_connection || "Declared gateway"),
        detailField("Authorized service", gatewayService.service_name),
        detailField("Endpoint", gatewayService.service_endpoints),
      ]));
    }
    if (actor.node && actor.node_details) {
      const details = actor.node_details;
      const attached = details.adapters || [];
      const outgoing = (details.links || []).map((name) => {
        const linkedActor = data.actors.find((item) => item.cn === name);
        return `<button class="detail-link" type="button" data-inspect-actor="${escapeHTML(name)}">${escapeHTML(actorDisplayName(linkedActor || { cn: name }))}</button>`;
      }).join(" ");
      sections.push(detailSection("Live node state", [
        detailField("Synchronization", details.in_sync ? "In sync" : "Not in sync"),
        detailField("Last contact", since(details.last_contact)),
        detailField("Pending visa installs", details.pending_install),
        detailField("Pending revocations", details.pending_revocation),
        detailField("Visa requests", details.visa_requests),
        detailField("Approved / denied", `${details.approved_vreqs} / ${details.denied_vreqs}`),
        detailField("Installed visas", (details.visas || []).length),
        detailHTMLField("Docked adapters", attached.map((name) => {
          const linkedActor = data.actors.find((item) => item.cn === name);
          return `<button class="detail-link" type="button" data-inspect-actor="${escapeHTML(name)}">${escapeHTML(actorDisplayName(linkedActor || { cn: name }))}</button>`;
        }).join(" ")),
        detailHTMLField("Node links", outgoing),
      ]));
    } else {
      refreshAdapterVisas(actor);
      const attachedTo = data.actors.filter((node) => node.node && (node.node_details?.adapters || []).includes(actor.cn));
      sections.push(detailSection("Live attachment", [
        detailField("Docked to", attachedTo.map((node) => node.cn).join(", ") || "No dock reported"),
        detailField("Current visas", !adapterVisaDetails.loaded ? adapterVisaDetails.pending ? "Loading" : "Unavailable" : adapterVisaDetails.items.filter((visa) => num(visa.expires) > Date.now() / 1000).length),
        detailField("Services registered", services.map((item) => item.service_name).join(", ") || "None"),
      ]));
    }
    if (services.length) {
      sections.push(`<section class="detail-section"><h3>Service registrations</h3><div class="detail-list">${services.map((service) => `<button class="detail-item" type="button" data-inspect-service="${escapeHTML(service.service_name)}"><strong>${escapeHTML(service.service_name)}</strong><span>${escapeHTML(service.service_kind)} · ${escapeHTML(service.service_endpoints || "no endpoints")}</span></button>`).join("")}</div></section>`);
    }
    if (!actor.node) {
      const current = adapterVisaDetails.items.filter((visa) => num(visa.expires) > Date.now() / 1000);
      const content = !adapterVisaDetails.loaded ? adapterVisaDetails.pending ? `<p>Loading current visas...</p>` : "" : current.length ? `<div class="detail-list">${current.map((visa) => `<div class="detail-item"><strong>Visa ${escapeHTML(visa.id)} · ${escapeHTML(visa.proto)}</strong><span title="${escapeHTML(visaEndpointTitle(visa.source_addr, visa.dest_addr))}">${escapeHTML(visaEndpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port))}</span><span>Direction: ${escapeHTML(visa.direction || "Not reported")}</span><span>Expires: ${escapeHTML(new Date(num(visa.expires) * 1000).toLocaleString())}</span><span>Node: ${escapeHTML(visa.requesting_node || "Not reported")} · Policy: ${escapeHTML(visa.policy_id || "Not reported")}</span></div>`).join("")}</div>` : `<p>No current visas.</p>`;
      const error = adapterVisaDetails.error ? `<p role="status">${escapeHTML(adapterVisaDetails.error)}${adapterVisaDetails.loaded ? "; showing last successful result." : ""}</p>` : "";
      sections.push(`<section class="detail-section"><h3>Current visas</h3>${error}${content}</section>`);
    } else if (relatedVisas.length) sections.push(`<section class="detail-section"><h3>Recent visas</h3><div class="detail-list">${relatedVisas.slice(0, 6).map((visa) => `<div class="detail-item"><strong>Visa ${escapeHTML(visa.id)} · ${escapeHTML(visa.proto)}</strong><span title="${escapeHTML(visaEndpointTitle(visa.source_addr, visa.dest_addr))}">${escapeHTML(visaEndpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port))}</span></div>`).join("")}</div></section>`);
  } else if (state.selection.kind === "service") {
    const service = data.services.find((item) => item.service_name === state.selection.key);
    if (!service) return closeInspector();
    const isTrusted = (service.service_kind || "").startsWith("Trusted(");
    const isGateway = isGatewayService(service);
    kindLabel = isGateway ? "ZPR GATEWAY SERVICE" : isTrusted ? "TRUSTED SOURCE" : "REGISTERED SERVICE";
    title = service.service_name;
    const actor = data.actors.find((item) => item.cn === service.actor_cn);
    const source = data.trusted_sources.find((item) => item.name === service.service_name);
    sections.push(detailSection("Configuration", [
      detailField("Kind", service.service_kind), detailField("Provider adapter", actorDisplayName(actor), "mono"),
      detailField("Provider common name", service.actor_cn, "mono"),
      ...(isGateway ? [detailField("External network", service.external_network_connection || "Gateway") ] : []),
      detailDNSAddressField("DNS name", service.zpr_addr),
      detailField("Endpoints", service.service_endpoints),
    ]));
    sections.push(detailSection("Live state", [
      detailField("Actor present", actor ? "Present in Visa Service" : "Not returned"),
      detailField("Last lookup", isTrusted ? (source?.health || "Unreported") : "Not applicable"),
      detailField("Observed at", source?.last_lookup_ms ? new Date(source.last_lookup_ms).toLocaleString() : "Not reported"),
    ]));
  } else if (state.selection.kind === "source") {
    const source = data.trusted_sources.find((item) => item.name === state.selection.key);
    if (!source) return closeInspector();
    kindLabel = "TRUSTED SOURCE";
    title = source.name;
    sections.push(detailSection("Who uses it", [
      detailField("Actor", source.actor_cn, "mono"),
      detailField("Provider", providerDescription(source.provider)),
      detailDNSAddressField("DNS name", source.zpr_addr),
      detailField("Configured endpoint", source.service_endpoints || (source.provider === "rest/1" ? "Managed by Visa Service" : "No network endpoint"), "mono"),
    ]));
    sections.push(detailSection("Lookup status", [
      detailField("Last outcome", lookupOutcome(source.health)),
      detailField("Last lookup", source.last_lookup_ms ? new Date(source.last_lookup_ms).toLocaleString() : "Never observed"),
      detailField("Last success", source.last_success_ms ? new Date(source.last_success_ms).toLocaleString() : "Never observed"),
      detailField("Basis", source.health_note || "No lookup status from the admin API"),
    ]));
    if (source.editor_url) {
      sections.push(`<div class="inspector-actions"><a class="button button-refresh" href="${escapeHTML(source.editor_url)}" target="_blank" rel="noopener noreferrer">Open LDAP editor ↗</a></div>`);
    }
  } else if (state.selection.kind === "link") {
    const [kind, fromName, toName] = state.selection.key.split("|");
    const from = data.actors.find((actor) => actor.cn === fromName);
    const to = data.actors.find((actor) => actor.cn === toName);
    if (!from || !to) return closeInspector();
    const link = kind === "network" ? (data.network || []).find((item) =>
      item.node_a_addr === from.zpr_addr && item.node_b_addr === to.zpr_addr) : null;
    if (kind === "network" && !link) return closeInspector();
    kindLabel = kind === "dock" ? "DOCK CONNECTION" : "INTER-NODE LINK";
    title = `${fromName} ↔ ${toName}`;
    sections.push(detailSection("Endpoints", [detailField("From", actorDisplayName(from)), detailDNSAddressField("From DNS name", from.zpr_addr, fromName), detailField("To", actorDisplayName(to)), detailDNSAddressField("To DNS name", to.zpr_addr, toName)]));
    sections.push(detailSection("Live state", kind === "dock" ? [
      detailField("Attachment", (from.node_details?.adapters || []).includes(toName) ? "Docked" : "Unreported"),
    ] : [
      detailField("Status", link.ctype), detailField("Link ID", link.link_id), detailField("Cost", link.link_cost),
      detailDNSAddressField("Node A DNS name", from.zpr_addr, fromName, `Node/substrate IP: ${from.zpr_addr || "not reported"} / ${link.node_a_substrate || "not reported"}`), detailDNSAddressField("Node B DNS name", to.zpr_addr, toName, `Node/substrate IP: ${to.zpr_addr || "not reported"} / ${link.node_b_substrate || "not reported"}`),
    ]));
  }

  byId("inspector-kind").textContent = kindLabel;
  byId("inspector-title").textContent = title;
  const markup = sections.join("");
  if (body.innerHTML !== markup) body.innerHTML = markup;
  panel.classList.add("open");
  panel.setAttribute("aria-hidden", "false");
}

function graphMotionAt(component) {
  const motion = component.graphMotion;
  if (!motion) return { x: 0, y: 0, scale: 1 };
  const progress = Math.min(1, (motion.animation.effect.getComputedTiming().progress ?? 1) / motion.endAt);
  return {
    x: motion.from.x + (motion.to.x - motion.from.x) * progress,
    y: motion.from.y + (motion.to.y - motion.from.y) * progress,
    scale: motion.from.scale + (motion.to.scale - motion.from.scale) * progress,
  };
}

function animateGraphMotion(component, animation, from, to, endAt = 1) {
  component.graphMotion = { animation, from, to, endAt };
  const update = () => {
    if (!component.isConnected) return;
    const { x, y, scale } = graphMotionAt(component);
    const centerX = Number(component.dataset.originX), centerY = Number(component.dataset.originY);
    component.setAttribute("transform", `translate(${x} ${y}) translate(${centerX} ${centerY}) scale(${scale}) translate(${-centerX} ${-centerY})`);
    if (animation.playState === "running") requestAnimationFrame(update);
  };
  update();
}

function renderTopology(data, exitComponents = []) {
  const nodes = data.actors.filter((actor) => actor.node).sort((a, b) => a.cn.localeCompare(b.cn));
  const adapters = data.actors.filter((actor) => !actor.node).sort((a, b) => a.cn.localeCompare(b.cn));
  const actors = [...nodes, ...adapters];
  const displayNames = new Map(actors.map((actor) => [actor.cn, actorDisplayName(actor)]));
  const actorsByName = new Map(actors.map((actor) => [actor.cn, actor]));
  const actorsByAddress = new Map(actors.map((actor) => [actor.zpr_addr, actor]));
  const edges = [];

  for (const node of nodes) {
    for (const adapterName of node.node_details?.adapters || []) {
      const adapter = actorsByName.get(adapterName);
      if (adapter) edges.push({ from: node, to: adapter, kind: "dock", state: "DOCKED" });
    }
  }
  for (const link of data.network || []) {
    const from = actorsByAddress.get(link.node_a_addr);
    const to = actorsByAddress.get(link.node_b_addr);
    if (from && to) edges.push({ from, to, kind: "network", state: link.ctype, detail: link });
  }

  const stage = byId("topology-stage");
  const positions = new Map();
  const servicePositions = new Map();
  const servicesByActor = new Map();
  for (const service of data.services || []) {
    if (!service.actor_cn) continue;
    const registered = servicesByActor.get(service.actor_cn) || [];
    registered.push(service);
    servicesByActor.set(service.actor_cn, registered);
  }
  const dockEdges = edges.filter((edge) => edge.kind === "dock");
  const networkEdges = edges.filter((edge) => edge.kind === "network");
  const unconnected = adapters.filter((adapter) => !dockEdges.some((edge) => edge.to.cn === adapter.cn));
  renderConnections(edges, unconnected);

  if (!actors.length && !exitComponents.length) {
    stage.innerHTML = `<div class="empty-state">No nodes or adapters reported.</div>`;
    return;
  }

  const visaServices = new Set((data.services || []).filter((service) => service.service_kind === "Visa").map((service) => service.actor_cn));
  const services = data.services || [];
  const gatewayActors = new Set(services.filter(isGatewayService).map((service) => service.actor_cn));
  const badgeWidthForService = (service) => Math.max(46, Math.min(132, Math.min(service.service_name.length, 22) * 5.6 + 16));
  const serviceRingRadius = (actorName) => {
    const registered = servicesByActor.get(actorName) || [];
    if (!registered.length) return 0;
    const circumference = registered.reduce((sum, service) => sum + badgeWidthForService(service) + 14, 0);
    return Math.max(64, circumference / (2 * Math.PI));
  };
  const actorRadius = (actor) => actor.node ? 54 : gatewayActors.has(actor.cn) ? 42 : visaServices.has(actor.cn) ? 39 : 32;
  const actorExtent = (actor) => actorRadius(actor) + serviceRingRadius(actor.cn) + (gatewayActors.has(actor.cn) ? 160 : 0);
  const attachedByNode = new Map(nodes.map((node) => [
    node.cn,
    (node.node_details?.adapters || [])
      .map((name) => actorsByName.get(name))
      .filter(Boolean)
      .sort((a, b) => a.cn.localeCompare(b.cn)),
  ]));
  const clusterRadius = (node) => {
    const attached = attachedByNode.get(node.cn) || [];
    const extentSum = attached.reduce((sum, actor) => sum + actorExtent(actor) + 18, 0);
    const ringRequirement = attached.length ? (2 * extentSum + attached.length * 20) / (2 * Math.PI) + 26 : 280;
    return Math.max(340, ringRequirement);
  };
  if (nodes.length && !state.topologyNodeColumns) state.topologyNodeColumns = Math.ceil(Math.sqrt(nodes.length));
  const nodeColumns = Math.max(1, state.topologyNodeColumns || Math.ceil(Math.sqrt(nodes.length)));
  let nextNodeSlot = Math.max(-1, ...state.topologyNodeSlots.values()) + 1;
  for (const node of nodes) {
    if (!state.topologyNodeSlots.has(node.cn)) state.topologyNodeSlots.set(node.cn, nextNodeSlot++);
  }
  const nodeRows = Math.max(1, ...nodes.map((node) => Math.floor(state.topologyNodeSlots.get(node.cn) / nodeColumns) + 1));
  const maximumClusterRadius = Math.max(340, ...nodes.map(clusterRadius));
  const margin = maximumClusterRadius + 150;
  const nodeSpacing = maximumClusterRadius * 2 + 180;
  nodes.forEach((node) => {
    const attached = attachedByNode.get(node.cn) || [];
    const maxChildExtent = Math.max(0, ...attached.map(actorExtent));
    const slot = state.topologyNodeSlots.get(node.cn);
    const nodeCenter = {
      x: margin + maxChildExtent + (slot % nodeColumns) * nodeSpacing,
      y: margin + maxChildExtent + Math.floor(slot / nodeColumns) * nodeSpacing,
    };
    positions.set(node.cn, nodeCenter);
    if (!attached.length) return;
    const radius = clusterRadius(node);
    const extents = attached.map((actor) => actorExtent(actor) + 18);
    const gaps = attached.map((_, slot) => {
      const next = (slot + 1) % attached.length;
      return (extents[slot] + extents[next] + 20) / radius;
    });
    const spareAngle = Math.max(0, 2 * Math.PI - gaps.reduce((sum, gap) => sum + gap, 0)) / attached.length;
    let angle = -Math.PI / 2;
    attached.forEach((adapter, slot) => {
      positions.set(adapter.cn, { x: nodeCenter.x + Math.cos(angle) * radius, y: nodeCenter.y + Math.sin(angle) * radius });
      angle += gaps[slot] + spareAngle;
    });
  });
  const unconnectedHosts = adapters.filter((host) => !positions.has(host.cn));
  const unconnectedColumns = Math.max(1, Math.min(4, unconnectedHosts.length));
  const hostRingRadius = Math.max(64, ...unconnectedHosts.map((host) => actorExtent(host) + 18));
  unconnectedHosts.forEach((host, index) => {
    const column = index % unconnectedColumns;
    const row = Math.floor(index / unconnectedColumns);
    positions.set(host.cn, {
      x: margin + column * (hostRingRadius * 2 + 100),
      y: margin + nodeRows * nodeSpacing + row * (hostRingRadius * 2 + 100),
    });
  });
  for (const service of services) {
    const ownerPosition = positions.get(service.actor_cn);
    const registered = servicesByActor.get(service.actor_cn) || [];
    if (!ownerPosition || !registered.length) continue;
    const index = registered.indexOf(service);
    const angle = -Math.PI / 2 + (2 * Math.PI * index) / registered.length;
    const radius = serviceRingRadius(service.actor_cn);
    servicePositions.set(service, {
      x: ownerPosition.x + Math.cos(angle) * radius,
      y: ownerPosition.y + Math.sin(angle) * radius,
    });
  }
  const oldGraph = stage.querySelector(".topology-graph");
  const oldViewBox = oldGraph?.viewBox.baseVal;
  const previousViewport = oldGraph ? {
    width: oldViewBox.width,
    height: oldViewBox.height,
    clientWidth: oldGraph.clientWidth,
    clientHeight: oldGraph.clientHeight,
  } : null;
  const width = Math.max(760, oldViewBox?.width || 0, 2 * margin + 2 * maximumClusterRadius + (nodeColumns - 1) * nodeSpacing, margin * 2 + (unconnectedColumns - 1) * (hostRingRadius * 2 + 100));
  const height = Math.max(460, oldViewBox?.height || 0, 2 * margin + 2 * maximumClusterRadius + (nodeRows - 1) * nodeSpacing, margin + nodeRows * nodeSpacing + unconnectedHosts.length * (hostRingRadius * 2 + 100));
  const query = byId("topology-search").value.trim().toLowerCase();
  const matches = (actor) => !query || `${displayNames.get(actor.cn)} ${actor.cn} ${actor.zpr_addr || ""} ${visaServices.has(actor.cn) ? "visa service" : ""} ${(servicesByActor.get(actor.cn) || []).map((service) => `${service.service_name} ${service.service_kind} ${service.external_network_connection || ""}`).join(" ")}`.toLowerCase().includes(query);

  const edgeMarkup = edges.map((edge) => {
    const from = positions.get(edge.from.cn), to = positions.get(edge.to.cn);
    const docked = edge.kind === "dock";
    const up = docked || edge.state === "UP";
    const cls = docked ? "dock-link" : up ? "up" : "down";
    const title = docked
      ? `${displayNames.get(edge.to.cn)} docked to ${displayNames.get(edge.from.cn)} · ${dnsAddressTitle(edge.to.zpr_addr, "Adapter IP")} · ${dnsAddressTitle(edge.from.zpr_addr, "Node IP")}`
      : `${displayNames.get(edge.from.cn)} to ${displayNames.get(edge.to.cn)}: ${edge.state} · ${dnsAddressTitle(edge.from.zpr_addr, "Node A IP")} · ${dnsAddressTitle(edge.to.zpr_addr, "Node B IP")}`;
    const highlighted = query && (matches(edge.from) || matches(edge.to)) ? "highlighted" : "";
    const filtered = query && !highlighted ? "filtered" : "";
    const key = `${edge.kind}|${edge.from.cn}|${edge.to.cn}`;
    return `<g class="graph-edge ${highlighted} ${filtered}" data-inspect-link="${escapeHTML(key)}" tabindex="0" role="button" aria-label="Inspect ${escapeHTML(title)}"><title>${escapeHTML(title)}</title><line class="graph-link ${cls}" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"/><line class="graph-link-hit" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"/></g>`;
  }).join("");

  const arrivalMarker = (key, x, y, radius) => {
    if (!state.graphAnimations) return "";
    const startedAt = state.topologyNewComponents.get(key);
    if (startedAt == null) return "";
    const elapsed = Date.now() - startedAt;
    if (elapsed >= GRAPH_ARRIVAL_DURATION) {
      state.topologyNewComponents.delete(key);
      return "";
    }
    return `<g class="graph-arrival-marker" data-arrival-age="${elapsed}" aria-hidden="true"><circle class="graph-arrival-ring" data-origin-x="${x}" data-origin-y="${y}" cx="${x}" cy="${y}" r="${radius}"/></g>`;
  };
  const arrivalOffset = (parent, target) => {
    if (!parent) return null;
    return { x: parent.x - target.x, y: parent.y - target.y };
  };
  const previousMovement = new Map([...stage.querySelectorAll("#graph-world > [data-topology-component]")].map((component) => [component.dataset.topologyComponent, graphMotionAt(component)]));
  const enteringOffsets = new Map();
  const offsetAttributes = (offset, parentKey, enteringOffset) => `${parentKey ? ` data-topology-parent="${escapeHTML(parentKey)}"` : ""}${offset ? ` data-arrival-dx="${offset.x}" data-arrival-dy="${offset.y}"` : ""}${enteringOffset ? ` data-entry-dx="${enteringOffset.x}" data-entry-dy="${enteringOffset.y}"` : ""}`;
  const vertexMarkup = actors.map((actor) => {
    const pos = positions.get(actor.cn);
    const displayName = displayNames.get(actor.cn) || actor.cn;
    const shortName = displayName.length > 20 ? `${displayName.slice(0, 18)}…` : displayName;
    const isNode = actor.node;
    const isGateway = !isNode && gatewayActors.has(actor.cn);
    const isVisaService = !isNode && visaServices.has(actor.cn);
    const synced = isNode && actor.node_details?.in_sync;
    const glyph = isNode
      ? `<rect class="graph-node ${synced ? "synced" : ""}" x="${pos.x - 46}" y="${pos.y - 25}" width="92" height="50" rx="7"/>`
      : isGateway
        ? `<polygon class="graph-gateway" points="${pos.x - 28},${pos.y - 16} ${pos.x},${pos.y - 32} ${pos.x + 28},${pos.y - 16} ${pos.x + 28},${pos.y + 16} ${pos.x},${pos.y + 32} ${pos.x - 28},${pos.y + 16}"/>`
      : isVisaService
        ? `<polygon class="graph-visa" points="${pos.x},${pos.y - 35} ${pos.x + 35},${pos.y} ${pos.x},${pos.y + 35} ${pos.x - 35},${pos.y}"/>`
        : `<circle class="graph-adapter" cx="${pos.x}" cy="${pos.y}" r="29"/>`;
    const componentKey = `actor:${JSON.stringify(actor.cn)}`;
    const marker = arrivalMarker(componentKey, pos.x, pos.y, actorRadius(actor) + 8);
    const arrivingClass = marker ? " arriving" : "";
    const parentNode = edges.find((edge) => edge.kind === "dock" && edge.to.cn === actor.cn)?.from;
    const parentKey = parentNode ? `actor:${JSON.stringify(parentNode.cn)}` : "";
    const offset = parentNode ? arrivalOffset(positions.get(parentNode.cn), pos) : null;
    const parentMovement = marker ? enteringOffsets.get(parentKey) || previousMovement.get(parentKey) : null;
    const entry = marker && offset && parentMovement ? { x: offset.x + parentMovement.x, y: offset.y + parentMovement.y } : null;
    if (marker && offset && !previousMovement.has(componentKey) && Date.now() - state.topologyNewComponents.get(componentKey) < 900) enteringOffsets.set(componentKey, entry || offset);
    const positionAttributes = offsetAttributes(offset, parentKey, entry);
    let cloudMarkup = "";
    if (isGateway) {
      const parentPosition = parentNode ? positions.get(parentNode.cn) : null;
      const angle = parentPosition ? Math.atan2(pos.y - parentPosition.y, pos.x - parentPosition.x) : 0;
      const distance = serviceRingRadius(actor.cn) + 110;
      const cloudX = pos.x + Math.cos(angle) * distance;
      const cloudY = pos.y + Math.sin(angle) * distance;
      const externalNetworks = [...new Set((servicesByActor.get(actor.cn) || []).filter(isGatewayService).map((service) => service.external_network_connection || "External network"))].join(", ");
      cloudMarkup = `<g class="graph-external-network"><title>${escapeHTML(externalNetworks)}</title><line class="graph-link gateway-cloud-link" x1="${pos.x}" y1="${pos.y}" x2="${cloudX}" y2="${cloudY}"/><path class="graph-cloud" transform="translate(${cloudX} ${cloudY})" d="M -34 22 C -60 22 -62 -12 -39 -17 C -40 -43 -2 -49 9 -28 C 31 -42 52 -22 46 -5 C 68 0 62 22 42 22 Z"/></g>`;
    }
    const highlighted = query && matches(actor) ? "highlighted" : "";
    return `<g class="graph-vertex ${isNode ? "node" : isGateway ? "gateway" : isVisaService ? "visa" : "adapter"}${arrivingClass} ${highlighted} ${query && !matches(actor) ? "filtered" : ""}" data-topology-component="${escapeHTML(componentKey)}" data-origin-x="${pos.x}" data-origin-y="${pos.y}"${positionAttributes} data-inspect-actor="${escapeHTML(actor.cn)}" tabindex="0" role="button" aria-label="Inspect ${escapeHTML(isGateway ? "gateway " : "")}${escapeHTML(displayName)}"><title>${escapeHTML(isGateway ? "ZPR gateway · " : "")}${escapeHTML(displayName)} · ${escapeHTML(actor.cn)} · ${escapeHTML(dnsAddressTitle(actor.zpr_addr))}</title>${cloudMarkup}${glyph}${marker}<text class="graph-label" x="${pos.x}" y="${pos.y + 3}">${escapeHTML(shortName)}</text></g>`;
  });

  const serviceEdgeMarkup = [];
  const serviceMarkup = services.map((service) => {
    const owner = actorsByName.get(service.actor_cn);
    const ownerPosition = positions.get(service.actor_cn);
    const position = servicePositions.get(service);
    if (!owner || !ownerPosition || !position) return "";
    const label = service.service_name;
    const shortLabel = label.length > 22 ? `${label.slice(0, 21)}…` : label;
    const badgeWidth = Math.max(46, Math.min(132, shortLabel.length * 5.6 + 16));
    const dx = position.x - ownerPosition.x;
    const dy = position.y - ownerPosition.y;
    const distance = Math.hypot(dx, dy) || 1;
    const outwardX = dx / distance;
    const outwardY = dy / distance;
    const actorRadius = owner.node ? 49 : visaServices.has(owner.cn) ? 39 : 32;
    const startX = ownerPosition.x + outwardX * actorRadius;
    const startY = ownerPosition.y + outwardY * actorRadius;
    const endX = position.x - outwardX * (badgeWidth / 2);
    const endY = position.y - outwardY * 10;
    const type = serviceTypeAppearance(isGatewayService(service) ? "Gateway" : service.service_kind);
    const trustedType = (service.service_kind || "").match(/^Trusted\("([^\"]+)"\)$/)?.[1];
    const trustedClass = trustedType ? ` trusted-${trustedType.toLowerCase().replace(/[^a-z0-9]+/g, "-")}` : "";
    const gatewayClass = isGatewayService(service) ? " gateway" : "";
    const serviceMatches = `${service.service_name} ${service.service_kind} ${service.external_network_connection || ""}`.toLowerCase().includes(query);
    const highlighted = query && (serviceMatches || matches(owner)) ? "highlighted" : "";
    const filtered = query && !highlighted ? "filtered" : "";
    serviceEdgeMarkup.push(`<g class="graph-service-edge ${highlighted} ${filtered}" aria-hidden="true"><line class="graph-link service-link" x1="${startX}" y1="${startY}" x2="${endX}" y2="${endY}"/></g>`);
    const providerName = displayNames.get(owner.cn) || owner.cn;
    const title = `${isGatewayService(service) ? "Gateway · " : ""}${service.service_name} registered by ${providerName} (${owner.cn})${service.external_network_connection ? ` · external network: ${service.external_network_connection}` : ""}${service.zpr_addr ? ` · ${dnsAddressTitle(service.zpr_addr)}` : ""}`;
    const labelForScreenReader = `Inspect service ${service.service_name}, registered by ${providerName}`;
    const componentKey = `service:${JSON.stringify([service.actor_cn, service.service_name])}`;
    const marker = arrivalMarker(componentKey, position.x, position.y, badgeWidth / 2 + 8);
    const arrivingClass = marker ? " arriving" : "";
    const parentKey = `actor:${JSON.stringify(service.actor_cn)}`;
    const offset = arrivalOffset(ownerPosition, position);
    const parentMovement = marker ? enteringOffsets.get(parentKey) || previousMovement.get(parentKey) : null;
    const entry = marker && parentMovement ? { x: offset.x + parentMovement.x, y: offset.y + parentMovement.y } : null;
    const positionAttributes = offsetAttributes(offset, parentKey, entry);
    return `<g class="graph-service-badge${gatewayClass}${trustedClass}${arrivingClass} ${highlighted} ${filtered}" data-service-type="${escapeHTML(type.key)}" fill="${escapeHTML(type.background)}" stroke="${escapeHTML(type.border)}" color="${escapeHTML(type.color)}" data-topology-component="${escapeHTML(componentKey)}" data-origin-x="${position.x}" data-origin-y="${position.y}"${positionAttributes} data-inspect-service="${escapeHTML(service.service_name)}" tabindex="0" role="button" aria-label="${escapeHTML(labelForScreenReader)}"><title>${escapeHTML(title)}</title><rect x="${position.x - badgeWidth / 2}" y="${position.y - 10}" width="${badgeWidth}" height="20" rx="4"/>${marker}<text x="${position.x}" y="${position.y + 3}">${escapeHTML(shortLabel)}</text></g>`;
  });

  const exiting = (kind) => exitComponents.filter((component) => component.kind === kind).map((component) => component.markup).join("");
  stage.innerHTML = `<div class="graph-controls" aria-label="Topology graph controls"><button class="graph-control" data-graph-action="in" type="button" aria-label="Zoom in" title="Zoom in">+</button><button class="graph-control" data-graph-action="out" type="button" aria-label="Zoom out" title="Zoom out">−</button><button class="graph-control graph-fit" data-graph-action="fit" type="button" aria-label="Fit graph" title="Fit graph">Fit</button></div><svg class="topology-graph" viewBox="0 0 ${width} ${height}" role="img" aria-label="Topology graph with ${nodes.length} nodes, ${adapters.length} adapters, ${dockEdges.length} dock connections, ${networkEdges.length} inter-node links, and ${(data.services || []).filter((service) => servicesByActor.has(service.actor_cn)).length} registered services"><g id="graph-world">${edgeMarkup}${serviceEdgeMarkup.join("")}${serviceMarkup.join("")}<g class="graph-exit-layer" aria-hidden="true">${exiting("service")}</g>${vertexMarkup.slice(nodes.length).join("")}<g class="graph-exit-layer" aria-hidden="true">${exiting("adapter")}</g>${vertexMarkup.slice(0, nodes.length).join("")}<g class="graph-exit-layer" aria-hidden="true">${exiting("node")}</g></g></svg>`;

  setupGraphControls(stage, width, height, previousViewport);
}

function renderConnections(edges, unconnected) {
  byId("connection-count").textContent = `${formatNumber(edges.length)} Connections`;
  const rows = [
    ...edges.map((edge) => ({
      from: actorDisplayName(edge.from), to: actorDisplayName(edge.to),
      type: edge.kind === "dock" ? "DOCK" : "INTER-NODE",
      address: `${dnsAddressLabel(edge.from.zpr_addr)} ↔ ${dnsAddressLabel(edge.to.zpr_addr)}`,
      addressTitle: edge.kind === "dock"
        ? `${dnsAddressTitle(edge.from.zpr_addr, "From IP")} · ${dnsAddressTitle(edge.to.zpr_addr, "To IP")}`
        : `Node IPs: ${edge.from.zpr_addr || "not reported"} ↔ ${edge.to.zpr_addr || "not reported"} · Substrate IPs: ${edge.detail.node_a_substrate || "not reported"} ↔ ${edge.detail.node_b_substrate || "not reported"}`,
      state: edge.kind === "dock" ? "DOCKED" : edge.state,
      key: `${edge.kind}|${edge.from.cn}|${edge.to.cn}`,
    })),
    ...unconnected.map((actor) => ({ from: "—", to: actorDisplayName(actor), type: "ADAPTER", address: dnsAddressLabel(actor.zpr_addr), addressTitle: dnsAddressTitle(actor.zpr_addr), state: "NO NODE", actor: actor.cn })),
  ];
  const columns = { from: (row) => row.from, to: (row) => row.to, type: (row) => row.type, address: (row) => `${row.address} ${row.addressTitle}`, state: (row) => row.state };
  const shown = visibleRows("connections", rows, columns);
  byId("link-list").innerHTML = shown.length ? shown.map((row) => {
    const target = row.actor ? `data-inspect-actor="${escapeHTML(row.actor)}"` : `data-inspect-link="${escapeHTML(row.key)}"`;
    return `<tr class="selectable-row" ${target} tabindex="0" role="button" aria-label="Inspect ${escapeHTML(row.from)} to ${escapeHTML(row.to)}"><td>${escapeHTML(row.from)}</td><td>${escapeHTML(row.to)}</td><td><span class="role-chip ${row.type === "INTER-NODE" ? "node" : ""}">${row.type}</span></td><td class="mono" title="${escapeHTML(row.addressTitle || "")}">${escapeHTML(row.address)}</td><td><span class="mini-state ${["UP", "DOCKED"].includes(row.state) ? "up" : ""}">${escapeHTML(row.state)}</span></td></tr>`;
  }).join("") : `<tr><td colspan="5" class="empty-row">${rows.length ? "No matching connections" : "No dock or inter-node connections reported"}</td></tr>`;
}

function setupGraphControls(stage, width, height, previousViewport) {
  const svg = stage.querySelector(".topology-graph");
  const world = stage.querySelector("#graph-world");
  if (!state.graphCamera) state.graphCamera = { x: 0, y: 0, scale: 1 };
  const camera = state.graphCamera;
  const maxZoom = 1e6;
  const apply = () => world.setAttribute("transform", `translate(${camera.x} ${camera.y}) scale(${camera.scale})`);
  if (previousViewport?.clientWidth > 0 && previousViewport?.clientHeight > 0 && svg.clientWidth > 0 && svg.clientHeight > 0) {
    const previousScale = Math.min(previousViewport.clientWidth / previousViewport.width, previousViewport.clientHeight / previousViewport.height);
    const nextScale = Math.min(svg.clientWidth / width, svg.clientHeight / height);
    if (previousScale > 0 && nextScale > 0 && Math.abs(previousScale - nextScale) > 0.0001) {
      const ratio = previousScale / nextScale;
      const previousOffsetX = (previousViewport.clientWidth - previousScale * previousViewport.width) / 2;
      const previousOffsetY = (previousViewport.clientHeight - previousScale * previousViewport.height) / 2;
      const nextOffsetX = (svg.clientWidth - nextScale * width) / 2;
      const nextOffsetY = (svg.clientHeight - nextScale * height) / 2;
      const initialTransform = `translate(${previousOffsetX - ratio * nextOffsetX}px, ${previousOffsetY - ratio * nextOffsetY}px) scale(${ratio})`;
      if (reducedMotion) {
        svg.style.transformOrigin = "0 0";
        svg.style.transform = initialTransform;
      } else {
        svg.animate([
          { transformOrigin: "0 0", transform: initialTransform },
          { transformOrigin: "0 0", transform: "none" },
        ], { duration: 700, easing: "ease-in-out", fill: "both" });
      }
    }
  }
  const zoomAt = (nextScale, x = width / 2, y = height / 2) => {
    const scale = Math.max(0.55, Math.min(maxZoom, nextScale));
    const ratio = scale / camera.scale;
    camera.x = x - (x - camera.x) * ratio;
    camera.y = y - (y - camera.y) * ratio;
    camera.scale = scale;
    apply();
  };
  const pointAt = (event) => {
    const point = svg.createSVGPoint();
    point.x = event.clientX;
    point.y = event.clientY;
    return point.matrixTransform(svg.getScreenCTM().inverse());
  };

  stage.querySelectorAll("[data-graph-action]").forEach((button) => button.addEventListener("click", () => {
    const action = button.dataset.graphAction;
    if (action === "in") zoomAt(camera.scale * 1.25);
    if (action === "out") zoomAt(camera.scale / 1.25);
    if (action === "fit") {
      camera.x = 0;
      camera.y = 0;
      camera.scale = 1;
      svg.getAnimations().filter((animation) => animation.effect?.target === svg).forEach((animation) => animation.cancel());
      svg.style.transform = "none";
      svg.style.transformOrigin = "";
      apply();
    }
  }));
  svg.addEventListener("wheel", (event) => {
    event.preventDefault();
    const point = pointAt(event);
    zoomAt(camera.scale * (event.deltaY < 0 ? 1.12 : 1 / 1.12), point.x, point.y);
  }, { passive: false });

  let drag = null;
  svg.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    if (event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-link]")) return;
    drag = { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY, x: camera.x, y: camera.y };
    svg.classList.add("panning");
    svg.setPointerCapture(event.pointerId);
  });
  svg.addEventListener("pointermove", (event) => {
    if (!drag || drag.pointerId !== event.pointerId) return;
    camera.x = drag.x + (event.clientX - drag.clientX) * width / svg.clientWidth;
    camera.y = drag.y + (event.clientY - drag.clientY) * height / svg.clientHeight;
    apply();
  });
  const endDrag = (event) => {
    if (!drag || drag.pointerId !== event.pointerId) return;
    drag = null;
    svg.classList.remove("panning");
  };
  svg.addEventListener("pointerup", endDrag);
  svg.addEventListener("pointercancel", endDrag);
  apply();
  stage.querySelectorAll(".graph-exit-layer .graph-exiting").forEach((component) => {
    const moving = component.hasAttribute("data-arrival-dx");
    const exitX = component.dataset.exitDx ?? component.dataset.arrivalDx;
    const exitY = component.dataset.exitDy ?? component.dataset.arrivalDy;
    const animation = component.animate([
      { opacity: 1 },
      { opacity: 0.65, offset: 0.8 },
      { opacity: 0 },
    ], { duration: GRAPH_REMOVAL_DURATION, easing: "ease-in", fill: "forwards" });
    animateGraphMotion(component, animation,
      { x: Number(component.dataset.exitStartX || 0), y: Number(component.dataset.exitStartY || 0), scale: Number(component.dataset.exitStartScale || 1) },
      { x: moving ? Number(exitX) : 0, y: moving ? Number(exitY) : 0, scale: 0.1 }, 0.8);
    const removalTimer = window.setTimeout(() => component.remove(), GRAPH_REMOVAL_DURATION + 150);
    animation.onfinish = () => {
      window.clearTimeout(removalTimer);
      component.remove();
    };
  });
  if (state.graphAnimations) {
    stage.querySelectorAll(".graph-arrival-marker").forEach((marker) => {
      const elapsed = Number(marker.dataset.arrivalAge) || 0;
      const ring = marker.querySelector(".graph-arrival-ring");
      const ringAnimation = ring.animate([
        { opacity: 0 },
        { opacity: 1, offset: 0.1 },
        { opacity: 0 },
      ], { duration: GRAPH_ARRIVAL_DURATION, easing: "ease-out", fill: "both" });
      ringAnimation.currentTime = elapsed;
      animateGraphMotion(ring, ringAnimation, { x: 0, y: 0, scale: 0.72 }, { x: 0, y: 0, scale: 1.18 });
      const entrant = marker.parentElement;
      if (entrant) {
        const moving = entrant.hasAttribute("data-arrival-dx");
        const entryX = entrant.dataset.entryDx ?? entrant.dataset.arrivalDx;
        const entryY = entrant.dataset.entryDy ?? entrant.dataset.arrivalDy;
        const componentAnimation = entrant.animate([{ opacity: moving ? 0.65 : 0.45 }, { opacity: 1 }], { duration: 900, easing: "cubic-bezier(.2, .7, .25, 1)", fill: "both" });
        componentAnimation.currentTime = Math.min(elapsed, 900);
        animateGraphMotion(entrant, componentAnimation,
          { x: moving ? Number(entryX) : 0, y: moving ? Number(entryY) : 0, scale: 0.1 },
          { x: 0, y: 0, scale: 1 });
        const glyph = entrant.querySelector(".graph-node, .graph-adapter, .graph-visa, rect");
        if (glyph) {
          const highlighted = getComputedStyle(glyph);
          const start = { fill: highlighted.fill, stroke: highlighted.stroke, strokeWidth: highlighted.strokeWidth, filter: highlighted.filter };
          entrant.classList.remove("arriving");
          const normal = getComputedStyle(glyph);
          const end = { fill: normal.fill, stroke: normal.stroke, strokeWidth: normal.strokeWidth, filter: normal.filter };
          entrant.classList.add("arriving");
          const colorAnimation = glyph.animate([start, end], { duration: 900, easing: "ease-out", fill: "forwards" });
          colorAnimation.currentTime = Math.min(elapsed, 900);
        }
      }
    });
  }
}

function nodeState(actor, data) {
  if (!actor.node) return "ADAPTER";
  const active = (data.network || []).some((link) => link.ctype === "UP" && [link.node_a_addr, link.node_b_addr].includes(actor.zpr_addr));
  if (active) return "LINK UP";
  return actor.node_details?.in_sync ? "IN SYNC" : "NO UP LINK";
}

function renderActors(data) {
  byId("actor-count").textContent = `${formatNumber(data.actors.length)} Actors`;
  const columns = { cn: (actor) => `${actorDisplayName(actor)} ${actor.cn}`, role: (actor) => actor.node ? "node" : "adapter", address: (actor) => `${dnsAddressLabel(actor.zpr_addr, actor.cn)} ${actor.zpr_addr || ""}`, state: (actor) => nodeState(actor, data) };
  const actors = visibleRows("actors", data.actors, columns);
  byId("actor-rows").innerHTML = actors.length ? actors.map((actor) => {
    const role = actor.node ? "node" : "adapter";
    const stateText = nodeState(actor, data);
    const up = stateText === "LINK UP" || stateText === "IN SYNC";
    const last = actor.node && actor.node_details ? ` · seen ${since(actor.node_details.last_contact)}` : "";
    return `<tr class="selectable-row" data-inspect-actor="${escapeHTML(actor.cn)}" tabindex="0" role="button" aria-label="Inspect actor ${escapeHTML(actorDisplayName(actor))}"><td>${escapeHTML(actorDisplayName(actor))}</td><td><span class="role-chip ${role}">${role}</span></td><td class="mono" title="${escapeHTML(dnsAddressTitle(actor.zpr_addr))}">${escapeHTML(dnsAddressLabel(actor.zpr_addr, actor.cn))}</td><td><span class="mini-state ${up ? "up" : ""}">${stateText}${last}</span></td></tr>`;
  }).join("") : `<tr><td colspan="4" class="empty-row">No matching actors</td></tr>`;
}

function renderTrusted(data) {
  const sources = data.trusted_sources || [];
  const columns = { name: (source) => source.name, provider: (source) => source.provider, actor: (source) => source.actor_cn, health: (source) => source.health, last_lookup: (source) => source.last_lookup_ms || 0 };
  const shown = visibleRows("sources", sources, columns);
  byId("trusted-list").innerHTML = shown.length ? shown.map((source) => {
    const sourceName = `<strong>${escapeHTML(source.name)}</strong><small>${escapeHTML(providerDescription(source.provider))}</small>`;
    const actorLabel = source.actor_cn || "No actor reported";
      const statusText = lookupOutcome(source.health);
    const editorAction = source.editor_url ? `<a class="source-editor-link" href="${escapeHTML(source.editor_url)}" target="_blank" rel="noopener noreferrer" title="Open ${escapeHTML(source.name)} in LDAP editor">Open directory ↗</a>` : "";
    return `<tr class="selectable-row" data-inspect-source="${escapeHTML(source.name)}" tabindex="0" role="button" aria-label="Inspect trusted source ${escapeHTML(source.name)}"><td class="source-primary">${sourceName}</td><td>${escapeHTML(providerDescription(source.provider))}</td><td>${escapeHTML(actorLabel)}</td><td><span class="health-badge ${["working", "failed"].includes(source.health) ? source.health : ""}">${escapeHTML(statusText)}</span></td><td><span class="source-time">${source.last_lookup_ms ? escapeHTML(new Date(source.last_lookup_ms).toLocaleString()) : "No lookup recorded"}</span>${source.last_success_ms ? `<small class="source-secondary">Last success ${escapeHTML(new Date(source.last_success_ms).toLocaleString())}</small>` : ""}${editorAction ? `<small class="source-secondary">${editorAction}</small>` : ""}</td></tr>`;
  }).join("") : `<tr><td colspan="5" class="empty-row">${sources.length ? "No matching trusted sources" : "No trusted services reported by the Visa Service."}</td></tr>`;
}

function providerDescription(provider) {
  if (provider === "file") return "Local attribute file";
  if (provider === "rest/1") return "HTTPS attribute service";
  if (provider === "validation/2") return "Authentication service";
  return "Trusted attribute provider";
}

function lookupOutcome(health) {
  if (health === "working") return "Lookup succeeded";
  if (health === "failed") return "Lookup failed";
  if (health === "unverified") return "Not queried";
  return "Status unavailable";
}
async function refreshPolicyContext() {
  const policy = state.policy;
  if (location.hash !== "#policy" || !policy.loaded || policy.contextPending) return;
  policy.contextPending = true;
  try {
    const response = await fetch("/api/policy/context", { cache: "no-store" });
    if (!response.ok) return;
    const context = await response.json();
    if (context.organization_id && context.organization_id !== policy.organizationID) {
      policy.loaded = false;
      await loadPolicyWorkspace();
    }
  } catch {
    // Retain the current editor while the backend context is restarting.
  } finally {
    policy.contextPending = false;
  }
}

function organizationPolicyCategoryID(policy = state.policy) {
  const categories = policy.categories || [];
  const records = policy.records || [];
  const organizationID = (policy.organizationID || "").toLowerCase();
  const organizationCategories = categories.filter((category) => category.path.split("/")[0].toLowerCase() === organizationID);
  const explicitPolicies = organizationCategories.find((category) => category.path.split("/").at(-1).toLowerCase() === "policies");
  if (explicitPolicies) return explicitPolicies.id;
  for (const category of organizationCategories) {
    const path = category.path;
    const containsPolicy = records.some((record) => {
      if (record.kind !== "policy" || record.archived) return false;
      const recordPath = categories.find((item) => item.id === record.category_id)?.path || "";
      return recordPath === path || recordPath.startsWith(`${path}/`);
    });
    if (containsPolicy) return category.id;
  }
  return categories.find((category) => records.some((record) => record.kind === "policy" && !record.archived && record.category_id === category.id))?.id
    || categories.find((category) => !category.parent_id)?.id
    || categories[0]?.id
    || "";
}

function categoryContainsRecord(policy, category, record) {
  const recordCategory = policy.categories.find((item) => item.id === record.category_id);
  return Boolean(recordCategory && (recordCategory.path === category.path || recordCategory.path.startsWith(`${category.path}/`)));
}

function policyCategoriesInUnifiedHierarchy(policy = state.policy) {
  return policy.categories.filter((category) => {
    if (category.path !== "Assertions") return true;
    const records = policy.records.filter((record) => record.category_id === category.id);
    const hasChildren = policy.categories.some((item) => item.parent_id === category.id);
    return hasChildren || !records.length || records.some((record) => record.kind !== "assertions");
  });
}

async function loadPolicyWorkspace() {
  const policy = state.policy;
  if (policy.loaded) return;
  const selectedRecordID = policy.record?.id;
  const editorSource = byId("policy-source").value;
  const hasUnsavedChanges = hasUnsavedPolicyChanges(policy, editorSource);
  try {
    const response = await fetch("/api/policy", { cache: "no-store", headers: { Accept: "application/json" } });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `Policy server responded ${response.status}`);
    const organizationChanged = policy.organizationID && data.organization_id && policy.organizationID !== data.organization_id;
    if (organizationChanged && policy.testMode) stopPolicyTest();
    Object.assign(policy, {
      organizationID: data.organization_id || "",
      organizationName: data.organization_name || "",
      loaded: true,
      configured: data.configured,
      categories: data.categories || [],
      records: data.records || [],
      attributes: data.attributes || [],
      ldapAttributeCount: data.ldap_attribute_count || 0,
      attributeScanError: data.attribute_scan_error || "",
      compilerReady: data.compiler_ready,
      testerReady: data.tester_ready,
      stagingReady: data.staging_ready,
      stagedCandidate: data.staged_candidate || null,
      assistantReady: data.assistant_ready,
    });
    renderPolicyAttributes();
    if (organizationChanged) {
      policy.treeInitialized = false;
      policy.testResult = null;
      policy.testSource = "";
      policy.testDimensions = [];
      policy.record = hasUnsavedChanges ? { isDraft: true, name: "Retained draft from previous organization", kind: "policy", category_id: "", content: editorSource } : null;
      policy.savedSource = "";
      policy.validSource = "";
      policy.browsingRevision = 0;
    }
    if (!policy.categories.some((category) => category.id === policy.categoryID)) {
      policy.categoryID = organizationPolicyCategoryID(policy);
    }
    if (!policy.treeInitialized) {
      policy.collapsedCategories = new Set(policy.categories.filter((category) => category.id !== policy.categoryID && policy.records.some((record) => record.category_id === category.id)).map((category) => category.id));
      policy.treeInitialized = true;
    }
    if (policy.record && !policy.record.isDraft && !policy.records.some((record) => record.id === policy.record.id)) policy.record = null;
    renderPolicyCatalog();
    if (policy.record?.isDraft) {
      if (organizationChanged) policy.record.category_id = policy.categoryID;
      byId("policy-source").disabled = false;
      renderPolicyIdentity(policy.record, 0, "");
      renderPolicyAttributes();
      updatePolicyDirtyState();
    } else if (policy.record && hasUnsavedChanges) {
      byId("policy-check-result").textContent = "Unsaved edits retained. Evaluate before saving.";
    } else if (policy.record) await selectPolicyRecord(policy.record.id, true, true);
    else {
      const selectedCategory = policy.categories.find((category) => category.id === policy.categoryID);
      const firstPolicy = policy.records.find((record) => record.kind === "policy" && (!selectedCategory || categoryContainsRecord(policy, selectedCategory, record)));
      if (firstPolicy) await selectPolicyRecord(firstPolicy.id, true, true);
      else clearPolicySelection();
    }
    byId("policy-check").disabled = !policy.configured || !policy.compilerReady;
    byId("new-category").disabled = !policy.configured;
    byId("new-policy-record").disabled = !policy.configured || !policy.categoryID;
    byId("assistant-state").textContent = policy.assistantReady ? "Ready" : "Not configured";
    byId("assistant-state").classList.toggle("ready", policy.assistantReady);
    const modelSelect = byId("assistant-model");
    const selectedModel = modelSelect.value || data.assistant_model;
    modelSelect.replaceChildren();
    for (const model of data.assistant_models || []) {
      const option = document.createElement("option");
      option.value = model;
      option.textContent = model;
      modelSelect.append(option);
    }
    modelSelect.value = (data.assistant_models || []).includes(selectedModel) ? selectedModel : data.assistant_model || "";
    updateAssistantControls();
    byId("assistant-disclosure").textContent = policy.assistantReady
      ? "Submitting sends the current policy, configured attribute catalog, and chat history to Anthropic. Suggestions are not applied automatically."
      : "Claude is off. Set ANTHROPIC_API_KEY on the server to enable it.";
  } catch (error) {
    policy.loaded = false;
    byId("policy-check-result").textContent = error.message;
  }
}

function updateAssistantControls() {
  const policy = state.policy;
  const available = Boolean(policy.configured && policy.assistantReady);
  const enabled = available && policy.assistantEnabled;
  byId("assistant-enabled").disabled = !available;
  byId("assistant-enabled").checked = enabled;
  byId("assistant-model").disabled = !enabled;
  byId("assistant-max-tokens").disabled = !enabled;
  byId("assistant-question").disabled = !enabled || policy.record?.kind !== "policy";
  byId("assistant-send").disabled = !enabled || policy.record?.kind !== "policy" || policy.assistantPending;
  byId("assistant-usage").textContent = `Session: ${formatNumber(policy.assistantUsage.input)} input · ${formatNumber(policy.assistantUsage.output)} output tokens`;
}

function renderPolicyAttributes() {
  const rescan = byId("policy-attribute-rescan");
  const status = byId("policy-attribute-status");
  rescan.disabled = !state.policy.configured || Boolean(state.policy.attributeScanPending);
  if (state.policy.attributeScanPending) {
    status.textContent = "Scanning LDAP…";
    status.dataset.state = "pending";
  } else if (state.policy.attributeScanError) {
    status.textContent = state.policy.attributeScanError;
    status.dataset.state = "error";
  } else {
    status.textContent = `${state.policy.ldapAttributeCount || 0} LDAP attributes scanned`;
    status.dataset.state = "ready";
  }
}

async function rescanPolicyAttributes() {
  const policy = state.policy;
  policy.attributeScanPending = true;
  renderPolicyAttributes();
  try {
    const response = await fetch("/api/policy/attributes/rescan", { method: "POST", headers: { Accept: "application/json" } });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || "Unable to scan LDAP attributes.");
    policy.attributes = data.attributes || [];
    policy.ldapAttributeCount = data.ldap_attribute_count || 0;
    policy.attributeScanError = data.error || "";
  } catch (error) {
    policy.attributeScanError = error.message || "Unable to scan LDAP attributes.";
  } finally {
    policy.attributeScanPending = false;
    renderPolicyAttributes();
  }
}

function formatZPL(source) {
  const lines = source.trimStart().match(/[^\n]*\n|[^\n]+$/g) || [];
  let formatted = "";
  let previousLineIsBlank = true;
  let isFirstLine = true;
  let permissionBlockIsOpen = false;
  let previousStatementWasDefine = false;
  let serviceBlockIsOpen = false;
  let pendingLines = [];

  for (const line of lines) {
    const content = line.replace(/\r?\n$/, "");
    if (!content.trim() || /^\s*(?:#|\/\/)/.test(content)) {
      pendingLines.push(line);
      continue;
    }
    const startsWith = (keyword) => new RegExp(`^\\s*${keyword}(?:\\s|$)`, "i").test(content);
    const isDefine = startsWith("define");
    const isServiceDeclaration = startsWith("provide") || startsWith("service");
    const endsPermissionBlock = ["define", "provide", "service"].some(startsWith);
    const isPolicyStatement = ["allow", "deny", "never"].some(startsWith);
    const removeBlankLines = isDefine && previousStatementWasDefine || isPolicyStatement && serviceBlockIsOpen;
    for (const pendingLine of pendingLines) {
      if (!pendingLine.trim() && (removeBlankLines || previousLineIsBlank)) continue;
      formatted += pendingLine;
      previousLineIsBlank = !pendingLine.trim();
      isFirstLine = false;
    }
    pendingLines = [];

    if (!isFirstLine && !previousLineIsBlank && (isServiceDeclaration || (permissionBlockIsOpen && endsPermissionBlock))) {
      formatted += line.endsWith("\r\n") || formatted.endsWith("\r\n") ? "\r\n" : "\n";
    }

    if (endsPermissionBlock) {
      permissionBlockIsOpen = false;
      previousStatementWasDefine = isDefine;
      serviceBlockIsOpen = isServiceDeclaration;
    }
    formatted += isPolicyStatement ? `  ${line.trimStart()}` : line;
    if (isPolicyStatement) {
      permissionBlockIsOpen = true;
      previousStatementWasDefine = false;
      serviceBlockIsOpen = true;
    }
    previousLineIsBlank = content.trim() === "";
    isFirstLine = false;
  }

  for (const pendingLine of pendingLines) {
    if (!pendingLine.trim() && previousLineIsBlank) continue;
    formatted += pendingLine;
    previousLineIsBlank = !pendingLine.trim();
  }
  return formatted;
}

function formatPolicySource() {
  const textarea = byId("policy-source");
  if (textarea.disabled) return;
  const formatted = formatZPL(textarea.value);
  if (formatted === textarea.value) return;
  textarea.value = formatted;
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

function renderPolicyCatalog() {
  closePolicyPickerMenu();
  const policy = state.policy;
  const children = new Map();
  const records = new Map();
  for (const category of policyCategoriesInUnifiedHierarchy(policy)) {
    const key = category.parent_id || "";
    if (!children.has(key)) children.set(key, []);
    children.get(key).push(category);
  }
  for (const items of children.values()) items.sort((a, b) => a.name.localeCompare(b.name));
  for (const record of policy.records) {
    if (record.archived && !policy.showArchived) continue;
    const categoryID = record.kind === "assertions" ? organizationPolicyCategoryID(policy) || record.category_id : record.category_id;
    if (!records.has(categoryID)) records.set(categoryID, []);
    records.get(categoryID).push(record);
  }
  for (const items of records.values()) items.sort((a, b) => a.name.localeCompare(b.name));
  const tree = byId("policy-category-tree");
  tree.replaceChildren();
  if (!policy.categories.length) {
    const empty = document.createElement("p"); empty.className = "catalog-empty"; empty.textContent = "No categories yet."; tree.append(empty);
  } else {
    const renderBranch = (container, parentID = "", depth = 0) => {
      for (const category of children.get(parentID) || []) {
        const button = document.createElement("button");
        button.type = "button"; button.className = "category-tree-item";
        button.dataset.categoryId = category.id;
        button.setAttribute("role", "treeitem");
        button.setAttribute("aria-haspopup", "menu");
        button.setAttribute("aria-controls", "policy-picker-menu");
        button.setAttribute("aria-label", category.path);
        button.title = category.path;
        button.setAttribute("aria-selected", String(policy.categoryID === category.id));
        button.setAttribute("aria-expanded", String(!policy.collapsedCategories.has(category.id)));
        button.style.setProperty("--tree-depth", depth);
        const marker = document.createElement("span"); marker.className = "tree-marker"; marker.setAttribute("aria-hidden", "true");
        const name = document.createElement("span"); name.textContent = category.name;
        const count = (records.get(category.id) || []).length;
        const tally = document.createElement("small"); tally.textContent = count ? String(count) : "";
        button.append(marker, name, tally);
        container.append(button);
        if (policy.collapsedCategories.has(category.id)) continue;
        const group = document.createElement("div"); group.className = "policy-tree-group"; group.setAttribute("role", "group");
        for (const record of records.get(category.id) || []) {
          const item = document.createElement("button"); item.type = "button"; item.className = "policy-record-item";
          item.dataset.recordId = record.id; item.dataset.kind = record.kind; item.setAttribute("role", "treeitem");
          item.setAttribute("aria-haspopup", "menu");
          item.setAttribute("aria-controls", "policy-picker-menu");
          item.setAttribute("aria-selected", String(policy.record?.id === record.id));
          item.style.setProperty("--tree-depth", depth + 1);
          const label = document.createElement("strong"); label.textContent = record.name;
          const meta = document.createElement("small"); meta.textContent = record.kind === "assertions" ? `Assertions · r${record.current_revision}` : `v${record.current_revision}`;
          item.append(label, meta); group.append(item);
        }
        renderBranch(group, category.id, depth + 1);
        container.append(group);
      }
    };
    renderBranch(tree);
  }
  byId("new-policy-record").disabled = !policy.configured || !policy.categories.some((category) => category.id === policy.categoryID);
  byId("new-assertion-record").disabled = !policy.configured || !policy.categoryID;
  renderPolicyFileActions();
}

function renderPolicyFileActions() {
  const policy = state.policy;
  const record = policy.record;
  const saved = Boolean(record && !record.isDraft);
  const protectedRecord = record?.kind === "assertions" && record.name === "Organization assertions" && record.content_type === "application/vnd.zpr.assertions+json";
  const archived = Boolean(record?.archived);
  const menu = byId("policy-picker-menu");
  const context = menu.hidden ? "picker" : menu.dataset.context;
  const categoryContext = context === "category";
  const recordContext = context === "record";
  for (const id of ["new-category", "new-policy-record", "new-assertion-record"]) byId(id).hidden = recordContext;
  byId("new-category").textContent = categoryContext ? "New subcategory" : "New category";
  byId("policy-copy").hidden = categoryContext;
  byId("policy-duplicate").hidden = categoryContext;
  byId("policy-copy").disabled = !saved || archived || protectedRecord;
  byId("policy-paste").disabled = !policy.fileClipboard || !policy.categoryID;
  byId("policy-duplicate").disabled = !saved || archived || protectedRecord;
  byId("policy-delete").disabled = !saved || archived || protectedRecord;
  byId("policy-delete").hidden = archived || categoryContext;
  byId("policy-restore").hidden = !archived || categoryContext;
  byId("policy-restore").disabled = !archived;
  byId("policy-show-archived").setAttribute("aria-pressed", String(policy.showArchived));
  byId("policy-show-archived").setAttribute("aria-checked", String(policy.showArchived));
  byId("policy-show-archived").textContent = policy.showArchived ? "Hide archived" : "Show archived";
}

function closePolicyPickerMenu(restoreFocus = false) {
  const menu = byId("policy-picker-menu");
  if (menu.hidden) return;
  menu.hidden = true;
  if (restoreFocus) {
    const record = state.policy.pickerMenuRecordID;
    const category = state.policy.pickerMenuCategoryID;
    const origin = [...byId("policy-category-tree").querySelectorAll("[data-record-id], [data-category-id]")]
      .find((item) => record ? item.dataset.recordId === record : category && item.dataset.categoryId === category);
    (origin || byId("policy-category-tree")).focus();
  }
  renderPolicyFileActions();
}

async function openPolicyPickerMenu(target, x, y) {
  closePolicyPickerMenu();
  const policy = state.policy;
  const record = target.closest("[data-record-id]");
  const category = target.closest("[data-category-id]");
  if (record && record.dataset.recordId !== policy.record?.id) {
    await selectPolicyRecord(record.dataset.recordId);
    if (policy.record?.id !== record.dataset.recordId || currentPage() !== "policy") return;
  }
  if (category) {
    policy.categoryID = category.dataset.categoryId;
    renderPolicyCatalog();
  }
  if (currentPage() !== "policy") return;
  policy.pickerMenuRecordID = record?.dataset.recordId || "";
  policy.pickerMenuCategoryID = category?.dataset.categoryId || "";
  const categoryName = policyCategoriesInUnifiedHierarchy().find((item) => item.id === category?.dataset.categoryId)?.path;
  byId("policy-picker-menu-title").textContent = record ? policy.record.name : categoryName || "Policy picker";
  byId("new-category").disabled = !policy.configured;
  const menu = byId("policy-picker-menu");
  menu.dataset.context = record ? "record" : category ? "category" : "picker";
  menu.hidden = false;
  renderPolicyFileActions();
  menu.style.left = `${Math.max(8, Math.min(x, innerWidth - menu.offsetWidth - 8))}px`;
  menu.style.top = `${Math.max(8, Math.min(y, innerHeight - menu.offsetHeight - 8))}px`;
  menu.querySelector("button:not(:disabled):not([hidden])")?.focus();
}

function clearPolicySelection() {
  const policy = state.policy;
  setPolicyRecordSurface("");
  if (policy.testAbort) policy.testAbort.abort();
  policy.testAbort = null;
  policy.testMode = false;
  policy.testPending = false;
  policy.record = null; policy.source = ""; policy.savedSource = ""; policy.revision = 0; policy.revisions = []; policy.validSource = null;
  policy.errorOffsets = [];
  policy.browsingRevision = 0;
  byId("policy-source").value = ""; byId("policy-source").disabled = true;
  renderPolicyAttributes();
  updatePolicyHighlight(); hidePolicyCompletions();
  renderPolicyIdentity(null);
  byId("policy-record-title").hidden = false;
  byId("policy-draft-name").hidden = true;
  byId("policy-draft-name").value = "";
  byId("policy-check").disabled = true; byId("policy-save").disabled = true; byId("policy-save-as").disabled = true; byId("policy-refresh").disabled = true;
  byId("policy-format").disabled = true;
  byId("policy-test").disabled = true;
  byId("policy-test").textContent = "Test";
  byId("policy-test").setAttribute("aria-pressed", "false");
  byId("policy-workbench").dataset.testMode = "false";
  byId("policy-code-editor").dataset.testMode = "false";
  state.policy.testResult = null;
  state.policy.testSource = "";
  byId("policy-test-gutter").replaceChildren();
  byId("policy-test-gutter").hidden = true;
  byId("policy-test-status").hidden = true;
  if (byId("policy-test-dialog").open) byId("policy-test-dialog").close();
  byId("policy-check-result").textContent = "Policy source is not loaded.";
  byId("policy-history-count").textContent = "—";
  byId("policy-history").innerHTML = '<p class="catalog-empty">Select a policy to browse versions.</p>';
  byId("assistant-question").disabled = true; byId("assistant-send").disabled = true;
  renderPolicyCatalog();
}

async function selectPolicyRecord(id, fetchRecord = true, discardEdits = false) {
  const policy = state.policy;
  const summary = policy.records.find((record) => record.id === id);
  if (!summary) return clearPolicySelection();
  if (!discardEdits && hasUnsavedPolicyChanges(policy) && !window.confirm("Discard unsaved changes or leave this historical version?")) return;
  try {
    let record = summary;
    if (fetchRecord) {
      const response = await fetch(`/api/policy/records/${encodeURIComponent(id)}`, { cache: "no-store" });
      record = await response.json();
      if (!response.ok) throw new Error(record.error || `Record load failed (${response.status})`);
    }
    policy.record = record;
    policy.categoryID = record.kind === "assertions" ? organizationPolicyCategoryID(policy) || record.category_id : record.category_id;
    policy.source = record.content || ""; policy.savedSource = policy.source; policy.revision = record.current_revision; policy.validSource = null;
    setPolicyRecordSurface(record.kind, record);
    policy.errorOffsets = [];
    policy.browsingRevision = 0;
    byId("policy-draft-name").hidden = true;
    byId("policy-draft-name").value = "";
    byId("policy-source").value = policy.source; byId("policy-source").disabled = record.kind !== "policy";
    renderPolicyAttributes();
    updatePolicyHighlight(); hidePolicyCompletions();
    renderPolicyIdentity(record, record.current_revision, record.content_hash);
    byId("policy-check").disabled = record.kind !== "policy" || !policy.compilerReady;
    byId("policy-save").disabled = true;
    byId("policy-check-result").textContent = record.kind === "policy" ? "Evaluate the current policy before saving." : `Record type: ${record.kind}`;
    byId("policy-check-result").dataset.state = "";
    updateAssistantControls();
    policy.messages = []; policy.assistantError = ""; renderAssistantMessages();
    renderPolicyCatalog(); await loadPolicyHistory(record.id);
    updatePolicyDirtyState();
  } catch (error) {
    byId("policy-check-result").textContent = error.message;
  }
}

function setPolicyRecordSurface(kind, record = state.policy.record) {
  const assertionsSelected = kind === "assertions";
  const policy = state.policy;
  if (assertionsSelected && policy.testMode) stopPolicyTest();
  byId("policy-assertion-editor").hidden = !assertionsSelected;
  byId("policy-source-surface").hidden = assertionsSelected;
  byId("policy-actions").hidden = assertionsSelected;
  byId("policy-stage-status").hidden = assertionsSelected;
  byId("policy-assistant-pane").hidden = assertionsSelected;
  byId("policy-workbench").dataset.recordKind = kind || "";
  updatePolicyWorkbenchLayout();
  window.dispatchEvent(new CustomEvent("policy-record-kind-changed", { detail: { kind, record } }));
}

function renderPolicyIdentity(record = state.policy.record, version = state.policy.revision, hash = record?.content_hash) {
  const title = byId("policy-record-title");
  const draftName = byId("policy-draft-name");
  draftName.hidden = !record?.isDraft;
  if (!record) {
    title.textContent = "Select a policy";
    title.hidden = false;
    title.removeAttribute("title");
    byId("policy-revision-label").textContent = "";
    return;
  }
  const categoryID = record.kind === "assertions" ? organizationPolicyCategoryID(state.policy) : record.category_id;
  const category = state.policy.categories.find((item) => item.id === categoryID);
  const path = [category?.path, record.name].filter(Boolean).join("/");
  title.hidden = Boolean(record.isDraft);
  const draftNameValue = record.isDraft ? record.name : "";
  if (draftName.value !== draftNameValue) draftName.value = draftNameValue;
  title.textContent = record.isDraft ? "" : record.name;
  title.title = path;
  byId("policy-revision-label").textContent = record.isDraft ? "New · unsaved" : `Version ${version} [${hash ? hash.slice(0, 12) : "hash unavailable"}]`;
  byId("policy-revision-label").title = record.isDraft ? "In-memory policy draft" : `${path} · ${hash || "hash unavailable"}`;
}

window.addEventListener("policy-assertion-saved", () => {
  if (state.policy.record?.kind !== "assertions") return;
  state.policy.loaded = false;
  loadPolicyWorkspace();
});

window.addEventListener("policy-assertion-created", async (event) => {
  const record = event.detail?.record;
  if (!record || record.kind !== "assertions") return;
  const policy = state.policy;
  policy.record = record;
  policy.records = [...policy.records.filter((item) => item.id !== record.id), record];
  policy.categoryID = organizationPolicyCategoryID(policy) || record.category_id;
  policy.source = record.content || "";
  policy.savedSource = policy.source;
  policy.revision = record.current_revision;
  policy.validSource = null;
  policy.browsingRevision = 0;
  policy.revisions = [];
  setPolicyRecordSurface("assertions", record);
  renderPolicyIdentity(record, record.current_revision, record.content_hash);
  renderPolicyCatalog();
  try { await loadPolicyHistory(record.id); }
  catch (error) { byId("policy-file-status").textContent = error.message; }
  updatePolicyDirtyState();
});

function hasUnsavedPolicyChanges(policy = state.policy, source = byId("policy-source").value) {
  if (policy.record?.kind === "assertions") return Boolean(policy.record.isDraft || window.policyAssertionDirty?.());
  return Boolean(policy.record && (policy.record.isDraft || source !== policy.savedSource));
}

function definedPolicyClasses(source) {
  return [...source.matchAll(/^[ \t]*define[ \t]+([\p{L}\p{N}_-]+)/gimu)].map((match) => match[1]);
}

function pluralPolicyClass(name) {
  return /(?:s|x|z|ch|sh)$/i.test(name) ? `${name}es` : `${name}s`;
}

function policyClassReferences(source) {
  const names = definedPolicyClasses(source);
  const forms = names.flatMap((name) => [name, pluralPolicyClass(name)]);
  return [...new Set(forms)];
}

function highlightZPL(source, errorOffsets = []) {
  const tokens = /#.*$|\/\/.*$|'(?:\\.|[^'\\])*'?|"(?:\\.|[^"\\])*"?|`(?:\\.|[^`\\])*`?|[\p{L}\p{N}_-]+|[^\s]/gmu;
  let output = "";
  let previousEnd = 0;
  for (const match of source.matchAll(tokens)) {
    const token = match[0];
    const start = match.index;
    output += escapeHTML(source.slice(previousEnd, start));
    let className = "";
    if (token.startsWith("#") || token.startsWith("//")) className = "zpl-comment";
    else if (/^[`'\"]/.test(token)) className = "zpl-string";
    else if (zplKeywords.has(token.toLowerCase())) className = "zpl-keyword";
    else if (/^[\p{L}\p{N}_-]+$/u.test(token)) {
      const before = source.slice(0, start).match(/([^\s]+\s*)$/)?.[0] || "";
      if (/^\s*define\s*$/i.test(before)) className = "zpl-class-definition";
      else if (!["a", "an"].includes(token.toLowerCase()) && /^\s*define\s+[^\s]+\s+(?:aka\s+[^\s]+\s+)?as\s*$/i.test(source.slice(0, start))) className = "zpl-class";
      else if (/^\s*[^\s]+\s*:\s*$/.test(before)) className = "zpl-value";
      else if (source.slice(start + token.length).startsWith(":")) className = "zpl-attribute";
      else if (policyClassReferences(source).some((name) => name.toLowerCase() === token.toLowerCase())) className = "zpl-class";
      else if (["user", "device", "service", "link"].includes(token.toLowerCase())) className = "zpl-class";
    } else if (token === ":" || token === "." || token === "," || token === "{" || token === "}") className = "zpl-punctuation";
    const isError = errorOffsets.some((offset) => offset >= start && offset < start + token.length);
    if (isError) className = className ? `${className} zpl-error` : "zpl-error";
    output += className ? `<span class="${className}">${escapeHTML(token)}</span>` : escapeHTML(token);
    previousEnd = start + token.length;
  }
  output += escapeHTML(source.slice(previousEnd));
  return output + (source.endsWith("\n") ? " " : "\n");
}

function extractZPLErrorOffsets(diagnostics, source) {
  const offsets = [];
  const lineStarts = [0];
  for (let index = 0; index < source.length; index += 1) {
    if (source[index] === "\n") lineStarts.push(index + 1);
  }
  for (const diagnostic of String(diagnostics || "").split(/\r?\n/)) {
    if (!/\berror\b/i.test(diagnostic)) continue;
    for (const match of diagnostic.matchAll(/\bline\s+(\d+)\s*,\s*column\s+(\d+)\b/gi)) {
      const line = Number(match[1]);
      const column = Number(match[2]);
      if (lineStarts[line - 1] === undefined || source.length === 0) continue;
      let offset = Math.min(source.length - 1, lineStarts[line - 1] + column - 1);
      if (offset < 0) continue;
      while (offset < source.length && source[offset] !== "\n" && /\s/.test(source[offset])) offset += 1;
      if (offset >= source.length || source[offset] === "\n") {
        offset = Math.min(source.length - 1, lineStarts[line - 1] + column - 2);
        while (offset >= lineStarts[line - 1] && /\s/.test(source[offset])) offset -= 1;
      }
      if (offset >= 0) offsets.push(offset);
    }
  }
  return [...new Set(offsets)];
}

function updatePolicyHighlight() {
  const textarea = byId("policy-source");
  const highlight = byId("policy-highlight");
  highlight.innerHTML = highlightZPL(textarea.value, state.policy.errorOffsets);
  highlight.scrollTop = textarea.scrollTop;
  highlight.scrollLeft = textarea.scrollLeft;
}

function completionCandidates(source, cursor) {
  const prefix = source.slice(0, cursor);
  let blocked = false;
  const context = prefix.replace(/#.*$|\/\/.*$|'(?:\\.|[^'\\])*'?|"(?:\\.|[^"\\])*"?|`(?:\\.|[^`\\])*`?/gmu, (token, offset) => {
    if (offset + token.length === cursor) blocked = true;
    return token.replace(/[^\n]/g, " ");
  });
  const wordMatch = context.match(/[\p{L}\p{N}_-]+(?:\.[\p{L}\p{N}_-]*)*$/u);
  const word = wordMatch?.[0] || "";
  const empty = { word, start: cursor - word.length, items: [] };
  if (blocked) return empty;
  const boundaries = [...context.matchAll(/\.[ \t]*(?:\r?\n|$)/g)];
  const boundary = boundaries.at(-1);
  const qualifiedAttribute = /^(?:user|device|service|link)\.$/i.test(word) && /\bdefine\b[\s\S]*\bwith\b/i.test(context.slice(boundaries.at(-2)?.index + 1 || 0));
  if (boundary && !context.slice(boundary.index).includes("\n") && !qualifiedAttribute) return empty;
  const statementStart = boundary && !qualifiedAttribute ? boundary.index + boundary[0].length : boundaries.at(-2)?.index + 1 || 0;
  const line = context.slice(statementStart).replace(/\s+/g, " ");
  const trimmed = line.trimStart();
  let candidates;
  let attributeMode = false;
  let filterPrefix = word;
  if (/^define\s+/i.test(trimmed) && /\bwith\b/i.test(trimmed)) {
    attributeMode = true;
    const attributes = trimmed.slice(trimmed.search(/\bwith\b/i) + 4);
    if (!/(?:^|\band\s+(?:with\s+)?|,)\s*(?:(?:optional|multiple|tag|tags)\s+)*[\p{L}\p{N}_.-]*$/iu.test(attributes)) return empty;
    const parent = trimmed.match(/^define\s+\S+\s+(?:aka\s+\S+\s+)?as\s+(?:a\s+|an\s+)?(user|device|service)\b/i)?.[1]?.toLowerCase();
    candidates = (state.policy.attributes || []).map((item) => item.attribute).filter((attribute) => !parent || !/^(user|device|service|link)\./i.test(attribute) || attribute.toLowerCase().startsWith(`${parent}.`)).map((attribute) => `${attribute}:`);
  } else if (/^[\p{L}\p{N}_-]*$/u.test(trimmed)) {
    candidates = ["allow", "never allow", "define"];
    filterPrefix = trimmed;
  } else if (/^never\s+[\p{L}\p{N}_-]*$/iu.test(trimmed)) {
    candidates = ["allow"];
  } else if (/^define\s+\S+\s+(?:aka\s+\S+\s+)?as\s+(?:a\s+|an\s+)?[\p{L}\p{N}_-]*$/iu.test(trimmed)) {
    candidates = ["user", "device", "service", ...definedPolicyClasses(source)];
  } else if (/^define\s+\S+\s+(?:aka\s+\S+\s+)?$/i.test(trimmed)) {
    candidates = ["as"];
  } else if (/^define\s+\S+\s+(?:aka\s+\S+\s+)?as\s+(?:a\s+|an\s+)?\S+\s+[\p{L}\p{N}_-]*$/iu.test(trimmed)) {
    candidates = ["with"];
  } else if (/\bsignal\b[\s\S]*\bto\s*[^\s]*$/i.test(line)) {
    candidates = policyClassReferences(source);
  } else if (/^\s*(?:allow|never\s+allow)\s+[^\n]*$/i.test(line) && !/\bto\b/i.test(line)) {
    candidates = ["users", "devices", "services", ...definedPolicyClasses(source).flatMap((name) => [pluralPolicyClass(name), name]), "on", "over", "and", "signal"];
  } else if (/^(?:allow|never\s+allow|define)\s+/i.test(trimmed)) {
    candidates = ["with", "and", "on", "over", "signal", "tag", "tags", "optional", "multiple"];
  } else return empty;
  const unique = [...new Set(candidates)];
  const filtered = filterPrefix ? unique.filter((item) => (item.toLowerCase().startsWith(filterPrefix.toLowerCase()) || attributeMode && !filterPrefix.includes(".") && item.split(".").at(-1).toLowerCase().startsWith(filterPrefix.toLowerCase())) && item.toLowerCase() !== filterPrefix.toLowerCase()) : unique;
  let end = cursor + (source.slice(cursor).match(/^[\p{L}\p{N}_-]*(?:\.[\p{L}\p{N}_-]+)*/u)?.[0].length || 0);
  if (attributeMode && source[end] === ":") end++;
  return { word, start: cursor - word.length, end, items: filtered.slice(0, 8) };
}

function hidePolicyCompletions() {
  const menu = byId("policy-completions");
  menu.hidden = true;
  menu.replaceChildren();
  menu.removeAttribute("data-start");
  menu.removeAttribute("data-end");
  byId("policy-source").setAttribute("aria-expanded", "false");
  byId("policy-source").removeAttribute("aria-activedescendant");
}

function positionPolicyCompletions() {
  const menu = byId("policy-completions");
  if (menu.hidden) return;
  const textarea = byId("policy-source");
  const editor = textarea.parentElement;
  const mirror = document.createElement("div");
  mirror.className = "policy-caret-mirror";
  mirror.setAttribute("aria-hidden", "true");
  const marker = document.createElement("span");
  marker.textContent = ".";
  mirror.append(document.createTextNode(textarea.value.slice(0, textarea.selectionStart)), marker, document.createTextNode(textarea.value.slice(textarea.selectionStart)));
  editor.append(mirror);
  mirror.scrollTop = textarea.scrollTop;
  mirror.scrollLeft = textarea.scrollLeft;
  const caret = marker.getBoundingClientRect();
  const bounds = editor.getBoundingClientRect();
  mirror.remove();
  if (caret.bottom < bounds.top + 8 || caret.top > bounds.bottom - 8 || caret.left < bounds.left || caret.left > bounds.right) return hidePolicyCompletions();
  const left = Math.max(8, Math.min(caret.left - bounds.left, editor.clientWidth - menu.offsetWidth - 8));
  const below = caret.bottom - bounds.top + 4;
  const top = below + menu.offsetHeight <= editor.clientHeight - 8 ? below : Math.max(8, caret.top - bounds.top - menu.offsetHeight - 4);
  menu.style.left = `${left}px`;
  menu.style.top = `${top}px`;
}

function showPolicyCompletions(force = false) {
  const textarea = byId("policy-source");
  if (textarea.disabled || textarea.selectionStart !== textarea.selectionEnd) return hidePolicyCompletions();
  const lineStart = textarea.value.lastIndexOf("\n", textarea.selectionStart - 1) + 1;
  if (!force && !textarea.value.slice(lineStart, textarea.selectionStart).trim()) return hidePolicyCompletions();
  const suggestions = completionCandidates(textarea.value, textarea.selectionStart);
  if (!suggestions.items.length && !force) return hidePolicyCompletions();
  if (!suggestions.items.length) return hidePolicyCompletions();
  const menu = byId("policy-completions");
  menu.replaceChildren();
  menu.dataset.start = String(suggestions.start);
  menu.dataset.end = String(suggestions.end ?? textarea.selectionStart);
  suggestions.items.forEach((item, index) => {
    const option = document.createElement("button");
    option.type = "button";
    option.className = "completion-option";
    option.id = `policy-completion-${index}`;
    option.dataset.insertion = item;
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", String(index === 0));
    const label = document.createElement("span");
    label.textContent = item;
    option.append(label);
    const attribute = item.endsWith(":") ? state.policy.attributes.find((entry) => `${entry.attribute}:` === item) : null;
    if (attribute) {
      option.classList.add("completion-attribute");
      const source = document.createElement("small");
      source.textContent = attribute.source || "attribute";
      option.append(source);
    }
    option.addEventListener("mousedown", (event) => event.preventDefault());
    option.addEventListener("click", () => acceptPolicyCompletion(index));
    menu.append(option);
  });
  menu.hidden = false;
  textarea.setAttribute("aria-expanded", "true");
  textarea.setAttribute("aria-activedescendant", "policy-completion-0");
  positionPolicyCompletions();
}

function acceptPolicyCompletion(index = 0) {
  const textarea = byId("policy-source");
  const menu = byId("policy-completions");
  const options = [...menu.querySelectorAll(".completion-option")];
  const option = options[index];
  if (!option) return;
  const start = Number(menu.dataset.start);
  const end = Number(menu.dataset.end);
  const insertion = option.dataset.insertion;
  textarea.setRangeText(insertion, start, end, "end");
  textarea.focus();
  state.policy.validSource = null;
  updatePolicyHighlight();
  updatePolicyDirtyState();
  hidePolicyCompletions();
}

function movePolicyCompletion(delta) {
  const options = [...byId("policy-completions").querySelectorAll(".completion-option")];
  if (!options.length) return false;
  const current = options.findIndex((option) => option.getAttribute("aria-selected") === "true");
  const next = (current + delta + options.length) % options.length;
  options.forEach((option, index) => option.setAttribute("aria-selected", String(index === next)));
  byId("policy-source").setAttribute("aria-activedescendant", options[next].id);
  options[next].scrollIntoView({ block: "nearest" });
  return true;
}

async function loadPolicyHistory(recordID) {
  const response = await fetch(`/api/policy/records/${encodeURIComponent(recordID)}/revisions`, { cache: "no-store" });
  const revisions = await response.json();
  if (!response.ok) throw new Error(revisions.error || `Version history failed (${response.status})`);
  state.policy.revisions = revisions;
  byId("policy-history-count").textContent = `${revisions.length} versions`;
  const history = byId("policy-history"); history.replaceChildren();
  for (const revision of revisions) {
    const button = document.createElement("button"); button.type = "button"; button.className = "history-item";
    button.dataset.revision = String(revision.number); button.setAttribute("aria-current", String(revision.number === state.policy.revision));
    const title = document.createElement("strong"); title.textContent = `Version ${revision.number}`;
    const summary = document.createElement("span"); summary.textContent = revision.summary || "No change summary";
    const meta = document.createElement("small"); meta.textContent = `${revision.author} · ${new Date(revision.created_at).toLocaleString()}`;
    button.append(title, summary, meta); history.append(button);
  }
}

async function refreshPolicyCatalog() {
  state.policy.loaded = false;
  await loadPolicyWorkspace();
}

async function browsePolicyRevision(number) {
  const policy = state.policy;
  if (!policy.record) return;
  try {
    const response = await fetch(`/api/policy/records/${encodeURIComponent(policy.record.id)}/revisions/${number}`, { cache: "no-store" });
    const revision = await response.json();
    if (!response.ok) throw new Error(revision.error || `Version load failed (${response.status})`);
    if (byId("policy-source").value !== policy.savedSource && !window.confirm("Discard unsaved edits and browse this version?")) return;
    policy.validSource = null;
    policy.errorOffsets = [];
    policy.browsingRevision = revision.number;
    byId("policy-source").value = revision.content;
    updatePolicyHighlight();
    renderPolicyIdentity(policy.record, revision.number, revision.content_hash);
    byId("policy-source").disabled = true;
    byId("policy-check").disabled = true;
    byId("policy-save").disabled = true;
    byId("policy-save-as").disabled = true;
    byId("policy-refresh").disabled = false;
    byId("policy-check-result").textContent = revision.summary || "Historical version";
    byId("policy-check-result").dataset.state = "";
    for (const item of byId("policy-history").querySelectorAll("[data-revision]")) item.setAttribute("aria-current", String(Number(item.dataset.revision) === revision.number));
    updatePolicyDirtyState();
  } catch (error) {
    byId("policy-check-result").textContent = error.message;
  }
}

function openCategoryDialog() {
  const options = byId("category-parent"); options.replaceChildren();
  const root = document.createElement("option"); root.value = ""; root.textContent = "Top level"; options.append(root);
  for (const category of policyCategoriesInUnifiedHierarchy()) {
    const option = document.createElement("option"); option.value = category.id; option.textContent = category.path; option.selected = category.id === state.policy.categoryID; options.append(option);
  }
  byId("category-name").value = ""; byId("category-error").textContent = "";
  byId("category-dialog").showModal(); byId("category-name").focus();
}

function openRecordDialog(saveAs = true) {
  const options = byId("record-category"); options.replaceChildren();
  for (const category of policyCategoriesInUnifiedHierarchy()) {
    const option = document.createElement("option"); option.value = category.id; option.textContent = category.path; option.selected = category.id === state.policy.categoryID; options.append(option);
  }
  state.policy.saveAs = saveAs;
  byId("record-dialog-title").textContent = saveAs ? "Save policy as" : "New policy";
  byId("record-submit").textContent = saveAs ? "Save As..." : "Create record";
  byId("record-name").value = saveAs && state.policy.record ? `${state.policy.record.name} copy` : "";
  byId("record-error").textContent = "";
  byId("record-dialog").showModal(); byId("record-name").focus();
}

function beginNewPolicyDraft() {
  const policy = state.policy;
  if (!policy.categoryID || !policy.configured) return;
  if (hasUnsavedPolicyChanges(policy) && !window.confirm("Discard the current edits and start a new policy draft?")) return;
  const draft = {
    id: "",
    category_id: policy.categoryID,
    name: "",
    kind: "policy",
    content_type: "text/vnd.zpr.zpl",
    metadata: { language: "zpl" },
    current_revision: 0,
    content_hash: "",
    isDraft: true,
  };
  policy.record = draft;
  setPolicyRecordSurface("policy", draft);
  policy.source = "";
  policy.savedSource = "";
  policy.revision = 0;
  policy.validSource = null;
  policy.errorOffsets = [];
  policy.browsingRevision = 0;
  policy.revisions = [];
  byId("policy-source").value = "";
  byId("policy-source").disabled = false;
  updatePolicyHighlight();
  hidePolicyCompletions();
  renderPolicyIdentity(draft, 0, "");
  byId("policy-check-result").textContent = "Name this policy, add ZPL, then Evaluate before saving.";
  byId("policy-check-result").dataset.state = "";
  byId("policy-history-count").textContent = "0 versions";
  byId("policy-history").innerHTML = '<p class="catalog-empty">The first version is created when this policy is saved.</p>';
  byId("policy-check").disabled = !policy.compilerReady;
  byId("policy-save").disabled = true;
  byId("policy-save-as").disabled = true;
  byId("policy-refresh").disabled = false;
  updateAssistantControls();
  renderPolicyCatalog();
  updatePolicyDirtyState();
  byId("policy-source").focus();
}

function beginNewAssertionDraft() {
  const policy = state.policy;
  const categoryID = organizationPolicyCategoryID(policy) || policy.categoryID;
  if (!categoryID || !policy.configured) return;
  if (hasUnsavedPolicyChanges(policy) && !window.confirm("Discard the current edits and start a new assertion set?")) return;
  const draft = {
    id: "", category_id: categoryID, name: "", kind: "assertions",
    content_type: "text/vnd.zpr.assertions", metadata: { language: "assertions" },
    current_revision: 0, content: "", content_hash: "", isDraft: true,
  };
  policy.record = draft;
  policy.categoryID = categoryID;
  policy.source = "";
  policy.savedSource = "";
  policy.revision = 0;
  policy.validSource = null;
  policy.browsingRevision = 0;
  policy.revisions = [];
  setPolicyRecordSurface("assertions", draft);
  renderPolicyIdentity(draft, 0, "");
  byId("policy-history-count").textContent = "0 versions";
  byId("policy-history").innerHTML = '<p class="catalog-empty">The first version is created when this assertion set is saved.</p>';
  renderPolicyCatalog();
  updatePolicyDirtyState();
  byId("assertion-source").focus();
}

function setPolicyFileStatus(text, isError = false) {
  const status = byId("policy-file-status");
  status.textContent = text;
  status.dataset.state = isError ? "error" : "";
}

function nextRecordCopyName(record, categoryID) {
  const existing = new Set(state.policy.records.filter((item) => item.category_id === categoryID).map((item) => item.name.toLowerCase()));
  const base = `${record.name.replace(/ copy(?: \d+)?$/i, "")} copy`;
  let candidate = base;
  let suffix = 2;
  while (existing.has(candidate.toLowerCase())) candidate = `${base} ${suffix++}`;
  return candidate.slice(0, 100);
}

async function pastePolicyRecord(sourceRecord, duplicate = false) {
  const policy = state.policy;
  if (!sourceRecord || !policy.categoryID) return;
  const name = nextRecordCopyName(sourceRecord, policy.categoryID);
  setPolicyFileStatus(`Creating ${name}…`);
  try {
    const path = duplicate
      ? `/api/policy/records/${encodeURIComponent(sourceRecord.id)}/duplicate`
      : "/api/policy/records";
    const body = duplicate
      ? { category_id: policy.categoryID, name }
      : {
        category_id: policy.categoryID, name, kind: sourceRecord.kind,
        content_type: sourceRecord.content_type, metadata: sourceRecord.metadata || {},
        content: sourceRecord.content || "", summary: `Copied from ${sourceRecord.name}`,
      };
    const response = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || result.diagnostics || `Paste failed (${response.status})`);
    policy.record = null;
    policy.categoryID = result.category_id;
    await refreshPolicyCatalog();
    await selectPolicyRecord(result.id, true, true);
    setPolicyFileStatus(`Created ${result.name}`);
  } catch (error) {
    setPolicyFileStatus(error.message, true);
  }
}

async function managePolicyFile(action) {
  const policy = state.policy;
  const record = policy.record;
  if (action === "show-archived") {
    policy.showArchived = !policy.showArchived;
    renderPolicyCatalog();
    return;
  }
  if (action === "copy") {
    if (!record || record.isDraft || record.archived || (record.kind === "assertions" && record.name === "Organization assertions" && record.content_type === "application/vnd.zpr.assertions+json")) return;
    policy.fileClipboard = {
      name: record.name, kind: record.kind, content_type: record.content_type,
      metadata: record.metadata, content: record.content,
    };
    setPolicyFileStatus(`Copied ${record.name}`);
    renderPolicyFileActions();
    return;
  }
  if (action === "paste") {
    await pastePolicyRecord(policy.fileClipboard);
    return;
  }
  if (action === "duplicate") {
    if (!record || record.isDraft || record.archived) return;
    await pastePolicyRecord(record, true);
    return;
  }
  if (!record || record.isDraft || !record.id) return;
  if (action === "delete") {
    if (!window.confirm(`Delete ${record.name}? Its revision history will be retained in Archived records.`)) return;
  }
  if (action !== "delete" && action !== "restore") return;
  try {
    const response = await fetch(`/api/policy/records/${encodeURIComponent(record.id)}${action === "restore" ? "/restore" : ""}`, {
      method: action === "delete" ? "DELETE" : "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expected_revision: record.current_revision }),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `${action} failed (${response.status})`);
    const name = record.name;
    policy.record = null;
    if (action === "restore") policy.showArchived = true;
    await refreshPolicyCatalog();
    setPolicyFileStatus(action === "delete" ? `Archived ${name}` : `Restored ${name}`);
  } catch (error) {
    setPolicyFileStatus(error.message, true);
  }
}

async function createPolicyCategory(event) {
  event.preventDefault();
  const name = byId("category-name").value.trim();
  const parentID = byId("category-parent").value || null;
  try {
    const response = await fetch("/api/policy/categories", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name, parent_id: parentID }) });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `Category creation failed (${response.status})`);
    if (!state.policy.categoryID) state.policy.categoryID = result.id;
    byId("category-dialog").close();
    await refreshPolicyCatalog();
  } catch (error) { byId("category-error").textContent = error.message; }
}

async function createPolicyRecord(event) {
  event.preventDefault();
  const name = byId("record-name").value.trim();
  const categoryID = byId("record-category").value;
  try {
    const saveAs = state.policy.saveAs;
    const content = saveAs ? byId("policy-source").value : "";
    if (saveAs && state.policy.validSource !== content) throw new Error("Evaluate the current policy before saving a copy.");
    const response = await fetch("/api/policy/records", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        category_id: categoryID, name, kind: "policy", content_type: "text/vnd.zpr.zpl",
        metadata: { language: "zpl" }, content,
        summary: saveAs ? `Copied from ${state.policy.record?.name || "policy"}` : "Created policy record",
      }),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || result.diagnostics || `Record creation failed (${response.status})`);
    state.policy.categoryID = categoryID;
    byId("record-dialog").close();
    await refreshPolicyCatalog();
    await selectPolicyRecord(result.id, true, saveAs);
  } catch (error) { byId("record-error").textContent = error.message; }
}

function updatePolicyDirtyState() {
  const policy = state.policy;
  const source = byId("policy-source").value;
  const contentDirty = source !== policy.savedSource;
  const dirty = !policy.browsingRevision && contentDirty;
  const checked = policy.validSource === source;
  const canEdit = policy.configured && policy.record?.kind === "policy" && !policy.record?.archived && !policy.browsingRevision && !policy.testMode;
  const canViewSource = policy.configured && policy.record?.kind === "policy" && !policy.browsingRevision;
  const canTest = policy.configured && policy.record?.kind === "policy" && !policy.browsingRevision && Boolean(source.trim());
  const isDraft = Boolean(policy.record?.isDraft);
  const canSave = canEdit && checked && (isDraft ? Boolean(policy.record.name.trim()) && source.trim() !== "" : contentDirty);
  const canStage = Boolean(!policy.testMode && policy.stagingReady && policy.record?.kind === "policy" && !policy.record?.archived && !isDraft && !policy.browsingRevision && !contentDirty && !policy.stagePending);
  byId("policy-save").disabled = !canSave;
  byId("policy-save-as").disabled = !canEdit || isDraft || !checked;
  byId("policy-stage").disabled = !canStage;
  byId("policy-check").disabled = !canEdit || !policy.compilerReady;
  byId("policy-format").disabled = !canEdit;
  byId("policy-source").disabled = !canViewSource;
  byId("policy-source").readOnly = policy.testMode;
  byId("policy-source").setAttribute("aria-readonly", String(policy.testMode));
  byId("policy-test").disabled = policy.testMode ? false : !canTest || !policy.compilerReady || !policy.testerReady || policy.testPending;
  byId("policy-test").textContent = policy.testMode ? "Exit test" : "Test";
  byId("policy-test").setAttribute("aria-pressed", String(policy.testMode));
  byId("policy-workbench").dataset.testMode = String(policy.testMode);
  byId("policy-code-editor").dataset.testMode = String(policy.testMode);
  updatePolicyWorkbenchLayout();
  byId("policy-test-gutter").hidden = !policy.testMode || !policy.testResult;
  byId("policy-test-status").hidden = !policy.testMode;
  byId("policy-attribute-rescan").disabled = !canEdit;
  byId("policy-refresh").disabled = policy.testMode || !policy.record || (!dirty && !policy.browsingRevision && !isDraft);
  byId("policy-check").classList.toggle("button-next-evaluate", canEdit && !checked);
  byId("policy-save").classList.toggle("button-save-next", !canSave);
  byId("policy-save").classList.toggle("button-next-save", canSave);
  byId("policy-save-as").classList.toggle("button-save-as-ready", canEdit && !isDraft && checked);
  renderPolicyStageStatus();
  renderPolicyIdentity(policy.record, policy.browsingRevision || policy.revision);
  const result = byId("policy-check-result");
  if (isDraft && !source.trim() && result.dataset.state !== "invalid") {
    result.textContent = "Name this policy, add ZPL, then Evaluate before saving.";
  } else if (isDraft && !policy.record.name.trim()) {
    result.textContent = checked ? "Add a policy name before saving its first version." : "Name this policy and Evaluate before saving.";
  } else if ((dirty || isDraft) && !checked && result.dataset.state !== "invalid") {
    result.dataset.state = "";
    result.textContent = "Evaluate the current policy before saving.";
  }
}

async function runPolicyTest() {
  const policy = state.policy;
  if (policy.testMode) {
    stopPolicyTest();
    return;
  }
  const source = byId("policy-source").value;
  if (policy.testPending || !source.trim()) return;
  const controller = new AbortController();
  policy.testMode = true;
  policy.testPending = true;
  policy.testAbort = controller;
  policy.testResult = null;
  policy.testSource = "";
  byId("policy-test-gutter").replaceChildren();
  byId("policy-test-gutter").hidden = true;
  byId("policy-test-status").hidden = false;
  byId("policy-test-status").textContent = "Preparing test cases and evaluating candidate…";
  byId("policy-test-status").dataset.state = "pending";
  closePolicyTestSubjects();
  updatePolicyDirtyState();
  try {
    const fixtureResponse = await fetch("/api/policy/test/fixtures", { cache: "no-store", signal: controller.signal });
    const fixtures = await fixtureResponse.json();
    if (!fixtureResponse.ok) throw new Error(fixtures.error || `Test fixture request failed (${fixtureResponse.status})`);
    policy.testDimensions = Array.from(new Set((fixtures.actors || []).flatMap((actor) => Object.keys(actor.dimensions || {})))).sort();
    const response = await fetch("/api/policy/test", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ source, actors: fixtures.actors, services: fixtures.services }),
      signal: controller.signal,
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `Policy test failed (${response.status})`);
    if (!policy.testMode) return;
    policy.testResult = result;
    policy.testSource = source;
    renderPolicyTestGutter(result);
    const warnings = fixtures.warnings || [];
    byId("policy-test-status").textContent = `${result.actor_count} actor samples · ${result.services.length} services tested${warnings.length ? ` · ${warnings.join(" ")}` : ""}`;
    byId("policy-test-status").dataset.state = "complete";
  } catch (error) {
    if (error.name !== "AbortError" && policy.testMode) {
      const errorLines = policyTestErrorLines(error.message, source.split("\n").length);
      if (errorLines.length) {
        policy.testResult = { error: error.message };
        policy.testSource = source;
        renderPolicyTestErrorGutter(error.message, errorLines);
        byId("policy-test-status").textContent = `Policy test failed on ${errorLines.length} source ${errorLines.length === 1 ? "line" : "lines"}. Click ERR for details.`;
      } else {
        byId("policy-test-status").textContent = error.message;
      }
      byId("policy-test-status").dataset.state = "error";
    }
  } finally {
    if (policy.testAbort === controller) {
      policy.testAbort = null;
      policy.testPending = false;
      updatePolicyDirtyState();
    }
  }
}

function renderPolicyTestGutter(result) {
  const gutter = byId("policy-test-gutter");
  const lineCount = byId("policy-source").value.split("\n").length;
  const rulesByLine = new Map();
  for (const service of result.services || []) {
    for (const rule of service.rules || []) {
      if (rule.line < 1 || rule.line > lineCount) continue;
      if (!rulesByLine.has(rule.line)) rulesByLine.set(rule.line, []);
      rulesByLine.get(rule.line).push({ service, rule });
    }
  }

  const fragment = document.createDocumentFragment();
  for (let line = 1; line <= lineCount; line++) {
    const row = document.createElement("div");
    row.className = "policy-test-line";
    row.dataset.line = String(line);
    for (const { service, rule } of rulesByLine.get(line) || []) {
      const summary = rule.matched || { count: 0, by_dimension: {}, subjects: [] };
      const countLabel = policyTestCountLabel(summary);
      const effect = rule.effect === "allow" ? "Allow" : "Deny";
      const button = document.createElement("button");
      button.type = "button";
      button.className = "policy-test-line-result";
      button.dataset.effect = rule.effect || "deny";
      button.disabled = !service.supported;
      if (!service.supported) button.textContent = "N/A";
      else renderPolicyTestCount(button, summary, countLabel);
      button.title = service.supported
        ? `Line ${line} · ${service.name || service.id} · ${effect} · ${countLabel}`
        : `Line ${line} · ${service.name || service.id} was not tested: ${service.reason || "No test endpoint is configured."}`;
      button.setAttribute("aria-label", button.title + "; show matching identities");
      const fullCountLabel = countLabel;
      if (service.supported) button.addEventListener("click", () => showPolicyTestSubjects(summary.subjects || [], "", `Line ${line} · ${service.name || service.id} · ${effect} · ${fullCountLabel}`));
      row.append(button);
    }
    fragment.append(row);
  }
  gutter.replaceChildren(fragment);
  gutter.hidden = false;
  gutter.style.transform = `translateY(${-byId("policy-source").scrollTop}px)`;
}

function policyTestErrorLines(message, lineCount) {
  return [...new Set(Array.from(String(message).matchAll(/\bline\s+(\d+)(?:\s*,\s*column\s+\d+)?\b/gi), (match) => Number(match[1]))) ]
    .filter((line) => line >= 1 && line <= lineCount)
    .sort((left, right) => left - right);
}

function renderPolicyTestErrorGutter(message, errorLines) {
  const gutter = byId("policy-test-gutter");
  const errorLineSet = new Set(errorLines);
  const fragment = document.createDocumentFragment();
  const lineCount = byId("policy-source").value.split("\n").length;
  for (let line = 1; line <= lineCount; line += 1) {
    const row = document.createElement("div");
    row.className = "policy-test-line";
    row.dataset.line = String(line);
    if (errorLineSet.has(line)) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "policy-test-line-result";
      button.dataset.effect = "error";
      button.textContent = "ERR";
      button.title = `Line ${line} · Policy test error · Show diagnostic`;
      button.setAttribute("aria-label", button.title);
      button.addEventListener("click", () => showPolicyTestError(line, message));
      row.append(button);
    }
    fragment.append(row);
  }
  gutter.replaceChildren(fragment);
  gutter.hidden = false;
  gutter.style.transform = `translateY(${-byId("policy-source").scrollTop}px)`;
}

function policyTestDimensionCounts(summary) {
  return state.policy.testDimensions
    .map((dimension) => ({ dimension, count: Number(summary.by_dimension?.[dimension]) || 0 }))
    .filter(({ count }) => count > 0);
}

function policyTestCountLabel(summary) {
  const dimensions = policyTestDimensionCounts(summary);
  if (!(Number(summary.count) > 0) && !dimensions.length) return "None";
  if (dimensions.length) {
    return dimensions.map(({ dimension, count }) => `${count} ${pluralPolicyTestDimensionLabel(dimension, count)}`).join(" · ");
  }
  const count = Number(summary.count) || 0;
  return `${count} ${count === 1 ? "Match" : "Matches"}`;
}

function renderPolicyTestCount(button, summary, countLabel) {
  if (countLabel === "None") {
    button.textContent = countLabel;
    return;
  }
  const dimensions = policyTestDimensionCounts(summary);
  if (!dimensions.length) {
    const number = document.createElement("strong");
    number.className = "policy-test-count-number";
    number.textContent = String(summary.count);
    button.append(number, document.createTextNode(` ${Number(summary.count) === 1 ? "Match" : "Matches"}`));
    return;
  }
  dimensions.forEach(({ dimension, count }, index) => {
    if (index) button.append(document.createTextNode(" · "));
    const item = document.createElement("span");
    item.className = "policy-test-count";
    const number = document.createElement("strong");
    number.className = "policy-test-count-number";
    number.textContent = String(count);
    const label = document.createElement("span");
    label.className = "policy-test-count-label";
    label.textContent = pluralPolicyTestDimensionLabel(dimension, count);
    item.append(number, document.createTextNode(" "), label);
    button.append(item);
  });
}

function pluralPolicyTestDimensionLabel(dimension, count) {
  const label = policyTestDimensionLabel(dimension);
  if (count === 1) return label;
  if (/(s|x|z|ch|sh)$/i.test(label)) return `${label}es`;
  if (/[^aeiou]y$/i.test(label)) return `${label.slice(0, -1)}ies`;
  return `${label}s`;
}

function policyTestDimensionLabel(name) {
  return String(name).split("_").map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(" ");
}

function showPolicyTestSubjects(subjects, dimension, title) {
  const list = byId("policy-test-subjects");
  const unique = new Map();
  for (const subject of subjects) {
    const key = dimension ? subject.dimensions?.[dimension] : subject.id;
    if (!key || unique.has(key)) continue;
    unique.set(key, subject);
  }
  list.replaceChildren();
  byId("policy-test-title").textContent = "Policy test results";
  byId("policy-test-subject-title").textContent = `${title} · ${unique.size} ${unique.size === 1 ? "member" : "members"}${unique.size > 100 ? " · First 100 shown" : ""}`;
  if (!unique.size) {
    const empty = document.createElement("p");
    empty.className = "policy-test-empty";
    empty.textContent = "No matching identities.";
    list.append(empty);
  } else {
    for (const subject of [...unique.values()].slice(0, 100)) {
      const item = document.createElement("div");
      item.className = "policy-test-subject";
      item.append(document.createTextNode(subject.label || subject.id));
      const meta = document.createElement("small");
      meta.textContent = Object.entries(subject.dimensions || {}).map(([name, value]) => `${policyTestDimensionLabel(name)}: ${value}`).join(" · ") || policyTestDimensionLabel(subject.kind || "actor");
      item.append(meta);
      list.append(item);
    }
  }
  byId("policy-test-dialog").showModal();
}

function showPolicyTestError(line, error) {
  const list = byId("policy-test-subjects");
  byId("policy-test-title").textContent = "Policy test error";
  byId("policy-test-subject-title").textContent = `Line ${line} diagnostic`;
  const detail = document.createElement("div");
  detail.className = "policy-test-subject";
  detail.textContent = error;
  list.replaceChildren(detail);
  byId("policy-test-dialog").showModal();
}

function closePolicyTestSubjects() {
  if (byId("policy-test-dialog").open) byId("policy-test-dialog").close();
}

function stopPolicyTest() {
  const policy = state.policy;
  if (policy.testAbort) policy.testAbort.abort();
  policy.testAbort = null;
  policy.testPending = false;
  policy.testMode = false;
  policy.testResult = null;
  policy.testSource = "";
  policy.testDimensions = [];
  closePolicyTestSubjects();
  byId("policy-test-gutter").replaceChildren();
  byId("policy-test-gutter").hidden = true;
  byId("policy-test-status").hidden = true;
  byId("policy-test-status").textContent = "";
  updatePolicyDirtyState();
}

function renderPolicyStageStatus() {
  const policy = state.policy;
  const status = byId("policy-stage-status");
  if (policy.stagePending) {
    status.textContent = "Compiling and staging candidate…";
    status.dataset.state = "pending";
  } else if (policy.stageError) {
    status.textContent = policy.stageError;
    status.dataset.state = "error";
  } else if (policy.stagedCandidate) {
    const candidate = policy.stagedCandidate;
    status.textContent = `Staged ${candidate.record_name} r${candidate.record_revision} · SHA-256 ${candidate.bundle_sha256.slice(0, 12)} · not pushed`;
    status.dataset.state = "staged";
  } else if (!policy.stagingReady) {
    status.textContent = "Policy staging is not configured.";
    status.dataset.state = "error";
  } else {
    status.textContent = policy.record?.kind === "policy" ? "Saved revision can be staged; live policy will not change." : "Select a saved policy revision to stage.";
    status.dataset.state = "";
  }
}

async function stageSelectedPolicy() {
  const policy = state.policy;
  if (!policy.record || policy.record.kind !== "policy" || policy.record.isDraft || policy.browsingRevision || byId("policy-source").value !== policy.savedSource || !policy.stagingReady || policy.stagePending) return;
  policy.stagePending = true;
  policy.stageError = "";
  updatePolicyDirtyState();
  try {
    const response = await fetch(`/api/policy/records/${encodeURIComponent(policy.record.id)}/stage`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ expected_revision: policy.record.current_revision }),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `Staging failed (${response.status})`);
    policy.stagedCandidate = result;
  } catch (error) {
    policy.stageError = error.message || "Could not stage policy candidate.";
  } finally {
    policy.stagePending = false;
    updatePolicyDirtyState();
  }
}

async function reloadPolicyWorkspace() {
  if ((hasUnsavedPolicyChanges(state.policy) || state.policy.browsingRevision || state.policy.record?.isDraft) && !window.confirm("Discard changes and return to the last saved policy?")) return;
  if (!state.policy.record) return loadPolicyWorkspace();
  if (state.policy.record.isDraft) {
    clearPolicySelection();
    return;
  }
  await selectPolicyRecord(state.policy.record.id, true, true);
}

async function checkPolicy() {
  const button = byId("policy-check");
  const source = byId("policy-source").value;
  state.policy.errorOffsets = [];
  updatePolicyHighlight();
  button.disabled = true;
  byId("policy-check-result").textContent = "Checking with ZPLC…";
  try {
    const response = await fetch("/api/policy/check", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ source }),
    });
    const result = await response.json();
    policySetCheckResult(response.ok && result.valid, result.diagnostics || result.error || "No compiler diagnostics.", source);
  } catch (error) {
    policySetCheckResult(false, error.message, source);
  } finally {
    updatePolicyDirtyState();
  }
}

function policySetCheckResult(valid, diagnostics, source) {
  const policy = state.policy;
  policy.validSource = valid ? source : null;
  policy.errorOffsets = valid ? [] : extractZPLErrorOffsets(diagnostics, source);
  updatePolicyHighlight();
  const result = byId("policy-check-result");
  result.textContent = diagnostics;
  result.dataset.state = valid ? "valid" : "invalid";
  if (policy.errorOffsets.length) {
    const beforeError = source.slice(0, policy.errorOffsets[0]);
    const line = beforeError.split("\n").length - 1;
    const lineHeight = Number.parseFloat(getComputedStyle(byId("policy-source")).lineHeight) || 20;
    byId("policy-source").scrollTop = Math.max(0, line * lineHeight - lineHeight * 2);
    updatePolicyHighlight();
  }
  updatePolicyDirtyState();
}

async function savePolicy() {
  byId("version-note").value = "";
  byId("version-error").textContent = "";
  byId("version-dialog").showModal();
  byId("version-note").focus();
}

async function appendPolicyVersion(summary) {
  const policy = state.policy;
  const button = byId("policy-save");
  const submittedSource = byId("policy-source").value;
  const isDraft = Boolean(policy.record.isDraft);
  const recordName = policy.record.name.trim();
  if (isDraft && !recordName) {
    byId("version-error").textContent = "Enter a policy name before saving.";
    return;
  }
  if (policy.validSource !== submittedSource) {
    byId("version-error").textContent = "Evaluate this exact buffer before saving.";
    return;
  }
  const recordID = policy.record.id;
  const expectedRevision = policy.revision;
  button.disabled = true;
  byId("version-dialog").close();
  byId("policy-check-result").textContent = "Saving version…";
  try {
    let response;
    let result;
    let nextRevision;
    let contentHash;
    if (isDraft) {
      response = await fetch("/api/policy/records", {
        method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({
          category_id: policy.record.category_id, name: recordName, kind: "policy",
          content_type: policy.record.content_type, metadata: policy.record.metadata,
          content: submittedSource, summary,
        }),
      });
      result = await response.json();
      if (!response.ok) throw new Error(result.error || result.diagnostics || `Policy save failed (${response.status})`);
      nextRevision = result.current_revision;
      contentHash = result.content_hash;
      policy.record = { ...result, isDraft: false };
      policy.records.push(policy.record);
    } else {
      response = await fetch(`/api/policy/records/${encodeURIComponent(recordID)}/revisions`, {
        method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ content: submittedSource, expected_revision: expectedRevision, summary }),
      });
      result = await response.json();
      if (!response.ok) throw new Error(result.error || result.diagnostics || `Version save failed (${response.status})`);
      nextRevision = result.number;
      contentHash = result.content_hash;
    }
    policy.revision = nextRevision;
    policy.savedSource = submittedSource;
    policy.validSource = policy.savedSource;
    policy.record.current_revision = nextRevision;
    policy.record.content = submittedSource;
    policy.record.content_hash = contentHash;
    policy.browsingRevision = 0;
    renderPolicyIdentity(policy.record, nextRevision, contentHash);
    byId("policy-check-result").textContent = result.diagnostics || `Created Version ${nextRevision}.`;
    byId("policy-check-result").dataset.state = "valid";
    await loadPolicyHistory(policy.record.id);
    await refreshPolicyCatalog();
    updatePolicyDirtyState();
  } catch (error) {
    policy.validSource = null;
    updatePolicyDirtyState();
    byId("version-dialog").showModal();
    byId("version-error").textContent = error.message;
  }
}

byId("version-form").addEventListener("submit", (event) => {
  event.preventDefault();
  appendPolicyVersion(byId("version-note").value.trim());
});

function renderAssistantMessages() {
  const list = byId("assistant-messages");
  list.replaceChildren();
  if (!state.policy.messages.length && !state.policy.assistantError) {
    const empty = document.createElement("p");
    empty.className = "assistant-empty";
    empty.textContent = "No conversation yet.";
    list.append(empty);
  }
  for (const message of state.policy.messages) {
    const entry = document.createElement("article");
    entry.className = `assistant-message ${message.role}`;
    const label = document.createElement("strong");
    label.textContent = message.role === "user" ? "YOU" : "CLAUDE";
    const content = document.createElement("pre");
    content.textContent = message.content;
    entry.append(label, content);
    list.append(entry);
  }
  if (state.policy.assistantError) {
    const error = document.createElement("p");
    error.className = "assistant-error";
    error.textContent = state.policy.assistantError;
    list.append(error);
  }
  if (state.policy.assistantPending) {
    const pending = document.createElement("p");
    pending.className = "assistant-pending";
    pending.textContent = "Claude is responding…";
    list.append(pending);
  }
  list.scrollTop = list.scrollHeight;
}

async function askPolicyAssistant(question) {
  const policy = state.policy;
  policy.messages = policy.messages.slice(-18);
  policy.messages.push({ role: "user", content: question });
  policy.assistantPending = true;
  policy.assistantError = "";
  byId("assistant-send").disabled = true;
  renderAssistantMessages();
  try {
    const response = await fetch("/api/policy/assistant", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ source: byId("policy-source").value, messages: policy.messages, model: byId("assistant-model").value, max_tokens: Number(byId("assistant-max-tokens").value) }),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `Claude request failed (${response.status})`);
    policy.messages.push({ role: "assistant", content: result.answer });
    policy.assistantUsage.input += Number(result.input_tokens) || 0;
    policy.assistantUsage.output += Number(result.output_tokens) || 0;
  } catch (error) {
    policy.assistantError = error.message;
  } finally {
    policy.assistantPending = false;
    updateAssistantControls();
    renderAssistantMessages();
  }
}

const serviceTypePalette = new Map([
  ["BuiltIn", { color: "#294879", background: "#e8edf9", border: "#8da4cb" }],
  ["Regular", { color: "#285b3a", background: "#e8f3e9", border: "#80ad8a" }],
  ["Visa", { color: "#873c2b", background: "#ffe5df", border: "#d89480" }],
  ["Gateway", { color: "#754000", background: "#fff1c8", border: "#cc963d" }],
  ["ZPR", { label: "ZPR service", color: "#124e78", background: "#e3eff8", border: "#598cb4" }],
  ["Policy", { label: "Policy service", color: "#214e68", background: "#e4f0f6", border: "#6395ae" }],
  ["Control", { label: "Control service", color: "#66521e", background: "#f4f0da", border: "#ab963e" }],
  ["Auth", { label: "Authentication service", color: "#793947", background: "#f7e8eb", border: "#bd7c88" }],
  ["Attribute", { label: "Attribute service", color: "#31594c", background: "#e6f1ec", border: "#78a591" }],
  ["Application", { label: "Application service", color: "#47525a", background: "#e8ebf0", border: "#909ba3" }],
  ["Node", { color: "#42613b", background: "#ecf1df", border: "#91a875" }],
  ["Logger", { color: "#6c3e1f", background: "#f5ece4", border: "#b68657" }],
  ['Trusted("file")', { color: "#285d64", background: "#e7f1f3", border: "#4e8790" }],
  ['Trusted("rest/1")', { color: "#754820", background: "#fff0df", border: "#b8753f" }],
  ["Not in current policy", { color: "#55616b", background: "#edf0f2", border: "#a5aeb6" }],
]);

document.addEventListener("click", (event) => {
  const key = document.querySelector(".service-type-key");
  if (key.open && !key.contains(event.target)) key.open = false;
});
document.addEventListener("keydown", (event) => {
  const key = document.querySelector(".service-type-key");
  if (event.key === "Escape" && key.open) {
    key.open = false;
    key.querySelector("summary").focus();
  }
});

function serviceTypeAppearance(kind) {
  const key = String(kind || "Not in current policy");
  const trusted = key.match(/^Trusted\("([^" ]+)"\)$/)?.[1];
  const label = trusted ? `TRUSTED ${trusted.toUpperCase()}` : key;
  let palette = serviceTypePalette.get(key);
  if (!palette) {
    let hash = 0;
    for (const character of key) hash = (Math.imul(hash, 31) + character.codePointAt(0)) >>> 0;
    const hue = hash % 360;
    palette = { color: `hsl(${hue} 50% 25%)`, background: `hsl(${hue} 55% 94%)`, border: `hsl(${hue} 35% 55%)` };
  }
  return { key, ...palette, label: palette.label || label };
}

function applyServiceTypeColor(element, type) {
  element.style.color = type.color;
  element.style.backgroundColor = type.background;
  element.style.borderColor = type.border;
}

function renderServiceTypeLegend(services) {
  const legend = byId("service-type-legend");
  const kinds = [...new Set([...serviceTypePalette.keys(), ...services.map((service) => isGatewayService(service) ? "Gateway" : service.service_kind || "Not in current policy")])];
  const signature = JSON.stringify(kinds);
  if (legend.dataset.types === signature) return;
  legend.replaceChildren();
  for (const kind of kinds) {
    const type = serviceTypeAppearance(kind);
    const item = document.createElement("div");
    item.className = "service-type-legend-item";
    item.setAttribute("role", "listitem");
    const swatch = document.createElement("span");
    swatch.className = "service-type-swatch";
    swatch.setAttribute("aria-hidden", "true");
    applyServiceTypeColor(swatch, type);
    swatch.style.backgroundColor = type.color;
    const label = document.createElement("span");
    label.textContent = type.label;
    item.append(swatch, label);
    legend.append(item);
  }
  legend.dataset.types = signature;
}

function renderServices(data) {
  const services = data.services || [];
  renderServiceTypeLegend(services);
  byId("service-count").textContent = `${formatNumber(services.length)} Services`;
  const providerFor = (service) => data.actors.find((actor) => actor.cn === service.actor_cn);
  const columns = { name: (service) => service.service_name, kind: (service) => service.service_kind, actor: (service) => `${actorDisplayName(providerFor(service))} ${service.actor_cn || ""}`, address: (service) => `${dnsAddressLabel(service.zpr_addr)} ${service.zpr_addr || ""}`, endpoints: (service) => service.service_endpoints };
  const shown = visibleRows("services", services, columns);
  byId("service-rows").innerHTML = shown.length ? shown.map((service) => {
    const type = serviceTypeAppearance(isGatewayService(service) ? "Gateway" : service.service_kind);
    const provider = providerFor(service);
    const providerLabel = actorDisplayName(provider || { cn: service.actor_cn });
    return `<tr class="selectable-row" data-inspect-service="${escapeHTML(service.service_name)}" tabindex="0" role="button" aria-label="Inspect service ${escapeHTML(service.service_name)}"><td>${escapeHTML(service.service_name || "—")}</td><td><span class="role-chip service-type-chip" data-service-type="${escapeHTML(type.key)}">${escapeHTML(type.label)}</span></td><td>${escapeHTML(providerLabel)}</td><td class="mono" title="${escapeHTML(dnsAddressTitle(service.zpr_addr))}">${escapeHTML(dnsAddressLabel(service.zpr_addr))}</td><td>${escapeHTML(service.service_endpoints || "—")}</td></tr>`;
  }).join("") : `<tr><td colspan="5" class="empty-row">${services.length ? "No matching services" : "No network services returned"}</td></tr>`;
  for (const chip of byId("service-rows").querySelectorAll("[data-service-type]")) applyServiceTypeColor(chip, serviceTypeAppearance(chip.dataset.serviceType));
}

function renderVisas(data) {
  byId("visa-total").textContent = `${formatNumber(data.visa_count)} Visas`;
  const rows = data.recent_visas || [];
  const columns = { id: (visa) => num(visa.id), flow: (visa) => `${visaEndpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port)} ${visa.source_addr || ""} ${visa.dest_addr || ""}`, proto: (visa) => visa.proto, node: (visa) => visa.requesting_node, expires: (visa) => num(visa.expires) };
  const shown = visibleRows("visas", rows, columns);
  byId("visa-rows").innerHTML = shown.length ? shown.map((visa) => {
    const flow = visaEndpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port);
    return `<tr><td class="mono">${escapeHTML(visa.id)}</td><td class="mono" title="${escapeHTML(visaEndpointTitle(visa.source_addr, visa.dest_addr))}">${escapeHTML(flow)}</td><td>${escapeHTML(visa.proto || "—")}</td><td>${escapeHTML(visa.requesting_node || "—")}</td><td class="mono">${escapeHTML(new Date(num(visa.expires) * 1000).toLocaleTimeString())}</td></tr>`;
  }).join("") : `<tr><td colspan="5" class="empty-row">${rows.length ? "No matching visas" : "No active visas returned"}</td></tr>`;
}

function protocolName(number) {
  return ({ 1: "ICMP", 6: "TCP", 17: "UDP", 58: "ICMPv6" })[number] || `IP ${number}`;
}

function renderDenies(data) {
  const denies = data.recent_denies || [];
  const total = denies.reduce((sum, record) => sum + num(record.count), 0);
  byId("deny-total").textContent = `${formatNumber(total)} Denys`;
  const columns = { source: (record) => `${dnsAddressLabel(record.source_addr)} ${record.source_addr || ""}`, destination: (record) => `${dnsAddressLabel(record.dest_addr)} ${record.dest_addr || ""}`, protocol: (record) => `${protocolName(record.protocol)}/${record.dest_port}`, reason: (record) => record.deny_code, count: (record) => num(record.count) };
  const shown = visibleRows("denies", denies, columns);
  byId("deny-list").innerHTML = shown.length ? shown.map((record) =>
    `<tr><td class="mono" title="${escapeHTML(dnsAddressTitle(record.source_addr))}">${escapeHTML(dnsAddressLabel(record.source_addr))}</td><td class="mono" title="${escapeHTML(dnsAddressTitle(record.dest_addr))}">${escapeHTML(dnsAddressLabel(record.dest_addr))}</td><td>${escapeHTML(columns.protocol(record))}</td><td>${escapeHTML(record.deny_code)}</td><td class="mono deny-count">${formatNumber(record.count)}</td></tr>`
  ).join("") : `<tr><td colspan="5" class="empty-row">${denies.length ? "No matching denials" : "No recent policy denials"}</td></tr>`;
}

function flashPolledChange(node) {
  node.classList.remove("poll-changed");
  void node.offsetWidth;
  node.classList.add("poll-changed");
  window.setTimeout(() => node.classList.remove("poll-changed"), 1800);
}

function capturePolledFields() {
  const fields = new Map();
  const summaryIDs = [
    "metric-actors", "metric-adapters", "metric-services", "metric-allowed", "metric-denied", "metric-visas", "metric-nodes", "metric-nodes-note",
    "connection-count", "actor-count", "service-count", "visa-total", "deny-total",
    "dns-stat-requests", "dns-stat-success", "dns-stat-nxdomain", "dns-stat-servfail",
  ];
  for (const id of summaryIDs) {
    const node = byId(id);
    if (node) fields.set(`summary:${id}`, { node, value: node.textContent.trim() });
  }
  for (const body of document.querySelectorAll("#link-list, #actor-rows, #service-rows, #trusted-list, #visa-rows, #deny-list, #dns-counter-rows, #dns-zone-rows, #dns-record-rows")) {
    for (const row of body.querySelectorAll("tr")) {
      if (row.querySelector(".empty-row")) continue;
      const identity = row.dataset.inspectActor || row.dataset.inspectService || row.dataset.inspectSource || row.dataset.inspectLink
        || [...row.cells].slice(0, Math.min(3, row.cells.length)).map((cell) => cell.textContent.trim()).join("|");
      [...row.cells].forEach((cell, index) => fields.set(`row:${body.id}:${identity}:${index}`, { node: cell, value: cell.textContent.trim() }));
    }
  }
  return fields;
}

function highlightChangedPolledFields() {
  const current = capturePolledFields();
  if (previousPolledValues) {
    for (const [key, field] of current) {
      if (previousPolledValues.has(key) && previousPolledValues.get(key) !== field.value) flashPolledChange(field.node);
    }
  }
  previousPolledValues = new Map([...current].map(([key, field]) => [key, field.value]));
}

function snapshotRemovedTopologyComponents(keys) {
  if (!keys.length) return [];
  const world = byId("topology-stage").querySelector("#graph-world");
  if (!world) return [];
  const removed = new Set(keys);
  const components = new Map([...world.children]
    .filter((component) => component.hasAttribute("data-topology-component"))
    .map((component) => [component.dataset.topologyComponent, component]));
  const exitOffset = (component) => {
    let x = 0, y = 0;
    const visited = new Set();
    while (component?.hasAttribute("data-arrival-dx")) {
      x += Number(component.dataset.arrivalDx);
      y += Number(component.dataset.arrivalDy);
      const parentKey = component.dataset.topologyParent;
      if (!parentKey || visited.has(parentKey)) break;
      if (!removed.has(parentKey)) {
        const parent = components.get(parentKey);
        if (parent) {
          const movement = graphMotionAt(parent);
          x += movement.x;
          y += movement.y;
        }
        break;
      }
      visited.add(parentKey);
      component = components.get(parentKey);
    }
    return { x, y };
  };
  return [...components.values()]
    .filter((component) => removed.has(component.dataset.topologyComponent))
    .map((component) => {
      const bounds = component.getBBox();
      const clone = component.cloneNode(true);
      const currentMotion = graphMotionAt(component);
      clone.dataset.exitStartX = String(currentMotion.x);
      clone.dataset.exitStartY = String(currentMotion.y);
      clone.dataset.exitStartScale = String(currentMotion.scale);
      if (component.hasAttribute("data-arrival-dx")) {
        const offset = exitOffset(component);
        clone.dataset.exitDx = String(offset.x);
        clone.dataset.exitDy = String(offset.y);
      }
      clone.classList.remove("filtered", "arriving");
      clone.classList.add("graph-exiting");
      clone.removeAttribute("tabindex");
      clone.removeAttribute("role");
      clone.removeAttribute("data-inspect-actor");
      clone.removeAttribute("data-inspect-service");
      clone.setAttribute("aria-hidden", "true");
      clone.querySelectorAll(".graph-arrival-marker").forEach((marker) => marker.remove());
      const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
      label.setAttribute("class", "graph-exit-label");
      label.setAttribute("x", String(bounds.x + bounds.width / 2));
      label.setAttribute("y", String(bounds.y - 7));
      label.textContent = "REMOVED";
      clone.append(label);
      const kind = component.classList.contains("graph-service-badge") ? "service" : component.classList.contains("node") ? "node" : "adapter";
      return { kind, markup: clone.outerHTML };
    });
}

function render(data) {
  const componentKeys = new Set([
    ...data.actors.map((actor) => `actor:${JSON.stringify(actor.cn)}`),
    ...(data.services || []).map((service) => `service:${JSON.stringify([service.actor_cn, service.service_name])}`),
  ]);
  const now = Date.now();
  const removedKeys = state.topologyComponents
    ? [...state.topologyComponents].filter((key) => !componentKeys.has(key))
    : [];
  const exitComponents = state.graphAnimations ? snapshotRemovedTopologyComponents(removedKeys) : [];
  for (const [key, startedAt] of state.topologyNewComponents) {
    if (now - startedAt >= GRAPH_ARRIVAL_DURATION) state.topologyNewComponents.delete(key);
  }
  if (state.topologyComponents && state.graphAnimations) {
    for (const key of componentKeys) {
      if (!state.topologyComponents.has(key)) state.topologyNewComponents.set(key, now);
    }
  }
  state.topologyComponents = componentKeys;
  state.snapshot = data;
  updateConnection(data);
  renderMetrics(data);
  renderTopology(data, exitComponents);
  renderActors(data);
  renderServices(data);
  renderTrusted(data);
  renderVisas(data);
  renderDenies(data);
  if (state.selection) renderInspector();
  void refreshPolicyContext();
  highlightChangedPolledFields();
  void loadDNSRecords();
}

async function refresh() {
  if (state.pending) return;
  state.pending = true;
  byId("refresh-now").disabled = true;
  if (currentPage() === "dns") void loadDNSStats();
  try {
    const response = await fetch("/api/snapshot", { cache: "no-store", headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error(`Monitor server responded ${response.status}`);
    render(await response.json());
  } catch (error) {
    updateConnection({ api_status: "disconnected", errors: [error.message] });
  } finally {
    state.pending = false;
    byId("refresh-now").disabled = false;
  }
}

function setPollTimer() {
  if (state.timer) clearInterval(state.timer);
  const seconds = Number(byId("poll-rate").value);
  byId("footer-poll").textContent = state.paused ? "Auto-refresh paused" : `Auto-refresh every ${seconds} seconds`;
  if (!state.paused) state.timer = setInterval(refresh, seconds * 1000);
}

byId("refresh-now").addEventListener("click", refresh);
function updatePolicyWorkbenchLayout() {
  const workbench = byId("policy-workbench");
  const pickerCollapsed = workbench.dataset.pickerCollapsed === "true";
  const assistantCollapsed = workbench.dataset.assistantCollapsed === "true";
  const assertionsSelected = workbench.dataset.recordKind === "assertions";
  const testMode = workbench.dataset.testMode === "true";
  const assertionTestMode = workbench.dataset.assertionTestMode === "true";
  const mobile = window.matchMedia("(max-width: 600px)").matches;
  const tablet = window.matchMedia("(max-width: 900px)").matches;
  let columns;

  if (!tablet && !mobile) {
    const picker = byId("policy-catalog-pane");
    const editor = byId("policy-editor-pane");
    const assistant = byId("policy-assistant-pane");
    workbench.style.display = "flex";
    workbench.style.removeProperty("grid-template-columns");
    picker.style.flex = `0 0 ${pickerCollapsed ? "42px" : "23%"}`;
    editor.style.flex = "1 1 0";
    assistant.style.flex = `0 0 ${assistantCollapsed ? "42px" : "27%"}`;
    return;
  }

  workbench.style.removeProperty("display");
  for (const id of ["policy-catalog-pane", "policy-editor-pane", "policy-assistant-pane"]) {
    byId(id).style.removeProperty("flex");
  }
  if (mobile || (assertionsSelected && assertionTestMode)) {
    columns = "minmax(0, 1fr)";
  } else if (testMode) {
    columns = assistantCollapsed ? "minmax(0, 1fr) 42px" : "minmax(0, 1.8fr) minmax(245px, .8fr)";
  } else if (assertionsSelected) {
    columns = pickerCollapsed ? "42px minmax(0, 1fr)" : "minmax(180px, .65fr) minmax(330px, 1.7fr)";
  } else if (tablet) {
    columns = pickerCollapsed ? "42px minmax(0, 1fr)" : "minmax(175px, .6fr) minmax(0, 1.6fr)";
  } else if (pickerCollapsed && assistantCollapsed) {
    columns = "42px minmax(0, 1fr) 42px";
  } else if (pickerCollapsed) {
    columns = "42px minmax(330px, 1.7fr) minmax(245px, .8fr)";
  } else if (assistantCollapsed) {
    columns = "minmax(180px, .65fr) minmax(330px, 1.7fr) 42px";
  } else {
    columns = "minmax(180px, .65fr) minmax(330px, 1.7fr) minmax(245px, .8fr)";
  }
  workbench.style.gridTemplateColumns = columns;
}

window.policyWorkbenchLayoutChanged = updatePolicyWorkbenchLayout;

function policyPaneCollapsed(name) {
  try { return localStorage.getItem(`zpr-policy-pane-${name}-collapsed`) === "true"; }
  catch { return false; }
}

function setPolicyPaneCollapsed(name, collapsed) {
  const workbench = byId("policy-workbench");
  const pane = byId(name === "picker" ? "policy-catalog-pane" : "policy-assistant-pane");
  const button = byId(name === "picker" ? "policy-picker-toggle" : "policy-assistant-toggle");
  const label = name === "picker" ? "policy picker" : "Claude assistant";
  workbench.dataset[`${name}Collapsed`] = String(collapsed);
  pane.dataset.collapsed = String(collapsed);
  button.setAttribute("aria-expanded", String(!collapsed));
  button.setAttribute("aria-label", `${collapsed ? "Expand" : "Collapse"} ${label}`);
  button.title = `${collapsed ? "Expand" : "Collapse"} ${label}`;
  updatePolicyWorkbenchLayout();
  try { localStorage.setItem(`zpr-policy-pane-${name}-collapsed`, String(collapsed)); }
  catch { /* The pane still works when browser storage is unavailable. */ }
}

for (const name of ["picker", "assistant"]) {
  setPolicyPaneCollapsed(name, policyPaneCollapsed(name));
  byId(`policy-${name === "picker" ? "picker" : "assistant"}-toggle`).addEventListener("click", () => {
    const button = byId(`policy-${name === "picker" ? "picker" : "assistant"}-toggle`);
    setPolicyPaneCollapsed(name, button.getAttribute("aria-expanded") === "true");
    closePolicyPickerMenu();
  });
}
const policyPicker = document.querySelector(".policy-catalog-pane");
policyPicker.addEventListener("contextmenu", (event) => {
  if (event.target.closest("#policy-picker-menu")) return;
  event.preventDefault();
  void openPolicyPickerMenu(event.target, event.clientX, event.clientY);
});
policyPicker.addEventListener("keydown", (event) => {
  if (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10")) return;
  event.preventDefault();
  const bounds = event.target.getBoundingClientRect();
  void openPolicyPickerMenu(event.target, bounds.left, bounds.bottom);
});
byId("policy-picker-menu").addEventListener("click", (event) => {
  if (event.target.closest("button:not(:disabled)")) closePolicyPickerMenu();
});
document.addEventListener("pointerdown", (event) => {
  if (!event.target.closest("#policy-picker-menu")) closePolicyPickerMenu();
});
document.addEventListener("keydown", (event) => {
  const menu = byId("policy-picker-menu");
  if (menu.hidden) return;
  if (event.key === "Escape" || event.key === "Tab") {
    if (event.key === "Escape") event.preventDefault();
    closePolicyPickerMenu(true);
    return;
  }
  if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
  event.preventDefault();
  const items = [...menu.querySelectorAll("button:not(:disabled):not([hidden])")];
  if (!items.length) return;
  const index = items.indexOf(document.activeElement);
  const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
  items[next].focus();
});
window.addEventListener("hashchange", () => closePolicyPickerMenu());
window.addEventListener("resize", () => { closePolicyPickerMenu(); updatePolicyWorkbenchLayout(); });
byId("policy-refresh").addEventListener("click", reloadPolicyWorkspace);
byId("new-category").addEventListener("click", openCategoryDialog);
byId("new-policy-record").addEventListener("click", beginNewPolicyDraft);
byId("new-assertion-record").addEventListener("click", beginNewAssertionDraft);
byId("policy-copy").addEventListener("click", () => managePolicyFile("copy"));
byId("policy-paste").addEventListener("click", () => managePolicyFile("paste"));
byId("policy-duplicate").addEventListener("click", () => managePolicyFile("duplicate"));
byId("policy-delete").addEventListener("click", () => managePolicyFile("delete"));
byId("policy-restore").addEventListener("click", () => managePolicyFile("restore"));
byId("policy-show-archived").addEventListener("click", () => managePolicyFile("show-archived"));
byId("policy-save-as").addEventListener("click", () => openRecordDialog(true));
byId("category-form").addEventListener("submit", createPolicyCategory);
byId("record-form").addEventListener("submit", createPolicyRecord);
document.addEventListener("click", (event) => {
  const close = event.target.closest("[data-close-dialog]");
  if (close) byId(close.dataset.closeDialog).close();
  const category = event.target.closest("[data-category-id]");
  if (category) {
    const id = category.dataset.categoryId;
    state.policy.categoryID = id;
    if (state.policy.collapsedCategories.has(id)) state.policy.collapsedCategories.delete(id);
    else state.policy.collapsedCategories.add(id);
    renderPolicyCatalog();
  }
  const record = event.target.closest("[data-record-id]");
  if (record) selectPolicyRecord(record.dataset.recordId);
  const revision = event.target.closest("[data-revision]");
  if (revision) browsePolicyRevision(Number(revision.dataset.revision));
});
byId("policy-source").addEventListener("input", () => {
  state.policy.validSource = null;
  state.policy.errorOffsets = [];
  if (state.policy.testResult && byId("policy-test-dialog").open) {
    byId("policy-test-status").textContent = "Source changed since these results were produced.";
    byId("policy-test-status").dataset.state = "stale";
  }
  byId("policy-check-result").dataset.state = "";
  updatePolicyHighlight();
  updatePolicyDirtyState();
  showPolicyCompletions();
});
byId("policy-format").addEventListener("click", formatPolicySource);
byId("policy-test").addEventListener("click", runPolicyTest);
byId("policy-attribute-rescan").addEventListener("click", rescanPolicyAttributes);
byId("policy-draft-name").addEventListener("input", () => {
  if (!state.policy.record?.isDraft) return;
  state.policy.record.name = byId("policy-draft-name").value;
  renderPolicyIdentity(state.policy.record, 0, "");
  updatePolicyDirtyState();
});
byId("policy-source").addEventListener("scroll", () => {
  const textarea = byId("policy-source");
  const highlight = byId("policy-highlight");
  highlight.scrollTop = textarea.scrollTop;
  highlight.scrollLeft = textarea.scrollLeft;
  byId("policy-test-gutter").style.transform = `translateY(${-textarea.scrollTop}px)`;
  positionPolicyCompletions();
});
byId("policy-source").addEventListener("click", () => showPolicyCompletions());
byId("policy-source").addEventListener("keyup", (event) => {
  if (["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) showPolicyCompletions();
});
window.addEventListener("resize", positionPolicyCompletions);
byId("policy-source").addEventListener("keydown", (event) => {
  const menu = byId("policy-completions");
  const open = !menu.hidden;
  if ((event.ctrlKey || event.metaKey) && event.code === "Space") {
    event.preventDefault(); showPolicyCompletions(true); return;
  }
  if (event.key === "Tab" && event.shiftKey) { hidePolicyCompletions(); return; }
  if (open && event.key === "ArrowDown") { event.preventDefault(); movePolicyCompletion(1); return; }
  if (open && event.key === "ArrowUp") { event.preventDefault(); movePolicyCompletion(-1); return; }
  if (open && event.key === "Tab") {
    event.preventDefault();
    const selected = [...menu.querySelectorAll(".completion-option")].findIndex((option) => option.getAttribute("aria-selected") === "true");
    acceptPolicyCompletion(selected < 0 ? 0 : selected); return;
  }
  if (event.key === "Escape" && open) { event.preventDefault(); hidePolicyCompletions(); return; }
  if (event.key === "Tab") {
    event.preventDefault();
    const start = byId("policy-source").selectionStart;
    byId("policy-source").setRangeText("  ", start, byId("policy-source").selectionEnd, "end");
    byId("policy-source").dispatchEvent(new Event("input", { bubbles: true }));
  }
});
byId("policy-source").addEventListener("blur", (event) => {
  if (!event.relatedTarget?.closest("#policy-completions")) hidePolicyCompletions();
});
byId("policy-check").addEventListener("click", checkPolicy);
byId("policy-save").addEventListener("click", savePolicy);
byId("policy-stage").addEventListener("click", stageSelectedPolicy);
byId("assistant-enabled").addEventListener("change", (event) => {
  state.policy.assistantEnabled = event.target.checked;
  updateAssistantControls();
});
byId("assistant-form").addEventListener("submit", (event) => {
  event.preventDefault();
  const question = byId("assistant-question").value.trim();
  if (!question || !state.policy.assistantEnabled || !state.policy.assistantReady || state.policy.assistantPending) return;
  byId("assistant-question").value = "";
  askPolicyAssistant(question);
});
byId("pause-poll").addEventListener("click", (event) => {
  state.paused = !state.paused;
  event.currentTarget.textContent = state.paused ? "Resume updates" : "Pause updates";
  setPollTimer();
});
byId("poll-rate").addEventListener("change", setPollTimer);
byId("topology-search").addEventListener("input", () => state.snapshot && renderTopology(state.snapshot));

const pageRenderers = {
  connections: (data) => renderTopology(data),
  actors: renderActors,
  services: renderServices,
  sources: renderTrusted,
  visas: renderVisas,
  denies: renderDenies,
};

for (const [page, renderPage] of Object.entries(pageRenderers)) {
  const filterId = page === "actors" ? "actor-search" : `${page}-filter`;
  byId(filterId).addEventListener("input", () => state.snapshot && renderPage(state.snapshot));
  const table = document.querySelector(`table[data-sort-page="${page}"]`);
  for (const header of table.querySelectorAll("th[data-sort-key]")) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "sort-button";
    button.textContent = header.textContent;
    header.replaceChildren(button);
    button.addEventListener("click", () => {
      const previous = state.sorts[page];
      const direction = previous?.key === header.dataset.sortKey && previous.direction === 1 ? -1 : 1;
      state.sorts[page] = { key: header.dataset.sortKey, direction };
      for (const column of table.querySelectorAll("th[data-sort-key]")) {
        column.setAttribute("aria-sort", column === header ? direction === 1 ? "ascending" : "descending" : "none");
      }
      if (state.snapshot) renderPage(state.snapshot);
    });
  }
}

mountPolicyAssertionEditor();
window.addEventListener("hashchange", () => {
  showPage();
  closeInspector();
});
showPage();

document.addEventListener("click", (event) => {
  if (event.target.closest(".source-editor-link")) return;
  const target = event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-source], [data-inspect-link]");
  if (!target) return;
  if (target.dataset.inspectActor) openInspector("actor", target.dataset.inspectActor);
  else if (target.dataset.inspectService) openInspector("service", target.dataset.inspectService);
  else if (target.dataset.inspectSource) openInspector("source", target.dataset.inspectSource);
  else openInspector("link", target.dataset.inspectLink);
});

document.addEventListener("keydown", (event) => {
  if (event.target.closest(".source-editor-link")) return;
  const target = event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-source], [data-inspect-link]");
  if (!target || (event.key !== "Enter" && event.key !== " ")) return;
  event.preventDefault();
  if (target.dataset.inspectActor) openInspector("actor", target.dataset.inspectActor);
  else if (target.dataset.inspectService) openInspector("service", target.dataset.inspectService);
  else if (target.dataset.inspectSource) openInspector("source", target.dataset.inspectSource);
  else openInspector("link", target.dataset.inspectLink);
});

byId("inspector-close").addEventListener("click", closeInspector);
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") closeInspector();
});

refresh();
setPollTimer();