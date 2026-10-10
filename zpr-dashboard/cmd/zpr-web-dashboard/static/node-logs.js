(async () => {
  const host = document.getElementById("node-logs");
  if (!host) return;
  const infoHost = document.getElementById("node-source-info");
  const { createLogPanel, updateLogPanel, setLogPanelMaximized } = await import("/log-panel.js?v=1");
  const { renderSourceMetrics } = await import("/source-metrics.js?v=1");
  const details = document.createElement("section");
  details.className = "node-source-details";
  details.setAttribute("aria-label", "Selected node source health and metrics");
  const metricSorts = new Map();
  const tracker = window.ZPRPollingDisplay.createTracker();
  const selectedTab = () => document.querySelector("#node-stats-nav [role=tab][aria-selected=true]");
  const selectedNodeID = () => selectedTab()?.dataset.nodeId || "";
  const selectedNodeName = () => selectedTab()?.textContent || selectedNodeID();
  const view = createLogPanel("Node logs");
  view.panel.classList.add("node-log-panel", "logs-nowrap");
  const status = document.createElement("small");
  status.setAttribute("role", "status");
  const identity = document.createElement("div");
  identity.append(view.title, status);
  view.header.prepend(identity);
  const pause = document.createElement("button");
  pause.type = "button";
  pause.textContent = "Pause";
  pause.setAttribute("aria-label", "Pause node log updates");
  pause.setAttribute("aria-pressed", "false");
  const refresh = document.createElement("button");
  refresh.type = "button";
  refresh.textContent = "Refresh";
  refresh.setAttribute("aria-label", "Refresh node logs");
  view.actions.prepend(pause, refresh);
  const toolbar = document.createElement("div");
  toolbar.className = "machine-logs-toolbar node-logs-toolbar";
  const wrapLabel = document.createElement("label");
  const wrap = document.createElement("input");
  wrap.type = "checkbox";
  wrapLabel.append(wrap, "Wrap lines");
  toolbar.append(wrapLabel);
  view.output.before(toolbar);
  const cache = new Map();
  const metricCache = new Map();
  const positions = new Map();
  let nodeID = "";
  let active = false;
  let paused = false;
  let pending = false;
  let source;
  let failure = "";

  function render(trackChanges = false) {
    const name = selectedNodeName() || nodeID;
    view.title.textContent = `Logs for ${name}`;
    view.output.setAttribute("aria-label", `${nodeID} node logs`);
    window.ZPRSafeDisplay.renderWindowControl(view.maximizeButton, view.panel.classList.contains("maximized"), `node logs for ${name}`);
    view.panel.hidden = !nodeID;
    if (!nodeID || !active) return;
    status.textContent = pending && !cache.has(nodeID) ? "Loading" : paused ? "Paused" : failure ? "Unavailable" : source?.state || "";
    refresh.disabled = pending;
    const lastGood = cache.get(nodeID);
    const unavailable = failure || source?.error || (source?.state === "unavailable" ? "Node log source unavailable." : "");
    details.replaceChildren();
    const heading = document.createElement("h3");
    heading.textContent = `Source health for ${name}`;
    const health = document.createElement("dl");
    health.className = "node-stats-fields";
    const sample = unavailable ? metricCache.get(nodeID) : source;
    const changes = new Map();
    const missing = value => value == null || value === "" ? "Not reported" : String(value);
    const updated = sample?.last_updated;
    const timestamp = !updated ? "Not reported" : Number.isFinite(Date.parse(updated)) ? window.ZPRSafeDisplay.formatDateTime(updated) : "Invalid timestamp";
    for (const [label, value] of [
      ["Source", missing(source?.name || sample?.name || `node:${nodeID}`)],
      ["State", failure ? "Unavailable" : missing(source?.state)],
      ["Identity", missing(source?.identity || sample?.identity)],
      ["Address", missing(source?.address || sample?.address)],
      ["Last update", timestamp],
      ["Sample", unavailable ? sample ? "Last known; source unavailable" : "Not available" : source?.state === "stale" ? "Last known; source stale" : pending ? "Loading" : source?.state === "partial" ? "Partial" : sample ? "Current" : "Not available"],
    ]) {
      const term = document.createElement("dt");
      term.textContent = label;
      const field = document.createElement("dd");
      field.textContent = value;
      changes.set(JSON.stringify([nodeID, label]), { node: field, value });
      health.append(term, field);
    }
    details.append(heading, health);
    if (unavailable || source?.state === "stale") {
      const error = document.createElement("p");
      error.className = "node-stats-error";
      error.setAttribute("role", "status");
      error.textContent = unavailable || "Source sample is stale; displayed metrics and logs are last known.";
      details.append(error);
    }
    if (sample) renderSourceMetrics(details, sample, {
      sort: metricSorts.get(nodeID) || { key: "name", direction: 1 },
      onSort: sort => { metricSorts.set(nodeID, sort); render(); },
      track: (fields, node, value) => changes.set(JSON.stringify([nodeID, ...fields]), { node, value }),
    });
    if (trackChanges) tracker.update(changes);
    const lines = source?.logs?.length ? source.logs.map(log => log.body == null || log.body === "" ? "No log body reported." : log.body)
      : unavailable && lastGood ? lastGood.logs.map(log => log.body == null || log.body === "" ? "No log body reported." : log.body) : [];
    if (host.hidden) return;
    updateLogPanel(view, source || unavailable ? {
      lines, disconnected: Boolean(unavailable),
      error: unavailable ? `${unavailable}${lastGood ? " Displayed logs are last known." : ""}`
        : source?.state === "stale" ? "Log sample is stale; displayed logs are last known." : "",
    } : null, pending ? "Loading" : paused ? "Logs paused. Refresh to load selected node." : "No node logs available.", { independentRecords: true });
  }

  const poller = window.ZPRPageRuntime.createPoller({
    async run({ signal, isCurrent }) {
      const requestedNode = nodeID;
      const result = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, `/api/diagnostics?source=${encodeURIComponent(`node:${requestedNode}`)}`, { signal, headers: { Accept: "application/json" } });
      if (!isCurrent() || requestedNode !== nodeID) return;
      if (!Array.isArray(result?.sources)) throw new Error("Node logs response has no source inventory.");
      const next = result.sources.find(candidate => candidate?.id === `node:${requestedNode}` && candidate.kind === "ZPR node");
      if (next && (!Array.isArray(next.logs) || next.logs.some(log => !log || typeof log !== "object" || log.body != null && typeof log.body !== "string"))) {
        throw new Error("Node logs response contains invalid log records.");
      }
      if (next && (!Array.isArray(next.metrics) || next.metrics.some(metric => !metric || typeof metric !== "object" ||
          typeof metric.name !== "string" || !["string", "number"].includes(typeof metric.value) && metric.value != null ||
          metric.unit != null && typeof metric.unit !== "string"))) {
        throw new Error("Node source response contains invalid metrics.");
      }
      source = next;
      failure = result.error || (!next ? "Selected node is absent from the diagnostics source inventory." : "");
      if (next && (next.logs.length || !failure && !next.error && ["available", "stale"].includes(next.state))) cache.set(nodeID, next);
      if (next && !failure && !next.error && ["available", "stale"].includes(next.state)) metricCache.set(nodeID, next);
    },
    onPending(value) { pending = value; render(!value); },
    onError(error) {
      failure = error.message || "Node logs unavailable.";
      source = undefined;
    },
  });

  function synchronize() {
    const nextID = selectedNodeID();
    const selectionChanged = nextID !== nodeID;
    const nextActive = location.hash === "#node-stats" && Boolean(nextID);
    if (selectionChanged || !nextActive) {
      if (active && nodeID && !host.hidden) positions.set(nodeID, { scrollTop: view.output.scrollTop, following: view.following });
      poller.stop();
      setLogPanelMaximized(view, false, "Node logs");
    }
    if (selectionChanged) {
      tracker.reset();
      nodeID = nextID;
      source = cache.get(nodeID);
      failure = "";
    }
    if (nextActive && (selectionChanged || !active)) {
      const saved = positions.get(nodeID);
      view.following = saved?.following ?? true;
      view.nextScrollTop = saved?.scrollTop ?? 0;
    }
    active = nextActive;
    if (active) {
      if (!view.panel.isConnected) {
        infoHost.append(details);
        host.append(view.panel);
      }
    } else {
      details.remove();
      view.panel.remove();
    }
    render();
    if (active) poller.start();
  }

  window.addEventListener("node-stats-selection-changed", synchronize);
  window.addEventListener("node-stats-area-changing", () => {
    if (active && nodeID && !host.hidden) {
      positions.set(nodeID, { scrollTop: view.output.scrollTop, following: view.following });
      setLogPanelMaximized(view, false, "Node logs", false);
    }
  });
  window.addEventListener("node-stats-area-changed", () => {
    if (!host.hidden) {
      const saved = positions.get(nodeID);
      view.following = saved?.following ?? true;
      view.nextScrollTop = saved?.scrollTop ?? 0;
    }
    render();
  });
  window.addEventListener("zpr-snapshot", () => {
    const present = new Set([...document.querySelectorAll("#node-stats-nav [role=tab]")].map(tab => tab.dataset.nodeId));
    for (const id of cache.keys()) if (!present.has(id)) cache.delete(id);
    for (const id of metricCache.keys()) if (!present.has(id)) metricCache.delete(id);
    for (const id of positions.keys()) if (!present.has(id)) positions.delete(id);
    for (const id of metricSorts.keys()) if (!present.has(id)) metricSorts.delete(id);
    synchronize();
  });
  window.addEventListener("hashchange", synchronize);
  window.addEventListener("operator-session-cleared", () => {
    poller.stop();
    cache.clear();
    metricCache.clear();
    positions.clear();
    metricSorts.clear();
    tracker.reset();
    source = undefined;
    failure = "Operator session ended. Sign in to load node source details.";
    view.output.replaceChildren();
    view.signature = "";
    setLogPanelMaximized(view, false, "Node logs");
    render();
  });
  document.addEventListener("control-room:refreshed", () => { if (active && !paused) void poller.refresh(); });
  pause.addEventListener("click", () => {
    paused = !paused;
    pause.textContent = paused ? "Resume" : "Pause";
    pause.setAttribute("aria-label", paused ? "Resume node log updates" : "Pause node log updates");
    pause.setAttribute("aria-pressed", String(paused));
    poller.setPaused(paused);
    render();
    if (!paused && active) void poller.refresh();
  });
  refresh.addEventListener("click", () => { if (active) void poller.refresh(); });
  wrap.addEventListener("change", () => view.panel.classList.toggle("logs-nowrap", !wrap.checked));
  view.maximizeButton.addEventListener("click", () => setLogPanelMaximized(view, !view.panel.classList.contains("maximized"), `node logs for ${selectedNodeName() || nodeID}`));
  document.addEventListener("keydown", event => {
    if (event.key === "Escape" && view.panel.classList.contains("maximized")) {
      setLogPanelMaximized(view, false, `node logs for ${selectedNodeName() || nodeID}`);
      view.maximizeButton.focus();
    }
  });
  synchronize();
})();
