const scenarioById = new Map();
let scenarioRun;
let scenarioCatalogSignature = "";
let selectedScenarioID = "";
let scenarioRunSignature = "";
let activeScenarioOrganization = "";
let scenarioLogContext = "";
let scenarioLogMachines = new Map();
let scenarioLogError = "";
let scenarioLogsLoaded = false;
let scenarioResetRunSignature = "";
let scenarioEditorArtifact;
let scenarioEditorOrganization = "";
let scenarioEditorDirty = false;
let scenarioEditorJsonDirty = false;
let scenarioEditorDraft = {};
let scenarioEditorSaved = null;
let scenarioEditorSummary = "";
let scenarioEditorViewing = 0;
const scenarioEditorPage = window.ZPREditorPage;
const scenarioRequest = (path, options) => scenarioEditorPage.requestJSON(
  (...args) => window.fetch(...args), path, options,
);
let scenarioResetIdentity = null;
let scenarioResetStatus = null;
let scenarioResetStatusOrganization = "";
let scenarioResetFlow = null;
let scenarioResetRequestSerial = 0;
let scenarioResetExpiryTimer = null;
const scenarioPlanState = {
  organizations: [],
  organizationID: "",
  scenarios: [],
  scenarioID: "",
  plan: null,
  unavailableBudget: null,
  approval: null,
  run: null,
  runAttempted: false,
  catalogRequestID: 0,
  planRequestID: 0,
  runRequestID: 0,
  busy: false,
};
let scenarioPlanDialogReturnFocus = null;
const scenarioResetPath = (organization) =>
  `/api/simulator/organizations/${encodeURIComponent(organization)}/scenario-reset`;
const scenarioResetRequest = (path, options = {}) => {
  if (typeof window.zprOperatorFetch !== "function") {
    throw new Error("Authenticated Simulator operator requests are unavailable.");
  }
  return scenarioEditorPage.requestJSON(
    (...args) => window.zprOperatorFetch(...args),
    path,
    { cache: "no-store", ...options },
  );
};

function scenarioPlanIsObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function scenarioPlanSetStatus(message) {
  document.getElementById("scenario-plan-workflow-status").textContent = message;
}

function scenarioPlanSetError(message) {
  const error = document.getElementById("scenario-plan-error");
  error.textContent = message;
  error.hidden = !message;
}

function scenarioPlanAppendOption(select, value, label) {
  const option = document.createElement("option");
  option.value = value;
  option.textContent = label;
  select.append(option);
}

function scenarioPlanRenderControls() {
  const busy = scenarioPlanState.busy;
  document.getElementById("scenario-plan-budget-approve").disabled = busy;
  const organization = document.getElementById("scenario-plan-organization");
  const scenario = document.getElementById("scenario-plan-scenario");
  organization.disabled = busy || scenarioPlanState.organizations.length === 0;
  scenario.disabled = busy || !scenarioPlanState.organizationID || scenarioPlanState.scenarios.length === 0;
  document.getElementById("scenario-plan-preflight").disabled =
    busy || !scenarioPlanState.organizationID || !scenarioPlanState.scenarioID || Boolean(scenarioPlanState.approval);
  const approve = document.getElementById("scenario-plan-approve");
  const canApprove = scenarioPlanCanApprove();
  approve.disabled = busy || !canApprove || Boolean(scenarioPlanState.approval);
  if (scenarioPlanState.approval) {
    approve.textContent = "Plan approved";
  } else {
    approve.textContent = "Approve plan…";
  }
  const explicitConfirmation = document.getElementById("scenario-plan-explicit-confirm").checked;
  document.getElementById("scenario-plan-confirm-approval").disabled =
    busy || !explicitConfirmation || !canApprove || Boolean(scenarioPlanState.approval);
  const runButton = document.getElementById("scenario-plan-run-button");
  runButton.disabled = busy || !scenarioPlanCanRun() || scenarioPlanState.runAttempted;
  runButton.textContent = scenarioPlanState.run
    ? "Scenario run submitted"
    : scenarioPlanState.runAttempted ? "Run request attempted" : "Run approved scenario";
}

function scenarioPlanInvalidate(message = "") {
  scenarioPlanState.planRequestID += 1;
  scenarioPlanState.runRequestID += 1;
  scenarioPlanState.busy = false;
  scenarioPlanState.plan = null;
  scenarioPlanState.unavailableBudget = null;
  scenarioPlanState.approval = null;
  scenarioPlanState.run = null;
  scenarioPlanState.runAttempted = false;
  document.getElementById("scenario-plan-preview").hidden = true;
  document.getElementById("scenario-plan-budget-handoff").hidden = true;
  document.getElementById("scenario-plan-handoff-organization").textContent = "";
  document.getElementById("scenario-plan-handoff-scenario").textContent = "";
  document.getElementById("scenario-plan-handoff-digest").textContent = "";
  document.getElementById("scenario-plan-handoff-blockers").replaceChildren();
  document.getElementById("scenario-plan-copy-status").textContent = "";
  document.getElementById("scenario-plan-run").hidden = true;
  document.getElementById("scenario-plan-run-status").textContent = "";
  document.getElementById("scenario-plan-run-details").hidden = true;
  document.getElementById("scenario-plan-approval-status").textContent = "";
  scenarioPlanSetError("");
  const dialog = document.getElementById("scenario-plan-approval-dialog");
  if (dialog.open) dialog.close();
  document.getElementById("scenario-plan-explicit-confirm").checked = false;
  document.getElementById("scenario-plan-approval-dialog-status").textContent = "";
  const dialogError = document.getElementById("scenario-plan-approval-dialog-error");
  dialogError.textContent = "";
  dialogError.hidden = true;
  scenarioPlanSetStatus(message);
  scenarioPlanRenderControls();
}

function scenarioPlanValidateResponse(plan, organizationID, scenarioID) {
  const requiredStringFields = [
    "status", "organization_id", "scenario_id", "manifest_name", "budget_status",
    "topology_status", "policy_status", "northstar_assets_status", "package_contract_status",
    "plan_digest", "authorization_scope_digest",
  ];
  if (!scenarioPlanIsObject(plan) || requiredStringFields.some(field => typeof plan[field] !== "string") ||
      !["blocked", "plan-only"].includes(plan.status) ||
      !["absent", "unverified", "approved", "unapproved"].includes(plan.budget_status) ||
      plan.organization_id !== organizationID || plan.scenario_id !== scenarioID ||
      !/^sha256:[0-9a-f]{64}$/.test(plan.plan_digest) ||
      !/^sha256:[0-9a-f]{64}$/.test(plan.authorization_scope_digest) ||
      !Array.isArray(plan.machine_ids) || plan.machine_ids.some(id => typeof id !== "string") ||
      !Array.isArray(plan.workloads) || plan.workloads.some(workload =>
        !scenarioPlanIsObject(workload) || typeof workload.machine_id !== "string" ||
        typeof workload.component_id !== "string" || typeof workload.agent !== "string") ||
      !Array.isArray(plan.budget_inputs) || plan.budget_inputs.some(input =>
        !scenarioPlanIsObject(input) || typeof input.phase !== "string" ||
        !Number.isSafeInteger(input.step) || typeof input.action !== "string" ||
        input.fanout_count !== undefined && (!Number.isFinite(input.fanout_count) || input.fanout_count < 0) ||
        input.service_count !== undefined && (!Number.isFinite(input.service_count) || input.service_count < 0)) ||
      !Array.isArray(plan.blockers) || plan.blockers.some(blocker => typeof blocker !== "string") ||
      plan.topology !== undefined && !scenarioPlanIsObject(plan.topology) ||
      plan.retained_topology_binding !== undefined &&
        (!scenarioPlanIsObject(plan.retained_topology_binding) ||
         plan.retained_topology_binding.version !== 1 ||
         !/^[0-9a-f]{24}$/.test(plan.retained_topology_binding.install_id) ||
         !/^sha256:[0-9a-f]{64}$/.test(plan.retained_topology_binding.image_id) ||
         ["logical_node_id", "node_container", "network", "substrate_address", "zpr_address"]
           .some(field => typeof plan.retained_topology_binding[field] !== "string" ||
             !plan.retained_topology_binding[field]))) {
    throw new Error("Preflight returned an invalid or mismatched scenario plan.");
  }
  return plan;
}

function scenarioPlanValidateUnavailableBudget(response, organizationID, scenarioID) {
  if (!scenarioPlanIsObject(response) ||
      response.organization_id !== organizationID ||
      response.scenario_id !== scenarioID ||
      typeof response.authorization_scope_digest !== "string" ||
      !/^sha256:[0-9a-f]{64}$/.test(response.authorization_scope_digest) ||
      !Array.isArray(response.blockers) ||
      response.blockers.length === 0 ||
      response.blockers.some(blocker => typeof blocker !== "string" || !blocker)) {
    throw new Error("Authorization service returned an invalid or mismatched unavailable-evidence response.");
  }
  return response;
}

async function scenarioPlanPreflightRequest(organizationID, scenarioID) {
  if (typeof window.zprOperatorFetch !== "function") {
    throw new Error("Authenticated Simulator operator requests are unavailable.");
  }
  const response = await window.zprOperatorFetch(
    `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenario-plans/preflight`,
    {
      method: "POST",
      cache: "no-store",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ organization_id: organizationID, scenario_id: scenarioID }),
    },
  );
  let body;
  try {
    body = await response.json();
  } catch (error) {
    if (!(error instanceof SyntaxError)) throw error;
    throw new Error(`Preflight returned invalid JSON (HTTP ${response.status}).`);
  }
  if (response.status !== 200 && response.status !== 503) {
    const message = scenarioPlanIsObject(body) && typeof body.error === "string"
      ? body.error
      : `HTTP ${response.status}`;
    throw new Error(`Preflight failed: ${message}`);
  }
  return { status: response.status, body };
}

function scenarioPlanRenderUnavailableBudget(response) {
  const evaluationBudget = response.budget_approval_available === true;
  document.getElementById("scenario-plan-budget-approve").hidden = !evaluationBudget;
  document.getElementById("scenario-plan-budget-approve-status").textContent = evaluationBudget
    ? "This bounded evaluation is ready. Your authorized identity can authorize its exact scope for 15 minutes; no digest entry is needed."
    : "";
  document.getElementById("scenario-plan-budget-handoff-title").textContent = evaluationBudget
    ? "Evaluation scenario authorization" : "Manual first-grant handoff";
  document.getElementById("scenario-plan-handoff-organization").textContent = response.organization_id;
  document.getElementById("scenario-plan-handoff-scenario").textContent = response.scenario_id;
  document.getElementById("scenario-plan-handoff-digest").textContent = response.authorization_scope_digest;
  const blockers = document.getElementById("scenario-plan-handoff-blockers");
  blockers.replaceChildren();
  for (const blocker of response.blockers) {
    const item = document.createElement("li");
    item.textContent = blocker;
    blockers.append(item);
  }
  document.getElementById("scenario-plan-budget-handoff").hidden = false;
  document.getElementById("scenario-plan-approval-status").textContent =
    "Authorization unavailable: obtain a valid Control Room authorization grant, then refresh preflight.";
  scenarioPlanSetStatus(
    "Authorization evidence is unavailable or invalid. No local approval or run can be requested.",
  );
}

function scenarioPlanApprovalBlockers(plan) {
  if (!plan) return ["Run preflight before requesting approval."];
  const blockers = [];
  if (plan.status !== "plan-only") blockers.push("The plan is blocked.");
  if (plan.budget_status !== "approved") blockers.push(`Authorization status is ${plan.budget_status}.`);
  if (plan.blockers.length) blockers.push("Preflight reported blockers.");
  if (!plan.machine_ids.length && plan.package_contract_status !== "evaluation-endpoints-v1") {
    blockers.push("Preflight returned no machine IDs; no organization inventory fallback is allowed.");
  }
  return blockers;
}

function scenarioPlanCanApprove() {
  return scenarioPlanApprovalBlockers(scenarioPlanState.plan).length === 0;
}

function scenarioPlanCanRun() {
  const { approval, plan, organizationID, scenarioID } = scenarioPlanState;
  return Boolean(approval && plan && approval.approved === true &&
    typeof approval.approval_id === "string" && approval.approval_id &&
    approval.organization_id === organizationID && approval.scenario_id === scenarioID &&
    approval.plan_digest === plan.plan_digest && plan.organization_id === organizationID &&
    plan.scenario_id === scenarioID && !scenarioPlanApprovalBlockers(plan).length);
}

function scenarioPlanValidateRun(run, organizationID, scenarioID, planDigest) {
  if (!scenarioPlanIsObject(run) || typeof run.run_id !== "string" || !run.run_id ||
      typeof run.run_namespace !== "string" || !run.run_namespace ||
      typeof run.state !== "string" || !run.state ||
      run.organization_id !== organizationID || run.scenario_id !== scenarioID ||
      run.plan_digest !== planDigest) {
    throw new Error("Run endpoint returned an invalid or mismatched scenario run.");
  }
  return run;
}

function scenarioPlanRenderRun(run) {
  const section = document.getElementById("scenario-plan-run");
  const details = document.getElementById("scenario-plan-run-details");
  if (!run) {
    details.hidden = true;
    return;
  }
  document.getElementById("scenario-plan-run-id").textContent = run.run_id;
  document.getElementById("scenario-plan-run-namespace").textContent = run.run_namespace;
  document.getElementById("scenario-plan-run-state").textContent = run.state;
  details.hidden = false;
  section.hidden = false;
  scenarioPlanState.run = run;
  scenarioPlanRenderControls();
}

function scenarioPlanRenderTable(container, headers, rows, emptyMessage) {
  container.replaceChildren();
  if (!rows.length) {
    const empty = document.createElement("p");
    empty.textContent = emptyMessage;
    container.append(empty);
    return;
  }
  const table = document.createElement("table");
  const head = document.createElement("thead");
  const headerRow = document.createElement("tr");
  for (const label of headers) {
    const cell = document.createElement("th");
    cell.scope = "col";
    cell.textContent = label;
    headerRow.append(cell);
  }
  head.append(headerRow);
  const body = document.createElement("tbody");
  for (const row of rows) {
    const tableRow = document.createElement("tr");
    for (const value of row) {
      const cell = document.createElement("td");
      cell.textContent = value;
      tableRow.append(cell);
    }
    body.append(tableRow);
  }
  table.append(head, body);
  container.append(table);
}

function scenarioPlanTopologyText(plan) {
  const topology = plan.topology === undefined
    ? "Not included in the preflight response." : JSON.stringify(plan.topology, null, 2);
  return plan.retained_topology_binding === undefined ? topology
    : `${topology}\n\nExplicit retained installation binding (not scenario acceptance):\n${JSON.stringify(plan.retained_topology_binding, null, 2)}`;
}

function scenarioPlanRender(plan) {
  const preview = document.getElementById("scenario-plan-preview");
  const summary = document.getElementById("scenario-plan-summary");
  summary.replaceChildren();
  for (const [label, value] of [
    ["Status", plan.status],
    ["Organization", plan.organization_id],
    ["Scenario", plan.scenario_id],
    ["Authorization status", plan.budget_status],
    ["Policy status", plan.policy_status],
  ]) {
    const term = document.createElement("dt");
    term.textContent = label;
    const description = document.createElement("dd");
    description.textContent = value;
    summary.append(term, description);
  }
  const machines = document.getElementById("scenario-plan-machines");
  machines.replaceChildren();
  if (plan.machine_ids.length) {
    for (const id of plan.machine_ids) {
      const item = document.createElement("li");
      item.textContent = id;
      machines.append(item);
    }
  } else {
    const item = document.createElement("li");
    item.textContent = "None returned by preflight.";
    machines.append(item);
  }
  document.getElementById("scenario-plan-topology").textContent =
    scenarioPlanTopologyText(plan);
  scenarioPlanRenderTable(
    document.getElementById("scenario-plan-workloads"),
    ["Machine ID", "Component ID", "Agent"],
    plan.workloads.map(workload => [workload.machine_id, workload.component_id, workload.agent]),
    "No workloads returned.",
  );
  scenarioPlanRenderTable(
    document.getElementById("scenario-plan-budget-inputs"),
    ["Phase", "Step", "Action", "Fanout count", "Service count"],
    plan.budget_inputs.map(input => [
      input.phase, String(input.step), input.action,
      input.fanout_count === undefined ? "—" : String(input.fanout_count),
      input.service_count === undefined ? "—" : String(input.service_count),
    ]),
    "No resource inputs returned.",
  );
  const blockers = document.getElementById("scenario-plan-blockers");
  blockers.replaceChildren();
  for (const blocker of plan.blockers.length ? plan.blockers : ["None reported by preflight."]) {
    const item = document.createElement("li");
    item.textContent = blocker;
    blockers.append(item);
  }
  const approvalStatus = document.getElementById("scenario-plan-approval-status");
  const reasons = scenarioPlanApprovalBlockers(plan);
  approvalStatus.textContent = reasons.length
    ? `Approval unavailable: ${reasons.join(" ")}`
    : "Preflight permits requesting approval; a separate explicit confirmation is required.";
  preview.hidden = false;
  scenarioPlanRenderControls();
}

async function loadScenarioPlanCatalog(organizationID) {
  const requestID = ++scenarioPlanState.catalogRequestID;
  scenarioPlanState.scenarios = [];
  scenarioPlanState.scenarioID = "";
  const scenarioSelect = document.getElementById("scenario-plan-scenario");
  scenarioSelect.replaceChildren();
  scenarioPlanAppendOption(scenarioSelect, "", organizationID ? "Loading scenarios…" : "Select an organization first");
  scenarioPlanInvalidate(organizationID
    ? "Loading scenarios for the selected organization…"
    : "Select an organization to load its scenarios.");
  scenarioPlanRenderControls();
  if (!organizationID) return;
  try {
    const data = await scenarioResetRequest(
      `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios`,
    );
    if (requestID !== scenarioPlanState.catalogRequestID || organizationID !== scenarioPlanState.organizationID) return;
    if (!scenarioPlanIsObject(data) || data.organization?.id !== organizationID || !Array.isArray(data.scenarios) ||
        data.scenarios.some(item => !scenarioPlanIsObject(item) ||
          typeof item.id !== "string" || typeof item.name !== "string" ||
          item.organization_id !== organizationID)) {
      throw new Error("Scenario catalog did not match the selected organization.");
    }
    const seen = new Set();
    for (const item of data.scenarios) {
      if (seen.has(item.id)) throw new Error("Scenario catalog contains duplicate scenario IDs.");
      seen.add(item.id);
    }
    scenarioPlanState.scenarios = data.scenarios;
    scenarioSelect.replaceChildren();
    scenarioPlanAppendOption(scenarioSelect, "", data.scenarios.length ? "Select a scenario" : "No scenarios available");
    for (const item of data.scenarios) scenarioPlanAppendOption(scenarioSelect, item.id, item.name);
    scenarioPlanSetStatus(data.scenarios.length
      ? "Choose a scenario, then preview its plan."
      : "No scenarios are available for this organization.");
  } catch (error) {
    if (requestID !== scenarioPlanState.catalogRequestID) return;
    scenarioSelect.replaceChildren();
    scenarioPlanAppendOption(scenarioSelect, "", "Scenario catalog unavailable");
    scenarioPlanSetError(error.message || "Could not load scenarios for the selected organization.");
    scenarioPlanSetStatus("Scenario catalog is unavailable; approval remains disabled.");
  }
  scenarioPlanRenderControls();
}

async function initializeScenarioPlanWorkflow() {
  const organizationSelect = document.getElementById("scenario-plan-organization");
  try {
    const data = await scenarioResetRequest("/api/simulator/organizations");
    if (!scenarioPlanIsObject(data) || !Array.isArray(data.organizations) ||
        typeof data.active_id !== "string" ||
        data.organizations.some(item => !scenarioPlanIsObject(item) ||
          typeof item.id !== "string" || typeof item.name !== "string")) {
      throw new Error("Organization catalog has an invalid response.");
    }
    const seen = new Set();
    for (const item of data.organizations) {
      if (seen.has(item.id)) throw new Error("Organization catalog contains duplicate IDs.");
      seen.add(item.id);
    }
    if (data.active_id && !seen.has(data.active_id)) throw new Error("Active organization is missing from the catalog.");
    scenarioPlanState.organizations = data.organizations;
    scenarioPlanState.organizationID = data.active_id ||
      (data.organizations.length === 1 ? data.organizations[0].id : "");
    organizationSelect.replaceChildren();
    scenarioPlanAppendOption(organizationSelect, "", "Select an organization");
    for (const item of data.organizations) scenarioPlanAppendOption(organizationSelect, item.id, item.name);
    organizationSelect.value = scenarioPlanState.organizationID;
    if (scenarioPlanState.organizationID) await loadScenarioPlanCatalog(scenarioPlanState.organizationID);
    else {
      scenarioPlanSetStatus("Select an organization to load its scenarios.");
      scenarioPlanRenderControls();
    }
  } catch (error) {
    organizationSelect.replaceChildren();
    scenarioPlanAppendOption(organizationSelect, "", "Organization catalog unavailable");
    scenarioPlanSetError(error.message || "Could not load organizations.");
    scenarioPlanSetStatus("Organization catalog is unavailable; approval remains disabled.");
    scenarioPlanRenderControls();
  }
}

async function preflightSelectedScenarioPlan() {
  const organizationID = scenarioPlanState.organizationID;
  const scenarioID = scenarioPlanState.scenarioID;
  if (!organizationID || !scenarioID || scenarioPlanState.busy) return;
  scenarioPlanInvalidate("");
  const requestID = ++scenarioPlanState.planRequestID;
  scenarioPlanState.busy = true;
  scenarioPlanSetError("");
  scenarioPlanSetStatus("Requesting read-only scenario preflight…");
  scenarioPlanRenderControls();
  try {
    const result = await scenarioPlanPreflightRequest(organizationID, scenarioID);
    if (requestID !== scenarioPlanState.planRequestID ||
        organizationID !== scenarioPlanState.organizationID || scenarioID !== scenarioPlanState.scenarioID) return;
    if (result.status === 503) {
      scenarioPlanState.unavailableBudget = scenarioPlanValidateUnavailableBudget(
        result.body, organizationID, scenarioID,
      );
      scenarioPlanRenderUnavailableBudget(scenarioPlanState.unavailableBudget);
      return;
    }
    scenarioPlanState.plan = scenarioPlanValidateResponse(result.body, organizationID, scenarioID);
    scenarioPlanRender(scenarioPlanState.plan);
    scenarioPlanSetStatus(
      `Preflight ${result.body.status}; authorization ${result.body.budget_status}; ` +
      `${result.body.machine_ids.length} machine references and ${result.body.blockers.length} blockers. Review the plan below.`,
    );
    document.getElementById("scenario-plan-preview-title").focus();
  } catch (error) {
    if (requestID !== scenarioPlanState.planRequestID) return;
    scenarioPlanState.plan = null;
    scenarioPlanState.approval = null;
    document.getElementById("scenario-plan-preview").hidden = true;
    document.getElementById("scenario-plan-run").hidden = true;
    scenarioPlanSetError(error.message || "Preflight failed; no approval was requested.");
    scenarioPlanSetStatus("Preflight failed; approval remains unavailable.");
  } finally {
    if (requestID === scenarioPlanState.planRequestID) {
      scenarioPlanState.busy = false;
      scenarioPlanRenderControls();
    }
  }
}

async function approveEvaluationScenarioBudget() {
  const unavailable = scenarioPlanState.unavailableBudget;
  if (scenarioPlanState.busy || unavailable?.budget_approval_available !== true) return;
  const organizationID = scenarioPlanState.organizationID;
  const scenarioID = scenarioPlanState.scenarioID;
  const requestID = scenarioPlanState.planRequestID;
  scenarioPlanState.busy = true;
  scenarioPlanRenderControls();
  scenarioPlanSetError("");
  try {
    const response = await scenarioResetRequest(
      `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenario-plans/budget-approval`,
      { method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ organization_id: organizationID, scenario_id: scenarioID }) },
    );
    if (requestID !== scenarioPlanState.planRequestID) return;
    if (response.organization_id !== organizationID || response.scenario_id !== scenarioID ||
        response.plan_digest !== unavailable.authorization_scope_digest || response.decision !== "approved") {
      throw new Error("Authorization service returned a mismatched response; refresh preflight.");
    }
    scenarioPlanState.busy = false;
    await preflightSelectedScenarioPlan();
  } catch (error) {
    if (requestID !== scenarioPlanState.planRequestID) return;
    scenarioPlanSetError(`${error.message || "Evaluation authorization was not confirmed."} Refresh preflight before retrying; the request may have reached the service.`);
  } finally {
    if (requestID === scenarioPlanState.planRequestID) {
      scenarioPlanState.busy = false;
      scenarioPlanRenderControls();
    }
  }
}

function openScenarioPlanApprovalDialog() {
  if (!scenarioPlanCanApprove() || scenarioPlanState.busy || scenarioPlanState.approval) return;
  document.getElementById("scenario-plan-approval-organization").textContent = scenarioPlanState.plan.organization_id;
  document.getElementById("scenario-plan-approval-scenario").textContent = scenarioPlanState.plan.scenario_id;
  document.getElementById("scenario-plan-approval-digest").textContent = scenarioPlanState.plan.plan_digest;
  document.getElementById("scenario-plan-explicit-confirm").checked = false;
  document.getElementById("scenario-plan-confirm-approval").disabled = true;
  document.getElementById("scenario-plan-approval-dialog-status").textContent = "";
  const dialogError = document.getElementById("scenario-plan-approval-dialog-error");
  dialogError.textContent = "";
  dialogError.hidden = true;
  scenarioPlanDialogReturnFocus = document.activeElement;
  document.getElementById("scenario-plan-approval-dialog").showModal();
  document.getElementById("scenario-plan-approval-title").focus();
}

async function confirmScenarioPlanApproval() {
  const plan = scenarioPlanState.plan;
  const confirmation = document.getElementById("scenario-plan-explicit-confirm");
  if (!scenarioPlanCanApprove() || scenarioPlanState.busy || scenarioPlanState.approval || !confirmation.checked) return;
  scenarioPlanState.busy = true;
  const requestID = scenarioPlanState.planRequestID;
  document.getElementById("scenario-plan-approval-dialog-status").textContent = "Submitting explicit approval…";
  scenarioPlanRenderControls();
  try {
    const response = await scenarioResetRequest(
      `/api/simulator/organizations/${encodeURIComponent(plan.organization_id)}/scenario-plans/approve`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          organization_id: plan.organization_id,
          scenario_id: plan.scenario_id,
          plan_digest: plan.plan_digest,
          approved: true,
        }),
      },
    );
    if (requestID !== scenarioPlanState.planRequestID || plan !== scenarioPlanState.plan) return;
    if (!scenarioPlanIsObject(response) || response.approved !== true ||
        typeof response.approval_id !== "string" || !response.approval_id ||
        response.organization_id !== plan.organization_id || response.scenario_id !== plan.scenario_id ||
        response.plan_digest !== plan.plan_digest || !Array.isArray(response.machine_ids) ||
        response.machine_ids.some(id => typeof id !== "string") ||
        response.machine_ids.length !== plan.machine_ids.length ||
        response.machine_ids.some((id, index) => id !== plan.machine_ids[index])) {
      throw new Error("Approval endpoint returned an invalid or mismatched response.");
    }
    scenarioPlanState.approval = response;
    document.getElementById("scenario-plan-run").hidden = false;
    document.getElementById("scenario-plan-run-status").textContent =
      "Approval is recorded for this selected plan. You may explicitly start its scenario run.";
    document.getElementById("scenario-plan-approval-status").textContent =
      `Approval recorded (${response.approval_id}).`;
    scenarioPlanSetStatus("Scenario plan approval recorded. No run has been started.");
    document.getElementById("scenario-plan-approval-dialog").close();
  } catch (error) {
    if (requestID !== scenarioPlanState.planRequestID) return;
    scenarioPlanState.plan = null;
    scenarioPlanState.approval = null;
    document.getElementById("scenario-plan-preview").hidden = true;
    document.getElementById("scenario-plan-explicit-confirm").checked = false;
    document.getElementById("scenario-plan-approval-dialog-status").textContent =
      "Approval was not confirmed. The preflight plan was cleared.";
    const dialogError = document.getElementById("scenario-plan-approval-dialog-error");
    dialogError.textContent =
      `${error.message || "Approval failed."} Run preflight again before requesting approval.`;
    dialogError.hidden = false;
    scenarioPlanSetStatus("Approval was not confirmed; the plan was cleared.");
  } finally {
    if (requestID === scenarioPlanState.planRequestID) {
      scenarioPlanState.busy = false;
      scenarioPlanRenderControls();
    }
  }
}

async function runApprovedScenario() {
  if (!scenarioPlanCanRun() || scenarioPlanState.busy || scenarioPlanState.runAttempted) return;
  const { approval, plan, organizationID, scenarioID } = scenarioPlanState;
  scenarioPlanState.busy = true;
  scenarioPlanState.runAttempted = true;
  const requestID = ++scenarioPlanState.runRequestID;
  document.getElementById("scenario-plan-run").hidden = false;
  document.getElementById("scenario-plan-run-status").textContent = "Submitting the approved scenario run…";
  scenarioPlanSetError("");
  scenarioPlanRenderControls();
  try {
    const response = await scenarioResetRequest(
      `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios/${encodeURIComponent(scenarioID)}/run`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          approval_id: approval.approval_id,
          plan_digest: plan.plan_digest,
        }),
      },
    );
    if (requestID !== scenarioPlanState.runRequestID ||
        organizationID !== scenarioPlanState.organizationID || scenarioID !== scenarioPlanState.scenarioID ||
        approval !== scenarioPlanState.approval || plan !== scenarioPlanState.plan) return;
    scenarioPlanRenderRun(scenarioPlanValidateRun(
      response, organizationID, scenarioID, plan.plan_digest,
    ));
    document.getElementById("scenario-plan-run-status").textContent =
      "Approved scenario run accepted. Status will refresh from the scenario catalog.";
    void refreshScenariosNow();
  } catch (error) {
    if (requestID !== scenarioPlanState.runRequestID) return;
    scenarioPlanState.plan = null;
    scenarioPlanState.approval = null;
    document.getElementById("scenario-plan-preview").hidden = true;
    document.getElementById("scenario-plan-run-status").textContent =
      "The run request was not confirmed. Its approval was cleared; preview and approve again before another request.";
    scenarioPlanSetError(error.message || "Could not confirm the approved scenario run.");
  } finally {
    if (requestID === scenarioPlanState.runRequestID) {
      scenarioPlanState.busy = false;
      scenarioPlanRenderControls();
    }
  }
}

const scenarioEditorController = scenarioEditorPage.createController({
  source: document.getElementById("scenario-editor-source"),
  status: document.getElementById("scenario-editor-status"),
  isDirty: () => scenarioEditorDirty,
  identity: { title: document.getElementById("scenario-editor-title"), version: document.getElementById("scenario-editor-revision-label"), modified: document.getElementById("scenario-editor-modified") },
  menu: { root: document.getElementById("scenario-editor-actions"), toggle: document.getElementById("scenario-editor-files-toggle"), menu: document.getElementById("scenario-editor-file-menu") },
  history: { menu: document.getElementById("scenario-editor-history-menu"), list: document.getElementById("scenario-editor-history"), count: document.getElementById("scenario-editor-history-count"), isAvailable: () => Boolean(scenarioEditorArtifact) },
  adapters: { load: openExistingScenarioEditor, analyze: analyzeScenarioEditor, save: saveScenarioDraft, render: updateScenarioEditorActions },
  syncControls: updateScenarioEditorActions,
});
const scenarioFormAnalysisScope = scenarioEditorController.createScope("form", () => [
  JSON.stringify(readScenarioEditorForm()), scenarioEditorOrganization,
  scenarioEditorArtifact?.id, scenarioEditorArtifact?.revision, scenarioEditorViewing,
  document.getElementById("scenario-editor-dialog").open,
  document.getElementById("scenario-editor-advanced").open,
]);
const scenarioHistory = scenarioEditorController.history;

function scenarioEscape(value) {
  return window.ZPRSafeDisplay.escapeHTML(value);
}

function scenarioPreviewContext() {
  return JSON.stringify([activeScenarioOrganization, scenarioRun?.scenario_id, scenarioRun?.started_at]);
}

function renderScenarioLogPreviews() {
  const busy = ["running", "cleaning"].includes(scenarioRun?.state);
  for (const lane of document.querySelectorAll(".scenario-lane[data-scenario-machine]")) {
    const existing = lane.querySelector(".scenario-log-preview");
    if (!busy) {
      existing?.remove();
      continue;
    }
    const id = lane.dataset.scenarioMachine;
    const machine = scenarioLogMachines.get(id);
    if (machine && machine.state !== "running") {
      existing?.remove();
      continue;
    }
    const preview = existing || document.createElement("div");
    preview.className = "scenario-log-preview";
    preview.setAttribute("role", "region");
    preview.setAttribute("aria-label", `Log preview for ${id}`);
    const status = preview.querySelector("span") || document.createElement("span");
    status.className = "scenario-log-preview-status";
    const sources = machine?.sources || [];
    const sourceErrors = sources.filter(source => source.error);
    status.textContent = scenarioLogError || (sourceErrors.length ? sourceErrors.map(source => `${source.name}: ${source.error}`).join("; ")
      : !machine ? scenarioLogsLoaded ? "Machine logs unavailable" : "Loading logs…" : "Live logs");
    preview.classList.toggle("unavailable", Boolean(scenarioLogError || sourceErrors.length || scenarioLogsLoaded && !machine));
    const output = preview.querySelector("pre") || document.createElement("pre");
    output.textContent = sources.flatMap(source => (source.lines || []).slice(-8).map(line => `${source.name}: ${line}`))
      .slice(-12).join("\n").slice(-8192) || (machine ? "No log entries." : "");
    const link = preview.querySelector("a") || document.createElement("a");
    link.href = "/machine-logs.html";
    link.dataset.simulatorNav = "";
    link.textContent = "Workers";
    link.setAttribute("aria-label", `Open Workers logs for ${id}`);
    if (!existing) {
      preview.append(status, output, link);
      lane.prepend(preview);
    }
    output.scrollTop = output.scrollHeight;
  }
  updateScenarioTrackScroll();
}

async function refreshScenarioLogPreviews({ signal, isCurrent }) {
  if (!["running", "cleaning"].includes(scenarioRun?.state)) return;
  const context = scenarioPreviewContext();
  let data;
  try {
    data = await scenarioRequest("/api/simulator/machine-logs", { cache: "no-store", signal });
  } catch (failure) {
    if (!isCurrent() || context !== scenarioPreviewContext() || !["running", "cleaning"].includes(scenarioRun?.state)) return;
    scenarioLogError = failure.message || "Machine logs unavailable";
    renderScenarioLogPreviews();
    return;
  }
  if (!isCurrent() || context !== scenarioPreviewContext() || !["running", "cleaning"].includes(scenarioRun?.state)) return;
  if (!Array.isArray(data?.machines) || data.machines.some(entry => !entry?.machine?.id || !Array.isArray(entry.sources)
    || entry.sources.some(source => !Array.isArray(source?.lines) || source.lines.some(line => typeof line !== "string")))) {
    throw new Error("Invalid machine log response");
  }
  scenarioLogsLoaded = true;
  if (data.organization_id !== activeScenarioOrganization) {
    scenarioLogMachines.clear();
    scenarioLogError = "Logs unavailable for this organization";
  } else {
    scenarioLogMachines = new Map((data.machines || []).map(entry => [entry.machine.id, entry]));
    scenarioLogError = "";
  }
  renderScenarioLogPreviews();
}

function renderScenarioList(scenarios) {
  scenarioById.clear();
  for (const scenario of scenarios) scenarioById.set(scenario.id, scenario);
  if (!scenarioById.has(selectedScenarioID)) selectedScenarioID = scenarios[0]?.id || "";
  renderSelectedScenario();
  if (!scenarios.length) {
    document.getElementById("scenario-browser-list").innerHTML = `<p class="scenario-empty">No scenarios in this organization. Create one from File... to get started.</p>`;
    return;
  }
  const folders = new Map();
  for (const scenario of scenarios) {
    const folder = scenario.folder?.trim() || "Unfiled";
    const entries = folders.get(folder) || [];
    entries.push(scenario);
    folders.set(folder, entries);
  }
  const browser = document.getElementById("scenario-browser-list");
  const openFolders = new Set([...browser.querySelectorAll("details[open]")].map(item => item.dataset.folder));
  browser.innerHTML = [...folders].sort(([left], [right]) => left.localeCompare(right)).map(([folder, entries]) => {
    const items = entries.slice().sort((a, b) => a.name.localeCompare(b.name)).map(scenario =>
      `<button class="scenario-browser-item quiet" type="button" data-select-scenario="${scenarioEscape(scenario.id)}" aria-pressed="${scenario.id === selectedScenarioID}"><strong>${scenarioEscape(scenario.name)}</strong><span>${scenarioEscape(scenario.description)}</span></button>`).join("");
    return `<details class="scenario-folder" data-folder="${scenarioEscape(folder)}" ${openFolders.has(folder) || entries.some(item => item.id === selectedScenarioID) ? "open" : ""}><summary>${scenarioEscape(folder)} <span>(${entries.length})</span></summary><div class="scenario-folder-items">${items}</div></details>`;
  }).join("");
}

function renderSelectedScenario() {
  const scenario = scenarioById.get(selectedScenarioID);
  document.getElementById("scenario-edit-selected").disabled = !scenario;
  const list = document.getElementById("scenario-list");
  if (!scenario) {
    list.innerHTML = `<p class="scenario-empty">No scenarios in this organization. Create one from File... to get started.</p>`;
    return;
  }
  const published = Number(scenario.published_revision || 0) > 0;
  list.innerHTML = `<article class="scenario-card selected"><div><h3>${scenarioEscape(scenario.name)}</h3><p>${scenarioEscape(scenario.description)}</p><span class="scenario-step-count">${scenarioEscape(scenario.folder || "Unfiled")} · ${(scenario.steps || []).length} steps${published ? "" : " · Unpublished draft"}</span></div><a class="button" href="/scenarios.html#simulate">Simulate</a></article>`;
}

document.getElementById("scenario-plan-organization").addEventListener("change", event => {
  scenarioPlanState.organizationID = event.target.value;
  scenarioPlanState.scenarios = [];
  scenarioPlanState.scenarioID = "";
  void loadScenarioPlanCatalog(scenarioPlanState.organizationID);
});
document.getElementById("scenario-plan-scenario").addEventListener("change", event => {
  const selected = event.target.value;
  scenarioPlanState.scenarioID = scenarioPlanState.scenarios.some(item => item.id === selected) ? selected : "";
  scenarioPlanInvalidate(scenarioPlanState.scenarioID
    ? "Scenario selection changed. Preview the selected scenario before requesting approval."
    : "Select a scenario to preview its plan.");
});
document.getElementById("scenario-plan-preflight").addEventListener("click", () => {
  void preflightSelectedScenarioPlan();
});
document.getElementById("scenario-plan-copy-handoff").addEventListener("click", async () => {
  const handoff = scenarioPlanState.unavailableBudget;
  if (!handoff || handoff.organization_id !== scenarioPlanState.organizationID ||
      handoff.scenario_id !== scenarioPlanState.scenarioID) return;
  const status = document.getElementById("scenario-plan-copy-status");
  try {
    await navigator.clipboard.writeText(JSON.stringify({
      organization_id: handoff.organization_id,
      scenario_id: handoff.scenario_id,
      authorization_scope_digest: handoff.authorization_scope_digest,
    }, null, 2));
    status.textContent = "Exact grant scope copied. Manually enter it in the authenticated Control Room authorization workflow.";
  } catch (error) {
    status.textContent = `Could not copy the grant scope: ${error.message || "clipboard access is unavailable"}.`;
  }
});
document.getElementById("scenario-plan-approve").addEventListener("click", openScenarioPlanApprovalDialog);
document.getElementById("scenario-plan-budget-approve").addEventListener("click", () => {
  void approveEvaluationScenarioBudget();
});
document.getElementById("scenario-plan-explicit-confirm").addEventListener("change", scenarioPlanRenderControls);
document.getElementById("scenario-plan-confirm-approval").addEventListener("click", () => {
  void confirmScenarioPlanApproval();
});
document.getElementById("scenario-plan-run-button").addEventListener("click", () => {
  void runApprovedScenario();
});
for (const id of ["scenario-plan-approval-cancel", "scenario-plan-approval-close-icon"]) {
  document.getElementById(id).addEventListener("click", () => {
    document.getElementById("scenario-plan-approval-dialog").close();
  });
}
document.getElementById("scenario-plan-approval-dialog").addEventListener("close", () => {
  document.getElementById("scenario-plan-explicit-confirm").checked = false;
  document.getElementById("scenario-plan-approval-dialog-status").textContent = "";
  const dialogError = document.getElementById("scenario-plan-approval-dialog-error");
  dialogError.textContent = "";
  dialogError.hidden = true;
  scenarioPlanRenderControls();
  const focusTarget = scenarioPlanState.approval
    ? document.getElementById("scenario-plan-approval-status")
    : scenarioPlanDialogReturnFocus;
  scenarioPlanDialogReturnFocus = null;
  if (focusTarget?.isConnected && !focusTarget.disabled && !focusTarget.closest("[hidden]")) {
    focusTarget.focus();
    return;
  }
  const scenarioSelect = document.getElementById("scenario-plan-scenario");
  (scenarioSelect.disabled ? document.getElementById("scenario-plan-organization") : scenarioSelect).focus();
});
window.addEventListener("hashchange", updateScenarioTrackScroll);
document.addEventListener("simulator:activate", event => {
  if (event.detail.path === "/scenarios.html") updateScenarioTrackScroll();
});

function updateScenarioTrackScroll() {
  const frame = document.querySelector(".scenario-track-frame");
  if (!frame) return;
  const steps = document.getElementById("scenario-steps");
  let scrollbar = frame.querySelector(".scenario-track-scrollbar");
  let viewport = frame.querySelector(".scenario-track-viewport");
  if (!viewport) {
    scrollbar = document.createElement("div");
    scrollbar.className = "scenario-track-scrollbar";
    scrollbar.setAttribute("role", "region");
    scrollbar.setAttribute("aria-label", "Scroll scenario tracks horizontally");
    scrollbar.tabIndex = 0;
    scrollbar.append(document.createElement("div"));

    viewport = document.createElement("div");
    viewport.className = "scenario-track-viewport";
    viewport.setAttribute("role", "region");
    viewport.setAttribute("aria-label", "Scenario machine tracks");
    viewport.tabIndex = 0;
    frame.removeAttribute("role");
    frame.removeAttribute("aria-label");
    frame.removeAttribute("tabindex");
    frame.insertBefore(scrollbar, steps);
    frame.insertBefore(viewport, steps);
    viewport.append(steps);

    let syncing = false;
    const syncScroll = (source, target) => {
      if (syncing) return;
      syncing = true;
      target.scrollLeft = source.scrollLeft;
      syncing = false;
    };
    scrollbar.addEventListener("scroll", () => syncScroll(scrollbar, viewport), { passive: true });
    viewport.addEventListener("scroll", () => syncScroll(viewport, scrollbar), { passive: true });
    window.addEventListener("resize", updateScenarioTrackScroll);
  }
  scrollbar.firstElementChild.style.width = `${steps.scrollWidth}px`;
  scrollbar.hidden = steps.scrollWidth <= viewport.clientWidth + 1;
  scrollbar.scrollLeft = viewport.scrollLeft;
}

function renderScenarioRun(run) {
  const sameRun = scenarioRun?.scenario_id === run?.scenario_id && scenarioRun?.started_at === run?.started_at;
  const previousProgress = JSON.stringify([scenarioRun?.scenario_id, scenarioRun?.state, scenarioRun?.current_step, scenarioRun?.results]);
  scenarioRun = run || { state: "idle", steps: [] };
  const logContext = scenarioPreviewContext();
  if (scenarioLogContext !== logContext) {
    scenarioLogContext = logContext;
    scenarioLogMachines.clear();
    scenarioLogError = "";
    scenarioLogsLoaded = false;
  }
  const busy = scenarioRun.state === "running" || scenarioRun.state === "cleaning";
  const listedScenario = scenarioById.get(scenarioRun.scenario_id);
  const scenarioName = scenarioRun.scenario_name || listedScenario?.name || "Scenario";
  const title = document.getElementById("scenario-run-title");
  const state = document.getElementById("scenario-run-state");
  const actions = document.querySelector(".scenario-run-actions");
  const heading = title.parentElement;
  if (actions.parentElement !== heading) heading.append(actions);
  title.textContent = scenarioRun.scenario_id ? scenarioName : "Scenario";
  const stateLabel = document.createElement("span");
  stateLabel.className = "scenario-state-label";
  stateLabel.textContent = String(scenarioRun.state || "idle");
  const progress = document.createElement("span");
  progress.className = "scenario-progress";
  const failures = (scenarioRun.steps || []).filter(step => step.status === "failed");
  const firstFailure = failures.find(step => step.phase !== "cleanup") || failures[0];
  const failedNumber = firstFailure?.number;
  const showFailure = firstFailure && (scenarioRun.state === "failed" || scenarioRun.state === "cleaning");
  progress.textContent = showFailure
    ? `Failed step ${failedNumber} / ${scenarioRun.total_steps || 0}${scenarioRun.state === "cleaning" ? ` · Cleanup ${scenarioRun.current_step || 0} / ${scenarioRun.total_steps || 0}` : ""}`
    : `${scenarioRun.current_step || 0} / ${scenarioRun.total_steps || 0}`;
  progress.hidden = !scenarioRun.scenario_id || !scenarioRun.total_steps;
  state.replaceChildren(stateLabel, progress);
  state.className = `scenario-state ${scenarioEscape(scenarioRun.state || "idle")}`;
  if (busy && previousProgress !== JSON.stringify([scenarioRun.scenario_id, scenarioRun.state, scenarioRun.current_step, scenarioRun.results])
    && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    state.animate([
      { opacity: 0.8, transform: "scale(1)", boxShadow: "var(--scenario-progress-shadow)" },
      { opacity: 1, transform: "scale(1.05)", boxShadow: "var(--scenario-progress-pulse-shadow)", offset: 0.45 },
      { opacity: 1, transform: "scale(1)", boxShadow: "var(--scenario-progress-shadow)" },
    ], { duration: 750, easing: "ease-in-out" });
  }
  document.getElementById("scenario-cancel").hidden = !busy;
  document.getElementById("scenario-cancel").disabled = !busy;
  document.getElementById("scenario-reset").hidden = busy || !scenarioRun.scenario_id;
  renderScenarioResetControl();
  const summary = document.getElementById("scenario-run-summary");
  if (!scenarioRun.scenario_id) {
    summary.hidden = false;
    summary.innerHTML = `<span>No run selected</span>`;
  } else {
    const redundantCancellation = scenarioRun.state === "cancelled" && /^context canceled\.?$/i.test(String(scenarioRun.error || "").trim());
    const error = redundantCancellation ? "" : scenarioRun.error;
    const failureDetail = showFailure
      ? `Step ${failedNumber}: ${String(firstFailure.action || "unknown action").replaceAll("_", " ")}${firstFailure.machine ? ` on ${firstFailure.machine}` : ""}${firstFailure.error ? ` — ${firstFailure.error}` : ""}`
      : "";
    summary.innerHTML = `${failureDetail ? `<p class="scenario-run-error">${scenarioEscape(failureDetail)}</p>` : ""}${error && error !== firstFailure?.error ? `<p class="scenario-run-error">${scenarioEscape(error)}</p>` : ""}`;
    summary.hidden = !error && !failureDetail;
  }
  const results = scenarioRun.steps || [];
  const scenario = scenarioRun.scenario || listedScenario?.published_scenario || listedScenario;
  renderScenarioResults(scenarioRun, scenario);
  const planned = scenario ? [
    ...(scenario.steps || []).map((step) => ({ ...step, phase: "run" })),
    ...(scenario.cleanup || []).map((step) => ({ ...step, phase: "cleanup" })),
  ] : results;
  const finished = new Map(results.map((step) => [step.number, step]));
  const openDetails = new Set(sameRun ? [...document.querySelectorAll(".scenario-step-details[open]")].map(item => item.dataset.step) : []);
  const activeSteps = new Set(scenarioRun.active_steps || []);
  const lanes = new Map();
  planned.forEach((step, index) => {
    const phase = step.phase === "cleanup" ? "cleanup" : "run";
    const result = scenario ? finished.get(index + 1) : step;
    const active = !result && activeSteps.has(index + 1);
    const status = result?.status || (active ? "running" : "pending");
    const waiting = phase === "run" ? scenarioRun.state === "running" : busy;
    let detail = result?.error || result?.output || (active ? "Running" : status === "pending" ? waiting ? "Waiting" : "Not run" : "Completed");
    if (scenarioRun.state === "cancelled" && /^context canceled\.?$/i.test(String(detail).trim())) detail = "";
    const time = result?.finished_at ? window.ZPRSafeDisplay.formatTime(result.finished_at) : "";
    const machine = step.machine || "Shared";
    const lane = lanes.get(machine) || [];
    const detailMarkup = !detail ? "" : busy && !result?.error && status !== "failed"
      ? result?.output ? `<details class="scenario-step-details" data-step="${index + 1}" ${openDetails.has(String(index + 1)) ? "open" : ""}><summary>Details</summary><p>${scenarioEscape(detail)}</p></details>` : ""
      : `<p>${scenarioEscape(detail)}</p>`;
    lane.push(`<li class="scenario-step ${scenarioEscape(status)} ${phase === "cleanup" ? "cleanup" : ""}"><span class="scenario-step-mark" aria-hidden="true"></span><div class="scenario-step-copy"><div><span class="scenario-step-number">${index + 1}</span><strong>${scenarioEscape(step.action.replaceAll("_", " "))}</strong>${step.component ? `<span>${scenarioEscape(step.component)}</span>` : ""}${phase === "cleanup" ? `<span>Cleanup</span>` : ""}</div>${detailMarkup}</div><time>${scenarioEscape(time)}</time></li>`);
    lanes.set(machine, lane);
  });
  document.getElementById("scenario-steps").innerHTML = [...lanes].map(([machine, steps]) => `<section class="scenario-lane"${machine === "Shared" ? "" : ` data-scenario-machine="${scenarioEscape(machine)}"`}><h3>${scenarioEscape(machine)}</h3><ol class="scenario-steps">${steps.join("")}</ol></section>`).join("");
  renderScenarioLogPreviews();
  updateScenarioTrackScroll();
  renderSelectedScenario();
  for (const button of document.querySelectorAll("[data-run-scenario]")) {
    button.disabled = busy || button.dataset.runOrganization !== activeScenarioOrganization || button.dataset.runPublished !== "true";
  }
}

function renderScenarioResults(run, scenario) {
  let section = document.getElementById("scenario-results");
  if (!section) {
    section = document.createElement("section");
    section.id = "scenario-results";
    section.className = "scenario-results";
    section.setAttribute("aria-label", "Scenario results");
    section.setAttribute("role", "tabpanel");
    section.setAttribute("aria-labelledby", "scenario-results-tab");
    const tracks = document.querySelector(".scenario-track-frame");
    tracks.id = "scenario-runner-panel";
    tracks.setAttribute("role", "tabpanel");
    tracks.setAttribute("aria-labelledby", "scenario-runner-tab");
    const tabs = document.createElement("div");
    tabs.className = "scenario-view-tabs";
    tabs.setAttribute("role", "tablist");
    tabs.setAttribute("aria-label", "Scenario run views");
    const selectView = (results) => {
      for (const tab of tabs.children) {
        const selected = (tab.id === "scenario-results-tab") === results;
        tab.setAttribute("aria-selected", String(selected));
        tab.tabIndex = selected ? 0 : -1;
      }
      tracks.hidden = results;
      section.hidden = !results;
      if (!results) updateScenarioTrackScroll();
    };
    for (const [id, label, panel] of [
      ["scenario-runner-tab", "Runner", tracks.id],
      ["scenario-results-tab", "Results (0)", section.id],
    ]) {
      const tab = document.createElement("button");
      tab.id = id;
      tab.type = "button";
      tab.textContent = label;
      tab.setAttribute("role", "tab");
      tab.setAttribute("aria-controls", panel);
      tab.addEventListener("click", () => selectView(id === "scenario-results-tab"));
      tabs.append(tab);
    }
    tabs.addEventListener("keydown", (event) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const enabled = [...tabs.children].filter(tab => !tab.disabled);
      const index = enabled.indexOf(document.activeElement);
      const next = event.key === "Home" ? 0 : event.key === "End" ? enabled.length - 1 :
        (index + (event.key === "ArrowLeft" ? -1 : 1) + enabled.length) % enabled.length;
      enabled[next].click();
      enabled[next].focus();
    });
    document.querySelector(".scenario-run-heading").insertBefore(tabs, document.getElementById("scenario-run-state"));
    tracks.after(section);
    selectView(false);
  }
  const steps = (run.steps || []).filter(step => step.result || step.result_error);
  const resultsTab = document.getElementById("scenario-results-tab");
  resultsTab.textContent = `Results (${steps.length})`;
  resultsTab.disabled = steps.length === 0;
  if (!steps.length && resultsTab.getAttribute("aria-selected") === "true") {
    const restoreFocus = document.activeElement === resultsTab;
    document.getElementById("scenario-runner-tab").click();
    if (restoreFocus) document.getElementById("scenario-runner-tab").focus();
  }
  const signature = JSON.stringify([run.scenario_id, run.started_at, steps, scenario?.steps, scenario?.cleanup]);
  if (section.dataset.signature === signature) return;
  section.dataset.signature = signature;
  section.replaceChildren();
  if (!steps.length) return;
  const heading = document.createElement("h3");
  heading.textContent = "Results";
  section.append(heading);
  const planned = [...(scenario?.steps || []), ...(scenario?.cleanup || [])];
  const text = (parent, tag, content, className = "") => {
    const element = document.createElement(tag);
    element.textContent = content;
    if (className) element.className = className;
    parent.append(element);
    return element;
  };
  for (const step of steps) {
    const card = document.createElement("article");
    card.className = "scenario-result";
    const definition = planned[step.number - 1];
    text(card, "h4", `Step ${step.number}${step.machine ? ` · ${step.machine}` : ""} · Gateway fetch`);
    if (!step.result) {
      text(card, "p", step.result_error || "Result unavailable", "scenario-run-error");
      section.append(card);
      continue;
    }
    const result = step.result;
    if (result.schema_version !== 1 || result.kind !== "http") {
      text(card, "p", "Unsupported result format", "scenario-run-error");
      section.append(card);
      continue;
    }
    const status = result.status_code ? `HTTP ${result.status_code} ${result.status_text || ""}`.trim() : "No HTTP response";
    text(card, "p", `${status} · ${Number(result.duration_ms || 0).toFixed(1)} ms · ${result.read_limited ? "at least " : ""}${result.bytes_read || 0} body bytes read`, "scenario-result-summary");
    text(card, "p", `${result.method || "GET"} ${result.requested_url || ""}`);
    if (result.final_url) text(card, "p", `Final URL: ${result.final_url}`);
    text(card, "p", `Content type: ${result.content_type || "not supplied"}`);
    const expected = definition?.expected;
    text(card, "p", expected
      ? `Expectation: ${expected} · Step ${step.status}${step.error ? `: ${step.error}` : ""}`
      : `Expectation: none (observational) · Step ${step.status}${step.error ? `: ${step.error}` : ""}`);
    if (result.error) text(card, "p", result.error, "scenario-run-error");
    const headers = document.createElement("details");
    text(headers, "summary", `Response headers (sensitive/unrecognized values redacted)${result.headers_limited ? " · truncated" : ""}`);
    const headerText = Object.entries(result.headers || {}).map(([name, values]) =>
      `${name}: ${Array.isArray(values) ? values.join("\n") : String(values)}`).join("\n");
    text(headers, "pre", headerText || "No response headers", "scenario-result-source");
    card.append(headers);
    const body = document.createElement("details");
    body.open = true;
    text(body, "summary", `Response body · ${result.body_encoding === "base64" ? "base64 (non-UTF-8 bytes)" : "raw source"}${result.body_truncated ? " · truncated" : ""}`);
    if (result.body_truncated) text(body, "p", "Only a bounded response preview is shown; this is not the complete body.");
    const source = text(body, "pre", result.body || (result.status_code ? "(empty body)" : "(no response body)"), "scenario-result-source");
    source.tabIndex = 0;
    source.setAttribute("aria-label", `Step ${step.number} raw response body`);
    card.append(body);
    section.append(card);
  }
}

function scenarioResetActorMatches(identity, actor) {
  return Boolean(identity?.issuer && identity?.subject &&
    actor?.issuer === identity.issuer && actor?.subject === identity.subject);
}

function scenarioResetHasPermission(identity, organization) {
  return Boolean(identity?.issuer && identity?.subject && organization &&
    Array.isArray(identity.organizations) &&
    (identity.organizations.includes(organization) || identity.organizations.includes("*")) &&
    Array.isArray(identity.permissions) && identity.permissions.includes("scenario.reset"));
}

function scenarioResetActorLabel(identity) {
  const label = identity?.display_name || identity?.email || identity?.subject || "Unknown operator";
  return `${label} (${identity?.issuer || "unknown issuer"} / ${identity?.subject || "unknown subject"})`;
}

function scenarioResetAvailability(message) {
  const element = document.getElementById("scenario-reset-availability");
  element.textContent = message;
  element.hidden = !message;
}

function renderScenarioResetControl() {
  const button = document.getElementById("scenario-reset");
  const hasRun = Boolean(scenarioRun?.scenario_id);
  const busy = ["running", "cleaning"].includes(scenarioRun?.state);
  button.hidden = !hasRun || busy;
  if (!hasRun || busy) {
    button.disabled = true;
    scenarioResetAvailability("");
    return;
  }
  const organization = activeScenarioOrganization;
  if (!scenarioResetHasPermission(scenarioResetIdentity, organization)) {
    button.disabled = true;
    scenarioResetAvailability(organization
      ? "Sign in with the named Simulator identity and scenario.reset permission for this organization to reset scenario state."
      : "Select a Simulator organization before resetting scenario state.");
    return;
  }
  if (scenarioResetStatusOrganization !== organization || !scenarioResetStatus) {
    button.disabled = true;
    scenarioResetAvailability(scenarioResetAvailabilityMessage ||
      `Checking reset availability for ${organization}…`);
    return;
  }
  const runState = scenarioResetStatus.run_state;
  const stateAllowsReset = ["idle", "completed", "cancelled", "failed"].includes(runState);
  button.disabled = !stateAllowsReset;
  if (!stateAllowsReset) {
    scenarioResetAvailability(`Reset unavailable while the configured organization reports ${runState}.`);
  } else if (scenarioResetFlow?.organization === organization &&
      scenarioResetFlow.phase === "pending") {
    scenarioResetAvailability("A reset intent is awaiting explicit confirmation.");
  } else {
    scenarioResetAvailability("");
  }
  button.textContent = scenarioResetFlow?.organization === organization &&
    ["pending", "unknown", "indeterminate", "interrupted"].includes(scenarioResetFlow.phase)
    ? "Review reset status"
    : "Reset scenario state…";
}

let scenarioResetAvailabilityMessage = "";

function clearScenarioResetExpiryTimer() {
  if (scenarioResetExpiryTimer !== null) {
    clearInterval(scenarioResetExpiryTimer);
    scenarioResetExpiryTimer = null;
  }
}

function closeScenarioResetDialog() {
  const dialog = document.getElementById("scenario-reset-dialog");
  if (dialog.open) dialog.close();
  clearScenarioResetExpiryTimer();
}

function invalidateScenarioReset(message) {
  scenarioResetRequestSerial++;
  scenarioResetStatus = null;
  scenarioResetStatusOrganization = "";
  scenarioResetFlow = null;
  closeScenarioResetDialog();
  scenarioResetAvailabilityMessage = message || "";
  renderScenarioResetControl();
}

function resetOperationIsValid(operation, flow) {
  const identity = scenarioResetIdentity;
  const created = Date.parse(operation?.created_at);
  const expires = Date.parse(operation?.expires_at);
  const updated = operation?.updated_at === undefined ? null : Date.parse(operation.updated_at);
  const hasCurrentEpoch = operation?.current_epoch !== undefined;
  const hasCurrentGeneration = operation?.current_generation !== undefined;
  return Boolean(operation && operation.operation_id === flow.operationID &&
    operation.organization === flow.organization &&
    operation.action === "scenario.reset" &&
    ["intent", "consumed", "completed", "failed", "indeterminate", "interrupted", "expired"].includes(operation.state) &&
    scenarioResetActorMatches(identity, operation.actor) &&
    operation.epoch === flow.epoch && operation.generation === flow.generation &&
    Number.isSafeInteger(operation.epoch) && operation.epoch > 0 &&
    Number.isSafeInteger(operation.generation) && operation.generation > 0 &&
    (!hasCurrentEpoch && !hasCurrentGeneration ||
      Number.isSafeInteger(operation.current_epoch) && operation.current_epoch > 0 &&
      Number.isSafeInteger(operation.current_generation) && operation.current_generation > 0) &&
    Number.isFinite(created) && Number.isFinite(expires) &&
    (updated === null || Number.isFinite(updated)) &&
    expires > created && expires - created <= 120_000 &&
    (operation.outcome === undefined || typeof operation.outcome === "string") &&
    (operation.error_code === undefined || typeof operation.error_code === "string"));
}

function resetOperationStatusIsValid(operation, flow) {
  return resetOperationIsValid(operation, flow) &&
    Number.isSafeInteger(operation.current_epoch) && operation.current_epoch > 0 &&
    Number.isSafeInteger(operation.current_generation) && operation.current_generation > 0 &&
    Number.isFinite(Date.parse(operation.updated_at));
}

function resetOwnRunTransitionIsCurrent(flow, organization, run) {
  return Boolean(flow && flow.organization === organization &&
    ["completed", "unknown", "indeterminate", "interrupted"].includes(flow.phase) &&
    ["completed", "consumed", "indeterminate", "interrupted"].includes(flow.operation?.state) &&
    flow.operation.current_epoch === flow.epoch &&
    Number.isSafeInteger(flow.generation) && flow.generation < Number.MAX_SAFE_INTEGER &&
    flow.operation.current_generation === flow.generation + 1 &&
    run.state === "idle" && !run.scenario_id);
}

function scenarioResetIntentIsCurrent(flow) {
  return Boolean(flow && scenarioResetIdentity && activeScenarioOrganization === flow.organization &&
    scenarioResetHasPermission(scenarioResetIdentity, flow.organization) &&
    scenarioResetActorMatches(scenarioResetIdentity, flow.operation?.actor) &&
    flow.operation?.organization === flow.organization &&
    flow.operation?.action === "scenario.reset" &&
    flow.operation?.epoch === flow.epoch && flow.operation?.generation === flow.generation &&
    scenarioResetStatusOrganization === flow.organization &&
    scenarioResetStatus?.organization === flow.organization &&
    scenarioResetStatus.epoch === flow.epoch && scenarioResetStatus.generation === flow.generation);
}

function renderScenarioResetDialog() {
  const dialog = document.getElementById("scenario-reset-dialog");
  const flow = scenarioResetFlow;
  if (!flow) return;
  const operation = flow.operation;
  const identity = scenarioResetIdentity;
  const pending = flow.phase === "pending";
  const expired = !operation || !Number.isFinite(Date.parse(operation.expires_at)) ||
    Date.parse(operation.expires_at) <= Date.now();
  const validForConfirm = pending && !expired && scenarioResetIntentIsCurrent(flow);
  document.getElementById("scenario-reset-organization").textContent = flow.organization;
  document.getElementById("scenario-reset-generation").textContent =
    `${flow.epoch} / ${flow.generation}`;
  document.getElementById("scenario-reset-actor").textContent = scenarioResetActorLabel(identity);
  document.getElementById("scenario-reset-expiry").textContent = operation?.expires_at
    ? new Date(operation.expires_at).toLocaleString()
    : "Not available";
  document.getElementById("scenario-reset-effect").textContent =
    "Clear only the selected organization's scenario run state. Definitions, machines, and endpoint identities are retained; production state is not changed.";
  const detail = document.getElementById("scenario-reset-dialog-status");
  const phaseMessages = {
    creating: "Creating a time-limited reset intent. No scenario state has been changed.",
    pending: expired ? "This reset intent has expired; it cannot be confirmed." :
      "Review the exact target and effect, then explicitly confirm before scenario state changes.",
    confirming: "Confirmation was sent once. Checking its recorded outcome; do not repeat the reset.",
    checking: "Checking the recorded operation status only. No reset mutation is being retried.",
    unknown: flow.note || "The reset outcome is unknown. Check operation status; do not repeat the mutation.",
    expired: "This reset intent has expired; it cannot be confirmed.",
    stale: flow.note || "The selected organization or run generation changed. This intent cannot be confirmed.",
    completed: "Scenario state reset completed.",
    failed: flow.note || "The reset failed. No successful reset is claimed.",
    indeterminate: flow.note || "The reset outcome is indeterminate. Do not repeat the mutation.",
    interrupted: flow.note || "The reset was interrupted. Do not repeat the mutation.",
  };
  detail.textContent = phaseMessages[flow.phase] || "Reset operation status is unavailable.";
  detail.dataset.state = flow.phase;
  document.getElementById("scenario-reset-confirm").disabled =
    !validForConfirm || flow.confirmAttempted;
  document.getElementById("scenario-reset-confirm").hidden = !pending;
  document.getElementById("scenario-reset-check-status").hidden =
    !["unknown", "indeterminate", "interrupted"].includes(flow.phase);
  document.getElementById("scenario-reset-close").textContent = pending ? "Not now" : "Close";
  if (pending && !expired && !scenarioResetExpiryTimer) {
    scenarioResetExpiryTimer = setInterval(() => {
      if (scenarioResetFlow !== flow) {
        clearScenarioResetExpiryTimer();
        return;
      }
      if (Date.parse(operation.expires_at) <= Date.now()) {
        flow.phase = "expired";
        clearScenarioResetExpiryTimer();
      }
      renderScenarioResetDialog();
    }, 1000);
  } else if ((!pending || expired) && scenarioResetExpiryTimer) {
    clearScenarioResetExpiryTimer();
  }
  if (!dialog.open) dialog.showModal();
  renderScenarioResetControl();
}

function validScenarioResetStatus(status, organization) {
  return Boolean(status && status.organization === organization &&
    Number.isSafeInteger(status.epoch) && status.epoch > 0 &&
    Number.isSafeInteger(status.generation) && status.generation > 0 &&
    ["idle", "running", "cleaning", "completed", "cancelled", "failed", "other-organization"]
      .includes(status.run_state));
}

async function refreshScenarioResetStatus(organization = activeScenarioOrganization) {
  const serial = ++scenarioResetRequestSerial;
  const identity = scenarioResetIdentity;
  scenarioResetStatus = null;
  scenarioResetStatusOrganization = organization;
  scenarioResetAvailabilityMessage = `Checking reset availability for ${organization || "the selected organization"}…`;
  renderScenarioResetControl();
  if (!scenarioResetHasPermission(identity, organization)) {
    scenarioResetAvailabilityMessage = "";
    scenarioResetStatusOrganization = organization;
    renderScenarioResetControl();
    return null;
  }
  try {
    const status = await scenarioResetRequest(
      `${scenarioResetPath(organization)}/status`,
    );
    if (serial !== scenarioResetRequestSerial || organization !== activeScenarioOrganization ||
        !scenarioResetActorMatches(scenarioResetIdentity, {
          issuer: identity.issuer, subject: identity.subject,
        })) return null;
    if (!validScenarioResetStatus(status, organization)) {
      throw new Error("Reset status does not match the selected organization or has invalid generation data.");
    }
    scenarioResetStatus = status;
    scenarioResetStatusOrganization = organization;
    scenarioResetAvailabilityMessage = "";
    if (scenarioResetFlow?.organization === organization && scenarioResetFlow.phase === "pending" &&
        (scenarioResetFlow.epoch !== status.epoch || scenarioResetFlow.generation !== status.generation)) {
      scenarioResetFlow.phase = "stale";
      scenarioResetFlow.note = "The selected organization's epoch or run generation changed; this reset intent is no longer valid.";
      if (document.getElementById("scenario-reset-dialog").open) renderScenarioResetDialog();
    }
    renderScenarioResetControl();
    return status;
  } catch (error) {
    if (serial !== scenarioResetRequestSerial) return null;
    scenarioResetAvailabilityMessage = `Reset unavailable: ${error.message}`;
    scenarioResetStatus = null;
    scenarioResetStatusOrganization = organization;
    if (scenarioResetFlow?.organization === organization && scenarioResetFlow.phase === "pending") {
      scenarioResetFlow.phase = "unknown";
      scenarioResetFlow.note = "Current target status could not be verified. Confirmation is disabled; check status before any further action.";
      if (document.getElementById("scenario-reset-dialog").open) renderScenarioResetDialog();
    }
    renderScenarioResetControl();
    return null;
  }
}

function mapScenarioResetOperation(operation) {
  if (operation.state === "intent") return "pending";
  if (operation.state === "completed") return "completed";
  if (operation.state === "failed") return "failed";
  if (operation.state === "indeterminate") return "indeterminate";
  if (operation.state === "interrupted") return "interrupted";
  if (operation.state === "expired") return "expired";
  return "unknown";
}

async function checkScenarioResetOperation(flow = scenarioResetFlow) {
  if (!flow || flow !== scenarioResetFlow || !scenarioResetIdentity ||
      activeScenarioOrganization !== flow.organization ||
      !scenarioResetHasPermission(scenarioResetIdentity, flow.organization)) return;
  flow.phase = "checking";
  flow.note = "";
  renderScenarioResetDialog();
  try {
    const operation = await scenarioResetRequest(
      `${scenarioResetPath(flow.organization)}/operations/${encodeURIComponent(flow.operationID)}`,
    );
    if (flow !== scenarioResetFlow || !resetOperationStatusIsValid(operation, flow)) {
      throw new Error("Operation status does not match this target, intent, or signed-in identity.");
    }
    flow.operation = operation;
    if (operation.state === "intent") {
      if (flow.confirmAttempted) {
        flow.phase = "unknown";
        flow.note = "The confirmation response was ambiguous. The operation is still an intent; no mutation was retried.";
      } else {
        const status = await refreshScenarioResetStatus(flow.organization);
        flow.phase = Date.parse(operation.expires_at) <= Date.now() ? "expired"
          : status && status.epoch === flow.epoch && status.generation === flow.generation
            ? "pending" : "stale";
      }
    } else {
      flow.phase = mapScenarioResetOperation(operation);
      if (operation.state === "consumed") {
        flow.phase = "unknown";
        flow.note = "The operation is consumed but has no definitive outcome. It remains unknown; do not retry the reset.";
      }
      if (flow.phase === "completed") {
        void refreshScenariosNow();
        void refreshScenarioResetStatus(flow.organization);
      }
    }
  } catch (error) {
    if (flow !== scenarioResetFlow) return;
    flow.phase = "unknown";
    flow.note = `Operation status unavailable: ${error.message}. No mutation was retried.`;
  }
  if (flow === scenarioResetFlow) renderScenarioResetDialog();
}

async function beginScenarioReset() {
  const organization = activeScenarioOrganization;
  const identity = scenarioResetIdentity;
  if (!scenarioResetHasPermission(identity, organization) ||
      !scenarioRun?.scenario_id || ["running", "cleaning"].includes(scenarioRun.state)) return;
  if (scenarioResetFlow?.organization === organization &&
      ["pending", "unknown", "indeterminate", "interrupted"].includes(scenarioResetFlow.phase)) {
    renderScenarioResetDialog();
    return;
  }
  const status = await refreshScenarioResetStatus(organization);
  if (!status || !scenarioResetActorMatches(scenarioResetIdentity, {
    issuer: identity.issuer, subject: identity.subject,
  }) || activeScenarioOrganization !== organization) return;
  const operationBytes = crypto.getRandomValues(new Uint8Array(16));
  const operationID = [...operationBytes].map(byte => byte.toString(16).padStart(2, "0")).join("");
  const flow = {
    organization,
    epoch: status.epoch,
    generation: status.generation,
    operationID,
    operation: null,
    phase: "creating",
    confirmAttempted: false,
    note: "",
  };
  scenarioResetFlow = flow;
  renderScenarioResetDialog();
  try {
    const operation = await scenarioResetRequest(`${scenarioResetPath(organization)}/intents`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        operation_id: operationID,
        organization,
        epoch: status.epoch,
        generation: status.generation,
      }),
    });
    if (scenarioResetFlow !== flow || !resetOperationIsValid(operation, flow) ||
        operation.state !== "intent" || Date.parse(operation.expires_at) <= Date.now() ||
        !scenarioResetIntentIsCurrent({ ...flow, operation })) {
      throw new Error("Reset intent did not match the selected target, generation, actor, or expiry.");
    }
    flow.operation = operation;
    flow.phase = "pending";
  } catch (error) {
    if (scenarioResetFlow !== flow) return;
    flow.phase = "unknown";
    flow.note = `Intent response was ambiguous or invalid: ${error.message}. Checking operation status without retrying creation.`;
    renderScenarioResetDialog();
    await checkScenarioResetOperation(flow);
    return;
  }
  renderScenarioResetDialog();
}

async function confirmScenarioReset() {
  const flow = scenarioResetFlow;
  if (!flow || flow.phase !== "pending" || flow.confirmAttempted ||
      !scenarioResetIntentIsCurrent(flow) ||
      Date.parse(flow.operation?.expires_at) <= Date.now()) {
    if (flow) {
      flow.phase = "stale";
      flow.note = "The signed-in identity, selected organization, run generation, or intent expiry changed. No confirmation was sent.";
      renderScenarioResetDialog();
    }
    return;
  }
  const status = await refreshScenarioResetStatus(flow.organization);
  if (!status || !scenarioResetIntentIsCurrent(flow) ||
      status.epoch !== flow.epoch || status.generation !== flow.generation ||
      Date.parse(flow.operation.expires_at) <= Date.now()) {
    flow.phase = "stale";
    flow.note = "The latest target status no longer matches this intent. No confirmation was sent.";
    renderScenarioResetDialog();
    return;
  }
  flow.confirmAttempted = true;
  flow.phase = "confirming";
  renderScenarioResetDialog();
  try {
    const operation = await scenarioResetRequest(
      `${scenarioResetPath(flow.organization)}/operations/${encodeURIComponent(flow.operationID)}/confirm`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          organization: flow.organization,
          epoch: flow.epoch,
          generation: flow.generation,
        }),
      },
    );
    if (flow !== scenarioResetFlow || !resetOperationStatusIsValid(operation, flow) ||
        operation.state !== "completed") {
      throw new Error("Confirmation response does not match the original intent.");
    }
    flow.operation = operation;
    flow.phase = operation.state === "consumed" ? "unknown" : mapScenarioResetOperation(operation);
    if (flow.phase === "unknown") {
      flow.note = "The reset operation is consumed but its outcome is unknown. Check status; do not retry the mutation.";
    }
    if (flow.phase === "completed") {
      void refreshScenariosNow();
      void refreshScenarioResetStatus(flow.organization);
    }
  } catch (error) {
    if (flow !== scenarioResetFlow) return;
    flow.phase = "unknown";
    flow.note = `Confirmation outcome is unknown: ${error.message}. Checking status only; the reset will not be retried.`;
    renderScenarioResetDialog();
    await checkScenarioResetOperation(flow);
    return;
  }
  renderScenarioResetDialog();
}

function scenarioCatalogOrganization() {
  const allowed = scenarioResetIdentity?.organizations;
  if (Array.isArray(allowed) && !allowed.includes("*")) {
    if (scenarioPlanState.organizationID && allowed.includes(scenarioPlanState.organizationID)) {
      return scenarioPlanState.organizationID;
    }
    if (activeScenarioOrganization && allowed.includes(activeScenarioOrganization)) {
      return activeScenarioOrganization;
    }
    return allowed.length === 1 ? allowed[0] : "";
  }
  return scenarioPlanState.organizationID || activeScenarioOrganization;
}

async function refreshScenarios({ signal, isCurrent }) {
  const requestedOrganizationID = scenarioCatalogOrganization();
  const path = requestedOrganizationID
    ? `/api/simulator/organizations/${encodeURIComponent(requestedOrganizationID)}/scenarios`
    : "/api/simulator/scenarios";
  const data = await scenarioRequest(path, { cache: "no-store", signal });
  if (!isCurrent()) return;
  const error = document.getElementById("scenario-error");
  error.hidden = true;
  const organization = requestedOrganizationID || data.active_organization_id || activeScenarioOrganization;
  if (requestedOrganizationID && data.organization?.id !== requestedOrganizationID) {
    throw new Error("Scenario catalog did not match the authorized organization.");
  }
  if (organization !== activeScenarioOrganization &&
      (scenarioPlanState.plan || scenarioPlanState.approval || scenarioPlanState.run ||
        scenarioPlanState.runAttempted || scenarioPlanState.busy) &&
      scenarioPlanState.organizationID !== organization) {
    scenarioPlanInvalidate("Active Simulator organization changed; the previous plan and approval view were cleared. Preview again before requesting approval.");
  }
  const runSignature = JSON.stringify(data.run || {});
  const run = data.run || {};
  if (scenarioPlanState.run && run.run_id === scenarioPlanState.run.run_id &&
      run.organization_id === scenarioPlanState.organizationID &&
      run.scenario_id === scenarioPlanState.scenarioID &&
      run.plan_digest === scenarioPlanState.plan?.plan_digest) {
    scenarioPlanRenderRun(run);
    document.getElementById("scenario-plan-run-status").textContent =
      `Scenario run status refreshed: ${run.state}.`;
  }
  const resetRunSignature = JSON.stringify({
    organization,
    scenario: run.scenario_id || "",
    started: run.started_at || "",
    generation: run.generation,
    state: run.state || "idle",
  });
  const runChanged = scenarioResetRunSignature !== "" &&
    resetRunSignature !== scenarioResetRunSignature;
  scenarioResetRunSignature = resetRunSignature;
  const resetFlow = scenarioResetFlow;
  const confirmationInFlight = resetFlow?.organization === organization &&
    resetFlow.phase === "confirming" && resetFlow.confirmAttempted;
  const expectedResetTransition =
    resetOwnRunTransitionIsCurrent(resetFlow, organization, run);
  if (organization !== activeScenarioOrganization) {
    selectedScenarioID = "";
    if (activeScenarioOrganization) document.getElementById("scenario-browser-dialog").close();
    invalidateScenarioReset("Reset availability is being checked for the selected Simulator organization.");
  } else if (runChanged && !confirmationInFlight && !expectedResetTransition) {
    invalidateScenarioReset("Scenario run state changed; reset evidence is being refreshed.");
  }
  activeScenarioOrganization = organization;
  if (organization && scenarioResetIdentity &&
      (scenarioResetStatusOrganization !== organization || runChanged)) {
    void refreshScenarioResetStatus(organization);
  }
  if (scenarioEditorOrganization && scenarioEditorOrganization !== activeScenarioOrganization && document.getElementById("scenario-editor-dialog").open) {
    document.getElementById("scenario-editor-dialog").close();
  }
  const scenarios = data.scenarios || [];
  const maxMachines = Number.isInteger(data.max_machines) && data.max_machines > 0 ? data.max_machines : 10;
  if (window.ZPRSimulatorScenarioLimits?.maxMachines !== maxMachines) {
    window.ZPRSimulatorScenarioLimits = { maxMachines };
    window.dispatchEvent(new CustomEvent("simulator:scenario-limits"));
  }
  const catalogSignature = JSON.stringify({ scenarios, active: activeScenarioOrganization, maxMachines });
  const catalogChanged = catalogSignature !== scenarioCatalogSignature;
  if (catalogChanged) {
    scenarioCatalogSignature = catalogSignature;
    renderScenarioList(scenarios);
  }
  if (runSignature !== scenarioRunSignature || catalogChanged) {
    scenarioRunSignature = runSignature;
    renderScenarioRun(data.run);
  }
}

const scenarioPoller = window.ZPRSimulatorNavigation.createPagePoller({
  path: "/scenarios.html",
  run: refreshScenarios,
  interval: 1000,
  onError: (failure) => {
    const error = document.getElementById("scenario-error");
    error.textContent = failure.message || "Scenario catalog unavailable";
    error.hidden = false;
  },
});
const refreshScenariosNow = () => scenarioPoller.refresh();
window.ZPRSimulatorNavigation.createPagePoller({
  path: "/scenarios.html",
  run: refreshScenarioLogPreviews,
  interval: 3000,
  onError: failure => {
    if (!["running", "cleaning"].includes(scenarioRun?.state)) return;
    scenarioLogError = failure.message || "Machine logs unavailable";
    renderScenarioLogPreviews();
  },
});

function setScenarioEditorStatus(message, state = "") {
  scenarioEditorController.setStatus(message || "", state === "saved" ? "success" : state);
}

function renderScenarioIdentity() {
  const artifact = scenarioEditorArtifact;
  const name = document.getElementById("scenario-editor-name").value.trim();
  let label = "New · unsaved";
  if (artifact) {
    const published = artifact.published_revision === artifact.revision ? "published"
      : artifact.published_revision ? `published v${artifact.published_revision}` : "not published";
    label = scenarioEditorViewing ? `Viewing version ${scenarioEditorViewing}` : `Version ${artifact.revision} · ${published}`;
  }
  scenarioEditorController.renderIdentity({ name, label, tooltip: artifact?.id || "", dirty: scenarioEditorDirty });
}

function setScenarioAnalyzeState(state = "") {
  for (const button of document.querySelectorAll("#scenario-source-analyze")) {
    if (!button) continue;
    scenarioEditorController.setAnalysisState(state);
    button.classList.toggle("button-next-evaluate", !state);
  }
}

function updateScenarioEditorActions() {
  const publish = document.getElementById("scenario-editor-publish");
  const saved = scenarioEditorArtifact && !scenarioEditorDirty;
  publish.disabled = !saved || scenarioEditorArtifact.published_revision === scenarioEditorArtifact.revision;
  document.getElementById("scenario-editor-delete").hidden = !scenarioEditorArtifact;
  document.getElementById("scenario-editor-discard").disabled = !scenarioEditorDirty;
  renderScenarioIdentity();
}

const scenarioStepActions = [
  "start_machine", "wait_controller", "login", "select_workloads", "logout", "stop_machine",
  "start_workload", "stop_workload", "start_test_service", "stop_test_service",
  "request_test_service", "request_business_service", "benchmark_test_service", "start_service_fleet",
  "stop_service_fleet", "stress_traffic", "traffic", "resolve_dns", "delay",
];

function scenarioStepMarkup(step, index) {
  const actions = step.action && !scenarioStepActions.includes(step.action) ? [...scenarioStepActions, step.action] : scenarioStepActions;
  const actionOptions = actions.map((action) => `<option value="${scenarioEscape(action)}" ${step.action === action ? "selected" : ""}>${scenarioEscape(action.replaceAll("_", " "))}</option>`).join("");
  const expectedOptions = ["", "allow", "deny"].map((value) => `<option value="${value}" ${step.expected === value ? "selected" : ""}>${value || (step.action === "request_web_gateway" ? "None (observe only)" : "Not set")}</option>`).join("");
  const values = {
    id: step.id || "",
    machine: step.machine || "",
    user: step.user || "",
    component: step.component || "",
    target: step.target || "",
    timeout_seconds: step.timeout_seconds ?? "",
    workloads: Array.isArray(step.workloads) ? step.workloads.join(", ") : "",
    after: Array.isArray(step.after) ? step.after.join(", ") : "",
  };
  return `<fieldset class="scenario-step-editor-row" data-scenario-step data-original-step="${scenarioEscape(JSON.stringify(step))}"><legend>Step ${index + 1}<button class="quiet" type="button" data-remove-step aria-label="Remove step" title="Remove step">×</button></legend><div class="scenario-step-fields"><label data-parallel-field>Step ID<input data-step-key="id" value="${scenarioEscape(values.id)}" placeholder="step-id"></label><label>Action<select data-step-key="action">${actionOptions}</select></label><label>Machine<input data-step-key="machine" value="${scenarioEscape(values.machine)}" placeholder="machine-01"></label><label>User<input data-step-key="user" value="${scenarioEscape(values.user)}" placeholder="Optional user"></label><label>Workload component<input data-step-key="component" value="${scenarioEscape(values.component)}" placeholder="Optional component"></label><label>Target<input data-step-key="target" value="${scenarioEscape(values.target)}" placeholder="Target or service"></label><label>Expected result<select data-step-key="expected">${expectedOptions}</select></label><label>Timeout (seconds)<input data-step-key="timeout_seconds" type="number" min="0" max="180" value="${scenarioEscape(values.timeout_seconds)}"></label><label>Workloads<input data-step-key="workloads" value="${scenarioEscape(values.workloads)}" placeholder="client, service"></label><label data-parallel-field>After step IDs<input data-step-key="after" value="${scenarioEscape(values.after)}" placeholder="step-id, other-step"></label></div></fieldset>`;
}

function renderScenarioStepGroup(containerID, steps) {
  const container = document.getElementById(containerID);
  container.innerHTML = steps.length ? steps.map(scenarioStepMarkup).join("") : `<p class="scenario-step-empty">No steps yet.</p>`;
  updateScenarioParallelFields();
}

function updateScenarioParallelFields() {
  const parallel = document.getElementById("scenario-editor-parallel").checked;
  for (const field of document.querySelectorAll("[data-parallel-field]")) field.hidden = !parallel;
}

function renderScenarioEditor(scenario) {
  scenarioFormAnalysisScope.invalidate();
  scenarioEditorDraft = { ...scenario };
  document.getElementById("scenario-editor-id").value = scenario.id || "";
  document.getElementById("scenario-editor-id").disabled = Boolean(scenarioEditorArtifact);
  document.getElementById("scenario-editor-name").value = scenario.name || "";
  document.getElementById("scenario-editor-folder").value = scenario.folder || "";
  document.getElementById("scenario-editor-description").value = scenario.description || "";
  document.getElementById("scenario-editor-parallel").checked = Boolean(scenario.parallel);
  renderScenarioStepGroup("scenario-editor-steps", scenario.steps || []);
  renderScenarioStepGroup("scenario-editor-cleanup", scenario.cleanup || []);
  document.getElementById("scenario-editor-source").value = JSON.stringify(scenario, null, 2);
  scenarioEditorJsonDirty = false;
  if (window.setScenarioSourceFromScenario) window.setScenarioSourceFromScenario(scenario);
  else window.renderScenarioSourceEditor?.();
}

function readScenarioSteps(containerID) {
  return [...document.querySelectorAll(`#${containerID} [data-scenario-step]`)].map((row) => {
    let step = {};
    try { step = JSON.parse(row.dataset.originalStep || "{}"); } catch { step = {}; }
    for (const input of row.querySelectorAll("[data-step-key]")) {
      const key = input.dataset.stepKey;
      const value = input.value.trim();
      if (!value) {
        delete step[key];
        continue;
      }
      if (key === "timeout_seconds") step[key] = Number(value);
      else if (key === "workloads" || key === "after") step[key] = value.split(",").map((entry) => entry.trim()).filter(Boolean);
      else step[key] = value;
    }
    return step;
  });
}

function readScenarioEditorForm() {
  const parallel = document.getElementById("scenario-editor-parallel").checked;
  const steps = readScenarioSteps("scenario-editor-steps");
  if (!parallel) {
    for (const step of steps) {
      delete step.id;
      delete step.after;
    }
  }
  const scenario = {
    ...scenarioEditorDraft,
    id: document.getElementById("scenario-editor-id").value.trim(),
    organization_id: scenarioEditorOrganization,
    name: document.getElementById("scenario-editor-name").value.trim(),
    description: document.getElementById("scenario-editor-description").value.trim(),
    steps,
    cleanup: readScenarioSteps("scenario-editor-cleanup"),
  };
  const folder = document.getElementById("scenario-editor-folder").value.trim();
  if (folder) scenario.folder = folder;
  else delete scenario.folder;
  if (parallel) scenario.parallel = true;
  else delete scenario.parallel;
  return scenario;
}

function markScenarioEditorDirty(fromJSON = false) {
  scenarioEditorDirty = true;
  scenarioFormAnalysisScope.invalidate();
  setScenarioAnalyzeState("pending");
  if (!fromJSON) {
    scenarioEditorJsonDirty = false;
    document.getElementById("scenario-editor-source").value = JSON.stringify(readScenarioEditorForm(), null, 2);
  }
  updateScenarioEditorActions();
  window.renderScenarioSourceEditor?.();
}

async function analyzeScenarioEditor() {
  if (document.getElementById("scenario-editor-advanced").open && window.analyzeScenarioSourceEditor) {
    const result = await window.analyzeScenarioSourceEditor();
    return result?.scenario || null;
  }
  const isCurrent = scenarioEditorController.beginAnalysis("form");
  setScenarioEditorStatus("");
  try {
    const scenario = readScenarioEditorForm();
    const result = await scenarioRequest(`/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenario-check`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ format: "json", source: JSON.stringify(scenario), scenario_id: scenarioEditorArtifact?.id || "" }),
    });
    if (!isCurrent()) return null;
    if (result.valid !== true) throw new Error(result.error || "Scenario analysis failed.");
    setScenarioAnalyzeState("success");
    setScenarioEditorStatus(result.diagnostics || "Scenario definition valid. Nothing saved, published, or run.");
    return result.scenario || scenario;
  } catch (error) {
    if (!isCurrent()) return null;
    setScenarioAnalyzeState("error");
    setScenarioEditorStatus(error.message || "Scenario analysis failed.", "error");
    return null;
  } finally {
    isCurrent.finish();
  }
}

function addScenarioStep(cleanup = false) {
  const scenario = readScenarioEditorForm();
  const steps = cleanup ? scenario.cleanup : scenario.steps;
  steps.push(cleanup ? { action: "stop_machine" } : { action: "delay", timeout_seconds: 1 });
  renderScenarioStepGroup(cleanup ? "scenario-editor-cleanup" : "scenario-editor-steps", steps);
  markScenarioEditorDirty();
}

function showScenarioEditorPage() {
  const dialog = document.getElementById("scenario-editor-dialog");
  if (!dialog.open) dialog.show();
  window.scrollTo(0, 0);
}

function closeScenarioEditorPage() {
  if (!scenarioEditorPage.confirmDiscard(scenarioEditorDirty, "Discard unsaved scenario changes?")) return false;
  scenarioEditorDirty = false;
  document.getElementById("scenario-editor-dialog").close();
  return true;
}

function newScenarioTemplate() {
  return {
    id: "new-scenario",
    organization_id: scenarioEditorOrganization,
    folder: "",
    name: "",
    description: "Describe the behavior this scenario exercises.",
    steps: [{ action: "delay", timeout_seconds: 1 }],
    cleanup: [],
  };
}

function openNewScenarioEditor() {
  scenarioEditorArtifact = null;
  scenarioEditorOrganization = activeScenarioOrganization;
  scenarioEditorDirty = false;
  scenarioEditorViewing = 0;
  scenarioEditorSummary = "Initial version";
  scenarioAssistant.reset();
  document.getElementById("scenario-editor-advanced").open = false;
  scenarioEditorSaved = newScenarioTemplate();
  renderScenarioEditor(scenarioEditorSaved);
  scenarioHistory.render([]);
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  showScenarioEditorPage();
}

async function openExistingScenarioEditor(scenarioID) {
  const organizationID = activeScenarioOrganization;
  const path = `/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios/${encodeURIComponent(scenarioID)}`;
  const artifact = await scenarioRequest(path);
  scenarioEditorArtifact = artifact;
  scenarioEditorOrganization = organizationID;
  scenarioEditorDirty = false;
  scenarioEditorViewing = 0;
  scenarioEditorSummary = "";
  scenarioAssistant.reset();
  document.getElementById("scenario-editor-advanced").open = false;
  scenarioEditorSaved = artifact.content;
  renderScenarioEditor(artifact.content);
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  showScenarioEditorPage();
  await refreshScenarioRevisions();
}

async function refreshScenarioRevisions() {
  const artifact = scenarioEditorArtifact;
  if (!artifact) { scenarioHistory.render([]); return; }
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(artifact.id)}/revisions`;
  const revisions = await scenarioRequest(path);
  if (artifact !== scenarioEditorArtifact) return;
  scenarioHistory.render([...revisions].reverse().map((revision) => ({ ...revision, number: revision.revision })), {
    current: scenarioEditorViewing || artifact.revision,
    detail: (revision) => revision.summary || "",
    meta: (revision) => [revision.revision === artifact.published_revision ? "published" : "", revision.author || ""].filter(Boolean).join(" · "),
    onSelect: (revision) => loadScenarioRevision(revision.number).catch((error) => setScenarioEditorStatus(error.message || "Could not load version.", "error")),
  });
}

async function readScenarioEditorSource() {
  let scenario;
  try { scenario = await window.readScenarioSourceEditor(); }
  catch (error) {
    if (error.sourceDiagnostic) throw error;
    throw new Error(`Scenario must be valid JSON or YAML: ${error.message}`);
  }
  if (!scenario || typeof scenario !== "object" || Array.isArray(scenario)) throw new Error("Scenario JSON must be an object.");
  if (scenario.organization_id && scenario.organization_id !== scenarioEditorOrganization) throw new Error("Scenario organization_id must match the selected organization.");
  if (scenarioEditorArtifact && scenario.id !== scenarioEditorArtifact.id) throw new Error("The existing scenario ID cannot be changed.");
  scenario.organization_id = scenarioEditorOrganization;
  return scenario;
}

async function saveScenarioDraft() {
  const scenario = scenarioEditorJsonDirty ? await readScenarioEditorSource() : readScenarioEditorForm();
  if (!scenarioEditorPage.isNamed(scenario.name)) throw new Error("Enter a scenario name other than Untitled before saving.");
  const summary = scenarioEditorSummary || (scenarioEditorArtifact ? "Updated scenario draft" : "Initial version");
  const creating = !scenarioEditorArtifact;
  const path = creating
    ? `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios`
    : `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}`;
  const result = await scenarioRequest(path, {
    method: creating ? "POST" : "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ scenario, summary, expected_revision: scenarioEditorArtifact?.revision || 0 }),
  });
  if (creating) scenarioEditorArtifact = result;
  else scenarioEditorArtifact = { ...scenarioEditorArtifact, revision: result.revision, content: result.content, content_hash: result.content_hash, published_revision: scenarioEditorArtifact.published_revision || 0 };
  scenarioEditorDraft = scenario;
  scenarioEditorDirty = false;
  scenarioEditorSaved = scenario;
  scenarioEditorSummary = "";
  scenarioEditorViewing = 0;
  renderScenarioEditor(scenario);
  if (scenarioEditorOrganization === activeScenarioOrganization) selectedScenarioID = scenario.id;
  setScenarioEditorStatus(`Saved version ${scenarioEditorArtifact.revision}. Publish it to enable runs.`, "saved");
  await refreshScenarioRevisions();
  updateScenarioEditorActions();
  await refreshScenariosNow();
}

async function publishScenarioRevision() {
  if (!scenarioEditorArtifact || scenarioEditorDirty) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}/publish`;
  const artifact = await scenarioRequest(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_revision: scenarioEditorArtifact.revision }),
  });
  scenarioEditorArtifact = artifact;
  setScenarioEditorStatus(`Published version ${artifact.published_revision} for ${scenarioEditorOrganization}.`, "saved");
  updateScenarioEditorActions();
  await refreshScenarioRevisions();
  await refreshScenariosNow();
}

async function loadScenarioRevision(number) {
  if (!number || !scenarioEditorArtifact) return;
  const path = `/api/simulator/organizations/${encodeURIComponent(scenarioEditorOrganization)}/scenarios/${encodeURIComponent(scenarioEditorArtifact.id)}/revisions/${encodeURIComponent(number)}`;
  const revision = await scenarioRequest(path);
  if (scenarioEditorDirty && !window.confirm("Replace unsaved scenario changes with this version?")) return;
  renderScenarioEditor(revision.content);
  scenarioEditorSummary = `Restore version ${revision.revision}`;
  scenarioEditorViewing = revision.revision === scenarioEditorArtifact.revision ? 0 : revision.revision;
  scenarioEditorDirty = Boolean(scenarioEditorViewing);
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  await refreshScenarioRevisions();
}

async function archiveScenario(scenarioID, organizationID, revision) {
  const scenario = scenarioById.get(scenarioID);
  if (!scenario || !window.confirm(`Delete “${scenario.name}” from this folder? Its revision history will be retained.`)) return;
  const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organizationID)}/scenarios/${encodeURIComponent(scenarioID)}`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_revision: Number(revision) }),
  });
  if (!response.ok) {
    const result = await window.ZPRPageRuntime.readJSON(response);
    throw new Error(result.error || `HTTP ${response.status}`);
  }
  if (scenarioEditorArtifact?.id === scenarioID) {
    scenarioEditorDirty = false;
    document.getElementById("scenario-editor-dialog").close();
  }
  await refreshScenariosNow();
}

const scenarioAssistant = window.mountSimulatorDesignAssistant("scenario-assistant-slot", {
  scope: "scenario",
  editorID: "scenario-editor-dialog",
  applyLabel: "Apply scenario draft",
  emptyMessage: "Ask for a scenario outline, step plan, or review of this draft.",
  getContext: () => ({
    organization_id: scenarioEditorOrganization,
    scenario: scenarioEditorJsonDirty ? window.peekScenarioSourceEditor() : readScenarioEditorForm(),
    source: document.getElementById("scenario-editor-source").value,
    revision: scenarioEditorArtifact?.revision,
    viewing: scenarioEditorViewing,
  }),
  onApply: (proposal) => {
    const scenario = proposal.scenario;
    if (!scenario) throw new Error("Claude did not return a scenario draft.");
    if (scenarioEditorArtifact && scenario.id !== scenarioEditorArtifact.id) throw new Error("The proposal changed the existing scenario ID.");
    const before = scenarioEditorJsonDirty ? window.peekScenarioSourceEditor() : readScenarioEditorForm();
    const previousSource = document.getElementById("scenario-editor-source").value;
    const previousJsonDirty = scenarioEditorJsonDirty;
    const previousDirty = scenarioEditorDirty;
    const previousSummary = scenarioEditorSummary;
    const organization = scenarioEditorOrganization;
    const artifact = scenarioEditorArtifact;
    const revision = artifact?.revision;
    const viewing = scenarioEditorViewing;
    const current = () => JSON.stringify({ form: readScenarioEditorForm(), source: document.getElementById("scenario-editor-source").value, rawDirty: scenarioEditorJsonDirty });
    let expected;
    const restore = (value, text, rawDirty, dirty, summary) => {
      if (scenarioEditorOrganization !== organization || scenarioEditorArtifact !== artifact || artifact?.revision !== revision || scenarioEditorViewing !== viewing || current() !== expected) {
        throw new Error("The scenario changed after the AI edit. Undo/redo is no longer available.");
      }
      renderScenarioEditor(value);
      document.getElementById("scenario-editor-source").value = text;
      scenarioEditorJsonDirty = rawDirty;
      window.renderScenarioSourceEditor?.();
      scenarioEditorDirty = dirty;
      scenarioEditorSummary = summary;
      scenarioFormAnalysisScope.invalidate();
      setScenarioAnalyzeState("pending");
      updateScenarioEditorActions();
      setScenarioEditorStatus("AI change restored as an unsaved editor change.");
      expected = current();
    };
    scenario.organization_id = scenarioEditorOrganization;
    renderScenarioEditor(scenario);
    scenarioEditorDirty = true;
    scenarioFormAnalysisScope.invalidate();
    setScenarioAnalyzeState("pending");
    scenarioEditorSummary = "Claude-assisted scenario draft";
    setScenarioEditorStatus("Claude proposal applied to the editor. Review and save it as a draft.");
    updateScenarioEditorActions();
    const appliedSource = document.getElementById("scenario-editor-source").value;
    expected = current();
    return {
      undo: () => restore(before, previousSource, previousJsonDirty, previousDirty, previousSummary),
      redo: () => restore(scenario, appliedSource, false, true, "Claude-assisted scenario draft"),
    };
  },
});

async function postScenarioAction(url) {
  await scenarioRequest(url, { method: "POST" });
  await refreshScenariosNow();
}


scenarioEditorPage.createMenu({
  root: document.getElementById("scenario-files"),
  toggle: document.getElementById("scenario-files-toggle"),
  menu: document.getElementById("scenario-file-menu"),
});
document.getElementById("scenario-browse").addEventListener("click", () => {
  document.getElementById("scenario-browser-dialog").showModal();
});
document.getElementById("scenario-browser-close").addEventListener("click", () => {
  document.getElementById("scenario-browser-dialog").close();
});
document.getElementById("scenario-browser-dialog").addEventListener("close", () => {
  document.getElementById("scenario-browse").focus();
});
document.getElementById("scenario-browser-list").addEventListener("click", event => {
  const button = event.target.closest("[data-select-scenario]");
  if (!button || !scenarioById.has(button.dataset.selectScenario)) return;
  selectedScenarioID = button.dataset.selectScenario;
  renderScenarioList([...scenarioById.values()]);
  document.getElementById("scenario-browser-dialog").close();
});
document.getElementById("scenario-edit-selected").addEventListener("click", async () => {
  if (!scenarioById.has(selectedScenarioID)) return;
  try { await scenarioEditorController.perform("load", selectedScenarioID); }
  catch (error) {
    const message = document.getElementById("scenario-error");
    message.textContent = error.message || "Could not open scenario";
    message.hidden = false;
  }
});
document.getElementById("scenario-new").addEventListener("click", openNewScenarioEditor);
document.getElementById("scenario-editor-dialog").addEventListener("close", () => {
  scenarioFormAnalysisScope.invalidate();
});
document.getElementById("scenario-editor-advanced").addEventListener("toggle", () => scenarioFormAnalysisScope.invalidate());
document.getElementById("scenario-editor-dialog").addEventListener("input", (event) => {
  if (event.target.id === "scenario-editor-source") {
    scenarioEditorJsonDirty = true;
    markScenarioEditorDirty(true);
    return;
  }
  if (event.target.closest("[data-scenario-step]") || ["scenario-editor-id", "scenario-editor-name", "scenario-editor-folder", "scenario-editor-description"].includes(event.target.id)) {
    markScenarioEditorDirty();
  }
});
document.getElementById("scenario-editor-dialog").addEventListener("change", (event) => {
  if (event.target.id === "scenario-editor-parallel") {
    updateScenarioParallelFields();
    markScenarioEditorDirty();
  } else if (event.target.matches("[data-step-key]")) {
    markScenarioEditorDirty();
  }
});
document.getElementById("scenario-editor-advanced").addEventListener("toggle", (event) => {
  const advanced = event.currentTarget;
  if (advanced.open) return;
  (async () => {
  try {
    const scenario = await readScenarioEditorSource();
    const previous = readScenarioEditorForm();
    scenarioEditorDirty = scenarioEditorDirty || scenarioEditorJsonDirty || JSON.stringify(scenario) !== JSON.stringify(previous);
    renderScenarioEditor(scenario);
    if (scenarioEditorDirty) setScenarioEditorStatus("Raw source loaded into the form. Save to create a new draft revision.");
    updateScenarioEditorActions();
  } catch (error) {
    advanced.open = true;
    setScenarioEditorStatus(error.sourceDiagnostic ? "" : error.message || "Scenario source must be valid.", "error");
  }
  })();
});
scenarioEditorController.bind("analyze", { button: document.getElementById("scenario-source-analyze") });
document.getElementById("scenario-editor-mode-toggle").addEventListener("click", async () => {
  const advanced = document.getElementById("scenario-editor-advanced");
  advanced.open = !advanced.open;
});
document.getElementById("scenario-editor-dialog").addEventListener("click", (event) => {
  if (event.target.closest("#scenario-add-step")) {
    addScenarioStep();
    return;
  }
  if (event.target.closest("#scenario-add-cleanup")) {
    addScenarioStep(true);
    return;
  }
  const removeButton = event.target.closest("[data-remove-step]");
  if (!removeButton) return;
  const row = removeButton.closest("[data-scenario-step]");
  const container = row.parentElement;
  const isCleanup = container.id === "scenario-editor-cleanup";
  const scenario = readScenarioEditorForm();
  const steps = isCleanup ? scenario.cleanup : scenario.steps;
  steps.splice([...container.querySelectorAll("[data-scenario-step]")].indexOf(row), 1);
  renderScenarioStepGroup(isCleanup ? "scenario-editor-cleanup" : "scenario-editor-steps", steps);
  markScenarioEditorDirty();
});
scenarioEditorController.bind("save", {
  button: document.getElementById("scenario-editor-save"),
  shortcutRoot: document.getElementById("scenario-editor-dialog"),
  onError: error => setScenarioEditorStatus(error.sourceDiagnostic ? "" : error.message || "Could not save scenario.", "error"),
});
document.getElementById("scenario-editor-publish").addEventListener("click", async () => {
  try { await publishScenarioRevision(); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not publish scenario.", "error"); }
});
document.getElementById("scenario-editor-discard").addEventListener("click", () => {
  if (!scenarioEditorDirty || !scenarioEditorPage.confirmDiscard(true, "Discard unsaved scenario changes?")) return;
  scenarioEditorDirty = false;
  scenarioEditorViewing = 0;
  scenarioEditorSummary = "";
  renderScenarioEditor(scenarioEditorSaved || newScenarioTemplate());
  setScenarioEditorStatus("");
  updateScenarioEditorActions();
  refreshScenarioRevisions().catch(() => {});
});
document.getElementById("scenario-editor-close").addEventListener("click", closeScenarioEditorPage);
document.getElementById("scenario-editor-form").addEventListener("submit", (event) => {
  event.preventDefault();
  closeScenarioEditorPage();
});
document.getElementById("scenario-editor-form").noValidate = true;
document.getElementById("scenario-editor-delete").addEventListener("click", async () => {
  if (!scenarioEditorArtifact) return;
  try { await archiveScenario(scenarioEditorArtifact.id, scenarioEditorOrganization, scenarioEditorArtifact.revision); }
  catch (error) { setScenarioEditorStatus(error.message || "Could not delete scenario.", "error"); }
});
document.getElementById("scenario-list").addEventListener("click", async (event) => {
  const editButton = event.target.closest("[data-edit-scenario]");
  if (editButton) {
    try { await scenarioEditorController.perform("load", editButton.dataset.editScenario); }
    catch (error) {
      const message = document.getElementById("scenario-error");
      message.textContent = error.message || "Could not open scenario";
      message.hidden = false;
    }
    return;
  }
  const deleteButton = event.target.closest("[data-delete-scenario]");
  if (deleteButton) {
    deleteButton.disabled = true;
    try {
      await archiveScenario(deleteButton.dataset.deleteScenario, deleteButton.dataset.deleteOrganization, deleteButton.dataset.deleteRevision);
    } catch (error) {
      const message = document.getElementById("scenario-error");
      message.textContent = error.message || "Could not delete scenario";
      message.hidden = false;
    } finally {
      await refreshScenariosNow();
    }
    return;
  }
});
document.getElementById("scenario-cancel").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  try {
    await postScenarioAction("/api/simulator/scenarios/cancel");
  } catch (error) {
    const message = document.getElementById("scenario-error");
    message.textContent = error.message || "Could not cancel scenario";
    message.hidden = false;
  }
});
document.getElementById("scenario-reset").addEventListener("click", () => { void beginScenarioReset(); });
document.getElementById("scenario-reset-confirm").addEventListener("click", () => { void confirmScenarioReset(); });
document.getElementById("scenario-reset-check-status").addEventListener("click", () => {
  void checkScenarioResetOperation();
});
document.getElementById("scenario-reset-close").addEventListener("click", closeScenarioResetDialog);
document.getElementById("scenario-reset-close-icon").addEventListener("click", closeScenarioResetDialog);
document.getElementById("scenario-reset-dialog").addEventListener("close", () => {
  clearScenarioResetExpiryTimer();
  document.getElementById("scenario-reset").focus();
});
window.addEventListener("operator-session-ready", event => {
  const identity = event.detail?.identity;
  if (!identity?.issuer || !identity?.subject) {
    scenarioResetIdentity = null;
    invalidateScenarioReset("A verified named Simulator operator session is required for reset.");
    return;
  }
  document.dispatchEvent(new Event("operator-login-checked"));
  if (scenarioResetIdentity && !scenarioResetActorMatches(identity, {
    issuer: scenarioResetIdentity.issuer, subject: scenarioResetIdentity.subject,
  })) invalidateScenarioReset("The signed-in operator changed; the previous reset intent was discarded.");
  scenarioResetIdentity = identity;
  void refreshScenariosNow();
  if (scenarioResetFlow &&
      !scenarioResetHasPermission(identity, scenarioResetFlow.organization)) {
    invalidateScenarioReset("The signed-in identity is not authorized for this reset target.");
  } else {
    renderScenarioResetControl();
    if (activeScenarioOrganization) void refreshScenarioResetStatus(activeScenarioOrganization);
  }
});
window.addEventListener("operator-session-cleared", () => {
  scenarioResetIdentity = null;
  invalidateScenarioReset("Sign in again to check reset availability.");
});
void initializeScenarioPlanWorkflow();