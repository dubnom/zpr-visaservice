import { readFileSync } from "node:fs";

const sourceFixture = name => JSON.parse(readFileSync(new URL(`../../cmd/zpr-web-dashboard/testdata/trusted-sources/${name}`, import.meta.url), "utf8"));

export function registerTrustedSourceGUITests(test, expect) {
  test("Trusted Sources GUI names the source and gives known managers a dedicated button", async ({ page, appURL, api }) => {
    api.assertionSource.source_name = "Trusted sources";
    api.assertionSource.default_source = "great_lakes_ldap";
    api.snapshot.trusted_sources = [
      { name: "great_lakes_ldap", provider: "LDAP", editor_url: `${appURL}/?manager=ldap` },
      { name: "unmanaged", provider: "file" },
    ];
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator("[data-source-title]")).toHaveText("Great Lakes LDAP");
    await expect(browser.locator("[data-source-meta]")).toContainText("alpha · great_lakes_ldap");
    const rows = page.locator("#trusted-list tr");
    await expect(rows.first().locator(".source-primary")).toHaveText("great_lakes_ldap");
    await expect(rows.first().locator(".source-primary small")).toHaveCount(0);
    const manager = rows.first().locator("td").last().getByRole("link", { name: "Manage", exact: true });
    await expect(manager).toHaveClass(/button/);
    await expect(manager).toContainText("↗");
    await expect(manager).toHaveAttribute("href", `${appURL}/?manager=ldap`);
    await expect(manager).toHaveAttribute("target", "zpr-directory-manager");
    const assertReadable = async () => {
      const contrast = await manager.evaluate(link => {
        const channels = value => value.match(/[\d.]+/g).slice(0, 3).map(Number);
        const luminance = value => channels(value).map(channel => {
          const normalized = channel / 255;
          return normalized <= 0.04045 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
        }).reduce((sum, channel, index) => sum + channel * [0.2126, 0.7152, 0.0722][index], 0);
        const background = luminance(getComputedStyle(link).backgroundColor);
        return [...link.querySelectorAll("span")].map(span => {
          const foreground = luminance(getComputedStyle(span).color);
          return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05);
        });
      });
      for (const ratio of contrast) expect(ratio).toBeGreaterThanOrEqual(4.5);
    };
    await assertReadable();
    await manager.hover();
    await assertReadable();
    await page.mouse.move(0, 0);
    await manager.focus();
    await expect(manager).toBeFocused();
    await assertReadable();
    await expect(rows.last().getByRole("link")).toHaveCount(0);
    await expect(page.locator('#page-sources th').filter({ hasText: /^MANAGE$/ })).toHaveCSS("font-weight", "700");
  });


  test("Trusted Sources GUI keeps source selection coherent, isolates delayed reads, and scopes updates", async ({ page, appURL, api }) => {
    const status = sourceFixture("source-status.json");
    const company = sourceFixture("source-read-company.json");
    const lab = sourceFixture("source-read-lab.json");
    api.assertions.organization_id = "northstar";
    api.assertions.organization_name = "Northstar";
    api.assertions.default_source = "company_directory";
    api.assertions.sources = status.sources;
    api.assertionSource = { ...company, organization_name: "Northstar", people: 1, groups: [{ name: "Operators", members: 1 }], attributes: [{ name: "department", people: 1, groups: 0 }] };
    api.snapshot.trusted_sources = status.sources.map((source, index) => ({ name: source.source_id, provider: "LDAP", health: index ? "failed" : "working", last_success_ms: Date.parse(index ? "2026-10-23T14:45:00Z" : "2026-10-23T14:50:00Z") }));
    api.trustedChangeFeeds = [{ name: "company_directory", source_id: "company_directory", display_name: "Company directory" }];
    api.handlers.set("/api/assertions/evaluate", route => route.fulfill({ json: { organization_id: "northstar", revision: 0, draft: false, status: "success", finished_at: "2026-10-23T14:50:00Z", results: [], warnings: [] } }));
    let releaseCompany;
    const companyPending = new Promise(resolve => { releaseCompany = resolve; });
    let delayCompany = true;
    let failLab = false;
    api.handlers.set("/api/assertions/source", async route => {
      const query = new URL(route.request().url()).searchParams;
      const sourceID = query.get("source");
      if (sourceID === "company_directory" && delayCompany) {
        await companyPending;
      }
      if (sourceID === "lab_directory" && failLab) {
        await route.fulfill({ status: 502, json: { error: "Lab directory is unavailable; no fallback was attempted" } });
        return;
      }
      const fixture = sourceID === "lab_directory" ? lab : company;
      const directory = structuredClone(fixture.directory);
      if (sourceID === "company_directory") directory.person_attributes.alice.cn = ["Alice Company"];
      await route.fulfill({ json: {
        ...fixture, organization_id: api.assertions.organization_id, source_id: sourceID,
        source_name: sourceID, organization_name: api.assertions.organization_name,
        people: 1, groups: [{ name: "Operators", members: directory.groups.Operators.length }],
        attributes: directory.attributes.map(name => ({ name, people: 1, groups: 0 })), directory,
      } });
    });

    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    const selector = browser.getByRole("combobox", { name: "Trusted source" });
    await expect(selector).toBeVisible();
    await expect(selector.locator("option")).toHaveCount(2);
    await expect(selector.locator("option").nth(0)).toContainText("company_directory");
    await expect(selector.locator("option").nth(1)).toContainText("lab_directory");
    const configuredRows = browser.locator("[data-source-registry] tbody tr");
    await expect(configuredRows).toHaveCount(2);
    await expect(configuredRows.nth(0)).toContainText("working");
    await expect(configuredRows.nth(1)).toContainText("failed");
    await expect(configuredRows.nth(1)).not.toContainText("Not reported");
    await expect.poll(() => api.counts.get("/api/assertions/source") || 0).toBe(1);
    await selector.selectOption("lab_directory");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("Alice Lab");
    await expect(browser.locator("[data-source-meta]")).toContainText("northstar · lab_directory · ldap");
    await expect(browser.locator("[data-source-meta]")).toContainText("Last success");
    await browser.getByRole("tab", { name: "Groups" }).click();
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("bob");
    await browser.getByRole("tab", { name: "Attributes" }).click();
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("70000");
    await browser.getByRole("tab", { name: "People" }).click();
    releaseCompany();
    delayCompany = false;
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).not.toContainText("Alice Company");

    await page.setViewportSize({ width: 768, height: 1024 });
    await selector.focus();
    await expect(selector).toBeFocused();
    const noHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
    expect(noHorizontalOverflow).toBe(true);
    await browser.locator("[data-source-tab='groups']").focus();
    await page.keyboard.press("ArrowRight");
    await expect(browser.locator("[data-source-tab='attributes']")).toBeFocused();
    await page.setViewportSize({ width: 1280, height: 900 });

    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    await expect(browser.locator("[data-source-message]")).toContainText("Updates are unsupported for this read-only source");
    await expect(browser.locator(".trusted-source-changes")).toHaveCount(0);

    await page.evaluate(() => { location.hash = "#policy"; });
    await page.locator("#policy-picker-toggle").click();
    await expect(page.locator('[data-record-id="test-assertions"]')).toBeVisible();
    await page.locator('[data-record-id="test-assertions"]').click();
    await expect(page.locator("#policy-assertion-editor")).toBeVisible();
    const assertionSource = page.locator("#assertion-catalog-source");
    await expect(assertionSource).toHaveValue("lab_directory");
    await expect(page.locator("#assertion-source-summary")).toContainText("1 people / 1 groups");
    await page.locator('.code-editor-host[data-source="assertion-source"] .cm-content').fill("assert true;");
    await page.locator("#assertion-analyze").click();
    await expect(page.locator("#assertion-run-status")).toContainText("SUCCESS");
    await assertionSource.selectOption("company_directory");
    await expect(page.locator("#assertion-run-status")).toHaveText("Not evaluated");
    await expect.poll(() => page.locator("#assertion-source-summary").textContent()).toContain("1 people / 1 groups");

    await page.evaluate(() => { location.hash = "#sources"; });
    await browser.getByRole("tab", { name: "People" }).click();
    failLab = true;
    await selector.selectOption("lab_directory");
    await expect(browser.locator("[data-source-message]")).toContainText("no fallback was attempted");
    await expect(configuredRows.nth(1)).toContainText("unavailable");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toHaveCount(0);
    await selector.selectOption("company_directory");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("Alice Company");
    failLab = false;
    await selector.selectOption("lab_directory");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("Alice Lab");
    api.snapshot.trusted_sources[1].health = "working";
    api.snapshot.trusted_sources[1].last_success_ms = Date.now();
    await page.evaluate(snapshot => document.dispatchEvent(new CustomEvent("control-room:refreshed", { detail: snapshot })), api.snapshot);
    await expect(configuredRows.nth(1)).toContainText("working");
  });

  test("Trusted Sources GUI resets source data when organization scope changes or has no sources", async ({ page, appURL, api }) => {
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("alice");
    api.assertions.organization_id = "beta";
    api.assertions.organization_name = "Beta Labs";
    api.assertions.default_source = "beta_directory";
    api.assertions.sources = [{ name: "beta_directory", source_id: "beta_directory", organization_id: "beta", display_name: "Beta directory", kind: "ldap" }];
    api.handlers.set("/api/assertions/source", route => route.fulfill({ json: {
      ...api.assertionSource, organization_id: "beta", organization_name: "Beta Labs", source_id: "beta_directory", source_name: "beta_directory",
      directory: { people: ["casey"], groups: { BetaOperators: ["casey"] }, attributes: ["uid"], person_attributes: { casey: { uid: ["casey"] } }, group_attributes: {} },
    } }));
    await page.locator("#refresh-now").click();
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("casey");
    await expect(browser.locator("[data-source-meta]")).toContainText("beta · beta_directory");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).not.toContainText("alice");

    api.assertions.sources = [];
    api.assertions.default_source = "";
    await page.locator("#refresh-now").click();
    await expect(browser.locator("[data-source-message]")).toHaveText("No trusted sources are configured for this organization.");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toHaveCount(0);
  });

    test("Trusted Sources GUI uses a single feed name as its heading without redundant controls", async ({ page, appURL, api }) => {
    api.assertionSource.source_name = "great_lakes_ldap";
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator("[data-source-title]")).toHaveText("Great Lakes LDAP");
    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    await expect(browser.locator(".trusted-source-changes")).toContainText("uid=alice");
    await expect(browser.locator("[data-source-title]")).toHaveText("Great Lakes LDAP");
    await expect(browser.getByRole("combobox")).toBeHidden();
    await expect(browser.locator("[data-source-feed]")).toHaveCount(0);
    await expect(browser).not.toContainText("Update feed");
    await expect(browser.locator("[data-source-meta]")).toBeHidden();
    await browser.getByRole("button", { name: "Load more updates" }).click();
    await expect(browser.locator(".trusted-source-changes tbody tr")).toHaveCount(2);
    await browser.getByRole("tab", { name: "People" }).click();
    await expect(browser.locator("[data-source-title]")).toHaveText("Great Lakes LDAP");
    await expect(browser.locator("[data-source-meta]")).toBeVisible();
    await expect(browser.locator("[data-source-update-controls]")).toBeHidden();
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("alice");
    expect(api.counts.get("/api/assertions/source")).toBe(1);
    expect(api.counts.get("/api/trusted-sources/change-feeds")).toBe(1);
  });

  test("Trusted Sources GUI selects multiple feeds in the heading and resets cursor on source changes", async ({ page, appURL, api }) => {
    api.assertionSource.source_name = "second";
    api.assertionSource.source_id = "second";
    api.assertions.default_source = "second";
    api.assertions.sources = ["first", "second"].map(name => ({ name, source_id: name, organization_id: "alpha", display_name: name, kind: "ldap" }));
    api.trustedChangeFeeds = [{ name: "first", source_id: "first", display_name: "First source" }, { name: "second", source_id: "second", display_name: "Second source" }];
    const queries = [];
    for (const name of ["first", "second"]) {
      api.handlers.set(`/api/trusted-sources/change-feeds/${name}/changes`, route => {
        const cursor = new URL(route.request().url()).searchParams.get("cursor");
        queries.push({ name, cursor });
        return route.fulfill({ json: {
          changes: [{ time: "2026-10-09T12:00:00Z", type: "modify", dn: `cn=${name}`, attributes: ["mail"] }],
          cursor: `${name}-cursor`, more: !cursor,
        } });
      });
    }
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator("[data-source-title]")).toHaveText("second");
    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    const selector = browser.getByRole("combobox", { name: "Trusted source", exact: true });
    await expect(selector).toBeVisible();
    await expect(selector).toHaveValue("second");
    await expect(browser.locator("[data-source-title]")).toBeHidden();
    await expect(browser.locator(".trusted-source-changes")).toContainText("cn=second");
    await browser.getByRole("button", { name: "Load more updates" }).click();
    await expect(browser.locator(".trusted-source-changes tbody tr")).toHaveCount(2);
    await selector.selectOption("first");
    await expect(browser.locator(".trusted-source-changes tbody tr")).toHaveCount(1);
    await expect(browser.locator(".trusted-source-changes")).toContainText("cn=first");
    await expect(browser.locator(".trusted-source-changes")).not.toContainText("cn=second");
    await selector.selectOption("second");
    await expect(browser.locator(".trusted-source-changes")).toContainText("cn=second");
    expect(queries).toEqual([
      { name: "second", cursor: null }, { name: "second", cursor: "second-cursor" },
      { name: "first", cursor: null }, { name: "second", cursor: null },
    ]);
    await browser.getByRole("tab", { name: "Groups" }).click();
    await expect(selector).toBeVisible();
    await expect(selector).toHaveValue("second");
    await expect(browser.locator("[data-source-title]")).toBeHidden();
  });

  test("Trusted Sources GUI keeps empty feeds explicit and never refreshes LDAP on snapshot polls", async ({ page, appURL, api }) => {
    api.trustedChangeFeeds = [];
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("alice");
    await page.locator("#pause-poll").click();
    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    await expect(browser.locator("[data-source-message]")).toContainText("Updates are unsupported for this read-only source");
    await expect(browser.getByRole("combobox")).toBeHidden();
    await page.evaluate(snapshot => document.dispatchEvent(new CustomEvent("control-room:refreshed", { detail: snapshot })), api.snapshot);
    expect(api.counts.get("/api/assertions/source")).toBe(1);
    await browser.getByRole("tab", { name: "People" }).click();
    await page.locator("#refresh-now").click();
    await expect.poll(() => api.counts.get("/api/assertions/source")).toBe(2);
  });

  test("Trusted Sources GUI preserves source identity on failed and expired reads and recovers with manual Refresh", async ({ page, appURL, api }) => {
    let status = 503;
    api.handlers.set("/api/trusted-sources/change-feeds/great_lakes_ldap/changes", route => route.fulfill({
      status, json: status === 200 ? api.trustedChanges : { error: "Directory feed is unavailable" },
    }));
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("alice");
    await page.locator("#pause-poll").click();
    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    await expect(browser.locator("[data-source-message]")).toHaveText("Directory feed is unavailable");
    await expect(browser.locator("[data-source-title]")).toHaveText("Great Lakes LDAP");
    await expect(browser.locator("[data-source-results]")).toBeHidden();
    await expect(browser.getByRole("button", { name: "Load more updates" })).toBeHidden();
    status = 410;
    await page.locator("#refresh-now").click();
    await expect(browser.locator("[data-source-message]")).toContainText("update history expired");
    status = 200;
    await page.locator("#refresh-now").click();
    await expect(browser.locator(".trusted-source-changes")).toContainText("uid=alice");
    expect(api.counts.get("/api/assertions/source")).toBe(3);
  });

  test("Trusted Sources GUI keeps a delayed update response from replacing a directory tab", async ({ page, appURL, api }) => {
    let release;
    const delayed = new Promise(resolve => { release = resolve; });
    api.handlers.set("/api/trusted-sources/change-feeds/great_lakes_ldap/changes", async route => {
      await delayed;
      await route.fulfill({ json: api.trustedChanges });
    });
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("alice");
    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    await expect.poll(() => api.counts.get("/api/trusted-sources/change-feeds/great_lakes_ldap/changes") || 0).toBe(1);
    await browser.getByRole("tab", { name: "People" }).click();
    const response = page.waitForResponse(response => response.url().includes("/great_lakes_ldap/changes"));
    release();
    await response;
    await expect(browser.locator(".trusted-source-results .trusted-source-table")).toContainText("alice");
    await expect(browser.locator(".trusted-source-changes")).toHaveCount(0);
    await expect(browser.locator("[data-source-update-controls]")).toBeHidden();
    await expect(browser.locator("[data-source-title]")).toHaveText("Great Lakes LDAP");
  });
}
