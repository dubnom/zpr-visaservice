import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test.skip(!process.env.ZPR_GATEWAY_LIVE, "Requires an explicitly enabled authenticated live gateway check.");
test.use({
  trace: "off", screenshot: "off", video: "off",
  launchOptions: { args: process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS ? JSON.parse(process.env.ZPR_LOCAL_OPERATOR_CHROME_ARGS) : [] },
});

test("deployed Gateway Analyze highlights exact source lines and validates the draft allowlist", async ({ page }) => {
  const directory = process.env.ZPR_LOCAL_OPERATOR_DIR;
  const settings = JSON.parse(readFileSync(`${directory}/stack.json`, "utf8"));
  await page.goto(settings.origin + "/#gateways");
  await page.locator('input[name="login"]').fill(settings.username);
  await page.locator('input[name="password"]').fill(readFileSync(`${directory}/operator-password`, "utf8").trim());
  await page.locator("#submit-login").click();
  await expect(page.locator("#operator-login-status")).toHaveText(/^Signed in(?:: .+)?$/);
  await expect(page).toHaveURL(settings.origin + "/#gateways");
  const source = page.getByRole("textbox", { name: "Gateway draft JSON" });
  await expect(source).toHaveValue(/"instance_id":/);
  const original = await source.inputValue();
  const before = await page.evaluate(async () => (await fetch("/api/gateways/configs")).json());
  const draft = JSON.parse(original);
  draft.destinations = [{ origin: "", path_prefixes: ["/"] }];
  const invalid = JSON.stringify(draft, null, 2);
  await source.fill(invalid);
  const line = invalid.split("\n").findIndex(value => value.includes('"origin"')) + 1;
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  await expect(page.locator("#gateway-analyze")).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
  await expect(page.locator("#gateway-destinations")).toHaveCount(0);
  const marker = page.locator("#gateway-gutter .policy-test-line-result");
  await expect(marker).toHaveText("ERR");
  await expect(marker).toHaveAttribute("aria-label", new RegExp(`line ${line}:`));
  const inset = await marker.evaluate(element => element.getBoundingClientRect().left -
    element.closest(".config-source-gutter").getBoundingClientRect().left);
  expect(inset).toBeCloseTo(10, 0);
  await marker.click();
  await expect(page.locator("#gateway-error-text")).toContainText("destination 1 requires an HTTPS origin");
  await page.keyboard.press("Escape");
  await expect(source).toHaveCSS("outline-style", "none");
  expect(await source.evaluate(element => element.value.slice(element.selectionStart, element.selectionEnd))).toContain('"origin": ""');
  await page.getByRole("button", { name: "Form editor", exact: true }).click();
  await expect(source).toBeHidden();
  await page.getByRole("textbox", { name: "Destination 1 HTTPS origin", exact: true }).fill("https://api.example.com");
  await page.getByRole("button", { name: "Add destination", exact: true }).click();
  await page.getByRole("textbox", { name: "Destination 2 HTTPS origin", exact: true }).fill("https://other.example.com");
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  await expect(page.locator("#gateway-analyze")).toHaveAttribute("data-analysis-state", "success");
  await expect(page.locator("#gateway-draft-message")).toContainText("runtime configuration is unchanged");
  await page.getByRole("button", { name: "Raw JSON editor", exact: true }).click();
  expect(JSON.parse(await source.inputValue()).destinations).toEqual([
    { origin: "https://api.example.com", path_prefixes: ["/"] },
    { origin: "https://other.example.com", path_prefixes: ["/"] },
  ]);
  const after = await page.evaluate(async () => (await fetch("/api/gateways/configs")).json());
  expect(after).toEqual(before);
  const snapshot = await page.evaluate(async () => (await fetch("/api/snapshot")).json());
  expect(snapshot.api_status).toBe("connected");
  expect(snapshot.errors).toEqual([]);
  expect(snapshot.actors.filter(actor => actor.node).every(actor => actor.node_details.in_sync)).toBe(true);
  await source.fill(original);
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.locator('input[name="login"]').fill(settings.username);
  await page.locator('input[name="password"]').fill(readFileSync(`${directory}/operator-password`, "utf8").trim());
  await page.locator("#submit-login").click();
  await expect(page.locator("#operator-login-status")).toHaveText(/^Signed in(?:: .+)?$/);
  await expect(page).toHaveURL(settings.origin + "/#gateways");
  await expect(source).toBeVisible();
});
