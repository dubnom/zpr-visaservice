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
      this.simulatorPage = !this.closest("#page-sources");
      this.view = this.simulatorPage ? "graph" : "people";
      this.sorts = { people: { key: "identity", direction: 1 }, groups: { key: "group", direction: 1 }, attributes: { key: "attribute", direction: 1 } };
      this.expandedPeople = new Set();
      this.innerHTML = `
        <section class="trusted-source-browser-panel" aria-label="Trusted source browser">
          <header class="trusted-source-browser-head">
            <div><h3 data-source-title>Trusted source</h3>${this.simulatorPage ? "" : `<select data-source-selector class="trusted-source-heading-select" aria-label="Trusted source" hidden></select>`}<p data-source-meta class="trusted-source-meta">Not loaded</p></div>
            <button type="button" class="button" data-source-refresh>Refresh</button>
          </header>
          <div class="trusted-source-summary" data-source-summary role="status"></div>
          <div class="trusted-source-browser-tools">
            <div class="trusted-source-tabs" role="tablist" aria-label="Trusted source records">
              ${this.simulatorPage ? `<button type="button" role="tab" data-source-tab="graph" aria-selected="true">LDAP graph</button>` : ""}
              <button type="button" role="tab" data-source-tab="tree" aria-selected="false" tabindex="-1">LDAP tree</button>
              <button type="button" role="tab" data-source-tab="people" aria-selected="${this.simulatorPage ? "false" : "true"}"${this.simulatorPage ? ' tabindex="-1"' : ""}>People</button>
              <button type="button" role="tab" data-source-tab="groups" aria-selected="false" tabindex="-1">Groups</button>
              <button type="button" role="tab" data-source-tab="attributes" aria-selected="false" tabindex="-1">Attributes</button>
              ${this.simulatorPage ? "" : `<button type="button" role="tab" data-source-tab="updates" aria-selected="false" tabindex="-1">Updates (24h)</button>`}
            </div>
            <label class="trusted-source-filter"><span>Filter</span><input type="search" data-source-filter aria-label="Filter trusted source records"></label>
            <span class="trusted-source-count" data-source-count></span>
          </div>
          ${this.simulatorPage ? "" : `<div class="trusted-source-update-controls" data-source-update-controls hidden>
            <button type="button" class="button" data-source-load-more hidden>Load more updates</button>
          </div>`}
          <p class="trusted-source-message" data-source-message role="status" aria-live="polite">Loading trusted source…</p>
          <div class="trusted-source-results" data-source-results hidden></div>
        </section>`;
      this.addEventListener("click", (event) => {
        const refresh = event.target.closest("[data-source-refresh]");
        if (refresh) this.view === "updates" ? this.loadUpdates(true) : this.load();
        const loadMore = event.target.closest("[data-source-load-more]");
        if (loadMore) this.loadUpdates(false);
        const person = event.target.closest("tr.trusted-source-person");
        if (person && !String(window.getSelection?.() || "")) {
          const uid = person.querySelector("[data-source-person]")?.dataset.sourcePerson;
          if (uid !== undefined) this.togglePerson(uid);
          return;
        }
        const tab = event.target.closest("[data-source-tab]");
        if (tab) {
          this.view = tab.dataset.sourceTab;
          for (const button of this.querySelectorAll("[data-source-tab]")) {
            const selected = button === tab;
            button.setAttribute("aria-selected", String(selected));
            button.tabIndex = selected ? 0 : -1;
          }
          this.render();
          if (this.view === "updates") this.loadUpdates(true);
        }
      });
      this.querySelector("[data-source-filter]").addEventListener("input", () => this.render());
      if (this.closest("#page-sources")) {
        this.querySelector("[data-source-refresh]").hidden = true;
        this.querySelector("[data-source-selector]").addEventListener("change", () => this.loadUpdates(true));
        document.addEventListener("control-room:refresh-requested", () => {
          if (location.hash === "#sources") {
            if (this.view === "updates") this.loadUpdates(true);
            else this.load();
          }
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
          this.renderSourceHeading();
          this.querySelector("[data-source-meta]").textContent = [data.organization_name, data.base_dn, data.observed_at && window.ZPRSafeDisplay.formatDateTime(data.observed_at)].filter(Boolean).join(" · ");
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
      if (this.view === "updates") {
        this.renderUpdates();
        return;
      }
      this.renderSourceHeading();
      if (!this.simulatorPage) this.querySelector("[data-source-update-controls]").hidden = true;
      const directory = this.snapshot.directory;
      const query = this.querySelector("[data-source-filter]").value.trim().toLowerCase();
      const results = this.querySelector("[data-source-results]");
      const message = this.querySelector("[data-source-message]");
      results.replaceChildren();
      if (this.view === "graph") this.renderGraph(results, directory);
      else if (this.view === "tree") this.renderTree(results, directory, query);
      else if (this.view === "people") this.renderPeople(results, directory, query);
      else if (this.view === "groups") this.renderGroups(results, directory, query);
      else this.renderAttributes(results, directory, query);
      const count = this.view === "graph" ? results.querySelectorAll(".ldap-graph-node").length : this.view === "tree" ? results.querySelectorAll("[data-ldap-entry]").length : results.querySelectorAll("tbody tr[data-source-row]").length;
      this.querySelector("[data-source-count]").textContent = `${count} ${this.view}`;
      message.textContent = count ? "" : `No ${this.view} match this filter.`;
      message.hidden = count > 0;
      results.hidden = false;
    }

    async loadUpdates(reset = true) {
      if (!this.closest("#page-sources") || this.updatesPending) return this.updatesPending;
      const controls = this.querySelector("[data-source-update-controls]");
      const message = this.querySelector("[data-source-message]");
      const results = this.querySelector("[data-source-results]");
      const moreButton = this.querySelector("[data-source-load-more]");
      controls.hidden = false;
      message.hidden = false;
      results.hidden = true;
      message.textContent = "Loading trusted source updates…";
      moreButton.disabled = true;
      this.updatesPending = (async () => {
        try {
          if (!this.updateSources) {
            const response = await fetch("/api/trusted-sources/change-feeds", { cache: "no-store", headers: { Accept: "application/json" } });
            const data = await response.json();
            if (!response.ok) throw new Error(data.error || `Change feeds could not be read (${response.status})`);
            if (!Array.isArray(data.sources)) throw new Error("Change feed list is malformed");
            this.updateSources = data.sources;
            const select = this.querySelector("[data-source-selector]");
            select.replaceChildren(...this.updateSources.map((source) => {
              const option = element("option", "", source.display_name || source.name);
              option.value = source.name;
              return option;
            }));
            if (this.updateSources.length) {
              const matching = this.updateSources.find(source => source.name === this.directorySourceName());
              select.value = (matching || this.updateSources[0]).name;
            }
          }
          const sourceName = this.querySelector("[data-source-selector]").value;
          this.renderSourceHeading();
          if (!sourceName) {
            this.updates = [];
            this.updateCursor = "";
            this.updateMore = false;
            this.updateEmptyMessage = "No trusted-source change feeds are configured.";
            this.renderUpdates();
            return;
          }
          if (reset || sourceName !== this.updateSourceName) {
            this.updates = [];
            this.updateCursor = "";
            this.updateMore = true;
            this.updateSourceName = sourceName;
          }
          this.querySelector("[data-source-selector]").disabled = true;
          const query = new URLSearchParams({ limit: "100" });
          if (this.updateCursor) query.set("cursor", this.updateCursor);
          const response = await fetch(`/api/trusted-sources/change-feeds/${encodeURIComponent(sourceName)}/changes?${query}`, { cache: "no-store", headers: { Accept: "application/json" } });
          const data = await response.json();
          if (!response.ok) {
            if (response.status === 410) throw new Error("This update history expired. Refresh to load the current 24-hour window.");
            throw new Error(data.error || `Trusted source updates could not be read (${response.status})`);
          }
          if (!Array.isArray(data.changes) || typeof data.cursor !== "string" || typeof data.more !== "boolean") {
            throw new Error("Trusted source update response is malformed");
          }
          this.updates.push(...data.changes);
          this.updateCursor = data.cursor;
          this.updateMore = data.more;
          this.updateEmptyMessage = "No trusted-source updates in the last 24 hours.";
          if (this.view === "updates") this.renderUpdates();
        } catch (error) {
          this.updateError = error.message || "Trusted source updates could not be read";
          if (this.view === "updates") {
            results.hidden = true;
            message.hidden = false;
            message.textContent = this.updateError;
            moreButton.hidden = true;
          }
        } finally {
          this.querySelector("[data-source-selector]").disabled = false;
          moreButton.disabled = false;
          this.updatesPending = null;
        }
      })();
      return this.updatesPending;
    }

    renderSourceHeading() {
      const title = this.querySelector("[data-source-title]");
      const selector = this.querySelector("[data-source-selector]");
      const updates = !this.simulatorPage && this.view === "updates";
      this.querySelector("[data-source-meta]").hidden = updates;
      this.querySelector("[data-source-summary]").hidden = updates;
      const selected = updates && this.updateSources?.find(source => source.name === selector.value);
      title.textContent = selected
        ? selected.display_name || selected.name
        : `${this.simulatorPage ? this.dataset.titlePrefix || "" : ""}${this.directorySourceName() || (this.simulatorPage ? "Trusted source" : "Source unavailable")}`;
      title.hidden = Boolean(updates && this.updateSources?.length > 1);
      if (selector) selector.hidden = !title.hidden;
    }

    directorySourceName() {
      return (!this.simulatorPage && this.snapshot?.default_source) || this.snapshot?.source_name;
    }

    renderUpdates() {
      if (this.view !== "updates") return;
      const results = this.querySelector("[data-source-results]");
      const message = this.querySelector("[data-source-message]");
      const count = this.querySelector("[data-source-count]");
      const moreButton = this.querySelector("[data-source-load-more]");
      const changes = this.updates || [];
      const query = this.querySelector("[data-source-filter]").value.trim().toLowerCase();
      const shown = query ? changes.filter((change) => [
        change.time, change.type, change.dn, change.new_dn, ...(change.attributes || []),
      ].some((value) => String(value || "").toLowerCase().includes(query))) : changes;
      const table = element("table", "trusted-source-table trusted-source-changes");
      const head = element("thead");
      const heading = element("tr");
      for (const label of ["WHEN", "CHANGE", "ENTRY DN", "NEW DN", "ATTRIBUTE NAMES"]) {
        heading.append(element("th", "", label));
      }
      head.append(heading);
      table.append(head);
      const body = element("tbody");
      for (const change of shown) {
        const row = element("tr");
        const values = [
          window.ZPRSafeDisplay.formatDateTime(change.time),
          change.type,
          change.dn,
          change.new_dn || "—",
          Array.isArray(change.attributes) && change.attributes.length ? change.attributes.join(", ") : "—",
        ];
        for (const value of values) row.append(element("td", "", value));
        body.append(row);
      }
      table.append(body);
      results.replaceChildren(table);
      results.hidden = false;
      count.textContent = query ? `${shown.length} of ${changes.length} updates` : `${changes.length} updates`;
      message.hidden = shown.length > 0;
      message.textContent = shown.length ? "" : query && changes.length
        ? "No updates match this filter."
        : this.updateMore && changes.length === 0
          ? "No updates on this page; more history is available."
          : this.updateEmptyMessage || "No trusted-source updates in the last 24 hours.";
      moreButton.hidden = !this.updateMore;
      moreButton.disabled = Boolean(this.updatesPending);
    }

    renderGraph(results, directory) {
      if (!customElements.get("ldap-org-graph")) {
        results.append(element("p", "trusted-source-message", "LDAP graph is unavailable; use the People, Groups, or Attributes tabs."));
        return;
      }
      const values = (attributes, names) => {
        for (const name of names) {
          const entry = Object.entries(attributes || {}).find(([key]) => key.toLowerCase() === name.toLowerCase());
          if (entry?.[1]?.[0]) return entry[1][0];
        }
        return "";
      };
      const people = (directory.people || []).map((uid) => {
        const attributes = directory.person_attributes?.[uid] || {};
        return {
          uid,
          name: values(attributes, ["displayname", "cn"]) || uid,
          title: values(attributes, ["title"]),
          department: values(attributes, ["ou", "departmentnumber", "department"]),
          location: values(attributes, ["l", "locality"]),
        };
      });
      const departmentNames = [...new Set(people.map((person) => person.department).filter(Boolean))].sort((left, right) => left.localeCompare(right));
      const graph = element("ldap-org-graph");
      graph.setAttribute("aria-label", "Trusted source LDAP graph");
      graph.directory = {
        base_dn: this.snapshot.base_dn || "Directory",
        departments: departmentNames.map((name) => ({ name })),
        people,
        groups: Object.entries(directory.groups || {}).map(([name, members]) => ({ name, members })),
      };
      results.append(graph);
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
      const first = (uid, ...names) => {
        for (const name of names) {
          const value = (attributes[uid]?.[name] || [])[0];
          if (value) return value;
        }
        return "";
      };
      const text = (className, value) => element("td", className, value || "—");
      const columns = [
        { key: "identity", label: "IDENTITY", value: (uid) => uid, render: (uid) => this.personDisclosure(uid) },
        { key: "name", label: "NAME", value: (uid) => first(uid, "displayname", "cn"), render: (uid) => text("", first(uid, "displayname", "cn")) },
        { key: "title", label: "TITLE", value: (uid) => first(uid, "title"), render: (uid) => text("", first(uid, "title")) },
        { key: "unit", label: "UNIT", value: (uid) => first(uid, "ou", "departmentnumber"), render: (uid) => text("", first(uid, "ou", "departmentnumber")) },
        { key: "mail", label: "MAIL", value: (uid) => first(uid, "mail"), render: (uid) => text("trusted-source-mail", first(uid, "mail")) },
      ];
      this.sortableTable(results, "people", columns, shown, (row, uid) => {
        row.classList.add("trusted-source-person");
        const expanded = this.expandedPeople.has(uid);
        row.setAttribute("aria-expanded", String(expanded));
        if (!expanded) return [];
        const detail = element("tr", "trusted-source-detail-row");
        detail.id = this.detailID(uid);
        const cell = this.attributeCell(attributes[uid] || {});
        cell.colSpan = columns.length;
        detail.append(cell);
        return [detail];
      });
    }

    personDisclosure(uid) {
      const cell = element("th", "trusted-source-identity");
      const button = element("button", "trusted-source-disclosure", uid);
      button.type = "button";
      button.dataset.sourcePerson = uid;
      const expanded = this.expandedPeople.has(uid);
      button.setAttribute("aria-expanded", String(expanded));
      button.setAttribute("aria-label", `${expanded ? "Hide" : "Show"} attributes for ${uid}`);
      if (expanded) button.setAttribute("aria-controls", this.detailID(uid));
      cell.append(button);
      return cell;
    }

    detailID(uid) {
      return `trusted-source-person-${uid.replace(/[^A-Za-z0-9_-]/g, "_")}`;
    }

    togglePerson(uid) {
      if (this.expandedPeople.has(uid)) this.expandedPeople.delete(uid);
      else this.expandedPeople.add(uid);
      this.render();
      this.querySelector(`[data-source-person="${CSS.escape(uid)}"]`)?.focus();
    }

    renderGroups(results, directory, query) {
      const groups = directory.groups || {};
      const attributes = directory.group_attributes || {};
      const names = Object.keys(groups);
      const shown = names.filter((name) => JSON.stringify([name, groups[name], attributes[name] || {}]).toLowerCase().includes(query));
      const values = (name, attribute) => (attributes[name]?.[attribute] || []).join(", ");
      const others = (name) => Object.fromEntries(Object.entries(attributes[name] || {}).filter(([key]) => !["cn", "objectclass"].includes(key.toLowerCase())));
      const columns = [
        { key: "group", label: "GROUP", value: (name) => name, render: (name) => element("th", "trusted-source-identity", name) },
        { key: "cn", label: "CN", value: (name) => values(name, "cn"), render: (name) => element("td", "trusted-source-cn", values(name, "cn") || "—") },
        { key: "objectclass", label: "OBJECTCLASS", value: (name) => values(name, "objectclass"), render: (name) => element("td", "trusted-source-objectclass", values(name, "objectclass") || "—") },
        { key: "members", label: "MEMBERS", value: (name) => (groups[name] || []).length, render: (name) => element("td", "trusted-source-members", (groups[name] || []).join(", ") || "No members") },
        { key: "attributes", label: "ATTRIBUTES", value: (name) => Object.values(others(name)).flat().join(", "), render: (name) => this.attributeCell(others(name)) },
      ];
      this.sortableTable(results, "groups", columns, shown);
    }

    renderAttributes(results, directory, query) {
      const summary = (this.snapshot.attributes || []).filter((item) => item.name.toLowerCase().includes(query));
      const valuesFor = (name) => {
        const values = new Set();
        for (const record of Object.values(directory.person_attributes || {})) for (const value of record[name] || []) values.add(value);
        for (const record of Object.values(directory.group_attributes || {})) for (const value of record[name] || []) values.add(value);
        return [...values].slice(0, 6).join(", ");
      };
      const columns = [
        { key: "attribute", label: "ATTRIBUTE", value: (item) => item.name, render: (item) => element("th", "trusted-source-identity", item.name) },
        { key: "people", label: "PEOPLE", value: (item) => item.people, render: (item) => element("td", "trusted-source-number", String(item.people)) },
        { key: "groups", label: "GROUPS", value: (item) => item.groups, render: (item) => element("td", "trusted-source-number", String(item.groups)) },
        { key: "values", label: "VALUES", value: (item) => valuesFor(item.name), render: (item) => element("td", "trusted-source-values", valuesFor(item.name) || "No values") },
      ];
      this.sortableTable(results, "attributes", columns, summary);
    }

    // Renders rows sorted by the view's selected column; afterRow may return extra rows (e.g. expanded details).
    sortableTable(target, view, columns, items, afterRow = () => []) {
      const sort = this.sorts[view];
      const column = columns.find((candidate) => candidate.key === sort.key) || columns[0];
      const compare = (left, right) => {
        const a = column.value(left);
        const b = column.value(right);
        const emptyA = a === "" || a === undefined || a === null;
        const emptyB = b === "" || b === undefined || b === null;
        if (emptyA || emptyB) return emptyA === emptyB ? 0 : emptyA ? 1 : -1;
        const result = window.ZPRSortableTable.compareValues(a, b);
        return result * sort.direction;
      };
      const sorted = items.map((item, index) => ({ item, index })).sort((left, right) => {
        const result = compare(left.item, right.item);
        if (result) return result;
        return String(columns[0].value(left.item)).localeCompare(String(columns[0].value(right.item)), undefined, { numeric: true, sensitivity: "base" }) || left.index - right.index;
      });
      const scroll = element("div", "trusted-source-table-scroll");
      const table = element("table", "trusted-source-table");
      table.dataset.sourceView = view;
      const head = element("thead");
      const header = element("tr");
      for (const candidate of columns) {
        const heading = element("th");
        heading.dataset.sortKey = candidate.key;
        if (view === "attributes" && ["people", "groups"].includes(candidate.key)) heading.dataset.numeric = "true";
        heading.textContent = candidate.label;
        header.append(heading);
      }
      head.append(header);
      const body = element("tbody");
      for (const { item } of sorted) {
        const row = element("tr");
        row.dataset.sourceRow = "";
        row.append(...columns.map((candidate) => candidate.render(item)));
        body.append(row, ...afterRow(row, item));
      }
      table.append(head, body);
      window.ZPRPollingDisplay?.markNumericColumns(table);
      window.ZPRSortableTable.bindSortableHeaders({
        table,
        getSort: () => this.sorts[view],
        onSort: (nextSort) => {
          this.sorts[view] = nextSort;
          this.render();
          this.querySelector(`[data-source-view="${CSS.escape(view)}"] th[data-sort-key="${CSS.escape(nextSort.key)}"] .sort-button`)?.focus();
        },
      });
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
