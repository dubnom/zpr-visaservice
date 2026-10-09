(() => {
  const byId = id => document.getElementById(id);
  let identity = null;
  let csrf = "";
  let records = [];
  let selected = null;
  let capabilities = null;
  let nextOffset = -1;
  let generation = 0;
  let controller = null;
  let busy = false;
  let uncertain = false;
  let sortKey = "revision";
  let descending = true;
  const permissions = permission => identity?.permissions?.includes(permission);
  const sameAuthor = record => identity?.issuer === record.proposal.author.issuer && identity?.subject === record.proposal.author.subject;
  function status(message, error = false) {
    byId("status").textContent = message;
    byId("status").classList.toggle("error", error);
  }
  function clearSession() {
    ++generation; controller?.abort();
    identity = null; csrf = ""; selected = null; records = []; capabilities = null;
    byId("identity").textContent = "";
    byId("workspace").disabled = true;
    byId("requests").replaceChildren();
    byId("review").hidden = true;
    for (const id of ["before", "after", "diff", "metadata", "audit"]) byId(id).replaceChildren();
    byId("login").hidden = false;
  }
  async function request(path, { method = "GET", body, signal } = {}) {
    const headers = { Accept: "application/json" };
    if (body) headers["Content-Type"] = "application/json";
    if (method !== "GET" && csrf) headers["X-ZPR-CSRF"] = csrf;
    const response = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : undefined,
      signal, credentials: "same-origin", cache: "no-store", redirect: "error" });
    if (response.status === 401 || response.status === 403) {
      clearSession();
      throw new Error("Your operator session or permissions changed. Sign in again.");
    }
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `Request failed (HTTP ${response.status}).`);
    return data;
  }
  function organization() {
    const value = byId("organization").value.trim();
    if (!/^[a-zA-Z0-9_-]{1,128}$/.test(value)) throw new Error("Enter an explicit organization ID.");
    return value;
  }
  function recordValid(record) {
    const proposal = record?.proposal;
    return record && typeof record.id === "string" && typeof record.digest === "string" &&
      /^[a-f0-9]{64}$/.test(record.digest) && Number.isSafeInteger(record.version) &&
      ["submitted", "approved", "rejected", "cancelled"].includes(record.state) && proposal &&
      ["policy", "gateway"].includes(proposal.kind) && typeof proposal.target === "string" &&
      Number.isSafeInteger(proposal.revision) && Number.isSafeInteger(proposal.base_revision) &&
      typeof proposal.after === "string" && typeof proposal.before === "string" &&
      typeof proposal.validation === "string" && typeof proposal.impact === "string" &&
      typeof proposal.reason === "string" && typeof proposal.author?.issuer === "string" &&
      typeof proposal.author?.subject === "string" && Array.isArray(record.events) &&
      record.events.every(event => typeof event.action === "string" && typeof event.reason === "string" &&
        typeof event.actor?.issuer === "string" && typeof event.actor?.subject === "string");
  }
  function render() {
    const tbody = byId("requests"); tbody.replaceChildren();
    const rows = [...records].sort((a, b) => {
      const left = a.proposal[sortKey] ?? a[sortKey];
      const right = b.proposal[sortKey] ?? b[sortKey];
      const comparison = sortKey === "revision" ? left - right : String(left).localeCompare(String(right));
      return descending ? -comparison : comparison;
    });
    for (const record of rows) {
      const tr = document.createElement("tr");
      for (const value of [record.proposal.kind, record.proposal.target, record.proposal.revision,
        record.state, record.proposal.author.name || record.proposal.author.subject, record.proposal.submitted_at]) {
        const td = document.createElement("td"); td.textContent = String(value); tr.append(td);
      }
      const td = document.createElement("td");
      const button = document.createElement("button"); button.type = "button"; button.textContent = "Review";
      button.disabled = busy;
      button.addEventListener("click", () => { selected = record; renderSelected(); });
      td.append(button); tr.append(td); tbody.append(tr);
    }
    if (!rows.length) {
      const tr = document.createElement("tr"); const td = document.createElement("td");
      td.colSpan = 7; td.textContent = "No change requests for this organization."; tr.append(td); tbody.append(tr);
    }
    byId("more").hidden = nextOffset < 0;
    renderSelected();
  }
  function diff(before, after) {
    const oldLines = before.split("\n"); const newLines = after.split("\n");
    let prefix = 0; let suffix = 0;
    while (prefix < oldLines.length && prefix < newLines.length && oldLines[prefix] === newLines[prefix]) prefix++;
    while (suffix < oldLines.length - prefix && suffix < newLines.length - prefix &&
      oldLines[oldLines.length - 1 - suffix] === newLines[newLines.length - 1 - suffix]) suffix++;
    if (prefix === oldLines.length && prefix === newLines.length) return "No source differences.";
    return ["--- previous saved revision", "+++ submitted revision",
      ...oldLines.slice(0, prefix).map(line => " " + line),
      ...oldLines.slice(prefix, oldLines.length - suffix).map(line => "-" + line),
      ...newLines.slice(prefix, newLines.length - suffix).map(line => "+" + line),
      ...newLines.slice(newLines.length - suffix).map(line => " " + line)].join("\n");
  }
  function renderSelected() {
    const panel = byId("review"); panel.hidden = !selected;
    if (!selected) return;
    const p = selected.proposal;
    byId("review-heading").textContent = `${p.kind}: ${p.target} revision ${p.revision}`;
    const metadata = byId("metadata"); metadata.replaceChildren();
    for (const [label, value] of Object.entries({ State: selected.state, Organization: p.organization,
      "Request ID": selected.id, "Revision digest": selected.digest, "Base saved revision": p.base_revision,
      Reason: p.reason, Validation: p.validation, Impact: p.impact })) {
      const dt = document.createElement("dt"); dt.textContent = label;
      const dd = document.createElement("dd"); dd.textContent = String(value); metadata.append(dt, dd);
    }
    byId("before").textContent = p.before || "(No preceding saved revision)";
    byId("after").textContent = p.after; byId("diff").textContent = diff(p.before, p.after);
    const audit = byId("audit"); audit.replaceChildren();
    for (const event of selected.events) {
      const li = document.createElement("li");
      li.textContent = `${event.at}: ${event.action} by ${event.actor.name || event.actor.subject} (${event.actor.issuer}) — ${event.reason}`;
      audit.append(li);
    }
    const pending = selected.state === "submitted" && !busy && !uncertain;
    for (const button of byId("decision").querySelectorAll("button")) {
      const action = button.dataset.action;
      button.disabled = !pending || (action === "cancel" ? !permissions("change.submit") || !sameAuthor(selected)
        : !permissions("change.review") || (action === "approve" && sameAuthor(selected) && !capabilities?.allow_self_approval));
    }
    byId("decision-note").textContent = uncertain ? "Outcome unknown. Reload the queue and reconcile before another decision."
      : sameAuthor(selected) && !capabilities?.allow_self_approval ? "Approval requires a different operator; single-operator approval is not enabled."
      : "Approval does not apply this revision.";
  }
  async function load(offset = 0) {
    if (busy) return;
    const token = ++generation;
    controller?.abort(); controller = new AbortController();
    const org = organization();
    status("Loading change queue");
    try {
      const [caps, page] = await Promise.all([
        request(`/api/change-review/capabilities?organization=${encodeURIComponent(org)}`, { signal: controller.signal }),
        request(`/api/change-review/requests?organization=${encodeURIComponent(org)}&offset=${offset}`, { signal: controller.signal }),
      ]);
      if (token !== generation) return;
      if (!page.requests?.every(recordValid) || typeof caps.allow_self_approval !== "boolean" ||
          caps.application_enabled !== false || !Number.isInteger(page.next_offset)) throw new Error("Invalid review queue response.");
      if (page.requests.some(record => record.proposal.organization !== org)) throw new Error("Review organization mismatch.");
      capabilities = caps; nextOffset = page.next_offset;
      records = offset ? [...new Map([...records, ...page.requests].map(record => [record.id, record])).values()] : page.requests;
      selected = records.find(record => record.id === selected?.id) || null;
      byId("capabilities").textContent = caps.allow_self_approval
        ? "Single-operator self-approval explicitly enabled. Application disabled." : "A different operator must approve. Application disabled.";
      uncertain = false;
      for (const button of byId("submission").querySelectorAll("button")) button.disabled = !permissions("change.submit");
      render(); status("Queue refreshed. Approval is separate from application.");
    } catch (error) {
      if (token !== generation || error.name === "AbortError") return;
      status(`Unable to refresh queue: ${error.message} Existing data may be stale.`, true);
    }
  }
  async function mutate(path, body, expectedStatus) {
    if (busy || uncertain) return;
    busy = true; controller?.abort(); ++generation; renderSelected();
    const token = generation;
    const mutationController = new AbortController();
    controller = mutationController;
    const timer = setTimeout(() => mutationController.abort(), 120000);
    byId("organization").disabled = true;
    for (const button of byId("requests").querySelectorAll("button")) button.disabled = true;
    for (const button of byId("submission").querySelectorAll("button")) button.disabled = true;
    status("Recording change review");
    try {
      const record = await request(path, { method: "POST", body, signal: mutationController.signal });
      if (token !== generation) return;
      if (!recordValid(record) || record.proposal.organization !== body.organization ||
          (body.kind && (record.proposal.kind !== body.kind || record.proposal.target !== body.target || record.proposal.revision !== body.revision)) ||
          (body.digest && (record.id !== selected?.id || record.digest !== body.digest || record.version !== body.expected_version + 1)) ||
          (expectedStatus && record.state !== expectedStatus)) throw new Error("Invalid mutation response.");
      records = [record, ...records.filter(item => item.id !== record.id)]; selected = record;
      status(`Recorded ${record.state}. Runtime unchanged.`); render();
    } catch (error) {
      // Reconcile even an HTTP rejection: do not infer a failed mutation from a lost/malformed response.
      uncertain = true;
      status(`No automatic retry. Outcome must be reconciled by loading the queue: ${error.message}`, true);
    } finally {
      clearTimeout(timer);
      busy = false; renderSelected();
      byId("organization").disabled = false;
      for (const button of byId("requests").querySelectorAll("button")) button.disabled = false;
      for (const button of byId("submission").querySelectorAll("button")) button.disabled = uncertain || !permissions("change.submit");
    }
  }
  byId("refresh").addEventListener("click", () => load().catch(error => status(error.message, true)));
  byId("more").addEventListener("click", () => load(nextOffset).catch(error => status(error.message, true)));
  byId("organization").addEventListener("input", () => {
    controller?.abort(); ++generation; records = []; selected = null; nextOffset = -1; render();
  });
  byId("submission").addEventListener("submit", event => {
    event.preventDefault();
    try {
      const revision = Number(byId("revision").value);
      if (!Number.isSafeInteger(revision) || revision < 1) throw new Error("Enter a saved revision number.");
      void mutate("/api/change-review/requests", { organization: organization(), kind: byId("kind").value,
        target: byId("target").value.trim(), revision, reason: byId("submit-reason").value.trim() }, "submitted");
    } catch (error) { status(error.message, true); }
  });
  byId("decision").addEventListener("submit", event => {
    event.preventDefault();
    if (!selected || !event.submitter || event.submitter.disabled) return;
    const action = event.submitter.dataset.action;
    if (!confirm(`${action} this exact saved revision? Runtime will not change.`)) return;
    void mutate(`/api/change-review/requests/${encodeURIComponent(selected.id)}/${action}`,
      { organization: selected.proposal.organization, digest: selected.digest, expected_version: selected.version,
        reason: byId("decision-reason").value.trim() }, { approve: "approved", reject: "rejected", cancel: "cancelled" }[action]);
  });
  for (const button of document.querySelectorAll("[data-sort]")) button.addEventListener("click", () => {
    descending = sortKey === button.dataset.sort ? !descending : false; sortKey = button.dataset.sort; render();
  });
  window.addEventListener("pagehide", () => { controller?.abort(); ++generation; clearSession(); });
  (async () => {
    try {
      const session = await request("/auth/operator/session");
      if (!session.identity?.issuer || !session.identity?.subject || !Array.isArray(session.identity.permissions) ||
          !Array.isArray(session.identity.organizations) || typeof session.csrf !== "string" || !session.csrf) throw new Error("Invalid operator session.");
      identity = session.identity; csrf = session.csrf;
      byId("identity").textContent = identity.display_name || identity.email || "Signed in";
      byId("workspace").disabled = false; byId("login").hidden = true;
      const exact = identity.organizations.filter(org => org !== "*");
      if (exact.length === 1) byId("organization").value = exact[0];
      status("Choose an explicit organization and load the review queue.");
    } catch (error) { clearSession(); status(`Sign in required: ${error.message}`, true); }
  })();
})();
