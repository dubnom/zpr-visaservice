(() => {
  const el = (id) => document.getElementById(`publication-${id}`);
  const value = (id) => el(id).value;
  const storageKey = "zpr.subject-link-publication.pending.v1";
  const idPattern = /^[A-Za-z0-9_-]{1,128}$/, sequencePattern = /^(0|[1-9][0-9]{0,19})$/, digestPattern = /^[0-9a-f]{64}$/;
  let identity = null, epoch = 0, busy = false, state = null, candidate = null, pending = null, pendingInvalid = false;
  const allowed = (permission) => !!identity?.permissions.includes(permission) &&
    (identity.organizations.includes(value("organization")) || identity.organizations.includes("*"));
  const samePrincipal = (a, b) => a?.issuer === b?.issuer && a?.subject === b?.subject;
  const status = (message, error = false) => {
    document.getElementById("status").textContent = message;
    document.getElementById("status").classList.toggle("error", error);
  };
  const scopeValid = () => idPattern.test(value("organization")) && idPattern.test(value("recipient")) && idPattern.test(value("epoch"));
  const base = () => {
    if (!scopeValid()) throw new Error("Enter exact organization, VS recipient and epoch identifiers.");
    return `/api/subject-link-publications/${value("recipient")}/${value("epoch")}`;
  };
  const query = () => `?organization_id=${encodeURIComponent(value("organization"))}`;
  const next = (sequence) => (BigInt(sequence) + 1n).toString();
  const sameScope = (scope, request) => scope?.organization_id === request.organization &&
    scope.vs_id === request.recipient && scope.epoch === request.epoch;
  const currentScope = () => ({ organization: value("organization"), recipient: value("recipient"), epoch: value("epoch") });

  function update() {
    const blocked = !!pending || pendingInvalid;
    el("workspace").disabled = !identity || busy;
    for (const id of ["read", "history-read"]) el(id).disabled = !scopeValid() || !allowed("subject-link.read");
    el("candidate-read").disabled = !state?.current_candidate || !allowed("subject-link.read");
    el("provision-save").disabled = blocked || !scopeValid() || !allowed("subject-link.publish");
    el("intent-save").disabled = blocked || !state || !allowed("subject-link.publish");
    el("capture-save").disabled = blocked || !state || !allowed("subject-link.snapshot.capture");
    el("sign-save").disabled = blocked || !candidate || candidate.sequence !== state?.current_candidate?.sequence ||
      !!state.current_candidate.signed_key_id || !state.signing_key_id || !allowed("subject-link.snapshot.sign");
    const promote = value("action") === "promote";
    el("digest-label").hidden = !promote;
    el("review-version-label").hidden = !promote;
    el("binding-version-label").hidden = promote;
    el("pending").hidden = (!pending && !pendingInvalid) || !identity;
    const owned = pending && samePrincipal(identity, pending.principal) && sameScope(
      { organization_id: value("organization"), vs_id: value("recipient"), epoch: value("epoch") }, pending);
    el("pending-note").textContent = pendingInvalid ? "Saved retry data is invalid. Read the stream state before discarding it or starting another change." : owned
      ? `Outcome unconfirmed for ${pending.path}. Retry key: ${pending.key}. The exact original body is saved in this tab; retry is explicit, never automatic.`
      : "A saved request belongs to another identity or recipient stream. Restore its original scope before retrying; do not assume it failed.";
    el("retry").disabled = pendingInvalid || !owned || !allowed(pending?.permission);
  }
  function invalidate() {
    ++epoch;
    state = null;
    candidate = null;
    for (const id of ["sequence", "current", "key", "receipt"]) el(id).textContent = "Not read";
    for (const id of ["state-evidence", "candidate-evidence", "candidate-check", "history-evidence"]) el(id).textContent = "";
    update();
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
  function publication(result, request) {
    if (!sameScope(result?.scope, request) || !sequencePattern.test(result.sequence) || !Array.isArray(result.bindings) ||
        result.bindings.some((binding) => !idPattern.test(binding.binding_id) || !sequencePattern.test(binding.version) ||
          typeof binding.state !== "string" || !digestPattern.test(binding.review_digest))) {
      throw new Error("Invalid scoped publication response; read the stream state.");
    }
    return result;
  }
  function validReceipt(receipt, request) {
    if (receipt === null) return true;
    return typeof receipt === "object" && sameScope(receipt.scope, request) &&
      sequencePattern.test(receipt.sequence) && receipt.sequence !== "0" && digestPattern.test(receipt.digest) &&
      typeof receipt.key_id === "string" && receipt.key_id !== "" && digestPattern.test(receipt.client_spki_sha256) && /^[0-9]{1,20}$/.test(receipt.received_at);
  }
  function showState(result) {
    const request = currentScope();
    publication(result.publication, request);
    const current = result.current_candidate;
    if (typeof result.signing_key_id !== "string" || current !== null && (!sequencePattern.test(current?.sequence) ||
        current.sequence !== result.publication.sequence || !digestPattern.test(current.digest) || typeof current.signed_key_id !== "string") ||
        !validReceipt(result.latest_receipt, request)) {
      throw new Error("Invalid stream state response.");
    }
    state = result;
    candidate = null;
    el("candidate-evidence").textContent = "";
    el("candidate-check").textContent = "";
    el("sequence").textContent = result.publication.sequence;
    el("current").textContent = current ? `Sequence ${current.sequence}, digest ${current.digest}, ${current.signed_key_id ? `signed by ${current.signed_key_id}` : "unsigned"}` : "None at the current sequence";
    el("key").textContent = result.signing_key_id || "Signing not configured";
    const receipt = result.latest_receipt;
    el("receipt").textContent = receipt ? `Sequence ${receipt.sequence}, digest ${receipt.digest}, key ${receipt.key_id}, reported ${new Date(Number(receipt.received_at) * 1000).toISOString()} by client SPKI ${receipt.client_spki_sha256} (recipient report, not enforcement)` : "No receipt reported";
    el("state-evidence").textContent = JSON.stringify(result, null, 2);
  }
  function loadPending() {
    pendingInvalid = true;
    const saved = sessionStorage.getItem(storageKey);
    pending = saved === null ? null : JSON.parse(saved);
    if (saved !== null) {
      const route = typeof pending?.path === "string" && pending.path.match(/^\/api\/subject-link-publications\/([A-Za-z0-9_-]{1,128})\/([A-Za-z0-9_-]{1,128})\/(intents|candidates|candidates\/([1-9][0-9]{0,19})\/sign)$/);
      const permission = route && { intents: "subject-link.publish", candidates: "subject-link.snapshot.capture" }[route[3]] || "subject-link.snapshot.sign";
      if (!route || typeof pending !== "object" || Array.isArray(pending) || pending.version !== 1 ||
          typeof pending.body !== "string" || pending.body.length > 8192 || !idPattern.test(pending.key) ||
          !idPattern.test(pending.organization) || pending.recipient !== route[1] || pending.epoch !== route[2] ||
          pending.permission !== permission || typeof pending.principal?.issuer !== "string" ||
          typeof pending.principal?.subject !== "string" || JSON.parse(pending.body).organization_id !== pending.organization) {
        pending = null;
        throw new Error("Saved retry data is invalid. Do not change the stream again until the original outcome has been checked.");
      }
    }
    pendingInvalid = false;
  }
  function savePending(path, body, permission) {
    if (pending || sessionStorage.getItem(storageKey) !== null) throw new Error("Resolve the saved publication request before starting another.");
    const request = { version: 1, path, body: JSON.stringify(body), permission, ...currentScope(),
      principal: { issuer: identity.issuer, subject: identity.subject }, key: crypto.randomUUID().replaceAll("-", "") };
    if (new TextEncoder().encode(request.body).length > 8192) throw new Error("Request exceeds the 8192-byte limit. Shorten its reason.");
    sessionStorage.setItem(storageKey, JSON.stringify(request));
    pending = request;
  }
  async function deliver(current) {
    const request = pending;
    if (pendingInvalid || !request || !samePrincipal(identity, request.principal) || !allowed(request.permission) ||
        !sameScope({ organization_id: value("organization"), vs_id: value("recipient"), epoch: value("epoch") }, request)) {
      throw new Error("The original authorized identity and recipient stream are required for this retry.");
    }
    const result = await api(request.path, "POST", request.body, request.key);
    if (!current()) return;
    const original = JSON.parse(request.body);
    let message;
    if (request.path.endsWith("/intents")) {
      publication(result, request);
      const binding = result.bindings.find((item) => item.binding_id === original.review_id);
      if (result.sequence !== next(original.expected_sequence) || !binding ||
          binding.state !== (original.action === "promote" ? "publication-intent" : "revoked") ||
          original.action === "promote" && binding.review_digest !== original.review_digest) {
        throw new Error("Intent response differs from the requested change. Read the stream state.");
      }
      message = `${original.action === "promote" ? "Publication intent recorded" : "Publication intent revoked"} at sequence ${result.sequence}. Nothing was delivered.`;
    } else if (request.path.endsWith("/candidates")) {
      if (!sameScope(result?.scope, request) || result.sequence !== next(original.expected_sequence) ||
          typeof result.payload !== "string" || !digestPattern.test(result.digest)) {
        throw new Error("Capture response differs from the requested candidate. Read the stream state.");
      }
      message = `Unsigned candidate captured at sequence ${result.sequence} (digest ${result.digest}). Read and inspect it before signing.`;
    } else {
      if (!sameScope(result?.scope, request) || result.sequence !== request.path.split("/")[6] || result.digest !== original.digest ||
          !idPattern.test(result.key_id) || typeof result.envelope !== "string" || !samePrincipal(result.actor, request.principal)) {
        throw new Error("Signing response differs from the inspected candidate. Read the stream state.");
      }
      message = `Candidate ${result.sequence} signed with key ${result.key_id}. Retained evidence only; not delivered or enforced.`;
    }
    sessionStorage.removeItem(storageKey);
    pending = null;
    el("history-evidence").textContent = JSON.stringify(result, null, 2);
    status(message);
    state = null;
    candidate = null;
  }
  async function readState(current) {
    const result = await api(`${base()}${query()}`);
    if (!current()) return;
    showState(result);
    status(`Stream state read at sequence ${result.publication.sequence}. This is retained intent, not delivered or enforced state.`);
  }
  const decodeBase64 = (text) => Uint8Array.from(atob(text), (character) => character.charCodeAt(0));
  const hex = (buffer) => Array.from(new Uint8Array(buffer), (byte) => byte.toString(16).padStart(2, "0")).join("");
  const unix = (input) => {
    const seconds = Math.floor(new Date(input).getTime() / 1000);
    if (!Number.isSafeInteger(seconds) || seconds <= 0) throw new Error("Enter exact issued and expiry times.");
    return String(seconds);
  };

  window.addEventListener("operator-session-cleared", () => {
    identity = null;
    pending = null;
    pendingInvalid = false;
    invalidate();
    status("Session cleared. Read the stream again after signing in; saved unconfirmed requests are not retried automatically.");
  });
  window.addEventListener("operator-session-ready", (event) => {
    identity = event.detail.identity;
    invalidate();
    if (!value("organization") && identity.organizations.length === 1 && identity.organizations[0] !== "*") {
      el("organization").value = identity.organizations[0];
    }
    try {
      loadPending();
      if (pending) {
        for (const id of ["organization", "recipient", "epoch"]) if (!value(id)) el(id).value = pending[id];
      }
      status(pending ? "An unconfirmed request is saved in this tab. Check its scope and retry explicitly." : "Session ready. Enter the exact recipient stream.");
    } catch (error) {
      status(error.message, true);
    }
    update();
  });
  for (const id of ["organization", "recipient", "epoch"]) el(id).addEventListener("input", invalidate);
  el("action").addEventListener("change", update);
  el("read").addEventListener("click", () => void run(readState));
  el("candidate-read").addEventListener("click", () => void run(async (current) => {
    candidate = null;
    el("candidate-check").textContent = "";
    const expected = state.current_candidate;
    const result = await api(`${base()}/candidates/${expected.sequence}${query()}`);
    if (!current()) return;
    if (!sameScope(result?.scope, currentScope()) || result.sequence !== expected.sequence || result.digest !== expected.digest ||
        typeof result.payload !== "string") throw new Error("Candidate response differs from the current stream state.");
    const bytes = decodeBase64(result.payload);
    const computed = hex(await crypto.subtle.digest("SHA-256", bytes));
    if (!current()) return;
    if (computed !== result.digest) throw new Error("Candidate bytes do not match their digest. Do not sign.");
    let text;
    try {
      text = JSON.stringify(JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)), null, 2);
    } catch {
      throw new Error("Candidate payload is not readable JSON. Do not sign.");
    }
    candidate = { sequence: result.sequence, digest: computed };
    el("candidate-evidence").textContent = text;
    el("candidate-check").textContent = `Browser-computed SHA-256 ${computed} matches sequence ${result.sequence}.`;
    status("Candidate bytes verified. Inspect bindings, validity and recipient before signing.");
  }));
  el("provision-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!allowed("subject-link.publish")) return;
    void run(async (current) => {
      const path = `${base()}/provision`, body = { organization_id: value("organization"), reason: value("provision-reason") };
      if (!window.confirm(`Provision recipient stream ${value("recipient")} epoch ${value("epoch")} in ${body.organization_id}?\nThis is not retried automatically.`)) return;
      const result = publication(await api(path, "POST", JSON.stringify(body)), currentScope());
      if (!current()) return;
      if (result.sequence !== "0") throw new Error("Provisioning response is not a new stream. Read the stream state.");
      el("history-evidence").textContent = JSON.stringify(result, null, 2);
      status("Recipient stream provisioned at sequence 0. Read the stream state before changing intent.");
    });
  });
  el("intent-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!state || !allowed("subject-link.publish")) return;
    void run(async (current) => {
      const action = value("action"), id = value("review-id");
      if (!idPattern.test(id)) throw new Error("Enter an exact review ID.");
      const body = action === "promote"
        ? { organization_id: value("organization"), action, expected_sequence: state.publication.sequence, review_id: id,
          review_digest: value("review-digest"), review_version: value("review-version"), reason: value("intent-reason") }
        : { organization_id: value("organization"), action, expected_sequence: state.publication.sequence, review_id: id,
          binding_version: value("binding-version"), reason: value("intent-reason") };
      if (action === "promote" && (!digestPattern.test(body.review_digest) || !sequencePattern.test(body.review_version)) ||
          action === "revoke" && !sequencePattern.test(body.binding_version)) {
        throw new Error("Enter the exact approved digest and review version, or the binding version to revoke.");
      }
      if (!window.confirm(`${action === "promote" ? "Promote" : "Revoke"} review ${id} for ${value("recipient")} epoch ${value("epoch")} at sequence ${body.expected_sequence}?\nNothing will be delivered or enforced.`)) return;
      savePending(`${base()}/intents`, body, "subject-link.publish");
      await deliver(current);
    });
  });
  el("capture-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!state || !allowed("subject-link.snapshot.capture")) return;
    void run(async (current) => {
      const body = { organization_id: value("organization"), expected_sequence: state.publication.sequence,
        issued_at: unix(value("issued")), expires_at: unix(value("expires")), reason: value("capture-reason") };
      if (BigInt(body.expires_at) <= BigInt(body.issued_at)) throw new Error("Expiry must be after the issued time.");
      if (!window.confirm(`Capture an unsigned candidate for ${value("recipient")} epoch ${value("epoch")} after sequence ${body.expected_sequence}?\nValid ${new Date(body.issued_at * 1000).toISOString()} to ${new Date(body.expires_at * 1000).toISOString()}.`)) return;
      savePending(`${base()}/candidates`, body, "subject-link.snapshot.capture");
      await deliver(current);
    });
  });
  el("sign-form").addEventListener("submit", (event) => {
    event.preventDefault();
    if (!candidate || !allowed("subject-link.snapshot.sign")) return;
    void run(async (current) => {
      const body = { organization_id: value("organization"), digest: candidate.digest, reason: value("sign-reason") };
      if (!window.confirm(`Sign candidate ${candidate.sequence} with server key ${state.signing_key_id}?\nDigest: ${candidate.digest}\nNothing will be delivered or enforced.`)) return;
      savePending(`${base()}/candidates/${candidate.sequence}/sign`, body, "subject-link.snapshot.sign");
      await deliver(current);
    });
  });
  el("history-form").addEventListener("submit", (event) => {
    event.preventDefault();
    void run(async (current) => {
      el("history-evidence").textContent = "";
      const kind = value("history-kind"), sequence = value("history-sequence");
      if (!["events", "candidates", "signed", "receipts"].includes(kind) || !sequencePattern.test(sequence) || kind !== "events" && sequence === "0") {
        throw new Error("Enter a canonical sequence (0 is only valid for events).");
      }
      const result = await api(`${base()}/${kind}/${sequence}${query()}`);
      if (!current()) return;
      if (!sameScope(result?.scope, currentScope()) || result.sequence !== sequence) throw new Error("Historical response has the wrong scope or sequence.");
      el("history-evidence").textContent = JSON.stringify(result, null, 2);
      status("Historical evidence loaded; it is not current, delivered or enforced state.");
    });
  });
  el("retry").addEventListener("click", () => void run(deliver));
  el("discard").addEventListener("click", () => {
    if (!window.confirm("Discard only this tab's saved retry after reading the stream state? This does not cancel or undo a server-side change.")) return;
    sessionStorage.removeItem(storageKey);
    pending = null;
    pendingInvalid = false;
    invalidate();
    status("Local retry discarded. No server-side state was changed.");
  });
})();
