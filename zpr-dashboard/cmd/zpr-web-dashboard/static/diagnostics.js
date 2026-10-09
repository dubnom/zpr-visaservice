(async () => {
  const page = document.querySelector("[data-diagnostics]");
  if (!page) return;
  const { renderColoredLog } = await import("/colored-log.js?v=1");
  const sourceList = document.getElementById("diagnostics-sources");
  const loading = document.getElementById("diagnostics-loading");
  const error = document.getElementById("diagnostics-error");
  const filter = document.getElementById("diagnostics-filter");
  const count = document.getElementById("diagnostics-count");
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

  function compareMetricValues(left, right) {
    const a = String(left ?? "");
    const b = String(right ?? "");
    if (/^-?\d+$/.test(a) && /^-?\d+$/.test(b)) {
      const integerA = BigInt(a);
      const integerB = BigInt(b);
      return integerA < integerB ? -1 : integerA > integerB ? 1 : 0;
    }
    return window.ZPRSortableTable.compareValues(a, b, { numericStrings: true });
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
    const heading = appendText(parent, "h4", "", "Metrics");
    const table = document.createElement("table");
    table.className = "diagnostics-metrics-table";
    table.setAttribute("aria-label", `Metrics for ${sourceName(source)}`);
    const head = document.createElement("thead");
    const headerRow = document.createElement("tr");
    for (const [sortKey, label] of [["name", "Metric"], ["value", "Value"], ["unit", "Unit"]]) {
      const cell = appendText(headerRow, "th", "", label);
      cell.scope = "col";
      cell.dataset.sortKey = sortKey;
      if (sortKey === "value") cell.dataset.numeric = "true";
    }
    head.append(headerRow);
    const body = document.createElement("tbody");
    const occurrences = new Map();
    const metrics = (source.metrics || []).map((metric, index) => {
      const name = displayMissing(metric.name);
      const occurrence = occurrences.get(name) || 0;
      occurrences.set(name, occurrence + 1);
      return { metric, index, name, occurrence };
    });
    const sort = metricSorts.get(key) || { key: "name", direction: 1 };

    metrics.sort((left, right) => {
      const leftValue = left.metric[sort.key];
      const rightValue = right.metric[sort.key];
      const order = sort.key === "value"
        ? compareMetricValues(leftValue, rightValue)
        : window.ZPRSortableTable.compareValues(leftValue, rightValue);
      return order * sort.direction
        || window.ZPRSortableTable.compareValues(left.metric.name, right.metric.name)
        || left.index - right.index;
    });

    if (!metrics.length) {
      window.ZPRSortableTable.renderEmptyRow(body, 3, "No metrics in the current window.");
    } else {
      for (const { metric, index, name, occurrence } of metrics) {
        const row = document.createElement("tr");
        appendText(row, "th", "", name).scope = "row";
        const valueCell = document.createElement("td");
        const value = appendText(valueCell, "span", "diagnostics-metric-value", displayMissing(metric.value, "Unavailable"));
        track(changes, [key, "metric", name, occurrence], value, value.textContent);
        row.append(valueCell);
        const unit = appendText(row, "td", "", displayMissing(metric.unit));
        track(changes, [key, "metric-unit", name, occurrence], unit, unit.textContent);
        body.append(row);
      }
    }
    table.append(head, body);
    window.ZPRPollingDisplay.markNumericColumns(table);
    window.ZPRSortableTable.bindSortableHeaders({
      table,
      getSort: () => metricSorts.get(key) || { key: "name", direction: 1 },
      onSort: nextSort => {
        metricSorts.set(key, nextSort);
        render();
      },
    });
    parent.append(heading, table);
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

    const logHeading = appendText(details, "h4", "", "Logs");
    const logs = document.createElement("ol");
    logs.className = "diagnostics-logs";
    logs.setAttribute("aria-labelledby", logHeading.id || (logHeading.id = `diagnostics-logs-${encodeURIComponent(key)}`));
    for (const log of source.logs || []) {
      const logRow = document.createElement("li");
      const body = log.body == null || log.body === "" ? "No log body reported." : log.body;
      const logBody = appendText(logRow, "span", "diagnostics-log-body", "");
      renderColoredLog(logBody, body);
      logs.append(logRow);
    }
    if (!logs.children.length) appendText(logs, "li", "diagnostics-no-logs", "No logs in the current window.");
    details.append(logs);
    cell.append(details);
    detailsRow.append(cell);
    row.after(detailsRow);
  }

  function render(trackChanges = false) {
    sourceList.replaceChildren();
    const query = filter.value.trim().toLowerCase();
    const allSources = (responseData?.sources || []).map((source, index) => ({ source, index }));
    const sources = allSources.filter(({ source }) => {
      const content = [
        source.name, source.kind, source.identity, source.address, source.state, source.error,
        ...(source.logs || []).map(log => log.body),
        ...(source.metrics || []).map(metric => `${metric.name} ${metric.value} ${metric.unit}`),
      ].join(" ").toLowerCase();
      return !query || content.includes(query);
    }).sort(compareSources);
    count.textContent = `${sources.length} / ${responseData?.sources?.length || 0} sources`;
    const changes = new Map();
    if (trackChanges) {
      for (const { source } of allSources) trackSourceSnapshot(changes, source, sourceKey(source));
    }
    if (!sources.length) {
      appendText(sourceList, "p", "diagnostics-empty", responseData ? "No sources match this filter." : "No diagnostics data loaded.");
      if (trackChanges) tracker.update(changes);
      return;
    }

    const scroll = document.createElement("div");
    scroll.className = "diagnostics-table-scroll";
    const table = document.createElement("table");
    table.className = "diagnostics-source-table";
    table.setAttribute("aria-label", "Diagnostics source overview");
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
    for (const { source, index } of sources) {
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
    sourceList.append(scroll);
    if (trackChanges) tracker.update(changes);
  }

  async function load({ signal, isCurrent }) {
    const result = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, "/api/diagnostics", { signal, headers: { Accept: "application/json" } });
    if (!isCurrent()) return;
    responseData = result;
    error.textContent = result.error || "";
    error.hidden = !result.error;
    render(true);
  }

  const poller = window.ZPRPageRuntime.createPoller({
    run: load,
    onPending(pending) { loading.hidden = !pending || responseData !== null; },
    onError(failure) {
      error.textContent = failure.message || "Diagnostics unavailable.";
      error.hidden = false;
    },
  });

  function setActive() {
    const next = location.hash === "#diagnostics";
    if (active && !next) {
      poller.stop();
    }
    active = next;
    if (active) poller.start();
  }

  filter.addEventListener("input", render);
  document.addEventListener("control-room:refreshed", () => { if (active) void poller.refresh(); });
  window.addEventListener("hashchange", setActive);
  setActive();
})();
