(() => {
  const source = document.getElementById("zpr-config-source");
  if (!source) return;
  const picker = document.getElementById("zpr-config-picker");
  const name = document.getElementById("zpr-config-name");
  const history = document.getElementById("zpr-config-history");
  const status = document.getElementById("zpr-config-status");
  let record;
  let catalog;
  let saved = "";
  let pending = false;
  let diagnostic = null;
  const gutter = document.getElementById("zpr-config-gutter");
  const highlight = document.getElementById("zpr-config-highlight");
  const analyze = document.getElementById("zpr-config-validate");
  let sourceVersion = 0;
  function clearAnalysis() {
    sourceVersion++;
    diagnostic = null;
    delete analyze.dataset.analysisState;
  }
  function syncScroll() {
    gutter.scrollTop = source.scrollTop;
    highlight.scrollTop = source.scrollTop;
    highlight.scrollLeft = source.scrollLeft;
    source.closest(".config-source-editor").dataset.horizontalOverflow = String(source.scrollWidth > source.clientWidth);
  }
  function renderEditor() {
    document.getElementById("zpr-config-modified").hidden = source.value === saved;
    gutter.replaceChildren(...source.value.split("\n").map((_, index) => {
      const row = document.createElement("div");
      row.className = "config-gutter-line";
      row.dataset.line = String(index + 1);
      if (diagnostic?.line === index + 1) {
        const marker = document.createElement("button");
        marker.type = "button";
        marker.className = "config-error-marker";
        marker.title = diagnostic.message;
        marker.setAttribute("aria-label", `Configuration error on line ${index + 1}: ${diagnostic.message}`);
        marker.textContent = "!";
        marker.addEventListener("click", () => {
          source.focus();
          const offset = source.value.split("\n").slice(0, index).reduce((length, line) => length + line.length + 1, 0);
          source.setSelectionRange(offset, offset + (source.value.split("\n")[index]?.length || 0));
        });
        row.append(marker);
      } else {
        const spacer = document.createElement("span");
        spacer.className = "config-gutter-marker-space";
        spacer.setAttribute("aria-hidden", "true");
        row.append(spacer);
      }
      return row;
    }));
    const tokens = /("""(?:\\[\s\S]|(?!""")[^\\])*"""|'''[\s\S]*?'''|"(?:\\.|[^"\\\n])*"|'[^'\n]*'|#[^\n]*|^[ \t]*\[\[?[^\n\]]+\]\]?|\b(?:true|false|inf|nan)\b|[+-]?\b\d[\w.+:-]*\b|[A-Za-z0-9_-]+(?=\s*(?:\.|=)))/gm;
    highlight.replaceChildren();
    let offset = 0;
    for (const match of source.value.matchAll(tokens)) {
      highlight.append(document.createTextNode(source.value.slice(offset, match.index)));
      const token = document.createElement("span");
      const text = match[0];
      const kind = text.startsWith("#") ? "comment" : /^["']/.test(text) ? "string" : text.trimStart().startsWith("[") ? "class-definition" : /^(true|false|inf|nan)$/.test(text) ? "keyword" : /^[+\-\d]/.test(text) ? "value" : "attribute";
      token.className = `zpl-${kind}`;
      token.textContent = text;
      highlight.append(token);
      offset = match.index + text.length;
    }
    highlight.append(document.createTextNode(source.value.slice(offset) + "\n"));
    syncScroll();
  }
  const request = async (url, options = {}) => {
    const response = await fetch(url, { cache: "no-store", ...options });
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
      diagnostic = error.line ? { line: error.line, message: error.message } : null;
      if (error.details?.valid === false) analyze.dataset.analysisState = "error";
      status.textContent = diagnostic ? `Syntax error on line ${diagnostic.line}; select its gutter marker for details.` : error.message;
      renderEditor();
    }
    finally { pending = false; document.querySelectorAll("[data-config-command]").forEach((button) => { button.disabled = false; }); renderEditor(); }
  };
  async function loadCatalog() {
    catalog = await request("/api/policy");
    const records = (catalog.records || []).filter((entry) => entry.kind === "configuration" && !entry.archived);
    picker.replaceChildren(new Option("New draft", ""), ...records.map((entry) => new Option(entry.name, entry.id)));
    picker.value = record?.id || "";
  }
  async function loadRecord(id) {
    record = await request(`/api/policy/records/${encodeURIComponent(id)}`);
    clearAnalysis();
    name.value = record.name;
    source.value = saved = record.content;
    const revisions = await request(`/api/policy/records/${encodeURIComponent(id)}/revisions`);
    history.replaceChildren(new Option("Current revision", ""), ...revisions.map((revision) => new Option(`r${revision.number || revision.revision} · ${revision.summary || ""}`, String(revision.number || revision.revision))));
    status.textContent = `Loaded r${record.current_revision}; not applied.`;
  }
  picker.addEventListener("change", () => run(async () => {
    if (source.value !== saved && !confirm("Discard unsaved configuration changes?")) { picker.value = record?.id || ""; return; }
    if (picker.value) await loadRecord(picker.value);
    else { record = null; clearAnalysis(); name.value = ""; source.value = saved = ""; history.replaceChildren(); status.textContent = ""; renderEditor(); }
  }));
  history.addEventListener("change", () => run(async () => {
    if (!record || !history.value) return;
    const revision = await request(`/api/policy/records/${encodeURIComponent(record.id)}/revisions/${encodeURIComponent(history.value)}`);
    clearAnalysis();
    source.value = revision.content;
    status.textContent = `Historical r${history.value}; save to create a new revision.`;
  }));
  analyze.addEventListener("click", () => run(async () => {
    const version = sourceVersion;
    const result = await post("/api/policy/config/check", { source: source.value });
    if (version !== sourceVersion) return;
    diagnostic = null;
    analyze.dataset.analysisState = "success";
    status.textContent = result.diagnostics;
    renderEditor();
  }));
  document.getElementById("zpr-config-format").addEventListener("click", () => run(async () => {
    const version = sourceVersion;
    const original = source.value;
    await post("/api/policy/config/check", { source: original });
    if (version !== sourceVersion) return;
    source.value = formatTOMLSpacing(original);
    clearAnalysis();
    status.textContent = "TOML spacing formatted. Review and save the draft when ready.";
    renderEditor();
  }));
  document.getElementById("zpr-config-save").addEventListener("click", () => run(async () => {
    fileMenu.open = false;
    if (!name.value.trim()) throw new Error("Enter a configuration name.");
    const current = await request("/api/policy");
    if (current.organization_id !== catalog.organization_id) throw new Error("Organization changed; reload before saving.");
    if (record) {
      await post(`/api/policy/records/${encodeURIComponent(record.id)}/revisions`, { content: source.value, expected_revision: record.current_revision, summary: "Updated ZPLC configuration draft" });
      await loadRecord(record.id);
    } else {
      let category = current.categories.find((entry) => entry.path === "ZPR Config");
      if (!category) category = await post("/api/policy/categories", { name: "ZPR Config" });
      record = await post("/api/policy/records", { category_id: category.id, name: name.value.trim(), kind: "configuration", content_type: "text/vnd.zpr.zplc", metadata: { language: "toml" }, content: source.value, summary: "Initial ZPLC configuration draft" });
      await loadRecord(record.id);
    }
    await loadCatalog();
    status.textContent = `Saved r${record.current_revision}; runtime unchanged.`;
  }));
  source.addEventListener("input", () => { clearAnalysis(); status.textContent = source.value === saved ? "Saved draft" : "Unsaved draft"; renderEditor(); });
  source.addEventListener("scroll", syncScroll);
  new ResizeObserver(syncScroll).observe(source);
  const fileMenu = document.querySelector(".config-file-menu");
  document.getElementById("zpr-config-new").addEventListener("click", () => {
    fileMenu.open = false;
    if (source.value !== saved && !confirm("Discard unsaved configuration changes?")) return;
    record = null;
    clearAnalysis();
    name.value = "";
    source.value = saved = "";
    picker.value = "";
    history.replaceChildren();
    status.textContent = "";
    renderEditor();
    source.focus();
  });
  const fileInput = document.getElementById("zpr-config-file-input");
  document.getElementById("zpr-config-open").addEventListener("click", () => { fileMenu.open = false; fileInput.click(); });
  fileInput.addEventListener("change", async () => {
    const file = fileInput.files?.[0];
    fileInput.value = "";
    if (!file) return;
    if (file.size > 1048576) { status.textContent = "Configuration file exceeds the 1 MiB editor limit."; return; }
    if (source.value !== saved && !confirm("Discard unsaved configuration changes?")) return;
    let content;
    try { content = await file.text(); } catch (error) {
      status.textContent = `Could not open ${file.name}: ${error.message}`;
      return;
    }
    source.value = content;
    saved = "";
    record = null;
    clearAnalysis();
    picker.value = "";
    name.value = file.name.replace(/\.(toml|zplc)$/i, "");
    history.replaceChildren();
    status.textContent = `Opened ${file.name}; save to create a versioned draft.`;
    renderEditor();
  });
  document.getElementById("zpr-config-download").addEventListener("click", () => {
    fileMenu.open = false;
    const baseName = name.value.trim().replace(/[^a-zA-Z0-9._-]+/g, "-") || "zpr-config";
    const url = URL.createObjectURL(new Blob([source.value], { type: "application/toml;charset=utf-8" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${baseName}.toml`;
    anchor.click();
    URL.revokeObjectURL(url);
  });
  window.addEventListener("hashchange", () => { if (location.hash === "#zpr-config" && !catalog) void run(loadCatalog); });
  if (location.hash === "#zpr-config") void run(loadCatalog);
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