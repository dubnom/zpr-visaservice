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
  const request = async (url, options = {}) => {
    const response = await fetch(url, { cache: "no-store", ...options });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || result.diagnostics || `HTTP ${response.status}`);
    return result;
  };
  const post = (url, body) => request(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const run = async (action) => {
    if (pending) return;
    pending = true;
    document.querySelectorAll("[data-config-command]").forEach((button) => { button.disabled = true; });
    try { await action(); } catch (error) { status.textContent = error.message; }
    finally { pending = false; document.querySelectorAll("[data-config-command]").forEach((button) => { button.disabled = false; }); }
  };
  async function loadCatalog() {
    catalog = await request("/api/policy");
    const records = (catalog.records || []).filter((entry) => entry.kind === "configuration" && !entry.archived);
    picker.replaceChildren(new Option("New draft", ""), ...records.map((entry) => new Option(entry.name, entry.id)));
    picker.value = record?.id || "";
  }
  async function loadRecord(id) {
    record = await request(`/api/policy/records/${encodeURIComponent(id)}`);
    name.value = record.name;
    source.value = saved = record.content;
    const revisions = await request(`/api/policy/records/${encodeURIComponent(id)}/revisions`);
    history.replaceChildren(new Option("Current revision", ""), ...revisions.map((revision) => new Option(`r${revision.number || revision.revision} · ${revision.summary || ""}`, String(revision.number || revision.revision))));
    status.textContent = `Loaded r${record.current_revision}; not applied.`;
  }
  picker.addEventListener("change", () => run(async () => {
    if (source.value !== saved && !confirm("Discard unsaved configuration changes?")) { picker.value = record?.id || ""; return; }
    if (picker.value) await loadRecord(picker.value);
    else { record = null; name.value = ""; source.value = saved = ""; history.replaceChildren(); status.textContent = "New draft"; }
  }));
  history.addEventListener("change", () => run(async () => {
    if (!record || !history.value) return;
    const revision = await request(`/api/policy/records/${encodeURIComponent(record.id)}/revisions/${encodeURIComponent(history.value)}`);
    source.value = revision.content;
    status.textContent = `Historical r${history.value}; save to create a new revision.`;
  }));
  document.getElementById("zpr-config-validate").addEventListener("click", () => run(async () => {
    const result = await post("/api/policy/config/check", { source: source.value });
    status.textContent = result.diagnostics;
  }));
  document.getElementById("zpr-config-save").addEventListener("click", () => run(async () => {
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
  source.addEventListener("input", () => { status.textContent = source.value === saved ? "Saved draft" : "Unsaved draft"; });
  window.addEventListener("hashchange", () => { if (location.hash === "#zpr-config" && !catalog) void run(loadCatalog); });
  if (location.hash === "#zpr-config") void run(loadCatalog);
})();