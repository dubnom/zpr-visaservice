export function registerOrganizationNavigationTests(test, expect) {
  test("Simulator panels omit redundant headings and trusted source totals without removing records", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/organizations.html#organization-policy");
    await expect(page.locator("#organization-policy")).toBeVisible();
    await expect(page.locator("#organization-policy > h3")).toHaveCount(0);
    await expect(page.locator("#organization-policy .organization-list-items, #organization-policy .organization-empty")).toBeVisible();
    await page.getByRole("navigation", { name: "Organization sections", exact: true }).getByRole("link", { name: "Services", exact: true }).click();
    await expect(page.locator("#organization-services")).toBeVisible();
    await expect(page.locator("#organization-services > h3")).toHaveCount(0);
    await expect(page.locator("#organization-services .organization-list-items, #organization-services .organization-empty")).toBeVisible();
    await page.getByRole("navigation", { name: "Organization sections", exact: true }).getByRole("link", { name: "Trusted Sources", exact: true }).click();
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator('[data-source-title]')).toBeHidden();
    await expect(browser.locator('[data-source-summary]')).toBeHidden();
    await browser.getByRole("tab", { name: "People" }).click();
    await expect(browser.locator(".trusted-source-table")).toContainText("alice@example.test");
    await expect(browser.locator('[data-source-title]')).toBeHidden();
    await expect(browser.locator('[data-source-summary]')).toBeHidden();
    await page.reload();
    await expect(browser.locator('[data-source-title]')).toBeHidden();
    await expect(browser.locator('[data-source-summary]')).toBeHidden();
    expect(api.counts.get("/api/simulator/trusted-source")).toBeGreaterThan(0);
  });

  test("Browse and File controls share colors typography and chevrons across catalogs and editors", async ({ page, appURL, api }) => {
    api.handlers.set("/api/simulator/organizations/alpha/profile", route => route.fulfill({ json: {
      revision: 1, content: { id: "alpha", name: "Alpha Labs", description: "Fictional organization", runtime: {}, directory: {}, services: [], policies: [] },
    } }));
    api.handlers.set("/api/simulator/organizations/alpha/profile/revisions", route => route.fulfill({ json: [] }));
    const styles = async selector => {
      await expect(page.locator(selector)).toBeVisible();
      return page.locator(selector).evaluate(button => {
        const style = getComputedStyle(button);
        const chevron = getComputedStyle(button, "::after");
        const properties = ["color", "backgroundColor", "borderColor", "fontFamily", "fontSize", "fontWeight", "lineHeight", "minHeight", "padding"];
        for (const span of button.querySelectorAll("span:not(.policy-picker-chevron)")) {
          const label = getComputedStyle(span);
          for (const property of ["color", "fontFamily", "fontSize", "fontWeight"]) {
            if (label[property] !== style[property]) throw new Error(`Label ${property} differs from button`);
          }
        }
        return {
          ...Object.fromEntries(properties.map(property => [property, style[property]])),
          chevron: Object.fromEntries(["content", "width", "height", "borderRight", "borderBottom", "transform"].map(property => [property, chevron[property]])),
        };
      });
    };
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    const expected = await styles("#organization-browse");
    // Reference values measured from the deployed Control Room's visible labels.
    expect(expected.color).toBe("rgb(181, 227, 79)");
    expect(expected.backgroundColor).toBe("rgb(23, 33, 30)");
    expect(expected.fontFamily).toBe('"Avenir Next", "Segoe UI", sans-serif');
    expect(expected.fontSize).toBe("16px");
    expect(expected.fontWeight).toBe("400");
    expect(expected.minHeight).toBe("31px");
    expect(expected.chevron.content).toBe('"⌄" / ""');
    await expect(page.locator("#organization-browse")).toHaveAccessibleName("Browse...");
    await expect(page.locator("#organization-files-toggle")).toHaveAccessibleName("File...");
    expect(await styles("#organization-files-toggle")).toEqual(expected);
    await page.locator("#organization-files-toggle").click();
    await page.locator("#organization-file-menu").getByRole("menuitem", { name: "Edit", exact: true }).click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha Labs");
    expect(await styles("#organization-profile-files-toggle")).toEqual(expected);
    expect(await styles("#organization-profile-picker-toggle")).toEqual(expected);
    await page.goto(appURL + "/scenarios.html");
    for (const width of [1440, 820, 390]) {
      await page.setViewportSize({ width, height: 900 });
      for (const selector of ["#organization-browse", "#organization-files-toggle", "#scenario-browse", "#scenario-files-toggle"]) {
        expect(await styles(selector)).toEqual(expected);
      }
    }
    await page.locator("#scenario-files-toggle").click();
    await page.getByRole("menuitem", { name: "New Scenario...", exact: true }).click();
    expect(await styles("#scenario-editor-files-toggle")).toEqual(expected);
    for (const [hash, prefix] of [["policy", "policy"], ["gateways", "gateway"], ["zpr-config", "zpr-config"]]) {
      await page.goto(`${appURL}/#${hash}`);
      expect(await styles(`#${prefix}-picker-toggle`)).toEqual(expected);
      expect(await styles(`#${prefix}-files-toggle`)).toEqual(expected);
    }
    expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
  });

  test("Organization horizontal navigation groups sections and preserves deep links and history", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    const navigation = page.getByRole("navigation", { name: "Organization sections", exact: true });
    await expect(navigation.getByRole("link")).toHaveText(["Scenarios", "Identity", "Trusted Sources", "Policy", "Services"]);
    await expect(page.locator('.primary-nav a[href="/scenarios.html"], .primary-nav a[href="/trusted-source.html"]')).toHaveCount(0);
    await expect(page.locator('.primary-nav a[href="/organizations.html"]')).toHaveClass(/active/);
    await expect(page.getByRole("button", { name: "Design with Claude", exact: true })).toHaveCount(0);
    await expect(page.locator("#organization-design-dialog")).toHaveCount(0);
    await expect(page.locator("#organization-identity")).toBeVisible();
    await expect(page.locator("#organization-policy")).toBeHidden();
    for (const [label, id] of [["Policy", "organization-policy"], ["Services", "organization-services"]]) {
      await navigation.getByRole("link", { name: label, exact: true }).click();
      await expect(navigation.getByRole("link", { name: label, exact: true })).toHaveAttribute("aria-current", "page");
      await expect(page.locator(`#${id}`)).toBeVisible();
      await expect(page.locator("#organization-identity")).toBeHidden();
      await page.reload();
      await expect(page.locator(`#${id}`)).toBeVisible();
    }
    await page.goBack();
    await expect(page.locator("#organization-policy")).toBeVisible();
    await navigation.getByRole("link", { name: "Scenarios", exact: true }).click();
    await expect(page.locator("#scenario-browse")).toBeVisible();
    await expect(navigation.getByRole("link", { name: "Scenarios", exact: true })).toHaveAttribute("aria-current", "page");
    await expect(page.locator('.primary-nav a[href="/organizations.html"]')).toHaveClass(/active/);
    await navigation.getByRole("link", { name: "Trusted Sources", exact: true }).click();
    await expect(page.locator("trusted-source-browser")).toBeVisible();
    await expect(navigation.getByRole("link", { name: "Trusted Sources", exact: true })).toHaveAttribute("aria-current", "page");
    await navigation.getByRole("link", { name: "Identity", exact: true }).click();
    await expect(page.locator("#organization-identity")).toBeVisible();
    await page.locator('.primary-nav a[href="/activity.html"]').click();
    await expect(navigation).toHaveCount(0);
    await page.locator('.primary-nav a[href="/organizations.html"]').click();
    await expect(navigation).toHaveCount(1);
    await expect(navigation.getByRole("link", { name: "Identity", exact: true })).toHaveAttribute("aria-current", "page");
    for (const width of [820, 390]) {
      await page.setViewportSize({ width, height: 844 });
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await navigation.getByRole("link", { name: "Scenarios", exact: true }).scrollIntoViewIfNeeded();
      await expect(navigation.getByRole("link", { name: "Scenarios", exact: true })).toBeInViewport();
    }
    expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
  });

  test("Organization controls precede tabs and Simulator sidebar groups stay consistent across navigation", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    const navigation = page.locator(".organization-navigation");
    const checkGroups = async () => {
      const configuration = page.locator('[data-simulator-nav-group="CONFIGURATION"]');
      const monitoring = page.locator('[data-simulator-nav-group="MONITORING"]');
      await expect(configuration.locator(".nav-group-label")).toHaveText("CONFIGURATION");
      await expect(configuration.getByRole("link")).toHaveText(["Organizations"]);
      await expect(monitoring.locator(".nav-group-label")).toHaveText("MONITORING");
      await expect(monitoring.getByRole("link")).toHaveText(["Simulate", "Activity", "Workers"]);
      await expect(page.locator(".primary-nav .nav-group-label")).toHaveText(["MONITORING", "CONFIGURATION"]);
      await expect(page.locator(".primary-nav .nav-group-label")).toHaveCount(2);
      const separator = page.locator(".primary-nav > [data-simulator-nav-separator]");
      await expect(separator).toHaveCount(1);
      await expect(separator).toHaveAttribute("role", "separator");
      await expect(separator).toHaveClass("nav-separator");
      expect(await separator.evaluate(element =>
        element.previousElementSibling?.dataset.simulatorNavGroup === "MONITORING" &&
        element.nextElementSibling?.dataset.simulatorNavGroup === "CONFIGURATION")).toBe(true);
      const bounds = await separator.boundingBox();
      expect(bounds.width).toBeGreaterThan(0);
      expect(bounds.height).toBeGreaterThan(0);
    };
    const checkControls = async () => {
      const tabs = await navigation.boundingBox();
      for (const selector of ["#organization-browse", "#organization-files-toggle", "#organization-title", "[data-organization-activation-control]"]) {
        const control = await page.locator(selector).boundingBox();
        expect(control.y + control.height).toBeLessThanOrEqual(tabs.y);
        expect(await page.locator(selector).evaluate(element =>
          Boolean(element.compareDocumentPosition(document.querySelector(".organization-navigation")) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
      }
      await expect(navigation.getByRole("link").first()).toHaveText("Scenarios");
      const description = page.locator("#organization-description");
      await expect(description).toHaveText(/.+/);
      const bounds = await description.boundingBox();
      expect(bounds.y + bounds.height).toBeLessThanOrEqual(tabs.y);
      for (const selector of ["#organization-browse", "#organization-files-toggle", "#organization-title", "[data-organization-activation-control]"]) {
        const control = await page.locator(selector).boundingBox();
        expect(control.y + control.height).toBeLessThanOrEqual(bounds.y);
      }
      await expect(page.locator(".organization-scenarios, #organization-scenario-summary")).toHaveCount(0);
      await expect(page.locator(".organization-directory-counts")).toHaveCount(0);
      await expect(page.locator(".organization-directory-heading")).not.toContainText(/\d+ (departments|people|groups)/);
    };
    for (const width of [1440, 820, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await checkControls();
      await checkGroups();
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    }
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.locator("#organization-browse").click();
    await page.locator('[data-organization-id="beta"]').click();
    await expect(page.locator('[data-activate-organization="beta"]')).toBeEnabled();
    await checkControls();
    for (const label of ["Scenarios", "Trusted Sources", "Policy", "Services", "Identity"]) {
      await navigation.getByRole("link", { name: label, exact: true }).click();
      await expect(navigation.getByRole("link", { name: label, exact: true })).toHaveAttribute("aria-current", "page");
      await checkGroups();
      await checkControls();
    }
    await expect(page.locator("#organization-title")).toHaveText("Beta Labs");
    await checkControls();
    for (const path of ["/activity.html", "/machine-logs.html", "/organizations.html"]) {
      await page.locator(`.primary-nav a[href="${path}"]`).click();
      await expect(page).toHaveURL(appURL + path);
      await checkGroups();
    }
    await checkControls();
    await page.reload();
    await checkGroups();
    await checkControls();
    expect(api.organizations.active_id).toBe("alpha");
  });

  for (const path of ["/scenarios.html", "/trusted-source.html"]) {
    test(`Organization header is functional on direct entry to ${path} and persists across tabs`, async ({ page, appURL, api }) => {
      api.handlers.set("/api/simulator/organizations/beta/profile", route => route.fulfill({
        json: { revision: 1, content: api.organizations.organizations[1] },
      }));
      api.handlers.set("/api/simulator/organizations/beta/profile/revisions", route => route.fulfill({ json: [] }));
      await page.goto(appURL + path);
      await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
      const navigation = page.locator(".organization-navigation");
      await expect(navigation.getByRole("link")).toHaveText(["Scenarios", "Identity", "Trusted Sources", "Policy", "Services"]);
      await expect(page.locator("#organization-detail")).toBeHidden();
      await page.locator("#organization-browse").click();
      await page.locator('[data-organization-id="beta"]').click();
      await expect(page.locator("#organization-title")).toHaveText("Beta Labs");
      await expect(page.locator('[data-activate-organization="beta"]')).toBeEnabled();
      await page.locator('[data-activate-organization="beta"]').click();
      const switchDialog = page.getByRole("dialog", { name: "Switch organization?", exact: true });
      await expect(switchDialog).toBeVisible();
      await switchDialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await page.locator("#organization-files-toggle").click();
      await page.locator("#organization-file-menu").getByRole("menuitem", { name: "Edit", exact: true }).click();
      await expect(page.locator("#organization-profile-editor")).toBeVisible();
      await expect(page.locator("#organization-profile-name")).toHaveValue("Beta Labs");
      await page.locator("[data-profile-close]").click();
      for (const label of ["Identity", "Policy", "Scenarios", "Services", "Trusted Sources"]) {
        await navigation.getByRole("link", { name: label, exact: true }).click();
        await expect(navigation.getByRole("link", { name: label, exact: true })).toHaveAttribute("aria-current", "page");
        await expect(page.locator("#organization-title")).toHaveText("Beta Labs");
        await expect(page.locator("#organization-workspace")).toHaveCount(1);
        const bounds = await navigation.boundingBox();
        for (const selector of ["#organization-browse", "#organization-files-toggle", "#organization-title", "[data-organization-activation-control]", "#organization-description"]) {
          const control = await page.locator(selector).boundingBox();
          expect(control.y + control.height).toBeLessThanOrEqual(bounds.y);
        }
        if (["Identity", "Policy", "Services"].includes(label)) await expect(page.locator("#organization-detail")).toBeVisible();
        else await expect(page.locator("#organization-detail")).toBeHidden();
      }
      expect(api.organizations.active_id).toBe("alpha");
      expect(api.counts.get("/api/simulator/organizations/beta/activate") || 0).toBe(0);
    });
  }

  test("Simulator zoom and dismiss symbols are large and remain keyboard operable", async ({ page, appURL, api }) => {
    api.organizations.organizations[0].directory.departments = [{ name: "Engineering" }];
    await page.goto(appURL + "/organizations.html");
    const graph = page.locator("#organization-identity ldap-org-graph");
    await expect(graph).toBeVisible();
    const world = graph.locator(":scope > svg > g");
    const initial = await world.getAttribute("transform");
    for (const name of ["Zoom in", "Zoom out"]) {
      const button = graph.getByRole("button", { name, exact: true });
      await expect(button).toHaveCSS("font-size", "26px");
      await expect(button).toHaveCSS("font-weight", "700");
      const bounds = await button.boundingBox();
      expect(bounds.width).toBeGreaterThanOrEqual(36);
      expect(bounds.height).toBeGreaterThanOrEqual(36);
      await button.focus();
      await expect(button).toHaveCSS("outline-width", "2px");
      await button.press("Enter");
      if (name === "Zoom in") await expect(world).not.toHaveAttribute("transform", initial);
    }
    await expect(graph.locator(".ldap-graph-toggle text").first()).toHaveCSS("font-size", "20px");
    await expect(graph.locator(".ldap-graph-toggle text").first()).toHaveCSS("font-weight", "700");
    await page.locator("#organization-browse").click();
    const close = page.getByRole("button", { name: "Close organization browser", exact: true });
    await expect(close).toHaveCSS("font-size", "26px");
    await expect(close).toHaveCSS("font-weight", "700");
    await page.keyboard.press("Tab");
    await close.focus();
    await expect(close).toHaveCSS("outline-width", "2px");
    await close.press("Enter");
    await expect(page.locator("#organization-browser-dialog")).toBeHidden();
    await expect(page.locator("#organization-browse")).toBeFocused();
    await graph.locator(".ldap-graph-node").first().click();
    const infoClose = graph.getByRole("button", { name: "Close component info", exact: true });
    await expect(infoClose).toHaveCSS("font-size", "26px");
    await infoClose.click();
    await expect(graph.locator("dialog")).toBeHidden();
    await page.goto(appURL + "/scenarios.html");
    await page.locator("#scenario-files-toggle").click();
    await page.locator("#scenario-new").click();
    const editorClose = page.getByRole("button", { name: "Close editor", exact: true });
    await expect(editorClose).toHaveCSS("font-size", "26px");
    await page.locator("#scenario-add-step").click();
    const remove = page.getByRole("button", { name: "Remove step", exact: true }).last();
    await expect(remove).toHaveCSS("font-size", "26px");
    await expect(remove).toHaveCSS("font-weight", "700");
    await remove.click();
  });
}
