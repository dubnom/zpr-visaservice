import { test, expect } from "@playwright/test";
import { lstatSync } from "node:fs";
import { resolve } from "node:path";

test.describe.configure({ mode: "serial", timeout: 60000 });
test.use({ trace: "off", screenshot: "off", video: "off" });

function validatedStorageStatePath() {
  const configuredPath = process.env.ZPR_LIVE_STORAGE_STATE;
  if (!configuredPath) {
    throw new Error("Live acceptance blocker: ZPR_LIVE_STORAGE_STATE is not set; no network requests were made.");
  }
  const localPath = resolve(configuredPath);
  let file;
  try {
    file = lstatSync(localPath);
  } catch {
    throw new Error("Live acceptance blocker: ZPR_LIVE_STORAGE_STATE must name an existing local regular file.");
  }
  if (!file.isFile()) {
    throw new Error("Live acceptance blocker: ZPR_LIVE_STORAGE_STATE must name an existing local regular file.");
  }
  if (process.platform !== "win32" && (file.mode & 0o777) !== 0o600) {
    throw new Error("Live acceptance blocker: ZPR_LIVE_STORAGE_STATE must have POSIX mode 0600.");
  }
  return localPath;
}

const storageStatePath = process.env.ZPR_LIVE_STORAGE_STATE
  ? validatedStorageStatePath()
  : undefined;
if (storageStatePath) test.use({ storageState: storageStatePath });

function liveOrigin(variableName) {
  const raw = process.env[variableName];
  if (!raw) {
    throw new Error(`Live acceptance blocker: ${variableName} is not set; no network requests were made.`);
  }
  let url;
  try {
    url = new URL(raw);
  } catch {
    throw new Error(`Live acceptance blocker: ${variableName} is not a valid absolute URL; no network requests were made.`);
  }
  if (url.protocol !== "https:" || url.pathname !== "/" ||
      url.username || url.password || url.search || url.hash) {
    throw new Error(`Live acceptance blocker: ${variableName} must be an HTTPS origin without embedded credentials, path, query, or fragment.`);
  }
  if (variableName === "ZPR_LIVE_SIMULATOR_URL" && url.origin !== "https://localhost:8890") {
    throw new Error("Live acceptance blocker: ZPR_LIVE_SIMULATOR_URL must be https://localhost:8890 for the loopback Simulator runtime.");
  }
  return url.origin;
}

async function getJSON(page, base, path, evidence) {
  let result;
  try {
    if (new URL(page.url()).origin !== base) throw new Error("Unexpected browser origin");
    result = await page.evaluate(async requestPath => {
      const response = await fetch(requestPath, {
        method: "GET",
        cache: "no-store",
        credentials: "same-origin",
        headers: { Accept: "application/json" },
        redirect: "manual",
      });
      if (response.type === "opaqueredirect") {
        return { status: 0, body: null };
      }
      return { status: response.status, body: await response.json() };
    }, path);
  } catch {
    throw new Error(`Live acceptance blocker: ${evidence.service} ${path} could not be reached or did not return valid JSON.`);
  }
  evidence.http.push({ method: "GET", service: evidence.service, path, status: result.status });
  if (result.status !== 200) {
    throw new Error(`Live acceptance blocker: ${evidence.service} ${path} returned HTTP ${result.status}; stopped without login or mutation.`);
  }
  return result.body;
}

function validateOperatorSession(session, service, evidence) {
  const identity = session?.identity;
  if (!identity || typeof identity.issuer !== "string" || !identity.issuer ||
      typeof identity.subject !== "string" || !identity.subject ||
      !Array.isArray(identity.organizations) ||
      !identity.organizations.every(value => typeof value === "string" && value) ||
      !Array.isArray(identity.permissions) ||
      !identity.permissions.every(value => typeof value === "string" && value) ||
      typeof session.csrf !== "string" || !session.csrf) {
    throw new Error(`Live acceptance blocker: ${service} operator session is absent or incomplete; no credentials were supplied.`);
  }
  evidence[service].session = "authenticated";
  evidence[service].oidcIssuerPresent = true;
  evidence[service].organizationCount = identity.organizations.length;
  evidence[service].permissionCount = identity.permissions.length;
  return identity;
}

function organizationAllowed(identity, organizationID) {
  return identity.organizations.includes("*") || identity.organizations.includes(organizationID);
}

test("live Control Room OIDC and Simulator catalog/preflight are available without mutations", async ({ page }) => {
  if (!storageStatePath) validatedStorageStatePath();
  const controlRoom = liveOrigin("ZPR_LIVE_CONTROL_ROOM_URL");
  const simulator = liveOrigin("ZPR_LIVE_SIMULATOR_URL");
  const evidence = {
    http: [],
    controlRoom: { oidc: "unchecked", session: "unchecked" },
    simulator: { oidc: "unchecked", session: "unchecked" },
    browser: { pageErrors: 0, consoleErrors: 0, unexpectedPosts: [] },
  };
  page.on("pageerror", () => { evidence.browser.pageErrors++; });
  page.on("console", message => {
    if (message.type() === "error") evidence.browser.consoleErrors++;
  });
  page.on("request", request => {
    if (request.method() !== "POST") return;
    const url = new URL(request.url());
    if (url.origin !== simulator || url.pathname !== "/api/simulator/scenario-plans/preflight") {
      evidence.browser.unexpectedPosts.push({
        service: url.origin === controlRoom ? "control-room" : "simulator",
        path: url.pathname,
      });
    }
  });

  const controlResponse = await page.goto(`${controlRoom}/budget-authority.html`, { waitUntil: "domcontentloaded" });
  evidence.http.push({
    method: "GET", service: "control-room", path: "/budget-authority.html",
    status: controlResponse?.status() ?? 0,
  });
  if (!controlResponse?.ok()) {
    throw new Error(`Live acceptance blocker: Control Room budget page returned HTTP ${controlResponse?.status() ?? 0}.`);
  }
  const controlConfig = await getJSON(page, controlRoom, "/auth/operator/config", {
    service: "control-room", http: evidence.http, controlRoom: evidence.controlRoom,
  });
  evidence.controlRoom.oidc = controlConfig?.enabled === true ? "enabled" : "disabled";
  if (controlConfig?.enabled !== true) {
    throw new Error("Live acceptance blocker: Control Room OIDC/login is not enabled; stopped before authenticated checks.");
  }
  const controlIdentity = validateOperatorSession(
    await getJSON(page, controlRoom, "/auth/operator/session", {
      service: "control-room", http: evidence.http, controlRoom: evidence.controlRoom,
    }),
    "controlRoom",
    evidence,
  );
  await expect(page.locator("#budget-workspace")).toBeAttached();
  await expect(page.locator("#budget-session-status")).toContainText("Signed in");
  await expect(page.locator("#budget-workspace")).toBeEnabled();
  evidence.controlRoom.pageLoaded = true;
  evidence.controlRoom.budgetApprovePermission =
    controlIdentity.permissions.includes("budget.approve");
  evidence.controlRoom.budgetPageReady = true;

  const simulatorResponse = await page.goto(`${simulator}/scenarios.html#simulate`, {
    waitUntil: "domcontentloaded",
  });
  evidence.http.push({
    method: "GET", service: "simulator", path: "/scenarios.html",
    status: simulatorResponse?.status() ?? 0,
  });
  if (!simulatorResponse?.ok()) {
    throw new Error(`Live acceptance blocker: Simulator scenarios page returned HTTP ${simulatorResponse?.status() ?? 0}.`);
  }
  const simulatorConfig = await getJSON(page, simulator, "/auth/operator/config", {
    service: "simulator", http: evidence.http, simulator: evidence.simulator,
  });
  evidence.simulator.oidc = simulatorConfig?.enabled === true ? "enabled" : "disabled";
  if (simulatorConfig?.enabled !== true) {
    throw new Error("Live acceptance blocker: Simulator OIDC/login is not enabled; stopped before authenticated checks.");
  }
  const simulatorIdentity = validateOperatorSession(
    await getJSON(page, simulator, "/auth/operator/session", {
      service: "simulator", http: evidence.http, simulator: evidence.simulator,
    }),
    "simulator",
    evidence,
  );
  if (!simulatorIdentity.permissions.includes("scenario.read")) {
    throw new Error("Live acceptance blocker: Simulator session lacks scenario.read; stopped before catalog access.");
  }
  const simulatorSignedIn = await page.locator("#operator-login-status").evaluate(element =>
    /^User(?:: .+)?$/.test(element.textContent.trim()),
  );
  if (!simulatorSignedIn) {
    throw new Error("Live acceptance blocker: Simulator page did not confirm an authenticated operator session.");
  }

  const organizationResponse = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/simulator/organizations", {
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    return { status: response.status, body: await response.json() };
  });
  evidence.http.push({
    method: "GET", service: "simulator", path: "/api/simulator/organizations",
    status: organizationResponse.status,
  });
  if (organizationResponse.status !== 200 ||
      !Array.isArray(organizationResponse.body?.organizations) ||
      organizationResponse.body.organizations.some(item =>
        !item || typeof item.id !== "string" || !item.id || typeof item.name !== "string")) {
    throw new Error(`Live acceptance blocker: Simulator organization catalog returned HTTP ${organizationResponse.status} or an invalid catalog.`);
  }
  const organizations = organizationResponse.body.organizations;
  const permittedOrganizations = organizations.filter(item =>
    organizationAllowed(simulatorIdentity, item.id) &&
    organizationAllowed(controlIdentity, item.id));
  if (!permittedOrganizations.length) {
    evidence.simulator.organizationCount = organizations.length;
    console.log(`[live-acceptance-evidence] ${JSON.stringify(evidence)}`);
    throw new Error("Live acceptance blocker: no catalog organization is in both named-user scopes.");
  }
  const activeID = organizationResponse.body.active_id;
  const selectedOrganization = permittedOrganizations.find(item => item.id === activeID) ||
    permittedOrganizations[0];
  evidence.simulator.organizationCount = organizations.length;
  evidence.simulator.selectedOrganizationInBothScopes = true;
  evidence.controlRoom.identityOrganizationsIncludeSimulatorScope =
    organizationAllowed(controlIdentity, selectedOrganization.id);

  const organizationSelect = page.locator("#scenario-plan-organization");
  await expect(organizationSelect).toBeEnabled();
  await organizationSelect.selectOption(selectedOrganization.id);
  const scenarioSelect = page.locator("#scenario-plan-scenario");
  await expect(scenarioSelect).toBeEnabled();

  const catalog = await page.evaluate(async organizationID => {
    const response = await window.zprOperatorFetch(
      `/api/simulator/scenarios?organization_id=${encodeURIComponent(organizationID)}`,
      { cache: "no-store", headers: { Accept: "application/json" } },
    );
    return { status: response.status, body: await response.json() };
  }, selectedOrganization.id);
  evidence.http.push({
    method: "GET", service: "simulator", path: "/api/simulator/scenarios",
    status: catalog.status,
  });
  if (catalog.status !== 200 || catalog.body?.organization?.id !== selectedOrganization.id ||
      !Array.isArray(catalog.body.scenarios) ||
      catalog.body.scenarios.some(item =>
        !item || typeof item.id !== "string" || item.organization_id !== selectedOrganization.id)) {
    throw new Error(`Live acceptance blocker: Simulator scenario catalog returned HTTP ${catalog.status} or mismatched organization data.`);
  }
  const publishedScenario = catalog.body.scenarios.find(item =>
    Number.isInteger(item.published_revision) && item.published_revision > 0);
  evidence.simulator.scenarioCount = catalog.body.scenarios.length;
  evidence.simulator.publishedScenarioAvailable = Boolean(publishedScenario);
  if (!publishedScenario) {
    console.log(`[live-acceptance-evidence] ${JSON.stringify(evidence)}`);
    throw new Error("Live acceptance blocker: selected organization has no published scenario to preview.");
  }
  await expect(scenarioSelect.locator(`option[value=${JSON.stringify(publishedScenario.id)}]`)).toHaveCount(1);
  await scenarioSelect.selectOption(publishedScenario.id);

  const preflightResponsePromise = page.waitForResponse(response =>
    new URL(response.url()).origin === simulator &&
    new URL(response.url()).pathname === "/api/simulator/scenario-plans/preflight" &&
    response.request().method() === "POST");
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  const preflightResponse = await preflightResponsePromise;
  const preflightRequest = preflightResponse.request();
  expect(preflightRequest.postDataJSON()).toEqual({
    organization_id: selectedOrganization.id,
    scenario_id: publishedScenario.id,
  });
  expect(preflightRequest.headers()["x-zpr-csrf"]).toBeTruthy();
  evidence.http.push({
    method: "POST", service: "simulator", path: "/api/simulator/scenario-plans/preflight",
    status: preflightResponse.status(),
  });
  if (![200, 503].includes(preflightResponse.status())) {
    console.log(`[live-acceptance-evidence] ${JSON.stringify(evidence)}`);
    throw new Error(`Live acceptance blocker: read-only Simulator preflight returned HTTP ${preflightResponse.status()}.`);
  }
  const preflight = await preflightResponse.json();
  if (preflightResponse.status() === 200) {
    if (preflight.organization_id !== selectedOrganization.id ||
        preflight.scenario_id !== publishedScenario.id ||
        !/^sha256:[0-9a-f]{64}$/.test(preflight.plan_digest || "") ||
        !/^sha256:[0-9a-f]{64}$/.test(preflight.authorization_scope_digest || "")) {
      throw new Error("Live acceptance blocker: successful preflight response failed scope/digest validation.");
    }
    await expect(page.locator("#scenario-plan-preview")).toBeVisible();
    evidence.simulator.preflight = "plan-response";
  } else {
    if (preflight.organization_id !== selectedOrganization.id ||
        preflight.scenario_id !== publishedScenario.id ||
        !/^sha256:[0-9a-f]{64}$/.test(preflight.authorization_scope_digest || "") ||
        !Array.isArray(preflight.blockers) || !preflight.blockers.length ||
        preflight.blockers.some(blocker => typeof blocker !== "string" || !blocker)) {
      throw new Error("Live acceptance blocker: unavailable-budget preflight response failed scope/digest validation.");
    }
    await expect(page.locator("#scenario-plan-budget-handoff")).toBeVisible();
    evidence.simulator.preflight = "validated-unavailable-evidence-response";
  }

  const unexpectedMutations = evidence.browser.unexpectedPosts;
  if (unexpectedMutations.length) {
    throw new Error("Live acceptance blocker: an unexpected POST occurred; stopped without continuing.");
  }
  evidence.browser.result = "read-only checks complete";
  console.log(`[live-acceptance-evidence] ${JSON.stringify(evidence)}`);
  expect(evidence.controlRoom.budgetApprovePermission,
    "Control Room identity needs budget.approve for an operator grant workflow").toBe(true);
  expect(evidence.controlRoom.identityOrganizationsIncludeSimulatorScope,
    "Control Room identity needs shared organization scope for the selected Simulator organization").toBe(true);
  expect(evidence.browser.pageErrors).toBe(0);
  expect(evidence.browser.consoleErrors).toBe(0);
  expect(evidence.browser.unexpectedPosts).toEqual([]);
});
