import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const origin = "https://control-room.example.test";
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const scope = {
  organization_id: "alpha",
  scenario_id: "release-scope-7",
  plan_digest: "authorization-scope-digest-v1",
};
const csrf = "synthetic-control-room-csrf";

function evidence(overrides = {}) {
  const now = new Date();
  return {
    approval_id: "budget-approval-1",
    organization_id: scope.organization_id,
    scenario_id: scope.scenario_id,
    plan_digest: scope.plan_digest,
    decision: "approved",
    approved_by: "https://issuer.example.test|operator-1",
    version: "1",
    checked_at: now.toISOString(),
    valid_from: now.toISOString(),
    expires_at: new Date(now.getTime() + 30 * 60 * 1000).toISOString(),
    revoked: false,
    ...overrides,
  };
}

async function setup(page, context, options = {}) {
  const state = { mutations: [], simulatorRequests: [] };
  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin) return route.abort();
    if (url.pathname === "/auth/operator/session") {
      if (options.sessionStatus && options.sessionStatus !== 200) {
        return route.fulfill({
          status: options.sessionStatus,
          contentType: "application/json",
          body: JSON.stringify({ error: "No active operator session." }),
        });
      }
      return route.fulfill({
        json: {
          identity: {
            issuer: "https://issuer.example.test",
            subject: "operator-1",
            display_name: "Budget Operator",
            organizations: options.organizations || ["alpha"],
            permissions: options.permissions ?? ["budget.approve"],
          },
          csrf,
        },
      });
    }
    if (url.pathname.startsWith("/api/simulator/")) {
      state.simulatorRequests.push({ method: request.method(), path: url.pathname });
      return route.fulfill({ status: 503, json: { error: "Simulator is unavailable in this fixture." } });
    }
    if (url.pathname === "/api/budget-authority/approvals" ||
        /^\/api\/budget-authority\/approvals\/[^/]+\/revoke$/.test(url.pathname)) {
      const body = request.postDataJSON();
      state.mutations.push({
        method: request.method(),
        path: url.pathname,
        body,
        csrf: request.headers()["x-zpr-csrf"],
      });
      if (url.pathname === "/api/budget-authority/approvals") {
        if (options.approveStatus && options.approveStatus !== 201) {
          return route.fulfill({
            status: options.approveStatus,
            contentType: "application/json",
            body: JSON.stringify({ error: options.approveError || "Budget approval refused." }),
          });
        }
        return route.fulfill({ status: 201, json: evidence({
          organization_id: body.scope.organization_id,
          scenario_id: body.scope.scenario_id,
          plan_digest: body.scope.plan_digest,
          expires_at: body.expires_at,
        }) });
      }
      if (options.revokeStatus && options.revokeStatus !== 200) {
        return route.fulfill({
          status: options.revokeStatus,
          contentType: "application/json",
          body: JSON.stringify({ error: options.revokeError || "Budget revocation refused." }),
        });
      }
      return route.fulfill({ status: 200, json: evidence({
        approval_id: decodeURIComponent(url.pathname.split("/").at(-2)),
        organization_id: body.organization_id,
        revoked: true,
      }) });
    }
    if (url.pathname.endsWith(".js") || url.pathname.endsWith(".css") ||
        url.pathname.endsWith(".html")) {
      const file = resolve(assets, `.${url.pathname}`);
      if (!file.startsWith(assets)) return route.fulfill({ status: 403 });
      try {
        const body = await readFile(file, "utf8");
        const contentType = url.pathname.endsWith(".js") ? "application/javascript"
          : url.pathname.endsWith(".css") ? "text/css" : "text/html";
        return route.fulfill({ contentType, body });
      } catch {
        return route.fulfill({ status: 404, body: "Fixture asset not found." });
      }
    }
    if (url.pathname.startsWith("/api/")) {
      return route.fulfill({ status: 404, json: { error: "Unexpected API request." } });
    }
    return route.fulfill({ status: 404, body: "Fixture page not found." });
  });
  await page.goto(`${origin}/budget-authority.html`);
  await expect(page.locator("#budget-workspace")).toBeEnabled();
  return state;
}

function localFutureDateTime(minutes = 30) {
  const value = new Date(Date.now() + minutes * 60 * 1000);
  const pad = number => String(number).padStart(2, "0");
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}` +
    `T${pad(value.getHours())}:${pad(value.getMinutes())}`;
}

async function fillApproval(page) {
  await page.locator("#budget-organization").fill(scope.organization_id);
  await page.locator("#budget-scenario").fill(scope.scenario_id);
  await page.locator("#budget-authorization-scope-digest").fill(scope.plan_digest);
  await page.locator("#budget-expires-at").fill(localFutureDateTime());
}

async function confirmDialog(page) {
  const dialog = page.getByRole("dialog", { name: "Confirm budget approval" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText(scope.organization_id);
  await expect(dialog).toContainText(scope.scenario_id);
  await expect(dialog).toContainText(scope.plan_digest);
  await dialog.locator("#budget-explicit-confirm").check();
  await dialog.locator("#budget-confirm-submit").click();
}

test("Control Room budget authority works without Simulator and explicitly approves then revokes exact scope", async ({ page, context }) => {
  const state = await setup(page, context);
  expect(await page.locator("#budget-organization").inputValue()).toBe("");
  expect(await page.locator("#budget-scenario").inputValue()).toBe("");
  expect(await page.locator("#budget-authorization-scope-digest").inputValue()).toBe("");
  expect(await page.locator("#budget-expires-at").inputValue()).toBe("");
  await expect(page.locator("#budget-permission-status")).toContainText(
    "organization explicitly listed in your operator session",
  );

  await fillApproval(page);
  await page.locator("#budget-approve").click();
  await confirmDialog(page);
  await expect(page.locator("#budget-evidence")).toBeVisible();
  await expect(page.locator("#budget-evidence-approval-id")).toHaveText("budget-approval-1");
  await expect(page.locator("#budget-evidence-decision")).toHaveText("approved");
  await expect(page.locator("#budget-evidence-revoked")).toHaveText("No");
  expect(state.mutations[0]).toEqual({
    method: "POST",
    path: "/api/budget-authority/approvals",
    body: {
      scope,
      decision: "approved",
      expires_at: expect.any(String),
    },
    csrf,
  });
  expect(Date.parse(state.mutations[0].body.expires_at)).toBeGreaterThan(Date.now());
  expect(Date.parse(state.mutations[0].body.expires_at)).toBeLessThanOrEqual(Date.now() + 60 * 60 * 1000);

  await page.locator("#budget-revoke").click();
  const revokeDialog = page.getByRole("dialog", { name: "Confirm budget approval revocation" });
  await expect(revokeDialog).toBeVisible();
  await expect(revokeDialog).toContainText("budget-approval-1");
  await revokeDialog.locator("#budget-explicit-confirm").check();
  await revokeDialog.locator("#budget-confirm-submit").click();
  await expect(page.locator("#budget-evidence-revoked")).toHaveText("Yes");
  expect(state.mutations[1]).toEqual({
    method: "POST",
    path: "/api/budget-authority/approvals/budget-approval-1/revoke",
    body: { organization_id: scope.organization_id },
    csrf,
  });
  expect(state.simulatorRequests).toEqual([]);
});

test("operator without budget.approve cannot submit grant or revocation", async ({ page, context }) => {
  const state = await setup(page, context, { permissions: ["scenario.run"] });
  await expect(page.locator("#budget-permission-status")).toContainText("lacks budget.approve");
  await page.locator("#budget-organization").fill(scope.organization_id);
  await page.locator("#budget-scenario").fill(scope.scenario_id);
  await page.locator("#budget-authorization-scope-digest").fill(scope.plan_digest);
  await page.locator("#budget-expires-at").fill(localFutureDateTime());
  await page.locator("#budget-revoke-organization").fill(scope.organization_id);
  await page.locator("#budget-revoke-approval-id").fill("known-approval-id");
  await expect(page.locator("#budget-approve")).toBeDisabled();
  await expect(page.locator("#budget-revoke")).toBeDisabled();
  expect(state.mutations).toEqual([]);
  expect(state.simulatorRequests).toEqual([]);
});

test("organization outside the named-user scope cannot be submitted", async ({ page, context }) => {
  const state = await setup(page, context, { organizations: ["other-org"] });
  await page.locator("#budget-organization").fill(scope.organization_id);
  await page.locator("#budget-scenario").fill(scope.scenario_id);
  await page.locator("#budget-authorization-scope-digest").fill(scope.plan_digest);
  await page.locator("#budget-expires-at").fill(localFutureDateTime());
  await expect(page.locator("#budget-approve")).toBeDisabled();
  await page.locator("#budget-revoke-organization").fill(scope.organization_id);
  await page.locator("#budget-revoke-approval-id").fill("known-approval-id");
  await expect(page.locator("#budget-revoke")).toBeDisabled();
  expect(state.mutations).toEqual([]);
  expect(state.simulatorRequests).toEqual([]);
});

test("server refusal is visible and never presented as a successful approval", async ({ page, context }) => {
  const state = await setup(page, context, {
    approveStatus: 403,
    approveError: "budget.approve permission denied",
  });
  await fillApproval(page);
  await page.locator("#budget-approve").click();
  await confirmDialog(page);

  await expect(page.locator("#budget-error")).toBeVisible();
  await expect(page.locator("#budget-error")).toContainText("Budget approval was refused (HTTP 403)");
  await expect(page.locator("#budget-error")).toContainText("budget.approve permission denied");
  await expect(page.locator("#budget-evidence")).toBeHidden();
  await expect(page.locator("#budget-session-status")).toHaveText("No successful budget authority response was received.");
  expect(state.mutations).toHaveLength(1);
  expect(state.simulatorRequests).toEqual([]);
});

test("missing OIDC session keeps mutations disabled and offers sign-in", async ({ page, context }) => {
  const state = await setup(page, context, { sessionStatus: 401 });
  await expect(page.locator("#budget-login")).toBeVisible();
  await expect(page.locator("#budget-workspace")).toHaveAttribute("disabled", "");
  expect(state.mutations).toEqual([]);
  expect(state.simulatorRequests).toEqual([]);
});
