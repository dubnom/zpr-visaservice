async function openPolicyPicker(page) {
  const toggle = page.locator("#policy-picker-toggle");
  if (await toggle.getAttribute("aria-expanded") !== "true") await toggle.click();
  await expect(page.locator("#policy-catalog-pane")).toBeVisible();
}

async function openPolicyFiles(page) {
  const toggle = page.locator("#policy-files-toggle");
  if (await toggle.getAttribute("aria-expanded") !== "true") await toggle.click();
  await expect(page.locator("#policy-file-menu")).toBeVisible();
}

async function openAssertionRecord(page, appURL) {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await expect(page.locator('[data-page-link="assertions"]')).toHaveCount(0);
  await expect(page.locator('[data-category-id="test"]')).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("treeitem", { name: "Assertions", exact: true })).toHaveCount(0);
  const record = page.locator('[data-record-id="test-assertions"]');
  await expect(record).toBeVisible();
  await record.click();
  await expect(page.locator("#policy-assertion-editor")).toBeVisible();
}

function registerAssertionBrowserTests() {
test("assertion editor shares policy file controls and clears the scrollbar gutter", async ({ page, appURL, api }) => {
  await openAssertionRecord(page, appURL);
  await expect(page.locator("#policy-editor-mode")).toHaveText("Assertion");
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  await expect(page.locator("#policy-picker-toggle")).toBeVisible();
  await expect(page.locator("#policy-files-toggle")).toBeVisible();
  await expect(page.locator("#assertion-save")).toBeHidden();
  await expect(page.locator("#assertion-message")).toBeHidden();
  const spacing = await page.evaluate(() => {
    const toolbar = document.querySelector(".policy-editor-tools").getBoundingClientRect();
    const editor = document.querySelector("#assertion-editor").getBoundingClientRect();
    return editor.top - toolbar.bottom;
  });
  expect(spacing).toBeLessThanOrEqual(9);
  await openPolicyFiles(page);
  await expect(page.locator("#assertion-save")).toBeVisible();
  await expect(page.locator("#assertion-reload")).toHaveText("Discard");
  await expect(page.locator("#policy-stage")).toBeHidden();
  await page.locator("#policy-files-toggle").click();
  await page.locator("#assertion-source").fill(`group "${"Operators".repeat(80)}" members >= 2;`);
  await expect(page.locator("#policy-modified-indicator")).toHaveText("Modified");
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-result-gutter")).toBeVisible();
  await expect.poll(() => page.evaluate(() => {
    const source = document.querySelector("#assertion-source");
    const gutter = document.querySelector("#assertion-result-gutter").getBoundingClientRect();
    const sourceBounds = source.getBoundingClientRect();
    return source.scrollWidth > source.clientWidth && sourceBounds.left >= gutter.right && gutter.bottom <= sourceBounds.top + source.clientHeight + 1;
  })).toBeTruthy();
  await openPolicyPicker(page);
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator(`[data-record-id="${api.policy.records.find((record) => record.kind === "policy").id}"]`).click();
  await openPolicyFiles(page);
  await expect(page.locator("#policy-save")).toBeVisible();
  await expect(page.locator("#assertion-save")).toBeHidden();
  await expect(page.locator(".policy-editor-tools > .assertion-actions")).toBeHidden();
});

test("assertion Analyze matches policy toolbar colors and gutter behavior", async ({ page, appURL, api }) => {
  let analyzed = "";
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    analyzed = route.request().postDataJSON().source;
    await route.fulfill({ json: { status: "pass", revision: 0, draft: true, finished_at: "2026-10-05T12:00:00Z", warnings: [], results: [{ rule: { line: 1, kind: "group", group: "Operators", operator: ">=", limit: 2 }, status: "pass", checked: 1, violations: 0, subjects: [], message: "1 checked; 0 violations" }] } });
  });
  api.handlers.set("/api/assertions/format", async (route) => {
    expect(route.request().postDataJSON().source).toBe('group   "Operators" members>=2;');
    await route.fulfill({ json: { rule_count: 1, source: 'group "Operators" members >= 2;\n', warnings: [] } });
  });
  await openAssertionRecord(page, appURL);
  await expect(page.getByRole("button", { name: "Test", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Exit test", exact: true })).toHaveCount(0);
  await expect(page.locator("#assertion-result-gutter")).toBeVisible();
  const before = await page.locator("#assertion-source").boundingBox();
  const toolbar = await page.evaluate(() => {
    const files = document.querySelector("#policy-files-toggle").getBoundingClientRect();
    const analyze = document.querySelector("#assertion-analyze").getBoundingClientRect();
    const format = document.querySelector("#assertion-format").getBoundingClientRect();
    return { gap: analyze.left - files.right, sameRow: Math.abs(analyze.top - files.top) < 1 && Math.abs(format.top - files.top) < 1 };
  });
  expect(toolbar.gap).toBeLessThanOrEqual(9);
  expect(toolbar.sameRow).toBeTruthy();
  await expect(page.locator("#assertion-periodic-control")).toBeHidden();
  await expect(page.locator("#assertion-interval-control")).toBeHidden();
  const source = page.locator("#assertion-source");
  await source.fill('group   "Operators" members>=2;');
  await expect(page.locator("#assertion-analyze")).toHaveClass(/button-next-evaluate/);
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-analyze")).toHaveAttribute("data-analysis-state", "success");
  await expect(source).toBeEditable();
  const after = await source.boundingBox();
  expect(after.x).toBe(before.x);
  expect(after.width).toBe(before.width);
  await page.mouse.move(0, 0);
  await expect.poll(() => page.evaluate(() => {
    const assertion = document.querySelector("#assertion-analyze");
    const policy = document.querySelector("#policy-check");
    policy.dataset.analysisState = "success";
    return getComputedStyle(assertion).backgroundColor === getComputedStyle(policy).backgroundColor;
  })).toBeTruthy();
  await page.locator(".assertion-result-marker").click();
  await expect(page.getByRole("dialog", { name: "Assertion pass" })).toBeVisible();
  await page.getByRole("dialog", { name: "Assertion pass" }).locator(".dialog-actions .button").click();
  expect(analyzed).toBe('group   "Operators" members>=2;');
  await page.locator("#assertion-format").click();
  await expect(source).toHaveValue('group "Operators" members >= 2;\n');
  await expect(page.locator("#assertion-message")).toHaveText("Assertions formatted.");
  await openPolicyFiles(page);
  await expect(page.locator("#assertion-save")).toBeEnabled();
  expect(api.counts.get("/api/assertions/evaluate") || 0).toBe(1);
  expect(api.assertions.settings.source).toBe("");
  await page.locator("#policy-files-toggle").click();
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    await route.fulfill({ status: 400, json: { error: "line 1: expected a number" } });
  });
  await source.fill('group "Operators" members >=;');
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-analyze")).toHaveAttribute("data-analysis-state", "error");
  await expect(source).toBeEditable();
  await page.mouse.move(0, 0);
  await expect.poll(() => page.evaluate(() => {
    const assertion = document.querySelector("#assertion-analyze");
    const policy = document.querySelector("#policy-check");
    policy.dataset.analysisState = "error";
    return getComputedStyle(assertion).backgroundColor === getComputedStyle(policy).backgroundColor;
  })).toBeTruthy();
  await page.locator('.assertion-result-marker[data-state="error"]').click();
  await expect(page.getByRole("dialog", { name: "Assertion error" })).toContainText("line 1: expected a number");
});

test("assertion lint warns on complexity without changing a passing result", async ({ page, appURL, api }) => {
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    const request = route.request().postDataJSON();
    await route.fulfill({ json: {
      revision: request.expected_revision, draft: true, status: "pass", finished_at: "2026-10-05T12:00:00Z", results: [],
      warnings: [{ code: "ASSERT_COMPLEXITY", severity: "warning", line: 1, message: "Split this complex assertion into smaller checks." }],
    } });
  });
  await openAssertionRecord(page, appURL);
  const editor = page.getByRole("textbox", { name: "Data assertion source", exact: true });
  await editor.fill(`assert ${"1 + ".repeat(13)}1 > 0;`);
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-run-status")).toContainText("PASS");
  await expect(page.locator("#assertion-lint-warnings")).toContainText("ASSERT_COMPLEXITY");
  await expect(page.locator("#assertion-run-error")).toBeEmpty();
  await page.getByRole("button", { name: "Exit test", exact: true }).click();
  await editor.fill('group "Operators" members >= 2;');
  await expect(page.locator("#assertion-lint-warnings")).toBeHidden();
});

test("organization assertions author, save and evaluate without policy compilation", async ({ page, appURL, api }) => {
  api.handlers.set("/api/assertions", async (route) => {
    if (route.request().method() === "PUT") {
      const request = route.request().postDataJSON();
      expect(request.expected_revision).toBe(api.assertions.settings.revision);
      api.assertions.settings = { revision: request.expected_revision + 1, source: request.source, enabled: request.enabled, interval_seconds: request.interval_seconds };
      const assertionRecord = api.policy.records.find((record) => record.id === "test-assertions");
      assertionRecord.current_revision = request.expected_revision + 1;
      assertionRecord.content = JSON.stringify({ source: request.source, enabled: request.enabled, interval_seconds: request.interval_seconds });
    }
    await route.fulfill({ json: api.assertions });
  });
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    const request = route.request().postDataJSON();
    expect(Object.keys(request).sort()).toEqual(["expected_revision", "source"]);
    api.assertions.last_run = {
      revision: request.expected_revision, draft: request.source !== api.assertions.settings.source,
      status: "pass", finished_at: "2026-10-02T12:00:00Z",
      results: [{ rule: { line: 1, kind: "group", group: "Operators", operator: ">=", limit: 2 }, status: "pass", checked: 1, violations: 0, subjects: [], message: "1 checked; 0 violations" }],
    };
    await route.fulfill({ json: api.assertions.last_run });
  });
  await openAssertionRecord(page, appURL);
  const editor = page.getByRole("textbox", { name: "Data assertion source", exact: true });
  await expect(editor).toBeEnabled();
  await expect(page.locator("#assertion-enabled")).not.toBeChecked();
  expect(await page.getByRole("button", { name: "Read source", exact: true }).evaluate((button) => button.scrollWidth <= button.clientWidth && button.scrollHeight <= button.clientHeight)).toBeTruthy();
  await page.getByRole("button", { name: "Read source", exact: true }).click();
  await expect(page.locator("#assertion-group-rows")).toContainText("Operators");
  await page.getByRole("button", { name: "Insert a cardinality assertion for Operators", exact: true }).click();
  await expect(editor).toHaveValue('group "Operators" members >= 2;\n');
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-run-status")).toContainText("Draft r0");
  const resultGutterStyle = await page.locator("#assertion-result-gutter").evaluate((gutter) => ({
    width: gutter.getBoundingClientRect().width,
    border: getComputedStyle(gutter).borderRightStyle,
    background: getComputedStyle(gutter).backgroundColor,
  }));
  expect(resultGutterStyle.width).toBeLessThan(90);
  expect(resultGutterStyle.border).toBe("solid");
  expect(resultGutterStyle.background).not.toBe("rgba(0, 0, 0, 0)");
  const passMarker = page.locator('#assertion-result-gutter [data-line="1"] .assertion-result-marker');
  await expect(passMarker).toHaveAttribute("data-state", "pass");
  await passMarker.click();
  const passDetails = page.getByRole("dialog", { name: "Assertion pass" });
  await expect(passDetails).toContainText('group "Operators" members >= 2');
  await expect(passDetails).toContainText("1 checked; 0 violations");
  await passDetails.locator(".dialog-actions .button").click();
  await expect(page.locator("#assertion-evaluate")).toHaveText("Exit test");
  await expect(editor).not.toBeEditable();
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-evaluate")).toHaveText("Test");
  await expect(editor).toBeEditable();
  expect(api.assertions.settings.source).toBe("");
  await page.locator("#assertion-save").click();
  await expect(page.locator("#assertion-revision")).toHaveText("Alpha Labs / r1");
  expect(api.assertions.settings.enabled).toBe(false);
  await page.locator("#assertion-enabled").check();
  await page.locator("#assertion-interval").fill("300");
  await page.locator("#assertion-save").click();
  await expect(page.locator("#assertion-revision")).toHaveText("Alpha Labs / r2");
  await expect(page.locator('[data-record-id="test-assertions"]')).toContainText("Assertions · r2");
  expect(api.assertions.settings.enabled).toBe(true);
  expect(api.assertions.settings.interval_seconds).toBe(300);
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows")).toContainText("PASS");
  await expect(page.locator("#assertion-run-status")).toContainText("Saved r2");
  await page.locator("#assertion-evaluate").click();
  expect(api.counts.get("/api/policy/test") || 0).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("assertion result gutter clips long documents and follows editor scroll", async ({ page, appURL, api }) => {
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    const request = route.request().postDataJSON();
    api.assertions.last_run = {
      revision: request.expected_revision, draft: true, status: "pass", finished_at: "2026-10-02T12:00:00Z",
      results: [{ rule: { line: 90, kind: "group", group: "Operators", operator: ">=", limit: 2 }, status: "pass", checked: 1, violations: 0, subjects: [], message: "1 checked; 0 violations" }],
    };
    await route.fulfill({ json: api.assertions.last_run });
  });
  await openAssertionRecord(page, appURL);
  const source = page.locator("#assertion-source");
  await source.fill(Array.from({ length: 100 }, (_, index) => `group "Operators${index}" members >= 2;`).join("\n"));
  await page.locator("#assertion-evaluate").click();
  const gutter = page.locator("#assertion-result-gutter");
  const marker = page.locator('#assertion-result-gutter [data-line="90"] .assertion-result-marker');
  await expect(marker).toHaveCount(1);
  await source.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  const initial = await page.evaluate(() => {
    const gutter = document.querySelector("#assertion-result-gutter");
    const rows = document.querySelector("#assertion-result-lines");
    const gutterBounds = gutter.getBoundingClientRect();
    return { overflow: getComputedStyle(gutter).overflowY, gutterHeight: gutterBounds.height, rowsHeight: rows.getBoundingClientRect().height };
  });
  expect(initial.overflow).toBe("hidden");
  expect(initial.rowsHeight).toBeGreaterThan(initial.gutterHeight);
  await source.evaluate((element) => { element.scrollTop = element.scrollHeight; element.dispatchEvent(new Event("scroll")); });
  await expect.poll(() => page.evaluate(() => {
    const source = document.querySelector("#assertion-source");
    return document.querySelector("#assertion-result-lines").style.transform === `translateY(${-source.scrollTop}px)`;
  })).toBeTruthy();
  await expect.poll(() => page.evaluate(() => {
    const bounds = document.querySelector("#assertion-result-gutter").getBoundingClientRect();
    const marker = document.querySelector('#assertion-result-gutter [data-line="90"] .assertion-result-marker').getBoundingClientRect();
    return marker.top >= bounds.top && marker.bottom <= bounds.bottom;
  })).toBeTruthy();
  const scrolled = await page.evaluate(() => {
    const source = document.querySelector("#assertion-source");
    const gutterBounds = document.querySelector("#assertion-result-gutter").getBoundingClientRect();
    const rows = document.querySelector("#assertion-result-lines");
    const markerBounds = document.querySelector('#assertion-result-gutter [data-line="90"] .assertion-result-marker').getBoundingClientRect();
    return { markerTop: markerBounds.top, markerBottom: markerBounds.bottom, gutterTop: gutterBounds.top, gutterBottom: gutterBounds.bottom, scrollTop: source.scrollTop, scrollHeight: source.scrollHeight, clientHeight: source.clientHeight, transform: getComputedStyle(rows).transform };
  });
  expect(scrolled.markerTop).toBeGreaterThanOrEqual(scrolled.gutterTop);
  expect(scrolled.markerBottom, JSON.stringify(scrolled)).toBeLessThanOrEqual(scrolled.gutterBottom);
  await expect(gutter).toBeVisible();
});

test("policy and assertion editors fill the available page height", async ({ page, appURL, api }) => {
  for (const width of [1440, 834]) {
    await page.setViewportSize({ width, height: 1200 });
    await page.goto(appURL + "/#policy");
    await openPolicyPicker(page);
    await page.locator(`[data-record-id="${api.policy.records[0].id}"]`).click();
    await expect(page.locator("#policy-source")).toBeEnabled();
    const policy = await page.locator("#policy-code-editor").boundingBox();
    const policyPane = await page.locator("#policy-editor-pane").boundingBox();
    expect(policy.y + policy.height).toBeGreaterThanOrEqual(policyPane.y + policyPane.height - 13);
    if (width > 900) expect(policy.y + policy.height).toBeGreaterThanOrEqual(1150);
    await openAssertionRecord(page, appURL);
    await expect(page.locator("#assertion-source")).toBeEnabled();
    const assertion = await page.locator("#assertion-editor").boundingBox();
    const assertionPanel = await page.locator("#policy-assertion-editor").boundingBox();
    expect(assertionPanel.y + assertionPanel.height).toBeGreaterThanOrEqual(1150);
    expect(assertion.height).toBeGreaterThan(360);
  }
});

test("assertion attribute catalog inserts rules and renders typed comparisons", async ({ page, appURL, api }) => {
  api.assertionSource.attributes = [{ name: "mail", people: 2, groups: 0 }, { name: "gidnumber", people: 2, groups: 1 }];
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    const request = route.request().postDataJSON();
    expect(Object.keys(request).sort()).toEqual(["expected_revision", "source"]);
    const numeric = request.source.includes("gidNumber");
    api.assertions.last_run = {
      revision: 0, draft: true, status: numeric ? "pass" : "fail", finished_at: "2026-10-02T12:00:00Z",
      results: [{ rule: numeric ? { line: 1, kind: "each_group_attribute", attribute: "gidnumber", operator: ">=", number: 1000 } : { line: 1, kind: "people_attribute", attribute: "mail", operator: "present" }, status: numeric ? "pass" : "fail", checked: 2, violations: numeric ? 0 : 1, subjects: numeric ? [] : ["bob"], message: numeric ? "2 checked; 0 violations" : "2 checked; 1 violations" }],
    };
    await route.fulfill({ json: api.assertions.last_run });
  });
  await openAssertionRecord(page, appURL);
  await expect(page.locator("#assertion-read-source")).toBeEnabled();
  await page.locator("#assertion-read-source").click();
  await page.getByRole("tab", { name: "Attributes", exact: true }).click();
  await expect(page.locator("#assertion-attribute-rows tr")).toHaveCount(2);
  await page.getByRole("button", { name: "Insert presence assertion for mail", exact: true }).click();
  await expect(page.locator("#assertion-source")).toHaveValue('people attribute "mail" present;\n');
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows")).toContainText('people attribute "mail" present');
  await expect(page.locator("#assertion-result-rows")).toContainText("bob");
  const failMarker = page.locator('#assertion-result-gutter [data-line="1"] .assertion-result-marker');
  await expect(failMarker).toHaveAttribute("data-state", "fail");
  await failMarker.click();
  const failDetails = page.getByRole("dialog", { name: "Assertion fail" });
  await expect(failDetails).toContainText("2 checked; 1 violations");
  await expect(failDetails).toContainText("bob");
  await failDetails.locator(".dialog-actions .button").click();
  await page.locator("#assertion-evaluate").click();
  await page.locator("#assertion-source").fill('each group attribute "gidNumber" >= 1000;');
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows")).toContainText('each group attribute "gidnumber" >= 1000');
  await expect(page.locator("#assertion-run-status")).toContainText("PASS");
  await page.locator("#assertion-evaluate").click();
  await page.getByRole("tab", { name: "Attributes", exact: true }).focus();
  await page.keyboard.press("ArrowLeft");
  await expect(page.getByRole("tab", { name: "Groups", exact: true })).toHaveAttribute("aria-selected", "true");
  await expect(page.locator("#assertion-attributes-panel")).toBeHidden();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("assertion catalogs insert named sources and expression results show their text", async ({ page, appURL, api }) => {
  api.assertionSource.sources = [
    { name: "staff", people: 2, groups: [{ name: "Operators", members: 2 }], attributes: [{ name: "mail", people: 2, groups: 0 }], observed_at: "2026-10-05T12:00:00Z" },
    { name: "hr", people: 2, groups: [{ name: "Employees", members: 2 }], attributes: [{ name: "salary", people: 2, groups: 0 }], observed_at: "2026-10-05T12:00:00Z" },
  ];
  api.assertionSource.default_source = "staff";
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    await route.fulfill({ json: { revision: 0, draft: true, status: "pass", finished_at: "2026-10-05T12:00:00Z", results: [{ rule: { line: 1, kind: "expression", expression: 'source("staff").group("Operators").members == source("hr").group("Employees").members' }, status: "pass", checked: 1, violations: 0, subjects: [], message: "1 checked; 0 violations" }] } });
  });
  await openAssertionRecord(page, appURL);
  await page.locator("#assertion-read-source").click();
  await expect(page.getByLabel("Assertion trusted source")).toHaveValue("staff");
  await page.getByLabel("Assertion trusted source").selectOption("hr");
  await expect(page.locator("#assertion-group-rows")).toContainText("Employees");
  await page.getByRole("button", { name: "Insert a cardinality assertion for Employees", exact: true }).click();
  await expect(page.locator("#assertion-source")).toHaveValue('group "Employees" from "hr" members >= 2;\n');
  await page.locator("#assertion-source").fill('assert source("staff").group("Operators").members == source("hr").group("Employees").members;');
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows")).toContainText('assert source("staff").group("Operators").members == source("hr").group("Employees").members');
  await page.locator(".assertion-result-marker").click();
  await expect(page.getByRole("dialog", { name: "Assertion pass" })).toContainText('source("hr").group("Employees").members');
});

test("assertion source failures are errors and never successful checks", async ({ page, appURL, api }) => {
  api.assertions.settings.source = 'group "Operators" members >= 2;';
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    api.assertions.last_run = { revision: 0, draft: false, status: "error", error: "Trusted LDAP read failed; no assertions were evaluated", results: [], finished_at: "2026-10-02T12:00:00Z" };
    await route.fulfill({ json: api.assertions.last_run });
  });
  await openAssertionRecord(page, appURL);
  await expect(page.locator("#assertion-evaluate")).toBeEnabled();
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-run-status")).toContainText("ERROR");
  await expect(page.locator("#assertion-run-error")).toContainText("no assertions were evaluated");
  const errorMarker = page.locator('#assertion-result-gutter [data-line="1"] .assertion-result-marker');
  await expect(errorMarker).toHaveAttribute("data-state", "error");
  await errorMarker.click();
  const errorDetails = page.getByRole("dialog", { name: "Assertion error" });
  await expect(errorDetails).toContainText("Trusted LDAP read failed");
  await errorDetails.locator(".dialog-actions .button").click();
  await page.locator("#assertion-evaluate").click();
  await expect(page.locator("#assertion-result-rows tr")).toHaveCount(0);
  api.assertions.configured = false;
  await page.locator("#assertion-reload").click();
  await expect(page.locator("#assertion-evaluate")).toBeDisabled();
  await expect(page.locator("#assertion-enabled")).toBeDisabled();
});

test("assertion polling preserves dirty drafts and blocks stale revisions", async ({ page, appURL, api }) => {
  await page.clock.install();
  await openAssertionRecord(page, appURL);
  const editor = page.locator("#assertion-source");
  await expect(editor).toBeEnabled();
  await editor.fill('group "Operators" members > 1;');
  api.assertions.settings = { revision: 1, source: 'group "Operators" members >= 2;', enabled: false, interval_seconds: 60 };
  await page.clock.runFor(5100);
  await expect(editor).toHaveValue('group "Operators" members > 1;');
  await expect(page.locator("#assertion-revision")).toContainText("Reload required");
  await expect(page.locator("#assertion-save")).toBeDisabled();
  await expect(page.locator("#assertion-evaluate")).toBeDisabled();
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator("#assertion-reload").click();
  await expect(editor).toHaveValue('group "Operators" members >= 2;');
  await expect(page.locator("#assertion-revision")).toHaveText("Alpha Labs / r1");
});

test("organization switch reloads that organization's assertions and protects dirty drafts", async ({ page, appURL, api }) => {
  await page.clock.install();
  await openAssertionRecord(page, appURL);
  const editor = page.locator("#assertion-source");
  await editor.fill('group "Local draft" members > 0;');
  api.assertions.organization_id = "beta";
  api.assertions.organization_name = "Beta Labs";
  api.assertions.base_dn = "dc=beta,dc=test";
  api.assertions.settings = { revision: 3, source: 'group "Beta Operators" members >= 2;', enabled: true, interval_seconds: 120 };
  api.assertions.last_run = null;
  await page.clock.runFor(5100);
  await expect(editor).toHaveValue('group "Local draft" members > 0;');
  await expect(page.locator("#assertion-revision")).toHaveText("Beta Labs / r0 / Unsaved / Reload required");
  await expect(page.locator("#assertion-save")).toBeDisabled();
  await expect(page.locator("#assertion-evaluate")).toBeDisabled();
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator("#assertion-reload").click();
  await expect(editor).toHaveValue('group "Beta Operators" members >= 2;');
  await expect(page.locator("#assertion-revision")).toHaveText("Beta Labs / r3");
  await expect(page.locator("#assertion-enabled")).toBeChecked();
});

test("new assertion records support copy, paste, duplicate, and protect built-in settings", async ({ page, appURL, api }) => {
  let nextRecordID = 1;
  const addRecord = (name, content) => {
    const record = {
      id: `custom-assertions-${nextRecordID++}`, category_id: "test", name,
      kind: "assertions", content_type: "text/vnd.zpr.assertions",
      metadata: { language: "assertions" }, current_revision: 1, content,
      content_hash: "custom-fixture", archived: false,
    };
    api.policy.records.push(record);
    api.handlers.set(`/api/policy/records/${record.id}`, async (route) => {
      if (route.request().method() === "DELETE") {
        record.archived = true;
        await route.fulfill({ json: { id: record.id, archived: true } });
        return;
      }
      await route.fulfill({ json: record });
    });
    api.handlers.set(`/api/policy/records/${record.id}/revisions`, async (route) => route.fulfill({ json: [] }));
    api.handlers.set(`/api/policy/records/${record.id}/restore`, async (route) => {
      record.archived = false;
      await route.fulfill({ json: { id: record.id, archived: false } });
    });
    api.handlers.set(`/api/policy/records/${record.id}/duplicate`, async (route) => {
      const request = route.request().postDataJSON();
      expect(request.category_id).toBe("test");
      await route.fulfill({ status: 201, json: addRecord(request.name, record.content) });
    });
    return record;
  };
  api.handlers.set("/api/policy/records", async (route) => {
    const request = route.request().postDataJSON();
    expect(request.kind).toBe("assertions");
    expect(request.content_type).toBe("text/vnd.zpr.assertions");
    expect(request.content).toBe('group "Operators" members >= 2;');
    await route.fulfill({ status: 201, json: addRecord(request.name, request.content) });
  });

  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await openPolicyPicker(page);
  await expect(page.locator("#new-assertion-record")).toBeEnabled();
  await expect(page.locator('[data-record-id="test-policy"] .policy-kind-icon')).toHaveAttribute("data-icon", "traffic-light");
  await expect(page.locator('[data-record-id="test-assertions"] .policy-kind-icon')).toHaveAttribute("data-icon", "database-check");
  const policyMarkerColor = await page.locator('[data-record-id="test-policy"] .policy-kind-icon').evaluate((item) => getComputedStyle(item).color);
  const assertionMarkerColor = await page.locator('[data-record-id="test-assertions"] .policy-kind-icon').evaluate((item) => getComputedStyle(item).color);
  expect(assertionMarkerColor).not.toBe(policyMarkerColor);
  await page.locator('[data-category-id="test"]').click({ button: "right" });
  await page.locator("#new-assertion-record").click();
  await page.locator("#policy-draft-name").fill("Operator assertions");
  await page.locator("#assertion-source").fill('group "Operators" members >= 2;');
  await page.locator("#assertion-save").click();
  await expect(page.locator('[data-record-id="custom-assertions-1"]')).toBeVisible();
  await expect(page.locator("#policy-revision-label")).toContainText("Version 1");

  await page.locator('[data-record-id="custom-assertions-1"]').click({ button: "right" });
  await page.locator("#policy-copy").click();
  await page.locator('[data-category-id="test"]').click({ button: "right" });
  await page.locator("#policy-paste").click();
  await expect(page.locator('[data-record-id="custom-assertions-2"]')).toBeVisible();
  await expect(page.locator('[data-record-id="custom-assertions-2"] strong')).toHaveText("Operator assertions copy");

  await page.locator('[data-record-id="custom-assertions-2"]').click({ button: "right" });
  await page.locator("#policy-duplicate").click();
  await expect(page.locator('[data-record-id="custom-assertions-3"]')).toBeVisible();
  await expect(page.locator('[data-record-id="custom-assertions-3"] strong')).toHaveText("Operator assertions copy 2");

  page.once("dialog", (dialog) => dialog.accept());
  await page.locator('[data-record-id="custom-assertions-3"]').click({ button: "right" });
  await page.locator("#policy-delete").click();
  await expect(page.locator('[data-record-id="custom-assertions-3"]')).toHaveCount(0);
  await page.locator('[data-category-id="test"]').click({ button: "right" });
  await page.locator("#policy-show-archived").click();
  await expect(page.locator('[data-record-id="custom-assertions-3"]')).toBeVisible();
  await page.locator('[data-record-id="custom-assertions-3"]').click();
  await page.locator('[data-record-id="custom-assertions-3"]').click({ button: "right" });
  await page.locator("#policy-restore").click();
  await expect(page.locator("#policy-file-status")).toContainText("Restored Operator assertions copy 2");
  expect(api.policy.records.find((record) => record.id === "custom-assertions-3").archived).toBe(false);

  await page.locator('[data-record-id="test-assertions"]').click();
  await expect(page.locator("#policy-copy")).toBeDisabled();
  await expect(page.locator("#policy-duplicate")).toBeDisabled();
  await expect(page.locator("#policy-delete")).toBeDisabled();
});
}
import { test as base, expect } from "@playwright/test";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const assets = fileURLToPath(new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url));
const csp = "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:";

function logFixture(names, count = 80) {
  return {
    organization_id: "browser-test",
    updated_at: "2026-10-02T12:00:00Z",
    machines: ["machine-first", "machine-second"].map((id) => ({
      machine: { id, model: "Test machine" },
      state: "running",
      sources: names.map((name) => ({
        name,
        lines: Array.from({ length: count }, (entry, index) => `${name} entry ${index}`),
      })),
    })),
  };
}

const test = base.extend({
  appURL: [async ({}, use) => {
    const server = createServer(async (request, response) => {
      const pathname = new URL(request.url, "http://localhost").pathname;
      if (pathname === "/__tests/ldap.html") {
        response.writeHead(200, { "Content-Type": "text/html", "Content-Security-Policy": csp });
        response.end('<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/ldap-org-graph.css"><script src="/ldap-org-graph.js" defer></script></head><body><ldap-org-graph></ldap-org-graph></body></html>');
        return;
      }
      const file = resolve(assets, pathname === "/" ? "index.html" : pathname.slice(1));
      if (!file.startsWith(assets.endsWith(sep) ? assets : assets + sep)) {
        response.writeHead(403).end();
        return;
      }
      try {
        const content = await readFile(file);
        const extension = file.split(".").at(-1);
        response.writeHead(200, {
          "Content-Type": ({ html: "text/html", js: "application/javascript", css: "text/css", woff2: "font/woff2" })[extension] || "application/octet-stream",
          "Content-Security-Policy": csp,
          "Cache-Control": "no-store",
        });
        response.end(content);
      } catch {
        response.writeHead(404).end();
      }
    });
    await new Promise((ready) => server.listen(0, "127.0.0.1", ready));
    try {
      await use(`http://127.0.0.1:${server.address().port}`);
    } finally {
      await new Promise((closed) => server.close(closed));
    }
  }, { scope: "worker" }],
  api: async ({ page }, use) => {
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.addInitScript(() => {
      window.testCSPViolations = [];
      document.addEventListener("securitypolicyviolation", (event) => window.testCSPViolations.push(event.violatedDirective));
    });
    const data = {
      adapterLogs: logFixture(["Controller", "Control adapter", "finance-client adapter"]),
      workloadLogs: logFixture(["finance-client events", "echo-service events"]),
      snapshot: {
        api_status: "connected", errors: [], stats: {}, actors: [], services: [],
        network: [], trusted_sources: [], recent_visas: [], recent_denies: [], visa_count: 0,
      },
      dnsRecords: { zone: "svc.zpr.", records: [] },
      visas: [{ id: 41, source_addr: "fd00::1", dest_addr: "fd00::2", proto: "TCP", expires: Math.floor(Date.now() / 1000) + 600 }],
      policy: {
        configured: true, organization_id: "alpha", organization_name: "Alpha Labs", compiler_ready: true, tester_ready: true, assistant_ready: false,
        categories: [{ id: "legacy-assertions", name: "Assertions", path: "Assertions" }, { id: "test", name: "Policies", path: "Alpha/Policies" }],
        records: [
          { id: "test-policy", category_id: "test", name: "Test policy", kind: "policy", current_revision: 1, content: "define Employee as user.\n", content_hash: "fixture" },
          { id: "test-assertions", category_id: "legacy-assertions", name: "Organization assertions", kind: "assertions", content_type: "application/vnd.zpr.assertions+json", current_revision: 1, content: '{"source":"","enabled":false,"interval_seconds":60}', content_hash: "fixture" },
        ],
        attributes: [{ attribute: "user.department", source: "LDAP" }, { attribute: "user.title", source: "LDAP" }, { attribute: "device.secure", source: "LDAP" }],
      },
      organizations: {
        active_id: "alpha",
        activation: { state: "idle" },
        organizations: [["alpha", "Alpha Labs"], ["beta", "Beta Labs"], ["gamma", "Gamma Labs"]].map(([id, name]) => ({
          id, name, description: "Test organization", policies: [], services: [],
          runtime: id === "beta"
            ? { driver: "docker-multinode", topology: "multi-node", nodes: [{ id: "north-hub", location: "North Hub" }, { id: "regional-yard", location: "Regional Yard" }] }
            : { driver: "linux-one-node", topology: "single-node", nodes: [{ id: `${id}-node`, location: "Test site" }] },
          directory: { base_dn: `dc=${id},dc=test`, seed_mode: "fixture", departments: [], people: [], groups: [] },
        })),
      },
      assertions: {
        scope: "organization", organization_id: "alpha", organization_name: "Alpha Labs", configured: true, source_kind: "ldap", base_dn: "dc=alpha,dc=test",
        settings: { revision: 0, source: "", enabled: false, interval_seconds: 60 },
        running: false, last_run: null, source_summary: null,
      },
      assertionSource: {
        source_name: "Trusted LDAP", organization_id: "alpha", organization_name: "Alpha Labs", base_dn: "dc=alpha,dc=test",
        observed_at: "2026-10-02T12:00:00Z", people: 2,
        groups: [{ name: "Operators", members: 2 }],
        attributes: [{ name: "description", people: 0, groups: 1 }, { name: "mail", people: 2, groups: 0 }, { name: "title", people: 2, groups: 0 }],
        directory: {
          people: ["alice", "bob"], groups: { Operators: ["alice", "bob"] }, attributes: ["description", "mail", "title"],
          person_attributes: { alice: { mail: ["alice@example.test"], title: ["Engineer"] }, bob: { mail: ["bob@example.test"], title: ["Reviewer"] } },
          group_attributes: { Operators: { description: ["Directory operators"] } },
        },
      },
      counts: new Map(),
      handlers: new Map(),
      statuses: new Map(),
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      data.counts.set(path, (data.counts.get(path) || 0) + 1);
      if (data.handlers.has(path)) {
        await data.handlers.get(path)(route);
        return;
      }
      const bodies = {
        "/api/adapter-logs": {
          updated_at: data.adapterLogs.updated_at,
          adapters: (data.adapterLogs.machines || []).map((entry) => ({
            id: entry.machine.id, name: entry.machine.id, state: entry.state,
            sources: entry.sources.map((source) => ({ ...source, kind: source.name === "Controller" ? "controller" : "adapter" })),
          })),
        },
        "/api/simulator/machine-logs": data.workloadLogs,
        "/api/snapshot": data.snapshot,
        "/api/dns/records": data.dnsRecords,
        "/api/actors/adapter/visas": data.visas,
        "/api/policy": data.policy,
        "/api/policy/records/test-policy": data.policy.records[0],
        "/api/policy/records/test-policy/revisions": [],
        "/api/policy/records/test-assertions": data.policy.records[1],
        "/api/policy/records/test-assertions/revisions": [],
        "/api/simulator/organizations": data.organizations,
        "/api/assertions": data.assertions,
        "/api/assertions/source": data.assertionSource,
        "/api/simulator/trusted-source": { ...data.assertionSource, source_name: "Organization LDAP" },
      };
      if (!Object.hasOwn(bodies, path)) {
        await route.fulfill({ status: 503, json: { error: "Not configured in this test" } });
        return;
      }
      await route.fulfill({ status: data.statuses.get(path) || 200, json: bodies[path] });
    });
    await use(data);
    expect(errors, "Unexpected browser JavaScript errors").toEqual([]);
    expect(await page.evaluate(() => window.testCSPViolations || []), "Unexpected CSP violations").toEqual([]);
  },
});

registerAssertionBrowserTests();

test("Control Room keeps Adapter Logs internal and Log Manager under Tools", async ({ page, appURL }) => {
  await page.goto(appURL + "/#map");
  const adapterLogs = page.getByRole("link", { name: "Adapter Logs", exact: true });
  await expect(adapterLogs).toHaveAttribute("href", "#adapter-logs");
  await expect(adapterLogs).not.toHaveAttribute("target");
  await adapterLogs.click();
  await expect(page.locator("#page-adapter-logs")).toBeVisible();
  const manager = page.locator(".sidebar-external-tools a");
  await expect(manager).toContainText("Log Manager");
  await expect(manager).toHaveAttribute("href", "http://127.0.0.1:8798/");
  await expect(manager).toHaveAttribute("target", "zpr-log-manager");
});

test("trusted source manager links appear only for configured providers", async ({ page, appURL, api }) => {
  api.snapshot.trusted_sources = [
    { name: "identity", provider: "rest/1", actor_cn: "auth", health: "working", editor_url: "https://auth.example.test/admin" },
    { name: "directory", provider: "rest/1", actor_cn: "ldap", health: "working" },
  ];
  await page.goto(appURL + "/#sources");
  await expect(page.locator(".source-editor-link")).toHaveCount(1);
  await expect(page.locator(".source-editor-link")).toHaveAttribute("href", "https://auth.example.test/admin");
  await expect(page.locator(".source-editor-link")).toContainText("Manage");
});

test("ZPR Config validates and saves versioned drafts without applying runtime configuration", async ({ page, appURL, api }) => {
  const content = '[visa_service]\ndock_node = "node"\n';
  let savedRecord;
  api.handlers.set("/api/policy/config/check", async (route) => {
    expect(route.request().postDataJSON().source).toBe(content);
    await route.fulfill({ json: { valid: true, diagnostics: "TOML syntax valid; runtime configuration is unchanged." } });
  });
  api.handlers.set("/api/policy/categories", async (route) => {
    const category = { id: "zpr-config", path: "ZPR Config", name: "ZPR Config" };
    api.policy.categories.push(category);
    await route.fulfill({ status: 201, json: category });
  });
  api.handlers.set("/api/policy/records", async (route) => {
    const request = route.request().postDataJSON();
    expect(request.kind).toBe("configuration");
    expect(request.content_type).toBe("text/vnd.zpr.zplc");
    expect(request.content).toBe(content);
    savedRecord = { id: "config-1", category_id: "zpr-config", name: request.name, kind: request.kind, content_type: request.content_type, current_revision: 1, content: request.content };
    api.policy.records.push(savedRecord);
    await route.fulfill({ status: 201, json: savedRecord });
  });
  api.handlers.set("/api/policy/records/config-1", async (route) => route.fulfill({ json: savedRecord }));
  api.handlers.set("/api/policy/records/config-1/revisions", async (route) => route.fulfill({ json: [{ number: 1, summary: "Initial configuration draft" }] }));
  await page.goto(appURL + "/#map");
  await page.getByRole("link", { name: "ZPR Config", exact: true }).click();
  await expect(page.locator("#page-zpr-config")).toBeVisible();
  await page.locator("#zpr-config-name").fill("Local node configuration");
  await page.getByLabel("ZPLC configuration source").fill(content);
  await page.getByRole("button", { name: "Validate syntax", exact: true }).click();
  await expect(page.locator("#zpr-config-status")).toContainText("runtime configuration is unchanged");
  await page.getByRole("button", { name: "Save draft", exact: true }).click();
  await expect(page.locator("#zpr-config-status")).toContainText("runtime unchanged");
  await expect(page.locator("#zpr-config-history option")).toHaveCount(2);
  expect(api.counts.get("/api/policy/config/apply") || 0).toBe(0);
});

test("Diagnostics shows source identity, current metrics, searchable bounded logs, and stale/unavailable states without Simulator", async ({ page, appURL, api }) => {
  const simulationRequests = [];
  page.on("request", (request) => { if (request.url().includes("/api/simulator/")) simulationRequests.push(request.url()); });
  api.handlers.set("/api/diagnostics", async (route) => route.fulfill({ json: {
    generated_at: "2026-10-05T12:00:00Z", state: "partial", sources: [
      { id: "node:node-a", name: "node-a", kind: "ZPR node", identity: "node-a", address: "fd00::1", state: "available", last_updated: "2026-10-05T11:59:00Z", metrics: [{ name: "packets_forwarded", value: "12", unit: "1" }], logs: [{ timestamp: "2026-10-05T11:59:00Z", severity: "INFO", body: "node forwarding ready" }] },
      { id: "trusted:ldap", name: "ldap", kind: "Trusted service · rest/1", identity: "ldap-service", state: "stale", last_updated: "2026-10-05T10:00:00Z", metrics: [], logs: [{ timestamp: "2026-10-05T10:00:00Z", body: "directory lookup ready" }] },
      { id: "service:auth", name: "AuthService", kind: "Required service · Auth", identity: "auth", state: "unavailable", error: "No OpenTelemetry signals received", metrics: [], logs: [] },
    ],
  } }));
  await page.goto(appURL + "/#diagnostics");
  await expect(page.locator("#page-diagnostics")).toBeVisible();
  await expect(page.locator(".diagnostics-source")).toHaveCount(3);
  await expect(page.locator('.diagnostics-source[data-state="available"]')).toContainText("12");
  await expect(page.locator('.diagnostics-source[data-state="stale"]')).toContainText("stale");
  await expect(page.locator('.diagnostics-source[data-state="unavailable"]')).toContainText("No OpenTelemetry signals received");
  await page.getByRole("searchbox", { name: "Filter logs and sources" }).fill("forwarding ready");
  await expect(page.locator(".diagnostics-source")).toHaveCount(1);
  await expect(page.locator(".diagnostics-source")).toContainText("node-a");
  expect(simulationRequests).toEqual([]);
});

test("Simulator Agents uses device terminology without a machine-count label", async ({ page, appURL }) => {
  await page.goto(appURL + "/agents.html");
  await expect(page.getByRole("heading", { name: "Devices", exact: true })).toBeVisible();
  await expect(page.locator("#machine-count")).toHaveCount(0);
  await expect(page.locator("#machine-type-filter option[value=all]")).toHaveText("All devices");
});

test("Simulator Activity uses sortable tables and keeps Refresh beside stream state", async ({ page, appURL, api }) => {
  api.handlers.set("/api/simulator/activity", async (route) => route.fulfill({ json: {
    generated_at: "2026-10-05T12:00:00Z",
    stats: { visa_requests: 4, visa_requests_approved: 2, visa_requests_denied: 2 },
    visas: [
      { id: 1, source_addr: "fd00::2", dest_addr: "fd00::8", proto: "TCP", dest_port: 8080, policy_id: "Echo", expires: 1200 },
      { id: 2, source_addr: "fd00::1", dest_addr: "fd00::8", proto: "TCP", dest_port: 8080, policy_id: "Echo", expires: 1800 },
    ],
    denies: [
      { source_addr: "fd00::2", dest_addr: "fd00::9", protocol: 6, dest_port: 443, code: "Denied", count: 3 },
      { source_addr: "fd00::1", dest_addr: "fd00::9", protocol: 6, dest_port: 443, code: "NoMatch", count: 8 },
    ],
  } }));
  await page.goto(appURL + "/activity.html");
  await expect(page.locator("#activity-state")).toHaveText("LIVE");
  await expect(page.locator("#refresh").locator("xpath=..")).toHaveClass(/activity-stream-actions/);
  await expect(page.getByText("DECISIONS", { exact: true })).toHaveCount(0);
  await expect(page.getByText("BLOCKED FLOWS", { exact: true })).toHaveCount(0);

  const visas = page.locator('table[data-sort-page="activity-visas"]');
  const flow = visas.locator('th[data-sort-key="flow"]');
  await flow.click();
  await expect(flow).toHaveAttribute("aria-sort", "ascending");
  await expect(visas.locator("tbody tr").first()).toContainText("fd00::1");
  await flow.click();
  await expect(flow).toHaveAttribute("aria-sort", "descending");
  await expect(visas.locator("tbody tr").first()).toContainText("fd00::2");
  await expect(page.locator('table[data-sort-page="activity-denies"] tbody tr')).toHaveCount(2);
});

test("Simulator Scenarios groups unfiled entries and clears only terminal run history", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  const scenario = {
    id: "sample-flow", name: "Sample flow", folder: "", description: "A browser fixture.",
    organization_id: "alpha", current_revision: 1, published_revision: 1, steps: [], cleanup: [],
  };
  let run = {
    scenario_id: scenario.id, scenario_name: scenario.name, state: "completed", current_step: 1, total_steps: 1,
    steps: [], scenario: { steps: [], cleanup: [] },
  };
  api.handlers.set("/api/simulator/scenarios", async (route) => route.fulfill({ json: {
    active_organization_id: "alpha", organization: { name: "Alpha Labs" }, scenarios: [scenario], run, max_machines: 10,
  } }));
  api.handlers.set("/api/simulator/scenarios/clear", async (route) => {
    run = { state: "idle", steps: [] };
    await route.fulfill({ json: run });
  });
  await page.goto(appURL + "/scenarios.html");
  await expect(page.locator("#scenario-new")).toHaveText("New Scenario...");
  await expect(page.locator(".scenario-list-pane #scenario-new")).toBeVisible();
  await expect(page.locator("#scenario-count")).toHaveCount(0);
  await expect(page.getByText("Available scenarios", { exact: true })).toHaveCount(0);
  await expect(page.locator(".scenario-folder")).toHaveCount(0);
  await expect(page.locator("#scenario-list")).toContainText("Sample flow");
  await expect(page.getByText("EXECUTION PLAN", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Run", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Run Published", exact: true })).toHaveCount(0);
  const scenarioCard = page.locator(`#scenario-list [data-run-scenario="${scenario.id}"]`).locator("xpath=ancestor::article");
  await expect(scenarioCard.locator(".scenario-version")).toHaveText("Version 1");
  await expect(scenarioCard.locator("button")).toHaveText(["Run", "Edit", "Delete"]);
  await expect(page.locator("#scenario-run-title")).toHaveText("Sample flow");
  await expect(page.locator("#scenario-run-state .scenario-state-label")).toHaveText("completed");
  await expect(page.locator("#scenario-cancel")).toBeHidden();
  await expect(page.locator("#scenario-clear")).toBeVisible();

  await page.evaluate(() => renderScenarioRun({ scenario_id: "sample-flow", scenario_name: "Sample flow", state: "running", steps: [], scenario: { steps: [], cleanup: [] } }));
  await expect(page.locator("#scenario-cancel")).toBeVisible();
  await expect(page.locator("#scenario-clear")).toBeHidden();
  await page.evaluate(() => renderScenarioRun({ scenario_id: "sample-flow", scenario_name: "Sample flow", state: "running", current_step: 2, total_steps: 5, steps: [], scenario: { steps: [], cleanup: [] } }));
  await expect(page.locator("#scenario-run-state")).toContainText("2 / 5");
  const animationCount = await page.locator("#scenario-run-state").evaluate((element) => element.getAnimations().length);
  expect(animationCount).toBeGreaterThan(0);
  await page.evaluate(() => renderScenarioRun({ scenario_id: "sample-flow", scenario_name: "Sample flow", state: "running", current_step: 2, total_steps: 5, steps: [], scenario: { steps: [], cleanup: [] } }));
  await expect.poll(() => page.locator("#scenario-run-state").evaluate((element) => element.getAnimations().length)).toBe(animationCount);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const reducedMotionAnimationCount = await page.locator("#scenario-run-state").evaluate((element) => element.getAnimations().length);
  await page.evaluate(() => renderScenarioRun({ scenario_id: "sample-flow", scenario_name: "Sample flow", state: "running", current_step: 3, total_steps: 5, steps: [], scenario: { steps: [], cleanup: [] } }));
  await expect(page.locator("#scenario-run-state")).toContainText("3 / 5");
  await expect.poll(() => page.locator("#scenario-run-state").evaluate((element) => element.getAnimations().length)).toBe(reducedMotionAnimationCount);
  const statusActionGeometry = await page.evaluate(() => {
    const state = document.querySelector("#scenario-run-state").getBoundingClientRect();
    const cancel = document.querySelector("#scenario-cancel").getBoundingClientRect();
    return { adjacent: cancel.left >= state.right && cancel.left - state.right < 20, sameRow: Math.abs(cancel.top + cancel.height / 2 - state.top - state.height / 2) < 5 };
  });
  expect(statusActionGeometry.adjacent).toBeTruthy();
  expect(statusActionGeometry.sameRow).toBeTruthy();
  await page.evaluate(() => renderScenarioRun({ scenario_id: "sample-flow", scenario_name: "Sample flow", state: "cancelled", current_step: 2, total_steps: 5, error: "Context canceled", steps: [{ number: 1, status: "failed", error: "context canceled" }], scenario: { steps: [{ action: "wait", machine: "node" }], cleanup: [] } }));
  await expect(page.locator("#scenario-run-state")).toContainText("2 / 5");
  await expect(page.locator("#scenario-run-summary")).toBeHidden();
  await expect(page.locator(".scenario-run-pane")).not.toContainText("Context canceled");
  await expect(page.locator("#scenario-cancel")).toBeHidden();
  await expect(page.locator("#scenario-clear")).toBeVisible();
  await page.evaluate(() => renderScenarioRun({ scenario_id: "sample-flow", scenario_name: "Sample flow", state: "completed", steps: [], scenario: { steps: [], cleanup: [] } }));
  await expect(page.locator("#scenario-cancel")).toBeHidden();
  await expect(page.locator("#scenario-clear")).toBeVisible();
  await page.locator("#scenario-clear").click();
  await expect(page.locator("#scenario-run-title")).toHaveText("Scenario");
  await expect(page.locator("#scenario-clear")).toBeHidden();
  expect(api.counts.get("/api/simulator/scenarios/clear")).toBe(1);
  await page.locator("#scenario-new").click();
  const editorAction = page.locator('#scenario-editor-steps [data-step-key="action"]');
  await expect(editorAction).toHaveValue("delay");
  await editorAction.selectOption("resolve_dns");
  await expect(editorAction).toHaveValue("resolve_dns");
});

test("assertion picker lines omit counts and revisions and distinguish record kinds by color", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  const tree = page.locator("#policy-category-tree");
  await expect(tree.locator(".category-tree-item small, .policy-record-item small")).toHaveCount(0);
  const policy = tree.locator('[data-record-id="test-policy"]');
  const assertions = tree.locator('[data-record-id="test-assertions"]');
  await expect(policy).toHaveText("Test policy");
  await expect(assertions).toHaveText("Organization assertions");
  await expect(policy.locator(".policy-kind-icon")).toHaveAttribute("data-icon", "traffic-light");
  await expect(assertions.locator(".policy-kind-icon")).toHaveAttribute("data-icon", "database-check");
  const policyColor = await policy.locator(".policy-kind-icon").evaluate((item) => getComputedStyle(item).color);
  const assertionColor = await assertions.locator(".policy-kind-icon").evaluate((item) => getComputedStyle(item).color);
  expect(assertionColor).not.toBe(policyColor);
  await assertions.click();
  await expect(page.getByRole("tab", { name: "Attributes", exact: true })).toBeVisible();
});

test("Security Review stacks baseline and current inventory with compact checkboxes", async ({ page, appURL, api }) => {
  api.snapshot.actors = [{ cn: "adapter-a", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.services = [{ service_name: "API", actor_cn: "adapter-a", service_kind: "Regular", service_endpoints: "TCP/443" }];
  await page.goto(appURL + "/#security-review");
  await expect(page.locator("#security-review-baseline-actors")).toContainText("adapter-a");
  api.snapshot.actors.push({ cn: "adapter-b", node: false, zpr_addr: "fd00::2" });
  await page.locator("#refresh-now").click();
  await expect(page.locator("#security-review-current-actors")).toContainText("adapter-b");
  await expect(page.locator("#security-review-baseline-actors")).not.toContainText("adapter-b");
  const baseline = await page.locator('[data-inventory-row="baseline"]').boundingBox();
  const current = await page.locator('[data-inventory-row="current"]').boundingBox();
  expect(current.y).toBeGreaterThanOrEqual(baseline.y + baseline.height - 1);
  const checkbox = await page.locator("#security-review-show-dismissed").boundingBox();
  expect(checkbox.width).toBeLessThanOrEqual(18);
  expect(checkbox.height).toBeLessThanOrEqual(18);
  await expect(page.locator("#security-review-current-time")).not.toContainText("Observed");
  await expect(page.locator('#security-review-current-actors [data-change="added"]')).toContainText("adapter-b");
  api.snapshot.actors = api.snapshot.actors.filter((actor) => actor.cn !== "adapter-a");
  await page.locator("#refresh-now").click();
  await expect(page.locator('#security-review-current-actors [data-change="removed"]')).toContainText("adapter-a");
  await expect(page.locator("#security-review-baseline-actors")).toContainText("adapter-a");
});

test("Security Review reports denial and log evidence and compares new inventory to its local baseline", async ({ page, appURL, api }) => {
  api.snapshot.actors = [{ cn: "adapter-a", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.services = [{ service_name: "base-service", actor_cn: "adapter-a", service_kind: "Regular" }];
  api.snapshot.recent_denies = [{ source_addr: "fd00::bad", dest_addr: "fd00::1", protocol: 6, dest_port: 443, count: 5, last_deny_ms: Date.now(), deny_code: "Denied" }];
  api.snapshot.actors.push({ cn: "node-a", node: true, zpr_addr: "fd00::3", node_details: { in_sync: false, last_contact: Math.floor(Date.now() / 1000) - 600, pending_install: 1 } });
  api.snapshot.actors.push({ cn: "node-b", node: true, zpr_addr: "fd00::4", node_details: { in_sync: true, last_contact: Math.floor(Date.now() / 1000) - 600 } });
  api.snapshot.trusted_sources = [{ name: "directory", health: "failed", health_note: "The most recent attribute lookup failed.", last_lookup_ms: Date.now() }];
  api.adapterLogs = { machines: [{ machine: { id: "machine-01" }, sources: [{ name: "Controller", lines: ["authentication failed for unknown identity"] }] }] };
  const mutations = [];
  page.on("request", (request) => { if (request.method() !== "GET") mutations.push(request.method()); });
  await page.goto(appURL + "/#security-review");
  await expect(page.getByRole("heading", { name: "Security review", exact: true })).toHaveCount(0);
  await expect(page.getByText("INVESTIGATION LEADS · READ ONLY", { exact: true })).toHaveCount(0);
  await expect(page.locator(".security-review-baseline-label")).toHaveText("Baseline");
  await expect(page.locator("#security-review-baseline-time")).not.toContainText("Saved");
  await expect(page.getByText("Inventory", { exact: true })).toHaveCount(0);
  await expect(page.locator("#security-review-status")).toBeEmpty();
  await expect(page.locator("#security-review-status")).not.toContainText("Scanning");
  await expect(page.locator(".security-review-caveat")).toHaveCount(0);
  const resetBox = await page.locator("#security-review-reset").boundingBox();
  const summaryBox = await page.locator("#security-review-baseline-details summary").boundingBox();
  expect(Math.abs((resetBox.y + resetBox.height / 2) - (summaryBox.y + summaryBox.height / 2))).toBeLessThan(3);
  const baselineTimeBox = await page.locator("#security-review-baseline-time").boundingBox();
  const currentTimeBox = await page.locator("#security-review-current-time").boundingBox();
  expect(Math.abs(baselineTimeBox.x - currentTimeBox.x)).toBeLessThan(1);
  await expect(page.locator(".security-review-baseline-help")).toHaveCount(0);
  await expect(page.locator("#security-review-status")).not.toContainText("Scan complete");
  await expect(page.locator(".security-review-select-visible")).toContainText("Select visible");
  const findings = page.locator("#security-review-findings");
  await expect(findings).toContainText("Repeated policy denials");
  await expect(findings).toContainText("5 denied requests");
  await expect(findings).toContainText("Security-related log message");
  await expect(findings).toContainText("authentication failed for unknown identity");
  await expect(findings).toContainText("Trusted-source lookup failed");
  await expect(findings).toContainText("The most recent attribute lookup failed.");
  await expect(findings).toContainText("Node out of sync");
  await expect(findings).toContainText("1 pending installs");
  await expect(findings).toContainText("Node contact stale");
  await expect(page.locator("#security-review-baseline")).toContainText("3 actors · 1 services");
  const baselineDetails = page.locator("#security-review-baseline-details");
  const currentDetails = page.locator("#security-review-current-details");
  await expect(baselineDetails).not.toHaveAttribute("open", "");
  await expect(currentDetails).not.toHaveAttribute("open", "");
  await expect(baselineDetails.locator(".security-review-inventory-details")).toBeHidden();
  await expect(currentDetails.locator(".security-review-inventory-details")).toBeHidden();
  await baselineDetails.locator("summary").click();
  await expect(baselineDetails.locator("#security-review-baseline-actors")).toBeVisible();
  await expect(baselineDetails.locator("#security-review-baseline-actors")).toContainText("adapter-a");
  await baselineDetails.locator("summary").click();
  await currentDetails.locator("summary").click();
  await expect(currentDetails.locator("#security-review-current-actors")).toContainText("adapter-a");
  await currentDetails.locator("summary").click();
  await expect(findings).not.toContainText("Actor first observed since baseline");
  await expect(findings).not.toContainText("Service first observed since baseline");

  api.snapshot.actors.push({ cn: "new-actor", node: false, zpr_addr: "fd00::2" });
  api.snapshot.services.push({ service_name: "new-service", actor_cn: "new-actor", service_kind: "Regular" });
  await page.locator("#refresh-now").click();
  await expect(findings).toContainText("Actor first observed since baseline");
  await expect(findings).toContainText("Service first observed since baseline");
  expect(mutations).toEqual([]);
});

test("Security Review dismisses individual, selected and all findings and can restore them", async ({ page, appURL, api }) => {
  api.snapshot.actors = [{ cn: "adapter-a", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.trusted_sources = [{ name: "directory", health: "failed", health_note: "Lookup failed", last_lookup_ms: Date.now() }];
  api.snapshot.recent_denies = [{ source_addr: "fd00::bad", dest_addr: "fd00::1", count: 5, deny_code: "Denied" }];
  await page.goto(appURL + "/#security-review");
  const findings = page.locator("#security-review-findings");
  const denial = findings.locator("tr").filter({ hasText: "Repeated policy denials" });
  const sourceFailure = findings.locator("tr").filter({ hasText: "Trusted-source lookup failed" });
  await expect(denial).toBeVisible();
  await expect(sourceFailure).toBeVisible();
  await denial.getByRole("button", { name: /^Dismiss/ }).click();
  await expect(findings).not.toContainText("Repeated policy denials");
  await sourceFailure.getByRole("checkbox").check();
  await page.getByRole("button", { name: "Dismiss selected", exact: true }).click();
  await expect(findings).toContainText("No active findings match this filter.");
  await page.getByRole("checkbox", { name: "Show dismissed" }).check();
  await expect(findings).toContainText("Repeated policy denials");
  await expect(findings).toContainText("Trusted-source lookup failed");

  const dismissedDenial = findings.locator("tr").filter({ hasText: "Repeated policy denials" });
  await dismissedDenial.getByRole("button", { name: /^Restore/ }).click();
  await expect(dismissedDenial).toHaveAttribute("data-dismissed", "false");
  await page.getByRole("button", { name: "Dismiss all", exact: true }).click();
  await expect(findings.locator('tr[data-dismissed="false"]')).toHaveCount(0);
  await page.reload();
  await page.getByRole("checkbox", { name: "Show dismissed" }).check();
  await expect(findings).toContainText("Repeated policy denials");
  await expect(findings).toContainText("Trusted-source lookup failed");
});

test("Security Review uses Control Room polling, pause, interval and manual refresh", async ({ page, appURL, api }) => {
  await page.clock.install();
  api.snapshot.actors = [{ cn: "adapter", node: false, zpr_addr: "fd00::1" }];
  await page.goto(appURL + "/#security-review");
  await expect(page.locator("#security-review-baseline")).toContainText("1 actors");
  await expect(page.locator("#security-review-scan")).toHaveCount(0);
  expect(api.counts.get("/api/snapshot")).toBe(1);
  expect(api.counts.get("/api/adapter-logs")).toBe(1);
  await page.locator("#poll-rate").selectOption("3");
  await page.clock.runFor(3100);
  await expect.poll(() => api.counts.get("/api/snapshot")).toBe(2);
  await expect.poll(() => api.counts.get("/api/adapter-logs")).toBe(2);
  await page.locator("#pause-poll").click();
  await expect(page.locator("#pause-poll")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#pause-poll")).toHaveCSS("background-color", "rgb(168, 62, 41)");
  await expect(page.locator("#pause-status")).toHaveText("Updates paused");
  const snapshots = api.counts.get("/api/snapshot");
  const logs = api.counts.get("/api/adapter-logs");
  await page.clock.runFor(10000);
  expect(api.counts.get("/api/snapshot")).toBe(snapshots);
  expect(api.counts.get("/api/adapter-logs")).toBe(logs);
  api.snapshot.recent_denies = [{ source_addr: "fd00::2", dest_addr: "fd00::1", count: 5, deny_code: "Denied" }];
  await page.locator("#refresh-now").click();
  await expect(page.locator("#security-review-findings")).toContainText("Repeated policy denials");
  await expect.poll(() => api.counts.get("/api/snapshot")).toBe(snapshots + 1);
  await page.locator('.primary-nav [data-page-group="status"]').click();
  await expect(page.locator("#status-tabs")).toBeVisible();
  await page.locator('#status-tabs [data-page-link="services"]').click();
  await page.locator("#pause-poll").click();
  await expect(page.locator("#pause-poll")).toHaveAttribute("aria-pressed", "false");
  await expect(page.locator("#pause-status")).toBeHidden();
  const before = api.counts.get("/api/adapter-logs");
  await page.clock.runFor(3100);
  expect(api.counts.get("/api/adapter-logs")).toBe(before);
});

test("Security Review resolves entity and evidence addresses with IP hover details", async ({ page, appURL, api }) => {
  api.snapshot.actors = [{ cn: "adapter", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.recent_denies = [{ source_addr: "fd00:0:0:0:0:0:0:bad", dest_addr: "fd00::1", count: 5, deny_code: "Denied" }];
  api.dnsRecords.records = [
    { name: "caller.svc.zpr.", type: "AAAA", value: "fd00::bad" },
    { name: "database.svc.zpr.", type: "AAAA", value: "fd00::1" },
    { name: "new-actor.svc.zpr.", type: "AAAA", value: "fd00::2" },
  ];
  api.adapterLogs.machines[0].sources[0].lines = ["authentication failed from fd00::bad to fd00::1 <img src=x>"];
  await page.goto(appURL + "/#security-review");
  const findings = page.locator("#security-review-findings");
  const denial = findings.locator("tr").filter({ hasText: "Repeated policy denials" });
  await expect(denial.locator("td").nth(3)).toHaveText("caller.svc.zpr");
  await expect(denial.locator("td").nth(3).locator(".security-address")).toHaveAttribute("title", "IP address: fd00:0:0:0:0:0:0:bad");
  await expect(denial.locator("td").nth(4)).toContainText("database.svc.zpr");
  await expect(denial.locator("td").nth(4).locator(".security-address")).toHaveAttribute("title", "IP address: fd00::1");
  await expect(findings).toContainText("authentication failed from caller.svc.zpr to database.svc.zpr <img src=x>");
  await expect(findings.locator("img")).toHaveCount(0);
  api.snapshot.actors.push({ cn: "new-actor", node: false, zpr_addr: "fd00::2" });
  await page.locator("#refresh-now").click();
  const actor = findings.locator("tr").filter({ hasText: "Actor first observed since baseline" });
  await expect(actor.locator("td").nth(3)).toHaveText("new-actor.svc.zpr");
  await expect(actor.locator("td").nth(3).locator(".security-address")).toHaveAttribute("title", "IP address: fd00::2");
  await page.locator("#security-review-filter").fill("caller.svc.zpr");
  await expect(denial).toBeVisible();
  await page.locator("#security-review-filter").fill("");
  await page.locator("#pause-poll").click();
  const collections = api.counts.get("/api/adapter-logs");
  api.dnsRecords.records[0].name = "renamed-caller.svc.zpr.";
  await page.evaluate(() => loadDNSRecords(true));
  await expect(denial.locator("td").nth(3)).toHaveText("renamed-caller.svc.zpr");
  expect(api.counts.get("/api/adapter-logs")).toBe(collections);
});

test("sortable tables show defaults and clicks select then reverse a column", async ({ page, appURL, api }) => {
  api.handlers.set("/api/dns/stats/json/v1/status", async (route) => route.fulfill({ json: { "current-time": "2026-10-04T12:00:00Z" } }));
  api.handlers.set("/api/dns/stats/json/v1/server", async (route) => route.fulfill({ json: { version: "9.18", nsstats: { Requestv4: 20, Requestv6: 1, QryUDP: 20, QryTCP: 20, QryAuthAns: 20, QrySuccess: 20, QryNXDOMAIN: 20, QrySERVFAIL: 20, UpdateDone: 20, UpdateFail: 20 } } }));
  api.handlers.set("/api/dns/stats/json/v1/zones", async (route) => route.fulfill({ json: { views: { default: { zones: [
    { name: "alpha.svc.zpr.", type: "master", serial: 10, rcodes: { QrySuccess: 20, QryNXDOMAIN: 2 }, qtypes: { AAAA: 12 } },
    { name: "zeta.svc.zpr.", type: "master", serial: 2, rcodes: { QrySuccess: 15, QryNXDOMAIN: 1 }, qtypes: { AAAA: 8 } },
  ] } } } }));
  api.dnsRecords.records = [
    { name: "zeta.svc.zpr.", ttl: 100, type: "AAAA", value: "fd00::2" },
    { name: "alpha.svc.zpr.", ttl: 300, type: "AAAA", value: "fd00::1" },
  ];
  api.snapshot.recent_denies = [
    { source_addr: "fd00::a", dest_addr: "fd00::1", count: 5, last_deny_ms: 1000, deny_code: "Denied" },
    { source_addr: "fd00::b", dest_addr: "fd00::1", count: 5, last_deny_ms: 2000, deny_code: "Denied" },
  ];
  const tables = [
    ["connections", "connections", "from", "ascending", "to"],
    ["actors", "actors", "cn", "ascending", "role"],
    ["services", "services", "name", "ascending", "kind"],
    ["sources", "sources", "name", "ascending", "provider"],
    ["visas", "visas", "id", "descending", "flow"],
    ["denies", "denies", "count", "descending", "source"],
    ["dns", "dns-counters", "counter", "ascending", "value"],
    ["dns", "dns-zones", "zone", "ascending", "serial"],
    ["dns", "dns-records", "name", "ascending", "ttl"],
    ["security-review", "security-review", "observed", "descending", "indicator"],
  ];
  for (const [route, pageName, defaultKey, defaultDirection, nextKey] of tables) {
    await page.goto(`${appURL}/#${route}`);
    const table = page.locator(`table[data-sort-page="${pageName}"]`);
    if (pageName === "dns-counters") await expect(table.locator("tbody tr")).toHaveCount(10);
    if (pageName === "dns-zones") await expect(table.locator("tbody tr")).toHaveCount(2);
    if (pageName === "dns-records") await expect(table.locator("tbody tr")).toHaveCount(2);
    if (pageName === "security-review") await expect(table.locator("tbody tr")).toHaveCount(2);
    const selected = table.locator(`th[data-sort-key="${defaultKey}"]`);
    await expect(selected).toHaveAttribute("aria-sort", defaultDirection);
    const expectedArrow = defaultDirection === "ascending" ? '"↑"' : '"↓"';
    await expect.poll(() => selected.locator(".sort-button").evaluate((button) => getComputedStyle(button, "::after").content)).toBe(expectedArrow);
    const selectedBackground = await selected.evaluate((header) => getComputedStyle(header).backgroundColor);
    const inactiveBackground = await table.locator(`th[data-sort-key="${nextKey}"]`).evaluate((header) => getComputedStyle(header).backgroundColor);
    expect(selectedBackground).not.toBe(inactiveBackground);
    const next = table.locator(`th[data-sort-key="${nextKey}"]`);
    const box = await next.boundingBox();
    await next.click({ position: { x: 2, y: (box?.height || 34) / 2 } });
    await expect(next).toHaveAttribute("aria-sort", "ascending");
    await expect.poll(() => next.locator(".sort-button").evaluate((button) => getComputedStyle(button, "::after").content)).toBe('"↑"');
    await next.getByRole("button").click();
    await expect(next).toHaveAttribute("aria-sort", "descending");
    await expect.poll(() => next.locator(".sort-button").evaluate((button) => getComputedStyle(button, "::after").content)).toBe('"↓"');
  }
  await page.goto(`${appURL}/#dns`);
  await expect(page.locator("#dns-record-rows tr").first().locator("td").first()).toHaveText("alpha.svc.zpr.");
  await page.locator('table[data-sort-page="dns-records"] th[data-sort-key="ttl"]').click();
  await expect(page.locator("#dns-record-rows tr").first().locator("td").first()).toHaveText("zeta.svc.zpr.");
  await page.goto(`${appURL}/#security-review`);
  await expect(page.locator("#security-review-findings tr")).toHaveCount(2);
  await expect(page.locator("#security-review-findings tr").first().locator("td").nth(3)).toHaveText("fd00::b");
  await page.locator('table[data-sort-page="security-review"] th[data-sort-key="entity"]').click();
  await expect(page.locator("#security-review-findings tr").first().locator("td").nth(3)).toHaveText("fd00::a");
  expect(api.counts.get("/api/snapshot")).toBeGreaterThan(0);
});

test("Trusted Sources omits explanatory headings and counts while retaining source controls", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#sources");
  const sources = page.locator("#page-sources");
  await expect(sources).toBeVisible();
  await expect(sources).not.toContainText("ATTRIBUTE PROVIDERS");
  await expect(sources).not.toContainText("Who supplies trusted attributes");
  await expect(sources).not.toContainText("attribute sources");
  await expect(sources).not.toContainText("Lookup labels reflect actual attribute requests");
  await expect(page.locator("#trusted-count")).toHaveCount(0);
  await expect(page.getByRole("searchbox", { name: "Filter trusted sources", exact: true })).toBeVisible();
  await expect(sources.locator("table[data-sort-page=sources]")).toBeVisible();
});

test("Control Room sidebar routes Logging internally and promotes external managers", async ({ page, appURL, api }) => {
  const directoryURLs = [`${appURL}/?directory=one`, `${appURL}/?directory=two`];
  api.snapshot.trusted_sources = ["Directory A", "Directory B"].map((name, index) => ({ name, provider: "file", actor_cn: "directory-service", health: "working", last_lookup_ms: Date.now(), last_success_ms: Date.now(), editor_url: directoryURLs[index] }));
  await page.goto(appURL + "/#sources");
  const nav = page.locator(".primary-nav");
  await expect(nav.getByRole("link", { name: "Logging", exact: true })).toHaveAttribute("href", "#adapter-logs");
  await expect(nav.getByRole("link", { name: "Adapter Logs", exact: true })).toHaveCount(0);
  await expect(nav.getByRole("link", { name: /LDAP control GUI/i })).toHaveCount(0);
  const logManager = page.getByRole("link", { name: "Log Manager", exact: true });
  await expect(logManager).toHaveAttribute("href", "http://127.0.0.1:8798/");
  await expect(logManager).toHaveAttribute("target", "zpr-log-manager");
  await expect(logManager).toHaveClass(/sidebar-tool-prominent/);
  const manageLinks = page.getByRole("link", { name: "Manage", exact: true });
  await expect(manageLinks).toHaveCount(2);
  await expect(manageLinks.nth(0)).toHaveAttribute("href", directoryURLs[0]);
  await expect(manageLinks.nth(0)).toHaveAttribute("target", "zpr-directory-manager");
  await expect(manageLinks.nth(1)).toHaveAttribute("target", "zpr-directory-manager");
  const [directoryPage] = await Promise.all([page.context().waitForEvent("page"), manageLinks.nth(0).click()]);
  await expect.poll(() => new URL(directoryPage.url()).searchParams.get("directory")).toBe("one");
  await manageLinks.nth(1).click();
  await expect.poll(() => new URL(directoryPage.url()).searchParams.get("directory")).toBe("two");
  expect(page.context().pages()).toHaveLength(2);
  await expect(manageLinks.nth(0)).toContainText("↗");
  await expect(manageLinks.nth(0)).toHaveCSS("background-color", "rgb(233, 243, 212)");
});

test("Control Room trusted source panel browses records read-only", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#sources");
  const browser = page.locator("trusted-source-browser");
  await expect(browser.locator("[data-source-title]")).toHaveText("Trusted LDAP");
  await expect(browser.locator("[data-source-meta]")).toContainText("dc=alpha,dc=test");
  await expect(browser.locator(".trusted-source-table")).toContainText("alice");
  await browser.getByRole("tab", { name: "Groups" }).click();
  await expect(browser.locator(".trusted-source-table")).toContainText("Operators");
  await browser.getByRole("tab", { name: "Attributes" }).click();
  const filter = browser.getByRole("searchbox", { name: "Filter trusted source records" });
  await filter.fill("mail");
  await expect(browser.locator(".trusted-source-table")).toContainText("mail");
  await expect(browser.locator(".trusted-source-table")).not.toContainText("title");
  await expect(browser.getByRole("button", { name: /Save|Edit|Delete|Publish/ })).toHaveCount(0);
  expect(api.counts.get("/api/assertions/source")).toBeGreaterThan(0);
});

test("Simulator trusted source page uses the same read-only browser", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/trusted-source.html");
  const browser = page.locator("trusted-source-browser");
  await expect(page.locator('.primary-nav a[href="/trusted-source.html"]')).toHaveClass(/active/);
  await expect(browser.locator("[data-source-title]")).toHaveText("Organization LDAP");
  await expect(browser.locator(".trusted-source-table")).toContainText("alice@example.test");
  await browser.getByRole("tab", { name: "Groups" }).click();
  await expect(browser.locator(".trusted-source-table")).toContainText("Operators");
  await expect(browser.getByRole("button", { name: /Save|Edit|Delete|Publish/ })).toHaveCount(0);
  expect(api.counts.get("/api/simulator/trusted-source")).toBeGreaterThan(0);
});

test("policy picker right-click menu targets records and categories without visible action rows", async ({ page, appURL, api }, testInfo) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  const category = page.locator('[data-category-id="test"]');
  const menu = page.getByRole("menu", { name: /./ });
  await expect(category).toBeVisible();
  await expect(page.locator("#policy-picker-actions")).toHaveCount(0);
  await expect(page.locator(".catalog-pane-heading button")).toHaveCount(1);
  await expect(page.locator("#policy-picker-toggle")).toBeVisible();
  await expect(page.locator(".policy-file-actions")).toHaveCount(0);
  await expect(page.locator("#new-category")).toBeHidden();
  await category.click({ button: "right" });
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "New policy", exact: true })).toBeEnabled();
  await expect(menu.getByRole("menuitem", { name: "New subcategory", exact: true })).toBeEnabled();
  await expect(page.locator("#policy-copy")).toBeHidden();
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(category).toBeFocused();

  await page.locator('[data-record-id="test-assertions"]').click({ button: "right" });
  await expect(menu).toBeVisible();
  await expect(page.locator("#policy-picker-menu-title")).toHaveText("Organization assertions");
  await expect(menu.getByRole("menuitem", { name: "Delete", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape");

  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click({ button: "right" });
  await expect(menu).toBeVisible();
  await expect(page.locator("#policy-picker-menu-title")).toHaveText("Test policy");
  await expect(menu.getByRole("menuitem", { name: "Copy", exact: true })).toBeEnabled();
  await expect(page.locator("#new-policy-record")).toBeHidden();
  const bounds = await menu.boundingBox();
  const viewport = page.viewportSize();
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.y).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(viewport.width);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(viewport.height);
  await page.screenshot({ path: testInfo.outputPath("policy-picker-menu.png") });
  await menu.getByRole("menuitem", { name: "Copy", exact: true }).click();
  await expect(menu).toBeHidden();
  await expect(page.locator("#policy-file-status")).toHaveText("Copied Test policy");
});

test("policy picker actions support keyboard opening, navigation and dismissal", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  const record = page.locator('[data-record-id="test-policy"]');
  await record.focus();
  await record.press("Shift+F10");
  const menu = page.locator("#policy-picker-menu");
  await expect(menu).toBeVisible();
  await expect(page.locator("#policy-copy")).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(page.locator("#policy-duplicate")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(record).toBeFocused();
  await openPolicyPicker(page);
  await record.press("Shift+F10");
  await expect(menu).toBeVisible();
  await page.locator("#policy-record-title").click();
  await expect(menu).toBeHidden();
  await openPolicyPicker(page);
  await page.locator("#policy-category-tree").focus();
  await page.locator("#policy-category-tree").press("Shift+F10");
  await expect(menu).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("#policy-category-tree")).toBeFocused();
});

test("each picker row opens its own menu from labels and metadata", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  const category = page.locator('[data-category-id="test"]');
  await expect(category).toBeVisible();
  await expect(category).toHaveAttribute("aria-haspopup", "menu");
  await category.locator("span").nth(1).click({ button: "right" });
  const menu = page.locator("#policy-picker-menu");
  await expect(menu).toHaveAttribute("data-context", "category");
  await expect(menu).toBeVisible();
  await expect(page.locator("#policy-duplicate")).toBeHidden();
  await page.keyboard.press("Escape");
  for (const [id, name] of [["test-policy", "Test policy"], ["test-assertions", "Organization assertions"]]) {
    await openPolicyPicker(page);
    const row = page.locator(`[data-record-id="${id}"]`);
    await expect(row).toHaveAttribute("aria-controls", "policy-picker-menu");
    await row.locator("strong").click({ button: "right" });
    await expect(menu).toBeVisible();
    await expect(menu).toHaveAttribute("data-context", "record");
    await expect(page.locator("#policy-picker-menu-title")).toHaveText(name);
    await expect(page.locator("#new-category")).toBeHidden();
    await page.keyboard.press("Escape");
    await expect(page.locator("#policy-picker-toggle")).toBeFocused();
  }
});

test("Control Room groups status pages into counted tabs and keeps metrics on Map only", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node-a", node: true, zpr_addr: "fd00::1", node_details: { adapters: ["adapter-a"], in_sync: true } },
    { cn: "adapter-a", node: false, zpr_addr: "fd00::2" },
  ];
  api.snapshot.services = [{ service_name: "Echo", actor_cn: "adapter-a", service_kind: "Regular" }];
  api.snapshot.visa_count = 7;
  api.snapshot.recent_denies = [{ source_addr: "fd00::2", dest_addr: "fd00::3", count: 4, deny_code: "Denied" }];
  api.handlers.set("/api/dns/stats/json/v1/status", async (route) => route.fulfill({ json: { "current-time": "2026-10-05T12:00:00Z" } }));
  api.handlers.set("/api/dns/stats/json/v1/server", async (route) => route.fulfill({ json: { version: "9", nsstats: {} } }));
  api.handlers.set("/api/dns/stats/json/v1/zones", async (route) => route.fulfill({ json: { views: { internal: { zones: [{ name: "svc.zpr." }, { name: "example.test." }] } } } }));
  await page.goto(appURL + "/#services");
  await expect(page.locator(".primary-nav [data-page-group=status]")).toHaveClass(/active/);
  await expect(page.locator("#status-tabs")).toBeVisible();
  await expect(page.locator('#status-tabs [data-page-link="services"]')).toHaveClass(/active/);
  await expect(page.locator("#status-count-adapters")).toHaveText("1");
  await expect(page.locator("#status-count-actors")).toHaveText("2");
  await expect(page.locator("#status-count-services")).toHaveText("1");
  await expect(page.locator("#status-count-visas")).toHaveText("7");
  await expect(page.locator("#status-count-denies")).toHaveText("4");
  await expect(page.locator(".status-banner")).toBeHidden();
  await page.locator('#status-tabs [data-page-link="dns"]').click();
  await expect(page.locator("#status-count-dns")).toHaveText("2");
  await page.goto(appURL + "/#map");
  await expect(page.locator("#status-tabs")).toBeHidden();
  await expect(page.locator(".status-banner")).toBeVisible();
});

for (const navigation of [
  { app: "Control Room", path: "/#services", label: "Status", next: "Visas", nextPath: "/#visas" },
  { app: "Simulator", path: "/organizations.html", label: "Organizations", next: "Workload logs", nextPath: "/machine-logs.html" },
]) {
  test(`${navigation.app} side menu condenses, remembers its state and keeps the active tab visible`, async ({ page, appURL, api }, testInfo) => {
    await page.goto(appURL + navigation.path);
    const activeLink = page.locator(".primary-nav .nav-link.active");
    await expect(activeLink).toHaveText(navigation.label);
    if (navigation.app === "Control Room") {
      await expect(page.locator('#status-tabs [data-page-link="services"]')).toHaveClass(/active/);
      await expect(page.locator(".status-banner")).toBeHidden();
      await expect(page.getByRole("link", { name: "Browse policy & assertions", exact: true })).toHaveCount(0);
    }
    {
      const brand = page.locator(".sidebar-header .brand");
      await expect(brand).toBeVisible();
      await expect(brand).toContainText("ZPR");
      await expect(brand).toContainText(navigation.app === "Control Room" ? "CONTROL ROOM" : "SIMULATOR");
      await expect(brand.locator(".brand-mark")).toBeVisible();
      const alignedBesideToggle = () => brand.evaluate((element) => {
        const brandRect = element.getBoundingClientRect();
        const headerRect = element.parentElement.getBoundingClientRect();
        const toggleRect = element.parentElement.querySelector(".sidebar-toggle").getBoundingClientRect();
        return Math.abs(brandRect.left - headerRect.left) <= 1
          && Math.abs((brandRect.top + brandRect.bottom) / 2 - (toggleRect.top + toggleRect.bottom) / 2) <= 2
          && Math.abs(toggleRect.right - headerRect.right) <= 1;
      });
      expect(await alignedBesideToggle()).toBeTruthy();
    }
    await page.getByRole("button", { name: "Condense side menu", exact: true }).click();
    await expect(page.locator("body")).toHaveClass(/sidebar-condensed/);
    await expect(activeLink).toBeVisible();
    {
      const brand = page.locator(".sidebar-header .brand");
      await expect(brand).toBeVisible();
      const alignedBesideToggle = await brand.evaluate((element) => {
        const brandRect = element.getBoundingClientRect();
        const headerRect = element.parentElement.getBoundingClientRect();
        const toggleRect = element.parentElement.querySelector(".sidebar-toggle").getBoundingClientRect();
        return Math.abs(brandRect.left - headerRect.left) <= 1
          && Math.abs((brandRect.top + brandRect.bottom) / 2 - (toggleRect.top + toggleRect.bottom) / 2) <= 2
          && Math.abs(toggleRect.right - headerRect.right) <= 1;
      });
      expect(alignedBesideToggle).toBeTruthy();
    }
    await expect(page.getByRole("button", { name: "Expand side menu", exact: true })).toHaveAttribute("aria-expanded", "false");
    await page.reload();
    await expect(page.locator("body")).toHaveClass(/sidebar-condensed/);
    await expect(page.locator(".primary-nav .nav-link.active")).toHaveText(navigation.label);
    if (navigation.app === "Control Room") await expect(page.locator('#status-tabs [data-page-link="services"]')).toHaveClass(/active/);
    if (testInfo.project.name === "desktop") {
      await expect(page.locator(".sidebar")).toHaveCSS("width", "104px");
      await expect(page.locator(".main-content")).toHaveCSS("margin-left", "104px");
    }
    await page.screenshot({ path: testInfo.outputPath("sidebar-condensed.png"), fullPage: true });
    await page.getByRole("button", { name: "Expand side menu", exact: true }).click();
    const nextLink = navigation.app === "Control Room"
      ? page.locator(`#status-tabs a[href="${navigation.nextPath.slice(1)}"]`)
      : page.getByRole("link", { name: navigation.next, exact: true });
    await nextLink.click();
    await expect(page).toHaveURL(appURL + navigation.nextPath);
    if (navigation.app === "Control Room") await expect(page.locator('#status-tabs [data-page-link="visas"]')).toHaveClass(/active/);
    else await expect(page.locator(".primary-nav .nav-link.active")).toHaveText(navigation.next);
    await page.getByRole("button", { name: "Condense side menu", exact: true }).click();
    await page.goBack();
    await expect(page.locator(".primary-nav .nav-link.active")).toHaveText(navigation.label);
    await expect(page.locator("body")).toHaveClass(/sidebar-condensed/);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  });
}

test("policy Format condenses repeated blank lines including trailing whitespace", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const editor = page.locator("#policy-source");
  await expect(editor).toHaveValue("define Employee as user.\n");
  await editor.fill("# Heading\n\n \n\ndefine Employee as user.\n\n\nprovide Api at api.example over TCP 443.\n\nallow Employee.\n\n\n");
  const expected = "# Heading\n\ndefine Employee as user.\n\nprovide Api at api.example over TCP 443.\n  allow Employee.\n\n";
  await page.locator("#policy-format").click();
  await expect(editor).toHaveValue(expected);
  await page.locator("#policy-format").click();
  await expect(editor).toHaveValue(expected);
  const crlf = await page.evaluate(() => formatZPL("# Heading\r\n\r\n \r\n\r\ndefine Employee as user.\r\n\r\n\r\n"));
  expect(crlf).toBe("# Heading\r\n\r\ndefine Employee as user.\r\n\r\n");
});

test("policy Format removes leading whitespace and gaps inside definition and service groups", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const editor = page.locator("#policy-source");
  await expect(editor).toHaveValue("define Employee as user.\n");
  const source = " \n\t\n  define Employee as user.\n\n# Staff\n\ndefine Guest as user.\nprovide Api at api.example over TCP 443.\n\n# Callers\n\nallow Employee.\n\ndeny Guest.\n\nnever allow Guest.\n";
  const expected = "define Employee as user.\n# Staff\ndefine Guest as user.\n\nprovide Api at api.example over TCP 443.\n# Callers\n  allow Employee.\n  deny Guest.\n  never allow Guest.\n";
  await editor.fill(source);
  await page.locator("#policy-format").click();
  await expect(editor).toHaveValue(expected);
  await page.locator("#policy-format").click();
  await expect(editor).toHaveValue(expected);
  const crlf = await page.evaluate((text) => formatZPL(text), "\r\nservice Api as json {}.\r\n\r\nallow Employee.\r\n");
  expect(crlf).toBe("service Api as json {}.\r\n  allow Employee.\r\n");
});

test("policy actions share a non-overlapping responsive toolbar", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/records/test-policy/revisions", async (route) => route.fulfill({ json: [
    { number: 1, content_hash: "fixture-revision", summary: "Saved fixture revision", author: "tester", created_at: "2026-10-01T12:00:00Z" },
  ] }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const sourceEditor = page.locator("#policy-source");
  const savedSource = await sourceEditor.inputValue();
  await expect(page.locator("#policy-editor-mode")).toHaveText("Policy");
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  await expect(page.locator(".policy-toolbar")).toHaveCSS("border-top-style", "none");
  const desktopModeCenter = await page.evaluate(() => {
    const tools = document.querySelector(".policy-editor-tools").getBoundingClientRect();
    const mode = document.querySelector("#policy-editor-mode").getBoundingClientRect();
    return Math.abs((tools.left + tools.right) / 2 - (mode.left + mode.right) / 2);
  });
  expect(desktopModeCenter).toBeLessThanOrEqual(1);
  await sourceEditor.fill(`${savedSource}# modified\n`);
  await expect(page.locator("#policy-modified-indicator")).toHaveText("Modified");
  await sourceEditor.fill(savedSource);
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  const actions = page.locator("#policy-actions");
  const browse = page.locator("#policy-picker-toggle");
  const files = page.locator("#policy-files-toggle");
  const filesMenu = page.locator("#policy-file-menu");
  const rescan = page.locator("#policy-attribute-rescan");
  await expect(rescan).toHaveText("Refresh Attributes");
  const analyze = page.locator("#policy-check");
  const format = page.locator("#policy-format");
  const history = page.locator("#policy-history-menu");
  await expect(browse).toHaveText("Browse...⌄");
  await expect(files).toHaveText("File...⌄");
  await expect(analyze).toHaveText("Analyze");
  await expect(analyze.locator("xpath=..")).toHaveClass(/policy-attribute-toolbar/);
  await expect(browse).toHaveCSS("background-color", "rgb(23, 33, 30)");
  await expect(files).toHaveCSS("background-color", "rgb(23, 33, 30)");
  await expect(page.locator("#policy-history-heading")).toHaveCount(0);
  await expect(history).toBeVisible();
  await expect(page.locator("#policy-history")).toBeHidden();
  await history.locator("summary").click();
  await expect(page.locator("#policy-history")).toBeVisible();
  await expect(page.locator("#policy-history .history-item")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(page.locator("#policy-history")).toBeHidden();
  await expect(history.locator("summary")).toBeFocused();
  const saveAsColor = await page.locator("#policy-save-as").evaluate((button) => getComputedStyle(button).backgroundColor);
  await expect(format).toHaveCSS("background-color", saveAsColor);
  await expect(rescan).toHaveCSS("background-color", saveAsColor);
  await openPolicyFiles(page);
  await expect(await actions.getByRole("menuitem").allTextContents()).toEqual(["Save", "Save As...", "Stage", "Discard"]);
  await page.keyboard.press("Escape");
  await expect(filesMenu).toBeHidden();
  await expect(files).toBeFocused();
  for (const width of [834, 768]) {
    await page.setViewportSize({ width, height: 900 });
    const layout = await page.evaluate(() => {
      const toolbar = document.querySelector(".policy-attribute-toolbar").getBoundingClientRect();
      const editor = document.querySelector("#policy-code-editor").getBoundingClientRect();
      const tools = document.querySelector(".policy-editor-tools").getBoundingClientRect();
      const mode = document.querySelector("#policy-editor-mode").getBoundingClientRect();
      const utilities = document.querySelector("#policy-editor-utilities").getBoundingClientRect();
      const rescanRect = document.querySelector("#policy-attribute-rescan").getBoundingClientRect();
      const historyRect = document.querySelector("#policy-history-menu summary").getBoundingClientRect();
      const analyzeRect = document.querySelector("#policy-check").getBoundingClientRect();
      const formatRect = document.querySelector("#policy-format").getBoundingClientRect();
      const pane = document.querySelector("#policy-editor-pane").getBoundingClientRect();
      const toolsStyle = getComputedStyle(document.querySelector(".policy-editor-tools"));
      const buttons = [...document.querySelectorAll(".policy-attribute-toolbar .button")].map((button) => button.getBoundingClientRect());
      return {
        toolbarBottom: toolbar.bottom,
        editorTop: editor.top,
        utilitiesRight: utilities.right,
        utilitiesLeft: utilities.left,
        utilitiesWidth: utilities.width,
        toolsRight: tools.right,
        toolsLeft: tools.left,
        toolsWidth: tools.width,
        toolsCssWidth: toolsStyle.width,
        toolsAlignSelf: toolsStyle.alignSelf,
        modeCenter: mode.left + mode.width / 2,
        toolsCenter: tools.left + tools.width / 2,
        paneRight: pane.right,
        paneLeft: pane.left,
        paneWidth: pane.width,
        rescanTop: rescanRect.top,
        historyTop: historyRect.top,
        analyzeTop: analyzeRect.top,
        formatTop: formatRect.top,
        analyzeRight: analyzeRect.right,
        formatLeft: formatRect.left,
        order: ["#policy-picker-toggle", "#policy-files-toggle", "#policy-check", "#policy-format", "#policy-attribute-rescan", "#policy-history-menu"].map((selector) => [...document.querySelectorAll("#policy-editor-pane *")].indexOf(document.querySelector(selector))),
        overlaps: buttons.some((first, index) => buttons.slice(index + 1).some((second) => first.left < second.right && first.right > second.left && first.top < second.bottom && first.bottom > second.top)),
      };
    });
    expect(layout.toolbarBottom).toBeLessThanOrEqual(layout.editorTop);
    expect(Math.abs(layout.modeCenter - layout.toolsCenter), JSON.stringify(layout)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.utilitiesRight - layout.toolsRight), JSON.stringify(layout)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.rescanTop - layout.historyTop)).toBeLessThanOrEqual(1);
    expect(layout.analyzeTop).toBe(layout.formatTop);
    expect(layout.analyzeRight).toBeLessThanOrEqual(layout.formatLeft);
    expect(layout.order).toEqual([...layout.order].sort((left, right) => left - right));
    expect(layout.overlaps).toBeFalsy();
    const { browseBounds, filesBounds } = await page.evaluate(() => {
      const bounds = (element) => {
        const rect = element.getBoundingClientRect();
        return { x: rect.x, width: rect.width };
      };
      return {
        browseBounds: bounds(document.querySelector("#policy-picker-toggle")),
        filesBounds: bounds(document.querySelector("#policy-files-toggle")),
      };
    });
    expect(browseBounds.x + browseBounds.width).toBeLessThanOrEqual(filesBounds.x);
  }
  await openPolicyFiles(page);
  await expect(actions.getByRole("menuitem", { name: "Stage", exact: true })).toBeEnabled();
});

test("Save is dirty-gated and uses the Save As color", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const save = page.locator("#policy-save");
  const saveAs = page.locator("#policy-save-as");
  await expect(save).toBeDisabled();
  await expect(saveAs).toBeEnabled();
  const saveAsColor = await saveAs.evaluate((button) => getComputedStyle(button).backgroundColor);
  await page.locator("#policy-source").fill(`${await page.locator("#policy-source").inputValue()}\n# unsaved change\n`);
  await expect(save).toBeEnabled();
  await expect(save).toHaveCSS("background-color", saveAsColor);
  await openPolicyFiles(page);
  await expect(page.locator("#policy-save")).toBeEnabled();
  await expect(page.locator("#policy-file-menu")).toBeVisible();
});

test("Stage analyzes, blocks errors, and requires confirmation", async ({ page, appURL, api }) => {
  api.policy.staging_ready = true;
  let checkCalls = 0;
  let testCalls = 0;
  let stageCalls = 0;
  api.handlers.set("/api/policy/check", async (route) => {
    checkCalls += 1;
    if (checkCalls === 1) return route.fulfill({ status: 422, json: { valid: false, diagnostics: "error: [ line 1, column 1 ] invalid" } });
    return route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } });
  });
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ json: { actors: [], services: [], warnings: [] } }));
  api.handlers.set("/api/policy/test", async (route) => {
    testCalls += 1;
    await route.fulfill({ json: { api_version: 1, actor_count: 0, services: [] } });
  });
  api.handlers.set("/api/policy/records/test-policy/stage", async (route) => {
    stageCalls += 1;
    await route.fulfill({ json: { record_name: "Test policy", record_revision: 1, bundle_sha256: "a".repeat(64) } });
  });
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const stage = page.locator("#policy-stage");
  const dialog = page.locator("#policy-stage-dialog");
  await expect(stage).toHaveText("Stage");
  await expect(stage).toBeEnabled();
  await openPolicyFiles(page);
  await stage.click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  await expect(dialog).toBeHidden();
  await expect(page.locator("#policy-stage-status")).toContainText("Analysis failed");
  expect(testCalls).toBe(0);
  expect(stageCalls).toBe(0);

  await openPolicyFiles(page);
  await stage.click();
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Test policy · Version 1");
  await expect(dialog).toContainText("does not push or activate");
  expect(testCalls).toBe(1);
  expect(stageCalls).toBe(0);
  await dialog.locator("#policy-stage-confirm").click();
  await expect.poll(() => stageCalls).toBe(1);
  await expect(page.locator("#policy-stage-status")).toContainText("Staged Test policy r1");
});

test("policy source with evaluation errors can be saved with warning but cannot be staged", async ({ page, appURL, api }) => {
  api.policy.staging_ready = true;
  const invalidSource = "define Employee as user.\nthis is not valid ZPL.\n";
  api.handlers.set("/api/policy/check", async (route) => route.fulfill({
    status: 422,
    json: { valid: false, diagnostics: "error: [ line 2, column 1 ] unexpected token" },
  }));
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ json: {
    actors: [{ id: "alice", label: "Alice", kind: "user", dimensions: { user: "alice" }, attributes: [] }],
    services: [{ id: "EchoWeb", name: "EchoWeb", protocol: "TCP", port: 8080, attributes: [] }], warnings: [],
  } }));
  api.handlers.set("/api/policy/test", async (route) => route.fulfill({ status: 422, json: { error: "Candidate policy compilation failed: test fixture error" } }));
  let savedRequest;
  api.handlers.set("/api/policy/records/test-policy/revisions", async (route) => {
    if (route.request().method() === "GET") return route.fulfill({ json: [] });
    savedRequest = route.request().postDataJSON();
    api.policy.records[0].current_revision = 2;
    api.policy.records[0].content = savedRequest.content;
    await route.fulfill({ json: { number: 2, content_hash: "invalid-revision" } });
  });
  await page.goto(appURL + "/#policy");
  await expect(page.locator("#policy-record-title")).toHaveText("Test policy");
  await page.locator("#policy-source").fill(invalidSource);
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  const compilerErrorMarker = page.locator('#policy-test-gutter [data-line="2"] .policy-test-line-result[data-effect="error"]');
  await expect(compilerErrorMarker).toHaveText("ERR");
  await expect(page.locator("#policy-check-result")).toBeHidden();
  await expect(page.locator("#policy-test-status")).toBeHidden();
  await compilerErrorMarker.click();
  await expect(page.locator("#policy-test-details")).toContainText("unexpected token");
  await page.locator("#policy-test-details-close").click();
  await page.mouse.move(0, 0);
  await expect(page.locator("#policy-check")).toHaveCSS("background-color", "rgb(184, 59, 59)");
  await expect(page.locator("#policy-check-result")).toBeHidden();
  await expect(page.locator("#policy-save")).toBeEnabled();
  await openPolicyFiles(page);
  await page.locator("#policy-save").click();
  await expect(page.locator("#version-warning")).toBeVisible();
  await expect(page.locator("#version-warning")).toContainText("Policy test failed");
  await expect(page.locator("#version-save")).toHaveText("Save anyway");
  await page.locator("#version-save").click();
  await expect.poll(() => savedRequest).toBeTruthy();
  expect(savedRequest.content).toBe(invalidSource);
  await expect(page.locator("#policy-check-result")).toBeHidden();
  await expect(page.locator("#policy-stage")).toBeEnabled();
  await expect(page.locator("#policy-stage-status")).toBeHidden();
  const copiedRecord = { ...api.policy.records[0], id: "invalid-policy-copy", name: "Test policy copy", current_revision: 1, content: invalidSource, content_hash: "invalid-copy" };
  let saveAsRequest;
  api.handlers.set("/api/policy/records", async (route) => {
    saveAsRequest = route.request().postDataJSON();
    api.policy.records.push(copiedRecord);
    await route.fulfill({ status: 201, json: copiedRecord });
  });
  api.handlers.set("/api/policy/records/invalid-policy-copy", async (route) => route.fulfill({ json: copiedRecord }));
  api.handlers.set("/api/policy/records/invalid-policy-copy/revisions", async (route) => route.fulfill({ json: [] }));
  await openPolicyFiles(page);
  await page.locator("#policy-save-as").click();
  await expect(page.locator("#record-warning")).toBeVisible();
  await expect(page.locator("#record-warning")).toContainText("cannot be staged");
  await page.locator('#record-form button[type="submit"]').click();
  await expect(page.locator("#record-warning")).toContainText("Policy test failed");
  await expect(page.locator("#record-submit")).toHaveText("Save anyway");
  expect(saveAsRequest).toBeUndefined();
  await page.locator('#record-form button[type="submit"]').click();
  await expect.poll(() => saveAsRequest).toBeTruthy();
  expect(saveAsRequest.content).toBe(invalidSource);
  await expect(page.locator("#policy-record-title")).toHaveText("Test policy copy");
});

test("policy picker opens as a pulldown, closes after selection and leaves the editor wide", async ({ page, appURL, api }, testInfo) => {
  await page.goto(appURL + "/#policy");
  await expect(page.locator("#policy-record-title")).toHaveText("Test policy");
  const editor = page.locator("#policy-editor-pane");
  const picker = page.locator("#policy-catalog-pane");
  const assistant = page.locator("#policy-assistant-pane");
  const pickerToggle = page.locator("#policy-picker-toggle");
  const assistantToggle = page.locator("#policy-assistant-toggle");
  const initialEditorWidth = await editor.evaluate((element) => element.getBoundingClientRect().width);
  await expect(picker).toBeHidden();
  await expect(pickerToggle).toHaveAttribute("aria-expanded", "false");
  if ((page.viewportSize()?.width || 1000) > 900) {
    const workbenchWidth = await page.locator("#policy-workbench").evaluate((element) => element.getBoundingClientRect().width);
    expect(initialEditorWidth / workbenchWidth).toBeGreaterThan(0.6);
  }

  await openPolicyPicker(page);
  await expect(page.locator("#policy-category-tree")).toBeVisible();
  const overlayEditorWidth = await editor.evaluate((element) => element.getBoundingClientRect().width);
  expect(Math.abs(overlayEditorWidth - initialEditorWidth)).toBeLessThanOrEqual(1);
  await page.locator('[data-record-id="test-assertions"]').click();
  await expect(picker).toBeHidden();
  await expect(page.locator("#policy-record-title")).toHaveText("Organization assertions");
  await expect(page.locator("#policy-assertion-editor")).toBeVisible();

  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await expect(picker).toBeHidden();
  await openPolicyPicker(page);
  await page.locator("#policy-record-title").click();
  await expect(picker).toBeHidden();
  await openPolicyPicker(page);
  await page.keyboard.press("Escape");
  await expect(picker).toBeHidden();
  await expect(pickerToggle).toBeFocused();

  await assistantToggle.click();
  await expect(assistant).toHaveAttribute("data-collapsed", "true");
  await expect(assistantToggle).toHaveAttribute("aria-expanded", "false");
  await expect(page.locator(".assistant-settings")).toBeHidden();
  const assistantLabel = assistantToggle.locator(".pane-toggle-label");
  await expect(assistantLabel).toHaveText("AI Assistant");
  await expect(assistantLabel).toHaveCSS("writing-mode", "vertical-rl");
  await expect(assistantLabel).toHaveCSS("text-orientation", "mixed");
  await expect(assistantLabel).toHaveCSS("transform", "matrix(-1, 0, 0, -1, 0, 0)");
  await expect(assistantLabel).toHaveCSS("font-size", "14px");
  for (const [button, label] of [[assistantToggle, assistantLabel]]) {
    const outer = await button.boundingBox();
    const inner = await label.boundingBox();
    expect(inner.x).toBeGreaterThanOrEqual(outer.x);
    expect(inner.y).toBeGreaterThanOrEqual(outer.y);
    expect(inner.x + inner.width).toBeLessThanOrEqual(outer.x + outer.width);
    expect(inner.y + inner.height).toBeLessThanOrEqual(outer.y + outer.height);
  }
  await page.screenshot({ path: testInfo.outputPath("policy-picker-pulldown.png"), fullPage: true });

  await page.reload();
  await expect(picker).toBeHidden();
  await expect(page.locator("#policy-workbench")).toHaveAttribute("data-assistant-collapsed", "true");
  await assistantToggle.click();
  await expect(assistant).toHaveAttribute("data-collapsed", "false");
});

test("read-only browser filters policy and assertion records and shows saved revisions", async ({ page, appURL, api }, testInfo) => {
  const methods = [];
  page.on("request", (request) => { if (new URL(request.url()).pathname.startsWith("/api/")) methods.push(request.method()); });
  api.policy.organization_id = "alpha";
  api.policy.organization_name = "Alpha Labs";
  api.policy.records[0].current_revision = 2;
  api.policy.records[0].content = "define Employee as user.\nprovide Payroll at payroll.svc.zpr over TCP 443.\nallow Employee.\n";
  api.policy.records[1].content = JSON.stringify({ source: 'group "Operators" members >= 2;\n// <img src=x onerror=alert(1)>', enabled: true, interval_seconds: 120 });
  api.handlers.set("/api/policy/records/test-policy/revisions", async (route) => route.fulfill({ json: [
    { number: 2, summary: "Payroll access", author: "operator" }, { number: 1, summary: "Initial definition", author: "operator" },
  ] }));
  api.handlers.set("/api/policy/records/test-policy/revisions/1", async (route) => route.fulfill({ json: {
    number: 1, content: "define Employee as user.\n", summary: "Initial definition", author: "operator",
  } }));
  await page.goto(appURL + "/policy-browser.html");
  const viewer = page.locator("zpr-policy-browser");
  await expect(viewer.locator(".pb-organization")).toHaveText("Alpha Labs");
  await expect(viewer.locator(".pb-record")).toHaveCount(2);
  await expect(viewer.locator('[data-record-id="test-policy"]')).toHaveText("Test policy");
  await expect(viewer.locator('[data-record-id="test-assertions"]')).toHaveText("Organization assertions");
  await expect(viewer.locator(".pb-record small")).toHaveCount(0);
  await expect(viewer.locator('[data-record-id="test-policy"] .policy-kind-icon')).toHaveAttribute("data-icon", "traffic-light");
  await expect(viewer.locator('[data-record-id="test-assertions"] .policy-kind-icon')).toHaveAttribute("data-icon", "database-check");
  const policyColor = await viewer.locator('[data-record-id="test-policy"] .policy-kind-icon').evaluate((item) => getComputedStyle(item).color);
  const assertionColor = await viewer.locator('[data-record-id="test-assertions"] .policy-kind-icon').evaluate((item) => getComputedStyle(item).color);
  expect(assertionColor).not.toBe(policyColor);
  await viewer.locator('[data-record-id="test-policy"]').click();
  await expect(viewer.locator(".pb-source")).toContainText("allow Employee.");
  await viewer.getByLabel("Revision", { exact: true }).selectOption("1");
  await expect(viewer.locator(".pb-source")).toHaveText("define Employee as user.\n");
  await expect(viewer.locator(".pb-meta")).toContainText("Initial definition");
  await viewer.getByLabel("Type", { exact: true }).selectOption("assertions");
  await expect(viewer.locator(".pb-record")).toHaveCount(1);
  await viewer.locator('[data-record-id="test-assertions"]').click();
  await expect(viewer.locator(".pb-source")).toContainText('group "Operators" members >= 2;');
  await expect(viewer.locator(".pb-schedule")).toHaveText("Periodic checks: Enabled / Interval: 120 seconds");
  await expect(viewer.locator("img, textarea, [contenteditable=true]")).toHaveCount(0);
  await viewer.getByLabel("Search", { exact: true }).fill("missing");
  await expect(viewer.locator(".pb-records")).toHaveText("No matching records");
  await viewer.getByLabel("Search", { exact: true }).fill("Operators");
  await expect(viewer.locator(".pb-record")).toHaveCount(0);
  await viewer.getByLabel("Search", { exact: true }).fill("Organization");
  await expect(viewer.locator(".pb-record")).toHaveCount(1);
  await viewer.getByLabel("Category", { exact: true }).selectOption("test");
  await viewer.getByLabel("Wrap lines", { exact: true }).uncheck();
  await expect(viewer.locator(".pb-source")).toHaveAttribute("data-wrap", "false");
  expect(methods.length).toBeGreaterThan(0);
  expect(methods.every((method) => method === "GET")).toBeTruthy();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath("policy-browser.png"), fullPage: true });
});

test("read-only browser keeps last good source on failures and clears it on an organization switch", async ({ page, appURL, api }) => {
  api.policy.organization_id = "alpha";
  api.policy.organization_name = "Alpha Labs";
  await page.goto(appURL + "/policy-browser.html");
  const viewer = page.locator("zpr-policy-browser");
  await viewer.locator('[data-record-id="test-policy"]').click();
  await expect(viewer.locator(".pb-source")).toContainText("define Employee as user.");
  api.statuses.set("/api/policy", 503);
  await viewer.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(viewer.locator(".pb-error")).not.toBeEmpty();
  await expect(viewer.locator(".pb-source")).toContainText("define Employee as user.");
  api.statuses.delete("/api/policy");
  api.policy.organization_id = "beta";
  api.policy.organization_name = "Beta Labs";
  api.policy.records = [];
  await viewer.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(viewer.locator(".pb-organization")).toHaveText("Beta Labs");
  await expect(viewer.locator(".pb-source code")).toBeEmpty();
  await expect(viewer.locator(".pb-title")).toHaveText("Select a record");
});

test("read-only browser can be embedded twice with independent filters and API bases", async ({ page, appURL, api }) => {
  api.handlers.set("/api/archive", async (route) => route.fulfill({ json: { ...api.policy, organization_name: "Archive" } }));
  api.handlers.set("/api/archive/records/test-policy", async (route) => route.fulfill({ json: api.policy.records[0] }));
  api.handlers.set("/api/archive/records/test-policy/revisions", async (route) => route.fulfill({ json: [] }));
  await page.goto(appURL + "/policy-browser.html");
  await page.evaluate(() => {
    const viewer = document.createElement("zpr-policy-browser");
    viewer.setAttribute("api-base", "/api/archive");
    document.querySelector("main").append(viewer);
  });
  const viewers = page.locator("zpr-policy-browser");
  await expect(viewers.nth(0).locator(".pb-record")).toHaveCount(2);
  await expect(viewers.nth(1).locator(".pb-record")).toHaveCount(2);
  await expect(viewers.nth(1).locator(".pb-organization")).toHaveText("Archive");
  await viewers.nth(0).getByLabel("Type", { exact: true }).selectOption("assertions");
  await expect(viewers.nth(0).locator(".pb-record")).toHaveCount(1);
  await expect(viewers.nth(1).locator(".pb-record")).toHaveCount(2);
  await viewers.nth(1).locator('[data-record-id="test-policy"]').click();
  await expect(viewers.nth(1).locator(".pb-source")).toContainText("define Employee as user.");
  await expect(viewers.nth(0).locator(".pb-title")).toHaveText("Select a record");
});

test("Analyze gutter opens a dialog and resets when switching policies", async ({ page, appURL, api }) => {
  const secondPolicy = { ...api.policy.records[0], id: "test-policy-next", name: "Second policy", content: "define SecondGroup as user.\n" };
  api.policy.records.push(secondPolicy);
  api.handlers.set("/api/policy/records/test-policy-next", async (route) => route.fulfill({ json: secondPolicy }));
  api.handlers.set("/api/policy/records/test-policy-next/revisions", async (route) => route.fulfill({ json: [] }));
  let checkCalls = 0;
  api.handlers.set("/api/policy/check", async (route) => {
    checkCalls += 1;
    if (checkCalls > 1) return route.fulfill({ status: 422, json: { valid: false, diagnostics: "error: [ line 2, column 1 ] unexpected token" } });
    return route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } });
  });
  api.handlers.set("/api/policy/test/fixtures", async (route) => {
    await route.fulfill({ json: {
      actors: [
        { id: "alice", label: "Alice Rivera", kind: "user", dimensions: { user: "alice" }, attributes: [{ key: "user.zpr.authority", values: ["demo"] }] },
        { id: "alice-machine", label: "Alice Rivera on machine-1", kind: "user_device", dimensions: { user: "alice", device: "machine-1" }, attributes: [{ key: "device.zpr.authority", values: ["zpr-bootstrap"] }] },
      ],
      services: [{ id: "EchoWeb", name: "EchoWeb", protocol: "TCP", port: 8080, attributes: [] }],
      warnings: [],
    } });
  });
  api.handlers.set("/api/policy/test", async (route) => {
    const request = route.request().postDataJSON();
    expect(Object.keys(request).sort()).toEqual(["actors", "services", "source"]);
    expect(request.source).toBe("define Employee as user.\n");
    await route.fulfill({ json: {
      api_version: 1, source_sha256: "fixture", tested_at: "2026-10-03T12:00:00Z", actor_count: 2,
      services: [{
        id: "EchoWeb", name: "EchoWeb", protocol: "TCP", port: 8080, supported: true, evaluated_actors: 2,
        allowed: { count: 1, by_kind: { user_device: 1 }, by_dimension: { user: 1, device: 1 }, subjects: [{ id: "alice-machine", label: "Alice Rivera on machine-1", kind: "user_device", dimensions: { user: "alice", device: "machine-1" } }] },
        denied: { count: 1, by_kind: { user: 1 }, by_dimension: { user: 1 }, subjects: [{ id: "alice", label: "Alice Rivera", kind: "user", dimensions: { user: "alice" } }] },
        default_denied: { count: 0, by_kind: {}, by_dimension: {}, subjects: [] },
        rules: [
          { indexes: [2], line: 2, source: "allow users to access EchoWeb", effect: "allow", matched: { count: 1, by_kind: { user_device: 1 }, by_dimension: { user: 1, device: 1 }, subjects: [{ id: "alice-machine", label: "Alice Rivera on machine-1", kind: "user_device", dimensions: { user: "alice", device: "machine-1" } }] } },
          { indexes: [4], line: 2, source: "allow users to access EchoWeb", effect: "allow", matched: { count: 0, by_kind: {}, by_dimension: { user: 0, device: 0 }, subjects: [] } },
          { indexes: [3], line: 3, source: "never allow users to access EchoWeb", effect: "deny", matched: { count: 1, by_kind: { user: 1 }, by_dimension: { user: 1 }, subjects: [{ id: "alice", label: "Alice Rivera", kind: "user", dimensions: { user: "alice" } }] } },
        ],
      }],
    } });
  });
  await page.goto(appURL + "/#policy");
  await expect(page.locator("#policy-stage-status")).not.toContainText("Saved revision can be staged");
  await expect(page.locator("#policy-attribute-status")).toBeHidden();
  await expect(page.locator("#policy-check")).toBeEnabled();
  const gutter = page.locator("#policy-test-gutter");
  await expect(gutter).toBeVisible();
  await expect(gutter).toHaveCSS("width", "48px");
  await expect(gutter).toHaveCSS("overflow-y", "hidden");
  await expect(page.locator("#policy-source")).toHaveCSS("padding-left", "0px");
  const scrollGeometry = await page.locator("#policy-code-editor").evaluate((editor) => {
    const source = editor.querySelector("#policy-source");
    const gutter = editor.querySelector("#policy-test-gutter");
    const original = source.value;
    source.value = `${original}\n${"x".repeat(600)}`;
    source.scrollLeft = 160;
    const sourceBounds = source.getBoundingClientRect();
    const gutterBounds = gutter.getBoundingClientRect();
    const geometry = {
      scrollable: source.scrollWidth > source.clientWidth,
      scrollLeft: source.scrollLeft,
      sourceLeft: sourceBounds.left,
      textPaddingLeft: getComputedStyle(source).paddingLeft,
      gutterRight: gutterBounds.right,
      sourceBottom: sourceBounds.bottom,
      gutterBottom: gutterBounds.bottom,
    };
    source.value = original;
    source.scrollLeft = 0;
    return geometry;
  });
  expect(scrollGeometry.scrollable).toBeTruthy();
  expect(scrollGeometry.scrollLeft).toBeGreaterThan(0);
  expect(scrollGeometry.sourceLeft).toBeGreaterThanOrEqual(scrollGeometry.gutterRight);
  expect(scrollGeometry.textPaddingLeft).toBe("0px");
  expect(scrollGeometry.sourceBottom).toBeLessThanOrEqual(scrollGeometry.gutterBottom + 1);
  const editor = page.locator("#policy-source");
  const originalSource = await editor.inputValue();
  await editor.fill(`${originalSource}\n${"x".repeat(600)}`);
  await expect(page.locator("#policy-code-editor")).toHaveAttribute("data-horizontal-overflow", "true");
  await expect.poll(() => page.locator("#policy-code-editor").evaluate((element) => getComputedStyle(element, "::after").backgroundColor)).toBe("rgb(255, 255, 255)");
  await editor.fill(originalSource);
  await expect(page.locator("#policy-code-editor")).toHaveAttribute("data-horizontal-overflow", "false");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveText("Analyze");
  await expect(page.locator("#policy-check")).toBeVisible();
  await expect(page.locator("#policy-check")).toBeEnabled();
  await expect(page.locator("#policy-source")).toBeEnabled();
  await expect(page.locator("#policy-source")).toBeEditable();
  await expect(page.locator(".policy-catalog-pane")).toBeHidden();
  await expect(page.locator("#policy-test-gutter")).toBeVisible();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "success");
  await page.mouse.move(0, 0);
  await expect(page.locator("#policy-check")).toHaveCSS("background-color", "rgb(35, 117, 76)");
  const lineResult = page.locator('#policy-test-gutter [data-line="2"] .policy-test-line-result');
  await expect(lineResult).toHaveCount(1);
  await expect(lineResult).toHaveText("1");
  await lineResult.click();
  const details = page.locator("#policy-test-details");
  await expect(details).toBeVisible();
  await expect(details.locator("#policy-analyze-title")).toBeVisible();
  await expect(details.locator("#policy-analyze-subjects")).toContainText("Alice Rivera on machine-1");
  await expect(details.locator("#policy-analyze-subject-title")).toContainText("Line 2 · EchoWeb · 1");
  await expect(details.locator("#policy-analyze-subjects")).toContainText("Device: machine-1");
  await page.evaluate(() => showPolicyTestSubjects(Array.from({ length: 105 }, (_, index) => ({ id: `member-${index}`, label: `Member ${index}`, dimensions: { device: `device-${index}` } })), "", "Device matches"));
  await expect(details.locator(".policy-test-subject")).toHaveCount(100);
  await expect(details.locator("#policy-analyze-subject-title")).toContainText("105 members · First 100 shown");
  await expect(details.locator(".policy-test-subject").last()).toContainText("Member 99");
  await details.locator("#policy-test-details-close").click();
  await expect(details).toBeHidden();
  await page.locator("#policy-check").click();
  await expect.poll(() => checkCalls).toBe(2);
  await expect(page.locator("#policy-check")).toHaveText("Analyze");
  await expect(page.locator("#policy-source")).toBeEditable();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator('#policy-test-gutter [data-line="2"] .policy-test-line-result[data-effect="error"]')).toHaveText("ERR");
  await expect(page.locator("#policy-check-result")).toBeHidden();
  await expect(page.locator("#policy-test-status")).toBeHidden();
  await expect(page.locator(".policy-catalog-pane")).toBeHidden();
  await expect(gutter).toBeVisible();
  await expect(page.locator("#policy-source")).toHaveCSS("padding-left", "0px");
  await page.evaluate(() => { window.confirm = () => true; });
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy-next"]').click();
  await expect(page.locator("#policy-record-title")).toHaveText("Second policy");
  await expect(page.locator("#policy-source")).toHaveValue("define SecondGroup as user.\n");
  await expect(gutter.locator(".policy-test-line-result")).toHaveCount(0);
  await expect(page.locator("#policy-check-result")).toBeHidden();
  await expect(page.locator("#policy-test-status")).toBeHidden();
  await expect(details).toBeHidden();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("browsing historical revisions only warns for dirty working source", async ({ page, appURL, api }) => {
  const savedSource = "define Employee as user.\n";
  const dirtySource = "define Employee as user.\n# unsaved edit\n";
  api.policy.records[0].current_revision = 3;
  api.policy.records[0].content = savedSource;
  api.handlers.set("/api/policy/records/test-policy/revisions", async (route) => route.fulfill({ json: [
    { number: 1, content_hash: "revision-1", summary: "First revision", author: "tester", created_at: "2026-10-01T12:00:00Z" },
    { number: 2, content_hash: "revision-2", summary: "Second revision", author: "tester", created_at: "2026-10-02T12:00:00Z" },
    { number: 3, content_hash: "revision-3", summary: "Current revision", author: "tester", created_at: "2026-10-03T12:00:00Z" },
  ] }));
  api.handlers.set("/api/policy/records/test-policy/revisions/1", async (route) => route.fulfill({ json: { number: 1, content: "define FirstGroup as user.\n", content_hash: "revision-1", summary: "First revision" } }));
  api.handlers.set("/api/policy/records/test-policy/revisions/2", async (route) => route.fulfill({ json: { number: 2, content: "define SecondGroup as user.\n", content_hash: "revision-2", summary: "Second revision" } }));
  await page.goto(appURL + "/#policy");
  await page.evaluate(() => { window.confirmCalls = 0; window.confirm = () => { window.confirmCalls += 1; return false; }; });
  await page.locator("#policy-source").fill(dirtySource);
  await page.locator('[data-revision="1"]').click();
  await expect.poll(() => page.evaluate(() => window.confirmCalls)).toBe(1);
  await expect(page.locator("#policy-source")).toHaveValue(dirtySource);
  await page.locator("#policy-source").fill(savedSource);
  await page.locator('[data-revision="1"]').click();
  await expect(page.locator("#policy-source")).toHaveValue("define FirstGroup as user.\n");
  await page.locator('[data-revision="2"]').click();
  await expect(page.locator("#policy-source")).toHaveValue("define SecondGroup as user.\n");
  await expect.poll(() => page.evaluate(() => window.confirmCalls)).toBe(1);
});

test("policy Test shows compiler diagnostics as clickable line error markers", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/check", async (route) => route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } }));
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ json: {
    actors: [{ id: "alice", label: "Alice Rivera", kind: "user", dimensions: { user: "alice" }, attributes: [] }],
    services: [{ id: "EchoWeb", name: "EchoWeb", protocol: "TCP", port: 8080, attributes: [] }],
    warnings: [],
  } }));
  api.handlers.set("/api/policy/test", async (route) => route.fulfill({
    status: 422,
    json: { error: "Candidate policy compilation failed: error: [ line 2, column 1 ] explicit service targets are rejected" },
  }));
  await page.goto(appURL + "/#policy");
  await page.locator("#policy-check").click();
  const errorMarker = page.locator('#policy-test-gutter [data-line="2"] .policy-test-line-result[data-effect="error"]');
  await expect(errorMarker).toHaveText("ERR");
  await expect(page.locator("#policy-test-gutter")).toHaveCSS("width", "48px");
  await expect(page.locator("#policy-test-gutter")).toHaveCSS("border-right-style", "solid");
  await expect(page.locator("#policy-test-gutter")).toHaveCSS("width", "48px");
  await expect(page.locator("#policy-test-gutter")).toHaveCSS("border-right-style", "solid");
  await expect(page.locator("#policy-test-status")).toBeHidden();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  await page.mouse.move(0, 0);
  await expect(page.locator("#policy-check")).toHaveCSS("background-color", "rgb(184, 59, 59)");
  await errorMarker.click();
  const details = page.locator("#policy-test-details");
  await expect(details).toBeVisible();
  await expect(details.locator("#policy-analyze-title")).toBeHidden();
  await expect(details.locator("#policy-analyze-subject-title")).toBeHidden();
  await expect(details.locator(".policy-test-diagnostic")).toHaveText("Candidate policy compilation failed: error: [ line 2, column 1 ] explicit service targets are rejected");
  await expect(details.locator(".policy-test-diagnostic")).toHaveCSS("font-size", "15px");
});

for (const view of [
  { name: "Adapter Logs", path: "/#adapter-logs", names: ["Control adapter", "finance-client adapter"], columns: true },
  { name: "Workload logs", path: "/machine-logs.html", names: ["finance-client events", "echo-service events"] },
]) {
  test(`${view.name} supports independent radio source selection`, async ({ page, appURL, api }) => {
    await page.goto(appURL + view.path);
    if (view.columns) {
      const columns = page.locator(".adapter-log-column");
      await expect(columns).toHaveCount(1);
      const first = columns.nth(0);
      const firstPicker = first.getByRole("combobox", { name: /Select adapter/ });
      await expect(firstPicker.locator("option")).toHaveCount(4);
      await firstPicker.selectOption({ label: "finance-client adapter · machine-first" });
      await expect(first.locator(".machine-log-output")).toContainText("finance-client adapter entry 79");
      await page.getByRole("button", { name: "Add adapter panel" }).click();
      await expect(columns).toHaveCount(2);
      const second = columns.nth(1);
      await expect(second.getByRole("combobox", { name: /Select adapter/ })).toHaveValue("machine-first\u001fControl adapter");
      await firstPicker.selectOption({ label: "Control adapter · machine-second" });
      await expect(first.locator(".machine-log-output")).toContainText("Control adapter entry 79");
      await expect(second.locator(".machine-log-output")).toContainText("Control adapter entry 79");
      await expect(second.getByRole("combobox", { name: /Select adapter/ })).toHaveValue("machine-first\u001fControl adapter");
      await second.getByRole("button", { name: "Remove adapter panel" }).click();
      await expect(columns).toHaveCount(1);
      await first.getByRole("button", { name: /Maximize/ }).click();
      await expect(first).toHaveClass(/maximized/);
      await page.keyboard.press("Escape");
      await expect(first).not.toHaveClass(/maximized/);
    } else {
      const panels = page.locator(".machine-log-panel");
      await expect(panels).toHaveCount(2);
      const first = panels.nth(0);
      const second = panels.nth(1);
      await expect(first.getByRole("radio")).toHaveCount(view.names.length);
      await first.getByRole("radio", { name: view.names[1], exact: true }).check();
      await expect(first.locator(".machine-log-output")).toContainText(`${view.names[1]} entry 79`);
      await expect(first.locator(".machine-log-output")).not.toContainText(`${view.names[0]} entry`);
      await expect(second.getByRole("radio", { name: view.names[0], exact: true })).toBeChecked();
      await first.getByRole("button", { name: /Maximize/ }).click();
      await expect(first).toHaveClass(/maximized/);
      await page.keyboard.press("Escape");
      await expect(first).not.toHaveClass(/maximized/);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  });
}

test("Adapter Logs grows horizontally and removes only the selected column", async ({ page, appURL, api }) => {
  await page.setViewportSize({ width: 834, height: 1194 });
  await page.goto(appURL + "/#adapter-logs");
  const columns = page.locator(".adapter-log-column");
  await expect(columns).toHaveCount(1);
  const grid = page.locator("#machine-logs-grid");
  await page.getByRole("button", { name: "Add adapter panel" }).click();
  await page.getByRole("button", { name: "Add adapter panel" }).click();
  await expect(columns).toHaveCount(3);
  expect(await grid.evaluate((element) => element.scrollWidth > element.clientWidth)).toBeTruthy();
  await columns.nth(1).getByRole("button", { name: "Remove adapter panel" }).click();
  await expect(columns).toHaveCount(2);
  await columns.nth(0).getByRole("combobox", { name: /Select adapter/ }).selectOption({ label: "finance-client adapter · machine-first" });
  await expect(columns.nth(1).locator(".machine-log-output")).toContainText("Control adapter entry 79");
  await expect(columns.nth(0).locator(".machine-log-output")).toContainText("finance-client adapter entry 79");
});

test("Control Room top buttons switch adapter and controller logs using the same panels", async ({ page, appURL, api }, testInfo) => {
  await page.goto(appURL + "/#adapter-logs");
  const columns = page.locator(".adapter-log-column");
  await expect(columns).toHaveCount(1);
  await expect(page.locator("#machine-logs-follow")).toHaveCount(0);
  await expect(page.locator("#adapter-log-count")).toHaveCount(0);
  await expect(page.locator("#machine-logs-refresh")).toHaveCount(0);
  const toolbar = page.locator(".adapter-logs-toolbar");
  await expect(toolbar.locator("#machine-logs-pause")).toHaveAttribute("aria-pressed", "false");
  await expect(toolbar.locator("[data-adapter-log-type]")).toHaveCount(2);
  await expect(toolbar.locator("#adapter-log-add")).toHaveCount(1);
  const toolbarTops = await toolbar.evaluate((element) => [...element.children].map((child) => Math.round(child.getBoundingClientRect().top)));
  expect(Math.max(...toolbarTops) - Math.min(...toolbarTops)).toBeLessThanOrEqual(3);
  await expect(columns.locator("header h2")).toHaveCount(1);
  const panelHeader = columns.locator("header").first();
  const headerGeometry = await panelHeader.evaluate((header) => {
    const title = header.querySelector("h2").getBoundingClientRect();
    const actions = header.querySelector(".machine-log-panel-actions").getBoundingClientRect();
    const bounds = header.getBoundingClientRect();
    return { titleRight: title.right, actionsLeft: actions.left, actionsRight: actions.right, headerRight: bounds.right };
  });
  expect(headerGeometry.actionsLeft).toBeGreaterThan(headerGeometry.titleRight);
  expect(headerGeometry.headerRight - headerGeometry.actionsRight).toBeLessThanOrEqual(16);
  await page.locator("#machine-logs-pause").click();
  await expect(page.locator("#machine-logs-pause")).toHaveAttribute("aria-pressed", "true");
  const first = columns.nth(0);
  await first.getByRole("combobox", { name: /Select adapter/ }).selectOption({ label: "finance-client adapter · machine-first" });
  const output = first.locator(".machine-log-output");
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  await page.getByRole("button", { name: "Add adapter panel", exact: true }).click();
  await expect(columns).toHaveCount(2);
  const second = columns.nth(1);
  const originalPanel = await first.elementHandle();
  await page.getByRole("button", { name: "Controller logs", exact: true }).click();
  await expect(page.getByRole("button", { name: "Controller logs", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(first.getByRole("combobox", { name: /Select controller/ }).locator("option")).toHaveCount(2);
  await first.getByRole("combobox", { name: /Select controller/ }).selectOption({ label: "Controller · machine-second" });
  await expect(first.locator(".machine-log-output")).toContainText("Controller entry 79");
  await expect(second.getByRole("combobox", { name: /Select controller/ })).toHaveValue("machine-first\u001fController");
  await expect(columns).toHaveCount(2);
  expect(await originalPanel.evaluate((panel) => panel.isConnected)).toBeTruthy();
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  await page.getByRole("button", { name: "Adapter logs", exact: true }).click();
  await expect(first.getByRole("combobox", { name: /Select adapter/ })).toHaveValue("machine-first\u001ffinance-client adapter");
  await expect(first.locator(".machine-log-output")).toContainText("finance-client adapter entry 79");
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  await page.getByRole("button", { name: "Controller logs", exact: true }).click();
  await expect(first.getByRole("combobox", { name: /Select controller/ })).toHaveValue("machine-second\u001fController");
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  expect(api.counts.get("/api/adapter-logs")).toBe(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath("control-room-log-types.png"), fullPage: true });
});

async function refreshLogs(page) {
  const button = page.locator("#machine-logs-refresh");
  if (await button.count()) {
    await button.click();
    await expect(button).toBeEnabled();
    return;
  }
  const pause = page.locator("#machine-logs-pause");
  const wasPaused = await pause.getAttribute("aria-pressed") === "true";
  if (!wasPaused) await pause.click();
  const response = page.waitForResponse((item) => item.url().includes("/api/adapter-logs"));
  await pause.click();
  await response;
  if (wasPaused) await pause.click();
}

async function expectAtBottom(output) {
  await expect.poll(() => output.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop)).toBeLessThanOrEqual(8);
}

test("log sources remember reader positions and follow independently", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#adapter-logs");
  const panels = page.locator(".adapter-log-column");
  await expect(panels).toHaveCount(1);
  await page.locator("#machine-logs-pause").click();
  const first = panels.nth(0);
  const output = first.locator(".machine-log-output");
  await expectAtBottom(output);
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  api.adapterLogs = logFixture(["Controller", "Control adapter", "finance-client adapter"], 95);
  await refreshLogs(page);
  await expect(output).toContainText("Control adapter entry 94");
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  const picker = first.getByRole("combobox", { name: "Select adapter" });
  await picker.selectOption({ label: "finance-client adapter · machine-first" });
  await expectAtBottom(output);
  await picker.selectOption({ label: "Control adapter · machine-first" });
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  await output.evaluate((element) => { element.scrollTop = element.scrollHeight; element.dispatchEvent(new Event("scroll")); });
  api.adapterLogs = logFixture(["Controller", "Control adapter", "finance-client adapter"], 100);
  await refreshLogs(page);
  await expect(output).toContainText("Control adapter entry 99");
  await expectAtBottom(output);
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  await output.evaluate((element) => { element.scrollTop = element.scrollHeight; element.dispatchEvent(new Event("scroll")); });
  api.adapterLogs = logFixture(["Controller", "Control adapter", "finance-client adapter"], 105);
  await refreshLogs(page);
  await expectAtBottom(output);
});

test("log rendering preserves ANSI colors without interpreting HTML", async ({ page, appURL, api }) => {
  api.adapterLogs.machines[0].sources.find((source) => source.name === "Control adapter").lines = [
    "\u001b[31mred\u001b[0m plain",
    "\u001b[38;2;10;20;30mRGB\u001b[0m",
    "<img src=x onerror=window.logInjected=true>",
    "\u001b]8;;https://example.com\u0007link label\u001b]8;;\u0007",
  ];
  await page.goto(appURL + "/#adapter-logs");
  const output = page.locator(".machine-log-output").first();
  await expect(output).toContainText("red plain");
  await expect(output.locator("span").filter({ hasText: /^red$/ })).toHaveCSS("color", "rgb(187, 0, 0)");
  await expect(output.locator("span").filter({ hasText: /^RGB$/ })).toHaveCSS("color", "rgb(10, 20, 30)");
  await expect(output).toContainText("<img src=x onerror=window.logInjected=true>");
  await expect(output).toContainText("link label");
  await expect(output.locator("img, a")).toHaveCount(0);
  expect(await page.evaluate(() => Boolean(window.logInjected))).toBeFalsy();
});

test("log errors, empty workloads and removed sources are explicit", async ({ page, appURL, api }) => {
  api.workloadLogs.machines[1].sources = [];
  await page.goto(appURL + "/machine-logs.html");
  const panels = page.locator(".machine-log-panel");
  await expect(panels).toHaveCount(2);
  await page.locator("#machine-logs-pause").click();
  await expect(panels.nth(1)).toContainText("No application/service logs assigned.");
  const first = panels.nth(0);
  await first.getByRole("radio", { name: "echo-service events", exact: true }).check();
  api.workloadLogs.machines[0].sources[1] = { name: "echo-service events", lines: [], error: "Log source unavailable" };
  await refreshLogs(page);
  await expect(first.locator(".machine-log-error")).toHaveText("Log source unavailable");
  api.workloadLogs.machines[0].sources.splice(1, 1);
  await refreshLogs(page);
  await expect(first.getByRole("radio", { name: "echo-service events", exact: true })).toBeChecked();
  await expect(first.getByRole("radio")).toHaveCount(2);
  await expect(first.locator(".machine-log-output")).toContainText("echo-service events entry 79");
  await expect(first).not.toHaveClass(/running/);
  api.statuses.set("/api/simulator/machine-logs", 503);
  await refreshLogs(page);
  await expect(page.locator("#machine-logs-error")).toBeVisible();
  await expect(first.locator(".machine-log-output")).toContainText("echo-service events entry 79");
});

for (const view of [
  { name: "adapter", path: "/#adapter-logs", data: "adapterLogs", source: "Control adapter", columns: true },
  { name: "controller", path: "/#adapter-logs", data: "adapterLogs", source: "Controller", columns: true },
  { name: "application/service", path: "/machine-logs.html", data: "workloadLogs", source: "finance-client events", columns: false },
]) {
  test(`${view.name} log panel retains its tail and turns white when disconnected`, async ({ page, appURL, api }) => {
    await page.goto(appURL + view.path);
    const panels = page.locator(view.columns ? ".adapter-log-column" : ".machine-log-panel");
    await expect(panels.first()).toBeVisible();
    await page.locator("#machine-logs-pause").click();
    if (view.name === "controller") await page.getByRole("button", { name: "Controller logs", exact: true }).click();
    const first = panels.first();
    const output = first.locator(".machine-log-output");
    await expect(output).toContainText(`${view.source} entry 79`);
    const tail = await output.locator("pre").textContent();
    await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
    const source = api[view.data].machines[0].sources.find((source) => source.name === view.source);
    api[view.data].machines[0].state = "stopped";
    api[view.data].machines[0].sources = [];
    await refreshLogs(page);
    await expect(output.locator("pre")).toHaveText(tail);
    await expect(output).toContainText("Disconnected");
    await expect(first).not.toHaveClass(/running/);
    await expect(first).toHaveCSS("background-color", "rgb(255, 255, 255)");
    expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
    api[view.data].machines[0].state = "running";
    api[view.data].machines[0].sources = [{ ...source, lines: ["Reconnected with a new tail"] }];
    await refreshLogs(page);
    await expect(output.locator("pre")).toHaveText("Reconnected with a new tail");
    await expect(first).toHaveClass(/running/);
    await expect(output).toHaveCSS("background-color", "rgb(0, 0, 0)");
  });
}

test("log tail retention is cleared when the organization changes", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#adapter-logs");
  const first = page.locator(".adapter-log-column").first();
  await expect(first.locator(".machine-log-output")).toContainText("Control adapter entry 79");
  await page.locator("#machine-logs-pause").click();
  api.adapterLogs.organization_id = "different-organization";
  api.adapterLogs.machines[0].state = "stopped";
  api.adapterLogs.machines[0].sources = [];
  api.adapterLogs.machines[1].sources = [];
  await refreshLogs(page);
  await expect(first.locator(".machine-log-output")).not.toContainText("Control adapter entry 79");
  await expect(first.locator(".machine-log-output")).toContainText("No adapter logs available");
});

test("Adapter Logs polls only while its Control Room page is active", async ({ page, appURL, api }) => {
  await page.clock.install();
  await page.goto(appURL + "/#services");
  await expect(page.locator("#page-services")).toBeVisible();
  expect(api.counts.get("/api/adapter-logs") || 0).toBe(0);
  await page.getByRole("link", { name: "Adapter Logs", exact: true }).click();
  await expect(page.locator("#machine-logs-status")).toHaveText("Live");
  expect(api.counts.get("/api/simulator/adapter-logs") || 0).toBe(0);
  await page.getByRole("button", { name: "Add adapter panel" }).click();
  await expect(page.locator(".adapter-log-column")).toHaveCount(2);
  await page.clock.runFor(2100);
  await expect.poll(() => api.counts.get("/api/adapter-logs")).toBeGreaterThanOrEqual(2);
  await page.getByRole("link", { name: "Services", exact: true }).click();
  await expect(page.locator("#page-services")).toBeVisible();
  await expect(page.locator("#page-adapter-logs")).toBeHidden();
  const before = api.counts.get("/api/adapter-logs");
  await page.clock.runFor(6000);
  expect(api.counts.get("/api/adapter-logs")).toBe(before);
  await page.getByRole("link", { name: "Adapter Logs", exact: true }).click();
  await expect.poll(() => api.counts.get("/api/adapter-logs")).toBeGreaterThan(before);
});

test("policy editor has no attribute picker and retains attribute completions", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const editor = page.locator("#policy-source");
  await expect(editor).toHaveValue("define Employee as user.\n");
  await expect(page.locator("#policy-attribute-picker, #policy-attribute-insert")).toHaveCount(0);
  await expect(page.locator("#policy-format")).toBeVisible();
  await expect(page.locator("#policy-check")).toBeVisible();
  await expect(page.locator("#policy-attribute-rescan")).toBeVisible();
  await editor.fill("define Employee as user with user.");
  await expect(page.locator("#policy-completions")).toContainText("user.department:");
});

test("policy completions respect statements, define attributes and punctuation", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const editor = page.locator("#policy-source");
  const menu = page.locator("#policy-completions");
  await expect(editor).toBeEnabled();
  await editor.fill("define Employee as user.");
  await editor.press("End");
  await editor.click();
  await expect(menu).toBeHidden();
  await editor.fill("# define Employee as user with ");
  await expect(menu).toBeHidden();
  await editor.fill('define Employee as user with user.title:"unfinished');
  await expect(menu).toBeHidden();
  await editor.fill("define Employee as user\n  with dep");
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("option").filter({ hasText: "user.department:" })).toBeVisible();
  await expect(menu).not.toContainText("device.secure");
  await editor.press("Tab");
  await expect(editor).toHaveValue("define Employee as user\n  with user.department:");
  await editor.fill("define Employee as user with user.");
  await expect(menu).toBeVisible();
  await editor.press("Escape");
  await expect(menu).toBeHidden();
  await editor.fill("define Employee as u.");
  await editor.press("ArrowLeft");
  await expect(menu.getByRole("option").filter({ hasText: /^user$/ })).toBeVisible();
  await editor.press("Tab");
  await expect(editor).toHaveValue("define Employee as user.");
  await editor.fill('define SignalService as service.\nallow users and signal "hello" to ');
  await expect(menu.getByRole("option", { name: "SignalService", exact: true })).toBeVisible();
  await editor.fill("define Employee as user.\nallow Employee to ");
  await expect(menu.getByRole("option", { name: "access", exact: true })).toHaveCount(0);
  await editor.fill("define Mouse as user.\nallow Mi");
  await expect(menu.getByRole("option", { name: "Mice", exact: true })).toBeVisible();
});

test("policy compiler errors highlight their source token", async ({ page, appURL, api }) => {
  let diagnostics = "error: unexpected tab char at line 2, column 1";
  api.handlers.set("/api/policy/check", async (route) => {
    await route.fulfill({ json: { valid: false, diagnostics } });
  });
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const editor = page.locator("#policy-source");
  await expect(editor).toHaveValue("define Employee as user.\n");
  await editor.fill("define Employee as user.\n\tallow Employee.");
  await expect(editor).toHaveValue("define Employee as user.\n\tallow Employee.");
  await expect(page.locator("#policy-highlight")).toContainText("allow");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-highlight .zpl-error")).toHaveText("allow");

  diagnostics = "error: [ line 2, column 1 ] explicit service targets are rejected";
  await editor.fill("define Employee as user.\nallow Employee to access Payroll.");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-highlight .zpl-error")).toHaveText("allow");
});

test("Analyze warnings belong only to the selected policy source", async ({ page, appURL, api }) => {
  api.policy.records[0].content = "define Operators as user.\nallow Operators to access Payroll.";
  const milwaukee = { ...api.policy.records[0], id: "milwaukee-boundaries", name: "Milwaukee department boundaries", content: "define MilwaukeeSupport as device with greatlakes.department:'Customer Support'.\ndefine MilwaukeeFinance as device with greatlakes.department:Finance." };
  api.policy.records.push(milwaukee);
  api.handlers.set("/api/policy/records/milwaukee-boundaries", async (route) => route.fulfill({ json: milwaukee }));
  api.handlers.set("/api/policy/records/milwaukee-boundaries/revisions", async (route) => route.fulfill({ json: [] }));
  api.handlers.set("/api/policy/check", async (route) => {
    const { source } = route.request().postDataJSON();
    await route.fulfill({ json: source === milwaukee.content ? {
      valid: true, diagnostics: "warning: no policy granting admin access to VisaService",
      warnings: [
        { code: "COMPILER_CONTEXT", severity: "warning", line: 0, message: "No source location." },
        { code: "FOREIGN_WARNING", severity: "warning", line: 200, message: "Outside this policy source." },
      ],
    } : { valid: true, diagnostics: "Compiled successfully.", warnings: [{ code: "POLICY_BROAD_GRANT", severity: "warning", line: 2, message: "Review the Payroll grant." }] } });
  });
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ status: 503, json: { error: "Fixtures unavailable" } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.locator("#policy-check").click();
  await expect(page.locator('#policy-test-gutter [data-line="2"] [data-has-warnings="true"]')).toHaveCount(1);
  await openPolicyPicker(page);
  await page.locator('[data-record-id="milwaukee-boundaries"]').click();
  await expect(page.locator("#policy-record-title")).toHaveText(milwaukee.name);
  await expect(page.locator('#policy-test-gutter [data-has-warnings="true"]')).toHaveCount(0);
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-test-status")).toContainText("Fixtures unavailable");
  await expect(page.locator('#policy-test-gutter [data-has-warnings="true"]')).toHaveCount(0);
});

test("policy warnings remain in the gutter when fixtures fail", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/check", async (route) => route.fulfill({ json: {
    valid: true, diagnostics: "Compiled successfully.",
    warnings: [{ code: "POLICY_SPECIFIC_IDENTITY", severity: "warning", line: 2, message: "Prefer groups instead of individual devices or users." }],
  } }));
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ status: 503, json: {
    error: "A directory attribute cannot be represented safely in the policy test request.",
  } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.locator("#policy-source").fill("define Operators as user.\nallow Operators to access Payroll.");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator("#policy-lint-warnings")).toHaveCount(0);
  await expect(page.locator("#policy-test-status")).toContainText("Analysis unavailable");
  await expect(page.locator('#policy-test-gutter [data-line="1"] [data-effect="error"]')).toHaveCount(0);
  const warning = page.locator('#policy-test-gutter [data-line="2"] [data-effect="warning"]');
  await expect(warning).toHaveText("WARN");
  await warning.click();
  await expect(page.getByRole("dialog", { name: "Analyze warning details" })).toContainText("POLICY_SPECIFIC_IDENTITY");
  await page.locator("#policy-test-details .dialog-actions .button").click();
  await page.locator("#policy-source").fill("define Operators as user.");
  await expect(page.locator('#policy-test-gutter [data-effect="warning"]')).toHaveCount(0);
});

test("policy gutter combines matching results and lint warning details", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/check", async (route) => route.fulfill({ json: {
    valid: true, diagnostics: "Compiled successfully.",
    warnings: [{ code: "POLICY_BROAD_GRANT", severity: "warning", line: 2, message: "Review the grant scope." }],
  } }));
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ json: { actors: [], services: [] } }));
  api.handlers.set("/api/policy/test", async (route) => route.fulfill({ json: {
    actor_count: 0,
    services: [{ id: "payroll", name: "Payroll", supported: true, rules: [{ line: 2, effect: "allow", matched: { count: 0, subjects: [] } }] }],
    warnings: [{ code: "POLICY_NO_HITS", severity: "warning", line: 2, message: "No hits in this population." }],
  } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.locator("#policy-source").fill("define Operators as user.\nallow Operators to access Payroll.");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "success");
  const marker = page.locator('#policy-test-gutter [data-line="2"] button');
  await expect(marker).toHaveCount(1);
  await expect(marker).toHaveText("None");
  await expect(marker).toHaveAttribute("data-has-warnings", "true");
  await marker.click();
  await expect(page.locator("#policy-test-details")).toContainText("No matching identities.");
  await expect(page.locator("#policy-test-details")).toContainText("POLICY_BROAD_GRANT");
  await expect(page.locator("#policy-test-details")).toContainText("POLICY_NO_HITS");
  await expect(page.locator("#policy-lint-warnings")).toHaveCount(0);
});

test("policy lint warns about specific identities without failing Analyze", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/check", async (route) => route.fulfill({ json: {
    valid: true, diagnostics: "Compiled successfully.",
    warnings: [{ code: "POLICY_SPECIFIC_IDENTITY", severity: "warning", line: 1, message: "Prefer groups instead of individual devices or users." }],
  } }));
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ json: { actors: [], services: [] } }));
  api.handlers.set("/api/policy/test", async (route) => route.fulfill({ json: {
    actor_count: 0, services: [], warnings: [{ code: "POLICY_NO_HITS", severity: "warning", line: 2, message: "No hits in this test population." }],
  } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "success");
  await expect(page.locator('#policy-test-gutter [data-line="1"] [data-effect="warning"]')).toHaveAttribute("data-has-warnings", "true");
  await expect(page.locator('#policy-test-gutter [data-line="2"] [data-effect="warning"]')).toHaveAttribute("data-has-warnings", "true");
  await expect(page.locator("#policy-lint-warnings")).toHaveCount(0);
  await page.locator("#policy-source").fill("define Operators as user with user.role:Operator.");
  await expect(page.locator('#policy-test-gutter [data-effect="warning"]')).toHaveCount(0);
});

for (const reducedMotion of ["no-preference", "reduce"]) {
test(`map pulses only the requesting adapter for new grants and denials (${reducedMotion})`, async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion });
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::ff", node_details: { adapters: ["requester", "target"], in_sync: true } },
    { cn: "requester", node: false, zpr_addr: "fd00::1" },
    { cn: "target", node: false, zpr_addr: "fd00::2" },
  ];
  api.snapshot.recent_visas = [{ id: 1, source_addr: "fd00::1", dest_addr: "fd00::2" }];
  api.snapshot.services = [
    { service_name: "API", service_kind: "Application", actor_cn: "target", zpr_addr: "fd00::2", service_endpoints: "TCP/443" },
    { service_name: "OtherAPI", service_kind: "Application", actor_cn: "target", zpr_addr: "fd00::2", service_endpoints: "TCP/444" },
  ];
  await page.goto(appURL + "/#map");
  await expect(page.locator(".graph-vertex.adapter")).toHaveCount(2);
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const requester = page.locator('.graph-vertex[data-inspect-actor="requester"]');
  const target = page.locator('.graph-vertex[data-inspect-actor="target"]');
  const wire = page.locator('.graph-edge[data-dock-adapter="requester"] .graph-link');
  const targetWire = page.locator('.graph-edge[data-dock-adapter="target"] .graph-link');
  await expect(page.locator(".graph-decision-ring")).toHaveCount(0);
  api.snapshot.recent_visas.push(
    { id: 2, source_addr: "fd00:0:0:0:0:0:0:1", dest_addr: "fd00::2", dest_port: 443, proto: "TCP", direction: "forward" },
    { id: 3, source_addr: "fd00::2", dest_addr: "fd00::1", source_port: 443, proto: "TCP", direction: "reverse" },
  );
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(requester.locator('.graph-decision-ring[data-decision="grant"]')).toHaveCount(1);
  await page.evaluate(() => render(state.snapshot));
  await expect(requester.locator('.graph-decision-ring[data-decision="grant"]')).toHaveCount(1);
  const service = page.locator('.graph-service-badge[data-inspect-service="API"]');
  await expect(service.locator(".graph-service-decision-ring")).toHaveCount(1);
  await expect(service.locator(".graph-service-decision-ring")).toHaveCSS("stroke", "rgb(24, 137, 75)");
  const servicePulseDuration = await service.locator(".graph-service-decision-ring").evaluate((ring) => ring.getAnimations()[0].effect.getTiming().duration);
  expect(servicePulseDuration).toBe(2000);
  for (const outline of [service.locator(".graph-service-decision-ring"), requester.locator(".graph-decision-ring")]) {
    const transforms = await outline.evaluate((element) => element.getAnimations().flatMap((animation) => animation.effect.getKeyframes().map((frame) => frame.transform).filter(Boolean)));
    if (reducedMotion === "reduce") expect(transforms).toEqual([]);
    else expect(transforms).toEqual(["scale(1)", "scale(1.25)", "scale(1)"]);
  }
  await expect(page.locator('.graph-service-badge[data-inspect-service="OtherAPI"] .graph-service-decision-ring')).toHaveCount(0);
  await expect(requester.locator('.graph-decision-ring')).toHaveCSS("stroke", "rgb(24, 137, 75)");
  await expect(wire).toHaveAttribute("data-decision", "grant");
  await expect(targetWire).not.toHaveAttribute("data-decision");
  if (reducedMotion === "reduce") await expect(requester.locator('.graph-adapter')).not.toHaveClass(/graph-decision-glyph/);
  else await expect(requester.locator('.graph-adapter')).toHaveClass(/graph-decision-glyph/);
  await expect(target.locator(".graph-decision-ring")).toHaveCount(0);
  await expect(page.locator(".graph-decision-ring")).toHaveCount(0);
  await expect(page.locator(".graph-service-decision-ring")).toHaveCount(0);
  await expect(wire).not.toHaveAttribute("data-decision");
  await expect(requester.locator('.graph-adapter')).not.toHaveClass(/graph-decision-glyph/);
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.locator(".graph-decision-ring")).toHaveCount(0);
  api.snapshot.recent_denies = [{ source_addr: "fd00::1", dest_addr: "fd00::2", protocol: 6, dest_port: 443, count: 1, last_deny_ms: Date.now() }];
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(requester.locator('.graph-decision-ring[data-decision="deny"]')).toHaveCount(1);
  await expect(requester.locator('.graph-decision-ring')).toHaveCSS("stroke", "rgb(208, 50, 50)");
  await expect(wire).toHaveAttribute("data-decision", "deny");
  await expect(target.locator(".graph-decision-ring")).toHaveCount(0);
  await expect(page.locator(".graph-decision-ring")).toHaveCount(0);
  api.snapshot.recent_denies[0].count = 2;
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(requester.locator('.graph-decision-ring[data-decision="deny"]')).toHaveCount(1);
  await expect(page.locator(".graph-decision-ring")).toHaveCount(0);
  api.snapshot.recent_denies = [];
  api.snapshot.recent_visas.push({ id: 4, source_addr: "fd00::2", dest_addr: "fd00::1", source_port: 443, proto: "TCP", direction: "reverse" });
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(requester.locator('.graph-decision-ring[data-decision="grant"]')).toHaveCount(1);
  await expect(target.locator(".graph-decision-ring")).toHaveCount(0);
  await expect(wire).toHaveAttribute("data-decision", "grant");
  await expect(service.locator(".graph-service-decision-ring")).toHaveCount(1);
});
}

test("service types share table and map colors and gateways have clouds", async ({ page, appURL, api }) => {
  const kinds = ["BuiltIn", "Regular", "Visa", "Gateway", "ZPR", "Policy", "Control", "Auth", "Attribute", "Application", "Node", "Logger", 'Trusted("file")', 'Trusted("rest/1")', null, 'Trusted("custom")'];
  api.snapshot.actors = [{ cn: "adapter", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.services = kinds.map((kind, index) => ({ service_name: `service-${index}`, service_kind: kind, actor_cn: "adapter", zpr_addr: "fd00::1", service_endpoints: "tcp:8080" }));
  await page.goto(appURL + "/#services");
  const chips = page.locator(".service-type-chip");
  await expect(chips).toHaveCount(kinds.length);
  await expect(page.locator('.service-type-chip[data-service-type="ZPR"]')).toHaveText("ZPR service");
  const colors = await chips.evaluateAll((elements) => Object.fromEntries(elements.map((element) => [element.dataset.serviceType, {
    fill: getComputedStyle(element).backgroundColor,
    stroke: getComputedStyle(element).borderColor,
    text: getComputedStyle(element).color,
  }])));
  expect(new Set(Object.values(colors).map((color) => color.fill)).size).toBe(kinds.length);
  await page.getByText("Type colors", { exact: true }).click();
  await expect(page.locator(".service-type-legend-item")).toHaveCount(kinds.length);
  for (const label of ["ZPR service", "Policy service", "Control service", "Authentication service", "Attribute service", "Application service", "Node", "Logger"]) {
    await expect(page.locator(".service-type-legend")).toContainText(label);
  }
  await page.keyboard.press("Escape");
  await expect(page.locator(".service-type-key")).not.toHaveAttribute("open", "");
  await page.getByRole("link", { name: "Map", exact: true }).click();
  await expect(page.locator("#graph-animation-toggle, .graph-hint")).toHaveCount(0);
  const badges = page.locator(".graph-service-badge");
  await expect(badges).toHaveCount(kinds.length);
  const readMapColors = () => badges.evaluateAll((elements) => Object.fromEntries(elements.map((element) => [element.dataset.serviceType, {
    fill: getComputedStyle(element.querySelector("rect")).fill,
    stroke: getComputedStyle(element.querySelector("rect")).stroke,
    text: getComputedStyle(element.querySelector("text")).fill,
  }])));
  expect(await readMapColors()).toEqual(colors);
  await badges.first().focus();
  await badges.first().hover();
  expect(await readMapColors()).toEqual(colors);
  await badges.evaluateAll((elements) => elements.forEach((element) => element.classList.add("arriving")));
  expect(await readMapColors()).toEqual(colors);
  await badges.evaluateAll((elements) => elements.forEach((element) => element.classList.replace("arriving", "graph-exiting")));
  expect(await readMapColors()).toEqual(colors);
  await expect(page.locator(".graph-cloud")).toHaveCount(1);
  await expect(page.locator(".graph-cloud")).toHaveCSS("fill", "rgb(69, 69, 69)");
  await expect(page.locator(".gateway-cloud-link")).toHaveCount(1);
});

test("adding topology parents keeps existing nodes at the same screen position", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  const parent = (index) => ({ cn: `node-${index}`, node: true, zpr_addr: `fd00::${index + 1}`, node_details: { adapters: [], in_sync: true } });
  api.snapshot.actors = [0, 1, 2, 3].map(parent);
  await page.goto(appURL + "/#map");
  const existing = page.locator('.graph-vertex.node[data-inspect-actor="node-0"]');
  await expect(existing).toBeVisible();
  const before = await existing.boundingBox();

  api.snapshot.actors.push(parent(4));
  await page.locator("#refresh-now").click();
  await expect(page.locator('.graph-vertex.node[data-inspect-actor="node-4"]')).toBeVisible();
  const graph = page.locator(".topology-graph");
  expect(await graph.evaluate((svg) => svg.getAnimations().some((candidate) => candidate.effect?.target === svg))).toBeTruthy();
  await graph.evaluate((svg) => { const candidate = svg.getAnimations().find((animation) => animation.effect?.target === svg); candidate.pause(); candidate.currentTime = 0; });
  const atStart = await page.locator('.graph-vertex.node[data-inspect-actor="node-0"]').boundingBox();
  expect(atStart.x).toBeCloseTo(before.x, 1);
  expect(atStart.y).toBeCloseTo(before.y, 1);
  await graph.evaluate((svg) => { svg.getAnimations().find((animation) => animation.effect?.target === svg).currentTime = 350; });
  const midway = await page.locator('.graph-vertex.node[data-inspect-actor="node-0"]').boundingBox();
  await graph.evaluate((svg) => { svg.getAnimations().find((animation) => animation.effect?.target === svg).finish(); });
  const afterNode = page.locator('.graph-vertex.node[data-inspect-actor="node-0"]');
  const after = await afterNode.boundingBox();
  const midwayDistance = Math.hypot(midway.x - before.x, midway.y - before.y);
  const finalDistance = Math.hypot(after.x - before.x, after.y - before.y);
  expect(midwayDistance).toBeGreaterThan(1);
  expect(midwayDistance).toBeLessThan(finalDistance);
  expect(finalDistance).toBeGreaterThan(1);
});

test("retained adapters animate when topology bounds move", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  api.snapshot.actors = [
    { cn: "node-0", node: true, zpr_addr: "fd00::1", node_details: { adapters: ["adapter-0"], in_sync: true } },
    { cn: "adapter-0", node: false, zpr_addr: "fd00::2" },
  ];
  await page.goto(appURL + "/#map");
  const retained = page.locator('.graph-vertex[data-inspect-actor="adapter-0"]');
  await expect(retained).toBeVisible();
  const beforePosition = await retained.evaluate((element) => [element.dataset.originX, element.dataset.originY]);
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();

  api.snapshot.actors = [
    { cn: "node-0", node: true, zpr_addr: "fd00::1", node_details: { adapters: ["adapter-0", "gateway-0"], in_sync: true } },
    { cn: "adapter-0", node: false, zpr_addr: "fd00::2" },
    { cn: "gateway-0", node: false, zpr_addr: "fd00::3" },
  ];
  api.snapshot.services = [{ service_name: "Public gateway", service_kind: "Gateway", actor_cn: "gateway-0", external_network_connection: "public-internet" }];
  await page.locator("#refresh-now").click();
  await expect(page.locator('.graph-vertex[data-inspect-actor="gateway-0"]')).toBeVisible();
  const afterPosition = await retained.evaluate((element) => [element.dataset.originX, element.dataset.originY]);
  expect(afterPosition).not.toEqual(beforePosition);
  const motion = await retained.evaluate((element) => element.graphMotion && ({ from: element.graphMotion.from, to: element.graphMotion.to }));
  expect(motion).not.toBeNull();
  expect(Math.hypot(motion.from.x - motion.to.x, motion.from.y - motion.to.y)).toBeGreaterThan(1);
  const dockLink = page.locator('.graph-edge[data-topology-edge="dock|node-0|adapter-0"]');
  const linkMotion = await dockLink.evaluate((element) => element.graphLinkMotion);
  expect(linkMotion).toBeTruthy();
  expect(Math.hypot(linkMotion.from.x1 - linkMotion.to.x1, linkMotion.from.y1 - linkMotion.to.y1)).toBeGreaterThan(1);
});

test("Fit centers the actual topology bounds", async ({ page, appURL, api }) => {
  api.snapshot.actors = [{ cn: "adapter", node: false, zpr_addr: "fd00::1" }];
  await page.goto(appURL + "/#map");
  await page.locator('[data-graph-action="fit"]').click();
  const geometry = await page.evaluate(() => {
    const stage = document.querySelector("#topology-stage").getBoundingClientRect();
    const bounds = document.querySelector("#graph-world").getBBox();
    const matrix = document.querySelector("#graph-world").getScreenCTM();
    const corners = [[bounds.x, bounds.y], [bounds.x + bounds.width, bounds.y + bounds.height]].map(([x, y]) => {
      const point = new DOMPoint(x, y).matrixTransform(matrix);
      return [point.x, point.y];
    });
    const [left, top] = corners[0];
    const [right, bottom] = corners[1];
    return {
      left, top, right, bottom,
      centerX: stage.left + stage.width / 2,
      centerY: stage.top + stage.height / 2,
      stageLeft: stage.left,
      stageTop: stage.top,
      stageRight: stage.right,
      stageBottom: stage.bottom,
    };
  });
  expect(Math.abs((geometry.left + geometry.right) / 2 - geometry.centerX)).toBeLessThan(1);
  expect(Math.abs((geometry.top + geometry.bottom) / 2 - geometry.centerY)).toBeLessThan(1);
  expect(geometry.left).toBeGreaterThan(geometry.stageLeft);
  expect(geometry.top).toBeGreaterThan(geometry.stageTop);
  expect(geometry.right).toBeLessThan(geometry.stageRight);
  expect(geometry.bottom).toBeLessThan(geometry.stageBottom);
});

test("Fit keeps five percent viewBox padding without stretching Map glyphs", async ({ page, appURL, api }) => {
  await page.setViewportSize({ width: 887, height: 394 });
  const adapters = Array.from({ length: 8 }, (_, index) => `adapter-${index}`);
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::1", node_details: { adapters, in_sync: true } },
    ...adapters.map((cn, index) => ({ cn, node: false, zpr_addr: `fd00::${index + 2}` })),
  ];
  api.snapshot.services = [{ service_name: "Web", service_kind: "Regular", actor_cn: "adapter-0", service_endpoints: "TCP/8080", zpr_addr: "fd00::2" }];
  await page.goto(appURL + "/#map");
  await page.locator('[data-graph-action="fit"]').click();
  const margins = await page.evaluate(() => {
    const svg = document.querySelector(".topology-graph");
    const world = document.querySelector("#graph-world");
    const bounds = world.getBBox();
    const viewBox = svg.viewBox.baseVal;
    const adapter = document.querySelector(".graph-adapter").getBoundingClientRect();
    return {
      margins: [
        (bounds.x - viewBox.x) / viewBox.width,
        (bounds.y - viewBox.y) / viewBox.height,
        (viewBox.x + viewBox.width - bounds.x - bounds.width) / viewBox.width,
        (viewBox.y + viewBox.height - bounds.y - bounds.height) / viewBox.height,
      ],
      adapterAspect: adapter.width / adapter.height,
    };
  });
  for (const margin of margins.margins) expect(Math.abs(margin - 0.05)).toBeLessThan(0.001);
  expect(Math.abs(margins.adapterAspect - 1)).toBeLessThan(0.01);
});

test("Map canvas leaves no large empty footer below the panel", async ({ page, appURL }) => {
  await page.setViewportSize({ width: 887, height: 394 });
  await page.goto(appURL + "/#map");
  const gap = await page.evaluate(() => window.innerHeight - document.querySelector("#topology-stage").getBoundingClientRect().bottom);
  expect(Math.abs(gap)).toBeLessThan(18);
});

test("visa refresh preserves current grants and resolves DNS labels", async ({ page, appURL, api }) => {
  await page.clock.install();
  api.snapshot.actors = [{ cn: "adapter", node: false, zpr_addr: "fd00::1" }];
  api.dnsRecords.records = [{ name: "api.svc.zpr.", type: "AAAA", value: "fd00:0:0:0:0:0:0:2" }];
  await page.goto(appURL + "/#actors");
  await page.locator('#actor-rows [data-inspect-actor="adapter"]').click();
  const inspector = page.locator("#inspector-body");
  await expect(inspector).toContainText("Visa 41");
  await expect(inspector).toContainText("api.svc.zpr");
  await expect(inspector.locator('[title*="fd00::2"]')).not.toHaveCount(0);
  const existingRow = await inspector.locator(".detail-item").first().elementHandle();
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  api.handlers.set("/api/actors/adapter/visas", async (route) => {
    await gate;
    await route.fulfill({ status: 503, json: { error: "Unavailable" } });
  });
  const before = api.counts.get("/api/actors/adapter/visas");
  try {
    await page.clock.runFor(5100);
    await expect.poll(() => api.counts.get("/api/actors/adapter/visas")).toBeGreaterThan(before);
    await expect(inspector).toContainText("Visa 41");
    await expect(inspector).not.toContainText("Loading current visas");
    expect(await existingRow.evaluate((element) => element.isConnected)).toBeTruthy();
  } finally {
    release();
  }
  await expect(inspector).toContainText("showing last successful result");
  await expect(inspector).toContainText("Visa 41");
});

test("LDAP nodes show popup attributes and preserve memberships across collapse", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/__tests/ldap.html");
  await page.evaluate(() => {
    const graph = document.querySelector("ldap-org-graph");
    graph.directory = {
      base_dn: "dc=example,dc=test",
      departments: [{ name: "Engineering" }, { name: "Platform", parent: "Engineering" }],
      people: [{ uid: "ada", name: "Ada", department: "Platform", attributes: { clearance: "high" } }],
      groups: [{ name: "Operators", members: ["ada"] }],
    };
  });
  const graph = page.locator("ldap-org-graph");
  const person = graph.getByRole("button", { name: "person: Ada", exact: true });
  await expect(graph.locator("dialog")).not.toBeVisible();
  await person.click();
  const dialog = graph.getByRole("dialog", { name: "Ada component info", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Operators");
  await expect(dialog).toContainText("attributes.clearance");
  await expect(dialog).toContainText("high");
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await graph.getByRole("button", { name: "Collapse Engineering", exact: true }).click();
  await expect(person).toHaveCount(0);
  await expect(graph.locator("dialog")).not.toBeVisible();
  await graph.getByRole("button", { name: "Expand Engineering", exact: true }).click();
  await expect(person).toBeVisible();
  await person.click();
  await expect(dialog).toContainText("Operators");
  await graph.getByRole("button", { name: "Close component info", exact: true }).click();
  await expect(person).toBeFocused();
});

test("organization activation requires explicit approval and is cancel-safe", async ({ page, appURL, api }) => {
  const activationPath = "/api/simulator/organizations/beta/activate";
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  api.handlers.set(activationPath, async (route) => {
    expect(route.request().method()).toBe("POST");
    await gate;
    api.organizations.active_id = "beta";
    api.organizations.activation = { state: "completed", organization_id: "beta" };
    await route.fulfill({ status: 202, json: { activation: api.organizations.activation } });
  });
  api.handlers.set("/api/simulator/scenarios", async (route) => {
    const organizationID = new URL(route.request().url()).searchParams.get("organization_id") || "alpha";
    const organization = api.organizations.organizations.find((entry) => entry.id === organizationID);
    await route.fulfill({ json: { organization, active_organization_id: "alpha", scenarios: [{ id: "beta-scenario", name: "Beta scenario", organization_id: organizationID }] } });
  });
  await page.goto(appURL + "/organizations.html");
  await expect(page.getByText("Organization directory", { exact: true })).toBeVisible();
  await expect(page.locator("#organization-connection")).not.toContainText("profiles");
  await expect(page.getByRole("heading", { name: "Organizations", exact: true })).toHaveCount(0);
  await expect(page.locator("#organization-count")).toHaveCount(0);
  await page.locator('[data-organization-id="beta"]').click();
  await expect(page.locator("#organization-detail")).toContainText("multi-node · 2 nodes · North Hub / Regional Yard");
  const activate = page.locator('[data-activate-organization="beta"]');
  const activationPlacement = await page.evaluate(() => {
    const title = document.getElementById("organization-title").getBoundingClientRect();
    const control = document.querySelector("[data-organization-activation-control]").getBoundingClientRect();
    const description = document.querySelector("#organization-detail > .organization-summary").getBoundingClientRect();
    return { adjacent: control.left >= title.right && control.top < title.bottom, descriptionBelow: description.top >= control.bottom };
  });
  expect(activationPlacement.adjacent).toBeTruthy();
  expect(activationPlacement.descriptionBelow).toBeTruthy();
  await expect(page.locator("#organization-scenario-summary a").first()).toBeVisible();
  await activate.click();
  const dialog = page.getByRole("dialog", { name: "Switch organization?", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Switch from Alpha Labs to Beta Labs?");
  await expect(dialog).toContainText("resets the simulated ZPR environment");
  await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
  expect(api.counts.get(activationPath) || 0).toBe(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator("#organization-active-name")).toHaveText("Alpha Labs");
  expect(api.counts.get(activationPath) || 0).toBe(0);
  await activate.click();
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  expect(api.counts.get(activationPath) || 0).toBe(0);
  await activate.click();
  try {
    await dialog.getByRole("button", { name: "Switch organization", exact: true }).click();
    await expect.poll(() => api.counts.get(activationPath)).toBe(1);
    await expect(activate).toBeDisabled();
    await activate.dispatchEvent("click");
    expect(api.counts.get(activationPath)).toBe(1);
  } finally {
    release();
  }
  await expect(page.locator("#organization-active-name")).toHaveText("Beta Labs");
  await expect(dialog).not.toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("organization approval is rejected after the active organization changes", async ({ page, appURL, api }) => {
  await page.clock.install();
  await page.goto(appURL + "/organizations.html");
  await page.locator('[data-organization-id="beta"]').click();
  await page.locator('[data-activate-organization="beta"]').click();
  const dialog = page.getByRole("dialog", { name: "Switch organization?", exact: true });
  await expect(dialog).toBeVisible();
  api.organizations.active_id = "gamma";
  await page.clock.runFor(5100);
  await expect(page.locator("#organization-active-name")).toHaveText("Gamma Labs");
  await dialog.getByRole("button", { name: "Switch organization", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("organization state changed");
  await expect(dialog.getByRole("button", { name: "Switch organization", exact: true })).toBeDisabled();
  expect(api.counts.get("/api/simulator/organizations/beta/activate") || 0).toBe(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).not.toBeVisible();
});

test("Organizations exposes the local reset log", async ({ page, appURL }) => {
  await page.goto(appURL + "/organizations.html");
  const resetLog = page.getByRole("link", { name: "Open reset log", exact: true });
  await expect(resetLog).toHaveAttribute("href", "/api/simulator/activation-log");
  await expect(resetLog).toHaveAttribute("target", "_blank");
  await expect(resetLog).toHaveAttribute("rel", "noopener noreferrer");
});

test("organization switching shows reset progress in the Simulator", async ({ page, appURL, api }) => {
  await page.clock.install();
  api.organizations.activation = { state: "resetting", organization_id: "beta", progress: "Waiting for ZPR node and company LDAP" };
  await page.goto(appURL + "/organizations.html");
  await expect(page.locator("#organization-connection")).toContainText("Waiting for ZPR node and company LDAP");
  await page.locator('[data-organization-id="beta"]').click();
  const activationButton = page.locator('[data-activate-organization="beta"]');
  await expect(activationButton).toBeDisabled();
  await expect(activationButton).toContainText("Resetting ZPR · Waiting for ZPR node and company LDAP");
  api.organizations.activation.progress = "Applying organization runtime policy";
  await page.clock.runFor(1100);
  await expect(page.locator("#organization-connection")).toContainText("Applying organization runtime policy");
  await expect(activationButton).toContainText("Applying organization runtime policy");
});

test("scenario machine lanes scroll from a scrollbar at the top of the panel", async ({ page, appURL }) => {
  await page.goto(appURL + "/scenarios.html");
  await page.evaluate(() => renderScenarioRun({
    scenario_id: "many-machines",
    scenario_name: "Many machines",
    state: "running",
    total_steps: 8,
    current_step: 1,
    steps: [],
    scenario: {
      steps: Array.from({ length: 8 }, (_, index) => ({
        action: "wait_controller",
        machine: `machine-${index + 1}`,
      })),
      cleanup: [],
    },
  }));

  const frame = page.locator(".scenario-track-frame");
  const scrollbar = frame.locator(".scenario-track-scrollbar");
  const viewport = frame.locator(".scenario-track-viewport");
  await expect(scrollbar).toBeVisible();
  await expect(viewport).toHaveCSS("scrollbar-width", "none");
  const scrollbarBox = await scrollbar.boundingBox();
  const viewportBox = await viewport.boundingBox();
  expect(scrollbarBox.y).toBeLessThan(viewportBox.y);

  await scrollbar.evaluate((element) => { element.scrollLeft = element.scrollWidth; });
  await expect.poll(() => viewport.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  await viewport.evaluate((element) => { element.scrollLeft = 0; });
  await expect.poll(() => scrollbar.evaluate((element) => element.scrollLeft)).toBe(0);
});