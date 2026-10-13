(() => {
  const dialog = document.createElement("dialog");
  dialog.id = "organization-profile-editor";
  dialog.className = "sim-editor-page section-block policy-page editor-page config-editor-panel";
  dialog.setAttribute("aria-labelledby", "organization-profile-title");
  dialog.innerHTML = `<div class="policy-toolbar"><div class="policy-identity"><strong id="organization-profile-title">Organization draft</strong><span id="organization-profile-revision"></span><span id="organization-profile-modified" class="policy-modified-indicator" hidden>Modified</span></div><button class="quiet" type="button" data-profile-close aria-label="Close organization draft" title="Close organization draft">×</button></div>
    <div class="editor-page-workbench"><div class="editor-page-main">
    <div class="policy-editor-tools"><div class="policy-editor-utilities"><button type="button" data-profile-save>Save draft</button><button type="button" data-profile-validate>Validate profile</button><button type="button" data-profile-reload>Reload draft</button><button type="button" data-profile-definitions hidden>Edit supporting definitions</button><button type="button" data-profile-export hidden>Export saved definitions</button><details class="policy-history-menu"><summary class="button">History</summary><div id="organization-profile-history" class="policy-history"></div></details></div></div>
    <p id="organization-profile-status" role="status" aria-live="polite"></p>
    <p>Draft only. Saving does not change bundled files, linked documents, the active organization or running machines. Profile validation is not runtime readiness.</p>

    <nav class="status-tabs organization-editor-tabs" role="tablist" aria-label="Organization draft sections">
      <button type="button" role="tab" class="active" data-editor-tab="overview" aria-selected="true" aria-controls="section-panel-overview">Overview</button>
      <button type="button" role="tab" data-editor-tab="sites" aria-selected="false" aria-controls="section-panel-sites">Sites &amp; Topology <span class="tab-badge" id="badge-sites-count">0</span></button>
      <button type="button" role="tab" data-editor-tab="directory" aria-selected="false" aria-controls="section-panel-directory">Directory <span class="tab-badge" id="badge-directory-count">0</span></button>
      <button type="button" role="tab" data-editor-tab="services" aria-selected="false" aria-controls="section-panel-services">Services &amp; Workloads <span class="tab-badge" id="badge-services-count">0</span></button>
      <button type="button" role="tab" data-editor-tab="source" aria-selected="false" aria-controls="section-panel-source">Advanced JSON</button>
    </nav>

    <div id="section-panel-overview" class="organization-section-panel" data-panel="overview" role="tabpanel">
      <div class="assistant-settings">
        <label>Name<input id="organization-profile-name"></label>
        <label>Description<input id="organization-profile-description"></label>
      </div>
      <div class="assistant-settings">
        <label>Directory Base DN<input id="organization-profile-base-dn" placeholder="dc=example,dc=test"></label>
        <label>Runtime Driver
          <select id="organization-profile-driver">
            <option value="">(unspecified)</option>
            <option value="docker-multinode">docker-multinode</option>
            <option value="linux-one-node">linux-one-node</option>
          </select>
        </label>
        <label>Topology
          <select id="organization-profile-topology">
            <option value="">(unspecified)</option>
            <option value="multi-node">multi-node</option>
            <option value="single-node">single-node</option>
          </select>
        </label>
      </div>
      <div class="organization-metrics-strip">
        <div class="metric-card"><span class="metric-label">Sites / Nodes</span><strong id="metric-sites-count">0</strong></div>
        <div class="metric-card"><span class="metric-label">Departments</span><strong id="metric-depts-count">0</strong></div>
        <div class="metric-card"><span class="metric-label">People</span><strong id="metric-people-count">0</strong></div>
        <div class="metric-card"><span class="metric-label">Groups</span><strong id="metric-groups-count">0</strong></div>
        <div class="metric-card"><span class="metric-label">Services</span><strong id="metric-services-count">0</strong></div>
      </div>
    </div>

    <div id="section-panel-sites" class="organization-section-panel" data-panel="sites" role="tabpanel" hidden>
      <div class="section-subhead">
        <h3>Sites &amp; Runtime Nodes</h3>
        <button type="button" class="button small" data-action="add-site">+ Add Site</button>
      </div>
      <div class="table-scroll">
        <table id="table-sites">
          <thead>
            <tr>
              <th>ID</th>
              <th>Location</th>
              <th>Substrate IPv4</th>
              <th>ZPR IPv6</th>
              <th>Lat</th>
              <th>Long</th>
              <th>Note</th>
              <th>Action</th>
            </tr>
          </thead>
          <tbody id="tbody-sites"></tbody>
        </table>
      </div>
      <div class="topology-preview-box">
        <h4 class="preview-title">Topology Preview</h4>
        <div class="topology-nodes-grid" id="topology-nodes-preview"></div>
      </div>
    </div>

    <div id="section-panel-directory" class="organization-section-panel" data-panel="directory" role="tabpanel" hidden>
      <div class="assistant-settings">
        <label>Base DN<input id="directory-base-dn-field" placeholder="dc=example,dc=test"></label>
        <label>Seed Mode
          <select id="directory-seed-mode-field">
            <option value="ldif">ldif</option>
            <option value="pregen">pregen</option>
            <option value="">(unspecified)</option>
          </select>
        </label>
        <label>Health UID<input id="directory-health-uid-field" placeholder="e.g. maya.brooks"></label>
      </div>

      <details class="directory-subsection" open>
        <summary class="section-subhead-summary"><span>Departments</span> <button type="button" class="button small" data-action="add-department">+ Add Department</button></summary>
        <div class="table-scroll">
          <table id="table-departments">
            <thead><tr><th>Department Name</th><th>Parent Department</th><th>Action</th></tr></thead>
            <tbody id="tbody-departments"></tbody>
          </table>
        </div>
      </details>

      <details class="directory-subsection" open>
        <summary class="section-subhead-summary"><span>People</span> <button type="button" class="button small" data-action="add-person">+ Add Person</button></summary>
        <div class="table-scroll">
          <table id="table-people">
            <thead><tr><th>UID</th><th>Name</th><th>Email</th><th>Department</th><th>Title</th><th>Location</th><th>Action</th></tr></thead>
            <tbody id="tbody-people"></tbody>
          </table>
        </div>
      </details>

      <details class="directory-subsection" open>
        <summary class="section-subhead-summary"><span>Groups</span> <button type="button" class="button small" data-action="add-group">+ Add Group</button></summary>
        <div class="table-scroll">
          <table id="table-groups">
            <thead><tr><th>Group Name</th><th>Members (comma-separated UIDs)</th><th>Action</th></tr></thead>
            <tbody id="tbody-groups"></tbody>
          </table>
        </div>
      </details>

      <details class="directory-subsection">
        <summary class="section-subhead-summary"><span>Machine Owners</span> <button type="button" class="button small" data-action="add-machine-owner">+ Add Machine Owner</button></summary>
        <div class="table-scroll">
          <table id="table-machine-owners">
            <thead><tr><th>Machine ID</th><th>Owner UIDs</th><th>Action</th></tr></thead>
            <tbody id="tbody-machine-owners"></tbody>
          </table>
        </div>
      </details>

      <div class="directory-graph-preview-box">
        <h4 class="preview-title">Directory Hierarchy Preview</h4>
        <ldap-org-graph id="profile-ldap-graph"></ldap-org-graph>
      </div>
    </div>

    <div id="section-panel-services" class="organization-section-panel" data-panel="services" role="tabpanel" hidden>
      <div class="section-subhead">
        <h3>Services</h3>
        <button type="button" class="button small" data-action="add-service">+ Add Service</button>
      </div>
      <div class="table-scroll">
        <table id="table-services">
          <thead><tr><th>Name</th><th>Kind</th><th>Endpoint</th><th>Policy ID</th><th>Actor CN</th><th>Description</th><th>Action</th></tr></thead>
          <tbody id="tbody-services"></tbody>
        </table>
      </div>

      <details class="services-subsection" open>
        <summary class="section-subhead-summary"><span>Policy Test Services</span> <button type="button" class="button small" data-action="add-test-service">+ Add Test Service</button></summary>
        <div class="table-scroll">
          <table id="table-test-services">
            <thead><tr><th>ID</th><th>Name</th><th>Actor CN</th><th>Protocol</th><th>Port</th><th>ZPR Address</th><th>Action</th></tr></thead>
            <tbody id="tbody-test-services"></tbody>
          </table>
        </div>
      </details>

      <details class="services-subsection">
        <summary class="section-subhead-summary"><span>Workload Services</span> <button type="button" class="button small" data-action="add-workload-service">+ Add Workload Mapping</button></summary>
        <div class="table-scroll">
          <table id="table-workload-services">
            <thead><tr><th>Service Key</th><th>Workloads (comma-separated)</th><th>Action</th></tr></thead>
            <tbody id="tbody-workload-services"></tbody>
          </table>
        </div>
      </details>
    </div>

    <details id="organization-profile-advanced" class="organization-profile-advanced" open>
      <summary class="button">Advanced JSON source</summary>
      <div class="config-source-editor"><div id="organization-profile-gutter" class="config-source-gutter"></div><div class="config-code-surface"><pre id="organization-profile-highlight" aria-hidden="true"></pre><textarea id="organization-profile-source" class="zpr-config-source" aria-label="Organization profile JSON" spellcheck="false" wrap="off"></textarea></div></div>
    </details>

    </div><div id="organization-profile-assistant"></div></div>`;
  document.body.append(dialog);
  const source = dialog.querySelector("#organization-profile-source");
  const name = dialog.querySelector("#organization-profile-name");
  const description = dialog.querySelector("#organization-profile-description");
  const baseDN = dialog.querySelector("#organization-profile-base-dn");
  const driver = dialog.querySelector("#organization-profile-driver");
  const topology = dialog.querySelector("#organization-profile-topology");
  const dirBaseDN = dialog.querySelector("#directory-base-dn-field");
  const dirSeedMode = dialog.querySelector("#directory-seed-mode-field");
  const dirHealthUID = dialog.querySelector("#directory-health-uid-field");
  const status = dialog.querySelector("#organization-profile-status");
  const history = dialog.querySelector("#organization-profile-history");
  const surface = window.ZPREditorPage.createSourceSurface({
    source, highlight: dialog.querySelector("#organization-profile-highlight"),
    gutter: dialog.querySelector("#organization-profile-gutter"), language: "json", label: "Organization profile",
  });
  let organizationID = "";
  let revision = 0;
  let saved = "";
  let generation = 0;
  let busy = false;
  let draftBundle = false;
  let activeTab = "overview";
  const dirty = () => source.value !== saved;
  const path = () => `/api/simulator/organizations/${encodeURIComponent(organizationID)}/${draftBundle ? "draft/" : ""}profile`;
  const request = (url, options) => window.ZPRPageRuntime.requestJSON(window.fetch.bind(window), url, options);

  function escapeHTML(str) {
    if (window.ZPRSafeDisplay?.escapeHTML) return window.ZPRSafeDisplay.escapeHTML(str);
    return String(str ?? "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  }

  function parseProfile() {
    try {
      return JSON.parse(source.value);
    } catch {
      return null;
    }
  }

  function updateProfile(mutator) {
    try {
      const profile = JSON.parse(source.value);
      mutator(profile);
      window.ZPRAssistant.editText(source, JSON.stringify(profile, null, 2), true);
      changed();
    } catch (err) {
      status.textContent = `Update error: ${err.message}`;
    }
  }

  function switchTab(tabName) {
    activeTab = tabName;
    for (const button of dialog.querySelectorAll("[data-editor-tab]")) {
      const isTarget = button.dataset.editorTab === tabName;
      button.classList.toggle("active", isTarget);
      button.setAttribute("aria-selected", String(isTarget));
    }
    for (const panel of dialog.querySelectorAll(".organization-section-panel[data-panel]")) {
      panel.hidden = panel.dataset.panel !== tabName;
    }
    if (tabName === "source") {
      dialog.querySelector("#organization-profile-advanced").open = true;
      source.focus();
    }
    render();
  }

  for (const button of dialog.querySelectorAll("[data-editor-tab]")) {
    button.addEventListener("click", () => switchTab(button.dataset.editorTab));
  }

  function renderTopologyPreview(profile) {
    const container = dialog.querySelector("#topology-nodes-preview");
    if (!container) return;
    container.replaceChildren();
    const nodes = profile?.runtime?.nodes || [];
    if (!nodes.length) {
      container.innerHTML = '<span class="count-label">No sites configured yet. Add sites above to define topology.</span>';
      return;
    }
    for (const node of nodes) {
      const card = document.createElement("div");
      card.className = "topology-node-card";
      card.innerHTML = `<strong>${escapeHTML(node.id || "(unnamed site)")}</strong><span>Location: ${escapeHTML(node.location || "Unspecified")}</span><span>Substrate: ${escapeHTML(node.substrate_address || "None")}</span><span>ZPR: ${escapeHTML(node.zpr_address || "None")}</span>`;
      container.append(card);
    }
  }

  function renderStructuredViews() {
    const profile = parseProfile();
    const disabled = busy || !revision || !profile;

    if (!profile) {
      name.disabled = description.disabled = baseDN.disabled = driver.disabled = topology.disabled = true;
      dirBaseDN.disabled = dirSeedMode.disabled = dirHealthUID.disabled = true;
      for (const btn of dialog.querySelectorAll('[data-action^="add-"]')) btn.disabled = true;
      return;
    }

    name.disabled = description.disabled = baseDN.disabled = driver.disabled = topology.disabled = disabled;
    dirBaseDN.disabled = dirSeedMode.disabled = dirHealthUID.disabled = disabled;
    for (const btn of dialog.querySelectorAll('[data-action^="add-"]')) btn.disabled = disabled;

    if (document.activeElement !== name) name.value = profile.name ?? "";
    if (document.activeElement !== description) description.value = profile.description ?? "";
    if (document.activeElement !== baseDN) baseDN.value = profile.directory?.base_dn ?? "";
    if (document.activeElement !== driver) driver.value = profile.runtime?.driver ?? "";
    if (document.activeElement !== topology) topology.value = profile.runtime?.topology ?? "";

    if (document.activeElement !== dirBaseDN) dirBaseDN.value = profile.directory?.base_dn ?? "";
    if (document.activeElement !== dirSeedMode) dirSeedMode.value = profile.directory?.seed_mode ?? "";
    if (document.activeElement !== dirHealthUID) dirHealthUID.value = profile.directory?.health_uid ?? "";

    const sitesCount = (profile.runtime?.nodes || []).length;
    const deptsCount = (profile.directory?.departments || []).length;
    const peopleCount = (profile.directory?.people || []).length;
    const groupsCount = (profile.directory?.groups || []).length;
    const servicesCount = (profile.services || []).length;

    dialog.querySelector("#badge-sites-count").textContent = String(sitesCount);
    dialog.querySelector("#badge-directory-count").textContent = String(peopleCount);
    dialog.querySelector("#badge-services-count").textContent = String(servicesCount);

    dialog.querySelector("#metric-sites-count").textContent = String(sitesCount);
    dialog.querySelector("#metric-depts-count").textContent = String(deptsCount);
    dialog.querySelector("#metric-people-count").textContent = String(peopleCount);
    dialog.querySelector("#metric-groups-count").textContent = String(groupsCount);
    dialog.querySelector("#metric-services-count").textContent = String(servicesCount);

    const isTypingInTable = dialog.querySelector("table tbody:focus-within");
    if (!isTypingInTable) {
      renderSitesTable(profile);
      renderDirectoryTables(profile);
      renderServicesTables(profile);
    }
    renderTopologyPreview(profile);

    const graph = dialog.querySelector("#profile-ldap-graph");
    if (graph) graph.directory = profile.directory || {};
  }

  function renderSitesTable(profile) {
    const tbody = dialog.querySelector("#tbody-sites");
    tbody.replaceChildren();
    const nodes = profile.runtime?.nodes || [];
    nodes.forEach((node, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="id" value="${escapeHTML(node.id || "")}"></td>
        <td><input class="compact-input" data-field="location" value="${escapeHTML(node.location || "")}"></td>
        <td><input class="compact-input" data-field="substrate_address" value="${escapeHTML(node.substrate_address || "")}"></td>
        <td><input class="compact-input" data-field="zpr_address" value="${escapeHTML(node.zpr_address || "")}"></td>
        <td><input class="compact-input input-coord" type="number" step="any" data-field="latitude" value="${node.latitude ?? ""}"></td>
        <td><input class="compact-input input-coord" type="number" step="any" data-field="longitude" value="${node.longitude ?? ""}"></td>
        <td><input class="compact-input" data-field="coordinate_note" value="${escapeHTML(node.coordinate_note || "")}"></td>
        <td><button type="button" class="button-delete" data-del-site="${idx}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.runtime) p.runtime = {};
          if (!p.runtime.nodes) p.runtime.nodes = [];
          if (!p.runtime.nodes[idx]) p.runtime.nodes[idx] = {};
          let val = e.target.value;
          if (field === "latitude" || field === "longitude") val = val === "" ? null : Number(val);
          p.runtime.nodes[idx][field] = val;
        });
        renderTopologyPreview(parseProfile());
      });
      tr.querySelector(`[data-del-site="${idx}"]`).addEventListener("click", () => {
        updateProfile(p => { p.runtime.nodes.splice(idx, 1); });
        renderStructuredViews();
      });
      tbody.append(tr);
    });
  }

  function renderDirectoryTables(profile) {
    const tbodyDepts = dialog.querySelector("#tbody-departments");
    tbodyDepts.replaceChildren();
    (profile.directory?.departments || []).forEach((dept, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="name" value="${escapeHTML(dept.name || "")}"></td>
        <td><input class="compact-input" data-field="parent" value="${escapeHTML(dept.parent || "")}"></td>
        <td><button type="button" class="button-delete" data-del-dept="${idx}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.directory?.departments?.[idx]) return;
          p.directory.departments[idx][field] = e.target.value;
        });
      });
      tr.querySelector(`[data-del-dept="${idx}"]`).addEventListener("click", () => {
        updateProfile(p => { p.directory.departments.splice(idx, 1); });
        renderStructuredViews();
      });
      tbodyDepts.append(tr);
    });

    const tbodyPeople = dialog.querySelector("#tbody-people");
    tbodyPeople.replaceChildren();
    (profile.directory?.people || []).forEach((person, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="uid" value="${escapeHTML(person.uid || "")}"></td>
        <td><input class="compact-input" data-field="name" value="${escapeHTML(person.name || "")}"></td>
        <td><input class="compact-input" data-field="email" value="${escapeHTML(person.email || "")}"></td>
        <td><input class="compact-input" data-field="department" value="${escapeHTML(person.department || "")}"></td>
        <td><input class="compact-input" data-field="title" value="${escapeHTML(person.title || "")}"></td>
        <td><input class="compact-input" data-field="location" value="${escapeHTML(person.location || "")}"></td>
        <td><button type="button" class="button-delete" data-del-person="${idx}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.directory?.people?.[idx]) return;
          p.directory.people[idx][field] = e.target.value;
        });
      });
      tr.querySelector(`[data-del-person="${idx}"]`).addEventListener("click", () => {
        updateProfile(p => { p.directory.people.splice(idx, 1); });
        renderStructuredViews();
      });
      tbodyPeople.append(tr);
    });

    const tbodyGroups = dialog.querySelector("#tbody-groups");
    tbodyGroups.replaceChildren();
    (profile.directory?.groups || []).forEach((group, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="name" value="${escapeHTML(group.name || "")}"></td>
        <td><input class="compact-input" data-field="members" value="${escapeHTML((group.members || []).join(", "))}"></td>
        <td><button type="button" class="button-delete" data-del-group="${idx}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.directory?.groups?.[idx]) return;
          if (field === "members") {
            p.directory.groups[idx].members = e.target.value.split(",").map(s => s.trim()).filter(Boolean);
          } else {
            p.directory.groups[idx][field] = e.target.value;
          }
        });
      });
      tr.querySelector(`[data-del-group="${idx}"]`).addEventListener("click", () => {
        updateProfile(p => { p.directory.groups.splice(idx, 1); });
        renderStructuredViews();
      });
      tbodyGroups.append(tr);
    });

    const tbodyOwners = dialog.querySelector("#tbody-machine-owners");
    tbodyOwners.replaceChildren();
    const owners = profile.machine_owners || {};
    Object.keys(owners).forEach((machineId, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="machine" value="${escapeHTML(machineId)}"></td>
        <td><input class="compact-input" data-field="owners" value="${escapeHTML((owners[machineId] || []).join(", "))}"></td>
        <td><button type="button" class="button-delete" data-del-owner="${machineId}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.machine_owners) p.machine_owners = {};
          const rowInputs = tr.querySelectorAll("input");
          const mID = rowInputs[0].value.trim();
          const uids = rowInputs[1].value.split(",").map(s => s.trim()).filter(Boolean);
          if (mID && mID !== machineId) {
            delete p.machine_owners[machineId];
            p.machine_owners[mID] = uids;
          } else if (mID) {
            p.machine_owners[mID] = uids;
          }
        });
      });
      tr.querySelector(`[data-del-owner="${machineId}"]`).addEventListener("click", () => {
        updateProfile(p => {
          if (p.machine_owners) delete p.machine_owners[machineId];
        });
        renderStructuredViews();
      });
      tbodyOwners.append(tr);
    });
  }

  function renderServicesTables(profile) {
    const tbodyServices = dialog.querySelector("#tbody-services");
    tbodyServices.replaceChildren();
    (profile.services || []).forEach((svc, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="name" value="${escapeHTML(svc.name || "")}"></td>
        <td><input class="compact-input" data-field="kind" value="${escapeHTML(svc.kind || "")}"></td>
        <td><input class="compact-input" data-field="endpoint" value="${escapeHTML(svc.endpoint || "")}"></td>
        <td><input class="compact-input" data-field="policy_id" value="${escapeHTML(svc.policy_id || "")}"></td>
        <td><input class="compact-input" data-field="actor_cn" value="${escapeHTML(svc.actor_cn || "")}"></td>
        <td><input class="compact-input" data-field="description" value="${escapeHTML(svc.description || "")}"></td>
        <td><button type="button" class="button-delete" data-del-svc="${idx}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.services?.[idx]) return;
          p.services[idx][field] = e.target.value;
        });
      });
      tr.querySelector(`[data-del-svc="${idx}"]`).addEventListener("click", () => {
        updateProfile(p => { p.services.splice(idx, 1); });
        renderStructuredViews();
      });
      tbodyServices.append(tr);
    });

    const tbodyTest = dialog.querySelector("#tbody-test-services");
    tbodyTest.replaceChildren();
    (profile.policy_test_services || []).forEach((tsvc, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="id" value="${escapeHTML(tsvc.id || "")}"></td>
        <td><input class="compact-input" data-field="name" value="${escapeHTML(tsvc.name || "")}"></td>
        <td><input class="compact-input" data-field="actor_cn" value="${escapeHTML(tsvc.actor_cn || "")}"></td>
        <td><input class="compact-input" data-field="protocol" value="${escapeHTML(tsvc.protocol || "")}"></td>
        <td><input class="compact-input input-port" type="number" data-field="port" value="${tsvc.port ?? ""}"></td>
        <td><input class="compact-input" data-field="zpr_address" value="${escapeHTML(tsvc.zpr_address || "")}"></td>
        <td><button type="button" class="button-delete" data-del-test-svc="${idx}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.policy_test_services?.[idx]) return;
          let val = e.target.value;
          if (field === "port") val = val === "" ? 0 : Number(val);
          p.policy_test_services[idx][field] = val;
        });
      });
      tr.querySelector(`[data-del-test-svc="${idx}"]`).addEventListener("click", () => {
        updateProfile(p => { p.policy_test_services.splice(idx, 1); });
        renderStructuredViews();
      });
      tbodyTest.append(tr);
    });

    const tbodyWorkload = dialog.querySelector("#tbody-workload-services");
    tbodyWorkload.replaceChildren();
    const workloads = profile.workload_services || {};
    Object.keys(workloads).forEach((svcKey, idx) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input class="compact-input" data-field="key" value="${escapeHTML(svcKey)}"></td>
        <td><input class="compact-input" data-field="workloads" value="${escapeHTML((workloads[svcKey] || []).join(", "))}"></td>
        <td><button type="button" class="button-delete" data-del-workload="${svcKey}">Delete</button></td>`;
      tr.addEventListener("input", e => {
        const field = e.target.dataset.field;
        if (!field) return;
        updateProfile(p => {
          if (!p.workload_services) p.workload_services = {};
          const rowInputs = tr.querySelectorAll("input");
          const key = rowInputs[0].value.trim();
          const list = rowInputs[1].value.split(",").map(s => s.trim()).filter(Boolean);
          if (key && key !== svcKey) {
            delete p.workload_services[svcKey];
            p.workload_services[key] = list;
          } else if (key) {
            p.workload_services[key] = list;
          }
        });
      });
      tr.querySelector(`[data-del-workload="${svcKey}"]`).addEventListener("click", () => {
        updateProfile(p => {
          if (p.workload_services) delete p.workload_services[svcKey];
        });
        renderStructuredViews();
      });
      tbodyWorkload.append(tr);
    });
  }

  // Add button actions
  dialog.querySelector('[data-action="add-site"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.runtime) p.runtime = { driver: "docker-multinode", topology: "multi-node", nodes: [] };
      if (!p.runtime.nodes) p.runtime.nodes = [];
      const next = p.runtime.nodes.length + 1;
      p.runtime.nodes.push({ id: `site-${next}`, location: `Site ${next}`, substrate_address: "", zpr_address: "" });
    });
    renderStructuredViews();
  });

  dialog.querySelector('[data-action="add-department"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.directory) p.directory = {};
      if (!p.directory.departments) p.directory.departments = [];
      const next = p.directory.departments.length + 1;
      p.directory.departments.push({ name: `Department ${next}`, parent: "" });
    });
    renderStructuredViews();
  });

  dialog.querySelector('[data-action="add-person"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.directory) p.directory = {};
      if (!p.directory.people) p.directory.people = [];
      const next = p.directory.people.length + 1;
      p.directory.people.push({ uid: `user-${next}`, name: `User ${next}`, email: `user${next}@example.com`, department: "", title: "" });
    });
    renderStructuredViews();
  });

  dialog.querySelector('[data-action="add-group"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.directory) p.directory = {};
      if (!p.directory.groups) p.directory.groups = [];
      const next = p.directory.groups.length + 1;
      p.directory.groups.push({ name: `Group${next}`, members: [] });
    });
    renderStructuredViews();
  });

  dialog.querySelector('[data-action="add-machine-owner"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.machine_owners) p.machine_owners = {};
      const next = Object.keys(p.machine_owners).length + 1;
      p.machine_owners[`machine-${next}`] = [];
    });
    renderStructuredViews();
  });

  dialog.querySelector('[data-action="add-service"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.services) p.services = [];
      const next = p.services.length + 1;
      p.services.push({ name: `Service ${next}`, kind: "Application", endpoint: "ZPR/TCP/8080", description: "" });
    });
    renderStructuredViews();
  });

  dialog.querySelector('[data-action="add-test-service"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.policy_test_services) p.policy_test_services = [];
      const next = p.policy_test_services.length + 1;
      p.policy_test_services.push({ id: `service-${next}.svc.zpr`, name: `Service ${next}`, actor_cn: `service-${next}`, protocol: "TCP", port: 8080 });
    });
    renderStructuredViews();
  });

  dialog.querySelector('[data-action="add-workload-service"]').addEventListener("click", () => {
    updateProfile(p => {
      if (!p.workload_services) p.workload_services = {};
      const next = Object.keys(p.workload_services).length + 1;
      p.workload_services[`service-${next}`] = [];
    });
    renderStructuredViews();
  });

  function render() {
    surface.render();
    dialog.querySelector("#organization-profile-revision").textContent = revision ? `Draft revision ${revision}` : "Loading";
    dialog.querySelector("#organization-profile-modified").hidden = !dirty();
    for (const button of dialog.querySelectorAll("[data-profile-save], [data-profile-validate], [data-profile-reload]")) button.disabled = busy || !revision;
    source.disabled = name.disabled = description.disabled = busy || !revision;
    dialog.querySelector("[data-profile-export]").hidden = !draftBundle;
    dialog.querySelector("[data-profile-export]").disabled = busy || !revision || dirty();
    dialog.querySelector("[data-profile-definitions]").hidden = !draftBundle;
    dialog.querySelector("[data-profile-definitions]").disabled = busy || !revision;
    renderStructuredViews();
    assistant.render();
  }
  function changed() {
    status.textContent = "Modified draft; validate and review before saving.";
    render();
  }
  dialog.querySelector("[data-profile-definitions]").addEventListener("click", () => {
    if (busy || !draftBundle || !revision || !window.ZPREditorPage.confirmDiscard(dirty(), "Discard unsaved profile edits and open the saved full definition bundle?")) return;
    generation++;
    assistant.reset();
    dialog.close();
    document.dispatchEvent(new CustomEvent("organization-definitions:open", { detail: { id: organizationID } }));
  });
  dialog.querySelector("[data-profile-export]").addEventListener("click", async () => {
    if (busy || !draftBundle || dirty()) return;
    const current = generation;
    busy = true;
    render();
    try {
      const bundle = await request(`/api/simulator/organizations/${encodeURIComponent(organizationID)}/draft/export`, { cache: "no-store" });
      if (current !== generation) return;
      const url = URL.createObjectURL(new Blob([JSON.stringify(bundle, null, 2) + "\n"], { type: "application/json" }));
      const link = document.createElement("a");
      link.href = url;
      link.download = `${organizationID}-draft.json`;
      document.body.append(link);
      link.click();
      link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      status.textContent = "Saved definitions exported without creation receipts or runtime state. Unsaved edits are not exported.";
    } catch (error) { if (current === generation) status.textContent = `Export failed: ${error.message}`; }
    finally { if (current === generation) { busy = false; render(); } }
  });
  source.addEventListener("input", changed);
  for (const [field, input] of [["name", name], ["description", description]]) {
    input.addEventListener("input", () => {
      try {
        const profile = JSON.parse(source.value);
        profile[field] = input.value;
        window.ZPRAssistant.editText(source, JSON.stringify(profile, null, 2), true);
      } catch (error) { status.textContent = error.message; }
    });
  }
  baseDN.addEventListener("input", () => {
    updateProfile(p => {
      if (!p.directory) p.directory = {};
      p.directory.base_dn = baseDN.value;
      if (dirBaseDN) dirBaseDN.value = baseDN.value;
    });
  });
  dirBaseDN.addEventListener("input", () => {
    updateProfile(p => {
      if (!p.directory) p.directory = {};
      p.directory.base_dn = dirBaseDN.value;
      if (baseDN) baseDN.value = dirBaseDN.value;
    });
  });
  driver.addEventListener("change", () => {
    updateProfile(p => {
      if (!p.runtime) p.runtime = {};
      p.runtime.driver = driver.value;
    });
  });
  topology.addEventListener("change", () => {
    updateProfile(p => {
      if (!p.runtime) p.runtime = {};
      p.runtime.topology = topology.value;
    });
  });
  dirSeedMode.addEventListener("change", () => {
    updateProfile(p => {
      if (!p.directory) p.directory = {};
      p.directory.seed_mode = dirSeedMode.value;
    });
  });
  dirHealthUID.addEventListener("input", () => {
    updateProfile(p => {
      if (!p.directory) p.directory = {};
      p.directory.health_uid = dirHealthUID.value;
    });
  });
  function close() {
    if (!window.ZPREditorPage.confirmDiscard(dirty(), "Discard unsaved organization draft changes?")) return;
    generation++;
    assistant.reset();
    dialog.close();
  }
  dialog.querySelector("[data-profile-close]").addEventListener("click", close);
  dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
  async function load(id, loadedMessage = "Draft loaded. Runtime and bundled configuration are unchanged.", bundle = false) {
    const current = ++generation;
    organizationID = id;
    draftBundle = bundle;
    revision = 0;
    saved = source.value = "";
    history.replaceChildren();
    busy = true;
    assistant.reset();
    status.textContent = "Loading organization draft...";
    dialog.querySelector("#organization-profile-title").textContent = `Organization draft: ${id}`;
    if (!dialog.open) dialog.showModal();
    render();
    try {
      const artifact = await request(path());
      if (current !== generation) return;
      revision = artifact.revision;
      saved = source.value = JSON.stringify(artifact.content, null, 2);
      status.textContent = loadedMessage;
      const revisions = await request(path() + "/revisions");
      if (current !== generation) return;
      for (const entry of revisions) {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = `Revision ${entry.revision}: ${entry.summary}`;
        button.addEventListener("click", async () => {
          if (busy || !window.ZPREditorPage.confirmDiscard(dirty(), "Replace unsaved changes with this historical profile?")) return;
          if (draftBundle) {
            generation++;
            assistant.reset();
            dialog.close();
            document.dispatchEvent(new CustomEvent("organization-definitions:open", { detail: { id: organizationID, revision: entry.revision } }));
            return;
          }
          const selected = ++generation;
          busy = true;
          render();
          assistant.reset();
          try {
            const record = await request(path() + `/revisions/${entry.revision}`);
            if (selected !== generation) return;
            source.value = JSON.stringify(record.content, null, 2);
            status.textContent = `Historical revision ${entry.revision} loaded for review. Save draft creates a new revision; runtime is unchanged.`;
          } catch (error) { if (selected === generation) status.textContent = error.message; }
          finally { if (selected === generation) { busy = false; render(); } }
        });
        history.append(button);
      }
    } catch (error) { if (current === generation) status.textContent = error.message; }
    finally { if (current === generation) { busy = false; render(); } }
  }
  dialog.querySelector("[data-profile-reload]").addEventListener("click", () => {
    if (window.ZPREditorPage.confirmDiscard(dirty(), "Reload the saved draft and discard unsaved changes?")) void load(organizationID, undefined, draftBundle);
  });
  for (const [selector, save] of [["[data-profile-save]", true], ["[data-profile-validate]", false]]) {
    dialog.querySelector(selector).addEventListener("click", async () => {
      const current = ++generation;
      busy = true;
      assistant.reset();
      render();
      try {
        const profile = JSON.parse(source.value);
        const result = await request(path() + (save ? "" : "/check"), {
          method: save ? "PUT" : "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ profile, expected_revision: revision, summary: "Organization profile draft edit" }),
        });
        if (current !== generation) return;
        if (save) {
          saved = source.value;
          busy = false;
          await load(organizationID, "Draft saved as a new revision. Runtime and bundled configuration are unchanged.", draftBundle);
          document.dispatchEvent(new Event("organization-drafts:changed"));
        } else status.textContent = result.notice;
      } catch (error) {
        if (current === generation) status.textContent = `${error.message}${save ? " Save was not confirmed; reload the draft to reconcile before retrying." : ""}`;
      } finally { if (current === generation) { busy = false; render(); } }
    });
  }
  const review = document.createElement("dialog");
  review.setAttribute("aria-label", "Review AI organization changes");
  review.className = "design-assistant-dialog";
  review.innerHTML = `<h2>Review AI organization changes</h2><p>Only the profile draft will change. Nothing is saved or activated.</p><pre></pre><button type="button" data-review-accept>Insert reviewed draft</button><button type="button" data-review-cancel>Cancel</button>`;
  document.body.append(review);
  const assistant = window.mountSimulatorDesignAssistant("organization-profile-assistant", {
    scope: "organization-profile", editorID: dialog.id, applyLabel: "Review profile proposal",
    editable: () => dialog.open && Boolean(revision) && !busy,
    disclosure: "Send sends only this fictional organization profile draft and conversation to Anthropic. Linked files, credentials and live runtime state are not included. Proposals require review and never save or activate.",
    getContext: () => ({ organization_id: organizationID, profile_source: source.value, revision, draft_bundle: draftBundle }),
    onApply: async (proposal, isCurrent) => {
      if (!proposal.profile) throw new Error("No organization profile proposal was returned.");
      const before = source.value;
      const after = JSON.stringify(proposal.profile, null, 2);
      const original = JSON.parse(before);
      const fields = [...new Set([...Object.keys(original), ...Object.keys(proposal.profile)])].sort();
      review.querySelector("pre").textContent = fields.filter(key => JSON.stringify(original[key]) !== JSON.stringify(proposal.profile[key]))
        .map(key => `${key}\nBefore: ${JSON.stringify(original[key], null, 2)}\nAfter: ${JSON.stringify(proposal.profile[key], null, 2)}`).join("\n\n") || "No profile changes.";
      const accepted = await new Promise(resolve => {
        review.querySelector("[data-review-accept]").onclick = () => { review.close(); resolve(true); };
        review.querySelector("[data-review-cancel]").onclick = () => { review.close(); resolve(false); };
        review.oncancel = event => { event.preventDefault(); review.close(); resolve(false); };
        review.showModal();
      });
      if (!accepted) throw new Error("Proposal review cancelled; draft unchanged.");
      if (!isCurrent() || source.value !== before || !dialog.open || busy) throw new Error("Organization draft changed; ask again before applying.");
      const id = organizationID;
      const baseRevision = revision;
      const restore = (expected, value) => {
        if (organizationID !== id || revision !== baseRevision || source.value !== expected || busy || !dialog.open) throw new Error("Draft changed; AI undo/redo is no longer available.");
        window.ZPRAssistant.editText(source, value, true);
      };
      window.ZPRAssistant.editText(source, after, true);
      return { undo: () => restore(after, before), redo: () => restore(before, after) };
    },
  });
  document.addEventListener("click", event => {
    if (!event.target.closest("#organization-edit-profile")) return;
    if (selectedOrganizationID) void load(selectedOrganizationID);
  });
  document.addEventListener("organization-profile:open", event => {
    if (dialog.open && !window.ZPREditorPage.confirmDiscard(dirty(), "Discard unsaved organization draft changes?")) return;
    void load(event.detail.id, "Private draft bundle loaded. Definitions are not runnable; no runtime or linked source files changed.", true);
  });
  window.addEventListener("operator-session-cleared", () => {
    generation++;
    organizationID = "";
    saved = source.value = "";
    name.value = description.value = "";
    revision = 0;
    busy = false;
    history.replaceChildren();
    assistant.reset();
    if (review.open) review.dispatchEvent(new Event("cancel", { cancelable: true }));
    if (dialog.open) dialog.close();
    render();
  });
  const button = document.createElement("button");
  button.id = "organization-edit-profile";
  button.className = "quiet";
  button.type = "button";
  button.textContent = "Edit organization draft";
  document.querySelector(".organization-top-actions").prepend(button);
})();
