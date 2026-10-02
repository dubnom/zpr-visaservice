class LDAPOrgGraph extends HTMLElement {
  constructor() {
    super();
    this._directory = {};
    this._camera = { x: 0, y: 0, scale: 1 };
    this._selected = null;
  }

  set directory(value) {
    this._directory = value || {};
    this._selected = null;
    this._camera = { x: 0, y: 0, scale: 1 };
    if (this.isConnected) this.render();
  }

  get directory() {
    return this._directory;
  }

  connectedCallback() {
    this.render();
  }

  render() {
    const directory = this._directory;
    const root = { id: "root", kind: "directory", label: directory.base_dn || "Directory", value: {}, children: [] };
    const nodes = [root];
    const departments = new Map();
    const people = new Map();
    const create = (kind, label, value) => {
      const node = { id: `node-${nodes.length}`, kind, label: String(label || "Unnamed"), value, children: [] };
      nodes.push(node);
      return node;
    };
    for (const department of directory.departments || []) {
      if (!departments.has(department.name)) departments.set(department.name, create("department", department.name, department));
    }
    for (const department of departments.values()) {
      let parent = departments.get(department.value.parent);
      const visited = new Set([department]);
      let ancestor = parent;
      while (ancestor && !visited.has(ancestor)) {
        visited.add(ancestor);
        ancestor = departments.get(ancestor.value.parent);
      }
      if (ancestor) parent = null;
      (parent || root).children.push(department);
    }
    for (const person of directory.people || []) {
      const node = create("person", person.name || person.uid, person);
      if (person.uid) people.set(person.uid, node);
      (departments.get(person.department) || root).children.push(node);
    }
    const memberships = [];
    if (directory.groups?.length) {
      const groups = create("branch", "Groups", {});
      root.children.push(groups);
      for (const group of directory.groups) {
        const node = create("group", group.name, group);
        groups.children.push(node);
        for (const uid of group.members || []) {
          const member = people.get(uid);
          if (member) memberships.push({ from: node, to: member });
        }
      }
    }
    let row = 0;
    let depth = 0;
    const layout = (node, level) => {
      depth = Math.max(depth, level);
      node.x = 24 + level * 224;
      if (!node.children.length) node.y = 24 + row++ * 72;
      else {
        node.children.forEach((child) => layout(child, level + 1));
        node.y = (node.children[0].y + node.children.at(-1).y) / 2;
      }
    };
    layout(root, 0);
    this._nodes = nodes;
    this._memberships = memberships;
    this._width = Math.max(450, (depth + 1) * 224 + 24);
    this._height = Math.max(180, row * 72 + 24);
    this.replaceChildren();
    this.classList.add("ldap-org-graph");
    const toolbar = document.createElement("div");
    toolbar.className = "ldap-graph-toolbar";
    const search = document.createElement("input");
    search.type = "search";
    search.placeholder = "Search directory";
    search.setAttribute("aria-label", "Search directory graph");
    toolbar.append(search);
    const command = (label, text, handler) => {
      const button = document.createElement("button");
      button.type = "button";
      button.title = label;
      button.setAttribute("aria-label", label);
      button.textContent = text;
      button.addEventListener("click", handler);
      toolbar.append(button);
    };
    command("Zoom in", "+", () => this.zoom(this._camera.scale * 1.25));
    command("Zoom out", "-", () => this.zoom(this._camera.scale / 1.25));
    command("Fit graph", "Fit", () => { this._camera = { x: 0, y: 0, scale: 1 }; this.applyCamera(); });
    const svg = this.svgElement("svg", { viewBox: `0 0 ${this._width} ${this._height}`, role: "group", "aria-label": "LDAP organizational relationships" });
    this._svg = svg;
    const world = this.svgElement("g");
    this._world = world;
    svg.append(world);
    const edge = (from, to, className) => this.svgElement("path", {
      d: `M ${from.x + 190} ${from.y + 24} C ${from.x + 207} ${from.y + 24}, ${to.x - 17} ${to.y + 24}, ${to.x} ${to.y + 24}`,
      class: className,
    });
    for (const node of nodes) for (const child of node.children) world.append(edge(node, child, "ldap-graph-edge"));
    for (const membership of memberships) {
      membership.element = edge(membership.from, membership.to, "ldap-graph-membership");
      membership.element.setAttribute("hidden", "");
      world.append(membership.element);
    }
    for (const node of nodes) {
      const element = this.svgElement("g", { class: `ldap-graph-node ${node.kind}`, transform: `translate(${node.x} ${node.y})`, tabindex: "0", role: "button", "aria-label": `${node.kind}: ${node.label}` });
      const title = this.svgElement("title");
      title.textContent = node.label;
      const rect = this.svgElement("rect", { width: "190", height: "48", rx: "5" });
      const label = this.svgElement("text", { x: "10", y: "19" });
      label.textContent = node.label.length > 24 ? node.label.slice(0, 21) + "..." : node.label;
      const subtitle = this.svgElement("text", { x: "10", y: "36", class: "ldap-graph-kind" });
      subtitle.textContent = node.kind === "branch" ? "Group membership" : node.kind;
      element.append(title, rect, label, subtitle);
      element.addEventListener("click", () => this.select(node));
      element.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); this.select(node); }
      });
      node.element = element;
      world.append(element);
    }
    const detail = document.createElement("div");
    detail.className = "ldap-graph-details";
    detail.setAttribute("aria-live", "polite");
    this._detail = detail;
    search.addEventListener("input", () => {
      const query = search.value.trim().toLowerCase();
      for (const node of nodes) node.element.classList.toggle("dimmed", !!query && ![node.label, ...Object.values(node.value).flat()].join(" ").toLowerCase().includes(query));
    });
    let drag;
    const pointAt = (event) => new DOMPoint(event.clientX, event.clientY).matrixTransform(svg.getScreenCTM().inverse());
    svg.addEventListener("wheel", (event) => {
      event.preventDefault();
      const point = pointAt(event);
      this.zoom(this._camera.scale * (event.deltaY < 0 ? 1.15 : 1 / 1.15), point.x, point.y);
    }, { passive: false });
    svg.addEventListener("pointerdown", (event) => {
      if (event.button !== 0 || event.target.closest(".ldap-graph-node")) return;
      drag = { point: pointAt(event), x: this._camera.x, y: this._camera.y, pointerId: event.pointerId };
      svg.setPointerCapture(event.pointerId);
    });
    svg.addEventListener("pointermove", (event) => {
      if (!drag || drag.pointerId !== event.pointerId) return;
      const point = pointAt(event);
      this._camera.x = drag.x + point.x - drag.point.x;
      this._camera.y = drag.y + point.y - drag.point.y;
      this.applyCamera();
    });
    const stopDrag = () => { drag = null; };
    svg.addEventListener("pointerup", stopDrag);
    svg.addEventListener("pointercancel", stopDrag);
    this.append(toolbar, svg, detail);
    this.applyCamera();
    this.select(root, false);
  }

  svgElement(tag, attributes = {}) {
    const element = document.createElementNS("http://www.w3.org/2000/svg", tag);
    for (const [name, value] of Object.entries(attributes)) element.setAttribute(name, value);
    return element;
  }

  zoom(nextScale, x = this._width / 2, y = this._height / 2) {
    const scale = Math.max(0.5, Math.min(8, nextScale));
    const ratio = scale / this._camera.scale;
    this._camera.x = x - (x - this._camera.x) * ratio;
    this._camera.y = y - (y - this._camera.y) * ratio;
    this._camera.scale = scale;
    this.applyCamera();
  }

  applyCamera() {
    this._world.setAttribute("transform", `translate(${this._camera.x} ${this._camera.y}) scale(${this._camera.scale})`);
  }

  select(node, notify = true) {
    this._selected = node.id;
    for (const item of this._nodes) {
      item.element.classList.toggle("selected", item === node);
      item.element.setAttribute("aria-pressed", String(item === node));
    }
    for (const edge of this._memberships) edge.element.toggleAttribute("hidden", edge.from !== node && edge.to !== node);
    const heading = document.createElement("strong");
    heading.textContent = node.label;
    const list = document.createElement("dl");
    const fields = { Type: node.kind, ...node.value };
    if (node.kind === "person") fields.Groups = this._memberships.filter((edge) => edge.to === node).map((edge) => edge.from.label);
    for (const [key, value] of Object.entries(fields)) {
      if (value == null || value === "") continue;
      const term = document.createElement("dt"), description = document.createElement("dd");
      term.textContent = key;
      description.textContent = Array.isArray(value) ? value.join(", ") || "None" : String(value);
      list.append(term, description);
    }
    this._detail.replaceChildren(heading, list);
    if (notify) this.dispatchEvent(new CustomEvent("ldap-node-select", { detail: { kind: node.kind, label: node.label, value: node.value }, bubbles: true, composed: true }));
  }
}

customElements.define("ldap-org-graph", LDAPOrgGraph);