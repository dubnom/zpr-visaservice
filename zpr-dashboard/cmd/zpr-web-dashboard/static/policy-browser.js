class ZPRPolicyBrowser extends HTMLElement {
  constructor() {
    super();
    this.catalog = null;
    this.record = null;
    this.revisions = [];
    this.catalogController = null;
    this.detailController = null;
  }

  connectedCallback() {
    this.record = null;
    this.revisions = [];
    this.shownRevision = null;
    this.innerHTML = `
      <div class="pb-toolbar"><strong class="pb-organization">Loading repository</strong><button class="pb-refresh" type="button">Refresh</button></div>
      <p class="pb-error" role="status" aria-live="polite"></p>
      <div class="pb-layout">
        <aside class="pb-catalog" aria-label="Policy and assertion records">
          <div class="pb-filters">
            <label class="pb-search-label">Search<input class="pb-search" type="search" aria-label="Search" placeholder="Name or category"></label>
            <label>Type<select class="pb-kind" aria-label="Type"><option value="">All types</option><option value="policy">Policies</option><option value="assertions">Assertions</option></select></label>
            <label>Category<select class="pb-category" aria-label="Category"><option value="">All categories</option></select></label>
          </div>
          <span class="pb-count" role="status"></span>
          <div class="pb-records"></div>
        </aside>
        <section class="pb-detail" aria-label="Record source" aria-busy="false">
          <div class="pb-detail-heading"><h2 class="pb-title">Select a record</h2><label class="pb-revision-control" hidden>Revision<select class="pb-revision" aria-label="Revision"></select></label></div>
          <p class="pb-meta"></p><p class="pb-schedule" hidden></p>
          <label class="pb-wrap"><input type="checkbox">Wrap</label>
          <pre class="pb-source" data-wrap="false" tabindex="0" aria-label="Read-only record source"><code></code></pre>
        </section>
      </div>`;
    this.querySelector(".pb-refresh").addEventListener("click", () => this.loadCatalog());
    for (const control of this.querySelectorAll(".pb-search, .pb-kind, .pb-category")) {
      control.addEventListener("input", () => this.renderRecords());
    }
    this.querySelector(".pb-revision").addEventListener("change", (event) => this.loadRevision(Number(event.target.value)));
    this.querySelector(".pb-wrap input").addEventListener("change", (event) => {
      this.querySelector(".pb-source").dataset.wrap = String(event.target.checked);
    });
    this.visibilityListener = () => { if (!document.hidden) void this.loadCatalog(); };
    document.addEventListener("visibilitychange", this.visibilityListener);
    void this.loadCatalog();
    this.timer = setInterval(() => { if (!document.hidden) void this.loadCatalog(); }, 10000);
  }

  disconnectedCallback() {
    clearInterval(this.timer);
    this.catalogController?.abort();
    this.detailController?.abort();
    document.removeEventListener("visibilitychange", this.visibilityListener);
  }

  get apiBase() {
    return (this.getAttribute("api-base") || "/api/policy").replace(/\/$/, "");
  }

  text(selector, value) {
    this.querySelector(selector).textContent = value || "";
  }

  async request(path, signal) {
    const response = await fetch(`${this.apiBase}${path}`, { method: "GET", signal, cache: "no-store", headers: { Accept: "application/json" } });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `Repository returned HTTP ${response.status}`);
    return data;
  }

  async loadCatalog() {
    if (this.catalogController) return;
    const controller = new AbortController();
    this.catalogController = controller;
    this.querySelector(".pb-refresh").disabled = true;
    try {
      const data = await this.request("", controller.signal);
      if (!this.isConnected || controller.signal.aborted) return;
      const changedOrganization = this.catalog && this.catalog.organization_id !== data.organization_id;
      this.catalog = data;
      if (changedOrganization) {
        this.clearRecord();
        this.querySelector(".pb-category").value = "";
      }
      this.text(".pb-organization", data.organization_name || data.organization_id || "Policy Repository");
      const categorySelect = this.querySelector(".pb-category");
      const selectedCategory = categorySelect.value;
      categorySelect.replaceChildren(new Option("All categories", ""));
      for (const category of data.categories || []) categorySelect.add(new Option(category.path || category.name, category.id));
      categorySelect.value = (data.categories || []).some((category) => category.id === selectedCategory) ? selectedCategory : "";
      if (this.record && !(data.records || []).some((record) => record.id === this.record.id)) this.clearRecord();
      this.renderRecords();
      this.text(".pb-error", "");
      const summary = (data.records || []).find((record) => record.id === this.record?.id);
      if (summary && summary.current_revision !== this.record.current_revision) {
        const historical = Number(this.querySelector(".pb-revision").value) !== this.record.current_revision;
        await this.selectRecord(summary.id, historical ? Number(this.querySelector(".pb-revision").value) : null);
      }
    } catch (error) {
      if (error.name !== "AbortError") this.text(".pb-error", error.message);
    } finally {
      if (this.catalogController === controller) {
        this.catalogController = null;
        this.querySelector(".pb-refresh").disabled = false;
      }
    }
  }

  renderRecords() {
    const query = this.querySelector(".pb-search").value.trim().toLowerCase();
    const kind = this.querySelector(".pb-kind").value;
    const categoryID = this.querySelector(".pb-category").value;
    const categories = new Map((this.catalog?.categories || []).map((category) => [category.id, category.path || category.name]));
    const records = (this.catalog?.records || []).filter((record) => ["policy", "assertions"].includes(record.kind)
      && (!kind || record.kind === kind) && (!categoryID || record.category_id === categoryID)
      && `${record.name} ${categories.get(record.category_id) || ""}`.toLowerCase().includes(query));
    const list = this.querySelector(".pb-records");
    list.replaceChildren();
    this.text(".pb-count", `${records.length} records`);
    for (const record of records) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "pb-record";
      button.dataset.recordId = record.id;
      button.dataset.kind = record.kind;
      button.setAttribute("aria-current", String(this.record?.id === record.id));
      const icon = window.zprPolicyRecordIcon(record.kind);
      const name = document.createElement("strong");
      name.textContent = record.name;
      if (icon) button.append(icon);
      button.append(name);
      button.addEventListener("click", () => this.selectRecord(record.id));
      list.append(button);
    }
    if (!records.length) {
      const empty = document.createElement("p");
      empty.className = "pb-empty";
      empty.textContent = this.catalog ? "No matching records" : "Loading records";
      list.append(empty);
    }
  }

  clearRecord() {
    this.detailController?.abort();
    this.record = null;
    this.revisions = [];
    this.shownRevision = null;
    this.text(".pb-title", "Select a record");
    this.text(".pb-meta", "");
    this.text(".pb-source code", "");
    this.querySelector(".pb-schedule").hidden = true;
    this.querySelector(".pb-revision-control").hidden = true;
    this.querySelector(".pb-detail").setAttribute("aria-busy", "false");
  }

  async selectRecord(id, revisionNumber = null) {
    this.detailController?.abort();
    const controller = new AbortController();
    this.detailController = controller;
    this.querySelector(".pb-detail").setAttribute("aria-busy", "true");
    try {
      const path = `/records/${encodeURIComponent(id)}`;
      const [record, revisions] = await Promise.all([
        this.request(path, controller.signal), this.request(`${path}/revisions`, controller.signal),
      ]);
      let revision = null;
      if (revisionNumber && revisionNumber !== record.current_revision) revision = await this.request(`${path}/revisions/${revisionNumber}`, controller.signal);
      if (!this.isConnected || controller.signal.aborted) return;
      this.record = record;
      this.revisions = revisions || [];
      const select = this.querySelector(".pb-revision");
      select.replaceChildren();
      const numbers = [...new Set([record.current_revision, ...this.revisions.map((item) => item.number)])].sort((first, second) => second - first);
      for (const number of numbers) select.add(new Option(`r${number}${number === record.current_revision ? " / Latest" : ""}`, String(number)));
      select.value = String(revision?.number || record.current_revision);
      this.querySelector(".pb-revision-control").hidden = false;
      this.text(".pb-error", "");
      this.showSource(revision || record, revision?.number || record.current_revision);
      this.renderRecords();
    } catch (error) {
      if (error.name !== "AbortError") {
        this.text(".pb-error", error.message);
        if (this.shownRevision) this.querySelector(".pb-revision").value = String(this.shownRevision);
      }
    } finally {
      if (this.detailController === controller) this.querySelector(".pb-detail").setAttribute("aria-busy", "false");
    }
  }

  async loadRevision(number) {
    if (!this.record) return;
    await this.selectRecord(this.record.id, number);
  }

  showSource(version, number) {
    this.shownRevision = number;
    this.text(".pb-title", this.record.name);
    const revision = this.revisions.find((item) => item.number === number);
    const date = revision?.created_at || (version.number ? version.created_at : this.record.updated_at || version.created_at);
    const metadata = [this.record.kind === "policy" ? "Policy" : "Assertions", `r${number}`, revision?.author, revision?.summary, date ? window.ZPRSafeDisplay.formatDateTime(date) : ""];
    this.text(".pb-meta", metadata.filter(Boolean).join(" / "));
    const schedule = this.querySelector(".pb-schedule");
    schedule.hidden = true;
    let source = version.content || "";
    if (this.record.kind === "assertions") {
      try {
        const settings = JSON.parse(source);
        if (typeof settings.source !== "string") throw new Error("Invalid assertion record");
        source = settings.source;
        schedule.textContent = `Periodic checks: ${settings.enabled ? "Enabled" : "Disabled"} / Interval: ${settings.interval_seconds} seconds`;
        schedule.hidden = false;
      } catch {
        this.text(".pb-error", "Invalid assertion record; showing stored content.");
      }
    }
    const code = this.querySelector(".pb-source code");
    code.replaceChildren();
    const tokens = /#[^\n]*|\/\/[^\n]*|"(?:\\.|[^"\\])*"?|'(?:\\.|[^'\\])*'?|\b(?:define|provide|allow|never|as|with|at|over|on|and|signal|to|group|each|members|people|in|exactly_one|not_both|attribute|present|absent|contains)\b/gi;
    let offset = 0;
    for (const match of source.matchAll(tokens)) {
      code.append(document.createTextNode(source.slice(offset, match.index)));
      const span = document.createElement("span");
      span.className = /^#|^\/\//.test(match[0]) ? "pb-comment" : /^["']/.test(match[0]) ? "pb-string" : "pb-keyword";
      span.textContent = match[0];
      code.append(span);
      offset = match.index + match[0].length;
    }
    code.append(document.createTextNode(source.slice(offset)));
    if (!source) code.textContent = "No source in this revision";
    this.querySelector(".pb-source").scrollTop = 0;
  }
}

customElements.define("zpr-policy-browser", ZPRPolicyBrowser);