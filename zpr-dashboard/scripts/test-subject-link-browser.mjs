import assert from "node:assert/strict";
import { chromium, expect } from "@playwright/test";

const origin = process.env.ZPR_TEST_SUBJECT_LINK_ORIGIN;
const idp = process.env.ZPR_TEST_SUBJECT_LINK_IDP;
assert.ok(origin?.startsWith("https://127.0.0.1:"));
assert.ok(idp?.startsWith("https://127.0.0.1:"));
const browser = await chromium.launch({
  args: [`--ignore-certificate-errors-spki-list=${process.env.ZPR_TEST_SUBJECT_LINK_SPKI}`],
});
try {
  const context = await browser.newContext({
    timezoneId: "UTC",
    viewport: { width: 1280, height: 900 },
    extraHTTPHeaders: { "X-Subject-Fixture": process.env.ZPR_TEST_SUBJECT_LINK_OWNER },
  });
  assert.deepEqual(await context.cookies(), [], "browser must start without imported sessions");
  context.setDefaultTimeout(7000);
  const errors = [], requests = [], mutations = [];
  context.on("page", page => page.on("pageerror", error => errors.push(error.message)));
  context.on("request", request => {
    const url = new URL(request.url());
    requests.push(url);
    if (request.method() === "POST" && url.pathname.startsWith("/api/subject-link-reviews/")) {
      mutations.push({ path: url.pathname, body: request.postData(), key: request.headers()["idempotency-key"] });
    }
  });
  const page = await context.newPage();
  const document = await page.goto(`${origin}/subject-links.html`);
  assert.equal(document.headers()["referrer-policy"], "same-origin");
  assert.ok(document.headers()["content-security-policy"].includes("script-src 'self'"));
  async function consent(subject) {
    await expect(page).toHaveURL(new RegExp(`^${idp.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/authorize`));
    await page.getByRole("button", { name: `Continue as fixture ${subject}`, exact: true }).click();
    await expect(page).toHaveURL(`${origin}/subject-links.html`);
    await expect(page.locator("#subject-workspace")).toBeEnabled();
    await expect(page.locator("#subject-organization")).toHaveValue("production");
  }
  await consent("administrator");
  await expect(page.locator("#operator-scope")).toContainText("subject-link.registry.manage");
  await page.locator("#subject-registry summary").click();
  async function edit(kind, id, config) {
    await page.locator("#subject-registry-kind").selectOption(kind);
    await page.locator("#subject-registry-id").fill(id);
    await page.locator("#subject-registry-version").fill("");
    await page.locator("#subject-registry-config").fill(JSON.stringify(config));
    await page.locator("#subject-registry-reason").fill(`Provision fixture ${id}`);
    page.once("dialog", dialog => dialog.accept());
    await page.locator("#subject-registry-save").click();
    await expect(page.locator("#status")).toContainText("Registration version 1 recorded");
    await expect(page.locator("#subject-registry-version")).toHaveValue("1");
  }
  await edit("authorities", "bas", { kind: "bas_device", service_id: "bas-service", issuer: "zpr/bas", enabled: true });
  await edit("sources", "company", { provider_id: "directory", selector_kind: "ldap_entry_uuid", enrichment_keys: ["user.department"], enabled: true });
  await page.locator("#subject-history-version").fill("1");
  await page.locator("#subject-registry-history").click();
  await expect(page.locator("#subject-registry-evidence")).toContainText('"subject": "administrator"');
  await expect(page.locator("#subject-propose")).toBeDisabled();
  await page.locator("#operator-logout").click();
  await consent("author");
  await expect(page.locator("#subject-registry-save")).toBeDisabled();
  for (const [id, value] of Object.entries({
    authority: "bas", "authority-version": "1", device: "verified-device",
    source: "company", "source-version": "1", uuid: "b6a21ab4-5554-4cde-ae65-41a406a3c07f",
  })) await page.locator(`#subject-${id}`).fill(value);
  await page.locator("#subject-context-load").click();
  await expect(page.locator("#status")).toContainText("Current registry context captured");
  await page.locator("#subject-keys").fill("user.department");
  await page.locator("#subject-not-before").fill(new Date(Date.now() - 60000).toISOString().slice(0, 16));
  await page.locator("#subject-expires").fill(new Date(Date.now() + 3600000).toISOString().slice(0, 16));
  await page.locator("#subject-proposal-reason").fill("Browser exact-subject candidate");
  await page.locator("#subject-keys").fill("x".repeat(8192));
  page.once("dialog", dialog => dialog.accept());
  await page.locator("#subject-propose").click();
  await expect(page.locator("#status")).toContainText("8192-byte limit");
  assert.equal(mutations.length, 0, "oversized proposal must not be persisted or sent");
  assert.equal(await page.evaluate(() => sessionStorage.getItem("zpr.subject-link.pending.v1")), null);
  await page.locator("#subject-keys").fill("user.department");
  page.once("dialog", dialog => dialog.dismiss());
  await page.locator("#subject-propose").click();
  assert.equal(mutations.length, 0, "cancelled submission must not reach the service");
  const armed = await page.evaluate(async () => (await fetch("/fixture/drop-next-review-response", { method: "POST" })).status);
  assert.equal(armed, 204);
  page.once("dialog", dialog => dialog.accept());
  await page.locator("#subject-propose").click();
  await expect(page.locator("#status")).toContainText("Control-Service unavailable");
  await expect(page.locator("#subject-retry")).toBeEnabled();
  assert.equal(mutations.length, 1);
  const saved = await page.evaluate(() => JSON.parse(sessionStorage.getItem("zpr.subject-link.pending.v1")));
  assert.equal(saved.body, mutations[0].body);
  assert.equal(saved.key, mutations[0].key);
  await page.reload();
  await expect(page.locator("#subject-retry")).toBeEnabled();
  await expect(page.locator("#subject-propose")).toBeDisabled();
  assert.equal(mutations.length, 1, "reload must not automatically retry a lost response");
  await page.locator("#subject-organization").fill("foreign");
  await expect(page.locator("#subject-retry")).toBeDisabled();
  await page.locator("#subject-organization").fill("production");
  await expect(page.locator("#subject-retry")).toBeEnabled();
  await page.locator("#operator-logout").click();
  await consent("reviewer");
  await expect(page.locator("#subject-retry")).toBeDisabled();
  await expect(page.locator("#subject-pending-note")).toContainText("another identity");
  await page.locator("#operator-logout").click();
  await consent("author");
  await expect(page.locator("#subject-retry")).toBeEnabled();
  assert.equal(mutations.length, 1, "scope or identity changes must not replay another operator's request");
  await page.locator("#subject-retry").click();
  await expect(page.locator("#status")).toContainText("Candidate submitted");
  assert.deepEqual(mutations[1], mutations[0], "explicit retry must preserve exact body and semantic key");
  const review = JSON.parse(await page.locator("#subject-review-evidence").textContent());
  assert.equal(review.proposal.author.subject, "author");
  assert.equal(review.state, "submitted");
  await expect(page.locator("#subject-approve")).toBeDisabled();
  assert.equal(await page.evaluate(() => sessionStorage.getItem("zpr.subject-link.pending.v1")), null);
  await page.locator("#operator-logout").click();
  await consent("reviewer");
  await page.locator("#subject-review-id").fill(review.id);
  await page.locator("#subject-review-read").click();
  await expect(page.locator("#subject-review-evidence")).toContainText(review.digest);
  await expect(page.locator("#subject-approve")).toBeEnabled();
  await page.locator("#subject-approval-reason").fill("Independent browser review of exact digest");
  page.once("dialog", dialog => dialog.dismiss());
  await page.locator("#subject-approve").click();
  assert.equal(mutations.length, 2, "cancelled approval must not mutate review");
  page.once("dialog", dialog => dialog.accept());
  await page.locator("#subject-approve").click();
  await expect(page.locator("#status")).toContainText("Independent review recorded");
  const approved = JSON.parse(await page.locator("#subject-review-evidence").textContent());
  assert.equal(approved.state, "review-approved");
  assert.equal(approved.events.length, 2);
  assert.equal(approved.events[1].actor.subject, "reviewer");
  await expect(page.locator("#subject-approve")).toBeDisabled();
  assert.equal(mutations.length, 3);
  for (const viewport of [{ width: 1280, height: 900 }, { width: 820, height: 1000 }]) {
    await page.setViewportSize(viewport);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "operator page must not overflow horizontally");
    assert.equal(await page.getByRole("button", { name: /^(Activate|Publish|Deploy)/ }).count(), 0);
  }
  await page.locator("#subject-review-id").fill("missing-review");
  await expect(page.locator("#subject-approve")).toBeDisabled();
  await page.locator("#subject-review-read").click();
  await expect(page.locator("#status")).toContainText("not found");
  await expect(page.locator("#subject-review-evidence")).toHaveText("");
  // Invalid persisted retry data must block new work until explicit resolution.
  for (const invalid of ["broken-json", "null", "false", "[]"]) {
    await page.evaluate((invalid) => sessionStorage.setItem("zpr.subject-link.pending.v1", invalid), invalid);
    await page.reload();
    await expect(page.locator("#subject-pending")).toBeVisible();
    await expect(page.locator("#subject-retry")).toBeDisabled();
    await expect(page.locator("#subject-propose")).toBeDisabled();
    page.once("dialog", dialog => dialog.accept());
    await page.locator("#subject-discard").click();
    await expect(page.locator("#subject-pending")).toBeHidden();
  }
  const cookies = await context.cookies(origin);
  const session = await page.evaluate(async () => (await fetch("/auth/operator/session", { cache: "no-store" })).json());
  await page.locator("#operator-logout").click();
  await expect(page).toHaveURL(new RegExp(`^${idp.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/authorize`));
  const stale = await browser.newContext({ extraHTTPHeaders: { "X-Subject-Fixture": process.env.ZPR_TEST_SUBJECT_LINK_OWNER } });
  await stale.addCookies(cookies);
  const stalePage = await stale.newPage();
  await stalePage.goto(`${origin}/fixture/stale-document.html`);
  const denied = await stalePage.evaluate(async ({ review, csrf }) => (await fetch(`/api/subject-link-reviews/${review.id}/approve`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-ZPR-CSRF": csrf, "Idempotency-Key": "stale-browser-review" },
    body: JSON.stringify({ organization_id: "production", digest: review.digest, version: "1", registry_revision: review.proposal.registry_revision, reason: "stale logout attempt" }),
  })).status, { review, csrf: session.csrf });
  assert.equal(denied, 403, "logged-out cookie and cached CSRF must not authorize a review");
  await stale.close();
  assert.deepEqual(errors, [], "browser scripts must not throw");
  assert.equal(requests.some(url => /simulator|manifest|workloads/.test(url.pathname)), false);
  assert.equal(requests.filter(url => url.origin === origin && url.pathname === "/auth/operator/callback").length, 5);
  console.log(`SUBJECT_LINK_BROWSER review: ${review.id}`);
  console.log("SUBJECT_LINK_BROWSER passed: native OIDC admin/proposer/reviewer, exact registry context, cancelled confirmations, lost-response reload/explicit retry, independent approval, desktop/tablet layout, stale-state and logout denial; no Simulator or active publication");
  await context.close();
} finally {
  await browser.close();
}
