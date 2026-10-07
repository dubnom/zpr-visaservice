import { test, expect } from "@playwright/test";
import { execFileSync, spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtempSync, writeFileSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

test.describe.configure({ mode: "serial" });
let directory;
let binary;
let sequence = 0;
let child;
let setupURL;
let state;

test.beforeAll(() => {
  directory = mkdtempSync(join(tmpdir(), "zpr-setup-browser-"));
  binary = process.env.ZPR_SETUP_TEST_BINARY || join(directory, "setup");
  if (!process.env.ZPR_SETUP_TEST_BINARY) {
    execFileSync("go", ["build", "-o", binary, "./cmd/zpr-enrollment-setup"], { timeout: 60000 });
  }
});

test.beforeEach(async () => {
  state = join(directory, `state-${sequence++}`);
  const config = join(directory, `config-${sequence}.json`);
  writeFileSync(config, JSON.stringify({
    version: 1,
    audience: "https://127.0.0.1:1",
    state_directory: state,
    allow_software_development: true
  }), { mode: 0o600 });
  child = spawn(binary, ["-config", config], { stdio: ["ignore", "pipe", "pipe"] });
  setupURL = await new Promise((resolve, reject) => {
    let output = "";
    const timeout = setTimeout(() => reject(new Error("Setup command did not become ready")), 10000);
    child.stdout.on("data", (data) => {
      output += data.toString();
      const match = output.match(/http:\/\/127\.0\.0\.1:\d+\/#session=[A-Z2-7]+/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[0]);
      }
    });
    child.once("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
    child.once("exit", (code) => {
      clearTimeout(timeout);
      reject(new Error(`Setup command exited before readiness (${code})`));
    });
  });
});

test.afterEach(async () => {
  if (child && child.exitCode === null) {
    const stopped = once(child, "exit");
    child.kill("SIGTERM");
    await stopped;
  }
});

test.afterAll(() => {
  if (directory) rmSync(directory, { recursive: true });
});

test("local wizard persists a key, clears the code, and surfaces uncertain delivery", async ({ page }) => {
  await page.goto(setupURL);
  await expect(page.locator("#audience")).toHaveText("https://127.0.0.1:1");
  await expect(page).toHaveURL(setupURL.split("#")[0]);
  await expect(page.locator(".warning")).toContainText("not hardware-backed");
  await page.locator("#organization").fill("company");
  await page.locator("#invitation").fill("development-invitation");
  await page.getByRole("button", { name: "Create and save local key" }).click();
  await expect(page.locator("#fingerprint")).toHaveText(/^[a-f0-9]{64}$/);
  await expect(page.locator("#prepare")).toBeHidden();
  await expect(page.locator("#claim")).toBeVisible();
  await page.locator("#code").fill("C".repeat(26));
  await page.getByRole("button", { name: "Submit code and request approval" }).click();
  await expect(page.locator("#message")).toContainText("Outcome is uncertain");
  await expect(page.locator("#code")).toHaveValue("");
  await expect(page.locator("#claim")).toBeHidden();
  await expect(page.locator("#status")).toBeDisabled();
  expect(readFileSync(join(state, "identity.json"), "utf8")).not.toContain("C".repeat(26));
  const persisted = JSON.parse(readFileSync(join(state, "identity.json"), "utf8"));
  expect(persisted.protection).toBe("software-development");
  expect(persisted.enrollment.invitation_id).toBe("development-invitation");
});

test("a page without the private fragment cannot open a setup session", async ({ page }) => {
  await page.goto(setupURL.split("#")[0]);
  await expect(page.locator("#message")).toContainText("Missing setup session");
  await expect(page.getByRole("button", { name: "Create and save local key" })).toBeDisabled();
  const response = await page.request.get(setupURL.split("#")[0] + "api/session");
  expect(response.status()).toBe(401);
  expect(await response.text()).not.toContain("private_key");
});
