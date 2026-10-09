(() => {
  const form = document.getElementById("provisioning-draft");
  if (!form) return;
  const status = document.getElementById("provisioning-draft-status");
  const createStatus = document.getElementById("provisioning-create-status");
  const createButton = document.getElementById("provisioning-create");
  const reconcileButton = document.getElementById("provisioning-reconcile");
  const note = document.getElementById("provisioning-catalog-note");
  const fields = [
    ["name", "Name"], ["owner", "Owner"], ["organization", "Organization"],
    ["asset_id", "Inventory reference"], ["type", "Type"],
    ["profile", "Profile"], ["recipient", "Instruction recipient"],
  ];
  const requestedFields = ["organization", "type", "profile"];
  let dialog = null;
  let catalog = null;
  let session = null;
  let pending = null;
  let uncertain = null;
  let secretVisible = false;
  let confirmedSummary = "";

  const input = (name) => form.elements.namedItem(name);
  const active = () => location.hash === "#provisioning-adapters";
  const canCreate = () => active() && location.protocol === "https:" && session &&
    session.identity.permissions.includes("create") &&
    (session.identity.organizations.includes("*") || session.identity.organizations.includes(input("organization").value)) &&
    catalog?.gui_create_organizations?.includes(input("organization").value);

  function updateControls() {
    createButton.disabled = !canCreate() || Boolean(pending || uncertain || secretVisible);
    reconcileButton.hidden = !uncertain || Boolean(pending);
    for (const control of form.elements) {
      if (control !== createButton && control !== reconcileButton) control.disabled = Boolean(pending);
    }
  }

  function closeDialog() {
    if (!dialog) return;
    const previous = dialog;
    dialog = null;
    previous.close();
    previous.replaceChildren();
    previous.remove();
    if (secretVisible) {
      secretVisible = false;
      createStatus.textContent = `${confirmedSummary}; one-time code cleared. Read endpoints cannot recover it. If it was not securely delivered, use authorized cancellation from fresh Details, certificate administration, or expiry before creating a replacement.`;
    }
    updateControls();
  }

  function showDialog(title, content) {
    closeDialog();
    dialog = document.createElement("dialog");
    dialog.className = "policy-dialog provisioning-review";
    dialog.setAttribute("aria-labelledby", "provisioning-dialog-title");
    const heading = document.createElement("h2");
    heading.id = "provisioning-dialog-title";
    heading.textContent = title;
    const close = document.createElement("button");
    close.type = "button";
    close.className = "button";
    close.textContent = "Close";
    close.addEventListener("click", closeDialog);
    dialog.append(heading, content, close);
    dialog.addEventListener("cancel", (event) => { event.preventDefault(); closeDialog(); });
    document.body.append(dialog);
    dialog.showModal();
  }

  function paragraph(content, text) {
    const element = document.createElement("p");
    element.textContent = text;
    content.append(element);
    return element;
  }

  function assetDetails(asset) {
    const details = document.createElement("dl");
    for (const [name, label] of fields) {
      const term = document.createElement("dt");
      term.textContent = label;
      const value = document.createElement("dd");
      value.textContent = asset[name];
      details.append(term, value);
    }
    return details;
  }

  function choice(name, names) {
    const previous = input(name);
    const selected = previous.value;
    const select = document.createElement("select");
    select.name = name;
    select.required = true;
    select.setAttribute("aria-describedby", "provisioning-catalog-note");
    select.replaceChildren(new Option("Choose approved value", ""), ...names.map((value) => new Option(value, value)));
    select.value = names.includes(selected) ? selected : "";
    previous.replaceWith(select);
  }

  function updateDependentChoices() {
    const approved = catalog?.organizations[input("organization").value];
    choice("type", approved?.types || []);
    choice("profile", approved?.profiles || []);
    updateControls();
  }

  function clearCatalog() {
    closeDialog();
    catalog = null;
    for (const name of requestedFields) {
      const previous = input(name);
      if (previous.tagName !== "SELECT") continue;
      const requested = document.createElement("input");
      requested.name = name;
      requested.required = true;
      requested.maxLength = 200;
      requested.value = previous.value;
      requested.setAttribute("aria-describedby", "provisioning-catalog-note");
      previous.replaceWith(requested);
    }
    note.textContent = "Organization, type, and profile are requests, not approved selections. Authoritative catalog unavailable; creation is locked.";
    updateControls();
  }

  function readAsset() {
    for (const [name] of fields) {
      const control = input(name);
      control.value = control.value.trim();
      const valid = control.value && new TextEncoder().encode(control.value).length <= 256 && !/[\0\r\n]/.test(control.value);
      control.setCustomValidity(valid ? "" : !control.value ? "Enter a value, not just spaces." :
        "Enter a value of at most 256 UTF-8 bytes, without line breaks.");
    }
    if (!form.reportValidity()) return null;
    return Object.fromEntries(fields.map(([name]) => [name, input(name).value]));
  }

  function publicInvitationLink(base, organization, invitationID) {
    let url;
    try {
      url = new URL(base.trim());
    } catch {
      return null;
    }
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) return null;
    if (url.protocol === 'http:') {
      const host = url.hostname.toLowerCase().replace(/^\[|\]$/g, '');
      const octets = host.split('.').map(Number);
      const privateIPv4 = octets.length === 4 && octets.every((part) => Number.isInteger(part) && part >= 0 && part <= 255) &&
        (octets[0] === 10 || (octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31) ||
          (octets[0] === 192 && octets[1] === 168));
      if (!privateIPv4 && !host.endsWith('.local')) return null;
    }
    const fragment = new URLSearchParams({ organization, invitation_id: invitationID });
    return `${url.origin}${url.pathname}#${fragment}`;
  }

  function emailDraft(asset, invitation, link) {
    return [
      `To: ${asset.recipient}`,
      'Subject: Set up your ZPR device',
      '',
      `Hello ${asset.owner},`,
      '',
      'Use this link to prepare your device:',
      link,
      '',
      `Organization: ${asset.organization}`,
      `Invitation ID: ${invitation.id}`,
      `Invitation expires: ${invitation.expires_at}`,
      '',
      'Your activation code will be supplied separately. Enter it only in the ZPR setup app; do not reply to this email with the code.',
      '',
      'After setup, verify the device fingerprint with your administrator through an authenticated channel. Approval does not itself issue runtime credentials or confirm connectivity.',
    ].join('\n');
  }

  function markUncertain(asset) {
    uncertain = asset;
    createStatus.textContent = `Creation outcome uncertain for ${asset.organization} / ${asset.asset_id} (${asset.name}). An invitation may exist, but no usable code was confirmed. Do not retry. Read the registry and resolve any existing invitation using authorized cancellation from fresh Details, certificate administration, or expiry. Reloading does not resolve this uncertainty.`;
    updateControls();
  }

  function invalidate() {
    closeDialog();
    if (pending && !pending.rejected) {
      markUncertain(pending.asset);
      pending.controller.abort();
    }
    updateControls();
  }

  async function submitInvitation(asset, identity, csrf, lifetime) {
    if (!canCreate() || pending || uncertain) return;
    const attempt = { asset, controller: new AbortController() };
    pending = attempt;
    closeDialog();
    createStatus.textContent = "Creating invitation once. Do not retry or navigate away until the outcome is confirmed.";
    updateControls();
    const timer = setTimeout(() => attempt.controller.abort(), 20000);
    try {
      const response = await provisioningContract.postMutation("/api/enrollment/v1/invitations", asset, csrf, attempt.controller.signal, 201);
      if (attempt.controller.signal.aborted) return;
      if (response.rejected) {
        attempt.rejected = true;
        createStatus.textContent = response.rejected === 409 ?
          "Creation rejected (HTTP 409). An active invitation or reserved asset may already exist. Read the registry before attempting a replacement." :
          `Creation rejected (HTTP ${response.rejected}); no invitation was created by this request. Check fields, current grants, and the backend opt-in setting.`;
        if (response.rejected === 401 || response.rejected === 403) {
          session = null;
          window.dispatchEvent(new Event("operator-session-cleared"));
        }
        return;
      }
      const result = response.value;
      if (attempt.controller.signal.aborted) return;
      const item = result.invitation;
      if (!provisioningContract.validInvitation(item, asset.organization) || item.state !== "invited" || item.revision !== 1 ||
          !provisioningContract.assetFields.every((key) => item.asset[key] === asset[key]) ||
          item.created_by !== `oidc:${JSON.stringify([identity.issuer, identity.subject])}` ||
          Date.parse(item.expires_at) - Date.parse(item.created_at) !== lifetime * 1000 || Date.parse(item.expires_at) <= Date.now() ||
          typeof result.enrollment_code !== "string" || !/^[A-Z2-7]{26}$/.test(result.enrollment_code)) {
        throw new Error("Unconfirmed creation identity or response shape.");
      }
      const content = document.createElement("div");
      paragraph(content, `Invitation ID: ${item.id}. Expires: ${item.expires_at}. Created by: ${item.created_by}.`);
      content.append(assetDetails(item.asset));
      paragraph(content, "Enrollment code is shown once, only here. Deliver it through a separate authenticated secure channel, never email or the installer URL. Closing, navigation, session checks/loss, or logout clears it; it cannot be recovered from registry reads.");
      const code = document.createElement("code");
      code.id = "provisioning-one-time-code";
      code.textContent = result.enrollment_code;
      content.append(code);
      const link = publicInvitationLink(input("public_enrollment_url").value, asset.organization, item.id);
      if (link) {
        paragraph(content, "No email was sent. Copy this draft; it contains the setup link and invitation details only, never the activation code.");
        const draft = document.createElement("textarea");
        draft.id = "provisioning-email-draft";
        draft.readOnly = true;
        draft.setAttribute("aria-label", "Email draft without activation code");
        draft.value = emailDraft(asset, item, link);
        const copy = document.createElement("button");
        copy.type = "button";
        copy.className = "button";
        copy.textContent = "Copy email draft";
        copy.addEventListener("click", async () => {
          try {
            await navigator.clipboard.writeText(draft.value);
            createStatus.textContent = "Email draft copied. The activation code was not included.";
          } catch {
            draft.focus();
            draft.select();
            createStatus.textContent = "Email draft selected. Copy the selection; the activation code is not included.";
          }
        });
        content.append(draft, copy);
      } else {
        paragraph(content, "No email was sent and no public setup URL is configured. Set a valid setup page URL to prepare the email draft. This invitation does not issue credentials or admit an adapter.");
      }
      paragraph(content, "The recipient's machine may be remote or offline now; it must reach the enrollment service when the setup app runs. No credentials are issued by this invitation flow.");
      showDialog("Invitation created — one-time code", content);
      confirmedSummary = `Invitation ${item.id} created for ${asset.organization} / ${asset.asset_id}, expiring ${item.expires_at}`;
      secretVisible = true;
      createStatus.textContent = "Invitation created. Record its ID and securely deliver the one-time code before closing.";
      window.dispatchEvent(new CustomEvent("provisioning-invitation-created", { detail: { organization: asset.organization } }));
    } catch {
      if (!uncertain) markUncertain(asset);
    } finally {
      clearTimeout(timer);
      if (attempt.controller.signal.aborted && !attempt.rejected && !uncertain) markUncertain(asset);
      pending = null;
      updateControls();
    }
  }

  form.addEventListener("input", (event) => {
    event.target.setCustomValidity("");
    status.textContent = "Unsaved worksheet. Nothing has been submitted.";
    updateControls();
  });
  form.addEventListener("change", (event) => {
    if (event.target.name === "organization" && catalog) updateDependentChoices();
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const asset = readAsset();
    if (!asset || pending) return;
    const content = document.createElement("div");
    paragraph(content, catalog ? "Unsaved worksheet. Catalog selections are approved, but this review does not create an invitation, reserve an asset, send email, or enroll a machine." :
      "Unsaved and unvalidated. This review does not create an invitation, reserve an asset, send email, or enroll a machine.");
    content.append(assetDetails(asset));
    showDialog("Review invitation worksheet", content);
    status.textContent = "Worksheet reviewed locally. No invitation was created.";
  });
  createButton.addEventListener("click", () => {
    const asset = readAsset();
    if (!asset || !canCreate() || pending || uncertain) return;
    const identity = { issuer: session.identity.issuer, subject: session.identity.subject };
    const csrf = session.csrf;
    const lifetime = catalog.invitation_lifetime_seconds;
    const content = document.createElement("div");
    content.append(assetDetails(asset));
    paragraph(content, `Create one expiring invitation as ${identity.subject}? Invitation lifetime: ${lifetime} seconds. The machine need not be present or online. This sends no email, issues no credentials, and creates no live adapter.`);
    const label = document.createElement("label");
    const acknowledge = document.createElement("input");
    acknowledge.type = "checkbox";
    label.append(acknowledge, document.createTextNode(" I will deliver the one-time code through a separate authenticated secure channel, never email."));
    const confirm = document.createElement("button");
    confirm.type = "button";
    confirm.className = "button";
    confirm.textContent = "Confirm creation";
    confirm.disabled = true;
    acknowledge.addEventListener("change", () => { confirm.disabled = !acknowledge.checked; });
    confirm.addEventListener("click", () => {
      if (acknowledge.checked) void submitInvitation(asset, identity, csrf, lifetime);
    });
    content.append(label, confirm);
    showDialog("Confirm invitation creation", content);
  });
  reconcileButton.addEventListener("click", () => {
    if (!uncertain || pending) return;
    const content = document.createElement("div");
    content.append(assetDetails(uncertain));
    paragraph(content, "Read all registry pages for this organization and inventory reference. If an invitation exists without a securely delivered code, use its fresh Details to cancel it with authorized named-user administration, use certificate-authorized cancellation, or wait until it expires. Do not assume a missing first-page row means creation failed.");
    const label = document.createElement("label");
    const acknowledge = document.createElement("input");
    acknowledge.type = "checkbox";
    label.append(acknowledge, document.createTextNode(" I checked the registry and resolved any existing invitation by cancellation or expiry."));
    const confirm = document.createElement("button");
    confirm.type = "button";
    confirm.className = "button";
    confirm.textContent = "Clear uncertainty";
    confirm.disabled = true;
    acknowledge.addEventListener("change", () => { confirm.disabled = !acknowledge.checked; });
    confirm.addEventListener("click", () => {
      if (!acknowledge.checked) return;
      uncertain = null;
      closeDialog();
      createStatus.textContent = "Registry reconciliation acknowledged. No mutation was retried.";
      updateControls();
    });
    content.append(label, confirm);
    showDialog("Resolve uncertain invitation creation", content);
  });
  form.addEventListener("reset", () => {
    closeDialog();
    for (const [name] of fields) input(name).setCustomValidity("");
    status.textContent = createStatus.textContent ? "Worksheet cleared. Previous creation outcome is unchanged." :
      "Worksheet cleared. Nothing has been submitted.";
    setTimeout(() => { if (catalog) updateDependentChoices(); else updateControls(); }, 0);
  });
  window.addEventListener("provisioning-catalog-cleared", () => { clearCatalog(); });
  window.addEventListener("provisioning-catalog-ready", (event) => {
    catalog = event.detail;
    choice("organization", Object.keys(catalog.organizations).sort());
    updateDependentChoices();
    note.textContent = "Choose an authorized organization and approved type/profile. These are administrative claims, not verified device attributes. Creation also requires named-user create permission and backend opt-in.";
  });
  window.addEventListener("operator-session-ready", (event) => {
    session = event.detail || null;
    updateControls();
  });
  window.addEventListener("operator-session-cleared", () => {
    session = null;
    invalidate();
  });
  window.addEventListener("hashchange", () => { if (!active()) invalidate(); });
  window.addEventListener("pagehide", () => {
    session = null;
    invalidate();
    form.reset();
    status.textContent = "";
  });
  updateControls();
})();
