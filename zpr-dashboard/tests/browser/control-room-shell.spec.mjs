import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const origin = "https://control-room.example.test";
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const pages = [
  ["approvals.html", "Approvals"],
  ["budget-authority.html", null],
  ["subject-links.html", null],
  ["subject-link-publications.html", null],
];

for (const [file, label] of pages) {
  test(`${label ?? file} uses the Control Room navigation`, async ({ page, context }) => {
    await context.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.origin !== origin) return route.abort();
      const name = url.pathname === "/" ? "/index.html" : url.pathname;
      if (/\.(js|css|html)$/.test(name)) {
        const path = resolve(assets, `.${name}`);
        if (!path.startsWith(assets)) return route.fulfill({ status: 403 });
        const contentType = name.endsWith(".js") ? "application/javascript" : name.endsWith(".css") ? "text/css" : "text/html";
        try {
          return route.fulfill({ contentType, body: await readFile(path, "utf8") });
        } catch {
          return route.fulfill({ status: 404 });
        }
      }
      return route.fulfill({ status: 401, json: { error: "No active operator session." } });
    });
    await page.goto(`${origin}/${file}`);
    const nav = page.locator("nav.primary-nav");
    await expect(nav).toHaveCount(1);
    if (label) {
      await expect(nav.locator("a.nav-link.active")).toHaveText(label);
      await expect(nav.locator("a.nav-link.active")).toHaveAttribute("aria-current", "page");
    } else {
      await expect(nav.locator("a.nav-link.active")).toHaveCount(0);
    }
    await expect(nav.locator("a.nav-link").first()).toHaveAttribute("href", "/#map");
    await expect(nav.locator('a[href="/approvals.html"]')).toHaveCount(1);
    await expect(nav.locator("a.nav-link", { hasText: /Budget Authority|Subject-Link/ })).toHaveCount(0);
  });
}
