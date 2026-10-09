(() => {
  const element = (id) => document.getElementById(id);
  const source = element("assertion-source");
  const enabled = element("assertion-enabled");
  const interval = element("assertion-interval");
  const fileMenu = element("policy-file-menu");
  element("policy-assertion-editor").prepend(element("assertion-message"));
  for (const id of ["assertion-save", "assertion-reload"]) {
    const button = element(id);
    button.setAttribute("role", "menuitem");
    button.hidden = true;
    fileMenu.append(button);
  }
  element("assertion-reload").textContent = "Discard";
  const lintWarnings = document.createElement("ul");
  lintWarnings.id = "assertion-lint-warnings";
  lintWarnings.className = "lint-warning-list";
  lintWarnings.setAttribute("aria-label", "Assertion lint warnings");
  lintWarnings.hidden = true;
  element("assertion-message").after(lintWarnings);
  source.addEventListener("input", () => { lintWarnings.replaceChildren(); lintWarnings.hidden = true; });
  const catalogSource = document.createElement("select");
  catalogSource.id = "assertion-catalog-source";
  catalogSource.setAttribute("aria-label", "Assertion trusted source");
  catalogSource.hidden = true;
  element("assertion-source-status").before(catalogSource);
  let catalogSummary;
  let status;
  let loadedRevision = 0;
  let loadedOrganizationID = "";
  let savedSource = "";
  let savedEnabled = false;
  let savedInterval = 60;
  let pending = false;
  let testPending = false;
  let testAbort = null;
  let formatAbort = null;
  let selectedRecord = null;
  let recordMode = false;
  let recordStale = false;
  let recordLastRun = null;
  let loadedRecordID = "";
  let timer;
  const analysisScope = window.ZPREditorPage.createAnalysisScope(() => [
    source.value, loadedRecordID, recordMode, loadedRevision,
    loadedOrganizationID, status?.organization_id, status?.settings?.revision,
  ]);
  const active = () => location.hash === "#policy" && !element("policy-assertion-editor")?.hidden;
  const dirty = () => recordMode
    ? selectedRecord?.isDraft ? Boolean(source.value.trim() || element("policy-draft-name").value.trim()) : source.value !== savedSource
    : source.value !== savedSource || enabled.checked !== savedEnabled || Number(interval.value) !== savedInterval;
  const stale = () => Boolean(status && (recordMode ? recordStale || status.organization_id !== loadedOrganizationID : status.settings.revision !== loadedRevision || (status.organization_id && status.organization_id !== loadedOrganizationID)));
  const escape = window.ZPRSafeDisplay.escapeHTML;
  const sourceLayout = window.ZPREditorPage.bindSourceLayout({
    source, highlight: element("assertion-highlight"),
    gutterContent: element("assertion-result-lines"),
    onResize: updateGutterBounds,
  });

  function updateGutterBounds() {
    element("assertion-result-gutter").style.height = `${Math.max(0, source.clientHeight - 15)}px`;
  }

  function highlight() {
    const keywords = new Set(["assert", "source", "from", "where", "and", "or", "person", "group", "each", "members", "people", "in", "exactly_one", "not_both", "attribute", "present", "absent", "contains"]);
    element("assertion-highlight").innerHTML = source.value.replace(/\/\/[^\n]*|"(?:\\.|[^"\\])*"?|[a-z_]+|\d+|[^a-z_\d]/g, (token) => {
      const kind = token.startsWith("//") ? "comment" : token.startsWith('"') ? "string" : keywords.has(token.toLowerCase()) ? "keyword" : /^\d+$/.test(token) ? "number" : "";
      return kind ? `<span class="assertion-token-${kind}">${escape(token)}</span>` : escape(token);
    }) + "\n";
    sourceLayout.syncScroll();
    updateGutterBounds();
  }

  function actions() {
    const organization = status?.organization_name || status?.organization_id || "Organization";
    element("assertion-revision").textContent = `${organization} / r${loadedRevision}${dirty() ? " / Unsaved" : ""}${stale() ? " / Reload required" : ""}`;
    element("policy-modified-indicator").hidden = !dirty();
    const hasDraftName = !recordMode || !selectedRecord?.isDraft || Boolean(element("policy-draft-name").value.trim());
    element("assertion-save").disabled = pending || !status || !dirty() || stale() || !hasDraftName || (recordMode && (!source.value.trim() || selectedRecord?.archived));
    element("assertion-read-source").disabled = pending || recordMode || !status?.configured || status?.running;
    element("assertion-reload").disabled = pending || !status || (recordMode && selectedRecord?.isDraft);
    element("assertion-periodic-control").hidden = true;
    element("assertion-interval-control").hidden = true;
    for (const id of ["assertion-analyze", "assertion-format"]) {
      element(id).disabled = pending || !status || stale() || !source.value.trim() || Boolean(selectedRecord?.archived);
    }
    element("assertion-analyze").disabled ||= !status?.configured || status?.running;
    element("assertion-analyze").classList.toggle("button-next-evaluate", !element("assertion-analyze").dataset.analysisState);
    element("assertion-format").classList.toggle("button-save-as-ready", !element("assertion-format").disabled);
    element("assertion-save").classList.toggle("button-save-as-ready", !element("assertion-save").disabled);
    element("assertion-save").classList.toggle("button-save-next", element("assertion-save").disabled);
    enabled.disabled = pending || recordMode || !status?.configured;
    interval.disabled = pending || recordMode;
    source.disabled = !status || Boolean(selectedRecord?.archived);
    source.readOnly = Boolean(selectedRecord?.archived);
    const catalog = document.querySelector(".assertion-catalog");
    if (catalog) catalog.dataset.testMode = String(recordMode);
    window.policyWorkbenchLayoutChanged?.();
  }

  function message(text, state = "") {
    element("assertion-message").textContent = text;
    element("assertion-message").dataset.state = state;
  }

  function renderSummary(summary) {
    catalogSummary = summary;
    const previous = catalogSource.value;
    const catalogs = summary?.sources || [];
    catalogSource.replaceChildren(...catalogs.map((catalog) => new Option(catalog.name, catalog.name)));
    catalogSource.hidden = !catalogs.length;
    catalogSource.value = catalogs.some((catalog) => catalog.name === previous) ? previous : summary?.default_source || status?.default_source || catalogs[0]?.name || "";
    const selected = catalogs.find((catalog) => catalog.name === catalogSource.value);
    const qualifier = selected ? ` from ${JSON.stringify(selected.name)}` : "";
    summary = selected || summary;
    element("assertion-source-summary").textContent = summary ? `${summary.people} people / ${summary.groups.length} groups / ${window.ZPRSafeDisplay.formatDateTime(summary.observed_at)}` : "Not read";
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
        analysisScope.invalidate();
        source.setRangeText(`group ${JSON.stringify(group.name)}${qualifier} members >= 2;\n`, source.selectionStart, source.selectionEnd, "end");
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
        analysisScope.invalidate();
        const target = attribute.people === 0 && attribute.groups > 0 ? "each group" : "people";
        source.setRangeText(`${target}${qualifier} attribute ${JSON.stringify(attribute.name)} present;\n`, source.selectionStart, source.selectionEnd, "end");
        source.focus(); highlight(); actions();
      });
      cell.append(insert);
      row.append(cell);
      attributeRows.append(row);
    }
  }

  function ruleLabel(rule) {
    const qualifier = rule.source ? ` from ${JSON.stringify(rule.source)}` : "";
    if (rule.expression) return rule.kind === "people_expression" ? `people${qualifier}${rule.scope ? ` in ${JSON.stringify(rule.scope)}` : ""} where ${rule.expression}` : `assert ${rule.expression}`;
    if (rule.attribute) {
      const target = rule.kind === "group_attribute" ? `group ${JSON.stringify(rule.group)}${qualifier}` : rule.kind === "each_group_attribute" ? `each group${qualifier}` : `people${qualifier}${rule.scope ? ` in ${JSON.stringify(rule.scope)}` : ""}`;
      const operand = ["present", "absent"].includes(rule.operator) ? "" : ` ${rule.operator === "in" ? JSON.stringify(rule.values) : rule.number != null ? rule.number : JSON.stringify(rule.value || "")}`;
      return `${target} attribute ${JSON.stringify(rule.attribute)} ${rule.operator}${operand}`;
    }
    if (rule.kind === "group") return `group ${JSON.stringify(rule.group)}${qualifier} members ${rule.operator} ${rule.limit}`;
    if (rule.kind === "each_group") return `each group${qualifier} members ${rule.operator} ${rule.limit}`;
    return `people${qualifier}${rule.scope ? ` in ${JSON.stringify(rule.scope)}` : ""} ${rule.kind} ${JSON.stringify(rule.groups)}`;
  }

  function renderRun(run) {
    if (run) {
      const state = run.status === "error" ? "error" : run.warnings?.length ? "warning" : "success";
      window.ZPREditorPage.setAnalysisState(element("assertion-analyze"), state);
    } else {
      window.ZPREditorPage.setAnalysisState(element("assertion-analyze"));
    }
    const heading = element("assertion-run-status");
    const rows = element("assertion-result-rows");
    rows.replaceChildren();
    heading.dataset.state = run?.status || "";
    heading.textContent = run ? `${run.status.toUpperCase()} / ${run.draft ? "Draft" : "Saved"} r${run.revision} / ${window.ZPRSafeDisplay.formatDateTime(run.finished_at)}${run.revision !== loadedRevision ? " / Stale revision" : ""}` : "Not evaluated";
    element("assertion-run-error").textContent = run?.error || "";
    if (run?.status === "error" || run?.error) {
      message(assertionErrorLine(run.error) === null ? run.error || "Assertion evaluation failed." : "", "error");
    }
    lintWarnings.replaceChildren();
    const lineCount = Math.max(1, source.value.split("\n").length);
    for (const warning of run?.warnings || []) {
      if (window.ZPREditorPage.sourceLine(warning.line, lineCount) !== null) continue;
      const item = document.createElement("li");
      item.textContent = `Warning [${warning.code}]: ${warning.message}`;
      lintWarnings.append(item);
    }
    lintWarnings.hidden = !lintWarnings.childElementCount;
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

  async function analyzeAssertions() {
    if (pending || testPending || !status?.configured || stale() || !source.value.trim()) return;
    const controller = new AbortController();
    testPending = true;
    testAbort = controller;
    pending = true;
    const analyzedSource = source.value;
    const isCurrent = analysisScope.begin();
    renderRun(null);
    window.ZPREditorPage.setAnalysisState(element("assertion-analyze"), "pending");
    message("");
    actions();
    try {
      const run = await request("/api/assertions/evaluate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ source: analyzedSource, expected_revision: recordMode ? status.settings.revision : loadedRevision }),
        signal: controller.signal,
      });
      if (controller.signal.aborted || !isCurrent()) return;
      if (recordMode) {
        run.revision = loadedRevision;
        run.draft = dirty();
        recordLastRun = run;
      }
      renderRun(run);
      message(run.status === "error" && assertionErrorLine(run.error) === null ? run.error || "Assertion evaluation failed." : "", "error");
    } catch (error) {
      if (error.name !== "AbortError" && !controller.signal.aborted && isCurrent()) {
        const failure = { status: "error", error: error.message, revision: loadedRevision, draft: source.value !== savedSource, finished_at: new Date().toISOString(), results: [] };
        renderRun(failure);
        window.ZPREditorPage.setAnalysisState(element("assertion-analyze"), "error");
        message(assertionErrorLine(error.message) === null ? error.message : "", "error");
      }
    } finally {
      if (testAbort === controller) {
        testAbort = null;
        testPending = false;
        pending = false;
        actions();
      }
    }
  }

  function stopTest() {
    analysisScope.invalidate();
    testAbort?.abort();
    testAbort = null;
    formatAbort?.abort();
    formatAbort = null;
    testPending = false;
    pending = false;
    renderRun(null);
    message("");
    actions();
  }

  function assertionErrorLine(error) {
    const match = String(error || "").match(/\bline\s+(\d+)(?![\d.])\b/i);
    return window.ZPREditorPage.sourceLine(match?.[1], source.value.split("\n").length);
  }

  function renderResultGutter(run) {
    const gutter = element("assertion-result-gutter");
    const resultLines = element("assertion-result-lines");
    const editor = element("assertion-editor");
    resultLines.replaceChildren();
    gutter.hidden = false;
    editor.dataset.resultState = "true";
    if (!run) {
      return;
    }
    const lineCount = Math.max(1, source.value.split("\n").length);
    const byLine = new Map();
    for (const result of run.results || []) {
      const line = window.ZPREditorPage.sourceLine(result.rule?.line, lineCount);
      if (line === null) continue;
      if (!byLine.has(line)) byLine.set(line, []);
      byLine.get(line).push({ result, error: "" });
    }
    for (const warning of run.warnings || []) {
      const line = window.ZPREditorPage.sourceLine(warning.line, lineCount);
      if (line === null) continue;
      if (!byLine.has(line)) byLine.set(line, []);
      byLine.get(line).push({ warning, result: null, error: "" });
    }
    if (run.error || run.status === "error" && !byLine.size) {
      const line = assertionErrorLine(run.error);
      if (line !== null) {
        if (!byLine.has(line)) byLine.set(line, []);
        byLine.get(line).push({ result: null, error: run.error });
      }
    }
    const fragment = document.createDocumentFragment();
    for (let line = 1; line <= lineCount; line++) {
      const row = document.createElement("div");
      row.className = "assertion-result-line";
      row.dataset.line = String(line);
      for (const entry of byLine.get(line) || []) {
        const button = document.createElement("button");
        const status = entry.warning ? "warning" : entry.error ? "error" : entry.result.status;
        button.type = "button";
        button.className = "assertion-result-marker";
        button.dataset.state = status;
        button.textContent = status === "warning" ? "WARN" : status === "pass" ? "PASS" : status === "fail" ? "FAIL" : "ERR";
        const rule = entry.result?.rule;
        const label = entry.warning ? `Warning [${entry.warning.code}]: ${entry.warning.message}` : rule ? ruleLabel(rule) : "Assertion evaluation error";
        button.title = `Line ${line} · ${button.textContent} · ${label}`;
        button.setAttribute("aria-label", `${button.title}; show details`);
        button.addEventListener("click", () => showResultDetail(line, entry.result, entry.error, entry.warning));
        row.append(button);
      }
      fragment.append(row);
    }
    resultLines.replaceChildren(fragment);
    gutter.hidden = false;
    editor.dataset.resultState = "true";
    resultLines.style.transform = `translateY(${-source.scrollTop}px)`;
  }

  function showResultDetail(line, result, error, warning) {
    const dialog = element("policy-test-dialog");
    const subjects = element("policy-test-subjects");
    const status = warning ? "WARNING" : error ? "ERROR" : String(result.status || "error").toUpperCase();
    element("policy-test-title").textContent = `Assertion ${status.toLowerCase()}`;
    element("policy-test-subject-title").textContent = warning ? `Line ${line} lint warning` : error ? `Line ${line} diagnostic` : `Line ${line} details`;
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
    } else if (warning) {
      appendDetail(`Warning [${warning.code}]`, warning.message);
    } else {
      appendDetail(ruleLabel(result.rule), `${result.status.toUpperCase()} · ${result.message || `${result.checked} checked; ${result.violations} violations`}`);
      for (const subject of result.subjects || []) appendDetail(subject);
    }
    dialog.showModal();
  }

  const request = (path, options) => window.ZPREditorPage.requestJSON(
    (...args) => window.zprOperatorFetch(...args), path, options,
  );

  function applyStatus(data, replace = false) {
    const organizationChanged = Boolean(loadedOrganizationID && data.organization_id && loadedOrganizationID !== data.organization_id);
    if (organizationChanged && testPending) stopTest();
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
    element("assertion-source-status").textContent = data.configured ? data.sources?.length ? data.sources.map((source) => `${source.name} / ${source.kind}`).join(" · ") : `Live LDAP / ${data.base_dn}` : "Trusted sources not configured";
    renderSummary(data.source_summary);
    renderRun(data.last_run);
    actions();
  }

  async function loadRecordStatus(data, replace = false) {
    status = data;
    let record = selectedRecord;
    if (record && !record.isDraft) record = await request(`/api/policy/records/${encodeURIComponent(record.id)}`);
    if (!record) return;
    const organizationChanged = Boolean(loadedOrganizationID && data.organization_id && loadedOrganizationID !== data.organization_id);
    const recordChanged = loadedRecordID !== (record.id || "");
    if (replace || organizationChanged || recordChanged) {
      source.value = record.content || "";
      savedSource = source.value;
      loadedRevision = record.current_revision || 0;
      loadedRecordID = record.id || "";
      recordStale = false;
      recordLastRun = null;
      source.dataset.loaded = "true";
      if (organizationChanged) message(`Loaded assertions for ${data.organization_name || data.organization_id}`);
    } else if (!record.isDraft && record.current_revision !== loadedRevision) {
      if (dirty()) {
        recordStale = true;
        message("This assertion record changed elsewhere. Reload before saving or testing.", "error");
      } else {
        selectedRecord = record;
        source.value = savedSource = record.content || "";
        loadedRevision = record.current_revision;
        recordLastRun = null;
      }
    }
    selectedRecord = record;
    loadedOrganizationID = data.organization_id || "";
    element("assertion-source-status").textContent = data.configured ? data.sources?.length ? data.sources.map((source) => `${source.name} / ${source.kind}`).join(" · ") : `Live LDAP / ${data.base_dn}` : "Trusted sources not configured";
    renderSummary(data.source_summary);
    renderRun(recordLastRun);
    actions();
  }

  async function load(replace = false) {
    try {
      const data = await request("/api/assertions");
      if (recordMode) await loadRecordStatus(data, replace);
      else applyStatus(data, replace);
    } catch (error) { message(error.message, "error"); }
  }

  async function saveAssertionRecord() {
    const recordName = selectedRecord.isDraft ? element("policy-draft-name").value : selectedRecord.name;
    if (!window.ZPREditorPage.isNamed(recordName)) throw new Error("Enter an assertion set name other than Untitled before saving.");
    if (selectedRecord.isDraft) {
      const name = element("policy-draft-name").value.trim();
      const record = await request("/api/policy/records", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          category_id: selectedRecord.category_id, name, kind: "assertions",
          content_type: "text/vnd.zpr.assertions", metadata: { language: "assertions" },
          content: source.value, summary: "Created assertion set",
        }),
      });
      record.isDraft = false;
      selectedRecord = record;
      loadedRecordID = record.id;
      loadedRevision = record.current_revision;
      savedSource = source.value;
      source.dataset.loaded = "true";
      recordStale = false;
      recordLastRun = null;
      window.dispatchEvent(new CustomEvent("policy-assertion-created", { detail: { record } }));
      return record;
    }
    const recordID = selectedRecord.id;
    await request(`/api/policy/records/${encodeURIComponent(recordID)}/revisions`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content: source.value, expected_revision: loadedRevision, summary: "Updated assertion set" }),
    });
    const record = await request(`/api/policy/records/${encodeURIComponent(recordID)}`);
    selectedRecord = record;
    loadedRevision = record.current_revision;
    savedSource = source.value;
    recordStale = false;
    recordLastRun = null;
    window.dispatchEvent(new Event("policy-assertion-saved"));
    return record;
  }

  async function command(action) {
    if (pending) return;
    pending = true; actions(); message("");
    try {
      if (action === "save") {
        if (recordMode) {
          const record = await saveAssertionRecord();
          message(`Saved ${record.name} / r${loadedRevision}`);
        } else {
          const data = await request("/api/assertions", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ source: source.value, expected_revision: loadedRevision, enabled: enabled.checked, interval_seconds: Number(interval.value) }) });
          applyStatus(data, true);
          message(`Saved ${status.organization_name || status.organization_id || "organization"} revision ${loadedRevision}`);
          window.dispatchEvent(new Event("policy-assertion-saved"));
        }
      } else if (action === "evaluate") {
        const run = await request("/api/assertions/evaluate", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ source: source.value, expected_revision: recordMode ? status.settings.revision : loadedRevision }) });
        if (recordMode) {
          run.revision = loadedRevision;
          run.draft = dirty();
          recordLastRun = run;
          renderRun(run);
        } else {
          renderRun(run);
          await load();
        }
      } else {
        renderSummary(await request("/api/assertions/source"));
      }
    } catch (error) {
      message(error.message, "error");
      if (action === "evaluate") renderResultGutter({ status: "error", error: error.message, results: [] });
    }
    finally { pending = false; actions(); }
  }

  for (const control of [source, enabled, interval]) control.addEventListener("input", () => { analysisScope.invalidate(); window.ZPREditorPage.setAnalysisState(element("assertion-analyze")); highlight(); renderRun(null); actions(); });
  source.addEventListener("keydown", (event) => {
    if (event.key === "Tab") { event.preventDefault(); source.setRangeText("  ", source.selectionStart, source.selectionEnd, "end"); source.dispatchEvent(new InputEvent("input", { bubbles: true })); }
  });
  element("assertion-save").addEventListener("click", () => command("save"));
  window.ZPREditorPage.bindSaveShortcut({ root: source, button: element("assertion-save") });
  async function formatAssertions() {
    if (pending || testPending || !status || stale() || !source.value.trim() || source.disabled || source.readOnly) return;
    const controller = new AbortController();
    formatAbort = controller;
    const original = source.value;
    const isCurrent = analysisScope.begin();
    pending = true;
    actions();
    message("");
    try {
      const result = await request("/api/assertions/format", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ source: original }),
        signal: controller.signal,
      });
      if (controller.signal.aborted || !isCurrent()) return;
      if (typeof result.source !== "string") throw new Error("Assertion formatter returned an invalid source response.");
      const warnings = (result.warnings || []).map((warning) => {
        const item = document.createElement("li");
        item.textContent = `Line ${warning.line}: ${warning.code} - ${warning.message}`;
        return item;
      });
      window.ZPRAssistant.editText(source, result.source, true);
      window.ZPREditorPage.setAnalysisState(element("assertion-analyze"));
      renderRun(null);
      highlight();
      lintWarnings.replaceChildren(...warnings);
      lintWarnings.hidden = !lintWarnings.childElementCount;
      message("Assertions formatted.");
    } catch (error) {
      if (error.name === "AbortError" || controller.signal.aborted || !isCurrent()) return;
      message(assertionErrorLine(error.message) === null ? error.message : "", "error");
      renderResultGutter({ status: "error", error: error.message, results: [] });
    } finally {
      if (formatAbort === controller) {
        formatAbort = null;
        pending = false;
        actions();
      }
    }
  }
  element("assertion-analyze").addEventListener("click", analyzeAssertions);
  element("assertion-format").addEventListener("click", formatAssertions);
  element("assertion-read-source").addEventListener("click", () => command("read"));
  element("assertion-reload").addEventListener("click", () => { if (window.ZPREditorPage.confirmDiscard(dirty(), "Discard unsaved assertion changes?")) load(true); });
  element("policy-draft-name").addEventListener("input", actions);
  window.policyAssertionDirty = () => recordMode ? dirty() : false;
  window.addEventListener("policy-record-kind-changed", (event) => {
    analysisScope.invalidate();
    const { kind, record } = event.detail || {};
    const nextRecordMode = kind === "assertions" && record && (record.isDraft || record.content_type === "text/vnd.zpr.assertions");
    if ((testPending || formatAbort) && (!nextRecordMode || selectedRecord?.id !== record?.id)) stopTest();
    selectedRecord = nextRecordMode ? record : null;
    recordMode = Boolean(nextRecordMode);
    recordStale = false;
    recordLastRun = null;
    loadedRecordID = recordMode ? record.id || "" : "";
    if (recordMode) {
      source.value = record.content || "";
      savedSource = source.value;
      loadedRevision = record.current_revision || 0;
      source.dataset.loaded = "true";
      enabled.checked = false;
      interval.value = "60";
    }
    highlight();
    renderRun(null);
    message("");
    navigation();
  });
  const catalogTabs = [...document.querySelectorAll("[data-assertion-catalog-tab]")];
  catalogSource.addEventListener("change", () => renderSummary(catalogSummary));
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
  window.policyAssertionDirty = () => recordMode ? dirty() : false;
  window.addEventListener("policy-record-kind-changed", (event) => {
    analysisScope.invalidate();
    const { kind, record } = event.detail || {};
    const nextRecordMode = kind === "assertions" && record && (record.isDraft || record.content_type === "text/vnd.zpr.assertions");
    if ((testPending || formatAbort) && (!nextRecordMode || selectedRecord?.id !== record?.id)) stopTest();
    selectedRecord = nextRecordMode ? record : null;
    recordMode = Boolean(nextRecordMode);
    recordStale = false;
    recordLastRun = null;
    loadedRecordID = recordMode ? record.id || "" : "";
    if (recordMode) {
      source.value = record.content || "";
      savedSource = source.value;
      loadedRevision = record.current_revision || 0;
      source.dataset.loaded = "true";
      enabled.checked = false;
      interval.value = "60";
    }
    highlight();
    renderRun(null);
    message("");
    actions();
    navigation();
  });
  element("policy-draft-name").addEventListener("input", actions);
  const navigation = () => {
    clearInterval(timer);
    if (!active() && (testPending || formatAbort)) stopTest();
    if (active()) { load(); timer = setInterval(() => { if (!pending) load(); }, 5000); }
  };
  window.addEventListener("hashchange", navigation);
  actions(); navigation();
})();