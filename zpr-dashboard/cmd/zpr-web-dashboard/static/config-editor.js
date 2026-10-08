(() => {
  const source = document.getElementById("zpr-config-source");
  if (!source) return;
  const byId = (id) => document.getElementById(id);
  const page = window.ZPREditorPage;
  let draftName = "";
  const status = byId("zpr-config-status");
  const records = byId("zpr-config-records");
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
    surface.setDiagnostic(null);
    delete analyze.dataset.analysisState;
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
        if (dirty() && !confirm("Discard unsaved configuration changes?")) return;
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
  const request = async (url, options = {}) => {
    const response = await window.zprOperatorFetch(url, { cache: "no-store", ...options });
    const result = await response.json();
    if (!response.ok) {
      const error = new Error(result.error || result.diagnostics || `HTTP ${response.status}`);
      error.line = result.line || 0;
      error.details = result;
      throw error;
    }
    return result;
  };
  const post = (url, body) => request(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const run = async (action) => {
    if (pending) return;
    pending = true;
    const version = sourceVersion;
    document.querySelectorAll("[data-config-command]").forEach((button) => { button.disabled = true; });
    try { await action(); } catch (error) {
      if (version !== sourceVersion) return;
      surface.setDiagnostic(error.line ? { line: error.line, message: error.message } : null);
      if (error.details?.valid === false) analyze.dataset.analysisState = "error";
      setStatus(surface.diagnostic ? "" : error.message, "error");
    }
    finally { pending = false; document.querySelectorAll("[data-config-command]").forEach((button) => { button.disabled = false; }); renderEditor(); }
  };
  async function loadCatalog() {
    catalog = await request("/api/policy");
    renderRecords();
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
    if (dirty() && !confirm("Discard unsaved configuration changes?")) return;
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
  analyze.addEventListener("click", () => run(async () => {
    const version = sourceVersion;
    const result = await post("/api/policy/config/check", { source: source.value });
    if (version !== sourceVersion) return;
    surface.setDiagnostic(null);
    analyze.dataset.analysisState = "success";
    setStatus(result.diagnostics, "success");
  }));
  format.addEventListener("click", () => run(async () => {
    const version = sourceVersion;
    const original = source.value;
    await post("/api/policy/config/check", { source: original });
    if (version !== sourceVersion) return;
    source.value = formatTOMLSpacing(original);
    clearAnalysis();
    setStatus("");
  }));
  byId("zpr-config-save").addEventListener("click", () => run(async () => {
    if (!record && !page.isNamed(draftName)) {
      const entered = prompt("Configuration name", "");
      if (entered === null) return;
      if (!page.isNamed(entered)) throw new Error("Enter a configuration name other than Untitled.");
      if (entered.trim().length > 160) throw new Error("Configuration names must be at most 160 characters.");
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
    if (!dirty() || !confirm("Discard unsaved configuration changes?")) return;
    if (record) void run(() => loadRecord(record.id));
    else reset();
  });
  source.addEventListener("input", () => { clearAnalysis(); setStatus(""); renderEditor(); });
  byId("zpr-config-new").addEventListener("click", () => {
    if (dirty() && !confirm("Discard unsaved configuration changes?")) return;
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
    if (dirty() && !confirm("Discard unsaved configuration changes?")) return;
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
  source.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
      event.preventDefault();
      byId("zpr-config-save").click();
    }
  });
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