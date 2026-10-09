export function registerWorkflowGuidelineTests(test, expect) {
  async function openAssertionRecord(page, appURL) {
    await page.goto(appURL + "/#policy");
    const picker = page.locator("#policy-picker-toggle");
    if (await picker.getAttribute("aria-expanded") !== "true") await picker.click();
    await expect(page.locator("#policy-catalog-pane")).toBeVisible();
    await page.locator('[data-record-id="test-assertions"]').click();
    await expect(page.locator("#policy-assertion-editor")).toBeVisible();
  }

  function invitation(id, name, inventory, state, expiry) {
    return {
      id, asset: { organization: "alpha", asset_id: inventory, name, owner: "Owner", type: "laptop", profile: "standard", recipient: "owner@example.org" },
      state, revision: 2, created_by: 'oidc:["https://identity.example","admin"]',
      created_at: "2026-10-07T12:00:00Z", expires_at: expiry,
      claimed_at: "2026-10-07T12:10:00Z", approval_expires_at: "2026-10-07T12:40:00Z",
      key_fingerprint: "b".repeat(64),
    };
  }

  function registry(api, handler) {
    api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: {
      organizations: { alpha: { types: ["laptop"], profiles: ["standard"] } },
      invitation_lifetime_seconds: 3600, approval_lifetime_seconds: 1800, gui_mutations_enabled: false,
    } }));
    api.handlers.set("/api/enrollment/v1/invitations", handler);
  }

  const firstColumn = (page, id) => page.locator(`#${id} tr td:first-child`);
  const header = (page, id, key) => page.locator(`#${id}`).locator("xpath=ancestor-or-self::table").locator(`th[data-sort-key="${key}"]`);

  test("workflow guidelines sort only the loaded invitation page and preserve fresh fingerprint details", async ({ page, appURL, api }) => {
    const items = [
      invitation("zeta", "Zeta", "INV-2", "pending_approval", "2026-10-07T14:00:00+02:00"),
      invitation("alpha", "Alpha", "INV-10", "invited", "2026-10-07T13:00:00Z"),
      invitation("beta", "Beta", "INV-1", "approved", "2026-10-07T11:00:00-03:00"),
    ];
    const nextItems = [
      invitation("next-z", "Next Z", "INV-22", "invited", "2026-10-07T16:00:00Z"),
      invitation("next-a", "Next A", "INV-11", "invited", "2026-10-07T15:00:00Z"),
    ];
    const queries = [];
    registry(api, (route) => {
      const query = new URL(route.request().url()).searchParams;
      queries.push(query.toString());
      return route.fulfill({ json: query.has("after") ? { invitations: nextItems } : { invitations: items, next_after: "beta" } });
    });
    api.handlers.set("/api/enrollment/v1/invitations/zeta", (route) => route.fulfill({ json: items[0] }));
    await page.goto(appURL + "/#provisioning-adapters");
    const rows = firstColumn(page, "provisioning-invitations");
    await expect(rows).toHaveText(["Zeta", "Alpha", "Beta"]);
    const initialReads = api.counts.get("/api/enrollment/v1/invitations");
    const name = header(page, "provisioning-invitations", "name");
    await name.getByRole("button").focus();
    await page.keyboard.press("Enter");
    await expect(rows).toHaveText(["Alpha", "Beta", "Zeta"]);
    await expect(name).toHaveAttribute("aria-sort", "ascending");
    await name.getByRole("button").press("Space");
    await expect(rows).toHaveText(["Zeta", "Beta", "Alpha"]);
    await expect(name).toHaveAttribute("aria-sort", "descending");
    await header(page, "provisioning-invitations", "inventory").getByRole("button").click();
    await expect(rows).toHaveText(["Beta", "Zeta", "Alpha"]);
    await expect(name).toHaveAttribute("aria-sort", "none");
    await header(page, "provisioning-invitations", "state").getByRole("button").click();
    await expect(rows).toHaveText(["Beta", "Alpha", "Zeta"]);
    await header(page, "provisioning-invitations", "expiry").getByRole("button").click();
    await expect(header(page, "provisioning-invitations", "expiry")).toHaveAttribute("aria-sort", "ascending");
    await expect(rows).toHaveText(["Zeta", "Alpha", "Beta"]);
    expect(api.counts.get("/api/enrollment/v1/invitations")).toBe(initialReads);
    await page.getByRole("button", { name: "Details for Zeta", exact: true }).click();
    const detail = page.getByRole("dialog", { name: "Enrollment request details" });
    await expect(detail).toContainText("b".repeat(64));
    await expect(detail).toContainText("independent trusted channel");
    await expect(detail).toContainText("No enrollment code is available");
    await page.keyboard.press("Escape");
    await page.locator("#provisioning-next").click();
    await expect(rows).toHaveText(["Next A", "Next Z"]);
    await expect(header(page, "provisioning-invitations", "expiry")).toHaveAttribute("aria-sort", "ascending");
    expect(queries).toEqual(["organization=alpha&limit=50", "organization=alpha&limit=50&after=beta"]);
  });

  test("workflow invitation sorting does not disguise denied or malformed registry reads", async ({ page, appURL, api }) => {
    let broken = false;
    registry(api, (route) => route.fulfill({ json: {
      invitations: broken ? [{ invalid: true }] : [invitation("valid", "Valid", "INV-1", "invited", "2026-10-07T13:00:00Z")],
    } }));
    await page.goto(appURL + "/#provisioning-adapters");
    await expect(firstColumn(page, "provisioning-invitations")).toHaveText(["Valid"]);
    await header(page, "provisioning-invitations", "name").getByRole("button").click();
    broken = true;
    await page.getByRole("button", { name: "Check again", exact: true }).click();
    await expect(page.locator("#provisioning-registry-status")).toContainText("not an empty queue");
    await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
    api.handlers.set("/api/enrollment/v1/invitations", (route) => route.fulfill({ status: 403, json: { error: "denied" } }));
    await page.getByRole("button", { name: "Check again", exact: true }).click();
    await expect(page.locator("#provisioning-invitations")).toBeHidden();
    await expect(page.locator("#provisioning-organization")).toBeDisabled();
  });

  test("workflow assertion catalogs sort exact counts and retain source-qualified insertion", async ({ page, appURL, api }) => {
    api.assertionSource.sources = [
      {
        name: "staff", people: 1001, observed_at: "2026-10-07T12:00:00Z",
        groups: [{ name: "Zeta", members: 10 }, { name: "Alpha", members: 1001 }, { name: "Beta", members: 2 }],
        attributes: [{ name: "zeta", people: 10, groups: 0 }, { name: "alpha", people: 1001, groups: 2 }, { name: "beta", people: 2, groups: 10 }],
      },
      { name: "hr", people: 10, observed_at: "2026-10-07T12:00:00Z", groups: [{ name: "HR Z", members: 10 }, { name: "HR A", members: 2 }], attributes: [] },
    ];
    api.assertionSource.default_source = "staff";
    await openAssertionRecord(page, appURL);
    await page.locator("#assertion-source").fill("");
    await page.locator("#assertion-read-source").click();
    const groups = firstColumn(page, "assertion-group-rows");
    await expect(groups).toHaveText(["Zeta", "Alpha", "Beta"]);
    const sourceReads = api.counts.get("/api/assertions/source");
    await header(page, "assertion-group-rows", "name").getByRole("button").click();
    await expect(groups).toHaveText(["Alpha", "Beta", "Zeta"]);
    const members = header(page, "assertion-group-rows", "members");
    await members.getByRole("button").click();
    await expect(groups).toHaveText(["Beta", "Zeta", "Alpha"]);
    await expect(members).toHaveAttribute("data-numeric", "true");
    await expect(page.locator("#assertion-group-rows td:nth-child(2)")).toHaveText(["2", "10", "1001"]);
    await expect(page.locator("#assertion-group-rows td:nth-child(2)").last()).toHaveAttribute("data-numeric", "true");
    await members.getByRole("button").click();
    await expect(groups).toHaveText(["Alpha", "Zeta", "Beta"]);
    await page.getByRole("button", { name: "Insert a cardinality assertion for Beta", exact: true }).click();
    await expect(page.locator("#assertion-source")).toHaveValue('group "Beta" from "staff" members >= 2;\n');
    await page.getByRole("tab", { name: "Attributes", exact: true }).click();
    const attributes = firstColumn(page, "assertion-attribute-rows");
    await header(page, "assertion-attribute-rows", "name").getByRole("button").click();
    await expect(attributes).toHaveText(["alpha", "beta", "zeta"]);
    await header(page, "assertion-attribute-rows", "people").getByRole("button").click();
    await expect(attributes).toHaveText(["beta", "zeta", "alpha"]);
    await expect(page.locator("#assertion-attribute-rows td:nth-child(2)")).toHaveText(["2", "10", "1001"]);
    await header(page, "assertion-attribute-rows", "groups").getByRole("button").click();
    await expect(attributes).toHaveText(["zeta", "alpha", "beta"]);
    await expect(page.locator("#assertion-group-rows .poll-changed, #assertion-attribute-rows .poll-changed")).toHaveCount(0);
    await page.locator("#assertion-source").fill("");
    await page.getByRole("button", { name: "Insert presence assertion for beta", exact: true }).click();
    await expect(page.locator("#assertion-source")).toHaveValue('people from "staff" attribute "beta" present;\n');
    expect(api.counts.get("/api/assertions/source")).toBe(sourceReads);
    api.assertionSource.sources[0].groups[1].members = 1002;
    await page.locator("#assertion-read-source").click();
    await expect(page.locator("#assertion-group-rows td:nth-child(2)").first()).toHaveText("1002");
    await expect(page.locator("#assertion-group-rows td:nth-child(2)").first().locator(".poll-changed")).toHaveCount(1);
    await page.getByLabel("Assertion trusted source").selectOption("hr");
    await page.getByRole("tab", { name: "Groups", exact: true }).click();
    await expect(groups).toHaveText(["HR Z", "HR A"]);
    await expect(page.locator("#assertion-group-rows .poll-changed")).toHaveCount(0);
    await expect(members).toHaveAttribute("aria-sort", "descending");
    await page.locator("#assertion-source").fill("");
    await page.getByRole("button", { name: "Insert a cardinality assertion for HR A", exact: true }).click();
    await expect(page.locator("#assertion-source")).toHaveValue('group "HR A" from "hr" members >= 2;\n');
  });

  test("workflow assertion result sorting leaves evaluation source, errors and gutter ownership intact", async ({ page, appURL, api }) => {
    const source = Array.from({ length: 12 }, (_, index) => `group "Group ${index + 1}" members >= 2;`).join("\n");
    const results = [
      { rule: { line: 10, kind: "group", group: "Zeta", operator: ">=", limit: 2 }, status: "pass", checked: 10, violations: 0, subjects: [], message: "10 checked; 0 violations" },
      { rule: { line: 2, kind: "group", group: "Alpha", operator: ">=", limit: 2 }, status: "fail", checked: 2, violations: 2, subjects: ["alice", "bob"], message: "2 checked; 2 violations" },
      { rule: { line: 12, kind: "group", group: "Beta", operator: ">=", limit: 2 }, status: "fail", checked: 1001, violations: 10, subjects: ["carol"], message: "1001 checked; 10 violations" },
    ];
    let failure = false;
    api.handlers.set("/api/assertions/evaluate", (route) => route.fulfill({ json: {
      revision: 0, draft: true, status: failure ? "error" : "fail", finished_at: "2026-10-07T12:00:00Z",
      error: failure ? "Trusted source unavailable; no assertions were evaluated" : "",
      results: failure ? [] : results,
    } }));
    await openAssertionRecord(page, appURL);
    await page.locator("#assertion-source").fill(source);
    await page.locator("#assertion-analyze").click();
    const lines = firstColumn(page, "assertion-result-rows");
    await expect(lines).toHaveText(["10", "2", "12"]);
    await expect(page.locator(".assertion-results")).toBeVisible();
    const evaluations = api.counts.get("/api/assertions/evaluate");
    for (const [key, expected] of [
      ["line", ["2", "10", "12"]], ["assertion", ["2", "12", "10"]],
      ["status", ["2", "12", "10"]], ["checked", ["2", "10", "12"]], ["violations", ["10", "2", "12"]],
    ]) {
      const heading = header(page, "assertion-result-rows", key);
      await heading.getByRole("button").click();
      await expect(lines).toHaveText(expected);
      await expect(heading).toHaveAttribute("aria-sort", "ascending");
    }
    await header(page, "assertion-result-rows", "violations").getByRole("button").click();
    await expect(lines).toHaveText(["12", "2", "10"]);
    await expect(page.locator("#assertion-result-rows td:nth-child(4)").first()).toContainText("1001");
    await expect(page.locator("#assertion-result-rows td:nth-child(4)").first()).toHaveAttribute("data-numeric", "true");
    await expect(page.locator("#assertion-result-rows")).toContainText("alice, bob");
    await expect(page.locator("#assertion-source")).toHaveValue(source);
    expect(api.counts.get("/api/assertions/evaluate")).toBe(evaluations);
    await expect(page.locator("#assertion-result-rows .poll-changed")).toHaveCount(0);
    const marker = page.locator('#assertion-result-gutter [data-line="2"] .assertion-result-marker');
    await expect(marker).toHaveAttribute("data-state", "fail");
    await marker.click();
    const detail = page.getByRole("dialog", { name: "Assertion fail" });
    await expect(detail).toContainText("Alpha");
    await expect(detail).toContainText("alice");
    await detail.locator(".dialog-actions .button").click();
    await page.locator("#assertion-analyze").click();
    failure = true;
    await page.locator("#assertion-analyze").click();
    await expect(page.locator("#assertion-run-error")).toContainText("Trusted source unavailable");
    await expect(lines).toHaveCount(0);
    await expect(page.locator("#assertion-result-gutter .assertion-result-marker")).toHaveCount(0);
    await expect(header(page, "assertion-result-rows", "violations")).toHaveAttribute("aria-sort", "descending");
    failure = false;
    await page.locator("#assertion-analyze").click();
    await page.locator("#assertion-analyze").click();
    await expect(lines).toHaveText(["12", "2", "10"]);
    await expect(page.locator("#assertion-run-error")).toBeEmpty();
  });
}
