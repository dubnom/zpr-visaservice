import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test.skip(!process.env.ZPR_LOCAL_OPERATOR_DIR, "Requires an explicitly configured local operator deployment.");
test.use({
  trace: "off", screenshot: "off", video: "off",
  launchOptions: { args: process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS ? JSON.parse(process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS) : [] },
});

test("deployed Control Room renders the configured gateway cloud in both maps", async ({ page }) => {
  test.skip(!process.env.ZPR_EXPECT_GATEWAY_SERVICE || !process.env.ZPR_EXPECT_GATEWAY_NETWORK,
    "Requires an explicitly selected production gateway service and external network.");
  const directory = process.env.ZPR_LOCAL_OPERATOR_DIR;
  const settings = JSON.parse(readFileSync(`${directory}/stack.json`, "utf8"));
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(settings.origin + "/#map");
  await expect(page.locator("#operator-login-form")).toBeVisible();
  await page.locator("#operator-login-form").evaluate(form => form.requestSubmit());
  await page.locator('input[name="login"]').fill(settings.username);
  await page.locator('input[name="password"]').fill(readFileSync(`${directory}/operator-password`, "utf8").trim());
  await page.locator("#submit-login").click();
  await expect(page.locator("#operator-login-status")).toHaveText(/^User(?:: .+)?$/);
  const snapshot = await page.evaluate(async () => {
    const response = await fetch("/api/snapshot");
    if (!response.ok) throw new Error(`Snapshot request failed: ${response.status}`);
    return response.json();
  });
  expect(snapshot.api_status).toBe("connected");
  expect(snapshot.errors).toEqual([]);
  const gateways = snapshot.services.filter(service => service.service_name === process.env.ZPR_EXPECT_GATEWAY_SERVICE);
  expect(gateways).toHaveLength(1);
  const gateway = gateways[0];
  expect(gateway.service_kind).toBe("Gateway");
  expect(gateway.external_network_connection).toBe(process.env.ZPR_EXPECT_GATEWAY_NETWORK);
  expect(snapshot.actors.filter(actor => actor.cn === gateway.actor_cn)).toHaveLength(1);
  for (const mode of ["Topology", "World Map"]) {
    await page.getByRole("button", { name: mode, exact: true }).click();
    const component = page.locator(`#topology-stage .graph-vertex[data-inspect-actor=${JSON.stringify(gateway.actor_cn)}]`);
    await expect(component.locator(".graph-external-network")).toHaveAttribute("aria-label",
      `Gateway connection to ${gateway.external_network_connection}`);
    await expect(component.locator(".graph-cloud")).toBeVisible();
    await expect(component.locator(".gateway-cloud-link")).toHaveCount(1);
    await expect(component.locator(".graph-cloud-label")).toHaveText(gateway.external_network_connection);
  }
  expect(errors).toEqual([]);
});

test("expired Dex login refresh shows timeout recovery and starts a fresh Control Room login", async ({ page }) => {
  await page.goto("https://zpr-id.localhost:5556/auth/local/login?state=expired");
  await page.reload();
  await expect(page.getByRole("heading", { name: "Sign-in timed out" })).toBeVisible();
  await expect(page.locator("body")).not.toContainText("Bad Request");
  const recovery = page.getByRole("link", { name: "Timed out. Try again.", exact: true });
  await expect(recovery).toHaveAttribute("href", "https://localhost:8787/");
  const freshLogin = page.waitForRequest(request => request.method() === "POST" &&
    request.url() === "https://localhost:8787/auth/operator/login");
  await recovery.click();
  const request = await freshLogin;
  expect(request.headers().origin).toBe("https://localhost:8787");
  await expect(page.locator('input[name="login"]')).toBeVisible();
  expect(new URL(page.url()).searchParams.get("state")).not.toBe("expired");
});

test("deployed local operator signs in through real Dex and authorizes monitoring, catalogs and editor CSRF", async ({ page }) => {
  const directory = process.env.ZPR_LOCAL_OPERATOR_DIR;
  const settings = JSON.parse(readFileSync(`${directory}/stack.json`, "utf8"));
  const password = readFileSync(`${directory}/operator-password`, "utf8").trim();
  const config = JSON.parse(readFileSync(`${directory}/oidc.json`, "utf8"));
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const loginRequest = page.waitForRequest((request) => new URL(request.url()).pathname === "/auth/operator/login");
  await page.goto(settings.origin + "/#map");
  expect((await loginRequest).headers().origin).toBe(settings.origin);
  await expect(page).toHaveURL(/^https:\/\/zpr-id\.localhost:5556\//);
  await page.locator('input[name="login"]').fill(settings.username);
  await page.locator('input[name="password"]').fill("intentionally-wrong-local-password");
  await page.locator("#submit-login").click();
  await expect(page.locator("#login-error")).toBeVisible();
  await page.locator('input[name="password"]').fill(password);
  await page.locator("#submit-login").click();
  await expect(page.locator("#operator-login-status")).toHaveText(/^User(?:: .+)?$/);
  await expect(page.locator("#operator-login-status")).not.toContainText(config.grants[0].subject);
  await expect(page.locator("#operator-scope")).toContainText("Configured organizations: *");
  for (const path of ["/api/snapshot", "/api/policy/context", "/api/policy", "/api/gateways/contracts", "/api/enrollment/v1/catalog"]) {
    expect(await page.evaluate(async (path) => (await fetch(path)).status, path), path).toBe(200);
  }
  const catalog = await page.evaluate(async () => (await fetch("/api/enrollment/v1/catalog")).json());
  expect(catalog.gui_cancel_organizations).toContain("great-lakes");
  expect(catalog.gui_create_organizations).toEqual([]);
  const missingCSRF = await page.evaluate(async () => (await fetch("/api/policy/config/check", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ source: '[visa_service]\ndock_node = "node"\n' }),
  })).status);
  expect(missingCSRF).toBe(403);
  await page.getByRole("link", { name: "Config", exact: true }).click();
  await page.getByLabel("ZPLC configuration source").fill('[visa_service]\ndock_node = "node"\n');
  const analyzed = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/policy/config/check");
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  const response = await analyzed;
  expect(response.status()).toBe(200);
  expect(response.request().headers()["x-zpr-csrf"]).toBeTruthy();
  await expect(page.locator("#zpr-config-status")).toContainText("runtime configuration is unchanged");
  await page.reload();
  await expect(page.locator("#operator-login-status")).toHaveText(/^User(?:: .+)?$/);
  await expect(page.locator("#operator-login-status")).not.toContainText(config.grants[0].subject);
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
  expect(await page.evaluate(async () => (await fetch("/api/snapshot")).status)).toBe(403);
  expect(errors).toEqual([]);
});
