(async () => {
  const { AnsiUp } = await import("/ansi_up.js?v=6.0.6");
  const grid = document.getElementById("machine-logs-grid");
  const controlRoom = grid.dataset.site === "control-room";
  const endpoint = grid.dataset.endpoint || "/api/simulator/machine-logs";
  const pageActive = () => controlRoom ? location.hash === "#adapter-logs" : location.pathname === "/machine-logs.html";
  const refreshButton = document.getElementById("machine-logs-refresh");
  const cards = new Map();
  let machines = [];
  let paused = false;
  let active = pageActive();
  let timer;
  let pending;

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
    card.maximizeButton.setAttribute("aria-label", `${maximized ? "Restore" : "Maximize"} logs for ${card.name.textContent}`);
    card.maximizeButton.setAttribute("aria-pressed", String(maximized));
    document.body.classList.toggle("machine-log-maximized", maximized);
    if (document.getElementById("machine-logs-follow").checked && card.following) card.output.scrollTop = card.output.scrollHeight;
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

  function render() {
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
      card.panel.classList.toggle("running", entry.state === "running");
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
      card.output.setAttribute("aria-label", `${machine.id} ${card.selectedSource || "machine"} logs`);
      const signature = JSON.stringify({ state: entry.state, source });
      if (signature !== card.signature) {
        const scrollTop = card.nextScrollTop ?? card.output.scrollTop;
        card.nextScrollTop = undefined;
        card.output.replaceChildren();
        if (!source) {
          const empty = document.createElement("p");
          empty.textContent = entry.state === "running" ? controlRoom ? "No adapter logs available." : "No application/service logs assigned." : "Machine is not running.";
          card.output.append(empty);
        }
        if (source) {
          const lines = document.createElement(source.error ? "p" : "pre");
          if (source.error) {
            lines.textContent = source.error;
            lines.className = "machine-log-error";
          } else renderColoredLog(lines, source.lines.join("\n") || "No log entries.");
          card.output.append(lines);
        }
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
    refreshButton.disabled = true;
    try {
      const response = await fetch(endpoint, { cache: "no-store", signal: request.signal });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
      if (!active) return;
      machines = result.machines || [];
      render();
      document.getElementById("machine-logs-error").hidden = true;
      document.getElementById("machine-logs-time").textContent = new Date(result.updated_at).toLocaleTimeString();
      document.getElementById("machine-logs-status").textContent = `${result.organization_id} / ${paused ? "Paused" : "Live"}`;
    } catch (error) {
      if (error.name !== "AbortError" && active) {
        const message = document.getElementById("machine-logs-error");
        message.textContent = error.message || "Machine logs unavailable";
        message.hidden = false;
        document.getElementById("machine-logs-status").textContent = "Disconnected";
      }
    } finally {
      if (pending === request) pending = null;
      refreshButton.disabled = false;
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

  document.getElementById("machine-logs-pause").addEventListener("click", () => {
    paused = !paused;
    document.getElementById("machine-logs-pause").textContent = paused ? "Resume" : "Pause";
    clearTimeout(timer);
    if (paused) {
      pending?.abort();
      document.getElementById("machine-logs-status").textContent = "Paused";
    } else start();
  });
  document.getElementById("machine-logs-refresh").addEventListener("click", () => { clearTimeout(timer); refresh(); });
  for (const id of ["machine-logs-search", "machine-logs-running"]) document.getElementById(id).addEventListener("input", render);
  document.getElementById("machine-logs-follow").addEventListener("input", (event) => {
    if (event.target.checked) for (const card of cards.values()) {
      card.following = true;
      for (const view of card.sourceViews.values()) view.following = true;
    }
    render();
  });
  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    for (const card of cards.values()) {
      if (card.panel.classList.contains("maximized")) maximizeCard(card, false);
    }
  });
  if (controlRoom) {
    window.addEventListener("hashchange", () => { if (pageActive()) start(); else stop(); });
  } else {
    document.addEventListener("simulator:activate", (event) => { if (event.detail.path === "/machine-logs.html") start(); });
    document.addEventListener("simulator:deactivate", (event) => { if (event.detail.path === "/machine-logs.html") stop(); });
  }
  if (active) start();
})();