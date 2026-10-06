(() => {
  if (customElements.get("trusted-source-browser")) return;

  const element = (tag, className, text) => {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  };

  class TrustedSourceBrowser extends HTMLElement {
    connectedCallback() {
      if (!this.ready) this.initialize();
      if (this.closest("#page-sources")) {
        if (!this.hashListener) {
          this.hashListener = () => {
            if (location.hash === "#sources" && !this.snapshot) this.load();
          };
          window.addEventListener("hashchange", this.hashListener);
        }
        if (location.hash === "#sources") this.load();
      } else {
        this.load();
      }
    }

    initialize() {
      this.ready = true;
      this.view = "people";
      this.innerHTML = `
        <section class="trusted-source-browser-panel" aria-label="Trusted source browser">
          <header class="trusted-source-browser-head">
            <div><h3 data-source-title>Trusted source</h3><p data-source-meta class="trusted-source-meta">Not loaded</p></div>
            <button type="button" class="button" data-source-refresh>Refresh</button>
          </header>
          <div class="trusted-source-summary" data-source-summary role="status"></div>
          <div class="trusted-source-browser-tools">
            <div class="trusted-source-tabs" role="tablist" aria-label="Trusted source records">
              <button type="button" role="tab" data-source-tab="tree" aria-selected="false" tabindex="-1">LDAP tree</button>
              <button type="button" role="tab" data-source-tab="people" aria-selected="true">People</button>
              <button type="button" role="tab" data-source-tab="groups" aria-selected="false" tabindex="-1">Groups</button>
              <button type="button" role="tab" data-source-tab="attributes" aria-selected="false" tabindex="-1">Attributes</button>
            </div>
            <label class="trusted-source-filter"><span>Filter</span><input type="search" data-source-filter aria-label="Filter trusted source records"></label>
            <span class="trusted-source-count" data-source-count></span>
          </div>
          <p class="trusted-source-message" data-source-message role="status" aria-live="polite">Loading trusted source…</p>
          <div class="trusted-source-results" data-source-results hidden></div>
        </section>`;
      this.addEventListener("click", (event) => {
        const refresh = event.target.closest("[data-source-refresh]");
        if (refresh) this.load();
        const tab = event.target.closest("[data-source-tab]");
        if (tab) {
          this.view = tab.dataset.sourceTab;
          for (const button of this.querySelectorAll("[data-source-tab]")) {
            const selected = button === tab;
            button.setAttribute("aria-selected", String(selected));
            button.tabIndex = selected ? 0 : -1;
          }
          this.render();
        }
      });
      this.querySelector("[data-source-filter]").addEventListener("input", () => this.render());
      if (this.closest("#page-sources")) {
        this.querySelector("[data-source-refresh]").hidden = true;
        document.addEventListener("control-room:refresh-requested", () => {
          if (location.hash === "#sources") this.load();
        });
      }
    }

    async load() {
      if (this.pending) return this.pending;
      const refresh = this.querySelector("[data-source-refresh]");
      const message = this.querySelector("[data-source-message]");
      refresh.disabled = true;
      message.hidden = false;
      message.textContent = "Reading trusted source…";
      this.pending = (async () => {
        try {
          const response = await fetch(this.dataset.endpoint, { cache: "no-store", headers: { Accept: "application/json" } });
          const data = await response.json();
          if (!response.ok) throw new Error(data.error || `Source read failed (${response.status})`);
          if (!data.directory) throw new Error("Trusted source did not return browseable records");
          this.snapshot = data;
          this.querySelector("[data-source-title]").textContent = data.source_name || "Trusted source";
          this.querySelector("[data-source-meta]").textContent = [data.organization_name, data.base_dn, data.observed_at && new Date(data.observed_at).toLocaleString()].filter(Boolean).join(" · ");
          this.querySelector("[data-source-summary]").textContent = `${data.people || 0} people · ${(data.groups || []).length} groups · ${(data.attributes || []).length} attributes`;
          this.render();
        } catch (error) {
          this.querySelector("[data-source-results]").hidden = true;
          this.querySelector("[data-source-count]").textContent = "";
          message.textContent = error.message || "Trusted source could not be read";
          message.hidden = false;
        } finally {
          refresh.disabled = false;
          this.pending = null;
        }
      })();
      return this.pending;
    }

    render() {
      if (!this.snapshot) return;
      const directory = this.snapshot.directory;
      const query = this.querySelector("[data-source-filter]").value.trim().toLowerCase();
      const results = this.querySelector("[data-source-results]");
      const message = this.querySelector("[data-source-message]");
      results.replaceChildren();
      if (this.view === "tree") this.renderTree(results, directory, query);
      else if (this.view === "people") this.renderPeople(results, directory, query);
      else if (this.view === "groups") this.renderGroups(results, directory, query);
      else this.renderAttributes(results, directory, query);
      const count = this.view === "tree" ? results.querySelectorAll("[data-ldap-entry]").length : results.querySelectorAll("tbody tr").length;
      this.querySelector("[data-source-count]").textContent = `${count} ${this.view}`;
      message.textContent = count ? "" : `No ${this.view} match this filter.`;
      message.hidden = count > 0;
      results.hidden = false;
    }

    renderTree(results, directory, query) {
      if (!Array.isArray(directory.entries) || !directory.entries.length) {
        results.append(element("p", "trusted-source-message", "This provider has not returned LDAP distinguished names. People, groups and attributes remain available."));
        return;
      }
      const root = { children: new Map() };
      // LDAP permits escaped commas inside RDN values; they are not path separators.
      const splitDN = (dn) => {
        const parts = [];
        let start = 0, escaped = false;
        for (let index = 0; index < dn.length; index++) {
          const character = dn[index];
          if (character === "," && !escaped) { parts.push(dn.slice(start, index).trim()); start = index + 1; }
          escaped = character === "\\" && !escaped;
        }
        parts.push(dn.slice(start).trim());
        return parts;
      };
      for (const entry of directory.entries) {
        if (!JSON.stringify(entry).toLowerCase().includes(query)) continue;
        let parent = root;
        for (const rdn of splitDN(entry.dn).reverse()) {
          const key = rdn.toLowerCase();
          if (!parent.children.has(key)) parent.children.set(key, { label: rdn, children: new Map() });
          parent = parent.children.get(key);
        }
        parent.entry = entry;
      }
      const list = element("ul", "trusted-source-tree");
      const add = (parent, nodes) => {
        for (const node of [...nodes.values()].sort((a, b) => a.label.localeCompare(b.label))) {
          const item = element("li");
          const branch = element("details");
          branch.open = Boolean(query) || !node.entry;
          const summary = element("summary", "", node.label);
          if (node.entry) {
            branch.dataset.ldapEntry = node.entry.dn;
            summary.title = node.entry.dn;
          }
          branch.append(summary);
          if (node.entry) {
            branch.append(element("p", "trusted-source-meta", node.entry.dn));
            const attributes = element("div", "trusted-source-attributes");
            attributes.append(...this.attributeCell(node.entry.attributes || {}).childNodes);
            branch.append(attributes);
          }
          if (node.children.size) {
            const children = element("ul");
            add(children, node.children);
            branch.append(children);
          }
          item.append(branch);
          parent.append(item);
        }
      };
      add(list, root.children);
      results.append(list);
    }

    renderPeople(results, directory, query) {
      const people = directory.people || [];
      const attributes = directory.person_attributes || {};
      const shown = people.filter((uid) => JSON.stringify([uid, attributes[uid] || {}]).toLowerCase().includes(query));
      const table = this.table(results, ["IDENTITY", "ATTRIBUTES"]);
      const body = table.querySelector("tbody");
      for (const uid of shown) {
        const row = element("tr");
        row.append(element("th", "trusted-source-identity", uid), this.attributeCell(attributes[uid] || {}));
        body.append(row);
      }
    }

    renderGroups(results, directory, query) {
      const groups = directory.groups || {};
      const attributes = directory.group_attributes || {};
      const names = Object.keys(groups).sort((left, right) => left.localeCompare(right));
      const shown = names.filter((name) => JSON.stringify([name, groups[name], attributes[name] || {}]).toLowerCase().includes(query));
      const table = this.table(results, ["GROUP", "MEMBERS", "ATTRIBUTES"]);
      const body = table.querySelector("tbody");
      for (const name of shown) {
        const row = element("tr");
        row.append(element("th", "trusted-source-identity", name));
        row.append(element("td", "trusted-source-members", (groups[name] || []).join(", ") || "No members"));
        row.append(this.attributeCell(attributes[name] || {}));
        body.append(row);
      }
    }

    renderAttributes(results, directory, query) {
      const summary = this.snapshot.attributes || [];
      const table = this.table(results, ["ATTRIBUTE", "PEOPLE", "GROUPS", "VALUES"]);
      const body = table.querySelector("tbody");
      for (const attribute of summary.filter((item) => item.name.toLowerCase().includes(query))) {
        const values = new Set();
        for (const record of Object.values(directory.person_attributes || {})) for (const value of record[attribute.name] || []) values.add(value);
        for (const record of Object.values(directory.group_attributes || {})) for (const value of record[attribute.name] || []) values.add(value);
        const row = element("tr");
        row.append(element("th", "trusted-source-identity", attribute.name));
        row.append(element("td", "trusted-source-number", String(attribute.people)));
        row.append(element("td", "trusted-source-number", String(attribute.groups)));
        row.append(element("td", "trusted-source-values", [...values].slice(0, 6).join(", ") || "No values"));
        body.append(row);
      }
    }

    table(target, headings) {
      const scroll = element("div", "trusted-source-table-scroll");
      const table = element("table", "trusted-source-table");
      const head = element("thead");
      const header = element("tr");
      for (const heading of headings) header.append(element("th", "", heading));
      head.append(header);
      table.append(head, element("tbody"));
      scroll.append(table);
      target.append(scroll);
      return table;
    }

    attributeCell(attributes) {
      const cell = element("td", "trusted-source-attributes");
      const list = element("dl");
      for (const name of Object.keys(attributes).sort((left, right) => left.localeCompare(right))) {
        const values = attributes[name] || [];
        if (!values.length) continue;
        list.append(element("dt", "", name), element("dd", "", values.join(", ")));
      }
      if (!list.childElementCount) cell.textContent = "No attributes";
      else cell.append(list);
      return cell;
    }
  }

  customElements.define("trusted-source-browser", TrustedSourceBrowser);
})();
