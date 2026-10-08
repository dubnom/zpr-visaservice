(() => {
  "use strict";
  const svgNS = "http://www.w3.org/2000/svg";
  const project = (latitude, longitude) => {
    if (typeof latitude !== "number" || typeof longitude !== "number" ||
        !Number.isFinite(latitude) || !Number.isFinite(longitude) ||
        Math.abs(latitude) > 90 || Math.abs(longitude) > 180) return null;
    return { x: Number(((longitude + 180) * 5).toFixed(6)), y: Number(((90 - latitude) * 5).toFixed(6)) };
  };
  const svgElement = (tag, attributes = {}) => {
    const element = document.createElementNS(svgNS, tag);
    for (const [key, value] of Object.entries(attributes)) element.setAttribute(key, String(value));
    return element;
  };
  class Geography extends HTMLElement {
    connectedCallback() {
      if (this.initialized) return;
      this.initialized = true;
      this.innerHTML = `<div class="geography-switch" role="group" aria-label="Map view">
        <button type="button" data-map-view="topology" aria-pressed="true">Topology</button>
        <button type="button" data-map-view="geography" aria-pressed="false">Geography</button>
      </div><section class="geography-panel" hidden aria-label="Node geography">
        <p class="geography-status" role="status">Waiting for node data.</p>
        <div class="geography-canvas"></div>
        <div class="geography-group" hidden></div>
        <details class="geography-located"><summary>Nodes with locations</summary><div></div></details>
        <div class="geography-unplaced"></div>
        <p class="geography-attribution">Land: <a href="https://www.naturalearthdata.com/about/terms-of-use/" target="_blank" rel="noopener noreferrer">Natural Earth</a>, public domain. Equirectangular projection. Node placement only; use Topology for links and runtime state.</p>
      </section>`;
      this.onClick = (event) => {
        const view = event.target.closest("[data-map-view]");
        if (view) this.setView(view.dataset.mapView);
        const group = event.target.closest("[data-geography-group]");
        if (group) this.showGroup(group.dataset.geographyGroup);
      };
      this.addEventListener("click", this.onClick);
      this.onKeydown = (event) => {
        const marker = event.target.closest(".geography-marker");
        if (marker && (event.key === "Enter" || event.key === " ")) {
          event.preventDefault();
          event.stopPropagation();
          marker.dispatchEvent(new MouseEvent("click", { bubbles: true }));
        }
      };
      this.addEventListener("keydown", this.onKeydown);
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
      this.removeEventListener("keydown", this.onKeydown);
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
      for (const marker of this.querySelectorAll(".geography-marker")) {
        const group = this.groups?.get(marker.dataset.geographyGroup);
        const selected = this.selectedCN != null && (marker.dataset.inspectActor === this.selectedCN ||
          Boolean(group?.nodes.some((node) => node.cn === this.selectedCN)));
        marker.classList.toggle("selected", selected);
        marker.setAttribute("aria-pressed", String(selected));
      }
    }
    showGroup(key) {
      this.groupKey = key;
      const panel = this.querySelector(".geography-group");
      const group = this.groups.get(key);
      panel.replaceChildren();
      panel.hidden = !group;
      if (!group) return;
      const heading = document.createElement("p");
      heading.textContent = `${group.nodes.length} nodes at ${group.latitude}, ${group.longitude}`;
      panel.append(heading, ...group.nodes.map((node) => this.nodeButton(node)));
    }
    update(actors) {
      const focused = this.contains(document.activeElement) ? document.activeElement : null;
      const focusKey = focused?.dataset.inspectActor || focused?.dataset.geographyGroup;
      const focusZones = ".geography-located, .geography-group, .geography-unplaced, .geography-canvas";
      const focusZone = focused?.closest(focusZones);
      this.groups = new Map();
      const unplaced = [];
      let placed = 0;
      for (const node of actors.filter((actor) => actor.node)) {
        const { latitude, longitude } = node.node_details || {};
        const position = project(latitude, longitude);
        if (!position) {
          const absent = latitude == null && longitude == null;
          unplaced.push({ node, reason: absent ? "Location not configured" : "Invalid coordinates" });
          continue;
        }
        placed++;
        const key = `${latitude},${longitude}`;
        if (!this.groups.has(key)) this.groups.set(key, { latitude, longitude, position, nodes: [] });
        this.groups.get(key).nodes.push(node);
      }
      const svg = svgElement("svg", { viewBox: "-25 -25 1850 950", role: "group", "aria-label": "World map node locations" });
      svg.append(svgElement("image", { href: "/geography-land.svg", width: 1800, height: 900 }));
      for (const [key, group] of this.groups) {
        const single = group.nodes.length === 1;
        const label = single ? `Inspect node ${group.nodes[0].cn}` : `Choose ${group.nodes.length} nodes at ${key}`;
        const marker = svgElement("g", {
          class: "geography-marker", tabindex: 0, role: "button",
          "aria-label": label, transform: `translate(${group.position.x},${group.position.y})`,
          ...(single ? { "data-inspect-actor": group.nodes[0].cn } : { "data-geography-group": key }),
        });
        const title = svgElement("title");
        title.textContent = `${group.nodes.map((node) => node.cn).join(", ")} (${key})`;
        marker.append(title, svgElement("circle", { r: 18 }));
        const text = svgElement("text", { y: 6, "text-anchor": "middle" });
        text.textContent = single ? "N" : String(group.nodes.length);
        marker.append(text);
        svg.append(marker);
      }
      this.querySelector(".geography-canvas").replaceChildren(svg);
      this.querySelector(".geography-status").textContent = `${placed} nodes placed; ${unplaced.length} without valid coordinates.`;
      const located = this.querySelector(".geography-located > div");
      located.replaceChildren();
      for (const group of this.groups.values()) {
        for (const node of group.nodes) {
          const row = document.createElement("div");
          const coordinates = document.createElement("span");
          coordinates.textContent = `${group.latitude}, ${group.longitude}`;
          row.append(this.nodeButton(node), coordinates);
          located.append(row);
        }
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
        const candidates = [...this.querySelectorAll("[data-inspect-actor], [data-geography-group]")]
          .filter((element) => (element.dataset.inspectActor || element.dataset.geographyGroup) === focusKey);
        const replacement = candidates.find((element) => element.closest(focusZones) === focusZone) || candidates[0];
        if (replacement) replacement.focus();
        else this.querySelector('[data-map-view="geography"]').focus();
      }
    }
  }
  window.ZPRGeography = Object.freeze({ project });
  customElements.define("zpr-geography", Geography);
})();
