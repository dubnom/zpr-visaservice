import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

test("bounded evaluation computes budget scope server-side and permits its machine-free approved run", async ({ page, context }) => {
  const origin = "https://simulator.example.test";
  const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
  const organization = { id: `evaluation-${"a".repeat(24)}`, name: "ZPR Evaluation" };
  const scenario = {
    id: "evaluation-endpoint-access", organization_id: organization.id, name: "Verify endpoint access policy",
    description: "Bounded evaluation.", current_revision: 1, published_revision: 1,
    steps: [{ action: "verify_evaluation_peer_access", timeout_seconds: 120 }],
  };
  const scopeDigest = `sha256:${"b".repeat(64)}`;
  const planDigest = `sha256:${"c".repeat(64)}`;
  let budgetApproved = false;
  const requests = [];
  let run = { state: "idle", steps: [] };
  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin) return route.abort();
    if (url.pathname === "/auth/operator/config") return route.fulfill({ json: { enabled: false } });
    if (url.pathname === "/api/simulator/organizations") {
      return route.fulfill({ json: { active_id: organization.id, organizations: [organization] } });
    }
    if (url.pathname.endsWith("/scenarios")) {
      return route.fulfill({ json: {
        active_organization_id: organization.id, organization, scenarios: [scenario], run,
      } });
    }
    if (url.pathname.endsWith("/scenario-plans/preflight")) {
      return route.fulfill(budgetApproved ? { json: {
        status: "plan-only", organization_id: organization.id, scenario_id: scenario.id,
        manifest_name: "Evaluation endpoints", machine_ids: [], topology: { nodes: [] }, workloads: [],
        budget_inputs: [{ phase: "step", step: 1, action: "verify_evaluation_peer_access", fanout_count: 3, service_count: 4 }],
        budget_status: "approved", topology_status: "resolved", policy_status: "resolved",
        northstar_assets_status: "not-required", package_contract_status: "evaluation-endpoints-v1",
        blockers: [], plan_digest: planDigest, authorization_scope_digest: scopeDigest,
      } } : { status: 503, json: {
        organization_id: organization.id, scenario_id: scenario.id, authorization_scope_digest: scopeDigest,
        budget_approval_available: true, blockers: ["current approved budget evidence is unavailable or invalid"],
      } });
    }
    if (url.pathname.endsWith("/scenario-plans/budget-approval")) {
      requests.push(request.postDataJSON());
      budgetApproved = true;
      return route.fulfill({ status: 201, json: {
        organization_id: organization.id, scenario_id: scenario.id, decision: "approved", plan_digest: scopeDigest,
      } });
    }
    if (url.pathname.endsWith("/scenario-plans/approve")) {
      return route.fulfill({ json: {
        approved: true, approval_id: "one-use", organization_id: organization.id, scenario_id: scenario.id,
        plan_digest: planDigest, authorization_scope_digest: scopeDigest, machine_ids: [],
      } });
    }
    if (url.pathname.endsWith("/run")) {
      run = {
        state: "running", run_id: "run-test", run_namespace: `${organization.id}/${scenario.id}/run-test`,
        organization_id: organization.id, scenario_id: scenario.id, plan_digest: planDigest, steps: [],
      };
      return route.fulfill({ status: 202, json: run });
    }
    if (/\.(js|css|html)$/.test(url.pathname)) {
      const file = resolve(assets, `.${url.pathname}`);
      if (!file.startsWith(assets)) return route.fulfill({ status: 403 });
      try {
        return route.fulfill({ body: await readFile(file, "utf8"),
          contentType: url.pathname.endsWith(".js") ? "application/javascript" :
            url.pathname.endsWith(".css") ? "text/css" : "text/html" });
      } catch {
        return route.fulfill({ status: 404 });
      }
    }
    return route.fulfill({ status: 404, json: { error: "Not configured by fixture" } });
  });
  await page.goto(`${origin}/scenarios.html#simulate`);
  await page.locator("#scenario-plan-scenario").selectOption(scenario.id);
  await page.getByRole("button", { name: "Preview plan", exact: true }).click();
  await expect(page.getByRole("button", { name: "Approve 15-minute evaluation budget" })).toBeVisible();
  await page.getByRole("button", { name: "Approve 15-minute evaluation budget" }).click();
  await expect(page.locator("#scenario-plan-approve")).toBeEnabled();
  expect(requests).toEqual([{ organization_id: organization.id, scenario_id: scenario.id }]);
  await page.locator("#scenario-plan-approve").click();
  await page.locator("#scenario-plan-explicit-confirm").check();
  await page.locator("#scenario-plan-confirm-approval").click();
  await expect(page.locator("#scenario-plan-run-button")).toBeEnabled();
  await page.locator("#scenario-plan-run-button").click();
  await expect(page.locator("#scenario-plan-run-id")).toHaveText("run-test");
});
