export function registerOrganizationDuplicateTests(test, expect) {
  const source = "/api/simulator/organizations/alpha/duplicates/alpha-copy";
  const profilePath = "/api/simulator/organizations/alpha-copy/draft/profile";
  const profile = () => ({ id: "alpha-copy", name: "Alpha copy", description: "Fictional copied definitions",
    directory: { base_dn: "dc=alpha,dc=test" }, services: [], policies: [] });
  function fixture(api, { lost = false, stale = false, creationMode = "duplicate" } = {}) {
    let created = null;
    let current = profile();
    let revision = 1;
    const creates = [];
    const saves = [];
    api.handlers.set("/api/simulator/organization-drafts", route => route.fulfill({ json: { drafts: created ? [
      { id: "alpha-copy", name: current.name, revision, source_id: "alpha", draft_only: true },
    ] : [] } }));
    const createPath = creationMode === "duplicate" ? source : "/api/simulator/organizations/alpha-copy/create";
    api.handlers.set(createPath + "/check", route => route.fulfill({ json: { [creationMode === "duplicate" ? "source_hash" : "reviewed_hash"]: "reviewed-hash", profile: current,
      documents: [{ path: "directory.ldif", kind: "directory", content: "dn: dc=alpha,dc=test\ncn: Fictional\n" }],
      notice: creationMode === "duplicate" ? "Passwords removed. Source base DN and desired addresses retained. Draft only, not runnable." : "Missing definitions: Runtime driver; Directory document. Draft only, not runnable." } }));
    api.handlers.set(createPath, route => {
      creates.push(route.request().postDataJSON());
      if (stale) return route.fulfill({ status: 409, json: { error: "source definitions changed", review_again: true } });
      created = { organization_id: "alpha-copy", revision, content: { profile: current, operation_id: creates[0].operation_id } };
      return route.fulfill(lost ? { status: 503, json: { error: "Response lost after persistence" } } : { json: created });
    });
    api.handlers.set("/api/simulator/organizations/alpha-copy/draft", route => route.fulfill(created ? { json: created } : { status: 404, json: { error: "No draft" } }));
    api.handlers.set(profilePath, route => {
      if (route.request().method() === "PUT") {
        const body = route.request().postDataJSON();
        saves.push(body);
        current = body.profile;
        revision++;
        return route.fulfill({ json: { revision, content: current } });
      }
      return route.fulfill({ json: { revision, content: current } });
    });
    api.handlers.set(profilePath + "/revisions", route => route.fulfill({ json: [{ revision: 1, summary: "Duplicated definitions" }] }));
    api.handlers.set(profilePath + "/check", route => route.fulfill({ json: { valid: true, notice: "Bundle references validated; draft not runnable." } }));
    api.handlers.set("/api/simulator/organizations/alpha-copy/draft/export", route => route.fulfill({ json: { version: 1, profile: current, documents: [] } }));
    api.handlers.set("/api/simulator/assistant/status", route => route.fulfill({ json: { ready: true, model: "test-model", models: ["test-model"] } }));
    api.handlers.set(profilePath + "/assistant", route => {
      const body = route.request().postDataJSON();
      return route.fulfill({ json: { answer: "Review a fictional draft.", proposal: { profile: { ...body.profile, description: "AI copied-organization description" } } } });
    });
    return { creates, saves };
  }
  async function review(page, appURL) {
    await page.goto(appURL + "/organizations.html");
    await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
    await page.getByRole("button", { name: "Duplicate organization", exact: true }).click();
    const dialog = page.locator("#organization-duplicate-dialog");
    await dialog.getByLabel("New organization ID").fill("alpha-copy");
    await dialog.getByRole("button", { name: "Review proposed draft" }).click();
    await expect(dialog.locator("#organization-duplicate-review")).toContainText("directory.ldif");
    await expect(dialog.locator("#organization-duplicate-review")).toContainText("Passwords removed");
    return dialog;
  }
  test("Organization duplicate reviews definitions, creates an inactive draft and supports AI editing", async ({ page, appURL, api }) => {
    const state = fixture(api);
    const dialog = await review(page, appURL);
    const initialViewport = page.viewportSize();
    await expect(dialog.getByRole("button", { name: "Create draft", exact: true })).toBeDisabled();
    expect(state.creates).toHaveLength(0);
    for (const width of [1280, 820, 390]) {
      await page.setViewportSize({ width, height: 844 });
      await expect.poll(() => dialog.evaluate(element => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth + 1 && element.scrollWidth <= element.clientWidth + 1;
      })).toBe(true);
    }
    await page.setViewportSize(initialViewport);
    await review(page, appURL);
    await dialog.getByLabel("I reviewed these definitions").check();
    await dialog.getByRole("button", { name: "Create draft", exact: true }).click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha copy");
    expect(state.creates).toHaveLength(1);
    expect(state.creates[0].confirmed).toBe(true);
    expect(state.creates[0].expected_source_hash).toBe("reviewed-hash");
    const pane = page.locator("#organization-profile-assistant");
    await pane.getByLabel("Message", { exact: true }).fill("Improve the description.");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    await pane.getByRole("button", { name: "Review profile proposal" }).click();
    await page.getByRole("dialog", { name: "Review AI organization changes" }).getByRole("button", { name: "Insert reviewed draft" }).click();
    await expect(page.locator("#organization-profile-description")).toHaveValue("AI copied-organization description");
    expect(state.saves).toHaveLength(0);
    await page.locator("#organization-profile-editor").getByRole("button", { name: "File...", exact: true }).click();
    await page.locator("#organization-profile-editor").getByRole("menuitem", { name: "Save", exact: true }).click();
    await expect(page.locator("#organization-profile-revision")).toHaveText("Draft revision 2");
    await page.locator("#organization-profile-editor").getByRole("button", { name: "Analyze" }).click();
    await expect(page.locator("#organization-profile-status")).toContainText("draft not runnable");
    await expect(page.locator("#organization-drafts-list")).toContainText("Alpha copy");
    expect([...api.counts.keys()].some(path => /activate|restore-base|publish/.test(path))).toBe(false);
  });
  test("Organization duplicate retains lost-response operation across reload and reconciles without re-creation", async ({ page, appURL, api }) => {
    const state = fixture(api, { lost: true });
    const dialog = await review(page, appURL);
    await dialog.getByLabel("I reviewed these definitions").check();
    await dialog.getByRole("button", { name: "Create draft", exact: true }).click();
    await expect(dialog.locator("#organization-duplicate-status")).toContainText("Creation is not confirmed");
    await expect(dialog.getByLabel("New organization ID")).toBeDisabled();
    const operation = state.creates[0].operation_id;
    await page.reload();
    await page.getByRole("button", { name: "Duplicate organization", exact: true }).click();
    await expect(dialog.getByLabel("New organization ID")).toHaveValue("alpha-copy");
    await dialog.getByRole("button", { name: "Check creation result" }).click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha copy");
    expect(state.creates).toHaveLength(1);
    expect(await page.evaluate(() => sessionStorage.getItem("zpr-organization-duplicate-pending"))).toBeNull();
    expect(operation).toBeTruthy();
  });
  test("Organization duplicate invalidates changed previews and releases a rejected stale-source operation", async ({ page, appURL, api }) => {
    const state = fixture(api, { stale: true });
    const dialog = await review(page, appURL);
    await dialog.getByLabel("Name", { exact: true }).fill("Changed copied name");
    await expect(dialog.getByRole("button", { name: "Create draft", exact: true })).toBeDisabled();
    await expect(dialog.locator("#organization-duplicate-review")).toBeEmpty();
    await dialog.getByRole("button", { name: "Review proposed draft" }).click();
    await dialog.getByLabel("I reviewed these definitions").check();
    await dialog.getByRole("button", { name: "Create draft", exact: true }).click();
    await expect(dialog.locator("#organization-duplicate-status")).toContainText("This operation did not create a draft");
    await expect(dialog.getByLabel("New organization ID")).toBeEnabled();
    expect(state.creates).toHaveLength(1);
    expect(await page.evaluate(() => sessionStorage.getItem("zpr-organization-duplicate-pending"))).toBeNull();
  });
  test("Organization duplicate ignores delayed creation receipt after operator session loss", async ({ page, appURL, api }) => {
    fixture(api);
    let release;
    let started = false;
    const gate = new Promise(resolve => { release = resolve; });
    api.handlers.set(source, async route => {
      started = true;
      const body = route.request().postDataJSON();
      await gate;
      await route.fulfill({ json: { organization_id: "alpha-copy", revision: 1, content: { profile: profile(), operation_id: body.operation_id } } });
    });
    const dialog = await review(page, appURL);
    await dialog.getByLabel("I reviewed these definitions").check();
    await dialog.getByRole("button", { name: "Create draft", exact: true }).click();
    await expect.poll(() => started).toBe(true);
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    await expect(dialog).not.toBeVisible();
    await expect(page.locator("#organization-drafts-status")).toContainText("Operator session ended");
    const response = page.waitForResponse(result => new URL(result.url()).pathname === source);
    release();
    await response;
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await expect(page.locator("#organization-profile-editor")).not.toBeVisible();
    expect(await page.evaluate(() => sessionStorage.getItem("zpr-organization-duplicate-pending"))).toBeNull();
  });
  for (const mode of ["new", "import"]) {
    test(`Organization ${mode} reviews readiness gaps, creates a private draft and exports definitions`, async ({ page, appURL, api }) => {
      const state = fixture(api, { creationMode: mode });
      await page.goto(appURL + "/organizations.html");
      await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
      await page.getByRole("button", { name: mode === "new" ? "New organization" : "Import organization", exact: true }).click();
      const dialog = page.locator("#organization-duplicate-dialog");
      await dialog.getByLabel("New organization ID").fill("alpha-copy");
      if (mode === "new") {
        await dialog.getByLabel("Name", { exact: true }).fill("Alpha copy");
        await dialog.getByLabel("Description", { exact: true }).fill("Fictional copied definitions");
        await dialog.getByLabel("Directory base DN (optional)").fill("dc=alpha,dc=test");
      } else {
        await dialog.getByLabel("Import bundle file").setInputFiles({
          name: "definitions.json", mimeType: "application/json",
          buffer: Buffer.from(JSON.stringify({ version: 1, profile: profile(), documents: [] })),
        });
        await expect(dialog.locator("#organization-duplicate-status")).toContainText("Import loaded locally");
      }
      await expect(dialog.getByRole("button", { name: "Create draft", exact: true })).toBeDisabled();
      await dialog.getByRole("button", { name: "Review proposed draft" }).click();
      await expect(dialog.locator("#organization-duplicate-review")).toContainText("Missing definitions");
      for (const width of [1280, 820, 390]) {
        await page.setViewportSize({ width, height: 844 });
        await expect.poll(() => dialog.evaluate(element => {
          const bounds = element.getBoundingClientRect();
          return bounds.left >= 0 && bounds.right <= innerWidth + 1 && element.scrollWidth <= element.clientWidth + 1;
        })).toBe(true);
      }
      await dialog.getByLabel("I reviewed these definitions").check();
      await dialog.getByRole("button", { name: "Create draft", exact: true }).click();
      await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha copy");
      expect(state.creates).toHaveLength(1);
      expect(state.creates[0].mode).toBe(mode);
      expect(state.creates[0].reviewed_hash).toBe("reviewed-hash");
      expect(state.creates[0].expected_source_hash).toBeUndefined();
      if (mode === "import") expect(state.creates[0].bundle.profile.id).toBe("alpha-copy");
      const download = page.waitForEvent("download");
      await page.locator("#organization-profile-editor").getByRole("button", { name: "File...", exact: true }).click();
      await page.getByRole("menuitem", { name: "Export saved definitions", exact: true }).click();
      expect((await download).suggestedFilename()).toBe("alpha-copy-draft.json");
      await expect(page.locator("#organization-profile-status")).toContainText("without creation receipts");
      expect([...api.counts.keys()].some(path => /activate|restore-base|publish/.test(path))).toBe(false);
    });
  }
  test("Organization new retains creation across reload and reconciles without re-creation", async ({ page, appURL, api }) => {
    const state = fixture(api, { creationMode: "new", lost: true });
    await page.goto(appURL + "/organizations.html");
    await page.getByRole("button", { name: "New organization", exact: true }).click();
    const dialog = page.locator("#organization-duplicate-dialog");
    await dialog.getByLabel("New organization ID").fill("alpha-copy");
    await dialog.getByLabel("Name", { exact: true }).fill("Alpha copy");
    await dialog.getByLabel("Description", { exact: true }).fill("Private definitions");
    await dialog.getByRole("button", { name: "Review proposed draft" }).click();
    await dialog.getByLabel("I reviewed these definitions").check();
    await dialog.getByRole("button", { name: "Create draft", exact: true }).click();
    await expect(dialog.locator("#organization-duplicate-status")).toContainText("Creation is not confirmed");
    await page.reload();
    await page.getByRole("button", { name: "Import organization", exact: true }).click();
    await expect(dialog.getByRole("heading")).toHaveText("New organization draft");
    await expect(dialog.getByLabel("New organization ID")).toBeDisabled();
    await dialog.getByRole("button", { name: "Check creation result" }).click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha copy");
    expect(state.creates).toHaveLength(1);
    expect(await page.evaluate(() => sessionStorage.getItem("zpr-organization-duplicate-pending"))).toBeNull();
  });
  test("Organization import rejects invalid JSON locally without creation or AI disclosure", async ({ page, appURL, api }) => {
    fixture(api, { creationMode: "import" });
    await page.goto(appURL + "/organizations.html");
    await page.getByRole("button", { name: "Import organization", exact: true }).click();
    const dialog = page.locator("#organization-duplicate-dialog");
    await dialog.getByLabel("New organization ID").fill("alpha-copy");
    await dialog.getByLabel("Portable organization bundle JSON").fill("{");
    await dialog.getByRole("button", { name: "Review proposed draft" }).click();
    await expect(dialog.locator("#organization-duplicate-status")).not.toContainText("Review the changed draft");
    await expect(dialog.getByRole("button", { name: "Create draft", exact: true })).toBeDisabled();
    expect(api.counts.get("/api/simulator/organizations/alpha-copy/create/check") || 0).toBe(0);
    expect(api.counts.get(profilePath + "/assistant") || 0).toBe(0);
  });
  test.describe("Organization narrow viewport", () => {
    test.use({ viewport: { width: 390, height: 844 } });
    test("Organization new supports reviewed AI editing at a fresh narrow viewport", async ({ page, appURL, api }) => {
      const state = fixture(api, { creationMode: "new" });
      await page.goto(appURL + "/organizations.html");
      await page.getByRole("button", { name: "New organization", exact: true }).click();
      const dialog = page.locator("#organization-duplicate-dialog");
      await dialog.getByLabel("New organization ID").fill("alpha-copy");
      await dialog.getByLabel("Name", { exact: true }).fill("Alpha copy");
      await dialog.getByLabel("Description", { exact: true }).fill("Private draft");
      await dialog.getByRole("button", { name: "Review proposed draft" }).click();
      await dialog.getByLabel("I reviewed these definitions").check();
      await dialog.getByRole("button", { name: "Create draft", exact: true }).click();
      await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha copy");
      const pane = page.locator("#organization-profile-assistant");
      await pane.getByLabel("Message", { exact: true }).fill("Improve the description.");
      await pane.getByRole("button", { name: "Send", exact: true }).click();
      await pane.getByRole("button", { name: "Review profile proposal" }).click();
      await page.getByRole("dialog", { name: "Review AI organization changes" }).getByRole("button", { name: "Insert reviewed draft" }).click();
      await expect(page.locator("#organization-profile-description")).toHaveValue("AI copied-organization description");
      expect(state.saves).toHaveLength(0);
      await page.locator("#organization-profile-editor").getByRole("button", { name: "File...", exact: true }).click();
      await page.locator("#organization-profile-editor").getByRole("menuitem", { name: "Save", exact: true }).click();
      await expect(page.locator("#organization-profile-revision")).toHaveText("Draft revision 2");
    });
  });
}
