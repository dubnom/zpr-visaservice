const display = (value, fallback = "Not reported") => value == null || value === "" ? fallback : String(value);

function compareValues(left, right) {
  const a = String(left ?? "");
  const b = String(right ?? "");
  if (/^-?\d+$/.test(a) && /^-?\d+$/.test(b)) {
    const x = BigInt(a);
    const y = BigInt(b);
    return x < y ? -1 : x > y ? 1 : 0;
  }
  return window.ZPRSortableTable.compareValues(a, b, { numericStrings: true });
}

export function renderSourceMetrics(parent, source, { sort, onSort, track = () => {} }) {
  const heading = document.createElement("h4");
  heading.textContent = "Metrics";
  const table = document.createElement("table");
  table.className = "diagnostics-metrics-table";
  table.setAttribute("aria-label", `Metrics for ${source.name || source.id || "Unnamed source"}`);
  const head = table.createTHead();
  const row = head.insertRow();
  for (const [key, label] of [["name", "Metric"], ["value", "Value"], ["unit", "Unit"]]) {
    const cell = document.createElement("th");
    cell.textContent = label;
    cell.scope = "col";
    cell.dataset.sortKey = key;
    if (key === "value") cell.dataset.numeric = "true";
    row.append(cell);
  }
  const body = table.createTBody();
  const occurrences = new Map();
  const metrics = (source.metrics || []).map((metric, index) => {
    const name = display(metric.name);
    const occurrence = occurrences.get(name) || 0;
    occurrences.set(name, occurrence + 1);
    return { metric, index, name, occurrence };
  });
  metrics.sort((left, right) => {
    const order = sort.key === "value"
      ? compareValues(left.metric.value, right.metric.value)
      : window.ZPRSortableTable.compareValues(left.metric[sort.key], right.metric[sort.key]);
    return order * sort.direction || window.ZPRSortableTable.compareValues(left.metric.name, right.metric.name) || left.index - right.index;
  });
  if (!metrics.length) window.ZPRSortableTable.renderEmptyRow(body, 3, "No metrics in the current window.");
  for (const { metric, name, occurrence } of metrics) {
    const row = body.insertRow();
    const label = document.createElement("th");
    label.scope = "row";
    label.textContent = name;
    row.append(label);
    const value = document.createElement("span");
    value.className = "diagnostics-metric-value";
    value.textContent = display(metric.value, "Unavailable");
    row.insertCell().append(value);
    const unit = row.insertCell();
    unit.textContent = display(metric.unit);
    track(["metric", name, occurrence], value, value.textContent);
    track(["metric-unit", name, occurrence], unit, unit.textContent);
  }
  window.ZPRPollingDisplay.markNumericColumns(table);
  window.ZPRSortableTable.bindSortableHeaders({ table, getSort: () => sort, onSort });
  parent.append(heading, table);
}
