import assert from "node:assert/strict";
import { chromium, expect } from "@playwright/test";

const origin = process.env.ZPR_TEST_SUBJECT_LINK_ORIGIN;
const idp = process.env.ZPR_TEST_SUBJECT_LINK_IDP;
const reviewID = process.env.ZPR_TEST_SUBJECT_LINK_REVIEW;
const reviewDigest = process.env.ZPR_TEST_SUBJECT_LINK_DIGEST;
assert.ok(origin?.startsWith("https://127.0.0.1:"));
assert.ok(idp?.startsWith("https://127.0.0.1:"));
assert.match(reviewDigest ?? "", /^[0-9a-f]{64}$/);
const pendingKey = "zpr.subject-link-publication.pending.v1";
const browser = await chromium.launch({
  args: [`--ignore-certificate-errors-spki-list=${process.env.ZPR_TEST_SUBJECT_LINK_SPKI}`],
});
try {
  const context = await browser.newContext({
    timezoneId: "UTC",
    viewport: { width: 1280, height: 900 },
    extraHTTPHeaders: { "X-Subject-Fixture": process.env.ZPR_TEST_SUBJECT_LINK_OWNER },
  });
  context.setDefaultTimeout(7000);
  const errors = [], mutations = [];
  context.on("page", page => page.on("pageerror", error => errors.push(error.message)));
  context.on("request", request => {
    const url = new URL(request.url());
    if (request.method() === "POST" && url.pathname.startsWith("/api/subject-link-publications/")) {
      mutations.push({ path: url.pathname, body: request.postData(), key: request.headers()["idempotency-key"] });
    }
  });
  const page = await context.newPage();
  const document = await page.goto(`${origin}/subject-link-publications.html`);
  assert.ok(document.headers()["content-security-policy"].includes("script-src 'self'"));
  const status = page.locator("#status");
  async function consent(subject) {
    await expect(page).toHaveURL(new RegExp(`^${idp.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/authorize`));
    await page.getByRole("button", { name: `Continue as fixture ${subject}`, exact: true }).click();
    await expect(page).toHaveURL(`${origin}/subject-link-publications.html`);
    await expect(page.locator("#publication-workspace")).toBeEnabled();
    await expect(page.locator("#publication-organization")).toHaveValue("production");
  }
  async function stream() {
    await page.locator("#publication-recipient").fill("vs-browser");
    await page.locator("#publication-epoch").fill("epoch-1");
  }
  async function readState(sequence) {
    await page.locator("#publication-read").click();
    await expect(page.locator("#publication-sequence")).toHaveText(sequence);
  }

  // Publisher: provision and promote; capture and signing controls stay unavailable.
  await consent("publisher");
  await stream();
  await expect(page.locator("#publication-capture-save")).toBeDisabled();
  await page.locator("#publication-read").click();
  await expect(status).toHaveClass(/error/);
  await page.locator("#publication-provision summary").click();
  await page.locator("#publication-provision-reason").fill("Provision browser stream");
  page.once("dialog", dialog => dialog.dismiss());
  await page.locator("#publication-provision-save").click();
  assert.equal(mutations.length, 0, "cancelled provisioning must not reach the service");
  page.once("dialog", dialog => dialog.accept());
  await page.locator("#publication-provision-save").click();
  await expect(status).toContainText("provisioned at sequence 0");
  assert.equal(mutations[0].key, undefined, "provisioning is not an idempotent retry");
  await readState("0");
  await expect(page.locator("#publication-current")).toHaveText("None at the current sequence");
  await expect(page.locator("#publication-key")).toHaveText("browser-key");
  await expect(page.locator("#publication-capture-save")).toBeDisabled();
  await expect(page.locator("#publication-sign-save")).toBeDisabled();
  await page.locator("#publication-review-id").fill(reviewID);
  await page.locator("#publication-review-digest").fill(reviewDigest);
  await page.locator("#publication-intent-reason").fill("Promote independently approved review");
  page.once("dialog", dialog => dialog.accept());
  await page.locator("#publication-intent-save").click();
  await expect(status).toContainText("Publication intent recorded at sequence 1");
  await expect(page.locator("#publication-intent-save")).toBeDisabled();
  assert.equal(JSON.parse(mutations[1].body).expected_sequence, "0");
  assert.equal(await page.evaluate(key => sessionStorage.getItem(key), pendingKey), null);

  // Capturer: lost response is saved, survives reload, and is retried only explicitly.
  await page.locator("#operator-logout").click();
  await consent("capturer");
  await stream();
  await readState("1");
  await expect(page.locator("#publication-intent-save")).toBeDisabled();
  await expect(page.locator("#publication-capture-save")).toBeEnabled();
  const now = Date.now();
  await page.locator("#publication-issued").fill(new Date(now - 60000).toISOString().slice(0, 19));
  await page.locator("#publication-expires").fill(new Date(now - 120000).toISOString().slice(0, 19));
  await page.locator("#publication-capture-reason").fill("Browser renewal capture");
  await page.locator("#publication-capture-save").click();
  await expect(status).toContainText("Expiry must be after");
  assert.equal(mutations.length, 2, "invalid validity must not be sent");
  await page.locator("#publication-expires").fill(new Date(now + 3600000).toISOString().slice(0, 19));
  assert.equal(await page.evaluate(async () => (await fetch("/fixture/drop-next-capture-response", { method: "POST" })).status), 204);
  page.once("dialog", dialog => dialog.accept());
  await page.locator("#publication-capture-save").click();
  await expect(status).toContainText("Control-Service unavailable");
  await expect(page.locator("#publication-retry")).toBeEnabled();
  assert.equal(mutations.length, 3);
  const saved = await page.evaluate(key => JSON.parse(sessionStorage.getItem(key)), pendingKey);
  assert.equal(saved.body, mutations[2].body);
  assert.equal(saved.key, mutations[2].key);
  await page.reload();
  await expect(page.locator("#publication-workspace")).toBeEnabled();
  await expect(page.locator("#publication-recipient")).toHaveValue("vs-browser");
  await expect(page.locator("#publication-retry")).toBeEnabled();
  await page.locator("#publication-read").click();
  await expect(page.locator("#publication-sequence")).toHaveText("2");
  await expect(page.locator("#publication-capture-save")).toBeDisabled();
  assert.equal(mutations.length, 3, "reload must not automatically retry a lost response");
  await page.locator("#publication-epoch").fill("epoch-2");
  await expect(page.locator("#publication-retry")).toBeDisabled();
  await page.locator("#publication-epoch").fill("epoch-1");
  await page.locator("#publication-retry").click();
  await expect(status).toContainText("Unsigned candidate captured at sequence 2");
  assert.deepEqual(mutations[3], mutations[2], "explicit retry must preserve exact body and semantic key");
  assert.equal(await page.evaluate(key => sessionStorage.getItem(key), pendingKey), null);

  // Signer: verifies the exact bytes in the browser before signing that digest.
  await page.locator("#operator-logout").click();
  await consent("snapshot-signer");
  await stream();
  await readState("2");
  await expect(page.locator("#publication-current")).toContainText("unsigned");
  await expect(page.locator("#publication-sign-save")).toBeDisabled();
  await page.locator("#publication-candidate-read").click();
  await expect(page.locator("#publication-candidate-check")).toContainText("matches sequence 2");
  await expect(page.locator("#publication-candidate-evidence")).toContainText("vs-browser");
  await page.locator("#publication-sign-reason").fill("Sign inspected browser candidate");
  page.once("dialog", dialog => dialog.accept());
  await page.locator("#publication-sign-save").click();
  await expect(status).toContainText("Candidate 2 signed with key browser-key");
  const signed = JSON.parse(await page.locator("#publication-history-evidence").textContent());
  assert.equal(signed.actor.subject, "snapshot-signer");
  await readState("2");
  await expect(page.locator("#publication-current")).toContainText("signed by browser-key");
  await expect(page.locator("#publication-sign-save")).toBeDisabled();
  await expect(page.locator("#publication-receipt")).toHaveText("No receipt reported");
  assert.equal(await page.evaluate(async () => (await fetch("/fixture/record-delivery-receipt", { method: "POST" })).status), 204);
  await readState("2");
  await expect(page.locator("#publication-receipt")).toContainText(`Sequence 2, digest ${signed.digest}, key browser-key`);
  await expect(page.locator("#publication-receipt")).toContainText("recipient report, not enforcement");
  for (const [kind, sequence, text] of [["events", "1", '"action": "promote"'], ["candidates", "2", signed.digest], ["signed", "2", '"key_id": "browser-key"'], ["receipts", "2", '"client_spki_sha256": "' + "c".repeat(64)]]) {
    await page.locator("#publication-history-kind").selectOption(kind);
    await page.locator("#publication-history-sequence").fill(sequence);
    await page.locator("#publication-history-read").click();
    await expect(page.locator("#publication-history-evidence")).toContainText(text);
  }
  for (const viewport of [{ width: 1280, height: 900 }, { width: 820, height: 1000 }]) {
    await page.setViewportSize(viewport);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "operator page must not overflow horizontally");
    assert.equal(await page.getByRole("button", { name: /^(Activate|Deliver|Deploy|Enforce)/ }).count(), 0);
  }
  assert.equal(mutations.length, 5);
  assert.deepEqual(errors, []);
  console.log("SUBJECT_LINK_PUBLICATION_BROWSER ok: provision, promote, lost-response capture retry, browser digest check, separated signing, receipt display and readback, desktop/tablet; no push, enforcement or Simulator");
  await context.close();
} finally {
  await browser.close();
}
