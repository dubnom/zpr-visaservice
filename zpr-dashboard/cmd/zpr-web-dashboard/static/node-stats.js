(() => {
  const navigation = document.getElementById("node-stats-nav");
  if (!navigation) return;
  const selectorContainer = document.getElementById("node-stats-selector");
  const selector = document.getElementById("node-stats-select");
  const status = document.getElementById("node-stats-status");
  const error = document.getElementById("node-stats-error");
  const content = document.getElementById("node-stats-content");
  const summary = document.getElementById("node-stats-summary");
  const groups = document.getElementById("node-stats-groups");
  const areaTabs = [...document.querySelectorAll("#node-stats-section-nav [role=tab]")];
  function selectArea(tab) {
    if (tab.getAttribute("aria-selected") === "true") return;
    window.dispatchEvent(new Event("node-stats-area-changing"));
    for (const candidate of areaTabs) {
      const selected = candidate === tab;
      candidate.setAttribute("aria-selected", String(selected));
      candidate.tabIndex = selected ? 0 : -1;
      document.getElementById(candidate.getAttribute("aria-controls")).hidden = !selected;
    }
    window.dispatchEvent(new Event("node-stats-area-changed"));
  }
  for (const [index, tab] of areaTabs.entries()) {
    tab.addEventListener("click", () => selectArea(tab));
    tab.addEventListener("keydown", event => {
      const next = event.key === "ArrowRight" ? (index + 1) % areaTabs.length
        : event.key === "ArrowLeft" ? (index - 1 + areaTabs.length) % areaTabs.length
        : event.key === "Home" ? 0 : event.key === "End" ? areaTabs.length - 1 : -1;
      if (next < 0) return;
      event.preventDefault();
      selectArea(areaTabs[next]);
      areaTabs[next].focus();
    });
  }
  let nodes = [];
  let selectedNodeID = "";
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
    return nodes.find((node) => node.cn === selectedNodeID);
  }

  function selectNode(id) {
    if (!nodes.some(node => node.cn === id) || id === selectedNodeID) return;
    selectedNodeID = id;
    renderNavigation();
    renderNode();
    window.dispatchEvent(new CustomEvent("node-stats-selection-changed", { detail: { nodeID: id } }));
  }

  function updateSelectorLayout() {
    if (!selectorContainer.clientWidth || !nodes.length) return;
    const overflow = navigation.scrollWidth > navigation.clientWidth;
    const tabFocused = navigation.contains(document.activeElement);
    const selectFocused = document.activeElement === selector;
    selectorContainer.classList.toggle("dropdown", overflow);
    selector.hidden = !overflow;
    content.setAttribute("aria-labelledby", overflow ? selector.id : `node-stats-tab-${nodes.findIndex(node => node.cn === selectedNodeID)}`);
    if (overflow && tabFocused || !overflow && selectFocused) {
      requestAnimationFrame(() => {
        if (selector.hidden) navigation.querySelector('[aria-selected="true"]')?.focus();
        else selector.focus();
      });
    }
  }

  function renderNavigation() {
    const focusedNodeID = navigation.contains(document.activeElement) ? document.activeElement.dataset.nodeId : "";
    const scrollLeft = navigation.scrollLeft;
    navigation.replaceChildren();
    selector.replaceChildren();
    navigation.hidden = !nodes.length;
    selector.disabled = !nodes.length;
    if (!nodes.length) {
      selector.hidden = true;
      selectorContainer.classList.remove("dropdown");
    }
    for (const [index, node] of nodes.entries()) {
      text(selector, "option", node.display_name || node.cn).value = node.cn;
      const tab = text(navigation, "button", node.display_name || node.cn);
      tab.type = "button";
      tab.id = `node-stats-tab-${index}`;
      tab.dataset.nodeId = node.cn;
      tab.setAttribute("role", "tab");
      tab.setAttribute("aria-controls", "node-stats-content");
      const selected = node.cn === selectedNodeID;
      tab.setAttribute("aria-selected", String(selected));
      tab.tabIndex = selected ? 0 : -1;
      tab.addEventListener("click", () => selectNode(node.cn));
      tab.addEventListener("keydown", event => {
        let nextIndex;
        if (event.key === "ArrowRight") nextIndex = (index + 1) % nodes.length;
        else if (event.key === "ArrowLeft") nextIndex = (index - 1 + nodes.length) % nodes.length;
        else if (event.key === "Home") nextIndex = 0;
        else if (event.key === "End") nextIndex = nodes.length - 1;
        else return;
        event.preventDefault();
        selectNode(nodes[nextIndex].cn);
        navigation.querySelectorAll('[role="tab"]')[nextIndex].focus();
      });
    }
    selector.value = selectedNodeID;
    if (focusedNodeID) {
      [...navigation.children].find(tab => tab.dataset.nodeId === focusedNodeID)?.focus();
    }
    navigation.scrollLeft = scrollLeft;
    requestAnimationFrame(updateSelectorLayout);
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
    span.className = "node-stats-value poll-value";
    span.dataset.nodeStatKey = JSON.stringify([selectedNodeID, ...identity]);
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
      if (key !== "counter") header.dataset.numeric = "true";
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
        window.ZPRPollingDisplay.markNumericColumns(table);
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
    content.hidden = !node;
    if (node) content.setAttribute("aria-labelledby", `node-stats-tab-${nodes.indexOf(node)}`);
    if (!node) {
      updateStatus();
      return;
    }
    text(summary, "h2", node.display_name || node.cn);
    const details = node.node_details;
    const fields = document.createElement("dl");
    fields.className = "node-stats-fields";
    const unavailable = (value) => value ?? "Unavailable";
    const entries = [
      ["Node identity", node.cn],
      ["ZPR address", unavailable(node.zpr_addr)],
      ["Synchronization", typeof details?.in_sync === "boolean" ? (details.in_sync ? "In sync" : "Not in sync") : "Unavailable"],
      ["Last contact", details?.last_contact != null ? window.ZPRSafeDisplay.formatDateTime(details.last_contact * 1000) : "Unavailable"],
      ["Pending installs", unavailable(details?.pending_install)],
      ["Pending revocations", unavailable(details?.pending_revocation)],
      ["Visa requests", unavailable(details?.visa_requests)],
      ["Approved requests", unavailable(details?.approved_vreqs)],
      ["Denied requests", unavailable(details?.denied_vreqs)],
      ["Buffered denials", details?.denial_stats_error ? "Unavailable" : unavailable(details?.buffered_denials)],
      ["Local denials", details?.denial_stats_error ? "Unavailable" : unavailable(details?.local_denials)],
      ["Installed visas", details ? (details.visas || []).length : "Unavailable"],
      ["Docked adapters", details ? (details.adapters || []).length : "Unavailable"],
      ["Node links", details ? (details.links || []).map(name => {
        const linked = nodes.find(candidate => candidate.cn === name);
        return linked?.display_name || name;
      }).join(", ") || "None" : "Unavailable"],
    ];
    for (const [label, value] of entries) {
      text(fields, "dt", label);
      const cell = text(fields, "dd", "");
      if (label === "Docked adapters" && details) {
        const button = text(cell, "button", "");
        button.type = "button";
        button.className = "quiet node-stats-adapter-count";
        button.setAttribute("aria-label", `Show docked adapters for ${node.display_name || node.cn}`);
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
    if (!groups.children.length) text(groups, "p", "Packet counters unavailable.");
    updateStatus();
  }

  window.addEventListener("zpr-snapshot", ({ detail }) => {
    const previous = selectedNodeID;
    actors = detail.actors || [];
    nodes = actors.filter((actor) => actor.node);
    snapshotReceived = true;
    connectionError = "";
    selectedNodeID = nodes.some(node => node.cn === previous) ? previous : nodes[0]?.cn || "";
    renderNavigation();
    document.getElementById("node-stats-count").textContent = `${nodes.length} nodes`;
    renderNode();
    if (selectedNodeID !== previous) {
      window.dispatchEvent(new CustomEvent("node-stats-selection-changed", { detail: { nodeID: selectedNodeID } }));
    }
  });
  window.addEventListener("node-stats-select-node", ({ detail }) => selectNode(detail?.nodeID));
  selector.addEventListener("change", () => selectNode(selector.value));
  new ResizeObserver(updateSelectorLayout).observe(selectorContainer);
  document.fonts.ready.then(updateSelectorLayout);
  window.addEventListener("zpr-snapshot-error", ({ detail }) => {
    connectionError = detail;
    updateStatus();
  });
  // Age the last sample while polling is paused, without making another request.
  setInterval(() => {
    if (location.hash === "#node-stats") updateStatus();
  }, 1000);
  window.addEventListener("hashchange", () => {
    updateStatus();
    requestAnimationFrame(updateSelectorLayout);
  });
})();
