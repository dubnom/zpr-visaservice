(() => {
  const byId = id => document.getElementById(id);
  const state = {
    identity: null,
    csrf: "",
    busy: false,
    uncertain: false,
    confirmation: null,
  };

  function setStatus(message) {
    byId("budget-session-status").textContent = message;
  }

  function setError(message) {
    const error = byId("budget-error");
    error.textContent = message;
    error.hidden = !message;
  }

  function clearSession(message = "Sign in required.") {
    state.identity = null;
    state.csrf = "";
    state.confirmation = null;
    byId("budget-identity").textContent = "";
    byId("budget-workspace").disabled = true;
    byId("budget-login").hidden = false;
    setStatus(message);
    updateControls();
  }

  function organizationAllowed(organization) {
    return Boolean(state.identity &&
      Array.isArray(state.identity.organizations) &&
      state.identity.organizations.includes(organization));
  }

  function canManage(organization) {
    return Boolean(state.identity &&
      Array.isArray(state.identity.permissions) &&
      state.identity.permissions.includes("budget.approve") &&
      organizationAllowed(organization));
  }

  function updateControls() {
    const organization = byId("budget-organization").value;
    const revokeOrganization = byId("budget-revoke-organization").value;
    const permission = Boolean(state.identity?.permissions?.includes("budget.approve"));
    byId("budget-permission-status").textContent = permission
      ? "Budget mutations require an organization explicitly listed in your operator session."
      : "This session lacks budget.approve. Mutations are disabled.";
    byId("budget-approve").disabled = state.busy || state.uncertain ||
      !canManage(organization) ||
      !byId("budget-scenario").value ||
      !byId("budget-authorization-scope-digest").value ||
      !byId("budget-expires-at").value;
    byId("budget-revoke").disabled = state.busy || state.uncertain ||
      !canManage(revokeOrganization) || !byId("budget-revoke-approval-id").value;
    byId("budget-confirm-submit").disabled = state.busy ||
      !byId("budget-explicit-confirm").checked || !state.confirmation;
  }

  function exactInput(id, label) {
    const value = byId(id).value;
    if (!value || value !== value.trim() || /[\u0000-\u001f\u007f]/.test(value)) {
      throw new Error(`Enter an exact ${label} without surrounding whitespace or control characters.`);
    }
    return value;
  }

  function readExpiry() {
    const value = byId("budget-expires-at").value;
    const expiresAt = new Date(value);
    const now = Date.now();
    if (!value || !Number.isFinite(expiresAt.getTime()) ||
        expiresAt.getTime() <= now || expiresAt.getTime() > now + 60 * 60 * 1000) {
      throw new Error("Choose an explicit future expiry no more than one hour from now.");
    }
    return expiresAt.toISOString();
  }

  function appendDetail(container, label, value) {
    const term = document.createElement("dt");
    term.textContent = label;
    const description = document.createElement("dd");
    description.textContent = value;
    container.append(term, description);
  }

  function openConfirmation(action, title, description, details, returnFocus) {
    const dialog = byId("budget-confirm-dialog");
    byId("budget-confirm-title").textContent = title;
    byId("budget-confirm-description").textContent = description;
    const detailList = byId("budget-confirm-details");
    detailList.replaceChildren();
    for (const [label, value] of details) appendDetail(detailList, label, value);
    byId("budget-explicit-confirm").checked = false;
    byId("budget-confirm-submit").textContent = action.label;
    state.confirmation = { run: action.run, returnFocus };
    updateControls();
    dialog.showModal();
    byId("budget-confirm-title").focus();
  }

  function parseResponseBody(text) {
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch {
      return { error: text };
    }
  }

  async function operatorRequest(path, body) {
    if (!state.identity || !state.csrf) {
      throw new Error("A current named-user operator session and CSRF proof are required.");
    }
    const response = await fetch(path, {
      method: "POST",
      credentials: "same-origin",
      cache: "no-store",
      redirect: "error",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        "X-ZPR-CSRF": state.csrf,
      },
      body: JSON.stringify(body),
    });
    const text = await response.text();
    const data = parseResponseBody(text);
    if (response.status === 401) {
      clearSession("Operator session expired. Sign in again before continuing.");
      const error = new Error(data?.error || "Operator session expired.");
      error.status = response.status;
      throw error;
    }
    if (!response.ok) {
      const error = new Error(data?.error || `Budget authority request failed (HTTP ${response.status}).`);
      error.status = response.status;
      throw error;
    }
    return data;
  }

  function validEvidence(evidence) {
    return evidence && typeof evidence === "object" && !Array.isArray(evidence) &&
      typeof evidence.approval_id === "string" && evidence.approval_id &&
      typeof evidence.organization_id === "string" && evidence.organization_id &&
      typeof evidence.scenario_id === "string" && evidence.scenario_id &&
      typeof evidence.plan_digest === "string" && evidence.plan_digest &&
      evidence.decision === "approved" &&
      typeof evidence.approved_by === "string" && evidence.approved_by &&
      typeof evidence.version === "string" && evidence.version &&
      ["checked_at", "valid_from", "expires_at"].every(key =>
        typeof evidence[key] === "string" && Number.isFinite(Date.parse(evidence[key]))) &&
      typeof evidence.revoked === "boolean";
  }

  function renderEvidence(evidence) {
    for (const [id, value] of [
      ["budget-evidence-approval-id", evidence.approval_id],
      ["budget-evidence-organization", evidence.organization_id],
      ["budget-evidence-scenario", evidence.scenario_id],
      ["budget-evidence-digest", evidence.plan_digest],
      ["budget-evidence-decision", evidence.decision],
      ["budget-evidence-approved-by", evidence.approved_by],
      ["budget-evidence-version", evidence.version],
      ["budget-evidence-checked-at", evidence.checked_at],
      ["budget-evidence-valid-from", evidence.valid_from],
      ["budget-evidence-expires-at", evidence.expires_at],
      ["budget-evidence-revoked", evidence.revoked ? "Yes" : "No"],
    ]) {
      byId(id).textContent = value;
    }
    byId("budget-evidence").hidden = false;
  }

  async function mutate(kind, path, body, expected, returnFocus) {
    if (state.busy || state.uncertain) return;
    state.busy = true;
    setError("");
    setStatus(`Submitting budget ${kind} request…`);
    updateControls();
    try {
      const evidence = await operatorRequest(path, body);
      if (!validEvidence(evidence) ||
          expected.approvalID && evidence.approval_id !== expected.approvalID ||
          evidence.organization_id !== expected.organizationID ||
          expected.scenarioID && evidence.scenario_id !== expected.scenarioID ||
          expected.planDigest && evidence.plan_digest !== expected.planDigest ||
          evidence.revoked !== (kind === "revocation")) {
        state.uncertain = true;
        throw new Error("The service response did not match the requested budget authority scope. Do not retry; reconcile the outcome.");
      }
      renderEvidence(evidence);
      if (kind === "approval") {
        byId("budget-revoke-organization").value = evidence.organization_id;
        byId("budget-revoke-approval-id").value = evidence.approval_id;
        setStatus("Budget approval recorded by the authority service. No Simulator or host action was performed.");
      } else {
        setStatus("Budget approval revocation recorded by the authority service. No Simulator or host action was performed.");
      }
    } catch (error) {
      if (error.status === undefined) state.uncertain = true;
      if (error.status === 401) {
        setError(error.message);
      } else if (error.status) {
        setError(`Budget ${kind} was refused (HTTP ${error.status}): ${error.message}`);
        setStatus("No successful budget authority response was received.");
      } else {
        setError(`${error.message || `Budget ${kind} outcome is unknown.`} No automatic retry was made.`);
        setStatus("Budget authority outcome is unknown; do not retry until it is reconciled.");
      }
    } finally {
      state.busy = false;
      updateControls();
      if (returnFocus?.isConnected) returnFocus.focus();
    }
  }

  byId("budget-approval-form").addEventListener("submit", event => {
    event.preventDefault();
    try {
      const organizationID = exactInput("budget-organization", "organization ID");
      const scenarioID = exactInput("budget-scenario", "scenario ID");
      const planDigest = exactInput("budget-authorization-scope-digest", "authorization scope digest");
      const expiresAt = readExpiry();
      if (!canManage(organizationID)) {
        throw new Error("Your operator session is not authorized with budget.approve for this exact organization.");
      }
      const submit = byId("budget-approve");
      openConfirmation({
        label: "Confirm budget approval",
        run: () => mutate("approval", "/api/budget-authority/approvals", {
          scope: {
            organization_id: organizationID,
            scenario_id: scenarioID,
            plan_digest: planDigest,
          },
          decision: "approved",
          expires_at: expiresAt,
        }, {
          approvalID: null,
          organizationID,
          scenarioID,
          planDigest,
        }, submit),
      }, "Confirm budget approval", "The authority service will record this exact scoped approval until its expiry.", [
        ["Organization", organizationID],
        ["Scenario", scenarioID],
        ["Authorization scope digest", planDigest],
        ["Expires at", expiresAt],
      ], submit);
    } catch (error) {
      setError(error.message);
      setStatus("Budget approval was not submitted.");
    }
  });

  byId("budget-revoke-form").addEventListener("submit", event => {
    event.preventDefault();
    try {
      const organizationID = exactInput("budget-revoke-organization", "organization ID");
      const approvalID = exactInput("budget-revoke-approval-id", "approval ID");
      if (!canManage(organizationID)) {
        throw new Error("Your operator session is not authorized with budget.approve for this exact organization.");
      }
      const submit = byId("budget-revoke");
      openConfirmation({
        label: "Confirm revocation",
        run: () => mutate("revocation",
          `/api/budget-authority/approvals/${encodeURIComponent(approvalID)}/revoke`,
          { organization_id: organizationID },
          { approvalID, organizationID },
          submit),
      }, "Confirm budget approval revocation", "The authority service will revoke only the identified approval for this organization.", [
        ["Organization", organizationID],
        ["Approval ID", approvalID],
      ], submit);
    } catch (error) {
      setError(error.message);
      setStatus("Budget revocation was not submitted.");
    }
  });

  byId("budget-explicit-confirm").addEventListener("change", updateControls);
  byId("budget-confirm-cancel").addEventListener("click", () => {
    byId("budget-confirm-dialog").close("cancel");
  });
  byId("budget-confirm-submit").addEventListener("click", () => {
    if (!byId("budget-explicit-confirm").checked || !state.confirmation || state.busy) return;
    const confirmation = state.confirmation;
    state.confirmation = null;
    byId("budget-confirm-dialog").close("confirmed");
    void confirmation.run();
  });
  byId("budget-confirm-dialog").addEventListener("close", () => {
    const returnFocus = state.confirmation?.returnFocus;
    state.confirmation = null;
    byId("budget-explicit-confirm").checked = false;
    updateControls();
    if (returnFocus?.isConnected) returnFocus.focus();
  });

  for (const id of [
    "budget-organization", "budget-scenario", "budget-authorization-scope-digest",
    "budget-expires-at", "budget-revoke-organization", "budget-revoke-approval-id",
  ]) {
    byId(id).addEventListener("input", () => {
      setError("");
      updateControls();
    });
  }

  byId("budget-login").hidden = true;
  (async () => {
    try {
      const response = await fetch("/auth/operator/session", {
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
        headers: { Accept: "application/json" },
      });
      const text = await response.text();
      const session = parseResponseBody(text);
      if (!response.ok) throw new Error(session?.error || `Operator session check failed (HTTP ${response.status}).`);
      if (!session?.identity?.issuer || !session.identity.subject ||
          !Array.isArray(session.identity.permissions) ||
          !Array.isArray(session.identity.organizations) ||
          typeof session.csrf !== "string" || !session.csrf) {
        throw new Error("The operator session response is incomplete.");
      }
      state.identity = session.identity;
      state.csrf = session.csrf;
      byId("budget-identity").textContent =
        state.identity.display_name || state.identity.email || "Signed in";
      byId("budget-workspace").disabled = false;
      byId("budget-login").hidden = true;
      if (state.identity.permissions.includes("budget.approve")) {
        setStatus("Signed in. Enter an explicit organization, scenario, scope digest, and expiry.");
      } else {
        setStatus("Signed in, but this session lacks the budget.approve permission.");
      }
      updateControls();
    } catch (error) {
      clearSession(`Sign in required: ${error.message}`);
      setError("No budget authority request is available until an authorized operator session is established.");
    }
  })();

  window.addEventListener("pagehide", () => {
    state.identity = null;
    state.csrf = "";
    state.confirmation = null;
  });
})();
