import { test, expect } from "@playwright/test";
import { spawn, execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";

test.describe.configure({ mode: "serial", timeout: 60000 });
let directory;
let binary;
let fixture;
let ready;
let lines;
let pending;
let failure;
let stopped;

test.beforeAll(() => {
  directory = mkdtempSync(join(tmpdir(), "zpr-enrollment-live-"));
  binary = process.env.ZPR_ENROLLMENT_TEST_BINARY || join(directory, "tests");
  if (!process.env.ZPR_ENROLLMENT_TEST_BINARY) {
    execFileSync("go", ["test", "-c", "-o", binary, "./internal/enrollment"], { timeout: 60000 });
  }
});

function response() {
  if (failure) return Promise.reject(failure);
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      pending = null;
      reject(new Error("Enrollment fixture response timeout"));
    }, 15000);
    pending = {
      resolve: (value) => { clearTimeout(timeout); pending = null; resolve(value); },
      reject: (error) => { clearTimeout(timeout); pending = null; reject(error); }
    };
  });
}

async function command(value) {
  const result = response();
  fixture.stdin.write(value + "\n");
  return result;
}

test.beforeEach(async () => {
  failure = null;
  fixture = spawn(binary, ["-test.run", "^TestEnrollmentBrowserFixture$", "-test.timeout", "90s"], {
    env: { ...process.env, ZPR_ENROLLMENT_BROWSER_FIXTURE: "1" },
    stdio: ["pipe", "pipe", "pipe"]
  });
  const initial = response();
  let stderr = "";
  fixture.stderr.on("data", (data) => { stderr += data.toString(); });
  lines = createInterface({ input: fixture.stdout });
  lines.on("line", (line) => {
    if (!line.startsWith("ZPR-FIXTURE ")) return;
    try {
      const value = JSON.parse(line.slice("ZPR-FIXTURE ".length));
      if (!pending) throw new Error("Unexpected fixture response");
      pending.resolve(value);
    } catch (error) {
      failure = error;
      pending?.reject(error);
    }
  });
  stopped = new Promise((resolve) => {
    fixture.once("exit", (code, signal) => {
      failure = new Error(`Fixture exited (${code}, ${signal}): ${stderr}`);
      pending?.reject(failure);
      resolve({ code, signal });
    });
    fixture.once("error", (error) => {
      failure = error;
      pending?.reject(error);
      resolve({ code: null, signal: "spawn-error" });
    });
  });
  ready = await initial;
});

test.afterEach(async () => {
  if (!fixture) return;
  if (fixture.exitCode === null && fixture.signalCode === null) fixture.stdin.end("stop\n");
  const timeout = setTimeout(() => fixture.kill("SIGKILL"), 5000);
  const result = await stopped;
  clearTimeout(timeout);
  lines.close();
  expect(result).toEqual({ code: 0, signal: null });
});

test.afterAll(() => {
  if (directory) rmSync(directory, { recursive: true });
});

async function prepare(page) {
  await page.goto(ready.url);
  await expect(page.locator("#audience")).toHaveText(ready.audience);
  await page.locator("#organization").fill("company");
  await page.locator("#invitation").fill(ready.invitation);
  await page.getByRole("button", { name: "Create and save local key" }).click();
  await expect(page.locator("#fingerprint")).toHaveText(/^[a-f0-9]{64}$/);
  return page.locator("#fingerprint").textContent();
}

async function claim(page) {
  await page.locator("#code").fill(ready.code);
  await page.getByRole("button", { name: "Submit code and request approval" }).click();
  await expect(page.locator("#message")).toContainText("Waiting for administrator approval");
  await expect(page.locator("#code")).toHaveValue("");
  await expect(page.locator("#claim")).toBeHidden();
  expect(readFileSync(join(ready.state_directory, "identity.json"), "utf8")).not.toContain(ready.code);
}

test("real TLS claim polls approval and resumes the same identity after restart", async ({ page }) => {
  const operations = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/")) operations.push(new URL(request.url()).pathname);
  });
  const fingerprint = await prepare(page);
  await claim(page);
  await expect(page.locator("#status")).toBeDisabled();
  const decision = await command("approved");
  expect(decision.fingerprint).toBe(fingerprint);
  await expect(page.locator("#message")).toContainText("Administrator approved", { timeout: 15000 });
  await expect(page.locator("#message")).toContainText("Credentials are not issued");
  await expect(page.locator("#message")).toContainText("connectivity is not established");
  expect(operations.filter((path) => path === "/api/claim")).toHaveLength(1);
  expect(operations.filter((path) => path === "/api/status").length).toBeGreaterThanOrEqual(1);
  const state = readFileSync(join(ready.state_directory, "identity.json"), "utf8");
  const restarted = await command("restart");
  await page.goto(restarted.url);
  await expect(page.locator("#fingerprint")).toHaveText(fingerprint);
  await expect(page.locator("#claim")).toBeHidden();
  await expect(page.locator("#state")).toHaveText("No server status verified in this session.");
  await page.getByRole("button", { name: "Check fresh status" }).click();
  await expect(page.locator("#message")).toContainText("Administrator approved");
  expect(readFileSync(join(ready.state_directory, "identity.json"), "utf8")).toBe(state);
});

test("lost browser claim response recovers by status without resubmitting the code", async ({ page }) => {
  let claims = 0;
  const fingerprint = await prepare(page);
  await page.route("**/api/claim", async (route) => {
    claims++;
    // Deliver the real claim all the way to the TLS service, then lose only
    // the local browser response. No enrollment response is mocked.
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    await route.abort("failed");
  });
  await page.locator("#code").fill(ready.code);
  await page.getByRole("button", { name: "Submit code and request approval" }).click();
  await expect(page.locator("#message")).toContainText("Outcome is uncertain");
  await expect(page.locator("#claim")).toBeHidden();
  await expect(page.locator("#code")).toHaveValue("");
  await expect(page.locator("#status")).toBeEnabled({ timeout: 15000 });
  await page.getByRole("button", { name: "Check fresh status" }).click();
  await expect(page.locator("#message")).toContainText("Waiting for administrator approval");
  await expect(page.locator("#fingerprint")).toHaveText(fingerprint);
  const decision = await command("approved");
  expect(decision.fingerprint).toBe(fingerprint);
  await expect(page.locator("#message")).toContainText("Administrator approved", { timeout: 15000 });
  expect(claims).toBe(1);
  expect(readFileSync(join(ready.state_directory, "identity.json"), "utf8")).not.toContain(ready.code);
});

test("administrator rejection is terminal in the wizard and after restart", async ({ page }) => {
  await prepare(page);
  await claim(page);
  await command("rejected");
  await expect(page.locator("#message")).toContainText("Enrollment state: rejected", { timeout: 15000 });
  await expect(page.locator("#claim")).toBeHidden();
  const restarted = await command("restart");
  await page.goto(restarted.url);
  await page.getByRole("button", { name: "Check fresh status" }).click();
  await expect(page.locator("#message")).toContainText("Enrollment state: rejected");
  await expect(page.locator("#claim")).toBeHidden();
});

test("unclaimed saved identity requires status and explicit administrator-confirmed recovery", async ({ page }) => {
  const fingerprint = await prepare(page);
  const restarted = await command("restart");
  await page.goto(restarted.url);
  await expect(page.locator("#claim")).toBeHidden();
  await page.getByRole("button", { name: "Check fresh status" }).click();
  await expect(page.locator("#message")).toContainText("Status denied; claim outcome remains unknown");
  await expect(page.locator("#recovery")).toBeVisible();
  await expect(page.locator("#confirm-recovery")).not.toBeChecked();
  await expect(page.getByRole("button", { name: "Submit code and request approval" })).toBeEnabled({ timeout: 15000 });
  await page.locator("#code").fill(ready.code);
  await page.getByRole("button", { name: "Submit code and request approval" }).click();
  await expect(page.locator("#message")).toContainText("Status denied");
  await expect(page.locator("#code")).toHaveValue(ready.code);
  await page.locator("#confirm-recovery").check();
  await claim(page);
  await expect(page.locator("#fingerprint")).toHaveText(fingerprint);
});
