const byId = (id) => document.getElementById(id);
const state = { snapshot: null, timer: null, paused: false, pending: false, graphCamera: null, graphNodeSlots: new Map(), graphAdapterSlots: new Map(), selection: null, sorts: {} };

const pages = {
  map: "MAP",
  connections: "CONNECTIONS",
  actors: "ACTORS",
  services: "SERVICES",
  sources: "TRUSTED SOURCES",
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
    sections.push(detailSection("Identity", [
      detailField("Role", actor.node ? "Node / forwarder" : kindLabel === "VISA SERVICE ADAPTER" ? "Visa Service adapter" : "Adapter"),
      detailField("Common name", actor.cn, "mono"),
      detailField("ZPR address", actor.zpr_addr, "mono"),
      detailField("Identity", actor.ident, "mono"),
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
    sections.push(detailSection("Configuration", [
      detailField("Kind", service.service_kind), detailField("Actor", service.actor_cn, "mono"),
      detailField("ZPR address", service.zpr_addr, "mono"),
      detailField("Endpoints", service.service_endpoints),
    ]));
    sections.push(detailSection("Live state", [
      detailField("Actor present", actor ? "Present in Visa Service" : "Not returned"),
      detailField("Provider health", isTrusted ? "Not exposed by admin API" : "Not applicable"),
      detailField("Health source", isTrusted ? "No trusted-source status endpoint is available" : "Service descriptors report configured state only"),
    ]));
  } else if (state.selection.kind === "source") {
    const source = data.trusted_sources.find((item) => item.name === state.selection.key);
    if (!source) return closeInspector();
    kindLabel = "TRUSTED SOURCE";
    title = source.name;
    sections.push(detailSection("Configuration", [detailField("Provider", source.provider), detailField("Actor", source.actor_cn, "mono")]));
    sections.push(detailSection("Live state", [detailField("Connection health", "Unreported"), detailField("Health source", "Not exposed by the admin API")]));
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
  const adapters = data.actors.filter((actor) => !actor.node).sort((a, b) => a.cn.localeCompare(b.cn));
  const actors = [...nodes, ...adapters];
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
  const width = Math.max(760, 520 + maxColumn * 520 + 280);
  const height = Math.max(460, 460 + maxRow * 500 + 240);
  const visaServices = new Set((data.services || []).filter((service) => service.service_kind === "Visa").map((service) => service.actor_cn));
  const query = byId("topology-search").value.trim().toLowerCase();
  const matches = (actor) => !query || `${actor.cn} ${actor.zpr_addr || ""} ${visaServices.has(actor.cn) ? "visa service" : ""}`.toLowerCase().includes(query);

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
    });
  });

  const unplaced = adapters.filter((adapter) => !positions.has(adapter.cn));
  unplaced.forEach((adapter, index) => {
    const angle = -Math.PI / 2 + (2 * Math.PI * (index % 8)) / 8;
    const center = { x: 380 + (index % columns) * 520, y: 230 + Math.floor(index / columns) * 500 };
    positions.set(adapter.cn, { x: center.x + Math.cos(angle) * 125, y: center.y + Math.sin(angle) * 125 });
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
        : `<circle class="graph-adapter" cx="${pos.x}" cy="${pos.y}" r="29"/>`;
    return `<g class="graph-vertex ${query && !matches(actor) ? "filtered" : ""}" data-inspect-actor="${escapeHTML(actor.cn)}" tabindex="0" role="button" aria-label="Inspect ${escapeHTML(actor.cn)}"><title>${escapeHTML(actor.cn)} · ${escapeHTML(actor.zpr_addr || "address pending")}</title>${glyph}<text class="graph-label" x="${pos.x}" y="${pos.y + 3}">${escapeHTML(shortName)}</text></g>`;
  }).join("");

  stage.innerHTML = `<div class="graph-controls" aria-label="Topology graph controls"><button class="graph-control" data-graph-action="in" type="button" aria-label="Zoom in" title="Zoom in">+</button><button class="graph-control" data-graph-action="out" type="button" aria-label="Zoom out" title="Zoom out">−</button><button class="graph-control graph-fit" data-graph-action="fit" type="button" aria-label="Fit graph" title="Fit graph">Fit</button><span class="graph-hint">DRAG TO PAN · SCROLL TO ZOOM</span></div><svg class="topology-graph" viewBox="0 0 ${width} ${height}" role="img" aria-label="Topology graph with ${nodes.length} nodes, ${adapters.length} adapters, ${dockEdges.length} dock connections, and ${networkEdges.length} inter-node links"><g id="graph-world">${edgeMarkup}${vertexMarkup}</g></svg>`;

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
    if (event.target.closest("[data-inspect-actor], [data-inspect-link]")) return;
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
  const columns = { cn: (actor) => actor.cn, role: (actor) => actor.node ? "node" : "adapter", address: (actor) => actor.zpr_addr, state: (actor) => nodeState(actor, data), identity: (actor) => actor.ident };
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
  byId("trusted-count").textContent = `${formatNumber(sources.length)} Trusted Services`;
  const columns = { name: (source) => source.name, provider: (source) => source.provider, actor: (source) => source.actor_cn, health: () => "UNREPORTED" };
  const shown = visibleRows("sources", sources, columns);
  byId("trusted-list").innerHTML = shown.length ? shown.map((source) =>
    `<tr class="selectable-row" data-inspect-source="${escapeHTML(source.name)}" tabindex="0" role="button" aria-label="Inspect trusted source ${escapeHTML(source.name)}"><td>${escapeHTML(source.name)}</td><td>${escapeHTML(source.provider || "—")}</td><td>${escapeHTML(source.actor_cn || "—")}</td><td><span class="health-badge">UNREPORTED</span></td></tr>`
  ).join("") : `<tr><td colspan="4" class="empty-row">${sources.length ? "No matching trusted sources" : "No trusted-source descriptors returned by the admin API."}</td></tr>`;
}

function renderServices(data) {
  const services = data.services || [];
  byId("service-count").textContent = `${formatNumber(services.length)} Services`;
  const columns = { name: (service) => service.service_name, kind: (service) => service.service_kind, actor: (service) => service.actor_cn, address: (service) => service.zpr_addr, endpoints: (service) => service.service_endpoints };
  const shown = visibleRows("services", services, columns);
  byId("service-rows").innerHTML = shown.length ? shown.map((service) => {
    const trusted = (service.service_kind || "").startsWith("Trusted(");
    const kind = service.service_kind || "Not in current policy";
    return `<tr class="selectable-row" data-inspect-service="${escapeHTML(service.service_name)}" tabindex="0" role="button" aria-label="Inspect service ${escapeHTML(service.service_name)}"><td>${escapeHTML(service.service_name || "—")}</td><td><span class="role-chip ${trusted ? "node" : ""}">${escapeHTML(trusted ? "TRUSTED SOURCE" : kind)}</span></td><td>${escapeHTML(service.actor_cn || "—")}</td><td class="mono">${escapeHTML(service.zpr_addr || "—")}</td><td>${escapeHTML(service.service_endpoints || "—")}</td></tr>`;
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
  const target = event.target.closest("[data-inspect-actor], [data-inspect-service], [data-inspect-source], [data-inspect-link]");
  if (!target) return;
  if (target.dataset.inspectActor) openInspector("actor", target.dataset.inspectActor);
  else if (target.dataset.inspectService) openInspector("service", target.dataset.inspectService);
  else if (target.dataset.inspectSource) openInspector("source", target.dataset.inspectSource);
  else openInspector("link", target.dataset.inspectLink);
});

document.addEventListener("keydown", (event) => {
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