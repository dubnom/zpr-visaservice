import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve, sep } from "node:path";
import { editorCSP } from "./code-editor.mjs";

const origin = "https://control.company.example";
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));

async function registrationFixture(page, context) {
  const registry = { organization_id: "northstar", revision: "initial", durable: true, sources: [] };
  const forbidden = [], operations = [], errors = [];
  let revision = 0;
  page.on("pageerror", error => errors.push(error.message));
  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin || /simulator|\/api\/machines/.test(url.pathname)) {
      forbidden.push(request.url());
      await route.abort("connectionrefused");
      return;
    }
    if (url.pathname === "/auth/operator/config") return route.fulfill({ json: { enabled: false } });
    if (url.pathname === "/api/assertions") return route.fulfill({ json: {
      organization_id: registry.organization_id, organization_name: "Northstar", default_source: registry.sources[0]?.source_id || "",
      sources: registry.sources.map(source => ({ source_id: source.source_id, organization_id: registry.organization_id, display_name: source.config.display_name || source.source_id, kind: source.config.kind })),
    } });
    if (url.pathname === "/api/assertions/sources" && request.method() === "GET") return route.fulfill({ json: registry });
    if (url.pathname.startsWith("/api/assertions/sources")) {
      const body = request.postDataJSON();
      operations.push({ method: request.method(), path: url.pathname, body });
      const config = body.config;
      if (body.organization_id !== registry.organization_id || body.revision !== registry.revision) return route.fulfill({ status: 409, json: { error: "Organization or source configuration changed; reload" } });
      if (url.pathname.endsWith("/test")) {
        if (registry.beforeTest) await registry.beforeTest(body);
        if (!config.ca_file || config.organizations.northstar?.base_dn === "invalid" || config.organizations.northstar?.bind_password_file.endsWith("wrong-bind")) {
          return route.fulfill({ status: 502, json: { error: `Connection test failed for ${body.source_id}; check TLS trust, credentials and organization mapping. No fallback was attempted.` } });
        }
        return route.fulfill({ json: { organization_id: registry.organization_id, source_id: body.source_id, status: "success", message: "Connection test succeeded (read-only); configuration was not saved" } });
      }
      if (body.policy_default) registry.policy_default_source = body.source_id;
      else if (registry.policy_default_source === body.source_id) registry.policy_default_source = "";
      const old = registry.sources.find(source => source.source_id === body.source_id);
      if (old) old.config = config;
      else registry.sources.push({ source_id: body.source_id, editable: true, config });
      registry.revision = `revision-${++revision}`;
      return route.fulfill({ json: { organization_id: registry.organization_id, source_id: body.source_id, revision: registry.revision, policy_default_source: registry.policy_default_source || "", status: "saved", message: "Configuration saved durably; connectivity is not verified by saving" } });
    }
    if (url.pathname === "/api/assertions/source") {
      const sourceID = url.searchParams.get("source");
      return route.fulfill({ json: {
        source_id: sourceID, source_name: sourceID, kind: "ldap", organization_id: registry.organization_id, organization_name: "Northstar", base_dn: "dc=company",
        people: 1, groups: [], attributes: [], directory: { people: ["ordinary-person"], groups: {}, attributes: [], person_attributes: {}, group_attributes: {} },
      } });
    }
    if (url.pathname === "/api/snapshot") return route.fulfill({ json: { api_status: "connected", errors: [], stats: {}, actors: [], services: [], network: [], trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0 } });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ status: 503, json: { error: "Production integration not configured" } });
    const file = resolve(assets, url.pathname === "/" ? "index.html" : url.pathname.slice(1));
    if (!file.startsWith(assets.endsWith(sep) ? assets : assets + sep)) return route.fulfill({ status: 403 });
    try {
      await route.fulfill({ body: await readFile(file), contentType: ({ html: "text/html", js: "application/javascript", css: "text/css", woff2: "font/woff2" })[file.split(".").at(-1)] || "application/octet-stream", headers: { "Content-Security-Policy": editorCSP } });
    } catch { await route.fulfill({ status: 404 }); }
  });
  await page.goto(origin + "/#sources");
  const browser = page.locator("#page-sources trusted-source-browser");
  await expect(browser.getByRole("button", { name: "Register source", exact: true })).toBeVisible();
  return { registry, operations, browser, verify: () => { expect(forbidden).toEqual([]); expect(errors).toEqual([]); } };
}

async function fillLDAP(browser, name) {
  await browser.getByRole("button", { name: "Register source", exact: true }).click();
  const form = browser.locator("[data-registration-form]");
  await expect(form).toBeVisible();
  await form.getByLabel("Source ID", { exact: true }).fill(name);
  await form.getByLabel("Display name", { exact: true }).fill(name === "company_directory" ? "Company directory" : "Independent lab");
  await form.getByLabel("Source URL", { exact: true }).fill(`ldaps://${name}.company.example:636`);
  await form.getByLabel("CA file reference (optional)", { exact: true }).fill("/etc/zpr/provider-ca.pem");
  await form.getByLabel("Approved attributes (comma separated)", { exact: true }).fill("cn,mail");
  await form.getByLabel("Base DN", { exact: true }).fill("dc=company");
  await form.getByLabel("Read-only bind DN", { exact: true }).fill("cn=reader,dc=company");
  await form.getByLabel("Password file reference", { exact: true }).fill("/etc/zpr/provider-password");
  return form;
}

async function visualEvidence(page, browser, testInfo, name) {
  await expect(browser.locator("[data-registration-form]")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  const path = testInfo.outputPath(name.replace(/[^a-zA-Z0-9-]/g, "-") + ".png");
  await browser.locator(".trusted-source-registration").screenshot({ path });
  await testInfo.attach(name, { path, contentType: "image/png" });
}

test("Source registration V02 registers two durable identities and edits without Simulator", async ({ page, context }, testInfo) => {
  const fixture = await registrationFixture(page, context);
  const { browser, registry } = fixture;
  for (const name of ["company_directory", "lab_directory"]) {
    const form = await fillLDAP(browser, name);
    await expect(form.getByLabel("Organization ID", { exact: true })).toHaveValue("northstar");
    await form.getByRole("button", { name: "Test connection (read-only)", exact: true }).click();
    await expect(browser.locator("[data-registration-message]")).toContainText(`${name}: Connection test succeeded`);
    expect(registry.sources).toHaveLength(name === "company_directory" ? 0 : 1);
    await form.getByRole("button", { name: "Save configuration", exact: true }).click();
    await expect(browser.locator("[data-registration-message]")).toContainText("saved durably");
    await expect(form.getByLabel("Source ID", { exact: true })).toHaveAttribute("readonly", "");
  }
  await expect(browser.locator("[data-source-registry]")).toContainText("company_directory");
  await expect(browser.locator("[data-source-registry]")).toContainText("lab_directory");
  await page.reload();
  await expect(browser.locator("[data-source-registry]")).toContainText("lab_directory");
  await browser.getByRole("button", { name: "Edit selected source", exact: true }).click();
  const form = browser.locator("[data-registration-form]");
  await expect(form.getByLabel("Source ID", { exact: true })).toHaveValue("company_directory");
  await expect(form.getByLabel("Source ID", { exact: true })).toHaveAttribute("readonly", "");
  await form.getByLabel("Display name", { exact: true }).fill("Company renamed display only");
  await form.getByRole("button", { name: "Save configuration", exact: true }).click();
  await expect(browser.locator("[data-registration-message]")).toContainText("connectivity is not verified");
  expect(registry.sources.map(source => source.source_id)).toEqual(["company_directory", "lab_directory"]);
  expect(fixture.operations.at(-1).method).toBe("PUT");
  await visualEvidence(page, browser, testInfo, "durable-source-editor");
  fixture.verify();
});

test("Source registration V06 shows separate failures and recovery without leaking credentials", async ({ page, context }, testInfo) => {
  const fixture = await registrationFixture(page, context);
  const { browser } = fixture;
  const form = await fillLDAP(browser, "company_directory");
  for (const [label, invalid, restored] of [
    ["CA file reference (optional)", "", "/etc/zpr/provider-ca.pem"],
    ["Password file reference", "/etc/zpr/wrong-bind", "/etc/zpr/provider-password"],
    ["Base DN", "invalid", "dc=company"],
  ]) {
    await form.getByLabel(label, { exact: true }).fill(invalid);
    await form.getByRole("button", { name: "Test connection (read-only)", exact: true }).click();
    await expect(browser.locator("[data-registration-message]")).toContainText("company_directory: Connection test failed");
    await expect(browser.locator("[data-connection-results]")).toContainText("not verified");
    await expect(browser.locator("[data-connection-results]")).not.toContainText("succeeded");
    await visualEvidence(page, browser, testInfo, `connection-failure-${label}`);
    await form.getByLabel(label, { exact: true }).fill(restored);
  }
  await form.getByRole("button", { name: "Test connection (read-only)", exact: true }).click();
  await expect(browser.locator("[data-registration-message]")).toContainText("company_directory: Connection test succeeded");
  const other = await fillLDAP(browser, "lab_directory");
  await other.getByLabel("CA file reference (optional)", { exact: true }).fill("");
  await other.getByRole("button", { name: "Test connection (read-only)", exact: true }).click();
  await expect(browser.locator("[data-registration-message]")).toContainText("lab_directory: Connection test failed");
  await expect(browser.locator("[data-connection-results]")).toContainText("northstar/company_directory: Connection test succeeded");
  await expect(browser.locator("[data-connection-results]")).toContainText("northstar/lab_directory: Connection test failed");
  expect(fixture.registry.sources).toHaveLength(0);
  expect(fixture.operations.every(operation => operation.path.endsWith("/test"))).toBe(true);
  expect(fixture.operations.every(operation => !JSON.stringify(operation.body).includes('"password":'))).toBe(true);
  await expect(form.locator('input[type="password"]')).toHaveCount(0);
  await visualEvidence(page, browser, testInfo, "source-specific-results");
  fixture.verify();
});

test("Source registration HTTPS uses only token file references and refuses session-only save", async ({ page, context }, testInfo) => {
  const fixture = await registrationFixture(page, context);
  fixture.registry.durable = false;
  const { browser } = fixture;
  await browser.getByRole("button", { name: "Register source", exact: true }).click();
  const form = browser.locator("[data-registration-form]");
  await expect(form).toBeVisible();
  await form.getByLabel("Source kind", { exact: true }).selectOption("https-json");
  await expect(form.getByLabel("Token file reference", { exact: true })).toBeVisible();
  await expect(form.getByLabel("Password file reference", { exact: true })).toBeHidden();
  await expect(form.getByRole("button", { name: "Save configuration", exact: true })).toBeDisabled();
  await expect(browser.locator("[data-registration-storage]")).toContainText("No session-only registrations");
  await visualEvidence(page, browser, testInfo, "https-protected-file-contract");
  fixture.verify();
});


test("Source registration discards delayed connection evidence after organization changes", async ({ page, context }) => {
  const fixture = await registrationFixture(page, context);
  const form = await fillLDAP(fixture.browser, "company_directory");
  let release;
  const held = new Promise(resolve => { release = resolve; });
  fixture.registry.beforeTest = () => held;
  await form.getByRole("button", { name: "Test connection (read-only)", exact: true }).click();
  await expect(fixture.browser.locator("[data-registration-message]")).toContainText("Testing connection");
  await expect.poll(() => fixture.operations.length).toBe(1);
  fixture.registry.organization_id = "other";
  await page.evaluate(() => window.ZPRTrustedSourceSelection.setContext("other", []));
  await expect(form).toBeHidden();
  const response = page.waitForResponse(response => response.url().endsWith("/api/assertions/sources/test"));
  release();
  await response;
  await expect(fixture.browser.locator("[data-registration-message]")).not.toContainText("succeeded");
  await expect(fixture.browser.locator("[data-connection-results]")).toBeEmpty();
  expect(fixture.registry.sources).toHaveLength(0);
  fixture.verify();
});


test("Source registration V03 explicitly designates one durable policy default", async ({ page, context }, testInfo) => {
  const fixture = await registrationFixture(page, context);
  const { browser, registry } = fixture;
  for (const name of ["company_directory", "lab_directory"]) {
    const form = await fillLDAP(browser, name);
    await form.getByLabel("Default source for unqualified policy attributes and groups in this organization").check();
    await form.getByRole("button", { name: "Save configuration", exact: true }).click();
    await expect(browser.locator("[data-registration-message]")).toContainText("saved durably");
    expect(registry.policy_default_source).toBe(name);
  }
  await page.reload();
  await browser.locator("[data-source-selector]").selectOption("lab_directory");
  await browser.getByRole("button", { name: "Edit selected source", exact: true }).click();
  const form = browser.locator("[data-registration-form]");
  await expect(form.getByLabel("Default source for unqualified policy attributes and groups in this organization")).toBeChecked();
  await visualEvidence(page, browser, testInfo, "explicit-policy-default");
  await form.getByLabel("Default source for unqualified policy attributes and groups in this organization").uncheck();
  await form.getByRole("button", { name: "Save configuration", exact: true }).click();
  await expect(browser.locator("[data-registration-message]")).toContainText("saved durably");
  expect(registry.policy_default_source).toBe("");
  expect(fixture.operations.filter(operation => operation.body.policy_default === true)).toHaveLength(2);
  fixture.verify();
});
