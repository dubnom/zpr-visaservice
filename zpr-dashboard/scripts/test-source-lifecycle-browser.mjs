import assert from "node:assert/strict";
import { chromium, expect } from "@playwright/test";

const origin = process.env.ZPR_TEST_SOURCE_BRIDGE;
const idp = process.env.ZPR_TEST_SOURCE_IDP;
assert.ok(origin?.startsWith("https://127.0.0.1:"));
assert.ok(idp?.startsWith("https://127.0.0.1:"));
const browser = await chromium.launch({
  args: [`--ignore-certificate-errors-spki-list=${process.env.ZPR_TEST_SOURCE_BROWSER_SPKI}`],
});
try {
  const context = await browser.newContext({
    extraHTTPHeaders: { "X-Source-Fixture": process.env.ZPR_TEST_SOURCE_BRIDGE_TOKEN },
  });
  assert.deepEqual(await context.cookies(), [], "browser must begin without an imported session");
  const page = await context.newPage();
  page.setDefaultTimeout(5000);
  const errors = [], requests = [], mutations = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("request", request => {
    const url = new URL(request.url());
    requests.push(url);
    if (request.method() === "PUT") mutations.push(request.postDataJSON());
  });
  const document = await page.goto(`${origin}/fixture/ui.html`);
  assert.equal(document.headers()["referrer-policy"], "same-origin");
  assert.equal(document.headers()["x-content-type-options"], "nosniff");
  assert.ok(document.headers()["content-security-policy"]?.includes("script-src 'self'"));
  try {
    await page.getByRole("button", { name: "Continue as fixture source writer" }).click();
  } catch (error) {
    throw new Error(`Initial sign-in failed at ${new URL(page.url()).pathname}: ${await page.locator("body").innerText()}; page errors: ${errors.join("; ")}`, { cause: error });
  }
  await expect(page).toHaveURL(`${origin}/fixture/ui.html`);
  await expect(page.locator("#operator-scope")).toContainText("trusted-source.control");
  assert.ok(requests.some(url => url.origin === idp && url.pathname === "/authorize"));
  const callbacks = requests.filter(url => url.origin === origin && url.pathname === "/auth/operator/callback");
  assert.equal(callbacks.length, 1, "browser must complete exactly one OIDC callback");
  const session = await page.evaluate(async () => {
    const response = await fetch("/auth/operator/session", { cache: "no-store" });
    if (!response.ok) throw new Error(`Session read failed: ${response.status}`);
    return response.json();
  });
  assert.equal(session.identity.subject, "source-writer");
  const cookies = await context.cookies(origin);
  const sessionCookie = cookies.find(cookie => cookie.httpOnly && cookie.value && cookie.expires > 0);
  assert.ok(sessionCookie?.secure, "browser callback must set a secure HttpOnly session");
  const disable = page.getByRole("button", { name: "Disable runtime source company" });
  await disable.waitFor();
  assert.equal(await disable.isEnabled(), true);
  page.once("dialog", dialog => dialog.dismiss());
  await disable.click();
  assert.equal(mutations.length, 0, "cancelled confirmation mutated source state");
  page.once("dialog", dialog => dialog.accept());
  await disable.click();
  const enable = page.getByRole("button", { name: "Enable runtime source company" });
  try {
    await enable.waitFor();
  } catch (error) {
    throw new Error(`Disable UI failed: ${await page.getByRole("status").last().textContent()}; page errors: ${errors.join("; ")}; mutations: ${JSON.stringify(mutations)}`, { cause: error });
  }
  await expect(page.getByRole("status").last()).toContainText("Change accepted");
  assert.match(await page.locator("tbody").textContent(), /Desired off \/ effective off \/ reconciliation pending/);
  const off = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/trusted-sources/runtime/company/control");
    return response.json();
  });
  assert.equal(off.enabled, false);
  assert.ok(off.revision);
  page.once("dialog", dialog => dialog.accept());
  await enable.click();
  await disable.waitFor();
  let on = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/trusted-sources/runtime/company/control");
    return response.json();
  });
  assert.equal(on.enabled, true);
  assert.equal(on.effective_enabled, true);
  assert.notEqual(on.revision, off.revision);
  assert.deepEqual(mutations, [{ enabled: false }, { enabled: true }]);
  const setServiceTrust = async trusted => {
    const status = await page.evaluate(async trusted => {
      const response = await fetch("/fixture/service-trust", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ trusted }),
      });
      return response.status;
    }, trusted);
    assert.equal(status, 204, "fixture failed to configure Control-Service trust");
  };
  await setServiceTrust(false);
  try {
    const failed = page.waitForResponse(response => response.request().method() === "PUT" &&
      new URL(response.url()).pathname === "/api/trusted-sources/runtime/company/control");
    page.once("dialog", dialog => dialog.accept());
    await disable.click();
    const response = await failed;
    assert.equal(response.status(), 502);
    assert.equal(response.headers()["cache-control"], "no-store");
    await expect(page.locator("[data-runtime-message]")).toContainText("HTTP 502");
    await expect(disable).toBeDisabled();
    await page.getByRole("button", { name: "Refresh runtime state" }).click();
    await expect(page.locator("[data-runtime-message]")).toContainText("HTTP 502");
    await expect(disable).toBeDisabled();
  } finally {
    await setServiceTrust(true);
  }
  await page.getByRole("button", { name: "Refresh runtime state" }).click();
  await expect(disable).toBeEnabled();
  const recovered = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/trusted-sources/runtime/company/control");
    if (!response.ok) throw new Error(`Recovered source read failed: ${response.status}`);
    return response.json();
  });
  assert.equal(recovered.enabled, true);
  assert.equal(recovered.revision, on.revision, "failed service TLS mutation changed durable source revision");
  assert.deepEqual(mutations, [{ enabled: false }, { enabled: true }, { enabled: false }]);
  const armed = await page.evaluate(async () => (await fetch("/fixture/lose-response", { method: "POST" })).status);
  assert.equal(armed, 204, "fixture failed to arm accepted-response loss");
  const lost = page.waitForResponse(response => response.request().method() === "PUT" &&
    new URL(response.url()).pathname === "/api/trusted-sources/runtime/company/control");
  page.once("dialog", dialog => dialog.accept());
  await disable.click();
  const lostResponse = await lost;
  assert.equal(lostResponse.status(), 502);
  assert.equal(lostResponse.headers()["cache-control"], "no-store");
  await expect(page.locator("[data-runtime-message]")).toContainText("HTTP 502");
  await expect(disable).toBeDisabled();
  assert.equal(mutations.length, 4, "ambiguous mutation was automatically retried");
  await page.getByRole("button", { name: "Refresh runtime state" }).click();
  await expect(enable).toBeEnabled();
  assert.match(await page.locator("tbody").textContent(), /Desired off \/ effective off \/ reconciliation pending/);
  const acceptedDespiteError = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/trusted-sources/runtime/company/control");
    if (!response.ok) throw new Error(`Ambiguous mutation readback failed: ${response.status}`);
    return response.json();
  });
  assert.equal(acceptedDespiteError.enabled, false, "lost response did not follow a stored disable");
  assert.equal(acceptedDespiteError.effective_enabled, false);
  assert.equal(acceptedDespiteError.pending, true);
  assert.notEqual(acceptedDespiteError.revision, on.revision, "accepted mutation did not change durable revision");
  assert.equal(mutations.length, 4, "explicit readback retried the ambiguous mutation");
  page.once("dialog", dialog => dialog.accept());
  await enable.click();
  await expect(disable).toBeEnabled();
  on = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/trusted-sources/runtime/company/control");
    if (!response.ok) throw new Error(`Explicit recovery readback failed: ${response.status}`);
    return response.json();
  });
  assert.equal(on.enabled, true);
  assert.notEqual(on.revision, acceptedDespiteError.revision);
  assert.deepEqual(mutations, [
    { enabled: false }, { enabled: true }, { enabled: false }, { enabled: false }, { enabled: true },
  ]);
  const logout = page.waitForResponse(response => new URL(response.url()).pathname === "/auth/operator/logout");
  await page.locator("#operator-logout").click();
  assert.equal((await logout).status(), 204);
  // Direct-sign-in intentionally starts a new login after logout. Leave the
  // fixture consent unaccepted and replay the old cookie in a separate browser.
  await page.getByRole("button", { name: "Continue as fixture source writer" }).waitFor();
  const stale = await browser.newContext({
    extraHTTPHeaders: { "X-Source-Fixture": process.env.ZPR_TEST_SOURCE_BRIDGE_TOKEN },
  });
  try {
    await stale.addCookies([sessionCookie]);
    const stalePage = await stale.newPage();
    const response = await stalePage.goto(`${origin}/auth/operator/session`);
    assert.equal(response.status(), 401, "logged-out cookie revived an operator session");
    const rejected = await stalePage.evaluate(async csrf => {
      const response = await fetch("/api/trusted-sources/runtime/company/control", {
        method: "PUT", headers: { "Content-Type": "application/json", "X-ZPR-CSRF": csrf },
        body: JSON.stringify({ enabled: false }),
      });
      return { status: response.status, cache: response.headers.get("Cache-Control") };
    }, session.csrf);
    assert.deepEqual(rejected, { status: 403, cache: "no-store" }, "stale cookie/CSRF authorized a mutation");
  } finally {
    await stale.close();
  }
  await page.getByRole("button", { name: "Continue as fixture source writer" }).click();
  await expect(page).toHaveURL(`${origin}/fixture/ui.html`);
  await expect(disable).toBeEnabled();
  const afterLogout = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/trusted-sources/runtime/company/control");
    return response.json();
  });
  assert.equal(afterLogout.enabled, true);
  assert.equal(afterLogout.revision, on.revision, "stale-session denial changed durable revision");
  const refreshed = (await context.cookies(origin)).find(cookie => cookie.name === sessionCookie.name);
  assert.ok(refreshed && refreshed.value !== sessionCookie.value, "reauthentication reused logged-out session");
  assert.deepEqual(errors, []);
  assert.ok(requests.every(url => [origin, idp].includes(url.origin) && !url.pathname.includes("simulator")));
  console.log("SOURCE_LIFECYCLE browser: real Chromium OIDC navigation/callback, source-only login/request/control assets, confirmation cancel, durable disable/readback/re-enable, service-mTLS failure/recovery, accepted-response-loss readback without retry and logout/stale-session denial passed");
  await context.close();
} finally {
  await browser.close();
}
