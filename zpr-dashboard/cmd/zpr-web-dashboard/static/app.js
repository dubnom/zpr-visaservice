const byId = (id) => document.getElementById(id);
const zplKeywords = new Set(["allow", "never", "define", "with", "to", "access", "and", "as", "aka", "tag", "tags", "on", "optional", "multiple", "signal", "over"]);
const GRAPH_ARRIVAL_DURATION = 2800;
const GRAPH_REMOVAL_DURATION = 900;
const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
const state = { snapshot: null, timer: null, paused: false, pending: false, graphCamera: null, graphAnimations: !reducedMotion, topologyComponents: null, topologyNodeColumns: null, topologyNodeSlots: new Map(), topologyNewComponents: new Map(), selection: null, sorts: {}, dnsPending: false, policy: { loaded: false, configured: false, categories: [], records: [], attributes: [], categoryID: "", collapsedCategories: new Set(), treeInitialized: false, record: null, source: "", savedSource: "", revision: 0, browsingRevision: 0, saveAs: false, compilerReady: false, testerReady: false, testMode: false, testPending: false, testAbort: null, testResult: null, testSource: "", testDimensions: [], saveTestPending: false, saveTestSource: "", saveTestError: "", saveAsTestSource: "", saveAsTestError: "", assistantReady: false, evaluatedSource: null, validSource: null, errorOffsets: [], checkDiagnostics: "", revisions: [] } };

let previousPolledValues = null;
let graphAutoFit = true;
let graphDarkMode = false;
let graphVisaFocus = null;
const mapViewports = new Map();
window.addEventListener("zpr-map-view", () => {
  if (state.snapshot) renderTopology(state.snapshot);
});
const policySourceLayout = window.ZPREditorPage.bindSourceLayout({
  source: byId("policy-source"), highlight: byId("policy-highlight"),
  gutterContent: byId("policy-test-gutter-content"), container: byId("policy-code-editor"),
  onScroll: positionPolicyCompletions, onResize: updatePolicyGutterBounds,
});
const policyAnalysisContext = () => [
  byId("policy-source").value, state.policy.record?.id, state.policy.record?.kind,
  state.policy.record?.isDraft, state.policy.record?.current_revision,
  state.policy.revision, state.policy.browsingRevision, state.policy.organizationID,
];
const policyEditorController = window.ZPREditorPage.createController({
  source: byId("policy-source"), status: byId("policy-file-status"), viewport: byId("policy-code-editor"),
  readContext: policyAnalysisContext, isDirty: () => hasUnsavedPolicyChanges(state.policy),
  identity: { title: byId("policy-record-title"), version: byId("policy-revision-label"), modified: byId("policy-modified-indicator") },
  menu: { root: byId("policy-actions"), toggle: byId("policy-files-toggle"), menu: byId("policy-file-menu") },
  history: { menu: byId("policy-history-menu"), list: byId("policy-history"), count: byId("policy-history-count"), isAvailable: () => Boolean(state.policy.record) },
  adapters: { load: loadPolicyRecord, analyze: evaluateAndTestPolicy, save: savePolicy, render: updatePolicyDirtyState },
  syncControls: updatePolicyDirtyState,
});
const policyCheckScope = policyEditorController.createScope("compiler");
const policyTestScope = policyEditorController.createScope("runtime");

function invalidatePolicyAnalysis() {
  policyCheckScope.invalidate();
  state.policy.checkAbort?.abort();
  state.policy.checkAbort = null;
  state.policy.checkPending = false;
  invalidatePolicyTest();
}

function invalidatePolicyTest() {
  policyTestScope.invalidate();
  state.policy.testAbort?.abort();
  state.policy.testAbort = null;
  state.policy.testPending = false;
  state.policy.saveTestOperation = null;
  state.policy.saveTestPending = false;
}

function activeMapVisas(data) {
  return Array.isArray(data.active_visas)
    ? [...new Map(data.active_visas.map(visa => [String(visa.id), visa])).values()].filter(visa => Number(visa.expires) > Date.now() / 1000)
    : null;
}

function applyMapVisaFocus(data) {
  const stage = byId("topology-stage");
  let focusedService = graphVisaFocus?.kind === "service"
    ? data.services.find(service => service.actor_cn === graphVisaFocus.actorCN && service.service_name === graphVisaFocus.serviceName)
    : null;
  const adapterCN = graphVisaFocus?.kind === "adapter" ? graphVisaFocus.actorCN : focusedService?.actor_cn;
  let adapter = data.actors.find(actor => !actor.node && actor.cn === adapterCN);
  if (!adapter || (graphVisaFocus?.kind === "service" && !focusedService)) {
    graphVisaFocus = null;
    focusedService = null;
    adapter = null;
  }
  stage.classList.toggle("graph-visa-focused", graphVisaFocus != null);
  syncMapClearHighlight();
  stage.querySelectorAll(".visa-focus").forEach(element => element.classList.remove("visa-focus"));
  const status = stage.querySelector(".graph-visa-focus-status");
  if (graphVisaFocus == null) {
    if (status) { status.hidden = true; status.textContent = ""; }
    return;
  }
  const actorKey = actor => `actor:${JSON.stringify(actor.cn)}`;
  const componentKeys = new Set([actorKey(adapter)]);
  if (focusedService) componentKeys.add(`service:${JSON.stringify([focusedService.actor_cn, focusedService.service_name])}`);
  const connectorPairs = new Set();
  const pairKey = (a, b) => JSON.stringify([a, b].sort());
  const addressActor = address => data.actors.find(actor => actor.zpr_addr && dnsAddressKey(actor.zpr_addr) === dnsAddressKey(address));
  const docks = actor => actor.node ? [actor] : data.actors.filter(node => node.node && node.node_details?.adapters?.includes(actor.cn));
  const visas = activeMapVisas(data);
  let matching = 0, missingRoutes = 0;
  for (const visa of visas || []) {
    const requester = String(visa.direction || "").toLowerCase() === "reverse" ? visa.dest_addr : visa.source_addr;
    const matchesFocus = focusedService
      ? serviceMatchesVisa(focusedService, visa, data.actors)
      : requester && adapter.zpr_addr && dnsAddressKey(requester) === dnsAddressKey(adapter.zpr_addr);
    if (!matchesFocus) continue;
    matching += 1;
    const source = addressActor(visa.source_addr), destination = addressActor(visa.dest_addr);
    for (const endpoint of [source, destination].filter(Boolean)) componentKeys.add(actorKey(endpoint));
    let nodes = [];
    if (Array.isArray(visa.path) && visa.path.length > 0) {
      nodes = visa.path.map(addressActor);
      if (nodes.some(node => !node?.node)) { missingRoutes += 1; nodes = []; }
    } else if (source && destination) {
      const sourceDocks = docks(source), destinationDocks = docks(destination);
      if (sourceDocks.length === 1 && destinationDocks.length === 1 && sourceDocks[0].cn === destinationDocks[0].cn) nodes = sourceDocks;
      else missingRoutes += 1;
    } else missingRoutes += 1;
    for (const node of nodes) componentKeys.add(actorKey(node));
    for (let index = 1; index < nodes.length; index += 1) connectorPairs.add(pairKey(actorKey(nodes[index - 1]), actorKey(nodes[index])));
    for (const [endpoint, node] of [[source, nodes[0]], [destination, nodes.at(-1)]]) {
      if (endpoint && node && !endpoint.node && node.node_details?.adapters?.includes(endpoint.cn)) connectorPairs.add(pairKey(actorKey(endpoint), actorKey(node)));
    }
    for (const service of focusedService ? [focusedService] : data.services || []) {
      if (!serviceMatchesVisa(service, visa, data.actors)) continue;
      const key = `service:${JSON.stringify([service.actor_cn, service.service_name])}`;
      componentKeys.add(key);
      connectorPairs.add(pairKey(`actor:${JSON.stringify(service.actor_cn)}`, key));
    }
  }
  for (const component of stage.querySelectorAll("#graph-world > [data-topology-component]")) {
    component.classList.toggle("visa-focus", componentKeys.has(component.dataset.topologyComponent));
  }
  for (const edge of stage.querySelectorAll("[data-connector-from][data-connector-to]")) {
    edge.classList.toggle("visa-focus", connectorPairs.has(pairKey(edge.dataset.connectorFrom, edge.dataset.connectorTo)));
  }
  if (status) {
    status.hidden = false;
    const focusName = focusedService ? focusedService.service_name : actorDisplayName(adapter);
    const focusDescription = focusedService ? "active visas serve this service" : "active outbound visas";
    status.textContent = visas == null ? `${focusName}: active visa inventory unavailable.`
      : `${focusName}: ${matching} ${focusDescription}.${missingRoutes ? ` Ordered route unavailable for ${missingRoutes}; no route inferred.` : ""} Right-click this ${focusedService ? "service" : "adapter"} or blank canvas to clear.`;
  }
}

const defaultTableSorts = {
  connections: { key: "from", direction: 1 },
  actors: { key: "cn", direction: 1 },
  services: { key: "name", direction: 1 },
  sources: { key: "name", direction: 1 },
  visas: { key: "id", direction: -1 },
  denies: { key: "count", direction: -1 },
  "dns-counters": { key: "counter", direction: 1 },
  "dns-zones": { key: "zone", direction: 1 },
  "dns-records": { key: "name", direction: 1 },
  "security-review": { key: "observed", direction: -1 },
};
for (const [page, sort] of Object.entries(defaultTableSorts)) state.sorts[page] = { ...sort };

state.policy.fileClipboard = null;
state.policy.showArchived = false;

const pages = {
  map: "MAP",
  connections: "ADAPTERS",
  actors: "ACTORS",
  "adapter-logs": "ADAPTER LOGS",
  services: "SERVICES",
  dns: "DNS",
  sources: "TRUSTED SOURCES",
  policy: "POLICY",
  visas: "VISAS",
  denies: "DENIALS",
  gateways: "GATEWAYS",
  "security-review": "SECURITY REVIEW",
  "zpr-config": "ZPR CONFIG",
  diagnostics: "DIAGNOSTICS",
  "provisioning-adapters": "ADAPTER PROVISIONING",
};
const statusPages = new Set(["connections", "actors", "services", "visas", "denies", "dns"]);

function updateStatusTabCounts(snapshot) {
  const actors = snapshot.actors || [];
  const denies = snapshot.recent_denies || [];
  const counts = {
    adapters: actors.filter((actor) => !actor.node).length,
    actors: actors.length,
    services: (snapshot.services || []).length,
    visas: num(snapshot.visa_count),
    denies: denies.reduce((sum, record) => sum + num(record.count), 0),
  };
  for (const [key, count] of Object.entries(counts)) {
    byId(`status-count-${key}`).textContent = formatNumber(count);
  }
}

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
  const statusPage = statusPages.has(page);
  document.querySelector(".status-banner").hidden = page !== "map";
  byId("status-tabs").hidden = !statusPage;
  for (const view of document.querySelectorAll(".page-view")) {
    view.hidden = view.dataset.page !== page;
    view.classList.toggle("active", !view.hidden);
  }
  for (const link of document.querySelectorAll("[data-page-link], [data-page-group]")) {
    const active = link.dataset.pageLink === page || (link.dataset.pageGroup === "status" && statusPage);
    link.classList.toggle("active", active);
    if (active) link.setAttribute("aria-current", "page");
    else link.removeAttribute("aria-current");
  }
  if (page === "policy") loadPolicyWorkspace();
  if (page === "dns") loadDNSStats();
  if (page === "provisioning-adapters") checkProvisioningAccess();
  if (page === "security-review" && !state.paused) void refresh();
  if (page === "map" && state.snapshot) renderTopology(state.snapshot);
}

function checkProvisioningAccess() {
  window.dispatchEvent(new Event("provisioning-refresh"));
}
byId("provisioning-recheck").addEventListener("click", () => void checkProvisioningAccess());

const escapeHTML = window.ZPRSafeDisplay.escapeHTML;

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
  byId("dns-stats-status").textContent = `BIND ${server.version || "9"} · Updated ${window.ZPRSafeDisplay.formatTime(status["current-time"])}`;
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
  byId("status-count-dns").textContent = formatNumber(zoneRows.length);
  byId("dns-zone-rows").innerHTML = zoneRows.length ? zoneRows.map((zone) =>
    `<tr><td data-sort-value="${escapeHTML(zone.name || "—")}">${escapeHTML(zone.name || "—")}<small class="dns-zone-view">${escapeHTML(zone.view)}</small></td><td>${escapeHTML(zone.type || "—")}</td><td class="mono" data-sort-value="${escapeHTML(zone.serial ?? "")}">${escapeHTML(zone.serial ?? "—")}</td><td class="mono">${dnsNumber(zone.rcodes, "QrySuccess")}</td><td class="mono">${dnsNumber(zone.rcodes, "QryNXDOMAIN")}</td><td class="mono">${dnsNumber(zone.qtypes, "AAAA")}</td></tr>`
  ).join("") : `<tr><td colspan="6" class="empty-row">No zone statistics returned</td></tr>`;
  sortControlRoomTableRows("dns-counters");
  sortControlRoomTableRows("dns-zones");
}

async function loadDNSStats() {
  if (state.dnsPending) return;
  state.dnsPending = true;
  if (byId("dns-stats-status").textContent.startsWith("Waiting")) byId("dns-stats-status").textContent = "Loading BIND statistics…";
  const recordsRequest = loadDNSRecords(true);
  try {
    const paths = ["status", "server", "zones"];
    const responses = await Promise.all(paths.map((path) => window.zprOperatorFetch(`/api/dns/stats/json/v1/${path}`, { cache: "no-store", headers: { Accept: "application/json" } })));
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
    const response = await window.zprOperatorFetch("/api/dns/records", { cache: "no-store", headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const result = await response.json();
    const records = Array.isArray(result.records) ? result.records : [];
    dnsAddressNames = indexDNSAddresses(records);
    document.dispatchEvent(new CustomEvent("control-room:dns-updated"));
    status.textContent = `${escapeHTML(result.zone || "DNS zone")} · ${formatNumber(records.length)} records`;
    rows.innerHTML = records.length ? records.map((record) =>
      `<tr><td class="mono">${escapeHTML(record.name || "—")}</td><td class="mono" data-sort-value="${escapeHTML(record.ttl ?? "")}">${escapeHTML(record.ttl ?? "—")}</td><td>${escapeHTML(record.type || "—")}</td><td class="mono dns-record-value">${escapeHTML(record.value || "—")}</td></tr>`
    ).join("") : `<tr><td colspan="4" class="empty-row">No records returned</td></tr>`;
    sortControlRoomTableRows("dns-records");
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
  const query = byId(`${page === "actors" ? "actor-search" : `${page}-filter`}`)?.value.trim().toLowerCase() || "";
  const filtered = rows.filter((row) => !query || Object.values(columns).some((value) => String(value(row) ?? "").toLowerCase().includes(query)));
  const sort = state.sorts[page];
  if (!sort) return filtered;
  return filtered.sort((left, right) => {
    const a = columns[sort.key](left) ?? "";
    const b = columns[sort.key](right) ?? "";
    const comparison = window.ZPRSortableTable.compareValues(a, b);
    return (comparison || compareStableRows(left, right, columns)) * sort.direction;
  });
}

function compareStableRows(left, right, columns) {
  for (const value of Object.values(columns)) {
    const a = String(value(left) ?? "");
    const b = String(value(right) ?? "");
    const comparison = a.localeCompare(b, undefined, { numeric: true }) || (a < b ? -1 : a > b ? 1 : 0);
    if (comparison) return comparison;
  }
  return 0;
}

function sortControlRoomTableRows(page) {
  const table = document.querySelector(`table[data-sort-page="${page}"]`);
  const sort = state.sorts[page];
  const body = table?.tBodies[0];
  if (!table || !sort || !body) return;
  const columnIndex = [...table.tHead.rows[0].cells].findIndex((cell) => cell.dataset.sortKey === sort.key);
  if (columnIndex < 0) return;
  const rows = [...body.rows].filter((row) => !row.querySelector(".empty-row"));
  rows.sort((left, right) => {
    const value = (row) => {
      const cell = row.querySelector(`[data-sort-cell="${sort.key}"]`) || row.cells[columnIndex];
      return cell?.dataset.sortValue ?? cell?.textContent.trim() ?? "";
    };
    const a = String(value(left));
    const b = String(value(right));
    const numericA = Number(a.replaceAll(",", ""));
    const numericB = Number(b.replaceAll(",", ""));
    const comparison = window.ZPRSortableTable.compareValues(a, b, { numericStrings: true });
    return (comparison || compareStableRows(left, right,
      Object.fromEntries([...table.tHead.rows[0].cells].map((_, index) =>
        [index, (row) => row.cells[index]?.dataset.sortValue ?? row.cells[index]?.textContent.trim() ?? ""])))) * sort.direction;
  });
  for (const row of rows) body.append(row);
}

window.sortControlRoomTableRows = sortControlRoomTableRows;

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
  byId("last-updated").textContent = snapshot.generated_at ? `Updated ${window.ZPRSafeDisplay.formatTime(snapshot.generated_at)}` : "Waiting for first snapshot";

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

// Visa Service reports no pair ID; a reverse visa pairs with the forward visa whose
// protocol matches, addresses are swapped and destination port is its source port.
function pairVisas(visas) {
  const direction = visa => String(visa.direction || "").toLowerCase();
  const key = (proto, from, to, port) => JSON.stringify([String(proto || "").toUpperCase(), from ? dnsAddressKey(from) : "", to ? dnsAddressKey(to) : "", String(port ?? "")]);
  const byID = (a, b) => num(a.id) - num(b.id) || String(a.id).localeCompare(String(b.id));
  const reverses = new Map();
  for (const visa of visas) {
    if (direction(visa) !== "reverse") continue;
    const reverseKey = key(visa.proto, visa.dest_addr, visa.source_addr, visa.source_port);
    if (!reverses.has(reverseKey)) reverses.set(reverseKey, []);
    reverses.get(reverseKey).push(visa);
  }
  const used = new Set();
  const groups = [];
  for (const visa of [...visas].sort(byID)) {
    const kind = direction(visa);
    if (kind === "reverse") continue;
    if (kind !== "forward") { groups.push({ forward: null, reverse: null, single: visa }); continue; }
    const candidates = (reverses.get(key(visa.proto, visa.source_addr, visa.dest_addr, visa.dest_port)) || []).filter(item => !used.has(item))
      .sort((a, b) => Math.abs(num(a.created) - num(visa.created)) - Math.abs(num(b.created) - num(visa.created)) || byID(a, b));
    const reverse = candidates[0] || null;
    if (reverse) used.add(reverse);
    groups.push({ forward: visa, reverse, single: null });
  }
  for (const visa of [...visas].sort(byID)) if (direction(visa) === "reverse" && !used.has(visa)) groups.push({ forward: null, reverse: visa, single: null });
  return groups.map(group => ({ ...group, members: [group.forward, group.reverse, group.single].filter(Boolean) }));
}

function visaPairLabel(group, visa) {
  if (group.single) return "Direction not reported";
  const partner = visa === group.forward ? group.reverse : group.forward;
  if (partner) return `↔ #${partner.id}`;
  return visa === group.forward ? "No reverse visa" : "No forward visa";
}

function currentVisaList(visas) {
  if (!visas.length) return `<p>No current visas.</p>`;
  const groups = pairVisas(visas).sort((a, b) => num(a.members[0].id) - num(b.members[0].id) || String(a.members[0].id).localeCompare(String(b.members[0].id)));
  const item = (group, visa) => `<div class="detail-item"><strong>Visa ${escapeHTML(visa.id)} · ${escapeHTML(visa.proto)}</strong><span title="${escapeHTML(visaEndpointTitle(visa.source_addr, visa.dest_addr))}">${escapeHTML(visaEndpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port))}</span><span>Direction: ${escapeHTML(visa.direction || "Not reported")} · Pair: ${escapeHTML(visaPairLabel(group, visa))}</span><span>Expires: ${escapeHTML(window.ZPRSafeDisplay.formatDateTime(num(visa.expires) * 1000))}</span><span>Node: ${escapeHTML(visa.requesting_node || "Not reported")} · Policy: ${escapeHTML(visa.policy_id || "Not reported")}</span><span>Route: ${escapeHTML(visa.path?.length ? visa.path.join(" → ") : "Not reported")}</span></div>`;
  const heading = group => group.forward && group.reverse
    ? `Connection · Visa ${group.forward.id} ↔ ${group.reverse.id}`
    : group.single ? `Visa ${group.single.id} · direction not reported` : group.forward ? `Visa ${group.forward.id} · no reverse visa` : `Visa ${group.reverse.id} · no forward visa`;
  return `<div class="detail-list">${groups.map(group => `<div class="visa-pair${group.forward && group.reverse ? " paired" : ""}" role="group" aria-label="${escapeHTML(heading(group))}"><p class="visa-pair-heading">${escapeHTML(heading(group))}</p>${group.members.map(visa => item(group, visa)).join("")}</div>`).join("")}</div>`;
}

let adapterVisaDetails;

function refreshAdapterVisas(actor) {
  if (adapterVisaDetails?.key === actor.cn && (adapterVisaDetails.pending || adapterVisaDetails.snapshot === state.snapshot)) return;
  const previous = adapterVisaDetails?.key === actor.cn ? adapterVisaDetails : null;
  adapterVisaDetails?.controller.abort();
  const request = { key: actor.cn, snapshot: state.snapshot, pending: true, loaded: previous?.loaded || false, items: previous?.items || [], error: "", controller: new AbortController() };
  adapterVisaDetails = request;
  window.zprOperatorFetch(`/api/actors/${encodeURIComponent(actor.cn)}/visas`, { cache: "no-store", signal: request.controller.signal }).then(async (response) => {
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
  window.dispatchEvent(new CustomEvent("zpr-selection", { detail: state.selection }));
  renderInspector();
}

function closeInspector() {
  adapterVisaDetails?.controller.abort();
  adapterVisaDetails = null;
  state.selection = null;
  window.dispatchEvent(new CustomEvent("zpr-selection", { detail: null }));
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

  if (state.selection.kind === "visa-count") {
    const selection = JSON.parse(state.selection.key);
    const actor = data.actors.find(item => item.cn === selection.actorCN);
    const service = selection.kind === "service"
      ? data.services.find(item => item.actor_cn === selection.actorCN && item.service_name === selection.serviceName)
      : null;
    if (!actor || selection.kind === "service" && !service) return closeInspector();
    kindLabel = "ACTIVE VISAS";
    title = service ? service.service_name : actorDisplayName(actor);
    const inventory = activeMapVisas(data);
    const visas = inventory?.filter(visa => service
      ? serviceMatchesVisa(service, visa, data.actors)
      : [visa.source_addr, visa.dest_addr].some(address => address && actor.zpr_addr && dnsAddressKey(address) === dnsAddressKey(actor.zpr_addr)));
    sections.push(detailSection("Visa inventory", [
      detailField("Scope", service ? `Service on ${actorDisplayName(actor)}` : "Adapter source or destination"),
      detailField("Active visas", visas?.length ?? "Unavailable"),
      detailField("Snapshot", data.generated_at ? window.ZPRSafeDisplay.formatDateTime(data.generated_at) : "Not reported"),
    ]));
    const content = visas == null
      ? `<p role="status">Active visa inventory unavailable. Recent decisions are not a complete current inventory.</p>`
      : currentVisaList(visas);
    sections.push(`<section class="detail-section"><h3>Current visas</h3>${content}</section>`);
  } else if (state.selection.kind === "denial-count") {
    const actor = data.actors.find(item => item.cn === state.selection.key && item.node);
    if (!actor) return closeInspector();
    const details = actor.node_details || {};
    kindLabel = "BUFFERED DENIALS";
    title = actorDisplayName(actor);
    sections.push(detailSection("Node denial telemetry", [
      detailField("Buffered denials", details.buffered_denials ?? "Unavailable"),
      detailField("Local denial occurrences", details.local_denials ?? "Unavailable"),
      ...(details.denial_stats_error ? [detailField("Telemetry", details.denial_stats_error)] : []),
      detailField("Snapshot", data.generated_at ? window.ZPRSafeDisplay.formatDateTime(data.generated_at) : "Not reported"),
    ]));
    const docked = new Set((details.adapters || []).map(name => data.actors.find(item => item.cn === name)?.zpr_addr).filter(Boolean).map(dnsAddressKey));
    const denials = (data.recent_denies || []).filter(record => record.source_addr && docked.has(dnsAddressKey(record.source_addr)))
      .sort((left, right) => num(right.last_deny_ms) - num(left.last_deny_ms));
    const items = denials.map(record => `<div class="detail-item"><strong>${escapeHTML(record.deny_code || "Denied")} · ${escapeHTML(`${protocolName(record.protocol)}/${record.dest_port}`)}</strong><span title="${escapeHTML(visaEndpointTitle(record.source_addr, record.dest_addr))}">${escapeHTML(`${dnsAddressLabel(record.source_addr)} → ${dnsAddressLabel(record.dest_addr)}`)}</span><span>Count: ${escapeHTML(formatNumber(record.count))} · Last: ${record.last_deny_ms ? escapeHTML(window.ZPRSafeDisplay.formatDateTime(num(record.last_deny_ms))) : "Not reported"}</span></div>`).join("");
    const content = !docked.size
      ? `<p role="status">No docked adapters with ZPR addresses are reported for this node.</p>`
      : items
        ? `<div class="detail-list" aria-label="Recent policy denials from docked adapters">${items}</div>`
        : `<p role="status">No recent Visa Service policy denials from adapters docked on this node.</p>`;
    sections.push(`<section class="detail-section"><h3>Recent policy denials from docked adapters</h3><p>The node reports buffered denials as a count only. These Visa Service denial records are matched by docked adapter source address and may not match the buffered count one-to-one.</p>${content}</section>`);
  } else if (state.selection.kind === "actor") {
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
      detailField("Authentication expires", actor.auth_exp ? window.ZPRSafeDisplay.formatDateTime(actor.auth_exp * 1000) : "No expiry reported"),
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
        detailField("Buffered denials", details.buffered_denials ?? "Unavailable"),
        detailField("Local denial occurrences", details.local_denials ?? "Unavailable"),
        ...(details.denial_stats_error ? [detailField("Denial telemetry", details.denial_stats_error)] : []),
        detailField("Installed visas", (details.visas || []).length),
        detailHTMLField("Docked adapters", attached.map((name) => {
          const linkedActor = data.actors.find((item) => item.cn === name);
          return `<button class="detail-link" type="button" data-inspect-actor="${escapeHTML(name)}">${escapeHTML(actorDisplayName(linkedActor || { cn: name }))}</button>`;
        }).join(" ")),
        detailHTMLField("Node links", outgoing),
      ]));
      const counters = details.counters || [];
      if (details.counter_stats_error || !counters.length) {
        sections.push(detailSection("Packet-processing counters", [
          detailField("Telemetry", details.counter_stats_error || "Node counters unavailable."),
        ]));
      } else {
        sections.push(detailSection("Packet-processing counters", [
          detailField("Sample time", details.counters_updated_at ? window.ZPRSafeDisplay.formatDateTime(details.counters_updated_at) : "Not reported"),
          detailField("Scope", "Cumulative since runtime restart or counter reset; per worker, not per route or link."),
        ]));
        const groups = new Map();
        for (const counter of counters) {
          const fields = groups.get(counter.group) || [];
          const label = counter.name.split("_").map(word => ["ttl", "micv", "zpi"].includes(word) ? word.toUpperCase() : word.charAt(0).toUpperCase() + word.slice(1)).join(" ");
          fields.push(detailField(label, counter.value, "mono"));
          groups.set(counter.group, fields);
        }
        for (const [group, fields] of groups) {
          sections.push(detailSection(group === "management" ? "Management counters" : `Fastpath worker ${group.split(".")[1]}`, fields));
        }
      }
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
      const content = !adapterVisaDetails.loaded ? adapterVisaDetails.pending ? `<p>Loading current visas...</p>` : "" : currentVisaList(current);
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
      detailField("Observed at", source?.last_lookup_ms ? window.ZPRSafeDisplay.formatDateTime(source.last_lookup_ms) : "Not reported"),
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
      detailField("Last lookup", source.last_lookup_ms ? window.ZPRSafeDisplay.formatDateTime(source.last_lookup_ms) : "Never observed"),
      detailField("Last success", source.last_success_ms ? window.ZPRSafeDisplay.formatDateTime(source.last_success_ms) : "Never observed"),
      detailField("Basis", source.health_note || "No lookup status from the admin API"),
    ]));
    if (source.editor_url) {
      sections.push(`<div class="inspector-actions"><a class="button button-refresh" href="${escapeHTML(source.editor_url)}" target="zpr-directory-manager" data-reuse-window="zpr-directory-manager" rel="noopener noreferrer">Open LDAP editor ↗</a></div>`);
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
    const world = component.closest("#graph-world");
    for (const connection of world?.graphConnections?.get(component.dataset.topologyComponent) || []) {
      connectGraphShapes(connection.lines, connection.from, connection.to, world);
    }
    if (animation.playState === "running") requestAnimationFrame(update);
  };
  update();
}

function animateGraphLink(group, start, duration = 700) {
  const lines = [...group.querySelectorAll("line.graph-link, line.graph-link-hit, line.graph-network-clearance")];
  const target = lines[0];
  if (!target) return;
  const to = group.graphConnection?.target || { x1: Number(target.getAttribute("x1")), y1: Number(target.getAttribute("y1")), x2: Number(target.getAttribute("x2")), y2: Number(target.getAttribute("y2")) };
  const from = { x1: start.x1, y1: start.y1, x2: start.x2, y2: start.y2 };
  if (Object.keys(to).every((key) => Math.abs(to[key] - from[key]) < 0.5)) return;
  group.graphLinkMotion = { from, to };
  const startedAt = performance.now();
  const update = (now) => {
    if (!group.isConnected) return;
    const progress = Math.min(1, (now - startedAt) / duration);
    const eased = 1 - (1 - progress) ** 3;
    if (group.graphConnection) {
      const connection = group.graphConnection;
      connectGraphShapes(connection.lines, connection.from, connection.to, group.closest("#graph-world"));
    } else {
      for (const line of lines) {
        for (const key of Object.keys(to)) line.setAttribute(key, String(from[key] + (to[key] - from[key]) * eased));
      }
    }
    if (progress < 1) requestAnimationFrame(update);
    else delete group.graphLinkMotion;
  };
  update(startedAt);
}

function setupGraphAppearance(stage) {
  stage.classList.toggle("graph-dark", graphDarkMode);
  stage.querySelector("[data-graph-dark-mode]").addEventListener("change", (event) => {
    graphDarkMode = event.currentTarget.checked;
    stage.classList.toggle("graph-dark", graphDarkMode);
  });
}

function connectGraphShapes(lines, fromShape, toShape, world) {
  const worldMatrix = world.getCTM();
  const shapeSpace = (shape) => shape.getCTM().inverse().multiply(worldMatrix);
  const center = (shape) => {
    const box = shape.getBBox();
    return new DOMPoint(box.x + box.width / 2, box.y + box.height / 2)
      .matrixTransform(worldMatrix.inverse().multiply(shape.getCTM()));
  };
  const from = center(fromShape), to = center(toShape);
  const boundary = (shape, start, end) => {
    const matrix = shapeSpace(shape);
    let inside = 0, outside = 1;
    // Intersect the ray with the actual SVG fill, including rounded corners and polygons.
    for (let step = 0; step < 32; step++) {
      const fraction = (inside + outside) / 2;
      const point = new DOMPoint(start.x + (end.x - start.x) * fraction, start.y + (end.y - start.y) * fraction);
      if (shape.isPointInFill(point.matrixTransform(matrix))) inside = fraction;
      else outside = fraction;
    }
    return { x: start.x + (end.x - start.x) * inside, y: start.y + (end.y - start.y) * inside };
  };
  const start = boundary(fromShape, from, to);
  const end = boundary(toShape, to, from);
  for (const line of lines) {
    const matrix = line.parentElement.getCTM().inverse().multiply(worldMatrix);
    const localStart = new DOMPoint(start.x, start.y).matrixTransform(matrix);
    const localEnd = new DOMPoint(end.x, end.y).matrixTransform(matrix);
    line.setAttribute("x1", String(localStart.x));
    line.setAttribute("y1", String(localStart.y));
    line.setAttribute("x2", String(localEnd.x));
    line.setAttribute("y2", String(localEnd.y));
  }
}

function mapComponentCounts(data) {
  const counts = new Map();
  const active = activeMapVisas(data);
  const addresses = new Map();
  if (active) for (const visa of active) {
    for (const address of new Set([visa.source_addr, visa.dest_addr].filter(Boolean).map(dnsAddressKey))) {
      addresses.set(address, (addresses.get(address) || 0) + 1);
    }
  }
  for (const actor of data.actors) {
    const value = actor.node ? actor.node_details?.buffered_denials : active && actor.zpr_addr ? addresses.get(dnsAddressKey(actor.zpr_addr)) || 0 : null;
    counts.set(`actor:${JSON.stringify(actor.cn)}`, value == null ? null : value);
  }
  for (const service of data.services || []) {
    counts.set(`service:${JSON.stringify([service.actor_cn, service.service_name])}`, active ? active.filter((visa) => serviceMatchesVisa(service, visa, data.actors)).length : null);
  }
  return counts;
}

function renderTopology(data, exitComponents = []) {
  const nodes = data.actors.filter((actor) => actor.node).sort((a, b) => a.cn.localeCompare(b.cn));
  const adapters = data.actors.filter((actor) => !actor.node).sort((a, b) => a.cn.localeCompare(b.cn));
  const actors = [...nodes, ...adapters];
  const displayNames = new Map(actors.map((actor) => [actor.cn, actorDisplayName(actor)]));
  const actorsByName = new Map(actors.map((actor) => [actor.cn, actor]));
  const actorsByAddress = new Map(actors.map((actor) => [actor.zpr_addr, actor]));
  const counts = mapComponentCounts(data);
  const visaCountBadge = (count, x, y, denial = false) => {
    const label = count == null ? "?" : count === 0 ? "" : String(count);
    const width = Math.max(22, label.length * 7 + 10);
    const description = count == null ? denial ? "Buffered denial count unavailable" : "Active visa count unavailable" : `${count} ${denial ? "buffered denials" : "active visas"}`;
    return `<g class="graph-visa-count${denial ? " graph-denial-count" : ""}${count === 0 ? " empty" : ""}"${denial ? ' role="button" tabindex="0" aria-description="Click, right-click or press Enter to show buffered denial details"' : ' role="button" tabindex="0" aria-description="Click, right-click or press Enter to show visas"'} aria-label="${description}"><title>${description}</title><rect x="${x - width / 2}" y="${y - 9}" width="${width}" height="18" rx="9"/>${label ? `<text x="${x}" y="${y + 3}">${label}</text>` : ""}</g>`;
  };
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
  const darkModeControl = `<label class="graph-auto-fit" hidden><input type="checkbox" data-graph-dark-mode${graphDarkMode ? " checked" : ""}>Dark mode</label>`;
  const geographic = byId("page-map").dataset.mapView === "geography";
  const mapView = geographic ? "geography" : "topology";
  const viewChanged = stage.dataset.renderedMapView !== mapView;
  if (viewChanged) {
    if (stage.dataset.renderedMapView) mapViewports.set(stage.dataset.renderedMapView, {
      camera: state.graphCamera ? { ...state.graphCamera } : null, autoFit: graphAutoFit,
      viewBox: stage.querySelector(".topology-graph")?.getAttribute("viewBox"),
    });
    const saved = mapViewports.get(mapView);
    state.graphCamera = saved?.camera ? { ...saved.camera } : null;
    graphAutoFit = saved?.autoFit ?? true;
    stage.dataset.renderedMapView = mapView;
  }
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
  // Hidden SVGs do not provide usable geometry for fitting or connector intersections.
  if (byId("page-map").hidden) return;

  if (!actors.length && !exitComponents.length) {
    if (geographic) {
      stage.innerHTML = `<div class="graph-controls" aria-label="Topology graph controls">${darkModeControl}<label class="graph-auto-fit"><input type="checkbox" data-graph-auto-fit${graphAutoFit ? " checked" : ""}>Auto-fit</label><button class="graph-control" data-graph-action="in" type="button" aria-label="Zoom in" title="Zoom in">+</button><button class="graph-control" data-graph-action="out" type="button" aria-label="Zoom out" title="Zoom out">−</button><button class="graph-control graph-fit" data-graph-action="fit" type="button" aria-label="Fit graph" title="Fit graph">Fit</button></div><svg class="topology-graph" viewBox="0 0 3600 1800" role="img" aria-label="World Map; no nodes or adapters reported"><g id="graph-world"><image href="/geography-land.svg?v=2" width="3600" height="1800" class="graph-geographic-basemap"/></g></svg><p class="empty-state" role="status">No nodes or adapters reported. The World Map is shown at its full extent.</p>`;
      setupGraphAppearance(stage);
      setupGraphControls(stage, 3600, 1800, null);
      applyMapVisaFocus(data);
      return;
    }
    const message = data.api_status === "connected"
      ? "No nodes or adapters reported. Visa Service returned an empty topology."
      : "Topology unavailable. Check the Visa Service connection and reported errors.";
    stage.innerHTML = `<div class="graph-controls" aria-label="Topology graph controls">${darkModeControl}</div><div class="empty-state" role="status">${message}</div>`;
    setupGraphAppearance(stage);
    applyMapVisaFocus(data);
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
    if (geographic) {
      const point = window.ZPRGeography.project(node.node_details?.latitude, node.node_details?.longitude);
      nodeCenter.x = point ? point.x * 2 : 4500 + (slot % nodeColumns) * nodeSpacing;
      nodeCenter.y = point ? point.y * 2 : margin + Math.floor(slot / nodeColumns) * nodeSpacing;
    }
    positions.set(node.cn, nodeCenter);
  });
  nodes.forEach((node) => {
    const nodeCenter = positions.get(node.cn);
    const attached = attachedByNode.get(node.cn) || [];
    if (!attached.length) return;
    let radius = clusterRadius(node);
    const extents = attached.map((actor) => actorExtent(actor) + 18);
    const directions = networkEdges.filter((edge) => edge.from.cn === node.cn || edge.to.cn === node.cn).map((edge) => {
      const other = positions.get(edge.from.cn === node.cn ? edge.to.cn : edge.from.cn);
      return other.x === nodeCenter.x && other.y === nodeCenter.y ? null : Math.atan2(other.y - nodeCenter.y, other.x - nodeCenter.x);
    }).filter((direction) => direction != null).sort((a, b) => a - b);
    if (geographic) {
      const containedFanout = geographicAdapterFanout(nodeCenter, attached, extents, radius, directions);
      if (containedFanout) {
        for (const [adapter, position] of containedFanout) positions.set(adapter.cn, position);
        return;
      }
    }
    let centerAngle = -Math.PI / 2;
    if (!directions.length) {
      attached.forEach((adapter, slot) => {
        const angle = centerAngle + slot * 2 * Math.PI / attached.length;
        positions.set(adapter.cn, { x: nodeCenter.x + Math.cos(angle) * radius, y: nodeCenter.y + Math.sin(angle) * radius });
      });
      return;
    }
    let openAngle = 0;
    directions.forEach((direction, index) => {
      const next = index + 1 < directions.length ? directions[index + 1] : directions[0] + 2 * Math.PI;
      if (next - direction > openAngle) {
        openAngle = next - direction;
        centerAngle = (direction + next) / 2;
      }
    });
    const availableAngle = Math.min(Math.PI, openAngle * 0.75);
    const offsetsAtRadius = () => {
      const offsets = [0];
      for (let slot = 1; slot < attached.length; slot++) {
        offsets.push(offsets.at(-1) + 2 * Math.asin(Math.min(1, (extents[slot - 1] + extents[slot] + 20) / (2 * radius))));
      }
      return offsets;
    };
    let offsets = offsetsAtRadius();
    const largestExtent = Math.max(...extents);
    // Grow the arc radius to keep service rings and gateway clouds inside a link-free sector.
    while (offsets.at(-1) + 2 * Math.asin(Math.min(1, largestExtent / radius)) > availableAngle) {
      radius *= 1.15;
      offsets = offsetsAtRadius();
    }
    const span = offsets.at(-1);
    attached.forEach((adapter, slot) => {
      const angle = centerAngle + offsets[slot] - span / 2;
      positions.set(adapter.cn, { x: nodeCenter.x + Math.cos(angle) * radius, y: nodeCenter.y + Math.sin(angle) * radius });
    });
  });
  const unconnectedHosts = adapters.filter((host) => !positions.has(host.cn));
  const unconnectedColumns = Math.max(1, Math.min(4, unconnectedHosts.length));
  const hostRingRadius = Math.max(64, ...unconnectedHosts.map((host) => actorExtent(host) + 18));
  unconnectedHosts.forEach((host, index) => {
    const column = index % unconnectedColumns;
    const row = Math.floor(index / unconnectedColumns);
    positions.set(host.cn, {
      x: (geographic ? 4500 : margin) + column * (hostRingRadius * 2 + 100),
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

  const measureTopologyBounds = () => {
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    const include = (left, top, right, bottom) => {
      minX = Math.min(minX, left);
      minY = Math.min(minY, top);
      maxX = Math.max(maxX, right);
      maxY = Math.max(maxY, bottom);
    };
    for (const actor of actors) {
      const position = positions.get(actor.cn);
      if (!position) continue;
      const radius = actorRadius(actor);
      const labelExtent = Math.min((displayNames.get(actor.cn) || actor.cn).length, 20) * 3.5;
      const extentX = Math.max(radius, labelExtent);
      include(position.x - extentX, position.y - radius, position.x + extentX, position.y + radius);
      if (gatewayActors.has(actor.cn)) {
        const parent = edges.find((edge) => edge.kind === "dock" && edge.to.cn === actor.cn)?.from;
        const parentPosition = parent && positions.get(parent.cn);
        const angle = parentPosition ? Math.atan2(position.y - parentPosition.y, position.x - parentPosition.x) : 0;
        const cloudDistance = serviceRingRadius(actor.cn) + 110;
        const cloudX = position.x + Math.cos(angle) * cloudDistance;
        const cloudY = position.y + Math.sin(angle) * cloudDistance;
        include(cloudX - 68, cloudY - 26, cloudX + 68, cloudY + 26);
      }
    }
    for (const service of services) {
      const position = servicePositions.get(service);
      if (!position) continue;
      const halfWidth = badgeWidthForService(service) / 2 + 3;
      include(position.x - halfWidth, position.y - 13, position.x + halfWidth, position.y + 13);
    }
    return { minX, minY, maxX, maxY, width: maxX - minX, height: maxY - minY };
  };

  const bounds = measureTopologyBounds();
  const paddingX = bounds.width * (5 / 90);
  const paddingY = bounds.height * (5 / 90);
  const offsetX = geographic ? 0 : paddingX - bounds.minX;
  const offsetY = geographic ? 0 : paddingY - bounds.minY;
  for (const position of positions.values()) {
    position.x += offsetX;
    position.y += offsetY;
  }
  for (const position of servicePositions.values()) {
    position.x += offsetX;
    position.y += offsetY;
  }

  const oldGraph = viewChanged ? null : stage.querySelector(".topology-graph");
  const oldViewBox = oldGraph?.viewBox.baseVal;
  const previousViewport = oldGraph ? {
    viewBox: oldGraph.getAttribute("viewBox"),
    x: oldViewBox.x,
    y: oldViewBox.y,
    width: oldViewBox.width,
    height: oldViewBox.height,
    clientWidth: oldGraph.clientWidth,
    clientHeight: oldGraph.clientHeight,
  } : null;
  let width = Math.max(1, bounds.width + paddingX * 2);
  let height = Math.max(1, bounds.height + paddingY * 2);
  const query = byId("topology-search").value.trim().toLowerCase();
  const legendKind = state.mapLegendKind || "";
  // Search and legend selections only highlight matches; unmatched components keep full emphasis.
  const mapMark = (queryMatch, legendMatch) => (query || legendKind) && (!query || queryMatch) && (!legendKind || legendMatch) ? "highlighted" : "";
  const matches = (actor) => !query || `${displayNames.get(actor.cn)} ${actor.cn} ${actor.zpr_addr || ""} ${visaServices.has(actor.cn) ? "visa service" : ""} ${(servicesByActor.get(actor.cn) || []).map((service) => `${service.service_name} ${service.service_kind} ${service.external_network_connection || ""}`).join(" ")}`.toLowerCase().includes(query);

  const edgeMarkup = [...dockEdges, ...networkEdges].map((edge) => {
    const from = positions.get(edge.from.cn), to = positions.get(edge.to.cn);
    const docked = edge.kind === "dock";
    const up = docked || edge.state === "UP";
    const cls = docked ? "dock-link" : up ? "up" : "down";
    const title = docked
      ? `${displayNames.get(edge.to.cn)} docked to ${displayNames.get(edge.from.cn)} · ${dnsAddressTitle(edge.to.zpr_addr, "Adapter IP")} · ${dnsAddressTitle(edge.from.zpr_addr, "Node IP")}`
      : `${displayNames.get(edge.from.cn)} to ${displayNames.get(edge.to.cn)}: ${edge.state} · ${dnsAddressTitle(edge.from.zpr_addr, "Node A IP")} · ${dnsAddressTitle(edge.to.zpr_addr, "Node B IP")}`;
    const highlighted = mapMark(matches(edge.from) || matches(edge.to), legendKind === (docked ? "dock" : "inter-node"));
    const key = `${edge.kind}|${edge.from.cn}|${edge.to.cn}`;
    return `<g class="graph-edge ${highlighted}" data-topology-edge="${escapeHTML(key)}" data-connector-from="${escapeHTML(`actor:${JSON.stringify(edge.from.cn)}`)}" data-connector-to="${escapeHTML(`actor:${JSON.stringify(edge.to.cn)}`)}" data-inspect-link="${escapeHTML(key)}"${docked ? ` data-dock-adapter="${escapeHTML(edge.to.cn)}"` : ""} tabindex="0" role="button" aria-label="Inspect ${escapeHTML(title)}"><title>${escapeHTML(title)}</title>${docked ? "" : `<line class="graph-network-clearance" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"/>`}<line class="graph-link ${cls}" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"/><line class="graph-link-hit" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"/></g>`;
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
  const previousMovement = new Map();
  const previousPositions = new Map();
  for (const component of stage.querySelectorAll("#graph-world > [data-topology-component]")) {
    const movement = graphMotionAt(component);
    const key = component.dataset.topologyComponent;
    previousMovement.set(key, movement);
    previousPositions.set(key, {
      x: Number(component.dataset.originX) + movement.x,
      y: Number(component.dataset.originY) + movement.y,
      scale: movement.scale,
    });
  }
  const previousEdgePositions = new Map([...stage.querySelectorAll("#graph-world [data-topology-edge]")].map((group) => {
    const line = group.querySelector("line.graph-link");
    return [group.dataset.topologyEdge, line && { x1: Number(line.getAttribute("x1")), y1: Number(line.getAttribute("y1")), x2: Number(line.getAttribute("x2")), y2: Number(line.getAttribute("y2")) }];
  }).filter(([, position]) => position));
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
      const cloudLabel = externalNetworks.length > 15 ? `${externalNetworks.slice(0, 14)}…` : externalNetworks;
      cloudMarkup = `<g class="graph-external-network" role="img" aria-label="Gateway connection to ${escapeHTML(externalNetworks)}"><title>Gateway connection to ${escapeHTML(externalNetworks)}</title><line class="graph-link gateway-cloud-link" x1="${pos.x}" y1="${pos.y}" x2="${cloudX}" y2="${cloudY}"/><path class="graph-cloud" transform="translate(${cloudX} ${cloudY})" d="M -34 22 C -60 22 -62 -12 -39 -17 C -40 -43 -2 -49 9 -28 C 31 -42 52 -22 46 -5 C 68 0 62 22 42 22 Z"/><text class="graph-cloud-label" x="${cloudX}" y="${cloudY + 8}" text-anchor="middle">${escapeHTML(cloudLabel)}</text></g>`;
    }
    const vertexKind = isNode ? "node" : isGateway ? "gateway" : isVisaService ? "visa" : "adapter";
    const highlighted = mapMark(matches(actor), legendKind === vertexKind);
    const countAnchor = isNode ? { x: 44, y: -23 }
      : isGateway ? { x: 28, y: -16 }
      : isVisaService ? { x: 17.5, y: -17.5 }
      : { x: 29 / Math.SQRT2, y: -29 / Math.SQRT2 };
    const count = visaCountBadge(counts.get(componentKey), pos.x + countAnchor.x, pos.y + countAnchor.y, isNode);
    return `<g class="graph-vertex ${isNode ? "node" : isGateway ? "gateway" : isVisaService ? "visa" : "adapter"}${arrivingClass} ${highlighted}" data-topology-component="${escapeHTML(componentKey)}" data-origin-x="${pos.x}" data-origin-y="${pos.y}"${positionAttributes} data-inspect-actor="${escapeHTML(actor.cn)}" tabindex="0" role="button" aria-label="Inspect ${escapeHTML(isGateway ? "gateway " : "")}${escapeHTML(displayName)}"><title>${escapeHTML(isGateway ? "ZPR gateway · " : "")}${escapeHTML(displayName)} · ${escapeHTML(actor.cn)} · ${escapeHTML(dnsAddressTitle(actor.zpr_addr))}</title>${cloudMarkup}${glyph}${marker}<text class="graph-label" x="${pos.x}" y="${pos.y + 3}">${escapeHTML(shortName)}</text>${count}</g>`;
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
    const type = serviceTypeAppearance(isGatewayService(service) ? "Gateway" : service.service_kind);
    const trustedType = (service.service_kind || "").match(/^Trusted\("([^\"]+)"\)$/)?.[1];
    const trustedClass = trustedType ? ` trusted-${trustedType.toLowerCase().replace(/[^a-z0-9]+/g, "-")}` : "";
    const gatewayClass = isGatewayService(service) ? " gateway" : "";
    const serviceMatches = `${service.service_name} ${service.service_kind} ${service.external_network_connection || ""}`.toLowerCase().includes(query);
    const badgeKind = trustedClass ? trustedClass.trim() : gatewayClass ? "gateway" : "service";
    const highlighted = mapMark(serviceMatches || matches(owner), legendKind === badgeKind);
    const edgeHighlighted = mapMark(serviceMatches || matches(owner), legendKind === "registration");
    const serviceEdgeKey = `service:${JSON.stringify([service.actor_cn, service.service_name])}`;
    serviceEdgeMarkup.push(`<g class="graph-service-edge ${edgeHighlighted}" data-topology-edge="${escapeHTML(serviceEdgeKey)}" data-connector-from="${escapeHTML(`actor:${JSON.stringify(service.actor_cn)}`)}" data-connector-to="${escapeHTML(serviceEdgeKey)}" aria-hidden="true"><line class="graph-link service-link" x1="${ownerPosition.x}" y1="${ownerPosition.y}" x2="${position.x}" y2="${position.y}"/></g>`);
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
    const count = visaCountBadge(counts.get(componentKey), position.x + badgeWidth / 2 - 2, position.y - 8);
    return `<g class="graph-service-badge${gatewayClass}${trustedClass}${arrivingClass} ${highlighted}" data-service-type="${escapeHTML(type.key)}" fill="${escapeHTML(type.background)}" stroke="${escapeHTML(type.border)}" color="${escapeHTML(type.color)}" data-topology-component="${escapeHTML(componentKey)}" data-origin-x="${position.x}" data-origin-y="${position.y}"${positionAttributes} data-service-actor="${escapeHTML(service.actor_cn)}" data-inspect-service="${escapeHTML(service.service_name)}" tabindex="0" role="button" aria-label="${escapeHTML(labelForScreenReader)}"><title>${escapeHTML(title)}</title><rect x="${position.x - badgeWidth / 2}" y="${position.y - 10}" width="${badgeWidth}" height="20" rx="4"/>${marker}<text x="${position.x}" y="${position.y + 3}">${escapeHTML(shortLabel)}</text>${count}</g>`;
  });

  const exiting = (kind) => exitComponents.filter((component) => component.kind === kind).map((component) => component.markup).join("");
  stage.innerHTML = `<div class="graph-controls" aria-label="Topology graph controls">${darkModeControl}<label class="graph-auto-fit"><input type="checkbox" data-graph-auto-fit${graphAutoFit ? " checked" : ""}>Auto-fit</label><button class="graph-control" data-graph-action="in" type="button" aria-label="Zoom in" title="Zoom in">+</button><button class="graph-control" data-graph-action="out" type="button" aria-label="Zoom out" title="Zoom out">−</button><button class="graph-control graph-fit" data-graph-action="fit" type="button" aria-label="Fit graph" title="Fit graph">Fit</button></div><svg class="topology-graph" viewBox="0 0 ${width} ${height}" role="img" aria-label="Topology graph with ${nodes.length} nodes, ${adapters.length} adapters, ${dockEdges.length} dock connections, ${networkEdges.length} inter-node links, and ${(data.services || []).filter((service) => servicesByActor.has(service.actor_cn)).length} registered services"><g id="graph-world">${edgeMarkup}${serviceEdgeMarkup.join("")}${serviceMarkup.join("")}<g class="graph-exit-layer" aria-hidden="true">${exiting("service")}</g>${vertexMarkup.slice(nodes.length).join("")}<g class="graph-exit-layer" aria-hidden="true">${exiting("adapter")}</g>${vertexMarkup.slice(0, nodes.length).join("")}<g class="graph-exit-layer" aria-hidden="true">${exiting("node")}</g></g></svg>`;
  setupGraphAppearance(stage);
  const world = stage.querySelector("#graph-world");
  if (geographic) {
    const basemap = document.createElementNS("http://www.w3.org/2000/svg", "image");
    basemap.setAttribute("href", "/geography-land.svg?v=2");
    basemap.setAttribute("width", "3600");
    basemap.setAttribute("height", "1800");
    basemap.setAttribute("class", "graph-geographic-basemap");
    world.prepend(basemap);
    stage.querySelector(".topology-graph").setAttribute("aria-label", `World Map with ${nodes.length} nodes and ${adapters.length} adapters; nodes without coordinates are outside the basemap`);
  }
  const shapes = new Map([...world.querySelectorAll(":scope > [data-topology-component]")].map((component) => [
    component.dataset.topologyComponent,
    component.querySelector(":scope > .graph-node, :scope > .graph-adapter, :scope > .graph-gateway, :scope > .graph-visa, :scope > rect"),
  ]));
  world.graphConnections = new Map();
  const registerConnection = (keys, lines, from, to) => {
    const connection = { lines, from, to };
    connectGraphShapes(lines, from, to, world);
    const line = lines[0];
    connection.target = Object.fromEntries(["x1", "y1", "x2", "y2"].map((key) => [key, Number(line.getAttribute(key))]));
    line.parentElement.graphConnection = connection;
    for (const key of new Set(keys)) {
      const connections = world.graphConnections.get(key) || [];
      connections.push(connection);
      world.graphConnections.set(key, connections);
    }
  };
  for (const edge of world.querySelectorAll("[data-connector-from]")) {
    registerConnection([edge.dataset.connectorFrom, edge.dataset.connectorTo], edge.querySelectorAll("line"), shapes.get(edge.dataset.connectorFrom), shapes.get(edge.dataset.connectorTo));
  }
  for (const cloud of world.querySelectorAll(".graph-external-network")) {
    const owner = cloud.closest("[data-topology-component]");
    registerConnection([owner.dataset.topologyComponent], cloud.querySelectorAll("line"), shapes.get(owner.dataset.topologyComponent), cloud.querySelector(".graph-cloud"));
  }

  const graph = stage.querySelector(".topology-graph");
  const renderedBounds = graphContentBounds(world);
  if (renderedBounds) {
    const frame = graphViewBox(renderedBounds);
    width = frame.width;
    height = frame.height;
    graph.setAttribute("viewBox", `${frame.x} ${frame.y} ${width} ${height}`);
  }
  if (!graphAutoFit && previousViewport) {
    width = previousViewport.width;
    height = previousViewport.height;
    stage.querySelector(".topology-graph").setAttribute("viewBox", previousViewport.viewBox);
  }
  if (viewChanged && !graphAutoFit && mapViewports.get(mapView)?.viewBox) {
    const savedBox = mapViewports.get(mapView).viewBox;
    stage.querySelector(".topology-graph").setAttribute("viewBox", savedBox);
    const values = savedBox.split(/\s+/).map(Number);
    width = values[2];
    height = values[3];
  }

  if (state.graphAnimations && !viewChanged) {
    for (const component of stage.querySelectorAll("#graph-world > [data-topology-component]")) {
      const previous = previousPositions.get(component.dataset.topologyComponent);
      if (!previous) continue;
      const originX = Number(component.dataset.originX);
      const originY = Number(component.dataset.originY);
      const from = { x: previous.x - originX, y: previous.y - originY, scale: previous.scale };
      if (Math.abs(from.x) < 0.5 && Math.abs(from.y) < 0.5 && Math.abs(from.scale - 1) < 0.01) continue;
      const animation = component.animate([{ opacity: 1 }, { opacity: 1 }], { duration: 700, easing: "ease-in-out", fill: "both" });
      animateGraphMotion(component, animation, from, { x: 0, y: 0, scale: 1 });
    }
    for (const group of stage.querySelectorAll("#graph-world [data-topology-edge]")) {
      const previous = previousEdgePositions.get(group.dataset.topologyEdge);
      if (previous) animateGraphLink(group, previous);
    }
  }

  setupGraphControls(stage, width, height, previousViewport);
  const focusStatus = document.createElement("p");
  focusStatus.className = "graph-visa-focus-status";
  focusStatus.setAttribute("role", "status");
  focusStatus.hidden = true;
  stage.append(focusStatus);
  applyMapVisaFocus(data);
  pulseAdapterDecisions(data);
  pulseMapCounts(counts);
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
  const viewBox = svg.viewBox.baseVal;
  const apply = () => world.setAttribute("transform", `translate(${camera.x} ${camera.y}) scale(${camera.scale})`);
  if (previousViewport?.clientWidth > 0 && previousViewport?.clientHeight > 0 && svg.clientWidth > 0 && svg.clientHeight > 0) {
    const previousScale = Math.min(previousViewport.clientWidth / previousViewport.width, previousViewport.clientHeight / previousViewport.height);
    const nextScale = Math.min(svg.clientWidth / width, svg.clientHeight / height);
    const previousOffsetX = (previousViewport.clientWidth - previousScale * previousViewport.width) / 2;
    const previousOffsetY = (previousViewport.clientHeight - previousScale * previousViewport.height) / 2;
    const nextOffsetX = (svg.clientWidth - nextScale * width) / 2;
    const nextOffsetY = (svg.clientHeight - nextScale * height) / 2;
    const viewportMoved = Math.abs(previousOffsetX - nextOffsetX) > 0.5 || Math.abs(previousOffsetY - nextOffsetY) > 0.5;
    if (previousScale > 0 && nextScale > 0 && (Math.abs(previousScale - nextScale) > 0.0001 || viewportMoved)) {
      const ratio = previousScale / nextScale;
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
  const cancelViewportAnimation = () => {
    svg.getAnimations().filter((animation) => animation.effect?.target === svg).forEach((animation) => animation.cancel());
    svg.style.transform = "none";
    svg.style.transformOrigin = "";
  };
  const beginManualNavigation = () => {
    graphAutoFit = false;
    stage.querySelector("[data-graph-auto-fit]").checked = false;
    cancelViewportAnimation();
  };
  const zoomAt = (nextScale, x = viewBox.x + width / 2, y = viewBox.y + height / 2) => {
    beginManualNavigation();
    const scale = Math.max(0.05, Math.min(maxZoom, nextScale));
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
  const fitBounds = () => {
    return graphContentBounds(world) || world.getBBox();
  };
  const fit = (cancelAnimation = true) => {
    if (cancelAnimation) cancelViewportAnimation();
    const bounds = fitBounds();
    camera.scale = Math.min((width * 0.9) / Math.max(1, bounds.width), (height * 0.9) / Math.max(1, bounds.height));
    camera.x = viewBox.x + width / 2 - (bounds.x + bounds.width / 2) * camera.scale;
    camera.y = viewBox.y + height / 2 - (bounds.y + bounds.height / 2) * camera.scale;
    apply();
  };
  stage.querySelector("[data-graph-auto-fit]").addEventListener("change", (event) => {
    graphAutoFit = event.currentTarget.checked;
    if (graphAutoFit) fit();
  });

  stage.querySelectorAll("[data-graph-action]").forEach((button) => button.addEventListener("click", () => {
    const action = button.dataset.graphAction;
    if (action === "in") zoomAt(camera.scale * 1.25);
    if (action === "out") zoomAt(camera.scale / 1.25);
    if (action === "fit") fit();
  }));
  svg.addEventListener("wheel", (event) => {
    event.preventDefault();
    if (event.deltaY === 0) return;
    cancelViewportAnimation();
    const point = pointAt(event);
    zoomAt(camera.scale * (event.deltaY < 0 ? 1.12 : 1 / 1.12), point.x, point.y);
  }, { passive: false });

  let drag = null;
  svg.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    if (event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-link]")) return;
    const point = pointAt(event);
    drag = { pointerId: event.pointerId, point, x: camera.x, y: camera.y };
    svg.classList.add("panning");
    svg.setPointerCapture(event.pointerId);
  });
  svg.addEventListener("pointermove", (event) => {
    if (!drag || drag.pointerId !== event.pointerId) return;
    const point = pointAt(event);
    const x = drag.x + point.x - drag.point.x;
    const y = drag.y + point.y - drag.point.y;
    if (x === camera.x && y === camera.y) return;
    beginManualNavigation();
    camera.x = x;
    camera.y = y;
    apply();
  });
  const endDrag = (event) => {
    if (!drag || drag.pointerId !== event.pointerId) return;
    drag = null;
    svg.classList.remove("panning");
  };
  svg.addEventListener("pointerup", endDrag);
  svg.addEventListener("pointercancel", endDrag);
  if (graphAutoFit) fit(false);
  else apply();
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
    const editorAction = source.editor_url ? `<a class="source-editor-link" href="${escapeHTML(source.editor_url)}" target="zpr-directory-manager" data-reuse-window="zpr-directory-manager" rel="noopener noreferrer" title="Manage ${escapeHTML(source.name)} in LDAP editor"><span>Manage</span><span aria-hidden="true">↗</span></a>` : "";
    return `<tr class="selectable-row" data-inspect-source="${escapeHTML(source.name)}" tabindex="0" role="button" aria-label="Inspect trusted source ${escapeHTML(source.name)}"><td class="source-primary">${sourceName}</td><td>${escapeHTML(providerDescription(source.provider))}</td><td>${escapeHTML(actorLabel)}</td><td><span class="health-badge ${["working", "failed"].includes(source.health) ? source.health : ""}">${escapeHTML(statusText)}</span></td><td><span class="source-time">${source.last_lookup_ms ? escapeHTML(window.ZPRSafeDisplay.formatDateTime(source.last_lookup_ms)) : "No lookup recorded"}</span>${source.last_success_ms ? `<small class="source-secondary">Last success ${escapeHTML(window.ZPRSafeDisplay.formatDateTime(source.last_success_ms))}</small>` : ""}${editorAction ? `<small class="source-secondary">${editorAction}</small>` : ""}</td></tr>`;
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
    const response = await window.zprOperatorFetch("/api/policy/context", { cache: "no-store" });
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

function graphContentBounds(world) {
  const boxes = [...world.children]
    .filter((element) => !element.classList.contains("graph-geographic-basemap") && !element.classList.contains("graph-exit-layer"))
    .map((element) => element.getBBox())
    .filter((box) => box.width > 0 || box.height > 0);
  if (!boxes.length) return null;
  const left = Math.min(...boxes.map((box) => box.x));
  const top = Math.min(...boxes.map((box) => box.y));
  const right = Math.max(...boxes.map((box) => box.x + box.width));
  const bottom = Math.max(...boxes.map((box) => box.y + box.height));
  return { x: left, y: top, width: Math.max(1, right - left), height: Math.max(1, bottom - top) };
}

function graphViewBox(bounds) {
  const centerX = bounds.x + bounds.width / 2;
  const centerY = bounds.y + bounds.height / 2;
  const paddingX = bounds.width * (5 / 90);
  const paddingY = bounds.height * (5 / 90);
  const width = bounds.width + paddingX * 2;
  const height = bounds.height + paddingY * 2;
  return { x: centerX - width / 2, y: centerY - height / 2, width, height };
}

function geographicAdapterFanout(nodeCenter, adapters, extents, initialRadius, directions) {
  const bounds = { left: 0, top: 0, right: 3600, bottom: 1800 };
  const inwardAngle = Math.atan2(900 - nodeCenter.y, 1800 - nodeCenter.x);
  const positionsAt = (centerAngle, radius, offsets) => {
    const span = offsets.at(-1);
    return adapters.map((adapter, index) => {
      const angle = centerAngle + offsets[index] - span / 2;
      return { adapter, extent: extents[index], x: nodeCenter.x + Math.cos(angle) * radius, y: nodeCenter.y + Math.sin(angle) * radius };
    });
  };
  const marginFor = (positions) => {
    let margin = Infinity;
    for (const position of positions) {
      const left = position.x - position.extent;
      const right = position.x + position.extent;
      const top = position.y - position.extent;
      const bottom = position.y + position.extent;
      if (left < bounds.left || right > bounds.right || top < bounds.top || bottom > bounds.bottom) return null;
      margin = Math.min(margin, left - bounds.left, bounds.right - right, top - bounds.top, bounds.bottom - bottom);
    }
    return margin;
  };
  if (!directions.length) {
    const ring = adapters.map((adapter, index) => {
      const angle = -Math.PI / 2 + (2 * Math.PI * index) / adapters.length;
      return { adapter, extent: extents[index], x: nodeCenter.x + Math.cos(angle) * initialRadius, y: nodeCenter.y + Math.sin(angle) * initialRadius };
    });
    if (marginFor(ring) != null) return ring.map(({ adapter, x, y }) => [adapter, { x, y }]);
  }

  const sectors = [];
  if (directions.length) {
    directions.forEach((start, index) => {
      const end = index + 1 < directions.length ? directions[index + 1] : directions[0] + 2 * Math.PI;
      sectors.push({ start, end, width: end - start });
    });
  } else {
    sectors.push({ start: inwardAngle - Math.PI / 2, end: inwardAngle + Math.PI / 2, width: Math.PI });
  }
  let best = null;
  let bestScore = -Infinity;
  for (const sector of sectors) {
    const availableAngle = Math.min(Math.PI, sector.width * 0.75);
    const inset = (sector.width - availableAngle) / 2;
    let radius = initialRadius;
    for (let attempt = 0; attempt < 48; attempt++) {
      let offsets = [0];
      for (let index = 1; index < adapters.length; index++) {
        offsets.push(offsets.at(-1) + 2 * Math.asin(Math.min(1, (extents[index - 1] + extents[index] + 20) / (2 * radius))));
      }
      const largestExtent = Math.max(...extents);
      const arcWidth = offsets.at(-1) + 2 * Math.asin(Math.min(1, largestExtent / radius));
      if (arcWidth > availableAngle) {
        radius *= 1.15;
        continue;
      }
      const centerMin = sector.start + inset + arcWidth / 2;
      const centerMax = sector.end - inset - arcWidth / 2;
      if (centerMin > centerMax) {
        radius *= 1.15;
        continue;
      }
      const angleCandidates = [centerMin, centerMax, (centerMin + centerMax) / 2];
      let nearestInward = null;
      let nearestDistance = Infinity;
      for (let turn = -2; turn <= 2; turn++) {
        const angle = inwardAngle + turn * 2 * Math.PI;
        const clamped = Math.max(centerMin, Math.min(centerMax, angle));
        const distance = Math.abs(angle - clamped);
        if (distance < nearestDistance) {
          nearestDistance = distance;
          nearestInward = clamped;
        }
      }
      angleCandidates.push(nearestInward);
      let foundContainedPosition = false;
      for (const centerAngle of new Set(angleCandidates)) {
        const candidate = positionsAt(centerAngle, radius, offsets);
        const margin = marginFor(candidate);
        if (margin == null) continue;
        foundContainedPosition = true;
        const preferredCenter = (sector.start + sector.end) / 2;
        const score = sector.width * 1_000_000 - Math.abs(centerAngle - preferredCenter) * 1_000 + margin;
        if (score > bestScore) {
          best = candidate;
          bestScore = score;
        }
      }
      if (foundContainedPosition) break;
      radius *= 1.15;
    }
  }
  return best?.map(({ adapter, x, y }) => [adapter, { x, y }]) || null;
}

async function loadPolicyWorkspace() {
  const policy = state.policy;
  if (policy.loaded) return;
  const selectedRecordID = policy.record?.id;
  const editorSource = byId("policy-source").value;
  const hasUnsavedChanges = hasUnsavedPolicyChanges(policy, editorSource);
  setPolicyLoadStatus();
  try {
    const data = await policyEditorRequest("/api/policy", { headers: { Accept: "application/json" } });
    const organizationChanged = policy.organizationID && data.organization_id && policy.organizationID !== data.organization_id;
    if (organizationChanged && policy.testPending) stopPolicyTest();
    if (organizationChanged) invalidatePolicyAnalysis();
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
      policy.evaluatedSource = null;
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
    policyAssistant.setStatus(data);
    updateAssistantControls();
  } catch (error) {
    policy.loaded = false;
    setPolicyLoadStatus(error.message);
  }
}

const policyAssistant = window.ZPRAssistant.mount({
  pane: byId("policy-assistant-pane"), prefix: "assistant",
  getTarget: policyAssistantTarget,
  getContext: () => ({
    source: policyAssistantTarget().value, id: state.policy.record?.id,
    kind: state.policy.record?.kind, revision: state.policy.revision,
    browsing: state.policy.browsingRevision, organization: state.policy.organizationID,
  }),
  editable: () => state.policy.configured && ["policy", "assertions"].includes(state.policy.record?.kind) && !policyAssistantTarget().disabled && !policyAssistantTarget().readOnly,
  disclosure: "Submitting sends the current draft and chat history to Anthropic. Suggestions are unsaved edits; use Insert to apply and Ctrl/Command+Z to undo.",
  request: (conversation) => window.ZPRAssistant.jsonRequest("/api/policy/assistant", {
    method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ ...conversation, editor: state.policy.record?.kind === "assertions" ? "assertion" : "policy", source: policyAssistantTarget().value }),
  }),
  watch(invalidate, reset) {
    for (const id of ["policy-source", "assertion-source"]) {
      byId(id).addEventListener("input", invalidate);
      new MutationObserver(() => policyAssistant.render()).observe(byId(id), { attributes: true, attributeFilter: ["disabled", "readonly"] });
    }
    document.addEventListener("zpr:policy-record-kind", reset);
  },
});

function updateAssistantControls() { policyAssistant.render(); }

function policyAssistantTarget() {
  return byId(state.policy.record?.kind === "assertions" ? "assertion-source" : "policy-source");
}

function renderPolicyAttributes() {
  const rescan = byId("policy-attribute-rescan");
  const status = byId("policy-attribute-status");
  rescan.disabled = !state.policy.configured || Boolean(state.policy.attributeScanPending);
  rescan.classList.toggle("button-save-as-ready", !rescan.disabled);
  if (state.policy.attributeScanPending) {
    status.hidden = false;
    status.textContent = "Refreshing trusted attributes…";
    status.dataset.state = "pending";
  } else if (state.policy.attributeScanError) {
    status.hidden = false;
    status.textContent = state.policy.attributeScanError;
    status.dataset.state = "error";
  } else {
    status.hidden = true;
    status.textContent = "";
    status.dataset.state = "";
  }
}

async function rescanPolicyAttributes() {
  const policy = state.policy;
  policy.attributeScanPending = true;
  renderPolicyAttributes();
  try {
    const response = await window.zprOperatorFetch("/api/policy/attributes/rescan", { method: "POST", headers: { Accept: "application/json" } });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || "Unable to refresh trusted attributes.");
    policy.attributes = data.attributes || [];
    policy.ldapAttributeCount = data.ldap_attribute_count || 0;
    policy.attributeScanError = data.error || "";
  } catch (error) {
    policy.attributeScanError = error.message || "Unable to refresh trusted attributes.";
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
        button.append(marker, name);
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
          const icon = window.zprPolicyRecordIcon(record.kind);
          const label = document.createElement("strong"); label.textContent = record.name;
          if (icon) item.append(icon);
          item.append(label);
          if (record.kind === "assertions") {
            const revision = document.createElement("small");
            revision.textContent = `Assertions · r${record.current_revision || 0}`;
            item.append(revision);
          }
          group.append(item);
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
  byId("policy-rename").hidden = !recordContext;
  byId("policy-rename").disabled = !saved || archived || protectedRecord || Boolean(policy.browsingRevision) || hasUnsavedPolicyChanges(policy);
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
    const pickerClosed = byId("policy-catalog-pane").hidden;
    (pickerClosed ? byId("policy-picker-toggle") : origin || byId("policy-category-tree")).focus();
  }
  renderPolicyFileActions();
}

async function openPolicyPickerMenu(target, x, y) {
  closePolicyPickerMenu();
  const policy = state.policy;
  const record = target.closest("[data-record-id]");
  const category = target.closest("[data-category-id]");
  if (record && record.dataset.recordId !== policy.record?.id) {
    await selectPolicyRecord(record.dataset.recordId, true, false, false);
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
  setPolicyLoadStatus();
  const policy = state.policy;
  invalidatePolicyAnalysis();
  setPolicyRecordSurface("");
  policy.record = null; policy.source = ""; policy.savedSource = ""; policy.revision = 0; policy.revisions = []; policy.evaluatedSource = null; policy.validSource = null;
  policy.saveTestSource = ""; policy.saveTestError = ""; policy.saveAsTestSource = ""; policy.saveAsTestError = "";
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
  byId("policy-check").disabled = true;
  byId("policy-check").textContent = "Analyze";
  state.policy.testResult = null;
  state.policy.testSource = "";
  state.policy.checkDiagnostics = "";
  byId("policy-test-gutter-content").replaceChildren();
  byId("policy-test-gutter").hidden = false;
  byId("policy-test-status").hidden = true;
  hidePolicyTestDetails();
  policyEditorController.setAnalysisState();
  byId("policy-check-result").textContent = "Policy source is not loaded.";
  byId("policy-check-result").hidden = true;
  renderPolicyHistory();
  byId("assistant-question").disabled = true; byId("assistant-send").disabled = true;
  renderPolicyCatalog();
}

function selectPolicyRecord(...args) {
  return policyEditorController.perform("load", ...args);
}

async function loadPolicyRecord(id, fetchRecord = true, discardEdits = false, closePicker = true) {
  const policy = state.policy;
  const pickerWasOpen = !byId("policy-catalog-pane").hidden;
  if (policy.record?.id === id) {
    if (pickerWasOpen && closePicker) setPolicyPickerOpen(false, true);
    return;
  }
  const summary = policy.records.find((record) => record.id === id);
  if (!summary) return clearPolicySelection();
  if (!discardEdits && !window.ZPREditorPage.confirmDiscard(hasUnsavedPolicyChanges(policy), "Discard unsaved changes or leave this historical version?")) return;
  if (policy.testPending) stopPolicyTest();
  invalidatePolicyAnalysis();
  policy.lintWarnings = [];
  policy.evaluatedSource = null;
  clearPolicyTestResults();
  policy.checkDiagnostics = "";
  policy.errorOffsets = [];
  byId("policy-check-result").hidden = true;
  setPolicyLoadStatus();
  try {
    let record = summary;
    if (fetchRecord) {
      record = await policyEditorRequest(`/api/policy/records/${encodeURIComponent(id)}`);
    }
    policy.record = record;
    policy.categoryID = record.kind === "assertions" ? organizationPolicyCategoryID(policy) || record.category_id : record.category_id;
    policy.source = record.content || ""; policy.savedSource = policy.source; policy.revision = record.current_revision; policy.evaluatedSource = null; policy.validSource = null;
    policy.checkDiagnostics = "";
    policy.saveTestSource = ""; policy.saveTestError = ""; policy.saveAsTestSource = ""; policy.saveAsTestError = "";
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
    byId("policy-check-result").hidden = true;
    byId("policy-check-result").dataset.state = "";
    updateAssistantControls();
    policyAssistant.reset();
    renderPolicyCatalog(); await loadPolicyHistory(record.id);
    updatePolicyDirtyState();
    if (pickerWasOpen && closePicker) setPolicyPickerOpen(false, true);
  } catch (error) {
    setPolicyLoadStatus(error.message);
  }
}

function setPolicyRecordSurface(kind, record = state.policy.record) {
  const assertionsSelected = kind === "assertions";
  const policy = state.policy;
  if (assertionsSelected && policy.testPending) stopPolicyTest();
  if (assertionsSelected) setPolicyFileMenuOpen(false);
  byId("policy-assertion-editor").hidden = !assertionsSelected;
  byId("policy-source-surface").hidden = assertionsSelected;
  byId("policy-attribute-toolbar").hidden = assertionsSelected;
  byId("policy-actions").hidden = false;
  for (const id of ["policy-save", "policy-save-as", "policy-stage", "policy-refresh"]) {
    byId(id).hidden = assertionsSelected;
  }
  for (const id of ["assertion-save", "assertion-reload"]) {
    byId(id).hidden = !assertionsSelected;
  }
  const assertionActions = document.querySelector(".assertion-actions");
  assertionActions.hidden = !assertionsSelected;
  const editorTools = document.querySelector("#page-policy .policy-editor-tools");
  if (assertionActions.parentElement !== editorTools) {
    editorTools.insertBefore(assertionActions, byId("policy-editor-utilities"));
  }
  byId("policy-editor-utilities").hidden = assertionsSelected;
  if (assertionsSelected) byId("policy-stage-status").hidden = true;
  byId("policy-assistant-pane").hidden = false;
  updateAssistantControls();
  byId("policy-workbench").dataset.recordKind = kind || "";
  updatePolicyWorkbenchLayout();
  window.dispatchEvent(new CustomEvent("policy-record-kind-changed", { detail: { kind, record } }));
}

function renderPolicyIdentity(record = state.policy.record, version = state.policy.revision, hash = record?.content_hash) {
  const title = byId("policy-record-title");
  const draftName = byId("policy-draft-name");
  const modified = byId("policy-modified-indicator");
  const identity = { title, version: byId("policy-revision-label"), modified };
  draftName.hidden = !record?.isDraft;
  if (!record) {
    policyEditorController.renderIdentity({}, identity);
    return;
  }
  const categoryID = record.kind === "assertions" ? organizationPolicyCategoryID(state.policy) : record.category_id;
  const category = state.policy.categories.find((item) => item.id === categoryID);
  const path = [category?.path, record.name].filter(Boolean).join("/");
  const draftNameValue = record.isDraft ? record.name : "";
  if (draftName.value !== draftNameValue) draftName.value = draftNameValue;
  policyEditorController.renderIdentity({
    name: record.name,
    tooltip: path,
    dirty: record.kind === "assertions"
      ? Boolean(window.policyAssertionDirty?.())
      : !record.isDraft && byId("policy-source").value !== state.policy.savedSource,
  }, identity);
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
  policy.evaluatedSource = null;
  policy.validSource = null;
  policy.browsingRevision = 0;
  policy.revisions = [];
  setPolicyRecordSurface("assertions", record);
  renderPolicyIdentity(record, record.current_revision, record.content_hash);
  renderPolicyCatalog();
  try { await loadPolicyHistory(record.id); }
  catch (error) { setPolicyFileStatus(error.message, true); }
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
  return window.pluralize(name);
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
  policySourceLayout.syncScroll();
  updatePolicyGutterBounds();
}

function updatePolicyGutterBounds() {
  const textarea = byId("policy-source");
  const gutter = byId("policy-test-gutter");
  gutter.style.bottom = `${textarea.offsetHeight - textarea.clientHeight}px`;
  byId("policy-code-editor").dataset.horizontalOverflow = String(textarea.scrollWidth > textarea.clientWidth);
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
  state.policy.evaluatedSource = null;
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
  const revisions = await policyEditorRequest(`/api/policy/records/${encodeURIComponent(recordID)}/revisions`);
  state.policy.revisions = revisions;
  renderPolicyHistory();
  setPolicyLoadStatus();
}

function renderPolicyHistory() {
  policyHistory.render(state.policy.revisions, {
    current: state.policy.browsingRevision || state.policy.revision,
    detail: (revision) => revision.summary || "No change summary",
    meta: (revision) => `${revision.author} · ${window.ZPRSafeDisplay.formatDateTime(revision.created_at)}`,
    onSelect: (revision) => { void browsePolicyRevision(revision.number); },
  });
}

async function refreshPolicyCatalog() {
  state.policy.loaded = false;
  await loadPolicyWorkspace();
}

async function browsePolicyRevision(number) {
  const policy = state.policy;
  if (!policy.record) return;
  setPolicyLoadStatus();
  try {
    const revision = await policyEditorRequest(`/api/policy/records/${encodeURIComponent(policy.record.id)}/revisions/${number}`);
    if (!window.ZPREditorPage.confirmDiscard(!policy.browsingRevision && byId("policy-source").value !== policy.savedSource, "Discard unsaved edits and browse this version?")) return;
    if (policy.testPending) stopPolicyTest();
    invalidatePolicyAnalysis();
    clearPolicyTestResults();
    policy.checkDiagnostics = "";
    policy.evaluatedSource = null;
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
    renderPolicyHistory();
    updatePolicyDirtyState();
  } catch (error) {
    setPolicyLoadStatus(error.message);
  }
}

function setPolicyLoadStatus(message = "") {
  window.ZPREditorPage.setStatus(byId("policy-load-status"), message, message ? "error" : "");
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

function policyHasCompileErrors(source = byId("policy-source").value) {
  return state.policy.evaluatedSource === source && state.policy.validSource !== source;
}

function policySaveWarning(source, testError = "") {
  const warnings = [];
  if (policyHasCompileErrors(source)) warnings.push("Compiler evaluation reported errors; this revision cannot be staged.");
  if (testError) warnings.push(`Policy test failed: ${testError}`);
  return warnings.length ? `${warnings.join(" ")} Continue saving anyway without a passing test.` : "";
}

function openRecordDialog(saveAs = true) {
  const options = byId("record-category"); options.replaceChildren();
  for (const category of policyCategoriesInUnifiedHierarchy()) {
    const option = document.createElement("option"); option.value = category.id; option.textContent = category.path; option.selected = category.id === state.policy.categoryID; options.append(option);
  }
  state.policy.saveAs = saveAs;
  state.policy.saveAsTestSource = "";
  state.policy.saveAsTestError = "";
  byId("record-dialog-title").textContent = saveAs ? "Save policy as" : "New policy";
  byId("record-submit").textContent = saveAs ? "Save As..." : "Create record";
  byId("record-name").value = saveAs && state.policy.record ? `${state.policy.record.name} copy` : "";
  const warning = policySaveWarning(byId("policy-source").value);
  byId("record-warning").hidden = !saveAs || !warning;
  byId("record-warning").textContent = warning;
  byId("record-error").textContent = "";
  byId("record-dialog").showModal(); byId("record-name").focus();
}

function beginNewPolicyDraft() {
  const policy = state.policy;
  if (!policy.categoryID || !policy.configured) return;
  if (!window.ZPREditorPage.confirmDiscard(hasUnsavedPolicyChanges(policy), "Discard the current edits and start a new policy draft?")) return;
  setPolicyLoadStatus();
  invalidatePolicyAnalysis();
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
  policy.evaluatedSource = null;
  policy.validSource = null;
  clearPolicyTestResults();
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
  renderPolicyHistory();
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
  if (!window.ZPREditorPage.confirmDiscard(hasUnsavedPolicyChanges(policy), "Discard the current edits and start a new assertion set?")) return;
  invalidatePolicyAnalysis();
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
  policy.evaluatedSource = null;
  policy.validSource = null;
  clearPolicyTestResults();
  policy.browsingRevision = 0;
  policy.revisions = [];
  setPolicyRecordSurface("assertions", draft);
  renderPolicyIdentity(draft, 0, "");
  renderPolicyHistory();
  renderPolicyCatalog();
  updatePolicyDirtyState();
  byId("assertion-source").focus();
}

function setPolicyFileStatus(text, isError = false) {
  policyEditorController.setStatus(text, isError ? "error" : "");
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
    const response = await window.zprOperatorFetch(path, {
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
  if (action === "rename") {
    if (!record || record.isDraft || record.archived || policy.browsingRevision || hasUnsavedPolicyChanges(policy)) return;
    const pickerWasOpen = !byId("policy-catalog-pane").hidden;
    const name = window.prompt("Rename record", record.name);
    if (name === null) return;
    try {
      const response = await window.zprOperatorFetch(`/api/policy/records/${encodeURIComponent(record.id)}/rename`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, expected_revision: record.current_revision }),
      });
      const renamed = await response.json();
      if (!response.ok) throw new Error(renamed.error || `Rename failed (${response.status})`);
      policy.record = renamed;
      await refreshPolicyCatalog();
      if (pickerWasOpen) setPolicyPickerOpen(true);
      renderPolicyIdentity(renamed, renamed.current_revision, renamed.content_hash);
      setPolicyFileStatus(`Renamed to ${renamed.name}`);
    } catch (error) {
      setPolicyFileStatus(error.message, true);
    }
    return;
  }
  if (!record || record.isDraft || !record.id) return;
  if (action === "delete") {
    if (!window.confirm(`Delete ${record.name}? Its revision history will be retained in Archived records.`)) return;
  }
  if (action !== "delete" && action !== "restore") return;
  try {
    const response = await window.zprOperatorFetch(`/api/policy/records/${encodeURIComponent(record.id)}${action === "restore" ? "/restore" : ""}`, {
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
    const response = await window.zprOperatorFetch("/api/policy/categories", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name, parent_id: parentID }) });
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
    if (!window.ZPREditorPage.isNamed(name)) throw new Error("Enter a record name other than Untitled.");
    const saveAs = state.policy.saveAs;
    const content = saveAs ? byId("policy-source").value : "";
    if (saveAs && state.policy.evaluatedSource !== content) {
      if (await checkPolicy(content) === null) return;
      if (content !== byId("policy-source").value) return;
    }
    const sourcePassedEvaluation = state.policy.validSource === content;
    const evaluationDetails = state.policy.checkDiagnostics || byId("policy-check-result").textContent;
    if (saveAs && state.policy.saveAsTestSource !== content) {
      const test = await runPolicyTestBeforeSave(content);
      if (test.cancelled) return;
      state.policy.saveAsTestSource = content;
      state.policy.saveAsTestError = test.passed ? "" : test.error || "The test was cancelled.";
      if (!test.passed) {
        byId("record-warning").textContent = `${policySaveWarning(content, state.policy.saveAsTestError)} Press Save anyway to confirm.`;
        byId("record-warning").hidden = false;
        byId("record-submit").textContent = "Save anyway";
        return;
      }
    }
    const response = await window.zprOperatorFetch("/api/policy/records", {
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
    if (saveAs) {
      state.policy.evaluatedSource = content;
      state.policy.validSource = sourcePassedEvaluation ? content : null;
      if (!sourcePassedEvaluation) {
        state.policy.checkDiagnostics = evaluationDetails;
        byId("policy-check-result").textContent = "Saved copy with evaluation errors. It cannot be staged until it passes.";
        byId("policy-check-result").hidden = true;
        byId("policy-check-result").dataset.state = "invalid";
      }
      updatePolicyDirtyState();
    }
  } catch (error) { byId("record-error").textContent = error.message; }
}

function updatePolicyDirtyState() {
  const policy = state.policy;
  const source = byId("policy-source").value;
  const contentDirty = source !== policy.savedSource;
  const dirty = !policy.browsingRevision && contentDirty;
  const checked = policy.evaluatedSource === source;
  const savedSourceHasErrors = policyHasCompileErrors(policy.savedSource);
  const canEdit = policy.configured && policy.record?.kind === "policy" && !policy.record?.archived && !policy.browsingRevision && !policy.saveTestPending && !policy.testPending && !policy.checkPending;
  const canViewSource = policy.configured && policy.record?.kind === "policy" && !policy.browsingRevision;
  const canTest = policy.configured && policy.record?.kind === "policy" && !policy.browsingRevision && Boolean(source.trim());
  const isDraft = Boolean(policy.record?.isDraft);
  const canSave = canEdit && contentDirty && (!isDraft || Boolean(policy.record.name.trim()) && source.trim() !== "");
  const canSaveAs = canEdit && !isDraft;
  byId("policy-save").disabled = !canSave;
  byId("policy-save-as").disabled = !canSaveAs;
  byId("policy-stage").disabled = policy.stagePending || policy.testPending || policy.checkPending;
  byId("policy-check").disabled = !canEdit || !canTest || !policy.compilerReady || !policy.testerReady || policy.testPending || policy.checkPending;
  byId("policy-check").textContent = "Analyze";
  if (!checked && !policy.testPending && !policy.checkPending) policyEditorController.setAnalysisState();
  byId("policy-format").disabled = !canEdit;
  byId("policy-format").classList.toggle("button-save-as-ready", canEdit);
  byId("policy-source").disabled = !canViewSource;
  byId("policy-source").readOnly = policy.saveTestPending;
  byId("policy-source").setAttribute("aria-readonly", String(policy.saveTestPending));
  updatePolicyWorkbenchLayout();
  byId("policy-test-gutter").hidden = false;
  // Errors without a source line have no gutter marker, so keep their message visible while Analyze is red.
  const testStatus = byId("policy-test-status");
  testStatus.hidden = !(testStatus.dataset.state === "error" && testStatus.textContent && byId("policy-check").dataset.analysisState === "error");
  byId("policy-check-result").hidden = true;
  byId("policy-attribute-rescan").disabled = !canEdit;
  byId("policy-attribute-rescan").classList.toggle("button-save-as-ready", canEdit && !policy.attributeScanPending);
  byId("policy-refresh").disabled = policy.testPending || !policy.record || (!dirty && !policy.browsingRevision && !isDraft);
  byId("policy-check").classList.toggle("button-next-evaluate", canEdit && !checked);
  byId("policy-save").classList.toggle("button-save-next", !canSave);
  byId("policy-save").classList.toggle("button-save-as-ready", canSave);
  byId("policy-save-as").classList.toggle("button-save-as-ready", canSaveAs);
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

function policyReferencedOmittedAttributes(source, omitted = []) {
  const code = source.split("\n").map((line) => line.replace(/\/\/.*$|#.*$/, "")).join("\n");
  return (omitted || []).filter((key) => new RegExp(`(^|[^\\w.])${key.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(?![\\w.])`, "i").test(code));
}

async function runPolicyTest(source = byId("policy-source").value) {
  const policy = state.policy;
  if (policy.testPending || !source.trim()) return { passed: false, error: "Policy test could not start for this source." };
  const controller = new AbortController();
  const isCurrent = policyEditorController.beginAnalysis("runtime");
  const current = () => !controller.signal.aborted && isCurrent() && source === byId("policy-source").value;
  clearPolicyTestResults();
  policy.testPending = true;
  policy.testAbort = controller;
  byId("policy-test-status").hidden = true;
  policyEditorController.setAnalysisState("pending");
  hidePolicyTestDetails();
  updatePolicyDirtyState();
  let outcome = { passed: false, error: "Policy test did not complete." };
  let errorTitle = "Analysis unavailable";
  try {
    const fixtures = await policyEditorRequest("/api/policy/test/fixtures", { signal: controller.signal });
    if (!current()) return { passed: false, cancelled: true };
    const unavailable = policyReferencedOmittedAttributes(source, fixtures.omitted_attributes);
    if (unavailable.length) {
      throw new Error(`policy references directory ${unavailable.length === 1 ? "attribute" : "attributes"} ${unavailable.map((key) => `"${key}"`).join(", ")} with values the ZPT fixture format cannot carry (comma, brace, control character or over 2048 bytes). This is not a policy compiler error.`);
    }
    errorTitle = "Policy evaluation error";
    policy.testDimensions = Array.from(new Set((fixtures.actors || []).flatMap((actor) => Object.keys(actor.dimensions || {})))).sort();
    const result = await policyEditorRequest("/api/policy/test", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ source, actors: fixtures.actors, services: fixtures.services }),
      signal: controller.signal,
    });
    if (!current()) return { passed: false, cancelled: true };
    policy.testResult = result;
    policy.testSource = source;
    policy.testWarnings = result.warnings || [];
    renderPolicyLintWarnings();
    policyEditorController.setAnalysisState("success");
    renderPolicyTestGutter(result);
    outcome = { passed: true, result };
  } catch (error) {
    if (error.name === "AbortError" || !current()) return { passed: false, cancelled: true };
    const parsedLines = policyTestErrorLines(error.message, source.split("\n").length);
    policy.testResult = { error: error.message };
    policy.testSource = source;
    policyEditorController.setAnalysisState("error");
    if (parsedLines.length) {
      renderPolicyTestErrorGutter(error.message, parsedLines, errorTitle);
      byId("policy-test-status").hidden = true;
      byId("policy-test-status").textContent = "";
    } else {
      byId("policy-test-status").textContent = `${errorTitle}: ${error.message}`;
      byId("policy-test-status").dataset.state = "error";
      byId("policy-test-status").hidden = false;
      renderPolicyLintWarnings();
    }
    outcome = { passed: false, error: error.message };
  } finally {
    isCurrent.finish();
    if (policy.testAbort === controller) {
      policy.testAbort = null;
      policy.testPending = false;
      updatePolicyDirtyState();
    }
  }
  return outcome;
}

function renderPolicyTestGutter(result) {
  const gutter = byId("policy-test-gutter");
  const content = byId("policy-test-gutter-content");
  const lineCount = byId("policy-source").value.split("\n").length;
  const resultsByLine = new Map();
  for (const service of result.services || []) {
    if (!service.supported) continue;
    for (const rule of service.rules || []) {
      const line = window.ZPREditorPage.sourceLine(rule.line, lineCount);
      if (line === null) continue;
      let lineResult = resultsByLine.get(line);
      if (!lineResult) {
        lineResult = { count: 0, effect: rule.effect || "deny", services: new Set(), subjects: new Map() };
        resultsByLine.set(line, lineResult);
      }
      const matched = rule.matched || { count: 0, subjects: [] };
      lineResult.count += Number(matched.count) || 0;
      if (rule.effect === "deny") lineResult.effect = "deny";
      lineResult.services.add(service.name || service.id);
      for (const subject of matched.subjects || []) {
        if (subject.id && !lineResult.subjects.has(subject.id)) lineResult.subjects.set(subject.id, subject);
      }
    }
  }

  const fragment = document.createDocumentFragment();
  for (let line = 1; line <= lineCount; line++) {
    const row = document.createElement("div");
    row.className = "policy-test-line";
    row.dataset.line = String(line);
    const lineResult = resultsByLine.get(line);
    if (lineResult) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "policy-test-line-result";
      button.dataset.effect = lineResult.effect;
      button.textContent = lineResult.count > 0 ? String(lineResult.count) : "None";
      const resultTitle = `Line ${line} · ${Array.from(lineResult.services).join(", ")} · ${button.textContent}`;
      button.title = `${resultTitle} · Show details`;
      button.setAttribute("aria-label", button.title);
      button.addEventListener("click", () => showPolicyTestSubjects([...lineResult.subjects.values()], "", resultTitle));
      row.append(button);
    }
    fragment.append(row);
  }
  content.replaceChildren(fragment);
  gutter.hidden = false;
  content.style.transform = `translateY(${-byId("policy-source").scrollTop}px)`;
  renderPolicyLintWarnings();
}

function policyTestErrorLines(message, lineCount) {
  return [...new Set(Array.from(String(message).matchAll(/\bline\s+(\d+)(?:\s*,\s*column\s+\d+)?\b/gi), (match) => Number(match[1]))) ]
    .filter((line) => window.ZPREditorPage.sourceLine(line, lineCount) !== null)
    .sort((left, right) => left - right);
}

function renderPolicyTestErrorGutter(message, errorLines, errorTitle = "Policy test error") {
  const gutter = byId("policy-test-gutter");
  const content = byId("policy-test-gutter-content");
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
      button.title = `Line ${line} · ${errorTitle} · Show diagnostic`;
      button.setAttribute("aria-label", button.title);
      button.addEventListener("click", () => showPolicyTestError(line, message, errorTitle));
      row.append(button);
    }
    fragment.append(row);
  }
  content.replaceChildren(fragment);
  gutter.hidden = false;
  content.style.transform = `translateY(${-byId("policy-source").scrollTop}px)`;
  renderPolicyLintWarnings();
}

function policyTestDimensionLabel(name) {
  return String(name).split("_").map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(" ");
}

function showPolicyTestSubjects(subjects, dimension, title) {
  const list = byId("policy-analyze-subjects");
  const details = byId("policy-test-details");
  details.dataset.detailKind = "subjects";
  const unique = new Map();
  for (const subject of subjects) {
    const key = dimension ? subject.dimensions?.[dimension] : subject.id;
    if (!key || unique.has(key)) continue;
    unique.set(key, subject);
  }
  list.replaceChildren();
  byId("policy-analyze-title").textContent = "Policy test results";
  byId("policy-analyze-subject-title").textContent = `${title} · ${unique.size} ${unique.size === 1 ? "member" : "members"}${unique.size > 100 ? " · First 100 shown" : ""}`;
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
  details.showModal();
}

function showPolicyTestError(line, error, title = "Policy test error") {
  const details = byId("policy-test-details");
  const list = byId("policy-analyze-subjects");
  details.dataset.detailKind = "error";
  details.setAttribute("aria-label", "Analyze error details");
  byId("policy-analyze-title").textContent = title;
  byId("policy-analyze-subject-title").textContent = "";
  const detail = document.createElement("div");
  detail.className = "policy-test-subject policy-test-diagnostic";
  detail.textContent = error;
  list.replaceChildren(detail);
  details.showModal();
}

function hidePolicyTestDetails() {
  const details = byId("policy-test-details");
  if (details.open) details.close();
  delete details.dataset.detailKind;
  details.setAttribute("aria-label", "Analyze details");
  byId("policy-analyze-subjects").replaceChildren();
}

function clearPolicyTestResults() {
  const policy = state.policy;
  policy.testWarnings = [];
  policy.testResult = null;
  policy.testSource = "";
  policy.testDimensions = [];
  byId("policy-test-gutter-content").replaceChildren();
  byId("policy-test-gutter").hidden = false;
  byId("policy-check-result").hidden = true;
  byId("policy-check-result").textContent = "";
  byId("policy-test-status").hidden = true;
  byId("policy-test-status").textContent = "";
  byId("policy-test-status").dataset.state = "";
  hidePolicyTestDetails();
  renderPolicyLintWarnings();
}

function stopPolicyTest() {
  const policy = state.policy;
  invalidatePolicyTest();
  clearPolicyTestResults();
  if (byId("policy-check").dataset.analysisState === "pending") {
    policyEditorController.setAnalysisState(policy.validSource === policy.evaluatedSource ? "success" : "");
  }
  byId("policy-test-gutter-content").replaceChildren();
  byId("policy-test-gutter").hidden = false;
  byId("policy-test-status").hidden = true;
  byId("policy-test-status").textContent = "";
  updatePolicyDirtyState();
}

function renderPolicyStageStatus() {
  const policy = state.policy;
  const status = byId("policy-stage-status");
  status.hidden = false;
  if (policy.stagePending) {
    status.textContent = "Compiling and staging candidate…";
    status.dataset.state = "pending";
  } else if (policy.stageError) {
    status.textContent = policy.stageError;
    status.dataset.state = "error";
  } else if (policy.record?.kind === "policy" && policyHasCompileErrors(policy.savedSource)) {
    status.hidden = true;
    status.textContent = "";
    status.dataset.state = "";
  } else if (policy.stagedCandidate) {
    const candidate = policy.stagedCandidate;
    status.textContent = `Staged ${candidate.record_name} r${candidate.record_revision} · SHA-256 ${candidate.bundle_sha256.slice(0, 12)} · not pushed`;
    status.dataset.state = "staged";
  } else {
    status.hidden = true;
    status.textContent = "";
    status.dataset.state = "";
  }
}

async function stageSelectedPolicy() {
  const policy = state.policy;
  if (policy.stagePending || policy.testPending || policy.checkPending) return;
  policy.stageError = "";
  renderPolicyStageStatus();
  if (!policy.record || policy.record.kind !== "policy" || policy.record.isDraft || policy.record.archived || policy.browsingRevision) {
    policy.stageError = "Only a saved policy version can be staged.";
    renderPolicyStageStatus();
    return;
  }
  const source = byId("policy-source").value;
  const analyzed = await analyzePolicySource(source);
  if (analyzed === null) return;
  if (!analyzed) {
    policy.stageError = "Analysis failed. Candidate was not staged.";
    renderPolicyStageStatus();
    return;
  }
  if (source !== policy.savedSource) {
    policy.stageError = "Save the analyzed changes before staging.";
    renderPolicyStageStatus();
    return;
  }
  if (!policy.stagingReady) {
    policy.stageError = "Policy staging is not configured.";
    renderPolicyStageStatus();
    return;
  }
  byId("policy-stage-confirm-summary").textContent = `${policy.record.name} · Version ${policy.record.current_revision} will be staged as a candidate.`;
  byId("policy-stage-dialog").showModal();
}

async function confirmPolicyStage() {
  const policy = state.policy;
  if (policy.stagePending) return;
  byId("policy-stage-dialog").close();
  if (!policy.record || policy.record.kind !== "policy" || policy.record.isDraft || policy.browsingRevision || byId("policy-source").value !== policy.savedSource) {
    policy.stageError = "Save the current policy revision before staging.";
    renderPolicyStageStatus();
    return;
  }
  policy.stagePending = true;
  policy.stageError = "";
  updatePolicyDirtyState();
  try {
    const result = await policyEditorRequest(`/api/policy/records/${encodeURIComponent(policy.record.id)}/stage`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ expected_revision: policy.record.current_revision }),
    });
    policy.stagedCandidate = result;
  } catch (error) {
    policy.stageError = error.message || "Could not stage policy candidate.";
  } finally {
    policy.stagePending = false;
    updatePolicyDirtyState();
  }
}

async function reloadPolicyWorkspace() {
  if (!window.ZPREditorPage.confirmDiscard(hasUnsavedPolicyChanges(state.policy) || state.policy.browsingRevision || state.policy.record?.isDraft, "Discard changes and return to the last saved policy?")) return;
  if (!state.policy.record) return loadPolicyWorkspace();
  if (state.policy.record.isDraft) {
    clearPolicySelection();
    return;
  }
  await selectPolicyRecord(state.policy.record.id, true, true);
}

const policyEditorRequest = (path, options) => window.ZPREditorPage.requestJSON(
  (...args) => window.zprOperatorFetch(...args), path, options,
);

async function checkPolicy(source = byId("policy-source").value) {
  const button = byId("policy-check");
  if (state.policy.checkPending) return false;
  const controller = new AbortController();
  const isCurrent = policyEditorController.beginAnalysis("compiler");
  state.policy.checkAbort = controller;
  state.policy.checkPending = true;
  state.policy.lintWarnings = [];
  state.policy.testWarnings = [];
  renderPolicyLintWarnings();
  state.policy.checkDiagnostics = "";
  state.policy.errorOffsets = [];
  updatePolicyHighlight();
  button.disabled = true;
  policyEditorController.setAnalysisState("pending");
  byId("policy-check-result").textContent = "Checking with ZPLC…";
  updatePolicyDirtyState();
  let valid = false;
  try {
    const result = await policyEditorRequest("/api/policy/check", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ source }),
      signal: controller.signal,
    });
    if (controller.signal.aborted || !isCurrent() || source !== byId("policy-source").value) return null;
    valid = result.valid;
    policySetCheckResult(valid, result.diagnostics || result.error || "No compiler diagnostics.", source, result.warnings || []);
  } catch (error) {
    if (error.name === "AbortError" || controller.signal.aborted || !isCurrent() || source !== byId("policy-source").value) return null;
    policySetCheckResult(false, error.details?.diagnostics || error.details?.error || error.message, source, error.details?.warnings || []);
  } finally {
    isCurrent.finish();
    if (state.policy.checkAbort === controller) {
      state.policy.checkAbort = null;
      state.policy.checkPending = false;
      updatePolicyDirtyState();
    }
  }
  return valid;
}

async function analyzePolicySource(source) {
  if (state.policy.testPending || state.policy.checkPending) return false;
  clearPolicyTestResults();
  state.policy.checkDiagnostics = "";
  const checked = await checkPolicy(source);
  if (checked === null || source !== byId("policy-source").value) return null;
  if (!checked) return false;
  const test = await runPolicyTest(source);
  return test.cancelled ? null : test.passed;
}

async function evaluateAndTestPolicy() {
  if (state.policy.testPending || state.policy.checkPending) return;
  await analyzePolicySource(byId("policy-source").value);
}

function renderPolicyLintWarnings() {
  byId("policy-lint-warnings")?.remove();
  const content = byId("policy-test-gutter-content");
  for (const button of content.querySelectorAll('[data-warning-only="true"]')) button.remove();
  for (const button of content.querySelectorAll("[data-warning-details]")) {
    delete button.dataset.warningDetails;
    delete button.dataset.hasWarnings;
    button.title = button.dataset.resultTitle;
    button.setAttribute("aria-label", button.title);
  }
  const warnings = state.policy.evaluatedSource === byId("policy-source").value
    ? [...state.policy.lintWarnings || [], ...state.policy.testWarnings || []] : [];
  const lineCount = byId("policy-source").value.split("\n").length;
  const byLine = new Map();
  for (const warning of warnings) {
    const line = window.ZPREditorPage.sourceLine(warning.line, lineCount);
    if (line === null) continue;
    if (!byLine.has(line)) byLine.set(line, []);
    byLine.get(line).push(warning);
  }
  if (byLine.size && !content.children.length) {
    for (let line = 1; line <= lineCount; line++) {
      const row = document.createElement("div");
      row.className = "policy-test-line";
      row.dataset.line = String(line);
      content.append(row);
    }
  }
  for (const [line, entries] of byLine) {
    const row = content.querySelector(`[data-line="${line}"]`);
    if (!row) continue;
    let button = row.querySelector("button");
    if (!button) {
      button = document.createElement("button");
      button.type = "button";
      button.className = "policy-test-line-result";
      button.dataset.effect = "warning";
      button.dataset.warningOnly = "true";
      button.textContent = "WARN";
      button.title = `Line ${line}`;
      row.append(button);
    }
    if (!button.dataset.resultTitle) button.dataset.resultTitle = button.title;
    button.dataset.hasWarnings = "true";
    button.dataset.warningDetails = JSON.stringify(entries);
    button.title = `${button.dataset.resultTitle} · ${entries.map((warning) => `Warning [${warning.code}]: ${warning.message}`).join(" · ")}`;
    button.setAttribute("aria-label", button.title);
    if (!button.dataset.warningListener) {
      button.dataset.warningListener = "true";
      button.addEventListener("click", () => {
        const entries = JSON.parse(button.dataset.warningDetails || "[]");
        if (!entries.length) return;
        if (button.dataset.warningOnly === "true") {
          showPolicyTestError(line, "", "Analysis warnings");
          byId("policy-test-details").dataset.detailKind = "warning";
          byId("policy-test-details").setAttribute("aria-label", "Analyze warning details");
          byId("policy-analyze-subjects").replaceChildren();
        }
        for (const warning of entries) {
          const detail = document.createElement("div");
          detail.className = "policy-test-subject policy-test-diagnostic";
          detail.textContent = `Line ${line} · Warning [${warning.code}]: ${warning.message}`;
          byId("policy-analyze-subjects").append(detail);
        }
      });
    }
  }
  byId("policy-test-gutter").hidden = false;
  content.style.transform = `translateY(${-byId("policy-source").scrollTop}px)`;
}

function policySetCheckResult(valid, diagnostics, source, warnings = []) {
  const policy = state.policy;
  policy.evaluatedSource = source;
  policy.lintWarnings = warnings;
  renderPolicyLintWarnings();
  policy.validSource = valid ? source : null;
  policy.errorOffsets = valid ? [] : extractZPLErrorOffsets(diagnostics, source);
  updatePolicyHighlight();
  const result = byId("policy-check-result");
  policy.checkDiagnostics = valid ? "" : diagnostics;
  result.hidden = true;
  result.textContent = "";
  result.dataset.state = valid ? "valid" : "invalid";
  policyEditorController.setAnalysisState(valid ? "success" : "error");
  byId("policy-test-status").hidden = true;
  byId("policy-test-status").textContent = "";
  if (!valid) {
    const parsedLines = policyTestErrorLines(diagnostics, source.split("\n").length);
    if (parsedLines.length) {
      renderPolicyTestErrorGutter(diagnostics, parsedLines, "Compiler error");
    } else {
      byId("policy-test-status").textContent = `Compiler error: ${diagnostics}`;
      byId("policy-test-status").dataset.state = "error";
      byId("policy-test-status").hidden = false;
      renderPolicyLintWarnings();
    }
  }
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
  const source = byId("policy-source").value;
  if (state.policy.testPending || state.policy.saveTestPending || state.policy.checkPending) return;
  if (!window.ZPREditorPage.isNamed(state.policy.record?.name)) {
    byId("policy-test-status").textContent = "Enter a policy name other than Untitled before saving.";
    byId("policy-test-status").dataset.state = "error";
    byId("policy-test-status").hidden = false;
    return;
  }
  if (state.policy.evaluatedSource !== source && await checkPolicy(source) === null) return;
  if (source !== byId("policy-source").value) return;
  const test = await runPolicyTestBeforeSave(source);
  if (test.cancelled) return;
  byId("version-note").value = "";
  byId("version-error").textContent = "";
  const warning = policySaveWarning(source, state.policy.saveTestError);
  byId("version-warning").hidden = !warning;
  byId("version-warning").textContent = warning;
  byId("version-save").textContent = warning ? "Save anyway" : "Save";
  byId("version-dialog").showModal();
  byId("version-note").focus();
}

async function runPolicyTestBeforeSave(source) {
  const policy = state.policy;
  const operation = {};
  policy.saveTestOperation = operation;
  policy.saveTestPending = true;
  policy.saveTestSource = "";
  policy.saveTestError = "";
  byId("policy-test-status").textContent = "Running policy test before save…";
  byId("policy-test-status").dataset.state = "pending";
  updatePolicyDirtyState();
  const test = await runPolicyTest(source);
  if (policy.saveTestOperation !== operation) return { passed: false, cancelled: true };
  policy.saveTestOperation = null;
  policy.saveTestPending = false;
  if (test.cancelled) {
    updatePolicyDirtyState();
    return test;
  }
  policy.saveTestSource = source;
  policy.saveTestError = test.passed ? "" : test.error || "The test was cancelled.";
  updatePolicyDirtyState();
  return test;
}

async function appendPolicyVersion(summary) {
  const policy = state.policy;
  const button = byId("policy-save");
  const submittedSource = byId("policy-source").value;
  const isDraft = Boolean(policy.record.isDraft);
  const recordName = policy.record.name.trim();
  if (!window.ZPREditorPage.isNamed(recordName)) {
    byId("version-error").textContent = "Enter a policy name other than Untitled before saving.";
    return;
  }
  if (policy.evaluatedSource !== submittedSource) {
    byId("version-error").textContent = "Evaluate this exact buffer before saving.";
    return;
  }
  if (policy.saveTestSource !== submittedSource) {
    byId("version-error").textContent = "Run the policy test before saving this source.";
    return;
  }
  const savedWithErrors = policy.validSource !== submittedSource;
  const diagnostics = policy.checkDiagnostics || byId("policy-check-result").textContent;
  const recordID = policy.record.id;
  const expectedRevision = policy.revision;
  button.disabled = true;
  byId("version-dialog").close();
  byId("policy-check-result").textContent = "Saving version…";
  try {
    let result;
    let nextRevision;
    let contentHash;
    if (isDraft) {
      result = await policyEditorRequest("/api/policy/records", {
        method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({
          category_id: policy.record.category_id, name: recordName, kind: "policy",
          content_type: policy.record.content_type, metadata: policy.record.metadata,
          content: submittedSource, summary,
        }),
      });
      nextRevision = result.current_revision;
      contentHash = result.content_hash;
      policy.record = { ...result, isDraft: false };
      policy.records.push(policy.record);
    } else {
      result = await policyEditorRequest(`/api/policy/records/${encodeURIComponent(recordID)}/revisions`, {
        method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ content: submittedSource, expected_revision: expectedRevision, summary }),
      });
      nextRevision = result.number;
      contentHash = result.content_hash;
    }
    policy.revision = nextRevision;
    policy.savedSource = submittedSource;
    policy.validSource = savedWithErrors ? null : policy.savedSource;
    policy.record.current_revision = nextRevision;
    policy.record.content = submittedSource;
    policy.record.content_hash = contentHash;
    policy.browsingRevision = 0;
    renderPolicyIdentity(policy.record, nextRevision, contentHash);
    await loadPolicyHistory(policy.record.id);
    await refreshPolicyCatalog();
    policy.evaluatedSource = submittedSource;
    policy.validSource = savedWithErrors ? null : submittedSource;
    byId("policy-check-result").textContent = savedWithErrors
      ? "Saved with evaluation errors. This revision cannot be staged until it passes."
      : result.diagnostics || `Created Version ${nextRevision}.`;
    byId("policy-check-result").hidden = true;
    byId("policy-check-result").dataset.state = savedWithErrors ? "invalid" : "valid";
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
  const groups = pairVisas(rows);
  const primary = group => group.members[0];
  const columns = {
    id: (group) => num(primary(group).id),
    flow: (group) => group.members.map(visa => `${visaEndpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port)} ${visa.source_addr || ""} ${visa.dest_addr || ""}`).join(" "),
    proto: (group) => primary(group).proto,
    node: (group) => group.members.map(visa => visa.requesting_node || "").join(" "),
    expires: (group) => Math.min(...group.members.map(visa => num(visa.expires))),
    pair: (group) => group.members.map(visa => `${visa.id} ${visaPairLabel(group, visa)}`).join(" "),
  };
  const shown = visibleRows("visas", groups, columns);
  byId("visa-rows").innerHTML = shown.length ? shown.flatMap((group) => group.members.map((visa, index) => {
    const flow = visaEndpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port);
    const classes = ["visa-row", index ? "visa-pair-partner" : "visa-pair-lead", group.forward && group.reverse ? "paired" : "unpaired"].join(" ");
    return `<tr class="${classes}" data-visa-id="${escapeHTML(visa.id)}"><td class="mono">${escapeHTML(visa.id)}</td><td class="mono" title="${escapeHTML(visaEndpointTitle(visa.source_addr, visa.dest_addr))}">${escapeHTML(flow)}</td><td>${escapeHTML(visa.proto || "—")}</td><td>${escapeHTML(visa.requesting_node || "—")}</td><td class="mono">${escapeHTML(window.ZPRSafeDisplay.formatTime(num(visa.expires) * 1000))}</td><td class="mono visa-pair-cell">${escapeHTML(visaPairLabel(group, visa))}</td></tr>`;
  })).join("") : `<tr><td colspan="6" class="empty-row">${rows.length ? "No matching visas" : "No active visas returned"}</td></tr>`;
}

function protocolName(number) {
  return window.ZPRSafeDisplay.protocolName(number) || `IP ${number}`;
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

const adapterDecisionPulses = new Map();
const serviceGrantPulses = new Map();
const mapCountPulses = new Map();
let previousMapCounts = null;
const MAP_COUNT_PULSE_DURATION = 2400;

function pulseMapCounts(counts) {
  const now = Date.now();
  if (previousMapCounts) for (const [key, count] of counts) {
    const previous = previousMapCounts.get(key);
    if (count != null && previous != null && count !== previous) mapCountPulses.set(key, now);
  }
  previousMapCounts = counts;
  for (const [key, startedAt] of mapCountPulses) if (now - startedAt >= MAP_COUNT_PULSE_DURATION || !counts.has(key)) mapCountPulses.delete(key);
  for (const component of document.querySelectorAll("#graph-world > [data-topology-component]")) {
    const startedAt = mapCountPulses.get(component.dataset.topologyComponent);
    const badge = component.querySelector(".graph-visa-count");
    if (startedAt == null || !badge) continue;
    badge.dataset.countPulse = "true";
    const bounds = badge.getBBox();
    const x = bounds.x + bounds.width / 2, y = bounds.y + bounds.height / 2;
    const motion = matchMedia("(prefers-reduced-motion: reduce)").matches;
    const animation = badge.animate([
      { opacity: 1 },
      { opacity: 0.65, offset: 0.25 },
      { opacity: 1, offset: 0.5 },
      { opacity: 0.65, offset: 0.75 },
      { opacity: 1 },
    ], { duration: MAP_COUNT_PULSE_DURATION, easing: "linear" });
    // Apply expansion in SVG coordinates, rather than relying on browser-specific g transform origins.
    const expand = () => {
      if (!badge.isConnected) return;
      const progress = animation.effect.getComputedTiming().progress ?? 1;
      const scale = motion ? 1 : 1 + 0.75 * Math.sin(progress * Math.PI * 2) ** 2;
      badge.setAttribute("transform", `translate(${x} ${y}) scale(${scale}) translate(${-x} ${-y})`);
      if (animation.playState === "running") requestAnimationFrame(expand);
    };
    animation.currentTime = now - startedAt;
    expand();
    animation.onfinish = () => { delete badge.dataset.countPulse; badge.removeAttribute("transform"); };
  }
}

function pulseGraphWire(wire, decision, color, duration, elapsed) {
  const normal = getComputedStyle(wire);
  const base = { stroke: normal.stroke, strokeWidth: normal.strokeWidth };
  wire.dataset.decision = decision;
  const animation = wire.animate([base, { stroke: color, strokeWidth: `${Number.parseFloat(normal.strokeWidth) + 3}px`, offset: 0.35 }, base], { duration, easing: "ease-in-out" });
  animation.onfinish = () => delete wire.dataset.decision;
  animation.currentTime = elapsed;
}
const ADAPTER_DECISION_DURATION = 1200;
const SERVICE_GRANT_DURATION = 2000;

function decisionPulseFrames(reducedMotion) {
  return [
    { opacity: 0, ...(reducedMotion ? {} : { transform: "scale(1)" }) },
    { opacity: 1, offset: 0.4, ...(reducedMotion ? {} : { transform: "scale(1.25)" }) },
    { opacity: 0, ...(reducedMotion ? {} : { transform: "scale(1)" }) },
  ];
}

function serviceMatchesVisa(service, visa, actors) {
  const reverse = String(visa.direction || "").toLowerCase() === "reverse";
  const address = reverse ? visa.source_addr : visa.dest_addr;
  const providerAddress = service.zpr_addr || actors.find((actor) => actor.cn === service.actor_cn)?.zpr_addr;
  if (!address || !providerAddress || dnsAddressKey(address) !== dnsAddressKey(providerAddress)) return false;
  const normalizeProtocol = (protocol) => String(protocol || "").toUpperCase().replace(/^(IPV6_ICMP|ICMPV6)$/, "ICMP6");
  const protocol = normalizeProtocol(visa.proto);
  const port = reverse ? visa.source_port : visa.dest_port;
  return [...String(service.service_endpoints || "").matchAll(/([A-Z0-9_]+)\/(\d+)(?:-(\d+))?/gi)].some((endpoint) => {
    if (normalizeProtocol(endpoint[1]) !== protocol) return false;
    if (protocol === "ICMP6") return true;
    return port != null && num(port) >= Number(endpoint[2]) && num(port) <= Number(endpoint[3] || endpoint[2]);
  });
}

function recordAdapterDecisions(data, previous) {
  if (!previous || data === previous) return;
  const decisions = new Map();
  const visaIDs = new Set((previous.recent_visas || []).map((visa) => String(visa.id)));
  for (const visa of data.recent_visas || []) {
    const requester = String(visa.direction || "").toLowerCase() === "reverse" ? visa.dest_addr : visa.source_addr;
    if (!visaIDs.has(String(visa.id))) {
      if (requester) decisions.set(dnsAddressKey(requester), "grant");
      for (const service of data.services || []) {
        if (serviceMatchesVisa(service, visa, data.actors)) {
          serviceGrantPulses.set(`service:${JSON.stringify([service.actor_cn, service.service_name])}`, { startedAt: Date.now() });
        }
      }
    }
  }
  const denyKey = (denial) => JSON.stringify([dnsAddressKey(denial.source_addr), dnsAddressKey(denial.dest_addr), denial.protocol, denial.dest_port, denial.deny_code]);
  const previousDenials = new Map((previous.recent_denies || []).map((denial) => [denyKey(denial), denial]));
  for (const denial of data.recent_denies || []) {
    const prior = previousDenials.get(denyKey(denial));
    if (denial.source_addr && num(denial.count) > 0 && (num(denial.count) > num(prior?.count) || num(denial.last_deny_ms) > num(prior?.last_deny_ms))) decisions.set(dnsAddressKey(denial.source_addr), "deny");
  }
  const now = Date.now();
  for (const [address, decision] of decisions) adapterDecisionPulses.set(address, { decision, startedAt: now });
}

function pulseAdapterDecisions(data) {
  const now = Date.now();
  for (const [address, pulse] of adapterDecisionPulses) {
    if (now - pulse.startedAt >= ADAPTER_DECISION_DURATION) adapterDecisionPulses.delete(address);
  }
  for (const [key, pulse] of serviceGrantPulses) {
    if (now - pulse.startedAt >= SERVICE_GRANT_DURATION) serviceGrantPulses.delete(key);
  }
  if (!["map", "connections"].includes(currentPage())) return;
  const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;
  for (const badge of document.querySelectorAll(".graph-service-badge[data-topology-component]")) {
    const pulse = serviceGrantPulses.get(badge.dataset.topologyComponent);
    const glyph = badge.querySelector("rect:not(.graph-service-decision-ring)");
    if (!pulse || !glyph) continue;
    const elapsed = now - pulse.startedAt;
    const outline = document.createElementNS("http://www.w3.org/2000/svg", "rect");
    outline.classList.add("graph-service-decision-ring", "graph-decision-glyph");
    outline.setAttribute("x", String(Number(glyph.getAttribute("x")) - 3));
    outline.setAttribute("y", String(Number(glyph.getAttribute("y")) - 3));
    outline.setAttribute("width", String(Number(glyph.getAttribute("width")) + 6));
    outline.setAttribute("height", String(Number(glyph.getAttribute("height")) + 6));
    outline.setAttribute("rx", "6");
    outline.setAttribute("aria-hidden", "true");
    badge.insertBefore(outline, glyph);
    const animation = outline.animate(decisionPulseFrames(reducedMotion), { duration: SERVICE_GRANT_DURATION, easing: "ease-in-out" });
    animation.onfinish = () => outline.remove();
    animation.currentTime = elapsed;
    for (const edge of document.querySelectorAll(".graph-service-edge")) {
      if (edge.dataset.connectorTo === badge.dataset.topologyComponent) {
        pulseGraphWire(edge.querySelector(".graph-link"), "grant", getComputedStyle(outline).stroke, SERVICE_GRANT_DURATION, elapsed);
      }
    }
    if (!reducedMotion) {
      glyph.classList.add("graph-decision-glyph");
      const expansion = glyph.animate([{ transform: "scale(1)" }, { transform: "scale(1.2)", offset: 0.4 }, { transform: "scale(1)" }], { duration: SERVICE_GRANT_DURATION, easing: "ease-in-out" });
      expansion.onfinish = () => glyph.classList.remove("graph-decision-glyph");
      expansion.currentTime = elapsed;
    }
  }
  for (const actor of data.actors.filter((actor) => !actor.node)) {
    const pulse = actor.zpr_addr && adapterDecisionPulses.get(dnsAddressKey(actor.zpr_addr));
    if (!pulse) continue;
    const { decision } = pulse;
    const elapsed = now - pulse.startedAt;
    const vertex = [...document.querySelectorAll(".graph-vertex[data-inspect-actor]")].find((element) => element.dataset.inspectActor === actor.cn);
    const glyph = vertex?.querySelector(".graph-adapter, .graph-gateway, .graph-visa");
    if (!vertex || !glyph) continue;
    const ring = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    ring.classList.add("graph-decision-ring", "graph-decision-glyph");
    ring.dataset.decision = decision;
    ring.setAttribute("cx", vertex.dataset.originX);
    ring.setAttribute("cy", vertex.dataset.originY);
    ring.setAttribute("r", glyph.classList.contains("graph-adapter") ? "34" : "41");
    ring.setAttribute("aria-hidden", "true");
    vertex.insertBefore(ring, glyph);
    const ringAnimation = ring.animate(decisionPulseFrames(reducedMotion), { duration: ADAPTER_DECISION_DURATION, easing: "ease-in-out" });
    ringAnimation.onfinish = () => ring.remove();
    ringAnimation.currentTime = elapsed;
    const color = getComputedStyle(ring).stroke;
    for (const edge of document.querySelectorAll(".graph-edge[data-dock-adapter]")) {
      if (edge.dataset.dockAdapter !== actor.cn) continue;
      const wire = edge.querySelector(".graph-link");
      pulseGraphWire(wire, decision, color, ADAPTER_DECISION_DURATION, elapsed);
    }
    if (!reducedMotion) {
      glyph.classList.add("graph-decision-glyph");
      const expansion = glyph.animate([{ transform: "scale(1)" }, { transform: "scale(1.2)", offset: 0.4 }, { transform: "scale(1)" }], { duration: ADAPTER_DECISION_DURATION, easing: "ease-in-out" });
      expansion.onfinish = () => glyph.classList.remove("graph-decision-glyph");
      expansion.currentTime = elapsed;
    }
  }
}

function render(data) {
  const previousSnapshot = state.snapshot;
  recordAdapterDecisions(data, previousSnapshot);
  const componentKeys = new Set([
    ...data.actors.map((actor) => `actor:${JSON.stringify(actor.cn)}`),
    ...(data.services || []).map((service) => `service:${JSON.stringify([service.actor_cn, service.service_name])}`),
  ]);
  const now = Date.now();
  const removedKeys = state.topologyComponents
    ? [...state.topologyComponents].filter((key) => !componentKeys.has(key))
    : [];
  const exitComponents = state.graphAnimations && !byId("page-map").hidden ? snapshotRemovedTopologyComponents(removedKeys) : [];
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
  window.dispatchEvent(new CustomEvent("zpr-snapshot", { detail: data }));
  updateStatusTabCounts(data);
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

function controlRoomSnapshot() {
  return state.snapshot;
}

async function refresh() {
  if (state.pending) return;
  state.pending = true;
  byId("refresh-now").disabled = true;
  if (currentPage() === "dns") void loadDNSStats();
  try {
    const response = await window.zprOperatorFetch("/api/snapshot", { cache: "no-store", headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error(`Monitor server responded ${response.status}`);
    const snapshot = await response.json();
    render(snapshot);
    document.dispatchEvent(new CustomEvent("control-room:refreshed", { detail: snapshot }));
  } catch (error) {
    updateConnection({ api_status: "disconnected", errors: [error.message] });
    if (!state.snapshot) byId("topology-stage").querySelector(".empty-state").textContent = "Unable to load topology. Check the connection error above or use Refresh to retry.";
  } finally {
    state.pending = false;
    byId("refresh-now").disabled = false;
  }
}

function setPollTimer() {
  if (state.timer) clearInterval(state.timer);
  const seconds = Number(byId("poll-rate").value);
  if (!state.paused) state.timer = setInterval(refresh, seconds * 1000);
}

byId("refresh-now").addEventListener("click", () => {
  document.dispatchEvent(new CustomEvent("control-room:refresh-requested"));
  void refresh();
});
function updatePolicyWorkbenchLayout() {
  const workbench = byId("policy-workbench");
  const assistantCollapsed = workbench.dataset.assistantCollapsed === "true";
  const assertionTestMode = workbench.dataset.assertionTestMode === "true";
  const tablet = window.matchMedia("(max-width: 900px)").matches;
  if (tablet || assertionTestMode) workbench.style.gridTemplateColumns = "minmax(0, 1fr)";
  else if (assistantCollapsed) workbench.style.gridTemplateColumns = "minmax(0, 1fr) 42px";
  else workbench.style.gridTemplateColumns = "minmax(0, 1.8fr) minmax(245px, .8fr)";
}

window.policyWorkbenchLayoutChanged = updatePolicyWorkbenchLayout;

function setPolicyPickerOpen(open, restoreFocus = false) {
  const pane = byId("policy-catalog-pane");
  const button = byId("policy-picker-toggle");
  pane.hidden = !open;
  button.setAttribute("aria-expanded", String(open));
  button.setAttribute("aria-label", open ? "Close policy picker" : "Choose policy");
  if (open) byId("policy-category-tree").focus();
  else if (restoreFocus) button.focus();
}

const policyFileMenu = policyEditorController.files;
function setPolicyFileMenuOpen(open, restoreFocus = false) {
  policyFileMenu.setOpen(open, restoreFocus);
}

byId("policy-picker-toggle").addEventListener("click", () => setPolicyPickerOpen(byId("policy-catalog-pane").hidden));
byId("policy-picker-close").addEventListener("click", () => setPolicyPickerOpen(false, true));
const policyHistory = policyEditorController.history;
byId("policy-source-surface").prepend(byId("policy-test-status"));
setPolicyPickerOpen(false);
window.ZPRAssistant.bindCollapse({
  pane: byId("policy-assistant-pane"), toggle: byId("policy-assistant-toggle"),
  layout: byId("policy-workbench"), storageKey: "zpr-policy-pane-assistant-collapsed",
  changed: updatePolicyWorkbenchLayout,
});
document.addEventListener("pointerdown", (event) => {
  if (!byId("policy-catalog-pane").hidden && !event.target.closest("#policy-catalog-pane, #policy-picker-toggle, #policy-picker-menu")) setPolicyPickerOpen(false);
});
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && !byId("policy-picker-menu").hidden) return;
  if (event.key === "Escape" && !byId("policy-catalog-pane").hidden) {
    setPolicyPickerOpen(false, true);
  }
});
const policyPicker = document.querySelector("#page-policy .policy-catalog-pane");
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
byId("policy-rename").addEventListener("click", () => managePolicyFile("rename"));
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
});
byId("policy-source").addEventListener("input", () => {
  invalidatePolicyAnalysis();
  clearPolicyTestResults();
  state.policy.checkDiagnostics = "";
  byId("policy-check-result").hidden = true;
  state.policy.evaluatedSource = null;
  state.policy.validSource = null;
  state.policy.saveTestSource = "";
  state.policy.saveTestError = "";
  state.policy.saveAsTestSource = "";
  state.policy.saveAsTestError = "";
  state.policy.errorOffsets = [];
  byId("policy-check-result").dataset.state = "";
  updatePolicyHighlight();
  updatePolicyDirtyState();
  showPolicyCompletions();
});
byId("policy-format").addEventListener("click", formatPolicySource);
policyEditorController.bind("analyze", { button: byId("policy-check") });
byId("policy-stage-cancel").addEventListener("click", () => byId("policy-stage-dialog").close());
byId("policy-stage-confirm").addEventListener("click", confirmPolicyStage);
byId("policy-test-details-close").addEventListener("click", hidePolicyTestDetails);
byId("policy-analyze-details-dismiss").addEventListener("click", hidePolicyTestDetails);
byId("policy-analyze-details-dismiss").addEventListener("click", hidePolicyTestDetails);
byId("policy-attribute-rescan").addEventListener("click", rescanPolicyAttributes);
byId("policy-draft-name").addEventListener("input", () => {
  if (!state.policy.record?.isDraft) return;
  state.policy.record.name = byId("policy-draft-name").value;
  renderPolicyIdentity(state.policy.record, 0, "");
  updatePolicyDirtyState();
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
policyEditorController.bind("save", { button: byId("policy-save"), shortcutRoot: byId("policy-source") });
byId("policy-stage").addEventListener("click", stageSelectedPolicy);
byId("pause-poll").addEventListener("click", (event) => {
  state.paused = !state.paused;
  event.currentTarget.textContent = state.paused ? "Resume updates" : "Pause updates";
  event.currentTarget.setAttribute("aria-pressed", String(state.paused));
  byId("pause-status").hidden = !state.paused;
  setPollTimer();
});
byId("poll-rate").addEventListener("change", setPollTimer);
function mapHighlightActive() {
  return graphVisaFocus != null || Boolean(state.mapLegendKind) || byId("topology-search").value.trim() !== "";
}
function syncMapClearHighlight() {
  byId("map-clear-highlight").hidden = !mapHighlightActive();
}
function clearMapHighlight() {
  const had = mapHighlightActive();
  graphVisaFocus = null;
  state.mapLegendKind = "";
  byId("topology-search").value = "";
  for (const item of document.querySelectorAll("[data-legend-kind]")) item.setAttribute("aria-pressed", "false");
  if (had && state.snapshot) renderTopology(state.snapshot);
  syncMapClearHighlight();
  return had;
}
byId("topology-search").addEventListener("input", () => { if (state.snapshot) renderTopology(state.snapshot); syncMapClearHighlight(); });
for (const item of document.querySelectorAll("[data-legend-kind]")) {
  item.addEventListener("click", () => {
    state.mapLegendKind = state.mapLegendKind === item.dataset.legendKind ? "" : item.dataset.legendKind;
    for (const other of document.querySelectorAll("[data-legend-kind]")) other.setAttribute("aria-pressed", String(other.dataset.legendKind === state.mapLegendKind));
    if (state.snapshot) renderTopology(state.snapshot);
    syncMapClearHighlight();
  });
}
byId("map-clear-highlight").addEventListener("click", () => {
  clearMapHighlight();
  closeInspector();
});
function inspectMapVisaCount(target) {
  const denialBadge = target.closest(".graph-denial-count");
  if (denialBadge && state.snapshot) {
    const node = denialBadge.closest(".graph-vertex[data-inspect-actor]");
    if (!node) return false;
    openInspector("denial-count", node.dataset.inspectActor);
    return true;
  }
  const badge = target.closest(".graph-visa-count:not(.graph-denial-count)");
  if (!badge || !state.snapshot) return false;
  const service = badge.closest(".graph-service-badge[data-inspect-service]");
  const component = badge.closest(".graph-vertex[data-inspect-actor]");
  if (!service && !component) return false;
  openInspector("visa-count", JSON.stringify(service
    ? { kind: "service", actorCN: service.dataset.serviceActor, serviceName: service.dataset.inspectService }
    : { kind: "actor", actorCN: component.dataset.inspectActor }));
  return true;
}
byId("topology-stage").addEventListener("keydown", event => {
  if (!["Enter", " "].includes(event.key) || !inspectMapVisaCount(event.target)) return;
  event.preventDefault();
  event.stopPropagation();
});
byId("topology-stage").addEventListener("click", event => {
  if (!inspectMapVisaCount(event.target)) return;
  event.stopPropagation();
});
function toggleMapVisaFocus(focus) {
  const same = graphVisaFocus?.kind === focus.kind && graphVisaFocus.actorCN === focus.actorCN && graphVisaFocus.serviceName === focus.serviceName;
  graphVisaFocus = same ? null : focus;
  applyMapVisaFocus(state.snapshot);
}
// Right-clicking an adapter or service (or its count badge) only toggles route focus;
// left-clicking the count badge opens the visa inventory.
byId("topology-stage").addEventListener("contextmenu", event => {
  if (!state.snapshot) return;
  const serviceBadge = event.target.closest(".graph-service-badge[data-inspect-service]");
  if (serviceBadge) {
    event.preventDefault();
    toggleMapVisaFocus({ kind: "service", actorCN: serviceBadge.dataset.serviceActor, serviceName: serviceBadge.dataset.inspectService });
    return;
  }
  const component = event.target.closest(".graph-vertex[data-inspect-actor]");
  const actor = component && state.snapshot.actors.find(actor => actor.cn === component.dataset.inspectActor);
  if (actor?.node) {
    event.preventDefault();
    const onVisaCount = event.target.closest(".graph-visa-count:not(.graph-denial-count)");
    if (!onVisaCount && component.querySelector(".graph-denial-count")) openInspector("denial-count", actor.cn);
    else openInspector("visa-count", JSON.stringify({ kind: "actor", actorCN: actor.cn }));
  } else if (actor) {
    event.preventDefault();
    toggleMapVisaFocus({ kind: "adapter", actorCN: actor.cn });
  } else if (!event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-link], .graph-controls")) {
    event.preventDefault();
    graphVisaFocus = null;
    applyMapVisaFocus(state.snapshot);
  }
});

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
  byId(filterId)?.addEventListener("input", () => state.snapshot && renderPage(state.snapshot));
}

for (const table of document.querySelectorAll("table[data-sort-page]")) {
  const page = table.dataset.sortPage;
  const renderPage = pageRenderers[page];
  window.ZPRSortableTable.bindSortableHeaders({
    table,
    getSort: () => state.sorts[page],
    onSort: (sort) => {
      state.sorts[page] = sort;
      if (renderPage && state.snapshot) renderPage(state.snapshot);
      else sortControlRoomTableRows(page);
    },
  });
}

mountPolicyAssertionEditor();
window.addEventListener("hashchange", () => {
  showPage();
  closeInspector();
});
showPage();

document.addEventListener("click", (event) => {
  const link = event.target.closest("a[data-reuse-window]");
  if (!link || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  const namedPage = window.open(link.href, link.dataset.reuseWindow);
  if (namedPage) {
    namedPage.opener = null;
    namedPage.focus();
  }
});

document.addEventListener("click", (event) => {
  if (event.target.closest(".source-editor-link")) return;
  const target = event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-source], [data-inspect-link]");
  if (!target) return;
  if (target.dataset.inspectActor) openInspector("actor", target.dataset.inspectActor);
  else if (target.dataset.inspectService) openInspector("service", target.dataset.inspectService);
  else if (target.dataset.inspectSource) openInspector("source", target.dataset.inspectSource);
  else openInspector("link", target.dataset.inspectLink);
});

document.addEventListener("pointerdown", (event) => {
  const inspector = byId("component-inspector");
  if (!inspector.classList.contains("open") || inspector.contains(event.target) ||
      event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-source], [data-inspect-link]")) return;
  closeInspector();
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

byId("inspector-close").addEventListener("click", () => closeInspector());
document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  const inspectorOpen = state.selection != null;
  closeInspector();
  if (currentPage() !== "map" || event.defaultPrevented) return;
  if (inspectorOpen) return;
  clearMapHighlight();
});

refresh();
setPollTimer();