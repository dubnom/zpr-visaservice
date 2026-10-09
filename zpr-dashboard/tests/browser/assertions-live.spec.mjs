import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test.skip(!process.env.ZPR_ASSERTIONS_LIVE, "Requires an explicitly enabled authenticated production assertion check.");
test.use({
  trace: "off", screenshot: "off", video: "off",
  launchOptions: { args: process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS ? JSON.parse(process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS) : [] },
});

test("deployed Assertion Analyze reads the production directory and uses a white scrollbar corner", async ({ page }) => {
  const directory = process.env.ZPR_LOCAL_OPERATOR_DIR;
  const settings = JSON.parse(readFileSync(`${directory}/stack.json`, "utf8"));
  await page.goto(settings.origin + "/#policy");
  await page.locator('input[name="login"]').fill(settings.username);
  await page.locator('input[name="password"]').fill(readFileSync(`${directory}/operator-password`, "utf8").trim());
  await page.locator("#submit-login").click();
  await expect(page.locator("#operator-login-status")).toHaveText(/^Signed in(?:: .+)?$/);
  const record = await page.evaluate(async () => {
    const catalog = await (await fetch("/api/policy")).json();
    const record = catalog.records.find(record => record.kind === "assertions" && !record.archived);
    return record ? { id: record.id, categoryID: record.category_id } : null;
  });
  expect(record).not.toBeNull();
  await page.locator("#policy-picker-toggle").click();
  await page.locator(`[data-record-id="${record.id}"]`).click();
  const source = page.locator("#assertion-source");
  await expect(source).toBeVisible();
  const original = await source.inputValue();
  const status = await page.evaluate(async () => (await fetch("/api/assertions")).json());
  expect(status.configured).toBe(true);
  const responsePromise = page.waitForResponse(response => response.url().endsWith("/api/assertions/evaluate") && response.request().method() === "POST");
  await page.locator("#assertion-analyze").click();
  const response = await responsePromise;
  expect(response.status()).toBe(200);
  const run = await response.json();
  expect(["pass", "fail"]).toContain(run.status);
  expect(run.error || "").toBe("");
  expect(await page.locator("#assertion-result-lines").evaluate(lines =>
    [...lines.children].every(line => line.querySelectorAll("button").length <= 1))).toBe(true);
  await source.fill(`// ${"horizontal-overflow ".repeat(100)}\n${original}`);
  await expect(page.locator("#assertion-editor")).toHaveAttribute("data-horizontal-overflow", "true");
  expect(await page.locator("#assertion-editor").evaluate(editor => getComputedStyle(editor, "::after").backgroundColor)).toBe("rgb(255, 255, 255)");
  await source.fill(original);
});
