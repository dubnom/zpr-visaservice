const byId = (id) => document.getElementById(id);
const zplKeywords = new Set(["allow", "never", "define", "with", "to", "access", "and", "as", "aka", "tag", "tags", "on", "optional", "multiple", "signal", "over"]);
const state = { snapshot: null, timer: null, paused: false, pending: false, graphCamera: null, graphNodeSlots: new Map(), graphAdapterSlots: new Map(), selection: null, sorts: {}, policy: { loaded: false, configured: false, categories: [], records: [], categoryID: "", record: null, source: "", savedSource: "", revision: 0, browsingRevision: 0, saveAs: false, compilerReady: false, assistantReady: false, validSource: null, errorOffsets: [], revisions: [], messages: [], assistantPending: false, assistantError: "" } };

const pages = {
  map: "MAP",
  connections: "CONNECTIONS",
  actors: "ACTORS",
  services: "SERVICES",
  sources: "TRUSTED SOURCES",
  policy: "POLICY",
  visas: "VISAS",
  denies: "DENIALS",
};

function currentPage() {
  const page = location.hash.replace(/^#/, "") || "map";
  return Object.hasOwn(pages, page) ? page : "map";
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
}

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  })[char]);
}

function num(value) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatNumber(value) {
  return num(value).toLocaleString();
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
  const dockedNames = new Set(nodes.flatMap((node) => node.node_details?.adapters || []));
  const approved = num(stats.visa_requests_approved);
  const denied = num(stats.visa_requests_denied);

  byId("metric-visas").textContent = formatNumber(data.visa_count);
  byId("metric-visas-note").textContent = `${formatNumber(data.recent_visas.length)} shown below`;
  byId("metric-nodes").textContent = `${inSync} / ${nodes.length}`;
  byId("metric-nodes-note").textContent = `${upLinks} inter-node links reported UP`;
  byId("metric-allowed").textContent = formatNumber(approved);
  byId("metric-denied").textContent = formatNumber(denied);
  byId("metric-actors").textContent = formatNumber(data.actors.length);
  byId("metric-adapters").textContent = formatNumber(data.actors.filter((actor) => !actor.node && dockedNames.has(actor.cn)).length);
  byId("metric-services").textContent = formatNumber(data.services.length);
  byId("metric-uptime").textContent = stats.uptime == null ? "—" : formatDuration(stats.uptime);
  for (const id of ["metric-visas-note", "metric-nodes-note"]) {
    byId(id).parentElement.title = byId(id).textContent;
  }
}

function detailField(label, value, className = "") {
  return `<div class="detail-field"><dt>${escapeHTML(label)}</dt><dd class="${className}">${escapeHTML(value == null || value === "" ? "—" : value)}</dd></div>`;
}

function detailHTMLField(label, markup) {
  return `<div class="detail-field"><dt>${escapeHTML(label)}</dt><dd>${markup || "—"}</dd></div>`;
}

function detailSection(title, fields) {
  return `<section class="detail-section"><h3>${escapeHTML(title)}</h3><dl>${fields.join("")}</dl></section>`;
}

function openInspector(kind, key) {
  state.selection = { kind, key };
  renderInspector();
}

function closeInspector() {
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
    const relatedVisas = data.recent_visas.filter((visa) => visa.source_addr === actor.zpr_addr || visa.dest_addr === actor.zpr_addr);
    kindLabel = actor.node ? "FORWARDING NODE" : services.some((item) => item.service_kind === "Visa") ? "VISA SERVICE ADAPTER" : "ADAPTER";
    title = actor.cn;
    sections.push(detailSection("Actor", [
      detailField("Role", actor.node ? "Node / forwarder" : kindLabel === "VISA SERVICE ADAPTER" ? "Visa Service adapter" : "Adapter"),
      detailField("Common name", actor.cn, "mono"),
      detailField("ZPR address", actor.zpr_addr, "mono"),
      detailField("Authentication expires", actor.auth_exp ? new Date(actor.auth_exp * 1000).toLocaleString() : "No expiry reported"),
    ]));
    if (actor.node && actor.node_details) {
      const details = actor.node_details;
      const attached = details.adapters || [];
      const outgoing = (details.links || []).map((name) => `<button class="detail-link" type="button" data-inspect-actor="${escapeHTML(name)}">${escapeHTML(name)}</button>`).join(" ");
      sections.push(detailSection("Live node state", [
        detailField("Synchronization", details.in_sync ? "In sync" : "Not in sync"),
        detailField("Last contact", since(details.last_contact)),
        detailField("Pending visa installs", details.pending_install),
        detailField("Pending revocations", details.pending_revocation),
        detailField("Visa requests", details.visa_requests),
        detailField("Approved / denied", `${details.approved_vreqs} / ${details.denied_vreqs}`),
        detailField("Installed visas", (details.visas || []).length),
        detailHTMLField("Docked adapters", attached.map((name) => `<button class="detail-link" type="button" data-inspect-actor="${escapeHTML(name)}">${escapeHTML(name)}</button>`).join(" ")),
        detailHTMLField("Node links", outgoing),
      ]));
    } else {
      const attachedTo = data.actors.filter((node) => node.node && (node.node_details?.adapters || []).includes(actor.cn));
      sections.push(detailSection("Live attachment", [
        detailField("Docked to", attachedTo.map((node) => node.cn).join(", ") || "No dock reported"),
        detailField("Recent visas involving actor", relatedVisas.length),
        detailField("Services registered", services.map((item) => item.service_name).join(", ") || "None"),
      ]));
    }
    if (services.length) {
      sections.push(`<section class="detail-section"><h3>Service registrations</h3><div class="detail-list">${services.map((service) => `<button class="detail-item" type="button" data-inspect-service="${escapeHTML(service.service_name)}"><strong>${escapeHTML(service.service_name)}</strong><span>${escapeHTML(service.service_kind)} · ${escapeHTML(service.service_endpoints || "no endpoints")}</span></button>`).join("")}</div></section>`);
    }
    if (relatedVisas.length) sections.push(`<section class="detail-section"><h3>Recent visas</h3><div class="detail-list">${relatedVisas.slice(0, 6).map((visa) => `<div class="detail-item"><strong>Visa ${escapeHTML(visa.id)} · ${escapeHTML(visa.proto)}</strong><span>${escapeHTML(endpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port))}</span></div>`).join("")}</div></section>`);
  } else if (state.selection.kind === "service") {
    const service = data.services.find((item) => item.service_name === state.selection.key);
    if (!service) return closeInspector();
    const isTrusted = (service.service_kind || "").startsWith("Trusted(");
    kindLabel = isTrusted ? "TRUSTED SOURCE" : "REGISTERED SERVICE";
    title = service.service_name;
    const actor = data.actors.find((item) => item.cn === service.actor_cn);
    const source = data.trusted_sources.find((item) => item.name === service.service_name);
    sections.push(detailSection("Configuration", [
      detailField("Kind", service.service_kind), detailField("Actor", service.actor_cn, "mono"),
      detailField("ZPR address", service.zpr_addr, "mono"),
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
      detailField("Service address", source.zpr_addr, "mono"),
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
    sections.push(detailSection("Endpoints", [detailField("From", fromName), detailField("From address", from.zpr_addr, "mono"), detailField("To", toName), detailField("To address", to.zpr_addr, "mono")]));
    sections.push(detailSection("Live state", kind === "dock" ? [
      detailField("Attachment", (from.node_details?.adapters || []).includes(toName) ? "Docked" : "Unreported"),
    ] : [
      detailField("Status", link.ctype), detailField("Link ID", link.link_id), detailField("Cost", link.link_cost),
      detailField("Substrate A", link.node_a_substrate, "mono"), detailField("Substrate B", link.node_b_substrate, "mono"),
    ]));
  }

  byId("inspector-kind").textContent = kindLabel;
  byId("inspector-title").textContent = title;
  body.innerHTML = sections.join("");
  panel.classList.add("open");
  panel.setAttribute("aria-hidden", "false");
}

function renderTopology(data) {
  const nodes = data.actors.filter((actor) => actor.node).sort((a, b) => a.cn.localeCompare(b.cn));
  const adapters = data.actors.filter((actor) => !actor.node && !actor.platform).sort((a, b) => a.cn.localeCompare(b.cn));
  const platformActors = data.actors.filter((actor) => actor.platform).sort((a, b) => a.cn.localeCompare(b.cn));
  const serviceHosts = [...adapters, ...platformActors];
  const actors = [...nodes, ...serviceHosts];
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
  const adapterOrigins = new Map();
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

  if (!actors.length) {
    stage.innerHTML = `<div class="empty-state">No nodes or adapters reported.</div>`;
    return;
  }

  const columns = Math.max(1, Math.ceil(Math.sqrt(nodes.length)));
  const activeNodes = new Set(nodes.map((node) => node.cn));
  for (const name of state.graphNodeSlots.keys()) {
    if (!activeNodes.has(name)) state.graphNodeSlots.delete(name);
  }
  const activeAdapters = new Set(adapters.map((adapter) => adapter.cn));
  for (const name of state.graphAdapterSlots.keys()) {
    if (!activeAdapters.has(name)) state.graphAdapterSlots.delete(name);
  }
  const usedSlots = new Set([...state.graphNodeSlots.values()].map(({ column, row }) => `${column}:${row}`));
  for (const node of nodes) {
    if (state.graphNodeSlots.has(node.cn)) continue;
    let slot = 0;
    while (usedSlots.has(`${slot % columns}:${Math.floor(slot / columns)}`)) slot++;
    const position = { column: slot % columns, row: Math.floor(slot / columns) };
    state.graphNodeSlots.set(node.cn, position);
    usedSlots.add(`${position.column}:${position.row}`);
  }
  const maxColumn = Math.max(0, ...[...state.graphNodeSlots.values()].map((position) => position.column));
  const maxRow = Math.max(0, ...[...state.graphNodeSlots.values()].map((position) => position.row));
  const unconnectedHosts = serviceHosts.filter((host) => !positions.has(host.cn));
  const unconnectedColumns = Math.max(1, Math.min(3, unconnectedHosts.length));
  const unconnectedRows = Math.ceil(unconnectedHosts.length / unconnectedColumns);
  const width = Math.max(760, 520 + maxColumn * 520 + 280, 160 + unconnectedColumns * 260);
  const height = Math.max(460, 460 + maxRow * 500 + 240, 520 + unconnectedRows * 190);
  const visaServices = new Set((data.services || []).filter((service) => service.service_kind === "Visa").map((service) => service.actor_cn));
  const query = byId("topology-search").value.trim().toLowerCase();
  const matches = (actor) => !query || `${actor.cn} ${actor.zpr_addr || ""} ${visaServices.has(actor.cn) ? "visa service" : ""} ${(servicesByActor.get(actor.cn) || []).map((service) => `${service.service_name} ${service.service_kind}`).join(" ")}`.toLowerCase().includes(query);

  nodes.forEach((node) => {
    const slot = state.graphNodeSlots.get(node.cn);
    const center = {
      x: 380 + slot.column * 520,
      y: 230 + slot.row * 500,
    };
    positions.set(node.cn, center);
    const attached = (node.node_details?.adapters || [])
      .map((name) => actors.find((actor) => !actor.node && actor.cn === name))
      .filter(Boolean)
      .sort((a, b) => a.cn.localeCompare(b.cn));
    const usedAdapterSlots = new Set(attached.map((adapter) => state.graphAdapterSlots.get(adapter.cn)).filter((slot) => slot !== undefined));
    attached.forEach((adapter) => {
      if (!state.graphAdapterSlots.has(adapter.cn)) {
        const order = [0, 4, 2, 6, 1, 3, 5, 7];
        let slot = order.find((candidate) => !usedAdapterSlots.has(candidate));
        if (slot === undefined) {
          slot = 8;
          while (usedAdapterSlots.has(slot)) slot++;
        }
        state.graphAdapterSlots.set(adapter.cn, slot);
        usedAdapterSlots.add(slot);
      }
      const slotIndex = state.graphAdapterSlots.get(adapter.cn);
      const adapterAngle = -Math.PI / 2 + (2 * Math.PI * (slotIndex % 8)) / 8;
      const adapterRadius = 140 + Math.floor(slotIndex / 8) * 90;
      positions.set(adapter.cn, {
        x: center.x + Math.cos(adapterAngle) * adapterRadius,
        y: center.y + Math.sin(adapterAngle) * adapterRadius,
      });
      adapterOrigins.set(adapter.cn, center);
    });
  });

  const unplaced = unconnectedHosts;
  unplaced.forEach((adapter, index) => {
    const column = index % unconnectedColumns;
    const row = Math.floor(index / unconnectedColumns);
    const center = { x: 160 + column * 260, y: 520 + row * 190 };
    positions.set(adapter.cn, center);
    adapterOrigins.set(adapter.cn, center);
  });

  const edgeMarkup = edges.map((edge) => {
    const from = positions.get(edge.from.cn), to = positions.get(edge.to.cn);
    const docked = edge.kind === "dock";
    const up = docked || edge.state === "UP";
    const cls = docked ? "dock-link" : up ? "up" : "down";
    const title = docked ? `${edge.to.cn} docked to ${edge.from.cn}` : `${edge.from.cn} to ${edge.to.cn}: ${edge.state}`;
    const filtered = query && !matches(edge.from) && !matches(edge.to) ? "filtered" : "";
    const key = `${edge.kind}|${edge.from.cn}|${edge.to.cn}`;
    return `<g class="graph-edge ${filtered}" data-inspect-link="${escapeHTML(key)}" tabindex="0" role="button" aria-label="Inspect ${escapeHTML(title)}"><title>${escapeHTML(title)}</title><line class="graph-link ${cls}" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"/><line class="graph-link-hit" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"/></g>`;
  }).join("");

  const vertexMarkup = actors.map((actor) => {
    const pos = positions.get(actor.cn);
    const shortName = actor.cn.length > 20 ? `${actor.cn.slice(0, 18)}…` : actor.cn;
    const isNode = actor.node;
    const isVisaService = !isNode && visaServices.has(actor.cn);
    const synced = isNode && actor.node_details?.in_sync;
    const glyph = isNode
      ? `<rect class="graph-node ${synced ? "synced" : ""}" x="${pos.x - 46}" y="${pos.y - 25}" width="92" height="50" rx="7"/>`
      : isVisaService
        ? `<polygon class="graph-visa" points="${pos.x},${pos.y - 35} ${pos.x + 35},${pos.y} ${pos.x},${pos.y + 35} ${pos.x - 35},${pos.y}"/>`
        : `<circle class="${actor.platform ? "graph-platform" : "graph-adapter"}" cx="${pos.x}" cy="${pos.y}" r="29"/>`;
    return `<g class="graph-vertex ${query && !matches(actor) ? "filtered" : ""}" data-inspect-actor="${escapeHTML(actor.cn)}" tabindex="0" role="button" aria-label="Inspect ${escapeHTML(actor.cn)}"><title>${escapeHTML(actor.cn)} · ${escapeHTML(actor.zpr_addr || "address pending")}</title>${glyph}<text class="graph-label" x="${pos.x}" y="${pos.y + 3}">${escapeHTML(shortName)}</text></g>`;
  }).join("");

  const serviceMarkup = actors.flatMap((adapter) => {
    const services = servicesByActor.get(adapter.cn) || [];
    if (!services.length) return [];
    const pos = positions.get(adapter.cn);
    const origin = adapterOrigins.get(adapter.cn) || pos;
    const dx = pos.x - origin.x, dy = pos.y - origin.y;
    const magnitude = Math.hypot(dx, dy);
    const outwardX = magnitude ? dx / magnitude : 0;
    const outwardY = magnitude ? dy / magnitude : -1;
    const visible = services.slice(0, 2);
    const entries = visible.map((service) => ({ service, overflow: false }));
    if (services.length > visible.length) entries.push({ service: null, overflow: services.length - visible.length });
    const badgeLayouts = entries.map(({ service, overflow }) => {
      const label = overflow ? `+${overflow}` : `${service.service_name} · ${service.service_kind.startsWith("Trusted(") ? "TRUSTED" : service.service_kind}`;
      const shortLabel = label.length > 19 ? `${label.slice(0, 18)}…` : label;
      const badgeWidth = Math.max(46, Math.min(132, shortLabel.length * 5.6 + 16));
      return { label, shortLabel, badgeWidth };
    });
    const maxBadgeWidth = Math.max(...badgeLayouts.map(({ badgeWidth }) => badgeWidth));
    const stackHeight = badgeLayouts.length * 20 + (badgeLayouts.length - 1) * 8;
    const glyphRadius = visaServices.has(adapter.cn) ? 35 : 29;
    const radialOffset = glyphRadius + Math.abs(outwardX) * maxBadgeWidth / 2 + Math.abs(outwardY) * stackHeight / 2 + 7;
    const stackCenterX = pos.x + outwardX * radialOffset;
    const stackCenterY = pos.y + outwardY * radialOffset;
    return entries.map(({ service, index, overflow }, entryIndex) => {
      const { label, shortLabel, badgeWidth } = badgeLayouts[entryIndex];
      const trustedType = service?.service_kind.match(/^Trusted\("([^\"]+)"\)$/)?.[1];
      const trustedClass = trustedType ? ` trusted-${trustedType.toLowerCase().replace(/[^a-z0-9]+/g, "-")}` : "";
      const stackOffset = (entryIndex - (entries.length - 1) / 2) * 28;
      const x = stackCenterX;
      const y = stackCenterY + stackOffset;
      const filtered = query && service && !`${service.service_name} ${service.service_kind}`.toLowerCase().includes(query) ? "filtered" : "";
      const inspect = overflow ? `data-inspect-actor="${escapeHTML(adapter.cn)}"` : `data-inspect-service="${escapeHTML(service.service_name)}"`;
      const title = overflow
        ? `${overflow} more services registered by ${adapter.cn}`
        : `${service.service_name} · ${service.service_kind} · registered by ${adapter.cn}`;
      const labelForScreenReader = overflow
        ? `Inspect ${overflow} more services registered by ${adapter.cn}`
        : `Inspect service ${service.service_name}, kind ${service.service_kind}, registered by ${adapter.cn}`;
      return `<g class="graph-service-badge${trustedClass} ${filtered}" ${inspect} tabindex="0" role="button" aria-label="${escapeHTML(labelForScreenReader)}"><title>${escapeHTML(title)}</title><rect x="${x - badgeWidth / 2}" y="${y - 10}" width="${badgeWidth}" height="20" rx="4"/><text x="${x}" y="${y + 3}">${escapeHTML(shortLabel)}</text></g>`;
    });
  }).join("");

  stage.innerHTML = `<div class="graph-controls" aria-label="Topology graph controls"><button class="graph-control" data-graph-action="in" type="button" aria-label="Zoom in" title="Zoom in">+</button><button class="graph-control" data-graph-action="out" type="button" aria-label="Zoom out" title="Zoom out">−</button><button class="graph-control graph-fit" data-graph-action="fit" type="button" aria-label="Fit graph" title="Fit graph">Fit</button><span class="graph-hint">DRAG TO PAN · SCROLL TO ZOOM</span></div><svg class="topology-graph" viewBox="0 0 ${width} ${height}" role="img" aria-label="Topology graph with ${nodes.length} nodes, ${adapters.length} adapters, ${platformActors.length} service hosts, ${dockEdges.length} dock connections, ${networkEdges.length} inter-node links, and ${(data.services || []).filter((service) => servicesByActor.has(service.actor_cn)).length} registered services"><g id="graph-world">${edgeMarkup}${vertexMarkup}${serviceMarkup}</g></svg>`;

  setupGraphControls(stage, width, height);
}

function renderConnections(edges, unconnected) {
  byId("connection-count").textContent = `${formatNumber(edges.length)} Connections`;
  const rows = [
    ...edges.map((edge) => ({
      from: edge.from.cn, to: edge.to.cn,
      type: edge.kind === "dock" ? "DOCK" : "INTER-NODE",
      address: edge.kind === "dock"
        ? `${edge.from.zpr_addr || "Address not assigned"} ↔ ${edge.to.zpr_addr || "Address not assigned"}`
        : [edge.detail.node_a_substrate, edge.detail.node_b_substrate].filter(Boolean).join(" ↔ ") || edge.detail.link_id || "No substrate details",
      state: edge.kind === "dock" ? "DOCKED" : edge.state,
      key: `${edge.kind}|${edge.from.cn}|${edge.to.cn}`,
    })),
    ...unconnected.map((actor) => ({ from: "—", to: actor.cn, type: "ADAPTER", address: actor.zpr_addr || "Address not assigned", state: "NO NODE", actor: actor.cn })),
  ];
  const columns = { from: (row) => row.from, to: (row) => row.to, type: (row) => row.type, address: (row) => row.address, state: (row) => row.state };
  const shown = visibleRows("connections", rows, columns);
  byId("link-list").innerHTML = shown.length ? shown.map((row) => {
    const target = row.actor ? `data-inspect-actor="${escapeHTML(row.actor)}"` : `data-inspect-link="${escapeHTML(row.key)}"`;
    return `<tr class="selectable-row" ${target} tabindex="0" role="button" aria-label="Inspect ${escapeHTML(row.from)} to ${escapeHTML(row.to)}"><td>${escapeHTML(row.from)}</td><td>${escapeHTML(row.to)}</td><td><span class="role-chip ${row.type === "INTER-NODE" ? "node" : ""}">${row.type}</span></td><td class="mono">${escapeHTML(row.address)}</td><td><span class="mini-state ${["UP", "DOCKED"].includes(row.state) ? "up" : ""}">${escapeHTML(row.state)}</span></td></tr>`;
  }).join("") : `<tr><td colspan="5" class="empty-row">${rows.length ? "No matching connections" : "No dock or inter-node connections reported"}</td></tr>`;
}

function setupGraphControls(stage, width, height) {
  const svg = stage.querySelector(".topology-graph");
  const world = stage.querySelector("#graph-world");
  if (!state.graphCamera) state.graphCamera = { x: 0, y: 0, scale: 1 };
  const camera = state.graphCamera;
  const apply = () => world.setAttribute("transform", `translate(${camera.x} ${camera.y}) scale(${camera.scale})`);
  const zoomAt = (nextScale, x = width / 2, y = height / 2) => {
    const scale = Math.max(0.55, Math.min(2.8, nextScale));
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
    if (action === "in") zoomAt(camera.scale * 1.2);
    if (action === "out") zoomAt(camera.scale / 1.2);
    if (action === "fit") {
      camera.x = 0;
      camera.y = 0;
      camera.scale = 1;
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
}

function nodeState(actor, data) {
  if (!actor.node) return "ADAPTER";
  const active = (data.network || []).some((link) => link.ctype === "UP" && [link.node_a_addr, link.node_b_addr].includes(actor.zpr_addr));
  if (active) return "LINK UP";
  return actor.node_details?.in_sync ? "IN SYNC" : "NO UP LINK";
}

function renderActors(data) {
  byId("actor-count").textContent = `${formatNumber(data.actors.length)} Actors`;
  const columns = { cn: (actor) => actor.cn, role: (actor) => actor.node ? "node" : "adapter", address: (actor) => actor.zpr_addr, state: (actor) => nodeState(actor, data) };
  const actors = visibleRows("actors", data.actors, columns);
  byId("actor-rows").innerHTML = actors.length ? actors.map((actor) => {
    const role = actor.node ? "node" : "adapter";
    const stateText = nodeState(actor, data);
    const up = stateText === "LINK UP" || stateText === "IN SYNC";
    const last = actor.node && actor.node_details ? ` · seen ${since(actor.node_details.last_contact)}` : "";
    return `<tr class="selectable-row" data-inspect-actor="${escapeHTML(actor.cn)}" tabindex="0" role="button" aria-label="Inspect actor ${escapeHTML(actor.cn)}"><td>${escapeHTML(actor.cn)}</td><td><span class="role-chip ${role}">${role}</span></td><td class="mono">${escapeHTML(actor.zpr_addr || "—")}</td><td><span class="mini-state ${up ? "up" : ""}">${stateText}${last}</span></td></tr>`;
  }).join("") : `<tr><td colspan="4" class="empty-row">No matching actors</td></tr>`;
}

function renderTrusted(data) {
  const sources = data.trusted_sources || [];
  byId("trusted-count").textContent = `${formatNumber(sources.length)} attribute sources`;
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
    Object.assign(policy, {
      loaded: true,
      configured: data.configured,
      categories: data.categories || [],
      records: data.records || [],
      compilerReady: data.compiler_ready,
      assistantReady: data.assistant_ready,
    });
    if (!policy.categories.some((category) => category.id === policy.categoryID)) {
      const policyCategories = new Set(policy.records.filter((record) => record.kind === "policy").map((record) => record.category_id));
      policy.categoryID = policy.categories.find((category) => policyCategories.has(category.id))?.id
        || policy.categories.find((category) => !category.parent_id)?.id
        || policy.categories[0]?.id
        || "";
    }
    if (policy.record && !policy.record.isDraft && !policy.records.some((record) => record.id === policy.record.id)) policy.record = null;
    renderPolicyCatalog();
    if (policy.record?.isDraft) {
      byId("policy-source").disabled = false;
      renderPolicyIdentity(policy.record, 0, "");
      updatePolicyDirtyState();
    } else if (policy.record && hasUnsavedChanges) {
      byId("policy-check-result").textContent = "Unsaved edits retained. Evaluate before saving.";
    } else if (policy.record) await selectPolicyRecord(policy.record.id, true, true);
    else {
      const firstPolicy = policy.records.find((record) => record.category_id === policy.categoryID && record.kind === "policy");
      if (firstPolicy) await selectPolicyRecord(firstPolicy.id, true, true);
      else clearPolicySelection();
    }
    byId("policy-check").disabled = !policy.configured || !policy.compilerReady;
    byId("new-category").disabled = !policy.configured;
    byId("new-policy-record").disabled = !policy.configured || !policy.categoryID;
    byId("assistant-state").textContent = policy.assistantReady ? "Ready" : "Not configured";
    byId("assistant-state").classList.toggle("ready", policy.assistantReady);
    byId("assistant-question").disabled = !policy.configured || !policy.assistantReady;
    byId("assistant-send").disabled = !policy.configured || !policy.assistantReady;
    byId("assistant-disclosure").textContent = policy.assistantReady
      ? "Submitting sends the current policy and chat history to Anthropic. Suggestions are not applied automatically."
      : "Claude is off. Set ANTHROPIC_API_KEY on the server to enable it.";
  } catch (error) {
    policy.loaded = false;
    byId("policy-check-result").textContent = error.message;
  }
}

function renderPolicyCatalog() {
  const policy = state.policy;
  const categories = new Map(policy.categories.map((category) => [category.id, category]));
  const children = new Map();
  for (const category of policy.categories) {
    const key = category.parent_id || "";
    if (!children.has(key)) children.set(key, []);
    children.get(key).push(category);
  }
  for (const items of children.values()) items.sort((a, b) => a.name.localeCompare(b.name));
  const tree = byId("policy-category-tree");
  tree.replaceChildren();
  if (!policy.categories.length) {
    const empty = document.createElement("p"); empty.className = "catalog-empty"; empty.textContent = "No categories yet."; tree.append(empty);
  } else {
    const renderBranch = (parentID = "", depth = 0) => {
      for (const category of children.get(parentID) || []) {
        const button = document.createElement("button");
        button.type = "button"; button.className = "category-tree-item";
        button.dataset.categoryId = category.id;
        button.setAttribute("role", "treeitem");
        button.setAttribute("aria-label", category.path);
        button.title = category.path;
        button.setAttribute("aria-selected", String(policy.categoryID === category.id));
        button.style.setProperty("--tree-depth", depth);
        const marker = document.createElement("span"); marker.className = "tree-marker"; marker.setAttribute("aria-hidden", "true");
        const name = document.createElement("span"); name.textContent = category.name;
        const count = policy.records.filter((record) => record.category_id === category.id).length;
        const tally = document.createElement("small"); tally.textContent = String(count);
        button.append(marker, name, tally);
        tree.append(button);
        renderBranch(category.id, depth + 1);
      }
    };
    renderBranch();
  }
  const selected = categories.get(policy.categoryID);
  byId("new-policy-record").disabled = !policy.configured || !selected;
  const list = byId("policy-record-list");
  list.replaceChildren();
  const records = policy.records.filter((record) => record.category_id === policy.categoryID).sort((a, b) => a.name.localeCompare(b.name));
  if (!selected) {
    const empty = document.createElement("p"); empty.className = "catalog-empty"; empty.textContent = "Create or select a category."; list.append(empty);
  } else if (!records.length) {
    const empty = document.createElement("p"); empty.className = "catalog-empty"; empty.textContent = "No records in this category."; list.append(empty);
  } else {
    for (const record of records) {
      const button = document.createElement("button"); button.type = "button"; button.className = "policy-record-item";
      button.dataset.recordId = record.id; button.setAttribute("aria-pressed", String(policy.record?.id === record.id));
      const name = document.createElement("strong"); name.textContent = record.name;
      const meta = document.createElement("small"); meta.textContent = `${record.kind} · v${record.current_revision}`;
      button.append(name, meta); list.append(button);
    }
  }
}

function clearPolicySelection() {
  const policy = state.policy;
  policy.record = null; policy.source = ""; policy.savedSource = ""; policy.revision = 0; policy.revisions = []; policy.validSource = null;
  policy.errorOffsets = [];
  policy.browsingRevision = 0;
  byId("policy-source").value = ""; byId("policy-source").disabled = true;
  updatePolicyHighlight(); hidePolicyCompletions();
  renderPolicyIdentity(null);
  byId("policy-record-title").hidden = false;
  byId("policy-draft-name").hidden = true;
  byId("policy-draft-name").value = "";
  byId("policy-check").disabled = true; byId("policy-save").disabled = true; byId("policy-save-as").disabled = true; byId("policy-refresh").disabled = true;
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
    policy.record = record; policy.source = record.content || ""; policy.savedSource = policy.source; policy.revision = record.current_revision; policy.validSource = null;
    policy.errorOffsets = [];
    policy.browsingRevision = 0;
    byId("policy-draft-name").hidden = true;
    byId("policy-draft-name").value = "";
    byId("policy-source").value = policy.source; byId("policy-source").disabled = record.kind !== "policy";
    updatePolicyHighlight(); hidePolicyCompletions();
    renderPolicyIdentity(record, record.current_revision, record.content_hash);
    byId("policy-check").disabled = record.kind !== "policy" || !policy.compilerReady;
    byId("policy-save").disabled = true;
    byId("policy-check-result").textContent = record.kind === "policy" ? "Evaluate the current policy before saving." : `Record type: ${record.kind}`;
    byId("policy-check-result").dataset.state = "";
    byId("assistant-question").disabled = record.kind !== "policy" || !policy.assistantReady;
    byId("assistant-send").disabled = record.kind !== "policy" || !policy.assistantReady;
    policy.messages = []; policy.assistantError = ""; renderAssistantMessages();
    renderPolicyCatalog(); await loadPolicyHistory(record.id);
    updatePolicyDirtyState();
  } catch (error) {
    byId("policy-check-result").textContent = error.message;
  }
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
  const category = state.policy.categories.find((item) => item.id === record.category_id);
  const path = [category?.path, record.name].filter(Boolean).join("/");
  title.hidden = Boolean(record.isDraft);
  const draftNameValue = record.isDraft ? record.name : "";
  if (draftName.value !== draftNameValue) draftName.value = draftNameValue;
  title.textContent = record.isDraft ? "" : record.name;
  title.title = path;
  byId("policy-revision-label").textContent = record.isDraft ? "New · unsaved" : `Version ${version} [${hash ? hash.slice(0, 12) : "hash unavailable"}]`;
  byId("policy-revision-label").title = record.isDraft ? "In-memory policy draft" : `${path} · ${hash || "hash unavailable"}`;
}

function hasUnsavedPolicyChanges(policy = state.policy, source = byId("policy-source").value) {
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
    for (const match of diagnostic.matchAll(/\[\s*line\s+(\d+)\s*,\s*column\s+(\d+)\s*\]/gi)) {
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
  const line = prefix.slice(prefix.lastIndexOf("\n") + 1);
  const wordMatch = line.match(/[\p{L}\p{N}_-]+$/u);
  const word = wordMatch?.[0] || "";
  const trimmed = line.trimStart();
  let candidates;
  let filterPrefix = word;
  if (/^[\p{L}\p{N}_-]*$/u.test(trimmed)) {
    candidates = ["allow", "never allow", "define"];
    filterPrefix = trimmed;
  } else if (/^never\s+[\p{L}\p{N}_-]*$/i.test(trimmed)) {
    candidates = ["allow"];
  } else if (/^define\s+\S+\s+(?:aka\s+\S+\s+)?as\s+(?:a\s+|an\s+)?[\p{L}\p{N}_-]*$/i.test(trimmed)) {
    candidates = ["user", "device", "service", ...definedPolicyClasses(source)];
  } else if (/^define\s+\S+\s+(?:aka\s+\S+\s+)?$/i.test(trimmed)) {
    candidates = ["as"];
  } else if (/\bto\s*[^\s]*$/i.test(line)) {
    candidates = ["access"];
  } else if (/\baccess\s+[^\s]*$/i.test(line)) {
    candidates = [...policyClassReferences(source), "services", "on", "over"];
  } else if (/^\s*(?:allow|never\s+allow)\s+[^\n]*$/i.test(line) && !/\bto\b/i.test(line)) {
    candidates = ["users", "devices", "services", ...definedPolicyClasses(source).flatMap((name) => [pluralPolicyClass(name), name]), "to"];
  } else {
    candidates = ["with", "and", "to", "access", "on", "over", "signal", "tag", "tags", "optional", "multiple"];
  }
  const unique = [...new Set(candidates)];
  const filtered = filterPrefix ? unique.filter((item) => item.toLowerCase().startsWith(filterPrefix.toLowerCase()) && item.toLowerCase() !== filterPrefix.toLowerCase()) : unique;
  return { word, start: cursor - word.length, items: filtered.slice(0, 8) };
}

function hidePolicyCompletions() {
  const menu = byId("policy-completions");
  menu.hidden = true;
  menu.replaceChildren();
  menu.removeAttribute("data-start");
  menu.removeAttribute("data-end");
}

function showPolicyCompletions(force = false) {
  const textarea = byId("policy-source");
  if (textarea.disabled || textarea.selectionStart !== textarea.selectionEnd) return hidePolicyCompletions();
  const suggestions = completionCandidates(textarea.value, textarea.selectionStart);
  if (!suggestions.items.length && !force) return hidePolicyCompletions();
  if (!suggestions.items.length) return hidePolicyCompletions();
  const menu = byId("policy-completions");
  menu.replaceChildren();
  menu.dataset.start = String(suggestions.start);
  menu.dataset.end = String(textarea.selectionStart);
  suggestions.items.forEach((item, index) => {
    const option = document.createElement("button");
    option.type = "button";
    option.className = "completion-option";
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", String(index === 0));
    option.textContent = item;
    option.addEventListener("mousedown", (event) => event.preventDefault());
    option.addEventListener("click", () => acceptPolicyCompletion(index));
    menu.append(option);
  });
  menu.hidden = false;
}

function acceptPolicyCompletion(index = 0) {
  const textarea = byId("policy-source");
  const menu = byId("policy-completions");
  const options = [...menu.querySelectorAll(".completion-option")];
  const option = options[index];
  if (!option) return;
  const start = Number(menu.dataset.start);
  const end = Number(menu.dataset.end);
  const insertion = option.textContent;
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
  for (const category of state.policy.categories) {
    const option = document.createElement("option"); option.value = category.id; option.textContent = category.path; option.selected = category.id === state.policy.categoryID; options.append(option);
  }
  byId("category-name").value = ""; byId("category-error").textContent = "";
  byId("category-dialog").showModal(); byId("category-name").focus();
}

function openRecordDialog(saveAs = true) {
  const options = byId("record-category"); options.replaceChildren();
  for (const category of state.policy.categories) {
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
  byId("assistant-question").disabled = !policy.assistantReady;
  byId("assistant-send").disabled = !policy.assistantReady;
  renderPolicyCatalog();
  updatePolicyDirtyState();
  byId("policy-source").focus();
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
  const canEdit = policy.configured && policy.record?.kind === "policy" && !policy.browsingRevision;
  const isDraft = Boolean(policy.record?.isDraft);
  const canSave = canEdit && checked && (isDraft ? Boolean(policy.record.name.trim()) && source.trim() !== "" : contentDirty);
  byId("policy-save").disabled = !canSave;
  byId("policy-save-as").disabled = !canEdit || isDraft || !checked;
  byId("policy-check").disabled = !canEdit || !policy.compilerReady;
  byId("policy-refresh").disabled = !policy.record || (!dirty && !policy.browsingRevision && !isDraft);
  byId("policy-check").classList.toggle("button-next-evaluate", canEdit && !checked);
  byId("policy-save").classList.toggle("button-save-next", !canSave);
  byId("policy-save").classList.toggle("button-next-save", canSave);
  byId("policy-save-as").classList.toggle("button-save-as-ready", canEdit && !isDraft && checked);
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
      body: JSON.stringify({ source: byId("policy-source").value, messages: policy.messages }),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `Claude request failed (${response.status})`);
    policy.messages.push({ role: "assistant", content: result.answer });
  } catch (error) {
    policy.assistantError = error.message;
  } finally {
    policy.assistantPending = false;
    byId("assistant-send").disabled = !policy.assistantReady;
    renderAssistantMessages();
  }
}

function renderServices(data) {
  const services = data.services || [];
  byId("service-count").textContent = `${formatNumber(services.length)} Services`;
  const columns = { name: (service) => service.service_name, kind: (service) => service.service_kind, actor: (service) => service.actor_cn, address: (service) => service.zpr_addr, endpoints: (service) => service.service_endpoints };
  const shown = visibleRows("services", services, columns);
  byId("service-rows").innerHTML = shown.length ? shown.map((service) => {
    const trustedType = (service.service_kind || "").match(/^Trusted\("([^"]+)"\)$/)?.[1];
    const trustedClass = trustedType ? ` trusted-${trustedType.toLowerCase().replace(/[^a-z0-9]+/g, "-")}` : "";
    const kind = service.service_kind || "Not in current policy";
    const kindLabel = trustedType ? `TRUSTED ${trustedType.toUpperCase()}` : kind;
    return `<tr class="selectable-row" data-inspect-service="${escapeHTML(service.service_name)}" tabindex="0" role="button" aria-label="Inspect service ${escapeHTML(service.service_name)}"><td>${escapeHTML(service.service_name || "—")}</td><td><span class="role-chip${trustedClass}">${escapeHTML(kindLabel)}</span></td><td>${escapeHTML(service.actor_cn || "—")}</td><td class="mono">${escapeHTML(service.zpr_addr || "—")}</td><td>${escapeHTML(service.service_endpoints || "—")}</td></tr>`;
  }).join("") : `<tr><td colspan="5" class="empty-row">${services.length ? "No matching services" : "No network services returned"}</td></tr>`;
}

function renderVisas(data) {
  byId("visa-total").textContent = `${formatNumber(data.visa_count)} Visas`;
  const rows = data.recent_visas || [];
  const columns = { id: (visa) => num(visa.id), flow: (visa) => endpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port), proto: (visa) => visa.proto, node: (visa) => visa.requesting_node, expires: (visa) => num(visa.expires) };
  const shown = visibleRows("visas", rows, columns);
  byId("visa-rows").innerHTML = shown.length ? shown.map((visa) => {
    const flow = endpoint(visa.source_addr, visa.source_port, visa.dest_addr, visa.dest_port);
    return `<tr><td class="mono">${escapeHTML(visa.id)}</td><td class="mono">${escapeHTML(flow)}</td><td>${escapeHTML(visa.proto || "—")}</td><td>${escapeHTML(visa.requesting_node || "—")}</td><td class="mono">${escapeHTML(new Date(num(visa.expires) * 1000).toLocaleTimeString())}</td></tr>`;
  }).join("") : `<tr><td colspan="5" class="empty-row">${rows.length ? "No matching visas" : "No active visas returned"}</td></tr>`;
}

function protocolName(number) {
  return ({ 1: "ICMP", 6: "TCP", 17: "UDP", 58: "ICMPv6" })[number] || `IP ${number}`;
}

function renderDenies(data) {
  const denies = data.recent_denies || [];
  const total = denies.reduce((sum, record) => sum + num(record.count), 0);
  byId("deny-total").textContent = `${formatNumber(total)} Denys`;
  const columns = { source: (record) => record.source_addr, destination: (record) => record.dest_addr, protocol: (record) => `${protocolName(record.protocol)}/${record.dest_port}`, reason: (record) => record.deny_code, count: (record) => num(record.count) };
  const shown = visibleRows("denies", denies, columns);
  byId("deny-list").innerHTML = shown.length ? shown.map((record) =>
    `<tr><td class="mono">${escapeHTML(record.source_addr)}</td><td class="mono">${escapeHTML(record.dest_addr)}</td><td>${escapeHTML(columns.protocol(record))}</td><td>${escapeHTML(record.deny_code)}</td><td class="mono deny-count">${formatNumber(record.count)}</td></tr>`
  ).join("") : `<tr><td colspan="5" class="empty-row">${denies.length ? "No matching denials" : "No recent policy denials"}</td></tr>`;
}

function render(data) {
  state.snapshot = data;
  updateConnection(data);
  renderMetrics(data);
  renderTopology(data);
  renderActors(data);
  renderServices(data);
  renderTrusted(data);
  renderVisas(data);
  renderDenies(data);
  if (state.selection) renderInspector();
}

async function refresh() {
  if (state.pending) return;
  state.pending = true;
  byId("refresh-now").disabled = true;
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
byId("policy-refresh").addEventListener("click", reloadPolicyWorkspace);
byId("new-category").addEventListener("click", openCategoryDialog);
byId("new-policy-record").addEventListener("click", beginNewPolicyDraft);
byId("policy-save-as").addEventListener("click", () => openRecordDialog(true));
byId("category-form").addEventListener("submit", createPolicyCategory);
byId("record-form").addEventListener("submit", createPolicyRecord);
document.addEventListener("click", (event) => {
  const close = event.target.closest("[data-close-dialog]");
  if (close) byId(close.dataset.closeDialog).close();
  const category = event.target.closest("[data-category-id]");
  if (category) { state.policy.categoryID = category.dataset.categoryId; renderPolicyCatalog(); }
  const record = event.target.closest("[data-record-id]");
  if (record) selectPolicyRecord(record.dataset.recordId);
  const revision = event.target.closest("[data-revision]");
  if (revision) browsePolicyRevision(Number(revision.dataset.revision));
});
byId("policy-source").addEventListener("input", () => {
  state.policy.validSource = null;
  state.policy.errorOffsets = [];
  byId("policy-check-result").dataset.state = "";
  updatePolicyHighlight();
  updatePolicyDirtyState();
  showPolicyCompletions();
});
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
});
byId("policy-source").addEventListener("click", () => showPolicyCompletions());
byId("policy-source").addEventListener("keydown", (event) => {
  const menu = byId("policy-completions");
  const open = !menu.hidden;
  if ((event.ctrlKey || event.metaKey) && event.code === "Space") {
    event.preventDefault(); showPolicyCompletions(true); return;
  }
  if (open && event.key === "ArrowDown") { event.preventDefault(); movePolicyCompletion(1); return; }
  if (open && event.key === "ArrowUp") { event.preventDefault(); movePolicyCompletion(-1); return; }
  if (open && (event.key === "Tab" || event.key === "Enter")) {
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
byId("assistant-form").addEventListener("submit", (event) => {
  event.preventDefault();
  const question = byId("assistant-question").value.trim();
  if (!question || !state.policy.assistantReady || state.policy.assistantPending) return;
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