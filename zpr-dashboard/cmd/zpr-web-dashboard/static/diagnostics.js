(async () => {
  const page = document.querySelector("[data-diagnostics]");
  if (!page) return;
  const { createLogPanel, updateLogPanel, setLogPanelMaximized } = await import("/log-panel.js?v=1");
  const { renderSourceMetrics } = await import("/source-metrics.js?v=1");
  const sourceList = document.getElementById("diagnostics-sources");
  const loading = document.getElementById("diagnostics-loading");
  const error = document.getElementById("diagnostics-error");
  const filter = document.getElementById("diagnostics-filter");
  const count = document.getElementById("diagnostics-count");
  const selector = document.getElementById("diagnostics-service-select");
  const logHost = document.getElementById("service-logs");
  const view = createLogPanel("Service logs");
  view.panel.classList.add("service-log-panel", "logs-nowrap");
  const pause = document.createElement("button");
  pause.type = "button";
  pause.textContent = "Pause";
  pause.setAttribute("aria-label", "Pause service log updates");
  pause.setAttribute("aria-pressed", "false");
  const refresh = document.createElement("button");
  refresh.type = "button";
  refresh.textContent = "Refresh";
  refresh.setAttribute("aria-label", "Refresh service logs");
  view.actions.prepend(pause, refresh);
  const toolbar = document.createElement("div");
  toolbar.className = "machine-logs-toolbar service-logs-toolbar";
  const wrapLabel = document.createElement("label");
  const wrap = document.createElement("input");
  wrap.type = "checkbox";
  wrapLabel.append(wrap, "Wrap lines");
  toolbar.append(wrapLabel);
  view.output.before(toolbar);
  const positions = new Map();
  const logCache = new Map();
  let selectedKey = "";
  let pending = false;
  let paused = false;
  const aggregate = document.createElement("p");
  aggregate.id = "diagnostics-aggregate";
  aggregate.setAttribute("role", "status");
  loading.after(aggregate);
  const tracker = window.ZPRPollingDisplay.createTracker();
  let responseData = null;
  let active = location.hash === "#diagnostics";
  let sourceSort = { key: "source", direction: 1 };
  const metricSorts = new Map();
  const collapsedSources = new Set();

  function appendText(parent, tag, className, text) {
    const child = document.createElement(tag);
    if (className) child.className = className;
    child.textContent = text;
    parent.append(child);
    return child;
  }

  function sourceKey(source) {
    return String(source.id ?? JSON.stringify([source.kind, source.identity, source.name, source.address]));
  }

  function sourceName(source) {
    return source.name || source.id || "Unnamed source";
  }

  function selectSource(key) {
    if (key === selectedKey) return;
    if (selectedKey && view.panel.isConnected) {
      positions.set(selectedKey, { scrollTop: view.output.scrollTop, following: view.following });
    }
    setLogPanelMaximized(view, false, "Service logs");
    selectedKey = key;
    const saved = positions.get(key);
    view.following = saved?.following ?? true;
    view.nextScrollTop = saved?.scrollTop ?? 0;
  }

  function renderLogs(source) {
    if (!active || !source) {
      setLogPanelMaximized(view, false, "Service logs");
      view.panel.remove();
      return;
    }
    if (!view.panel.isConnected) logHost.append(view.panel);
    const name = sourceName(source);
    const label = `service logs for ${name}`;
    view.title.textContent = `Logs for ${name}`;
    view.output.setAttribute("aria-label", label);
    window.ZPRSafeDisplay.renderWindowControl(view.maximizeButton, view.panel.classList.contains("maximized"), label);
    refresh.disabled = pending;
    const unavailable = !error.hidden ? error.textContent : source.error ||
      (source.state === "unavailable" ? "Service log source unavailable." : "");
    const cached = logCache.get(selectedKey);
    const logs = unavailable && !source.logs.length && cached ? cached : source.logs;
    updateLogPanel(view, {
      lines: logs.map(log => log.body == null || log.body === "" ? "No log body reported." : log.body),
      disconnected: Boolean(unavailable),
      error: unavailable ? `${unavailable}${logs.length ? " Displayed logs are last known." : ""}`
        : source.state === "stale" ? "Log sample is stale; displayed logs are last known."
        : source.state === "partial" ? "Service log sample is partial." : "",
    }, "No service logs available.", { independentRecords: true });
  }

  function sourceIdentity(source) {
    const identity = displayMissing(source.identity);
    return source.address ? `${identity} · ${source.address}` : identity;
  }

  function displayMissing(value, fallback = "Not reported") {
    return value == null || value === "" ? fallback : String(value);
  }

  function displayTimestamp(value) {
    if (value == null || value === "") return "Not reported";
    return Number.isFinite(Date.parse(value))
      ? window.ZPRSafeDisplay.formatDateTime(value)
      : "Invalid timestamp";
  }

  function compareSources(left, right) {
    const leftValue = sourceSort.key === "source"
      ? sourceName(left.source)
      : sourceSort.key === "identity"
        ? sourceIdentity(left.source)
        : sourceSort.key === "kind"
          ? displayMissing(left.source.kind)
          : sourceSort.key === "state"
            ? displayMissing(left.source.state, "Unavailable")
            : left.source.last_updated ? Date.parse(left.source.last_updated) : null;
    const rightValue = sourceSort.key === "source"
      ? sourceName(right.source)
      : sourceSort.key === "identity"
        ? sourceIdentity(right.source)
        : sourceSort.key === "kind"
          ? displayMissing(right.source.kind)
          : sourceSort.key === "state"
            ? displayMissing(right.source.state, "Unavailable")
            : right.source.last_updated ? Date.parse(right.source.last_updated) : null;
    const order = sourceSort.key === "last_updated"
      ? (leftValue == null || !Number.isFinite(leftValue))
        ? (rightValue == null || !Number.isFinite(rightValue) ? 0 : 1)
        : (rightValue == null || !Number.isFinite(rightValue) ? -1 : leftValue - rightValue)
      : window.ZPRSortableTable.compareValues(leftValue, rightValue);
    return order * sourceSort.direction
      || window.ZPRSortableTable.compareValues(sourceKey(left.source, left.index), sourceKey(right.source, right.index));
  }

  function track(changes, key, node, value) {
    changes.set(JSON.stringify(key), { node, value });
  }

  function trackSourceSnapshot(changes, source, key) {
    const placeholder = (value) => {
      const node = document.createElement("span");
      node.textContent = value;
      return node;
    };
    for (const [field, value] of [
      ["name", sourceName(source)],
      ["identity", sourceIdentity(source)],
      ["kind", displayMissing(source.kind)],
      ["state", displayMissing(source.state, "Unavailable")],
      ["last_updated", displayTimestamp(source.last_updated)],
      ["error", displayMissing(source.error, "No source error")],
    ]) {
      track(changes, [key, field === "error" ? "error" : "source", ...(field === "error" ? [] : [field])], placeholder(value), value);
    }

    const occurrences = new Map();
    for (const metric of source.metrics || []) {
      const name = displayMissing(metric.name);
      const occurrence = occurrences.get(name) || 0;
      occurrences.set(name, occurrence + 1);
      const metricKey = [key, "metric", name, occurrence];
      const metricValue = displayMissing(metric.value, "Unavailable");
      const unit = displayMissing(metric.unit);
      track(changes, metricKey, placeholder(metricValue), metricValue);
      track(changes, [key, "metric-unit", name, occurrence], placeholder(unit), unit);
    }
  }

  function renderMetricTable(parent, source, key, changes) {
    renderSourceMetrics(parent, source, {
      sort: metricSorts.get(key) || { key: "name", direction: 1 },
      onSort: nextSort => { metricSorts.set(key, nextSort); render(); },
      track: (fields, node, value) => track(changes, [key, ...fields], node, value),
    });
  }

  function renderDetails(row, source, key, changes, visible) {
    const detailsRow = document.createElement("tr");
    detailsRow.className = "diagnostics-details-row";
    detailsRow.hidden = !visible;
    detailsRow.dataset.sourceKey = key;
    detailsRow.id = `diagnostics-details-${encodeURIComponent(key)}`;
    const cell = document.createElement("td");
    cell.colSpan = 5;
    const details = document.createElement("section");
    details.className = "diagnostics-details";
    const title = appendText(details, "h3", "diagnostics-visually-hidden", `Details for ${sourceName(source)}`);
    details.setAttribute("aria-labelledby", title.id || (title.id = `diagnostics-title-${encodeURIComponent(key)}`));
    if (source.error) {
      const sourceError = appendText(details, "p", "diagnostics-source-error", source.error);
      track(changes, [key, "error"], sourceError, sourceError.textContent);
    }
    renderMetricTable(details, source, key, changes);

    cell.append(details);
    detailsRow.append(cell);
    row.after(detailsRow);
  }

  function render(trackChanges = false) {
    for (const child of [...sourceList.children]) if (child !== logHost) child.remove();
    const query = filter.value.trim().toLowerCase();
    const allSources = (responseData?.sources || [])
      .filter(source => source.kind !== "ZPR node" && !String(source.id || "").startsWith("node:"))
      .map((source, index) => ({ source, index }));
    const unavailable = allSources.filter(({ source }) => source.state !== "available").length;
    aggregate.textContent = responseData && allSources.length
      ? `Service telemetry: ${allSources.length - unavailable} current · ${unavailable} stale, partial, or unavailable.`
      : "";
    const sources = allSources.filter(({ source }) => {
      const content = [
        source.name, source.kind, source.identity, source.address, source.state, source.error,
        ...(source.logs || []).map(log => log.body),
        ...(source.metrics || []).map(metric => `${metric.name} ${metric.value} ${metric.unit}`),
      ].join(" ").toLowerCase();
      return !query || content.includes(query);
    }).sort(compareSources);
    count.textContent = `${sources.length} / ${allSources.length} services`;
    const keys = new Set(allSources.map(({ source }) => sourceKey(source)));
    for (const key of positions.keys()) if (!keys.has(key)) positions.delete(key);
    for (const key of logCache.keys()) if (!keys.has(key)) logCache.delete(key);
    for (const key of metricSorts.keys()) if (!keys.has(key)) metricSorts.delete(key);
    for (const key of collapsedSources) if (!keys.has(key)) collapsedSources.delete(key);
    const options = sources.map(({ source }) => {
      const option = document.createElement("option");
      option.value = sourceKey(source);
      option.textContent = `${sourceName(source)}${source.identity ? ` · ${source.identity}` : ""}`;
      return option;
    });
    if (!options.length) {
      const option = document.createElement("option");
      option.value = "";
      option.textContent = responseData ? "No matching services" : "Loading services...";
      options.push(option);
    }
    selectSource(sources.some(({ source }) => sourceKey(source) === selectedKey) ? selectedKey : options[0].value);
    selector.replaceChildren(...options);
    selector.value = selectedKey;
    selector.disabled = !sources.length;
    const selected = sources.find(({ source }) => sourceKey(source) === selectedKey)?.source;
    renderLogs(selected);
    const changes = new Map();
    if (trackChanges) {
      for (const { source } of allSources) trackSourceSnapshot(changes, source, sourceKey(source));
    }
    if (!sources.length) {
      const empty = appendText(sourceList, "p", "diagnostics-empty", responseData
        ? allSources.length ? "No services match this filter." : "No service telemetry sources configured."
        : "No service telemetry loaded.");
      logHost.before(empty);
      if (trackChanges) tracker.update(changes);
      return;
    }

    const scroll = document.createElement("div");
    scroll.className = "diagnostics-table-scroll";
    const table = document.createElement("table");
    table.className = "diagnostics-source-table";
    table.setAttribute("aria-label", "Selected service health");
    const head = document.createElement("thead");
    const headerRow = document.createElement("tr");
    for (const [key, label] of [
      ["source", "Source"], ["identity", "Identity"], ["kind", "Kind"],
      ["state", "State"], ["last_updated", "Last update"],
    ]) {
      const cell = appendText(headerRow, "th", "", label);
      cell.scope = "col";
      cell.dataset.sortKey = key;
    }
    head.append(headerRow);
    for (const { source } of sources.filter(({ source }) => sourceKey(source) === selectedKey)) {
      const key = sourceKey(source);
      const body = document.createElement("tbody");
      body.className = "diagnostics-source";
      body.dataset.state = source.state || "unavailable";
      body.dataset.sourceKey = key;
      const row = document.createElement("tr");
      row.className = "diagnostics-source-row";
      row.dataset.sourceKey = key;
      row.dataset.state = source.state || "unavailable";
      const nameCell = document.createElement("th");
      nameCell.scope = "row";
      const button = appendText(nameCell, "button", "diagnostics-expand", sourceName(source));
      button.type = "button";
      const isExpanded = !collapsedSources.has(key);
      button.setAttribute("aria-expanded", String(isExpanded));
      button.setAttribute("aria-label", `${isExpanded ? "Hide" : "Show"} details for ${sourceName(source)}`);
      button.setAttribute("aria-controls", `diagnostics-details-${encodeURIComponent(key)}`);
      button.addEventListener("click", () => {
        if (collapsedSources.has(key)) collapsedSources.delete(key);
        else collapsedSources.add(key);
        render();
      });
      row.append(nameCell);
      appendText(row, "td", "", sourceIdentity(source));
      appendText(row, "td", "", displayMissing(source.kind));
      const state = appendText(row, "td", "diagnostics-state", displayMissing(source.state, "Unavailable"));
      appendText(row, "td", "", displayTimestamp(source.last_updated));
      track(changes, [key, "source", "name"], button, button.textContent);
      track(changes, [key, "source", "identity"], row.cells[1], row.cells[1].textContent);
      track(changes, [key, "source", "kind"], row.cells[2], row.cells[2].textContent);
      track(changes, [key, "source", "state"], state, state.textContent);
      track(changes, [key, "source", "last_updated"], row.cells[4], row.cells[4].textContent);
      body.append(row);
      renderDetails(row, source, key, changes, !collapsedSources.has(key));
      table.append(body);
    }

    table.prepend(head);
    window.ZPRPollingDisplay.markNumericColumns(table);
    window.ZPRSortableTable.bindSortableHeaders({
      table,
      getSort: () => sourceSort,
      onSort: sort => { sourceSort = sort; render(); },
    });
    scroll.append(table);
    logHost.before(scroll);
    if (trackChanges) tracker.update(changes);
  }

  async function load({ signal, isCurrent }) {
    const result = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, "/api/diagnostics", { signal, headers: { Accept: "application/json" } });
    if (!isCurrent()) return;
    if (!Array.isArray(result?.sources) || result.sources.some(source => !source || typeof source !== "object" ||
        typeof source.id !== "string" || !source.id || !Array.isArray(source.logs) || !Array.isArray(source.metrics) ||
        source.logs.some(log => !log || typeof log !== "object" || log.body != null && typeof log.body !== "string") ||
        source.metrics.some(metric => !metric || typeof metric !== "object" || typeof metric.name !== "string" ||
          !["string", "number"].includes(typeof metric.value) && metric.value != null ||
          metric.unit != null && typeof metric.unit !== "string")) ||
        new Set(result.sources.map(source => source.id)).size !== result.sources.length) {
      throw new Error("Service telemetry response contains invalid source or log records.");
    }
    responseData = result;
    error.textContent = result.error || "";
    error.hidden = !result.error;
    for (const source of result.sources) {
      if (source.kind === "ZPR node" || source.id.startsWith("node:")) continue;
      if (source.logs.length || !source.error && ["available", "stale"].includes(source.state)) {
        logCache.set(sourceKey(source), source.logs);
      }
    }
    render(true);
  }

  const poller = window.ZPRPageRuntime.createPoller({
    run: load,
    onPending(value) {
      pending = value;
      loading.hidden = !pending || responseData !== null;
      refresh.disabled = pending;
    },
    onError(failure) {
      error.textContent = failure.message || "Service telemetry unavailable.";
      error.hidden = false;
      render();
    },
  });

  function setActive() {
    const next = location.hash === "#diagnostics";
    if (active && !next) {
      if (selectedKey) positions.set(selectedKey, { scrollTop: view.output.scrollTop, following: view.following });
      poller.stop();
      setLogPanelMaximized(view, false, "Service logs");
      view.panel.remove();
    }
    if (!active && next) {
      const saved = positions.get(selectedKey);
      view.following = saved?.following ?? true;
      view.nextScrollTop = saved?.scrollTop ?? 0;
    }
    active = next;
    render();
    if (active) poller.start();
  }

  filter.addEventListener("input", render);
  selector.addEventListener("change", () => { selectSource(selector.value); render(); });
  pause.addEventListener("click", () => {
    paused = !paused;
    pause.textContent = paused ? "Resume" : "Pause";
    pause.setAttribute("aria-label", paused ? "Resume service log updates" : "Pause service log updates");
    pause.setAttribute("aria-pressed", String(paused));
    poller.setPaused(paused);
    if (!paused && active) void poller.refresh();
  });
  refresh.addEventListener("click", () => { if (active) void poller.refresh(); });
  wrap.addEventListener("change", () => view.panel.classList.toggle("logs-nowrap", !wrap.checked));
  view.maximizeButton.addEventListener("click", () => setLogPanelMaximized(view, !view.panel.classList.contains("maximized"), `service logs for ${view.title.textContent.slice(9)}`));
  document.addEventListener("keydown", event => {
    if (event.key === "Escape" && view.panel.classList.contains("maximized")) {
      setLogPanelMaximized(view, false, `service logs for ${view.title.textContent.slice(9)}`);
      view.maximizeButton.focus();
    }
  });
  window.addEventListener("operator-session-cleared", () => {
    poller.stop();
    responseData = null;
    positions.clear();
    logCache.clear();
    metricSorts.clear();
    collapsedSources.clear();
    tracker.reset();
    selectedKey = "";
    view.output.replaceChildren();
    view.signature = "";
    error.textContent = "Operator session ended. Sign in to load service logs.";
    error.hidden = false;
    render();
  });
  document.addEventListener("control-room:refreshed", () => { if (active && !paused) void poller.refresh(); });
  window.addEventListener("hashchange", setActive);
  setActive();
})();
