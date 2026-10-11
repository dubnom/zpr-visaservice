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
const authSource = await readFile(new URL("../../internal/operatorauth/auth.go", import.meta.url), "utf8");
const recovery = authSource.match(/io\.WriteString\(w, `([\s\S]*?)`\)/)[1];
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
</section><script src="/operator-login.js"></script></body></html>`;

async function setup(page, providerURL) {
  const requests = { posts: 0, recoveries: 0, signedIn: false };
  await page.route(`${origin}/**`, async route => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/operator-login.js") {
      await route.fulfill({ contentType: "application/javascript", body: script });
    } else if (path === "/auth/operator/config") {
      await route.fulfill({ json: { enabled: true } });
    } else if (path === "/auth/operator/session") {
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
      await route.fulfill({ status: 403, contentType: "text/html", body: recovery, headers: {
        "Cache-Control": "no-store", "Referrer-Policy": "no-referrer",
        "Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'",
      } });
    } else {
      await route.fulfill({ contentType: "text/html", body: html });
    }
  });
  return requests;
}

test("idle application sign-in stays available without starting an expiring attempt", async ({ page, providerURL }) => {
  await page.clock.install();
  const requests = await setup(page, providerURL);
  await page.goto(`${origin}/#topology`);
  await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
  await page.clock.fastForward(10 * 60 * 1000);
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
  expect(requests.posts).toBe(0);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
  expect(requests.posts).toBe(1);
});

for (const provider of ["application", "identity-provider"]) {
  test(`${provider} expiry recovers once and preserves the original destination`, async ({ page, providerURL }) => {
    const requests = await setup(page, providerURL);
    await page.goto(`${origin}/#topology`);
    await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
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
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in");
    expect(await page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBeNull();
  });
}
