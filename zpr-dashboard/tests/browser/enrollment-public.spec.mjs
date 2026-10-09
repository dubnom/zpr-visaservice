import { test, expect } from "@playwright/test";

const enrollmentPage = new URL("../../packaging/enrollment/public/index.html", import.meta.url).href;

test("public enrollment link displays only invitation identifiers without network access", async ({ page }) => {
  const networkRequests = [];
  page.on("request", (request) => {
    if (!request.url().startsWith("file:")) networkRequests.push(request.url());
  });
  await page.goto(`${enrollmentPage}#organization=acme&invitation_id=demo-123`);

  await expect(page.locator("#invitation")).toBeVisible();
  await expect(page.locator("#organization")).toHaveText("acme");
  await expect(page.locator("#invitation-id")).toHaveText("demo-123");
  await expect(page.locator("#installer-unavailable")).toBeVisible();
  await expect(page.locator("#installer-link")).toBeHidden();
  await expect(page.locator("#setup-config-link")).toBeHidden();
  await expect(page.locator('input[type="password"]')).toHaveCount(0);
  expect(page.url()).not.toContain("#");
  expect(networkRequests).toEqual([]);
});

test("public enrollment link rejects activation codes in the URL", async ({ page }) => {
  await page.goto(`${enrollmentPage}?activation_code=dummy-only#organization=acme&invitation_id=demo-123`);

  await expect(page.locator("#unsafe-link")).toBeVisible();
  await expect(page.locator("#invitation")).toBeHidden();
  await expect(page.locator('input[type="password"]')).toHaveCount(0);
  expect(page.url()).not.toContain("activation_code");
  expect(page.url()).not.toContain("#");
});