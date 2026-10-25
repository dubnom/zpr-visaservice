import assert from "node:assert/strict";
import { chromium } from "@playwright/test";

const origin = process.env.ZPR_TEST_SOURCE_BRIDGE;
assert.ok(origin?.startsWith("https://127.0.0.1:"));
const browser = await chromium.launch({
  args: [`--ignore-certificate-errors-spki-list=${process.env.ZPR_TEST_SOURCE_BROWSER_SPKI}`],
});
try {
  const context = await browser.newContext({
    extraHTTPHeaders: { "X-Source-Fixture": process.env.ZPR_TEST_SOURCE_BRIDGE_TOKEN },
  });
  const [name, ...value] = process.env.ZPR_TEST_SOURCE_OPERATOR_COOKIE.split("=");
  await context.addCookies([{ name, value: value.join("="), url: origin, secure: true, httpOnly: true, sameSite: "Lax" }]);
  const page = await context.newPage();
  page.setDefaultTimeout(5000);
  const errors = [], requests = [], mutations = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("request", request => {
    const url = new URL(request.url());
    requests.push(url);
    if (request.method() === "PUT") mutations.push(request.postDataJSON());
  });
  await page.goto(`${origin}/fixture/ui`);
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
  assert.match(await page.getByRole("status").last().textContent(), /Change accepted/);
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
  const on = await page.evaluate(async () => {
    const response = await window.zprOperatorFetch("/api/trusted-sources/runtime/company/control");
    return response.json();
  });
  assert.equal(on.enabled, true);
  assert.equal(on.effective_enabled, true);
  assert.notEqual(on.revision, off.revision);
  assert.deepEqual(mutations, [{ enabled: false }, { enabled: true }]);
  assert.deepEqual(errors, []);
  assert.ok(requests.every(url => url.origin === origin && !url.pathname.includes("simulator")));
  console.log("SOURCE_LIFECYCLE browser: real Chromium, production login/request/control assets, confirmation cancel, durable disable/readback and re-enable passed");
  await context.close();
} finally {
  await browser.close();
}
