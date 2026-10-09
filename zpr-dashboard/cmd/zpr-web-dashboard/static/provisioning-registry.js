(() => {
  const registryStatus = document.getElementById("provisioning-registry-status");
  if (!registryStatus) return;
  const accessStatus = document.getElementById("provisioning-status");
  const organizationPicker = document.getElementById("provisioning-organization");
  const catalogView = document.getElementById("provisioning-approved-catalog");
  const table = document.getElementById("provisioning-invitations");
  const nextButton = document.getElementById("provisioning-next");
  const cancelStatus = document.getElementById("provisioning-cancel-status");
  const reviewStatus = document.getElementById("provisioning-review-status");
  let catalog = null;
  let next = "";
  let generation = 0;
  let controller = null;
  let detailGeneration = 0;
  let detailController = null;
  let dialog = null;
  let session = null;
  let pendingCancellation = null;
  let pendingReview = null;
  const uncertainCancellations = new Set();
  const uncertainReviews = new Set();
  const active = () => location.hash === "#provisioning-adapters";
  const text = (value) => typeof value === "string" && value.length > 0 && value.length <= 1024;
  const { validInvitation } = provisioningContract;
  let invitationSort = null;

  for (const [header, key] of [...table.tHead.rows[0].cells].map((cell, index) => [
    cell,
    ["name", "inventory", "state", "expiry", null][index],
  ])) {
    if (key) header.dataset.sortKey = key;
  }

  function sortInvitationRows() {
    if (!invitationSort) return;
    const body = table.tBodies[0];
    const rows = [...body.rows];
    rows.sort((left, right) => {
      const leftValue = left.dataset[invitationSort.key];
      const rightValue = right.dataset[invitationSort.key];
      const order = invitationSort.key === "expiry"
        ? Date.parse(leftValue) - Date.parse(rightValue)
        : window.ZPRSortableTable.compareValues(leftValue, rightValue);
      return order * invitationSort.direction
        || window.ZPRSortableTable.compareValues(left.dataset.invitationId, right.dataset.invitationId);
    });
    body.append(...rows);
  }

  window.ZPRSortableTable.bindSortableHeaders({
    table,
    getSort: () => invitationSort,
    onSort: (sort) => {
      invitationSort = sort;
      sortInvitationRows();
    },
  });

  function closeDetail() {
    if (pendingCancellation && !pendingCancellation.settled) {
      uncertainCancellation(pendingCancellation.item);
      pendingCancellation.controller.abort();
    }
    if (pendingReview && !pendingReview.settled) {
      uncertainReview(pendingReview.item);
      pendingReview.controller.abort();
    }
    ++detailGeneration;
    detailController?.abort();
    detailController = null;
    if (dialog) {
      dialog.close();
      dialog.remove();
      dialog = null;
    }
  }

  function resetRegistry(message) {
    ++generation;
    controller?.abort();
    controller = null;
    closeDetail();
    if (!pendingCancellation && !uncertainCancellations.size) {
      cancelStatus.textContent = "No cancellation pending; use fresh Details to review an invitation.";
    }
    if (!pendingReview && !uncertainReviews.size) {
      reviewStatus.textContent = "No review decision submitted.";
    }
    catalog = null;
    window.dispatchEvent(new Event("provisioning-catalog-cleared"));
    next = "";
    organizationPicker.replaceChildren(new Option("Catalog unavailable", ""));
    organizationPicker.disabled = true;
    catalogView.replaceChildren();
    catalogView.hidden = true;
    table.querySelector("tbody").replaceChildren();
    table.hidden = true;
    nextButton.hidden = true;
    registryStatus.textContent = message;
  }

  async function readJSON(path, signal) {
    const response = await fetch(path, { cache: "no-store", credentials: "same-origin", redirect: "error", signal, headers: { Accept: "application/json" } });
    if (!response.ok) {
      let message = `HTTP ${response.status}`;
      if (response.headers.get("Content-Type")?.includes("application/json")) {
        const error = await response.json();
        if (text(error.error)) message += `: ${error.error}`;
      }
      throw new Error(message);
    }
    if (!response.headers.get("Content-Type")?.includes("application/json")) throw new Error("Expected an enrollment JSON response.");
    return response.json();
  }

  function clearDeniedAccess(message) {
    resetRegistry(message);
    accessStatus.dataset.state = "unavailable";
    accessStatus.textContent = `Enrollment administration unavailable: ${message}`;
  }

  function detailsList(pairs) {
    const list = document.createElement("dl");
    for (const [label, value] of pairs) {
      const term = document.createElement("dt");
      term.textContent = label;
      const description = document.createElement("dd");
      description.textContent = value ?? "Not recorded";
      list.append(term, description);
    }
    return list;
  }

  function uncertainCancellation(item) {
    uncertainCancellations.add(item.id);
    cancelStatus.textContent = `Cancellation outcome uncertain for invitation ${item.id}. It may have been cancelled. Do not retry; reopen fresh Details and acknowledge the refreshed outcome before a new decision. Reloading does not prove failure.`;
  }

  function uncertainReview(item) {
    uncertainReviews.add(item.id);
    reviewStatus.textContent = `Review outcome uncertain for invitation ${item.id}. It may have been decided. Do not retry; reopen fresh Details and acknowledge the refreshed outcome before another decision.`;
  }

  function reviewControls(content, item) {
    const organization = item.asset.organization;
    const section = document.createElement("section");
    const status = document.createElement("p");
    status.setAttribute("role", "status");
    section.append(status);
    content.append(section);
    if (uncertainReviews.has(item.id)) {
      status.textContent = `This is a fresh read after an uncertain review: state ${item.state}, revision ${item.revision}, decision ${item.decision_by ?? "not recorded"}. A pending request may still finish; no request has been retried.`;
      const acknowledge = document.createElement("button");
      acknowledge.type = "button";
      acknowledge.className = "button";
      acknowledge.textContent = "Acknowledge refreshed outcome";
      acknowledge.addEventListener("click", () => {
        if (pendingReview) {
          status.textContent = "The previous browser request has not settled yet. Wait, then acknowledge this fresh read; no review has been retried.";
          return;
        }
        uncertainReviews.delete(item.id);
        reviewStatus.textContent = `Refreshed review outcome acknowledged for ${item.id}: ${item.state}, revision ${item.revision}. No request was sent. Reopen Details before another decision.`;
        closeDetail();
      });
      section.append(acknowledge);
      return;
    }
    if (item.state !== "pending_approval" || !item.key_fingerprint || !item.approval_expires_at) {
      status.textContent = "This record is not awaiting a key-bound review. Expired and terminal decisions cannot be changed here.";
      return;
    }
    const canApprove = location.protocol === "https:" && session?.csrf &&
      session.identity.permissions?.includes("approve") &&
      (session.identity.organizations?.includes("*") || session.identity.organizations?.includes(organization)) &&
      catalog?.gui_approve_organizations?.includes(organization);
    const canReject = location.protocol === "https:" && session?.csrf &&
      session.identity.permissions?.includes("reject") &&
      (session.identity.organizations?.includes("*") || session.identity.organizations?.includes(organization)) &&
      catalog?.gui_reject_organizations?.includes(organization);
    if (!canApprove && !canReject) {
      status.textContent = "Approval or rejection requires the matching named-user permission and independent backend grant for this organization.";
      return;
    }
    status.textContent = "Review only this fresh revision and key fingerprint. Independently verify the fingerprint through a trusted channel. Approval does not issue credentials or establish connectivity.";
    const form = document.createElement("form");
    const reasonLabel = document.createElement("label");
    reasonLabel.textContent = "Review reason";
    const reason = document.createElement("textarea");
    reason.name = "review_reason";
    reason.required = true;
    reasonLabel.append(reason);
    const acknowledgement = document.createElement("label");
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.required = true;
    acknowledgement.append(checkbox, document.createTextNode(
      ` I independently verified fingerprint ${item.key_fingerprint} for ${item.id} at revision ${item.revision} through a trusted channel.`,
    ));
    form.append(reasonLabel, acknowledgement);
    if (canApprove) {
      const approve = document.createElement("button");
      approve.type = "submit";
      approve.className = "button";
      approve.value = "approve";
      approve.textContent = "Approve verified key";
      form.append(approve);
    }
    if (canReject) {
      const reject = document.createElement("button");
      reject.type = "submit";
      reject.className = "button button-quiet";
      reject.value = "reject";
      reject.textContent = "Reject key proof";
      form.append(reject);
    }
    section.append(form);
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      const decision = event.submitter?.value;
      const value = reason.value.trim();
      reason.setCustomValidity(value && new TextEncoder().encode(value).length <= 256 && !/[\0\r\n]/.test(value) ? "" :
        "Enter a reason of 1 to 256 UTF-8 bytes, without line breaks.");
      if (!form.reportValidity()) return;
      if ((decision !== "approve" || !canApprove) && (decision !== "reject" || !canReject)) return;
      if (pendingCancellation || pendingReview || uncertainReviews.has(item.id)) {
        status.textContent = "A review is pending or uncertain. Wait for it to settle, then reopen fresh Details before another decision.";
        return;
      }
      for (const control of form.elements) control.disabled = true;
      void decideInvitation(item, decision, value, session.identity, session.csrf, status);
    });
    reason.addEventListener("input", () => reason.setCustomValidity(""));
  }

  async function decideInvitation(item, decision, reason, identity, csrf, status) {
    const attempt = { item, controller: new AbortController(), settled: false };
    pendingReview = attempt;
    const state = decision === "approve" ? "approved" : "rejected";
    reviewStatus.textContent = `Submitting ${state} decision for invitation ${item.id}, revision ${item.revision}, once. Do not retry or navigate away until confirmed.`;
    status.textContent = reviewStatus.textContent;
    const timer = setTimeout(() => attempt.controller.abort(), 20000);
    try {
      const result = await provisioningContract.postMutation(
        `/api/enrollment/v1/invitations/${encodeURIComponent(item.id)}/${decision}`,
        { organization: item.asset.organization, revision: item.revision, key_fingerprint: item.key_fingerprint, reason },
        csrf, attempt.controller.signal, 200,
      );
      if (attempt.controller.signal.aborted) throw new Error("Review interrupted.");
      if (result.rejected) {
        attempt.settled = true;
        reviewStatus.textContent = `${state} rejected (HTTP ${result.rejected}); this request did not decide the invitation. Reopen fresh Details before another decision.`;
        status.textContent = reviewStatus.textContent;
        closeDetail();
        if (result.rejected === 401 || result.rejected === 403) window.dispatchEvent(new Event("operator-session-cleared"));
        return;
      }
      const changed = result.value;
      const auditIdentity = `oidc:${JSON.stringify([identity.issuer, identity.subject])}`;
      if (!validInvitation(changed, item.asset.organization) || changed.id !== item.id || changed.state !== state ||
          changed.revision !== item.revision + 1 ||
          !provisioningContract.assetFields.every((key) => changed.asset[key] === item.asset[key]) ||
          ["created_at", "expires_at", "created_by", "key_fingerprint", "claimed_at", "approval_expires_at"].some((key) => changed[key] !== item[key]) ||
          changed.decision_by !== auditIdentity || changed.decision_reason !== reason || !changed.decided_at) {
        throw new Error("Unconfirmed key-bound review record.");
      }
      attempt.settled = true;
      reviewStatus.textContent = `Invitation ${item.id} ${state} at revision ${changed.revision} by ${changed.decision_by}. Reason: ${reason}. Approval does not issue credentials or establish connectivity.`;
      status.textContent = reviewStatus.textContent;
      closeDetail();
      window.dispatchEvent(new CustomEvent("provisioning-invitation-changed", { detail: { organization: item.asset.organization } }));
    } catch {
      uncertainReview(item);
      status.textContent = reviewStatus.textContent;
    } finally {
      clearTimeout(timer);
      pendingReview = null;
    }
  }

  function cancellationControls(content, item) {
    const organization = item.asset.organization;
    const section = document.createElement("section");
    const status = document.createElement("p");
    status.setAttribute("role", "status");
    section.append(status);
    content.append(section);
    if (uncertainCancellations.has(item.id)) {
      status.textContent = `This is a fresh read after an uncertain cancellation: state ${item.state}, revision ${item.revision}. A pending request can still finish after this read. No request has been retried.`;
      const acknowledge = document.createElement("button");
      acknowledge.type = "button";
      acknowledge.className = "button";
      acknowledge.textContent = "Acknowledge refreshed outcome";
      acknowledge.addEventListener("click", () => {
        if (pendingCancellation) {
          status.textContent = "The previous browser request has not settled yet. Wait, then acknowledge this fresh read; no cancellation has been retried.";
          return;
        }
        uncertainCancellations.delete(item.id);
        cancelStatus.textContent = `Refreshed outcome acknowledged for ${item.id}: ${item.state}, revision ${item.revision}. No cancellation was sent. Reopen Details before another decision.`;
        closeDetail();
      });
      section.append(acknowledge);
      return;
    }
    if (!["invited", "pending_approval"].includes(item.state)) {
      status.textContent = "This record is no longer cancellable. Approved credentials require a separate revocation flow.";
      return;
    }
    if (location.protocol !== "https:" || !session?.identity || !session.csrf ||
        !session.identity.permissions?.includes("cancel") ||
        !(session.identity.organizations?.includes("*") || session.identity.organizations?.includes(organization)) ||
        !catalog?.gui_cancel_organizations?.includes(organization)) {
      status.textContent = "Cancellation requires named-user cancel permission and independent backend authorization for this organization.";
      return;
    }
    status.textContent = "Cancel this exact invitation and any pending key submission. This does not revoke issued credentials. A state/key/revision change requires a fresh review.";
    const form = document.createElement("form");
    const label = document.createElement("label");
    label.textContent = "Cancellation reason";
    const reason = document.createElement("textarea");
    reason.name = "cancellation_reason";
    reason.required = true;
    label.append(reason);
    const acknowledgement = document.createElement("label");
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.required = true;
    acknowledgement.append(checkbox, document.createTextNode(` I confirm cancellation of ${item.id}, revision ${item.revision}, for ${item.asset.name} (${item.asset.asset_id}).`));
    const confirm = document.createElement("button");
    confirm.type = "submit";
    confirm.className = "button";
    confirm.textContent = "Confirm cancellation";
    form.append(label, acknowledgement, confirm);
    section.append(form);
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      const value = reason.value.trim();
      reason.setCustomValidity(value && new TextEncoder().encode(value).length <= 256 && !/[\0\r\n]/.test(value) ? "" :
        "Enter a reason of 1 to 256 UTF-8 bytes, without line breaks.");
      if (!form.reportValidity()) return;
      if (pendingCancellation || uncertainCancellations.has(item.id)) {
        status.textContent = "A cancellation is pending or uncertain. Wait for it to settle, then reopen fresh Details before another decision.";
        return;
      }
      for (const control of form.elements) control.disabled = true;
      void cancelInvitation(item, value, session.identity, session.csrf, status);
    });
    reason.addEventListener("input", () => reason.setCustomValidity(""));
  }

  async function cancelInvitation(item, reason, identity, csrf, status) {
    const attempt = { item, controller: new AbortController(), settled: false };
    pendingCancellation = attempt;
    cancelStatus.textContent = `Cancelling invitation ${item.id} once. Do not retry or navigate away until the outcome is confirmed.`;
    status.textContent = cancelStatus.textContent;
    const timer = setTimeout(() => attempt.controller.abort(), 20000);
    try {
      const result = await provisioningContract.postMutation(`/api/enrollment/v1/invitations/${encodeURIComponent(item.id)}/cancel`, {
        organization: item.asset.organization, revision: item.revision, key_fingerprint: item.key_fingerprint ?? "", reason,
      }, csrf, attempt.controller.signal, 200);
      if (attempt.controller.signal.aborted) throw new Error("Cancellation interrupted.");
      if (result.rejected) {
        attempt.settled = true;
        cancelStatus.textContent = `Cancellation rejected (HTTP ${result.rejected}); this request did not cancel the invitation. Reopen fresh Details before another decision.`;
        status.textContent = cancelStatus.textContent;
        if (result.rejected === 401 || result.rejected === 403) window.dispatchEvent(new Event("operator-session-cleared"));
        return;
      }
      const changed = result.value;
      if (!validInvitation(changed, item.asset.organization) || changed.id !== item.id || changed.state !== "cancelled" ||
          changed.revision !== item.revision + 1 ||
          !provisioningContract.assetFields.every((key) => changed.asset[key] === item.asset[key]) ||
          ["created_at", "expires_at", "created_by", "key_fingerprint", "claimed_at", "approval_expires_at"].some((key) => changed[key] !== item[key]) ||
          changed.decision_by !== `oidc:${JSON.stringify([identity.issuer, identity.subject])}` ||
          changed.decision_reason !== reason || !changed.decided_at) throw new Error("Unconfirmed cancellation record.");
      attempt.settled = true;
      cancelStatus.textContent = `Invitation ${item.id} cancelled at revision ${changed.revision} by ${changed.decision_by}. Reason: ${reason}. No credentials were revoked.`;
      status.textContent = cancelStatus.textContent;
      window.dispatchEvent(new CustomEvent("provisioning-invitation-changed", { detail: { organization: item.asset.organization } }));
    } catch {
      uncertainCancellation(item);
      status.textContent = cancelStatus.textContent;
    } finally {
      clearTimeout(timer);
      pendingCancellation = null;
    }
  }

  async function showInvitation(id, organization) {
    closeDetail();
    const current = ++detailGeneration;
    detailController = new AbortController();
    const content = document.createElement("div");
    const message = document.createElement("p");
    message.textContent = "Loading fresh registry details...";
    message.setAttribute("role", "status");
    content.append(message);
    dialog = document.createElement("dialog");
    dialog.className = "policy-dialog provisioning-review";
    dialog.setAttribute("aria-labelledby", "provisioning-detail-title");
    const heading = document.createElement("h2");
    heading.id = "provisioning-detail-title";
    heading.textContent = "Enrollment request details";
    const close = document.createElement("button");
    close.type = "button";
    close.className = "button";
    close.textContent = "Close";
    close.addEventListener("click", closeDetail);
    dialog.append(heading, content, close);
    dialog.addEventListener("cancel", (event) => { event.preventDefault(); closeDetail(); });
    document.body.append(dialog);
    dialog.showModal();
    try {
      const item = await readJSON(`/api/enrollment/v1/invitations/${encodeURIComponent(id)}?organization=${encodeURIComponent(organization)}`, detailController.signal);
      if (current !== detailGeneration || !dialog) return;
      if (!validInvitation(item, organization) || item.id !== id) throw new Error("Registry detail identity or response shape is invalid.");
      message.textContent = "Fresh registry record. Approval does not issue credentials or establish connectivity. Verify the fingerprint through an independent trusted channel before any later approval.";
      content.append(detailsList([
        ["Invitation ID", item.id], ["Organization", organization], ["Name", item.asset.name],
        ["Owner", item.asset.owner], ["Inventory reference", item.asset.asset_id],
        ["Type", item.asset.type], ["Profile", item.asset.profile], ["Instruction recipient", item.asset.recipient],
        ["State", item.state], ["Revision", String(item.revision)], ["Key fingerprint", item.key_fingerprint],
        ["Created by", item.created_by], ["Created at", item.created_at], ["Invitation expiry", item.expires_at],
        ["Claimed at", item.claimed_at], ["Approval deadline", item.approval_expires_at],
        ["Decision by", item.decision_by], ["Decision reason", item.decision_reason], ["Decided at", item.decided_at],
      ]));
      const notice = document.createElement("p");
      notice.textContent = "No enrollment code is available from read endpoints.";
      content.append(notice);
      reviewControls(content, item);
      cancellationControls(content, item);
    } catch (error) {
      if (current !== detailGeneration || error.name === "AbortError") return;
      message.textContent = `Registry detail unavailable: ${error.message}`;
      if (error.message.startsWith("HTTP 401") || error.message.startsWith("HTTP 403")) clearDeniedAccess(message.textContent);
    }
  }

  async function loadPage(after = "") {
    const organization = organizationPicker.value;
    if (!catalog || !Object.hasOwn(catalog.organizations, organization)) return;
    const current = ++generation;
    controller?.abort();
    controller = new AbortController();
    closeDetail();
    next = "";
    nextButton.hidden = true;
    table.hidden = true;
    table.querySelector("tbody").replaceChildren();
    registryStatus.textContent = `Loading registry for ${organization}...`;
    catalogView.hidden = false;
    const approved = catalog.organizations[organization];
    catalogView.textContent = `Approved types: ${approved.types.join(", ")}. Approved profiles: ${approved.profiles.join(", ")}. Invitation lifetime: ${catalog.invitation_lifetime_seconds} seconds. Approval lifetime: ${catalog.approval_lifetime_seconds} seconds. No invitation is submitted by reading this catalog.`;
    try {
      const query = new URLSearchParams({ organization, limit: "50" });
      if (after) query.set("after", after);
      const result = await readJSON(`/api/enrollment/v1/invitations?${query}`, controller.signal);
      if (current !== generation) return;
      if (!Array.isArray(result.invitations) || result.invitations.length > 50 ||
          !result.invitations.every((item) => validInvitation(item, organization)) ||
          new Set(result.invitations.map((item) => item.id)).size !== result.invitations.length ||
          (result.next_after !== undefined && (!text(result.next_after) || result.next_after === after ||
            result.next_after !== result.invitations.at(-1)?.id))) throw new Error("Registry page identity or response shape is invalid.");
      const rows = result.invitations.map((item) => {
        const row = document.createElement("tr");
        row.dataset.invitationId = item.id;
        row.dataset.name = item.asset.name;
        row.dataset.inventory = item.asset.asset_id;
        row.dataset.state = item.state;
        row.dataset.expiry = item.expires_at;
        for (const value of [item.asset.name, item.asset.asset_id, item.state, item.expires_at]) {
          const cell = document.createElement("td");
          cell.textContent = value;
          row.append(cell);
        }
        const actions = document.createElement("td");
        const button = document.createElement("button");
        button.type = "button";
        button.className = "button";
        button.textContent = "Details";
        button.setAttribute("aria-label", `Details for ${item.asset.name}`);
        button.addEventListener("click", () => void showInvitation(item.id, organization));
        actions.append(button);
        row.append(actions);
        return row;
      });
      table.querySelector("tbody").replaceChildren(...rows);
      sortInvitationRows();
      table.hidden = rows.length === 0;
      next = result.next_after || "";
      nextButton.hidden = !next;
      registryStatus.textContent = rows.length ? `${organization}: ${rows.length} records on this page${next ? "; more available" : ""}. Registry state is not live adapter connectivity.` :
        `${organization}: no invitations on this registry page. This is not a live adapter inventory.`;
    } catch (error) {
      if (current !== generation || error.name === "AbortError") return;
      registryStatus.textContent = `Registry unavailable: ${error.message}. This is not an empty queue.`;
      if (error.message.startsWith("HTTP 401") || error.message.startsWith("HTTP 403")) clearDeniedAccess(registryStatus.textContent);
    }
  }

  async function loadCatalog() {
    const selected = organizationPicker.value;
    resetRegistry("Checking authorized catalog...");
    const current = generation;
    controller = new AbortController();
    accessStatus.dataset.state = "checking";
    accessStatus.textContent = "Checking enrollment administration...";
    try {
      const result = await readJSON("/api/enrollment/v1/catalog", controller.signal);
      if (current !== generation) return;
      const validNames = (list) => Array.isArray(list) && list.length > 0 && list.length <= 1000 &&
        list.every(text) && new Set(list).size === list.length;
      if (!result.organizations || typeof result.organizations !== "object" || Array.isArray(result.organizations) ||
          Object.keys(result.organizations).length > 1000 ||
          !Object.entries(result.organizations).every(([name, value]) => text(name) && value && validNames(value.types) && validNames(value.profiles)) ||
          !Number.isInteger(result.invitation_lifetime_seconds) || result.invitation_lifetime_seconds < 1 ||
          !Number.isInteger(result.approval_lifetime_seconds) || result.approval_lifetime_seconds < 0 ||
          typeof result.gui_mutations_enabled !== "boolean") throw new Error("Invalid enrollment catalog response.");
      if (result.gui_create_organizations !== undefined && (!Array.isArray(result.gui_create_organizations) ||
          new Set(result.gui_create_organizations).size !== result.gui_create_organizations.length ||
          !result.gui_create_organizations.every((name) => text(name) && Object.hasOwn(result.organizations, name)))) {
        throw new Error("Invalid invitation creation capabilities.");
      }
      if (result.gui_cancel_organizations !== undefined && (!Array.isArray(result.gui_cancel_organizations) ||
          new Set(result.gui_cancel_organizations).size !== result.gui_cancel_organizations.length ||
          !result.gui_cancel_organizations.every((name) => text(name) && Object.hasOwn(result.organizations, name)))) {
        throw new Error("Invalid invitation cancellation capabilities.");
      }
      for (const key of ["gui_approve_organizations", "gui_reject_organizations"]) {
        if (result[key] !== undefined && (!Array.isArray(result[key]) ||
            new Set(result[key]).size !== result[key].length ||
            !result[key].every((name) => text(name) && Object.hasOwn(result.organizations, name)))) {
          throw new Error("Invalid invitation review capabilities.");
        }
      }
      catalog = result;
      window.dispatchEvent(new CustomEvent("provisioning-catalog-ready", { detail: result }));
      accessStatus.dataset.state = "available";
      accessStatus.textContent = "Authorized enrollment catalog loaded. Creation, cancellation, approval and rejection each require named-user permission and independent backend authorization. Review is revision/key-bound and does not issue credentials.";
      const names = Object.keys(result.organizations).sort();
      organizationPicker.replaceChildren(...names.map((name) => new Option(name, name)));
      organizationPicker.disabled = names.length === 0;
      if (!names.length) {
        registryStatus.textContent = "No organizations are authorized by this catalog; no registry queue was queried. This is not an empty queue.";
        return;
      }
      organizationPicker.value = names.includes(selected) ? selected : names[0];
      await loadPage();
    } catch (error) {
      if (current !== generation || error.name === "AbortError") return;
      accessStatus.dataset.state = "unavailable";
      accessStatus.textContent = `Enrollment administration unavailable: ${error.message}`;
      registryStatus.textContent = "Registry unavailable; this is not an empty queue or a live adapter inventory.";
    }
  }

  organizationPicker.addEventListener("change", () => void loadPage());
  nextButton.addEventListener("click", () => { if (next) void loadPage(next); });
  window.addEventListener("provisioning-refresh", () => { if (active()) void loadCatalog(); });
  function refreshInvitation(event) {
    if (active() && catalog && Object.hasOwn(catalog.organizations, event.detail.organization)) {
      organizationPicker.value = event.detail.organization;
      void loadPage();
    }
  }
  window.addEventListener("provisioning-invitation-created", refreshInvitation);
  window.addEventListener("provisioning-invitation-changed", refreshInvitation);
  window.addEventListener("operator-session-cleared", () => {
    session = null;
    resetRegistry("Registry cleared while operator session is unavailable or being checked.");
    accessStatus.dataset.state = "unavailable";
    accessStatus.textContent = "Operator session unavailable or being checked; use Check again after sign-in.";
  });
  window.addEventListener("operator-session-ready", (event) => {
    session = event.detail ?? null;
    if (active()) void loadCatalog();
  });
  window.addEventListener("hashchange", () => { if (!active()) resetRegistry("Registry cleared on navigation."); });
  window.addEventListener("pagehide", () => resetRegistry("Registry cleared."));
  if (active()) void loadCatalog();
})();
