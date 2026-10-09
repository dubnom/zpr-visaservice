import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const staticRoot = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const origin = "https://review.example.test";
function fixtureRecord(id = "request-one") {
  return { id, digest: "a".repeat(64), version: 1, state: "submitted",
    proposal: { organization: "great-lakes", kind: "gateway", target: "internet-gateway", revision: 2, base_revision: 1,
      before: "allowed old", after: "allowed new\n<script>not markup</script>", validation: "Validated saved revision",
      impact: "Draft only; no runtime activation.", reason: "Restrict access",
      author: { issuer: "https://idp.example", subject: "author", name: "Author" }, submitted_at: "2026-10-09T18:00:00Z" },
    events: [{ action: "submitted", actor: { issuer: "https://idp.example", subject: "author" }, reason: "Restrict access", at: "2026-10-09T18:00:00Z" }] };
}
async function setup(page, { subject = "reviewer", self = false, mutate } = {}) {
  let record = fixtureRecord();
  const mutations = [];
  let queueFailure = false;
  await page.route(origin + "/**", async route => {
    const url = new URL(route.request().url());
    if (url.pathname === "/auth/operator/session") return route.fulfill({ json: {
      identity: { issuer: "https://idp.example", subject, display_name: subject, organizations: ["great-lakes"],
        permissions: ["change.read", "change.submit", "change.review", "gateway.read", "gateway.edit", "policy.read"] }, csrf: "proof" } });
    if (url.pathname === "/api/change-review/capabilities") return route.fulfill({ json: { allow_self_approval: self, application_enabled: false } });
    if (url.pathname.startsWith("/api/change-review/")) {
      if (route.request().method() === "GET") {
        if (queueFailure) return route.fulfill({ status: 503, json: { error: "Review storage unavailable" } });
        return route.fulfill({ json: { requests: [record], next_offset: -1 } });
      }
      mutations.push(route.request());
      if (mutate) return mutate(route, mutations);
      const action = url.pathname.split("/").pop();
      record = { ...record, version: 2, state: { approve: "approved", reject: "rejected", cancel: "cancelled" }[action] };
      return route.fulfill({ json: record });
    }
    const name = url.pathname.slice(1);
    if (["change-review.html", "change-review.js", "change-review.css"].includes(name)) {
      return route.fulfill({ contentType: name.endsWith(".js") ? "application/javascript" : name.endsWith(".css") ? "text/css" : "text/html",
        body: readFileSync(staticRoot + name, "utf8") });
    }
    return route.fulfill({ status: 404, body: "Not found" });
  });
  await page.goto(origin + "/change-review.html");
  await expect(page.locator("#workspace")).toBeEnabled();
  await page.getByRole("button", { name: "Load queue", exact: true }).click();
  await page.getByRole("button", { name: "Review", exact: true }).click();
  return { mutations, failQueue: () => { queueFailure = true; } };
}

test("review queue shows immutable saved diff, records exact CSRF-bound approval and never applies", async ({ page }) => {
  const { mutations } = await setup(page);
  await expect(page.locator("#diff")).toContainText("-allowed old");
  await expect(page.locator("#diff")).toContainText("+allowed new");
  await expect(page.locator("#after")).toContainText("<script>not markup</script>");
  await expect(page.locator("#after script")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Apply unavailable" })).toBeDisabled();
  await page.locator("#decision-reason").fill("Reviewed exact diff");
  page.on("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Approve revision", exact: true }).click();
  await expect(page.locator("#status")).toHaveText("Recorded approved. Runtime unchanged.");
  expect(mutations).toHaveLength(1);
  expect(mutations[0].headers()["x-zpr-csrf"]).toBe("proof");
  expect(mutations[0].postDataJSON()).toEqual({ organization: "great-lakes", digest: "a".repeat(64), expected_version: 1, reason: "Reviewed exact diff" });
  await expect(page.getByRole("button", { name: "Approve revision", exact: true })).toBeDisabled();
});

test("self-approval is disabled unless single-operator mode is explicitly enabled", async ({ page }) => {
  await setup(page, { subject: "author" });
  await expect(page.getByRole("button", { name: "Approve revision", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Withdraw my request", exact: true })).toBeEnabled();
  await expect(page.locator("#decision-note")).toContainText("requires a different operator");
});

test("explicit single-operator mode enables author approval", async ({ page }) => {
  await setup(page, { subject: "author", self: true });
  await expect(page.getByRole("button", { name: "Approve revision", exact: true })).toBeEnabled();
  await expect(page.locator("#capabilities")).toContainText("explicitly enabled");
});

test("failed mutation is not retried and reconciliation restores available controls", async ({ page }) => {
  const { mutations } = await setup(page, { mutate: route => route.abort("failed") });
  await page.locator("#decision-reason").fill("Review");
  page.on("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Approve revision", exact: true }).click();
  await expect(page.locator("#status")).toContainText("No automatic retry");
  await expect(page.getByRole("button", { name: "Approve revision", exact: true })).toBeDisabled();
  expect(mutations).toHaveLength(1);
  await page.getByRole("button", { name: "Load queue", exact: true }).click();
  await expect(page.locator("#status")).toContainText("Queue refreshed");
  await expect(page.getByRole("button", { name: "Approve revision", exact: true })).toBeEnabled();
});

test("failed refresh retains last-good diff with an explicit stale warning", async ({ page }) => {
  const control = await setup(page);
  control.failQueue();
  await page.getByRole("button", { name: "Load queue", exact: true }).click();
  await expect(page.locator("#status")).toContainText("Existing data may be stale");
  await expect(page.locator("#diff")).toContainText("+allowed new");
});

test("organization changes clear old source details and reject invalid scope", async ({ page }) => {
  await setup(page);
  await page.locator("#organization").fill("*");
  await expect(page.locator("#review")).toBeHidden();
  await page.getByRole("button", { name: "Load queue", exact: true }).click();
  await expect(page.locator("#status")).toHaveText("Enter an explicit organization ID.");
});

test("mutation timeout leaves outcome unknown without resubmission or review switching", async ({ page }) => {
  await page.clock.install();
  const { mutations } = await setup(page, { mutate: () => {} });
  await page.locator("#decision-reason").fill("Review");
  page.on("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Approve revision", exact: true }).click();
  await expect(page.getByRole("button", { name: "Review", exact: true })).toBeDisabled();
  await expect(page.locator("#submission button")).toBeDisabled();
  await page.clock.runFor(120001);
  await expect(page.locator("#status")).toContainText("No automatic retry");
  await expect(page.getByRole("button", { name: "Approve revision", exact: true })).toBeDisabled();
  expect(mutations).toHaveLength(1);
  await page.getByRole("button", { name: "Load queue", exact: true }).click();
  await expect(page.locator("#status")).toContainText("Queue refreshed");
  await expect(page.getByRole("button", { name: "Approve revision", exact: true })).toBeEnabled();
  expect(mutations).toHaveLength(1);
});

test("authorization loss erases source details and offers sign-in instead of retrying", async ({ page }) => {
  const { mutations } = await setup(page, { mutate: route => route.fulfill({ status: 403, json: { error: "Session expired" } }) });
  await page.locator("#decision-reason").fill("Review");
  page.on("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Approve revision", exact: true }).click();
  await expect(page.locator("#workspace")).toHaveAttribute("disabled", "");
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
  await expect(page.locator("#review")).toBeHidden();
  await expect(page.locator("#after")).toHaveText("");
  await expect(page.locator("#before")).toHaveText("");
  await expect(page.locator("#diff")).toHaveText("");
  await expect(page.locator("#requests")).toHaveText("");
  expect(mutations).toHaveLength(1);
});

test("malformed audit response retains the verified source instead of replacing it", async ({ page }) => {
  await setup(page);
  await page.route(origin + "/api/change-review/requests?**", route => route.fulfill({
    json: { requests: [{ ...fixtureRecord(), events: [{ action: "approve" }] }], next_offset: -1 },
  }));
  await page.getByRole("button", { name: "Load queue", exact: true }).click();
  await expect(page.locator("#status")).toContainText("Invalid review queue response");
  await expect(page.locator("#diff")).toContainText("+allowed new");
});
