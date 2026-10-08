(() => {
if (customElements.get("ldap-org-graph")) return;

class LDAPOrgGraph extends HTMLElement {
  constructor() {
    super();
    this._directory = {};
    this._camera = { x: 0, y: 0, scale: 1 };
    this._selected = null;
    this._collapsed = new Set();
    this._query = "";
  }

  set directory(value) {
    this._directory = value || {};
    this._selected = null;
    this._collapsed.clear();
    this._query = "";
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
    const rootValue = Object.fromEntries(Object.entries(directory).filter(([key]) => !["departments", "people", "groups"].includes(key)));
    const root = { id: "root", kind: "directory", label: directory.base_dn || "Directory", value: rootValue, children: [] };
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
    const visible = [];
    const layout = (node, level) => {
      visible.push(node);
      depth = Math.max(depth, level);
      node.x = 24 + level * 224;
      if (!node.children.length || this._collapsed.has(node.id)) node.y = 24 + row++ * 72;
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
    if (this._dialog?.open) this._dialog.close();
    this.replaceChildren();
    this.classList.add("ldap-org-graph");
    const toolbar = document.createElement("div");
    toolbar.className = "ldap-graph-toolbar";
    const search = document.createElement("input");
    search.type = "search";
    search.value = this._query;
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
    command("Expand all branches", "Expand all", () => { this._collapsed.clear(); this.render(); });
    command("Collapse all branches", "Collapse all", () => {
      this._collapsed = new Set(nodes.filter((node) => node.children.length && node !== root).map((node) => node.id));
      this.render();
    });
    const svg = this.svgElement("svg", { viewBox: `0 0 ${this._width} ${this._height}`, role: "group", "aria-label": "LDAP organizational relationships" });
    this._svg = svg;
    const world = this.svgElement("g");
    this._world = world;
    svg.append(world);
    const edge = (from, to, className) => this.svgElement("path", {
      d: `M ${from.x + 190} ${from.y + 24} C ${from.x + 207} ${from.y + 24}, ${to.x - 17} ${to.y + 24}, ${to.x} ${to.y + 24}`,
      class: className,
    });
    const visibleNodes = new Set(visible);
    for (const node of visible) for (const child of node.children) if (visibleNodes.has(child)) world.append(edge(node, child, "ldap-graph-edge"));
    for (const membership of memberships) {
      if (!visibleNodes.has(membership.from) || !visibleNodes.has(membership.to)) continue;
      membership.element = edge(membership.from, membership.to, "ldap-graph-membership");
      membership.element.setAttribute("hidden", "");
      world.append(membership.element);
    }
    for (const node of visible) {
      const element = this.svgElement("g", { class: `ldap-graph-node ${node.kind}`, transform: `translate(${node.x} ${node.y})`, tabindex: "0", role: "button", "aria-label": `${node.kind}: ${node.label}` });
      const title = this.svgElement("title");
      title.textContent = node.label;
      const rect = this.svgElement("rect", { width: "190", height: "48", rx: "5" });
      const label = this.svgElement("text", { x: "10", y: "19" });
      const labelLimit = node.children.length ? 20 : 24;
      label.textContent = node.label.length > labelLimit ? node.label.slice(0, labelLimit - 3) + "..." : node.label;
      const subtitle = this.svgElement("text", { x: "10", y: "36", class: "ldap-graph-kind" });
      subtitle.textContent = this._collapsed.has(node.id) ? `${node.children.length} hidden items` : node.kind === "branch" ? "Group membership" : node.kind;
      element.append(title, rect, label, subtitle);
      element.addEventListener("click", () => this.select(node));
      element.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); this.select(node); }
        if (node.children.length && ["ArrowLeft", "ArrowRight"].includes(event.key)) {
          event.preventDefault();
          this.toggleBranch(node, event.key === "ArrowLeft");
        }
      });
      node.element = element;
      world.append(element);
      if (node.children.length) {
        const expanded = !this._collapsed.has(node.id);
        const toggle = this.svgElement("g", { class: "ldap-graph-toggle", transform: `translate(${node.x + 172} ${node.y + 24})`, tabindex: "0", role: "button", "aria-expanded": String(expanded), "aria-label": `${expanded ? "Collapse" : "Expand"} ${node.label}`, "data-branch-id": node.id });
        const toggleTitle = this.svgElement("title");
        toggleTitle.textContent = `${expanded ? "Collapse" : "Expand"} ${node.label}`;
        const symbol = this.svgElement("text", { x: "0", y: "4", "text-anchor": "middle" });
        symbol.textContent = expanded ? "-" : "+";
        toggle.append(toggleTitle, this.svgElement("circle", { r: "11" }), symbol);
        toggle.addEventListener("click", () => this.toggleBranch(node));
        toggle.addEventListener("keydown", (event) => {
          if (event.key === "Enter" || event.key === " ") { event.preventDefault(); this.toggleBranch(node); }
          if (["ArrowLeft", "ArrowRight"].includes(event.key)) {
            event.preventDefault();
            this.toggleBranch(node, event.key === "ArrowLeft");
          }
        });
        world.append(toggle);
      }
    }
    const dialog = document.createElement("dialog");
    dialog.className = "ldap-graph-dialog";
    const header = document.createElement("header");
    header.className = "ldap-graph-dialog-header";
    const heading = document.createElement("h2");
    this._detailHeading = heading;
    const close = document.createElement("button");
    close.type = "button";
    close.textContent = "\u00d7";
    close.title = "Close component info";
    close.setAttribute("aria-label", "Close component info");
    close.setAttribute("autofocus", "");
    close.addEventListener("click", () => dialog.close());
    header.append(heading, close);
    const detail = document.createElement("div");
    detail.className = "ldap-graph-details";
    detail.setAttribute("aria-live", "polite");
    this._detail = detail;
    dialog.append(header, detail);
    dialog.addEventListener("click", (event) => {
      if (event.target !== dialog) return;
      const bounds = dialog.getBoundingClientRect();
      if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) dialog.close();
    });
    dialog.addEventListener("close", () => this._nodes.find((node) => node.id === this._selected)?.element?.focus({ preventScroll: true }));
    this._dialog = dialog;
    const applySearch = () => {
      this._query = search.value;
      const query = this._query.trim().toLowerCase();
      for (const node of visible) node.element.classList.toggle("dimmed", !!query && !`${node.label} ${JSON.stringify(node.value)}`.toLowerCase().includes(query));
    };
    search.addEventListener("input", applySearch);
    applySearch();
    let drag;
    const pointAt = (event) => new DOMPoint(event.clientX, event.clientY).matrixTransform(svg.getScreenCTM().inverse());
    svg.addEventListener("wheel", (event) => {
      event.preventDefault();
      const point = pointAt(event);
      this.zoom(this._camera.scale * (event.deltaY < 0 ? 1.15 : 1 / 1.15), point.x, point.y);
    }, { passive: false });
    svg.addEventListener("pointerdown", (event) => {
      if (event.button !== 0 || event.target.closest(".ldap-graph-node, .ldap-graph-toggle")) return;
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
    this.append(toolbar, svg, dialog);
    this.applyCamera();
    this.select(visible.find((node) => node.id === this._selected) || root, false);
  }

  toggleBranch(node, collapsed = !this._collapsed.has(node.id)) {
    if (collapsed) {
      this._collapsed.add(node.id);
      const containsSelection = (item) => item.id === this._selected || item.children.some(containsSelection);
      if (node.children.some(containsSelection)) this._selected = node.id;
    } else this._collapsed.delete(node.id);
    this.render();
    this.querySelector(`[data-branch-id="${node.id}"]`)?.focus({ preventScroll: true });
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
      item.element?.classList.toggle("selected", item === node);
      item.element?.setAttribute("aria-pressed", String(item === node));
    }
    for (const edge of this._memberships) edge.element?.toggleAttribute("hidden", edge.from !== node && edge.to !== node);
    this._detailHeading.textContent = node.label;
    this._dialog.setAttribute("aria-label", `${node.label} component info`);
    const list = document.createElement("dl");
    const fields = { Type: node.kind, ...node.value };
    if (node.kind === "person") fields.Groups = (this._directory.groups || []).filter((group) => (group.members || []).includes(node.value.uid)).map((group) => group.name);
    if (node.kind === "group") fields.Members = (node.value.members || []).map((uid) => {
      const person = (this._directory.people || []).find((item) => item.uid === uid);
      return person?.name ? `${person.name} (${uid})` : uid;
    });
    if (node.children.length) fields.Children = node.children.map((child) => child.label);
    const appendField = (key, value) => {
      if (value == null || value === "") return;
      if (typeof value === "object" && !Array.isArray(value)) {
        for (const [attribute, entry] of Object.entries(value)) appendField(`${key}.${attribute}`, entry);
        return;
      }
      const term = document.createElement("dt"), description = document.createElement("dd");
      term.textContent = key;
      description.textContent = Array.isArray(value) ? value.map((entry) => typeof entry === "object" ? JSON.stringify(entry) : String(entry)).join(", ") || "None" : String(value);
      list.append(term, description);
    };
    for (const [key, value] of Object.entries(fields)) appendField(key, value);
    this._detail.replaceChildren(list);
    if (notify) {
      if (!this._dialog.open) this._dialog.showModal();
      this.dispatchEvent(new CustomEvent("ldap-node-select", { detail: { kind: node.kind, label: node.label, value: node.value }, bubbles: true, composed: true }));
    }
  }
}

customElements.define("ldap-org-graph", LDAPOrgGraph);
})();