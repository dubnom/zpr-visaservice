import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

const staticRoot = new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url);
const pin = "a".repeat(64);

async function fixture(page, { development = false, savedPin } = {}) {
  const mutations = [];
  const html = (await readFile(new URL("index.html", staticRoot), "utf8"))
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, "");
  const record = (attestation) => ({ attestation, revision: 1, updated_by: "named-admin", updated_at: new Date().toISOString() });
  await page.route("https://development-enrollment.test/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/api/enrollment/v1/preflight") {
      if (request.method() === "POST") {
        const body = request.postDataJSON();
        mutations.push(body);
        return route.fulfill({ json: { preflight: record(body) } });
      }
      if (savedPin) {
        return route.fulfill({ json: { ready: true, preflight: record({
          organization: "company", deployment_mode: "development", host_label: "workbench.local",
          host_address: "10.0.1.160", delivery_channel: "development-endpoint-local",
          private_ca_fingerprint: savedPin, acme_verified: false, backup_verified: false,
          dns_verified: true, tls_verified: true, network_verified: true, docker_verified: true,
          listeners_verified: true, package_verified: true, recovery_verified: true,
        }) } });
      }
      return route.fulfill({ json: { ready: false } });
    }
    if (path === "/") return route.fulfill({ contentType: "text/html", body: html });
    return route.fulfill({ status: 404 });
  });
  await page.goto("https://development-enrollment.test/#provisioning-adapters");
  await page.locator("#provisioning-preflight").evaluate((form) => {
    for (let node = form; node; node = node.parentElement) {
      node.hidden = false;
      node.style.display = "block";
    }
  });
  for (const script of ["page-runtime.js", "provisioning-contract.js", "provisioning.js"]) {
    await page.addScriptTag({ content: await readFile(new URL(script, staticRoot), "utf8") });
  }
  await page.evaluate(({ development, pin }) => {
    window.dispatchEvent(new CustomEvent("operator-session-ready", { detail: {
      csrf: "fixture-csrf", identity: { issuer: "https://identity.example", subject: "named-admin",
        organizations: ["company"], permissions: ["read", "create"] },
    } }));
    window.dispatchEvent(new CustomEvent("provisioning-catalog-ready", { detail: {
      organizations: { company: { profiles: ["standard"], types: ["laptop"] } },
      gui_create_organizations: ["company"], invitation_lifetime_seconds: 3600,
      ...(development ? { software_development_ca_fingerprint: pin } : {}),
    } }));
  }, { development, pin });
  return mutations;
}

test("private-CA development choice is absent unless the backend explicitly enables it", async ({ page }) => {
  const mutations = await fixture(page);
  const form = page.locator("#provisioning-preflight");
  await expect(form.locator('[name="deployment_mode"] option[value="development"]')).toHaveCount(0);
  await expect(form.locator('[name="acme_verified"]')).toBeVisible();
  await expect(form.locator('[name="deployment_mode"]')).toHaveValue("pilot");
  expect(mutations).toEqual([]);
});

test("development submits exact private CA trust without fabricating ACME or notarization", async ({ page }) => {
  const mutations = await fixture(page, { development: true });
  const form = page.locator("#provisioning-preflight");
  await form.locator('[name="deployment_mode"]').selectOption("development");
  await form.locator('[name="delivery_channel"]').selectOption("development-endpoint-local");
  await form.locator('[name="host_label"]').fill("workbench.local");
  await form.locator('[name="host_address"]').fill("10.0.1.160");
  await expect(form.locator('[name="acme_verified"]')).toBeHidden();
  await expect(form).toContainText("No ACME, notarization, production readiness or runtime admission is claimed");
  for (const check of await form.locator('.provisioning-preflight-checks input[type="checkbox"]:visible').all()) await check.check();
  await form.getByRole("button", { name: "Evaluate readiness", exact: true }).click();
  await expect(page.locator("#provisioning-preflight-status")).toHaveAttribute("data-state", "ready");
  expect(mutations).toHaveLength(1);
  expect(mutations[0]).toMatchObject({
    deployment_mode: "development", private_ca_fingerprint: pin, acme_verified: false,
    delivery_channel: "development-endpoint-local", tls_verified: true, recovery_verified: true,
  });
  await page.evaluate(() => window.dispatchEvent(new Event("provisioning-catalog-cleared")));
  await expect(form.locator('[name="deployment_mode"] option[value="development"]')).toHaveCount(0);
  await expect(form.locator('[name="acme_verified"]')).toBeVisible();
  await expect(page.locator("#provisioning-create")).toBeDisabled();
});

test("saved development readiness for another private CA cannot unlock invitation creation", async ({ page }) => {
  await fixture(page, { development: true, savedPin: "b".repeat(64) });
  await expect(page.locator("#provisioning-preflight-status")).toContainText("Invalid readiness attestation response");
  await expect(page.locator("#provisioning-create")).toBeDisabled();
});
