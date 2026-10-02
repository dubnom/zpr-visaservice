let organizationCatalog = [];
let activeOrganizationID = "";
let selectedOrganizationID = "";
let organizationCatalogSignature = "";
let organizationRefreshTimer;
let organizationRefreshPromise;
let directoryEditorArtifact;
let directoryEditorOrganizationID = "";
let directoryEditorDirty = false;

function organizationEscape(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[character]);
}

function organizationItems(items, title, emptyLabel) {
  if (!items?.length) return `<section class="organization-section"><h3>${organizationEscape(title)}</h3><p class="organization-empty">No ${organizationEscape(emptyLabel)}.</p></section>`;
  return `<section class="organization-section"><h3>${organizationEscape(title)}</h3><div class="organization-list-items">${items.map((item) => `<div class="organization-item"><strong>${organizationEscape(item.name || item.uid || item.id)}</strong><small>${organizationEscape(item.description || [item.title, item.department, item.endpoint].filter(Boolean).join(" · "))}</small></div>`).join("")}</div></section>`;
}

function renderOrganizationList() {
  const selected = selectedOrganizationID || activeOrganizationID;
  document.getElementById("organization-count").textContent = `${organizationCatalog.length} profiles`;
  document.getElementById("organization-list").innerHTML = organizationCatalog.map((organization) => {
    const active = organization.id === activeOrganizationID;
    return `<button class="organization-card${organization.id === selected ? " selected" : ""}" type="button" data-organization-id="${organizationEscape(organization.id)}"><strong>${organizationEscape(organization.name)}${active ? " · ACTIVE" : ""}</strong><small>${organizationEscape(organization.directory.base_dn)} · ${organization.policies.length} policies · ${organization.services.length} services</small></button>`;
  }).join("");
  const organization = organizationCatalog.find((item) => item.id === selected) || organizationCatalog[0];
  if (organization) renderOrganizationDetails(organization);
}

function renderOrganizationDetails(organization) {
  selectedOrganizationID = organization.id;
  const isActive = organization.id === activeOrganizationID;
  const directory = organization.directory || {};
  document.getElementById("organization-title").textContent = organization.name;
  document.getElementById("organization-active-name").textContent = organizationCatalog.find((item) => item.id === activeOrganizationID)?.name || "Unavailable";
  document.getElementById("organization-active-id").textContent = activeOrganizationID;
  document.getElementById("organization-detail").innerHTML = `
    <div class="organization-summary"><span class="scenario-state ${isActive ? "completed" : "idle"}">${isActive ? "Active" : "Profile"}</span>${isActive ? "" : `<button type="button" data-activate-organization="${organizationEscape(organization.id)}">Activate organization</button>`}<span>${organizationEscape(organization.description)}</span></div>
    <div class="organization-sections">
      <section class="organization-section"><div class="organization-directory-heading"><h3>Directory · ${organizationEscape(directory.base_dn)}</h3><button class="quiet" type="button" data-edit-directory="${organizationEscape(organization.id)}">Edit LDAP seed</button></div><div class="organization-summary"><span>${(directory.departments || []).length} departments</span><span>${(directory.people || []).length} people</span><span>${(directory.groups || []).length} groups</span><span>LDAP seed: ${organizationEscape(directory.seed_mode)}</span></div><ldap-org-graph></ldap-org-graph></section>
      ${organizationItems(organization.policies || [], "Policies", "policy records")}
      ${organizationItems(organization.services || [], "Services", "services")}
    </div>`;
  document.querySelector("#organization-detail ldap-org-graph").directory = directory;
  renderOrganizationListSelection();
}

function setDirectoryEditorStatus(message, state = "") {
  const status = document.getElementById("directory-editor-status");
  status.textContent = message;
  status.dataset.state = state;
}

function updateDirectoryEditorActions() {
  const publish = document.getElementById("directory-editor-publish");
  const current = directoryEditorArtifact?.revision || 0;
  const published = directoryEditorArtifact?.published_revision || 0;
  publish.disabled = !directoryEditorArtifact || directoryEditorDirty || current === published;
}

async function openDirectoryEditor(organizationID) {
  const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organizationID)}/directory`, { cache: "no-store" });
  const artifact = await response.json();
  if (!response.ok) throw new Error(artifact.error || `HTTP ${response.status}`);
  directoryEditorArtifact = artifact;
  directoryEditorOrganizationID = organizationID;
  directoryEditorDirty = false;
  document.getElementById("directory-editor-title").textContent = `Edit ${organizationCatalog.find((item) => item.id === organizationID)?.name || organizationID} directory`;
  document.getElementById("directory-editor-base-dn").textContent = artifact.content.base_dn;
  document.getElementById("directory-editor-source").value = artifact.content.ldif;
  document.getElementById("directory-editor-summary").value = "";
  document.getElementById("directory-editor-dialog").showModal();
  await refreshDirectoryRevisions();
  setDirectoryEditorStatus(`Revision ${artifact.revision}${artifact.published_revision ? ` · published r${artifact.published_revision}` : " · not yet published"}.`);
  updateDirectoryEditorActions();
}

async function refreshDirectoryRevisions() {
  if (!directoryEditorArtifact) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(directoryEditorOrganizationID)}/directory/revisions`;
  const response = await fetch(path, { cache: "no-store" });
  const revisions = await response.json();
  if (!response.ok) throw new Error(revisions.error || `HTTP ${response.status}`);
  const selector = document.getElementById("directory-editor-revisions");
  selector.replaceChildren(...revisions.map((revision) => new Option(`r${revision.revision} · ${revision.summary || revision.author}`, String(revision.revision))));
  selector.disabled = revisions.length === 0;
}

async function saveDirectoryDraft() {
  const ldif = document.getElementById("directory-editor-source").value;
  const summary = document.getElementById("directory-editor-summary").value.trim();
  const path = `/api/simulator/organizations/${encodeURIComponent(directoryEditorOrganizationID)}/directory`;
  const response = await fetch(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ document: { base_dn: directoryEditorArtifact.content.base_dn, ldif }, expected_revision: directoryEditorArtifact.revision, summary }),
  });
  const revision = await response.json();
  if (!response.ok) throw new Error(revision.error || `HTTP ${response.status}`);
  directoryEditorArtifact = { ...directoryEditorArtifact, revision: revision.revision, content: revision.content, content_hash: revision.content_hash };
  directoryEditorDirty = false;
  document.getElementById("directory-editor-summary").value = "";
  setDirectoryEditorStatus(`Saved draft revision ${revision.revision}. Publish it for the next LDAP reseed.`, "saved");
  await refreshDirectoryRevisions();
  updateDirectoryEditorActions();
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
  setDirectoryEditorStatus(`Published r${result.artifact.published_revision}; applies on the next explicit LDAP reseed or rig restart.`, "saved");
  updateDirectoryEditorActions();
}

async function loadDirectoryRevision(revisionNumber) {
  if (!revisionNumber || !directoryEditorArtifact) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(directoryEditorOrganizationID)}/directory/revisions/${encodeURIComponent(revisionNumber)}`;
  const response = await fetch(path, { cache: "no-store" });
  const revision = await response.json();
  if (!response.ok) throw new Error(revision.error || `HTTP ${response.status}`);
  document.getElementById("directory-editor-source").value = revision.content.ldif;
  document.getElementById("directory-editor-summary").value = `Restore revision ${revision.revision}`;
  directoryEditorDirty = true;
  setDirectoryEditorStatus(`Viewing revision ${revision.revision}; save to create a new revision.`);
  updateDirectoryEditorActions();
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
      const signature = JSON.stringify({ active_id: nextActiveID, organizations: nextOrganizations });
      if (signature !== organizationCatalogSignature) {
        organizationCatalog = nextOrganizations;
        activeOrganizationID = nextActiveID;
        organizationCatalogSignature = signature;
        if (!organizationCatalog.some((item) => item.id === selectedOrganizationID)) selectedOrganizationID = activeOrganizationID;
        renderOrganizationList();
      }
      document.getElementById("organization-connection").textContent = `${organizationCatalog.length} profiles loaded`;
      error.hidden = true;
    } catch (failure) {
      error.textContent = failure.message || "Organization catalog unavailable";
      error.hidden = false;
      document.getElementById("organization-connection").textContent = "Organization service unavailable";
    }
  })();
  try { await organizationRefreshPromise; } finally { organizationRefreshPromise = undefined; }
}

const designButton = document.createElement("button");
designButton.className = "quiet";
designButton.id = "organization-design-assistant";
designButton.type = "button";
designButton.textContent = "Design with Claude";
const refreshButton = document.getElementById("organization-refresh");
const topActions = document.createElement("div");
topActions.className = "organization-top-actions";
refreshButton.before(topActions);
topActions.append(designButton, refreshButton);

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
    const source = document.getElementById("directory-editor-source");
    source.value = proposal.directory_ldif;
    source.dispatchEvent(new Event("input", { bubbles: true }));
    document.getElementById("directory-editor-summary").value = "Claude-assisted directory draft";
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

document.getElementById("organization-refresh").addEventListener("click", refreshOrganizations);
document.getElementById("organization-list").addEventListener("click", (event) => {
  const button = event.target.closest("[data-organization-id]");
  const organization = organizationCatalog.find((item) => item.id === button?.dataset.organizationId);
  if (organization) renderOrganizationDetails(organization);
});
document.getElementById("organization-detail").addEventListener("click", async (event) => {
  const activate = event.target.closest("[data-activate-organization]");
  if (activate) {
    activate.disabled = true;
    try {
      const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(activate.dataset.activateOrganization)}/activate`, { method: "POST" });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
      await refreshOrganizations();
    } catch (error) {
      const message = document.getElementById("organization-error");
      message.textContent = error.message || "Could not activate organization";
      message.hidden = false;
    } finally {
      activate.disabled = false;
    }
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
document.getElementById("directory-editor-source").addEventListener("input", () => {
  directoryEditorDirty = true;
  updateDirectoryEditorActions();
});
document.getElementById("directory-editor-save").addEventListener("click", async () => {
  try { await saveDirectoryDraft(); }
  catch (error) { setDirectoryEditorStatus(error.message || "Could not save directory.", "error"); }
});
document.getElementById("directory-editor-publish").addEventListener("click", async () => {
  try { await publishDirectoryRevision(); }
  catch (error) { setDirectoryEditorStatus(error.message || "Could not publish directory.", "error"); }
});
document.getElementById("directory-editor-revisions").addEventListener("change", async (event) => {
  try { await loadDirectoryRevision(event.target.value); }
  catch (error) { setDirectoryEditorStatus(error.message || "Could not load revision.", "error"); }
});
document.addEventListener("simulator:activate", (event) => {
  if (event.detail.path !== "/organizations.html" || organizationRefreshTimer) return;
  refreshOrganizations();
  organizationRefreshTimer = setInterval(refreshOrganizations, 5000);
});
document.addEventListener("simulator:deactivate", (event) => {
  if (event.detail.path !== "/organizations.html") return;
  clearInterval(organizationRefreshTimer);
  organizationRefreshTimer = undefined;
});