import { test as base, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";

const assets = process.env.ZPR_LOGIN_TEST_ASSETS
  ? new URL(`file://${process.env.ZPR_LOGIN_TEST_ASSETS}/`)
  : new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url);
const authSource = await readFile(process.env.ZPR_LOGIN_TEST_AUTH_SOURCE || new URL("../../internal/operatorauth/auth.go", import.meta.url), "utf8");
const recovery = authSource.match(/io\.WriteString\(w, `([\s\S]*?)`\)/)[1];

const test = base.extend({
  provider: async ({}, use) => {
    const requests = [];
    const server = createServer((request, response) => {
      requests.push(request.headers);
      response.writeHead(200, { "Content-Type": "text/html" });
      response.end("<h1>Fresh provider sign-in</h1>");
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    try {
      await use({ url: `http://127.0.0.1:${server.address().port}/login?state=fresh`, requests });
    } finally {
      await new Promise(resolve => server.close(resolve));
    }
  },
});

test("expired login retry returns through Control Room before starting a fresh provider login", async ({ page, provider }) => {
  const errors = [];
  const posts = [];
  page.on("console", message => { if (message.type() === "error") errors.push(message.text()); });
  await page.route("https://room.example/**", async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/auth/operator/callback") {
      await route.fulfill({
        status: 403, contentType: "text/html",
        headers: {
          "Referrer-Policy": "no-referrer",
          "Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'",
        },
        body: recovery,
      });
    } else if (path === "/auth/operator/config") {
      await route.fulfill({ json: { enabled: true } });
    } else if (path === "/auth/operator/session") {
      await route.fulfill({ status: 401, json: { error: "Not signed in" } });
    } else if (path === "/auth/operator/login") {
      posts.push({ method: request.method(), headers: request.headers() });
      if (request.method() !== "POST" || request.headers().origin !== "https://room.example") {
        await route.fulfill({ status: 403, body: "Operator authentication unavailable or denied." });
        return;
      }
      await route.fulfill({
        status: 303, headers: { Location: provider.url, "Referrer-Policy": "no-referrer" }, body: "",
      });
    } else if (path.startsWith("/api/")) {
      await route.fulfill({ status: 401, json: { error: "Not signed in" } });
    } else {
      const file = new URL(path === "/" ? "index.html" : path.slice(1), assets);
      const body = await readFile(file);
      const extension = file.pathname.split(".").at(-1);
      await route.fulfill({
        body,
        contentType: ({ html: "text/html", js: "application/javascript", css: "text/css", svg: "image/svg+xml", woff2: "font/woff2" })[extension] || "application/octet-stream",
        headers: {
          "Referrer-Policy": path === "/" ? "same-origin" : "no-referrer",
          "Content-Security-Policy": "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:",
        },
      });
    }
  });
  await page.goto("https://room.example/auth/operator/callback?state=expired&code=rejected");
  await expect(page.getByRole("button", { name: "Timed out. Try again.", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Timed out. Try again.", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Fresh provider sign-in" })).toBeVisible();
  expect(posts).toHaveLength(1);
  expect(posts[0].method).toBe("POST");
  expect(posts[0].headers.origin).toBe("https://room.example");
  expect(posts[0].headers.referer).not.toContain("expired");
  expect(provider.requests).toHaveLength(1);
  expect(provider.requests[0].referer).toBeUndefined();
  expect(errors.filter(message => message.includes("Content Security Policy") || message.includes("violates"))).toEqual([]);
});
