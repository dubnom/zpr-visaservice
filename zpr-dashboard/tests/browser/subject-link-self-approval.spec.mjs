import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const staticRoot = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const origin = "https://subject-review.example.test";

async function setup(page, subject, self) {
  const identity = { issuer: "https://idp.example", subject, organizations: ["production"],
    permissions: ["subject-link.read", "subject-link.approve", ...(self ? ["approval.self"] : [])] };
  const record = { id: "review-one", digest: "a".repeat(64), version: "1", state: "submitted",
    proposal: { organization_id: "production", author: { issuer: identity.issuer, subject: "author" },
      registry_revision: "b".repeat(64),
      root: { subject_kind: "device", subject_id: "device-one", authority_id: "bas", authority_version: "1" },
      target: { selector_kind: "ldap_entry_uuid", subject_id: "entry-one", source_id: "directory", source_version: "1" },
      enrichment_keys: ["user.department"] }, events: [] };
  const mutations = [];
  await page.addInitScript(() => { window.zprOperatorFetch = (path, options) => fetch(path, options); });
  await page.route(origin + "/**", async route => {
    const request = route.request(), path = new URL(request.url()).pathname;
    if (path === "/api/subject-link-reviews/review-one" && request.method() === "GET") {
      return route.fulfill({ json: record });
    }
    if (path === "/api/subject-link-reviews/review-one/approve") {
      mutations.push(request);
      return route.fulfill({ json: { ...record, version: "2", state: "review-approved",
        events: [{ actor: { issuer: identity.issuer, subject }, action: "approve-review", self_review: subject === "author" }] } });
    }
    const name = path.slice(1);
    if (["subject-links.html", "subject-links.js", "change-review.css"].includes(name)) {
      return route.fulfill({ contentType: name.endsWith(".js") ? "application/javascript" : name.endsWith(".css") ? "text/css" : "text/html",
        body: readFileSync(staticRoot + name, "utf8") });
    }
    return route.fulfill({ contentType: "application/javascript", body: "" });
  });
  await page.goto(origin + "/subject-links.html");
  await page.evaluate(identity => window.dispatchEvent(new CustomEvent("operator-session-ready", { detail: { identity } })), identity);
  await page.locator("#subject-review-id").fill(record.id);
  await page.locator("#subject-review-read").click();
  await expect(page.locator("#subject-review-evidence")).toContainText(record.digest);
  return { mutations, record };
}

for (const mode of ["self-denied", "self-allowed", "independent"]) {
  test(`subject-link ${mode} uses explicit identity permission and never auto-approves`, async ({ page }) => {
    const { mutations, record } = await setup(page, mode === "independent" ? "reviewer" : "author", mode === "self-allowed");
    const approve = page.locator("#subject-approve");
    expect(mutations).toHaveLength(0);
    if (mode === "self-denied") {
      await expect(approve).toBeDisabled();
      return;
    }
    await expect(approve).toBeEnabled();
    await page.locator("#subject-approval-reason").fill("Reviewed exact candidate");
    page.once("dialog", async dialog => {
      expect(dialog.message()).toContain(mode === "self-allowed" ? "Explicitly self-approve" : "Approve review");
      await dialog.accept();
    });
    await approve.click();
    await expect(page.locator("#status")).toContainText(mode === "self-allowed" ? "Self-approval recorded" : "Independent review recorded");
    expect(mutations).toHaveLength(1);
    expect(mutations[0].postDataJSON()).toEqual({ organization_id: "production", digest: record.digest,
      version: record.version, registry_revision: record.proposal.registry_revision, reason: "Reviewed exact candidate" });
    await expect(approve).toBeDisabled();
  });
}
