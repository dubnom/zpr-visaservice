(() => {
  const el = (id) => document.getElementById(`subject-${id}`);
  const value = (id) => el(id).value;
  const storageKey = "zpr.subject-link.pending.v1";
  let identity = null, epoch = 0, busy = false, capture = null, review = null, pending = null;
  let registryUncertain = false;
  let pendingInvalid = false;
  const allowed = (permission) => !!identity?.permissions.includes(permission) &&
    (identity.organizations.includes(value("organization")) || identity.organizations.includes("*"));
  const samePrincipal = (a, b) => a?.issuer === b?.issuer && a?.subject === b?.subject;
  const status = (message, error = false) => {
    document.getElementById("status").textContent = message;
    document.getElementById("status").classList.toggle("error", error);
  };
  function update() {
    el("workspace").disabled = !identity || busy;
    for (const id of ["registry-read", "registry-history", "context-load", "review-read"]) el(id).disabled = !allowed("subject-link.read");
    el("registry-save").disabled = registryUncertain || !allowed("subject-link.registry.manage");
    el("propose").disabled = !capture || !!pending || pendingInvalid || !allowed("subject-link.propose");
    el("approve").disabled = !review || !!pending || pendingInvalid || !allowed("subject-link.approve") ||
      review.state !== "submitted" || samePrincipal(identity, review.proposal.author);
    el("pending").hidden = (!pending && !pendingInvalid) || !identity;
    const owned = pending && samePrincipal(identity, pending.principal) && pending.organization === value("organization");
    el("pending-note").textContent = pendingInvalid ? "Saved retry data is invalid. Check the original outcome before discarding it or creating another review." : owned
      ? `Outcome unconfirmed. Retry key: ${pending.key}. The exact original body is saved in this tab; retry is explicit, never automatic.`
      : "A saved request belongs to another identity or organization. Sign in with its original scope before retrying; do not assume it failed.";
    el("retry").disabled = pendingInvalid || !owned || !allowed(pending?.permission);
  }
  function invalidate() {
    ++epoch;
    capture = null;
    review = null;
    el("context-evidence").textContent = "";
    el("review-evidence").textContent = "";
    update();
  }
  function record(value, organization) {
    if (!value || !/^[A-Za-z0-9_-]{1,128}$/.test(value.id) || !/^[0-9a-f]{64}$/.test(value.digest) ||
        !["submitted", "review-approved"].includes(value.state) || typeof value.version !== "string" ||
        value.proposal?.organization_id !== organization || !value.proposal.author?.issuer || !value.proposal.author?.subject ||
        !/^[0-9a-f]{64}$/.test(value.proposal.registry_revision) || value.proposal.root?.subject_kind !== "device" ||
        typeof value.proposal.root.subject_id !== "string" || !value.proposal.root.subject_id ||
        typeof value.proposal.root.authority_id !== "string" || typeof value.proposal.root.authority_version !== "string" ||
        value.proposal.target?.selector_kind !== "ldap_entry_uuid" || typeof value.proposal.target.source_id !== "string" ||
        typeof value.proposal.target.source_version !== "string" || typeof value.proposal.target.subject_id !== "string" ||
        !Array.isArray(value.proposal.enrichment_keys) ||
        !Array.isArray(value.events)) throw new Error("Invalid scoped review response; outcome must be checked.");
    return value;
  }
  async function api(path, method = "GET", body, key) {
    const headers = { "Content-Type": "application/json" };
    if (key) headers["Idempotency-Key"] = key;
    const response = await window.zprOperatorFetch(path, {
      method, body, headers, credentials: "same-origin", cache: "no-store", redirect: "error",
    });
    const result = await response.json();
    if (!response.ok) throw new Error(typeof result.error === "string" ? result.error : `Request failed (HTTP ${response.status}).`);
    return result;
  }
  async function run(action) {
    if (busy) return;
    busy = true;
    const current = epoch;
    update();
    try {
      await action(() => current === epoch);
    } catch (error) {
      if (current === epoch) status(error.message, true);
    } finally {
      busy = false;
      update();
    }
  }
  function loadPending() {
    pendingInvalid = true;
    const saved = sessionStorage.getItem(storageKey);
    pending = saved === null ? null : JSON.parse(saved);
    if (saved !== null && (!pending || typeof pending !== "object" || Array.isArray(pending) ||
        pending.version !== 1 || typeof pending.body !== "string" || pending.body.length > 8192 ||
        typeof pending.key !== "string" || typeof pending.organization !== "string" ||
        !/^[A-Za-z0-9_-]{1,128}$/.test(pending.key) || !/^[A-Za-z0-9_-]{1,128}$/.test(pending.organization) ||
        typeof pending.principal?.issuer !== "string" || typeof pending.principal?.subject !== "string" ||
        !(pending.path === "/api/subject-link-reviews/proposals" && pending.permission === "subject-link.propose" ||
          /^\/api\/subject-link-reviews\/[A-Za-z0-9_-]{1,128}\/approve$/.test(pending.path) && pending.permission === "subject-link.approve") ||
        JSON.parse(pending.body).organization_id !== pending.organization)) {
      pending = null;
      throw new Error("Saved retry data is invalid. Do not submit again until the original outcome has been checked.");
    }
    pendingInvalid = false;
  }
  function savePending(path, body, permission) {
    if (pending || sessionStorage.getItem(storageKey) !== null) throw new Error("Resolve the saved review request before starting another.");
    const next = { version: 1, path, body: JSON.stringify(body), permission, organization: value("organization"),
      principal: { issuer: identity.issuer, subject: identity.subject }, key: crypto.randomUUID().replaceAll("-", "") };
    if (new TextEncoder().encode(next.body).length > 8192) throw new Error("Review request exceeds the 8192-byte limit. Shorten its exact input before submitting.");
    sessionStorage.setItem(storageKey, JSON.stringify(next));
    pending = next;
  }
  async function deliver(current) {
    const request = pending;
    if (pendingInvalid || !request || !samePrincipal(identity, request.principal) || !allowed(request.permission) || request.organization !== value("organization")) {
      throw new Error("The original authorized identity and organization are required for this retry.");
    }
    const result = record(await api(request.path, "POST", request.body, request.key), request.organization);
    if (!current()) return;
    if (request.permission === "subject-link.approve") {
      const original = JSON.parse(request.body);
      if (result.id !== request.path.split("/")[3] || result.digest !== original.digest || result.state !== "review-approved" ||
          !samePrincipal(result.events.at(-1)?.actor, request.principal)) {
        throw new Error("Approval response differs from the reviewed request. Check its original outcome.");
      }
    } else {
      const original = JSON.parse(request.body), proposal = result.proposal;
      if (result.state !== "submitted" || result.version !== "1" ||
          !samePrincipal(proposal.author, request.principal) || proposal.registry_revision !== original.registry_revision ||
          Object.entries(original.root).some(([key, value]) => proposal.root[key] !== value) ||
          Object.entries(original.target).some(([key, value]) => proposal.target[key] !== value) ||
          JSON.stringify(proposal.enrichment_keys) !== JSON.stringify(original.enrichment_keys) ||
          proposal.reason !== original.reason || Date.parse(proposal.not_before) !== Date.parse(original.not_before) ||
          Date.parse(proposal.expires_at) !== Date.parse(original.expires_at)) {
        throw new Error("Submission response differs from the original candidate. Check its original outcome.");
      }
    }
    sessionStorage.removeItem(storageKey);
    pending = null;
    review = result;
    el("review-id").value = result.id;
    el("review-evidence").textContent = JSON.stringify(result, null, 2);
    status(`${result.state === "review-approved" ? "Independent review recorded" : "Candidate submitted"}: ${result.id}. Review evidence only; no active binding.`);
  }
  window.addEventListener("operator-session-cleared", () => {
    identity = null;
    pending = null;
    pendingInvalid = false;
    invalidate();
    el("registry-evidence").textContent = "";
    status("Session cleared. Reload evidence after signing in; saved unconfirmed requests are not retried automatically.");
  });
  window.addEventListener("operator-session-ready", (event) => {
    identity = event.detail.identity;
    invalidate();
    if (!value("organization") && identity.organizations.length === 1 && identity.organizations[0] !== "*") {
      el("organization").value = identity.organizations[0];
    }
    try {
      loadPending();
      status(pending ? "An unconfirmed request is saved in this tab. Check its scope and retry explicitly." : "Session ready. Select exact organization and operation.");
    } catch (error) {
      status(error.message, true);
    }
    update();
  });
  el("organization").addEventListener("input", () => {
    registryUncertain = false;
    el("registry-evidence").textContent = "";
    invalidate();
  });
  el("context-form").addEventListener("input", invalidate);
  el("review-id").addEventListener("input", () => {
    review = null;
    el("review-evidence").textContent = "";
    ++epoch;
    update();
  });
  el("context-form").addEventListener("submit", (event) => {
    event.preventDefault();
    void run(async (current) => {
      capture = null;
      el("context-evidence").textContent = "";
      const root = { authority_id: value("authority"), authority_version: value("authority-version"), subject_kind: "device", subject_id: value("device") };
      const target = { source_id: value("source"), source_version: value("source-version"), selector_kind: "ldap_entry_uuid", subject_id: value("uuid") };
      const result = await api("/api/subject-link-registry/context", "POST", JSON.stringify({ organization_id: value("organization"), root, target }));
      if (!current()) return;
      if (!/^[0-9a-f]{64}$/.test(result.revision) || result.authority?.organization_id !== value("organization") ||
          result.source?.organization_id !== value("organization") || result.authority.authority_id !== root.authority_id ||
          result.authority.version !== root.authority_version || result.source.source_id !== target.source_id ||
          result.source.version !== target.source_version || !result.authority.config?.enabled || !result.source.config?.enabled) {
        throw new Error("Invalid coherent registry-context response.");
      }
      capture = { revision: result.revision, root, target };
      el("context-evidence").textContent = JSON.stringify(result, null, 2);
      status("Current registry context captured. No directory presence or authentication was established by this read.");
    });
  });
  el("proposal-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!capture || !allowed("subject-link.propose")) return;
    void run(async (current) => {
      const body = { organization_id: value("organization"), registry_revision: capture.revision, root: capture.root, target: capture.target,
        enrichment_keys: value("keys").split(",").map((key) => key.trim()), not_before: new Date(value("not-before")).toISOString(),
        expires_at: new Date(value("expires")).toISOString(), reason: value("proposal-reason") };
      if (!window.confirm(`Submit review-only candidate for ${body.organization_id}?\nDevice: ${body.root.subject_id}\nLDAP UUID: ${body.target.subject_id}\nRegistry: ${body.registry_revision}\nNo binding will be activated.`)) return;
      savePending("/api/subject-link-reviews/proposals", body, "subject-link.propose");
      await deliver(current);
    });
  });
  el("review-form").addEventListener("submit", (event) => {
    event.preventDefault();
    void run(async (current) => {
      review = null;
      el("review-evidence").textContent = "";
      const id = value("review-id");
      if (!/^[A-Za-z0-9_-]{1,128}$/.test(id)) throw new Error("Enter an exact review ID.");
      const result = record(await api(`/api/subject-link-reviews/${id}?organization_id=${encodeURIComponent(value("organization"))}`), value("organization"));
      if (!current()) return;
      if (result.id !== id) throw new Error("Review response has the wrong ID.");
      review = result;
      el("review-evidence").textContent = JSON.stringify(result, null, 2);
      status("Immutable review evidence loaded. Check the exact root, target, scope, validity, digest and audit before deciding.");
    });
  });
  el("approval-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!review || !allowed("subject-link.approve") || samePrincipal(identity, review.proposal.author)) return;
    void run(async (current) => {
      const body = { organization_id: value("organization"), digest: review.digest, version: review.version,
        registry_revision: review.proposal.registry_revision, reason: value("approval-reason") };
      if (!window.confirm(`Approve review evidence ${review.id}?\nDigest: ${review.digest}\nDevice: ${review.proposal.root.subject_id}\nLDAP UUID: ${review.proposal.target.subject_id}\nNo binding will be activated.`)) return;
      savePending(`/api/subject-link-reviews/${review.id}/approve`, body, "subject-link.approve");
      review = null;
      await deliver(current);
    });
  });
  el("retry").addEventListener("click", () => void run(deliver));
  el("discard").addEventListener("click", () => {
    if (!window.confirm("Discard only this tab's saved retry after checking the original operation's outcome? This does not cancel or undo a server-side review.")) return;
    try {
      sessionStorage.removeItem(storageKey);
      pending = null;
      pendingInvalid = false;
      invalidate();
      status("Local retry discarded. No server-side state was changed.");
    } catch (error) { status(error.message, true); }
  });
  function registryPath(history = false) {
    const kind = value("registry-kind"), id = value("registry-id"), version = value("history-version");
    if (!["authorities", "sources"].includes(kind) || !/^[A-Za-z0-9_-]{1,128}$/.test(id) ||
        history && !/^[1-9][0-9]{0,19}$/.test(version)) throw new Error("Enter an exact registration ID and, for audit, a canonical version.");
    return `/api/subject-link-registry/${kind}/${id}${history ? `/versions/${version}` : ""}`;
  }
  async function readRegistry(history) {
    await run(async (current) => {
      el("registry-evidence").textContent = "";
      const result = await api(`${registryPath(history)}?organization_id=${encodeURIComponent(value("organization"))}`);
      if (!current()) return;
      const registration = history ? result.authority ?? result.source : result;
      if (registration?.organization_id !== value("organization") || typeof registration.version !== "string" || !registration.config ||
          (registration.authority_id ?? registration.source_id) !== value("registry-id")) {
        throw new Error("Invalid scoped registration evidence.");
      }
      el("registry-evidence").textContent = JSON.stringify(result, null, 2);
      if (!history) {
        registryUncertain = false;
        el("registry-version").value = registration.version;
        el("registry-config").value = JSON.stringify(registration.config, null, 2);
      }
      status(history ? "Historical audit loaded; not evidence of current enablement." : "Current registration loaded. Review its version and config before any edit.");
    });
  }
  el("registry-read").addEventListener("click", () => void readRegistry(false));
  el("registry-history").addEventListener("click", () => void readRegistry(true));
  for (const id of ["registry-kind", "registry-id"]) el(id).addEventListener("input", () => {
    ++epoch;
    registryUncertain = false;
    el("registry-evidence").textContent = "";
    el("registry-version").value = "";
    el("history-version").value = "";
    el("registry-config").value = "";
    el("registry-reason").value = "";
    update();
  });
  el("registry-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!allowed("subject-link.registry.manage") || registryUncertain) return;
    void run(async (current) => {
      const path = registryPath(), config = JSON.parse(value("registry-config"));
      const body = { organization_id: value("organization"), expected_version: value("registry-version"), config, reason: value("registry-reason") };
      if (!window.confirm(`Edit ${path} in ${body.organization_id} at ${body.expected_version || "create-only"}?\n${JSON.stringify(config)}\nThis changes registration intent only, not provider trust or active bindings.`)) return;
      registryUncertain = true;
      el("registry-evidence").textContent = "";
      capture = null;
      review = null;
      el("context-evidence").textContent = "";
      el("review-evidence").textContent = "";
      const result = await api(path, "PUT", JSON.stringify(body));
      if (!current()) return;
      if (result.organization_id !== body.organization_id || !result.config || typeof result.version !== "string" ||
          (result.authority_id ?? result.source_id) !== value("registry-id")) {
        throw new Error("Invalid edit response. Read current state and audit before another edit.");
      }
      registryUncertain = false;
      el("registry-version").value = result.version;
      el("registry-evidence").textContent = JSON.stringify(result, null, 2);
      status(`Registration version ${result.version} recorded. No review or active binding was created.`);
    });
  });
  update();
})();
