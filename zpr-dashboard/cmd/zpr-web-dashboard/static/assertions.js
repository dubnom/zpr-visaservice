(() => {
  const element = (id) => document.getElementById(id);
  const source = element("assertion-source");
  const enabled = element("assertion-enabled");
  const interval = element("assertion-interval");
  let status;
  let loadedRevision = 0;
  let loadedOrganizationID = "";
  let savedSource = "";
  let savedEnabled = false;
  let savedInterval = 60;
  let pending = false;
  let timer;
  const active = () => location.hash === "#policy" && !element("policy-assertion-editor")?.hidden;
  const dirty = () => source.value !== savedSource || enabled.checked !== savedEnabled || Number(interval.value) !== savedInterval;
  const stale = () => status && (status.settings.revision !== loadedRevision || (status.organization_id && status.organization_id !== loadedOrganizationID));
  const escape = (text) => String(text ?? "").replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character]);

  function highlight() {
    const keywords = new Set(["group", "each", "members", "people", "in", "exactly_one", "not_both", "attribute", "present", "absent", "contains"]);
    element("assertion-highlight").innerHTML = source.value.replace(/\/\/[^\n]*|"(?:\\.|[^"\\])*"?|[a-z_]+|\d+|[^a-z_\d]/g, (token) => {
      const kind = token.startsWith("//") ? "comment" : token.startsWith('"') ? "string" : keywords.has(token) ? "keyword" : /^\d+$/.test(token) ? "number" : "";
      return kind ? `<span class="assertion-token-${kind}">${escape(token)}</span>` : escape(token);
    }) + "\n";
    element("assertion-highlight").scrollTop = source.scrollTop;
    element("assertion-highlight").scrollLeft = source.scrollLeft;
    element("assertion-result-gutter").style.transform = `translateY(${-source.scrollTop}px)`;
  }

  function actions() {
    const organization = status?.organization_name || status?.organization_id || "Organization";
    element("assertion-revision").textContent = `${organization} / r${loadedRevision}${dirty() ? " / Unsaved" : ""}${stale() ? " / Reload required" : ""}`;
    element("assertion-save").disabled = pending || !status || !dirty() || stale();
    element("assertion-evaluate").disabled = pending || !status?.configured || status?.running || stale() || !source.value.trim();
    element("assertion-read-source").disabled = pending || !status?.configured || status?.running;
    element("assertion-reload").disabled = pending || !status;
    enabled.disabled = pending || !status?.configured;
    interval.disabled = pending;
    source.disabled = pending || !status;
  }

  function message(text, state = "") {
    element("assertion-message").textContent = text;
    element("assertion-message").dataset.state = state;
  }

  function renderSummary(summary) {
    element("assertion-source-summary").textContent = summary ? `${summary.people} people / ${summary.groups.length} groups / ${new Date(summary.observed_at).toLocaleString()}` : "Not read";
    const rows = element("assertion-group-rows");
    rows.replaceChildren();
    for (const group of summary?.groups || []) {
      const row = document.createElement("tr");
      const name = document.createElement("td");
      name.textContent = group.name;
      const count = document.createElement("td");
      count.textContent = group.members;
      const control = document.createElement("td");
      const insert = document.createElement("button");
      insert.type = "button";
      insert.textContent = "+";
      insert.title = `Insert a cardinality assertion for ${group.name}`;
      insert.setAttribute("aria-label", insert.title);
      insert.addEventListener("click", () => {
        source.setRangeText(`group ${JSON.stringify(group.name)} members >= 2;\n`, source.selectionStart, source.selectionEnd, "end");
        source.focus();
        highlight(); actions();
      });
      control.append(insert);
      row.append(name, count, control);
      rows.append(row);
    }
    const attributeRows = element("assertion-attribute-rows");
    attributeRows.replaceChildren();
    for (const attribute of summary?.attributes || []) {
      const row = document.createElement("tr");
      for (const value of [attribute.name, attribute.people, attribute.groups]) {
        const cell = document.createElement("td");
        cell.textContent = value;
        row.append(cell);
      }
      const cell = document.createElement("td");
      const insert = document.createElement("button");
      insert.type = "button";
      insert.textContent = "+";
      insert.title = `Insert presence assertion for ${attribute.name}`;
      insert.setAttribute("aria-label", insert.title);
      insert.addEventListener("click", () => {
        const target = attribute.people === 0 && attribute.groups > 0 ? "each group" : "people";
        source.setRangeText(`${target} attribute ${JSON.stringify(attribute.name)} present;\n`, source.selectionStart, source.selectionEnd, "end");
        source.focus(); highlight(); actions();
      });
      cell.append(insert);
      row.append(cell);
      attributeRows.append(row);
    }
  }

  function ruleLabel(rule) {
    if (rule.attribute) {
      const target = rule.kind === "group_attribute" ? `group ${JSON.stringify(rule.group)}` : rule.kind === "each_group_attribute" ? "each group" : `people${rule.scope ? ` in ${JSON.stringify(rule.scope)}` : ""}`;
      const operand = ["present", "absent"].includes(rule.operator) ? "" : ` ${rule.operator === "in" ? JSON.stringify(rule.values) : rule.number != null ? rule.number : JSON.stringify(rule.value || "")}`;
      return `${target} attribute ${JSON.stringify(rule.attribute)} ${rule.operator}${operand}`;
    }
    if (rule.kind === "group") return `group ${JSON.stringify(rule.group)} members ${rule.operator} ${rule.limit}`;
    if (rule.kind === "each_group") return `each group members ${rule.operator} ${rule.limit}`;
    return `people${rule.scope ? ` in ${JSON.stringify(rule.scope)}` : ""} ${rule.kind} ${JSON.stringify(rule.groups)}`;
  }

  function renderRun(run) {
    const heading = element("assertion-run-status");
    const rows = element("assertion-result-rows");
    rows.replaceChildren();
    heading.dataset.state = run?.status || "";
    heading.textContent = run ? `${run.status.toUpperCase()} / ${run.draft ? "Draft" : "Saved"} r${run.revision} / ${new Date(run.finished_at).toLocaleString()}${run.revision !== loadedRevision ? " / Stale revision" : ""}` : "Not evaluated";
    element("assertion-run-error").textContent = run?.error || "";
    renderResultGutter(run);
    for (const result of run?.results || []) {
      const row = document.createElement("tr");
      for (const value of [result.rule.line, ruleLabel(result.rule), result.status.toUpperCase(), result.message, result.subjects.join(", ")]) {
        const cell = document.createElement("td");
        cell.textContent = value;
        row.append(cell);
      }
      row.dataset.state = result.status;
      rows.append(row);
    }
  }

  function renderResultGutter(run) {
    const gutter = element("assertion-result-gutter");
    const editor = element("assertion-editor");
    gutter.replaceChildren();
    if (!run) {
      gutter.hidden = true;
      editor.dataset.resultState = "false";
      return;
    }
    const lineCount = Math.max(1, source.value.split("\n").length);
    const byLine = new Map();
    for (const result of run.results || []) {
      const line = Math.min(lineCount, Math.max(1, Number(result.rule?.line) || 1));
      if (!byLine.has(line)) byLine.set(line, []);
      byLine.get(line).push({ result, error: "" });
    }
    if (run.error || run.status === "error" && !byLine.size) {
      const match = String(run.error || "").match(/\bline\s+(\d+)\b/i);
      const line = Math.min(lineCount, Math.max(1, Number(match?.[1]) || 1));
      if (!byLine.has(line)) byLine.set(line, []);
      byLine.get(line).push({ result: null, error: run.error || "Assertion evaluation failed." });
    }
    const fragment = document.createDocumentFragment();
    for (let line = 1; line <= lineCount; line++) {
      const row = document.createElement("div");
      row.className = "assertion-result-line";
      row.dataset.line = String(line);
      for (const entry of byLine.get(line) || []) {
        const button = document.createElement("button");
        const status = entry.error ? "error" : entry.result.status;
        button.type = "button";
        button.className = "assertion-result-marker";
        button.dataset.state = status;
        button.textContent = status === "pass" ? "PASS" : status === "fail" ? "FAIL" : "ERR";
        const rule = entry.result?.rule;
        const label = rule ? ruleLabel(rule) : "Assertion evaluation error";
        button.title = `Line ${line} · ${button.textContent} · ${label}`;
        button.setAttribute("aria-label", `${button.title}; show details`);
        button.addEventListener("click", () => showResultDetail(line, entry.result, entry.error));
        row.append(button);
      }
      fragment.append(row);
    }
    gutter.replaceChildren(fragment);
    gutter.hidden = false;
    editor.dataset.resultState = "true";
    gutter.style.transform = `translateY(${-source.scrollTop}px)`;
  }

  function showResultDetail(line, result, error) {
    const dialog = element("policy-test-dialog");
    const subjects = element("policy-test-subjects");
    const status = error ? "ERROR" : String(result.status || "error").toUpperCase();
    element("policy-test-title").textContent = `Assertion ${status.toLowerCase()}`;
    element("policy-test-subject-title").textContent = error ? `Line ${line} diagnostic` : `Line ${line} details`;
    subjects.replaceChildren();
    const appendDetail = (text, meta = "") => {
      const item = document.createElement("div");
      item.className = "policy-test-subject";
      item.textContent = text;
      if (meta) {
        const note = document.createElement("small");
        note.textContent = meta;
        item.append(note);
      }
      subjects.append(item);
    };
    if (error) {
      appendDetail(error);
    } else {
      appendDetail(ruleLabel(result.rule), `${result.status.toUpperCase()} · ${result.message || `${result.checked} checked; ${result.violations} violations`}`);
      for (const subject of result.subjects || []) appendDetail(subject);
    }
    dialog.showModal();
  }

  async function request(path, options) {
    const response = await fetch(path, { cache: "no-store", ...options });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    return data;
  }

  function applyStatus(data, replace = false) {
    const organizationChanged = Boolean(loadedOrganizationID && data.organization_id && loadedOrganizationID !== data.organization_id);
    const keepDraft = organizationChanged && dirty() && !replace;
    status = data;
    if (replace || !keepDraft && (organizationChanged || !source.dataset.loaded)) {
      loadedRevision = data.settings.revision;
      savedSource = source.value = data.settings.source;
      savedEnabled = enabled.checked = data.settings.enabled;
      savedInterval = Number(data.settings.interval_seconds);
      interval.value = savedInterval;
      source.dataset.loaded = "true";
      loadedOrganizationID = data.organization_id || "";
      highlight();
      if (organizationChanged) message(`Loaded assertions for ${data.organization_name || data.organization_id}`);
    } else if (keepDraft) {
      message(`Organization changed to ${data.organization_name || data.organization_id}. Reload to discard this draft.`, "error");
    }
    element("assertion-source-status").textContent = data.configured ? `Live LDAP / ${data.base_dn}` : "Trusted LDAP source not configured";
    renderSummary(data.source_summary);
    renderRun(data.last_run);
    actions();
  }

  async function load(replace = false) {
    try { applyStatus(await request("/api/assertions"), replace); }
    catch (error) { message(error.message, "error"); }
  }

  async function command(action) {
    if (pending) return;
    pending = true; actions(); message("");
    try {
      if (action === "save") {
        const data = await request("/api/assertions", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ source: source.value, expected_revision: loadedRevision, enabled: enabled.checked, interval_seconds: Number(interval.value) }) });
        applyStatus(data, true);
        message(`Saved ${status.organization_name || status.organization_id || "organization"} revision ${loadedRevision}`);
        window.dispatchEvent(new Event("policy-assertion-saved"));
      } else if (action === "evaluate") {
        renderRun(await request("/api/assertions/evaluate", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ source: source.value, expected_revision: loadedRevision }) }));
        await load();
      } else {
        renderSummary(await request("/api/assertions/source"));
      }
    } catch (error) {
      message(error.message, "error");
      if (action === "evaluate") renderResultGutter({ status: "error", error: error.message, results: [] });
    }
    finally { pending = false; actions(); }
  }

  for (const control of [source, enabled, interval]) control.addEventListener("input", () => {
    highlight();
    renderRun(null);
    actions();
  });
  source.addEventListener("scroll", highlight);
  source.addEventListener("keydown", (event) => {
    if (event.key === "Tab") { event.preventDefault(); source.setRangeText("  ", source.selectionStart, source.selectionEnd, "end"); highlight(); actions(); }
  });
  element("assertion-save").addEventListener("click", () => command("save"));
  element("assertion-evaluate").addEventListener("click", () => command("evaluate"));
  element("assertion-read-source").addEventListener("click", () => command("read"));
  element("assertion-reload").addEventListener("click", () => { if (!dirty() || window.confirm("Discard unsaved assertion changes?")) load(true); });
  const catalogTabs = [...document.querySelectorAll("[data-assertion-catalog-tab]")];
  const selectCatalog = (selected) => {
    for (const tab of catalogTabs) {
      const active = tab === selected;
      tab.setAttribute("aria-selected", String(active));
      tab.tabIndex = active ? 0 : -1;
      element(tab.getAttribute("aria-controls")).hidden = !active;
    }
  };
  for (const tab of catalogTabs) {
    tab.addEventListener("click", () => selectCatalog(tab));
    tab.addEventListener("keydown", (event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const index = catalogTabs.indexOf(tab);
      const next = event.key === "Home" ? 0 : event.key === "End" ? catalogTabs.length - 1 : (index + (event.key === "ArrowRight" ? 1 : -1) + catalogTabs.length) % catalogTabs.length;
      selectCatalog(catalogTabs[next]); catalogTabs[next].focus();
    });
  }
  const navigation = () => {
    clearInterval(timer);
    if (active()) { load(); timer = setInterval(() => { if (!pending) load(); }, 5000); }
  };
  window.addEventListener("hashchange", navigation);
  window.addEventListener("policy-record-kind-changed", navigation);
  actions(); navigation();
})();