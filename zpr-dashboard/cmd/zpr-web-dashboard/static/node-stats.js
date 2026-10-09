(() => {
  const selector = document.getElementById("node-stats-select");
  if (!selector) return;
  const status = document.getElementById("node-stats-status");
  const error = document.getElementById("node-stats-error");
  const summary = document.getElementById("node-stats-summary");
  const groups = document.getElementById("node-stats-groups");
  let nodes = [];
  let snapshotReceived = false;
  let connectionError = "";
  let adaptersExpandedFor = "";
  let actors = [];
  let adapterSort = { key: "name", direction: 1 };
  const sorts = {
    management: { key: "counter", direction: 1 },
    fastpath: { key: "counter", direction: 1 },
  };

  function text(parent, tag, value) {
    const element = document.createElement(tag);
    element.textContent = value;
    parent.append(element);
    return element;
  }

  function selectedNode() {
    return nodes.find((node) => node.cn === selector.value);
  }

  function renderAdapters(parent, node) {
    const section = text(parent, "section", "");
    section.id = "node-stats-adapters";
    section.hidden = adaptersExpandedFor !== node.cn;
    text(section, "h3", "Docked adapters");
    const table = text(section, "table", "");
    table.setAttribute("aria-label", "Docked adapters");
    const header = text(text(table, "thead", ""), "tr", "");
    for (const [key, label] of [["name", "Adapter"], ["address", "ZPR address"]]) {
      const cell = text(header, "th", label);
      cell.scope = "col";
      cell.dataset.sortKey = key;
    }
    const body = text(table, "tbody", "");
    const rows = (node.node_details.adapters || []).map(name => ({
      name, address: actors.find(actor => actor.cn === name)?.zpr_addr || "Unavailable",
    }));
    const renderRows = () => {
      body.replaceChildren();
      for (const adapter of [...rows].sort((a, b) => window.ZPRSortableTable.compareValues(a[adapterSort.key], b[adapterSort.key])
        * adapterSort.direction || window.ZPRSortableTable.compareValues(a.name, b.name))) {
        const row = text(body, "tr", "");
        text(row, "th", adapter.name).scope = "row";
        text(row, "td", adapter.address);
      }
    };
    window.ZPRSortableTable.bindSortableHeaders({
      table, getSort: () => adapterSort,
      onSort: sort => { adapterSort = sort; renderRows(); },
    });
    renderRows();
    if (!rows.length) text(section, "p", "No docked adapters.");
  }

  function statValue(parent, value, identity) {
    const span = text(parent, "span", value);
    span.className = "node-stats-value";
    span.dataset.nodeStatKey = JSON.stringify([selector.value, ...identity]);
    return span;
  }

  function compareCounts(left, right) {
    if (left == null || right == null) return left == null ? (right == null ? 0 : 1) : -1;
    const a = BigInt(left);
    const b = BigInt(right);
    return a < b ? -1 : a > b ? 1 : 0;
  }

  function renderCounterTable(kind, title, columns, rows) {
    const section = text(groups, "section", "");
    section.className = `node-stats-${kind}`;
    const heading = text(section, "h3", title);
    heading.id = `node-stats-${kind}-heading`;
    const scroll = text(section, "div", "");
    scroll.className = "node-stats-table-scroll";
    const table = text(scroll, "table", "");
    table.setAttribute("aria-labelledby", heading.id);
    const head = text(table, "thead", "");
    const headerRow = text(head, "tr", "");
    for (const [key, label] of [["counter", "Counter"], ...columns]) {
      const header = text(headerRow, "th", label);
      header.scope = "col";
      header.dataset.sortKey = key;
    }
    const body = text(table, "tbody", "");
    function renderRows() {
      const { key, direction } = sorts[kind];
      const sorted = [...rows].sort((left, right) => {
        const order = key === "counter"
          ? window.ZPRSortableTable.compareValues(left.name, right.name)
          : compareCounts(left.values.get(key), right.values.get(key));
        return order * direction || window.ZPRSortableTable.compareValues(left.name, right.name);
      });
      body.replaceChildren();
      for (const counter of sorted) {
        const row = text(body, "tr", "");
        text(row, "th", counter.name).scope = "row";
        for (const [column] of columns) {
          statValue(text(row, "td", ""), counter.values.get(column) ?? "Unavailable", [kind, counter.name, column]);
        }
      }
    }
    window.ZPRSortableTable.bindSortableHeaders({
      table,
      getSort: () => sorts[kind],
      onSort: (sort) => {
        sorts[kind] = sort;
        renderRows();
      },
    });
    renderRows();
  }

  function updateStatus() {
    const details = selectedNode()?.node_details;
    const stamp = Date.parse(details?.counters_updated_at);
    const age = Date.now() - stamp;
    const stale = !Number.isFinite(stamp) || age > 10000 || age < -1000;
    const messages = [
      connectionError && `Snapshot refresh failed: ${connectionError}. Displayed data is last known.`,
      details?.counter_stats_error,
      details?.denial_stats_error,
    ].filter(Boolean);
    if (details?.counters?.length && stale) messages.push("Counter sample is stale or its timestamp is unavailable; displayed totals are last known.");
    error.textContent = messages.join(" ");
    error.hidden = !messages.length;
    if (!selectedNode()) {
      status.textContent = snapshotReceived ? "No nodes in the production snapshot." : "Waiting for a production snapshot.";
    } else if (!details?.counters?.length || details.counter_stats_error) {
      status.textContent = "Packet counters unavailable.";
    } else {
      const timestamp = Number.isFinite(stamp) ? window.ZPRSafeDisplay.formatDateTime(details.counters_updated_at) : "Not reported";
      status.textContent = `${stale ? "Stale" : "Fresh"} counter sample: ${timestamp}.`;
    }
  }

  function renderNode() {
    summary.replaceChildren();
    groups.replaceChildren();
    const node = selectedNode();
    if (!node) {
      updateStatus();
      return;
    }
    text(summary, "h2", node.cn);
    const details = node.node_details;
    const fields = document.createElement("dl");
    fields.className = "node-stats-fields";
    const unavailable = (value) => value ?? "Unavailable";
    const entries = [
      ["ZPR address", unavailable(node.zpr_addr)],
      ["Synchronization", typeof details?.in_sync === "boolean" ? (details.in_sync ? "In sync" : "Not in sync") : "Unavailable"],
      ["Last contact", details?.last_contact != null ? window.ZPRSafeDisplay.formatDateTime(details.last_contact * 1000) : "Unavailable"],
      ["Pending installs", unavailable(details?.pending_install)],
      ["Pending revocations", unavailable(details?.pending_revocation)],
      ["Visa requests", unavailable(details?.visa_requests)],
      ["Approved requests", unavailable(details?.approved_vreqs)],
      ["Denied requests", unavailable(details?.denied_vreqs)],
      ["Buffered denials (current)", details?.denial_stats_error ? "Unavailable" : unavailable(details?.buffered_denials)],
      ["Local denial occurrences (cumulative)", details?.denial_stats_error ? "Unavailable" : unavailable(details?.local_denials)],
      ["Installed visas", details ? (details.visas || []).length : "Unavailable"],
      ["Docked adapters", details ? (details.adapters || []).length : "Unavailable"],
      ["Node links", details ? (details.links || []).join(", ") || "None" : "Unavailable"],
    ];
    for (const [label, value] of entries) {
      text(fields, "dt", label);
      const cell = text(fields, "dd", "");
      if (label === "Docked adapters" && details) {
        const button = text(cell, "button", "");
        button.type = "button";
        button.className = "quiet node-stats-adapter-count";
        button.setAttribute("aria-label", `Show docked adapters for ${node.cn}`);
        button.setAttribute("aria-controls", "node-stats-adapters");
        button.setAttribute("aria-expanded", String(adaptersExpandedFor === node.cn));
        statValue(button, value, ["summary", label]);
        button.addEventListener("click", () => {
          adaptersExpandedFor = adaptersExpandedFor === node.cn ? "" : node.cn;
          button.setAttribute("aria-expanded", String(adaptersExpandedFor === node.cn));
          document.getElementById("node-stats-adapters").hidden = adaptersExpandedFor !== node.cn;
        });
      } else statValue(cell, value, ["summary", label]);
    }
    summary.append(fields);
    if (details) renderAdapters(summary, node);
    const grouped = new Map();
    if (!details?.counter_stats_error) {
      for (const counter of details?.counters || []) {
        if (!grouped.has(counter.group)) grouped.set(counter.group, []);
        grouped.get(counter.group).push(counter);
      }
    }
    const management = grouped.get("management");
    if (management?.length) {
      renderCounterTable("management", "Management counters", [["total", "Total"]],
        management.map((counter) => ({ name: counter.name, values: new Map([["total", counter.value]]) })));
    }
    const workers = [...grouped.keys()].filter((group) => group.startsWith("fastpath."))
      .map((group) => group.slice("fastpath.".length))
      .sort(compareCounts);
    const countersByName = new Map();
    for (const worker of workers) {
      for (const counter of grouped.get(`fastpath.${worker}`)) {
        if (!countersByName.has(counter.name)) countersByName.set(counter.name, { name: counter.name, values: new Map() });
        countersByName.get(counter.name).values.set(worker, counter.value);
      }
    }
    if (workers.length) {
      if (sorts.fastpath.key !== "counter" && !workers.includes(sorts.fastpath.key)) sorts.fastpath = { key: "counter", direction: 1 };
      renderCounterTable("fastpath", "Fastpath workers", workers.map((worker) => [worker, worker]), [...countersByName.values()]);
    }
    updateStatus();
  }

  window.addEventListener("zpr-snapshot", ({ detail }) => {
    const previous = selector.value;
    actors = detail.actors || [];
    nodes = actors.filter((actor) => actor.node);
    snapshotReceived = true;
    connectionError = "";
    selector.replaceChildren();
    for (const node of nodes) text(selector, "option", node.cn).value = node.cn;
    if (nodes.some((node) => node.cn === previous)) selector.value = previous;
    selector.disabled = !nodes.length;
    document.getElementById("node-stats-count").textContent = `${nodes.length} nodes`;
    renderNode();
  });
  window.addEventListener("zpr-snapshot-error", ({ detail }) => {
    connectionError = detail;
    updateStatus();
  });
  selector.addEventListener("change", renderNode);
  // Age the last sample while polling is paused, without making another request.
  setInterval(() => {
    if (location.hash === "#node-stats") updateStatus();
  }, 1000);
  window.addEventListener("hashchange", updateStatus);
})();
