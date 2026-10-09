(() => {
  "use strict";
  const project = (latitude, longitude) => {
    if (typeof latitude !== "number" || typeof longitude !== "number" ||
        !Number.isFinite(latitude) || !Number.isFinite(longitude) ||
        Math.abs(latitude) > 90 || Math.abs(longitude) > 180) return null;
    return { x: Number(((longitude + 180) * 5).toFixed(6)), y: Number(((90 - latitude) * 5).toFixed(6)) };
  };
  class Geography extends HTMLElement {
    connectedCallback() {
      if (this.initialized) return;
      this.initialized = true;
      this.innerHTML = `<div class="geography-switch" role="group" aria-label="Map view">
        <button type="button" data-map-view="topology" aria-pressed="true">Topology</button>
        <button type="button" data-map-view="geography" aria-pressed="false">World Map</button>
      </div><section class="geography-panel" hidden aria-label="World Map node choices">
        <div class="geography-group" hidden></div>
        <div class="geography-unplaced"></div>
      </section>`;
      this.onClick = (event) => {
        const view = event.target.closest("[data-map-view]");
        if (view) this.setView(view.dataset.mapView);
      };
      this.addEventListener("click", this.onClick);
      this.onNodeActivation = (event) => {
        if (this.closest("#page-map")?.dataset.mapView !== "geography" ||
            (event.type === "keydown" && event.key !== "Enter" && event.key !== " ")) return;
        const node = event.target.closest(".graph-vertex.node[data-inspect-actor]");
        const groupKey = node && this.nodeGroups?.get(node.dataset.inspectActor);
        if (!groupKey || this.groups.get(groupKey).nodes.length < 2) return;
        event.preventDefault();
        event.stopPropagation();
        this.showGroup(groupKey);
        this.querySelector(".geography-group button")?.focus();
      };
      document.addEventListener("click", this.onNodeActivation, true);
      document.addEventListener("keydown", this.onNodeActivation, true);
      this.onSnapshot = (event) => this.update(event.detail.actors || []);
      window.addEventListener("zpr-snapshot", this.onSnapshot);
      this.onSelection = (event) => {
        this.selectedCN = event.detail?.kind === "actor" ? event.detail.key : null;
        this.updateSelection();
      };
      window.addEventListener("zpr-selection", this.onSelection);
      this.setView("topology");
      this.onHash = () => {
        if (location.hash !== "#map") this.setView("topology");
      };
      window.addEventListener("hashchange", this.onHash);
    }
    disconnectedCallback() {
      this.removeEventListener("click", this.onClick);
      document.removeEventListener("click", this.onNodeActivation, true);
      document.removeEventListener("keydown", this.onNodeActivation, true);
      window.removeEventListener("zpr-snapshot", this.onSnapshot);
      window.removeEventListener("zpr-selection", this.onSelection);
      window.removeEventListener("hashchange", this.onHash);
      this.initialized = false;
    }
    setView(view) {
      const page = this.closest("#page-map");
      if (!page) return;
      page.dataset.mapView = view;
      this.querySelector(".geography-panel").hidden = view !== "geography";
      for (const button of this.querySelectorAll("[data-map-view]")) {
        button.setAttribute("aria-pressed", String(button.dataset.mapView === view));
      }
      window.dispatchEvent(new CustomEvent("zpr-map-view", { detail: view }));
    }
    nodeButton(node) {
      const button = document.createElement("button");
      button.type = "button";
      button.dataset.inspectActor = node.cn;
      button.textContent = node.cn;
      button.setAttribute("aria-label", `Inspect node ${node.cn}`);
      return button;
    }
    updateSelection() {
      for (const button of this.querySelectorAll("[data-inspect-actor]")) {
        button.setAttribute("aria-pressed", String(button.dataset.inspectActor === this.selectedCN));
      }
    }
    showGroup(key) {
      this.groupKey = key;
      const panel = this.querySelector(".geography-group");
      const group = this.groups.get(key);
      panel.replaceChildren();
      panel.hidden = !group || group.nodes.length < 2;
      if (panel.hidden) return;
      const heading = document.createElement("p");
      heading.textContent = `${group.nodes.length} nodes at ${group.latitude}, ${group.longitude}`;
      panel.append(heading, ...group.nodes.map((node) => this.nodeButton(node)));
      this.updateSelection();
    }
    update(actors) {
      const focused = this.contains(document.activeElement) ? document.activeElement : null;
      const focusKey = focused?.dataset.inspectActor;
      const focusZones = ".geography-group, .geography-unplaced";
      const focusZone = focused?.closest(focusZones);
      this.groups = new Map();
      this.nodeGroups = new Map();
      const unplaced = [];
      for (const node of actors.filter((actor) => actor.node)) {
        const { latitude, longitude } = node.node_details || {};
        const position = project(latitude, longitude);
        if (!position) {
          const absent = latitude == null && longitude == null;
          unplaced.push({ node, reason: absent ? "Location not configured" : "Invalid coordinates" });
          continue;
        }
        const key = `${latitude},${longitude}`;
        if (!this.groups.has(key)) this.groups.set(key, { latitude, longitude, nodes: [] });
        this.groups.get(key).nodes.push(node);
        this.nodeGroups.set(node.cn, key);
      }
      const missing = this.querySelector(".geography-unplaced");
      missing.replaceChildren();
      for (const { node, reason } of unplaced) {
        const row = document.createElement("div");
        const message = document.createElement("span");
        message.textContent = reason;
        row.append(this.nodeButton(node), message);
        missing.append(row);
      }
      this.showGroup(this.groupKey);
      this.updateSelection();
      if (focusKey) {
        const candidates = [...this.querySelectorAll("[data-inspect-actor]")]
          .filter((element) => element.dataset.inspectActor === focusKey);
        const replacement = candidates.find((element) => element.closest(focusZones) === focusZone) || candidates[0];
        if (replacement) replacement.focus();
        else this.querySelector('[data-map-view="geography"]').focus();
      }
    }
  }
  window.ZPRGeography = Object.freeze({ project });
  customElements.define("zpr-geography", Geography);
})();
