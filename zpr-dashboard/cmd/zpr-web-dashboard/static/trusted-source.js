(() => {
  if (customElements.get("trusted-source-browser")) return;

  const selection = window.ZPRTrustedSourceSelection || (() => {
    let context = { organizationID: "", sources: [], sourceID: "" };
    const announce = () => document.dispatchEvent(new CustomEvent("trusted-source:selection-changed", { detail: { ...context } }));
    return window.ZPRTrustedSourceSelection = {
      get: () => ({ ...context }),
      setContext(organizationID, sources, preferred = "") {
        const available = (Array.isArray(sources) ? sources : []).filter(source => source?.source_id && (!source.organization_id || source.organization_id === organizationID));
        const changedOrganization = context.organizationID !== organizationID;
        const currentAvailable = available.some(source => source.source_id === context.sourceID);
        const preferredAvailable = available.some(source => source.source_id === preferred);
        const sourceID = changedOrganization || !currentAvailable ? (preferredAvailable ? preferred : available[0]?.source_id || "") : context.sourceID;
        const changed = changedOrganization || sourceID !== context.sourceID || JSON.stringify(available) !== JSON.stringify(context.sources);
        context = { organizationID, sources: available, sourceID };
        if (changed) announce();
        return { ...context };
      },
      select(sourceID) {
        if (!context.sources.some(source => source.source_id === sourceID) || context.sourceID === sourceID) return;
        context = { ...context, sourceID };
        announce();
      },
    };
  })();

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
            if (location.hash === "#sources") {
              if (!this.snapshot) this.load();
              if (this.view === "updates") this.loadUpdates(true);
              this.contextPoller.start({ immediate: false });
            }
            if (location.hash !== "#sources") {
              const cancelledDirectory = this.directoryPending;
              this.directoryPoller.stop();
              this.updatesPoller.stop();
              this.contextPoller.stop();
              if (cancelledDirectory && this.snapshot) {
                const message = this.querySelector("[data-source-message]");
                message.textContent = "Refresh cancelled · Displayed records are last known.";
                message.hidden = false;
              }
            }
          };
          window.addEventListener("hashchange", this.hashListener);
        }
        if (location.hash === "#sources") {
          this.load();
          this.contextPoller.start({ immediate: false });
        }
      } else {
        this.load();
      }
    }

    disconnectedCallback() {
      this.directoryPoller?.stop();
      this.updatesPoller?.stop();
      this.contextPoller?.stop();
      document.removeEventListener("trusted-source:selection-changed", this.selectionListener);
      document.removeEventListener("control-room:refreshed", this.snapshotListener);
      if (this.hashListener) window.removeEventListener("hashchange", this.hashListener);
      this.hashListener = null;
    }

    initialize() {
      this.ready = true;
      this.simulatorPage = !this.closest("#page-sources");
      this.classList.toggle("trusted-source-simulator", this.simulatorPage);
      this.view = this.simulatorPage ? "graph" : "people";
      this.sorts = { people: { key: "identity", direction: 1 }, groups: { key: "group", direction: 1 }, attributes: { key: "attribute", direction: 1 } };
      this.expandedPeople = new Set();
      this.sourceHealth = [];
      this.sourceMetaText = "";
      this.innerHTML = `
        <section class="trusted-source-browser-panel" aria-label="Trusted source browser">
          <header class="trusted-source-browser-head">
            <div><h3 data-source-title${this.simulatorPage ? " hidden" : ""}>Trusted source</h3>${this.simulatorPage ? "" : `<select data-source-selector class="trusted-source-heading-select" aria-label="Trusted source" hidden></select>`}<p data-source-meta class="trusted-source-meta">Not loaded</p></div>
            <button type="button" class="button" data-source-refresh>Refresh</button>
          </header>
          <div class="trusted-source-summary" data-source-summary role="status"${this.simulatorPage ? " hidden" : ""}></div>
          ${this.simulatorPage ? "" : `<section class="trusted-source-registry" aria-label="Configured trusted sources">
            <h4>Configured sources</h4>
            <div class="trusted-source-registry-scroll" data-source-registry role="region" aria-label="Configured source status"></div>
          </section>`}
          <div class="trusted-source-browser-tools">
            <div class="trusted-source-tabs" role="tablist" aria-label="Trusted source records">
              ${this.simulatorPage ? `<button type="button" role="tab" data-source-tab="graph" aria-selected="true">LDAP graph</button>` : ""}
              <button type="button" role="tab" data-source-tab="tree" aria-selected="false" tabindex="-1">LDAP tree</button>
              <button type="button" role="tab" data-source-tab="people" aria-selected="${this.simulatorPage ? "false" : "true"}"${this.simulatorPage ? ' tabindex="-1"' : ""}>People${this.simulatorPage ? '<span class="trusted-source-tab-count" data-record-count="people" aria-hidden="true">—</span>' : ""}</button>
              <button type="button" role="tab" data-source-tab="groups" aria-selected="false" tabindex="-1">Groups${this.simulatorPage ? '<span class="trusted-source-tab-count" data-record-count="groups" aria-hidden="true">—</span>' : ""}</button>
              <button type="button" role="tab" data-source-tab="attributes" aria-selected="false" tabindex="-1">Attributes${this.simulatorPage ? '<span class="trusted-source-tab-count" data-record-count="attributes" aria-hidden="true">—</span>' : ""}</button>
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
      if (!this.simulatorPage) this.initializeRegistration();
      this.querySelector(".trusted-source-tabs").addEventListener("keydown", event => {
        if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
        const tabs = [...this.querySelectorAll("[data-source-tab]")];
        const current = tabs.indexOf(event.target.closest("[data-source-tab]"));
        if (current < 0) return;
        event.preventDefault();
        const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (current + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
        tabs[next].focus();
        tabs[next].click();
      });
      this.addEventListener("click", (event) => {
        const sourceButton = event.target.closest("[data-source-select]");
        if (sourceButton) selection.select(sourceButton.dataset.sourceSelect);
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
          else if (!this.snapshot) this.load();
        }
      });
      this.querySelector("[data-source-filter]").addEventListener("input", () => this.render());
      if (this.closest("#page-sources")) {
        this.querySelector("[data-source-refresh]").hidden = true;
        this.querySelector("[data-source-selector]").addEventListener("change", event => selection.select(event.target.value));
        this.selectionListener = event => this.onSelectionChanged(event.detail);
        document.addEventListener("trusted-source:selection-changed", this.selectionListener);
        this.snapshotListener = event => {
          if (event.detail?.organization_id && event.detail.organization_id !== selection.get().organizationID) {
            this.sourceHealth = [];
            return;
          }
          this.sourceHealth = event.detail?.trusted_sources || [];
          this.renderSourceHealth();
          this.renderSourceRegistry();
        };
        document.addEventListener("control-room:refreshed", this.snapshotListener);
        document.addEventListener("control-room:refresh-requested", () => {
          if (!this.isConnected || location.hash !== "#sources") return;
          this.load();
          if (this.view === "updates") this.loadUpdates(true);
        });
      }
      this.directoryPoller = window.ZPRPageRuntime.createPoller({
        run: context => this.collectDirectory(context),
        onPending: value => {
          this.directoryPending = value;
          this.querySelector("[data-source-refresh]").disabled = value;
        },
        onError: error => {
          this.directoryError = `${error.message || "Trusted source could not be read"}${this.snapshot ? " · Displayed records are last known." : ""}`;
          this.renderSourceHealth();
          this.renderSourceRegistry();
          if (this.view === "updates") return;
          const message = this.querySelector("[data-source-message]");
          message.textContent = this.directoryError;
          message.hidden = false;
        },
      });
      this.updatesPoller = window.ZPRPageRuntime.createPoller({
        run: context => this.collectUpdates(context),
        onPending: value => {
          this.updatesPending = value;
          this.querySelector("[data-source-load-more]").disabled = value;
          this.renderSourceHeading();
        },
        onError: error => {
          this.updateError = error.status === 410 ? "This update history expired. Refresh to load the current 24-hour window."
            : error.message || "Trusted source updates could not be read";
          if (this.view === "updates") {
            const message = this.querySelector("[data-source-message]");
            this.querySelector("[data-source-results]").hidden = true;
            message.textContent = this.updateError;
            message.hidden = false;
            this.querySelector("[data-source-load-more]").hidden = true;
          }
        },
      });
      this.contextPoller = window.ZPRPageRuntime.createPoller({
        interval: 10000,
        run: async ({ signal, isCurrent }) => {
          const status = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, "/api/assertions", { signal, headers: { Accept: "application/json" } });
          if (!isCurrent()) return;
          const before = selection.get();
          this.settingSelectionContext = true;
          const current = selection.setContext(status.organization_id || "", status.sources, status.default_source);
          this.settingSelectionContext = false;
          this.sourceContextError = "";
          if (before.organizationID !== current.organizationID) this.sourceHealth = [];
          this.sourceStatus = status;
          this.sourceContextOrganizationID = current.organizationID;
          this.renderSourceHeading();
          this.renderSourceRegistry();
          if (before.organizationID !== current.organizationID || before.sourceID !== current.sourceID) this.onSelectionChanged(current);
        },
        onError: error => {
          this.sourceContextError = error.message || "Organization source context could not be confirmed";
          if (this.snapshot) {
            const message = this.querySelector("[data-source-message]");
            message.textContent = `${this.sourceContextError} · Displayed records are last known.`;
            message.hidden = false;
            this.renderSourceHealth();
          }
        },
      });
    }

    initializeRegistration() {
      const section = element("section", "trusted-source-registration");
      section.setAttribute("aria-label", "Trusted source registration");
      section.innerHTML = `
        <div class="trusted-source-registration-actions">
          <button type="button" class="button" data-register-source>Register source</button>
          <button type="button" class="button" data-edit-source>Edit selected source</button>
        </div>
        <form data-registration-form hidden>
          <h4 data-registration-heading>Register trusted source</h4>
          <p>Organization and source IDs are stable. Credentials must be protected server file references, never passwords or tokens.</p>
          <div class="trusted-source-registration-fields">
            <label>Organization ID<input name="organization_id" readonly required></label>
            <label>Source ID<input name="source_id" required pattern="[A-Za-z0-9_-]{1,64}" maxlength="64"></label>
            <label>Display name<input name="display_name" maxlength="200"></label>
            <label><input type="checkbox" name="policy_default">Default source for unqualified policy attributes and groups in this organization</label>
            <label>Source kind<select name="kind" aria-label="Source kind"><option value="ldap">LDAP (verified TLS)</option><option value="https-json">HTTPS JSON (verified TLS)</option></select></label>
            <label>Source URL<input name="url" required placeholder="ldaps://directory.company.example:636"></label>
            <label>CA file reference (optional)<input name="ca_file" autocomplete="off"></label>
            <label>Approved attributes (comma separated)<input name="attributes" required placeholder="cn,mail,departmentNumber"></label>
            <label data-ldap-field>Base DN<input name="base_dn"></label>
            <label data-ldap-field>Read-only bind DN<input name="bind_dn"></label>
            <label data-ldap-field>Password file reference<input name="bind_password_file" autocomplete="off"></label>
            <label data-https-field hidden>Token file reference<input name="token_file" autocomplete="off"></label>
          </div>
          <div class="trusted-source-registration-actions">
            <button type="button" class="button" data-test-source>Test connection (read-only)</button>
            <button type="submit" class="button" data-save-source>Save configuration</button>
            <button type="button" class="button" data-close-registration>Close</button>
          </div>
        </form>
        <p data-registration-storage></p>
        <p data-registration-message role="status" aria-live="polite"></p>
        <ul data-connection-results aria-label="Source connection test results"></ul>`;
      this.querySelector(".trusted-source-registry").append(section);
      this.registrationForm = section.querySelector("form");
      this.connectionResults = new Map();
      this.registrationGeneration = 0;
      section.querySelector("[data-register-source]").addEventListener("click", () => this.openRegistration(false));
      section.querySelector("[data-edit-source]").addEventListener("click", () => this.openRegistration(true));
      section.querySelector("[data-close-registration]").addEventListener("click", () => {
        this.registrationGeneration++;
        this.registrationForm.hidden = true;
      });
      this.registrationForm.addEventListener("input", () => {
        this.registrationGeneration++;
        const fields = this.registrationForm.elements;
        if (this.connectionResults.has(`${fields.organization_id.value}/${fields.source_id.value}`)) this.recordConnectionResult(fields.organization_id.value, fields.source_id.value, "Configuration changed; connection is not verified for these values");
        section.querySelector("[data-registration-message]").textContent = "Configuration changed; connection has not been tested for these values.";
      });
      this.registrationForm.elements.kind.addEventListener("change", () => this.renderRegistrationKind());
      section.querySelector("[data-test-source]").addEventListener("click", () => this.submitRegistration(true));
      this.registrationForm.addEventListener("submit", event => { event.preventDefault(); this.submitRegistration(false); });
    }

    renderRegistrationKind() {
      const ldap = this.registrationForm.elements.kind.value === "ldap";
      for (const field of this.registrationForm.querySelectorAll("[data-ldap-field]")) { field.hidden = !ldap; field.querySelector("input").required = ldap; }
      for (const field of this.registrationForm.querySelectorAll("[data-https-field]")) { field.hidden = ldap; field.querySelector("input").required = !ldap; }
    }

    async openRegistration(editing) {
      const generation = ++this.registrationGeneration;
      const message = this.querySelector("[data-registration-message]");
      this.registrationForm.hidden = true;
      message.textContent = "Loading production source configuration…";
      try {
        const registry = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, "/api/assertions/sources", { headers: { Accept: "application/json" } });
        if (generation !== this.registrationGeneration) return;
        const current = selection.get();
        if (!registry.organization_id || registry.organization_id !== current.organizationID) throw new Error("Organization changed; refresh before configuring a source");
        const selected = editing && registry.sources.find(source => source.source_id === current.sourceID);
        if (editing && !selected?.editable) throw new Error("Selected source is not editable here; use its operator-managed configuration file");
        this.registrationRegistry = registry;
        this.registrationEditing = editing;
        this.registrationForm.reset();
        const fields = this.registrationForm.elements;
        fields.organization_id.value = registry.organization_id;
        fields.source_id.readOnly = editing;
        const config = selected?.config || {};
        const scope = config.organizations?.[registry.organization_id] || {};
        for (const name of ["display_name", "kind", "url", "ca_file", "token_file"]) fields[name].value = config[name] || (name === "kind" ? "ldap" : "");
        fields.source_id.value = config.name || "";
        fields.policy_default.checked = registry.policy_default_source === config.name;
        fields.attributes.value = (config.attributes || []).join(",");
        for (const name of ["base_dn", "bind_dn", "bind_password_file"]) fields[name].value = scope[name] || "";
        if (config.kind === "https-json") fields.url.value = scope.url || "";
        this.querySelector("[data-registration-heading]").textContent = editing ? "Edit trusted source (identity is fixed)" : "Register trusted source";
        this.querySelector("[data-save-source]").disabled = !registry.durable;
        this.renderRegistrationKind();
        this.registrationForm.hidden = false;
        this.querySelector("[data-registration-storage]").textContent = registry.durable ? "Changes are saved to the production source configuration file. Saving does not verify connectivity." : "Registration is unavailable: configure a protected ZPR_ASSERTION_SOURCES_FILE. No session-only registrations are saved.";
        message.textContent = "Configuration loaded; use Test connection for read-only verification.";
        fields.source_id.focus();
      } catch (error) { if (generation === this.registrationGeneration) message.textContent = error.message || "Source configuration could not be loaded"; }
    }

    async submitRegistration(testing) {
      if (!this.registrationForm.reportValidity()) return;
      const fields = this.registrationForm.elements;
      const organizationID = fields.organization_id.value;
      const sourceID = fields.source_id.value;
      const scope = fields.kind.value === "ldap" ? { base_dn: fields.base_dn.value, bind_dn: fields.bind_dn.value, bind_password_file: fields.bind_password_file.value } : { url: fields.url.value };
      const config = { name: sourceID, display_name: fields.display_name.value, kind: fields.kind.value, ca_file: fields.ca_file.value,
        attributes: fields.attributes.value.split(",").map(value => value.trim()).filter(Boolean), organizations: { [organizationID]: scope } };
      if (config.kind === "ldap") config.url = fields.url.value;
      else config.token_file = fields.token_file.value;
      const generation = ++this.registrationGeneration;
      const message = this.querySelector("[data-registration-message]");
      const buttons = [...this.registrationForm.querySelectorAll("[data-test-source], [data-save-source]")];
      buttons.forEach(button => { button.disabled = true; });
      message.textContent = `${sourceID}: ${testing ? "Testing connection (read-only)…" : "Saving configuration…"}`;
      try {
        const result = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, `/api/assertions/sources${testing ? "/test" : ""}`, {
          method: testing || !this.registrationEditing ? "POST" : "PUT", headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: JSON.stringify({ organization_id: organizationID, source_id: sourceID, revision: this.registrationRegistry.revision, policy_default: fields.policy_default.checked, config }),
        });
        if (generation !== this.registrationGeneration || selection.get().organizationID !== organizationID) return;
        if (result.organization_id !== organizationID || result.source_id !== sourceID || result.status !== (testing ? "success" : "saved")) throw new Error("Source operation response did not match the requested identity or result");
        message.textContent = `${sourceID}: ${result.message}`;
        if (testing) this.recordConnectionResult(organizationID, sourceID, "Connection test succeeded (read-only)");
        else {
          this.registrationRegistry.revision = result.revision;
          this.registrationRegistry.policy_default_source = result.policy_default_source || "";
          this.registrationEditing = true;
          fields.source_id.readOnly = true;
          this.recordConnectionResult(organizationID, sourceID, "Configuration saved; test the saved configuration to verify connectivity");
          this.load();
        }
      } catch (error) {
        if (generation !== this.registrationGeneration || selection.get().organizationID !== organizationID) return;
        message.textContent = `${sourceID}: ${error.message || "Source operation failed"}`;
        if (testing) this.recordConnectionResult(organizationID, sourceID, "Connection test failed; source is not verified");
      } finally {
        buttons.forEach(button => { button.disabled = button.hasAttribute("data-save-source") && !this.registrationRegistry?.durable; });
      }
    }

    recordConnectionResult(organizationID, sourceID, text) {
      this.connectionResults.set(`${organizationID}/${sourceID}`, text);
      const list = this.querySelector("[data-connection-results]");
      list.replaceChildren();
      for (const [identity, result] of this.connectionResults) {
        if (identity.startsWith(`${selection.get().organizationID}/`)) list.append(element("li", "", `${identity}: ${result}`));
      }
    }

    load() {
      this.directoryPoller.start({ immediate: false });
      return this.directoryPoller.refresh();
    }

    async collectDirectory({ signal, isCurrent }) {
      const message = this.querySelector("[data-source-message]");
      message.hidden = false;
      message.textContent = "Loading configured trusted sources…";
      const fetcher = this.simulatorPage ? window.fetch.bind(window) : window.zprOperatorFetch;
      let data;
      if (this.simulatorPage) {
        data = await window.ZPRPageRuntime.requestJSON(fetcher, this.dataset.endpoint, { signal, headers: { Accept: "application/json" } });
      } else {
        const status = await window.ZPRPageRuntime.requestJSON(fetcher, "/api/assertions", { signal, headers: { Accept: "application/json" } });
        if (!isCurrent()) return;
        if (!status.organization_id) throw new Error("No organization is selected for trusted sources");
        const previous = selection.get();
        this.settingSelectionContext = true;
        const current = selection.setContext(status.organization_id, status.sources, status.default_source);
        this.settingSelectionContext = false;
        if (previous.organizationID !== current.organizationID || previous.sourceID !== current.sourceID) this.onSelectionChanged(current, { keepDirectoryRequest: true });
        if (previous.organizationID !== current.organizationID) this.sourceHealth = [];
        this.sourceStatus = status;
        this.sourceContextOrganizationID = current.organizationID;
        this.renderSourceRegistry();
        this.renderSourceHeading();
        if (!current.sourceID) {
          this.updateSources = null;
          this.renderSourceHeading();
          this.clearSourceData("No trusted sources are configured for this organization.");
          return;
        }
        const requestedSourceID = current.sourceID;
        const query = new URLSearchParams({ source: requestedSourceID });
        data = await window.ZPRPageRuntime.requestJSON(fetcher, `${this.dataset.endpoint}?${query}`, { signal, headers: { Accept: "application/json" } });
        const latest = selection.get();
        if (!isCurrent() || latest.organizationID !== status.organization_id || latest.sourceID !== requestedSourceID) return;
        if (data.organization_id !== status.organization_id || data.source_id !== requestedSourceID) throw new Error("Trusted source response did not match the selected source and organization");
        data.source_metadata = current.sources.find(source => source.source_id === requestedSourceID);
        this.sourceStatus = status;
      }
      if (!data.directory) throw new Error("Trusted source did not return browseable records");
      if (!isCurrent()) return;
      this.snapshot = data;
      this.directoryError = "";
      this.renderSourceHeading();
      this.sourceMetaText = [data.organization_id, data.source_id || data.source_name, data.source_metadata?.kind || data.kind, data.base_dn, data.observed_at && window.ZPRSafeDisplay.formatDateTime(data.observed_at)].filter(Boolean).join(" · ");
      this.querySelector("[data-source-meta]").textContent = this.sourceMetaText;
      this.querySelector("[data-source-summary]").textContent = `${data.people || 0} people · ${(data.groups || []).length} groups · ${(data.attributes || []).length} attributes`;
      this.renderSourceHealth();
      this.renderSourceRegistry();
      if (this.view !== "updates") this.render();
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
      if (this.simulatorPage) {
        const counts = {
          people: (directory.people || []).length,
          groups: Object.keys(directory.groups || {}).length,
          attributes: (this.snapshot.attributes || []).length,
        };
        for (const [view, value] of Object.entries(counts)) {
          this.querySelector(`[data-record-count="${view}"]`).textContent = String(value);
          this.querySelector(`[data-source-tab="${view}"]`).setAttribute("aria-label", `${view[0].toUpperCase()}${view.slice(1)} (${value} records)`);
        }
      }
      if (this.view === "graph") this.renderGraph(results, directory);
      else if (this.view === "tree") this.renderTree(results, directory, query);
      else if (this.view === "people") this.renderPeople(results, directory, query);
      else if (this.view === "groups") this.renderGroups(results, directory, query);
      else this.renderAttributes(results, directory, query);
      const count = this.view === "graph" ? results.querySelectorAll(".ldap-graph-node").length : this.view === "tree" ? results.querySelectorAll("[data-ldap-entry]").length : results.querySelectorAll("tbody tr[data-source-row]").length;
      this.querySelector("[data-source-count]").textContent = `${count} ${this.view}`;
      message.textContent = this.directoryError || (count ? "" : `No ${this.view} match this filter.`);
      message.hidden = !this.directoryError && count > 0;
      results.hidden = false;
    }

    loadUpdates(reset = true) {
      if (!this.closest("#page-sources")) return Promise.resolve();
      if (!this.updatesPending) this.resetUpdates = reset;
      this.updatesPoller.start({ immediate: false });
      return this.updatesPoller.refresh();
    }

    async collectUpdates({ signal, isCurrent }) {
      const reset = this.resetUpdates;
      const controls = this.querySelector("[data-source-update-controls]");
      const message = this.querySelector("[data-source-message]");
      controls.hidden = false;
      message.hidden = false;
      message.textContent = "Loading trusted source updates…";
      if (!this.updateSources) {
        const data = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, "/api/trusted-sources/change-feeds", { signal, headers: { Accept: "application/json" } });
        if (!Array.isArray(data.sources)) throw new Error("Change feed list is malformed");
        if (!isCurrent()) return;
        this.updateSources = data.sources;
      }
      const selectedContext = selection.get();
      const sourceName = selectedContext.sourceID;
      this.renderSourceHeading();
      if (!sourceName) {
        this.updates = [];
        this.updateCursor = "";
        this.updateMore = false;
        this.updateEmptyMessage = "No trusted source is selected.";
        this.renderUpdates();
        return;
      }
      if (!this.updateSources.some(source => (source.source_id || source.name) === sourceName)) {
        this.updates = [];
        this.updateCursor = "";
        this.updateMore = false;
        this.updateSourceName = sourceName;
        this.updateEmptyMessage = "Updates are unsupported for this read-only source; no change feed is configured.";
        this.renderUpdates();
        return;
      }
      if (reset || sourceName !== this.updateSourceName) {
        this.updates = [];
        this.updateCursor = "";
        this.updateMore = true;
        this.updateSourceName = sourceName;
      }
      const query = new URLSearchParams({ limit: "100" });
      if (this.updateCursor) query.set("cursor", this.updateCursor);
      const data = await window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch, `/api/trusted-sources/change-feeds/${encodeURIComponent(sourceName)}/changes?${query}`, { signal, headers: { Accept: "application/json" } });
      if (!Array.isArray(data.changes) || typeof data.cursor !== "string" || typeof data.more !== "boolean") {
        throw new Error("Trusted source update response is malformed");
      }
      const latest = selection.get();
      if (!isCurrent() || latest.organizationID !== selectedContext.organizationID || latest.sourceID !== sourceName) return;
      this.updates.push(...data.changes);
      this.updateCursor = data.cursor;
      this.updateMore = data.more;
      this.updateError = "";
      this.updateEmptyMessage = "No trusted-source updates in the last 24 hours.";
      if (this.view === "updates") this.renderUpdates();
    }

    renderSourceHeading() {
      const title = this.querySelector("[data-source-title]");
      const selector = this.querySelector("[data-source-selector]");
      const updates = !this.simulatorPage && this.view === "updates";
      this.querySelector("[data-source-meta]").hidden = updates;
      this.querySelector("[data-source-summary]").hidden = this.simulatorPage || updates;
      const current = selection.get();
      const metadata = current.sources.find(source => source.source_id === current.sourceID);
      const selected = updates && this.updateSources?.find(source => (source.source_id || source.name) === current.sourceID);
      title.textContent = selected?.display_name || metadata?.display_name || `${this.simulatorPage ? this.dataset.titlePrefix || "" : ""}${this.directorySourceName() || (this.simulatorPage ? "Trusted source" : "Source unavailable")}`;
      if (selector && !this.simulatorPage) {
        selector.replaceChildren(...current.sources.map(source => {
          const option = element("option", "", `${source.display_name || source.source_id} · ${source.source_id}`);
          option.value = source.source_id;
          return option;
        }));
        selector.value = current.sourceID;
      }
      title.hidden = this.simulatorPage || current.sources.length > 1;
      if (selector) selector.hidden = this.simulatorPage || current.sources.length < 2;
    }

    directorySourceName() {
      return (!this.simulatorPage && this.snapshot?.default_source) || this.snapshot?.source_name;
    }

    clearSourceData(messageText) {
      this.snapshot = null;
      this.expandedPeople.clear();
      this.querySelector("[data-source-filter]").value = "";
      this.querySelector("[data-source-results]").replaceChildren();
      this.querySelector("[data-source-results]").hidden = true;
      this.querySelector("[data-source-summary]").textContent = "";
      this.sourceMetaText = selection.get().organizationID || "Organization unavailable";
      this.querySelector("[data-source-meta]").textContent = this.sourceMetaText;
      const message = this.querySelector("[data-source-message]");
      message.textContent = messageText;
      message.hidden = false;
    }

    onSelectionChanged(context, { keepDirectoryRequest = false } = {}) {
      if (this.simulatorPage || this.settingSelectionContext) return;
      if (this.registrationForm && this.registrationForm.elements.organization_id.value !== context.organizationID) {
        this.registrationGeneration++;
        this.registrationForm.hidden = true;
        this.querySelector("[data-registration-message]").textContent = "";
        this.querySelector("[data-registration-storage]").textContent = "";
        this.querySelector("[data-connection-results]").replaceChildren();
      }
      if (!keepDirectoryRequest) this.directoryPoller.stop();
      this.updatesPoller.stop();
      this.updateSources = null;
      this.updates = [];
      this.updateCursor = "";
      this.updateMore = false;
      this.updateSourceName = "";
      this.resetUpdates = true;
      this.expandedPeople.clear();
      this.querySelector("[data-source-filter]").value = "";
      this.querySelector("[data-source-count]").textContent = "";
      this.querySelector("[data-source-results]").replaceChildren();
      this.querySelector("[data-source-results]").hidden = true;
      this.querySelector("[data-source-summary]").textContent = "";
      this.directoryError = "";
      this.sourceContextError = "";
      this.updateError = "";
      if (this.sourceContextOrganizationID && this.sourceContextOrganizationID !== context.organizationID) this.sourceHealth = [];
      this.sourceContextOrganizationID = context.organizationID;
      this.renderSourceHeading();
      this.renderSourceRegistry();
      this.clearSourceData(context.sourceID ? `Loading ${context.sourceID} for ${context.organizationID}…` : `No trusted sources are configured for ${context.organizationID || "this organization"}.`);
      if (keepDirectoryRequest || !context.sourceID) return;
      if (location.hash === "#sources") {
        this.load();
        if (this.view === "updates") this.loadUpdates(true);
      }
    }

    renderSourceRegistry() {
      if (this.simulatorPage) return;
      const target = this.querySelector("[data-source-registry]");
      if (!target) return;
      const context = selection.get();
      const sources = this.sourceStatus?.organization_id === context.organizationID ? this.sourceStatus.sources || [] : context.sources;
      if (!sources.length) {
        target.replaceChildren(element("p", "trusted-source-message", `No trusted sources are configured for ${context.organizationID || "this organization"}.`));
        return;
      }
      const table = element("table", "trusted-source-table trusted-source-registry-table");
      const head = element("thead");
      const header = element("tr");
      for (const label of ["SOURCE", "PROVIDER", "ORGANIZATION", "HEALTH", "LAST SUCCESS"]) header.append(element("th", "", label));
      head.append(header);
      table.append(head);
      const body = element("tbody");
      for (const source of sources) {
        const sourceID = source.source_id || source.name;
        const health = this.sourceHealth.find(item => item.name === sourceID);
        const unavailable = Boolean((this.directoryError || this.sourceContextError) && sourceID === context.sourceID);
        const healthText = unavailable ? "unavailable" : health?.health || "Not reported";
        const healthClass = unavailable ? " is-failed" : health?.health === "working" ? " is-working" : health?.health === "failed" ? " is-failed" : "";
        const row = element("tr");
        const sourceCell = element("th", "trusted-source-source-identity");
        const button = element("button", "trusted-source-source-select", `${source.display_name || sourceID} · ${sourceID}`);
        button.type = "button";
        button.dataset.sourceSelect = sourceID;
        button.setAttribute("aria-pressed", String(sourceID === context.sourceID));
        sourceCell.append(button);
        row.append(sourceCell, element("td", "", source.kind || "Not reported"), element("td", "", source.organization_id || context.organizationID || "Not reported"), element("td", `trusted-source-health${healthClass}`, healthText), element("td", "", health?.last_success_ms ? window.ZPRSafeDisplay.formatDateTime(health.last_success_ms) : "Not reported"));
        body.append(row);
      }
      table.append(body);
      const scroll = element("div", "trusted-source-table-scroll");
      scroll.append(table);
      target.replaceChildren(scroll);
    }

    renderSourceHealth() {
      if (this.simulatorPage || !this.snapshot) return;
      const item = this.sourceHealth.find(source => source.name === this.snapshot.source_id);
      const health = this.directoryError || this.sourceContextError
        ? `Health: unavailable · Last success: ${item?.last_success_ms ? window.ZPRSafeDisplay.formatDateTime(item.last_success_ms) : "not reported"}`
        : item
          ? `Health: ${item.health || "not reported"} · Last success: ${item.last_success_ms ? window.ZPRSafeDisplay.formatDateTime(item.last_success_ms) : "not reported"}`
          : "Health: not reported by the production inventory";
      this.querySelector("[data-source-meta]").textContent = `${this.sourceMetaText} · ${health}`;
      this.renderSourceRegistry();
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
      if (!changes.length) {
        results.replaceChildren();
        results.hidden = true;
        count.textContent = "";
        message.textContent = this.updateMore ? "No updates on this page; more history is available." : this.updateEmptyMessage || "No trusted-source updates in the last 24 hours.";
        message.hidden = false;
        moreButton.hidden = !this.updateMore;
        moreButton.disabled = Boolean(this.updatesPending);
        return;
      }
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
