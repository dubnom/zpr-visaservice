export function registerScenarioBrowserTests(test, expect) {
  const scenario = (id, folder, published = 1) => ({
    id, name: `Scenario ${id}`, description: `Description for ${id}`, folder,
    organization_id: "alpha", current_revision: 1, published_revision: published,
    steps: [{ action: "delay", timeout_seconds: 1 }], cleanup: [],
  });
  const setup = (api, scenarios) => {
    const data = { active_organization_id: "alpha", scenarios, run: { state: "idle", steps: [] } };
    api.handlers.set("/api/simulator/scenarios", route => route.fulfill({ json: data }));
    return data;
  };

  test("Scenario browser selects folder files without running and leaves full-width tracks", async ({ page, appURL, api }) => {
    const data = setup(api, [scenario("one", "Finance"), scenario("two", "Runtime"), scenario("three", "Runtime")]);
    await page.goto(appURL + "/scenarios.html");
    await expect(page.locator("#scenario-list article")).toHaveCount(1);
    await expect(page.locator("#scenario-list")).toContainText("Scenario one");
    await expect(page.locator("#scenario-browser-dialog")).not.toBeVisible();
    await page.getByRole("button", { name: "Browse...", exact: true }).click();
    const browser = page.getByRole("dialog", { name: "Browse scenarios" });
    await expect(browser).toBeVisible();
    await browser.locator('summary').filter({ hasText: "Runtime" }).click();
    await browser.getByRole("button", { name: "Scenario two Description for two", exact: true }).click();
    await expect(browser).not.toBeVisible();
    await expect(page.locator("#scenario-browse")).toBeFocused();
    await expect(page.locator("#scenario-list")).toContainText("Scenario two");
    await expect(page.locator("#scenario-list article")).toHaveCount(1);
    expect([...api.counts.keys()].some(path => path.endsWith("/run"))).toBe(false);
    data.scenarios[1].description = "Updated catalog";
    await expect(page.locator("#scenario-list")).toContainText("Updated catalog");
    await page.locator("#scenario-browse").click();
    await expect(browser.locator('[data-select-scenario="two"]')).toHaveAttribute("aria-pressed", "true");
    await page.keyboard.press("Escape");
    await expect(page.locator("#scenario-browse")).toBeFocused();
    const layout = await page.locator(".scenario-layout").boundingBox();
    const tracks = await page.locator(".scenario-run-pane").boundingBox();
    expect(Math.abs(layout.width - tracks.width)).toBeLessThan(2);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.locator("#scenario-browse").click();
    await expect.poll(() => browser.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      return bounds.left >= 0 && bounds.right <= innerWidth && element.scrollWidth <= element.clientWidth;
    })).toBe(true);
    await page.keyboard.press("Escape");
    data.scenarios = [data.scenarios[0]];
    await expect(page.locator("#scenario-list")).toContainText("Scenario one");
    await expect(page.locator("#scenario-list")).not.toContainText("Scenario two");
  });

  test("Scenario browser File operations preserve draft and busy run guards", async ({ page, appURL, api }) => {
    const draft = scenario("draft", "");
    draft.published_revision = 0;
    const data = setup(api, [draft]);
    api.handlers.set("/api/simulator/organizations/alpha/scenarios/draft", route => route.fulfill({ json: {
      id: "draft", revision: 1, published_revision: 0, content: draft,
    } }));
    api.handlers.set("/api/simulator/organizations/alpha/scenarios/draft/revisions", route => route.fulfill({ json: [] }));
    await page.goto(appURL + "/scenarios.html");
    await expect(page.locator("[data-run-scenario]")).toBeDisabled();
    await page.locator("#scenario-files-toggle").click();
    await page.getByRole("menuitem", { name: "New Scenario...", exact: true }).click();
    await expect(page.locator("#scenario-editor-dialog")).toBeVisible();
    await page.locator("#scenario-editor-form .sim-editor-close").click();
    await page.locator("#scenario-files-toggle").click();
    await page.getByRole("menuitem", { name: "Edit", exact: true }).click();
    await expect(page.locator("#scenario-editor-name")).toHaveValue(draft.name);
    await page.locator("#scenario-editor-form .sim-editor-close").click();
    draft.published_revision = 1;
    await expect(page.locator("[data-run-scenario]")).toBeEnabled();
    data.run = { scenario_id: "draft", state: "running", steps: [], scenario: draft };
    await expect(page.locator("[data-run-scenario]")).toBeDisabled();
    data.active_organization_id = "beta";
    data.scenarios = [];
    await page.locator("#scenario-browse").click();
    await expect(page.locator("#scenario-browser-dialog")).not.toBeVisible();
    await expect(page.locator("#scenario-edit-selected")).toBeDisabled();
    await expect(page.locator("#scenario-list .scenario-empty")).toBeVisible();
  });

  test("Scenario browser runs only the selected published file and preserves the active run when browsing", async ({ page, appURL, api }) => {
    const one = scenario("one", "Runtime");
    const two = scenario("two", "Runtime");
    const data = setup(api, [one, two]);
    api.handlers.set("/api/simulator/scenarios/two/run", route => {
      expect(route.request().method()).toBe("POST");
      data.run = { scenario_id: "two", state: "running", scenario: two, steps: [] };
      return route.fulfill({ json: data.run });
    });
    await page.goto(appURL + "/scenarios.html");
    await page.locator("#scenario-browse").click();
    await page.locator('[data-select-scenario="two"]').click();
    await page.locator("[data-run-scenario]").click();
    await expect(page.locator("#scenario-run-title")).toHaveText(two.name);
    expect(api.counts.get("/api/simulator/scenarios/two/run")).toBe(1);
    expect(api.counts.get("/api/simulator/scenarios/one/run") || 0).toBe(0);
    await page.locator("#scenario-browse").click();
    await page.locator('[data-select-scenario="one"]').click();
    await expect(page.locator("#scenario-list")).toContainText(one.name);
    await expect(page.locator("#scenario-run-title")).toHaveText(two.name);
    await expect(page.locator("[data-run-scenario]")).toBeDisabled();
    api.handlers.set("/api/simulator/organizations/alpha/scenarios/one", route =>
      route.fulfill({ status: 503, json: { error: "Scenario store unavailable" } }));
    await page.locator("#scenario-files-toggle").click();
    await page.locator("#scenario-edit-selected").click();
    await expect(page.locator("#scenario-error")).toHaveText("Scenario store unavailable");
    await expect(page.locator("#scenario-editor-dialog")).not.toBeVisible();
  });

  test("Scenario browser selects a newly saved draft and enables Run only after publishing", async ({ page, appURL, api }) => {
    const data = setup(api, [scenario("one", "Runtime")]);
    let artifact;
    const endpoint = "/api/simulator/organizations/alpha/scenarios";
    api.handlers.set(endpoint, route => {
      const request = route.request().postDataJSON();
      expect(request.expected_revision).toBe(0);
      artifact = { id: request.scenario.id, revision: 1, published_revision: 0, content: request.scenario };
      data.scenarios.push({ ...request.scenario, current_revision: 1, published_revision: 0 });
      return route.fulfill({ json: artifact });
    });
    api.handlers.set(endpoint + "/created/revisions", route => route.fulfill({ json: [{ revision: 1 }] }));
    api.handlers.set(endpoint + "/created/publish", route => {
      expect(route.request().postDataJSON()).toEqual({ expected_revision: 1 });
      artifact.published_revision = 1;
      data.scenarios[1].published_revision = 1;
      return route.fulfill({ json: artifact });
    });
    api.handlers.set("/api/simulator/organizations/alpha/scenario-check", route => {
      const content = JSON.parse(route.request().postDataJSON().source);
      return route.fulfill({ json: { valid: true, scenario: content, canonical_json: JSON.stringify(content), diagnostics: "Valid draft" } });
    });
    await page.goto(appURL + "/scenarios.html");
    await page.locator("#scenario-files-toggle").click();
    await page.locator("#scenario-new").click();
    await page.locator("#scenario-editor-id").fill("created");
    await page.locator("#scenario-editor-name").fill("Created draft");
    await page.locator("#scenario-editor-files-toggle").click();
    await page.locator("#scenario-editor-save").click();
    await expect(page.locator("#scenario-editor-status")).toHaveText("Saved version 1. Publish it to enable runs.");
    await expect(page.locator("#scenario-list")).toContainText("Created draft");
    await expect(page.locator("[data-run-scenario]")).toBeDisabled();
    await page.locator("#scenario-editor-files-toggle").click();
    await page.locator("#scenario-editor-publish").click();
    await expect(page.locator("#scenario-editor-status")).toHaveText("Published version 1 for alpha.");
    await page.locator("#scenario-editor-form .sim-editor-close").click();
    await expect(page.locator('[data-run-scenario="created"]')).toBeEnabled();
    expect([...api.counts.keys()].some(path => path.endsWith("/run"))).toBe(false);
  });

  test("Scenario browser full-width tracks retain synchronized top scrolling when lanes overflow", async ({ page, appURL, api }) => {
    const definition = scenario("lanes", "Runtime");
    definition.steps = Array.from({ length: 8 }, (_, index) => ({
      action: "wait_controller", machine: `machine-${index + 1}`,
    }));
    const data = setup(api, [definition]);
    data.run = { scenario_id: "lanes", scenario: definition, state: "running", total_steps: 8, current_step: 1, steps: [] };
    await page.goto(appURL + "/scenarios.html");
    const scrollbar = page.locator(".scenario-track-scrollbar");
    const viewport = page.locator(".scenario-track-viewport");
    await expect(scrollbar).toBeVisible();
    const scrollbarBox = await scrollbar.boundingBox();
    const viewportBox = await viewport.boundingBox();
    expect(scrollbarBox.y).toBeLessThan(viewportBox.y);
    await scrollbar.evaluate(element => { element.scrollLeft = element.scrollWidth; });
    await expect.poll(() => viewport.evaluate(element => element.scrollLeft)).toBeGreaterThan(0);
    await viewport.evaluate(element => { element.scrollLeft = 0; });
    await expect.poll(() => scrollbar.evaluate(element => element.scrollLeft)).toBe(0);
  });

  test("Scenario compact running steps disclose output but keep failures visible", async ({ page, appURL, api }) => {
    const definition = scenario("compact", "Runtime");
    definition.steps = [
      { action: "delay", machine: "machine-01" },
      { action: "delay", machine: "machine-01" },
      { action: "delay", machine: "machine-02" },
    ];
    const data = setup(api, [definition]);
    data.run = {
      scenario_id: "compact", started_at: "2026-10-15T12:00:00Z", state: "running",
      current_step: 2, total_steps: 3, active_steps: [2], scenario: definition,
      steps: [{ number: 1, action: "delay", status: "completed", output: "Detailed output <script>inert</script>" }],
    };
    await page.goto(appURL + "/scenarios.html");
    const details = page.locator(".scenario-step-details");
    await expect(details).toHaveCount(1);
    await expect(details.locator("p")).not.toBeVisible();
    await expect(page.locator("#scenario-steps")).not.toContainText("Waiting");
    await details.locator("summary").click();
    await expect(details.locator("p")).toBeVisible();
    await expect(details.locator("script")).toHaveCount(0);
    data.run.current_step = 3;
    await expect(page.locator(".scenario-progress")).toHaveText("3 / 3");
    await expect(details.locator("p")).toBeVisible();
    data.run.started_at = "2026-10-15T13:00:00Z";
    await expect(details.locator("p")).not.toBeVisible();
    data.run.state = "cleaning";
    data.run.steps.push({ number: 2, action: "delay", status: "failed", error: "Explicit step failure" });
    await expect(page.locator(".scenario-step.failed")).toContainText("Explicit step failure");
    data.run.state = "failed";
    await expect(page.locator(".scenario-step-details")).toHaveCount(0);
    await expect(page.locator(".scenario-step-copy p").first()).toHaveText("Detailed output <script>inert</script>");
    await expect(page.locator(".scenario-step.failed p")).toHaveText("Explicit step failure");
  });
}
