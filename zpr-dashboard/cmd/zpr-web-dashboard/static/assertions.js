(() => {
  const element = (id) => document.getElementById(id);
  const source = element("assertion-source");
  const enabled = element("assertion-enabled");
  const interval = element("assertion-interval");
  let status;
  let loadedRevision = 0;
  let savedSource = "";
  let savedEnabled = false;
  let savedInterval = 60;
  let pending = false;
  let timer;
  const active = () => location.hash === "#assertions";
  const dirty = () => source.value !== savedSource || enabled.checked !== savedEnabled || Number(interval.value) !== savedInterval;
  const escape = (text) => String(text ?? "").replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character]);

  function highlight() {
    const keywords = new Set(["group", "each", "members", "people", "in", "exactly_one", "not_both"]);
    element("assertion-highlight").innerHTML = source.value.replace(/\/\/[^\n]*|"(?:\\.|[^"\\])*"?|[a-z_]+|\d+|[^a-z_\d]/g, (token) => {
      const kind = token.startsWith("//") ? "comment" : token.startsWith('"') ? "string" : keywords.has(token) ? "keyword" : /^\d+$/.test(token) ? "number" : "";
      return kind ? `<span class="assertion-token-${kind}">${escape(token)}</span>` : escape(token);
    }) + "\n";
    element("assertion-highlight").scrollTop = source.scrollTop;
    element("assertion-highlight").scrollLeft = source.scrollLeft;
  }

  function actions() {
    const stale = status && status.settings.revision !== loadedRevision;
    element("assertion-revision").textContent = `Global / r${loadedRevision}${dirty() ? " / Unsaved" : ""}${stale ? " / Reload required" : ""}`;
    element("assertion-save").disabled = pending || !status || !dirty() || stale;
    element("assertion-evaluate").disabled = pending || !status?.configured || status?.running || stale || !source.value.trim();
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
  }

  function ruleLabel(rule) {
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

  async function request(path, options) {
    const response = await fetch(path, { cache: "no-store", ...options });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    return data;
  }

  function applyStatus(data, replace = false) {
    status = data;
    if (replace || !source.dataset.loaded) {
      loadedRevision = data.settings.revision;
      savedSource = source.value = data.settings.source;
      savedEnabled = enabled.checked = data.settings.enabled;
      savedInterval = Number(data.settings.interval_seconds);
      interval.value = savedInterval;
      source.dataset.loaded = "true";
      highlight();
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
        message(`Saved global revision ${loadedRevision}`);
      } else if (action === "evaluate") {
        renderRun(await request("/api/assertions/evaluate", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ source: source.value, expected_revision: loadedRevision }) }));
        await load();
      } else {
        renderSummary(await request("/api/assertions/source"));
      }
    } catch (error) { message(error.message, "error"); }
    finally { pending = false; actions(); }
  }

  for (const control of [source, enabled, interval]) control.addEventListener("input", () => { highlight(); actions(); });
  source.addEventListener("scroll", highlight);
  source.addEventListener("keydown", (event) => {
    if (event.key === "Tab") { event.preventDefault(); source.setRangeText("  ", source.selectionStart, source.selectionEnd, "end"); highlight(); actions(); }
  });
  element("assertion-save").addEventListener("click", () => command("save"));
  element("assertion-evaluate").addEventListener("click", () => command("evaluate"));
  element("assertion-read-source").addEventListener("click", () => command("read"));
  element("assertion-reload").addEventListener("click", () => { if (!dirty() || window.confirm("Discard unsaved assertion changes?")) load(true); });
  const navigation = () => {
    clearInterval(timer);
    if (active()) { load(); timer = setInterval(() => { if (!pending) load(); }, 5000); }
  };
  window.addEventListener("hashchange", navigation);
  actions(); navigation();
})();