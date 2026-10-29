(() => {
  const el = (id) => document.getElementById(`approvals-${id}`);
  let identity = null, busy = false, epoch = 0, reviews = [];
  const same = (a, b) => a?.issuer === b?.issuer && a?.subject === b?.subject;
  const status = (message, error = false) => {
    const node = document.getElementById("status");
    node.textContent = message;
    node.classList.toggle("error", error);
  };
  const allowed = (permission) => !!identity?.permissions.includes(permission) &&
    (identity.organizations.includes(el("organization").value) || identity.organizations.includes("*"));

  function stateOf(review) {
    if (review.state === "review-approved") return "Approved";
    if (review.state === "submitted") {
      return Date.parse(review.proposal.expires_at) < Date.now() ? "Expired" : "Waiting";
    }
    return review.state;
  }
  function describe(review) {
    const { root, target } = review.proposal;
    return `Link device ${root.subject_id} to directory entry ${target.subject_id}`;
  }
  function requester(review) {
    return `${review.proposal.author.subject} on ${new Date(review.proposal.submitted_at).toLocaleString()}`;
  }
  function render() {
    const list = el("list");
    list.replaceChildren();
    const waitingOnly = el("filter").value === "waiting";
    const shown = reviews.filter((review) => !waitingOnly || stateOf(review) === "Waiting");
    el("empty").hidden = shown.length !== 0;
    for (const review of shown) {
      const item = document.createElement("li");
      const state = stateOf(review);
      const badge = document.createElement("strong");
      badge.textContent = state;
      badge.className = `approval-state approval-state-${state.toLowerCase()}`;
      const title = document.createElement("span");
      title.textContent = ` ${describe(review)}`;
      const meta = document.createElement("p");
      meta.textContent = `Requested by ${requester(review)}. Valid until ${new Date(review.proposal.expires_at).toLocaleString()}. Reason: ${review.proposal.reason}`;
      item.append(badge, title, meta);
      if (state === "Approved") {
        const last = review.events.at(-1);
        const note = document.createElement("p");
        note.textContent = `Approved by ${last?.actor?.subject ?? "unknown"}${same(last?.actor, review.proposal.author) ? " (the requester: single-operator approval, less secure than independent review)" : ""}.`;
        item.append(note);
      }
      if (state === "Waiting") {
        if (same(identity, review.proposal.author)) {
          const warning = document.createElement("p");
          warning.textContent = "You requested this. Approving it yourself is recorded in the audit as a single-operator approval.";
          item.append(warning);
        }
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = "Approve";
        button.disabled = busy || !allowed("subject-link.approve");
        button.addEventListener("click", () => void approve(review));
        item.append(button);
      }
      list.append(item);
    }
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
  async function refresh() {
    const organization = el("organization").value;
    const current = ++epoch;
    reviews = [];
    render();
    if (!/^[A-Za-z0-9_-]{1,128}$/.test(organization) || !allowed("subject-link.read")) {
      status("Choose an organization you are allowed to read.", true);
      return;
    }
    busy = true;
    try {
      const result = await api(`/api/subject-link-reviews/queue?organization_id=${encodeURIComponent(organization)}`);
      if (current !== epoch) return;
      if (!Array.isArray(result.reviews) || result.reviews.some((review) => review?.proposal?.organization_id !== organization)) {
        throw new Error("Unexpected approvals response.");
      }
      reviews = result.reviews;
      status(`${reviews.filter((review) => stateOf(review) === "Waiting").length} waiting for approval.`);
    } catch (error) {
      if (current === epoch) status(error.message, true);
    } finally {
      busy = false;
      if (current === epoch) render();
    }
  }
  async function approve(review) {
    const organization = el("organization").value;
    const reason = el("reason").value.trim();
    if (busy || !reason || !allowed("subject-link.approve")) return;
    if (!window.confirm(`Approve this request?\n${describe(review)}`)) return;
    busy = true;
    render();
    try {
      const body = JSON.stringify({ organization_id: organization, digest: review.digest, version: review.version,
        registry_revision: review.proposal.registry_revision, reason });
      const key = crypto.randomUUID().replaceAll("-", "");
      const result = await api(`/api/subject-link-reviews/${review.id}/approve`, "POST", body, key);
      if (result.id !== review.id || result.state !== "review-approved") throw new Error("Approval response did not match. Refresh to check the outcome.");
      status("Approved.");
    } catch (error) {
      status(`${error.message} Refresh to check the current state.`, true);
    } finally {
      busy = false;
      await refresh();
    }
  }
  window.addEventListener("operator-session-ready", (event) => {
    identity = event.detail.identity;
    el("workspace").disabled = false;
    const list = el("organizations");
    list.replaceChildren(...identity.organizations.filter((name) => name !== "*").map((name) => {
      const option = document.createElement("option");
      option.value = name;
      return option;
    }));
    const named = identity.organizations.filter((name) => name !== "*");
    if (!el("organization").value && named.length === 1) el("organization").value = named[0];
    if (el("organization").value) void refresh();
    else status("Choose an organization.");
  });
  window.addEventListener("operator-session-cleared", () => {
    identity = null;
    ++epoch;
    reviews = [];
    el("workspace").disabled = true;
    render();
    status("Session cleared. Sign in to continue.");
  });
  el("refresh").addEventListener("click", () => void refresh());
  el("filter").addEventListener("change", render);
  el("organization").addEventListener("change", () => void refresh());
})();
