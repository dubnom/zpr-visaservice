export function registerOrganizationBrowserTests(test, expect) {
  test("Organization browser and File work after simulator navigation and returning to the page", async ({ page, appURL, api }) => {
    api.handlers.set("/api/simulator/scenarios", route => route.fulfill({ json: {
      active_organization_id: "alpha", organization: api.organizations.organizations[0],
      scenarios: [], run: { state: "idle", steps: [] },
    } }));
    await page.goto(appURL + "/scenarios.html");
    for (let visit = 0; visit < 2; visit++) {
      await page.locator('.primary-nav a[href="/organizations.html"]').click();
      await expect(page.locator("#organization-title")).toHaveText(visit ? "Beta Labs" : "Alpha Labs");
      await page.locator("#organization-browse").click();
      await expect(page.locator("#organization-browser-dialog")).toBeVisible();
      await page.locator('[data-organization-id="beta"]').click();
      await expect(page.locator("#organization-title")).toHaveText("Beta Labs");
      await page.locator("#organization-files-toggle").click();
      await expect(page.locator("#organization-file-menu")).toBeVisible();
      await page.getByRole("menuitem", { name: "New organization", exact: true }).click();
      await expect(page.locator("#organization-duplicate-dialog")).toBeVisible();
      await page.locator("[data-duplicate-close]").click();
      await page.locator('.primary-nav a[href="/scenarios.html"]').click();
    }
  });
  test("Organization layout gives the graph screen height and keeps configuration accessible", async ({ page, appURL, api }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    const graph = page.locator("#organization-identity ldap-org-graph > svg");
    const bounds = await graph.boundingBox();
    expect(bounds.y).toBeLessThan(320);
    expect(bounds.height).toBeGreaterThanOrEqual(600);
    await page.locator(".organization-runtime-details summary").click();
    await expect(page.locator(".organization-runtime-details")).toContainText("single-node");
    await page.locator(".organization-scenarios summary").click();
    await expect(page.locator("#organization-scenario-summary")).toBeVisible();
    await page.setViewportSize({ width: 390, height: 844 });
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.locator("#organization-browse").click();
    await expect(page.locator("#organization-browser-dialog")).toBeVisible();
  });
  const browser = page => page.getByRole("dialog", { name: "Browse organizations", exact: true });
  async function openFiles(page) {
    await page.locator("#organization-files-toggle").click();
    return page.locator("#organization-file-menu");
  }
  async function selectBundled(page, id) {
    await page.locator("#organization-browse").click();
    await browser(page).locator(`[data-organization-id="${id}"]`).click();
    await expect(browser(page)).not.toBeVisible();
  }
  function drafts(api) {
    const profile = { ...api.organizations.organizations[0], id: "private", name: "Private draft" };
    const state = { drafts: [{ id: profile.id, name: profile.name, revision: 1 }], profile, deletes: [] };
    api.handlers.set("/api/simulator/organization-drafts", route => route.fulfill({ json: { drafts: state.drafts } }));
    api.handlers.set("/api/simulator/organizations/private/draft/profile", route =>
      route.fulfill({ json: { revision: 1, content: profile } }));
    api.handlers.set("/api/simulator/organizations/private/draft", route => {
      state.deletes.push(route.request().postDataJSON());
      state.drafts = [];
      return route.fulfill({ status: 204 });
    });
    return state;
  }

  test("Organization browser selects without activation and keeps full-width details", async ({ page, appURL, api }) => {
    drafts(api);
    const mutations = [];
    page.on("request", request => {
      if (request.method() !== "GET" && request.url().includes("/api/simulator/organizations/")) mutations.push(request.url());
    });
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    await expect(page.locator("#organization-list")).not.toBeVisible();
    await expect(page.locator("#organization-drafts-list")).not.toBeVisible();
    await selectBundled(page, "beta");
    await expect(page.locator("#organization-title")).toHaveText("Beta Labs");
    await expect(page.locator("#organization-browse")).toBeFocused();
    await expect(page.locator('[data-activate-organization="beta"]')).toBeEnabled();
    expect(api.organizations.active_id).toBe("alpha");
    expect(mutations).toEqual([]);
    const layout = await page.locator(".organization-layout").boundingBox();
    const detail = await page.locator(".organization-detail").boundingBox();
    expect(Math.abs(layout.width - detail.width)).toBeLessThan(2);
    for (const width of [1280, 820, 390]) {
      await page.setViewportSize({ width, height: 844 });
      await page.locator("#organization-browse").click();
      await expect(browser(page).locator('[data-organization-id="beta"]')).toHaveAttribute("aria-pressed", "true");
      await expect.poll(() => browser(page).evaluate(element => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth + 1 && element.scrollWidth <= element.clientWidth + 1;
      })).toBe(true);
      await page.keyboard.press("Escape");
      await expect(page.locator("#organization-browse")).toBeFocused();
    }
    await page.locator('[data-activate-organization="beta"]').click();
    const confirmation = page.getByRole("dialog", { name: "Switch organization?", exact: true });
    await expect(confirmation).toBeVisible();
    expect(mutations).toEqual([]);
    api.handlers.set("/api/simulator/organizations/beta/activate", route => {
      api.organizations.active_id = "beta";
      api.organizations.activation = { state: "completed", organization_id: "beta" };
      return route.fulfill({ status: 202, json: { activation: api.organizations.activation } });
    });
    await confirmation.getByRole("button", { name: "Switch organization", exact: true }).click();
    await expect(confirmation).not.toBeVisible();
    await expect(page.locator(".organization-active-status")).toHaveText("Active");
    expect(api.organizations.active_id).toBe("beta");
    expect(mutations).toHaveLength(1);
  });

  test("Organization browser File menu uses shared keyboard controls and creation workflow", async ({ page, appURL, api }) => {
    drafts(api);
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    const menu = await openFiles(page);
    await expect(menu.getByRole("menuitem", { name: "New organization", exact: true })).toBeFocused();
    await page.keyboard.press("End");
    await expect(menu.getByRole("menuitem", { name: "Restore", exact: true })).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(menu).not.toBeVisible();
    await expect(page.locator("#organization-files-toggle")).toBeFocused();
    await openFiles(page);
    await menu.getByRole("menuitem", { name: "New organization", exact: true }).click();
    await expect(menu).not.toBeVisible();
    const creation = page.locator("#organization-duplicate-dialog");
    await expect(creation).toBeVisible();
    await expect(creation.locator("[data-create-mode='new']")).toHaveAttribute("aria-pressed", "true");
    await creation.locator("[data-duplicate-close]").click();
    await openFiles(page);
    await menu.getByRole("menuitem", { name: "Duplicate", exact: true }).click();
    await expect(creation.locator("#organization-duplicate-source")).toHaveValue("alpha");
    await creation.locator("[data-duplicate-close]").click();
    await openFiles(page);
    await menu.getByRole("menuitem", { name: "Restore", exact: true }).click();
    await expect(page.locator("#organization-restore-dialog")).toBeVisible();
    expect(api.counts.get("/api/simulator/organizations/alpha/restore-base") || 0).toBe(0);
  });

  test("Organization browser preserves private draft selection and File protections through polling and deletion", async ({ page, appURL, api }) => {
    const state = drafts(api);
    await page.goto(appURL + "/organizations.html");
    await page.locator("#organization-browse").click();
    await browser(page).locator('[data-organization-draft-id="private"]').click();
    await expect(browser(page)).not.toBeVisible();
    await expect(page.locator("#organization-title")).toHaveText("Private draft");
    await expect(page.locator("[data-activate-organization]")).toHaveCount(0);
    await page.locator("#organization-browse").click();
    await expect(browser(page).locator("[aria-pressed=true]")).toHaveCount(1);
    await expect(browser(page).locator('[data-organization-draft-id="private"]')).toHaveAttribute("aria-pressed", "true");
    await page.locator("#organization-drafts-refresh").click();
    await expect(browser(page)).toBeVisible();
    await page.keyboard.press("Escape");
    api.organizations.activation = { state: "completed", organization_id: "alpha" };
    await expect(page.locator("#organization-operation-status")).toContainText("completed", { timeout: 12000 });
    await expect(page.locator("#organization-title")).toHaveText("Private draft");
    let menu = await openFiles(page);
    for (const name of ["Duplicate", "Restore"]) await expect(menu.getByRole("menuitem", { name, exact: true })).toBeDisabled();
    for (const name of ["Edit", "Rename", "Delete"]) await expect(menu.getByRole("menuitem", { name, exact: true })).toBeEnabled();
    await menu.getByRole("menuitem", { name: "Delete", exact: true }).click();
    await expect(page.locator("#organization-delete-dialog")).toBeVisible();
    expect(state.deletes).toEqual([]);
    await page.locator("[data-confirm-organization-delete]").click();
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    expect(state.deletes).toEqual([{ expected_revision: 1 }]);
    menu = await openFiles(page);
    for (const name of ["Rename", "Delete"]) await expect(menu.getByRole("menuitem", { name, exact: true })).toBeDisabled();
  });

  test("Organization browser reports failed draft loads and rejects dismissed responses", async ({ page, appURL, api }) => {
    drafts(api);
    const path = "/api/simulator/organizations/private/draft/profile";
    api.handlers.set(path, route => route.fulfill({ status: 503, json: { error: "Draft store unavailable" } }));
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    await page.locator("#organization-browse").click();
    await browser(page).locator('[data-organization-draft-id="private"]').click();
    await expect(browser(page).getByRole("alert")).toHaveText("Draft store unavailable");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    const profile = { ...api.organizations.organizations[0], id: "private", name: "Late private draft" };
    api.handlers.set(path, async route => {
      await gate;
      await route.fulfill({ json: { revision: 1, content: profile } });
    });
    const before = api.counts.get(path);
    await browser(page).locator('[data-organization-draft-id="private"]').click();
    await expect.poll(() => api.counts.get(path)).toBe(before + 1);
    await page.keyboard.press("Escape");
    await selectBundled(page, "beta");
    const response = page.waitForResponse(result => new URL(result.url()).pathname === path);
    release();
    await response;
    await expect(page.locator("#organization-title")).toHaveText("Beta Labs");
    await expect(browser(page)).not.toBeVisible();
  });

  test("Organization browser handles empty catalogs and disables Restore during activation", async ({ page, appURL, api }) => {
    drafts(api);
    api.organizations.organizations = [];
    api.organizations.active_id = "";
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-detail")).toContainText("Choose an organization");
    const menu = await openFiles(page);
    await expect(menu.getByRole("menuitem", { name: "New organization", exact: true })).toBeEnabled();
    for (const name of ["Edit", "Duplicate", "Rename", "Restore", "Delete"]) {
      await expect(menu.getByRole("menuitem", { name, exact: true })).toBeDisabled();
    }
    await page.keyboard.press("Escape");
    await page.locator("#organization-browse").click();
    await expect(browser(page)).toContainText("No bundled organizations available.");
    await browser(page).locator('[data-organization-draft-id="private"]').click();
    await expect(page.locator("#organization-title")).toHaveText("Private draft");
    api.organizations.organizations = [{ id: "alpha", name: "Alpha Labs", directory: {}, policies: [], services: [] }];
    api.organizations.active_id = "alpha";
    api.organizations.activation = { state: "resetting", organization_id: "alpha" };
    await expect(page.locator("#organization-operation-status")).toContainText("in progress", { timeout: 12000 });
    await selectBundled(page, "alpha");
    await openFiles(page);
    await expect(menu.getByRole("menuitem", { name: "Restore", exact: true })).toBeDisabled();
  });
}
