(() => {
  const preflightForm = document.getElementById("provisioning-preflight");
  const form = document.getElementById("provisioning-draft");
  if (!preflightForm || !form) return;
  const preflightStatus = document.getElementById("provisioning-preflight-status");
  const preflightBlockers = document.getElementById("provisioning-preflight-blockers");
  const preflightOrganization = preflightForm.elements.namedItem("organization");
  const preflightMode = preflightForm.elements.namedItem("deployment_mode");
  const hostAddress = preflightForm.elements.namedItem("host_address");
  const backupCheck = document.getElementById("provisioning-backup-check");
  const preflightSubmit = preflightForm.querySelector('[type="submit"]');
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
  const text = (value) => typeof value === "string" && value.trim().length > 0 &&
    value.trim() === value && value.length <= 256 && !/[\0\r\n]/.test(value);
  let dialog = null;
  let catalog = null;
  let session = null;
  let pending = null;
  let uncertain = null;
  let secretVisible = false;
  let confirmedSummary = "";
  let preflightReady = false;
  let preflightPending = false;
  let preflightGeneration = 0;
  let preflightController = null;
  let preflightSaveController = null;
  let preflightExpiresAt = 0;
  let preflightExpiryTimer = null;

  const input = (name) => form.elements.namedItem(name);
  const active = () => location.hash === "#provisioning-adapters";
  const privateIPv4 = (value) => {
    if (!/^\d{1,3}(?:\.\d{1,3}){3}$/.test(value)) return false;
    const octets = value.split(".").map(Number);
    if (octets.length !== 4 || octets.some((octet) => !Number.isInteger(octet) || octet < 0 || octet > 255)) return false;
    return octets[0] === 10 || (octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31) ||
      (octets[0] === 192 && octets[1] === 168) ||
      (octets[0] === 100 && octets[1] >= 64 && octets[1] <= 127);
  };
  const preflightChecks = [
    ["dns_verified", "Verify the private dnsmasq answer from the target laptop."],
    ["acme_verified", "Configure narrowly scoped Route 53 DNS-01 automation."],
    ["tls_verified", "Verify the direct TLS 1.3 certificate and complete chain."],
    ["network_verified", "Restrict TCP 443 to the intended LAN/VPN; keep public ingress closed."],
    ["docker_verified", "Use an isolated Compose project and dedicated service volume."],
    ["listeners_verified", "Deploy and test separate administrative and device listeners."],
    ["package_verified", "Verify a signed, notarized package that installs the adapter/runtime."],
    ["recovery_verified", "Test invitation recovery, cancellation/revocation and audit review."],
  ];
  const developmentEnabled = () => /^[a-f0-9]{64}$/.test(catalog?.software_development_ca_fingerprint || "");
  const developmentMode = () => developmentEnabled() && preflightMode.value === "development";
  const developmentLabels = {
    dns_verified: " The development host name resolves to the intended private host from the endpoint.",
    tls_verified: " Direct TLS 1.3 verifies the exact endpoint address and the operator-configured private installation CA; certificate checks are not disabled.",
    network_verified: " The configured development ports are reachable only by the intended private LAN/VPN clients; public ingress remains closed.",
    docker_verified: " Each development organization has isolated retained state and exact supervised services; unrelated containers are untouched.",
    package_verified: " The engineering runtime and enrollment client match their approved checksums. Software-held keys are explicitly development-only.",
  };
  const originalCheckLabels = new Map(Object.keys(developmentLabels).map((name) => {
    const control = preflightForm.elements.namedItem(name);
    return [name, [...control.closest("label").childNodes].filter((node) => node !== control).map((node) => node.cloneNode(true))];
  }));
  const endpointHostname = preflightForm.querySelector('input[readonly][aria-readonly="true"]');
  const originalEndpointHostname = endpointHostname?.value;
  const developmentNotice = document.createElement("p");
  developmentNotice.textContent = "Development uses the operator-configured private installation CA and endpoint-local code delivery. No ACME, notarization, production readiness or runtime admission is claimed.";
  developmentNotice.hidden = true;
  preflightForm.querySelector(".provisioning-preflight-checks").before(developmentNotice);
  let developmentLabelsActive = false;

  function updateDevelopmentLabels() {
    const development = developmentMode();
    preflightForm.elements.namedItem("acme_verified").closest("label").hidden = development;
    developmentNotice.hidden = !development;
    if (development === developmentLabelsActive) return;
    developmentLabelsActive = development;
    for (const [name, label] of Object.entries(developmentLabels)) {
      const control = preflightForm.elements.namedItem(name);
      control.closest("label").replaceChildren(control, ...(development
        ? [document.createTextNode(label)] : originalCheckLabels.get(name).map((node) => node.cloneNode(true))));
    }
    if (endpointHostname) endpointHostname.value = development ? "Installation-specific private-CA endpoint" : originalEndpointHostname;
  }

  function updateDevelopmentChoices() {
    const delivery = preflightForm.elements.namedItem("delivery_channel");
    const enabled = developmentEnabled();
    for (const [select, value, label] of [
      [preflightMode, "development", "Local development / private installation CA"],
      [delivery, "development-endpoint-local", "Development: private endpoint-local code file"],
    ]) {
      const existing = select.querySelector(`option[value="${value}"]`);
      if (enabled && !existing) select.add(new Option(label, value));
      if (!enabled && existing) {
        const selected = select.value === value;
        existing.remove();
        if (selected) select.selectedIndex = 0;
      }
    }
    updateDevelopmentLabels();
  }
  const canCreate = () => active() && location.protocol === "https:" && session &&
    preflightReady &&
    preflightExpiresAt > Date.now() &&
    preflightOrganization.value === input("organization").value &&
    session.identity.permissions.includes("create") &&
    (session.identity.organizations.includes("*") || session.identity.organizations.includes(input("organization").value)) &&
    catalog?.gui_create_organizations?.includes(input("organization").value);

  function updateControls() {
    createButton.disabled = !canCreate() || Boolean(pending || uncertain || secretVisible);
    reconcileButton.hidden = !uncertain || Boolean(pending);
    preflightSubmit.disabled = preflightPending || !active() || location.protocol !== "https:" ||
      !session?.csrf || !session.identity.permissions.includes("create") ||
      !(session.identity.organizations.includes("*") || session.identity.organizations.includes(preflightOrganization.value)) ||
      !catalog?.organizations?.[preflightOrganization.value];
    for (const control of form.elements) {
      if (control !== createButton && control !== reconcileButton) control.disabled = Boolean(pending);
    }
    for (const control of preflightForm.elements) {
      if (control !== preflightSubmit) control.disabled = preflightPending;
    }
  }

  function clearPreflightExpiry() {
    if (preflightExpiryTimer !== null) clearTimeout(preflightExpiryTimer);
    preflightExpiryTimer = null;
    preflightExpiresAt = 0;
  }

  function setPreflightExpiry(updatedAt) {
    clearPreflightExpiry();
    preflightExpiresAt = Date.parse(updatedAt) + 30 * 24 * 60 * 60 * 1000;
    const expire = () => {
      const remaining = preflightExpiresAt - Date.now();
      if (remaining > 0) {
        preflightExpiryTimer = setTimeout(expire, Math.min(remaining, 2147483647));
        return;
      }
      preflightExpiryTimer = null;
      preflightReady = false;
      preflightStatus.dataset.state = "blocked";
      preflightStatus.textContent = "The saved readiness attestation has expired. Re-attest before creating invitations.";
      updateControls();
    };
    expire();
  }

  function cancelPreflightRequests() {
    ++preflightGeneration;
    preflightController?.abort();
    preflightController = null;
    preflightSaveController?.abort();
    preflightSaveController = null;
    preflightPending = false;
    preflightReady = false;
    clearPreflightExpiry();
  }

  function preflightPayload() {
    const payload = {
      organization: preflightOrganization.value,
      deployment_mode: preflightMode.value,
      host_label: preflightForm.elements.namedItem("host_label").value.trim(),
      host_address: hostAddress.value.trim(),
      delivery_channel: preflightForm.elements.namedItem("delivery_channel").value,
      dns_verified: preflightForm.elements.namedItem("dns_verified").checked,
      acme_verified: preflightForm.elements.namedItem("acme_verified").checked,
      tls_verified: preflightForm.elements.namedItem("tls_verified").checked,
      network_verified: preflightForm.elements.namedItem("network_verified").checked,
      docker_verified: preflightForm.elements.namedItem("docker_verified").checked,
      listeners_verified: preflightForm.elements.namedItem("listeners_verified").checked,
      package_verified: preflightForm.elements.namedItem("package_verified").checked,
      recovery_verified: preflightForm.elements.namedItem("recovery_verified").checked,
      backup_verified: preflightForm.elements.namedItem("backup_verified").checked,
    };
    if (developmentMode()) {
      payload.acme_verified = false;
      payload.private_ca_fingerprint = catalog.software_development_ca_fingerprint;
    }
    return payload;
  }

  function validPreflightRecord(record, organization) {
    const attestation = record?.attestation;
    return attestation && attestation.organization === organization &&
      (["pilot", "production"].includes(attestation.deployment_mode) ||
        (developmentEnabled() && attestation.deployment_mode === "development" && !attestation.acme_verified &&
          attestation.private_ca_fingerprint === catalog.software_development_ca_fingerprint)) &&
      text(attestation.host_label) && privateIPv4(attestation.host_address) &&
      ["pilot-secure-share", "pilot-verified-handoff", "production-portal", "development-endpoint-local"].includes(attestation.delivery_channel) &&
      [...preflightChecks.map(([name]) => name), "backup_verified"].every((name) => typeof attestation[name] === "boolean") &&
      Number.isSafeInteger(record.revision) && record.revision > 0 &&
      text(record.updated_by) && Number.isFinite(Date.parse(record.updated_at));
  }

  function populatePreflight(attestation) {
    preflightMode.value = attestation.deployment_mode;
    preflightForm.elements.namedItem("host_label").value = attestation.host_label;
    hostAddress.value = attestation.host_address;
    preflightForm.elements.namedItem("delivery_channel").value = attestation.delivery_channel;
    for (const [name] of preflightChecks) {
      preflightForm.elements.namedItem(name).checked = attestation[name];
    }
    preflightForm.elements.namedItem("backup_verified").checked = attestation.backup_verified;
    backupCheck.hidden = attestation.deployment_mode !== "production";
    updateDevelopmentLabels();
  }

  async function loadPreflight() {
    const organization = preflightOrganization.value;
    ++preflightGeneration;
    const current = preflightGeneration;
    preflightController?.abort();
    preflightController = null;
    preflightReady = false;
    clearPreflightExpiry();
    updateControls();
    if (!organization || !session || !active()) {
      preflightStatus.dataset.state = "blocked";
      preflightStatus.textContent = "Choose an authorized organization and sign in to load its service readiness record.";
      return;
    }
    const controller = new AbortController();
    preflightController = controller;
    preflightStatus.textContent = "Loading the saved organization readiness attestation…";
    try {
      const response = await fetch(`/api/enrollment/v1/preflight?organization=${encodeURIComponent(organization)}`, {
        cache: "no-store", credentials: "same-origin", redirect: "error", signal: controller.signal,
        headers: { Accept: "application/json" },
      });
      if (!response.ok || !response.headers.get("Content-Type")?.includes("application/json")) {
        throw new Error(`HTTP ${response.status} while reading preflight.`);
      }
      const result = await window.ZPRPageRuntime.readJSON(response);
      if (current !== preflightGeneration || controller.signal.aborted || !active()) return;
      if (result.ready === false && result.preflight === undefined) {
        preflightStatus.dataset.state = "blocked";
        preflightStatus.textContent = "No current readiness attestation exists for this organization. Attestations expire after 30 days; complete and submit the checks before preparing an invitation.";
        preflightBlockers.replaceChildren();
        updateControls();
        return;
      }
      const record = result.preflight;
      if (result.ready !== true || !validPreflightRecord(record, organization)) {
        throw new Error("Invalid readiness attestation response.");
      }
      populatePreflight(record.attestation);
      const blockers = preflightBlockersFor(preflightPayload());
      if (blockers.length) throw new Error("Saved readiness attestation is incomplete.");
      preflightBlockers.replaceChildren();
      preflightReady = true;
      setPreflightExpiry(record.updated_at);
      preflightStatus.dataset.state = "ready";
      preflightStatus.textContent = `Saved ${preflightMode.value} readiness attestation revision ${record.revision}, updated ${new Date(record.updated_at).toLocaleString()} by ${record.updated_by}. These are persisted operator attestations, not automated network tests.`;
    } catch (error) {
      if (current !== preflightGeneration || controller.signal.aborted) return;
      preflightStatus.dataset.state = "blocked";
      preflightStatus.textContent = `Could not load saved readiness: ${error.message}`;
    } finally {
      if (current === preflightGeneration) {
        preflightController = null;
        updateControls();
      }
    }
  }

  function preflightBlockersFor(payload) {
    const blockers = [];
    if (!payload.organization) blockers.push("Choose an authorized organization.");
    if (!payload.host_label) blockers.push("Enter the Docker host label.");
    if (!privateIPv4(payload.host_address)) blockers.push("Enter a valid private host IPv4 address.");
    for (const [name, message] of preflightChecks) {
      if (payload.deployment_mode === "development" && name === "acme_verified") continue;
      if (!payload[name]) blockers.push(payload.deployment_mode === "development" && developmentLabels[name]
        ? developmentLabels[name].trim() : message);
    }
    if (payload.deployment_mode === "production") {
      if (payload.delivery_channel !== "production-portal") blockers.push("Production requires an authenticated SSO/MFA code-delivery portal.");
      if (!payload.backup_verified) blockers.push("Production requires encrypted backups and a tested recovery process.");
    } else if (payload.deployment_mode === "development") {
      if (!developmentEnabled() || payload.private_ca_fingerprint !== catalog.software_development_ca_fingerprint || payload.acme_verified) {
        blockers.push("Development requires this service's explicitly configured private installation CA; do not attest ACME.");
      }
      if (payload.delivery_channel !== "development-endpoint-local") blockers.push("Development requires private endpoint-local code delivery.");
    } else if (payload.deployment_mode !== "pilot") {
      blockers.push("Choose a supported deployment mode.");
    } else if (!["pilot-secure-share", "pilot-verified-handoff"].includes(payload.delivery_channel)) {
      blockers.push("Choose a secure, separate code-delivery channel for the pilot.");
    }
    return blockers;
  }

  function updatePreflight(complete = false) {
    preflightReady = false;
    clearPreflightExpiry();
    preflightStatus.dataset.state = "blocked";
    backupCheck.hidden = preflightMode.value !== "production";
    updateDevelopmentLabels();
    hostAddress.setCustomValidity(hostAddress.value.trim() && !privateIPv4(hostAddress.value.trim())
      ? "Enter a private LAN or ZeroTier IPv4 address." : "");
    const payload = preflightPayload();
    const blockers = preflightBlockersFor(payload);
    preflightBlockers.replaceChildren(...blockers.map((message) => {
      const item = document.createElement("li");
      item.textContent = message;
      return item;
    }));
    if (complete && blockers.length === 0) {
      preflightStatus.dataset.state = "ready";
      preflightStatus.textContent = "All checks are complete. Submitting the organization-scoped attestation to the Enrollment Service…";
    } else {
      preflightStatus.textContent = blockers.length
        ? `Preflight incomplete: ${blockers.length} readiness blocker${blockers.length === 1 ? "" : "s"} remain. Invitation creation is locked.`
        : "All checks are complete. Select Evaluate readiness to attest and unlock the invitation worksheet.";
    }
    updateControls();
    return blockers;
  }

  function clearPreflightState() {
    preflightReady = false;
    clearPreflightExpiry();
    preflightStatus.dataset.state = "blocked";
    preflightStatus.textContent = "Invitation creation remains locked until this preflight is complete.";
    preflightBlockers.replaceChildren();
    backupCheck.hidden = true;
    hostAddress.setCustomValidity("");
    updateControls();
  }

  function clearPreflight() {
    preflightForm.reset();
    clearPreflightState();
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
    updateDevelopmentChoices();
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
    cancelPreflightRequests();
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
  preflightForm.addEventListener("input", () => {
    preflightController?.abort();
    preflightController = null;
    ++preflightGeneration;
    updatePreflight();
  });
  preflightForm.addEventListener("change", (event) => {
    if (event.target !== preflightOrganization) {
      preflightController?.abort();
      preflightController = null;
      ++preflightGeneration;
    }
    updatePreflight();
    if (event.target === preflightOrganization) void loadPreflight();
  });
  preflightForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    hostAddress.value = hostAddress.value.trim();
    const blockers = updatePreflight(true);
    if (blockers.length) return;
    if (!session?.csrf || preflightPending) {
      preflightStatus.textContent = "A signed-in operator session with create permission is required to save preflight.";
      return;
    }
    const payload = preflightPayload();
    const attempt = new AbortController();
    const generation = ++preflightGeneration;
    const submittingSession = session;
    preflightSaveController = attempt;
    preflightPending = true;
    preflightReady = false;
    updateControls();
    preflightStatus.textContent = "Saving organization readiness attestation to the Enrollment Service…";
    try {
      const result = await provisioningContract.postMutation(
        "/api/enrollment/v1/preflight", payload, session.csrf, attempt.signal, 200,
      );
      if (generation !== preflightGeneration || attempt.signal.aborted || !active() || session !== submittingSession) return;
      if (result.rejected) {
        preflightStatus.dataset.state = "blocked";
        preflightStatus.textContent = `The Enrollment Service rejected the readiness attestation (HTTP ${result.rejected}); invitation creation remains locked.`;
        return;
      }
      const record = result.value.preflight;
      if (!validPreflightRecord(record, payload.organization) ||
          Object.entries(payload).some(([key, value]) => record.attestation[key] !== value)) {
        throw new Error("The service response did not match the submitted readiness attestation.");
      }
      preflightReady = true;
      setPreflightExpiry(record.updated_at);
      preflightStatus.dataset.state = "ready";
      const expiresAt = new Date(Date.parse(record.updated_at) + 30 * 24 * 60 * 60 * 1000);
      preflightStatus.textContent = `Saved ${record.attestation.deployment_mode} readiness attestation revision ${record.revision} at ${new Date(record.updated_at).toLocaleString()} by ${record.updated_by}; expires ${expiresAt.toLocaleString()}. These are persisted operator attestations, not automated network tests.`;
    } catch (error) {
      if (generation !== preflightGeneration || attempt.signal.aborted || !active() || session !== submittingSession) return;
      preflightStatus.dataset.state = "blocked";
      preflightStatus.textContent = `Preflight save could not be confirmed: ${error.message}. Creation remains locked; repeating the same attestation is safe.`;
    } finally {
      if (generation === preflightGeneration) {
        preflightSaveController = null;
        preflightPending = false;
        updateControls();
      }
    }
  });
  preflightForm.addEventListener("reset", () => {
    cancelPreflightRequests();
    preflightReady = false;
    updateControls();
    setTimeout(clearPreflightState, 0);
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
    updateDevelopmentChoices();
    const selected = preflightOrganization.value;
    const organizations = Object.keys(catalog.organizations).sort();
    preflightOrganization.replaceChildren(new Option("Choose authorized organization", ""), ...organizations.map((name) => new Option(name, name)));
    preflightOrganization.value = organizations.includes(selected) ? selected : organizations.length === 1 ? organizations[0] : "";
    choice("organization", Object.keys(catalog.organizations).sort());
    updateDependentChoices();
    note.textContent = "Choose an authorized organization and approved type/profile. These are administrative claims, not verified device attributes. Creation also requires named-user create permission and backend opt-in.";
    void loadPreflight();
  });
  window.addEventListener("operator-session-ready", (event) => {
    session = event.detail || null;
    updateControls();
    void loadPreflight();
  });
  window.addEventListener("operator-session-cleared", () => {
    session = null;
    clearPreflight();
    invalidate();
  });
  window.addEventListener("hashchange", () => {
    if (active()) void loadPreflight();
    else invalidate();
  });
  window.addEventListener("pagehide", () => {
    session = null;
    clearPreflight();
    invalidate();
    form.reset();
    status.textContent = "";
  });
  updateControls();
})();
