import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

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
