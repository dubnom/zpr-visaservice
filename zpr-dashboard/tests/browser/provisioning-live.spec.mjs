import { test, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";

test.use({ ignoreHTTPSErrors: true });

test("live provisioning reads registry after real OIDC login and clears it on logout", async ({ page }) => {
  test.skip(!process.env.ZPR_OPERATOR_TEST_BINARY, "Requires the container-built operator test fixture.");
  const child = spawn(process.env.ZPR_OPERATOR_TEST_BINARY, ["-test.run=^TestOperatorDelegationRealTLSLoginToAuditedEnrollment$"], {
    env: { ...process.env, ZPR_OPERATOR_BROWSER_FIXTURE: "1", ZPR_SIMULATOR_URL: "http://127.0.0.1:1", SIMULATION_MANIFEST: "/does/not/exist" },
    stdio: ["pipe", "pipe", "pipe"],
  });
  let output = "";
  child.stderr.on("data", (chunk) => { output += chunk; });
  const lines = createInterface({ input: child.stdout });
  const exit = new Promise((resolve, reject) => { child.once("exit", resolve); child.once("error", reject); });
  try {
    const fixture = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`Fixture startup timed out: ${output}`)), 30000);
      lines.on("line", (line) => {
        output += line + "\n";
        if (line.startsWith("ZPR-OPERATOR-FIXTURE ")) {
          clearTimeout(timer);
          resolve(JSON.parse(line.slice("ZPR-OPERATOR-FIXTURE ".length)));
        }
      });
      child.once("exit", (code) => { clearTimeout(timer); reject(new Error(`Fixture exited ${code}: ${output}`)); });
      child.once("error", (error) => { clearTimeout(timer); reject(error); });
    });
    const requests = [];
    page.on("request", (request) => requests.push({ path: new URL(request.url()).pathname, method: request.method() }));
    await page.goto(fixture.url + "/#provisioning-adapters");
    await expect(page.locator("#provisioning-status")).toHaveAttribute("data-state", "unavailable");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: named-admin");
    await page.getByRole("group", { name: "Provisioning" }).getByRole("link", { name: "Adapters", exact: true }).click();
    await expect(page.locator("#provisioning-approved-catalog")).toContainText("standard");
    await expect(page.locator("#provisioning-organization option")).toHaveCount(1);
    await expect(page.locator("#provisioning-organization")).toHaveValue("production");
    await expect(page.locator("#provisioning-invitations")).toContainText("Fixture laptop");
    await expect(page.locator("#provisioning-invitations")).toContainText("pending_approval");
    await page.getByRole("button", { name: "Details for Fixture laptop", exact: true }).click();
    const detail = page.getByRole("dialog", { name: "Enrollment request details" });
    await expect(detail).toContainText(fixture.invitation);
    await expect(detail).toContainText(fixture.fingerprint);
    await expect(detail).toContainText("Approval deadline");
    await expect(detail).toContainText("Approval does not issue credentials");
    await detail.getByRole("button", { name: "Close", exact: true }).click();
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
    await expect(page.locator("#provisioning-invitations")).toBeHidden();
    await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
    await expect(page.locator("#provisioning-organization")).toBeDisabled();
    expect(requests.some((request) => request.path.startsWith("/api/simulator"))).toBe(false);
    expect(requests.filter((request) => request.path.startsWith("/api/enrollment/") && request.method !== "GET")).toEqual([]);
  } finally {
    lines.close();
    child.stdin.end("stop\n");
    const timer = setTimeout(() => child.kill("SIGTERM"), 10000);
    const code = await exit;
    clearTimeout(timer);
    expect(code, output).toBe(0);
  }
});
