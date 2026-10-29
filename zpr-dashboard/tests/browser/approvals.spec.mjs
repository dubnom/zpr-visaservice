import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const origin = "https://control-room.example.test";
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const digest = "a".repeat(64);

function review(id, state, extra = {}) {
  const future = new Date(Date.now() + 3600e3).toISOString();
  return {
    id, digest, state, version: "1",
    proposal: {
      organization_id: "alpha", registry_revision: "b".repeat(64),
      root: { authority_id: "auth", authority_version: "1", subject_kind: "device", subject_id: "laptop-7" },
      target: { source_id: "ldap", source_version: "1", selector_kind: "ldap_entry_uuid", subject_id: "11111111-2222-3333-4444-555555555555" },
      enrichment_keys: ["mail"], not_before: new Date().toISOString(), expires_at: future,
      reason: "Onboarding", author: { issuer: "https://issuer.example.test", subject: extra.author ?? "simulator" },
      submitted_at: new Date().toISOString(),
    },
    events: extra.events ?? [],
  };
}

async function open(page, context, { permissions = ["subject-link.read", "subject-link.approve"], reviews }) {
  const approvals = [];
  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin) return route.abort();
    if (url.pathname === "/auth/operator/config") return route.fulfill({ json: { enabled: true } });
    if (url.pathname === "/auth/operator/session") {
      return route.fulfill({ json: { identity: { issuer: "https://issuer.example.test", subject: "operator-1",
        display_name: "Op", organizations: ["alpha"], permissions }, csrf: "csrf" } });
    }
    if (url.pathname === "/api/subject-link-reviews/queue") return route.fulfill({ json: { reviews } });
    const approve = url.pathname.match(/^\/api\/subject-link-reviews\/([^/]+)\/approve$/);
    if (approve) {
      approvals.push({ id: approve[1], body: request.postDataJSON(), csrf: request.headers()["x-zpr-csrf"] });
      return route.fulfill({ json: { ...review(approve[1], "review-approved"), events: [] } });
    }
    const name = url.pathname === "/" ? "/index.html" : url.pathname;
    if (/\.(js|css|html)$/.test(name)) {
      const path = resolve(assets, `.${name}`);
      const type = name.endsWith(".js") ? "application/javascript" : name.endsWith(".css") ? "text/css" : "text/html";
      try { return route.fulfill({ contentType: type, body: await readFile(path, "utf8") }); } catch { return route.fulfill({ status: 404 }); }
    }
    return route.fulfill({ status: 404 });
  });
  await page.goto(`${origin}/approvals.html`);
  return approvals;
}

test("Approvals lists waiting requests in plain words and approves without typed identifiers", async ({ page, context }) => {
  const approvals = await open(page, context, { reviews: [review("r1", "submitted"), review("r2", "review-approved", { events: [{ actor: { issuer: "https://issuer.example.test", subject: "simulator" } }] })] });
  const items = page.locator("#approvals-list li");
  await expect(items).toHaveCount(1);
  await expect(items.first()).toContainText("Link device laptop-7 to directory entry");
  await expect(items.first()).toContainText("Waiting");
  page.on("dialog", dialog => dialog.accept());
  await items.first().locator("button", { hasText: "Approve" }).click();
  await expect.poll(() => approvals.length).toBe(1);
  expect(approvals[0]).toMatchObject({ id: "r1", csrf: "csrf", body: { organization_id: "alpha", digest, version: "1" } });
  await page.locator("#approvals-filter").selectOption("all");
  await expect(items).toHaveCount(2);
});

test("Approvals disables Approve without the approve permission and warns on self-approval", async ({ page, context }) => {
  await open(page, context, { permissions: ["subject-link.read"], reviews: [review("r1", "submitted", { author: "operator-1" })] });
  const item = page.locator("#approvals-list li").first();
  await expect(item.locator("button", { hasText: "Approve" })).toBeDisabled();
  await expect(item).toContainText("single-operator");
});
