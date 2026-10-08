let organizationCatalog = [];
let activeOrganizationID = "";
let organizationActivationStatus = { state: "idle" };
let selectedOrganizationID = "";
let organizationCatalogSignature = "";
let organizationRefreshTimer;
let organizationRefreshInterval = 0;
let organizationRefreshPromise;
let directoryEditorArtifact;
let directoryEditorOrganizationID = "";
let directoryEditorDirty = false;
let organizationActivationInFlight = false;
let pendingOrganizationActivation = null;
let pendingOrganizationRestore = null;

function organizationEscape(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[character]);
}

function organizationItems(items, title, emptyLabel) {
  if (!items?.length) return `<section class="organization-section"><h3>${organizationEscape(title)}</h3><p class="organization-empty">No ${organizationEscape(emptyLabel)}.</p></section>`;
  return `<section class="organization-section"><h3>${organizationEscape(title)}</h3><div class="organization-list-items">${items.map((item) => `<div class="organization-item"><strong>${organizationEscape(item.name || item.uid || item.id)}</strong><small>${organizationEscape(item.description || [item.title, item.department, item.location, item.endpoint].filter(Boolean).join(" · "))}</small></div>`).join("")}</div></section>`;
}

function organizationRuntimeLabel(organization) {
  const runtime = organization.runtime || {};
  const nodes = runtime.nodes || [];
  const locations = [...new Set(nodes.map((node) => node.location).filter(Boolean))];
  return `${runtime.topology || "runtime unspecified"} · ${nodes.length} ${nodes.length === 1 ? "node" : "nodes"}${locations.length ? ` · ${locations.join(" / ")}` : ""}`;
}

function renderOrganizationList() {
  const selected = selectedOrganizationID || activeOrganizationID;
  document.getElementById("organization-list").innerHTML = organizationCatalog.map((organization) => {
    const active = organization.id === activeOrganizationID;
    return `<button class="organization-card${organization.id === selected ? " selected" : ""}" type="button" data-organization-id="${organizationEscape(organization.id)}"><strong>${organizationEscape(organization.name)}${active ? " · ACTIVE" : ""}</strong><small>${organizationEscape(organization.directory.base_dn)} · ${organizationEscape(organizationRuntimeLabel(organization))} · ${organization.policies.length} policies · ${organization.services.length} services</small></button>`;
  }).join("");
  const organization = organizationCatalog.find((item) => item.id === selected) || organizationCatalog[0];
  if (organization) renderOrganizationDetails(organization);
}

function renderOrganizationDetails(organization) {
  selectedOrganizationID = organization.id;
  const isActive = organization.id === activeOrganizationID;
  const activationBusy = organizationActivationInFlight || organizationActivationStatus.state === "resetting";
  const operationLabel = organizationActivationStatus.operation === "restore-base" ? "Restoring base" : "Resetting ZPR";
  const activationProgress = organizationActivationStatus.progress || "Preparing organization reset";
  const directory = organization.directory || {};
  document.getElementById("organization-title").textContent = organization.name;
  const heading = document.getElementById("organization-title").closest(".section-head");
  heading.querySelector("[data-organization-activation-control]")?.remove();
  const control = document.createElement("div");
  control.dataset.organizationActivationControl = "true";
  const activationAction = isActive
    ? '<span class="scenario-state completed">Active</span>'
    : `<button type="button" data-activate-organization="${organizationEscape(organization.id)}" ${activationBusy ? "disabled" : ""}>${activationBusy ? `${operationLabel} · ${organizationEscape(activationProgress)}` : "Activate organization"}</button>`;
  control.innerHTML = `${activationAction}<button class="quiet" type="button" data-restore-base="${organizationEscape(organization.id)}" ${activationBusy ? "disabled" : ""}>Restore base state</button>`;
  heading.append(control);
  document.getElementById("organization-active-name").textContent = organizationCatalog.find((item) => item.id === activeOrganizationID)?.name || "Unavailable";
  document.getElementById("organization-active-id").textContent = activeOrganizationID;
  document.getElementById("organization-detail").innerHTML = `
    <div class="organization-summary"><span>${organizationEscape(organization.description)}</span></div>
    <div id="organization-scenario-summary" class="organization-summary" aria-label="Organization scenarios">Loading scenarios</div>
    <div class="organization-sections">
      <section class="organization-section"><div class="organization-directory-heading"><h3>Directory · ${organizationEscape(directory.base_dn)}</h3><button class="quiet" type="button" data-edit-directory="${organizationEscape(organization.id)}">Edit LDAP seed</button></div><div class="organization-summary"><span>${(directory.departments || []).length} departments</span><span>${(directory.people || []).length} people</span><span>${(directory.groups || []).length} groups</span><span>LDAP seed: ${organizationEscape(directory.seed_mode)}</span><span>Runtime profile: ${organizationEscape(organizationRuntimeLabel(organization))}</span></div><ldap-org-graph></ldap-org-graph></section>
      ${organizationItems(organization.policies || [], "Policies", "policy records")}
      ${organizationItems(organization.services || [], "Services", "services")}
    </div>`;
  document.querySelector("#organization-detail ldap-org-graph").directory = directory;
  renderOrganizationListSelection();
  void loadOrganizationScenarioSummary(organization.id);
}

async function loadOrganizationScenarioSummary(organizationID) {
  try {
      const response = await fetch(`/api/simulator/scenarios?organization_id=${encodeURIComponent(organizationID)}`, { cache: "no-store" });
    if (!response.ok) throw new Error("unavailable");
    const catalog = await response.json();
    if (selectedOrganizationID !== organizationID) return;
    const summary = document.getElementById("organization-scenario-summary");
    summary.replaceChildren();
    const scenarios = (catalog.scenarios || []).filter((scenario) => scenario.organization_id === organizationID);
    if (!scenarios.length) { summary.textContent = "No scenarios"; return; }
    for (const scenario of scenarios) {
      const link = document.createElement("a");
        link.href = `/scenarios.html?organization_id=${encodeURIComponent(organizationID)}#${encodeURIComponent(scenario.id)}`;
      link.textContent = scenario.name || scenario.id;
      summary.append(link);
    }
  } catch {
    if (selectedOrganizationID === organizationID) document.getElementById("organization-scenario-summary").textContent = "Scenarios unavailable";
  }
}

const directoryEditorPage = window.ZPREditorPage;
const directoryEditorSource = document.getElementById("directory-editor-source");
const directoryEditorSurface = directoryEditorPage.createSourceSurface({
  source: directoryEditorSource,
  highlight: document.getElementById("directory-editor-highlight"),
  gutter: document.getElementById("directory-editor-gutter"),
  language: "ldif",
  label: "Directory",
});
const directoryEditorMenu = directoryEditorPage.createMenu({
  root: document.getElementById("directory-editor-actions"),
  toggle: document.getElementById("directory-editor-files-toggle"),
  menu: document.getElementById("directory-editor-file-menu"),
});
const directoryEditorHistory = directoryEditorPage.createHistory({
  menu: document.getElementById("directory-editor-history-menu"),
  list: document.getElementById("directory-editor-history"),
  count: document.getElementById("directory-editor-history-count"),
  isAvailable: () => Boolean(directoryEditorArtifact),
});
let directoryEditorSaved = "";
let directoryEditorSummary = "";
let directoryEditorRevisions = [];
let directoryEditorViewing = 0;

function setDirectoryEditorStatus(message, state = "") {
  directoryEditorPage.setStatus(document.getElementById("directory-editor-status"), message, state === "saved" ? "success" : state);
}

function updateDirectoryEditorActions() {
  directoryEditorDirty = directoryEditorSource.value !== directoryEditorSaved;
  const current = directoryEditorArtifact?.revision || 0;
  const published = directoryEditorArtifact?.published_revision || 0;
  document.getElementById("directory-editor-publish").disabled = !directoryEditorArtifact || directoryEditorDirty || current === published;
  document.getElementById("directory-editor-discard").disabled = !directoryEditorDirty;
  const organization = organizationCatalog.find((item) => item.id === directoryEditorOrganizationID);
  const revision = directoryEditorViewing || current;
  directoryEditorPage.renderIdentity({
    title: document.getElementById("directory-editor-title"),
    version: document.getElementById("directory-editor-revision-label"),
    modified: document.getElementById("directory-editor-modified"),
  }, {
    name: directoryEditorArtifact ? `${organization?.name || directoryEditorOrganizationID} directory` : "",
    label: directoryEditorArtifact ? `Version ${revision}${directoryEditorViewing ? "" : published ? (published === current ? " · published" : ` · published v${published}`) : " · not published"}` : "",
    tooltip: directoryEditorArtifact?.content?.base_dn || "",
    dirty: directoryEditorDirty,
  });
}

function renderDirectoryEditor() {
  directoryEditorSurface.render();
  updateDirectoryEditorActions();
}

function closeDirectoryEditor() {
  const dialog = document.getElementById("directory-editor-dialog");
  if (dialog.open) dialog.close();
}

async function openDirectoryEditor(organizationID) {
  const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organizationID)}/directory`, { cache: "no-store" });
  const artifact = await response.json();
  if (!response.ok) throw new Error(artifact.error || `HTTP ${response.status}`);
  directoryEditorArtifact = artifact;
  directoryEditorOrganizationID = organizationID;
  directoryEditorViewing = 0;
  directoryEditorSummary = "";
  directoryEditorSource.value = directoryEditorSaved = artifact.content.ldif;
  setDirectoryEditorStatus("");
  const dialog = document.getElementById("directory-editor-dialog");
  if (!dialog.open) dialog.show();
  window.scrollTo({ top: 0 });
  renderDirectoryEditor();
  await refreshDirectoryRevisions();
}

async function refreshDirectoryRevisions() {
  if (!directoryEditorArtifact) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(directoryEditorOrganizationID)}/directory/revisions`;
  const response = await fetch(path, { cache: "no-store" });
  const revisions = await response.json();
  if (!response.ok) throw new Error(revisions.error || `HTTP ${response.status}`);
  directoryEditorRevisions = revisions.map((revision) => ({ ...revision, number: revision.revision })).sort((left, right) => right.number - left.number);
  renderDirectoryHistory();
}

function renderDirectoryHistory() {
  directoryEditorHistory.render(directoryEditorRevisions, {
    current: directoryEditorViewing || directoryEditorArtifact?.revision,
    detail: (revision) => revision.summary || "",
    meta: (revision) => [revision.author, revision.created_at ? new Date(revision.created_at).toLocaleString() : ""].filter(Boolean).join(" · "),
    onSelect: (revision) => {
      loadDirectoryRevision(revision.number).catch((error) => setDirectoryEditorStatus(error.message || "Could not load revision.", "error"));
    },
  });
}

async function saveDirectoryDraft() {
  const path = `/api/simulator/organizations/${encodeURIComponent(directoryEditorOrganizationID)}/directory`;
  const response = await fetch(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ document: { base_dn: directoryEditorArtifact.content.base_dn, ldif: directoryEditorSource.value }, expected_revision: directoryEditorArtifact.revision, summary: directoryEditorSummary || "Updated directory draft" }),
  });
  const revision = await response.json();
  if (!response.ok) throw new Error(revision.error || `HTTP ${response.status}`);
  directoryEditorArtifact = { ...directoryEditorArtifact, revision: revision.revision, content: revision.content, content_hash: revision.content_hash };
  directoryEditorSaved = directoryEditorSource.value;
  directoryEditorSummary = "";
  directoryEditorViewing = 0;
  setDirectoryEditorStatus(`Saved version ${revision.revision}. Publish it for the next LDAP reseed.`, "saved");
  renderDirectoryEditor();
  await refreshDirectoryRevisions();
}

async function publishDirectoryRevision() {
  if (!directoryEditorArtifact || directoryEditorDirty) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(directoryEditorOrganizationID)}/directory/publish`;
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_revision: directoryEditorArtifact.revision }),
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
  directoryEditorArtifact = result.artifact;
  setDirectoryEditorStatus(`Published version ${result.artifact.published_revision}; applies on the next explicit LDAP reseed or rig restart.`, "saved");
  updateDirectoryEditorActions();
}

async function loadDirectoryRevision(revisionNumber) {
  if (!revisionNumber || !directoryEditorArtifact) return;
  if (directoryEditorDirty && !confirm("Discard unsaved directory changes?")) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(directoryEditorOrganizationID)}/directory/revisions/${encodeURIComponent(revisionNumber)}`;
  const response = await fetch(path, { cache: "no-store" });
  const revision = await response.json();
  if (!response.ok) throw new Error(revision.error || `HTTP ${response.status}`);
  const current = Number(revision.revision) === Number(directoryEditorArtifact.revision);
  directoryEditorViewing = current ? 0 : Number(revision.revision);
  directoryEditorSource.value = revision.content.ldif;
  directoryEditorSaved = current ? revision.content.ldif : "";
  directoryEditorSummary = current ? "" : `Restore version ${revision.revision}`;
  setDirectoryEditorStatus("");
  renderDirectoryEditor();
  renderDirectoryHistory();
}

function renderOrganizationListSelection() {
  document.querySelectorAll("[data-organization-id]").forEach((button) => {
    button.classList.toggle("selected", button.dataset.organizationId === selectedOrganizationID);
  });
}

async function refreshOrganizations() {
  if (organizationRefreshPromise) return organizationRefreshPromise;
  organizationRefreshPromise = (async () => {
    const error = document.getElementById("organization-error");
    try {
      const response = await fetch("/api/simulator/organizations", { cache: "no-store" });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
      const nextOrganizations = data.organizations || [];
      const nextActiveID = data.active_id || "";
      organizationActivationStatus = data.activation || { state: "idle" };
      const signature = JSON.stringify({ active_id: nextActiveID, organizations: nextOrganizations, activation: organizationActivationStatus });
      if (signature !== organizationCatalogSignature) {
        organizationCatalog = nextOrganizations;
        activeOrganizationID = nextActiveID;
        organizationCatalogSignature = signature;
        if (!organizationCatalog.some((item) => item.id === selectedOrganizationID)) selectedOrganizationID = activeOrganizationID;
        renderOrganizationList();
      }
      document.getElementById("organization-connection").textContent = organizationActivationStatus.state === "resetting"
        ? `Resetting ZPR · ${organizationActivationStatus.progress || "Preparing organization reset"}`
        : "Directory ready";
      error.hidden = organizationActivationStatus.state !== "failed";
      if (!error.hidden) error.textContent = organizationActivationStatus.error || "Organization activation failed";
    } catch (failure) {
      error.textContent = failure.message || "Organization catalog unavailable";
      error.hidden = false;
      document.getElementById("organization-connection").textContent = "Organization service unavailable";
    }
  })();
  try { await organizationRefreshPromise; } finally { organizationRefreshPromise = undefined; }
  if (organizationRefreshTimer) scheduleOrganizationRefresh();
}

function scheduleOrganizationRefresh() {
  const interval = organizationActivationStatus.state === "resetting" ? 1000 : 5000;
  if (organizationRefreshTimer && organizationRefreshInterval === interval) return;
  clearInterval(organizationRefreshTimer);
  organizationRefreshInterval = interval;
  organizationRefreshTimer = setInterval(refreshOrganizations, interval);
}

const designButton = document.createElement("button");
designButton.className = "quiet";
designButton.id = "organization-design-assistant";
designButton.type = "button";
designButton.textContent = "Design with Claude";
const resetLogLink = document.createElement("a");
resetLogLink.className = "organization-reset-log";
resetLogLink.href = "/api/simulator/activation-log";
resetLogLink.target = "_blank";
resetLogLink.rel = "noopener noreferrer";
resetLogLink.textContent = "Open reset log";
const refreshButton = document.getElementById("organization-refresh");
const topActions = document.createElement("div");
topActions.className = "organization-top-actions";
refreshButton.before(topActions);
topActions.append(designButton, resetLogLink, refreshButton);

const organizationDesignDialog = document.createElement("dialog");
organizationDesignDialog.id = "organization-design-dialog";
organizationDesignDialog.className = "design-assistant-dialog";
organizationDesignDialog.setAttribute("aria-labelledby", "organization-design-title");
organizationDesignDialog.innerHTML = `<header class="design-assistant-dialog-head"><div><p class="eyebrow">ORGANIZATION DESIGN</p><h2 id="organization-design-title">Design with Claude</h2></div><button class="quiet" type="button" aria-label="Close assistant" data-close-design-assistant>×</button></header><p class="design-assistant-dialog-context" id="organization-design-context"></p><div id="organization-assistant-slot"></div>`;
document.body.append(organizationDesignDialog);
let organizationAssistantID = "";
const organizationAssistant = window.mountSimulatorDesignAssistant("organization-assistant-slot", {
  scope: "organization",
  applyLabel: "Apply LDIF draft",
  emptyMessage: "Ask for organization, identity, group, service, or LDAP seed design advice.",
  getContext: () => ({ organization_id: organizationAssistantID }),
  onApply: async (proposal) => {
    if (!proposal.directory_ldif) throw new Error("Claude did not return an LDIF proposal.");
    await openDirectoryEditor(organizationAssistantID);
    directoryEditorSource.value = proposal.directory_ldif;
    directoryEditorSource.dispatchEvent(new Event("input", { bubbles: true }));
    directoryEditorSummary = "Claude-assisted directory draft";
    setDirectoryEditorStatus("Claude proposal loaded. Review it before saving or publishing.");
  },
});

designButton.addEventListener("click", () => {
  organizationAssistantID = selectedOrganizationID || activeOrganizationID;
  const organization = organizationCatalog.find((item) => item.id === organizationAssistantID);
  document.getElementById("organization-design-context").textContent = organization
    ? `${organization.name} · ${organization.directory.base_dn}`
    : "Select an organization profile first.";
  organizationAssistant.reset();
  organizationDesignDialog.showModal();
});
organizationDesignDialog.querySelector("[data-close-design-assistant]").addEventListener("click", () => organizationDesignDialog.close());

const organizationSwitchDialog = document.createElement("dialog");
organizationSwitchDialog.id = "organization-switch-dialog";
organizationSwitchDialog.className = "organization-switch-dialog";
organizationSwitchDialog.setAttribute("aria-labelledby", "organization-switch-title");
organizationSwitchDialog.setAttribute("aria-describedby", "organization-switch-warning");
organizationSwitchDialog.innerHTML = `<h2 id="organization-switch-title">Switch organization?</h2><p id="organization-switch-target"></p><p id="organization-switch-warning">Switching organizations resets the simulated ZPR environment and can interrupt connections and workloads. This is not just a change of view. Finish or cancel any running scenario and log out all machine users before continuing.</p><p id="organization-switch-error" role="alert" hidden></p><div class="organization-switch-actions"><button class="quiet" type="button" data-cancel-organization-switch autofocus>Cancel</button><button type="button" data-confirm-organization-switch>Switch organization</button></div>`;
document.body.append(organizationSwitchDialog);
const organizationSwitchConfirm = organizationSwitchDialog.querySelector("[data-confirm-organization-switch]");
const organizationSwitchError = document.getElementById("organization-switch-error");
organizationSwitchDialog.querySelector("[data-cancel-organization-switch]").addEventListener("click", () => organizationSwitchDialog.close());
organizationSwitchDialog.addEventListener("close", () => { pendingOrganizationActivation = null; });

const organizationRestoreDialog = document.createElement("dialog");
organizationRestoreDialog.id = "organization-restore-dialog";
organizationRestoreDialog.className = "organization-switch-dialog";
organizationRestoreDialog.setAttribute("aria-labelledby", "organization-restore-title");
organizationRestoreDialog.setAttribute("aria-describedby", "organization-restore-warning");
organizationRestoreDialog.innerHTML = `<h2 id="organization-restore-title">Restore base state?</h2><p id="organization-restore-target"></p><p id="organization-restore-warning">This replaces the selected organization's saved policy, assertions, scenarios, and LDAP edits with its bundled defaults, then reseeds its directory. The current state is backed up locally. Its runtime will restart and may interrupt connections.</p><p id="organization-restore-error" role="alert" hidden></p><div class="organization-switch-actions"><button class="quiet" type="button" data-cancel-organization-restore autofocus>Cancel</button><button type="button" data-confirm-organization-restore>Restore base state</button></div>`;
document.body.append(organizationRestoreDialog);
const organizationRestoreConfirm = organizationRestoreDialog.querySelector("[data-confirm-organization-restore]");
const organizationRestoreError = document.getElementById("organization-restore-error");
organizationRestoreDialog.querySelector("[data-cancel-organization-restore]").addEventListener("click", () => organizationRestoreDialog.close());
organizationRestoreDialog.addEventListener("close", () => { pendingOrganizationRestore = null; });

organizationRestoreConfirm.addEventListener("click", async () => {
  const organizationID = pendingOrganizationRestore;
  if (!organizationID || organizationActivationInFlight) return;
  if (organizationActivationStatus.state === "resetting" || !organizationCatalog.some((item) => item.id === organizationID)) {
    organizationRestoreError.textContent = "The organization state changed while this dialog was open. Cancel and review the current state before trying again.";
    organizationRestoreError.hidden = false;
    organizationRestoreConfirm.disabled = true;
    return;
  }
  organizationActivationInFlight = true;
  organizationRestoreDialog.close();
  renderOrganizationList();
  try {
    const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organizationID)}/restore-base`, { method: "POST" });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
    organizationActivationStatus = result.activation || { state: "resetting", operation: "restore-base", progress: "Preparing organization base restore" };
    renderOrganizationList();
    await refreshOrganizations();
  } catch (error) {
    const message = document.getElementById("organization-error");
    message.textContent = error.message || "Could not restore organization base state";
    message.hidden = false;
  } finally {
    organizationActivationInFlight = false;
    renderOrganizationList();
  }
});

organizationSwitchConfirm.addEventListener("click", async () => {
  const approval = pendingOrganizationActivation;
  if (!approval || organizationActivationInFlight) return;
  if (activeOrganizationID !== approval.from || organizationActivationStatus.state === "resetting" || !organizationCatalog.some((item) => item.id === approval.to)) {
    organizationSwitchError.textContent = "The organization state changed while this dialog was open. Cancel and review the current organization before trying again.";
    organizationSwitchError.hidden = false;
    organizationSwitchConfirm.disabled = true;
    return;
  }
  organizationActivationInFlight = true;
  organizationSwitchDialog.close();
  renderOrganizationList();
  try {
    const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(approval.to)}/activate`, { method: "POST" });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
    organizationActivationStatus = result.activation || { state: "resetting", progress: "Preparing organization reset" };
    renderOrganizationList();
    await refreshOrganizations();
  } catch (error) {
    const message = document.getElementById("organization-error");
    message.textContent = error.message || "Could not activate organization";
    message.hidden = false;
  } finally {
    organizationActivationInFlight = false;
    renderOrganizationList();
  }
});

document.getElementById("organization-refresh").addEventListener("click", refreshOrganizations);
document.getElementById("organization-list").addEventListener("click", (event) => {
  const button = event.target.closest("[data-organization-id]");
  const organization = organizationCatalog.find((item) => item.id === button?.dataset.organizationId);
  if (organization) renderOrganizationDetails(organization);
});
document.querySelector(".organization-detail").addEventListener("click", async (event) => {
  const restore = event.target.closest("[data-restore-base]");
  if (restore) {
    if (organizationActivationInFlight || organizationActivationStatus.state === "resetting") return;
    const target = organizationCatalog.find((item) => item.id === restore.dataset.restoreBase);
    if (!target) return;
    pendingOrganizationRestore = target.id;
    document.getElementById("organization-restore-target").textContent = `Restore ${target.name} to its checked-in profile defaults?`;
    organizationRestoreError.hidden = true;
    organizationRestoreConfirm.disabled = false;
    organizationRestoreDialog.showModal();
    return;
  }
  const activate = event.target.closest("[data-activate-organization]");
  if (activate) {
    if (organizationActivationInFlight || organizationActivationStatus.state === "resetting") return;
    const target = organizationCatalog.find((item) => item.id === activate.dataset.activateOrganization);
    if (!target || target.id === activeOrganizationID) return;
    const current = organizationCatalog.find((item) => item.id === activeOrganizationID);
    pendingOrganizationActivation = { from: activeOrganizationID, to: target.id };
    document.getElementById("organization-switch-target").textContent = `Switch from ${current?.name || activeOrganizationID || "the current organization"} to ${target.name}?`;
    organizationSwitchError.hidden = true;
    organizationSwitchConfirm.disabled = false;
    organizationSwitchDialog.showModal();
    return;
  }
  const button = event.target.closest("[data-edit-directory]");
  if (!button) return;
  try { await openDirectoryEditor(button.dataset.editDirectory); }
  catch (error) {
    const message = document.getElementById("organization-error");
    message.textContent = error.message || "Could not open LDAP directory editor";
    message.hidden = false;
  }
});
directoryEditorSource.addEventListener("input", () => {
  directoryEditorSurface.setDiagnostic(null);
  setDirectoryEditorStatus("");
  renderDirectoryEditor();
});
directoryEditorSource.addEventListener("keydown", (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
    event.preventDefault();
    document.getElementById("directory-editor-save").click();
  }
});
document.getElementById("directory-editor-save").addEventListener("click", async () => {
  directoryEditorMenu.setOpen(false);
  try { await saveDirectoryDraft(); }
  catch (error) { setDirectoryEditorStatus(error.message || "Could not save directory.", "error"); }
});
document.getElementById("directory-editor-publish").addEventListener("click", async () => {
  directoryEditorMenu.setOpen(false);
  try { await publishDirectoryRevision(); }
  catch (error) { setDirectoryEditorStatus(error.message || "Could not publish directory.", "error"); }
});
document.getElementById("directory-editor-discard").addEventListener("click", () => {
  directoryEditorMenu.setOpen(false);
  if (!directoryEditorDirty || !confirm("Discard unsaved directory changes?")) return;
  directoryEditorSource.value = directoryEditorArtifact.content.ldif;
  directoryEditorSaved = directoryEditorSource.value;
  directoryEditorViewing = 0;
  directoryEditorSummary = "";
  renderDirectoryEditor();
  renderDirectoryHistory();
});
document.getElementById("directory-editor-close").addEventListener("click", () => {
  directoryEditorMenu.setOpen(false);
  closeDirectoryEditor();
});
document.getElementById("directory-editor-form").addEventListener("submit", (event) => {
  if (directoryEditorDirty && !confirm("Close the directory editor and discard unsaved changes?")) event.preventDefault();
});
document.getElementById("directory-editor-dialog").addEventListener("close", () => {
  directoryEditorArtifact = null;
  directoryEditorSource.value = directoryEditorSaved = "";
  directoryEditorDirty = false;
});
document.addEventListener("simulator:activate", (event) => {
  if (event.detail.path !== "/organizations.html" || organizationRefreshTimer) return;
  refreshOrganizations();
  organizationRefreshInterval = 5000;
  organizationRefreshTimer = setInterval(refreshOrganizations, organizationRefreshInterval);
});
document.addEventListener("simulator:deactivate", (event) => {
  if (event.detail.path !== "/organizations.html") return;
  if (organizationSwitchDialog.open) organizationSwitchDialog.close();
  clearInterval(organizationRefreshTimer);
  organizationRefreshTimer = undefined;
  organizationRefreshInterval = 0;
});