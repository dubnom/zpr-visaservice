(async () => {
  const { createLogPanel, updateLogPanel, setLogPanelMaximized } = await import("/log-panel.js?v=1");
  const grid = document.getElementById("machine-logs-grid");
  const controlRoom = grid.dataset.site === "control-room";
  const followLogs = () => controlRoom || document.getElementById("machine-logs-follow").checked;
  const endpoint = grid.dataset.endpoint;
  const pageActive = () => controlRoom ? location.hash === "#adapter-logs" : location.pathname === "/machine-logs.html";
  const pauseButton = document.getElementById("machine-logs-pause");
  const refreshButton = document.getElementById("machine-logs-refresh");
  const cards = new Map();
  const sourceCache = new Map();
  let organizationID;
  const adapterColumns = [];
  let nextAdapterColumnID = 1;
  let adapterColumnsInitialized = false;
  let adapterLogType = "adapter";
  let showingAll = false;
  let runningOnly = false;
  let machines = [];
  let paused = false;
  let active = pageActive();
  const jsonToggle = document.getElementById(controlRoom ? "adapter-log-json" : "machine-logs-json");

  function retainSourceTails(result) {
    if (organizationID !== result.organization_id) sourceCache.clear();
    organizationID = result.organization_id;
    const present = new Set();
    const entries = controlRoom ? (result.adapters || []).map((adapter) => ({
      machine: { id: adapter.id, name: adapter.name }, state: adapter.state, sources: adapter.sources,
    })) : result.machines || [];
    const retained = entries.map((entry) => {
      present.add(entry.machine.id);
      const cached = sourceCache.get(entry.machine.id) || { entry, sources: new Map() };
      const names = new Set();
      const sources = (entry.sources || []).map((source) => {
        names.add(source.name);
        const previous = cached.sources.get(source.name);
        const runtimeState = source.state || entry.state;
        const disconnected = !["running", "unknown"].includes(runtimeState) || Boolean(source.error);
        const lines = Array.isArray(source.lines) ? source.lines : [];
        if (!source.error && !disconnected) cached.sources.set(source.name, { ...source, lines });
        return { ...source, lines: disconnected && previous ? previous.lines : lines, disconnected };
      });
      for (const [name, source] of cached.sources) {
        if (!names.has(name)) sources.push({ ...source, disconnected: true });
      }
      cached.entry = entry;
      sourceCache.set(entry.machine.id, cached);
      return { ...entry, sources };
    });
    for (const [id, cached] of sourceCache) {
      if (!present.has(id)) retained.push({ ...cached.entry, state: "missing", controller: null, session: null, workloads: null, user: "", sources: [...cached.sources.values()].map((source) => ({ ...source, disconnected: true })) });
    }
    return retained;
  }

  function setMaximized(card, maximized) {
    setLogPanelMaximized(card, maximized,
      card.id ? `adapter panel ${card.id}` : `logs for ${card.name?.textContent || card.title?.textContent || "adapter"}`, followLogs());
  }

  function maximizeCard(card, maximized) {
    if (maximized) {
      for (const other of cards.values()) {
        if (other !== card && other.panel.classList.contains("maximized")) setMaximized(other, false);
      }
    }
    setMaximized(card, maximized);
  }

  function switchLogSource(card, name) {
    if (card.selectedSource === name) return;
    if (card.selectedSource) card.sourceViews.set(card.selectedSource, { scrollTop: card.output.scrollTop, following: card.following });
    card.selectedSource = name;
    const saved = card.sourceViews.get(name);
    card.following = saved?.following ?? true;
    card.nextScrollTop = saved?.scrollTop ?? 0;
  }

  function adapterSources() {
    return machines.flatMap((entry) => (entry.sources || [])
      .filter((source) => source.kind === adapterLogType && (!runningOnly || (source.state || entry.state) === "running" && !source.disconnected))
      .map((source) => ({ key: `${entry.machine.id}\u001f${source.name}`, machine: entry, source, label: `${source.name} · ${entry.machine.id}` })));
  }

  function switchAdapterLogType(type) {
    if (type === adapterLogType || !["adapter", "controller"].includes(type)) return;
    for (const column of adapterColumns) {
      saveAdapterColumnPosition(column);
      column.selectedKeys.set(adapterLogType, column.selectedKey);
    }
    adapterLogType = type;
    for (const column of adapterColumns) {
      column.selectedKey = column.selectedKeys.get(type) || "";
      const saved = column.sourceViews.get(column.selectedKey);
      column.following = saved?.following ?? true;
      column.nextScrollTop = saved?.scrollTop ?? 0;
      column.signature = "";
    }
    for (const button of document.querySelectorAll("[data-adapter-log-type]")) {
      button.setAttribute("aria-pressed", String(button.dataset.adapterLogType === type));
    }
    renderAdapterColumns();
  }

  function makeAdapterColumn() {
    grid.querySelector(".adapter-columns-empty")?.remove();
    const view = createLogPanel("Choose adapter");
    const { panel, header, title, actions, maximizeButton, output } = view;
    panel.classList.add("adapter-log-column");
    const removeButton = document.createElement("button");
    removeButton.type = "button";
    removeButton.className = "adapter-column-remove";
    removeButton.textContent = "−";
    removeButton.title = "Remove adapter panel";
    removeButton.setAttribute("aria-label", "Remove adapter panel");
    const pickerButton = document.createElement("button");
    pickerButton.type = "button";
    pickerButton.className = "adapter-source-picker-button";
    pickerButton.textContent = "⌄";
    pickerButton.setAttribute("aria-haspopup", "dialog");
    const pickerDialog = document.createElement("dialog");
    pickerDialog.className = "adapter-source-picker-dialog";
    pickerDialog.setAttribute("aria-label", `Choose ${adapterLogType} log source`);
    const select = document.createElement("select");
    select.size = 2;
    select.setAttribute("aria-label", `Select ${adapterLogType} and log source`);
    pickerDialog.append(select);
    pickerDialog.addEventListener("click", (event) => {
      if (event.target !== pickerDialog) return;
      const bounds = pickerDialog.getBoundingClientRect();
      if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) pickerDialog.close();
    });
    pickerButton.addEventListener("click", () => {
      if (!pickerDialog.open) pickerDialog.showModal();
      const bounds = title.getBoundingClientRect();
      const width = Math.min(460, panel.getBoundingClientRect().width, window.innerWidth - 32);
      pickerDialog.style.width = `${width}px`;
      pickerDialog.style.left = `${Math.max(16, Math.min(bounds.left, window.innerWidth - width - 16))}px`;
      pickerDialog.style.top = `${bounds.bottom + 6}px`;
      pickerDialog.style.maxHeight = `${Math.max(36, window.innerHeight - bounds.bottom - 22)}px`;
      select.focus();
    });
    const dismissPicker = () => {
      if (pickerDialog.open) pickerDialog.close();
    };
    pickerDialog.addEventListener("close", () => {
      window.removeEventListener("resize", dismissPicker);
      grid.removeEventListener("scroll", dismissPicker);
    });
    pickerButton.addEventListener("click", () => {
      window.addEventListener("resize", dismissPicker);
      grid.addEventListener("scroll", dismissPicker);
    });
    const toolbar = document.createElement("div");
    toolbar.className = "machine-log-source-toolbar adapter-column-picker";
    select.setAttribute("aria-label", `Select adapter for panel ${nextAdapterColumnID}`);
    const column = Object.assign(view, {
      id: nextAdapterColumnID++, panel, title, maximizeButton, removeButton, select, pickerButton, pickerDialog,
      toolbar, output, selectedKey: "", selectedKeys: new Map(), choices: "", signature: "", sourceViews: new Map(),
      following: true, nextScrollTop: undefined,
    });
    window.ZPRSafeDisplay.renderWindowControl(maximizeButton, false, `adapter panel ${column.id}`);
    maximizeButton.addEventListener("click", () => maximizeAdapterColumn(column, !panel.classList.contains("maximized")));
    removeButton.addEventListener("click", () => removeAdapterColumn(column));
    select.addEventListener("change", () => {
      saveAdapterColumnPosition(column);
      column.selectedKey = select.value;
      pickerDialog.close();
      const saved = column.sourceViews.get(column.selectedKey);
      column.following = saved?.following ?? true;
      column.nextScrollTop = saved?.scrollTop ?? 0;
      renderAdapterColumns();
    });
    select.addEventListener("click", (event) => {
      if (event.target.closest("option")) pickerDialog.close();
    });
    select.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        pickerDialog.close();
      }
    });
    actions.prepend(removeButton);
    toolbar.append(pickerButton);
    header.insertBefore(toolbar, actions);
    panel.append(header, output, pickerDialog);
    grid.append(panel);
    adapterColumns.push(column);
    return column;
  }

  function saveAdapterColumnPosition(column) {
    if (column.selectedKey) column.sourceViews.set(column.selectedKey, { scrollTop: column.output.scrollTop, following: column.following });
  }

  function removeAdapterColumn(column) {
    showingAll = false;
    saveAdapterColumnPosition(column);
    if (column.pickerDialog.open) column.pickerDialog.close();
    if (column.panel.classList.contains("maximized")) setMaximized(column, false);
    column.panel.remove();
    const index = adapterColumns.indexOf(column);
    if (index >= 0) adapterColumns.splice(index, 1);
    renderAdapterColumns();
  }

  function maximizeAdapterColumn(column, maximized) {
    if (maximized) for (const other of adapterColumns) {
      if (other !== column && other.panel.classList.contains("maximized")) setMaximized(other, false);
    }
    setMaximized(column, maximized);
  }

  function renderAdapterColumns() {
    const choices = adapterSources();
    const type = adapterLogType === "controller" ? "controller" : "adapter";
    const addButton = document.getElementById("adapter-log-add");
    addButton.setAttribute("aria-label", `Add ${type} panel`);
    addButton.title = `Add ${type} panel`;
    if (!adapterColumnsInitialized && choices.length) {
      adapterColumnsInitialized = true;
      makeAdapterColumn();
    }
    if (showingAll) {
      const keys = new Set(choices.map((choice) => choice.key));
      for (const column of [...adapterColumns]) {
        if (!keys.has(column.selectedKey)) {
          if (column.pickerDialog.open) column.pickerDialog.close();
          if (column.panel.classList.contains("maximized")) setMaximized(column, false);
          column.panel.remove();
          adapterColumns.splice(adapterColumns.indexOf(column), 1);
        }
      }
      for (const choice of choices) {
        if (!adapterColumns.some((column) => column.selectedKey === choice.key)) makeAdapterColumn().selectedKey = choice.key;
      }
    }
    const allButton = document.getElementById("adapter-log-all");
    document.getElementById("adapter-log-running").checked = runningOnly;
    allButton.textContent = showingAll ? "Hide all adapters" : "Show all adapters";
    allButton.setAttribute("aria-pressed", String(showingAll));
    addButton.disabled = showingAll || choices.length === 0;
    allButton.disabled = choices.length === 0;
    grid.querySelector(".adapter-columns-empty")?.remove();
    if (!adapterColumns.length || !choices.length) {
      const empty = document.createElement("p");
      empty.className = "adapter-columns-empty";
      empty.setAttribute("role", "status");
      empty.textContent = !choices.length
        ? runningOnly ? `No running ${type}s available.` : `No ${type}s available.`
        : `No ${type} panels. Use + to add one.`;
      grid.append(empty);
    }
    for (const column of adapterColumns) {
      column.panel.hidden = choices.length === 0;
      if (!choices.length) {
        if (column.pickerDialog.open) column.pickerDialog.close();
        if (column.panel.classList.contains("maximized")) setMaximized(column, false);
      }
      column.pickerDialog.setAttribute("aria-label", `Choose ${type} log source`);
      column.pickerButton.setAttribute("aria-label", `Choose ${type} and log source for panel ${column.id}`);
      column.select.setAttribute("aria-label", `Select ${type} for panel ${column.id}`);
      column.removeButton.setAttribute("aria-label", `Remove ${type} panel`);
      column.removeButton.title = `Remove ${type} panel`;
      const choicesSignature = JSON.stringify(choices.map(({ key, label }) => [key, label]));
      if (choicesSignature !== column.choices) {
        column.select.replaceChildren();
        for (const choice of choices) {
          const option = document.createElement("option");
          option.value = choice.key;
          option.textContent = choice.label;
          column.select.append(option);
        }
        column.choices = choicesSignature;
        column.select.size = Math.max(2, Math.min(8, choices.length));
      }
      if (!choices.some((choice) => choice.key === column.selectedKey)) {
        const previous = column.selectedKey;
        column.selectedKey = choices[0]?.key || "";
        const saved = column.sourceViews.get(column.selectedKey);
        column.following = saved?.following ?? true;
        column.nextScrollTop = saved?.scrollTop ?? 0;
        if (previous && column.selectedKey !== previous) column.signature = "";
      }
      column.select.value = column.selectedKey;
      column.select.disabled = choices.length === 0 || showingAll;
      const selected = choices.find((choice) => choice.key === column.selectedKey);
      const entry = selected?.machine;
      const source = selected?.source;
      column.panel.classList.toggle("running", Boolean(selected) && (source.state || entry.state) === "running" && !source.disconnected);
      column.title.textContent = selected ? entry.machine.name || entry.machine.id : "No source available";
      column.title.title = selected ? `${entry.machine.id} / ${source.name}` : `No ${type} source available`;
      column.pickerButton.title = selected ? `Choose log source: ${entry.machine.id} / ${source.name}` : `Choose ${type} log source`;
      column.pickerButton.disabled = choices.length === 0;
      column.output.setAttribute("aria-label", selected ? `${entry.machine.id} ${source.name} logs` : `${type} logs`);
      updateLogPanel(column, source, `No ${type} logs available.`, { formatJSON: jsonToggle.checked, follow: followLogs() && !column.panel.hidden });
    }
  }

  function addAdapterColumn() {
    const choices = adapterSources();
    const used = new Set(adapterColumns.map((column) => column.selectedKey));
    const next = choices.find((choice) => !used.has(choice.key)) || choices[0];
    const column = makeAdapterColumn();
    if (next) column.selectedKey = next.key;
    renderAdapterColumns();
    column.select.focus();
  }

  function render() {
    if (controlRoom) { renderAdapterColumns(); return; }
    const query = document.getElementById("machine-logs-search").value.trim().toLowerCase();
    const runningOnly = document.getElementById("machine-logs-running").checked;
    const follow = document.getElementById("machine-logs-follow").checked;
    const present = new Set();
    let visible = 0;
    for (const entry of machines) {
      const machine = entry.machine;
      present.add(machine.id);
      let card = cards.get(machine.id);
      if (!card) {
        const view = createLogPanel(`logs for ${machine.id}`);
        const { panel, header, title: name, actions, maximizeButton, output } = view;
        const identity = document.createElement("div");
        const meta = document.createElement("small");
        const status = document.createElement("span");
        status.className = "machine-log-state";
        maximizeButton.addEventListener("click", () => maximizeCard(card, !panel.classList.contains("maximized")));
        output.id = `machine-log-output-${machine.id}`;
        output.setAttribute("aria-label", `${machine.id} logs`);
        const sourceToolbar = document.createElement("div");
        sourceToolbar.className = "machine-log-source-toolbar";
        const sourceLabel = document.createElement("span");
        sourceLabel.textContent = "Log";
        const sourceOptions = document.createElement("div");
        sourceOptions.className = "machine-log-source-options";
        sourceOptions.setAttribute("role", "radiogroup");
        sourceOptions.setAttribute("aria-label", `Log source for ${machine.id}`);
        sourceToolbar.append(sourceLabel, sourceOptions);
        const details = document.createElement("details");
        details.className = "worker-details";
        const summary = document.createElement("summary");
        summary.textContent = "Device and workloads";
        const inventory = document.createElement("div");
        details.append(summary, inventory);
        identity.append(name, meta);
        actions.prepend(status);
        header.replaceChildren(identity, actions);
        panel.append(header, details, sourceToolbar, output);
        grid.append(panel);
        card = Object.assign(view, { name, meta, status, sourceOptions, inventory, sourceViews: new Map(), selectedSource: "", sourceChoices: "" });
        cards.set(machine.id, card);
      }
      card.name.textContent = machine.id;
      card.meta.textContent = [machine.model, machine.location, entry.user].filter(Boolean).join(" / ");
      card.status.textContent = entry.state || "missing";
      card.inventory.replaceChildren();
      const field = (label, value) => {
        const row = document.createElement("p");
        const title = document.createElement("strong");
        title.textContent = `${label}: `;
        row.append(title, document.createTextNode(value));
        card.inventory.append(row);
      };
      field("Type", machine.type || "Not reported");
      field("Owner", machine.owner || "Not reported");
      field("Security posture", machine.secure == null ? "Not reported" : machine.secure ? "Secure" : "Unsecured");
      field("Container", entry.state || "Not reported");
      field("Controller", entry.controller ? entry.controller.connected ? "Connected" : "Offline" : "Unavailable");
      field("Session", entry.session ? entry.session.authenticated ? `Authenticated as ${entry.session.user}` : "No authenticated user" : "Unavailable");
      const workloads = entry.workloads;
      field("Workloads", workloads == null ? "Unavailable" : workloads.length ? workloads.map(workload => `${workload.name} (${workload.kind || "workload"}): ${workload.state || "unknown"} · ${workload.agent || "agent not reported"} · ${workload.address || "address not reported"}`).join("\n") : "None selected");
      const sources = entry.sources || [];
      const choices = JSON.stringify(sources.map((source) => source.name));
      if (choices !== card.sourceChoices) {
        card.sourceOptions.replaceChildren();
        for (const source of sources) {
          const option = document.createElement("label");
          const radio = document.createElement("input");
          radio.type = "radio";
          radio.name = `machine-log-source-${machine.id}`;
          radio.value = source.name;
          radio.setAttribute("aria-controls", card.output.id);
          radio.addEventListener("change", () => {
            if (!radio.checked) return;
            switchLogSource(card, radio.value);
            render();
          });
          const label = document.createElement("span");
          label.textContent = source.name;
          option.append(radio, label);
          card.sourceOptions.append(option);
        }
        if (!sources.length) {
          const option = document.createElement("span");
          option.className = "machine-log-source-empty";
          option.textContent = "No logs available";
          card.sourceOptions.append(option);
        }
        card.sourceChoices = choices;
      }
      if (!sources.some((source) => source.name === card.selectedSource)) switchLogSource(card, sources[0]?.name || "");
      for (const radio of card.sourceOptions.querySelectorAll("input")) {
        radio.checked = radio.value === card.selectedSource;
        radio.disabled = sources.length < 2;
      }
      const source = sources.find((source) => source.name === card.selectedSource);
      card.panel.classList.toggle("running", entry.state === "running" && !source?.disconnected);
      if (source?.disconnected) card.status.textContent = "disconnected";
      card.output.setAttribute("aria-label", `${machine.id} ${card.selectedSource || "machine"} logs`);
      const matches = !query || JSON.stringify(entry).toLowerCase().includes(query);
      const type = document.getElementById("machine-type-filter").value;
      card.panel.hidden = !matches || (runningOnly && entry.state !== "running") || (type !== "all" && machine.type !== type);
      updateLogPanel(card, source, entry.state === "running" ? "No application/service logs assigned." : "Machine is not running.", { formatJSON: jsonToggle.checked, follow: follow && !card.panel.hidden });
      if (!card.panel.hidden) visible++;
    }
    for (const [id, card] of cards) {
      if (!present.has(id)) {
        if (card.panel.classList.contains("maximized")) setMaximized(card, false);
        card.panel.remove();
        cards.delete(id);
      }
    }
    document.getElementById("machine-logs-count").textContent = `${visible} / ${machines.length} workers`;
    let empty = grid.querySelector(".workers-empty");
    if (!visible) {
      if (!empty) {
        empty = document.createElement("p");
        empty.className = "workers-empty";
        empty.setAttribute("role", "status");
        grid.append(empty);
      }
      empty.textContent = machines.length ? "No workers match the filters." : "No workers available.";
    } else empty?.remove();
  }

  async function collectLogs({ signal, isCurrent }) {
    const fetcher = controlRoom ? window.zprOperatorFetch : window.fetch.bind(window);
    const result = await window.ZPRPageRuntime.requestJSON(fetcher, endpoint, { signal });
    if (!isCurrent()) return;
    machines = retainSourceTails(result);
    render();
    document.getElementById("machine-logs-error").hidden = true;
    document.getElementById("machine-logs-time").textContent = window.ZPRSafeDisplay.formatTime(result.updated_at);
    const status = paused ? "Paused" : "Live";
    document.getElementById("machine-logs-status").textContent = controlRoom ? status : `${result.organization_id} / ${status}`;
  }

  const poller = window.ZPRPageRuntime.createPoller({
    run: collectLogs,
    interval: 2000,
    onPending(value) { if (refreshButton) refreshButton.disabled = value; },
    onError(error) {
      machines = machines.map((entry) => ({ ...entry, state: "disconnected", sources: (entry.sources || []).map((source) => ({ ...source, disconnected: true })) }));
      render();
      const message = document.getElementById("machine-logs-error");
      message.textContent = error.message.startsWith("HTTP 5") && error.message.includes("invalid JSON")
        ? `Visa Service or Control-Service is unavailable (${error.message.split(":")[0]}).`
        : error.message || (controlRoom ? "Adapter logs unavailable" : "Machine logs unavailable");
      message.hidden = false;
      document.getElementById("machine-logs-status").textContent = "Disconnected";
    },
  });

  function start() {
    active = true;
    poller.start();
  }

  function stop() {
    active = false;
    poller.stop();
    for (const card of cards.values()) {
      if (card.panel.classList.contains("maximized")) maximizeCard(card, false);
    }
  }

  pauseButton.addEventListener("click", () => {
    paused = !paused;
    pauseButton.textContent = paused ? "Resume" : "Pause";
    pauseButton.setAttribute("aria-pressed", String(paused));
    pauseButton.setAttribute("aria-label", paused ? "Resume log updates" : "Pause log updates");
    poller.setPaused(paused);
    if (paused) {
      document.getElementById("machine-logs-status").textContent = "Paused";
    } else if (active) void poller.refresh();
  });
  refreshButton?.addEventListener("click", () => { void poller.refresh(); });
  const wrapToggle = document.getElementById(controlRoom ? "adapter-log-wrap" : "machine-logs-wrap");
  wrapToggle.checked = false;
  grid.classList.add("logs-nowrap");
  wrapToggle.addEventListener("change", () => grid.classList.toggle("logs-nowrap", !wrapToggle.checked));
  jsonToggle.addEventListener("change", render);
  if (controlRoom) {
    document.getElementById("adapter-log-add").addEventListener("click", addAdapterColumn);
    document.getElementById("adapter-log-all").addEventListener("click", () => {
      showingAll = !showingAll;
      for (const column of [...adapterColumns]) {
        if (column.pickerDialog.open) column.pickerDialog.close();
        if (column.panel.classList.contains("maximized")) setMaximized(column, false);
        column.panel.remove();
      }
      adapterColumns.length = 0;
      adapterColumnsInitialized = true;
      renderAdapterColumns();
    });
    document.getElementById("adapter-log-running").addEventListener("change", (event) => {
      runningOnly = event.currentTarget.checked;
      renderAdapterColumns();
    });
    for (const button of document.querySelectorAll("[data-adapter-log-type]")) {
      button.addEventListener("click", () => switchAdapterLogType(button.dataset.adapterLogType));
    }
  }
  if (!controlRoom) for (const id of ["machine-logs-search", "machine-logs-running", "machine-type-filter"]) document.getElementById(id).addEventListener("input", render);
  document.getElementById("machine-logs-follow")?.addEventListener("input", (event) => {
    if (event.target.checked) for (const card of cards.values()) {
      card.following = true;
      for (const view of card.sourceViews.values()) view.following = true;
    }
    if (event.target.checked) for (const column of adapterColumns) {
      column.following = true;
      for (const view of column.sourceViews.values()) view.following = true;
    }
    render();
  });
  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    for (const card of cards.values()) {
      if (card.panel.classList.contains("maximized")) maximizeCard(card, false);
    }
    for (const column of adapterColumns) if (column.panel.classList.contains("maximized")) maximizeAdapterColumn(column, false);
  });
  if (controlRoom) {
    window.addEventListener("hashchange", () => { if (pageActive()) start(); else stop(); });
  } else {
    document.addEventListener("simulator:activate", (event) => { if (event.detail.path === "/machine-logs.html") start(); });
    document.addEventListener("simulator:deactivate", (event) => { if (event.detail.path === "/machine-logs.html") stop(); });
  }
  if (active) start();
})();