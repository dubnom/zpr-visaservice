import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve, sep } from "node:path";
import { editorCSP } from "./code-editor.mjs";

const origin = "https://control.company.example";
const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));

test("Multisource contract shows identical subjects in separate company and evaluation catalogs", async ({ page, context }, testInfo) => {
  const sources = ["company_directory", "evaluation_directory"].map(source_id => ({ source_id, organization_id: "company", display_name: source_id, kind: "ldap" }));
  const forbidden = [], reads = [], errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await context.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /simulator|\/api\/machines/.test(url.pathname)) {
      forbidden.push(url.href);
      return route.abort("connectionrefused");
    }
    if (url.pathname === "/auth/operator/config") return route.fulfill({ json: { enabled: false } });
    if (url.pathname === "/api/assertions") return route.fulfill({ json: { organization_id: "company", organization_name: "Company", default_source: "company_directory", sources } });
    if (url.pathname === "/api/assertions/sources") return route.fulfill({ json: { organization_id: "company", revision: "1", durable: true, sources: sources.map(source => ({ ...source, editable: false })) } });
    if (url.pathname === "/api/assertions/source") {
      const source_id = url.searchParams.get("source");
      reads.push(source_id);
      expect(sources.map(source => source.source_id)).toContain(source_id);
      const salary = source_id === "company_directory" ? "50000" : "60000";
      return route.fulfill({ json: {
        source_id, source_name: source_id, organization_id: "company", organization_name: "Company", kind: "ldap", people: 1, groups: [], attributes: [{ name: "salary", people: 1, groups: 0 }],
        directory: { people: ["alice"], groups: {}, attributes: ["salary"], person_attributes: { alice: { salary: [salary] } }, group_attributes: {} },
      } });
    }
    if (url.pathname === "/api/snapshot") return route.fulfill({ json: { api_status: "connected", errors: [], stats: {}, actors: [], services: [], network: [], trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0 } });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ status: 503, json: { error: "Production integration not configured" } });
    const file = resolve(assets, url.pathname === "/" ? "index.html" : url.pathname.slice(1));
    if (!file.startsWith(assets.endsWith(sep) ? assets : assets + sep)) return route.fulfill({ status: 403 });
    try {
      return route.fulfill({ body: await readFile(file), contentType: ({ html: "text/html", js: "application/javascript", css: "text/css", woff2: "font/woff2" })[file.split(".").at(-1)] || "application/octet-stream", headers: { "Content-Security-Policy": editorCSP } });
    } catch { return route.fulfill({ status: 404 }); }
  });
  await page.goto(origin + "/#sources");
  const browser = page.locator("#page-sources trusted-source-browser");
  const selector = browser.getByRole("combobox", { name: "Trusted source" });
  for (const [source, salary, otherSalary] of [["company_directory", "50000", "60000"], ["evaluation_directory", "60000", "50000"]]) {
    await selector.selectOption(source);
    await browser.getByRole("tab", { name: "People", exact: true }).click();
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("alice");
    await browser.getByRole("tab", { name: "Attributes", exact: true }).click();
    await expect(browser.locator("[data-source-meta]")).toContainText("company · " + source);
    const table = browser.locator(".trusted-source-results .trusted-source-table");
    await expect(table).toContainText(salary);
    await expect(table).not.toContainText(otherSalary);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    const path = testInfo.outputPath(source + ".png");
    await browser.screenshot({ path });
    await testInfo.attach(source, { path, contentType: "image/png" });
  }
  expect(new Set(reads)).toEqual(new Set(sources.map(source => source.source_id)));
  expect(forbidden).toEqual([]);
  expect(errors).toEqual([]);
});
