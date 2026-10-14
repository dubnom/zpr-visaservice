export function registerOrganizationDefinitionsTests(test, expect) {
  const endpoint = "/api/simulator/organizations/alpha-copy/draft/definitions";
  const initial = () => ({ version: 1, profile: { id: "alpha-copy", name: "Alpha copy", description: "Private fictional definitions",
    directory: { base_dn: "dc=alpha,dc=test", seed_mode: "ldif", ldif_files: ["directory.ldif"] } },
    documents: [{ path: "directory.ldif", kind: "directory", content: "dn: dc=alpha,dc=test\nobjectClass: domain\ndc: alpha\n" },
      { path: "policy.zpl", kind: "policy", content: "Fictional initial policy\n" }] });
  function fixture(api, { conflict = false } = {}) {
    let bundle = initial();
    let revision = 1;
    const saves = [];
    const checks = [];
    const snapshots = new Map([[1, structuredClone(bundle)]]);
    api.handlers.set("/api/simulator/organization-drafts", route => route.fulfill({ json: { drafts: [{ id: "alpha-copy", name: "Alpha copy", revision, draft_only: true }] } }));
    api.handlers.set("/api/simulator/organizations/alpha-copy/draft/profile", route => route.fulfill({ json: { revision, content: bundle.profile } }));
    api.handlers.set("/api/simulator/organizations/alpha-copy/draft/profile/revisions", route => route.fulfill({ json: [{ revision: 1, summary: "Initial snapshot" }] }));
    api.handlers.set(endpoint, route => {
      if (route.request().method() === "PUT") {
        const body = route.request().postDataJSON();
        saves.push(body);
        if (conflict || body.expected_revision !== revision) return route.fulfill({ status: 409, json: { error: "organization definitions changed" } });
        bundle = body.bundle;
        revision++;
        snapshots.set(revision, structuredClone(bundle));
      }
      return route.fulfill({ json: { revision, content: bundle } });
    });
    api.handlers.set(endpoint + "/check", route => {
      checks.push(route.request().postDataJSON());
      return route.fulfill({ json: { valid: true, runnable: false, readiness_gaps: ["Runtime nodes"], notice: "Definitions validated; runtime readiness not certified. Draft is not runnable." } });
    });
    api.handlers.set(endpoint + "/revisions", route => route.fulfill({ json: [...snapshots.keys()].map(revision => ({ revision, summary: `Snapshot ${revision}` })) }));
    api.handlers.set(endpoint + "/revisions/1", route => route.fulfill({ json: { revision: 1, content: snapshots.get(1) } }));
    api.handlers.set("/api/simulator/assistant/status", route => route.fulfill({ json: { ready: true, model: "test-model", models: ["test-model"] } }));
    const assistantRequests = [];
    api.handlers.set(endpoint + "/assistant", route => {
      const request = route.request().postDataJSON();
      assistantRequests.push(request);
      const proposal = structuredClone(request.bundle);
      proposal.profile.description = "AI coherent profile edit";
      proposal.documents[0].content = "dn: dc=alpha,dc=test\nobjectClass: domain\ndc: alpha\n# AI directory edit\n";
      return route.fulfill({ json: { answer: "Review profile and directory together.", proposal: { bundle: proposal } } });
    });
    return { saves, checks, assistantRequests };
  }
  async function open(page, appURL) {
    await page.goto(appURL + "/organizations.html");
    await page.locator("#organization-drafts-list .organization-card").click();
    await expect(page.locator("#organization-profile-name")).toHaveValue("Alpha copy");
    await page.locator("#organization-profile-editor").getByRole("button", { name: "File...", exact: true }).click();
    await page.getByRole("menuitem", { name: "Edit supporting definitions", exact: true }).click();
    const editor = page.locator("#organization-definitions-editor");
    await expect(editor).toBeVisible();
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 1");
    return editor;
  }
  async function choose(editor, index) {
    await editor.getByLabel("Definition file", { exact: true }).selectOption(String(index));
  }
  test("Organization definitions save profile and document edits together and restore coherent history", async ({ page, appURL, api }) => {
    const state = fixture(api);
    const editor = await open(page, appURL);
    const source = editor.getByLabel("Selected organization definition source");
    const profile = initial().profile;
    profile.description = "Edited profile with directory";
    await source.fill(JSON.stringify(profile, null, 2));
    await choose(editor, 0);
    await source.fill("dn: dc=alpha,dc=test\nobjectClass: domain\ndc: alpha\n# unsaved directory\n");
    await choose(editor, 1);
    await expect(source).toHaveValue("Fictional initial policy\n");
    await editor.getByRole("button", { name: "Validate all definitions" }).click();
    await expect(editor.locator("#organization-definitions-status")).toContainText("not runnable");
    expect(state.saves).toHaveLength(0);
    expect(state.checks[0].bundle.profile.description).toBe(profile.description);
    expect(state.checks[0].bundle.documents[0].content).toContain("unsaved directory");
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 2");
    expect(state.saves).toHaveLength(1);
    expect(state.saves[0].expected_revision).toBe(1);
    expect(state.saves[0].bundle.documents[1].content).toBe("Fictional initial policy\n");
    await editor.getByText("History", { exact: true }).click();
    await editor.getByRole("button", { name: "Revision 1: Snapshot 1" }).click();
    await expect(source).toHaveValue(JSON.stringify(initial().profile, null, 2));
    await choose(editor, 0);
    await expect(source).not.toHaveValue(/unsaved directory/);
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 3");
    expect(state.saves[1].expected_revision).toBe(2);
    expect(state.saves[1].bundle).toEqual(initial());
    expect([...api.counts.keys()].some(path => /activate|publish|restore-base/.test(path))).toBe(false);
  });
  test("Organization definitions AI uses unsaved documents and reviews atomic insertion with undo redo", async ({ page, appURL, api }) => {
    const state = fixture(api);
    const editor = await open(page, appURL);
    const source = editor.getByLabel("Selected organization definition source");
    await choose(editor, 0);
    await source.fill("dn: dc=alpha,dc=test\nobjectClass: domain\ndc: alpha\n# unsaved requirement\n");
    const pane = editor.locator("#organization-definitions-assistant");
    await expect(pane).toContainText("every supporting document");
    await pane.getByLabel("Message", { exact: true }).fill("Update profile and directory together");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    expect(state.assistantRequests[0].bundle.documents[0].content).toContain("unsaved requirement");
    expect(state.assistantRequests[0].scope).toBe("organization-definitions");
    expect(state.assistantRequests[0].bundle.operation_id).toBeUndefined();
    await pane.getByRole("button", { name: "Review definition proposal" }).click();
    const review = page.getByRole("dialog", { name: "Review AI definition changes" });
    await expect(review).toContainText("Profile: description");
    await expect(review).toContainText("Document: directory.ldif");
    await expect(review).toContainText("unsaved requirement");
    await review.getByRole("button", { name: "Insert reviewed definitions" }).click();
    await expect(source).toHaveValue(/AI coherent profile edit/);
    expect(state.saves).toHaveLength(0);
    await pane.getByRole("button", { name: "Undo AI change" }).click();
    await choose(editor, 0);
    await expect(source).toHaveValue(/unsaved requirement/);
    await pane.getByRole("button", { name: "Redo AI change" }).click();
    await choose(editor, 0);
    await expect(source).toHaveValue(/AI directory edit/);
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 2");
    expect(state.saves[0].bundle.profile.description).toBe("AI coherent profile edit");
    expect(state.saves[0].bundle.documents[0].content).toContain("AI directory edit");
  });
  test("Organization definitions add remove documents and preserve local edits on conflict", async ({ page, appURL, api }) => {
    const state = fixture(api, { conflict: true });
    const editor = await open(page, appURL);
    const source = editor.getByLabel("Selected organization definition source");
    await editor.getByRole("button", { name: "Add document", exact: true }).click();
    await editor.getByLabel("Document path", { exact: true }).fill("new-policy.zpl");
    await editor.getByLabel("Document kind", { exact: true }).selectOption("policy");
    await source.fill("Unsaved new policy\n");
    await choose(editor, -1);
    await choose(editor, 2);
    await expect(source).toHaveValue("Unsaved new policy\n");
    const dialogHandler = dialog => dialog.accept();
    page.once("dialog", dialogHandler);
    await editor.getByRole("button", { name: "Remove document", exact: true }).click();
    await expect(editor.getByLabel("Definition file", { exact: true }).locator("option")).toHaveCount(3);
    await choose(editor, 1);
    await source.fill("Keep this unsaved policy\n");
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect(editor.locator("#organization-definitions-status")).toContainText("Save not confirmed");
    await expect(source).toHaveValue("Keep this unsaved policy\n");
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 1");
    expect(state.saves[0].bundle.documents).toHaveLength(2);
  });
  test("Organization definitions reject delayed AI after a document edit and clear all data on logout", async ({ page, appURL, api }) => {
    fixture(api);
    let release;
    let started = false;
    const gate = new Promise(resolve => { release = resolve; });
    api.handlers.set(endpoint + "/assistant", async route => {
      started = true;
      const request = route.request().postDataJSON();
      await gate;
      await route.fulfill({ json: { answer: "Delayed suggestion", proposal: { bundle: request.bundle } } });
    });
    const editor = await open(page, appURL);
    const source = editor.getByLabel("Selected organization definition source");
    const pane = editor.locator("#organization-definitions-assistant");
    await pane.getByLabel("Message", { exact: true }).fill("Review definitions");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    await expect.poll(() => started).toBe(true);
    await choose(editor, 1);
    await source.fill("Newer local policy");
    const response = page.waitForResponse(result => new URL(result.url()).pathname === endpoint + "/assistant");
    release();
    await response;
    await expect(pane.getByRole("button", { name: "Review definition proposal" })).toHaveCount(0);
    await expect(source).toHaveValue("Newer local policy");
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    await expect(editor).not.toBeVisible();
    await expect(source).toHaveValue("");
    await expect(editor.getByLabel("Definition file", { exact: true }).locator("option")).toHaveCount(1);
  });
  test("Organization definitions historical profile selection loads the complete bundle not mixed documents", async ({ page, appURL, api }) => {
    const state = fixture(api);
    const editor = await open(page, appURL);
    await choose(editor, 0);
    await editor.getByLabel("Selected organization definition source").fill("dn: dc=alpha,dc=test\nobjectClass: domain\ndc: alpha\n# changed directory\n");
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 2");
    await editor.getByRole("button", { name: "Close definitions" }).click();
    await page.locator("#organization-drafts-list .organization-card").click();
    const profileEditor = page.locator("#organization-profile-editor");
    await profileEditor.getByLabel("Version history", { exact: true }).click();
    await profileEditor.getByRole("button", { name: "Revision 1: Initial snapshot" }).click();
    await expect(editor).toBeVisible();
    await expect(editor.locator("#organization-definitions-status")).toContainText("Profile and documents are from the same snapshot");
    await choose(editor, 0);
    await expect(editor.getByLabel("Selected organization definition source")).toHaveValue(initial().documents[0].content);
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 3");
    expect(state.saves[1].bundle).toEqual(initial());
    expect(state.saves[1].expected_revision).toBe(2);
  });
  test("Organization definitions lost save response reloads the persisted coherent revision without another save", async ({ page, appURL, api }) => {
    fixture(api);
    let persisted = initial();
    let writes = 0;
    api.handlers.set(endpoint, route => {
      if (route.request().method() === "PUT") {
        writes++;
        persisted = route.request().postDataJSON().bundle;
        return route.fulfill({ status: 503, json: { error: "Receipt lost after persistence" } });
      }
      return route.fulfill({ json: { revision: writes ? 2 : 1, content: persisted } });
    });
    const editor = await open(page, appURL);
    await choose(editor, 1);
    const source = editor.getByLabel("Selected organization definition source");
    await source.fill("Saved but response uncertain\n");
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect(editor.locator("#organization-definitions-status")).toContainText("reload to reconcile");
    await expect(source).toHaveValue("Saved but response uncertain\n");
    page.once("dialog", dialog => dialog.accept());
    await editor.getByRole("button", { name: "Reload definitions" }).click();
    await expect(editor.locator("#organization-definitions-revision")).toHaveText("Draft revision 2");
    await choose(editor, 1);
    await expect(source).toHaveValue("Saved but response uncertain\n");
    expect(writes).toBe(1);
  });
  test("Organization definitions delayed save receipt cannot reopen the workspace after logout", async ({ page, appURL, api }) => {
    fixture(api);
    let release;
    let started = false;
    const gate = new Promise(resolve => { release = resolve; });
    api.handlers.set(endpoint, async route => {
      if (route.request().method() !== "PUT") return route.fulfill({ json: { revision: 1, content: initial() } });
      started = true;
      const bundle = route.request().postDataJSON().bundle;
      await gate;
      return route.fulfill({ json: { revision: 2, content: bundle } });
    });
    const editor = await open(page, appURL);
    await choose(editor, 1);
    await editor.getByLabel("Selected organization definition source").fill("Unsaved policy");
    await editor.getByRole("button", { name: "Save all definitions" }).click();
    await expect.poll(() => started).toBe(true);
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    const response = page.waitForResponse(result => result.request().method() === "PUT" && new URL(result.url()).pathname === endpoint);
    release();
    await response;
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await expect(editor).not.toBeVisible();
    await expect(editor.getByLabel("Selected organization definition source")).toHaveValue("");
    await expect(page.locator("#organization-drafts-status")).toContainText("Operator session ended");
  });
  test.describe("Organization definitions narrow viewport", () => {
    test.use({ viewport: { width: 390, height: 844 } });
    test("Organization definitions narrow source and AI review remain usable without activation", async ({ page, appURL, api }) => {
      fixture(api);
      const editor = await open(page, appURL);
      await expect.poll(() => editor.evaluate(element => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth + 1 && element.scrollWidth <= element.clientWidth + 1;
      })).toBe(true);
      await choose(editor, 0);
      const pane = editor.locator("#organization-definitions-assistant");
      await pane.getByLabel("Message", { exact: true }).fill("Improve definitions");
      await pane.getByRole("button", { name: "Send", exact: true }).click();
      await pane.getByRole("button", { name: "Review definition proposal" }).click();
      await page.getByRole("dialog", { name: "Review AI definition changes" }).getByRole("button", { name: "Cancel", exact: true }).click();
      await expect(editor.getByLabel("Selected organization definition source")).toHaveValue(initial().documents[0].content);
      await editor.getByRole("button", { name: "Validate all definitions" }).click();
      await expect(editor.locator("#organization-definitions-status")).toContainText("not runnable");
    });
  });
}
