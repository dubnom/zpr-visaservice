export function registerOrganizationProfileTests(test, expect) {
  test("Organization section links jump to identity, policy and services without changing organization", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    const links = page.getByRole("navigation", { name: "Organization sections", exact: true });
    for (const [label, id, heading] of [
      ["IDENTITY", "organization-identity", /Directory/],
      ["POLICY", "organization-policy", "Policies"],
      ["SERVICES", "organization-services", "Services"],
    ]) {
      const link = links.getByRole("link", { name: label, exact: true });
      await expect(link).toHaveAttribute("href", `#${id}`);
      await link.focus();
      await link.press("Enter");
      await expect(page).toHaveURL(`${appURL}/organizations.html#${id}`);
      await expect(page.locator(`#${id}`)).toBeFocused();
      await expect(page.locator(`#${id}`).getByRole("heading")).toHaveText(heading);
      await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
      const bounds = await page.locator(`#${id}`).evaluate(element => {
        const rect = element.getBoundingClientRect();
        return { top: rect.top, bottom: rect.bottom, height: innerHeight };
      });
      expect(bounds.bottom).toBeGreaterThan(0);
      expect(bounds.top).toBeLessThan(bounds.height);
    }
  });
  const endpoint = "/api/simulator/organizations/alpha/profile";
  const initial = () => ({
    id: "alpha", name: "Alpha Labs", description: "Fictional organization",
    runtime: { driver: "linux-one-node", topology: "single-node", nodes: [{ id: "node-1", location: "Test site" }] },
    directory: { base_dn: "dc=alpha,dc=test", departments: [], people: [], groups: [] },
    services: [], policies: [], machine_owners: { laptop: ["test-user"] },
  });
  function fixture(api) {
    let profile = initial();
    let revision = 1;
    let conflict = false;
    const writes = [];
    api.handlers.set(endpoint, async route => {
      if (route.request().method() === "PUT") {
        const body = route.request().postDataJSON();
        writes.push(body);
        if (conflict) return route.fulfill({ status: 409, json: { error: "organization draft changed" } });
        profile = body.profile;
        revision++;
        return route.fulfill({ json: { revision, content: profile } });
      }
      return route.fulfill({ json: { revision, content: profile } });
    });
    api.handlers.set(endpoint + "/revisions", route => route.fulfill({ json: [{ revision: 1, summary: "Imported organization profile" }] }));
    api.handlers.set(endpoint + "/revisions/1", route => route.fulfill({ json: { revision: 1, content: initial() } }));
    api.handlers.set(endpoint + "/check", route => route.fulfill({ json: { valid: true, notice: "Profile schema validated; runtime readiness not certified." } }));
    api.handlers.set("/api/simulator/assistant/status", route => route.fulfill({ json: { ready: true, model: "test-model", models: ["test-model"] } }));
    return { writes, setConflict: () => { conflict = true; } };
  }
  async function open(page, appURL) {
    await page.goto(appURL + "/organizations.html");
    await page.locator("#organization-files-toggle").click();
    await page.locator("#organization-file-menu").getByRole("menuitem", { name: "Edit", exact: true }).click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha Labs");
  }
  test("Organization profile drafts preserve structured/source fields, save separately and load history", async ({ page, appURL, api }) => {
    const state = fixture(api);
    await open(page, appURL);
    const editor = page.locator("#organization-profile-editor");
    await editor.getByLabel("Name", { exact: true }).fill("Alpha draft");
    const content = JSON.parse(await page.locator("#organization-profile-source").inputValue());
    expect(content.name).toBe("Alpha draft");
    expect(content.machine_owners).toEqual({ laptop: ["test-user"] });
    await editor.getByRole("button", { name: "Analyze", exact: true }).click();
    await expect(page.locator("#organization-profile-status")).toContainText("runtime readiness not certified");
    expect(state.writes).toHaveLength(0);
    await editor.getByRole("button", { name: "File...", exact: true }).click();
    await editor.getByRole("menuitem", { name: "Save", exact: true }).click();
    await expect(page.locator("#organization-profile-revision")).toHaveText("Draft revision 2");
    expect(state.writes[0].expected_revision).toBe(1);
    await editor.getByLabel("Version history", { exact: true }).click();
    await editor.getByRole("button", { name: /Revision 1:/ }).click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha Labs");
    await expect(page.locator("#organization-profile-status")).toContainText("Historical revision 1");
    await expect(page.locator("#organization-profile-modified")).toBeVisible();
    expect([...api.counts.keys()].some(path => /activate|restore-base|publish/.test(path))).toBe(false);
    for (const width of [1280, 820, 390]) {
      await page.setViewportSize({ width, height: 844 });
      await expect.poll(() => editor.evaluate(element => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth + 1 && element.scrollWidth <= element.clientWidth + 1;
      })).toBe(true);
    }
  });
  test("Organization profile AI sends the unsaved draft and requires reviewed insertion with undo", async ({ page, appURL, api }) => {
    const state = fixture(api);
    let request;
    api.handlers.set(endpoint + "/assistant", route => {
      request = route.request().postDataJSON();
      return route.fulfill({ json: { answer: "A revised fictional description.", proposal: { profile: { ...request.profile, description: "AI proposed description" } } } });
    });
    await open(page, appURL);
    await page.locator("#organization-profile-description").fill("Unsaved requirements");
    const pane = page.locator("#organization-profile-assistant");
    await pane.getByLabel("Message", { exact: true }).fill("Improve the description.");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    await pane.getByRole("button", { name: "Review profile proposal", exact: true }).click();
    const review = page.getByRole("dialog", { name: "Review AI organization changes" });
    await expect(review).toContainText("Unsaved requirements");
    await expect(review).toContainText("AI proposed description");
    expect(request.profile.description).toBe("Unsaved requirements");
    expect(request.scope).toBe("organization-profile");
    await expect(page.locator("#organization-profile-description")).toHaveValue("Unsaved requirements");
    await review.getByRole("button", { name: "Insert reviewed draft" }).click();
    await expect(page.locator("#organization-profile-description")).toHaveValue("AI proposed description");
    expect(state.writes).toHaveLength(0);
    await pane.getByRole("button", { name: "Undo AI change" }).click();
    await expect(page.locator("#organization-profile-description")).toHaveValue("Unsaved requirements");
    await pane.getByRole("button", { name: "Redo AI change" }).click();
    await expect(page.locator("#organization-profile-description")).toHaveValue("AI proposed description");
  });
  test("Organization profile stale AI proposals and conflicting saves leave local drafts intact", async ({ page, appURL, api }) => {
    const state = fixture(api);
    api.handlers.set(endpoint + "/assistant", route => route.fulfill({ json: { answer: "Rename.", proposal: { profile: { ...initial(), name: "Obsolete AI name" } } } }));
    await open(page, appURL);
    const pane = page.locator("#organization-profile-assistant");
    await pane.getByLabel("Message", { exact: true }).fill("Rename.");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    await expect(pane.getByRole("button", { name: "Review profile proposal" })).toBeVisible();
    await page.locator("#organization-profile-name").fill("User changed name");
    await pane.getByRole("button", { name: "Review profile proposal" }).click();
    await expect(pane).toContainText("editor changed");
    await expect(page.getByRole("dialog", { name: "Review AI organization changes" })).not.toBeVisible();
    state.setConflict();
    await page.locator("#organization-profile-editor").getByRole("button", { name: "File...", exact: true }).click();
    await page.locator("#organization-profile-editor").getByRole("menuitem", { name: "Save", exact: true }).click();
    await expect(page.locator("#organization-profile-status")).toContainText("reload the draft to reconcile");
    await expect(page.locator("#organization-profile-name")).toHaveValue("User changed name");
    await expect(page.locator("#organization-profile-revision")).toHaveText("Draft revision 1");
  });
  test("Organization profile proposal cancellation and invalid JSON never save drafts", async ({ page, appURL, api }) => {
    const state = fixture(api);
    api.handlers.set(endpoint + "/assistant", route => route.fulfill({ json: { answer: "Rename.", proposal: { profile: { ...initial(), name: "Cancelled name" } } } }));
    await open(page, appURL);
    const pane = page.locator("#organization-profile-assistant");
    await pane.getByLabel("Message", { exact: true }).fill("Rename.");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    await pane.getByRole("button", { name: "Review profile proposal" }).click();
    await page.getByRole("dialog", { name: "Review AI organization changes" }).getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha Labs");
    await page.locator("#organization-profile-mode-toggle").click();
    await page.locator("#organization-profile-source").fill("{ broken");
    await page.locator("#organization-profile-editor").getByRole("button", { name: "File...", exact: true }).click();
    await page.locator("#organization-profile-editor").getByRole("menuitem", { name: "Save", exact: true }).click();
    await expect(page.locator("#organization-profile-status")).toContainText("Save was not confirmed");
    expect(state.writes).toHaveLength(0);
    await expect(page.locator("#organization-profile-source")).toHaveValue("{ broken");
  });
  test("Organization profile structured section editors synchronize with raw JSON and previews", async ({ page, appURL, api }) => {
    fixture(api);
    await open(page, appURL);
    const editor = page.locator("#organization-profile-editor");

    // Overview tab is active initially with metrics
    await expect(editor.locator('[data-editor-tab="overview"]')).toHaveAttribute("aria-selected", "true");
    await expect(editor.locator("#metric-sites-count")).toHaveText("1");
    await expect(editor.locator("#metric-services-count")).toHaveText("0");

    // Sites & Topology section
    await editor.locator('[data-editor-tab="sites"]').click();
    await expect(editor.locator('[data-editor-tab="sites"]')).toHaveAttribute("aria-selected", "true");
    await expect(editor.locator("#section-panel-sites")).toBeVisible();
    await expect(editor.locator('#tbody-sites input[data-field="id"]').first()).toHaveValue("node-1");

    // Add a new runtime node and verify two-way sync
    await editor.locator('[data-action="add-site"]').click();
    const siteRows = editor.locator("#tbody-sites tr");
    await expect(siteRows).toHaveCount(2);
    await expect(editor.locator("#topology-nodes-preview .topology-node-card")).toHaveCount(2);
    let parsed = JSON.parse(await page.locator("#organization-profile-source").inputValue());
    expect(parsed.runtime.nodes).toHaveLength(2);
    expect(parsed.runtime.nodes[1].id).toBe("site-2");

    // Directory section
    await editor.locator('[data-editor-tab="directory"]').click();
    await expect(editor.locator('[data-editor-tab="directory"]')).toHaveAttribute("aria-selected", "true");
    await expect(editor.locator("#section-panel-directory")).toBeVisible();

    // Add department and person
    await editor.locator('[data-action="add-department"]').click();
    await expect(editor.locator("#tbody-departments tr")).toHaveCount(1);
    await editor.locator('[data-action="add-person"]').click();
    await expect(editor.locator("#tbody-people tr")).toHaveCount(1);

    parsed = JSON.parse(await page.locator("#organization-profile-source").inputValue());
    expect(parsed.directory.departments).toHaveLength(1);
    expect(parsed.directory.departments[0].name).toBe("Department 1");
    expect(parsed.directory.people).toHaveLength(1);
    expect(parsed.directory.people[0].uid).toBe("user-1");

    // Services & Workloads section
    await editor.locator('[data-editor-tab="services"]').click();
    await expect(editor.locator('[data-editor-tab="services"]')).toHaveAttribute("aria-selected", "true");
    await expect(editor.locator("#section-panel-services")).toBeVisible();

    await editor.locator('[data-action="add-service"]').click();
    await expect(editor.locator("#tbody-services tr")).toHaveCount(1);
    parsed = JSON.parse(await page.locator("#organization-profile-source").inputValue());
    expect(parsed.services).toHaveLength(1);
    expect(parsed.services[0].name).toBe("Service 1");

    // Check updated metrics on Overview tab
    await editor.locator('[data-editor-tab="overview"]').click();
    await expect(editor.locator("#metric-sites-count")).toHaveText("2");
    await expect(editor.locator("#metric-services-count")).toHaveText("1");
    await expect(editor.locator("#metric-people-count")).toHaveText("1");
    await expect(editor.locator("#metric-depts-count")).toHaveText("1");
  });
  test("Organization profile uses standard toolbar, exclusive modes and identity history", async ({ page, appURL, api }) => {
    const state = fixture(api);
    api.handlers.set("/api/simulator/organization-drafts", route => route.fulfill({ json: { drafts: [] } }));
    await open(page, appURL);
    const editor = page.locator("#organization-profile-editor");
    const mode = editor.locator("#organization-profile-mode-toggle");
    const source = editor.locator("#organization-profile-source");
    await expect(editor.locator(".policy-identity #organization-profile-history-menu")).toHaveCount(1);
    await expect(editor.getByText("Advanced JSON source", { exact: true })).toHaveCount(0);
    await expect(editor.locator("#organization-profile-form")).toBeVisible();
    await expect(source).toBeHidden();
    await expect(mode).toHaveText("Raw");
    await editor.locator('[data-editor-tab="sites"]').click();
    await mode.click();
    await expect(mode).toHaveText("Form");
    await expect(editor.locator("#organization-profile-form")).toBeHidden();
    await expect(source).toBeVisible();
    await editor.getByLabel("Wrap", { exact: true }).check();
    await expect(source).toHaveAttribute("wrap", "soft");
    await expect(editor.locator(".config-code-surface")).toHaveClass(/editor-word-wrap/);
    await editor.getByLabel("Wrap", { exact: true }).uncheck();
    const profile = JSON.parse(await source.inputValue());
    profile.name = "Raw draft name";
    await source.fill(JSON.stringify(profile));
    await editor.getByRole("button", { name: "Format", exact: true }).click();
    await expect(source).toHaveValue(JSON.stringify(profile, null, 2));
    await mode.click();
    await expect(editor.locator("#section-panel-sites")).toBeVisible();
    await editor.locator('[data-editor-tab="overview"]').click();
    await expect(editor.getByLabel("Name", { exact: true })).toHaveValue("Raw draft name");
    await mode.click();
    await source.fill("{ invalid");
    await mode.click();
    await expect(source).toBeVisible();
    await expect(mode).toHaveText("Form");
    await expect(editor.locator("#organization-profile-status")).toContainText("Form unavailable");
    await source.fill(JSON.stringify(profile));
    await mode.click();
    page.once("dialog", dialog => dialog.dismiss());
    await editor.getByRole("button", { name: "Browse...", exact: true }).click();
    await editor.locator("#organization-profile-records").getByRole("button", { name: "Alpha Labs", exact: true }).click();
    await expect(editor.getByLabel("Name", { exact: true })).toHaveValue("Raw draft name");
    expect(state.writes).toHaveLength(0);
    await mode.click();
    await source.press("Control+s");
    await expect(editor.locator("#organization-profile-revision")).toHaveText("Draft revision 2");
    await expect(source).toBeVisible();
    await expect(mode).toHaveText("Form");
    expect(state.writes).toHaveLength(1);
    await mode.click();
    await editor.getByRole("button", { name: "Browse...", exact: true }).click();
    await editor.locator("#organization-profile-records").getByRole("button", { name: "Alpha Labs", exact: true }).click();
    await expect(editor.locator("#organization-profile-catalog-pane")).toBeHidden();
    await expect(editor.getByLabel("Name", { exact: true })).toHaveValue("Raw draft name");
    await editor.getByLabel("Version history", { exact: true }).click();
    await page.setViewportSize({ width: 390, height: 844 });
    await expect.poll(() => editor.locator("#organization-profile-history").evaluate(element => {
      const bounds = element.getBoundingClientRect();
      return bounds.left >= 0 && bounds.right <= innerWidth;
    })).toBe(true);
  });
  test("Organization profile Browse reports failures and ignores catalog responses after logout", async ({ page, appURL, api }) => {
    fixture(api);
    api.handlers.set("/api/simulator/organization-drafts", route => route.fulfill({ status: 503, json: { error: "Draft catalog unavailable" } }));
    await open(page, appURL);
    const editor = page.locator("#organization-profile-editor");
    await editor.getByRole("button", { name: "Browse...", exact: true }).click();
    await expect(editor.locator("[data-profile-picker-status]")).toContainText("Browse failed");
    await expect(editor.locator("#organization-profile-records").getByRole("button")).toHaveCount(0);
    await editor.getByRole("button", { name: "Close organization browser", exact: true }).click();
    let release;
    let started = false;
    const gate = new Promise(resolve => { release = resolve; });
    api.handlers.set("/api/simulator/organization-drafts", async route => {
      started = true;
      await gate;
      await route.fulfill({ json: { drafts: [{ id: "private", name: "Private draft" }] } });
    });
    await editor.getByRole("button", { name: "Browse...", exact: true }).click();
    await expect.poll(() => started).toBe(true);
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    const response = page.waitForResponse(result => new URL(result.url()).pathname === "/api/simulator/organization-drafts");
    release();
    await response;
    await expect(editor).not.toBeVisible();
    await expect(editor.locator("#organization-profile-records").getByRole("button")).toHaveCount(0);
  });
  test("Organization profile Browse opens private drafts and History retains whole-bundle ownership", async ({ page, appURL, api }) => {
    fixture(api);
    const profile = { ...initial(), id: "private", name: "Private draft" };
    const bundle = { version: 1, profile, documents: [{ path: "policy.zpl", kind: "policy", content: "Fictional policy\n" }] };
    api.handlers.set("/api/simulator/organization-drafts", route => route.fulfill({ json: { drafts: [{ id: "private", name: "Private draft" }] } }));
    for (const kind of ["profile", "definitions"]) {
      const path = `/api/simulator/organizations/private/draft/${kind}`;
      api.handlers.set(path, route => route.fulfill({ json: { revision: 1, content: kind === "profile" ? profile : bundle } }));
      api.handlers.set(path + "/revisions", route => route.fulfill({ json: [{ revision: 1, summary: "Private snapshot" }] }));
      api.handlers.set(path + "/revisions/1", route => route.fulfill({ json: { revision: 1, content: kind === "profile" ? profile : bundle } }));
    }
    await open(page, appURL);
    const editor = page.locator("#organization-profile-editor");
    await editor.getByRole("button", { name: "Browse...", exact: true }).click();
    await editor.locator("#organization-profile-records").getByRole("button", { name: "Private draft · Private draft", exact: true }).click();
    await expect(editor.getByLabel("Name", { exact: true })).toHaveValue("Private draft");
    await editor.getByLabel("Version history", { exact: true }).click();
    await editor.getByRole("button", { name: "Revision 1: Private snapshot", exact: true }).click();
    const definitions = page.locator("#organization-definitions-editor");
    await expect(definitions).toBeVisible();
    await expect(editor).not.toBeVisible();
    await expect(definitions.getByLabel("Selected organization definition source")).toHaveValue(JSON.stringify(profile, null, 2));
    await definitions.getByLabel("Definition file", { exact: true }).selectOption("0");
    await expect(definitions.getByLabel("Selected organization definition source")).toHaveValue(bundle.documents[0].content);
    expect([...api.counts.keys()].some(path => /activate|restore-base|publish/.test(path))).toBe(false);
  });
}
