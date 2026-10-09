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
    await expect(browser.locator("[data-source-title]")).toHaveText("great_lakes_ldap");
    const rows = page.locator("#trusted-list tr");
    await expect(rows.first().locator(".source-primary")).toHaveText("great_lakes_ldap");
    await expect(rows.first().locator(".source-primary small")).toHaveCount(0);
    const manager = rows.first().locator("td").last().getByRole("link", { name: "Manage", exact: true });
    await expect(manager).toHaveClass(/button/);
    await expect(manager).toContainText("↗");
    await expect(manager).toHaveAttribute("href", `${appURL}/?manager=ldap`);
    await expect(manager).toHaveAttribute("target", "zpr-directory-manager");
    await expect(rows.last().getByRole("link")).toHaveCount(0);
    await expect(page.locator('#page-sources th').filter({ hasText: /^MANAGE$/ })).toHaveCSS("font-weight", "700");
  });

  test("Trusted Sources GUI uses a single feed name as its heading without redundant controls", async ({ page, appURL, api }) => {
    api.assertionSource.source_name = "great_lakes_ldap";
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator("[data-source-title]")).toHaveText("great_lakes_ldap");
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
    await expect(browser.locator("[data-source-title]")).toHaveText("great_lakes_ldap");
    await expect(browser.locator("[data-source-meta]")).toBeVisible();
    await expect(browser.locator("[data-source-update-controls]")).toBeHidden();
    await expect(browser.locator(".trusted-source-table")).toContainText("alice");
    expect(api.counts.get("/api/assertions/source")).toBe(1);
    expect(api.counts.get("/api/trusted-sources/change-feeds")).toBe(1);
  });

  test("Trusted Sources GUI selects multiple feeds in the heading and resets cursor on source changes", async ({ page, appURL, api }) => {
    api.assertionSource.source_name = "second";
    api.trustedChangeFeeds = [{ name: "first", display_name: "First source" }, { name: "second", display_name: "Second source" }];
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
    await expect(selector).toBeHidden();
    await expect(browser.locator("[data-source-title]")).toHaveText("second");
  });

  test("Trusted Sources GUI keeps empty feeds explicit and never refreshes LDAP on snapshot polls", async ({ page, appURL, api }) => {
    api.trustedChangeFeeds = [];
    await page.goto(`${appURL}/#sources`);
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator(".trusted-source-table")).toContainText("alice");
    await page.locator("#pause-poll").click();
    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    await expect(browser.locator("[data-source-message]")).toHaveText("No trusted-source change feeds are configured.");
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
    await expect(browser.locator(".trusted-source-table")).toContainText("alice");
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
    expect(api.counts.get("/api/assertions/source")).toBe(1);
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
    await expect(browser.locator(".trusted-source-table")).toContainText("alice");
    await browser.getByRole("tab", { name: "Updates (24h)" }).click();
    await expect.poll(() => api.counts.get("/api/trusted-sources/change-feeds/great_lakes_ldap/changes") || 0).toBe(1);
    await browser.getByRole("tab", { name: "People" }).click();
    const response = page.waitForResponse(response => response.url().includes("/great_lakes_ldap/changes"));
    release();
    await response;
    await expect(browser.locator(".trusted-source-table")).toContainText("alice");
    await expect(browser.locator(".trusted-source-changes")).toHaveCount(0);
    await expect(browser.locator("[data-source-update-controls]")).toBeHidden();
    await expect(browser.locator("[data-source-title]")).toHaveText("Trusted LDAP");
  });
}
