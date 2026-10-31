import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

const script = await readFile(new URL("../../scripts/operator-idp/remember-login.js", import.meta.url), "utf8");
const origin = "https://idp.example.test";
const key = "zpr.operator-idp.remembered-login";

async function setup(page, username = "") {
  await page.route(`${origin}/**`, route => {
    if (new URL(route.request().url()).pathname === "/remember-login.js") {
      return route.fulfill({ contentType: "application/javascript", body: script });
    }
    return route.fulfill({ contentType: "text/html", body: `<!doctype html>
      <label for="login">Email</label><form method="post">
      <input id="login" name="login" required autofocus value="${username}">
      <label for="password">Password</label><input id="password" name="password" type="password" required>
      <button type="submit">Login</button></form>
      <script>document.querySelector("form").addEventListener("submit", event => event.preventDefault());</script>
      <script src="/remember-login.js" defer></script>` });
  });
  await page.goto(`${origin}/auth/local?state=first`);
}

test("remembers only the submitted email across fresh login attempts and browser contexts", async ({ page, browser }) => {
  await setup(page);
  await expect(page.getByRole("checkbox")).toBeChecked();
  await expect(page.locator("#login")).toHaveAttribute("autocomplete", "username");
  await expect(page.locator("#password")).toHaveAttribute("autocomplete", "current-password");
  await page.locator("#login").fill("operator@example.test");
  await page.locator("#password").fill("not-a-real-password");
  await page.getByRole("button", { name: "Login" }).click();
  expect(await page.evaluate(() => ({ ...localStorage }))).toEqual({ [key]: "operator@example.test" });
  const context = await browser.newContext({ storageState: await page.context().storageState() });
  try {
    const fresh = await context.newPage();
    await setup(fresh);
    await expect(fresh.locator("#login")).toHaveValue("operator@example.test");
    await expect(fresh.locator("#password")).toHaveValue("");
    await expect(fresh.locator("#password")).toBeFocused();
  } finally {
    await context.close();
  }
});

test("opt-out immediately forgets the email and does not save the next submission", async ({ page }) => {
  await setup(page);
  await page.evaluate(({ key }) => localStorage.setItem(key, "old@example.test"), { key });
  await page.reload();
  await expect(page.locator("#login")).toHaveValue("old@example.test");
  await page.getByRole("checkbox").uncheck();
  expect(await page.evaluate(key => localStorage.getItem(key), key)).toBeNull();
  await page.locator("#login").fill("other@example.test");
  await page.locator("#password").fill("not-a-real-password");
  await page.getByRole("button", { name: "Login" }).click();
  await page.reload();
  await expect(page.locator("#login")).toHaveValue("");
  await expect(page.getByRole("checkbox")).not.toBeChecked();
  await page.getByRole("checkbox").check();
  await page.locator("#login").fill("remember-again@example.test");
  await page.locator("#password").fill("not-a-real-password");
  await page.getByRole("button", { name: "Login" }).click();
  await page.reload();
  await expect(page.locator("#login")).toHaveValue("remember-again@example.test");
  await expect(page.getByRole("checkbox")).toBeChecked();
});

test("preserves provider-prefilled usernames and replaces the remembered account on submit", async ({ page }) => {
  await setup(page, "provider-user");
  await page.evaluate(({ key }) => localStorage.setItem(key, "old@example.test"), { key });
  await page.reload();
  await expect(page.locator("#login")).toHaveValue("provider-user");
  await page.locator("#password").fill("not-a-real-password");
  await page.getByRole("button", { name: "Login" }).click();
  expect(await page.evaluate(key => localStorage.getItem(key), key)).toBe("provider-user");
});

test("blocked browser storage is reported without blocking sign-in", async ({ page }) => {
  await page.addInitScript(() => {
    for (const method of ["getItem", "setItem", "removeItem"]) {
      Storage.prototype[method] = () => { throw new DOMException("Storage blocked", "SecurityError"); };
    }
    window.submissions = 0;
    document.addEventListener("submit", () => { window.submissions++; });
  });
  await setup(page);
  await expect(page.getByRole("status")).toContainText("Sign-in is still available");
  await page.locator("#login").fill("operator@example.test");
  await page.locator("#password").fill("not-a-real-password");
  await page.getByRole("button", { name: "Login" }).click();
  expect(await page.evaluate(() => window.submissions)).toBe(1);
  await expect(page.getByRole("status")).toContainText("Sign-in is still available");
});
