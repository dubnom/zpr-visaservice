(async () => {
  const { AnsiUp } = await import("/ansi_up.js?v=6.0.6");
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
  let timer;
  let pending;

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
      if (!present.has(id)) retained.push({ ...cached.entry, state: "missing", sources: [...cached.sources.values()].map((source) => ({ ...source, disconnected: true })) });
    }
    return retained;
  }

  function renderSource(output, source, emptyMessage) {
    output.replaceChildren();
    if (!source || source.disconnected || source.error) {
      const status = document.createElement("p");
      status.className = "machine-log-error";
      status.textContent = source?.error || (source?.disconnected ? "Disconnected" : emptyMessage);
      output.append(status);
    }
    if (source && (source.lines.length || !source.disconnected && !source.error)) {
      const content = document.createElement("pre");
      renderColoredLog(content, source.lines.join("\n") || "No log entries.");
      output.append(content);
    }
  }

  function renderColoredLog(target, text) {
    const ansi = new AnsiUp();
    ansi.escape_html = true;
    const parsed = document.createElement("template");
    parsed.innerHTML = ansi.ansi_to_html(text).replaceAll(' style="', ' data-ansi-style="');
    const stylesheet = new CSSStyleSheet();
    const copy = (node) => {
      if (node.nodeType === 3) return document.createTextNode(node.textContent);
      const span = document.createElement("span");
      stylesheet.replaceSync(`span {${node.getAttribute("data-ansi-style") || ""}}`);
      const style = stylesheet.cssRules[0].style;
      for (const property of ["color", "backgroundColor", "fontWeight", "fontStyle", "textDecoration", "opacity"]) {
        if (style[property]) span.style[property] = style[property];
      }
      span.append(...[...node.childNodes].map(copy));
      return span;
    };
    target.replaceChildren(...[...parsed.content.childNodes].map(copy));
  }

  function setMaximized(card, maximized) {
    card.panel.classList.toggle("maximized", maximized);
    card.maximizeButton.textContent = maximized ? "Restore" : "Maximize";
    card.maximizeButton.setAttribute("aria-label", `${maximized ? "Restore" : "Maximize"} logs for ${card.name?.textContent || card.title?.textContent || "adapter"}`);
    card.maximizeButton.setAttribute("aria-pressed", String(maximized));
    document.body.classList.toggle("machine-log-maximized", maximized);
    if (followLogs() && card.following) card.output.scrollTop = card.output.scrollHeight;
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
    const panel = document.createElement("article");
    panel.className = "machine-log-panel adapter-log-column";
    const header = document.createElement("header");
    const title = document.createElement("h2");
    title.textContent = "Choose adapter";
    const actions = document.createElement("div");
    actions.className = "machine-log-panel-actions";
    const maximizeButton = document.createElement("button");
    maximizeButton.className = "quiet";
    maximizeButton.type = "button";
    maximizeButton.textContent = "Maximize";
    maximizeButton.setAttribute("aria-pressed", "false");
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
    const pickerHeader = document.createElement("header");
    const pickerTitle = document.createElement("h3");
    pickerTitle.id = `adapter-source-picker-title-${nextAdapterColumnID}`;
    pickerDialog.setAttribute("aria-labelledby", pickerTitle.id);
    const pickerClose = document.createElement("button");
    pickerClose.type = "button";
    pickerClose.className = "quiet";
    pickerClose.textContent = "Close";
    pickerClose.addEventListener("click", () => pickerDialog.close());
    pickerHeader.append(pickerTitle, pickerClose);
    const pickerLabel = document.createElement("label");
    pickerLabel.textContent = "Adapter and log source";
    const select = document.createElement("select");
    select.setAttribute("aria-label", `Select ${adapterLogType} and log source`);
    pickerLabel.append(select);
    pickerDialog.append(pickerHeader, pickerLabel);
    pickerButton.addEventListener("click", () => {
      if (!pickerDialog.open) pickerDialog.showModal();
      select.focus();
    });
    const toolbar = document.createElement("div");
    toolbar.className = "machine-log-source-toolbar adapter-column-picker";
    select.setAttribute("aria-label", `Select adapter for panel ${nextAdapterColumnID}`);
    const output = document.createElement("div");
    output.className = "machine-log-output";
    output.tabIndex = 0;
    const column = {
      id: nextAdapterColumnID++, panel, title, maximizeButton, removeButton, select, pickerButton, pickerDialog, pickerTitle,
      toolbar, output, selectedKey: "", selectedKeys: new Map(), choices: "", signature: "", sourceViews: new Map(),
      following: true, nextScrollTop: undefined,
    };
    maximizeButton.setAttribute("aria-label", `Maximize adapter panel ${column.id}`);
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
    output.addEventListener("scroll", () => {
      if (output.clientHeight) column.following = output.scrollHeight - output.clientHeight - output.scrollTop <= 8;
    });
    actions.append(removeButton, maximizeButton);
    pickerTitle.textContent = `Choose ${adapterLogType} log source`;
    toolbar.append(pickerButton);
    header.append(title, toolbar, actions);
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
    if (!adapterColumnsInitialized) {
      adapterColumnsInitialized = true;
      makeAdapterColumn();
    }
    if (showingAll) {
      const keys = new Set(choices.map((choice) => choice.key));
      for (const column of [...adapterColumns]) {
        if (!keys.has(column.selectedKey)) {
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
    addButton.disabled = showingAll;
    if (!adapterColumns.length) {
      grid.replaceChildren();
      const empty = document.createElement("p");
      empty.className = "adapter-columns-empty";
      empty.textContent = `No ${type} panels. Use + to add one.`;
      grid.append(empty);
    }
    for (const column of adapterColumns) {
      column.pickerTitle.textContent = `Choose ${type} log source`;
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
      const contentSignature = JSON.stringify({ state: entry?.state || "missing", source });
      if (contentSignature !== column.signature) {
        const scrollTop = column.nextScrollTop ?? column.output.scrollTop;
        column.nextScrollTop = undefined;
        renderSource(column.output, source, `No ${type} logs available.`);
        column.signature = contentSignature;
        column.output.scrollTop = scrollTop;
      }
    }
    if (followLogs()) for (const column of adapterColumns) {
      if (column.following && column.selectedKey) column.output.scrollTop = column.output.scrollHeight;
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
        const panel = document.createElement("article");
        panel.className = "machine-log-panel";
        const header = document.createElement("header");
        const identity = document.createElement("div");
        const name = document.createElement("h2");
        const meta = document.createElement("small");
        const status = document.createElement("span");
        status.className = "machine-log-state";
        const actions = document.createElement("div");
        actions.className = "machine-log-panel-actions";
        const maximizeButton = document.createElement("button");
        maximizeButton.className = "quiet";
        maximizeButton.type = "button";
        maximizeButton.textContent = "Maximize";
        maximizeButton.setAttribute("aria-label", `Maximize logs for ${machine.id}`);
        maximizeButton.setAttribute("aria-pressed", "false");
        maximizeButton.addEventListener("click", () => maximizeCard(card, !panel.classList.contains("maximized")));
        const output = document.createElement("div");
        output.className = "machine-log-output";
        output.tabIndex = 0;
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
        identity.append(name, meta);
        actions.append(status, maximizeButton);
        header.append(identity, actions);
        panel.append(header, sourceToolbar, output);
        grid.append(panel);
        card = { panel, name, meta, status, maximizeButton, output, sourceOptions, sourceViews: new Map(), selectedSource: "", sourceChoices: "", signature: "", following: true };
        output.addEventListener("scroll", () => {
          if (output.clientHeight) card.following = output.scrollHeight - output.clientHeight - output.scrollTop <= 8;
        });
        cards.set(machine.id, card);
      }
      card.name.textContent = machine.id;
      card.meta.textContent = [machine.model, machine.location, entry.user].filter(Boolean).join(" / ");
      card.status.textContent = entry.state || "missing";
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
      const signature = JSON.stringify({ state: entry.state, source });
      if (signature !== card.signature) {
        const scrollTop = card.nextScrollTop ?? card.output.scrollTop;
        card.nextScrollTop = undefined;
        renderSource(card.output, source, entry.state === "running" ? "No application/service logs assigned." : "Machine is not running.");
        card.signature = signature;
        card.output.scrollTop = scrollTop;
      }
      const matches = !query || JSON.stringify(entry).toLowerCase().includes(query);
      card.panel.hidden = !matches || (runningOnly && entry.state !== "running");
      if (!card.panel.hidden) visible++;
    }
    for (const [id, card] of cards) {
      if (!present.has(id)) {
        if (card.panel.classList.contains("maximized")) setMaximized(card, false);
        card.panel.remove();
        cards.delete(id);
      }
    }
    document.getElementById("machine-logs-count").textContent = `${visible} / ${machines.length} machines`;
    if (follow) for (const card of cards.values()) {
      if (card.following && !card.panel.hidden) card.output.scrollTop = card.output.scrollHeight;
    }
  }

  async function refresh() {
    if (pending || !active) return;
    pending = new AbortController();
    const request = pending;
    if (refreshButton) refreshButton.disabled = true;
    try {
      const response = await fetch(endpoint, { cache: "no-store", signal: request.signal });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
      if (!active) return;
      machines = retainSourceTails(result);
      render();
      document.getElementById("machine-logs-error").hidden = true;
      document.getElementById("machine-logs-time").textContent = new Date(result.updated_at).toLocaleTimeString();
      const status = paused ? "Paused" : "Live";
      document.getElementById("machine-logs-status").textContent = controlRoom ? status : `${result.organization_id} / ${status}`;
    } catch (error) {
      if (error.name !== "AbortError" && active) {
        machines = machines.map((entry) => ({ ...entry, state: "disconnected", sources: (entry.sources || []).map((source) => ({ ...source, disconnected: true })) }));
        render();
        const message = document.getElementById("machine-logs-error");
        message.textContent = error.message || (controlRoom ? "Adapter logs unavailable" : "Machine logs unavailable");
        message.hidden = false;
        document.getElementById("machine-logs-status").textContent = "Disconnected";
      }
    } finally {
      if (pending === request) pending = null;
      if (refreshButton) refreshButton.disabled = false;
      if (active && !paused) timer = setTimeout(refresh, 2000);
    }
  }

  function start() {
    active = true;
    clearTimeout(timer);
    if (!paused) refresh();
  }

  function stop() {
    active = false;
    clearTimeout(timer);
    pending?.abort();
    for (const card of cards.values()) {
      if (card.panel.classList.contains("maximized")) maximizeCard(card, false);
    }
  }

  pauseButton.addEventListener("click", () => {
    paused = !paused;
    pauseButton.textContent = paused ? "Resume" : "Pause";
    pauseButton.setAttribute("aria-pressed", String(paused));
    pauseButton.setAttribute("aria-label", paused ? "Resume log updates" : "Pause log updates");
    clearTimeout(timer);
    if (paused) {
      pending?.abort();
      document.getElementById("machine-logs-status").textContent = "Paused";
    } else start();
  });
  refreshButton?.addEventListener("click", () => { clearTimeout(timer); refresh(); });
  if (controlRoom) {
    document.getElementById("adapter-log-add").addEventListener("click", addAdapterColumn);
    document.getElementById("adapter-log-all").addEventListener("click", () => {
      showingAll = !showingAll;
      for (const column of [...adapterColumns]) {
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
    document.getElementById("adapter-log-wrap").addEventListener("click", (event) => {
      const wrapped = event.currentTarget.getAttribute("aria-pressed") !== "true";
      event.currentTarget.setAttribute("aria-pressed", String(wrapped));
      grid.classList.toggle("logs-nowrap", !wrapped);
    });
    for (const button of document.querySelectorAll("[data-adapter-log-type]")) {
      button.addEventListener("click", () => switchAdapterLogType(button.dataset.adapterLogType));
    }
  }
  if (!controlRoom) for (const id of ["machine-logs-search", "machine-logs-running"]) document.getElementById(id).addEventListener("input", render);
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