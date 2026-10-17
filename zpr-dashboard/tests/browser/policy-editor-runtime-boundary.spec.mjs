import { test, expect } from "@playwright/test";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const staticRoot = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const csp = "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:";
let server;
let appURL;

test.beforeAll(async () => {
  server = createServer(async (request, response) => {
    const pathname = new URL(request.url, "http://localhost").pathname;
    const file = resolve(staticRoot, pathname === "/" ? "index.html" : pathname.slice(1));
    if (!file.startsWith(staticRoot.endsWith(sep) ? staticRoot : staticRoot + sep)) {
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
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
      response.writeHead(404).end();
    }
  });
  await new Promise((ready, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", ready);
  });
  appURL = `http://127.0.0.1:${server.address().port}`;
});

test.afterAll(async () => {
  if (server?.listening) await new Promise((closed) => server.close(closed));
});

test("synthetic Control Room UI keeps policy revision save separate from runtime changes", async ({ page }) => {
  const policyRecord = {
    id: "test-policy",
    category_id: "policies",
    name: "Network policy",
    kind: "policy",
    current_revision: 1,
    content: "define Employee as user.\n",
    content_hash: "revision-1",
  };
  const revisions = [{ number: 1, summary: "Initial revision", author: "fixture" }];
  const apiCalls = [];
  await page.route(`${appURL}/auth/operator/config`, (route) => route.fulfill({ json: { enabled: false } }));
  await page.route(`${appURL}/api/**`, async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const call = { method: request.method(), path };
    if (request.method() !== "GET") call.body = request.postDataJSON();
    apiCalls.push(call);

    if (path.startsWith("/api/simulator/")) {
      await route.fulfill({ status: 503, json: { error: "Simulator is unavailable in this synthetic test." } });
      return;
    }
    if (path === "/api/snapshot") {
      await route.fulfill({ json: {
        api_status: "connected", errors: [], stats: {}, actors: [], services: [],
        network: [], trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0,
      } });
      return;
    }
    if (path === "/api/policy") {
      await route.fulfill({ json: {
        configured: true,
        organization_id: "alpha",
        organization_name: "Alpha Labs",
        compiler_ready: true,
        tester_ready: true,
        staging_ready: true,
        assistant_ready: false,
        categories: [{ id: "policies", name: "Policies", path: "Alpha/Policies" }],
        records: [policyRecord],
        attributes: [],
      } });
      return;
    }
    if (path === "/api/policy/records/test-policy") {
      await route.fulfill({ json: policyRecord });
      return;
    }
    if (path === "/api/policy/records/test-policy/revisions") {
      if (request.method() === "GET") {
        await route.fulfill({ json: revisions });
        return;
      }
      policyRecord.current_revision = 2;
      policyRecord.content = call.body.content;
      policyRecord.content_hash = "revision-2";
      revisions.push({ number: 2, summary: call.body.summary, author: "fixture" });
      await route.fulfill({ json: { number: 2, content_hash: "revision-2", diagnostics: "Created Version 2." } });
      return;
    }
    if (path === "/api/policy/check") {
      await route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } });
      return;
    }
    if (path === "/api/policy/test/fixtures") {
      await route.fulfill({ json: { actors: [], services: [], warnings: [] } });
      return;
    }
    if (path === "/api/policy/test") {
      await route.fulfill({ json: { api_version: 1, actor_count: 0, services: [] } });
      return;
    }
    await route.fulfill({ status: 503, json: { error: "Endpoint not configured in this synthetic test." } });
  });

  await page.goto(`${appURL}/#policy`);
  await expect(page.locator("#policy-record-title")).toHaveText("Network policy");
  const savedSource = "define Employee as user.\n# saved revision boundary\n";
  await page.locator('.code-editor-host[data-source="policy-source"] .cm-content').fill(savedSource);
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "success");

  const filesToggle = page.locator("#policy-files-toggle");
  if (await filesToggle.getAttribute("aria-expanded") !== "true") {
    await filesToggle.focus();
    await filesToggle.press("Enter");
  }
  await expect(filesToggle).toHaveAttribute("aria-expanded", "true");
  await page.locator("#policy-save").click();
  await expect(page.locator("#version-dialog")).toBeVisible();
  await page.locator("#version-note").fill("Save remains a draft revision");
  await page.locator("#version-save").click();

  await expect(page.locator("#policy-source")).toHaveValue(savedSource);
  await expect(page.locator("#policy-history-count")).toHaveText("2 versions");
  const history = page.locator("#policy-history-menu");
  if (await history.getAttribute("open") === null) await history.locator("summary").click();
  await expect(history.locator('[data-revision="2"]')).toHaveAttribute("aria-current", "true");
  expect(apiCalls).toContainEqual({
    method: "POST",
    path: "/api/policy/records/test-policy/revisions",
    body: { content: savedSource, expected_revision: 1, summary: "Save remains a draft revision" },
  });
  expect(apiCalls.some(({ path }) => path.startsWith("/api/simulator/"))).toBe(false);
  expect(apiCalls.some(({ path }) => path.startsWith("/api/change-review/"))).toBe(false);
  const runtimeCallsAfterSave = apiCalls.filter(({ method, path }) => method === "POST" && /\/(stage|push|apply|install)(\/|$)/.test(path));
  expect(runtimeCallsAfterSave).toEqual([]);

  if (await filesToggle.getAttribute("aria-expanded") !== "true") {
    await filesToggle.focus();
    await filesToggle.press("Enter");
  }
  await expect(filesToggle).toHaveAttribute("aria-expanded", "true");
  await page.locator("#policy-stage").click();
  const stageDialog = page.locator("#policy-stage-dialog");
  await expect(stageDialog).toBeVisible();
  await expect(stageDialog).toContainText("does not push or activate");
  expect(apiCalls.some(({ path }) => path.endsWith("/stage"))).toBe(false);
  await page.locator("#policy-stage-cancel").click();
  await expect(stageDialog).toBeHidden();
  expect(apiCalls.filter(({ method, path }) => method === "POST" && /\/(stage|push|apply|install)(\/|$)/.test(path))).toEqual([]);
});
