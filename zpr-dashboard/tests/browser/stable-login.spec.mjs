import { test as base, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";

const test = base.extend({
  providerURL: async ({}, use) => {
    const server = createServer((request, response) => {
      response.writeHead(200, { "Content-Type": "text/html" });
      response.end("<h1>Fresh provider sign-in</h1>");
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    try {
      await use(`http://127.0.0.1:${server.address().port}/login?state=fresh`);
    } finally {
      await new Promise(resolve => server.close(resolve));
    }
  },
});

const script = await readFile(new URL("../../cmd/zpr-web-dashboard/static/operator-login.js", import.meta.url), "utf8");
const requestScript = await readFile(new URL("../../cmd/zpr-web-dashboard/static/operator-request.js", import.meta.url), "utf8");
const roomHTML = await readFile(new URL("../../cmd/zpr-web-dashboard/static/index.html", import.meta.url), "utf8");
const roomCSS = await readFile(new URL("../../cmd/zpr-web-dashboard/static/app.css", import.meta.url), "utf8");
const idpSource = await readFile(new URL("../../scripts/operator-idp/error.html", import.meta.url), "utf8");
const idpRecovery = idpSource.split('{{ if eq .ErrMsg "Requested resource does not exist." }}')[1].split("{{ else }}")[0];
const origin = "https://localhost:8787";
const html = `<!doctype html><html><body>
<section class="operator-login">
<h1 class="operator-login-title" id="operator-login-title">Operator sign-in</h1>
<p class="operator-login-description" id="operator-login-description">Sign in to continue.</p>
<p id="operator-login-status" role="status"></p>
<form id="operator-login-form" method="post" action="/auth/operator/login"><button>Sign in</button></form>
<button id="operator-logout" hidden>Sign out</button><p id="operator-scope" hidden></p>
</section><script src="/operator-request.js"></script><script src="/operator-login.js"></script></body></html>`;

async function setup(page, providerURL, fullPage = false) {
  const requests = { posts: 0, recoveries: 0, signedIn: false, sessionChecks: 0, configFailure: false };
  await page.route(`${origin}/**`, async route => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/operator-login.js") {
      await route.fulfill({ contentType: "application/javascript", body: script });
    } else if (path === "/operator-request.js") {
      await route.fulfill({ contentType: "application/javascript", body: requestScript });
    } else if (path === "/auth/operator/config") {
      await route.fulfill(requests.configFailure ? { status: 503 } : { json: { enabled: true } });
    } else if (path === "/auth/operator/session") {
      requests.sessionChecks++;
      await route.fulfill(requests.signedIn ? { json: {
        identity: { subject: "operator", issuer: "https://issuer.example", organizations: ["production"], permissions: ["read"] },
        csrf: "test-csrf",
      } } : { status: 401, json: { error: "Not signed in" } });
    } else if (path === "/auth/operator/login") {
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers().origin).toBe(origin);
      requests.posts++;
      await route.fulfill({ status: 303, headers: { Location: providerURL }, body: "" });
    } else if (path === "/auth/operator/callback") {
      requests.recoveries++;
      await route.fulfill({ status: 303, headers: { Location: "/?operator_login=expired", "Cache-Control": "no-store", "Referrer-Policy": "no-referrer" } });
    } else if (path.startsWith("/api/")) {
      await route.fulfill({ status: 403 });
    } else if (path === "/auth/operator/logout") {
      requests.signedIn = false;
      await route.fulfill({ status: 204 });
    } else if (fullPage && path.endsWith(".css")) {
      await route.fulfill({ contentType: "text/css", body: path === "/app.css" ? roomCSS : "" });
    } else if (fullPage && path.endsWith(".js")) {
      await route.fulfill({ contentType: "application/javascript", body: "" });
    } else {
      await route.fulfill({ contentType: "text/html", body: fullPage ? roomHTML : html });
    }
  });
  return requests;
}

test("application opens provider sign-in directly without an interim screen", async ({ page, providerURL }) => {
  const requests = await setup(page, providerURL);
  await page.goto(`${origin}/#topology`);
  await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
  expect(requests.posts).toBe(1);
});

test("real Control Room markup visibly announces expiry without Simulator", async ({ page, providerURL }) => {
  const requests = await setup(page, providerURL, true);
  await page.route("**/*", route => {
    const url = route.request().url();
    if (url.startsWith(origin) || url.startsWith(providerURL.split("/login")[0])) return route.fallback();
    return route.abort();
  });
  requests.signedIn = true;
  await page.goto(`${origin}/#policy`);
  await expect(page.locator("#operator-login-status")).toHaveText("User");
  await expect(page.locator("#operator-login-form")).toBeHidden();
  requests.signedIn = false;
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(page.locator("#operator-login-status")).toHaveText("Session expired. Redirecting to sign in...");
  await expect(page.locator("#operator-login-status")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
  expect(requests.posts).toBe(1);
});

test("real Control Room sign-out opens provider without the interim page", async ({ page, providerURL }) => {
  const requests = await setup(page, providerURL, true);
  requests.signedIn = true;
  await page.goto(`${origin}/#policy`);
  await expect(page.locator("#operator-login-status")).toHaveText("User");
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
  expect(requests.signedIn).toBe(false);
  expect(requests.posts).toBe(1);
});

for (const trigger of ["focus", "reload", "api"]) {
  test(`session expiry on ${trigger} announces expiry then opens provider once and retains destination`, async ({ page, providerURL }) => {
    const requests = await setup(page, providerURL);
    requests.signedIn = true;
    await page.goto(`${origin}/#policy`);
    await expect(page.locator("#operator-scope")).toBeVisible();
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect.poll(() => requests.sessionChecks).toBe(2);
    await expect(page.locator("#operator-scope")).toBeVisible();
    await expect(page.locator("#operator-login-status")).toHaveText("User");
    requests.signedIn = false;
    if (trigger === "reload") await page.reload();
    else if (trigger === "api") await page.evaluate(() => window.zprOperatorFetch("/api/test"));
    else await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.locator("#operator-login-status")).toHaveText("Session expired. Redirecting to sign in...");
    await expect(page.locator("#operator-scope")).toBeHidden();
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
    expect(requests.posts).toBe(1);
    await page.goto(`${origin}/#policy`);
    await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    expect(requests.posts).toBe(1);
    expect(await page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBe("/#policy");
  });
}

test("permission rejection preserves a valid session and logout goes directly to provider sign-in", async ({ page, providerURL }) => {
  const requests = await setup(page, providerURL);
  requests.signedIn = true;
  await page.goto(`${origin}/#policy`);
  await expect(page.locator("#operator-scope")).toBeVisible();
  await page.evaluate(() => window.zprOperatorFetch("/api/forbidden"));
  await expect.poll(() => requests.sessionChecks).toBe(2);
  await expect(page.locator("#operator-scope")).toBeVisible();
  expect(requests.posts).toBe(0);
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
  expect(requests.posts).toBe(1);
  await page.goto(`${origin}/#policy`);
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
  expect(requests.posts).toBe(1);
  requests.signedIn = true;
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(page.locator("#operator-scope")).toBeVisible();
  await expect(page.locator("#operator-login-form")).toBeHidden();
});

test("login configuration failure is visible and recovery never creates an automatic loop", async ({ page, providerURL }) => {
  const requests = await setup(page, providerURL);
  requests.configFailure = true;
  await page.goto(`${origin}/#policy`);
  await expect(page.locator("#operator-login-status")).toContainText("Login configuration unavailable");
  expect(requests.posts).toBe(0);
  requests.configFailure = false;
  await page.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
  expect(requests.posts).toBe(0);
});

for (const provider of ["application", "identity-provider"]) {
  test(`${provider} expiry recovers once and preserves the original destination`, async ({ page, providerURL }) => {
    const requests = await setup(page, providerURL);
    await page.goto(`${origin}/#topology`);
    await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
    if (provider === "identity-provider") {
      await page.route("https://issuer.example/expired", route => route.fulfill({
        contentType: "text/html", body: `<!doctype html><html><body>${idpRecovery}</body></html>`,
      }));
      await page.goto("https://issuer.example/expired");
    } else {
      await page.goto(`${origin}/auth/operator/callback?state=old&code=secret`);
    }
    await expect(page.locator("#operator-login-status")).toContainText("previous sign-in attempt expired");
    await expect(page).toHaveURL(`${origin}/`);
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.locator("#operator-login-status")).toContainText("Select Sign in");
    expect(requests.posts).toBe(1);
    expect(requests.recoveries).toBe(provider === "application" ? 1 : 0);
    expect(await page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBe("/#topology");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
    expect(requests.posts).toBe(2);
    requests.signedIn = true;
    await page.goto(`${origin}/`);
    await expect(page).toHaveURL(`${origin}/#topology`);
    await expect(page.locator("#operator-login-status")).toHaveText("User");
    expect(await page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBeNull();
  });
}
