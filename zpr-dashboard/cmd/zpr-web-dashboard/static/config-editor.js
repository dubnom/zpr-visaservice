(() => {
  const source = document.getElementById("zpr-config-source");
  if (!source) return;
  const byId = (id) => document.getElementById(id);
  const page = window.ZPREditorPage;
  let draftName = "";
  const status = byId("zpr-config-status");
  page.placeStatus(status);
  const records = byId("zpr-config-records");
  const recordMenu = byId("zpr-config-record-menu");
  const analyze = byId("zpr-config-validate");
  const format = byId("zpr-config-format");
  page.fitSourceToViewport(source.closest(".config-source-editor"));
  let record = null;
  let catalog;
  let revisions = [];
  let browsingRevision = 0;
  let saved = "";
  let pending = false;
  let sourceVersion = 0;
  let menuRecordID = "";
  const analysisScope = page.createAnalysisScope(() => [
    source.value, catalog?.organization_id, record?.id,
    record?.current_revision, browsingRevision,
  ]);
  window.getConfigAssistantContext = () => [catalog?.organization_id, record?.id, record?.current_revision, browsingRevision];
  const surface = page.createSourceSurface({
    source, highlight: byId("zpr-config-highlight"), gutter: byId("zpr-config-gutter"), language: "toml", label: "Configuration",
  });
  const picker = page.createPicker({ toggle: byId("zpr-config-picker-toggle"), pane: byId("zpr-config-catalog-pane"), close: byId("zpr-config-picker-close"), focus: records });
  page.createMenu({ root: byId("zpr-config-actions"), toggle: byId("zpr-config-files-toggle"), menu: byId("zpr-config-file-menu") });
  const history = page.createHistory({
    menu: byId("zpr-config-history-menu"), list: byId("zpr-config-history"), count: byId("zpr-config-history-count"),
    isAvailable: () => Boolean(record),
  });
  const dirty = () => source.value !== saved;
  const setStatus = (text = "", kind = "") => page.setStatus(status, text, kind);
  function clearAnalysis() {
    sourceVersion++;
    analysisScope.invalidate();
    surface.setDiagnostic(null);
    page.setAnalysisState(analyze);
  }
  function syncAnalysisButtons() {
    const disabled = pending || !source.value.trim();
    analyze.disabled = disabled;
    format.disabled = disabled;
    format.classList.toggle("button-save-as-ready", !disabled);
    analyze.classList.toggle("button-next-evaluate", !disabled && !analyze.dataset.analysisState);
    byId("zpr-config-discard").disabled = pending || !dirty();
  }
  function renderIdentity() {
    page.renderIdentity(
      { title: byId("zpr-config-title"), version: byId("zpr-config-revision-label"), modified: byId("zpr-config-modified") },
      {
        name: record?.name || draftName,
        label: record ? `Version ${browsingRevision || record.current_revision}` : "",
        tooltip: record ? `ZPR Config/${record.name}` : "",
        dirty: dirty(),
      },
    );
  }
  function renderHistory() {
    history.render(revisions.map((revision) => ({ ...revision, number: revision.number || revision.revision })).sort((a, b) => b.number - a.number), {
      current: browsingRevision || record?.current_revision,
      onSelect: (revision) => void run(() => loadRevision(revision.number)),
      detail: (revision) => revision.summary || "",
      meta: (revision) => [revision.author, revision.created_at ? new Date(revision.created_at).toLocaleString() : ""].filter(Boolean).join(" · "),
    });
  }
  function renderRecords() {
    const entries = (catalog?.records || []).filter((entry) => entry.kind === "configuration" && !entry.archived);
    byId("zpr-config-count").textContent = catalog ? String(entries.length) : "—";
    records.replaceChildren();
    if (!entries.length) {
      const empty = document.createElement("p");
      empty.className = "catalog-empty";
      empty.textContent = catalog ? "No saved configurations." : "Loading configurations…";
      records.append(empty);
      return;
    }
    for (const entry of entries) {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "policy-record-item config-record-item";
      item.dataset.kind = "configuration";
      item.dataset.recordId = entry.id;
      item.setAttribute("role", "treeitem");
      item.setAttribute("aria-haspopup", "menu");
      item.setAttribute("aria-controls", "zpr-config-record-menu");
      item.setAttribute("aria-selected", String(record?.id === entry.id));
      const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      icon.setAttribute("class", "policy-kind-icon");
      icon.setAttribute("viewBox", "0 0 16 16");
      icon.setAttribute("aria-hidden", "true");
      icon.innerHTML = '<path d="M3 2h7l3 3v9H3Z M10 2v3h3 M5 8h6 M5 10.5h6"/>';
      const title = document.createElement("strong");
      title.textContent = entry.name;
      const detail = document.createElement("small");
      detail.textContent = `Version ${entry.current_revision}`;
      item.append(icon, title, detail);
      item.addEventListener("click", () => void run(async () => {
        if (record?.id === entry.id) { picker.setOpen(false, true); return; }
        if (!page.confirmDiscard(dirty(), "Discard unsaved configuration changes?")) return;
        await loadRecord(entry.id);
        picker.setOpen(false, true);
      }));
      records.append(item);
    }
  }
  function renderEditor() {
    syncAnalysisButtons();
    surface.render();
    renderIdentity();
  }
  const request = (url, options) => page.requestJSON((...args) => window.zprOperatorFetch(...args), url, options);
  const post = (url, body) => request(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  function closeRecordMenu(restoreFocus = false) {
    if (recordMenu.hidden) return;
    recordMenu.hidden = true;
    if (restoreFocus) {
      const origin = records.querySelector(`[data-record-id="${CSS.escape(menuRecordID)}"]`);
      (origin || records).focus();
    }
    menuRecordID = "";
  }
  function openRecordMenu(item, x, y) {
    if (!item) return;
    closeRecordMenu();
    menuRecordID = item.dataset.recordId;
    recordMenu.hidden = false;
    recordMenu.style.left = `${Math.max(8, Math.min(x, innerWidth - recordMenu.offsetWidth - 8))}px`;
    recordMenu.style.top = `${Math.max(8, Math.min(y, innerHeight - recordMenu.offsetHeight - 8))}px`;
    recordMenu.querySelector("button:not(:disabled)")?.focus();
  }
  const run = async (action, isCurrent = null) => {
    if (pending) return;
    pending = true;
    const version = sourceVersion;
    document.querySelectorAll("[data-config-command]").forEach((button) => { button.disabled = true; });
    try { await action(); } catch (error) {
      if (isCurrent ? !isCurrent() : version !== sourceVersion) return;
      surface.setDiagnostic(error.line ? { line: error.line, message: error.message } : null);
      if (error.details?.valid === false) page.setAnalysisState(analyze, "error");
      setStatus(surface.diagnostic ? "" : error.message, "error");
    }
    finally { pending = false; document.querySelectorAll("[data-config-command]").forEach((button) => { button.disabled = false; }); renderEditor(); }
  };
  async function loadCatalog() {
    catalog = await request("/api/policy");
    renderRecords();
  }
  async function renameRecord(id) {
    const latest = await request(`/api/policy/records/${encodeURIComponent(id)}`);
    if (latest.kind !== "configuration" || latest.archived) throw new Error("This configuration is no longer available to rename.");
    const name = prompt("Rename configuration", latest.name);
    if (name === null) return;
    if (name.trim() === latest.name) return;
    const renamed = await post(`/api/policy/records/${encodeURIComponent(id)}/rename`, {
      name, expected_revision: latest.current_revision,
    });
    if (record?.id === id) {
      record = renamed;
      draftName = renamed.name;
    }
    await loadCatalog();
    setStatus(`Renamed to ${renamed.name}`, "success");
  }
  async function loadRecord(id) {
    record = await request(`/api/policy/records/${encodeURIComponent(id)}`);
    clearAnalysis();
    browsingRevision = 0;
    draftName = record.name;
    source.value = saved = record.content;
    revisions = await request(`/api/policy/records/${encodeURIComponent(id)}/revisions`);
    setStatus("");
    renderHistory();
    renderRecords();
  }
  async function loadRevision(number) {
    if (!record) return;
    if (!page.confirmDiscard(dirty(), "Discard unsaved configuration changes?")) return;
    const revision = await request(`/api/policy/records/${encodeURIComponent(record.id)}/revisions/${encodeURIComponent(number)}`);
    clearAnalysis();
    browsingRevision = number === record.current_revision ? 0 : number;
    source.value = revision.content;
    saved = browsingRevision ? "" : revision.content;
    setStatus("");
    renderHistory();
  }
  function reset() {
    record = null;
    revisions = [];
    browsingRevision = 0;
    clearAnalysis();
    draftName = "";
    source.value = saved = "";
    setStatus("");
    renderHistory();
    renderRecords();
    renderEditor();
  }
  const runSourceAction = (action) => {
    if (pending) return;
    const isCurrent = analysisScope.begin();
    return run(() => action(isCurrent), isCurrent);
  };
  analyze.addEventListener("click", () => runSourceAction(async (isCurrent) => {
    const result = await post("/api/policy/config/check", { source: source.value });
    if (!isCurrent()) return;
    surface.setDiagnostic(null);
    page.setAnalysisState(analyze, "success");
    setStatus(result.diagnostics, "success");
  }));
  format.addEventListener("click", () => runSourceAction(async (isCurrent) => {
    const original = source.value;
    await post("/api/policy/config/check", { source: original });
    if (!isCurrent()) return;
    source.value = formatTOMLSpacing(original);
    clearAnalysis();
    setStatus("");
  }));
  byId("zpr-config-save").addEventListener("click", () => run(async () => {
    if (!record && !page.isNamed(draftName)) {
      const entered = prompt("Configuration name", "");
      if (entered === null) return;
      if (!page.isNamed(entered)) throw new Error("Enter a configuration name other than Untitled.");
      if (entered.trim().length > 100) throw new Error("Configuration names must be at most 100 characters.");
      draftName = entered.trim();
      renderIdentity();
    }
    if (!page.isNamed(record?.name || draftName)) throw new Error("Rename the configuration before saving; Untitled is reserved for unnamed drafts.");
    const current = await request("/api/policy");
    if (catalog && current.organization_id !== catalog.organization_id) throw new Error("Organization changed; reload before saving.");
    if (record) {
      await post(`/api/policy/records/${encodeURIComponent(record.id)}/revisions`, { content: source.value, expected_revision: record.current_revision, summary: "Updated ZPLC configuration draft" });
      await loadRecord(record.id);
    } else {
      let category = current.categories.find((entry) => entry.path === "ZPR Config");
      if (!category) category = await post("/api/policy/categories", { name: "ZPR Config" });
      record = await post("/api/policy/records", { category_id: category.id, name: draftName, kind: "configuration", content_type: "text/vnd.zpr.zplc", metadata: { language: "toml" }, content: source.value, summary: "Initial ZPLC configuration draft" });
      await loadRecord(record.id);
    }
    await loadCatalog();
    setStatus(`Saved version ${record.current_revision}; runtime unchanged.`, "success");
  }));
  byId("zpr-config-discard").addEventListener("click", () => {
    if (!dirty() || !page.confirmDiscard(true, "Discard unsaved configuration changes?")) return;
    if (record) void run(() => loadRecord(record.id));
    else reset();
  });
  records.addEventListener("contextmenu", (event) => {
    const item = event.target.closest("[data-record-id]");
    if (!item) return;
    event.preventDefault();
    openRecordMenu(item, event.clientX, event.clientY);
  });
  records.addEventListener("keydown", (event) => {
    if (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10")) return;
    event.preventDefault();
    openRecordMenu(event.target.closest("[data-record-id]"), event.target.getBoundingClientRect().left, event.target.getBoundingClientRect().bottom);
  });
  byId("zpr-config-rename").addEventListener("click", () => {
    const id = menuRecordID;
    closeRecordMenu();
    if (id) void run(() => renameRecord(id));
  });
  recordMenu.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      event.preventDefault();
      closeRecordMenu(true);
    }
  });
  document.addEventListener("pointerdown", (event) => {
    if (!event.target.closest("#zpr-config-record-menu")) closeRecordMenu();
  });
  document.addEventListener("keydown", (event) => {
    if (recordMenu.hidden || !["Escape", "Tab"].includes(event.key)) return;
    if (event.key === "Escape") event.preventDefault();
    closeRecordMenu(true);
  });
  window.addEventListener("hashchange", () => closeRecordMenu());
  window.addEventListener("resize", () => closeRecordMenu());
  source.addEventListener("input", () => { clearAnalysis(); setStatus(""); renderEditor(); });
  byId("zpr-config-new").addEventListener("click", () => {
    if (!page.confirmDiscard(dirty(), "Discard unsaved configuration changes?")) return;
    reset();
    source.focus();
  });
  const fileInput = byId("zpr-config-file-input");
  byId("zpr-config-open").addEventListener("click", () => fileInput.click());
  fileInput.addEventListener("change", async () => {
    const file = fileInput.files?.[0];
    fileInput.value = "";
    if (!file) return;
    if (file.size > 1048576) { setStatus("Configuration file exceeds the 1 MiB editor limit.", "error"); return; }
    if (!page.confirmDiscard(dirty(), "Discard unsaved configuration changes?")) return;
    let content;
    try { content = await file.text(); } catch (error) {
      setStatus(`Could not open ${file.name}: ${error.message}`, "error");
      return;
    }
    reset();
    source.value = content;
    draftName = file.name.replace(/\.(toml|zplc)$/i, "");
    renderEditor();
  });
  byId("zpr-config-download").addEventListener("click", () => {
    const baseName = (record?.name || draftName).trim().replace(/[^a-zA-Z0-9._-]+/g, "-") || "zpr-config";
    const url = URL.createObjectURL(new Blob([source.value], { type: "application/toml;charset=utf-8" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${baseName}.toml`;
    anchor.click();
    URL.revokeObjectURL(url);
  });
  page.bindSaveShortcut({ root: source, button: byId("zpr-config-save") });
  window.addEventListener("hashchange", () => { if (location.hash === "#zpr-config" && !catalog) void run(loadCatalog); });
  if (location.hash === "#zpr-config") void run(loadCatalog);
  renderRecords();
  renderHistory();
  renderEditor();
})();

function formatTOMLSpacing(source) {
  let multiline = "";
  const findAssignment = (line) => {
    let assignment = -1;
    for (let index = 0; index < line.length;) {
      if (multiline) {
        const end = line.indexOf(multiline, index);
        if (end < 0) return assignment;
        let escapes = 0;
        for (let cursor = end - 1; cursor >= 0 && line[cursor] === "\\"; cursor--) escapes++;
        if (multiline === '"""' && escapes % 2) { index = end + multiline.length; continue; }
        multiline = "";
        index = end + 3;
        continue;
      }
      const character = line[index];
      if (character === "#") break;
      if (character === '"' || character === "'") {
        const delimiter = line.slice(index, index + 3) === character.repeat(3) ? character.repeat(3) : character;
        if (delimiter.length === 3) { multiline = delimiter; index += 3; continue; }
        index++;
        while (index < line.length) {
          if (character === '"' && line[index] === "\\") { index += 2; continue; }
          if (line[index++] === character) break;
        }
        continue;
      }
      if (character === "=" && assignment < 0) assignment = index;
      index++;
    }
    return assignment;
  };
  return source.split("\n").map((line) => {
    const index = findAssignment(line);
    if (index < 0) return line;
    const left = line.slice(0, index).replace(/[ \t]+$/, "");
    const right = line.slice(index + 1).replace(/^[ \t]+/, "");
    return `${left} = ${right}`;
  }).join("\n");
}