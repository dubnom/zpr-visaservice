(() => {
  const root = document.getElementById("world-content");
  if (!root) return;

  const status = document.getElementById("world-status");
  const error = document.getElementById("world-error");
  const release = document.getElementById("world-release");
  const refreshButton = document.getElementById("world-refresh");
  const snapshotURL = "/api/simulator/world";
  const machineURLs = Object.freeze({
    alice: "/api/simulator/world/machines/alice",
    "oci-web": "/api/simulator/world/machines/oci-web",
  });
  const serviceURLs = Object.freeze({
    "oci-web-http": "/api/simulator/world/services/oci-web",
  });
  const expectedMachineIDs = new Set(["alice", "oci-web"]);
  const expectedServiceIDs = new Set(["oci-web-http"]);
  const expectedPersonaIDs = new Set(["alice"]);
  let available = false;
  let operationPending = false;
  let snapshot;

  function isString(value) {
    return typeof value === "string";
  }

  function validSnapshot(value) {
    if (!value || typeof value !== "object" || value.version !== 1 ||
        typeof value.available !== "boolean" || !isString(value.release_id) ||
        !Array.isArray(value.machines) || !Array.isArray(value.services) ||
        !Array.isArray(value.personas)) return false;
    const machines = new Set();
    for (const machine of value.machines) {
      if (!machine || !expectedMachineIDs.has(machine.id) || machines.has(machine.id) ||
          !isString(machine.name) || !isString(machine.container_state) ||
          !isString(machine.adapter_state) || !isString(machine.identity_note)) return false;
      machines.add(machine.id);
    }
    const services = new Set();
    for (const service of value.services) {
      if (!service || !expectedServiceIDs.has(service.id) || services.has(service.id) ||
          !isString(service.name) || service.machine_id !== "oci-web" ||
          service.port !== 80 || !isString(service.state) || !isString(service.probe)) return false;
      services.add(service.id);
    }
    const personas = new Set();
    for (const persona of value.personas) {
      if (!persona || !expectedPersonaIDs.has(persona.id) || personas.has(persona.id) ||
          !isString(persona.name) || persona.machine_id !== "alice" ||
          persona.state !== "simulated-demo-principal") return false;
      personas.add(persona.id);
    }
    return machines.size === expectedMachineIDs.size &&
      services.size === expectedServiceIDs.size &&
      personas.size === expectedPersonaIDs.size;
  }

  function element(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function stateLine(label, value) {
    const row = element("div", "world-state-line");
    row.append(element("span", "world-state-label", label), element("strong", "world-state-value", value));
    return row;
  }

  function actionButton(label, target, action) {
    const button = element("button", "button world-action", label);
    button.type = "button";
    button.dataset.target = target;
    button.dataset.action = action;
    return button;
  }

  function card(title, identifier, details, target, controls) {
    const section = element("article", "world-card");
    const heading = element("div", "world-card-heading");
    const headingText = element("div");
    headingText.append(element("h3", "", title), element("p", "world-identifier", identifier));
    heading.append(headingText);
    if (controls) {
      const actions = element("div", "world-actions");
      actions.append(
        actionButton(`Start ${target.label}`, target.id, "start"),
        actionButton(`Stop ${target.label}`, target.id, "stop"),
      );
      heading.append(actions);
    }
    section.append(heading);
    const state = element("div", "world-state");
    for (const [label, value] of details) state.append(stateLine(label, value));
    section.append(state);
    return section;
  }

  function render(value) {
    snapshot = value;
    available = value.available;
    release.textContent = `Release: ${value.release_id}`;
    status.textContent = available
      ? "Paired demo runtime available. Displaying reported state."
      : "Paired demo runtime reports unavailable.";
    root.replaceChildren();

    const machineHeading = element("h2", "", "Machines");
    root.append(machineHeading);
    const machineGrid = element("div", "world-grid");
    for (const machine of value.machines) {
      machineGrid.append(card(machine.name, machine.id, [
        ["Container", machine.container_state],
        ["Adapter", machine.adapter_state],
        ["Identity", machine.identity_note],
      ], { id: machine.id, label: machine.name }, true));
    }
    root.append(machineGrid);

    root.append(element("h2", "world-section-title", "Services"));
    const serviceGrid = element("div", "world-grid world-grid-services");
    for (const service of value.services) {
      serviceGrid.append(card(service.name, `${service.id} · ${service.port}/TCP`, [
        ["State", service.state],
        ["Probe", service.probe],
        ["Machine", service.machine_id],
      ], { id: service.id, label: service.name }, true));
    }
    root.append(serviceGrid);

    root.append(element("h2", "world-section-title", "Personas"));
    const personaGrid = element("div", "world-grid world-grid-personas");
    for (const persona of value.personas) {
      personaGrid.append(card(persona.name, persona.id, [
        ["State", persona.state],
        ["Machine", persona.machine_id],
      ], null, false));
    }
    root.append(personaGrid);
    root.setAttribute("aria-busy", "false");
    updateControls();
  }

  function updateControls() {
    root.querySelectorAll(".world-action").forEach(button => {
      button.disabled = !available || operationPending;
    });
    refreshButton.disabled = operationPending;
  }

  function showError(message) {
    release.textContent = "Release: unavailable";
    error.textContent = message;
    error.hidden = false;
    status.textContent = "Could not verify current demo world state.";
    root.setAttribute("aria-busy", "false");
  }

  async function refresh() {
    error.hidden = true;
    status.textContent = "Refreshing actual runtime state…";
    root.setAttribute("aria-busy", "true");
    try {
      const value = await window.ZPRPageRuntime.requestJSON(
        window.zprOperatorFetch,
        snapshotURL,
        { headers: { Accept: "application/json" } },
      );
      if (!validSnapshot(value)) {
        available = false;
        root.replaceChildren();
        updateControls();
        showError("The world API response does not match the expected version-1 contract. Controls are disabled; ask the operator to resolve the contract mismatch.");
        return;
      }
      render(value);
    } catch (requestError) {
      available = false;
      root.replaceChildren();
      updateControls();
      const code = Number.isInteger(requestError?.status) ? ` (HTTP ${requestError.status})` : "";
      showError(`Paired demo world state is unavailable${code}. The runtime may be missing or unpaired. Refresh to check again.`);
    }
  }

  function endpointFor(target) {
    return machineURLs[target] || serviceURLs[target];
  }

  async function performAction(target, action) {
    const endpoint = endpointFor(target);
    if (!endpoint || (action !== "start" && action !== "stop") || !available || operationPending) return;
    const item = [...(snapshot.machines || []), ...(snapshot.services || [])]
      .find(entry => entry.id === target);
    if (!item) return;
    const kind = machineURLs[target] ? "container" : "service";
    const name = item.name;
    const warning = `This will ${action} the real demo ${kind} “${name}” and may disrupt the selected demo. Continue?`;
    if (!window.confirm(warning)) return;

    operationPending = true;
    updateControls();
    status.textContent = `${action === "start" ? "Starting" : "Stopping"} ${name}…`;
    error.hidden = true;
    try {
      const response = await window.zprOperatorFetch(`${endpoint}/${action}`, { method: "POST" });
      if (!response.ok) {
        showError(`The ${action} request for ${name} failed (HTTP ${response.status}). Refresh to inspect actual state.`);
        return;
      }
      status.textContent = `${action === "start" ? "Start" : "Stop"} request returned HTTP ${response.status}; refreshing actual state.`;
      await refresh();
    } catch {
      showError(`The ${action} request for ${name} could not be confirmed. Refresh to inspect actual state.`);
    } finally {
      operationPending = false;
      updateControls();
    }
  }

  refreshButton.addEventListener("click", () => { void refresh(); });
  root.addEventListener("click", event => {
    const button = event.target.closest("button[data-target][data-action]");
    if (!button || !root.contains(button)) return;
    void performAction(button.dataset.target, button.dataset.action);
  });
  void refresh();
})();
