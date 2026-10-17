import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const staticRoot = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const origin = "https://application.example.test";
const review = {
  id: "review-one",
  digest: "a".repeat(64),
  version: 2,
  state: "approved",
  proposal: {
    organization: "lab-one",
    kind: "policy",
    target: "policy-one",
    revision: 3,
    base_revision: 2,
    before: "old policy",
    after: "new policy",
    validation: "Validated saved revision",
    impact: "One explicit policy target",
    reason: "Restrict access",
    author: { issuer: "https://idp.example", subject: "author", name: "Author" },
    submitted_at: "2026-10-17T12:00:00Z",
  },
  events: [
    { action: "submitted", actor: { issuer: "https://idp.example", subject: "author" }, reason: "Restrict access", at: "2026-10-17T12:00:00Z" },
    { action: "approved", actor: { issuer: "https://idp.example", subject: "reviewer" }, reason: "Reviewed exact candidate", at: "2026-10-17T12:01:00Z" },
  ],
};

async function openReviewQueue(page, { applicationEnabled, permissions, onApply }) {
  await page.route(origin + "/**", async route => {
    const url = new URL(route.request().url());
    if (url.pathname === "/auth/operator/session") {
      return route.fulfill({ json: {
        identity: { issuer: "https://idp.example", subject: "reviewer", display_name: "Reviewer",
          organizations: ["lab-one"], permissions },
        csrf: "application-csrf",
      } });
    }
    if (url.pathname === "/api/change-review/capabilities") {
      return route.fulfill({ json: { allow_self_approval: false, application_enabled: applicationEnabled } });
    }
    if (url.pathname === "/api/change-review/requests") {
      return route.fulfill({ json: { requests: [review], next_offset: -1 } });
    }
    if (url.pathname === "/api/change-review/requests/review-one/apply" && route.request().method() === "POST" && onApply) {
      return onApply(route);
    }
    const name = url.pathname.slice(1);
    if (["change-review.html", "change-review.js", "change-review.css"].includes(name)) {
      return route.fulfill({
        contentType: name.endsWith(".js") ? "application/javascript" : name.endsWith(".css") ? "text/css" : "text/html",
        body: readFileSync(staticRoot + name, "utf8"),
      });
    }
    return route.fulfill({ status: 404, json: { error: "Unexpected synthetic request." } });
  });
  await page.goto(origin + "/change-review.html");
  await expect(page.locator("#workspace")).toBeEnabled();
  await page.getByRole("button", { name: "Load queue", exact: true }).click();
  await page.getByRole("button", { name: "Review", exact: true }).click();
}

test("policy Apply stays unavailable without configured application or scoped permission", async ({ page }) => {
  await openReviewQueue(page, {
    applicationEnabled: false,
    permissions: ["change.read", "change.review", "policy.read", "policy.apply"],
  });
  await expect(page.locator("#apply-policy")).toBeDisabled();
  await expect(page.locator("#apply-policy")).toHaveText("Apply unavailable");
  await expect(page.locator("#application-boundary")).toContainText("Production application is not configured");
});

test("policy Apply requires explicit organization-scoped policy.apply permission", async ({ page }) => {
  await openReviewQueue(page, {
    applicationEnabled: true,
    permissions: ["change.read", "change.review", "policy.read"],
  });
  await expect(page.locator("#apply-policy")).toBeDisabled();
  await expect(page.locator("#application-note")).toContainText("policy.apply");
});

test("applying an approved policy requires confirmation and reports verified operation", async ({ page }) => {
  let applicationRequest;
  await openReviewQueue(page, {
    applicationEnabled: true,
    permissions: ["change.read", "change.review", "policy.read", "policy.apply"],
    onApply: async route => {
      applicationRequest = route.request();
      await route.fulfill({ json: {
        operation_id: "operation-one",
        status: "succeeded",
        version: 1,
        result: { status: "succeeded", receipt: { status: "succeeded" }, readback: { target_id: "policy-one" } },
      } });
    },
  });
  const apply = page.locator("#apply-policy");
  await expect(apply).toBeEnabled();
  await expect(apply).toHaveText("Apply approved policy");
  await expect(page.locator("#application-boundary")).toContainText("explicit confirmation");

  let confirmation;
  page.once("dialog", async dialog => {
    confirmation = dialog.message();
    await dialog.accept();
  });
  await apply.click();
  expect(confirmation).toContain("lab-one policy revision 3");
  await expect.poll(() => applicationRequest).toBeTruthy();
  expect(applicationRequest.method()).toBe("POST");
  expect(applicationRequest.postDataJSON()).toEqual({
    organization: "lab-one",
    digest: review.digest,
    expected_version: review.version,
    reason: "Operator explicitly confirmed application of the approved policy.",
  });
  expect(applicationRequest.headers()["x-zpr-csrf"]).toBe("application-csrf");
  await expect(page.locator("#status")).toContainText("succeeded with verified receipt and readback");
  await expect(page.locator("#status")).toContainText("operation-one");
  await expect(apply).toBeDisabled();
});
