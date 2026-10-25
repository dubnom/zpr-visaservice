import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

test("Source-only operators receive application access and mutation CSRF", async ({ page, context }) => {
  let enabled = true;
  const mutations = [];
  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== "https://control.example") return route.abort();
    if (url.pathname === "/") return route.fulfill({
      contentType: "text/html",
      body: `<script src="/operator-request.js"></script><div class="operator-login"><p id="operator-login-status"></p><form id="operator-login-form"></form><button id="operator-logout" hidden>Sign out</button><span id="operator-scope" hidden></span></div><runtime-source-controls></runtime-source-controls><script src="/operator-login.js"></script><script src="/trusted-source.js"></script>`,
    });
    if (["/operator-request.js", "/operator-login.js", "/trusted-source.js"].includes(url.pathname)) {
      return route.fulfill({
        contentType: "application/javascript",
        body: await readFile(new URL(`../../cmd/zpr-web-dashboard/static${url.pathname}`, import.meta.url), "utf8"),
      });
    }
    if (url.pathname === "/auth/operator/config") return route.fulfill({ json: { enabled: true } });
    if (url.pathname === "/auth/operator/session") return route.fulfill({ json: {
      identity: { subject: "source-writer", issuer: "https://idp.example", organizations: ["production"], permissions: ["trusted-source.read", "trusted-source.control"] },
      csrf: "source-only-csrf",
    } });
    if (url.pathname === "/api/trusted-sources/runtime") return route.fulfill({ json: [{ name: "company", enabled }] });
    if (url.pathname === "/api/trusted-sources/runtime/company/control") {
      if (request.method() === "PUT") {
        expect(request.headers()["x-zpr-csrf"]).toBe("source-only-csrf");
        mutations.push(request.postDataJSON());
        enabled = request.postDataJSON().enabled;
      }
      return route.fulfill({ status: request.method() === "PUT" ? 202 : 200, json: { enabled, effective_enabled: enabled, declared: true, pending: true, revision: "stored" } });
    }
    return route.abort();
  });
  await page.goto("https://control.example/");
  await expect(page.locator("body")).not.toHaveClass(/operator-auth-gated/);
  await expect(page.locator("#operator-scope")).toContainText("trusted-source.control");
  page.once("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Disable runtime source company" }).click();
  await expect(page.getByRole("button", { name: "Enable runtime source company" })).toBeEnabled();
  expect(mutations).toEqual([{ enabled: false }]);
});

test("Runtime source toggle reads back pending state and fails closed without Simulator", async ({ page, context }) => {
  let enabled = true, fail = false, failReadback = false;
  let pauseInventory = false, releaseInventory;
  const mutations = [], forbidden = [];
  await context.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (url.origin !== "https://control.example" || url.pathname.includes("simulator")) {
      forbidden.push(url.href);
      return route.abort();
    }
    if (url.pathname === "/") return route.fulfill({ contentType: "text/html", body: `<script>window.zprOperatorFetch = fetch;</script><runtime-source-controls></runtime-source-controls><script src="/trusted-source.js"></script>` });
    if (url.pathname === "/trusted-source.js") return route.fulfill({ contentType: "application/javascript", body: await readFile(new URL("../../cmd/zpr-web-dashboard/static/trusted-source.js", import.meta.url), "utf8") });
    if (url.pathname === "/api/trusted-sources/runtime") {
      if (pauseInventory) await new Promise(resolve => { releaseInventory = resolve; });
      if (failReadback) return route.fulfill({ status: 503, json: { error: "readback unavailable" } });
      return route.fulfill({ json: [{ name: "company", enabled }] });
    }
    if (url.pathname.endsWith("/company/control")) {
      if (route.request().method() === "PUT") {
        mutations.push(route.request().postDataJSON());
        if (fail) return route.fulfill({ status: 503, json: { error: "unavailable" } });
        enabled = route.request().postDataJSON().enabled;
      }
      return route.fulfill({ json: { enabled, effective_enabled: enabled, declared: true, revision: "stored", pending: true } });
    }
    forbidden.push(url.href);
    return route.abort();
  });
  await page.goto("https://control.example/");
  await expect(page.getByRole("button", { name: "Disable runtime source company" })).toBeEnabled();
  pauseInventory = true;
  await page.getByRole("button", { name: "Refresh runtime state" }).click();
  await expect(page.getByRole("button", { name: "Disable runtime source company" })).toBeDisabled();
  await expect.poll(() => typeof releaseInventory).toBe("function");
  pauseInventory = false;
  releaseInventory();
  await expect(page.getByRole("button", { name: "Disable runtime source company" })).toBeEnabled();
  page.on("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "Disable runtime source company" }).click();
  await expect(page.getByRole("button", { name: "Enable runtime source company" })).toBeEnabled();
  await expect(page.getByRole("cell", { name: /Desired off \/ effective off \/ reconciliation pending/ })).toBeVisible();
  fail = true;
  await page.getByRole("button", { name: "Enable runtime source company" }).click();
  await expect(page.getByRole("status")).toContainText("HTTP 503");
  await expect(page.getByRole("button", { name: "Enable runtime source company" })).toBeDisabled();
  await page.getByRole("button", { name: "Refresh runtime state" }).click();
  await expect(page.getByRole("button", { name: "Enable runtime source company" })).toBeEnabled();
  fail = false;
  failReadback = true;
  await page.getByRole("button", { name: "Enable runtime source company" }).click();
  await expect(page.getByRole("status")).toContainText("HTTP 503");
  await expect(page.getByRole("status")).not.toContainText("Stored");
  await expect(page.getByRole("button", { name: "Enable runtime source company" })).toBeDisabled();
  failReadback = false;
  await page.getByRole("button", { name: "Refresh runtime state" }).click();
  await expect(page.getByRole("button", { name: "Disable runtime source company" })).toBeEnabled();
  expect(mutations).toEqual([{ enabled: false }, { enabled: true }, { enabled: true }]);
  expect(forbidden).toEqual([]);
});
