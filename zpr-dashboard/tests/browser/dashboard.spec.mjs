async function openAssertionRecord(page, appURL) {
  await page.goto(appURL + "/#policy");
  await expect(page.locator('[data-page-link="assertions"]')).toHaveCount(0);
  await expect(page.locator('[data-category-id="test"]')).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("treeitem", { name: "Assertions", exact: true })).toHaveCount(0);
  const record = page.locator('[data-record-id="test-assertions"]');
  await expect(record).toBeVisible();
  await record.click();
  await expect(page.locator("#policy-assertion-editor")).toBeVisible();
}

function registerAssertionBrowserTests() {
test("organization assertions author, save and evaluate without policy compilation", async ({ page, appURL, api }) => {
  api.handlers.set("/api/assertions", async (route) => {
    if (route.request().method() === "PUT") {
      const request = route.request().postDataJSON();
      expect(request.expected_revision).toBe(api.assertions.settings.revision);
      api.assertions.settings = { revision: request.expected_revision + 1, source: request.source, enabled: request.enabled, interval_seconds: request.interval_seconds };
      const assertionRecord = api.policy.records.find((record) => record.id === "test-assertions");
      assertionRecord.current_revision = request.expected_revision + 1;
      assertionRecord.content = JSON.stringify({ source: request.source, enabled: request.enabled, interval_seconds: request.interval_seconds });
    }
    await route.fulfill({ json: api.assertions });
  });
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    const request = route.request().postDataJSON();
    expect(Object.keys(request).sort()).toEqual(["expected_revision", "source"]);
    api.assertions.last_run = {
      revision: request.expected_revision, draft: request.source !== api.assertions.settings.source,
      status: "pass", finished_at: "2026-10-02T12:00:00Z",
      results: [{ rule: { line: 1, kind: "group", group: "Operators", operator: ">=", limit: 2 }, status: "pass", checked: 1, violations: 0, subjects: [], message: "1 checked; 0 violations" }],
    };
    await route.fulfill({ json: api.assertions.last_run });
  });
  await openAssertionRecord(page, appURL);
  const editor = page.getByRole("textbox", { name: "Data assertion source", exact: true });
  await expect(editor).toBeEnabled();
  await expect(page.locator("#assertion-enabled")).not.toBeChecked();
  expect(await page.getByRole("button", { name: "Read source", exact: true }).evaluate((button) => button.scrollWidth <= button.clientWidth && button.scrollHeight <= button.clientHeight)).toBeTruthy();
  await page.getByRole("button", { name: "Read source", exact: true }).click();
  await expect(page.locator("#assertion-group-rows")).toContainText("Operators");
  await page.getByRole("button", { name: "Insert a cardinality assertion for Operators", exact: true }).click();
  await expect(editor).toHaveValue('group "Operators" members >= 2;\n');
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-run-status")).toContainText("Draft r0");
  const resultGutterStyle = await page.locator("#assertion-result-gutter").evaluate((gutter) => ({
    width: gutter.getBoundingClientRect().width,
    border: getComputedStyle(gutter).borderRightStyle,
    background: getComputedStyle(gutter).backgroundColor,
  }));
  expect(resultGutterStyle.width).toBeLessThan(90);
  expect(resultGutterStyle.border).toBe("solid");
  expect(resultGutterStyle.background).not.toBe("rgba(0, 0, 0, 0)");
  const passMarker = page.locator('#assertion-result-gutter [data-line="1"] .assertion-result-marker');
  await expect(passMarker).toHaveAttribute("data-state", "pass");
  await passMarker.click();
  const passDetails = page.getByRole("dialog", { name: "Assertion pass" });
  await expect(passDetails).toContainText('group "Operators" members >= 2');
  await expect(passDetails).toContainText("1 checked; 0 violations");
  await passDetails.locator(".dialog-actions .button").click();
  expect(api.assertions.settings.source).toBe("");
  await page.locator("#assertion-save").click();
  await expect(page.locator("#assertion-revision")).toHaveText("Alpha Labs / r1");
  expect(api.assertions.settings.enabled).toBe(false);
  await page.locator("#assertion-enabled").check();
  await page.locator("#assertion-interval").fill("300");
  await page.locator("#assertion-save").click();
  await expect(page.locator("#assertion-revision")).toHaveText("Alpha Labs / r2");
  await expect(page.locator('[data-record-id="test-assertions"]')).toContainText("Assertions · r2");
  expect(api.assertions.settings.enabled).toBe(true);
  expect(api.assertions.settings.interval_seconds).toBe(300);
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows")).toContainText("PASS");
  await expect(page.locator("#assertion-run-status")).toContainText("Saved r2");
  expect(api.counts.get("/api/policy/test") || 0).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("assertion attribute catalog inserts rules and renders typed comparisons", async ({ page, appURL, api }) => {
  api.assertionSource.attributes = [{ name: "mail", people: 2, groups: 0 }, { name: "gidnumber", people: 2, groups: 1 }];
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    const request = route.request().postDataJSON();
    expect(Object.keys(request).sort()).toEqual(["expected_revision", "source"]);
    const numeric = request.source.includes("gidNumber");
    api.assertions.last_run = {
      revision: 0, draft: true, status: numeric ? "pass" : "fail", finished_at: "2026-10-02T12:00:00Z",
      results: [{ rule: numeric ? { line: 1, kind: "each_group_attribute", attribute: "gidnumber", operator: ">=", number: 1000 } : { line: 1, kind: "people_attribute", attribute: "mail", operator: "present" }, status: numeric ? "pass" : "fail", checked: 2, violations: numeric ? 0 : 1, subjects: numeric ? [] : ["bob"], message: numeric ? "2 checked; 0 violations" : "2 checked; 1 violations" }],
    };
    await route.fulfill({ json: api.assertions.last_run });
  });
  await openAssertionRecord(page, appURL);
  await expect(page.locator("#assertion-read-source")).toBeEnabled();
  await page.locator("#assertion-read-source").click();
  await page.getByRole("tab", { name: "Attributes", exact: true }).click();
  await expect(page.locator("#assertion-attribute-rows tr")).toHaveCount(2);
  await page.getByRole("button", { name: "Insert presence assertion for mail", exact: true }).click();
  await expect(page.locator("#assertion-source")).toHaveValue('people attribute "mail" present;\n');
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows")).toContainText('people attribute "mail" present');
  await expect(page.locator("#assertion-result-rows")).toContainText("bob");
  const failMarker = page.locator('#assertion-result-gutter [data-line="1"] .assertion-result-marker');
  await expect(failMarker).toHaveAttribute("data-state", "fail");
  await failMarker.click();
  const failDetails = page.getByRole("dialog", { name: "Assertion fail" });
  await expect(failDetails).toContainText("2 checked; 1 violations");
  await expect(failDetails).toContainText("bob");
  await failDetails.locator(".dialog-actions .button").click();
  await page.locator("#assertion-source").fill('each group attribute "gidNumber" >= 1000;');
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows")).toContainText('each group attribute "gidnumber" >= 1000');
  await expect(page.locator("#assertion-run-status")).toContainText("PASS");
  await page.getByRole("tab", { name: "Attributes", exact: true }).focus();
  await page.keyboard.press("ArrowLeft");
  await expect(page.getByRole("tab", { name: "Groups", exact: true })).toHaveAttribute("aria-selected", "true");
  await expect(page.locator("#assertion-attributes-panel")).toBeHidden();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("assertion source failures are errors and never successful checks", async ({ page, appURL, api }) => {
  api.assertions.settings.source = 'group "Operators" members >= 2;';
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    api.assertions.last_run = { revision: 0, draft: false, status: "error", error: "Trusted LDAP read failed; no assertions were evaluated", results: [], finished_at: "2026-10-02T12:00:00Z" };
    await route.fulfill({ json: api.assertions.last_run });
  });
  await openAssertionRecord(page, appURL);
  await expect(page.locator("#assertion-evaluate")).toBeEnabled();
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-run-status")).toContainText("ERROR");
  await expect(page.locator("#assertion-run-error")).toContainText("no assertions were evaluated");
  const errorMarker = page.locator('#assertion-result-gutter [data-line="1"] .assertion-result-marker');
  await expect(errorMarker).toHaveAttribute("data-state", "error");
  await errorMarker.click();
  const errorDetails = page.getByRole("dialog", { name: "Assertion error" });
  await expect(errorDetails).toContainText("Trusted LDAP read failed");
  await errorDetails.locator(".dialog-actions .button").click();
  await expect(page.locator("#assertion-result-rows tr")).toHaveCount(0);
  api.assertions.configured = false;
  await page.locator("#assertion-reload").click();
  await expect(page.locator("#assertion-evaluate")).toBeDisabled();
  await expect(page.locator("#assertion-enabled")).toBeDisabled();
});

test("assertion polling preserves dirty drafts and blocks stale revisions", async ({ page, appURL, api }) => {
  await page.clock.install();
  await openAssertionRecord(page, appURL);
  const editor = page.locator("#assertion-source");
  await expect(editor).toBeEnabled();
  await editor.fill('group "Operators" members > 1;');
  api.assertions.settings = { revision: 1, source: 'group "Operators" members >= 2;', enabled: false, interval_seconds: 60 };
  await page.clock.runFor(5100);
  await expect(editor).toHaveValue('group "Operators" members > 1;');
  await expect(page.locator("#assertion-revision")).toContainText("Reload required");
  await expect(page.locator("#assertion-save")).toBeDisabled();
  await expect(page.locator("#assertion-evaluate")).toBeDisabled();
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator("#assertion-reload").click();
  await expect(editor).toHaveValue('group "Operators" members >= 2;');
  await expect(page.locator("#assertion-revision")).toHaveText("Alpha Labs / r1");
});

test("organization switch reloads that organization's assertions and protects dirty drafts", async ({ page, appURL, api }) => {
  await page.clock.install();
  await openAssertionRecord(page, appURL);
  const editor = page.locator("#assertion-source");
  await editor.fill('group "Local draft" members > 0;');
  api.assertions.organization_id = "beta";
  api.assertions.organization_name = "Beta Labs";
  api.assertions.base_dn = "dc=beta,dc=test";
  api.assertions.settings = { revision: 3, source: 'group "Beta Operators" members >= 2;', enabled: true, interval_seconds: 120 };
  api.assertions.last_run = null;
  await page.clock.runFor(5100);
  await expect(editor).toHaveValue('group "Local draft" members > 0;');
  await expect(page.locator("#assertion-revision")).toHaveText("Beta Labs / r0 / Unsaved / Reload required");
  await expect(page.locator("#assertion-save")).toBeDisabled();
  await expect(page.locator("#assertion-evaluate")).toBeDisabled();
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator("#assertion-reload").click();
  await expect(editor).toHaveValue('group "Beta Operators" members >= 2;');
  await expect(page.locator("#assertion-revision")).toHaveText("Beta Labs / r3");
  await expect(page.locator("#assertion-enabled")).toBeChecked();
});
}
import { test as base, expect } from "@playwright/test";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const csp = "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:";

function logFixture(names, count = 80) {
  return {
    organization_id: "browser-test",
    updated_at: "2026-10-02T12:00:00Z",
    machines: ["machine-first", "machine-second"].map((id) => ({
      machine: { id, model: "Test machine" },
      state: "running",
      sources: names.map((name) => ({
        name,
        lines: Array.from({ length: count }, (entry, index) => `${name} entry ${index}`),
      })),
    })),
  };
}

const test = base.extend({
  appURL: [async ({}, use) => {
    const server = createServer(async (request, response) => {
      const pathname = new URL(request.url, "http://localhost").pathname;
      if (pathname === "/__tests/ldap.html") {
        response.writeHead(200, { "Content-Type": "text/html", "Content-Security-Policy": csp });
        response.end('<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/ldap-org-graph.css"><script src="/ldap-org-graph.js" defer></script></head><body><ldap-org-graph></ldap-org-graph></body></html>');
        return;
      }
      const file = resolve(assets, pathname === "/" ? "index.html" : pathname.slice(1));
      if (!file.startsWith(assets.endsWith(sep) ? assets : assets + sep)) {
        response.writeHead(403).end();
        return;
      }
      try {
        const content = await readFile(file);
        const extension = file.split(".").at(-1);
        response.writeHead(200, {
          "Content-Type": ({ html: "text/html", js: "application/javascript", css: "text/css", woff2: "font/woff2" })[extension] || "application/octet-stream",
          "Content-Security-Policy": csp,
          "Cache-Control": "no-store",
        });
        response.end(content);
      } catch {
        response.writeHead(404).end();
      }
    });
    await new Promise((ready) => server.listen(0, "127.0.0.1", ready));
    try {
      await use(`http://127.0.0.1:${server.address().port}`);
    } finally {
      await new Promise((closed) => server.close(closed));
    }
  }, { scope: "worker" }],
  api: async ({ page }, use) => {
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.addInitScript(() => {
      window.testCSPViolations = [];
      document.addEventListener("securitypolicyviolation", (event) => window.testCSPViolations.push(event.violatedDirective));
    });
    const data = {
      adapterLogs: logFixture(["Controller", "Control adapter", "finance-client adapter"]),
      workloadLogs: logFixture(["finance-client events", "echo-service events"]),
      snapshot: {
        api_status: "connected", errors: [], stats: {}, actors: [], services: [],
        network: [], trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0,
      },
      dnsRecords: { zone: "svc.zpr.", records: [] },
      visas: [{ id: 41, source_addr: "fd00::1", dest_addr: "fd00::2", proto: "TCP", expires: Math.floor(Date.now() / 1000) + 600 }],
      policy: {
        configured: true, organization_id: "alpha", organization_name: "Alpha Labs", compiler_ready: true, tester_ready: true, assistant_ready: false,
        categories: [{ id: "legacy-assertions", name: "Assertions", path: "Assertions" }, { id: "test", name: "Policies", path: "Alpha/Policies" }],
        records: [
          { id: "test-policy", category_id: "test", name: "Test policy", kind: "policy", current_revision: 1, content: "define Employee as user.\n", content_hash: "fixture" },
          { id: "test-assertions", category_id: "legacy-assertions", name: "Organization assertions", kind: "assertions", current_revision: 1, content: '{"source":"","enabled":false,"interval_seconds":60}', content_hash: "fixture" },
        ],
        attributes: [{ attribute: "user.department", source: "LDAP" }, { attribute: "user.title", source: "LDAP" }, { attribute: "device.secure", source: "LDAP" }],
      },
      organizations: {
        active_id: "alpha",
        activation: { state: "idle" },
        organizations: [["alpha", "Alpha Labs"], ["beta", "Beta Labs"], ["gamma", "Gamma Labs"]].map(([id, name]) => ({
          id, name, description: "Test organization", policies: [], services: [],
          directory: { base_dn: `dc=${id},dc=test`, seed_mode: "fixture", departments: [], people: [], groups: [] },
        })),
      },
      assertions: {
        scope: "organization", organization_id: "alpha", organization_name: "Alpha Labs", configured: true, source_kind: "ldap", base_dn: "dc=alpha,dc=test",
        settings: { revision: 0, source: "", enabled: false, interval_seconds: 60 },
        running: false, last_run: null, source_summary: null,
      },
      assertionSource: { observed_at: "2026-10-02T12:00:00Z", people: 2, groups: [{ name: "Operators", members: 2 }] },
      counts: new Map(),
      handlers: new Map(),
      statuses: new Map(),
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      data.counts.set(path, (data.counts.get(path) || 0) + 1);
      if (data.handlers.has(path)) {
        await data.handlers.get(path)(route);
        return;
      }
      const bodies = {
        "/api/adapter-logs": data.adapterLogs,
        "/api/simulator/machine-logs": data.workloadLogs,
        "/api/snapshot": data.snapshot,
        "/api/dns/records": data.dnsRecords,
        "/api/actors/adapter/visas": data.visas,
        "/api/policy": data.policy,
        "/api/policy/records/test-policy": data.policy.records[0],
        "/api/policy/records/test-policy/revisions": [],
        "/api/policy/records/test-assertions": data.policy.records[1],
        "/api/policy/records/test-assertions/revisions": [],
        "/api/simulator/organizations": data.organizations,
        "/api/assertions": data.assertions,
        "/api/assertions/source": data.assertionSource,
      };
      if (!Object.hasOwn(bodies, path)) {
        await route.fulfill({ status: 503, json: { error: "Not configured in this test" } });
        return;
      }
      await route.fulfill({ status: data.statuses.get(path) || 200, json: bodies[path] });
    });
    await use(data);
    expect(errors, "Unexpected browser JavaScript errors").toEqual([]);
    expect(await page.evaluate(() => window.testCSPViolations || []), "Unexpected CSP violations").toEqual([]);
  },
});

registerAssertionBrowserTests();

test("read-only browser filters policy and assertion records and shows saved revisions", async ({ page, appURL, api }, testInfo) => {
  const methods = [];
  page.on("request", (request) => { if (new URL(request.url()).pathname.startsWith("/api/")) methods.push(request.method()); });
  api.policy.organization_id = "alpha";
  api.policy.organization_name = "Alpha Labs";
  api.policy.records[0].current_revision = 2;
  api.policy.records[0].content = "define Employee as user.\nprovide Payroll at payroll.svc.zpr over TCP 443.\nallow Employee.\n";
  api.policy.records[1].content = JSON.stringify({ source: 'group "Operators" members >= 2;\n// <img src=x onerror=alert(1)>', enabled: true, interval_seconds: 120 });
  api.handlers.set("/api/policy/records/test-policy/revisions", async (route) => route.fulfill({ json: [
    { number: 2, summary: "Payroll access", author: "operator" }, { number: 1, summary: "Initial definition", author: "operator" },
  ] }));
  api.handlers.set("/api/policy/records/test-policy/revisions/1", async (route) => route.fulfill({ json: {
    number: 1, content: "define Employee as user.\n", summary: "Initial definition", author: "operator",
  } }));
  await page.goto(appURL + "/policy-browser.html");
  const viewer = page.locator("zpr-policy-browser");
  await expect(viewer.locator(".pb-organization")).toHaveText("Alpha Labs");
  await expect(viewer.locator(".pb-record")).toHaveCount(2);
  await viewer.locator('[data-record-id="test-policy"]').click();
  await expect(viewer.locator(".pb-source")).toContainText("allow Employee.");
  await viewer.getByLabel("Revision", { exact: true }).selectOption("1");
  await expect(viewer.locator(".pb-source")).toHaveText("define Employee as user.\n");
  await expect(viewer.locator(".pb-meta")).toContainText("Initial definition");
  await viewer.getByLabel("Type", { exact: true }).selectOption("assertions");
  await expect(viewer.locator(".pb-record")).toHaveCount(1);
  await viewer.locator('[data-record-id="test-assertions"]').click();
  await expect(viewer.locator(".pb-source")).toContainText('group "Operators" members >= 2;');
  await expect(viewer.locator(".pb-schedule")).toHaveText("Periodic checks: Enabled / Interval: 120 seconds");
  await expect(viewer.locator("img, textarea, [contenteditable=true]")).toHaveCount(0);
  await viewer.getByLabel("Search", { exact: true }).fill("missing");
  await expect(viewer.locator(".pb-records")).toHaveText("No matching records");
  await viewer.getByLabel("Search", { exact: true }).fill("Operators");
  await expect(viewer.locator(".pb-record")).toHaveCount(0);
  await viewer.getByLabel("Search", { exact: true }).fill("Organization");
  await expect(viewer.locator(".pb-record")).toHaveCount(1);
  await viewer.getByLabel("Category", { exact: true }).selectOption("test");
  await viewer.getByLabel("Wrap lines", { exact: true }).uncheck();
  await expect(viewer.locator(".pb-source")).toHaveAttribute("data-wrap", "false");
  expect(methods.length).toBeGreaterThan(0);
  expect(methods.every((method) => method === "GET")).toBeTruthy();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath("policy-browser.png"), fullPage: true });
});

test("read-only browser keeps last good source on failures and clears it on an organization switch", async ({ page, appURL, api }) => {
  api.policy.organization_id = "alpha";
  api.policy.organization_name = "Alpha Labs";
  await page.goto(appURL + "/policy-browser.html");
  const viewer = page.locator("zpr-policy-browser");
  await viewer.locator('[data-record-id="test-policy"]').click();
  await expect(viewer.locator(".pb-source")).toContainText("define Employee as user.");
  api.statuses.set("/api/policy", 503);
  await viewer.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(viewer.locator(".pb-error")).not.toBeEmpty();
  await expect(viewer.locator(".pb-source")).toContainText("define Employee as user.");
  api.statuses.delete("/api/policy");
  api.policy.organization_id = "beta";
  api.policy.organization_name = "Beta Labs";
  api.policy.records = [];
  await viewer.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(viewer.locator(".pb-organization")).toHaveText("Beta Labs");
  await expect(viewer.locator(".pb-source code")).toBeEmpty();
  await expect(viewer.locator(".pb-title")).toHaveText("Select a record");
});

test("read-only browser can be embedded twice with independent filters and API bases", async ({ page, appURL, api }) => {
  api.handlers.set("/api/archive", async (route) => route.fulfill({ json: { ...api.policy, organization_name: "Archive" } }));
  api.handlers.set("/api/archive/records/test-policy", async (route) => route.fulfill({ json: api.policy.records[0] }));
  api.handlers.set("/api/archive/records/test-policy/revisions", async (route) => route.fulfill({ json: [] }));
  await page.goto(appURL + "/policy-browser.html");
  await page.evaluate(() => {
    const viewer = document.createElement("zpr-policy-browser");
    viewer.setAttribute("api-base", "/api/archive");
    document.querySelector("main").append(viewer);
  });
  const viewers = page.locator("zpr-policy-browser");
  await expect(viewers.nth(0).locator(".pb-record")).toHaveCount(2);
  await expect(viewers.nth(1).locator(".pb-record")).toHaveCount(2);
  await expect(viewers.nth(1).locator(".pb-organization")).toHaveText("Archive");
  await viewers.nth(0).getByLabel("Type", { exact: true }).selectOption("assertions");
  await expect(viewers.nth(0).locator(".pb-record")).toHaveCount(1);
  await expect(viewers.nth(1).locator(".pb-record")).toHaveCount(2);
  await viewers.nth(1).locator('[data-record-id="test-policy"]').click();
  await expect(viewers.nth(1).locator(".pb-source")).toContainText("define Employee as user.");
  await expect(viewers.nth(0).locator(".pb-title")).toHaveText("Select a record");
});

test("policy Test opens per-line counts and matching identities in the editor modal", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/test/fixtures", async (route) => {
    await route.fulfill({ json: {
      actors: [
        { id: "alice", label: "Alice Rivera", kind: "user", dimensions: { user: "alice" }, attributes: [{ key: "user.zpr.authority", values: ["demo"] }] },
        { id: "alice-machine", label: "Alice Rivera on machine-1", kind: "user_device", dimensions: { user: "alice", device: "machine-1" }, attributes: [{ key: "device.zpr.authority", values: ["zpr-bootstrap"] }] },
      ],
      services: [{ id: "EchoWeb", name: "EchoWeb", protocol: "TCP", port: 8080, attributes: [] }],
      warnings: [],
    } });
  });
  api.handlers.set("/api/policy/test", async (route) => {
    const request = route.request().postDataJSON();
    expect(Object.keys(request).sort()).toEqual(["actors", "services", "source"]);
    expect(request.source).toBe("define Employee as user.\n");
    await route.fulfill({ json: {
      api_version: 1, source_sha256: "fixture", tested_at: "2026-10-03T12:00:00Z", actor_count: 2,
      services: [{
        id: "EchoWeb", name: "EchoWeb", protocol: "TCP", port: 8080, supported: true, evaluated_actors: 2,
        allowed: { count: 1, by_kind: { user_device: 1 }, by_dimension: { user: 1, device: 1 }, subjects: [{ id: "alice-machine", label: "Alice Rivera on machine-1", kind: "user_device", dimensions: { user: "alice", device: "machine-1" } }] },
        denied: { count: 1, by_kind: { user: 1 }, by_dimension: { user: 1 }, subjects: [{ id: "alice", label: "Alice Rivera", kind: "user", dimensions: { user: "alice" } }] },
        default_denied: { count: 0, by_kind: {}, by_dimension: {}, subjects: [] },
        rules: [
          { indexes: [2], line: 2, source: "allow users to access EchoWeb", effect: "allow", matched: { count: 1, by_kind: { user_device: 1 }, by_dimension: { user: 1, device: 1 }, subjects: [{ id: "alice-machine", label: "Alice Rivera on machine-1", kind: "user_device", dimensions: { user: "alice", device: "machine-1" } }] } },
          { indexes: [3], line: 3, source: "never allow users to access EchoWeb", effect: "deny", matched: { count: 1, by_kind: { user: 1 }, by_dimension: { user: 1 }, subjects: [{ id: "alice", label: "Alice Rivera", kind: "user", dimensions: { user: "alice" } }] } },
        ],
      }],
    } });
  });
  await page.goto(appURL + "/#policy");
  await expect(page.locator("#policy-test")).toBeEnabled();
  await page.locator("#policy-test").click();
  await expect(page.locator("#policy-test")).toHaveText("Exit test");
  await expect(page.locator("#policy-test")).toBeVisible();
  await expect(page.locator("#policy-test")).toBeEnabled();
  await expect(page.locator("#policy-source")).toBeEnabled();
  await expect(page.locator("#policy-source")).not.toBeEditable();
  await expect(page.locator(".policy-catalog-pane")).toBeHidden();
  await expect(page.locator("#policy-test-gutter")).toBeVisible();
  const lineResult = page.locator('#policy-test-gutter [data-line="2"] .policy-test-line-result');
  await expect(lineResult).toContainText("U1");
  await expect(lineResult).toContainText("D1");
  await lineResult.click();
  const dialog = page.getByRole("dialog", { name: "Policy test" });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("#policy-test-subjects")).toContainText("Alice Rivera on machine-1");
  await expect(dialog.locator("#policy-test-subject-title")).toContainText("User 1");
  await expect(dialog.locator("#policy-test-subject-title")).toContainText("Device 1");
  await expect(dialog.locator("#policy-test-subjects")).toContainText("Device: machine-1");
  await page.evaluate(() => showPolicyTestSubjects(Array.from({ length: 105 }, (_, index) => ({ id: `member-${index}`, label: `Member ${index}`, dimensions: { device: `device-${index}` } })), "", "Device matches"));
  await expect(dialog.locator(".policy-test-subject")).toHaveCount(100);
  await expect(dialog.locator("#policy-test-subject-title")).toContainText("105 members · First 100 shown");
  await expect(dialog.locator(".policy-test-subject").last()).toContainText("Member 99");
  await dialog.locator(".dialog-actions .button").click();
  await page.locator("#policy-test").click();
  await expect(page.locator("#policy-test")).toHaveText("Test");
  await expect(page.locator("#policy-source")).toBeEnabled();
  await expect(page.locator(".policy-catalog-pane")).toBeVisible();
  await expect(page.locator("#policy-test-gutter")).toBeHidden();
  if ((page.viewportSize()?.width || 1000) <= 600) {
    const gridColumns = await page.locator("#policy-workbench").evaluate((element) => getComputedStyle(element).gridTemplateColumns.trim().split(/\s+/).length);
    expect(gridColumns).toBe(1);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

for (const view of [
  { name: "Adapter Logs", path: "/#adapter-logs", names: ["Controller", "Control adapter", "finance-client adapter"] },
  { name: "Workload logs", path: "/machine-logs.html", names: ["finance-client events", "echo-service events"] },
]) {
  test(`${view.name} supports independent radio source selection`, async ({ page, appURL, api }) => {
    await page.goto(appURL + view.path);
    const panels = page.locator(".machine-log-panel");
    await expect(panels).toHaveCount(2);
    const first = panels.nth(0);
    const second = panels.nth(1);
    await expect(first.getByRole("radio")).toHaveCount(view.names.length);
    await first.getByRole("radio", { name: view.names[1], exact: true }).check();
    await expect(first.locator(".machine-log-output")).toContainText(`${view.names[1]} entry 79`);
    await expect(first.locator(".machine-log-output")).not.toContainText(`${view.names[0]} entry`);
    await expect(second.getByRole("radio", { name: view.names[0], exact: true })).toBeChecked();
    await first.getByRole("button", { name: /Maximize/ }).click();
    await expect(first).toHaveClass(/maximized/);
    await page.keyboard.press("Escape");
    await expect(first).not.toHaveClass(/maximized/);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  });
}

async function refreshLogs(page) {
  const button = page.locator("#machine-logs-refresh");
  await button.click();
  await expect(button).toBeEnabled();
}

async function expectAtBottom(output) {
  await expect.poll(() => output.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop)).toBeLessThanOrEqual(8);
}

test("log sources remember reader positions and follow independently", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#adapter-logs");
  const panels = page.locator(".machine-log-panel");
  await expect(panels).toHaveCount(2);
  await page.locator("#machine-logs-pause").click();
  const first = panels.nth(0);
  const output = first.locator(".machine-log-output");
  const secondOutput = panels.nth(1).locator(".machine-log-output");
  await expectAtBottom(output);
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  api.adapterLogs = logFixture(["Controller", "Control adapter"], 95);
  await refreshLogs(page);
  await expect(output).toContainText("Controller entry 94");
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  await expectAtBottom(secondOutput);
  await first.getByRole("radio", { name: "Control adapter", exact: true }).check();
  await expectAtBottom(output);
  await first.getByRole("radio", { name: "Controller", exact: true }).check();
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  await output.evaluate((element) => { element.scrollTop = element.scrollHeight; element.dispatchEvent(new Event("scroll")); });
  api.adapterLogs = logFixture(["Controller", "Control adapter"], 100);
  await refreshLogs(page);
  await expect(output).toContainText("Controller entry 99");
  await expectAtBottom(output);
  await page.locator("#machine-logs-follow").uncheck();
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  await page.locator("#machine-logs-follow").check();
  await expectAtBottom(output);
});

test("log rendering preserves ANSI colors without interpreting HTML", async ({ page, appURL, api }) => {
  api.adapterLogs.machines[0].sources[0].lines = [
    "\u001b[31mred\u001b[0m plain",
    "\u001b[38;2;10;20;30mRGB\u001b[0m",
    "<img src=x onerror=window.logInjected=true>",
    "\u001b]8;;https://example.com\u0007link label\u001b]8;;\u0007",
  ];
  await page.goto(appURL + "/#adapter-logs");
  const output = page.locator(".machine-log-output").first();
  await expect(output).toContainText("red plain");
  await expect(output.locator("span").filter({ hasText: /^red$/ })).toHaveCSS("color", "rgb(187, 0, 0)");
  await expect(output.locator("span").filter({ hasText: /^RGB$/ })).toHaveCSS("color", "rgb(10, 20, 30)");
  await expect(output).toContainText("<img src=x onerror=window.logInjected=true>");
  await expect(output).toContainText("link label");
  await expect(output.locator("img, a")).toHaveCount(0);
  expect(await page.evaluate(() => Boolean(window.logInjected))).toBeFalsy();
});

test("log errors, empty workloads and removed sources are explicit", async ({ page, appURL, api }) => {
  api.workloadLogs.machines[1].sources = [];
  await page.goto(appURL + "/machine-logs.html");
  const panels = page.locator(".machine-log-panel");
  await expect(panels).toHaveCount(2);
  await page.locator("#machine-logs-pause").click();
  await expect(panels.nth(1)).toContainText("No application/service logs assigned.");
  const first = panels.nth(0);
  await first.getByRole("radio", { name: "echo-service events", exact: true }).check();
  api.workloadLogs.machines[0].sources[1] = { name: "echo-service events", lines: [], error: "Log source unavailable" };
  await refreshLogs(page);
  await expect(first.locator(".machine-log-error")).toHaveText("Log source unavailable");
  api.workloadLogs.machines[0].sources.splice(1, 1);
  await refreshLogs(page);
  await expect(first.getByRole("radio", { name: "finance-client events", exact: true })).toBeChecked();
  await expect(first.getByRole("radio")).toHaveCount(1);
  api.statuses.set("/api/simulator/machine-logs", 503);
  await refreshLogs(page);
  await expect(page.locator("#machine-logs-error")).toBeVisible();
  await expect(first.locator(".machine-log-output")).toContainText("finance-client events entry 79");
});

test("Adapter Logs polls only while its Control Room page is active", async ({ page, appURL, api }) => {
  await page.clock.install();
  await page.goto(appURL + "/#services");
  await expect(page.locator("#page-services")).toBeVisible();
  expect(api.counts.get("/api/adapter-logs") || 0).toBe(0);
  await page.getByRole("link", { name: "Adapter Logs", exact: true }).click();
  await expect(page.locator(".machine-log-panel")).toHaveCount(2);
  await page.clock.runFor(2100);
  await expect.poll(() => api.counts.get("/api/adapter-logs")).toBeGreaterThanOrEqual(2);
  await page.getByRole("link", { name: "Services", exact: true }).click();
  await expect(page.locator("#page-services")).toBeVisible();
  await expect(page.locator("#page-adapter-logs")).toBeHidden();
  const before = api.counts.get("/api/adapter-logs");
  await page.clock.runFor(6000);
  expect(api.counts.get("/api/adapter-logs")).toBe(before);
  await page.getByRole("link", { name: "Adapter Logs", exact: true }).click();
  await expect.poll(() => api.counts.get("/api/adapter-logs")).toBeGreaterThan(before);
});

test("policy completions respect statements, define attributes and punctuation", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await page.locator('[data-record-id="test-policy"]').click();
  const editor = page.locator("#policy-source");
  const menu = page.locator("#policy-completions");
  await expect(editor).toBeEnabled();
  await editor.fill("define Employee as user.");
  await editor.press("End");
  await editor.click();
  await expect(menu).toBeHidden();
  await editor.fill("# define Employee as user with ");
  await expect(menu).toBeHidden();
  await editor.fill('define Employee as user with user.title:"unfinished');
  await expect(menu).toBeHidden();
  await editor.fill("define Employee as user\n  with dep");
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("option").filter({ hasText: "user.department:" })).toBeVisible();
  await expect(menu).not.toContainText("device.secure");
  await editor.press("Tab");
  await expect(editor).toHaveValue("define Employee as user\n  with user.department:");
  await editor.fill("define Employee as user with user.");
  await expect(menu).toBeVisible();
  await editor.press("Escape");
  await expect(menu).toBeHidden();
  await editor.fill("define Employee as u.");
  await editor.press("ArrowLeft");
  await expect(menu.getByRole("option").filter({ hasText: /^user$/ })).toBeVisible();
  await editor.press("Tab");
  await expect(editor).toHaveValue("define Employee as user.");
  await editor.fill('define SignalService as service.\nallow users and signal "hello" to ');
  await expect(menu.getByRole("option", { name: "SignalService", exact: true })).toBeVisible();
  await editor.fill("define Employee as user.\nallow Employee to ");
  await expect(menu.getByRole("option", { name: "access", exact: true })).toHaveCount(0);
});

test("policy compiler errors highlight their source token", async ({ page, appURL, api }) => {
  let diagnostics = "error: unexpected tab char at line 2, column 1";
  api.handlers.set("/api/policy/check", async (route) => {
    await route.fulfill({ json: { valid: false, diagnostics } });
  });
  await page.goto(appURL + "/#policy");
  await page.locator('[data-record-id="test-policy"]').click();
  const editor = page.locator("#policy-source");
  await expect(editor).toHaveValue("define Employee as user.\n");
  await editor.fill("define Employee as user.\n\tallow Employee.");
  await expect(editor).toHaveValue("define Employee as user.\n\tallow Employee.");
  await expect(page.locator("#policy-highlight")).toContainText("allow");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-highlight .zpl-error")).toHaveText("allow");

  diagnostics = "error: [ line 2, column 1 ] explicit service targets are rejected";
  await editor.fill("define Employee as user.\nallow Employee to access Payroll.");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-highlight .zpl-error")).toHaveText("allow");
});

test("service types share table and map colors and gateways have clouds", async ({ page, appURL, api }) => {
  const kinds = ["BuiltIn", "Regular", "Visa", "Gateway", 'Trusted("file")', 'Trusted("rest/1")', null, 'Trusted("custom")'];
  api.snapshot.actors = [{ cn: "adapter", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.services = kinds.map((kind, index) => ({ service_name: `service-${index}`, service_kind: kind, actor_cn: "adapter", zpr_addr: "fd00::1", service_endpoints: "tcp:8080" }));
  await page.goto(appURL + "/#services");
  const chips = page.locator(".service-type-chip");
  await expect(chips).toHaveCount(kinds.length);
  const colors = await chips.evaluateAll((elements) => Object.fromEntries(elements.map((element) => [element.dataset.serviceType, {
    fill: getComputedStyle(element).backgroundColor,
    stroke: getComputedStyle(element).borderColor,
    text: getComputedStyle(element).color,
  }])));
  expect(new Set(Object.values(colors).map((color) => color.fill)).size).toBe(kinds.length);
  await page.getByText("Type colors", { exact: true }).click();
  await expect(page.locator(".service-type-legend-item")).toHaveCount(kinds.length);
  await page.keyboard.press("Escape");
  await expect(page.locator(".service-type-key")).not.toHaveAttribute("open", "");
  await page.getByRole("link", { name: "Map", exact: true }).click();
  await expect(page.locator("#graph-animation-toggle, .graph-hint")).toHaveCount(0);
  const badges = page.locator(".graph-service-badge");
  await expect(badges).toHaveCount(kinds.length);
  const readMapColors = () => badges.evaluateAll((elements) => Object.fromEntries(elements.map((element) => [element.dataset.serviceType, {
    fill: getComputedStyle(element.querySelector("rect")).fill,
    stroke: getComputedStyle(element.querySelector("rect")).stroke,
    text: getComputedStyle(element.querySelector("text")).fill,
  }])));
  expect(await readMapColors()).toEqual(colors);
  await badges.first().focus();
  await badges.first().hover();
  expect(await readMapColors()).toEqual(colors);
  await badges.evaluateAll((elements) => elements.forEach((element) => element.classList.add("arriving")));
  expect(await readMapColors()).toEqual(colors);
  await badges.evaluateAll((elements) => elements.forEach((element) => element.classList.replace("arriving", "graph-exiting")));
  expect(await readMapColors()).toEqual(colors);
  await expect(page.locator(".graph-cloud")).toHaveCount(1);
  await expect(page.locator(".graph-cloud")).toHaveCSS("fill", "rgb(69, 69, 69)");
  await expect(page.locator(".gateway-cloud-link")).toHaveCount(1);
});

test("adding topology parents keeps existing nodes at the same screen position", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  const parent = (index) => ({ cn: `node-${index}`, node: true, zpr_addr: `fd00::${index + 1}`, node_details: { adapters: [], in_sync: true } });
  api.snapshot.actors = [0, 1, 2, 3].map(parent);
  await page.goto(appURL + "/#map");
  const existing = page.locator('.graph-vertex.node[data-inspect-actor="node-0"]');
  await expect(existing).toBeVisible();
  const before = await existing.boundingBox();

  api.snapshot.actors.push(parent(4));
  await page.locator("#refresh-now").click();
  await expect(page.locator('.graph-vertex.node[data-inspect-actor="node-4"]')).toBeVisible();
  const graph = page.locator(".topology-graph");
  expect(await graph.evaluate((svg) => svg.getAnimations().some((candidate) => candidate.effect?.target === svg))).toBeTruthy();
  await graph.evaluate((svg) => { const candidate = svg.getAnimations().find((animation) => animation.effect?.target === svg); candidate.pause(); candidate.currentTime = 0; });
  const atStart = await page.locator('.graph-vertex.node[data-inspect-actor="node-0"]').boundingBox();
  expect(atStart.x).toBeCloseTo(before.x, 1);
  expect(atStart.y).toBeCloseTo(before.y, 1);
  await graph.evaluate((svg) => { svg.getAnimations().find((animation) => animation.effect?.target === svg).currentTime = 350; });
  const midway = await page.locator('.graph-vertex.node[data-inspect-actor="node-0"]').boundingBox();
  await graph.evaluate((svg) => { svg.getAnimations().find((animation) => animation.effect?.target === svg).finish(); });
  const afterNode = page.locator('.graph-vertex.node[data-inspect-actor="node-0"]');
  const after = await afterNode.boundingBox();
  expect(Math.abs(midway.x - before.x)).toBeGreaterThan(1);
  expect(Math.abs(midway.x - before.x)).toBeLessThan(Math.abs(after.x - before.x));
  expect(after.x).not.toBeCloseTo(before.x, 1);
});

test("visa refresh preserves current grants and resolves DNS labels", async ({ page, appURL, api }) => {
  await page.clock.install();
  api.snapshot.actors = [{ cn: "adapter", node: false, zpr_addr: "fd00::1" }];
  api.dnsRecords.records = [{ name: "api.svc.zpr.", type: "AAAA", value: "fd00:0:0:0:0:0:0:2" }];
  await page.goto(appURL + "/#actors");
  await page.locator('#actor-rows [data-inspect-actor="adapter"]').click();
  const inspector = page.locator("#inspector-body");
  await expect(inspector).toContainText("Visa 41");
  await expect(inspector).toContainText("api.svc.zpr");
  await expect(inspector.locator('[title*="fd00::2"]')).not.toHaveCount(0);
  const existingRow = await inspector.locator(".detail-item").first().elementHandle();
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  api.handlers.set("/api/actors/adapter/visas", async (route) => {
    await gate;
    await route.fulfill({ status: 503, json: { error: "Unavailable" } });
  });
  const before = api.counts.get("/api/actors/adapter/visas");
  try {
    await page.clock.runFor(5100);
    await expect.poll(() => api.counts.get("/api/actors/adapter/visas")).toBeGreaterThan(before);
    await expect(inspector).toContainText("Visa 41");
    await expect(inspector).not.toContainText("Loading current visas");
    expect(await existingRow.evaluate((element) => element.isConnected)).toBeTruthy();
  } finally {
    release();
  }
  await expect(inspector).toContainText("showing last successful result");
  await expect(inspector).toContainText("Visa 41");
});

test("LDAP nodes show popup attributes and preserve memberships across collapse", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/__tests/ldap.html");
  await page.evaluate(() => {
    const graph = document.querySelector("ldap-org-graph");
    graph.directory = {
      base_dn: "dc=example,dc=test",
      departments: [{ name: "Engineering" }, { name: "Platform", parent: "Engineering" }],
      people: [{ uid: "ada", name: "Ada", department: "Platform", attributes: { clearance: "high" } }],
      groups: [{ name: "Operators", members: ["ada"] }],
    };
  });
  const graph = page.locator("ldap-org-graph");
  const person = graph.getByRole("button", { name: "person: Ada", exact: true });
  await expect(graph.locator("dialog")).not.toBeVisible();
  await person.click();
  const dialog = graph.getByRole("dialog", { name: "Ada component info", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Operators");
  await expect(dialog).toContainText("attributes.clearance");
  await expect(dialog).toContainText("high");
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await graph.getByRole("button", { name: "Collapse Engineering", exact: true }).click();
  await expect(person).toHaveCount(0);
  await expect(graph.locator("dialog")).not.toBeVisible();
  await graph.getByRole("button", { name: "Expand Engineering", exact: true }).click();
  await expect(person).toBeVisible();
  await person.click();
  await expect(dialog).toContainText("Operators");
  await graph.getByRole("button", { name: "Close component info", exact: true }).click();
  await expect(person).toBeFocused();
});

test("organization activation requires explicit approval and is cancel-safe", async ({ page, appURL, api }) => {
  const activationPath = "/api/simulator/organizations/beta/activate";
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  api.handlers.set(activationPath, async (route) => {
    expect(route.request().method()).toBe("POST");
    await gate;
    api.organizations.active_id = "beta";
    api.organizations.activation = { state: "completed", organization_id: "beta" };
    await route.fulfill({ status: 202, json: { activation: api.organizations.activation } });
  });
  await page.goto(appURL + "/organizations.html");
  await page.locator('[data-organization-id="beta"]').click();
  const activate = page.locator('[data-activate-organization="beta"]');
  await activate.click();
  const dialog = page.getByRole("dialog", { name: "Switch organization?", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Switch from Alpha Labs to Beta Labs?");
  await expect(dialog).toContainText("resets the simulated ZPR environment");
  await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
  expect(api.counts.get(activationPath) || 0).toBe(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator("#organization-active-name")).toHaveText("Alpha Labs");
  expect(api.counts.get(activationPath) || 0).toBe(0);
  await activate.click();
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  expect(api.counts.get(activationPath) || 0).toBe(0);
  await activate.click();
  try {
    await dialog.getByRole("button", { name: "Switch organization", exact: true }).click();
    await expect.poll(() => api.counts.get(activationPath)).toBe(1);
    await expect(activate).toBeDisabled();
    await activate.dispatchEvent("click");
    expect(api.counts.get(activationPath)).toBe(1);
  } finally {
    release();
  }
  await expect(page.locator("#organization-active-name")).toHaveText("Beta Labs");
  await expect(dialog).not.toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("organization approval is rejected after the active organization changes", async ({ page, appURL, api }) => {
  await page.clock.install();
  await page.goto(appURL + "/organizations.html");
  await page.locator('[data-organization-id="beta"]').click();
  await page.locator('[data-activate-organization="beta"]').click();
  const dialog = page.getByRole("dialog", { name: "Switch organization?", exact: true });
  await expect(dialog).toBeVisible();
  api.organizations.active_id = "gamma";
  await page.clock.runFor(5100);
  await expect(page.locator("#organization-active-name")).toHaveText("Gamma Labs");
  await dialog.getByRole("button", { name: "Switch organization", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("organization state changed");
  await expect(dialog.getByRole("button", { name: "Switch organization", exact: true })).toBeDisabled();
  expect(api.counts.get("/api/simulator/organizations/beta/activate") || 0).toBe(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).not.toBeVisible();
});

test("scenario machine lanes scroll from a scrollbar at the top of the panel", async ({ page, appURL }) => {
  await page.goto(appURL + "/scenarios.html");
  await page.evaluate(() => renderScenarioRun({
    scenario_id: "many-machines",
    scenario_name: "Many machines",
    state: "running",
    total_steps: 8,
    current_step: 1,
    steps: [],
    scenario: {
      steps: Array.from({ length: 8 }, (_, index) => ({
        action: "wait_controller",
        machine: `machine-${index + 1}`,
      })),
      cleanup: [],
    },
  }));

  const frame = page.locator(".scenario-track-frame");
  const scrollbar = frame.locator(".scenario-track-scrollbar");
  const viewport = frame.locator(".scenario-track-viewport");
  await expect(scrollbar).toBeVisible();
  await expect(viewport).toHaveCSS("scrollbar-width", "none");
  const scrollbarBox = await scrollbar.boundingBox();
  const viewportBox = await viewport.boundingBox();
  expect(scrollbarBox.y).toBeLessThan(viewportBox.y);

  await scrollbar.evaluate((element) => { element.scrollLeft = element.scrollWidth; });
  await expect.poll(() => viewport.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  await viewport.evaluate((element) => { element.scrollLeft = 0; });
  await expect.poll(() => scrollbar.evaluate((element) => element.scrollLeft)).toBe(0);
});