(() => {
  const registryStatus = document.getElementById("provisioning-registry-status");
  if (!registryStatus) return;
  const accessStatus = document.getElementById("provisioning-status");
  const organizationPicker = document.getElementById("provisioning-organization");
  const catalogView = document.getElementById("provisioning-approved-catalog");
  const table = document.getElementById("provisioning-invitations");
  const nextButton = document.getElementById("provisioning-next");
  let catalog = null;
  let next = "";
  let generation = 0;
  let controller = null;
  let detailGeneration = 0;
  let detailController = null;
  let dialog = null;
  const active = () => location.hash === "#provisioning-adapters";
  const text = (value) => typeof value === "string" && value.length > 0 && value.length <= 1024;
  const { validInvitation } = provisioningContract;

  function closeDetail() {
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
      message.textContent = "Read-only registry record. Approval does not issue credentials or establish connectivity. Verify the fingerprint through an independent trusted channel before any later decision.";
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
      notice.textContent = "Approve, reject, and cancel controls are not enabled in this release. No enrollment code is available from read endpoints.";
      content.append(notice);
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
      catalog = result;
      window.dispatchEvent(new CustomEvent("provisioning-catalog-ready", { detail: result }));
      accessStatus.dataset.state = "available";
      accessStatus.textContent = "Authorized enrollment catalog loaded. Invitation creation requires explicit backend opt-in and named-user create grants. Review and cancellation mutations remain locked.";
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
  window.addEventListener("provisioning-invitation-created", (event) => {
    if (active() && catalog && Object.hasOwn(catalog.organizations, event.detail.organization)) {
      organizationPicker.value = event.detail.organization;
      void loadPage();
    }
  });
  window.addEventListener("operator-session-cleared", () => {
    resetRegistry("Registry cleared while operator session is unavailable or being checked.");
    accessStatus.dataset.state = "unavailable";
    accessStatus.textContent = "Operator session unavailable or being checked; use Check again after sign-in.";
  });
  window.addEventListener("operator-session-ready", () => { if (active()) void loadCatalog(); });
  window.addEventListener("hashchange", () => { if (!active()) resetRegistry("Registry cleared on navigation."); });
  window.addEventListener("pagehide", () => resetRegistry("Registry cleared."));
  if (active()) void loadCatalog();
})();
