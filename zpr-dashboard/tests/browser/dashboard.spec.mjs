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

async function selectAdapterLogSource(panel, label, kind = "adapter") {
  await panel.getByRole("button", { name: new RegExp(`Choose ${kind} and log source`) }).click();
  const dialog = panel.getByRole("dialog", { name: `Choose ${kind} log source` });
  await dialog.getByRole("combobox").selectOption({ label });
}

function registerAssertionBrowserTests() {
for (const [worst, states, warnings, error] of [
  ["error", ["pass", "fail"], true, "line 1: evaluation error"],
  ["fail", ["pass", "fail"], true, ""],
  ["warning", ["pass", "pass"], true, ""],
  ["pass", ["pass", "pass"], false, ""],
]) {
  test(`Assertion gutter has one ${worst} tag per line and retains all details`, async ({ page, appURL, api }) => {
    api.handlers.set("/api/assertions/evaluate", route => route.fulfill({ json: {
      revision: 0, status: error ? "error" : "pass", error, finished_at: "2026-10-05T12:00:00Z",
      results: states.map((status, index) => ({
        rule: { line: 1, kind: "group", group: `Group${index}`, operator: ">=", limit: 1 },
        status, message: `result-${index}`, subjects: [`subject-${index}`],
      })),
      warnings: warnings ? [
        { line: 1, code: "FIRST_WARNING", message: "first warning" },
        { line: 1, code: "SECOND_WARNING", message: "second warning" },
      ] : [],
    } }));
    await openAssertionRecord(page, appURL);
    await page.locator("#assertion-source").fill('assert true; assert false;');
    await page.locator("#assertion-analyze").click();
    const marker = page.locator('#assertion-result-lines [data-line="1"] button');
    await expect(marker).toHaveCount(1);
    await expect(marker).toHaveAttribute("data-state", worst);
    await marker.click();
    const dialog = page.getByRole("dialog", { name: `Assertion ${worst}` });
    for (const detail of ["result-0", "result-1", "subject-0", "subject-1"]) await expect(dialog).toContainText(detail);
    if (warnings) {
      await expect(dialog).toContainText("first warning");
      await expect(dialog).toContainText("second warning");
    }
    if (error) await expect(dialog).toContainText(error);
  });
}

test("Assertion scrollbar corner matches the standard editor on overflow only", async ({ page, appURL, api }) => {
  await openAssertionRecord(page, appURL);
  const source = page.locator("#assertion-source");
  const editor = page.locator("#assertion-editor");
  await source.fill(`group "${"Operators".repeat(80)}" members >= 2;`);
  await expect(editor).toHaveAttribute("data-horizontal-overflow", "true");
  expect(await editor.evaluate(element => {
    const corner = getComputedStyle(element, "::after");
    return { background: corner.backgroundColor, width: corner.width, left: corner.left, bottom: corner.bottom };
  })).toEqual({ background: "rgb(255, 255, 255)", width: "68px", left: "0px", bottom: "0px" });
  await source.fill('group "Operators" members >= 2;');
  await expect(editor).toHaveAttribute("data-horizontal-overflow", "false");
});
test("assertion editor shares policy file controls and clears the scrollbar gutter", async ({ page, appURL, api }) => {
  await openAssertionRecord(page, appURL);
  await expect(page.locator(".assertion-syntax")).toHaveCount(0);
  await expect(page.locator("#assertion-result-gutter")).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(page.locator("#policy-editor-mode")).toHaveCount(0);
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
      warnings: [
        { code: "ASSRT_HUMAN_SCOPE", severity: "warning", line: 4, message: "Review the scope for human identities." },
        { code: "ASSERT_CONTEXT", severity: "warning", message: "Context could not be determined." },
      ],
    } });
  });
  await openAssertionRecord(page, appURL);
  const editor = page.getByRole("textbox", { name: "Data assertion source", exact: true });
  await editor.fill('group "Operators" members >= 2;\npeople >= 1;\npeople exactly_one ["alice"];\nassert true;');
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-run-status")).toContainText("PASS");
  await expect(page.locator("#assertion-lint-warnings")).toContainText("ASSERT_CONTEXT");
  await expect(page.locator("#assertion-result-lines [data-line=\"4\"] .assertion-result-marker[data-state=\"warning\"]")).toHaveAttribute("aria-label", /ASSRT_HUMAN_SCOPE/);
  await expect(page.locator("#assertion-result-lines [data-line=\"1\"] .assertion-result-marker")).toHaveCount(0);
  await expect(page.locator("#assertion-analyze")).toHaveAttribute("data-analysis-state", "warning");
  await page.mouse.move(0, 0);
  await expect(page.locator("#assertion-analyze")).toHaveCSS("background-color", "rgb(231, 185, 63)");
  await expect(page.locator("#assertion-run-error")).toBeEmpty();
  await expect(page.getByRole("textbox", { name: "Data assertion source", exact: true })).toBeEditable();
  await page.locator('#assertion-result-lines [data-line="4"] .assertion-result-marker').click();
  await expect(page.getByRole("dialog", { name: "Assertion warning" })).toContainText("Review the scope for human identities.");
  await editor.fill('group "Operators" members >= 2;');
  await expect(page.locator("#assertion-lint-warnings")).toBeHidden();
  await expect(page.locator("#assertion-result-gutter button")).toHaveCount(0);
  await expect(page.locator("#assertion-analyze")).not.toHaveAttribute("data-analysis-state", /.+/);
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
  await page.locator("#assertion-analyze").click();
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
  await expect(editor).toBeEditable();
  expect(api.assertions.settings.source).toBe("");
  await openPolicyFiles(page);
  await page.locator("#assertion-save").click();
  await expect(page.locator("#assertion-revision")).toHaveText("Alpha Labs / r1");
  expect(api.assertions.settings.enabled).toBe(false);
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-result-rows")).toContainText("PASS");
  await expect(page.locator("#assertion-run-status")).toContainText("Saved r1");
  await page.locator("#assertion-analyze").click();
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
  await page.locator("#assertion-analyze").click();
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
    await expect.poll(async () => page.locator("#policy-code-editor").evaluate((element) => element.getBoundingClientRect().bottom + window.scrollY)).toBeGreaterThanOrEqual(1158);
    const policy = await page.locator("#policy-code-editor").boundingBox();
    const policyPane = await page.locator("#policy-editor-pane").boundingBox();
    expect(policy.y + policy.height).toBeGreaterThanOrEqual(policyPane.y + policyPane.height - 13);
    if (width > 900) expect(policy.y + policy.height).toBeGreaterThanOrEqual(1150);
    await openAssertionRecord(page, appURL);
    await expect(page.locator("#assertion-source")).toBeEnabled();
    const assertion = await page.locator("#assertion-editor").boundingBox();
    const assertionPanel = await page.locator("#policy-assertion-editor").boundingBox();
    if (width > 900) expect(assertionPanel.y + assertionPanel.height).toBeGreaterThanOrEqual(1150);
    expect(assertion.height).toBeGreaterThanOrEqual(360);
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
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-result-rows")).toContainText('people attribute "mail" present');
  await expect(page.locator("#assertion-result-rows")).toContainText("bob");
  const failMarker = page.locator('#assertion-result-gutter [data-line="1"] .assertion-result-marker');
  await expect(failMarker).toHaveAttribute("data-state", "fail");
  await failMarker.click();
  const failDetails = page.getByRole("dialog", { name: "Assertion fail" });
  await expect(failDetails).toContainText("2 checked; 1 violations");
  await expect(failDetails).toContainText("bob");
  await failDetails.locator(".dialog-actions .button").click();
  await page.locator("#assertion-analyze").click();
  await page.locator("#assertion-source").fill('each group attribute "gidNumber" >= 1000;');
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-result-rows")).toContainText('each group attribute "gidnumber" >= 1000');
  await expect(page.locator("#assertion-run-status")).toContainText("PASS");
  await page.locator("#assertion-analyze").click();
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
  await page.locator("#assertion-analyze").click();
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
  await expect(page.locator("#assertion-analyze")).toBeEnabled();
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-run-status")).toContainText("ERROR");
  await expect(page.locator("#assertion-run-error")).toContainText("no assertions were evaluated");
  await expect(page.locator("#assertion-result-gutter .assertion-result-marker")).toHaveCount(0);
  await expect(page.locator("#assertion-message")).toBeVisible();
  await expect(page.locator("#assertion-message")).toContainText("Trusted LDAP read failed");
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-result-rows tr")).toHaveCount(0);
  api.assertions.configured = false;
  await openPolicyFiles(page);
  await page.locator("#assertion-reload").click();
  await expect(page.locator("#assertion-analyze")).toBeDisabled();
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
  await expect(page.locator("#assertion-analyze")).toBeDisabled();
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
  await expect(page.locator("#assertion-analyze")).toBeDisabled();
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
  await expect(page.locator("#policy-revision-label")).toBeHidden();

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
import { readFile, mkdtemp, rm } from "node:fs/promises";
import { createServer as createHTTPSServer } from "node:https";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
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

async function browserAppURL(use, operatorHTTPS = false) {
  let tlsDirectory;
  let server;
  try {
    let credentials;
    if (operatorHTTPS) {
      tlsDirectory = await mkdtemp(resolve(tmpdir(), "zpr-operator-browser-"));
      const key = resolve(tlsDirectory, "key.pem");
      const cert = resolve(tlsDirectory, "cert.pem");
      execFileSync("openssl", ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
        "-keyout", key, "-out", cert, "-subj", "/CN=localhost", "-addext", "subjectAltName=DNS:localhost,IP:127.0.0.1"], { stdio: "ignore" });
      credentials = { key: await readFile(key), cert: await readFile(cert), minVersion: "TLSv1.3" };
    }
    const handler = async (request, response) => {
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
    };
    server = operatorHTTPS ? createHTTPSServer(credentials, handler) : createServer(handler);
    await new Promise((ready) => server.listen(0, "127.0.0.1", ready));
    await use(`${operatorHTTPS ? "https" : "http"}://127.0.0.1:${server.address().port}`);
  } finally {
    if (server?.listening) await new Promise((closed) => server.close(closed));
    if (tlsDirectory) await rm(tlsDirectory, { recursive: true });
  }
}

const test = base.extend({
  appURL: [async ({}, use) => browserAppURL(use), { scope: "worker" }],
  secureAppURL: [async ({}, use) => browserAppURL(use, true), { scope: "worker" }],
  api: async ({ page }, use) => {
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.addInitScript(() => {
      window.testCSPViolations = [];
      document.addEventListener("securitypolicyviolation", (event) => window.testCSPViolations.push(event.violatedDirective));
    });
    await page.route("**/auth/operator/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/auth/operator/config") await route.fulfill({ json: { enabled: false } });
      else await route.fulfill({ status: 503, json: { error: "Named operator login not configured in this test." } });
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
      trustedChangeFeeds: [{ name: "great_lakes_ldap", display_name: "Great Lakes LDAP" }],
      trustedChanges: {
        changes: [{
          cursor: "change-1", time: "2026-10-07T14:00:00Z", type: "modify",
          dn: "uid=alice,dc=alpha,dc=test", attributes: ["mail", "title"],
        }],
        cursor: "cursor-1", more: true,
      },
      trustedChangesNext: {
        changes: [{
          cursor: "change-2", time: "2026-10-07T14:01:00Z", type: "modrdn",
          dn: "uid=alice,dc=alpha,dc=test", new_dn: "uid=alice,ou=People,dc=alpha,dc=test",
          attributes: ["cn"],
        }],
        cursor: "cursor-2", more: false,
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
      if (path.startsWith("/api/trusted-sources/change-feeds/") && path.endsWith("/changes")) {
        const cursor = new URL(route.request().url()).searchParams.get("cursor");
        await route.fulfill({ json: cursor ? data.trustedChangesNext : data.trustedChanges });
        return;
      }
      const bodies = {
        "/api/adapter-logs": {
          organization_id: data.adapterLogs.organization_id,
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
        "/api/trusted-sources/change-feeds": { sources: data.trustedChangeFeeds },
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

test("GUI status sorting breaks primary ties independently of snapshot order", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node", node: true, node_details: { adapters: ["charlie", "alpha", "bravo"] } },
    ...["charlie", "alpha", "bravo"].map((cn) => ({ cn, node: false })),
  ];
  await page.goto(appURL + "/#connections");
  await page.locator('[data-sort-page="connections"] th[data-sort-key="state"]').click();
  const before = await page.locator("#link-list tr").allTextContents();
  api.snapshot.actors[0].node_details.adapters.reverse();
  api.snapshot.actors.reverse();
  await page.locator("#refresh-now").click();
  await expect.poll(() => page.locator("#link-list tr").allTextContents()).toEqual(before);
});

test("GUI top actions keep uptime, refresh controls, and Help together", async ({ page, appURL }) => {
  await page.goto(appURL + "/#map");
  const actions = page.locator(".main-content .top-actions");
  await expect(actions.locator(".uptime-status")).toBeVisible();
  await expect(actions.locator("#poll-rate")).toBeVisible();
  await expect(actions.locator("#refresh-now")).toBeVisible();
  const help = actions.getByRole("button", { name: "Help for this page" });
  await expect(help).toBeVisible();
  const bounds = await help.boundingBox();
  const actionsBounds = await actions.boundingBox();
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(actionsBounds.x + actionsBounds.width);
});

test("GUI Security nav alerts on repeated unknown or varied denials, not new actors", async ({ page, appURL, api }) => {
  const nav = page.locator('.primary-nav [data-page-link="security-review"]');
  await page.goto(appURL + "/#map");
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  api.snapshot.actors.push({ cn: "new-device", node: false, zpr_addr: "fd00::20" });
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  api.snapshot.recent_denies = [{ source_addr: "fd00::99", dest_addr: "fd00::20", count: 5, deny_code: "DENY", last_deny_ms: Date.now() }];
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "true");
  await expect(nav).toHaveAccessibleName("Security: 1 high alerts");
  await nav.click();
  await page.locator("#security-review-dismiss-all").click();
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  api.snapshot.recent_denies = [80, 443, 22].map((port) => ({ source_addr: "fd00::20", dest_addr: "fd00::30", protocol: 6, dest_port: port, count: 1, deny_code: "DENY", last_deny_ms: Date.now() }));
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "true");
});

test("GUI Security visit acknowledges highlights without dismissing findings and new alerts rearm them", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  const nav = page.locator('.primary-nav [data-page-link="security-review"]');
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  api.snapshot.recent_denies = [{ source_addr: "fd00::99", dest_addr: "fd00::20", count: 5, deny_code: "DENY", last_deny_ms: 1000 }];
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "true");
  await nav.click();
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  await expect(nav).toHaveAccessibleName("Security");
  await expect(page.locator("#security-review-findings")).toContainText("Repeated policy denials");
  await expect(page.locator("#security-review-count")).toContainText("1 active");
  await page.getByRole("link", { name: "Map", exact: true }).click();
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  api.snapshot.recent_denies[0].last_deny_ms = 2000;
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "true");
  await nav.click();
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  await page.locator("#security-review-filter").fill("does not match");
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  api.snapshot.recent_denies.push({ source_addr: "fd00::98", dest_addr: "fd00::20", count: 8, deny_code: "DENY", last_deny_ms: 3000 });
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "true");
  await page.getByRole("link", { name: "Map", exact: true }).click();
  await nav.click();
  await expect(nav).toHaveAttribute("data-high-alert", "false");
  api.snapshot.recent_denies = [];
  await page.locator("#refresh-now").click();
  await expect(page.locator("#security-review-count")).toContainText("0 active");
  api.snapshot.recent_denies = [{ source_addr: "fd00::99", dest_addr: "fd00::20", count: 5, deny_code: "DENY", last_deny_ms: 2000 }];
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "true");
});

test("Security Review flags aggregate DNS probing without raising a source-attributed high alert", async ({ page, appURL, api }) => {
  let statsRequest = 0;
  api.handlers.set("/api/dns/stats/json/v1/server", async (route) => {
    statsRequest += 1;
    const nsstats = statsRequest === 1
      ? { Requestv4: 0, Requestv6: 0, QryNXDOMAIN: 0 }
      : { Requestv4: 20, Requestv6: 10, QryNXDOMAIN: 20 };
    await route.fulfill({ json: { nsstats } });
  });

  await page.goto(`${appURL}/#security-review`);
  await page.locator("#refresh-now").click();

  const finding = page.locator("#security-review-findings tr").filter({ hasText: "DNS probing pattern" });
  await expect(finding).toBeVisible();
  await expect(finding).toHaveAttribute("data-severity", "review");
  await expect(finding).toContainText("20 NXDOMAIN responses");
  await expect(finding).toContainText("30 requests");
  await expect(page.locator('.primary-nav [data-page-link="security-review"]')).toHaveAttribute("data-high-alert", "false");
});

test("Security Review ignores DNS NXDOMAIN activity below the aggregate threshold", async ({ page, appURL, api }) => {
  let statsRequest = 0;
  api.handlers.set("/api/dns/stats/json/v1/server", async (route) => {
    statsRequest += 1;
    const nsstats = statsRequest === 1
      ? { Requestv4: 0, Requestv6: 0, QryNXDOMAIN: 0 }
      : { Requestv4: 20, Requestv6: 10, QryNXDOMAIN: 19 };
    await route.fulfill({ json: { nsstats } });
  });

  await page.goto(`${appURL}/#security-review`);
  await page.locator("#refresh-now").click();
  await expect(page.locator("#security-review-findings")).not.toContainText("DNS probing pattern");
});

test("shared JSON log formatter preserves tokens and leaves non-JSON entries untouched", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#adapter-logs");
  const cases = [
    ['{"id":18446744073709551615,"id":1e+30,"nested":[true,null,{},[]]}', '{\n  "id": 18446744073709551615,\n  "id": 1e+30,\n  "nested": [\n    true,\n    null,\n    {},\n    []\n  ]\n}'],
    [' { "text": "space  and \\"quote\\" and \\\\ slash", "empty": [] } ', '{\n  "text": "space  and \\"quote\\" and \\\\ slash",\n  "empty": []\n}'],
    ['{"value":-0.000,"escape":"\\u0061","html":"<img src=x>"}', '{\n  "value": -0.000,\n  "escape": "\\u0061",\n  "html": "<img src=x>"\n}'],
    ["  plain log  ", "  plain log  "],
    ['{"truncated":', '{"truncated":'],
    ['{"invalid":true,}', '{"invalid":true,}'],
    ["[1,]", "[1,]"],
    ['prefix {"id":1}', 'prefix {"id":1}'],
    ["{}", "{}"],
    ["[]", "[]"],
    ["[".repeat(65) + "0" + "]".repeat(65), "[".repeat(65) + "0" + "]".repeat(65)],
  ];
  for (const [source, expected] of cases) {
    expect(await page.evaluate((source) => window.ZPRLogFormat.formatJSON(source), source)).toBe(expected);
  }
});

for (const view of [
  { name: "Adapter Logs", path: "/#adapter-logs", data: "adapterLogs", source: 1 },
  { name: "Workers", path: "/machine-logs.html", data: "workloadLogs", source: 0 },
]) {
  test(`${view.name} toggles formatted and exact raw JSON without collecting logs`, async ({ page, appURL, api }) => {
    await page.clock.install();
    const raw = ' {"id":18446744073709551615,"items":[1,2],"html":"<img src=x onerror=alert(1)>"} ';
    const lines = [raw, "plain log", '{"truncated":'];
    api[view.data].machines[0].sources[view.source].lines = lines;
    await page.goto(appURL + view.path);
    const output = page.locator(".machine-log-output pre").first();
    await expect(output).toHaveText(lines.join("\n"), { useInnerText: false });
    const toggle = page.getByRole("checkbox", { name: "Format JSON", exact: true });
    await expect(toggle).not.toBeChecked();
    await page.locator("#machine-logs-pause").click();
    const collections = api.counts.get(view.data === "adapterLogs" ? "/api/adapter-logs" : "/api/simulator/machine-logs");
    await toggle.check();
    await expect(output).toContainText('"id": 18446744073709551615');
    expect(await output.textContent()).toContain('\n  "items": [\n    1,\n    2\n  ]');
    await expect(output).toContainText('plain log\n{"truncated":');
    await expect(output.locator("img")).toHaveCount(0);
    if (view.data === "adapterLogs") {
      const toolbarTops = await page.locator(".adapter-logs-toolbar").evaluate((element) =>
        [...element.children].map((child) => Math.round(child.getBoundingClientRect().top)));
      expect(Math.max(...toolbarTops) - Math.min(...toolbarTops)).toBeLessThanOrEqual(44);
    }
    await page.getByRole("checkbox", { name: "Wrap", exact: true }).check();
    await expect(output).toHaveCSS("white-space", "pre-wrap");
    await toggle.uncheck();
    expect(await output.textContent()).toBe(lines.join("\n"));
    expect(api.counts.get(view.data === "adapterLogs" ? "/api/adapter-logs" : "/api/simulator/machine-logs")).toBe(collections);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  });
}

test("Diagnostics toggles JSON bodies locally with Simulator unavailable", async ({ page, appURL, api }) => {
  const simulationRequests = [];
  page.on("request", (request) => { if (request.url().includes("/api/simulator/")) simulationRequests.push(request.url()); });
  await page.clock.install();
  const raw = '{"count":18446744073709551615,"html":"<img src=x>"}';
  api.handlers.set("/api/diagnostics", (route) => route.fulfill({ json: {
    generated_at: "2026-10-08T12:00:00Z", state: "available", sources: [
      { id: "node:a", name: "Node A", kind: "ZPR node", identity: "a", state: "available", metrics: [], logs: [{ severity: "INFO", body: raw }, { body: "plain text" }] },
    ],
  } }));
  await page.goto(appURL + "/#diagnostics");
  const bodies = page.locator(".diagnostics-log-body");
  await expect(bodies.first()).toHaveText(raw);
  const collections = api.counts.get("/api/diagnostics");
  await page.getByRole("checkbox", { name: "Format JSON", exact: true }).check();
  expect(await bodies.first().textContent()).toBe('{\n  "count": 18446744073709551615,\n  "html": "<img src=x>"\n}');
  await expect(bodies.last()).toHaveText("plain text");
  await expect(bodies.locator("img")).toHaveCount(0);
  await page.getByRole("checkbox", { name: "Format JSON", exact: true }).uncheck();
  expect(await bodies.first().textContent()).toBe(raw);
  expect(api.counts.get("/api/diagnostics")).toBe(collections);
  expect(simulationRequests).toEqual([]);
});

test("Control Room map and shared logs work while Simulator APIs are unavailable", async ({ page, appURL, api }) => {
  const simulatorRequests = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.startsWith("/api/simulator/")) simulatorRequests.push(request.url());
  });
  await page.route("**/api/simulator/**", (route) => route.fulfill({
    status: 503, json: { error: "Simulator is unavailable" },
  }));
  await page.goto(appURL + "/#map");
  await expect(page.locator("#page-map")).toBeVisible();
  await expect.poll(() => api.counts.get("/api/snapshot") || 0).toBeGreaterThan(0);
  await page.evaluate(() => { location.hash = "#adapter-logs"; });
  await expect(page.locator("#machine-logs-status")).toHaveText("Live");
  await expect(page.locator(".machine-log-output").first()).toContainText("Control adapter entry 79");
  expect(api.counts.get("/api/adapter-logs") || 0).toBeGreaterThan(0);
  expect(simulatorRequests).toEqual([]);
});

test("Controller JSON formatting survives type switches, refresh and disconnection", async ({ page, appURL, api }) => {
  await page.clock.install();
  const raw = '{"controller":true,"id":18446744073709551615}';
  api.adapterLogs.machines[0].sources[0].lines = [raw];
  await page.goto(appURL + "/#adapter-logs");
  await expect(page.locator(".adapter-log-column")).toHaveCount(1);
  const toggle = page.getByRole("checkbox", { name: "Format JSON", exact: true });
  await toggle.check();
  await page.getByRole("button", { name: "Controller logs", exact: true }).click();
  const output = page.locator(".machine-log-output pre").first();
  expect(await output.textContent()).toBe('{\n  "controller": true,\n  "id": 18446744073709551615\n}');
  await page.getByRole("button", { name: "Adapter logs", exact: true }).click();
  await page.getByRole("button", { name: "Controller logs", exact: true }).click();
  await expect(output).toContainText('"controller": true');
  api.adapterLogs.machines[0].sources[0].lines = [raw, '{"refreshed":true}'];
  await page.clock.runFor(2100);
  await expect(output).toContainText('"refreshed": true');
  api.adapterLogs.machines[0].sources[0].error = "Controller unavailable";
  await page.clock.runFor(2100);
  await expect(page.locator(".machine-log-error").first()).toHaveText("Controller unavailable");
  await toggle.uncheck();
  expect(await output.textContent()).toBe(raw + '\n{"refreshed":true}');
});

test("GUI Adapter Logs places pickers in headers and toggles all panels and wrapping", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#adapter-logs");
  const panels = page.locator(".adapter-log-column");
  await expect(panels).toHaveCount(1);
  await expect(panels.locator("header .adapter-source-picker-button")).toBeVisible();
  await expect(panels.locator("header h2")).toHaveText("machine-first");
  await expect(panels.locator(".adapter-source-picker-button")).toHaveText("⌄");
  await expect(panels.locator(".adapter-source-picker-button")).toHaveAttribute("aria-haspopup", "dialog");
  await expect(page.locator(".adapter-logs-heading #adapter-log-all")).toBeVisible();
  await page.locator("#adapter-log-all").click();
  await expect(panels).toHaveCount(4);
  await page.getByRole("checkbox", { name: "Wrap", exact: true }).uncheck();
  await expect(panels.first().locator("pre")).toHaveCSS("white-space", "pre");
  await page.locator("#adapter-log-wrap").check();
  await expect(panels.first().locator("pre")).toHaveCSS("white-space", "pre-wrap");
  await page.locator("#adapter-log-all").click();
  await expect(panels).toHaveCount(0);
  await page.locator("#adapter-log-add").click();
  await expect(panels).toHaveCount(1);
});

test("GUI Adapter Logs starts unwrapped, including long lines and maximized panels", async ({ page, appURL, api }) => {
  api.adapterLogs.machines[0].sources[1].lines = ["x".repeat(400)];
  await page.goto(appURL + "/#adapter-logs");
  const panel = page.locator(".adapter-log-column").first();
  const output = panel.locator(".machine-log-output");
  await expect(output).toContainText("x".repeat(400));
  await expect(page.getByRole("checkbox", { name: "Wrap", exact: true })).not.toBeChecked();
  await expect(output.locator("pre")).toHaveCSS("white-space", "pre");
  await expect.poll(() => output.evaluate(el => el.scrollWidth > el.clientWidth)).toBe(true);
  await page.locator("#adapter-log-wrap").check();
  await expect(output.locator("pre")).toHaveCSS("white-space", "pre-wrap");
  await expect.poll(() => output.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  await panel.getByRole("button", { name: "Maximize adapter panel 1", exact: true }).click();
  await expect(output.locator("pre")).toHaveCSS("white-space", "pre-wrap");
  await expect.poll(() => output.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
});

test("GUI Adapter Logs filters sources to running devices and selects them in a dialog", async ({ page, appURL, api }) => {
  api.adapterLogs.machines[1].state = "stopped";
  await page.goto(appURL + "/#adapter-logs");
  const panel = page.locator(".adapter-log-column").first();
  await expect(panel.locator("header h2")).toHaveText("machine-first");
  await page.locator("#adapter-log-running").check();
  await panel.getByRole("button", { name: /Choose adapter and log source/ }).click();
  const dialog = panel.getByRole("dialog", { name: "Choose adapter log source" });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("header, h3, label, button")).toHaveCount(0);
  await expect(dialog.getByRole("listbox").locator("option")).toHaveCount(2);
  await expect(dialog.getByRole("listbox")).toHaveAttribute("size", "2");
  const titleBounds = await panel.locator("header h2").boundingBox();
  const pickerBounds = await dialog.boundingBox();
  expect(pickerBounds.y).toBeCloseTo(titleBounds.y + titleBounds.height + 6, 0);
  expect(pickerBounds.x).toBeCloseTo(titleBounds.x, 0);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await panel.getByRole("button", { name: /Choose adapter and log source/ }).click();
  await dialog.getByRole("listbox").selectOption({ label: "finance-client adapter · machine-first" });
  await expect(dialog).toBeHidden();
  await expect(panel.locator(".machine-log-output")).toContainText("finance-client adapter entry 79");
  await expect(panel.locator(".adapter-source-picker-button")).toHaveAttribute("title", /finance-client adapter/);
});

test("GUI Adapter Logs distinguishes no available adapters from closed panels and recovers", async ({ page, appURL, api }) => {
  api.adapterLogs.machines = [];
  await page.goto(appURL + "/#adapter-logs");
  await expect(page.locator(".adapter-columns-empty")).toHaveText("No adapters available.");
  await expect(page.locator(".adapter-log-column:visible")).toHaveCount(0);
  await expect(page.locator("#adapter-log-add")).toBeDisabled();
  await expect(page.locator("#adapter-log-all")).toBeDisabled();
  api.adapterLogs.machines = [{
    machine: { id: "restored", name: "Restored adapter" }, state: "stopped",
    sources: [{ name: "Restored adapter", state: "stopped", lines: ["restored logs"] }],
  }];
  await expect(page.locator(".adapter-log-column:visible")).toHaveCount(1);
  await expect(page.locator("#adapter-log-add")).toBeEnabled();
  await page.locator("#adapter-log-running").check();
  await expect(page.locator(".adapter-columns-empty")).toHaveText("No running adapters available.");
  await expect(page.locator(".adapter-log-column:visible")).toHaveCount(0);
  await page.locator("#adapter-log-running").uncheck();
  await expect(page.locator(".adapter-log-column:visible")).toHaveCount(1);
  await expect(page.locator(".machine-log-output")).toContainText("restored logs");
  await page.getByRole("button", { name: "Controller logs", exact: true }).click();
  await expect(page.locator(".adapter-columns-empty")).toHaveText("No controllers available.");
});

test("GUI Running only excludes stopped, failed and unknown sources even on a running device", async ({ page, appURL, api }) => {
  api.adapterLogs.machines[0].sources[1].state = "exited";
  api.adapterLogs.machines[0].sources[2].state = "running";
  api.adapterLogs.machines[1].sources[1].state = "unknown";
  api.adapterLogs.machines[1].sources[2].state = "running";
  api.adapterLogs.machines[1].sources[2].error = "Log source unavailable";
  await page.goto(appURL + "/#adapter-logs");
  await page.locator("#adapter-log-all").click();
  await expect(page.locator(".adapter-log-column")).toHaveCount(4);
  await page.locator("#adapter-log-running").check();
  await expect(page.locator(".adapter-log-column")).toHaveCount(1);
  const panel = page.locator(".adapter-log-column");
  await expect(panel.locator("header h2")).toHaveText("machine-first");
  await expect(panel.locator(".machine-log-output")).toContainText("finance-client adapter entry");
  await expect(panel.locator("select option")).toHaveCount(1);
  await page.locator("#adapter-log-running").uncheck();
  await expect(page.locator(".adapter-log-column")).toHaveCount(4);
  expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
});

test("GUI LDAP tree uses real nested DNs, escaped commas, approved attributes and filtering", async ({ page, appURL, api }) => {
  api.assertionSource.directory.entries = [
    { dn: "uid=alice,ou=Engineering,dc=alpha,dc=test", attributes: { mail: ["alice@example.test"] } },
    { dn: "cn=Operations\\, East,ou=Groups,dc=alpha,dc=test", attributes: { description: ["Operators"] } },
  ];
  await page.goto(appURL + "/#sources");
  const browser = page.locator("trusted-source-browser");
  await browser.getByRole("tab", { name: "LDAP tree" }).click();
  await expect(browser.locator("[data-ldap-entry]")).toHaveCount(2);
  await expect(browser.getByText("READ ONLY", { exact: true })).toHaveCount(0);
  await browser.locator('summary').filter({ hasText: "uid=alice" }).click();
  await expect(browser.locator(".trusted-source-tree")).toContainText("alice@example.test");
  await expect(browser.locator('summary').filter({ hasText: "cn=Operations\\, East" })).toBeVisible();
  await browser.getByRole("searchbox").fill("alice");
  await expect(browser.locator("[data-ldap-entry]")).toHaveCount(1);
  await expect(browser.locator(".trusted-source-tree")).toContainText("ou=Engineering");
  expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
});

test("GUI LDAP keeps its tree during polling and navigation, but manual Refresh reloads", async ({ page, appURL, api }) => {
  api.assertionSource.directory.entries = [
    { dn: "uid=alice,ou=Engineering,dc=alpha,dc=test", attributes: { mail: ["alice@example.test"] } },
  ];
  await page.goto(appURL + "/#sources");
  const browser = page.locator("trusted-source-browser");
  await browser.getByRole("tab", { name: "LDAP tree" }).click();
  const entry = browser.locator("[data-ldap-entry]");
  await expect(entry).toHaveCount(1);
  await entry.locator("summary").click();
  await expect(entry).toHaveAttribute("open", "");
  const sourceReads = api.counts.get("/api/assertions/source");
  const snapshots = api.counts.get("/api/snapshot") || 0;
  await page.locator("#poll-rate").selectOption("3");
  await expect.poll(() => api.counts.get("/api/snapshot"), { timeout: 10000 }).toBeGreaterThan(snapshots + 1);
  expect(api.counts.get("/api/assertions/source")).toBe(sourceReads);
  await expect(entry).toHaveAttribute("open", "");
  await page.getByRole("link", { name: "Map", exact: true }).click();
  await page.getByRole("link", { name: "Trusted Sources", exact: true }).click();
  await expect(entry).toHaveAttribute("open", "");
  expect(api.counts.get("/api/assertions/source")).toBe(sourceReads);
  api.assertionSource.directory.entries[0].attributes.mail = ["updated@example.test"];
  await page.locator("#refresh-now").click();
  await expect.poll(() => api.counts.get("/api/assertions/source")).toBe(sourceReads + 1);
  await entry.locator("summary").click();
  await expect(entry).toContainText("updated@example.test");
  expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
});

test("GUI ZPR Config shares editor controls and keeps an error-only gutter", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#zpr-config");
  await expect(page.locator("#zpr-config-name")).toHaveCount(0);
  await expect(page.locator("#zpr-config-title")).toHaveText("Untitled");
  await expect(page.locator("#zpr-config-revision-label")).toBeEmpty();
  const frameStyle = await page.locator("#page-zpr-config .editor-assistant-layout").evaluate((element) => ({
    border: getComputedStyle(element).borderTopWidth,
    padding: getComputedStyle(element.querySelector(".editor-assistant-main")).paddingTop,
  }));
  expect(frameStyle).toEqual({ border: "0px", padding: "0px" });
  await expect(page.locator("#zpr-config-status")).toBeHidden();
  await expect(page.locator("#zpr-config-modified")).toBeHidden();
  const configAnalyze = page.locator("#zpr-config-validate");
  const configFormat = page.locator("#zpr-config-format");
  await expect(configAnalyze).toBeDisabled();
  await expect(configFormat).toBeDisabled();
  await expect(configFormat).not.toHaveClass(/button-save-as-ready/);
  const disabledOpacity = await configAnalyze.evaluate((button) => getComputedStyle(button).opacity);
  expect(disabledOpacity).toBe(await page.locator("#policy-check").evaluate((button) => getComputedStyle(button).opacity));
  await page.locator("#zpr-config-source").fill('[visa_service]\ndock_node = "node"\n');
  await expect(configAnalyze).toBeEnabled();
  await expect(configFormat).toBeEnabled();
  await expect(configAnalyze).toHaveClass(/button-next-evaluate/);
  await expect(configFormat).toHaveClass(/button-save-as-ready/);
  await expect(page.locator("#zpr-config-gutter .config-gutter-line")).toHaveText(["", "", ""]);
  expect(await page.locator("#zpr-config-gutter .config-gutter-line").evaluateAll((rows) => rows.map((row) => row.dataset.line))).toEqual(["1", "2", "3"]);
  await expect(page.locator("#zpr-config-modified")).toBeVisible();
  await expect(page.locator("#page-zpr-config")).not.toContainText("Select a");
  await expect(page.locator("#page-zpr-config").getByRole("button", { name: "Refresh Attributes" })).toHaveCount(0);
  await expect(page.locator("#zpr-config-picker-toggle")).toContainText("Browse...");
  await expect(configAnalyze.locator("xpath=..")).toHaveClass(/policy-attribute-toolbar/);
  await expect(configFormat.locator("xpath=..")).toHaveClass(/policy-attribute-toolbar/);
  const toolbarLayout = await page.evaluate(() => {
    const rect = (selector) => document.querySelector(selector).getBoundingClientRect();
    const browse = rect("#zpr-config-picker-toggle");
    const file = rect("#zpr-config-files-toggle");
    const actions = rect("#page-zpr-config .policy-attribute-toolbar");
    return { browseRight: browse.right, fileLeft: file.left, fileRight: file.right, actionsLeft: actions.left };
  });
  expect(toolbarLayout.browseRight).toBeLessThanOrEqual(toolbarLayout.fileLeft);
  expect(toolbarLayout.fileRight).toBeLessThanOrEqual(toolbarLayout.actionsLeft);
  expect(toolbarLayout.actionsLeft - toolbarLayout.fileRight).toBeLessThanOrEqual(10);
  const styles = await page.evaluate(() => {
    document.querySelector("#policy-check").classList.add("button-next-evaluate");
    const properties = (selector) => {
      const element = document.querySelector(selector);
      const style = getComputedStyle(element);
      return [style.backgroundColor, style.color, style.minHeight, style.fontSize, style.padding];
    };
    return {
      policyFile: properties("#policy-files-toggle"),
      configFile: properties("#zpr-config-files-toggle"),
      policyAnalyze: properties("#policy-check"),
      configAnalyze: properties("#zpr-config-validate"),
    };
  });
  expect(styles.configFile).toEqual(styles.policyFile);
  expect(styles.configAnalyze).toEqual(styles.policyAnalyze);
  const analysisColors = await page.evaluate(async () => {
    const result = {};
    for (const state of ["success", "error"]) {
      const buttons = ["#policy-check", "#zpr-config-validate"].map((selector) => document.querySelector(selector));
      buttons.forEach((button) => { button.dataset.analysisState = state; });
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const values = buttons.map((button) => {
        const style = getComputedStyle(button);
        return { matches: button.matches(`:is(#policy-check, #assertion-analyze, #zpr-config-validate, #scenario-source-analyze)[data-analysis-state="${state}"]`), colors: [style.backgroundColor, style.borderColor, style.color] };
      });
      result[state] = values;
    }
    return result;
  });
  expect(analysisColors.success[1].matches).toBeTruthy();
  await page.goto(appURL + "/#map");
  expect(analysisColors.success[1].colors).toEqual(analysisColors.success[0].colors);
  expect(analysisColors.error[1].colors).toEqual(analysisColors.error[0].colors);
});

test("Analyze styling is blue pending and keeps green success and red errors across editors", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  await page.addStyleTag({ url: appURL + "/simulator.css" });
  await page.evaluate(() => {
    const ids = ["policy-check", "assertion-analyze", "zpr-config-validate", "gateway-analyze", "scenario-source-analyze"];
    const fixture = document.createElement("div");
    fixture.className = "scenario-editor-dialog";
    fixture.style.cssText = "position:fixed;inset:0;z-index:10000;background:white;display:flex;align-items:flex-start";
    for (const id of ids) {
      document.getElementById(id)?.remove();
      const button = document.createElement("button");
      button.id = id;
      button.className = "button button-next-evaluate";
      button.textContent = id;
      fixture.append(button);
    }
    document.body.append(fixture);
  });
  for (const [state, normal, hover] of [
    ["", "rgb(36, 99, 166)", "rgb(29, 80, 136)"],
    ["pending", "rgb(36, 99, 166)", "rgb(29, 80, 136)"],
    ["success", "rgb(35, 117, 76)", "rgb(25, 92, 58)"],
    ["error", "rgb(184, 59, 59)", "rgb(153, 47, 47)"],
  ]) {
    await page.evaluate((value) => {
      document.querySelectorAll(".scenario-editor-dialog button").forEach((button) => {
        if (value) button.dataset.analysisState = value;
        else delete button.dataset.analysisState;
      });
    }, state);
    for (const id of ["policy-check", "assertion-analyze", "zpr-config-validate", "gateway-analyze", "scenario-source-analyze"]) {
      await page.mouse.move(0, 200);
      const button = page.locator(`#${id}`);
      await expect(button).toHaveCSS("background-color", normal);
      await expect(button).toHaveCSS("color", "rgb(255, 255, 255)");
      await button.hover();
      await expect(button).toHaveCSS("background-color", hover);
    }
  }
});

test("Configuration navigation labels the ZPR Config editor Config and keeps its route", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  if (await page.locator("body").evaluate((body) => body.classList.contains("sidebar-condensed"))) {
    await page.getByRole("button", { name: "Expand side menu", exact: true }).click();
  }
  const configLink = page.getByRole("link", { name: "Config", exact: true });
  await expect(configLink).toHaveAttribute("href", "#zpr-config");
  await configLink.click();
  await expect(page.locator("#page-zpr-config")).toBeVisible();
  await expect(page.locator("#zpr-config-title")).toBeVisible();
});

test("GUI editor search and replace is literal, case-aware, bounded, and updates config analysis", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/config/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Valid TOML" } }));
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  const tools = page.locator('[data-editor-search-target="zpr-config-source"]');
  const query = tools.getByRole("textbox", { name: "Find what" });
  await source.fill('name = "Alpha alpha a.b a.b"\n');
  await page.locator("#zpr-config-validate").click();
  await source.press("Control+f");
  await query.fill("ALPHA");
  await expect(tools.getByRole("status")).toHaveText("2 matches");
  await expect(tools.getByRole("tab", { name: "Find" })).toHaveAttribute("aria-selected", "true");
  await expect(tools.getByRole("textbox", { name: "Replace with" })).toBeHidden();
  await expect(tools.getByRole("checkbox", { name: "Match case" })).toBeHidden();
  await tools.getByRole("button", { name: "More >>" }).click();
  await expect(tools.getByRole("group", { name: "Search Options" })).toBeVisible();
  await tools.getByRole("checkbox", { name: "Match case" }).check();
  await expect(tools.getByRole("status")).toHaveText("0 matches");
  await tools.getByRole("checkbox", { name: "Match case" }).uncheck();
  await tools.getByRole("button", { name: "Find Next", exact: true }).click();
  await expect(tools.getByRole("status")).toContainText("Match 1 of 2");
  await query.press("Shift+Enter");
  await expect(tools.getByRole("status")).toContainText("Match 2 of 2");
  await tools.getByRole("combobox", { name: "Search direction" }).selectOption("down");
  await tools.getByRole("button", { name: "Find Next", exact: true }).click();
  await expect(tools.getByRole("status")).toHaveText("Reached the end of the source.");
  await tools.getByRole("combobox", { name: "Search direction" }).selectOption("all");
  await tools.getByRole("tab", { name: "Replace" }).click();
  await expect(tools.getByRole("textbox", { name: "Replace with" })).toBeVisible();
  await query.fill("a.b");
  await expect(tools.getByRole("status")).toHaveText("2 matches");
  await tools.getByRole("textbox", { name: "Replace with" }).fill("$&");
  await tools.getByRole("button", { name: "Replace all" }).click();
  await expect(source).toHaveValue('name = "Alpha alpha $& $&"\n');
  await expect(page.locator("#zpr-config-modified")).toBeVisible();
  await expect(page.locator("#zpr-config-validate")).not.toHaveAttribute("data-analysis-state", "success");
  await expect(tools.getByRole("status")).toContainText("Replaced 2 matches");
  await source.evaluate((element) => { element.maxLength = element.value.length; });
  await query.fill("$&");
  await tools.getByRole("textbox", { name: "Replace with" }).fill("longer");
  await tools.getByRole("button", { name: "Replace all" }).click();
  await expect(tools.getByRole("status")).toContainText("No changes made");
  await expect(source).toHaveValue('name = "Alpha alpha $& $&"\n');
  await query.press("Escape");
  await expect(tools.getByRole("search")).toBeHidden();
  await expect(source).toBeFocused();
  const toggle = tools.getByRole("button", { name: "Find & Replace", exact: true });
  await expect(tools.getByRole("button")).toHaveCount(1);
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  const idleBackground = await toggle.evaluate(async (element) => {
    await Promise.all(element.getAnimations().map((animation) => animation.finished));
    return getComputedStyle(element).backgroundColor;
  });
  await toggle.click();
  await expect(tools.getByRole("dialog", { name: "Find and Replace" })).toBeVisible();
  await expect(tools.getByRole("search")).toBeVisible();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(toggle).toHaveCSS("background-color", "rgb(23, 77, 61)");
  expect(idleBackground).not.toBe("rgb(23, 77, 61)");
  await expect(tools.getByRole("tab", { name: "Replace" })).toHaveAttribute("aria-selected", "true");
  await tools.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(tools.getByRole("search")).toBeHidden();
  await toggle.click();
  await toggle.click();
  await expect(tools.getByRole("search")).toBeHidden();
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
});

test("GUI editor search supports regular expressions, compact status, and undoable replacements", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  const tools = page.locator('[data-editor-search-target="zpr-config-source"]');
  const query = tools.getByRole("textbox", { name: "Find what" });
  const status = tools.getByRole("status");
  const original = 'port = 8080\nhost = "a-1"\n';
  await source.fill(original);
  await tools.getByRole("button", { name: "Find & Replace", exact: true }).click();
  await expect(status).toHaveText("");
  await expect(tools.getByText(/Enter text/)).toHaveCount(0);
  await query.fill("\\d+");
  await expect(status).toHaveText("0 matches");
  await tools.getByRole("button", { name: "More >>" }).click();
  await tools.getByRole("checkbox", { name: "Use regular expressions" }).check();
  await expect(status).toHaveText("2 matches");
  await tools.getByRole("button", { name: "Find Next", exact: true }).click();
  await expect(status).toHaveText("Match 1 of 2");
  await query.fill("(");
  await expect(status).toContainText("Invalid regular expression");
  await expect(tools.getByRole("button", { name: "Replace all" })).toBeDisabled();
  await query.fill("(\\w)-(\\d)");
  await tools.getByRole("textbox", { name: "Replace with" }).fill("$2_$1$$");
  await tools.getByRole("button", { name: "Replace all" }).click();
  await expect(source).toHaveValue('port = 8080\nhost = "1_a$"\n');
  await expect(page.locator("#zpr-config-modified")).toBeVisible();
  await source.focus();
  await page.keyboard.press("ControlOrMeta+z");
  await expect(source).toHaveValue(original);
  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect(source).toHaveValue('port = 8080\nhost = "1_a$"\n');
});

test("GUI editor search controls follow the policy or assertion source and precede History", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const tools = page.locator('[data-editor-search-target="policy-source,assertion-source"]');
  await page.locator("#policy-source").fill('define One as user.\ndefine Two as user.\n');
  await tools.getByRole("button", { name: "Find & Replace", exact: true }).click();
  await tools.getByRole("textbox", { name: "Find what" }).fill("user");
  await tools.getByRole("textbox", { name: "Replace with" }).fill("service");
  await tools.getByRole("button", { name: "Replace", exact: true }).click();
  await expect(page.locator("#policy-source")).toHaveValue('define One as service.\ndefine Two as user.\n');
  await tools.getByRole("button", { name: "Close", exact: true }).click();
  await openPolicyPicker(page);
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator('[data-record-id="test-assertions"]').click();
  const assertion = page.locator("#assertion-source");
  await expect(assertion).toBeVisible();
  const policyBefore = await page.locator("#policy-source").inputValue();
  await assertion.fill('group "Operators" members >= 2;\n');
  await assertion.press("Control+h");
  await tools.getByRole("textbox", { name: "Find what" }).fill("Operators");
  await tools.getByRole("textbox", { name: "Replace with" }).fill("Reviewers");
  await tools.getByRole("button", { name: "Replace all" }).click();
  await expect(assertion).toHaveValue('group "Reviewers" members >= 2;\n');
  await expect(page.locator("#policy-modified-indicator")).toBeVisible();
  await expect(page.locator("#policy-source")).toHaveValue(policyBefore);
  await expect(page.locator("#policy-edit-state > #policy-history-menu")).toBeVisible();
});

test("GUI editor search works in Simulator scenario JSON without saving or running", async ({ page, appURL, api }) => {
  api.handlers.set("/api/simulator/scenarios", (route) => route.fulfill({ json: {
    active_organization_id: "alpha", organization: { name: "Alpha Labs" }, scenarios: [], run: { state: "idle" }, max_machines: 12,
  } }));
  await page.goto(appURL + "/scenarios.html");
  await expect(page.locator("#scenario-organization")).toHaveText("Alpha Labs");
  await page.locator("#scenario-new").click();
  const tools = page.locator('[data-editor-search-target="scenario-editor-source"]');
  await tools.getByRole("button", { name: "Find & Replace", exact: true }).click();
  await expect(page.locator("#scenario-editor-advanced")).toHaveAttribute("open", "");
  await tools.getByRole("textbox", { name: "Find what" }).fill('"name": ""');
  await tools.getByRole("textbox", { name: "Replace with" }).fill('"name": "Search draft"');
  await tools.getByRole("button", { name: "Replace all" }).click();
  expect(JSON.parse(await page.locator("#scenario-editor-source").inputValue()).name).toBe("Search draft");
  await tools.getByRole("button", { name: "Close", exact: true }).click();
  await page.locator("#scenario-editor-mode-toggle").click();
  await expect(page.locator("#scenario-editor-name")).toHaveValue("Search draft");
  expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
});

async function openRawScenario(page, appURL, api, assistantReady = false) {
  api.handlers.set("/api/simulator/scenarios", (route) => route.fulfill({ json: {
    active_organization_id: "alpha", organization: { name: "Alpha Labs" }, scenarios: [], run: { state: "idle" }, max_machines: 12,
  } }));
  api.handlers.set("/api/simulator/assistant/status", (route) => route.fulfill({ json: {
    ready: assistantReady, model: "test-model", models: ["test-model"],
  } }));
  api.handlers.set("/api/simulator/organizations/alpha/scenario-check", (route) => {
    const request = route.request().postDataJSON();
    let scenario;
    try {
      if (request.format === "yaml") {
        scenario = Object.fromEntries(request.source.split("\n").map((line) => line.match(/^([A-Za-z0-9_]+):\s*(.*)$/)).filter(Boolean).map((match) => [match[1], match[2].replace(/^"|"$/g, "")]));
        scenario.steps = [{ action: "delay", timeout_seconds: 1 }];
        scenario.cleanup = [];
      } else {
        scenario = JSON.parse(request.source);
      }
    } catch {
      return route.fulfill({ status: 422, json: { valid: false, error: "Scenario must be valid JSON: invalid character", line: 3 } });
    }
    if (scenario.organization_id !== "alpha") return route.fulfill({ status: 422, json: { valid: false, error: "Scenario organization_id must match the selected organization." } });
    return route.fulfill({ json: {
      valid: true,
      diagnostics: "Scenario source and definition valid. Nothing saved, published, or run.",
      scenario,
      canonical_json: JSON.stringify(scenario, null, 2),
      canonical_yaml: `id: ${scenario.id}\norganization_id: ${scenario.organization_id}\nname: "${scenario.name}"\ndescription: "${scenario.description}"\nsteps:\n  - action: delay\n    timeout_seconds: 1\ncleanup: []\n`,
    } });
  });
  await page.goto(appURL + "/scenarios.html");
  await expect(page.locator("#scenario-organization")).toHaveText("Alpha Labs");
  await page.locator("#scenario-new").click();
  await page.locator("#scenario-editor-mode-toggle").click();
  await expect(page.locator("#scenario-editor-source")).toBeVisible();
}

for (const mode of ["raw", "form"]) {
  test(`Scenario JSON transport rejects invalid successful ${mode} analysis responses without assuming an HTTP status`, async ({ page, appURL, api }) => {
    await openRawScenario(page, appURL, api);
    if (mode === "form") await page.locator("#scenario-editor-mode-toggle").click();
    api.handlers.set("/api/simulator/organizations/alpha/scenario-check", (route) => route.fulfill({ status: 201, json: { valid: false } }));
    await page.locator("#scenario-source-analyze").click();
    await expect(page.locator("#scenario-editor-status")).toHaveText("Scenario analysis failed.");
    await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "error");
    await expect(page.locator("#scenario-source-gutter button")).toHaveCount(0);
    expect(api.counts.get("/api/simulator/organizations/alpha/scenario-check")).toBe(1);
  });

  test(`Scenario JSON transport reports malformed ${mode} analysis without changing the draft or adding line markers`, async ({ page, appURL, api }) => {
    await openRawScenario(page, appURL, api);
    if (mode === "form") await page.locator("#scenario-editor-mode-toggle").click();
    const source = page.locator("#scenario-editor-source");
    const original = await source.inputValue();
    api.handlers.set("/api/simulator/organizations/alpha/scenario-check", (route) => route.fulfill({ status: 502, contentType: "text/html", body: "upstream unavailable" }));
    await page.locator("#scenario-source-analyze").click();
    await expect(page.locator("#scenario-editor-status")).toHaveText("HTTP 502: invalid JSON response");
    await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "error");
    await expect(source).toHaveValue(original);
    await expect(page.locator("#scenario-source-gutter button")).toHaveCount(0);
    expect(api.counts.get("/api/simulator/organizations/alpha/scenario-check")).toBe(1);
    expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
  });
}

for (const operation of ["open", "history", "revision", "save", "publish"]) {
  test(`Scenario JSON transport rejects malformed ${operation} responses and preserves draft revision ownership`, async ({ page, appURL, api }) => {
    await openRawScenario(page, appURL, api);
    const scenario = { ...JSON.parse(await page.locator("#scenario-editor-source").inputValue()), id: "transport-test", name: "Transport test" };
    delete scenario.folder;
    const endpoint = "/api/simulator/organizations/alpha/scenarios/transport-test";
    const artifact = { id: scenario.id, revision: 2, published_revision: 0, content: scenario };
    const paths = { open: endpoint, history: endpoint + "/revisions", revision: endpoint + "/revisions/1", save: endpoint, publish: endpoint + "/publish" };
    const bad = (route) => route.fulfill({ status: 502, contentType: "text/html", body: "not JSON" });
    api.handlers.set(endpoint, (route) => route.fulfill({ json: artifact }));
    api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1 }, { revision: 2 }] }));
    if (operation === "open" || operation === "history") api.handlers.set(paths[operation], bad);
    const initial = await page.locator("#scenario-editor-source").inputValue();
    const openingError = await page.evaluate(async () => {
      try { await openExistingScenarioEditor("transport-test"); return null; }
      catch (error) { return error.message; }
    });
    if (operation === "open") {
      expect(openingError).toBe("HTTP 502: invalid JSON response");
      await expect(page.locator("#scenario-editor-source")).toHaveValue(initial);
    } else {
      expect(openingError).toBe(operation === "history" ? "HTTP 502: invalid JSON response" : null);
      await expect(page.locator("#scenario-editor-name")).toHaveValue(scenario.name);
      if (operation !== "history") {
        api.handlers.set(paths[operation], bad);
        if (operation === "revision") {
          await page.locator("#scenario-editor-history-menu > summary").click();
          await page.locator('#scenario-editor-history [data-revision="1"]').click();
        } else {
          if (operation === "save") await page.locator("#scenario-editor-name").fill("Unsaved transport draft");
          await page.locator("#scenario-editor-files-toggle").click();
          await page.locator(`#scenario-editor-${operation}`).click();
        }
        await expect(page.locator("#scenario-editor-status")).toHaveText("HTTP 502: invalid JSON response");
      }
      await expect(page.locator("#scenario-editor-name")).toHaveValue(operation === "save" ? "Unsaved transport draft" : scenario.name);
      expect(await page.evaluate(() => scenarioEditorArtifact.revision)).toBe(2);
      expect(await page.evaluate(() => scenarioEditorArtifact.published_revision)).toBe(0);
      if (operation === "save") await expect(page.locator("#scenario-editor-modified")).toBeVisible();
    }
    expect(api.counts.get(paths[operation])).toBe(operation === "save" ? 2 : 1);
    expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
  });
}

test("Scenario JSON transport keeps failed creation unsaved and preserves explicit creation payloads", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  const source = page.locator("#scenario-editor-source");
  const scenario = { ...JSON.parse(await source.inputValue()), id: "created-transport", name: "Created transport" };
  const text = JSON.stringify(scenario, null, 2);
  await source.fill(text);
  const endpoint = "/api/simulator/organizations/alpha/scenarios";
  const writes = [];
  api.handlers.set(endpoint, async (route) => {
    writes.push(route.request().postDataJSON());
    if (writes.length === 1) await route.fulfill({ status: 502, contentType: "text/html", body: "not JSON" });
    else await route.fulfill({ json: { id: scenario.id, revision: 1, published_revision: 0, content: scenario } });
  });
  api.handlers.set(endpoint + "/created-transport/revisions", (route) => route.fulfill({ json: [{ revision: 1 }] }));
  await page.locator("#scenario-editor-files-toggle").click();
  await page.locator("#scenario-editor-save").click();
  await expect(page.locator("#scenario-editor-status")).toHaveText("HTTP 502: invalid JSON response");
  await expect(source).toHaveValue(text);
  expect(await page.evaluate(() => Boolean(scenarioEditorArtifact))).toBe(false);
  expect(writes).toHaveLength(1);
  await page.locator("#scenario-editor-files-toggle").click();
  await page.locator("#scenario-editor-save").click();
  await expect(page.locator("#scenario-editor-status")).toHaveText("Saved version 1. Publish it to enable runs.");
  expect(writes).toEqual(Array.from({ length: 2 }, () => ({ scenario, summary: "Initial version", expected_revision: 0 })));
  expect(api.counts.get(endpoint + "/created-transport/publish") || 0).toBe(0);
  expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
});

test("Scenario JSON transport preserves Save Publish and revision payloads without running a scenario", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  const scenario = { ...JSON.parse(await page.locator("#scenario-editor-source").inputValue()), id: "transport-test", name: "Transport test" };
  delete scenario.folder;
  const endpoint = "/api/simulator/organizations/alpha/scenarios/transport-test";
  const updated = { ...scenario, name: "Updated transport test" };
  const writes = [];
  api.handlers.set(endpoint, async (route) => {
    if (route.request().method() === "PUT") {
      writes.push(route.request().postDataJSON());
      await route.fulfill({ json: { revision: 3, content: updated, content_hash: "saved-hash" } });
    } else await route.fulfill({ json: { id: scenario.id, revision: 2, published_revision: 0, content: scenario } });
  });
  api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1 }, { revision: 2 }, { revision: 3 }] }));
  api.handlers.set(endpoint + "/publish", async (route) => {
    writes.push(route.request().postDataJSON());
    await route.fulfill({ json: { id: scenario.id, revision: 3, published_revision: 3, content: updated } });
  });
  api.handlers.set(endpoint + "/revisions/1", (route) => route.fulfill({ json: { revision: 1, content: scenario } }));
  await page.evaluate(() => openExistingScenarioEditor("transport-test"));
  await page.locator("#scenario-editor-name").fill(updated.name);
  await page.locator("#scenario-editor-files-toggle").click();
  await page.locator("#scenario-editor-save").click();
  await expect(page.locator("#scenario-editor-status")).toHaveText("Saved version 3. Publish it to enable runs.");
  await expect(page.locator("#scenario-editor-modified")).toBeHidden();
  await page.locator("#scenario-editor-files-toggle").click();
  await page.locator("#scenario-editor-publish").click();
  await expect(page.locator("#scenario-editor-status")).toHaveText("Published version 3 for alpha.");
  await page.locator("#scenario-editor-history-menu > summary").click();
  await page.locator('#scenario-editor-history [data-revision="1"]').click();
  await expect(page.locator("#scenario-editor-name")).toHaveValue(scenario.name);
  await expect(page.locator("#scenario-editor-modified")).toBeVisible();
  expect(writes).toEqual([{ scenario: updated, summary: "Updated scenario draft", expected_revision: 2 }, { expected_revision: 3 }]);
  expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
});

for (const outcome of ["success", "error"]) {
  for (const change of ["edit-return", "replacement", "record", "organization", "revision", "mode", "close-reopen"]) {
    test(`Scenario form analysis scope rejects delayed ${outcome} after ${change}`, async ({ page, appURL, api }) => {
      await openRawScenario(page, appURL, api);
      await page.locator("#scenario-editor-mode-toggle").click();
      await expect(page.locator("#scenario-editor-name")).toBeVisible();
      const completions = [];
      let oldFinished = false;
      api.handlers.set("/api/simulator/organizations/alpha/scenario-check", async (route) => {
        const index = completions.length;
        const scenario = JSON.parse(route.request().postDataJSON().source);
        expect(route.request().postDataJSON().format).toBe("json");
        await new Promise((resolve) => completions.push(resolve));
        await route.fulfill(index === 0 && outcome === "error"
          ? { status: 503, json: { error: "Obsolete form failure" } }
          : { json: { valid: true, scenario, diagnostics: index === 0 ? "Obsolete form success" : "Fresh form success" } });
        if (index === 0) oldFinished = true;
      });
      await page.locator("#scenario-source-analyze").click();
      await expect.poll(() => completions.length).toBe(1);
      if (change === "edit-return") {
        await page.locator("#scenario-editor-name").fill("Temporary name");
        await page.locator("#scenario-editor-name").fill("");
      } else if (change === "replacement") {
        await page.evaluate(() => renderScenarioEditor({ ...readScenarioEditorForm() }));
      } else if (change === "record") {
        await page.evaluate(() => { scenarioEditorArtifact = { id: "another-scenario", revision: 1 }; });
      } else if (change === "organization") {
        await page.evaluate(() => { scenarioEditorOrganization = "beta"; });
      } else if (change === "revision") {
        await page.evaluate(() => { scenarioEditorViewing = 1; });
      } else if (change === "mode") {
        await page.locator("#scenario-editor-mode-toggle").click();
        await expect(page.locator("#scenario-editor-source")).toBeVisible();
      } else {
        await page.locator("#scenario-editor-files-toggle").click();
        page.once("dialog", (dialog) => dialog.accept());
        await page.locator("#scenario-editor-close").click();
        await expect(page.locator("#scenario-editor-dialog")).not.toHaveAttribute("open");
        await page.locator("#scenario-new").click();
      }
      if (change === "close-reopen") {
        await page.locator("#scenario-source-analyze").click();
        await expect.poll(() => completions.length).toBe(2);
      }
      completions[0]();
      await expect.poll(() => oldFinished).toBe(true);
      await expect(page.locator("#scenario-editor-status")).not.toContainText(/Obsolete form/);
      await expect(page.locator("#scenario-source-analyze")).not.toHaveAttribute("data-analysis-state", /success|error/);
      await expect(page.locator("#scenario-source-gutter button")).toHaveCount(0);
      if (change === "close-reopen") {
        completions[1]();
        await expect(page.locator("#scenario-editor-status")).toHaveText("Fresh form success");
        await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "success");
      }
      expect([...api.counts.keys()].some((path) => /\/(run|publish)$/.test(path))).toBe(false);
    });
  }
  test(`Scenario form analysis scope rejects superseded ${outcome} without source changes`, async ({ page, appURL, api }) => {
    await openRawScenario(page, appURL, api);
    await page.locator("#scenario-editor-mode-toggle").click();
    const completions = [];
    let oldFinished = false;
    api.handlers.set("/api/simulator/organizations/alpha/scenario-check", async (route) => {
      const index = completions.length;
      const scenario = JSON.parse(route.request().postDataJSON().source);
      await new Promise((resolve) => completions.push(resolve));
      await route.fulfill(index === 0 && outcome === "error"
        ? { status: 503, json: { error: "Obsolete form failure" } }
        : { json: { valid: true, scenario, diagnostics: index === 0 ? "Obsolete form success" : "Fresh form success" } });
      if (index === 0) oldFinished = true;
    });
    await page.locator("#scenario-source-analyze").click();
    await expect.poll(() => completions.length).toBe(1);
    await page.locator("#scenario-source-analyze").click();
    await expect.poll(() => completions.length).toBe(2);
    completions[1]();
    await expect(page.locator("#scenario-editor-status")).toHaveText("Fresh form success");
    completions[0]();
    await expect.poll(() => oldFinished).toBe(true);
    await expect(page.locator("#scenario-editor-status")).toHaveText("Fresh form success");
    await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "success");
  });
}

for (const outcome of ["success", "error"]) {
  test(`Scenario raw analysis scope rejects delayed ${outcome} after editor close`, async ({ page, appURL, api }) => {
    await openRawScenario(page, appURL, api);
    let complete;
    let finished = false;
    api.handlers.set("/api/simulator/organizations/alpha/scenario-check", async (route) => {
      const scenario = JSON.parse(route.request().postDataJSON().source);
      await new Promise((resolve) => { complete = resolve; });
      await route.fulfill(outcome === "success"
        ? { json: { valid: true, scenario, diagnostics: "Obsolete raw success" } }
        : { status: 422, json: { error: "Obsolete raw failure", line: 1 } });
      finished = true;
    });
    await page.locator("#scenario-source-analyze").click();
    await expect.poll(() => Boolean(complete)).toBe(true);
    await page.locator("#scenario-editor-files-toggle").click();
    await page.locator("#scenario-editor-close").click();
    await expect(page.locator("#scenario-editor-dialog")).not.toHaveAttribute("open");
    complete();
    await expect.poll(() => finished).toBe(true);
    await expect(page.locator("#scenario-editor-status")).not.toContainText(/Obsolete raw/);
    await expect(page.locator("#scenario-source-gutter button")).toHaveCount(0);
  });
}

test("GUI raw scenario editor has safe syntax colors, aligned error gutter, and form round trips", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  const source = page.locator("#scenario-editor-source");
  const scenario = JSON.parse(await source.inputValue());
  scenario.description = "<img src=x> JSON only";
  scenario.parallel = true;
  scenario.steps[0].id = "retained-step";
  scenario.steps[0].after = [];
  await source.fill(JSON.stringify(scenario));
  await expect(page.locator(".scenario-builder-grid")).toBeHidden();
  await expect(page.locator("#scenario-source-highlight .zpl-string").filter({ hasText: "<img src=x>" })).toHaveCSS("color", "rgb(215, 167, 207)");
  await expect(page.locator("#scenario-source-highlight img")).toHaveCount(0);
  await expect(page.locator("#scenario-source-highlight .zpl-attribute").first()).toHaveCSS("color", "rgb(184, 215, 139)");
  await expect(page.locator("#scenario-source-gutter")).toHaveCSS("border-right-style", "solid");
  await expect(page.locator("#scenario-editor-files-toggle")).toBeVisible();
  await expect(page.locator("#scenario-editor-title")).toHaveText("Untitled");
  await expect(page.locator("#scenario-editor-revision-label")).toBeHidden();
  await expect(page.locator("#scenario-editor-dialog .policy-identity > #scenario-editor-history-menu")).toBeVisible();
  await expect(page.locator("#scenario-editor-modified")).toBeVisible();
  await page.locator("#scenario-source-analyze").click();
  await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "success");
  await expect(page.locator("#scenario-editor-status")).toContainText("Nothing saved, published, or run");
  await page.locator("[data-source-format]").click();
  await expect(source).toHaveValue(JSON.stringify(scenario, null, 2));
  await expect(page.locator("#scenario-source-analyze")).not.toHaveAttribute("data-analysis-state", "success");
  await page.locator("#scenario-editor-mode-toggle").click();
  await expect(page.locator("#scenario-editor-description")).toHaveValue(scenario.description);
  await page.locator("#scenario-editor-name").fill("Round trip");
  await page.locator("#scenario-editor-mode-toggle").click();
  const updated = JSON.parse(await source.inputValue());
  expect(updated.name).toBe("Round trip");
  expect(updated.steps[0].id).toBe("retained-step");
  await source.fill('{\n  "id": "new-scenario",\n  "steps": [}\n');
  await page.locator("#scenario-source-analyze").click();
  await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "error");
  const marker = page.locator("#scenario-source-gutter button");
  await expect(marker).toBeVisible();
  await expect(page.locator("#scenario-editor-status")).toBeHidden();
  await marker.click();
  await expect(source).toBeFocused();
  await page.locator("#scenario-editor-mode-toggle").click();
  await expect(page.locator("#scenario-editor-advanced")).toHaveAttribute("open", "");
  await expect(page.locator("#scenario-editor-status")).toBeHidden();
  await source.fill(JSON.stringify(updated, null, 2));
  await expect(marker).toHaveCount(0);
  await source.press("Tab");
  expect(api.counts.get("/api/simulator/organizations/alpha/scenarios") || 0).toBe(0);
});

test("GUI raw scenario source scroll tracks highlighting and preserves unavailable assistant state", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  await page.locator(".editor-wrap-toggle:visible input").uncheck();
  const source = page.locator("#scenario-editor-source");
  await source.fill(Array.from({ length: 100 }, (_, index) => `  "key${index}": "${"x".repeat(180)}",`).join("\n"));
  await source.evaluate((element) => { element.scrollTop = 300; element.scrollLeft = 70; element.dispatchEvent(new Event("scroll")); });
  const geometry = await page.evaluate(() => {
    const source = document.getElementById("scenario-editor-source");
    const highlight = document.getElementById("scenario-source-highlight");
    const gutter = document.getElementById("scenario-source-gutter");
    return {
      source: [source.scrollTop, source.scrollLeft], highlight: [highlight.scrollTop, highlight.scrollLeft],
      gutter: gutter.scrollTop, rowHeight: document.querySelector("#scenario-source-gutter .config-gutter-line").getBoundingClientRect().height,
      lineHeight: Number.parseFloat(getComputedStyle(source).lineHeight),
    };
  });
  expect(geometry.highlight).toEqual(geometry.source);
  expect(geometry.gutter).toBe(geometry.source[0]);
  expect(Math.abs(geometry.rowHeight - geometry.lineHeight)).toBeLessThan(1);
  await expect(page.locator("#scenario-assistant-slot [data-assistant-state]")).toHaveText("Unavailable");
  await expect(page.locator("#scenario-assistant-slot [data-assistant-submit]")).toBeDisabled();
  await expect(page.locator("#scenario-assistant-slot")).toContainText("sends the current organization/scenario context");
});

test("GUI raw scenario assistant uses exact raw context and applies only unsaved proposals in either mode", async ({ page, appURL, api }) => {
  let request;
  api.handlers.set("/api/simulator/design-assistant", async (route) => {
    request = route.request().postDataJSON();
    await route.fulfill({ json: {
      answer: "Reviewed.", proposal: { scenario: { ...request.scenario, name: "Assistant draft" } }, input_tokens: 10, output_tokens: 20,
    } });
  });
  await openRawScenario(page, appURL, api, true);
  const source = page.locator("#scenario-editor-source");
  const scenario = JSON.parse(await source.inputValue());
  scenario.name = "Raw request";
  await source.fill(JSON.stringify(scenario, null, 2));
  const assistant = page.locator("#scenario-assistant-slot");
  await assistant.locator("[data-assistant-question]").fill("Review this draft");
  await assistant.locator("[data-assistant-submit]").click();
  await expect(assistant.locator("[data-assistant-apply]")).toBeVisible();
  expect(request.scenario.name).toBe("Raw request");
  await assistant.locator("[data-assistant-apply]").click();
  expect(JSON.parse(await source.inputValue()).name).toBe("Assistant draft");
  await expect(page.locator("#scenario-editor-modified")).toBeVisible();
  await expect(page.locator("#scenario-editor-publish")).toBeDisabled();
  await page.locator("#scenario-editor-mode-toggle").click();
  await expect(page.locator("#scenario-editor-name")).toHaveValue("Assistant draft");
  await assistant.locator("[data-assistant-question]").fill("Review form too");
  await assistant.locator("[data-assistant-submit]").click();
  await expect(assistant.locator("[data-assistant-apply]")).toBeVisible();
  await assistant.locator("[data-assistant-apply]").click();
  expect(request.scenario.name).toBe("Assistant draft");
  expect(api.counts.get("/api/simulator/organizations/alpha/scenarios") || 0).toBe(0);
});

test("GUI raw scenario assistant rejects proposals after source changes", async ({ page, appURL, api }) => {
  let complete;
  api.handlers.set("/api/simulator/design-assistant", async (route) => {
    const request = route.request().postDataJSON();
    await new Promise((resolve) => { complete = resolve; });
    await route.fulfill({ json: { answer: "Old draft.", proposal: { scenario: { ...request.scenario, name: "Old proposal" } } } });
  });
  await openRawScenario(page, appURL, api, true);
  const assistant = page.locator("#scenario-assistant-slot");
  await assistant.locator("[data-assistant-question]").fill("Review");
  await assistant.locator("[data-assistant-submit]").click();
  await expect.poll(() => Boolean(complete)).toBe(true);
  const source = page.locator("#scenario-editor-source");
  const scenario = JSON.parse(await source.inputValue());
  scenario.name = "Newer edits";
  await source.fill(JSON.stringify(scenario));
  complete();
  await expect(assistant).toContainText("editor changed while Claude was responding");
  await expect(assistant.locator("[data-assistant-apply]")).toBeHidden();
  expect(JSON.parse(await source.inputValue()).name).toBe("Newer edits");
});

test("GUI raw scenario file import, download, and identity checks never save implicitly", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  const source = page.locator("#scenario-editor-source");
  const imported = { id: "new-scenario", organization_id: "alpha", name: "Imported JSON", description: "Local", steps: [{ action: "delay", timeout_seconds: 1 }], cleanup: [] };
  await page.locator("#scenario-editor-files-toggle").click();
  await page.locator("[data-source-open]").click();
  await page.locator("[data-source-file]").setInputFiles({ name: "draft.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(imported)) });
  await expect(page.locator("#scenario-editor-status")).toContainText("Opened draft.json");
  await page.locator("#scenario-source-analyze").click();
  await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "success");
  await page.locator("#scenario-editor-files-toggle").click();
  const downloading = page.waitForEvent("download");
  await page.locator("[data-source-download]").click();
  expect((await downloading).suggestedFilename()).toBe("new-scenario.json");
  await source.fill(JSON.stringify({ ...imported, organization_id: "beta" }));
  await page.locator("#scenario-source-analyze").click();
  await expect(page.locator("#scenario-editor-status")).toContainText("organization_id must match");
  await expect(page.locator("#scenario-source-gutter button")).toHaveCount(0);
  await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "error");
  expect(api.counts.get("/api/simulator/organizations/alpha/scenarios") || 0).toBe(0);
});

test("GUI raw scenario analysis ignores delayed errors for an edited source", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  let complete;
  api.handlers.set("/api/simulator/organizations/alpha/scenario-check", async (route) => {
    await new Promise((resolve) => { complete = resolve; });
    await route.fulfill({ status: 422, json: { valid: false, line: 2, error: "Old syntax error" } });
  });
  const source = page.locator("#scenario-editor-source");
  await source.fill('{\n"old": }');
  await page.locator("#scenario-source-analyze").click();
  await expect.poll(() => Boolean(complete)).toBe(true);
  await source.fill('{"id":"new-scenario","name":"Current","steps":[]}');
  complete();
  await expect(page.locator("#scenario-editor-status")).toBeHidden();
  await expect(page.locator("#scenario-source-gutter button")).toHaveCount(0);
  await expect(page.locator("#scenario-source-analyze")).not.toHaveAttribute("data-analysis-state", "error");
});

test("GUI raw scenario assistant discards a proposal changed before Apply and after resetting the editor", async ({ page, appURL, api }) => {
  let complete;
  let delay = false;
  api.handlers.set("/api/simulator/design-assistant", async (route) => {
    const request = route.request().postDataJSON();
    if (delay) await new Promise((resolve) => { complete = resolve; });
    await route.fulfill({ json: { answer: "Old conversation.", proposal: { scenario: { ...request.scenario, name: "Old proposal" } } } });
  });
  await openRawScenario(page, appURL, api, true);
  const assistant = page.locator("#scenario-assistant-slot");
  await assistant.locator("[data-assistant-question]").fill("Review");
  await assistant.locator("[data-assistant-submit]").click();
  await expect(assistant.locator("[data-assistant-apply]")).toBeVisible();
  const source = page.locator("#scenario-editor-source");
  const scenario = JSON.parse(await source.inputValue());
  scenario.name = "Newer source";
  await source.fill(JSON.stringify(scenario));
  await assistant.locator("[data-assistant-apply]").click();
  await expect(assistant).toContainText("editor changed after this proposal");
  expect(JSON.parse(await source.inputValue()).name).toBe("Newer source");
  delay = true;
  await assistant.locator("[data-assistant-question]").fill("Review again");
  await assistant.locator("[data-assistant-submit]").click();
  await expect.poll(() => Boolean(complete)).toBe(true);
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator("#scenario-editor-dialog").getByRole("button", { name: "Close editor" }).click();
  await expect(page.locator("#scenario-editor-dialog")).toBeHidden();
  await page.locator("#scenario-new").click();
  complete();
  await expect(assistant.locator("[data-assistant-thread]")).not.toContainText("Old conversation");
  await expect(assistant.locator("[data-assistant-apply]")).toBeHidden();
});

for (const operation of ["open", "history", "revision", "save", "publish"]) {
  test(`Directory JSON transport reports malformed ${operation} responses without replacing source or retrying mutations`, async ({ page, appURL, api }) => {
    const endpoint = "/api/simulator/organizations/alpha/directory";
    const original = "dn: dc=alpha,dc=test\n";
    const artifact = { revision: 2, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: original } };
    const bad = (route) => route.fulfill({ status: 502, contentType: "text/html", body: "upstream unavailable" });
    api.handlers.set(endpoint, (route) => route.fulfill({ json: artifact }));
    api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1, summary: "Initial" }, { revision: 2, summary: "Current" }] }));
    const paths = { open: endpoint, history: endpoint + "/revisions", revision: endpoint + "/revisions/1", save: endpoint, publish: endpoint + "/publish" };
    if (operation === "open" || operation === "history") api.handlers.set(paths[operation], bad);
    await page.goto(appURL + "/organizations.html");
    await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
    const source = page.locator("#directory-editor-source");
    let expected = original;
    if (operation === "open") {
      await expect(page.locator("#organization-error")).toHaveText("HTTP 502: invalid JSON response");
      await expect(page.locator("#directory-editor-dialog")).toBeHidden();
    } else {
      await expect(source).toHaveValue(original);
      if (operation !== "history") {
        api.handlers.set(paths[operation], bad);
        if (operation === "revision") {
          await page.locator("#directory-editor-history-menu > summary").click();
          await page.locator('#directory-editor-history [data-revision="1"]').click();
        } else {
          if (operation === "save") {
            expected = original + "description: Unsaved\n";
            await source.fill(expected);
          }
          await page.locator("#directory-editor-files-toggle").click();
          await page.locator(`#directory-editor-${operation}`).click();
        }
      }
      await expect(page.locator(operation === "history" ? "#organization-error" : "#directory-editor-status")).toHaveText("HTTP 502: invalid JSON response");
      await expect(source).toHaveValue(expected);
      await expect(page.locator("#directory-editor-title")).toHaveText("Alpha Labs directory");
      if (operation === "save") await expect(page.locator("#directory-editor-modified")).toBeVisible();
      await expect(page.locator("#directory-editor-gutter button")).toHaveCount(0);
    }
    expect(api.counts.get(paths[operation])).toBe(operation === "save" ? 2 : 1);
  });
}

test("Directory JSON transport preserves save, publish and revision contracts without reseeding LDAP", async ({ page, appURL, api }) => {
  const endpoint = "/api/simulator/organizations/alpha/directory";
  const original = "dn: dc=alpha,dc=test\n";
  const draft = original + "description: Updated\n";
  const content = { base_dn: "dc=alpha,dc=test", ldif: draft };
  const writes = [];
  api.handlers.set(endpoint, async (route) => {
    if (route.request().method() === "PUT") {
      writes.push(route.request().postDataJSON());
      await route.fulfill({ json: { revision: 3, content, content_hash: "saved-hash" } });
    } else await route.fulfill({ json: { revision: 2, published_revision: 0, content: { ...content, ldif: original } } });
  });
  api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1 }, { revision: 2 }, { revision: 3 }] }));
  api.handlers.set(endpoint + "/publish", async (route) => {
    writes.push(route.request().postDataJSON());
    await route.fulfill({ json: { artifact: { revision: 3, published_revision: 3, content } } });
  });
  api.handlers.set(endpoint + "/revisions/1", (route) => route.fulfill({ json: { revision: 1, content: { ...content, ldif: original } } }));
  await page.goto(appURL + "/organizations.html");
  await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
  const source = page.locator("#directory-editor-source");
  await expect(source).toHaveValue(original);
  await source.fill(draft);
  await page.locator("#directory-editor-files-toggle").click();
  await page.locator("#directory-editor-save").click();
  await expect(page.locator("#directory-editor-status")).toHaveText("Saved version 3. Publish it for the next LDAP reseed.");
  await expect(page.locator("#directory-editor-modified")).toBeHidden();
  await page.locator("#directory-editor-files-toggle").click();
  await page.locator("#directory-editor-publish").click();
  await expect(page.locator("#directory-editor-status")).toContainText("Published version 3; applies on the next explicit LDAP reseed or rig restart.");
  await page.locator("#directory-editor-history-menu > summary").click();
  await page.locator('#directory-editor-history [data-revision="1"]').click();
  await expect(source).toHaveValue(original);
  await expect(page.locator("#directory-editor-modified")).toBeVisible();
  expect(writes).toEqual([{ document: content, expected_revision: 2, summary: "Updated directory draft" }, { expected_revision: 3 }]);
  expect([...api.counts.keys()].some((path) => /reseed|restore-base|activate/.test(path))).toBe(false);
});

test("GUI editor search replaces directory source locally and keeps revision controls", async ({ page, appURL, api }) => {
  const endpoint = "/api/simulator/organizations/alpha/directory";
  api.handlers.set(endpoint, (route) => route.fulfill({ json: {
    revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: cn=Operators,dc=alpha,dc=test\ncn: Operators\n" },
  } }));
  api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1, summary: "Initial" }] }));
  await page.goto(appURL + "/organizations.html");
  await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
  await expect(page.locator("#directory-editor-publish")).toBeEnabled();
  const tools = page.locator('[data-editor-search-target="directory-editor-source"]');
  await tools.getByRole("button", { name: "Find & Replace", exact: true }).click();
  await tools.getByRole("textbox", { name: "Find what" }).fill("Operators");
  await tools.getByRole("textbox", { name: "Replace with" }).fill("Reviewers");
  await tools.getByRole("button", { name: "Replace all" }).click();
  await expect(page.locator("#directory-editor-source")).toHaveValue("dn: cn=Reviewers,dc=alpha,dc=test\ncn: Reviewers\n");
  await expect(page.locator("#directory-editor-publish")).toBeDisabled();
  await expect(tools.getByRole("status")).toContainText("Changes are unsaved");
  await expect(page.locator("#directory-editor-history-menu")).toBeVisible();
  await expect(page.locator("#directory-editor-history-count")).toHaveText("1 version");
  await expect(page.locator("#directory-editor-modified")).toBeVisible();
  expect(api.counts.get(endpoint)).toBe(1);
});

test("GUI Simulator directory editor opens as a Policy-blueprint page without idle text", async ({ page, appURL, api }) => {
  const endpoint = "/api/simulator/organizations/alpha/directory";
  api.handlers.set(endpoint, (route) => route.fulfill({ json: {
    revision: 2, published_revision: 1, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: cn=Operators,dc=alpha,dc=test\ncn: Operators\n" },
  } }));
  api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1, summary: "Initial" }, { revision: 2, summary: "Edit" }] }));
  await page.goto(appURL + "/organizations.html");
  await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
  const editor = page.locator("#directory-editor-dialog");
  await expect(editor).toBeVisible();
  await expect(page.locator("#directory-editor-title")).toHaveText("Alpha Labs directory");
  await expect(page.locator("#directory-editor-revision-label")).toBeHidden();
  await expect(page.locator("#directory-editor-dialog .policy-identity > #directory-editor-history-menu")).toBeVisible();
  await expect(page.locator("#directory-editor-status")).toBeHidden();
  await expect(page.getByRole("button", { name: "Edit LDAP seed", exact: true })).toBeHidden();
  await page.locator("#directory-editor-files-toggle").click();
  await expect(page.getByRole("menuitem", { name: "Save" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Publish for next reseed" })).toBeVisible();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Close directory editor" }).click();
  await expect(editor).toBeHidden();
  await expect(page.getByRole("button", { name: "Edit LDAP seed", exact: true })).toBeVisible();
});

test("GUI editor search reveals distant matches and prevents accidental form submission", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#zpr-config");
  await page.locator("#page-zpr-config").getByRole("checkbox", { name: "Wrap", exact: true }).uncheck();
  const source = page.locator("#zpr-config-source");
  await source.fill(Array.from({ length: 100 }, (_, index) => index === 90 ? `key = "${"x".repeat(200)}NEEDLE"` : `key${index} = "value"`).join("\n"));
  await source.evaluate((element) => { element.scrollTop = 0; element.scrollLeft = 0; element.setSelectionRange(0, 0); });
  await source.press("Control+f");
  const tools = page.locator('[data-editor-search-target="zpr-config-source"]');
  await tools.getByRole("textbox", { name: "Find what" }).fill("NEEDLE");
  await tools.getByRole("textbox", { name: "Find what" }).press("Enter");
  await expect(tools.getByRole("status")).toHaveText("Match 1 of 1");
  const position = await source.evaluate((element) => ({
    scrollTop: element.scrollTop, scrollLeft: element.scrollLeft,
    selected: element.value.slice(element.selectionStart, element.selectionEnd),
    lineTop: 90 * Number.parseFloat(getComputedStyle(element).lineHeight) + Number.parseFloat(getComputedStyle(element).paddingTop) - element.scrollTop,
    height: element.clientHeight,
  }));
  expect(position.scrollTop).toBeGreaterThan(0);
  expect(position.lineTop).toBeGreaterThanOrEqual(0);
  expect(position.lineTop).toBeLessThan(position.height);
  expect(position.scrollLeft).toBeGreaterThan(0);
  expect(position.selected).toBe("NEEDLE");
  await source.press("Escape");
  await expect(tools.getByRole("search")).toBeHidden();
});

test("GUI topology keeps dock rays distinct from inter-node links and paints network links last", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node-a", zpr_addr: "fd00::1", node: true, node_details: { adapters: ["adapter-a", "adapter-b", "adapter-c", "adapter-d"] } },
    { cn: "node-b", zpr_addr: "fd00::2", node: true, node_details: { adapters: [] } },
    ...["adapter-a", "adapter-b", "adapter-c", "adapter-d"].map((cn) => ({ cn, node: false })),
  ];
  api.snapshot.network = [{ node_a_addr: "fd00::1", node_b_addr: "fd00::2", ctype: "UP" }];
  await page.goto(appURL + "/#map");
  await expect(page.locator(".graph-network-clearance")).toHaveCount(1);
  const geometry = await page.evaluate(() => {
    const edges = [...document.querySelectorAll(".graph-edge")];
    const angle = (line) => Math.atan2(Number(line.getAttribute("y2")) - Number(line.getAttribute("y1")), Number(line.getAttribute("x2")) - Number(line.getAttribute("x1")));
    const network = edges.find((edge) => !edge.dataset.dockAdapter);
    const direction = angle(network.querySelector(".graph-link"));
    return {
      last: edges.at(-1) === network,
      separation: Math.min(...edges.filter((edge) => edge.dataset.dockAdapter).map((edge) => {
        const delta = angle(edge.querySelector(".graph-link")) - direction;
        return Math.abs(Math.atan2(Math.sin(delta), Math.cos(delta)));
      })),
    };
  });
  expect(geometry.last).toBe(true);
  expect(geometry.separation).toBeGreaterThan(0.2);
});

test("Control Room keeps Adapter Logs internal, Log Manager beneath it, and a Configuration group", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  if (await page.locator("body").evaluate((body) => body.classList.contains("sidebar-condensed"))) {
    await page.getByRole("button", { name: "Expand side menu", exact: true }).click();
  }
  const adapterLogs = page.getByRole("link", { name: "Adapter Logs", exact: true });
  await expect(adapterLogs).toHaveAttribute("href", "#adapter-logs");
  await expect(adapterLogs).not.toHaveAttribute("target");
  await adapterLogs.click();
  await expect(page.locator("#page-adapter-logs")).toBeVisible();
  const manager = page.locator(".primary-nav .sidebar-external-link");
  await expect(manager).toContainText("Log Manager");
  await expect(manager.locator(".external-arrow")).toHaveCSS("color", "rgb(181, 227, 79)");
  const labels = await page.locator(".primary-nav").evaluate((nav) => [...nav.querySelectorAll(".nav-link, .nav-group-label")].map((item) => item.textContent.replace("↗", "").trim()));
  expect(labels).toEqual(["Monitoring", "Map", "Status", "Security", "Diagnostics", "Trusted Sources", "Adapter Logs", "Log Manager", "Configuration", "Policy/Assertions", "Gateways", "Config", "Provisioning", "Adapters"]);
  await expect(page.getByRole("group", { name: "Monitoring" }).getByRole("link")).toHaveCount(7);
  await expect(page.getByRole("group", { name: "Configuration" }).getByRole("link")).toHaveCount(3);
  await expect(page.getByRole("group", { name: "Provisioning" }).getByRole("link", { name: "Adapters", exact: true })).toHaveAttribute("href", "#provisioning-adapters");
  await expect(manager).toHaveAttribute("href", "http://127.0.0.1:8800/");
  await expect(manager).toHaveAttribute("target", "zpr-log-manager");
});

test("Log Manager click focuses its reusable named window", async ({ page, appURL }) => {
  await page.addInitScript(() => {
    window.__logManagerOpen = null;
    window.open = (url, target) => {
      const result = { opener: window, focused: false, focus() { this.focused = true; } };
      window.__logManagerOpen = { url, target, result };
      return result;
    };
  });
  await page.goto(appURL + "/#map");
  await page.locator('.sidebar-external-link[data-reuse-window="zpr-log-manager"]').evaluate((link) => link.click());
  const opened = await page.evaluate(() => window.__logManagerOpen && ({
    url: window.__logManagerOpen.url,
    target: window.__logManagerOpen.target,
    focused: window.__logManagerOpen.result.focused,
    openerCleared: window.__logManagerOpen.result.opener === null,
  }));
  expect(opened).toEqual({ url: "http://127.0.0.1:8800/", target: "zpr-log-manager", focused: true, openerCleared: true });
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

test("shared File menu skips hidden and disabled actions and does not capture outside arrow keys", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  await page.evaluate(() => {
    const root = document.createElement("div");
    root.id = "test-file-actions";
    root.innerHTML = '<button id="test-file-toggle">File</button><div id="test-file-menu" role="menu" hidden><button hidden>Hidden</button><button disabled>Disabled</button><div hidden><button>Hidden parent</button></div><button id="test-file-first">First</button><button id="test-file-last">Last</button></div><input id="test-file-outside">';
    document.querySelector("#page-map").append(root);
    window.ZPREditorPage.createMenu({ root, toggle: root.querySelector("#test-file-toggle"), menu: root.querySelector("#test-file-menu") });
  });
  await page.locator("#test-file-toggle").click();
  await expect(page.locator("#test-file-first")).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await expect(page.locator("#test-file-last")).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(page.locator("#test-file-first")).toBeFocused();
  await page.locator("#test-file-outside").focus();
  expect(await page.locator("#test-file-outside").evaluate((input) => input.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true, cancelable: true })))).toBe(true);
  await expect(page.locator("#test-file-outside")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.locator("#test-file-menu")).toBeHidden();
  await expect(page.locator("#test-file-toggle")).toBeFocused();
  await page.locator("#test-file-toggle").click();
  await page.locator("#test-file-first").click();
  await expect(page.locator("#test-file-menu")).toBeHidden();
});

for (const editor of [
  { name: "Policy", prefix: "policy", path: "/#policy" },
  { name: "Assertions", prefix: "policy", path: "/#policy" },
  { name: "Config", prefix: "zpr-config", path: "/#zpr-config" },
  { name: "Gateway", prefix: "gateway", path: "/#gateways" },
  { name: "Directory", prefix: "directory-editor", path: "/organizations.html" },
  { name: "Scenario", prefix: "scenario-editor", path: "/scenarios.html" },
]) {
  test(`${editor.name} shared File menu supports keyboard navigation and focus restoration`, async ({ page, appURL, api }) => {
    if (editor.name === "Gateway") {
      api.handlers.set("/api/gateways/contracts", (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [
        { organization_id: "alpha", instance_id: "egress", adapter_cn: "gateway-egress", service_name: "egress.svc.zpr" },
      ] } }));
      api.handlers.set("/api/gateways/configs", (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
    }
    if (editor.name === "Directory") {
      api.handlers.set("/api/simulator/organizations/alpha/directory", (route) => route.fulfill({ json: {
        revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" },
      } }));
      api.handlers.set("/api/simulator/organizations/alpha/directory/revisions", (route) => route.fulfill({ json: [] }));
    }
    if (editor.name === "Scenario") await openRawScenario(page, appURL, api);
    else if (editor.name === "Assertions") await openAssertionRecord(page, appURL);
    else await page.goto(appURL + editor.path);
    if (editor.name === "Directory") await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
    if (editor.name === "Gateway") {
      const source = page.locator("#gateway-source");
      await expect(source).toHaveValue(/"instance_id": "egress"/);
      await source.fill(JSON.stringify({ ...JSON.parse(await source.inputValue()), description: "Menu navigation draft" }, null, 2));
    }
    const toggle = page.locator(`#${editor.prefix}-files-toggle`);
    const menu = page.locator(`#${editor.prefix}-file-menu`);
    await toggle.click();
    await expect(menu).toBeVisible();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    const items = menu.locator("button:visible:enabled");
    expect(await items.count()).toBeGreaterThan(0);
    await expect(items.first()).toBeFocused();
    await page.keyboard.press("End");
    await expect(items.last()).toBeFocused();
    await page.keyboard.press("ArrowDown");
    await expect(items.first()).toBeFocused();
    await page.keyboard.press("ArrowUp");
    await expect(items.last()).toBeFocused();
    await page.keyboard.press("Home");
    await expect(items.first()).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toBeFocused();
    await toggle.click();
    await page.locator(`#${editor.name === "Assertions" ? "assertion-source" : editor.name === "Policy" ? "policy-source" : editor.name === "Config" ? "zpr-config-source" : editor.name === "Gateway" ? "gateway-source" : `${editor.prefix}-source`}`).click();
    await expect(menu).toBeHidden();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
  });
}

test("shared analysis scope rejects superseded runs, context changes and edit-return cycles", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  const states = await page.evaluate(() => {
    let context = ["source", "record", "organization", 1, "json"];
    const scope = window.ZPREditorPage.createAnalysisScope(() => context);
    const first = scope.begin();
    const initial = first();
    const second = scope.begin();
    const superseded = first();
    const latest = second();
    scope.invalidate();
    const invalidated = second();
    const changes = context.map((_, index) => {
      const run = scope.begin();
      const original = context[index];
      context[index] = `${original}-changed`;
      const changed = run();
      context[index] = original;
      return changed;
    });
    const edit = scope.begin();
    context[0] = "edited";
    scope.invalidate();
    context[0] = "source";
    const returned = edit();
    const loaded = scope.begin();
    context = [...context];
    return { initial, superseded, latest, invalidated, changes, returned, sameValues: loaded() };
  });
  expect(states).toEqual({ initial: true, superseded: false, latest: true, invalidated: false, changes: [false, false, false, false, false], returned: false, sameValues: true });
});

for (const outcome of ["success", "error"]) {
  test(`Gateway analysis scope rejects delayed ${outcome} after editing back to the original source`, async ({ page, appURL, api }) => {
    api.handlers.set("/api/gateways/contracts", (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [
      { organization_id: "alpha", instance_id: "egress", adapter_cn: "gateway-egress", service_name: "egress.svc.zpr" },
    ] } }));
    api.handlers.set("/api/gateways/configs", (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
    let complete;
    api.handlers.set("/api/gateways/config/check", async (route) => {
      await new Promise((resolve) => { complete = resolve; });
      await route.fulfill({ status: outcome === "success" ? 200 : 422, json: {
        valid: outcome === "success", diagnostics: "Obsolete gateway analysis", error: "Obsolete gateway analysis",
      } });
    });
    await page.goto(appURL + "/#gateways");
    const source = page.locator("#gateway-source");
    await expect(source).toHaveValue(/"instance_id": "egress"/);
    const original = await source.inputValue();
    const analyzed = JSON.stringify({ ...JSON.parse(original), description: "Draft needing analysis" }, null, 2);
    await source.fill(analyzed);
    const button = page.locator("#gateway-analyze");
    await button.click();
    await expect.poll(() => Boolean(complete)).toBe(true);
    await source.fill(analyzed + "\n");
    await source.fill(analyzed);
    complete();
    await expect(button).toBeEnabled();
    await expect(button).not.toHaveAttribute("data-analysis-state", /.+/);
    await expect(page.locator("#gateway-draft-message")).toBeHidden();
    await expect(page.locator("#gateway-gutter button")).toHaveCount(0);
    await page.locator("#gateway-files-toggle").click();
    await expect(page.locator("#gateway-save")).toBeDisabled();
    expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
  });

  test(`Scenario analysis scope rejects delayed ${outcome} after editing back to the original source`, async ({ page, appURL, api }) => {
    await openRawScenario(page, appURL, api);
    const source = page.locator("#scenario-editor-source");
    const original = await source.inputValue();
    let complete;
    api.handlers.set("/api/simulator/organizations/alpha/scenario-check", async (route) => {
      await new Promise((resolve) => { complete = resolve; });
      await route.fulfill({ status: outcome === "success" ? 200 : 422, json: {
        valid: outcome === "success", diagnostics: "Obsolete scenario analysis", error: "Obsolete scenario analysis", line: 2,
        scenario: JSON.parse(original),
      } });
    });
    const analysis = page.evaluate(() => window.analyzeScenarioSourceEditor());
    await expect.poll(() => Boolean(complete)).toBe(true);
    await source.fill(original + "\n");
    await source.fill(original);
    complete();
    expect(await analysis).toBeNull();
    await expect(source).toHaveValue(original);
    await expect(page.locator("#scenario-editor-status")).toBeHidden();
    await expect(page.locator("#scenario-source-gutter button")).toHaveCount(0);
    await expect(page.locator("#scenario-source-analyze")).not.toHaveAttribute("data-analysis-state", /success|error/);
  });
}

test("Gateway analysis scope rejects a delayed result after switching installed gateways", async ({ page, appURL, api }) => {
  api.handlers.set("/api/gateways/contracts", (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [
    { organization_id: "alpha", instance_id: "first", adapter_cn: "gateway-first", service_name: "first.svc.zpr" },
    { organization_id: "alpha", instance_id: "second", adapter_cn: "gateway-second", service_name: "second.svc.zpr" },
  ] } }));
  api.handlers.set("/api/gateways/configs", (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
  let complete;
  api.handlers.set("/api/gateways/config/check", async (route) => {
    await new Promise((resolve) => { complete = resolve; });
    await route.fulfill({ json: { valid: true, diagnostics: "Old gateway valid" } });
  });
  await page.goto(appURL + "/#gateways");
  const source = page.locator("#gateway-source");
  await expect(source).toHaveValue(/"instance_id": "first"/);
  await page.locator("#gateway-analyze").click();
  await expect.poll(() => Boolean(complete)).toBe(true);
  await page.locator("#gateway-picker-toggle").click();
  await page.locator('#gateway-contracts [data-instance-id="second"]').click();
  await expect(source).toHaveValue(/"instance_id": "second"/);
  complete();
  await expect(page.locator("#gateway-analyze")).toBeEnabled();
  await expect(page.locator("#gateway-analyze")).not.toHaveAttribute("data-analysis-state", /.+/);
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
  await expect(page.locator("#gateway-record-title")).toHaveText("second.svc.zpr");
});

for (const action of ["Analyze", "Format"]) {
  for (const outcome of ["success", "error"]) {
    test(`Config analysis scope rejects delayed ${outcome} for ${action} after an edit-return cycle`, async ({ page, appURL, api }) => {
      let complete;
      api.handlers.set("/api/policy/config/check", async (route) => {
        await new Promise((resolve) => { complete = resolve; });
        await route.fulfill({ status: outcome === "success" ? 200 : 422, json: {
          valid: outcome === "success", diagnostics: "Obsolete config analysis",
          error: "Obsolete config analysis", line: 1,
        } });
      });
      await page.goto(appURL + "/#zpr-config");
      const source = page.locator("#zpr-config-source");
      const original = 'name="Original"\n';
      await source.fill(original);
      await page.getByRole("button", { name: action, exact: true }).click();
      await expect.poll(() => Boolean(complete)).toBe(true);
      await source.fill('name="Replacement"\n');
      await source.fill(original);
      complete();
      await expect(page.locator("#zpr-config-validate")).toBeEnabled();
      await expect(source).toHaveValue(original);
      await expect(page.locator("#zpr-config-validate")).not.toHaveAttribute("data-analysis-state", /.+/);
      await expect(page.locator("#zpr-config-status")).toBeHidden();
      await expect(page.locator("#zpr-config-gutter button")).toHaveCount(0);
      api.handlers.set("/api/policy/config/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Current config valid" } }));
      await page.getByRole("button", { name: action, exact: true }).click();
      await expect(page.locator("#zpr-config-validate")).toBeEnabled();
      if (action === "Analyze") {
        await expect(page.locator("#zpr-config-validate")).toHaveAttribute("data-analysis-state", "success");
        await expect(page.locator("#zpr-config-status")).toHaveText("Current config valid");
      } else {
        await expect(source).toHaveValue('name = "Original"\n');
      }
      expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
    });
  }
}

for (const phase of ["compiler", "fixtures", "evaluation"]) {
  test(`Policy JSON transport reports malformed ${phase} responses without source markers or mutation retries`, async ({ page, appURL, api }) => {
    const paths = { compiler: "/api/policy/check", fixtures: "/api/policy/test/fixtures", evaluation: "/api/policy/test" };
    api.handlers.set(paths.compiler, (route) => route.fulfill({ json: { valid: true, diagnostics: "Valid" } }));
    api.handlers.set(paths.fixtures, (route) => route.fulfill({ json: { actors: [], services: [] } }));
    api.handlers.set(paths.evaluation, (route) => route.fulfill({ json: { services: [], warnings: [] } }));
    api.handlers.set(paths[phase], (route) => route.fulfill({ status: 502, contentType: "text/html", body: "upstream unavailable" }));
    await page.goto(appURL + "/#policy");
    await openPolicyPicker(page);
    await page.locator('[data-record-id="test-policy"]').click();
    const source = page.locator("#policy-source");
    const original = await source.inputValue();
    await page.locator("#policy-check").click();
    const titles = { compiler: "Compiler error", fixtures: "Analysis unavailable", evaluation: "Policy evaluation error" };
    await expect(page.locator("#policy-test-status")).toHaveText(`${titles[phase]}: HTTP 502: invalid JSON response`);
    await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
    await expect(source).toHaveValue(original);
    await expect(page.locator("#policy-highlight .zpl-error, #policy-test-gutter .policy-test-line-result")).toHaveCount(0);
    expect(api.counts.get(paths[phase])).toBe(1);
    if (phase === "compiler") expect(api.counts.get(paths.fixtures) || 0).toBe(0);
    if (phase !== "evaluation") expect(api.counts.get(paths.evaluation) || 0).toBe(0);
    expect(api.counts.get("/api/policy/records/test-policy/revisions")).toBe(1);
    expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
  });
}

test("Policy JSON transport retains compiler diagnostics priority and warnings from non-success responses", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/check", (route) => route.fulfill({ status: 422, json: {
    valid: true, error: "Secondary compiler message",
    diagnostics: "Compiler validation unavailable",
    warnings: [{ line: 1, code: "SOURCE_WARNING", message: "Keep this source-local warning" }],
  } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator("#policy-test-status")).toHaveText("Compiler error: Compiler validation unavailable");
  await expect(page.locator('#policy-test-gutter [data-line="1"] [data-has-warnings="true"]')).toHaveAttribute("title", /Keep this source-local warning/);
  expect(api.counts.get("/api/policy/test/fixtures") || 0).toBe(0);
  expect(await page.evaluate(() => state.policy.validSource)).toBeNull();
});

for (const outcome of ["success", "error"]) {
  for (const edit of ["replacement", "Tab"]) {
    test(`Assertions analysis scope rejects delayed ${outcome} after ${edit} and restored source`, async ({ page, appURL, api }) => {
      const original = 'group "Operators" members >= 1;\n';
      Object.assign(api.policy.records.find((record) => record.id === "test-assertions"), {
        content_type: "text/vnd.zpr.assertions", content: original,
      });
      let complete;
      api.handlers.set("/api/assertions/evaluate", async (route) => {
        await new Promise((resolve) => { complete = resolve; });
        await route.fulfill({ json: {
          status: outcome === "success" ? "pass" : "error", revision: 1, draft: false,
          finished_at: "2026-10-08T18:00:00Z", error: outcome === "error" ? "Obsolete assertion error on line 1" : "",
          results: [{ rule: { line: 1, kind: "group", group: "Operators", operator: ">=", limit: 1 }, status: "pass", subjects: [], message: "Obsolete result" }],
        } });
      });
      await openAssertionRecord(page, appURL);
      const source = page.locator("#assertion-source");
      await expect(source).toHaveValue(original);
      const button = page.locator("#assertion-analyze");
      await button.click();
      await expect.poll(() => Boolean(complete)).toBe(true);
      if (edit === "Tab") await source.press("Tab");
      else await source.fill(original + "# edited\n");
      await source.fill(original);
      complete();
      await expect(button).toBeEnabled();
      await expect(button).not.toHaveAttribute("data-analysis-state", /.+/);
      await expect(source).toHaveValue(original);
      await expect(page.locator("#assertion-result-gutter button")).toHaveCount(0);
      await expect(page.locator("#assertion-result-rows tr")).toHaveCount(0);
      await expect(page.locator("#assertion-message")).toBeHidden();
      api.handlers.set("/api/assertions/evaluate", (route) => route.fulfill({ json: {
        status: "pass", revision: 1, draft: false, finished_at: "2026-10-08T18:00:01Z",
        results: [{ rule: { line: 1, kind: "group", group: "Operators", operator: ">=", limit: 1 }, status: "pass", subjects: [], message: "Current result" }],
      } }));
      await button.click();
      await expect(button).toHaveAttribute("data-analysis-state", "success");
      await expect(page.locator("#assertion-result-rows")).toContainText("Current result");
      await expect(page.locator("#assertion-result-gutter button")).toHaveCount(1);
      expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
    });
  }
}

for (const outcome of ["success", "error"]) {
  for (const edit of ["replacement", "Tab"]) {
    test(`Assertion Format scope rejects delayed ${outcome} after ${edit} and restored source`, async ({ page, appURL, api }) => {
      const original = 'group   "Operators" members>=1;\n';
      const formatted = 'group "Operators" members >= 1;\n';
      Object.assign(api.policy.records.find((record) => record.id === "test-assertions"), {
        content_type: "text/vnd.zpr.assertions", content: original,
      });
      let complete;
      api.handlers.set("/api/assertions/format", async (route) => {
        expect(route.request().postDataJSON().source).toBe(original);
        await new Promise((resolve) => { complete = resolve; });
        return route.fulfill(outcome === "success"
          ? { json: { source: formatted, warnings: [{ line: 1, code: "OBSOLETE", message: "Obsolete warning" }] } }
          : { status: 422, json: { error: "Obsolete format failure on line 1" } });
      });
      await openAssertionRecord(page, appURL);
      const source = page.locator("#assertion-source");
      const format = page.locator("#assertion-format");
      await format.click();
      await expect.poll(() => Boolean(complete)).toBe(true);
      await expect(format).toBeDisabled();
      if (edit === "Tab") await source.press("Tab");
      else await source.fill(original + "# changed\n");
      await source.fill(original);
      complete();
      await expect(format).toBeEnabled();
      await expect(source).toHaveValue(original);
      await expect(page.locator("#assertion-message")).toBeHidden();
      await expect(page.locator("#assertion-lint-warnings")).toBeHidden();
      await expect(page.locator("#assertion-result-gutter button")).toHaveCount(0);
      await expect(page.locator("#assertion-analyze")).not.toHaveAttribute("data-analysis-state", /.+/);
      api.handlers.set("/api/assertions/format", (route) => route.fulfill({ json: { source: formatted, warnings: [] } }));
      await format.click();
      await expect(source).toHaveValue(formatted);
      await expect(page.locator("#assertion-message")).toHaveText("Assertions formatted.");
      await source.press("ControlOrMeta+z");
      await expect(source).toHaveValue(original);
      expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
    });
  }
}

for (const outcome of ["success", "error"]) {
  test(`Assertion Format scope rejects delayed ${outcome} after a same-source record revision change`, async ({ page, appURL, api }) => {
    const original = 'group "Operators" members >= 1;\n';
    const record = api.policy.records.find((record) => record.id === "test-assertions");
    Object.assign(record, { content_type: "text/vnd.zpr.assertions", content: original });
    let complete;
    api.handlers.set("/api/assertions/format", async (route) => {
      await new Promise((resolve) => { complete = resolve; });
      return route.fulfill(outcome === "success"
        ? { json: { source: 'group "Obsolete" members >= 99;\n', warnings: [{ line: 1, code: "OBSOLETE", message: "Obsolete warning" }] } }
        : { status: 422, json: { error: "Obsolete format failure on line 1" } });
    });
    await openAssertionRecord(page, appURL);
    await page.locator("#assertion-format").click();
    await expect.poll(() => Boolean(complete)).toBe(true);
    await page.evaluate((record) => {
      window.dispatchEvent(new CustomEvent("policy-record-kind-changed", { detail: { kind: "assertions", record } }));
    }, { ...record, current_revision: record.current_revision + 1 });
    complete();
    await expect(page.locator("#assertion-format")).toBeEnabled();
    await expect(page.locator("#assertion-source")).toHaveValue(original);
    await expect(page.locator("#assertion-message")).toBeHidden();
    await expect(page.locator("#assertion-lint-warnings")).toBeHidden();
    await expect(page.locator("#assertion-result-gutter button")).toHaveCount(0);
  });
}

test("Assertion Format preserves current source-local warnings", async ({ page, appURL, api }) => {
  api.handlers.set("/api/assertions/format", (route) => route.fulfill({
    json: { source: 'group "Operators" members >= 1;\n', warnings: [{ line: 1, code: "LOCAL", message: "Current source warning" }] },
  }));
  await openAssertionRecord(page, appURL);
  await page.locator("#assertion-source").fill('group   "Operators" members>=1;');
  await page.locator("#assertion-format").click();
  await expect(page.locator("#assertion-lint-warnings")).toHaveText("Line 1: LOCAL - Current source warning");
  await expect(page.locator("#assertion-message")).toHaveText("Assertions formatted.");
});

test("Assertion Format cancellation cannot release a newer pending analysis after record switching", async ({ page, appURL, api }) => {
  const original = 'group "Operators" members >= 1;\n';
  Object.assign(api.policy.records.find((record) => record.id === "test-assertions"), {
    content_type: "text/vnd.zpr.assertions", content: original,
  });
  let completeFormat;
  let completeAnalysis;
  api.handlers.set("/api/assertions/format", async (route) => {
    await new Promise((resolve) => { completeFormat = resolve; });
    return route.fulfill({ json: { source: 'group "Obsolete" members >= 99;\n', warnings: [] } });
  });
  api.handlers.set("/api/assertions/evaluate", async (route) => {
    await new Promise((resolve) => { completeAnalysis = resolve; });
    return route.fulfill({ json: { status: "pass", revision: 1, draft: false, finished_at: "2026-10-08T21:00:00Z", results: [] } });
  });
  await openAssertionRecord(page, appURL);
  await page.locator("#assertion-format").click();
  await expect.poll(() => Boolean(completeFormat)).toBe(true);
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await expect(page.locator("#policy-source")).toBeVisible();
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-assertions"]').click();
  await expect(page.locator("#assertion-format")).toBeEnabled();
  await page.locator("#assertion-analyze").click();
  await expect.poll(() => Boolean(completeAnalysis)).toBe(true);
  completeFormat();
  await expect(page.locator("#assertion-analyze")).toBeDisabled();
  await expect(page.locator("#assertion-format")).toBeDisabled();
  await expect(page.locator("#assertion-source")).toHaveValue(original);
  completeAnalysis();
  await expect(page.locator("#assertion-analyze")).toHaveAttribute("data-analysis-state", "success");
  await expect(page.locator("#assertion-format")).toBeEnabled();
  await expect(page.locator("#assertion-message")).toBeHidden();
});

test("Assertion Format failures retain source and display current service errors without invented lines", async ({ page, appURL, api }) => {
  api.handlers.set("/api/assertions/format", (route) => route.fulfill({ status: 503, json: { error: "Formatter unavailable" } }));
  await openAssertionRecord(page, appURL);
  const source = page.locator("#assertion-source");
  const original = 'group "Operators" members >= 1;\n';
  await source.fill(original);
  await page.locator("#assertion-format").click();
  await expect(page.locator("#assertion-message")).toHaveText("Formatter unavailable");
  await expect(source).toHaveValue(original);
  await expect(page.locator("#assertion-result-gutter button")).toHaveCount(0);
  await expect(page.locator("#assertion-format")).toBeEnabled();
});

test("shared source-line validation never clamps invalid locations", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  expect(await page.evaluate(() => [undefined, null, true, [], 0, -1, 1.5, 4, Infinity, "bad", 1, "2", 3]
    .map((value) => window.ZPREditorPage.sourceLine(value, 3)))).toEqual([null, null, null, null, null, null, null, null, null, null, 1, 2, 3]);
});

test("Assertions unlocated errors stay under controls and valid diagnostics alone get gutter markers", async ({ page, appURL, api }) => {
  let error = "Trusted LDAP unavailable";
  api.handlers.set("/api/assertions/evaluate", (route) => route.fulfill({ json: {
    status: "error", error, revision: 0, draft: true, finished_at: "2026-10-08T18:00:00Z", results: [],
  } }));
  await openAssertionRecord(page, appURL);
  await page.locator("#assertion-source").fill('group "Operators" members >= 1;\n# second line');
  for (error of ["Trusted LDAP unavailable", "Invalid line 99", "Invalid line 0", "Invalid line 1.5", "Invalid line 2"]) {
    await page.locator("#assertion-analyze").click();
    await expect(page.locator("#assertion-analyze")).toHaveAttribute("data-analysis-state", "error");
    const markers = page.locator("#assertion-result-gutter .assertion-result-marker");
    if (error === "Invalid line 2") {
      await expect(markers).toHaveCount(1);
      await expect(page.locator('#assertion-result-gutter [data-line="2"] button')).toBeVisible();
      await expect(page.locator("#assertion-message")).toBeHidden();
      await markers.click();
      await expect(page.getByRole("dialog", { name: "Assertion error" })).toContainText(error);
      await page.getByRole("dialog", { name: "Assertion error" }).locator(".dialog-actions .button").click();
    } else {
      await expect(markers).toHaveCount(0);
      await expect(page.locator("#assertion-message")).toBeVisible();
      await expect(page.locator("#assertion-message")).toHaveText(error);
    }
  }
  api.handlers.set("/api/assertions/evaluate", (route) => route.fulfill({ json: {
    status: "pass", revision: 0, draft: true, finished_at: "2026-10-08T18:00:00Z",
    results: [0, 99, 1.5, 2].map((line) => ({ rule: { line, kind: "group", group: "Operators", operator: ">=", limit: 1 }, status: "pass", subjects: [], message: "Checked" })),
  } }));
  await page.locator("#assertion-analyze").click();
  await expect(page.locator("#assertion-result-gutter .assertion-result-marker")).toHaveCount(1);
  await expect(page.locator('#assertion-result-gutter [data-line="2"] button')).toHaveText("PASS");
});

for (const editor of [
  { name: "Policy", path: "/#policy", source: "policy-source", status: "policy-test-status", frame: "policy-code-editor", button: "policy-check", endpoint: "/api/policy/check", content: "define Employee as user.\n" },
  { name: "Assertions", path: "/#policy", source: "assertion-source", status: "assertion-message", frame: "assertion-editor", button: "assertion-analyze", endpoint: "/api/assertions/evaluate", content: 'group "Operators" members >= 1;' },
  { name: "Config", path: "/#zpr-config", source: "zpr-config-source", status: "zpr-config-status", button: "zpr-config-validate", endpoint: "/api/policy/config/check", content: 'name = "Example"\n' },
  { name: "Gateway", path: "/#gateways", source: "gateway-source", status: "gateway-draft-message", button: "gateway-analyze", endpoint: "/api/gateways/config/check" },
  { name: "Directory", path: "/organizations.html", source: "directory-editor-source", status: "directory-editor-status", button: "directory-editor-save", endpoint: "/api/simulator/organizations/alpha/directory" },
  { name: "Scenario", path: "/scenarios.html", source: "scenario-editor-source", status: "scenario-editor-status", button: "scenario-source-analyze", endpoint: "/api/simulator/organizations/alpha/scenario-check" },
]) {
  test(`${editor.name} non-line editor errors appear under controls and above source`, async ({ page, appURL, api }) => {
    if (editor.name === "Gateway") {
      api.handlers.set("/api/gateways/contracts", (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [
        { organization_id: "alpha", instance_id: "egress", adapter_cn: "gateway-egress", service_name: "egress.svc.zpr" },
      ] } }));
      api.handlers.set("/api/gateways/configs", (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
    }
    if (editor.name === "Directory") {
      api.handlers.set(editor.endpoint, (route) => route.fulfill({ json: { revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" } } }));
      api.handlers.set(editor.endpoint + "/revisions", (route) => route.fulfill({ json: [] }));
    }
    if (editor.name === "Scenario") await openRawScenario(page, appURL, api);
    else if (editor.name === "Assertions") await openAssertionRecord(page, appURL);
    else await page.goto(appURL + editor.path);
    if (editor.name === "Policy") {
      await openPolicyPicker(page);
      await page.locator('[data-record-id="test-policy"]').click();
    }
    if (editor.name === "Directory") await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
    const source = page.locator(`#${editor.source}`);
    await expect(source).toBeVisible();
    if (editor.name === "Gateway") await expect(source).toHaveValue(/"instance_id": "egress"/);
    await source.fill(editor.content || (await source.inputValue()) + "\n");
    api.handlers.set(editor.endpoint, (route) => route.fulfill({ status: 503, json: { valid: false, line: 0, error: "Editor service unavailable", diagnostics: "Editor service unavailable" } }));
    if (editor.name === "Directory") await page.locator("#directory-editor-files-toggle").click();
    await page.locator(`#${editor.button}`).click();
    const status = page.locator(`#${editor.status}`);
    await expect(status).toBeVisible();
    await expect(status).toContainText("Editor service unavailable");
    const geometry = await status.evaluate((element, { sourceID, frameID }) => {
      const controls = element.closest(".policy-page").querySelector(".policy-editor-tools").getBoundingClientRect();
      const status = element.getBoundingClientRect();
      const source = document.getElementById(sourceID);
      const frame = (frameID ? document.getElementById(frameID) : source.closest(".config-source-editor")).getBoundingClientRect();
      return { gap: status.top - controls.bottom, beforeSource: status.bottom <= frame.top + 1 };
    }, { sourceID: editor.source, frameID: editor.frame });
    expect(geometry.gap).toBeGreaterThanOrEqual(-1);
    expect(geometry.gap).toBeLessThan(50);
    expect(geometry.beforeSource).toBe(true);
    if (!["Scenario", "Directory"].includes(editor.name)) expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
  });
}

test("shared Analyze state accepts explicit states and leaves diagnostics, actions and readiness untouched", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  const result = await page.evaluate(() => {
    const button = document.createElement("button");
    button.disabled = true;
    button.textContent = "Analyze";
    button.className = "button-next-evaluate";
    const states = ["pending", "success", "error", ""].map((state) => {
      window.ZPREditorPage.setAnalysisState(button, state);
      return { state: button.getAttribute("data-analysis-state"), disabled: button.disabled, text: button.textContent, classes: button.className };
    });
    window.ZPREditorPage.setAnalysisState(button, "success");
    let error = "";
    try { window.ZPREditorPage.setAnalysisState(button, "unknown"); }
    catch (failure) { error = failure.message; }
    return { states, error, retained: button.dataset.analysisState };
  });
  expect(result.states).toEqual(["pending", "success", "error", null].map((state) => ({
    state, disabled: true, text: "Analyze", classes: "button-next-evaluate",
  })));
  expect(result.error).toBe("Unsupported editor analysis state: unknown");
  expect(result.retained).toBe("success");
});

for (const editor of [
  { name: "Policy", path: "/#policy", source: "policy-source", button: "policy-check", endpoint: "/api/policy/check", content: "define Employee as user.\n" },
  { name: "Assertions", path: "/#policy", source: "assertion-source", button: "assertion-analyze", endpoint: "/api/assertions/evaluate", content: 'group "Operators" members >= 1;\n' },
  { name: "Config", path: "/#zpr-config", source: "zpr-config-source", button: "zpr-config-validate", endpoint: "/api/policy/config/check", content: 'name = "Example"\n' },
  { name: "Gateway", path: "/#gateways", source: "gateway-source", button: "gateway-analyze", endpoint: "/api/gateways/config/check" },
  { name: "Scenario", path: "/scenarios.html", source: "scenario-editor-source", button: "scenario-source-analyze", endpoint: "/api/simulator/organizations/alpha/scenario-check" },
]) {
  test(`${editor.name} shared Analyze state handles success, edits and failures through its own adapter`, async ({ page, appURL, api }) => {
    if (editor.name === "Gateway") {
      api.handlers.set("/api/gateways/contracts", (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [
        { organization_id: "alpha", instance_id: "egress", adapter_cn: "gateway-egress", service_name: "egress.svc.zpr" },
      ] } }));
      api.handlers.set("/api/gateways/configs", (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
    }
    if (editor.name === "Scenario") await openRawScenario(page, appURL, api);
    else if (editor.name === "Assertions") await openAssertionRecord(page, appURL);
    else {
      await page.goto(appURL + editor.path);
      if (editor.name === "Policy") {
        await openPolicyPicker(page);
        await page.locator('[data-record-id="test-policy"]').click();
      }
    }
    if (editor.name !== "Scenario") {
      api.handlers.set(editor.endpoint, (route) => route.fulfill({ json: editor.name === "Assertions"
        ? { status: "pass", revision: 0, draft: true, finished_at: "2026-10-08T17:00:00Z", results: [], warnings: [] }
        : { valid: true, diagnostics: "Valid source" } }));
    }
    if (editor.name === "Policy") {
      api.handlers.set("/api/policy/test/fixtures", (route) => route.fulfill({ json: { actors: [], services: [] } }));
      api.handlers.set("/api/policy/test", (route) => route.fulfill({ json: { results: [], warnings: [] } }));
    }
    await page.evaluate(() => {
      const original = window.ZPREditorPage.setAnalysisState;
      window.analysisTransitions = [];
      window.ZPREditorPage.setAnalysisState = (button, state = "") => {
        window.analysisTransitions.push({ id: button.id, state });
        return original(button, state);
      };
    });
    const source = page.locator(`#${editor.source}`);
    if (editor.content) await source.fill(editor.content);
    if (editor.name === "Gateway") await expect(source).toHaveValue(/"instance_id": "egress"/);
    const button = page.locator(`#${editor.button}`);
    await button.click();
    await expect(button).toHaveAttribute("data-analysis-state", "success");
    await source.fill((await source.inputValue()) + "\n");
    await expect(button).not.toHaveAttribute("data-analysis-state", "success");
    api.handlers.set(editor.endpoint, (route) => route.fulfill({ status: 422, json: {
      valid: false, error: "Analysis test failure", diagnostics: "Analysis test failure", line: 0,
    } }));
    await button.click();
    await expect(button).toHaveAttribute("data-analysis-state", "error");
    const transitions = await page.evaluate((id) => window.analysisTransitions.filter((entry) => entry.id === id).map((entry) => entry.state), editor.button);
    expect(transitions).toContain("success");
    expect(transitions).toContain("error");
    if (editor.name !== "Scenario") expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
  });
}

test("shared History safely renders versions, empty states and explicit selection once", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  await page.evaluate(() => {
    const root = document.createElement("div");
    root.innerHTML = '<details id="test-history"><summary>History <span id="test-history-count"></span></summary><div id="test-history-list"></div></details>';
    document.querySelector("#page-map").append(root);
    let available = false;
    window.historySelections = [];
    const history = window.ZPREditorPage.createHistory({
      menu: root.querySelector("details"), list: root.querySelector("#test-history-list"),
      count: root.querySelector("#test-history-count"), isAvailable: () => available,
    });
    window.renderTestHistory = (ready, revisions) => {
      available = ready;
      history.render(revisions, {
        current: 2, detail: (revision) => revision.summary,
        meta: () => "<img src=x>", onSelect: (revision) => window.historySelections.push(revision.number),
      });
    };
    window.renderTestHistory(false, []);
  });
  const menu = page.locator("#test-history");
  const summary = menu.locator("summary");
  await summary.click();
  await expect(menu).not.toHaveAttribute("open");
  await expect(page.locator("#test-history-count")).toHaveText("—");
  await page.evaluate(() => window.renderTestHistory(true, []));
  await expect(page.locator("#test-history-count")).toHaveText("0 versions");
  await summary.click();
  await expect(menu).toContainText("No saved versions.");
  await page.keyboard.press("Escape");
  await expect(summary).toBeFocused();
  await page.evaluate(() => window.renderTestHistory(true, [{ number: 2, summary: "<script>unsafe</script>" }]));
  await expect(page.locator("#test-history-count")).toHaveText("1 version");
  await summary.click();
  const item = menu.locator("[data-revision='2']");
  await expect(item).toHaveAttribute("aria-current", "true");
  await expect(item).toContainText("<script>unsafe</script>");
  await expect(item).toContainText("<img src=x>");
  await expect(item.locator("script, img")).toHaveCount(0);
  await item.click();
  await expect(menu).not.toHaveAttribute("open");
  expect(await page.evaluate(() => window.historySelections)).toEqual([2]);
  await summary.click();
  await page.locator("#topology-stage").click();
  await expect(menu).not.toHaveAttribute("open");
});

for (const operation of ["workspace", "record", "history", "revision"]) {
  test(`Policy loading JSON transport reports malformed ${operation} responses separately from source diagnostics and supports retry`, async ({ page, appURL, api }) => {
    const target = { ...api.policy.records[0], id: "load-target", name: "Load target" };
    api.policy.records.push(target);
    api.handlers.set("/api/policy/records/load-target", (route) => route.fulfill({ json: target }));
    api.handlers.set("/api/policy/records/load-target/revisions", (route) => route.fulfill({ json: [] }));
    const paths = { workspace: "/api/policy", record: "/api/policy/records/load-target", history: "/api/policy/records/load-target/revisions", revision: "/api/policy/records/test-policy/revisions/1" };
    await page.goto(appURL + "/#policy");
    await openPolicyPicker(page);
    await page.locator('[data-record-id="test-policy"]').click();
    const source = page.locator("#policy-source");
    await expect(source).toHaveValue("define Employee as user.\n");
    const original = await source.inputValue();
    const previousCounts = api.counts.get(paths[operation]) || 0;
    api.handlers.set(paths[operation], (route) => route.fulfill({ status: 502, contentType: "text/html", body: "upstream unavailable" }));
    const invoke = () => page.evaluate(async (action) => {
      if (action === "workspace") await refreshPolicyCatalog();
      else if (action === "record" || action === "history") await selectPolicyRecord("load-target", true, true);
      else await browsePolicyRevision(1);
    }, operation);
    await invoke();
    await expect(page.locator("#policy-load-status")).toBeVisible();
    await expect(page.locator("#policy-load-status")).toHaveText("HTTP 502: invalid JSON response");
    await expect(page.locator("#policy-test-status")).toBeHidden();
    await expect(source).toHaveValue(original);
    await expect(page.locator("#policy-highlight .zpl-error")).toHaveCount(0);
    expect(await page.evaluate(() => state.policy.browsingRevision)).toBe(0);
    expect(api.counts.get(paths[operation])).toBe(previousCounts + 1);
    if (operation === "revision") api.handlers.set(paths[operation], (route) => route.fulfill({ json: { number: 1, content: "define Historical as user.\n", summary: "Historical", content_hash: "old" } }));
    else if (operation === "record") api.handlers.set(paths.record, (route) => route.fulfill({ json: target }));
    else if (operation === "history") api.handlers.set(paths.history, (route) => route.fulfill({ json: [] }));
    else api.handlers.delete(paths.workspace);
    if (operation === "history") await page.evaluate(() => loadPolicyHistory(state.policy.record.id));
    else await invoke();
    await expect(page.locator("#policy-load-status")).toBeHidden();
    await expect(source).toHaveValue(operation === "revision" ? "define Historical as user.\n" : original);
    expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
  });
}

test("Policy loading JSON transport exposes initial structured workspace failures without inventing source diagnostics", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy", (route) => route.fulfill({ status: 503, json: { error: "Policy workspace unavailable", line: 1, diagnostics: "Workspace configuration detail" } }));
  await page.goto(appURL + "/#policy");
  await expect(page.locator("#policy-load-status")).toBeVisible();
  await expect(page.locator("#policy-load-status")).toHaveText("Policy workspace unavailable");
  await expect(page.locator("#policy-source")).toBeDisabled();
  await expect(page.locator("#policy-test-status")).toBeHidden();
  await expect(page.locator("#policy-highlight .zpl-error, #policy-test-gutter .policy-test-line-result")).toHaveCount(0);
  expect(await page.evaluate(() => state.policy.loaded)).toBe(false);
  api.handlers.delete("/api/policy");
  await page.evaluate(() => loadPolicyWorkspace());
  await expect(page.locator("#policy-load-status")).toBeHidden();
  await expect(page.locator("#policy-source")).toBeEditable();
  expect(await page.evaluate(() => state.policy.loaded)).toBe(true);
  expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
});

test("Policy loading errors clear when starting a new draft without changing Analyze state", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  api.handlers.set("/api/policy/records/test-policy/revisions/1", (route) => route.fulfill({ status: 503, json: { error: "Version unavailable", line: 1 } }));
  await page.evaluate(() => browsePolicyRevision(1));
  await expect(page.locator("#policy-load-status")).toHaveText("Version unavailable");
  await expect(page.locator("#policy-check")).not.toHaveAttribute("data-analysis-state", /error|success/);
  await page.evaluate(() => beginNewPolicyDraft());
  await expect(page.locator("#policy-load-status")).toBeHidden();
  await expect(page.locator("#policy-source")).toHaveValue("");
  await expect(page.locator("#policy-test-status")).toBeHidden();
});

for (const operation of ["revision", "create", "stage"]) {
  test(`Policy mutation JSON transport rejects malformed ${operation} responses without advancing revision or retrying`, async ({ page, appURL, api }) => {
    await page.goto(appURL + "/#policy");
    await openPolicyPicker(page);
    await page.locator('[data-record-id="test-policy"]').click();
    const original = await page.locator("#policy-source").inputValue();
    const endpoint = operation === "create" ? "/api/policy/records" : `/api/policy/records/test-policy/${operation === "stage" ? "stage" : "revisions"}`;
    const requests = [];
    api.handlers.set(endpoint, async (route) => {
      requests.push({ method: route.request().method(), body: route.request().postDataJSON() });
      await route.fulfill({ status: 502, contentType: "text/html", body: "upstream unavailable" });
    });
    if (operation === "stage") {
      await page.evaluate(() => confirmPolicyStage());
      await expect(page.locator("#policy-stage-status")).toContainText("HTTP 502: invalid JSON response");
      expect(await page.evaluate(() => state.policy.stagePending)).toBe(false);
    } else {
      await page.evaluate((creating) => {
        const policy = state.policy;
        if (creating) policy.record = { ...policy.record, id: "", isDraft: true, name: "Created draft" };
        const text = document.getElementById("policy-source").value;
        policy.evaluatedSource = text;
        policy.validSource = text;
        policy.saveTestSource = text;
      }, operation === "create");
      await page.evaluate(() => appendPolicyVersion("Transport test"));
      await expect(page.locator("#version-dialog")).toBeVisible();
      await expect(page.locator("#version-error")).toHaveText("HTTP 502: invalid JSON response");
      expect(await page.evaluate(() => state.policy.record.isDraft || false)).toBe(operation === "create");
    }
    await expect(page.locator("#policy-source")).toHaveValue(original);
    expect(await page.evaluate(() => state.policy.revision)).toBe(1);
    expect(await page.evaluate(() => state.policy.record.current_revision)).toBe(1);
    expect(requests).toHaveLength(1);
    expect(requests[0].method).toBe("POST");
    if (operation === "stage") expect(requests[0].body).toEqual({ expected_revision: 1 });
    else if (operation === "revision") expect(requests[0].body).toEqual({ content: original, expected_revision: 1, summary: "Transport test" });
    else expect(requests[0].body).toMatchObject({ content: original, name: "Created draft", kind: "policy", summary: "Transport test" });
    expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
  });
}

test("Policy shared History keeps dirty cancellation and loads each selected revision once", async ({ page, appURL, api }) => {
  api.policy.records[0].current_revision = 2;
  api.handlers.set("/api/policy/records/test-policy/revisions", (route) => route.fulfill({ json: [
    { number: 1, summary: "Original", author: "tester", created_at: "2026-10-01T12:00:00Z" },
    { number: 2, summary: "Current", author: "tester", created_at: "2026-10-02T12:00:00Z" },
  ] }));
  api.handlers.set("/api/policy/records/test-policy/revisions/1", (route) => route.fulfill({ json: {
    number: 1, content: "define Original as user.\n", content_hash: "old", summary: "Original",
  } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const source = page.locator("#policy-source");
  const saved = await source.inputValue();
  const draft = saved + "# unsaved\n";
  await source.fill(draft);
  const menu = page.locator("#policy-history-menu");
  const summary = menu.locator("summary");
  await expect(page.locator("#policy-history-count")).toHaveText("2 versions");
  await summary.click();
  await expect(menu.locator('[data-revision="2"]')).toHaveAttribute("aria-current", "true");
  page.once("dialog", async (dialog) => {
    expect(dialog.message()).toBe("Discard unsaved edits and browse this version?");
    await dialog.dismiss();
  });
  await menu.locator('[data-revision="1"]').click();
  await expect(menu).not.toHaveAttribute("open");
  await expect(source).toHaveValue(draft);
  expect(api.counts.get("/api/policy/records/test-policy/revisions/1")).toBe(1);
  await source.fill(saved);
  await summary.click();
  await menu.locator('[data-revision="1"]').click();
  await expect(source).toHaveValue("define Original as user.\n");
  await expect(source).toBeDisabled();
  await summary.click();
  await expect(menu.locator('[data-revision="1"]')).toHaveAttribute("aria-current", "true");
  await expect(menu.locator('[data-revision="2"]')).toHaveAttribute("aria-current", "false");
  expect(api.counts.get("/api/policy/records/test-policy/revisions/1")).toBe(2);
  await page.keyboard.press("Escape");
  await expect(summary).toBeFocused();
});

test("Assertions shared History uses the shared count, metadata and empty rendering", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/records/test-assertions/revisions", (route) => route.fulfill({ json: [
    { number: 1, summary: "<img src=x>", author: "tester", created_at: "2026-10-01T12:00:00Z" },
  ] }));
  await openAssertionRecord(page, appURL);
  const menu = page.locator("#policy-history-menu");
  await expect(page.locator("#policy-history-count")).toHaveText("1 version");
  await menu.locator("summary").click();
  await expect(menu.locator('[data-revision="1"]')).toHaveAttribute("aria-current", "true");
  await expect(menu.locator(".history-item")).toContainText("<img src=x>");
  await expect(menu.locator(".history-item img")).toHaveCount(0);
  await page.keyboard.press("Escape");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await expect(page.locator("#policy-history-count")).toHaveText("0 versions");
  await menu.locator("summary").click();
  await expect(menu).toContainText("No saved versions.");
  await expect(menu.locator(".history-item")).toHaveCount(0);
});

test("Config shared History selection never invokes the Policy revision adapter", async ({ page, appURL, api }) => {
  const config = { id: "history-config", category_id: "test", kind: "configuration", name: "History config", current_revision: 2, content: 'name = "Current"\n' };
  api.policy.records.push(config);
  api.handlers.set("/api/policy/records/history-config", (route) => route.fulfill({ json: config }));
  api.handlers.set("/api/policy/records/history-config/revisions", (route) => route.fulfill({ json: [
    { number: 1, summary: "Original" }, { number: 2, summary: "Current" },
  ] }));
  api.handlers.set("/api/policy/records/history-config/revisions/1", (route) => route.fulfill({ json: { number: 1, content: 'name = "Original"\n' } }));
  api.handlers.set("/api/policy/records/test-policy/revisions/1", (route) => route.fulfill({ json: { number: 1, content: "define Wrong as user.\n" } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.locator('a[data-page-link="zpr-config"]').click();
  await page.locator("#zpr-config-picker-toggle").click();
  await page.locator('#zpr-config-records [data-record-id="history-config"]').click();
  await expect(page.locator("#zpr-config-source")).toHaveValue(config.content);
  const menu = page.locator("#zpr-config-history-menu");
  await menu.locator("summary").click();
  await menu.locator('[data-revision="1"]').click();
  await expect(page.locator("#zpr-config-source")).toHaveValue('name = "Original"\n');
  await expect(menu).not.toHaveAttribute("open");
  expect(api.counts.get("/api/policy/records/history-config/revisions/1")).toBe(1);
  expect(api.counts.get("/api/policy/records/test-policy/revisions/1") || 0).toBe(0);
  await expect(page.locator("#policy-source")).not.toHaveValue('name = "Original"\n');
});

test("shared discard guard prompts only for dirty state and returns the explicit decision", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  const result = await page.evaluate(() => {
    const original = window.confirm;
    const messages = [];
    let approved = false;
    window.confirm = (message) => { messages.push(message); return approved; };
    try {
      const clean = window.ZPREditorPage.confirmDiscard(false, "Must not prompt");
      const cancelled = window.ZPREditorPage.confirmDiscard(true, "Discard test draft?");
      approved = true;
      const accepted = window.ZPREditorPage.confirmDiscard(true, "Discard test draft?");
      return { clean, cancelled, accepted, messages };
    } finally {
      window.confirm = original;
    }
  });
  expect(result).toEqual({ clean: true, cancelled: false, accepted: true, messages: ["Discard test draft?", "Discard test draft?"] });
});

for (const editor of [
  { name: "Config", path: "/#zpr-config", source: "zpr-config-source", discard: "zpr-config-discard", menu: "zpr-config-files-toggle", message: "Discard unsaved configuration changes?" },
  { name: "Gateway", path: "/#gateways", source: "gateway-source", discard: "gateway-discard", menu: "gateway-files-toggle", message: "Discard unsaved gateway draft edits?" },
  { name: "Directory", path: "/organizations.html", source: "directory-editor-source", discard: "directory-editor-discard", menu: "directory-editor-files-toggle", message: "Discard unsaved directory changes?" },
  { name: "Scenario", path: "/scenarios.html", source: "scenario-editor-source", discard: "scenario-editor-discard", menu: "scenario-editor-files-toggle", message: "Discard unsaved scenario changes?" },
]) {
  test(`${editor.name} shared discard preserves cancelled edits and restores only after approval`, async ({ page, appURL, api }) => {
    if (editor.name === "Gateway") {
      api.handlers.set("/api/gateways/contracts", (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [
        { organization_id: "alpha", instance_id: "egress", adapter_cn: "gateway-egress", service_name: "egress.svc.zpr" },
      ] } }));
      api.handlers.set("/api/gateways/configs", (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
    }
    if (editor.name === "Directory") {
      api.handlers.set("/api/simulator/organizations/alpha/directory", (route) => route.fulfill({ json: {
        revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" },
      } }));
      api.handlers.set("/api/simulator/organizations/alpha/directory/revisions", (route) => route.fulfill({ json: [] }));
    }
    if (editor.name === "Scenario") await openRawScenario(page, appURL, api);
    else await page.goto(appURL + editor.path);
    if (editor.name === "Directory") await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
    const source = page.locator(`#${editor.source}`);
    await expect(source).toBeVisible();
    if (editor.name === "Gateway") await expect(source).toHaveValue(/"instance_id": "egress"/);
    const saved = await source.inputValue();
    const draft = ["Gateway", "Scenario"].includes(editor.name)
      ? JSON.stringify({ ...JSON.parse(saved), description: "Unsaved discard test" }, null, 2)
      : saved + "# Unsaved discard test\n";
    await source.fill(draft);
    await page.locator(`#${editor.menu}`).click();
    page.once("dialog", async (dialog) => { expect(dialog.message()).toBe(editor.message); await dialog.dismiss(); });
    await page.locator(`#${editor.discard}`).click();
    await expect(source).toHaveValue(draft);
    if (!await page.locator(`#${editor.discard}`).isVisible()) await page.locator(`#${editor.menu}`).click();
    page.once("dialog", async (dialog) => { expect(dialog.message()).toBe(editor.message); await dialog.accept(); });
    await page.locator(`#${editor.discard}`).click();
    await expect(source).toHaveValue(saved);
  });
}

test("Policy and Assertions shared discard guard protects dirty record switching", async ({ page, appURL, api }) => {
  Object.assign(api.policy.records.find((record) => record.id === "test-assertions"), {
    content_type: "text/vnd.zpr.assertions", content: 'group "Operators" members >= 1;\n',
  });
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  for (const [sourceID, nextRecord] of [
    ["policy-source", "test-assertions"],
    ["assertion-source", "test-policy"],
  ]) {
    const source = page.locator(`#${sourceID}`);
    await expect(source).toBeVisible();
    if (sourceID === "assertion-source") await expect(source).toHaveValue(api.policy.records.find((record) => record.id === "test-assertions").content);
    const draft = (await source.inputValue()) + "\n# Unsaved switch test\n";
    await source.fill(draft);
    await expect(page.locator("#policy-modified-indicator")).toBeVisible();
    await openPolicyPicker(page);
    page.once("dialog", async (dialog) => {
      expect(dialog.message()).toBe("Discard unsaved changes or leave this historical version?");
      await dialog.dismiss();
    });
    await page.locator(`[data-record-id="${nextRecord}"]`).click();
    await expect(source).toHaveValue(draft);
    await expect(page.locator("#policy-modified-indicator")).toBeVisible();
    if (!await page.locator("#policy-catalog-pane").isVisible()) await openPolicyPicker(page);
    page.once("dialog", (dialog) => dialog.accept());
    await page.locator(`[data-record-id="${nextRecord}"]`).click();
    await expect(page.locator("#policy-record-title")).toHaveText(nextRecord === "test-policy" ? "Test policy" : "Organization assertions");
    await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  }
});

test("shared editor identity safely renders names and clears stale title, version and dirty state", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  const states = await page.evaluate(() => {
    const title = document.createElement("strong");
    const version = document.createElement("span");
    const modified = document.createElement("span");
    const fields = { title, version, modified };
    const read = () => ({
      name: title.textContent, hidden: title.hidden, tooltip: title.getAttribute("title"),
      version: version.textContent, versionHidden: version.hidden, versionTooltip: version.getAttribute("title"),
      dirty: !modified.hidden, children: title.children.length,
    });
    version.textContent = "Version 99";
    version.title = "old hash";
    window.ZPREditorPage.renderIdentity(fields, { name: "  <img src=x>  ", tooltip: "Policies/<img src=x>", dirty: true });
    const named = read();
    window.ZPREditorPage.renderIdentity(fields);
    const empty = read();
    window.ZPREditorPage.renderIdentity(fields, { name: "   ", dirty: true });
    return [named, empty, read()];
  });
  expect(states).toEqual([
    { name: "<img src=x>", hidden: false, tooltip: "Policies/<img src=x>", version: "", versionHidden: true, versionTooltip: null, dirty: true, children: 0 },
    { name: "Untitled", hidden: false, tooltip: null, version: "", versionHidden: true, versionTooltip: null, dirty: false, children: 0 },
    { name: "Untitled", hidden: false, tooltip: null, version: "", versionHidden: true, versionTooltip: null, dirty: true, children: 0 },
  ]);
});

test("Policy shared identity retains path and clears Modified when source returns to saved text", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const source = page.locator("#policy-source");
  const saved = await source.inputValue();
  await expect(page.locator("#policy-record-title")).toHaveText("Test policy");
  await expect(page.locator("#policy-record-title")).toHaveAttribute("title", "Alpha/Policies/Test policy");
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  await source.fill(saved + "# edited\n");
  await expect(page.locator("#policy-modified-indicator")).toBeVisible();
  await source.fill(saved);
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  await expect(page.locator("#policy-revision-label")).toBeHidden();
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-assertions"]').click();
  await expect(page.locator("#policy-record-title")).toHaveText("Organization assertions");
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  const assertion = page.locator("#assertion-source");
  const assertionSaved = await assertion.inputValue();
  await assertion.fill(assertionSaved + "\n");
  await expect(page.locator("#policy-modified-indicator")).toBeVisible();
  await assertion.fill(assertionSaved);
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
});

test("shared Save shortcut respects modifiers, disabled actions, repeats, composition and cleanup", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  const results = await page.evaluate(() => {
    const root = document.createElement("div");
    const input = document.createElement("textarea");
    const button = document.createElement("button");
    root.append(input, button);
    document.body.append(root);
    let saves = 0;
    button.addEventListener("click", () => saves++);
    const dispose = window.ZPREditorPage.bindSaveShortcut({ root, button });
    const send = (options = {}) => {
      const event = new KeyboardEvent("keydown", { key: "s", ctrlKey: true, bubbles: true, cancelable: true, ...options });
      if (options.handled) event.preventDefault();
      input.dispatchEvent(event);
      return { saves, prevented: event.defaultPrevented };
    };
    const outcomes = [send(), send({ ctrlKey: false, metaKey: true, key: "S" })];
    for (const options of [{ repeat: true }, { isComposing: true }, { shiftKey: true }, { altKey: true }, { ctrlKey: false }, { key: "x" }, { handled: true }]) outcomes.push(send(options));
    button.disabled = true;
    outcomes.push(send());
    button.disabled = false;
    dispose();
    outcomes.push(send());
    root.remove();
    return outcomes;
  });
  expect(results).toEqual([
    { saves: 1, prevented: true }, { saves: 2, prevented: true },
    ...Array.from({ length: 6 }, () => ({ saves: 2, prevented: false })),
    { saves: 2, prevented: true }, { saves: 2, prevented: true }, { saves: 2, prevented: false },
  ]);
});

for (const editor of [
  { name: "Policy", path: "/#policy", source: "policy-source", save: "policy-save" },
  { name: "Assertions", path: "/#policy", source: "assertion-source", save: "assertion-save" },
  { name: "Config", path: "/#zpr-config", source: "zpr-config-source", save: "zpr-config-save" },
  { name: "Gateways", path: "/#gateways", source: "gateway-source", save: "gateway-save" },
  { name: "Directory", path: "/organizations.html", source: "directory-editor-source", save: "directory-editor-save" },
  { name: "Scenario", path: "/scenarios.html", source: "scenario-editor-source", save: "scenario-editor-save" },
]) {
  test(`${editor.name} source uses shared Ctrl and Cmd Save action without publishing`, async ({ page, appURL, api }) => {
    if (editor.name === "Assertions") await openAssertionRecord(page, appURL);
    else if (editor.name === "Scenario") await openRawScenario(page, appURL, api);
    else {
      if (editor.name === "Directory") {
        api.handlers.set("/api/simulator/organizations/alpha/directory", (route) => route.fulfill({ json: {
          revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" },
        } }));
        api.handlers.set("/api/simulator/organizations/alpha/directory/revisions", (route) => route.fulfill({ json: [] }));
      }
      await page.goto(appURL + editor.path);
      if (editor.name === "Policy") {
        await openPolicyPicker(page);
        await page.locator('[data-record-id="test-policy"]').click();
      }
      if (editor.name === "Directory") await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
    }
    await expect(page.locator(`#${editor.source}`)).toBeVisible();
    const result = await page.evaluate(({ source, save }) => {
      const target = document.getElementById(source);
      const button = document.getElementById(save);
      let saves = 0;
      document.addEventListener("click", (event) => {
        if (event.target !== button) return;
        event.preventDefault();
        event.stopImmediatePropagation();
        saves++;
      }, true);
      button.disabled = false;
      const ctrl = new KeyboardEvent("keydown", { key: "s", ctrlKey: true, bubbles: true, cancelable: true });
      const cmd = new KeyboardEvent("keydown", { key: "s", metaKey: true, bubbles: true, cancelable: true });
      target.dispatchEvent(ctrl);
      target.dispatchEvent(cmd);
      button.disabled = true;
      const disabled = new KeyboardEvent("keydown", { key: "s", ctrlKey: true, bubbles: true, cancelable: true });
      target.dispatchEvent(disabled);
      return { saves, ctrl: ctrl.defaultPrevented, cmd: cmd.defaultPrevented, disabled: disabled.defaultPrevented };
    }, editor);
    expect(result).toEqual({ saves: 2, ctrl: true, cmd: true, disabled: true });
  });
}

test("Config Save shortcut creates a named draft without applying runtime configuration", async ({ page, appURL, api }) => {
  const content = '[visa_service]\ndock_node = "node"\n';
  let savedRecord;
  api.handlers.set("/api/policy/categories", (route) => {
    const category = { id: "zpr-config", path: "ZPR Config", name: "ZPR Config" };
    api.policy.categories.push(category);
    return route.fulfill({ status: 201, json: category });
  });
  api.handlers.set("/api/policy/records", (route) => {
    const request = route.request().postDataJSON();
    expect(request).toMatchObject({ name: "Keyboard draft", content, kind: "configuration" });
    const record = { ...request, id: "keyboard-config", current_revision: 1 };
    savedRecord = record;
    api.policy.records.push(record);
    return route.fulfill({ status: 201, json: record });
  });
  api.handlers.set("/api/policy/records/keyboard-config", (route) => route.fulfill({ json: savedRecord }));
  api.handlers.set("/api/policy/records/keyboard-config/revisions", (route) => route.fulfill({ json: [{ number: 1 }] }));
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  await source.fill(content);
  await expect(page.locator("#zpr-config-save")).toBeEnabled();
  page.once("dialog", (dialog) => dialog.accept("Keyboard draft"));
  await source.press("Control+s");
  await expect(page.locator("#zpr-config-status")).toContainText("runtime unchanged");
  await expect(page.locator("#zpr-config-title")).toHaveText("Keyboard draft");
  expect(api.counts.get("/api/policy/records")).toBe(1);
  expect(api.counts.get("/api/policy/config/apply") || 0).toBe(0);
});

test("Policy Save shortcut opens the existing revision confirmation without saving implicitly", async ({ page, appURL, api }) => {
  const mutations = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (request.method() === "POST" && path.startsWith("/api/policy/") && path !== "/api/policy/check") mutations.push(request.url());
  });
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const source = page.locator("#policy-source");
  await source.fill("define KeyboardEmployee as user.\n");
  await expect(page.locator("#policy-save")).toBeEnabled();
  await source.press("Control+s");
  await expect(page.locator("#version-dialog")).toBeVisible();
  expect(mutations).toEqual([]);
  await page.locator("#version-dialog").getByRole("button", { name: "Close", exact: true }).click();
  await expect(page.locator("#version-dialog")).toBeHidden();
  await expect(source).toHaveValue("define KeyboardEmployee as user.\n");
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
  await page.getByRole("link", { name: "Config", exact: true }).click();
  await expect(page.locator("#page-zpr-config")).toBeVisible();
  await page.getByLabel("ZPLC configuration source").fill(content);
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  await expect(page.locator("#zpr-config-status")).toContainText("runtime configuration is unchanged");
  await page.locator("#zpr-config-files-toggle").click();
  page.once("dialog", (dialog) => dialog.accept("Local node configuration"));
  await page.getByRole("menuitem", { name: "Save", exact: true }).click();
  await expect(page.locator("#zpr-config-status")).toContainText("runtime unchanged");
  await expect(page.locator("#zpr-config-title")).toHaveText("Local node configuration");
  await expect(page.locator("#zpr-config-name")).toHaveCount(0);
  await expect(page.locator("#zpr-config-history-count")).toHaveText("1 version");
  expect(api.counts.get("/api/policy/config/apply") || 0).toBe(0);
});

test("GUI Policy keeps History beside the title and blocks saving Untitled", async ({ page, appURL, api }) => {
  api.policy.records[0].name = "Untitled";
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await expect(page.locator("#policy-record-title")).toHaveText("Untitled");
  await expect(page.locator("#policy-edit-state > #policy-history-menu")).toBeVisible();
  await expect(page.locator("#policy-revision-label")).toBeHidden();
  await page.locator("#policy-source").fill("define Reserved as user.\n");
  await openPolicyFiles(page);
  await page.locator("#policy-save").click();
  await expect(page.locator("#policy-test-status")).toHaveText("Enter a policy name other than Untitled before saving.");
  await expect(page.locator("#version-dialog")).toBeHidden();
  expect(api.counts.get("/api/policy/records/test-policy/revisions") || 0).toBe(1);
});

test("GUI ZPR Config asks for a name on Save and never persists Untitled", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#zpr-config");
  await page.locator("#zpr-config-source").fill('[service]\nname = "example"\n');
  for (const name of [null, "", " Untitled ", "uNtItLeD", "x".repeat(161)]) {
    await page.locator("#zpr-config-files-toggle").click();
    page.once("dialog", (dialog) => name === null ? dialog.dismiss() : dialog.accept(name));
    await page.locator("#zpr-config-save").click();
    await expect(page.locator("#zpr-config-title")).toHaveText("Untitled");
    if (name !== null) await expect(page.locator("#zpr-config-status")).toHaveAttribute("data-state", "error");
    expect(api.counts.get("/api/policy/records") || 0).toBe(0);
    expect(api.counts.get("/api/policy/categories") || 0).toBe(0);
  }
});

test("shared editor JSON transport preserves options, structured errors, abort identity and single-attempt mutations", async ({ page, appURL }) => {
  await page.goto(appURL + "/#policy");
  const result = await page.evaluate(async () => {
    const request = window.ZPREditorPage.requestJSON;
    const controller = new AbortController();
    const options = { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": "fixture" }, body: '{"source":"draft"}', signal: controller.signal };
    let calls = 0;
    let forwarded;
    const fetcher = async (url, received) => {
      calls++;
      forwarded = { url, cache: received.cache, method: received.method, headers: received.headers, body: received.body, signal: received.signal === options.signal };
      return new Response('{"valid":true}', { status: 200 });
    };
    const data = await request(fetcher, "/independent-editor-service", options);
    const failure = async (body, status, policy) => {
      try {
        return { data: await request(async () => { calls++; return new Response(body, { status }); }, "/editor", options, policy) };
      } catch (error) {
        return { message: error.message, line: error.line, details: error.details, cause: error.cause?.name };
      }
    };
    const structured = await failure('{"error":"Revision conflict","diagnostics":"Secondary","line":2,"valid":false}', 409);
    const diagnostics = await failure('{"diagnostics":"Source invalid","line":3}', 422);
    const fallback = await failure("null", 503);
    const malformedSuccess = await failure("<html>not JSON</html>", 200);
    const malformedFailure = await failure("<html>not JSON</html>", 502);
    const validation = await failure('{"valid":false,"diagnostics":"Destination missing"}', 422, { acceptError: (value) => value?.valid === false });
    const unauthorized = await failure('{"error":"Sign in required"}', 403, { acceptError: (value) => value?.valid === false });
    const abort = new DOMException("Cancelled", "AbortError");
    let sameAbort = false;
    try { await request(async () => { throw abort; }, "/editor", options); } catch (error) { sameAbort = error === abort; }
    let sameBodyAbort = false;
    try { await request(async () => ({ json: async () => { throw abort; } }), "/editor", options); } catch (error) { sameBodyAbort = error === abort; }
    let sameNetworkError = false;
    const networkError = new TypeError("Connection lost");
    try { await request(async () => { throw networkError; }, "/editor", options); } catch (error) { sameNetworkError = error === networkError; }
    let cacheOverride;
    await request(async (_, received) => { cacheOverride = received.cache; return new Response("{}"); }, "/editor", { cache: "reload" });
    return { data, forwarded, calls, structured, diagnostics, fallback, malformedSuccess, malformedFailure, validation, unauthorized, sameAbort, sameBodyAbort, sameNetworkError, cacheOverride };
  });
  expect(result.data).toEqual({ valid: true });
  expect(result.forwarded).toEqual({ url: "/independent-editor-service", cache: "no-store", method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": "fixture" }, body: '{"source":"draft"}', signal: true });
  expect(result.calls).toBe(8);
  expect(result.structured).toMatchObject({ message: "Revision conflict", line: 2, details: { valid: false, error: "Revision conflict" } });
  expect(result.diagnostics).toMatchObject({ message: "Source invalid", line: 3 });
  expect(result.fallback.message).toBe("HTTP 503");
  expect(result.malformedSuccess).toMatchObject({ message: "HTTP 200: invalid JSON response", cause: "SyntaxError" });
  expect(result.malformedFailure).toMatchObject({ message: "HTTP 502: invalid JSON response", cause: "SyntaxError" });
  expect(result.validation.data).toEqual({ valid: false, diagnostics: "Destination missing" });
  expect(result.unauthorized.message).toBe("Sign in required");
  expect(result.sameAbort).toBe(true);
  expect(result.sameBodyAbort).toBe(true);
  expect(result.sameNetworkError).toBe(true);
  expect(result.cacheOverride).toBe("reload");
});

test("GUI ZPR Config reports malformed validation JSON without changing source or inventing gutter lines", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/config/check", (route) => route.fulfill({ status: 200, contentType: "text/html", body: "<html>service unavailable</html>" }));
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  const text = '[service]\nname   =   "unchanged"\n';
  await source.fill(text);
  await page.locator("#zpr-config-format").click();
  await expect(page.locator("#zpr-config-status")).toHaveText("HTTP 200: invalid JSON response");
  await expect(source).toHaveValue(text);
  await expect(page.locator("#zpr-config-gutter button")).toHaveCount(0);
  expect(api.counts.get("/api/policy/config/check")).toBe(1);
});

test("Assertion Format reports malformed JSON and preserves the unsaved source", async ({ page, appURL, api }) => {
  api.handlers.set("/api/assertions/format", (route) => route.fulfill({ status: 502, contentType: "text/html", body: "upstream failure" }));
  await openAssertionRecord(page, appURL);
  const source = page.locator("#assertion-source");
  const text = 'group "Operators" members >= 1;';
  await source.fill(text);
  await page.locator("#assertion-format").click();
  await expect(page.locator("#assertion-message")).toHaveText("HTTP 502: invalid JSON response");
  await expect(source).toHaveValue(text);
  expect(api.counts.get("/api/assertions/format")).toBe(1);
});

test("Gateways rejects malformed analysis and failed saves without retrying or replacing the draft", async ({ page, appURL, api }) => {
  const contract = { organization_id: "alpha", instance_id: "public-egress", adapter_cn: "gateway-public-egress", service_name: "public-egress.svc.zpr" };
  api.handlers.set("/api/gateways/contracts", (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [contract] } }));
  api.handlers.set("/api/gateways/configs", (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
  api.handlers.set("/api/gateways/config/check", (route) => route.fulfill({ status: 200, contentType: "text/html", body: "not JSON" }));
  api.handlers.set("/api/gateways/configs/public-egress/revisions", (route) => route.fulfill({ status: 422, json: { valid: false, error: "Draft rejected; not saved" } }));
  await page.goto(appURL + "/#gateways");
  const source = page.locator("#gateway-source");
  await expect(source).toHaveValue(/public-egress/);
  const text = (await source.inputValue()).replace('"origin": ""', '"origin": "https://example.com"');
  await source.fill(text);
  await page.locator("#gateway-analyze").click();
  await expect(page.locator("#gateway-draft-message")).toHaveText("HTTP 200: invalid JSON response");
  await expect(page.locator("#gateway-analyze")).toHaveAttribute("data-analysis-state", "error");
  await expect(source).toHaveValue(text);
  api.handlers.set("/api/gateways/config/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Valid" } }));
  await page.locator("#gateway-analyze").click();
  await expect(page.locator("#gateway-analyze")).toHaveAttribute("data-analysis-state", "success");
  await page.getByRole("button", { name: "File..." }).click();
  await page.locator("#gateway-save").click();
  await expect(page.locator("#gateway-draft-message")).toHaveText("Draft rejected; not saved");
  await expect(source).toHaveValue(text);
  await page.getByRole("button", { name: "File..." }).click();
  await expect(page.locator("#gateway-save")).toBeEnabled();
  expect(api.counts.get("/api/gateways/configs/public-egress/revisions")).toBe(1);
  expect(api.counts.get("/api/gateways/config/check")).toBe(2);
});

test("shared source layout synchronizes overlay and both gutter modes and disposes scroll and resize wiring", async ({ page, appURL }) => {
  await page.goto(appURL + "/#policy");
  const result = await page.evaluate(async () => {
    const host = document.createElement("div");
    host.style.cssText = "position:fixed;top:0;left:0;width:500px;z-index:10000";
    const source = document.createElement("textarea");
    const pre = document.createElement("pre");
    const gutter = document.createElement("div");
    const rows = document.createElement("div");
    for (const element of [source, pre, gutter]) {
      element.style.cssText = "display:block;width:240px;height:100px;overflow:scroll;white-space:pre;font:12px/20px monospace";
    }
    source.wrap = "off";
    source.value = Array.from({ length: 100 }, () => "long-source ".repeat(100)).join("\n");
    pre.textContent = source.value;
    rows.textContent = source.value;
    gutter.append(rows); host.append(source, pre, gutter); document.body.append(host);
    const tick = () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    let scrolls = 0;
    let resizes = 0;
    const native = window.ZPREditorPage.bindSourceLayout({
      source, highlight: pre, gutter, container: host,
      onScroll: () => scrolls++, onResize: () => resizes++,
    });
    await tick();
    source.scrollTop = 240; source.scrollLeft = 70;
    source.dispatchEvent(new Event("scroll"));
    await tick();
    const expected = [source.scrollTop, source.scrollLeft];
    const actual = [pre.scrollTop, pre.scrollLeft, gutter.scrollTop];
    const overflow = host.dataset.horizontalOverflow;
    source.style.height = "130px";
    const beforeResize = resizes;
    await tick();
    const resizeObserved = resizes > beforeResize;
    native.dispose(); native.dispose();
    const callbacks = [scrolls, resizes];
    source.scrollTop = 400; source.scrollLeft = 100;
    source.style.height = "150px";
    source.dispatchEvent(new Event("scroll"));
    native.syncScroll();
    await tick();
    const disposedActual = [pre.scrollTop, pre.scrollLeft, gutter.scrollTop];
    const disposedCallbacks = [scrolls, resizes];
    const translated = window.ZPREditorPage.bindSourceLayout({ source, highlight: pre, gutterContent: rows });
    translated.syncScroll();
    const transform = rows.style.transform;
    const translatedExpected = `translateY(${-source.scrollTop}px)`;
    translated.dispose(); host.remove();
    return { expected, actual, overflow, resizeObserved, callbacks, disposedActual, disposedCallbacks, transform, translatedExpected };
  });
  expect(result.actual).toEqual([...result.expected, result.expected[0]]);
  expect(result.overflow).toBe("true");
  expect(result.resizeObserved).toBe(true);
  expect(result.disposedActual).toEqual(result.actual);
  expect(result.disposedCallbacks).toEqual(result.callbacks);
  expect(result.transform).toBe(result.translatedExpected);
});

test("Policy editor maximizes and restores with accessible focus, scroll and background ownership", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  const source = page.locator("#policy-source");
  const original = await source.inputValue();
  const pane = page.locator("#policy-editor-pane");
  const maximize = page.locator("#policy-editor-maximize");
  const before = await pane.boundingBox();
  await page.evaluate(() => { document.getElementById("policy-assistant-pane").inert = true; });
  await maximize.click();
  await expect(maximize).toHaveText("Restore");
  await expect(maximize).toHaveAttribute("aria-label", "Restore editor");
  await expect(maximize).toHaveAttribute("aria-pressed", "true");
  await expect(pane).toHaveAttribute("role", "dialog");
  await expect(pane).toHaveAttribute("aria-modal", "true");
  await expect(maximize).toBeFocused();
  const maximized = await pane.boundingBox();
  const state = await page.evaluate(() => ({
    viewport: [innerWidth, innerHeight],
    assistantInert: document.getElementById("policy-assistant-pane").inert,
    catalogInert: document.getElementById("policy-catalog-pane").inert,
    navInert: document.querySelector(".topbar").inert,
    bodyLocked: document.body.classList.contains("editor-page-maximized"),
  }));
  expect(Math.abs(maximized.x)).toBeLessThanOrEqual(1);
  expect(Math.abs(maximized.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(maximized.width - state.viewport[0])).toBeLessThanOrEqual(1);
  expect(Math.abs(maximized.height - state.viewport[1])).toBeLessThanOrEqual(1);
  expect(state).toMatchObject({ assistantInert: true, catalogInert: true, navInert: true, bodyLocked: true });
  const focusOrder = await pane.evaluate((element) => {
    const focusable = [...element.querySelectorAll('a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), summary, [tabindex]:not([tabindex="-1"])')].filter((item) => item.getClientRects().length && !item.closest("[hidden]"));
    focusable[0].focus();
    return { first: focusable[0].id, last: focusable.at(-1).id };
  });
  await page.keyboard.press("Shift+Tab");
  await expect(page.locator(`#${focusOrder.last}`)).toBeFocused();
  await maximize.click();
  await expect(maximize).toHaveText("Maximize");
  await expect(maximize).toHaveAttribute("aria-pressed", "false");
  await expect(pane).not.toHaveAttribute("role", "dialog");
  await expect(source).toHaveValue(original);
  expect(await page.evaluate(() => ({
    assistantInert: document.getElementById("policy-assistant-pane").inert,
    catalogInert: document.getElementById("policy-catalog-pane").inert,
    navInert: document.querySelector(".topbar").inert,
    bodyLocked: document.body.classList.contains("editor-page-maximized"),
  }))).toEqual({ assistantInert: true, catalogInert: false, navInert: false, bodyLocked: false });
  const restored = await pane.boundingBox();
  expect(restored.x).toBeCloseTo(before.x, 0);
  expect(restored.y).toBeCloseTo(before.y, 0);
});

test("Policy editor restores from Escape and returns focus to the restore control", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  const maximize = page.locator("#policy-editor-maximize");
  await maximize.click();
  await expect(maximize).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("Escape");
  await expect(maximize).toHaveAttribute("aria-pressed", "false");
  await expect(maximize).toBeFocused();
  await expect(page.locator("#policy-editor-pane")).not.toHaveAttribute("aria-modal", "true");
});

test("Shared editor-page maximize works for Config, Gateways, Scenarios, and Directory", async ({ page, appURL, api }) => {
  const maximizeAndRestore = async (buttonID, paneID, keepsDialogOpen = false) => {
    const button = page.locator(`#${buttonID}`);
    const pane = page.locator(`#${paneID}`);
    await expect(button).toBeVisible();
    await button.click();
    await expect(button).toHaveAttribute("aria-pressed", "true");
    await expect(button).toHaveText("Restore");
    await expect(button).toBeFocused();
    expect(await page.locator("body").evaluate((element) => element.classList.contains("editor-page-maximized"))).toBe(true);
    expect(await page.locator(".topbar").evaluate((element) => element.inert)).toBe(true);
    await expect.poll(async () => {
      const box = await pane.boundingBox();
      if (!box) return false;
      const viewport = page.viewportSize();
      return Math.abs(box.x) <= 1 && Math.abs(box.y) <= 1 &&
        Math.abs(box.width - viewport.width) <= 1 && Math.abs(box.height - viewport.height) <= 1;
    }).toBe(true);
    await page.keyboard.press("Escape");
    await expect(button).toHaveAttribute("aria-pressed", "false");
    await expect(button).toHaveText("Maximize");
    await expect(button).toBeFocused();
    expect(await page.locator("body").evaluate((element) => element.classList.contains("editor-page-maximized"))).toBe(false);
    if (keepsDialogOpen) await expect(pane).toHaveAttribute("open", "");
    else await expect(pane).not.toHaveAttribute("role", "dialog");
  };

  await page.goto(appURL + "/#zpr-config");
  await maximizeAndRestore("editor-maximize-page-zpr-config", "page-zpr-config");
  await page.goto(appURL + "/#gateways");
  await maximizeAndRestore("editor-maximize-page-gateways", "page-gateways");
  await openRawScenario(page, appURL, api);
  await maximizeAndRestore("editor-maximize-scenario-editor-dialog", "scenario-editor-dialog", true);
  api.handlers.set("/api/simulator/organizations/alpha/directory", (route) => route.fulfill({ json: {
    revision: 2, published_revision: 1, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" },
  } }));
  api.handlers.set("/api/simulator/organizations/alpha/directory/revisions", (route) => route.fulfill({ json: [] }));
  await page.goto(appURL + "/organizations.html");
  await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
  await expect(page.locator("#directory-editor-dialog")).toBeVisible();
  await maximizeAndRestore("editor-maximize-directory-editor-dialog", "directory-editor-dialog", true);
});

test("Assertion scrolling retains highlighted nodes while synchronizing the overlay", async ({ page, appURL, api }) => {
  await openAssertionRecord(page, appURL);
  await page.locator("#assertion-source").fill(Array.from({ length: 100 }, () => `group "${"Operators".repeat(70)}" members >= 1;`).join("\n"));
  const result = await page.evaluate(() => {
    const source = document.getElementById("assertion-source");
    const pre = document.getElementById("assertion-highlight");
    const token = pre.firstChild;
    source.scrollTop = 250; source.scrollLeft = 80;
    source.dispatchEvent(new Event("scroll"));
    return { retained: token === pre.firstChild, source: [source.scrollTop, source.scrollLeft], overlay: [pre.scrollTop, pre.scrollLeft], transform: document.getElementById("assertion-result-lines").style.transform };
  });
  expect(result.retained).toBe(true);
  expect(result.overlay).toEqual(result.source);
  expect(result.transform).toBe(`translateY(${-result.source[0]}px)`);
});

test("Policy shared source layout preserves token nodes and gutter bounds through scrolling and resize", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await expect(page.locator("#policy-source")).toBeEditable();
  await page.locator("#policy-source").fill(`# ${"wide ".repeat(80)}\n` + Array.from({ length: 100 }, () => "define Employee as user.").join("\n"));
  const result = await page.evaluate(() => {
    const source = document.getElementById("policy-source");
    const pre = document.getElementById("policy-highlight");
    const token = pre.firstChild;
    source.scrollTop = 250; source.scrollLeft = 80;
    source.dispatchEvent(new Event("scroll"));
    return {
      retained: token === pre.firstChild, source: [source.scrollTop, source.scrollLeft],
      overlay: [pre.scrollTop, pre.scrollLeft], transform: document.getElementById("policy-test-gutter-content").style.transform,
      overflow: document.getElementById("policy-code-editor").dataset.horizontalOverflow,
    };
  });
  expect(result.retained).toBe(true);
  expect(result.overlay).toEqual(result.source);
  expect(result.transform).toBe(`translateY(${-result.source[0]}px)`);
  expect(result.overflow).toBe("true");
  const viewport = page.viewportSize();
  await page.setViewportSize({ width: viewport.width - 80, height: viewport.height + 80 });
  await expect.poll(() => page.evaluate(() => {
    const source = document.getElementById("policy-source");
    return document.getElementById("policy-test-gutter").style.bottom === `${source.offsetHeight - source.clientHeight}px`;
  })).toBe(true);
});

test("shared viewport binding is idempotent, coalesces requests and disposes pending layout work", async ({ page, appURL }) => {
  await page.goto(appURL + "/#policy");
  const result = await page.evaluate(async () => {
    const host = document.createElement("div");
    host.className = "main-content policy-page";
    host.style.cssText = "position:fixed;top:120px;left:0;width:400px;padding:0 0 20px";
    host.innerHTML = '<div class="policy-editor-pane" style="padding:0 0 10px"><div id="viewport-contract"></div></div>';
    document.body.append(host);
    const container = host.querySelector("#viewport-contract");
    container.style.setProperty("--editor-source-height", "401px", "important");
    const expectedHeight = () => {
      const padding = Number.parseFloat(getComputedStyle(host).paddingBottom) +
        Number.parseFloat(getComputedStyle(container.parentElement).paddingBottom);
      return Math.max(320, innerHeight - Math.min(innerHeight * .04, padding) - container.getBoundingClientRect().top - scrollY);
    };
    const tick = () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const first = window.ZPREditorPage.fitSourceToViewport(container);
    const same = first === window.ZPREditorPage.fitSourceToViewport(container);
    const changes = [];
    const observer = new MutationObserver((records) => changes.push(...records));
    observer.observe(container, { attributes: true, attributeFilter: ["style"] });
    first.schedule(); first.schedule(); first.schedule();
    await tick();
    const writes = changes.length;
    const fitted = Number.parseFloat(container.style.getPropertyValue("--editor-source-height"));
    const expected = expectedHeight();
    host.hidden = true;
    changes.length = 0;
    first.schedule();
    await tick();
    const hiddenWrites = changes.length;
    host.hidden = false;
    first.schedule();
    first.dispose(); first.dispose();
    window.dispatchEvent(new Event("resize"));
    window.dispatchEvent(new Event("hashchange"));
    await tick();
    const restored = container.style.getPropertyValue("--editor-source-height");
    const priority = container.style.getPropertyPriority("--editor-source-height");
    changes.length = 0;
    first.schedule();
    await tick();
    const disposedWrites = changes.length;
    const second = window.ZPREditorPage.fitSourceToViewport(container);
    const rebound = second !== first;
    await tick();
    const reboundHeight = Number.parseFloat(container.style.getPropertyValue("--editor-source-height"));
    const reboundExpected = expectedHeight();
    second.dispose();
    observer.disconnect(); host.remove();
    return { same, writes, fitted, expected, hiddenWrites, restored, priority, disposedWrites, rebound, reboundHeight, reboundExpected };
  });
  expect(result.same).toBe(true);
  expect(result.writes).toBe(1);
  expect(result.fitted).toBe(result.expected);
  expect(result.hiddenWrites).toBe(0);
  expect(result.restored).toBe("401px");
  expect(result.priority).toBe("important");
  expect(result.disposedWrites).toBe(0);
  expect(result.rebound).toBe(true);
  expect(result.reboundHeight).toBe(result.reboundExpected);
});

test("GUI ZPR Config stretches to the same bottom margin as Policy with History beside its title", async ({ page, appURL, api }) => {
  for (const [width, height] of [[1280, 720], [1280, 1000], [834, 900]]) {
    await page.setViewportSize({ width, height });
    await page.goto(appURL + "/#policy");
    await openPolicyPicker(page);
    await page.locator('[data-record-id="test-policy"]').click();
    await expect(page.locator("#policy-source")).toBeEnabled();
    await expect.poll(async () => Math.abs(await page.locator("#policy-code-editor").evaluate((element) => element.getBoundingClientRect().bottom) - height * .96)).toBeLessThanOrEqual(2);
    const policyBottom = await page.locator("#policy-code-editor").evaluate((element) => element.getBoundingClientRect().bottom);
    await page.goto(appURL + "/#zpr-config");
    await expect(page.locator("#editor-assistant-zpr-config-pane")).toBeVisible();
    await expect.poll(async () => Math.abs(await page.locator("#page-zpr-config .config-source-editor").evaluate((element) => element.getBoundingClientRect().bottom) - height * .96)).toBeLessThanOrEqual(2);
    const layout = await page.evaluate(() => {
      const title = document.getElementById("zpr-config-title").getBoundingClientRect();
      const history = document.querySelector("#zpr-config-history-menu summary").getBoundingClientRect();
      const editor = document.querySelector("#page-zpr-config .config-source-editor").getBoundingClientRect();
      return { bottom: editor.bottom, titleRight: title.right, titleTop: title.top, historyLeft: history.left, historyTop: history.top };
    });
    expect(Math.abs(layout.bottom - policyBottom), JSON.stringify({ width, height, policyBottom, ...layout })).toBeLessThanOrEqual(2);
    expect(height - layout.bottom).toBeGreaterThanOrEqual(height * .02);
    expect(height - layout.bottom).toBeLessThanOrEqual(height * .08);
    expect(layout.historyLeft).toBeGreaterThanOrEqual(layout.titleRight);
    expect(Math.abs(layout.historyTop - layout.titleTop)).toBeLessThanOrEqual(12);
  }
});

test("GUI new scenarios display Untitled and cannot save that placeholder", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  await expect(page.locator("#scenario-editor-title")).toHaveText("Untitled");
  const source = page.locator("#scenario-editor-source");
  const scenario = JSON.parse(await source.inputValue());
  for (const name of ["Untitled", " untitled "]) {
    await source.fill(JSON.stringify({ ...scenario, name }));
    await page.locator("#scenario-editor-files-toggle").click();
    await page.locator("#scenario-editor-save").click();
    await expect(page.locator("#scenario-editor-status")).toContainText("other than Untitled");
    expect(api.counts.get("/api/simulator/organizations/alpha/scenarios") || 0).toBe(0);
  }
});

test("GUI ZPR Config analyzes gutter errors, formats whitespace safely, and uses File commands", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/config/check", async (route) => {
    const source = route.request().postDataJSON().source;
    if (source.includes("unterminated")) {
      await route.fulfill({ status: 422, json: { valid: false, line: 2, diagnostics: 'Invalid TOML configuration: line 2: unterminated string' } });
      return;
    }
    await route.fulfill({ json: { valid: true, diagnostics: "TOML syntax valid; runtime configuration is unchanged." } });
  });
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  await source.fill('[service]\nname = "unterminated\n');
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  const marker = page.getByRole("button", { name: /Configuration error on line 2/ });
  await expect(marker).toBeVisible();
  await expect(marker).toHaveAttribute("title", /unterminated string/);
  await expect(page.locator("#zpr-config-status")).toBeHidden();
  await expect(page.locator("#zpr-config-validate")).toHaveAttribute("data-analysis-state", "error");
  await marker.click();
  await expect(source).toBeFocused();
  await expect(page.locator("#zpr-config-status")).toBeHidden();
  const toml = '[service]\n name   =   "A = B"  # Keep this comment\nvalue=["x", "y"]\n';
  await source.fill(toml);
  await page.getByRole("button", { name: "Format", exact: true }).click();
  await expect(source).toHaveValue('[service]\n name = "A = B"  # Keep this comment\nvalue = ["x", "y"]\n');
  await expect(page.locator("#zpr-config-gutter button")).toHaveCount(0);
  await page.locator("#zpr-config-files-toggle").click();
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator("#zpr-config-open").click();
  await page.locator("#zpr-config-file-input").setInputFiles({ name: "imported.toml", mimeType: "application/toml", buffer: Buffer.from("[source]\nname = \"Imported\"\n") });
  await expect(source).toHaveValue('[source]\nname = "Imported"\n');
  await expect(page.locator("#zpr-config-title")).toHaveText("imported");
  await page.locator("#zpr-config-files-toggle").click();
  const downloadPromise = page.waitForEvent("download");
  await page.locator("#zpr-config-download").click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("imported.toml");
  await page.locator("#zpr-config-files-toggle").click();
  page.once("dialog", (dialog) => dialog.accept());
  await page.locator("#zpr-config-new").click();
  await expect(source).toHaveValue("");
  await expect(page.locator("#zpr-config-title")).toHaveText("Untitled");
});

test("GUI ZPR Config keeps unlocated failures visible without inventing gutter lines", async ({ page, appURL, api }) => {
  let line = 0;
  api.handlers.set("/api/policy/config/check", async (route) => route.fulfill({
    status: 422, json: { valid: false, line, error: "Configuration validation unavailable." },
  }));
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  for (line of [0, 99, 1.5]) {
    await source.fill('[service]\nname = "example"\n');
    await page.locator("#zpr-config-validate").click();
    await expect(page.locator("#zpr-config-gutter button")).toHaveCount(0);
    await expect(page.locator("#zpr-config-status")).toHaveText("Configuration validation unavailable.");
    await expect(page.locator("#zpr-config-status")).toBeVisible();
    await source.fill("");
    await expect(page.locator("#zpr-config-status")).toBeHidden();
  }
});

test("GUI ZPR Config colors TOML safely, keeps a visible aligned gutter, and colors Analyze", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/config/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Valid TOML" } }));
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  const content = '# Comment\n[service]\nname = "<img src=x>"\nenabled = true\nport = 123\n';
  await source.fill(content);
  const highlight = page.locator("#zpr-config-highlight");
  await expect(highlight.locator(".zpl-comment")).toHaveText("# Comment");
  await expect(highlight.locator(".zpl-string")).toHaveText('"<img src=x>"');
  await expect(highlight.locator("img")).toHaveCount(0);
  await expect(highlight.locator(".zpl-keyword")).toHaveText("true");
  await expect(highlight.locator(".zpl-value")).toHaveText("123");
  await source.fill(content + 'list = ["red", "green"]\n');
  await expect(highlight.locator(".zpl-string")).toHaveText(['"<img src=x>"', '"red"', '"green"']);
  await expect(highlight.locator(".zpl-string").first()).toHaveCSS("color", "rgb(215, 167, 207)");
  await expect(page.locator("#zpr-config-gutter")).toHaveCSS("border-right-style", "solid");
  const lineGeometry = await page.evaluate(() => {
    const sourceStyle = getComputedStyle(document.getElementById("zpr-config-source"));
    const row = document.querySelector("#zpr-config-gutter .config-gutter-line");
    return { rowHeight: row.getBoundingClientRect().height, lineHeight: Number.parseFloat(sourceStyle.lineHeight) };
  });
  expect(Math.abs(lineGeometry.rowHeight - lineGeometry.lineHeight)).toBeLessThan(1);
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  await expect(page.locator("#zpr-config-validate")).toHaveAttribute("data-analysis-state", "success");
  await source.fill(Array.from({ length: 100 }, (_, index) => `key${index} = "${"x".repeat(150)}"`).join("\n"));
  await source.evaluate((element) => { element.scrollTop = 150; element.scrollLeft = 60; element.dispatchEvent(new Event("scroll")); });
  const scrolls = await page.evaluate(() => ({
    source: [document.getElementById("zpr-config-source").scrollTop, document.getElementById("zpr-config-source").scrollLeft],
    highlight: [document.getElementById("zpr-config-highlight").scrollTop, document.getElementById("zpr-config-highlight").scrollLeft],
    gutter: document.getElementById("zpr-config-gutter").scrollTop,
  }));
  expect(scrolls.highlight).toEqual(scrolls.source);
  expect(scrolls.gutter).toBe(scrolls.source[0]);
  await expect(page.locator("#zpr-config-validate")).not.toHaveAttribute("data-analysis-state", "success");
  await expect(page.locator("#zpr-config-file-menu #zpr-config-save")).toHaveCount(1);
});

test("GUI ZPR Config rejects stale diagnostics after source edits", async ({ page, appURL, api }) => {
  let finish;
  api.handlers.set("/api/policy/config/check", async (route) => {
    await new Promise((resolve) => { finish = resolve; });
    await route.fulfill({ status: 422, json: { valid: false, line: 2, diagnostics: "Old source error" } });
  });
  await page.goto(appURL + "/#zpr-config");
  await page.locator("#zpr-config-source").fill('[old]\nkey = "broken');
  await page.locator("#zpr-config-validate").click();
  await expect.poll(() => Boolean(finish)).toBe(true);
  await page.locator("#zpr-config-source").fill('[new]\nkey = "valid"\n');
  finish();
  await expect(page.locator("#zpr-config-validate")).toBeEnabled();
  await expect(page.locator("#zpr-config-gutter button")).toHaveCount(0);
  await expect(page.locator("#zpr-config-status")).toBeHidden();
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

test("Simulator Agents redirects to Workers with device filtering", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/agents.html");
  await expect(page).toHaveURL(appURL + "/machine-logs.html");
  await expect(page).toHaveTitle("ZPR Simulator Workers");
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

test("Simulator Activity uses shared empty rows for empty result sets", async ({ page, appURL, api }) => {
  api.handlers.set("/api/simulator/activity", async (route) => route.fulfill({ json: {
    generated_at: "2026-10-05T12:00:00Z",
    stats: {},
    visas: [],
    denies: [],
  } }));
  await page.goto(appURL + "/activity.html");
  const visas = page.locator('table[data-sort-page="activity-visas"] tbody tr');
  const denials = page.locator('table[data-sort-page="activity-denies"] tbody tr');
  await expect(visas).toHaveCount(1);
  await expect(visas.locator("td")).toHaveAttribute("colspan", "5");
  await expect(visas).toContainText("No visa activity yet.");
  await expect(denials).toHaveCount(1);
  await expect(denials.locator("td")).toHaveAttribute("colspan", "5");
  await expect(denials).toContainText("No denied flows yet.");
});

test("shared sortable-table helpers preserve comparison, accessibility, idempotence and empty-row contracts", async ({ page, appURL }) => {
  await page.goto(appURL + "/activity.html");
  const result = await page.evaluate(() => {
    const table = document.createElement("table");
    const head = table.createTHead().insertRow();
    for (const [key, label] of [["name", "Name"], ["count", "Count"]]) {
      const header = document.createElement("th");
      header.dataset.sortKey = key;
      header.textContent = label;
      head.append(header);
    }
    const body = table.createTBody();
    document.body.append(table);
    let sort = { key: "name", direction: 1 };
    const changes = [];
    const options = {
      table,
      getSort: () => sort,
      onSort: (next) => { changes.push(next); sort = next; },
    };
    const binding = window.ZPRSortableTable.bindSortableHeaders(options);
    const [name, count] = head.querySelectorAll("th");
    name.click();
    const afterReverse = { ariaSort: name.getAttribute("aria-sort"), label: name.querySelector("button").getAttribute("aria-label") };
    count.click();
    const afterSelect = { oldSort: name.getAttribute("aria-sort"), newSort: count.getAttribute("aria-sort") };
    binding.update();
    window.ZPRSortableTable.bindSortableHeaders(options);
    count.click();
    window.ZPRSortableTable.renderEmptyRow(body, 2, "<No rows>");
    const cell = body.querySelector("td");
    return {
      numeric: window.ZPRSortableTable.compareValues(10, 2),
      numericStrings: window.ZPRSortableTable.compareValues("10", "2", { numericStrings: true }),
      natural: Math.sign(window.ZPRSortableTable.compareValues("entry 2", "entry 10")),
      afterReverse,
      afterSelect,
      finalSort: count.getAttribute("aria-sort"),
      changes,
      empty: { rows: body.rows.length, columns: cell.colSpan, text: cell.textContent, children: cell.childElementCount },
    };
  });
  expect(result.numeric).toBe(8);
  expect(result.numericStrings).toBe(8);
  expect(result.natural).toBe(-1);
  expect(result.afterReverse).toEqual({ ariaSort: "descending", label: "Sort by Name, currently descending" });
  expect(result.afterSelect).toEqual({ oldSort: "none", newSort: "ascending" });
  expect(result.finalSort).toBe("descending");
  expect(result.changes).toEqual([
    { key: "name", direction: -1 },
    { key: "count", direction: 1 },
    { key: "count", direction: -1 },
  ]);
  expect(result.empty).toEqual({ rows: 1, columns: 2, text: "<No rows>", children: 0 });
});

test("shared safe display helpers escape HTML and label known protocols", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  const result = await page.evaluate(() => ({
    escaped: window.ZPRSafeDisplay.escapeHTML(`<tag attr="&">'`),
    protocols: [1, 6, 17, 58, 99, null].map((value) => window.ZPRSafeDisplay.protocolName(value)),
    dates: ["2026-10-08T12:34:56Z", 1791487200000, "invalid"].map((value) => ({
      dateTime: window.ZPRSafeDisplay.formatDateTime(value),
      time: window.ZPRSafeDisplay.formatTime(value),
      nativeDateTime: new Date(value).toLocaleString(),
      nativeTime: new Date(value).toLocaleTimeString(),
    })),
  }));
  expect(result.escaped).toBe("&lt;tag attr=&quot;&amp;&quot;&gt;&#39;");
  expect(result.protocols).toEqual(["ICMP", "TCP", "UDP", "ICMPv6", "", ""]);
  expect(result.dates.map(({ dateTime, nativeDateTime }) => dateTime)).toEqual(result.dates.map(({ nativeDateTime }) => nativeDateTime));
  expect(result.dates.map(({ time, nativeTime }) => time)).toEqual(result.dates.map(({ nativeTime }) => nativeTime));
});

test("Simulator Activity polling aborts on navigation and rejects a late response", async ({ page, appURL, api }) => {
  await page.addInitScript(() => {
    const originalFetch = window.fetch.bind(window);
    const pending = [];
    window.fetch = (input, options = {}) => {
      if (new URL(input, location.href).pathname !== "/api/simulator/activity") return originalFetch(input, options);
      return new Promise((resolve) => pending.push({ signal: options.signal, resolve }));
    };
    window.pendingActivityRequests = pending;
    window.resolveActivityRequest = (index, data) => pending[index].resolve(new Response(JSON.stringify(data), {
      headers: { "Content-Type": "application/json" },
    }));
  });
  await page.goto(appURL + "/activity.html");
  await expect.poll(() => page.evaluate(() => window.pendingActivityRequests.length)).toBe(1);
  await page.evaluate(() => {
    document.getElementById("refresh").click();
    document.getElementById("refresh").click();
  });
  await page.waitForTimeout(50);
  expect(await page.evaluate(() => window.pendingActivityRequests.length)).toBe(1);
  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:deactivate", {
    detail: { path: "/activity.html" },
  })));
  expect(await page.evaluate(() => window.pendingActivityRequests[0].signal.aborted)).toBe(true);
  await page.evaluate(() => window.resolveActivityRequest(0, {
    generated_at: "2026-10-05T12:00:00Z",
    stats: { visa_requests: 99, visa_requests_approved: 99, visa_requests_denied: 0 },
    visas: [], denies: [],
  }));
  await expect(page.locator("#requests")).toHaveText("—");

  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:activate", {
    detail: { path: "/activity.html" },
  })));
  await expect.poll(() => page.evaluate(() => window.pendingActivityRequests.length)).toBe(2);
  await page.evaluate(() => window.resolveActivityRequest(1, {
    generated_at: "2026-10-05T12:00:00Z",
    stats: { visa_requests: 4, visa_requests_approved: 3, visa_requests_denied: 1 },
    visas: [], denies: [],
  }));
  await expect(page.locator("#requests")).toHaveText("4");
});

test("Simulator Scenario polling aborts on navigation and ignores late catalog responses", async ({ page, appURL, api }) => {
  await page.addInitScript(() => {
    const originalFetch = window.fetch.bind(window);
    const pending = [];
    window.fetch = (input, options = {}) => {
      if (new URL(input, location.href).pathname !== "/api/simulator/scenarios") return originalFetch(input, options);
      return new Promise((resolve) => pending.push({ signal: options.signal, resolve }));
    };
    window.pendingScenarioRequests = pending;
    window.resolveScenarioRequest = (index, data) => pending[index].resolve(new Response(JSON.stringify(data), {
      headers: { "Content-Type": "application/json" },
    }));
  });
  await page.goto(appURL + "/scenarios.html");
  await expect.poll(() => page.evaluate(() => window.pendingScenarioRequests.length)).toBe(1);
  await page.evaluate(() => {
    document.getElementById("scenario-refresh").click();
    document.getElementById("scenario-refresh").click();
  });
  await page.waitForTimeout(50);
  expect(await page.evaluate(() => window.pendingScenarioRequests.length)).toBe(1);
  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:deactivate", {
    detail: { path: "/scenarios.html" },
  })));
  expect(await page.evaluate(() => window.pendingScenarioRequests[0].signal.aborted)).toBe(true);
  await page.evaluate(() => window.resolveScenarioRequest(0, {
    active_organization_id: "stale-org",
    organization: { name: "Stale Organization" },
    scenarios: [{ id: "stale-scenario", name: "Stale Scenario", organization_id: "stale-org", steps: [], cleanup: [] }],
    run: { state: "idle", steps: [] },
  }));
  await expect(page.locator("#scenario-connection")).not.toContainText("Stale Organization");
  await expect(page.locator("#scenario-list")).not.toContainText("Stale Scenario");

  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:activate", {
    detail: { path: "/scenarios.html" },
  })));
  await expect.poll(() => page.evaluate(() => window.pendingScenarioRequests.length)).toBe(2);
  await page.evaluate(() => window.resolveScenarioRequest(1, {
    active_organization_id: "alpha",
    organization: { name: "Alpha Labs" },
    scenarios: [{ id: "fresh-scenario", name: "Fresh Scenario", organization_id: "alpha", steps: [], cleanup: [] }],
    run: { state: "idle", steps: [] },
  }));
  await expect(page.locator("#scenario-connection")).toHaveText("Alpha Labs: 1 scenarios");
  await expect(page.locator("#scenario-list")).toContainText("Fresh Scenario");
});

test("Simulator Organization polling aborts on navigation and ignores late catalog responses", async ({ page, appURL, api }) => {
  await page.addInitScript(() => {
    const originalFetch = window.fetch.bind(window);
    const pending = [];
    window.fetch = (input, options = {}) => {
      if (new URL(input, location.href).pathname !== "/api/simulator/organizations" || !options.signal) return originalFetch(input, options);
      return new Promise((resolve) => pending.push({ signal: options.signal, resolve }));
    };
    window.pendingOrganizationRequests = pending;
    window.resolveOrganizationRequest = (index, data) => pending[index].resolve(new Response(JSON.stringify(data), {
      headers: { "Content-Type": "application/json" },
    }));
  });
  await page.goto(appURL + "/organizations.html");
  await expect.poll(() => page.evaluate(() => window.pendingOrganizationRequests.length)).toBe(1);
  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:deactivate", {
    detail: { path: "/organizations.html" },
  })));
  expect(await page.evaluate(() => window.pendingOrganizationRequests[0].signal.aborted)).toBe(true);
  await page.evaluate(() => window.resolveOrganizationRequest(0, {
    active_id: "stale-org",
    activation: { state: "idle" },
    organizations: [{
      id: "stale-org", name: "Stale Organization", policies: [], services: [], runtime: {},
      directory: { base_dn: "dc=stale", people: [], groups: [], departments: [] },
    }],
  }));
  await expect(page.locator("#organization-title")).not.toHaveText("Stale Organization");

  await page.evaluate(() => document.dispatchEvent(new CustomEvent("simulator:activate", {
    detail: { path: "/organizations.html" },
  })));
  await expect.poll(() => page.evaluate(() => window.pendingOrganizationRequests.length)).toBe(2);
  await page.evaluate(() => window.resolveOrganizationRequest(1, {
    active_id: "alpha",
    activation: { state: "idle" },
    organizations: [{
      id: "alpha", name: "Alpha Labs", policies: [], services: [], runtime: {},
      directory: { base_dn: "dc=alpha", people: [], groups: [], departments: [] },
    }],
  }));
  await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
});

test("Simulator catalog pollers report malformed JSON and recover without losing last-good data", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/organizations.html");
  await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
  api.handlers.set("/api/simulator/organizations", (route) => route.fulfill({
    status: 502, contentType: "text/html", body: "upstream unavailable",
  }));
  await page.locator("#organization-refresh").evaluate((button) => button.click());
  await expect(page.locator("#organization-error")).toHaveText("HTTP 502: invalid JSON response");
  await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");

  api.handlers.delete("/api/simulator/organizations");
  await page.locator("#organization-refresh").evaluate((button) => button.click());
  await expect(page.locator("#organization-error")).toBeHidden();
  await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");

  api.handlers.set("/api/simulator/scenarios", async (route) => route.fulfill({ json: {
    active_organization_id: "alpha",
    organization: { name: "Alpha Labs" },
    scenarios: [],
    run: { state: "idle", steps: [] },
  } }));
  await page.goto(appURL + "/scenarios.html");
  await expect(page.locator("#scenario-connection")).toHaveText("Alpha Labs: 0 scenarios");
  api.handlers.set("/api/simulator/scenarios", (route) => route.fulfill({
    status: 502, contentType: "text/html", body: "upstream unavailable",
  }));
  await page.locator("#scenario-refresh").evaluate((button) => button.click());
  await expect(page.locator("#scenario-error")).toHaveText("HTTP 502: invalid JSON response");
  await expect(page.locator("#scenario-connection")).toHaveText("Scenario service unavailable");
  api.handlers.set("/api/simulator/scenarios", async (route) => route.fulfill({ json: {
    active_organization_id: "alpha",
    organization: { name: "Alpha Labs" },
    scenarios: [],
    run: { state: "idle", steps: [] },
  } }));
  await page.locator("#scenario-refresh").evaluate((button) => button.click());
  await expect(page.locator("#scenario-error")).toBeHidden();
  await expect(page.locator("#scenario-connection")).toHaveText("Alpha Labs: 0 scenarios");
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

test("Scenario run-state mutations report malformed JSON without retrying", async ({ page, appURL, api }) => {
  const scenario = {
    id: "sample-flow", name: "Sample flow", folder: "", description: "A browser fixture.",
    organization_id: "alpha", current_revision: 1, published_revision: 1, steps: [], cleanup: [],
  };
  api.handlers.set("/api/simulator/scenarios", (route) => route.fulfill({ json: {
    active_organization_id: "alpha",
    organization: { name: "Alpha Labs" },
    scenarios: [scenario],
    run: { scenario_id: scenario.id, scenario_name: scenario.name, state: "completed", steps: [] },
  } }));
  api.handlers.set("/api/simulator/scenarios/clear", (route) => route.fulfill({
    status: 502, contentType: "text/html", body: "upstream unavailable",
  }));
  await page.goto(appURL + "/scenarios.html");
  await page.locator("#scenario-clear").click();
  await expect(page.locator("#scenario-error")).toHaveText("HTTP 502: invalid JSON response");
  await expect(page.locator("#scenario-clear")).toBeEnabled();
  expect(api.counts.get("/api/simulator/scenarios/clear")).toBe(1);
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

test("Trusted Sources omits explanatory headings, counts, and the top table filter", async ({ page, appURL, api }) => {
  api.snapshot.trusted_sources = [
    { name: "Primary LDAP", provider: "LDAP", actor_cn: "directory-a", health: "working", last_lookup_ms: 1780000000000 },
    { name: "Secondary Directory", provider: "LDAP", actor_cn: "directory-b", health: "unverified", last_lookup_ms: null },
  ];
  await page.goto(appURL + "/#sources");
  const sources = page.locator("#page-sources");
  await expect(sources).toBeVisible();
  await expect(sources).not.toContainText("ATTRIBUTE PROVIDERS");
  await expect(sources).not.toContainText("Who supplies trusted attributes");
  await expect(sources).not.toContainText("attribute sources");
  await expect(sources).not.toContainText("Lookup labels reflect actual attribute requests");
  await expect(page.locator("#trusted-count")).toHaveCount(0);
  await expect(page.locator("#sources-filter")).toHaveCount(0);
  await expect(sources.locator("table[data-sort-page=sources]")).toBeVisible();
  await expect(sources.locator("#trusted-list tr[data-inspect-source]")).toHaveCount(2);
  const browser = sources.locator("trusted-source-browser");
  await expect(browser.locator("[data-source-title]")).toHaveText("Trusted source: Trusted LDAP");
});

test("GUI raw scenario editor switches to YAML and analyzes before saving", async ({ page, appURL, api }) => {
  await openRawScenario(page, appURL, api);
  const source = page.locator("#scenario-editor-source");
  await page.locator("#scenario-editor-files-toggle").click();
  await page.getByRole("menuitem", { name: "Convert to YAML" }).click();
  await expect(source).toHaveValue(/id: new-scenario/);
  await source.fill('id: new-scenario\norganization_id: alpha\nname: "YAML scenario"\ndescription: "Edited as YAML"\n');
  await page.locator("#scenario-source-analyze").click();
  await expect(page.locator("#scenario-source-analyze")).toHaveAttribute("data-analysis-state", "success");
  await expect(page.locator("#scenario-source-highlight .zpl-attribute").first()).toBeVisible();
  await page.locator("#scenario-editor-files-toggle").click();
  await page.getByRole("menuitem", { name: "Save", exact: true }).click();
  await expect.poll(() => api.counts.get("/api/simulator/organizations/alpha/scenarios") || 0).toBe(1);
});

test("Control Room sidebar links Adapter Logs internally and promotes external managers", async ({ page, appURL, api }) => {
  const directoryURLs = [`${appURL}/?directory=one`, `${appURL}/?directory=two`];
  api.snapshot.trusted_sources = ["Directory A", "Directory B"].map((name, index) => ({ name, provider: "file", actor_cn: "directory-service", health: "working", last_lookup_ms: Date.now(), last_success_ms: Date.now(), editor_url: directoryURLs[index] }));
  await page.goto(appURL + "/#sources");
  const nav = page.locator(".primary-nav");
  await expect(nav.getByRole("link", { name: "Adapter Logs", exact: true })).toHaveAttribute("href", "#adapter-logs");
  await expect(nav.getByRole("link", { name: /LDAP control GUI/i })).toHaveCount(0);
  const logManager = nav.getByRole("link", { name: "Log Manager", exact: true });
  await expect(logManager).toHaveAttribute("href", "http://127.0.0.1:8800/");
  await expect(logManager).toHaveAttribute("target", "zpr-log-manager");
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
  await expect(browser.locator("[data-source-title]")).toHaveText("Trusted source: Trusted LDAP");
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

test("Control Room Trusted Sources shows one-day metadata-only updates with cursor paging", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#sources");
  const browser = page.locator("trusted-source-browser");
  await browser.getByRole("tab", { name: "Updates (24h)" }).click();
  await expect(browser.getByRole("combobox", { name: "Trusted source update feed" })).toHaveValue("great_lakes_ldap");
  await expect(browser.locator(".trusted-source-changes")).toContainText("uid=alice,dc=alpha,dc=test");
  await expect(browser.locator(".trusted-source-changes")).toContainText("mail, title");
  await expect(browser.locator(".trusted-source-changes")).not.toContainText("alice@example.test");
  const nextPage = browser.getByRole("button", { name: "Load more updates" });
  await expect(nextPage).toBeVisible();
  await nextPage.click();
  await expect(browser.locator(".trusted-source-changes tbody tr")).toHaveCount(2);
  await expect(browser.locator(".trusted-source-changes")).toContainText("uid=alice,ou=People,dc=alpha,dc=test");
  expect(api.counts.get("/api/trusted-sources/change-feeds") || 0).toBe(1);
  expect(api.counts.get("/api/trusted-sources/change-feeds/great_lakes_ldap/changes") || 0).toBe(2);
});

test("Control Room Trusted Sources can continue through an empty update page", async ({ page, appURL, api }) => {
  api.trustedChanges = { changes: [], cursor: "empty-page-cursor", more: true };
  await page.goto(appURL + "/#sources");
  const browser = page.locator("trusted-source-browser");
  await browser.getByRole("tab", { name: "Updates (24h)" }).click();
  await expect(browser.locator("[data-source-message]")).toHaveText("No updates on this page; more history is available.");
  const nextPage = browser.getByRole("button", { name: "Load more updates" });
  await expect(nextPage).toBeVisible();
  await nextPage.click();
  await expect(browser.locator(".trusted-source-changes tbody tr")).toHaveCount(1);
  await expect(browser.locator(".trusted-source-changes")).toContainText("uid=alice,dc=alpha,dc=test");
});

test("Trusted Sources tables sort, split group cn/objectClass and condense people rows", async ({ page, appURL, api }) => {
  const directory = api.assertionSource.directory;
  directory.person_attributes.alice.cn = ["Alice Adams"];
  directory.person_attributes.alice.ou = ["Engineering"];
  directory.person_attributes.alice.l = ["Milwaukee, Wisconsin"];
  directory.person_attributes.bob.cn = ["Bob Brown"];
  directory.groups.Admins = ["alice"];
  directory.group_attributes.Operators = { cn: ["Operators"], objectclass: ["groupOfNames", "top"], description: ["Directory operators"] };
  directory.group_attributes.Admins = { cn: ["Admins"], objectclass: ["groupOfNames"] };
  await page.goto(appURL + "/#sources");
  const browser = page.locator("trusted-source-browser");
  const table = browser.locator(".trusted-source-table");
  await expect(table.locator("thead th")).toHaveText(["IDENTITY", "NAME", "TITLE", "UNIT", "MAIL"]);
  await expect(table.locator("thead th.sortable-heading .sort-button")).toHaveCount(5);
  await expect(table.locator('thead th[aria-sort="ascending"]')).toHaveText("IDENTITY");
  await expect(browser.locator("[data-source-count]")).toHaveText("2 people");
  await expect(table).not.toContainText("Milwaukee");
  const rows = table.locator("tbody tr[data-source-row]");
  await expect(rows.locator("th")).toHaveText(["alice", "bob"]);
  await table.getByRole("button", { name: /Sort by TITLE/ }).click();
  await expect(rows.locator("th")).toHaveText(["alice", "bob"]);
  await table.getByRole("button", { name: /Sort by TITLE/ }).click();
  await expect(table.locator('thead th[aria-sort="descending"]')).toHaveText("TITLE");
  await expect(rows.locator("th")).toHaveText(["bob", "alice"]);
  const background = await table.locator('thead th[aria-sort="descending"]').evaluate((node) => getComputedStyle(node).backgroundColor);
  const controlRoomBackground = await page.evaluate(() => {
    const probe = document.createElement("th");
    probe.setAttribute("aria-sort", "ascending");
    const row = document.createElement("tr");
    const table = document.createElement("table");
    row.append(probe); table.append(row); document.body.append(table);
    const value = getComputedStyle(probe).backgroundColor;
    table.remove();
    return value;
  });
  expect(background).toBe(controlRoomBackground);
  await rows.filter({ hasText: "alice" }).locator("td").first().click();
  await expect(table.getByRole("button", { name: "Hide attributes for alice" })).toHaveAttribute("aria-expanded", "true");
  await expect(table.locator(".trusted-source-detail-row")).toContainText("Milwaukee, Wisconsin");
  await expect(browser.locator("[data-source-count]")).toHaveText("2 people");
  await table.getByRole("button", { name: "Hide attributes for alice" }).press("Enter");
  await expect(table.locator(".trusted-source-detail-row")).toHaveCount(0);
  await expect(table.getByRole("button", { name: "Show attributes for alice" })).toBeFocused();
  await browser.getByRole("tab", { name: "Groups" }).click();
  await expect(table.locator("thead th")).toHaveText(["GROUP", "CN", "OBJECTCLASS", "MEMBERS", "ATTRIBUTES"]);
  const operators = table.locator("tbody tr").filter({ hasText: "Directory operators" });
  await expect(operators.locator(".trusted-source-cn")).toHaveText("Operators");
  await expect(operators.locator(".trusted-source-objectclass")).toHaveText("groupOfNames, top");
  await expect(operators.locator(".trusted-source-attributes")).not.toContainText("objectclass");
  await expect(table.locator("tbody th")).toHaveText(["Admins", "Operators"]);
  await table.getByRole("button", { name: /Sort by MEMBERS/ }).click();
  await table.getByRole("button", { name: /Sort by MEMBERS/ }).click();
  await expect(table.locator("tbody th")).toHaveText(["Operators", "Admins"]);
  await browser.getByRole("tab", { name: "Attributes" }).click();
  await table.getByRole("button", { name: /Sort by PEOPLE/ }).click();
  await table.getByRole("button", { name: /Sort by PEOPLE/ }).click();
  await expect(table.locator("tbody th").last()).toHaveText("description");
});

test("Simulator trusted source page uses the same read-only browser", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/trusted-source.html");
  const browser = page.locator("trusted-source-browser");
  await expect(page.getByRole("link", { name: "Trusted Sources", exact: true })).toHaveClass(/active/);
  await expect(page.locator(".simulator-operator-login")).toBeVisible();
  await expect(browser.getByRole("tab", { name: "LDAP graph" })).toHaveAttribute("aria-selected", "true");
  const graph = browser.locator("ldap-org-graph");
  await expect(graph.locator('.ldap-graph-node.person[aria-label="person: alice"]')).toBeVisible();
  await expect(graph.locator(".ldap-graph-node.group")).toContainText("Operators");
  await expect(browser.locator("[data-source-title]")).toHaveText("Organization LDAP");
  await browser.getByRole("tab", { name: "People" }).click();
  await expect(browser.locator(".trusted-source-table")).toContainText("alice@example.test");
  await browser.getByRole("tab", { name: "Groups" }).click();
  await expect(browser.locator(".trusted-source-table")).toContainText("Operators");
  await expect(browser.getByRole("button", { name: /Save|Edit|Delete|Publish/ })).toHaveCount(0);
  expect(api.counts.get("/api/simulator/trusted-source")).toBeGreaterThan(0);
});

test.describe("Simulator operator login UI", () => {
  test.use({ ignoreHTTPSErrors: true });

  test("shows scopes and signs out with CSRF", async ({ page, secureAppURL: appURL }) => {
    let signedIn = true;
    let logoutCSRF = "";
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill(signedIn ? { json: {
      identity: { issuer: "https://identity.example", subject: "simulator-operator", display_name: "Simulator Operator", email: "simulator@example.test", organizations: ["*"], permissions: ["simulator.read", "scenario.run"] },
      csrf: "simulator-csrf-proof",
    } } : { status: 401 }));
    await page.route("**/auth/operator/logout", (route) => {
      logoutCSRF = route.request().headers()["x-zpr-csrf"] || "";
      signedIn = false;
      return route.fulfill({ status: 204 });
    });
    await page.goto(appURL + "/scenarios.html");
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Simulator Operator");
    await expect(page.locator("#operator-scope")).toContainText("Configured organizations: *");
    await expect(page.locator("#operator-scope")).toContainText("scenario.run");
    await expect(page.locator("#operator-scope")).toContainText("simulator@example.test");
    await expect(page.locator("#operator-scope")).not.toContainText("Enrollment actions");
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
    expect(logoutCSRF).toBe("simulator-csrf-proof");
    await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
    await expect(page.locator("body")).toHaveClass(/operator-auth-gated/);
    await expect(page.locator(".app-shell")).toHaveCSS("visibility", "hidden");
    await expect(page.locator(".simulator-operator-login")).toHaveAttribute("role", "dialog");
    await expect(page.locator(".simulator-operator-login")).toHaveAttribute("aria-modal", "true");
    await expect(page.locator("#operator-login-title")).toBeVisible();
  });
});

test("policy picker right-click menu targets records and categories without visible action rows", async ({ page, appURL, api }, testInfo) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  const category = page.locator('[data-category-id="test"]');
  const menu = page.getByRole("menu", { name: /./ });
  await expect(category).toBeVisible();
  await expect(page.locator("#policy-picker-actions")).toHaveCount(0);
  await expect(page.locator("#policy-catalog-pane .catalog-pane-heading button")).toHaveCount(1);
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

test("Policy and Assertion Browse menus rename saved records without changing their revisions", async ({ page, appURL, api }) => {
  const assertion = {
    id: "rename-assertions", category_id: "legacy-assertions", name: "Custom assertions",
    kind: "assertions", content_type: "text/vnd.zpr.assertions", current_revision: 3,
    content: 'group "Operators" members >= 2;', content_hash: "assertion-hash",
  };
  api.policy.records.push(assertion);
  api.handlers.set(`/api/policy/records/${assertion.id}`, async (route) => route.fulfill({ json: assertion }));
  api.handlers.set(`/api/policy/records/${assertion.id}/revisions`, async (route) => route.fulfill({ json: [] }));
  for (const record of [api.policy.records[0], assertion]) {
    api.handlers.set(`/api/policy/records/${record.id}/rename`, async (route) => {
      const request = route.request().postDataJSON();
      expect(request.expected_revision).toBe(record.current_revision);
      record.name = request.name;
      await route.fulfill({ json: record });
    });
  }

  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click({ button: "right" });
  await expect(page.locator("#policy-rename")).toBeEnabled();
  page.once("dialog", (dialog) => dialog.accept("Renamed policy"));
  await page.locator("#policy-rename").click();
  await expect(page.locator('[data-record-id="test-policy"] strong')).toHaveText("Renamed policy");
  await expect(page.locator("#policy-file-status")).toHaveText("Renamed to Renamed policy");
  expect(api.policy.records[0].current_revision).toBe(1);

  const assertionCategory = page.locator('[data-category-id="test"]');
  if (await assertionCategory.getAttribute("aria-expanded") === "false") await assertionCategory.click();
  await page.locator('[data-record-id="rename-assertions"]').click({ button: "right" });
  await expect(page.locator("#policy-rename")).toBeEnabled();
  page.once("dialog", (dialog) => dialog.accept("Renamed assertions"));
  await page.locator("#policy-rename").click();
  await expect(page.locator('[data-record-id="rename-assertions"] strong')).toHaveText("Renamed assertions");
  await expect(page.locator("#policy-file-status")).toHaveText("Renamed to Renamed assertions");
  expect(assertion.current_revision).toBe(3);
});

test("ZPR Config Browse right-click menu renames saved configurations", async ({ page, appURL, api }) => {
  const config = {
    id: "rename-config", category_id: "config-category", name: "Base config",
    kind: "configuration", content_type: "text/vnd.zpr.zplc", current_revision: 2,
    content: "[control]\n", content_hash: "config-hash",
  };
  api.policy.categories.push({ id: "config-category", name: "ZPR Config", path: "ZPR Config" });
  api.policy.records.push(config);
  api.handlers.set(`/api/policy/records/${config.id}`, async (route) => route.fulfill({ json: config }));
  api.handlers.set(`/api/policy/records/${config.id}/revisions`, async (route) => route.fulfill({ json: [] }));
  api.handlers.set(`/api/policy/records/${config.id}/rename`, async (route) => {
    const request = route.request().postDataJSON();
    expect(request.expected_revision).toBe(2);
    config.name = request.name;
    await route.fulfill({ json: config });
  });

  await page.goto(appURL + "/#zpr-config");
  const browse = page.locator("#zpr-config-picker-toggle");
  if (await browse.getAttribute("aria-expanded") !== "true") await browse.click();
  const item = page.locator('[data-record-id="rename-config"]');
  await item.click({ button: "right" });
  await expect(page.locator("#zpr-config-record-menu")).toBeVisible();
  page.once("dialog", (dialog) => dialog.accept("Renamed config"));
  await page.locator("#zpr-config-rename").click();
  await expect(page.locator('[data-record-id="rename-config"] strong')).toHaveText("Renamed config");
  await expect(page.locator("#zpr-config-status")).toHaveText("Renamed to Renamed config");
  expect(config.current_revision).toBe(2);
});

test("policy picker actions support keyboard opening, navigation and dismissal", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  const record = page.locator('[data-record-id="test-policy"]');
  await record.focus();
  await record.press("Shift+F10");
  const menu = page.locator("#policy-picker-menu");
  await expect(menu).toBeVisible();
  await expect(page.locator("#policy-rename")).toBeFocused();
  await page.keyboard.press("ArrowDown");
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
  { app: "Simulator", path: "/organizations.html", label: "Organizations", next: "Workers", nextPath: "/machine-logs.html" },
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

test("page Help is keyboard accessible, contextual, and reachable in the condensed mobile menu", async ({ page, appURL, api }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(appURL + "/#policy");
  await page.getByRole("button", { name: "Condense side menu", exact: true }).click();
  const helpButton = page.getByRole("button", { name: "Help for this page" });
  await expect(helpButton).toBeVisible();
  await expect(helpButton).toHaveAttribute("aria-haspopup", "dialog");
  await helpButton.click();
  const dialog = page.getByRole("dialog", { name: "Policy editor" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Stage creates a review candidate. It does not deploy or activate that candidate.");
  await expect(dialog.getByRole("link", { name: "Policy authoring guide" })).toHaveAttribute("target", "_blank");
  const bounds = await dialog.boundingBox();
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(helpButton).toBeFocused();

  await openAssertionRecord(page, appURL);
  await page.getByRole("button", { name: "Help for this page" }).click();
  const assertionHelp = page.getByRole("dialog", { name: "Assertion editor" });
  await expect(assertionHelp).toBeVisible();
  await expect(assertionHelp).toContainText("Analyze evaluates the exact unsaved assertion source and shows checks/results.");
  await assertionHelp.getByRole("button", { name: "Close", exact: true }).click();
});

test("Simulator Help warns before organization activation and stays usable on mobile", async ({ page, appURL }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(appURL + "/organizations.html");
  await page.getByRole("button", { name: "Condense side menu", exact: true }).click();
  const helpButton = page.getByRole("button", { name: "Help for this page" });
  await expect(helpButton).toBeVisible();
  await helpButton.click();
  const dialog = page.getByRole("dialog", { name: "Organizations" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Switching resets the simulated runtime and can interrupt connections and workloads");
  await expect(dialog).toContainText("finish/cancel scenarios and log out users first");
  await expect(dialog.getByRole("link", { name: "Organization runtime profiles" })).toHaveAttribute("href", /organizations/);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(helpButton).toBeFocused();
});

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
  await expect(page.locator("#policy-editor-mode")).toHaveCount(0);
  await expect(page.locator("#policy-modified-indicator")).toBeHidden();
  await expect(page.locator("#page-policy .policy-toolbar")).toHaveCSS("border-top-style", "none");
  await expect(page.locator("#page-policy .policy-page")).toHaveCSS("border-top-style", "none");
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
      const toolbar = document.querySelector("#page-policy .policy-attribute-toolbar").getBoundingClientRect();
      const editor = document.querySelector("#policy-code-editor").getBoundingClientRect();
      const tools = document.querySelector("#page-policy .policy-editor-tools").getBoundingClientRect();
      const utilities = document.querySelector("#policy-editor-utilities").getBoundingClientRect();
      const rescanRect = document.querySelector("#policy-attribute-rescan").getBoundingClientRect();
      const historyRect = document.querySelector("#policy-history-menu summary").getBoundingClientRect();
      const analyzeRect = document.querySelector("#policy-check").getBoundingClientRect();
      const formatRect = document.querySelector("#policy-format").getBoundingClientRect();
      const pane = document.querySelector("#policy-editor-pane").getBoundingClientRect();
      const toolsStyle = getComputedStyle(document.querySelector("#page-policy .policy-editor-tools"));
      const buttons = [...document.querySelectorAll("#page-policy .policy-attribute-toolbar .button")].map((button) => button.getBoundingClientRect());
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
        paneRight: pane.right,
        paneLeft: pane.left,
        paneWidth: pane.width,
        rescanTop: rescanRect.top,
        historyTop: historyRect.top,
        analyzeTop: analyzeRect.top,
        formatTop: formatRect.top,
        analyzeRight: analyzeRect.right,
        formatLeft: formatRect.left,
        order: ["#policy-picker-toggle", "#policy-files-toggle", "#policy-check", "#policy-format", "#policy-attribute-rescan"].map((selector) => [...document.querySelectorAll("#policy-editor-pane *")].indexOf(document.querySelector(selector))),
        overlaps: buttons.some((first, index) => buttons.slice(index + 1).some((second) => first.left < second.right && first.right > second.left && first.top < second.bottom && first.bottom > second.top)),
      };
    });
    expect(layout.toolbarBottom).toBeLessThanOrEqual(layout.editorTop);
    expect(Math.abs(layout.utilitiesRight - layout.toolsRight), JSON.stringify(layout)).toBeLessThanOrEqual(1);
    expect(layout.historyTop).toBeLessThan(layout.rescanTop);
    expect(layout.historyTop).toBeLessThan(layout.editorTop);
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
  await expect(assistant.locator(".assistant-settings")).toBeHidden();
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
  await viewer.getByLabel("Wrap", { exact: true }).uncheck();
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
  await page.locator("#page-policy").getByRole("checkbox", { name: "Wrap", exact: true }).uncheck();
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
  await expect(page.locator("#policy-catalog-pane")).toBeHidden();
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
  await expect(page.locator("#policy-catalog-pane")).toBeHidden();
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
      const firstPicker = first.locator("select");
      await expect(firstPicker.locator("option")).toHaveCount(4);
      await selectAdapterLogSource(first, "finance-client adapter · machine-first");
      await expect(first.locator(".machine-log-output")).toContainText("finance-client adapter entry 79");
      await page.getByRole("button", { name: "Add adapter panel" }).click();
      await expect(columns).toHaveCount(2);
      const second = columns.nth(1);
      await expect(second.locator("select")).toHaveValue("machine-first\u001fControl adapter");
      await selectAdapterLogSource(first, "Control adapter · machine-second");
      await expect(first.locator(".machine-log-output")).toContainText("Control adapter entry 79");
      await expect(second.locator(".machine-log-output")).toContainText("Control adapter entry 79");
      await expect(second.locator("select")).toHaveValue("machine-first\u001fControl adapter");
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
  await selectAdapterLogSource(columns.nth(0), "finance-client adapter · machine-first");
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
  expect(Math.max(...toolbarTops) - Math.min(...toolbarTops)).toBeLessThanOrEqual(44);
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
  await selectAdapterLogSource(first, "finance-client adapter · machine-first");
  const output = first.locator(".machine-log-output");
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  await page.getByRole("button", { name: "Add adapter panel", exact: true }).click();
  await expect(columns).toHaveCount(2);
  const second = columns.nth(1);
  const originalPanel = await first.elementHandle();
  await page.getByRole("button", { name: "Controller logs", exact: true }).click();
  await expect(page.getByRole("button", { name: "Controller logs", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(first.locator("select option")).toHaveCount(2);
  await selectAdapterLogSource(first, "Controller · machine-second", "controller");
  await expect(first.locator(".machine-log-output")).toContainText("Controller entry 79");
  await expect(second.locator("select")).toHaveValue("machine-first\u001fController");
  await expect(columns).toHaveCount(2);
  expect(await originalPanel.evaluate((panel) => panel.isConnected)).toBeTruthy();
  await output.evaluate((element) => { element.scrollTop = 0; element.dispatchEvent(new Event("scroll")); });
  await page.getByRole("button", { name: "Adapter logs", exact: true }).click();
  await expect(first.locator("select")).toHaveValue("machine-first\u001ffinance-client adapter");
  await expect(first.locator(".machine-log-output")).toContainText("finance-client adapter entry 79");
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  await page.getByRole("button", { name: "Controller logs", exact: true }).click();
  await expect(first.locator("select")).toHaveValue("machine-second\u001fController");
  expect(await output.evaluate((element) => element.scrollTop)).toBe(0);
  expect(api.counts.get("/api/adapter-logs")).toBe(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath("control-room-log-types.png"), fullPage: true });
});

async function refreshLogs(page) {
  const button = page.locator("#machine-logs-refresh");
  if (await button.isVisible()) {
    await button.click();
    await expect(button).toBeEnabled();
    return;
  }
  const pause = page.locator("#machine-logs-pause");
  const wasPaused = await pause.getAttribute("aria-pressed") === "true";
  if (!wasPaused) await pause.click();
  const endpoint = new URL(page.url()).pathname === "/machine-logs.html" ? "/api/simulator/machine-logs" : "/api/adapter-logs";
  const response = page.waitForResponse((item) => item.url().includes(endpoint));
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
  await selectAdapterLogSource(first, "finance-client adapter · machine-first");
  await expectAtBottom(output);
  await selectAdapterLogSource(first, "Control adapter · machine-first");
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

test("non-JSON worker responses report service unavailability instead of parser errors", async ({ page, appURL }) => {
  await page.route("**/api/simulator/machine-logs", (route) => route.fulfill({
    status: 502, contentType: "text/plain", body: "Client sent HTTP request to an HTTPS server.",
  }));
  await page.goto(appURL + "/machine-logs.html");
  await expect(page.locator("#machine-logs-error")).toHaveText("Visa Service or Control-Service is unavailable (HTTP 502).");
  await expect(page.locator("#machine-logs-error")).not.toContainText("Unexpected token");
});

test("workload logs remain visible when first opened after a machine exits", async ({ page, appURL, api }) => {
  api.workloadLogs.machines[0].state = "exited";
  await page.goto(appURL + "/machine-logs.html");
  const panel = page.locator(".machine-log-panel").first();
  const output = panel.locator(".machine-log-output");
  await expect(output).toContainText("finance-client events entry 79");
  await expect(panel).toContainText("disconnected");
  await expect(panel).not.toHaveClass(/running/);
  await expect(panel).toHaveCSS("background-color", "rgb(255, 255, 255)");
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
  await page.evaluate(() => { location.hash = "#services"; });
  await expect(page.locator("#page-services")).toBeVisible();
  await expect(page.locator("#page-adapter-logs")).toBeHidden();
  const before = api.counts.get("/api/adapter-logs");
  await page.clock.runFor(6000);
  expect(api.counts.get("/api/adapter-logs")).toBe(before);
  await page.evaluate(() => { location.hash = "#adapter-logs"; });
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

for (const outcome of ["success", "error"]) {
  for (const edit of ["replacement", "Tab"]) {
    test(`Policy compiler scope rejects delayed ${outcome} after ${edit} and restored source`, async ({ page, appURL, api }) => {
      const original = "define Employee as user.\n";
      let complete;
      let finished = false;
      api.handlers.set("/api/policy/check", async (route) => {
        expect(route.request().postDataJSON().source).toBe(original);
        await new Promise((resolve) => { complete = resolve; });
        await route.fulfill(outcome === "success"
          ? { json: { valid: true, diagnostics: "Obsolete compiler result", warnings: [{ line: 1, code: "OBSOLETE", message: "Obsolete compiler warning" }] } }
          : { status: 503, json: { error: "Obsolete compiler failure" } });
        finished = true;
      });
      await page.goto(appURL + "/#policy");
      await openPolicyPicker(page);
      await page.locator('[data-record-id="test-policy"]').click();
      const source = page.locator("#policy-source");
      await page.locator("#policy-check").click();
      await expect.poll(() => Boolean(complete)).toBe(true);
      if (edit === "Tab") await source.press("Tab");
      else await source.fill(original + "# changed\n");
      await source.fill(original);
      complete();
      await expect.poll(() => finished).toBe(true);
      await expect(source).toHaveValue(original);
      await expect(page.locator("#policy-check")).not.toHaveAttribute("data-analysis-state", /.+/);
      await expect(page.locator('#policy-test-gutter [data-has-warnings="true"], #policy-highlight .zpl-error')).toHaveCount(0);
      await expect(page.locator("#policy-test-status")).toBeHidden();
      expect(api.counts.get("/api/policy/test/fixtures") || 0).toBe(0);
      api.handlers.set("/api/policy/check", (route) => route.fulfill({ json: { valid: false, diagnostics: "error: unexpected token at line 1, column 1" } }));
      await page.locator("#policy-check").click();
      await expect(page.locator("#policy-highlight .zpl-error")).toHaveText("define");
      await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
      expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
    });
  }
  test(`Policy compiler cancellation rejects old ${outcome} and preserves a newer same-source record check`, async ({ page, appURL, api }) => {
    const record = { ...api.policy.records.find((item) => item.id === "test-policy"), id: "another-policy", name: "Another policy" };
    api.policy.records.push(record);
    api.handlers.set("/api/policy/records/another-policy", (route) => route.fulfill({ json: record }));
    api.handlers.set("/api/policy/records/another-policy/revisions", (route) => route.fulfill({ json: [] }));
    const completions = [];
    let oldFinished = false;
    api.handlers.set("/api/policy/check", async (route) => {
      const index = completions.length;
      await new Promise((resolve) => completions.push(resolve));
      await route.fulfill(index === 0
        ? outcome === "success"
          ? { json: { valid: true, diagnostics: "Obsolete compiler result", warnings: [{ line: 1, code: "OBSOLETE", message: "Obsolete compiler warning" }] } }
          : { status: 503, json: { error: "Obsolete compiler failure" } }
        : { json: { valid: false, diagnostics: "error: unexpected token at line 1, column 1" } });
      if (index === 0) oldFinished = true;
    });
    await page.goto(appURL + "/#policy");
    await openPolicyPicker(page);
    await page.locator('[data-record-id="test-policy"]').click();
    await page.locator("#policy-check").click();
    await expect.poll(() => completions.length).toBe(1);
    await openPolicyPicker(page);
    await page.locator('[data-record-id="another-policy"]').click();
    await expect(page.locator("#policy-record-title")).toHaveText(record.name);
    await expect(page.locator("#policy-check")).toBeEnabled();
    await page.locator("#policy-check").click();
    await expect.poll(() => completions.length).toBe(2);
    completions[0]();
    await expect.poll(() => oldFinished).toBe(true);
    await expect(page.locator("#policy-check")).toBeDisabled();
    await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "pending");
    await expect(page.locator('#policy-test-gutter [data-has-warnings="true"], #policy-highlight .zpl-error')).toHaveCount(0);
    expect(api.counts.get("/api/policy/test/fixtures") || 0).toBe(0);
    completions[1]();
    await expect(page.locator("#policy-highlight .zpl-error")).toHaveText("define");
    await expect(page.locator("#policy-check")).toBeEnabled();
  });
  for (const action of ["Save", "Save As"]) {
    test(`Policy compiler cancellation stops ${action} after a delayed ${outcome} for edited-then-restored source`, async ({ page, appURL, api }) => {
      let complete;
      let finished = false;
      api.handlers.set("/api/policy/check", async (route) => {
        await new Promise((resolve) => { complete = resolve; });
        await route.fulfill(outcome === "success"
          ? { json: { valid: true, diagnostics: "Obsolete compiler result" } }
          : { status: 503, json: { error: "Obsolete compiler failure" } });
        finished = true;
      });
      await page.goto(appURL + "/#policy");
      await openPolicyPicker(page);
      await page.locator('[data-record-id="test-policy"]').click();
      const source = page.locator("#policy-source");
      const draft = "define Employee as user.\n# unsaved\n";
      await source.fill(draft);
      await openPolicyFiles(page);
      if (action === "Save") await page.locator("#policy-save").click();
      else {
        await page.locator("#policy-save-as").click();
        await page.locator("#record-submit").click();
      }
      await expect.poll(() => Boolean(complete)).toBe(true);
      if (action === "Save As") await page.locator("#record-dialog").getByRole("button", { name: "Cancel", exact: true }).click();
      await source.fill(draft + "# changed\n");
      await source.fill(draft);
      complete();
      await expect.poll(() => finished).toBe(true);
      await expect(source).toHaveValue(draft);
      await expect(page.locator("#version-dialog")).toBeHidden();
      expect(api.counts.get("/api/policy/test/fixtures") || 0).toBe(0);
      expect(api.counts.get("/api/policy/test") || 0).toBe(0);
      expect(api.counts.get("/api/policy/records") || 0).toBe(0);
    });
  }
  for (const context of ["revision", "organization"]) {
    test(`Policy compiler scope rejects delayed ${outcome} after a same-source ${context} change`, async ({ page, appURL, api }) => {
      const original = api.policy.records.find((record) => record.id === "test-policy").content;
      api.policy.organization_id = "first-organization";
      api.handlers.set("/api/policy/records/test-policy/revisions", (route) => route.fulfill({ json: [
        { number: 1, summary: "Original", author: "tester", created_at: "2026-10-01T12:00:00Z" },
      ] }));
      api.handlers.set("/api/policy/records/test-policy/revisions/1", (route) => route.fulfill({ json: {
        number: 1, content: original, content_hash: "old", summary: "Original",
      } }));
      let complete;
      let finished = false;
      api.handlers.set("/api/policy/check", async (route) => {
        await new Promise((resolve) => { complete = resolve; });
        await route.fulfill(outcome === "success"
          ? { json: { valid: true, diagnostics: "Obsolete compiler result", warnings: [{ line: 1, code: "OBSOLETE", message: "Obsolete compiler warning" }] } }
          : { status: 503, json: { error: "Obsolete compiler failure" } });
        finished = true;
      });
      await page.goto(appURL + "/#policy");
      await openPolicyPicker(page);
      await page.locator('[data-record-id="test-policy"]').click();
      await page.locator("#policy-check").click();
      await expect.poll(() => Boolean(complete)).toBe(true);
      if (context === "revision") {
        await page.locator("#policy-history-menu summary").click();
        await page.locator('#policy-history-menu [data-revision="1"]').click();
        await expect(page.locator("#policy-source")).toBeDisabled();
      } else {
        api.policy.organization_id = "second-organization";
        await page.evaluate(() => refreshPolicyCatalog());
      }
      complete();
      await expect.poll(() => finished).toBe(true);
      await expect(page.locator("#policy-source")).toHaveValue(original);
      await expect(page.locator('#policy-test-gutter [data-has-warnings="true"], #policy-highlight .zpl-error')).toHaveCount(0);
      await expect(page.locator("#policy-test-status")).toBeHidden();
      expect(api.counts.get("/api/policy/test/fixtures") || 0).toBe(0);
    });
  }
}

for (const phase of ["fixtures", "evaluation"]) {
  for (const outcome of ["success", "error"]) {
    for (const change of ["source", "record", "revision", "organization"]) {
      test(`Policy runtime scope rejects delayed ${phase} ${outcome} after ${change} changes`, async ({ page, appURL, api }) => {
        const original = api.policy.records.find((record) => record.id === "test-policy").content;
        const next = { ...api.policy.records[0], id: "runtime-next-policy", name: "Runtime next policy", content: original };
        api.policy.records.push(next);
        api.policy.organization_id = "first-organization";
        api.handlers.set("/api/policy/records/runtime-next-policy", (route) => route.fulfill({ json: next }));
        api.handlers.set("/api/policy/records/runtime-next-policy/revisions", (route) => route.fulfill({ json: [] }));
        api.handlers.set("/api/policy/records/test-policy/revisions", (route) => route.fulfill({ json: [
          { number: 1, summary: "Original", author: "tester", created_at: "2026-10-01T12:00:00Z" },
        ] }));
        api.handlers.set("/api/policy/records/test-policy/revisions/1", (route) => route.fulfill({ json: {
          number: 1, content: original, content_hash: "old", summary: "Original",
        } }));
        api.handlers.set("/api/policy/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully.", warnings: [] } }));
        const fixtures = { actors: [], services: [] };
        const result = { api_version: 1, actor_count: 0, services: [], warnings: [{ line: 1, code: "OBSOLETE", message: "Obsolete runtime warning" }] };
        const endpoint = phase === "fixtures" ? "/api/policy/test/fixtures" : "/api/policy/test";
        api.handlers.set("/api/policy/test/fixtures", (route) => route.fulfill({ json: fixtures }));
        api.handlers.set("/api/policy/test", (route) => route.fulfill({ json: result }));
        const completions = [];
        let oldFinished = false;
        api.handlers.set(endpoint, async (route) => {
          const index = completions.length;
          await new Promise((resolve) => completions.push(resolve));
          await route.fulfill(index === 0 && outcome === "error"
            ? { status: 503, json: { error: "Obsolete runtime failure on line 1" } }
            : { json: phase === "fixtures" ? fixtures : { ...result, warnings: index === 0 ? result.warnings : [] } });
          if (index === 0) oldFinished = true;
        });
        await page.goto(appURL + "/#policy");
        await openPolicyPicker(page);
        await page.locator('[data-record-id="test-policy"]').click();
        await page.locator("#policy-check").click();
        await expect.poll(() => completions.length).toBe(1);
        const source = page.locator("#policy-source");
        if (change === "source") {
          await source.press("Tab");
          await source.fill(original);
        } else if (change === "record") {
          await openPolicyPicker(page);
          await page.locator('[data-record-id="runtime-next-policy"]').click();
          await expect(page.locator("#policy-record-title")).toHaveText(next.name);
        } else if (change === "revision") {
          await page.locator("#policy-history-menu summary").click();
          await page.locator('#policy-history-menu [data-revision="1"]').click();
          await expect(source).toBeDisabled();
        } else {
          api.policy.organization_id = "second-organization";
          await page.evaluate(() => refreshPolicyCatalog());
        }
        if (change === "record") {
          await page.locator("#policy-check").click();
          await expect.poll(() => completions.length).toBe(2);
        }
        completions[0]();
        await expect.poll(() => oldFinished).toBe(true);
        await expect(source).toHaveValue(original);
        await expect(page.locator('#policy-test-gutter [data-has-warnings="true"], #policy-test-gutter [data-effect="error"], #policy-highlight .zpl-error')).toHaveCount(0);
        await expect(page.locator("#policy-test-status")).toBeHidden();
        if (phase === "fixtures") expect(api.counts.get("/api/policy/test") || 0).toBe(0);
        if (change === "record") {
          await expect(page.locator("#policy-check")).toBeDisabled();
          await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "pending");
          completions[1]();
          await expect(page.locator("#policy-check")).toBeEnabled();
          await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "success");
        } else await expect(page.locator("#policy-check")).not.toHaveAttribute("data-analysis-state", "error");
        expect([...api.counts.keys()].some((path) => path.startsWith("/api/simulator"))).toBe(false);
      });
    }
    for (const action of ["Save", "Save As"]) {
      test(`Policy runtime cancellation stops ${action} after delayed ${phase} ${outcome} on another record`, async ({ page, appURL, api }) => {
        const next = { ...api.policy.records[0], id: "runtime-save-next", name: "Runtime save next" };
        api.policy.records.push(next);
        api.handlers.set("/api/policy/records/runtime-save-next", (route) => route.fulfill({ json: next }));
        api.handlers.set("/api/policy/records/runtime-save-next/revisions", (route) => route.fulfill({ json: [] }));
        api.handlers.set("/api/policy/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } }));
        const fixtures = { actors: [], services: [] };
        const result = { api_version: 1, actor_count: 0, services: [] };
        api.handlers.set("/api/policy/test/fixtures", (route) => route.fulfill({ json: fixtures }));
        api.handlers.set("/api/policy/test", (route) => route.fulfill({ json: result }));
        let complete;
        let finished = false;
        api.handlers.set(phase === "fixtures" ? "/api/policy/test/fixtures" : "/api/policy/test", async (route) => {
          await new Promise((resolve) => { complete = resolve; });
          await route.fulfill(outcome === "error"
            ? { status: 503, json: { error: "Obsolete pre-save failure" } }
            : { json: phase === "fixtures" ? fixtures : result });
          finished = true;
        });
        await page.goto(appURL + "/#policy");
        await openPolicyPicker(page);
        await page.locator('[data-record-id="test-policy"]').click();
        await page.locator("#policy-source").fill("define Employee as user.\n# unsaved\n");
        await openPolicyFiles(page);
        if (action === "Save") await page.locator("#policy-save").click();
        else {
          await page.locator("#policy-save-as").click();
          await page.locator("#record-submit").click();
        }
        await expect.poll(() => Boolean(complete)).toBe(true);
        await expect(page.locator("#policy-source")).toHaveJSProperty("readOnly", true);
        if (action === "Save As") await page.locator("#record-dialog").getByRole("button", { name: "Cancel", exact: true }).click();
        page.once("dialog", (dialog) => dialog.accept());
        await openPolicyPicker(page);
        await page.locator('[data-record-id="runtime-save-next"]').click();
        await expect(page.locator("#policy-record-title")).toHaveText(next.name);
        await expect(page.locator("#policy-source")).toBeEditable();
        complete();
        await expect.poll(() => finished).toBe(true);
        await expect(page.locator("#version-dialog")).toBeHidden();
        await expect(page.locator("#record-warning")).toBeHidden();
        expect(api.counts.get("/api/policy/records") || 0).toBe(0);
        const cached = await page.evaluate(() => ({
          source: state.policy.saveTestSource, error: state.policy.saveTestError,
          copySource: state.policy.saveAsTestSource, copyError: state.policy.saveAsTestError,
          pending: state.policy.saveTestPending,
        }));
        expect(cached).toEqual({ source: "", error: "", copySource: "", copyError: "", pending: false });
      });
    }
  }
}

for (const outcome of ["success", "error"]) {
  test(`Policy runtime scope rejects ${outcome} when revision changes without an abort event`, async ({ page, appURL, api }) => {
    api.handlers.set("/api/policy/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } }));
    api.handlers.set("/api/policy/test/fixtures", (route) => route.fulfill({ json: { actors: [], services: [] } }));
    let complete;
    let finished = false;
    api.handlers.set("/api/policy/test", async (route) => {
      await new Promise((resolve) => { complete = resolve; });
      await route.fulfill(outcome === "success"
        ? { json: { api_version: 1, actor_count: 0, services: [], warnings: [{ line: 1, code: "OBSOLETE", message: "Obsolete warning" }] } }
        : { status: 503, json: { error: "Obsolete runtime failure on line 1" } });
      finished = true;
    });
    await page.goto(appURL + "/#policy");
    await page.locator("#policy-check").click();
    await expect.poll(() => Boolean(complete)).toBe(true);
    expect(await page.evaluate(() => {
      state.policy.record.current_revision++;
      return state.policy.testAbort.signal.aborted;
    })).toBe(false);
    complete();
    await expect.poll(() => finished).toBe(true);
    await expect(page.locator("#policy-check")).toBeEnabled();
    await expect(page.locator('#policy-test-gutter [data-has-warnings="true"], #policy-test-gutter [data-effect="error"]')).toHaveCount(0);
    await expect(page.locator("#policy-test-status")).toBeHidden();
    expect(await page.evaluate(() => state.policy.testResult)).toBeNull();
  });
}

for (const phase of ["fixtures", "evaluation"]) {
  test(`Policy runtime cancellation cannot release a newer pre-save ${phase} request`, async ({ page, appURL, api }) => {
    const next = { ...api.policy.records[0], id: "new-save-owner", name: "New save owner" };
    api.policy.records.push(next);
    api.handlers.set("/api/policy/records/new-save-owner", (route) => route.fulfill({ json: next }));
    api.handlers.set("/api/policy/records/new-save-owner/revisions", (route) => route.fulfill({ json: [] }));
    api.handlers.set("/api/policy/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } }));
    const fixtures = { actors: [], services: [] };
    const result = { api_version: 1, actor_count: 0, services: [] };
    api.handlers.set("/api/policy/test/fixtures", (route) => route.fulfill({ json: fixtures }));
    api.handlers.set("/api/policy/test", (route) => route.fulfill({ json: result }));
    const completions = [];
    let oldFinished = false;
    api.handlers.set(phase === "fixtures" ? "/api/policy/test/fixtures" : "/api/policy/test", async (route) => {
      const index = completions.length;
      await new Promise((resolve) => completions.push(resolve));
      await route.fulfill({ json: phase === "fixtures" ? fixtures : result });
      if (index === 0) oldFinished = true;
    });
    await page.goto(appURL + "/#policy");
    await page.locator("#policy-source").fill("define Employee as user.\n# old draft\n");
    await openPolicyFiles(page);
    await page.locator("#policy-save").click();
    await expect.poll(() => completions.length).toBe(1);
    page.once("dialog", (dialog) => dialog.accept());
    await openPolicyPicker(page);
    await page.locator('[data-record-id="new-save-owner"]').click();
    const draft = "define Employee as user.\n# new draft\n";
    await page.locator("#policy-source").fill(draft);
    await openPolicyFiles(page);
    await page.locator("#policy-save").click();
    await expect.poll(() => completions.length).toBe(2);
    completions[0]();
    await expect.poll(() => oldFinished).toBe(true);
    await expect(page.locator("#policy-source")).toHaveJSProperty("readOnly", true);
    await expect(page.locator("#policy-save")).toBeDisabled();
    await expect(page.locator("#version-dialog")).toBeHidden();
    expect(await page.evaluate(() => state.policy.saveTestPending)).toBe(true);
    completions[1]();
    await expect(page.locator("#version-dialog")).toBeVisible();
    await expect(page.locator("#version-warning")).toBeHidden();
    expect(await page.evaluate(() => state.policy.saveTestSource)).toBe(draft);
    await expect(page.locator("#policy-source")).toHaveValue(draft);
  });

  test(`Policy runtime cancellation during Stage ${phase} does not attach an old staging error`, async ({ page, appURL, api }) => {
    api.policy.staging_ready = true;
    api.handlers.set("/api/policy/check", (route) => route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully." } }));
    api.handlers.set("/api/policy/test/fixtures", (route) => route.fulfill({ json: { actors: [], services: [] } }));
    let complete;
    let finished = false;
    api.handlers.set(phase === "fixtures" ? "/api/policy/test/fixtures" : "/api/policy/test", async (route) => {
      await new Promise((resolve) => { complete = resolve; });
      await route.fulfill({ status: 503, json: { error: "Obsolete Stage analysis failure" } });
      finished = true;
    });
    await page.goto(appURL + "/#policy");
    await openPolicyFiles(page);
    await page.locator("#policy-stage").click();
    await expect.poll(() => Boolean(complete)).toBe(true);
    const original = await page.locator("#policy-source").inputValue();
    await page.locator("#policy-source").fill(original + "# changed\n");
    await page.locator("#policy-source").fill(original);
    complete();
    await expect.poll(() => finished).toBe(true);
    await expect(page.locator("#policy-stage-dialog")).toBeHidden();
    await expect(page.locator("#policy-stage-status")).toBeHidden();
    await expect(page.locator("#policy-test-status")).toBeHidden();
    expect(api.counts.get("/api/policy/records/test-policy/stage") || 0).toBe(0);
  });
}

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
  await expect(page.locator("#policy-test-status")).toBeVisible();
  await expect(page.locator('#policy-test-gutter [data-line="1"] [data-effect="error"]')).toHaveCount(0);
  const warning = page.locator('#policy-test-gutter [data-line="2"] [data-effect="warning"]');
  await expect(warning).toHaveText("WARN");
  await warning.click();
  await expect(page.getByRole("dialog", { name: "Analyze warning details" })).toContainText("POLICY_SPECIFIC_IDENTITY");
  await page.locator("#policy-test-details .dialog-actions .button").click();
  await page.locator("#policy-source").fill("define Operators as user.");
  await expect(page.locator('#policy-test-gutter [data-effect="warning"]')).toHaveCount(0);
});

test("policy Analyze tolerates omitted directory attributes unless the policy references them", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/check", async (route) => route.fulfill({ json: { valid: true, diagnostics: "Compiled successfully.", warnings: [] } }));
  api.handlers.set("/api/policy/test/fixtures", async (route) => route.fulfill({ json: {
    actors: [], services: [], omitted_attributes: ["user.l"],
    warnings: ['Directory attribute "user.l" was omitted from test fixtures.'],
  } }));
  let tests = 0;
  api.handlers.set("/api/policy/test", async (route) => {
    tests += 1;
    await route.fulfill({ json: { matrix: [], rules: [], warnings: [] } });
  });
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.locator("#policy-source").fill("define employee as a user with user.bas_id.\n# user.l is not used here");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "success");
  await expect(page.locator("#policy-test-status")).toBeHidden();
  expect(tests).toBe(1);
  await page.locator("#policy-source").fill("define locals as a user with user.l:Milwaukee.");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator("#policy-test-status")).toBeVisible();
  await expect(page.locator("#policy-test-status")).toContainText('Analysis unavailable: policy references directory attribute "user.l"');
  expect(tests).toBe(1);
  await page.locator("#policy-source").fill("define locals as a user.");
  await expect(page.locator("#policy-test-status")).toBeHidden();
});

test("policy compiler errors without a line stay visible", async ({ page, appURL, api }) => {
  api.handlers.set("/api/policy/check", async (route) => route.fulfill({ json: { valid: false, diagnostics: "no service contract or configuration found for webserver", warnings: [] } }));
  await page.goto(appURL + "/#policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-policy"]').click();
  await page.locator("#policy-source").fill("define webserver as a service with tag:web.");
  await page.locator("#policy-check").click();
  await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator("#policy-test-status")).toBeVisible();
  await expect(page.locator("#policy-test-status")).toContainText("Compiler error: no service contract or configuration found for webserver");
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
  api.snapshot.services = kinds.map((kind, index) => ({ service_name: `service-${index}`, service_kind: kind, actor_cn: "adapter", zpr_addr: "fd00::1", service_endpoints: "tcp:8080", ...(kind === "Gateway" ? { external_network_connection: "partner-network" } : {}) }));
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
  await expect(page.locator(".graph-cloud")).toHaveCSS("fill", "rgb(217, 234, 243)");
  await expect(page.locator(".graph-cloud-label")).toHaveText("partner-network");
  await expect(page.locator(".graph-external-network")).toHaveAttribute("aria-label", "Gateway connection to partner-network");
  await expect(page.locator(".gateway-cloud-link")).toHaveCount(1);
  await expect(page.locator(".gateway-cloud-link")).toHaveCSS("stroke", "rgb(49, 95, 120)");
  await expect(page.locator(".gateway-cloud-link")).toHaveCSS("stroke-width", "3px");
});

test("Gateways edits multiple gateway drafts with the policy editor paradigm without activating them", async ({ page, appURL, api }) => {
  const contracts = [
    { organization_id: "alpha", instance_id: "public-egress", adapter_cn: "gateway-public-egress", service_name: "public-egress.svc.zpr", external_network: "" },
    { organization_id: "alpha", instance_id: "partner-egress", adapter_cn: "gateway-partner-egress", service_name: "partner-egress.svc.zpr", external_network: "partner" },
  ];
  const savedRequests = [];
  api.handlers.set("/api/gateways/contracts", async (route) => route.fulfill({ json: { organization_id: "alpha", contracts } }));
  api.handlers.set("/api/gateways/configs", async (route) => route.fulfill({ json: { organization_id: "alpha", configs: [{
    organization_id: "alpha", instance_id: "partner-egress", current_revision: 2,
    revisions: [
      { revision: 1, saved_at: "2026-10-01T12:00:00Z", config: { schema_version: 1, instance_id: "partner-egress", destinations: [{ origin: "https://old.partner.example", path_prefixes: ["/"] }] } },
      { revision: 2, saved_at: "2026-10-02T12:00:00Z", config: { schema_version: 1, instance_id: "partner-egress", destinations: [{ origin: "https://partner.example", path_prefixes: ["/v2/"] }] } },
    ],
  }] } }));
  api.handlers.set("/api/gateways/config/check", async (route) => {
    const { config } = route.request().postDataJSON();
    expect(config.organization_id).toBe("alpha");
    expect(config.instance_id).toBe("public-egress");
    expect(config.adapter_cn).toBe("gateway-public-egress");
    expect(config.service_name).toBe("public-egress.svc.zpr");
    expect(config.external_network).toBeUndefined();
    if (!config.destinations[0].origin) {
      await route.fulfill({ status: 422, json: { valid: false, diagnostics: "destination origin is required" } });
      return;
    }
    await route.fulfill({ json: { valid: true, diagnostics: "Gateway draft matches the live Gateway service identity; runtime configuration is unchanged.", contract: contracts[0] } });
  });
  api.handlers.set("/api/gateways/configs/public-egress/revisions", async (route) => {
    const body = route.request().postDataJSON();
    savedRequests.push(body);
    await route.fulfill({ json: {
      organization_id: "alpha", instance_id: "public-egress", current_revision: 1,
      revisions: [{ revision: 1, saved_at: new Date().toISOString(), config: body.config }],
    } });
  });

  await page.goto(`${appURL}/#gateways`);
  const source = page.getByRole("textbox", { name: "Gateway draft JSON" });
  await expect(page.locator("#gateway-picker-label")).toHaveText("Browse...");
  await expect(page.locator("#gateway-record-title")).toHaveText("public-egress.svc.zpr");
  await expect(page.locator("#gateway-editor-mode")).toHaveCount(0);
  await expect(source).toHaveValue(/"instance_id": "public-egress"/);
  await expect(page.locator("#gateway-source").locator("xpath=ancestor::div[contains(@class,'config-source-editor')]")).toHaveCSS("background-color", "rgb(23, 33, 30)");
  await expect(page.getByRole("button", { name: "Find & Replace" })).toBeVisible();

  const analyze = page.getByRole("button", { name: "Analyze", exact: true });
  await expect(analyze).toHaveClass(/button-next-evaluate/);
  await analyze.click();
  await expect(analyze).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator("#gateway-draft-message")).toContainText("destination origin is required");

  await source.fill((await source.inputValue()).replace('"origin": ""', '"origin": "https://api.example.com",'));
  await analyze.click();
  await expect(analyze).toHaveAttribute("data-analysis-state", "error");
  await expect(page.locator("#gateway-gutter .policy-test-line-result")).toHaveCount(1);
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
  await page.locator("#gateway-gutter .policy-test-line-result").click();
  await expect(page.locator("#gateway-error-dialog")).toBeVisible();
  await expect(page.locator("#gateway-error-text")).toContainText("Invalid JSON");
  await page.getByRole("button", { name: "Close Gateway Analyze error", exact: true }).click();
  await expect(source).toBeFocused();
  await expect(page.locator("#gateway-draft-message")).toBeHidden();

  await source.fill((await source.inputValue()).replace(",,", ","));
  await expect(analyze).not.toHaveAttribute("data-analysis-state", /.+/);
  await page.getByRole("button", { name: "File..." }).click();
  await expect(page.getByRole("menuitem", { name: "Save", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape");
  await analyze.click();
  await expect(analyze).toHaveAttribute("data-analysis-state", "success");
  await expect(analyze).toHaveCSS("background-color", /rgb\((35, 117, 76|25, 92, 58)\)/);
  await expect(page.locator("#gateway-draft-message")).toContainText("runtime configuration is unchanged");
  await page.getByRole("button", { name: "File..." }).click();
  await page.getByRole("menuitem", { name: "Save", exact: true }).click();
  await expect(page.locator("#gateway-draft-message")).toContainText("Saved draft revision 1");
  await expect(page.locator("#gateway-history-count")).toHaveText("1 version");
  expect(savedRequests).toHaveLength(1);
  expect(savedRequests[0].expected_revision).toBe(0);
  expect(savedRequests[0].config.destinations[0].origin).toBe("https://api.example.com");

  await page.getByRole("button", { name: "Browse gateways" }).click();
  const items = page.locator("#gateway-contracts [role=treeitem]");
  await expect(items).toHaveCount(2);
  await expect(items.nth(0)).toHaveAttribute("aria-selected", "true");
  await items.filter({ hasText: "partner-egress.svc.zpr" }).click();
  await expect(page.locator("#gateway-catalog-pane")).toBeHidden();
  await expect(page.locator("#gateway-record-title")).toHaveText("partner-egress.svc.zpr");
  await expect(page.locator("#gateway-revision-label")).toBeHidden();
  await expect(page.locator("#page-gateways .policy-identity > #gateway-history-menu")).toBeVisible();
  await expect(source).toHaveValue(/https:\/\/partner\.example/);
  await page.locator("#gateway-history-menu > summary").click();
  await page.locator('#gateway-history [data-revision="1"]').click();
  await expect(source).toHaveValue(/old\.partner\.example/);
  await expect(page.getByRole("button", { name: "Activate", exact: true })).toHaveCount(0);
});

test("Gateway form and raw editors share one draft, preserve fields, and save only analyzed revisions", async ({ page, appURL, api }) => {
  const contract = { organization_id: "alpha", instance_id: "egress", adapter_cn: "egress", service_name: "egress.svc.zpr" };
  api.handlers.set("/api/gateways/contracts", route => route.fulfill({ json: { contracts: [contract] } }));
  api.handlers.set("/api/gateways/configs", route => route.fulfill({ json: { configs: [] } }));
  const checks = [];
  const saves = [];
  api.handlers.set("/api/gateways/config/check", route => {
    checks.push(route.request().postDataJSON().config);
    return route.fulfill({ json: { valid: true, diagnostics: "Runtime unchanged." } });
  });
  api.handlers.set("/api/gateways/configs/egress/revisions", route => {
    const body = route.request().postDataJSON();
    saves.push(body);
    return route.fulfill({ json: { instance_id: "egress", current_revision: 1, revisions: [{ revision: 1, config: body.config }] } });
  });
  await page.goto(`${appURL}/#gateways`);
  const source = page.locator("#gateway-source");
  await expect(source).toHaveValue(/"instance_id":/);
  const config = JSON.parse(await source.inputValue());
  config.extra = { preserve: true };
  config.destinations[0].extra = "also preserved";
  const original = JSON.stringify(config, null, 4);
  await source.fill(original);
  await page.getByRole("button", { name: "Form editor", exact: true }).click();
  await expect(source).toBeHidden();
  await expect(page.getByRole("region", { name: "Gateway draft form" })).toBeVisible();
  expect(await source.inputValue()).toBe(original);
  await expect(page.locator("#gateway-form-identity")).toContainText("egress.svc.zpr");
  await page.getByRole("textbox", { name: "Destination 1 HTTPS origin", exact: true }).fill("https://api.example.com");
  await page.getByRole("textbox", { name: "Destination 1 path prefixes", exact: true }).fill("/v1/\n/health");
  await page.getByRole("button", { name: "Add destination", exact: true }).click();
  await page.getByRole("textbox", { name: "Destination 2 HTTPS origin", exact: true }).fill("https://other.example.com");
  await page.getByRole("checkbox", { name: "HEAD", exact: true }).uncheck();
  await page.getByRole("spinbutton", { name: "Timeout (milliseconds)", exact: true }).fill("9000");
  await page.getByRole("spinbutton", { name: "Maximum response size (bytes)", exact: true }).fill("1024");
  const edited = JSON.parse(await source.inputValue());
  expect(edited.extra).toEqual({ preserve: true });
  expect(edited.destinations[0]).toEqual({ origin: "https://api.example.com", path_prefixes: ["/v1/", "/health"], extra: "also preserved" });
  expect(edited.methods).toEqual(["GET"]);
  expect(edited.timeout_ms).toBe(9000);
  expect(edited.max_response_bytes).toBe(1024);
  await page.getByRole("button", { name: "Raw JSON editor", exact: true }).click();
  await expect(source).toBeVisible();
  await expect(page.locator("#gateway-form")).toBeHidden();
  expect(JSON.parse(await source.inputValue())).toEqual(edited);
  await page.getByRole("button", { name: "Form editor", exact: true }).click();
  await page.locator("#gateway-analyze").click();
  await expect(page.locator("#gateway-analyze")).toHaveAttribute("data-analysis-state", "success");
  await page.getByRole("textbox", { name: "Destination 1 HTTPS origin", exact: true }).fill("https://changed.example.com");
  await expect(page.locator("#gateway-analyze")).not.toHaveAttribute("data-analysis-state", /.+/);
  await page.locator("#gateway-files-toggle").click();
  await expect(page.locator("#gateway-save")).toBeDisabled();
  await page.keyboard.press("Escape");
  await page.locator("#gateway-analyze").click();
  await expect(page.locator("#gateway-analyze")).toHaveAttribute("data-analysis-state", "success");
  const origin = page.getByRole("textbox", { name: "Destination 1 HTTPS origin", exact: true });
  await origin.focus();
  await page.keyboard.press("Control+s");
  await expect(page.locator("#gateway-draft-message")).toContainText("Saved draft revision 1");
  expect(saves).toHaveLength(1);
  expect(saves[0].expected_revision).toBe(0);
  expect(saves[0].config).toEqual(checks.at(-1));
  await expect(page.getByRole("button", { name: "Activate", exact: true })).toHaveCount(0);
});

test("Gateway form rejects incompatible raw drafts and incomplete numbers without overwriting source", async ({ page, appURL, api }) => {
  const contract = { organization_id: "alpha", instance_id: "egress", adapter_cn: "egress", service_name: "egress.svc.zpr" };
  api.handlers.set("/api/gateways/contracts", route => route.fulfill({ json: { contracts: [contract] } }));
  api.handlers.set("/api/gateways/configs", route => route.fulfill({ json: { configs: [] } }));
  api.handlers.set("/api/gateways/config/check", route => route.fulfill({
    status: 422, json: { valid: false, diagnostics: "destination 1 requires an HTTPS origin", source_line: 10 },
  }));
  await page.goto(`${appURL}/#gateways`);
  const source = page.locator("#gateway-source");
  await expect(source).toHaveValue(/"instance_id":/);
  const original = await source.inputValue();
  for (const invalid of ["{broken", JSON.stringify({ ...JSON.parse(original), methods: ["POST"] })]) {
    await source.fill(invalid);
    await page.getByRole("button", { name: "Form editor", exact: true }).click();
    await expect(page.locator("#gateway-draft-message")).toContainText("Cannot open form editor");
    await expect(source).toHaveValue(invalid);
    await expect(page.locator("#gateway-form")).toBeHidden();
  }
  await source.fill(original);
  await page.getByRole("button", { name: "Form editor", exact: true }).click();
  const timeout = page.getByRole("spinbutton", { name: "Timeout (milliseconds)", exact: true });
  await timeout.fill("");
  await expect(page.locator("#gateway-analyze")).toBeDisabled();
  expect(await source.inputValue()).toBe(original);
  await page.getByRole("button", { name: "Raw JSON editor", exact: true }).click();
  await expect(page.locator("#gateway-form-error")).toContainText("Enter numeric");
  await expect(source).toBeHidden();
  await timeout.fill("8000");
  await page.locator("#gateway-analyze").click();
  await expect(page.locator("#gateway-form-error")).toContainText("requires an HTTPS origin");
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
  await page.getByRole("button", { name: "Raw JSON editor", exact: true }).click();
  await expect(page.locator("#gateway-form-error")).toBeHidden();
  await page.locator("#gateway-analyze").click();
  await expect(page.locator("#gateway-gutter .policy-test-line-result")).toHaveCount(1);
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
});

test("Gateway raw editor keeps destination form controls separate without runtime activation", async ({ page, appURL, api }) => {
  const contract = { organization_id: "alpha", instance_id: "egress", adapter_cn: "egress", service_name: "egress.svc.zpr" };
  api.handlers.set("/api/gateways/contracts", route => route.fulfill({ json: { organization_id: "alpha", contracts: [contract] } }));
  api.handlers.set("/api/gateways/configs", route => route.fulfill({ json: { configs: [] } }));
  const checks = [];
  api.handlers.set("/api/gateways/config/check", route => {
    checks.push(route.request().postDataJSON().config);
    return route.fulfill({ json: { valid: true, diagnostics: "Runtime is unchanged." } });
  });
  await page.goto(`${appURL}/#gateways`);
  const source = page.getByRole("textbox", { name: "Gateway draft JSON" });
  await expect(source).toHaveValue(/"instance_id":/);
  await expect(page.locator("#gateway-destinations")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Add destination" })).toHaveCount(0);
  const config = JSON.parse(await source.inputValue());
  config.destinations = [
    { origin: "https://api.example.com", path_prefixes: ["/"] },
    { origin: "https://other.example.com", path_prefixes: ["/v1/", "/health"] },
  ];
  await source.fill(JSON.stringify(config, null, 2));
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  await expect(page.locator("#gateway-analyze")).toHaveAttribute("data-analysis-state", "success");
  expect(checks[0].destinations).toEqual([
    { origin: "https://api.example.com", path_prefixes: ["/"] },
    { origin: "https://other.example.com", path_prefixes: ["/v1/", "/health"] },
  ]);
  config.destinations.pop();
  await source.fill(JSON.stringify(config, null, 2));
  await expect(page.locator("#gateway-analyze")).not.toHaveAttribute("data-analysis-state", /.+/);
  expect(JSON.parse(await source.inputValue()).destinations).toHaveLength(1);
  await expect(page.getByRole("button", { name: "Activate", exact: true })).toHaveCount(0);
});

test("Gateway Analyze preserves source lines and selects the offending destination", async ({ page, appURL, api }) => {
  const contract = { organization_id: "alpha", instance_id: "egress", adapter_cn: "egress", service_name: "egress.svc.zpr" };
  api.handlers.set("/api/gateways/contracts", route => route.fulfill({ json: { contracts: [contract] } }));
  api.handlers.set("/api/gateways/configs", route => route.fulfill({ json: { configs: [] } }));
  let submitted = "";
  api.handlers.set("/api/gateways/config/check", route => {
    submitted = route.request().postData();
    const line = submitted.split("\n").findIndex(value => value.includes('"origin"')) + 1;
    return route.fulfill({ status: 422, json: { valid: false, diagnostics: "destination requires an HTTPS origin", source_line: line } });
  });
  await page.goto(`${appURL}/#gateways`);
  const source = page.getByRole("textbox", { name: "Gateway draft JSON" });
  await expect(source).toHaveValue(/"origin": ""/);
  const original = await source.inputValue();
  const line = original.split("\n").findIndex(value => value.includes('"origin"')) + 1;
  await page.getByRole("button", { name: "Analyze", exact: true }).click();
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
  await expect(page.locator("#gateway-error-dialog")).toBeHidden();
  expect(submitted).toBe(`{"config":${original}}`);
  const marker = page.locator("#gateway-gutter .policy-test-line-result");
  const geometry = await marker.evaluate(element => {
    const marker = element.getBoundingClientRect();
    const gutter = element.closest(".config-source-gutter").getBoundingClientRect();
    return { left: marker.left - gutter.left, right: gutter.right - marker.right };
  });
  expect(geometry.left).toBeCloseTo(10, 0);
  expect(geometry.right).toBeGreaterThan(4);
  await page.getByRole("button", { name: `Gateway draft error on line ${line}: destination requires an HTTPS origin` }).click();
  await expect(page.locator("#gateway-error-dialog")).toBeVisible();
  await expect(page.locator("#gateway-error-text")).toHaveText("destination requires an HTTPS origin");
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
  await page.keyboard.press("Escape");
  await expect(source).toBeFocused();
  await expect(source).toHaveCSS("outline-style", "none");
  expect(await source.evaluate(element => element.value.slice(element.selectionStart, element.selectionEnd))).toContain('"origin": ""');
  await source.fill(original.replace('"origin": ""', '"origin": "https://example.com"'));
  await expect(page.locator("#gateway-gutter .policy-test-line-result")).toHaveCount(0);
  await expect(page.locator("#gateway-draft-message")).toBeHidden();
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

test("GUI Map Auto-fit checkbox controls refresh fitting and preserves manual Fit", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::1", node_details: { adapters: ["client"] } },
    { cn: "client", node: false, zpr_addr: "fd00::2" },
  ];
  await page.goto(appURL + "/#map");
  const autoFit = page.getByRole("checkbox", { name: "Auto-fit", exact: true });
  await expect(autoFit).toBeChecked();
  await autoFit.uncheck();
  await page.getByRole("button", { name: "Zoom in", exact: true }).click();
  const viewport = () => page.evaluate(() => ({
    box: document.querySelector(".topology-graph").getAttribute("viewBox"),
    transform: document.querySelector("#graph-world").getAttribute("transform"),
  }));
  const manual = await viewport();
  api.snapshot.actors.push({ cn: "new-node", node: true, zpr_addr: "fd00::3", node_details: { adapters: [] } });
  await page.locator("#refresh-now").click();
  await expect(page.locator('.topology-graph [data-inspect-actor="new-node"]')).toHaveCount(1);
  await expect(autoFit).not.toBeChecked();
  expect(await viewport()).toEqual(manual);
  await page.getByRole("button", { name: "Fit graph", exact: true }).click();
  expect((await viewport()).transform).not.toBe(manual.transform);
  await expect(autoFit).not.toBeChecked();
  await autoFit.check();
  await page.getByRole("button", { name: "Zoom in", exact: true }).click();
  const zoomed = await viewport();
  await expect(autoFit).not.toBeChecked();
  await page.locator("#refresh-now").click();
  expect(await viewport()).toEqual(zoomed);
  await autoFit.check();
  await expect(autoFit).toBeChecked();
  const fitted = await page.evaluate(() => {
    const svg = document.querySelector(".topology-graph");
    const world = document.querySelector("#graph-world");
    const bounds = world.getBBox();
    const matrix = world.transform.baseVal.consolidate().matrix;
    const box = svg.viewBox.baseVal;
    return {
      centerX: (bounds.x + bounds.width / 2) * matrix.a + matrix.e,
      centerY: (bounds.y + bounds.height / 2) * matrix.d + matrix.f,
      expectedX: box.x + box.width / 2, expectedY: box.y + box.height / 2,
    };
  });
  expect(Math.abs(fitted.centerX - fitted.expectedX)).toBeLessThan(1);
  expect(Math.abs(fitted.centerY - fitted.expectedY)).toBeLessThan(1);
});

for (const reducedMotion of ["no-preference", "reduce"]) {
test(`GUI Map draws cached topology on navigation without waiting for refresh (${reducedMotion})`, async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion });
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::ff", node_details: { adapters: ["client"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
  ];
  api.snapshot.services = [{ service_name: "API", actor_cn: "client", service_endpoints: "TCP/443" }];
  let requests = 0;
  api.handlers.set("/api/snapshot", async (route) => {
    requests += 1;
    await route.fulfill({ json: api.snapshot });
  });
  await page.goto(appURL + "/#adapter-logs");
  await expect(page.locator("#connection-count")).toHaveText("1 Connections");
  await page.locator("#pause-poll").click();
  const before = requests;
  await page.locator('[data-page-link="map"]').click();
  await expect(page.locator(".graph-vertex")).toHaveCount(2);
  await expect(page.locator(".graph-service-badge")).toHaveCount(1);
  const geometry = () => page.evaluate(() => {
    const svg = document.querySelector(".topology-graph");
    const world = document.querySelector("#graph-world");
    const view = svg.getBoundingClientRect();
    const bounds = world.getBoundingClientRect();
    return {
      fitted: bounds.width > 0 && bounds.height > 0 && bounds.left >= view.left - 1
        && bounds.right <= view.right + 1 && bounds.top >= view.top - 1 && bounds.bottom <= view.bottom + 1,
      links: [...document.querySelectorAll(".graph-link")].every(line => {
        const values = ["x1", "y1", "x2", "y2"].map(key => Number(line.getAttribute(key)));
        return values.every(Number.isFinite) && Math.hypot(values[2] - values[0], values[3] - values[1]) > 1;
      }),
    };
  });
  await expect.poll(geometry).toEqual({ fitted: true, links: true });
  expect(requests).toBe(before);
  await page.locator('[data-page-link="adapter-logs"]').click();
  api.snapshot.actors[0].node_details.adapters.push("server");
  api.snapshot.actors.push({ cn: "server", node: false, zpr_addr: "fd00::2" });
  api.snapshot.services.push({ service_name: "Other", actor_cn: "server", service_endpoints: "TCP/80" });
  await page.locator("#refresh-now").click();
  await expect(page.locator("#connection-count")).toHaveText("2 Connections");
  const refreshed = requests;
  await page.locator('[data-page-link="map"]').click();
  await expect(page.locator(".graph-vertex")).toHaveCount(3);
  await expect(page.locator(".graph-service-badge")).toHaveCount(2);
  await expect.poll(geometry).toEqual({ fitted: true, links: true });
  expect(requests).toBe(refreshed);
  await page.getByRole("button", { name: "Zoom in", exact: true }).click();
  const camera = await page.locator("#graph-world").getAttribute("transform");
  const viewBox = await page.locator(".topology-graph").getAttribute("viewBox");
  await page.locator('[data-page-link="adapter-logs"]').click();
  await page.locator("#refresh-now").click();
  await expect(page.locator("#refresh-now")).toBeEnabled();
  await page.locator('[data-page-link="map"]').click();
  await expect(page.locator("[data-graph-auto-fit]")).not.toBeChecked();
  await expect(page.locator("#graph-world")).toHaveAttribute("transform", camera);
  await expect(page.locator(".topology-graph")).toHaveAttribute("viewBox", viewBox);
});
}

test("GUI Map distinguishes loading, unavailable, and empty topology", async ({ page, appURL, api }) => {
  let release;
  const pending = new Promise(resolve => { release = resolve; });
  api.handlers.set("/api/snapshot", async (route) => {
    await pending;
    await route.fulfill({ status: 503, json: { error: "Unavailable" } });
  });
  await page.goto(appURL + "/#map");
  await expect(page.locator("#topology-stage")).toContainText("Loading topology");
  release();
  await expect(page.locator("#topology-stage")).toContainText("Unable to load topology");
  await expect(page.locator("#alert-strip")).toContainText("503");
  await page.locator("#pause-poll").click();
  api.handlers.delete("/api/snapshot");
  await page.locator("#refresh-now").click();
  await expect(page.locator("#topology-stage")).toContainText("No nodes or adapters reported");
  api.snapshot.api_status = "partial";
  api.snapshot.errors = ["Actor inventory unavailable"];
  await page.locator("#refresh-now").click();
  await expect(page.locator("#topology-stage")).toContainText("Topology unavailable");
  api.snapshot.api_status = "connected";
  api.snapshot.errors = [];
  api.snapshot.actors = [{ cn: "node", node: true }];
  await page.locator("#refresh-now").click();
  await expect(page.locator(".graph-vertex")).toHaveCount(1);
  api.statuses.set("/api/snapshot", 503);
  await page.locator("#refresh-now").click();
  await expect(page.locator("#alert-strip")).toContainText("503");
  await expect(page.locator(".graph-vertex")).toHaveCount(1);
});

test("GUI visa count opens complete current adapter and service inventories", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::a", node_details: { adapters: ["client", "server"], buffered_denials: 2 } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
    { cn: "server", node: false, zpr_addr: "fd00::2" },
  ];
  api.snapshot.services = [
    { service_name: "HTTPS", actor_cn: "server", service_endpoints: "TCP/443" },
    { service_name: "HTTP", actor_cn: "server", service_endpoints: "TCP/80" },
  ];
  const visa = { expires: Date.now() / 1000 + 3600, source_addr: "fd00:0:0:0:0:0:0:1", dest_addr: "fd00::2", dest_port: 443, proto: "TCP", path: ["fd00::a"], policy_id: "<script>bad()</script>" };
  api.snapshot.active_visas = [
    ...Array.from({ length: 15 }, (_, index) => ({ ...visa, id: index + 1 })),
    { ...visa, id: 1 }, // Duplicate snapshots must match the badge's deduplicated inventory.
    { ...visa, id: 16, direction: "reverse", source_addr: "fd00::2", source_port: 443, dest_addr: "fd00::1", dest_port: 50000 },
    { ...visa, id: 99, expires: Date.now() / 1000 - 10 },
  ];
  api.snapshot.recent_visas = [{ ...visa, id: 888 }];
  await page.route("**/api/simulator/**", route => route.abort());
  await page.goto(appURL + "/#map");
  await page.locator("#pause-poll").click();
  const clientCount = page.locator('.graph-vertex[data-inspect-actor="client"] .graph-visa-count');
  await expect(clientCount).toHaveAttribute("aria-label", "16 active visas");
  await clientCount.click({ button: "right" });
  const inspector = page.locator("#component-inspector");
  await expect(page.locator("#topology-stage")).toHaveClass(/graph-visa-focused/);
  await expect(inspector).not.toHaveClass(/open/);
  await clientCount.click();
  await expect(inspector).toHaveClass(/open/);
  await expect(inspector.locator(".detail-item")).toHaveCount(16);
  await expect(inspector).toContainText("Route: fd00::a");
  await expect(inspector).toContainText("Policy: <script>bad()</script>");
  await expect(inspector.locator("script")).toHaveCount(0);
  await expect(inspector).not.toContainText("Visa 888");
  await expect(page.locator("#topology-stage")).toHaveClass(/graph-visa-focused/);
  await page.locator(".status-banner").click({ position: { x: 8, y: 8 } });
  await expect(inspector).not.toHaveClass(/open/);
  await clientCount.click();
  await expect(inspector).toHaveClass(/open/);
  await page.locator("#inspector-close").click();
  await clientCount.click();
  await expect(page.locator("#inspector-kind")).toHaveText("ACTIVE VISAS");
  await expect(inspector.locator(".detail-item")).toHaveCount(16);
  await page.locator("#inspector-close").click();
  const httpsCount = page.locator('.graph-service-badge[data-inspect-service="HTTPS"] .graph-visa-count');
  await httpsCount.focus();
  await page.keyboard.press("Enter");
  await expect(page.locator("#inspector-kind")).toHaveText("ACTIVE VISAS");
  await expect(page.locator("#inspector-title")).toHaveText("HTTPS");
  await expect(inspector.locator(".detail-item")).toHaveCount(16);
  await page.locator("#inspector-close").click();
  const httpCount = page.locator('.graph-service-badge[data-inspect-service="HTTP"] .graph-visa-count');
  await httpCount.click();
  await expect(inspector).toContainText("No current visas.");
  await expect(inspector.locator(".detail-item")).toHaveCount(0);
  await page.locator("#inspector-close").click();
  await httpsCount.click();
  api.snapshot.active_visas = [];
  await page.locator("#refresh-now").click();
  await expect(inspector).not.toHaveClass(/open/);
  await clientCount.click();
  await expect(inspector).toContainText("No current visas.");
  delete api.snapshot.active_visas;
  await page.locator("#refresh-now").click();
  await expect(inspector).not.toHaveClass(/open/);
  await clientCount.click();
  await expect(inspector).toContainText("Active visa inventory unavailable");
  await expect(inspector.locator(".detail-item")).toHaveCount(0);
  api.snapshot.services = [];
  await page.locator("#refresh-now").click();
  await expect(inspector).not.toHaveClass(/open/);
});

test("GUI Simulator navigation order survives page switching", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/machine-logs.html");
  const paths = ["/organizations.html", "/scenarios.html", "/trusted-source.html", "/activity.html", "/machine-logs.html"];
  const nav = page.locator(".primary-nav a[data-simulator-nav]");
  await expect.poll(() => nav.evaluateAll(links => links.map(link => link.getAttribute("href")))).toEqual(paths);
  for (const path of paths.slice(0, -1)) {
    await page.locator(`.primary-nav a[href="${path}"]`).click();
    await expect(page).toHaveURL(appURL + path);
    await expect.poll(() => nav.evaluateAll(links => links.map(link => link.getAttribute("href")))).toEqual(paths);
    await expect(page.locator(".primary-nav a.active")).toHaveAttribute("href", path);
  }
  await page.locator('.primary-nav a[href="/machine-logs.html"]').click();
  await expect(page.locator(".primary-nav a.active")).toHaveAttribute("href", "/machine-logs.html");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("GUI Workers merges passive device inventory and logs with type filtering and word wrap", async ({ page, appURL, api }) => {
  const [laptop, desktop] = api.workloadLogs.machines;
  laptop.machine = { ...laptop.machine, type: "laptop", owner: "alice", secure: true, location: "HQ" };
  laptop.controller = { connected: true };
  laptop.session = { authenticated: true, user: "alice" };
  laptop.workloads = [{ name: "finance-client", kind: "client", agent: "adapter-1", address: "fd00::1", state: "running" }];
  laptop.sources[0].lines = ["x".repeat(1000)];
  desktop.machine = { ...desktop.machine, type: "desktop", secure: false, owner: "bob" };
  desktop.state = "stopped";
  desktop.controller = { connected: false };
  desktop.session = { authenticated: false };
  desktop.workloads = [];
  const mutations = [];
  page.on("request", request => {
    if (request.url().includes("/api/simulator/") && request.method() !== "GET") mutations.push(request.url());
  });
  await page.goto(appURL + "/agents.html");
  await expect(page).toHaveURL(appURL + "/machine-logs.html");
  await expect(page).toHaveTitle("ZPR Simulator Workers");
  await expect(page.locator('.primary-nav a[href="/agents.html"]')).toHaveCount(0);
  await expect(page.locator('.primary-nav a[href="/machine-logs.html"]')).toHaveText("Workers");
  const panels = page.locator(".machine-log-panel");
  await expect(panels).toHaveCount(2);
  const first = panels.first();
  await first.locator(".worker-details summary").click();
  await expect(first.locator(".worker-details")).toContainText("Controller: Connected");
  await expect(first.locator(".worker-details")).toContainText("Authenticated as alice");
  await expect(first.locator(".worker-details")).toContainText("finance-client (client): running");
  await expect(first.locator(".worker-details")).toContainText("fd00::1");
  await expect(page.locator('[data-machine-action], [data-session-action], [data-login-machine], [data-workload-name], [data-save-workloads]')).toHaveCount(0);
  const output = first.locator(".machine-log-output");
  await expect(page.getByRole("checkbox", { name: "Wrap", exact: true })).not.toBeChecked();
  await expect.poll(() => output.evaluate(element => element.scrollWidth > element.clientWidth)).toBe(true);
  await page.getByRole("checkbox", { name: "Wrap", exact: true }).check();
  await expect.poll(() => output.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.getByRole("checkbox", { name: "Wrap", exact: true }).uncheck();
  await expect.poll(() => output.evaluate(element => element.scrollWidth > element.clientWidth)).toBe(true);
  await first.getByRole("button", { name: /Maximize/ }).click();
  await expect(first.locator("pre")).toHaveCSS("white-space", "pre");
  await page.keyboard.press("Escape");
  await page.getByRole("checkbox", { name: "Wrap", exact: true }).check();
  await page.locator("#machine-type-filter").selectOption("desktop");
  await expect(first).toBeHidden();
  await expect(panels.nth(1)).toBeVisible();
  await panels.nth(1).locator(".worker-details summary").click();
  await expect(panels.nth(1)).toContainText("Controller: Offline");
  await expect(panels.nth(1)).toContainText("No authenticated user");
  await page.locator("#machine-logs-running").check();
  await expect(page.locator(".workers-empty")).toHaveText("No workers match the filters.");
  await page.locator("#machine-type-filter").selectOption("all");
  await expect(first).toBeVisible();
  await page.locator("#machine-logs-pause").click();
  await expect(page.locator("#machine-logs-status")).toHaveText("Paused");
  expect(mutations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test("GUI scenario failure progress preserves the original step through cleanup", async ({ page, appURL, api }) => {
  api.handlers.set("/api/simulator/scenarios", async route => route.fulfill({ json: {
    active_organization_id: "alpha", scenarios: [], run: { state: "idle", steps: [] },
  } }));
  await page.goto(appURL + "/scenarios.html");
  await expect(page.locator("#scenario-run-state")).toContainText("idle");
  await page.evaluate(() => {
    window.guiFailureRun = {
      scenario_id: "failed-fixture", state: "cleaning", current_step: 8, total_steps: 9,
      scenario: { steps: [{ action: "start_machine", machine: "machine-03" }], cleanup: [] },
      steps: [
        { number: 1, phase: "run", action: "start_machine", machine: "machine-03", status: "failed", error: "Owner missing" },
        { number: 8, phase: "cleanup", action: "stop_workload", status: "failed", error: "Controller offline" },
      ],
    };
    renderScenarioRun(window.guiFailureRun);
  });
  await expect(page.locator(".scenario-progress")).toHaveText("Failed step 1 / 9 · Cleanup 8 / 9");
  await expect(page.locator("#scenario-run-summary")).toContainText("Step 1: start machine on machine-03");
  await page.evaluate(() => renderScenarioRun({ ...window.guiFailureRun, state: "failed", current_step: 9, error: "Owner missing" }));
  await expect(page.locator(".scenario-progress")).toHaveText("Failed step 1 / 9");
  await expect(page.locator("#scenario-run-summary")).toContainText("Owner missing");
  await expect(page.locator("#scenario-run-summary .scenario-run-error")).toHaveCount(1);
  await expect(page.locator("#scenario-clear")).toBeVisible();
  await page.evaluate(() => renderScenarioRun({
    ...window.guiFailureRun, state: "failed",
    steps: [{ number: 8, phase: "cleanup", action: "stop_workload", status: "failed", error: "Cleanup failed" }],
  }));
  await expect(page.locator(".scenario-progress")).toHaveText("Failed step 8 / 9");
  await page.evaluate(() => renderScenarioRun({ scenario_id: "running-fixture", state: "running", current_step: 1, total_steps: 2, steps: [] }));
  await expect.poll(() => page.locator("#scenario-run-state").evaluate(element => ({
    background: getComputedStyle(element).backgroundColor, color: getComputedStyle(element).color,
  }))).toEqual({ background: "rgb(21, 95, 192)", color: "rgb(255, 255, 255)" });
});

test("GUI Status tables scroll vertically with retained sortable headings", async ({ page, appURL, api }) => {
  api.snapshot.actors = Array.from({ length: 120 }, (_, index) => ({
    cn: `actor-${String(index).padStart(3, "0")}`, node: false, zpr_addr: `fd00::${index + 1}`,
  }));
  await page.goto(appURL + "/#actors");
  const scroll = page.locator("#page-actors .table-scroll");
  await expect(page.locator("#actor-rows tr")).toHaveCount(120);
  const dimensions = await scroll.evaluate(element => ({
    height: element.clientHeight, total: element.scrollHeight, viewport: innerHeight,
    overflow: getComputedStyle(element).overflowY,
  }));
  expect(dimensions.total).toBeGreaterThan(dimensions.height);
  expect(dimensions.height).toBeLessThanOrEqual(dimensions.viewport * 0.65 + 1);
  expect(dimensions.overflow).toBe("auto");
  await scroll.evaluate(element => { element.scrollTop = 500; });
  const positions = await page.locator('#page-actors th[data-sort-key="cn"]').evaluate(element => ({
    heading: element.getBoundingClientRect().top,
    container: element.closest(".table-scroll").getBoundingClientRect().top,
  }));
  expect(Math.abs(positions.heading - positions.container)).toBeLessThan(2);
  await page.locator('#page-actors th[data-sort-key="cn"]').click();
  await expect(page.locator('#page-actors th[data-sort-key="cn"]')).toHaveAttribute("aria-sort", "descending");
  const headingColor = await page.locator('#page-actors th[data-sort-key="cn"]').evaluate(element => getComputedStyle(element).backgroundColor);
  expect(headingColor).toBe("rgb(233, 243, 212)");
});

test("GUI Map count badges overlap upper-right glyph boundaries", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::ff", node_details: { adapters: ["client", "vs", "gateway"], buffered_denials: 123456 } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
    { cn: "vs", node: false, zpr_addr: "fd00::2" },
    { cn: "gateway", node: false, zpr_addr: "fd00::3" },
  ];
  api.snapshot.services = [
    { service_name: "API", actor_cn: "client", zpr_addr: "fd00::1", service_kind: "Regular", service_endpoints: "TCP/443" },
    { service_name: "Visa", actor_cn: "vs", zpr_addr: "fd00::2", service_kind: "Visa", service_endpoints: "TCP/5002" },
    { service_name: "Gateway", actor_cn: "gateway", zpr_addr: "fd00::3", service_kind: "Gateway", service_endpoints: "TCP/443" },
  ];
  api.snapshot.active_visas = [];
  await page.goto(appURL + "/#map");
  await expect(page.locator(".graph-visa")).toHaveCount(1);
  const anchors = await page.locator("[data-topology-component]").evaluateAll(elements => elements.map(element => {
    const badge = element.querySelector(".graph-visa-count rect");
    const glyph = element.querySelector(":scope > .graph-node, :scope > .graph-adapter, :scope > .graph-visa, :scope > .graph-gateway, :scope > rect");
    if (!badge || !glyph) return null;
    const bounds = badge.getBBox();
    const shape = glyph.getBBox();
    const x = bounds.x + bounds.width / 2, y = bounds.y + bounds.height / 2;
    const origin = { x: Number(element.dataset.originX), y: Number(element.dataset.originY) };
    return { type: glyph.tagName, gateway: glyph.classList.contains("graph-gateway"), dx: x - origin.x, dy: y - origin.y, halfWidth: shape.width / 2, halfHeight: shape.height / 2 };
  }).filter(Boolean));
  expect(anchors).toHaveLength(7);
  for (const anchor of anchors) {
    expect(anchor.dx).toBeGreaterThan(0);
    expect(anchor.dy).toBeLessThan(0);
    if (anchor.type === "circle") expect(Math.hypot(anchor.dx, anchor.dy)).toBeCloseTo(29, 4);
    else if (anchor.type === "polygon") {
      expect(anchor.dx).toBeCloseTo(anchor.gateway ? 28 : 17.5, 4);
      expect(anchor.dy).toBeCloseTo(anchor.gateway ? -16 : -17.5, 4);
    } else {
      expect(anchor.dx).toBeCloseTo(anchor.halfWidth - 2, 4);
      expect(anchor.dy).toBeCloseTo(-anchor.halfHeight + 2, 4);
    }
  }
});

test("GUI Map shows complete active visa counts and solid component links", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const expires = Math.floor(Date.now() / 1000) + 3600;
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::ff", node_details: { adapters: ["client", "server"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
    { cn: "server", node: false, zpr_addr: "fd00::2" },
  ];
  api.snapshot.services = [
    { service_name: "API", actor_cn: "server", zpr_addr: "fd00::2", service_endpoints: "TCP/443-444" },
    { service_name: "Other", actor_cn: "server", zpr_addr: "fd00::2", service_endpoints: "UDP/443" },
  ];
  api.snapshot.active_visas = Array.from({ length: 12 }, (_, index) => ({
    id: index + 1, expires, source_addr: "fd00:0:0:0:0:0:0:1", dest_addr: "fd00::2",
    proto: "TCP", dest_port: 443, direction: "forward",
  }));
  api.snapshot.active_visas.push(
    { ...api.snapshot.active_visas[0] },
    { id: 13, expires, source_addr: "fd00::2", dest_addr: "fd00::1", proto: "TCP", source_port: 444, direction: "reverse" },
    { id: 14, expires: 1, source_addr: "fd00::1", dest_addr: "fd00::2", proto: "TCP", dest_port: 443 },
    { id: 15, expires, source_addr: "fd00::1", dest_addr: "fd00:0:0:0:0:0:0:1", proto: "TCP", dest_port: 443 },
  );
  api.snapshot.recent_visas = api.snapshot.active_visas.slice(0, 10);
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const clientCount = page.locator('[data-inspect-actor="client"] .graph-visa-count');
  const serverCount = page.locator('[data-inspect-actor="server"] .graph-visa-count');
  const serviceCount = page.locator('[data-inspect-service="API"] .graph-visa-count');
  await expect(clientCount).toHaveAttribute("aria-label", "14 active visas");
  await expect(clientCount.locator("text")).toHaveText("14");
  await expect(serverCount).toHaveAttribute("aria-label", "13 active visas");
  await expect(serviceCount).toHaveAttribute("aria-label", "13 active visas");
  await expect(page.locator('[data-inspect-service="Other"] .graph-visa-count')).toHaveAttribute("aria-label", "0 active visas");
  await expect(page.locator('.graph-vertex[data-inspect-actor="node"] .graph-denial-count')).toHaveAttribute("aria-label", "Buffered denial count unavailable");
  for (const line of await page.locator(".graph-link").all()) await expect(line).toHaveCSS("stroke-dasharray", "none");
  await expect(page.locator(".legend-service-link")).toHaveCSS("border-top-style", "solid");
  const fitted = await page.evaluate(() => {
    const world = document.querySelector("#graph-world");
    const box = document.querySelector(".topology-graph").viewBox.baseVal;
    const bounds = world.getBBox();
    const matrix = world.transform.baseVal.consolidate().matrix;
    return {
      left: bounds.x * matrix.a + matrix.e,
      right: (bounds.x + bounds.width) * matrix.a + matrix.e,
      top: bounds.y * matrix.d + matrix.f,
      bottom: (bounds.y + bounds.height) * matrix.d + matrix.f,
      x: box.x, y: box.y, width: box.width, height: box.height,
    };
  });
  expect(fitted.left).toBeGreaterThan(fitted.x);
  expect(fitted.right).toBeLessThan(fitted.x + fitted.width);
  expect(fitted.top).toBeGreaterThan(fitted.y);
  expect(fitted.bottom).toBeLessThan(fitted.y + fitted.height);
  api.snapshot.active_visas = null;
  await page.locator("#refresh-now").click();
  await expect(clientCount).toHaveAttribute("aria-label", "Active visa count unavailable");
  await expect(serviceCount.locator("text")).toHaveText("?");
  api.snapshot.active_visas = [];
  await page.locator("#refresh-now").click();
  await expect(clientCount).toHaveAttribute("aria-label", "0 active visas");
});

for (const reducedMotion of ["no-preference", "reduce"]) {
test(`GUI Map denial badges and count changes pulse without replay (${reducedMotion})`, async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion });
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::ff", node_details: { adapters: ["client"], buffered_denials: 0, local_denials: 19 } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
  ];
  api.snapshot.services = [{ service_name: "API", actor_cn: "client", zpr_addr: "fd00::1", service_endpoints: "TCP/443" }];
  api.snapshot.active_visas = [];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const node = page.locator('.graph-vertex[data-inspect-actor="node"]');
  const nodeCount = node.locator(".graph-denial-count");
  await expect(nodeCount).toHaveAttribute("aria-label", "0 buffered denials");
  await expect(nodeCount.locator("text")).toHaveCount(0);
  await expect(nodeCount.locator("rect")).toHaveCSS("fill", "rgb(255, 255, 255)");
  await expect(nodeCount.locator("rect")).toHaveCSS("stroke", "rgb(161, 44, 44)");
  await expect(page.locator("[data-count-pulse]")).toHaveCount(0);
  api.snapshot.actors[0].node_details.buffered_denials = 3;
  api.snapshot.active_visas = [{ id: 1, expires: Math.floor(Date.now() / 1000) + 3600, source_addr: "fd00::2", dest_addr: "fd00::1", dest_port: 443, proto: "TCP" }];
  await page.locator("#refresh-now").click();
  await expect(nodeCount).toHaveAttribute("data-count-pulse", "true");
  await expect(nodeCount.locator("text")).toHaveText("3");
  await expect(nodeCount.locator("rect")).toHaveCSS("fill", "rgb(161, 44, 44)");
  await expect(page.locator('[data-inspect-service="API"] .graph-visa-count')).toHaveAttribute("data-count-pulse", "true");
  const pulseGeometry = await nodeCount.evaluate(async el => {
    const animation = el.getAnimations()[0];
    const base = el.getBBox();
    animation.pause();
    animation.currentTime = 600;
    await new Promise(resolve => requestAnimationFrame(resolve));
    await new Promise(resolve => requestAnimationFrame(resolve));
    const matrix = el.parentElement.getScreenCTM().inverse().multiply(el.getScreenCTM());
    const center = new DOMPoint(base.x + base.width / 2, base.y + base.height / 2);
    const expandedCenter = center.matrixTransform(matrix);
    return {
      duration: animation.effect.getTiming().duration,
      scale: matrix.a,
      centerDrift: Math.hypot(expandedCenter.x - center.x, expandedCenter.y - center.y),
    };
  });
  expect(pulseGeometry.duration).toBe(2400);
  expect(pulseGeometry.scale).toBeCloseTo(reducedMotion === "reduce" ? 1 : 1.75, 4);
  expect(pulseGeometry.centerDrift).toBeLessThan(0.1);
  await nodeCount.evaluate(el => el.getAnimations()[0].finish());
  await expect(page.locator("[data-count-pulse]")).toHaveCount(0);
  await page.locator("#refresh-now").click();
  await expect(page.locator("[data-count-pulse]")).toHaveCount(0);
  api.snapshot.actors[0].node_details.buffered_denials = 0;
  api.snapshot.active_visas = [];
  await page.locator("#refresh-now").click();
  await expect(nodeCount).toHaveAttribute("data-count-pulse", "true");
  await expect(nodeCount.locator("text")).toHaveCount(0);
  if (reducedMotion !== "reduce") await expect.poll(() => nodeCount.evaluate(el => Number(el.getAttribute("transform")?.match(/scale\(([^)]+)/)?.[1]))).toBeGreaterThan(1.6);
  await expect(page.locator("[data-count-pulse]")).toHaveCount(0);
  api.snapshot.actors[0].node_details.buffered_denials = null;
  await page.locator("#refresh-now").click();
  await expect(nodeCount).toHaveAttribute("aria-label", "Buffered denial count unavailable");
  await expect(page.locator("[data-count-pulse]")).toHaveCount(0);
  await node.click();
  await expect(page.locator("#inspector-body")).toContainText("Local denial occurrences");
  await expect(page.locator("#inspector-body")).toContainText("19");
});
}

test("GUI Security high alerts only color the side indicator", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const nav = page.locator('.primary-nav [data-page-link="security-review"]');
  const colors = () => nav.evaluate(el => {
    const css = getComputedStyle(el);
    return { background: css.backgroundColor, color: css.color };
  });
  const original = await colors();
  api.snapshot.recent_denies = [{ source_addr: "fd00::99", dest_addr: "fd00::20", count: 5, deny_code: "DENY", last_deny_ms: Date.now() }];
  await page.locator("#refresh-now").click();
  await expect(nav).toHaveAttribute("data-high-alert", "true");
  expect(await colors()).toEqual(original);
  expect(await nav.evaluate(el => getComputedStyle(el).boxShadow)).toContain("rgb(255, 121, 102)");
});

test("GUI node details show live management and worker counters without losing integer precision", async ({ page, appURL, api }) => {
  api.snapshot.actors = [{
    cn: "node", node: true, node_details: {
      counters_updated_at: new Date().toISOString(),
      counters: [
        { group: "management", name: "internal_routing_error", value: "0" },
        { group: "fastpath.0", name: "inbound_packets_received", value: "18446744073709551615" },
        { group: "fastpath.1", name: "ttl_reached_0", value: "3" },
      ],
    },
  }];
  await page.goto(appURL + "/#map");
  await page.locator("#pause-poll").click();
  await page.locator('[data-inspect-actor="node"].graph-vertex').click();
  const inspector = page.locator("#inspector-body");
  await expect(inspector).toContainText("Management counters");
  await expect(inspector).toContainText("Fastpath worker 0");
  await expect(inspector).toContainText("Fastpath worker 1");
  await expect(inspector).toContainText("18446744073709551615");
  await expect(inspector).toContainText("TTL Reached 0");
  await expect(inspector).toContainText("not per route or link");
  api.snapshot.actors[0].node_details.counters[1].value = "12";
  await page.locator("#refresh-now").click();
  await expect(inspector).not.toContainText("18446744073709551615");
  await expect(inspector).toContainText("12");
  api.snapshot.actors[0].node_details.counters = null;
  api.snapshot.actors[0].node_details.counter_stats_error = "Node counters are stale or invalid.";
  await page.locator("#refresh-now").click();
  await expect(inspector).toContainText("Node counters are stale or invalid.");
  await expect(inspector).not.toContainText("Fastpath worker");
  await expect(inspector).not.toContainText("Management counters");
});

test("GUI Map service grants pulse their connectors as well as adapters", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node", node: true, node_details: { adapters: ["client", "server"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
    { cn: "server", node: false, zpr_addr: "fd00::2" },
  ];
  api.snapshot.services = [{ service_name: "API", actor_cn: "server", service_endpoints: "TCP/443" }];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  api.snapshot.recent_visas = [{ id: 100, source_addr: "fd00::1", dest_addr: "fd00::2", proto: "TCP", dest_port: 443 }];
  await page.locator("#refresh-now").click();
  await expect(page.locator('.graph-edge[data-dock-adapter="client"] .graph-link')).toHaveAttribute("data-decision", "grant");
  const wire = page.locator(".graph-service-edge .graph-link");
  await expect(wire).toHaveAttribute("data-decision", "grant");
  const widths = await wire.evaluate(el => el.getAnimations()[0].effect.getKeyframes().map(frame => Number.parseFloat(frame.strokeWidth)));
  expect(Math.max(...widths)).toBeGreaterThanOrEqual(4.5);
  await expect(wire).not.toHaveAttribute("data-decision");
});

test("GUI Map connectors meet actual glyph edges for every shape", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  api.snapshot.actors = [
    { cn: "node-a", node: true, zpr_addr: "fd00::a", node_details: { adapters: ["circle", "visa", "gateway"] } },
    { cn: "node-b", node: true, zpr_addr: "fd00::b", node_details: { adapters: [] } },
    { cn: "circle", node: false, zpr_addr: "fd00::1" },
    { cn: "visa", node: false, zpr_addr: "fd00::2" },
    { cn: "gateway", node: false, zpr_addr: "fd00::3" },
  ];
  api.snapshot.network = [{ node_a_addr: "fd00::a", node_b_addr: "fd00::b", ctype: "UP" }];
  api.snapshot.services = ["node-a", "circle", "visa", "gateway"].flatMap((actor, index) =>
    Array.from({ length: 5 }, (_, slot) => ({
      actor_cn: actor, service_name: `${actor}-service-${slot}`,
      service_kind: index === 2 ? "Visa" : index === 3 ? "Gateway" : "Application",
      ...(index === 3 ? { external_network_connection: "partner-network" } : {}),
      service_endpoints: "TCP/443",
    })));
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const inspectEdges = () => page.evaluate(() => {
    const world = document.querySelector("#graph-world");
    const shapes = new Map([...world.querySelectorAll(":scope > [data-topology-component]")].map(component => [
      component.dataset.topologyComponent,
      component.querySelector(":scope > .graph-node, :scope > .graph-adapter, :scope > .graph-gateway, :scope > .graph-visa, :scope > rect"),
    ]));
    const pairs = [...world.querySelectorAll("[data-connector-from]")].map(edge => ({
      name: edge.dataset.topologyEdge,
      lines: [...edge.querySelectorAll("line")],
      from: shapes.get(edge.dataset.connectorFrom), to: shapes.get(edge.dataset.connectorTo),
    }));
    for (const cloud of world.querySelectorAll(".graph-external-network")) pairs.push({
      name: "cloud", lines: [...cloud.querySelectorAll("line")],
      from: shapes.get(cloud.closest("[data-topology-component]").dataset.topologyComponent),
      to: cloud.querySelector(".graph-cloud"),
    });
    return pairs.map(pair => {
      const line = pair.lines[0];
      const matrix = world.getCTM().inverse().multiply(line.parentElement.getCTM());
      const start = new DOMPoint(Number(line.getAttribute("x1")), Number(line.getAttribute("y1"))).matrixTransform(matrix);
      const end = new DOMPoint(Number(line.getAttribute("x2")), Number(line.getAttribute("y2"))).matrixTransform(matrix);
      const x1 = start.x, y1 = start.y, x2 = end.x, y2 = end.y;
      const length = Math.hypot(x2 - x1, y2 - y1);
      const dx = (x2 - x1) / length * 0.2, dy = (y2 - y1) / length * 0.2;
      const filled = (shape, x, y) => shape.isPointInFill(new DOMPoint(x, y).matrixTransform(shape.getCTM().inverse().multiply(world.getCTM())));
      return {
        name: pair.name, length,
        fromInside: filled(pair.from, x1 - dx, y1 - dy),
        fromOutside: filled(pair.from, x1 + dx, y1 + dy),
        toInside: filled(pair.to, x2 + dx, y2 + dy),
        toOutside: filled(pair.to, x2 - dx, y2 - dy),
        layersAgree: pair.lines.every(other => ["x1", "y1", "x2", "y2"].every(key => other.getAttribute(key) === line.getAttribute(key))),
      };
    });
  });
  const verify = async () => {
    const edges = await inspectEdges();
    expect(edges.length).toBe(25);
    for (const edge of edges) {
      expect(edge.length, edge.name).toBeGreaterThan(0);
      expect(edge.fromInside, edge.name).toBe(true);
      expect(edge.fromOutside, edge.name).toBe(false);
      expect(edge.toInside, edge.name).toBe(true);
      expect(edge.toOutside, edge.name).toBe(false);
      expect(edge.layersAgree, edge.name).toBe(true);
    }
  };
  await verify();
  api.snapshot.actors.push({ cn: "node-c", node: true, zpr_addr: "fd00::c", node_details: { adapters: [] } });
  await page.locator("#refresh-now").click();
  await expect(page.locator('.graph-vertex[data-inspect-actor="node-c"]')).toHaveCount(1);
  await verify();
  await page.evaluate(() => {
    for (const name of ["node-a", "gateway", "visa"]) {
      const component = [...document.querySelectorAll(".graph-vertex")].find(element => element.dataset.inspectActor === name);
      const animation = component.animate([{ opacity: 1 }, { opacity: 1 }], { duration: 700, fill: "both" });
      animation.pause();
      animation.currentTime = 350;
      animateGraphMotion(component, animation, { x: 10, y: -5, scale: 0.8 }, { x: 0, y: 0, scale: 1 });
    }
  });
  await verify();
  await expect(page.locator(".graph-cloud-label")).toHaveText("partner-network");
  await page.getByRole("button", { name: "World Map", exact: true }).click();
  await verify();
  await expect(page.locator(".graph-cloud-label")).toHaveText("partner-network");
});

test("GUI Map keeps Dark mode hidden on populated and empty maps", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  api.snapshot.actors = [{ cn: "client", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.services = [{ service_name: "API", actor_cn: "client", service_endpoints: "TCP/443" }];
  api.snapshot.active_visas = [];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const checkbox = page.locator("[data-graph-dark-mode]");
  const stage = page.locator("#topology-stage");
  await expect(checkbox).toBeHidden();
  await expect(checkbox).not.toBeChecked();
  await expect(stage).not.toHaveClass(/graph-dark/);
  await page.locator("#refresh-now").click();
  await expect(checkbox).toBeHidden();
  await page.getByRole("link", { name: "Status", exact: true }).click();
  await page.getByRole("link", { name: "Map", exact: true }).click();
  await expect(checkbox).toBeHidden();
  await expect(stage).not.toHaveClass(/graph-dark/);
  api.snapshot.actors = [];
  api.snapshot.services = [];
  await page.locator("#refresh-now").click();
  await expect(stage).toContainText("No nodes or adapters reported.");
  await expect(checkbox).toBeHidden();
  api.snapshot.actors = [{ cn: "returned", node: false, zpr_addr: "fd00::1" }];
  await page.locator("#refresh-now").click();
  await expect(page.locator('.graph-vertex[data-inspect-actor="returned"]')).toHaveCount(1);
  await expect(checkbox).toBeHidden();
  await expect(stage).not.toHaveClass(/graph-dark/);
});

test("GUI Map right-click highlights only current outbound visa services and ordered routes", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "n1", node: true, zpr_addr: "fd00::a", node_details: { adapters: ["client"] } },
    { cn: "n2", node: true, zpr_addr: "fd00::b", node_details: { adapters: [] } },
    { cn: "n3", node: true, zpr_addr: "fd00::c", node_details: { adapters: ["server", "other"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
    { cn: "server", node: false, zpr_addr: "fd00::2" },
    { cn: "other", node: false, zpr_addr: "fd00::3" },
  ];
  api.snapshot.network = [
    { node_a_addr: "fd00::a", node_b_addr: "fd00::b", ctype: "UP" },
    { node_a_addr: "fd00::b", node_b_addr: "fd00::c", ctype: "UP" },
    { node_a_addr: "fd00::a", node_b_addr: "fd00::c", ctype: "UP" },
  ];
  api.snapshot.services = [
    { service_name: "Allowed", actor_cn: "server", service_endpoints: "TCP/443" },
    { service_name: "Wrong port", actor_cn: "server", service_endpoints: "TCP/80" },
    { service_name: "Expired", actor_cn: "other", service_endpoints: "TCP/443" },
  ];
  api.snapshot.active_visas = [
    { id: 1, expires: Date.now() / 1000 + 3600, source_addr: "fd00::1", dest_addr: "fd00::2", dest_port: 443, proto: "TCP", path: ["fd00:0:0:0:0:0:0:a", "fd00::b", "fd00::c"] },
    { id: 2, expires: Date.now() / 1000 - 10, source_addr: "fd00::1", dest_addr: "fd00::3", dest_port: 443, proto: "TCP", path: ["fd00::a", "fd00::c"] },
    { id: 3, expires: Date.now() / 1000 + 3600, source_addr: "fd00::2", dest_addr: "fd00::1", source_port: 443, proto: "TCP", direction: "reverse", path: ["fd00::c", "fd00::b", "fd00::a"] },
  ];
  await page.goto(appURL + "/#map");
  await page.locator("#pause-poll").click();
  const client = page.locator('.graph-vertex[data-inspect-actor="client"]');
  const stage = page.locator("#topology-stage");
  await client.click({ button: "right" });
  await expect(stage).toHaveClass(/graph-visa-focused/);
  await expect(page.locator(".graph-service-badge.visa-focus")).toHaveCount(1);
  await expect(page.locator(".graph-service-badge.visa-focus")).toHaveAttribute("data-inspect-service", "Allowed");
  await expect(page.locator(".graph-edge.visa-focus")).toHaveCount(4);
  await expect(page.locator('[data-topology-edge="network|n1|n3"]')).not.toHaveClass(/visa-focus/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="other"]')).not.toHaveClass(/visa-focus/);
  await expect(page.locator(".graph-visa-focus-status")).toContainText("2 active outbound visas");
  await page.locator("#refresh-now").click();
  await expect(page.locator(".graph-edge.visa-focus")).toHaveCount(4);
  await client.click({ button: "right" });
  await expect(stage).not.toHaveClass(/graph-visa-focused/);
  await client.click({ button: "right" });
  await page.locator(".topology-graph").dispatchEvent("contextmenu", { bubbles: true });
  await expect(stage).not.toHaveClass(/graph-visa-focused/);
  api.snapshot.active_visas[0].path = null;
  api.snapshot.active_visas = api.snapshot.active_visas.slice(0, 1);
  await page.locator("#refresh-now").click();
  await client.click({ button: "right" });
  await expect(page.locator(".graph-visa-focus-status")).toContainText("Ordered route unavailable");
  await expect(page.locator(".graph-edge.visa-focus")).toHaveCount(0);
  api.snapshot.active_visas = null;
  await page.locator("#refresh-now").click();
  await expect(page.locator(".graph-visa-focus-status")).toContainText("inventory unavailable");
  api.snapshot.active_visas = [{
    id: 4, expires: Date.now() / 1000 + 3600, source_addr: "fd00::2", dest_addr: "fd00::3",
    dest_port: 443, proto: "TCP", path: null,
  }];
  await page.locator("#refresh-now").click();
  await page.locator('.graph-vertex[data-inspect-actor="server"]').click({ button: "right" });
  await expect(page.locator(".graph-visa-focus-status")).toContainText("server: 1 active outbound visas");
  await expect(page.locator(".graph-visa-focus-status")).not.toContainText("unavailable");
  await expect(page.locator(".graph-edge.visa-focus")).toHaveCount(2);
  await expect(page.locator(".graph-service-badge.visa-focus")).toHaveAttribute("data-inspect-service", "Expired");
  api.snapshot.active_visas[0].expires = Date.now() / 1000 - 1;
  await page.locator("#refresh-now").click();
  await expect(page.locator(".graph-visa-focus-status")).toContainText("0 active outbound visas");
  await expect(page.locator(".graph-edge.visa-focus")).toHaveCount(0);
  api.snapshot.active_visas = [{
    id: 5, expires: Date.now() / 1000 + 3600, source_addr: "fd00::1", dest_addr: "fd00::2",
    dest_port: 443, proto: "TCP", path: ["fd00::a", "fd00::b", "fd00::c"],
  }];
  await page.locator("#refresh-now").click();
  const serviceBadge = page.locator('.graph-service-badge[data-inspect-service="Allowed"]');
  await serviceBadge.click({ button: "right" });
  await expect(stage).toHaveClass(/graph-visa-focused/);
  await expect(page.locator(".graph-visa-focus-status")).toContainText("Allowed: 1 active visas serve this service");
  await expect(serviceBadge).toHaveClass(/visa-focus/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="client"]')).toHaveClass(/visa-focus/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="server"]')).toHaveClass(/visa-focus/);
  await expect(page.locator(".graph-edge.visa-focus")).toHaveCount(4);
  await serviceBadge.click({ button: "right" });
  await expect(stage).not.toHaveClass(/graph-visa-focused/);
  await expect(serviceBadge).not.toHaveClass(/visa-focus/);
});

test("Forward and reverse visas are grouped as pairs on the Visas page and Map visa list", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::a", node_details: { adapters: ["client", "server"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
    { cn: "server", node: false, zpr_addr: "fd00::2" },
  ];
  api.snapshot.services = [];
  const expires = Date.now() / 1000 + 3600;
  const visas = [
    { id: 7, created: 100, expires, source_addr: "fd00::1", dest_addr: "fd00::2", source_port: 0, dest_port: 443, proto: "TCP", direction: "forward", policy_id: "p1" },
    { id: 12, created: 101, expires, source_addr: "fd00::2", dest_addr: "fd00::1", source_port: 443, dest_port: 0, proto: "TCP", direction: "reverse", policy_id: "p2" },
    { id: 9, created: 102, expires, source_addr: "fd00::1", dest_addr: "fd00::2", source_port: 0, dest_port: 22, proto: "TCP", direction: "forward" },
    { id: 10, created: 103, expires, source_addr: "fd00::2", dest_addr: "fd00::1", source_port: 80, dest_port: 0, proto: "TCP", direction: "reverse" },
  ];
  api.snapshot.recent_visas = visas;
  api.snapshot.active_visas = visas;
  await page.goto(appURL + "/#visas");
  await page.locator("#pause-poll").click();
  const rows = page.locator("#visa-rows tr");
  await expect(rows).toHaveCount(4);
  const partner = page.locator('#visa-rows tr[data-visa-id="7"] + tr');
  await expect(partner).toHaveAttribute("data-visa-id", "12");
  await expect(partner).toHaveClass(/visa-pair-partner/);
  await expect(page.locator('#visa-rows tr[data-visa-id="7"] .visa-pair-cell')).toHaveText("↔ #12");
  await expect(partner.locator(".visa-pair-cell")).toHaveText("↔ #7");
  await expect(page.locator('#visa-rows tr[data-visa-id="9"] .visa-pair-cell')).toHaveText("No reverse visa");
  await expect(page.locator('#visa-rows tr[data-visa-id="10"] .visa-pair-cell')).toHaveText("No forward visa");
  await page.locator("#visas-filter").fill("12");
  await expect(rows).toHaveCount(2);

  await page.goto(appURL + "/#map");
  await page.locator("#pause-poll").click();
  await page.locator('.graph-vertex[data-inspect-actor="client"] .graph-visa-count').click();
  const inspector = page.locator("#inspector-body");
  await expect(inspector.locator(".visa-pair")).toHaveCount(3);
  await expect(inspector.locator(".visa-pair.paired")).toHaveCount(1);
  await expect(inspector.locator(".visa-pair.paired")).toContainText("Connection · Visa 7 ↔ 12");
  await expect(inspector.locator(".visa-pair.paired .detail-item")).toHaveCount(2);
  await expect(inspector).toContainText("Visa 9 · no reverse visa");
  await expect(inspector).toContainText("Visa 10 · no forward visa");
});

test("GUI Map legend items highlight their component type and right-click matches count badges", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "n1", node: true, zpr_addr: "fd00::a", node_details: { adapters: ["client"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
  ];
  api.snapshot.services = [{ service_name: "API", actor_cn: "client", zpr_addr: "fd00::1", service_endpoints: "TCP/443" }];
  api.snapshot.active_visas = [];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const legendNode = page.locator('[data-legend-kind="node"]');
  await legendNode.click();
  await expect(legendNode).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator('.graph-vertex[data-inspect-actor="n1"]')).toHaveClass(/highlighted/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="client"]')).not.toHaveClass(/highlighted/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="client"]')).toHaveCSS("opacity", "1");
  await expect(page.locator('.graph-service-badge[data-inspect-service="API"]')).toHaveCSS("opacity", "1");
  await expect(page.locator(".filtered")).toHaveCount(0);
  await page.locator('[data-legend-kind="service"]').click();
  await expect(legendNode).toHaveAttribute("aria-pressed", "false");
  await expect(page.locator('.graph-service-badge[data-inspect-service="API"]')).toHaveClass(/highlighted/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="n1"]')).not.toHaveClass(/highlighted/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="n1"]')).toHaveCSS("opacity", "1");
  await page.locator('[data-legend-kind="service"]').click();
  await expect(page.locator(".graph-vertex.highlighted, .graph-service-badge.highlighted")).toHaveCount(0);

  const client = page.locator('.graph-vertex[data-inspect-actor="client"]');
  await client.locator(".graph-adapter, circle, rect").first().click({ button: "right", force: true });
  await expect(page.locator("#topology-stage")).toHaveClass(/graph-visa-focused/);
  await expect(page.locator("#component-inspector")).not.toHaveClass(/open/);
  await client.locator(".graph-visa-count").dispatchEvent("contextmenu", { bubbles: true });
  await expect(page.locator("#topology-stage")).not.toHaveClass(/graph-visa-focused/);
  await client.locator(".graph-visa-count").dispatchEvent("contextmenu", { bubbles: true });
  await expect(page.locator("#topology-stage")).toHaveClass(/graph-visa-focused/);
  await expect(page.locator('.graph-vertex[data-inspect-actor="client"]')).toHaveClass(/visa-focus/);
  await expect(page.locator('.graph-service-badge[data-inspect-service="API"]')).toHaveCSS("opacity", "1");
  await expect(page.locator("#component-inspector")).not.toHaveClass(/open/);
  await client.locator(".graph-visa-count").click();
  await expect(page.locator("#component-inspector")).toHaveClass(/open/);
  await page.locator("#inspector-close").click();
  await expect(page.locator("#topology-stage")).toHaveClass(/graph-visa-focused/);
});

test("GUI Map Clear highlight and Esc remove highlighting; the info panel is independent", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "n1", node: true, zpr_addr: "fd00::a", node_details: { adapters: ["client"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
  ];
  api.snapshot.services = [{ service_name: "API", actor_cn: "client", zpr_addr: "fd00::1", service_endpoints: "TCP/443" }];
  api.snapshot.active_visas = [];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const clear = page.getByRole("button", { name: "Clear highlight", exact: true });
  const stage = page.locator("#topology-stage");
  const highlighted = page.locator(".graph-vertex.highlighted, .graph-service-badge.highlighted, .graph-edge.highlighted, .graph-service-edge.highlighted");
  await expect(page.locator('.graph-vertex[data-inspect-actor="n1"]')).toBeVisible();
  await expect(clear).toBeHidden();

  await page.locator('[data-legend-kind="node"]').click();
  await expect(highlighted).not.toHaveCount(0);
  await clear.click();
  await expect(highlighted).toHaveCount(0);
  await expect(page.locator('[data-legend-kind][aria-pressed="true"]')).toHaveCount(0);
  await expect(clear).toBeHidden();

  await page.locator("#topology-search").fill("API");
  await expect(clear).toBeVisible();
  await page.locator('[data-legend-kind="adapter"]').click();
  await page.keyboard.press("Escape");
  await page.locator("body").press("Escape");
  await expect(page.locator("#topology-search")).toHaveValue("");
  await expect(highlighted).toHaveCount(0);
  await expect(clear).toBeHidden();

  const client = page.locator('.graph-vertex[data-inspect-actor="client"]');
  await client.locator(".graph-visa-count").dispatchEvent("contextmenu", { bubbles: true });
  await expect(stage).toHaveClass(/graph-visa-focused/);
  await expect(clear).toBeVisible();
  await expect(page.locator("#component-inspector")).not.toHaveClass(/open/);
  await client.locator(".graph-visa-count").click();
  await expect(page.locator("#component-inspector")).toHaveClass(/open/);
  await page.locator("#inspector-close").click();
  await expect(stage).toHaveClass(/graph-visa-focused/);
  await client.locator(".graph-visa-count").click();
  await page.keyboard.press("Escape");
  await expect(page.locator("#component-inspector")).not.toHaveClass(/open/);
  await expect(stage).toHaveClass(/graph-visa-focused/);
  await page.locator("body").press("Escape");
  await expect(stage).not.toHaveClass(/graph-visa-focused/);
  await expect(clear).toBeHidden();

  await client.locator(".graph-visa-count").dispatchEvent("contextmenu", { bubbles: true });
  await page.locator('[data-legend-kind="service"]').click();
  await clear.click();
  await expect(stage).not.toHaveClass(/graph-visa-focused/);
  await expect(page.locator("#component-inspector")).not.toHaveClass(/open/);
  await expect(highlighted).toHaveCount(0);
});

test("GUI Provisioning Adapters reports the Control Room enrollment gate without Simulator", async ({ page, appURL, api }) => {
  const requested = [];
  page.on("request", (request) => requested.push(new URL(request.url()).pathname));
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ status: 403, json: { error: "Control Room enrollment requires named-user authorization; use the certificate-authorized administration API directly." } }));
  await page.goto(appURL + "/#map");
  await page.getByRole("group", { name: "Provisioning" }).getByRole("link", { name: "Adapters", exact: true }).click();
  await expect(page.locator("#page-provisioning-adapters")).toBeVisible();
  const status = page.locator("#provisioning-status");
  await expect(status).toHaveAttribute("data-state", "unavailable");
  await expect(status).toContainText("named-user authorization");
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: { organizations: {}, invitation_lifetime_seconds: 3600, approval_lifetime_seconds: 3600, gui_mutations_enabled: false } }));
  await page.getByRole("button", { name: "Check again", exact: true }).click();
  await expect(status).toHaveAttribute("data-state", "available");
  expect(requested.filter((path) => path.startsWith("/api/simulator"))).toEqual([]);
});

test("GUI provisioning worksheet reviews locally without transmitting or saving asset details", async ({ page, appURL, api }) => {
  const requests = [];
  page.on("request", (request) => requests.push({ path: new URL(request.url()).pathname, body: request.postData(), method: request.method() }));
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ status: 403, json: { error: "Named-user authorization is required." } }));
  await page.goto(appURL + "/#provisioning-adapters");
  const form = page.locator("#provisioning-draft");
  await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
  const values = {
    name: '<img src=x onerror="alert(1)">',
    owner: "Unique private owner",
    organization: "requested-org",
    asset_id: "inventory-private-007",
    type: "workstation",
    profile: "requested-profile",
    recipient: "private-owner@example.org",
  };
  for (const [name, value] of Object.entries(values)) await form.locator(`[name="${name}"]`).fill(value);
  await form.getByRole("button", { name: "Review worksheet" }).click();
  const review = page.getByRole("dialog", { name: "Review invitation worksheet" });
  await expect(review).toBeVisible();
  await expect(review).toContainText("Unsaved and unvalidated");
  for (const value of Object.values(values)) await expect(review).toContainText(value);
  await expect(review.locator("img")).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(review).toHaveCount(0);
  await expect(form.locator('[name="owner"]')).toHaveValue(values.owner);
  await page.goto(appURL + "/#map");
  await page.goto(appURL + "/#provisioning-adapters");
  await expect(form.locator('[name="owner"]')).toHaveValue(values.owner);
  await page.reload();
  await expect(form.locator('[name="owner"]')).toHaveValue("");
  expect(requests.filter((request) => request.path.startsWith("/api/enrollment/") && request.method !== "GET")).toEqual([]);
  expect(requests.some((request) => request.path.startsWith("/api/simulator"))).toBe(false);
  expect(requests.some((request) => request.body?.includes(values.asset_id))).toBe(false);
  expect(await page.evaluate((value) => [JSON.stringify(localStorage), JSON.stringify(sessionStorage)].some((stored) => stored.includes(value)), values.asset_id)).toBe(false);
});

test("GUI operator login remains unavailable on HTTP even when configuration claims enabled", async ({ page, appURL, api }) => {
  let sessionReads = 0;
  await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
  await page.route("**/auth/operator/session", (route) => {
    sessionReads++;
    return route.fulfill({ status: 401 });
  });
  await page.goto(appURL + "/#provisioning-adapters");
  await expect(page.locator("#operator-login-status")).toHaveText("Operator login requires direct HTTPS.");
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeHidden();
  await expect(page.locator(".app-shell")).toHaveCSS("visibility", "hidden");
  expect(sessionReads).toBe(0);
  await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeHidden();
});

test.describe("GUI operator HTTPS login", () => {
  test.use({ ignoreHTTPSErrors: true });

  for (const path of ["/#gateways", "/?view=operator#gateways", "/scenarios.html"]) {
    test(`operator re-login restores the original application location ${path}`, async ({ page, secureAppURL: appURL }) => {
      let signedIn = false;
      await page.route("**/auth/operator/config", route => route.fulfill({ json: { enabled: true } }));
      await page.route("**/auth/operator/session", route => route.fulfill(signedIn ? { json: {
        identity: { issuer: "https://identity.example", subject: "operator", organizations: ["*"], permissions: ["read"] },
        csrf: "return-location-proof",
      } } : { status: 401 }));
      await page.addInitScript(() => document.addEventListener("submit", event => {
        if (event.target.id === "operator-login-form") event.preventDefault();
      }));
      await page.goto(appURL + path);
      await expect.poll(() => page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBe(path);
      await page.goto(`${appURL}/?operator_login=failed`);
      await expect(page.locator("#operator-login-status")).toContainText("Sign-in failed");
      await page.getByRole("button", { name: "Sign in", exact: true }).click();
      expect(await page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBe(path);
      signedIn = true;
      await page.goto(appURL + "/");
      await expect(page).toHaveURL(appURL + path);
      await expect(page.locator("#operator-login-status")).toHaveText("Signed in");
      expect(await page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBeNull();
    });
  }

  for (const target of ["https://outside.example/#gateways", "//outside.example/", "/auth/operator/login", "/\\outside.example/"]) {
    test(`operator login refuses unsafe return location ${target}`, async ({ page, secureAppURL: appURL }) => {
      await page.addInitScript(value => sessionStorage.setItem("zpr.operator-login.return-location", value), target);
      await page.route("**/auth/operator/config", route => route.fulfill({ json: { enabled: true } }));
      await page.route("**/auth/operator/session", route => route.fulfill({ json: {
        identity: { issuer: "https://identity.example", subject: "operator", organizations: ["*"], permissions: ["read"] },
        csrf: "return-location-proof",
      } }));
      await page.goto(appURL + "/#map");
      await expect(page.locator("#operator-login-status")).toHaveText("Signed in");
      await expect(page).toHaveURL(appURL + "/#map");
      expect(await page.evaluate(() => sessionStorage.getItem("zpr.operator-login.return-location"))).toBeNull();
    });
  }

  test("failed sign-in stays reachable and scrollable on short viewports", async ({ page, secureAppURL: appURL }) => {
    await page.setViewportSize({ width: 390, height: 160 });
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill({ status: 401 }));
    await page.goto(`${appURL}/?operator_login=failed#map`);
    const dialog = page.locator(".operator-login");
    await expect(dialog).toHaveAttribute("role", "dialog");
    await expect(dialog).toHaveAttribute("aria-modal", "true");
    const bounds = await dialog.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      return { top: rect.top, bottom: rect.bottom, viewportHeight: window.innerHeight, height: element.clientHeight, scrollHeight: element.scrollHeight, position: style.position, maxHeight: style.maxHeight, insetBlock: `${style.top}/${style.bottom}`, overflowY: style.overflowY };
    });
    expect(bounds.top).toBeGreaterThanOrEqual(0);
    expect(bounds.bottom, JSON.stringify(bounds)).toBeLessThanOrEqual(bounds.viewportHeight);
    expect(bounds.scrollHeight).toBeGreaterThan(bounds.height);
  });

  test("OIDC callback failures return to a retryable sign-in message", async ({ page, secureAppURL: appURL }) => {
    let submissions = 0;
    await page.route("**/auth/operator/login", (route) => {
      submissions++;
      return route.fulfill({ status: 204 });
    });
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill({ status: 401 }));
    await page.goto(`${appURL}/?operator_login=failed#map`);
    await expect(page.locator("#operator-login-status")).toHaveText("Sign-in failed. Check your credentials and try again.");
    await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
    await expect(page).not.toHaveURL(/operator_login=/);
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.locator("#operator-login-status")).toHaveText("Sign-in failed. Check your credentials and try again.");
    await page.goto(`${appURL}/?operator_login=denied#map`);
    await expect(page.locator("#operator-login-status")).toHaveText("This account is not authorized. Contact your operator administrator.");
    await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.locator("#operator-login-status")).toHaveText("This account is not authorized. Contact your operator administrator.");
    expect(submissions).toBe(0);
  });

  for (const path of ["/#map", "/organizations.html"]) {
    test(`automatic operator login starts once and does not loop on focus at ${path}`, async ({ page, secureAppURL: appURL, api }) => {
      await page.addInitScript(() => {
        window.loginSubmissions = 0;
        document.addEventListener("submit", (event) => {
          if (event.target.id !== "operator-login-form") return;
          event.preventDefault();
          window.loginSubmissions++;
        });
      });
      await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
      await page.route("**/auth/operator/session", (route) => route.fulfill({ status: 401 }));
      await page.goto(appURL + path);
      await expect.poll(() => page.evaluate(() => window.loginSubmissions)).toBe(1);
      const checked = page.waitForResponse("**/auth/operator/session");
      await page.evaluate(() => window.dispatchEvent(new Event("focus")));
      await checked;
      await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
      expect(await page.evaluate(() => window.loginSubmissions)).toBe(1);
      await expect(page.locator(".app-shell")).toHaveCSS("visibility", "hidden");
    });
  }

  test("explicit logout stays signed out across focus without automatic SSO reentry", async ({ page, secureAppURL: appURL, api }) => {
    let signedIn = true;
    let attempts = 0;
    let submissions = 0;
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill(signedIn ? { json: {
      identity: { issuer: "https://identity.example", subject: "admin", display_name: "Admin Operator", organizations: ["*"], permissions: ["read"] },
      csrf: "logout-proof",
    } } : { status: 401 }));
    await page.route("**/auth/operator/login", (route) => {
      submissions++;
      return route.fulfill({ status: 204 });
    });
    await page.route("**/auth/operator/logout", (route) => {
      expect(route.request().headers()["x-zpr-csrf"]).toBe("logout-proof");
      if (++attempts === 1) return route.fulfill({ status: 503 });
      signedIn = false;
      return route.fulfill({ status: 204 });
    });
    await page.goto(appURL + "/#map");
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Admin Operator");
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toContainText("Sign out was not confirmed");
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
    const checked = page.waitForResponse("**/auth/operator/session");
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await checked;
    await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
    await expect(page.locator("#operator-scope")).toBeHidden();
    await expect(page.locator(".app-shell")).toHaveCSS("visibility", "hidden");
    await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
    expect(submissions).toBe(0);
  });

  test("login service recovery needs explicit sign-in after an error", async ({ page, secureAppURL: appURL, api }) => {
    let available = false;
    let submissions = 0;
    await page.route("**/auth/operator/config", (route) => route.fulfill(available ? { json: { enabled: true } } : { status: 503 }));
    await page.route("**/auth/operator/session", (route) => route.fulfill({ status: 401 }));
    await page.route("**/auth/operator/login", (route) => {
      submissions++;
      return route.fulfill({ status: 204 });
    });
    await page.goto(appURL + "/#map");
    await expect(page.locator("#operator-login-status")).toHaveText("Login configuration unavailable (HTTP 503).");
    available = true;
    await page.getByRole("button", { name: "Retry", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
    expect(submissions).toBe(0);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect.poll(() => submissions).toBe(1);
  });

  test("operator request helper attaches CSRF only to same-origin mutations and erases it on session loss", async ({ page, secureAppURL: appURL, api }) => {
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill({ json: {
      identity: { issuer: "https://id.example", subject: "admin", display_name: "Admin Operator", organizations: ["*"], permissions: ["read", "policy.analyze"] },
      csrf: "test-csrf",
    } }));
    const observed = [];
    api.handlers.set("/api/policy/config/check", (route) => {
      observed.push(route.request().headers()["x-zpr-csrf"] ?? "");
      return route.fulfill({ json: { valid: true } });
    });
    await page.goto(appURL + "/#map");
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Admin Operator");
    await page.evaluate(() => window.zprOperatorFetch("/api/policy/config/check", { method: "POST", body: "{}" }));
    expect(observed).toEqual(["test-csrf"]);
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    await page.evaluate(() => window.zprOperatorFetch("/api/policy/config/check", { method: "POST", body: "{}" }));
    expect(observed).toEqual(["test-csrf", ""]);
    const externalCSRF = await page.evaluate(async () => {
      window.dispatchEvent(new CustomEvent("operator-session-ready", { detail: { csrf: "test-csrf" } }));
      const original = window.fetch;
      let header = "";
      window.fetch = (path, options) => {
        if (path === "https://external.example/api/check") {
          header = new Headers(options.headers).get("X-ZPR-CSRF") ?? "";
          return Promise.resolve(new Response("ok"));
        }
        return original(path, options);
      };
      try {
        await window.zprOperatorFetch("https://external.example/api/check", { method: "POST" });
        return header;
      } finally {
        window.fetch = original;
      }
    });
    expect(externalCSRF).toBe("");
  });

  test("protected API rejection starts sign-in only when the operator session has expired", async ({ page, secureAppURL: appURL, api }) => {
    let expired = false;
    let sessionChecks = 0;
    await page.addInitScript(() => {
      window.loginSubmissions = 0;
      document.addEventListener("submit", (event) => {
        if (event.target.id !== "operator-login-form") return;
        event.preventDefault();
        window.loginSubmissions++;
      });
    });
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => {
      sessionChecks++;
      return route.fulfill(expired ? { status: 401 } : { json: {
        identity: { issuer: "https://id.example", subject: "admin", display_name: "Admin Operator", organizations: ["*"], permissions: ["read"] },
        csrf: "test-csrf",
      } });
    });
    api.handlers.set("/api/operator-test/expired", (route) => route.fulfill({ status: 403 }));
    api.handlers.set("/api/operator-test/permission-denied", (route) => route.fulfill({ status: 403 }));
    api.handlers.set("/api/operator-test/unavailable", (route) => route.fulfill({ status: 503 }));

    await page.goto(appURL + "/#map");
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Admin Operator");
    expired = true;
    const expiredStatus = await page.evaluate(async () =>
      (await window.zprOperatorFetch("/api/operator-test/expired")).status);
    expect(expiredStatus).toBe(403);
    await expect.poll(() => page.evaluate(() => window.loginSubmissions)).toBe(1);
    await expect(page.locator("#operator-login-status")).toHaveText("Opening sign-in page");
    expect(sessionChecks).toBe(2);
  });

  test("permission denial and Visa Service outage do not trigger automatic sign-in", async ({ page, secureAppURL: appURL, api }) => {
    let sessionChecks = 0;
    await page.addInitScript(() => {
      window.loginSubmissions = 0;
      document.addEventListener("submit", (event) => {
        if (event.target.id !== "operator-login-form") return;
        event.preventDefault();
        window.loginSubmissions++;
      });
    });
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => {
      sessionChecks++;
      return route.fulfill({ json: {
        identity: { issuer: "https://id.example", subject: "admin", display_name: "Admin Operator", organizations: ["*"], permissions: ["read"] },
        csrf: "test-csrf",
      } });
    });
    api.handlers.set("/api/operator-test/permission-denied", (route) => route.fulfill({ status: 403 }));
    api.handlers.set("/api/operator-test/unavailable", (route) => route.fulfill({ status: 503 }));

    await page.goto(appURL + "/#map");
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Admin Operator");
    await expect.poll(() => sessionChecks).toBe(1);
    expect(await page.evaluate(async () =>
      (await window.zprOperatorFetch("/api/operator-test/permission-denied")).status)).toBe(403);
    await expect.poll(() => sessionChecks).toBe(2);
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Admin Operator");
    expect(await page.evaluate(async () =>
      (await window.zprOperatorFetch("/api/operator-test/unavailable")).status)).toBe(503);
    await page.waitForTimeout(100);
    expect(sessionChecks).toBe(2);
    expect(await page.evaluate(() => window.loginSubmissions)).toBe(0);
  });

  test("opens sign-in directly with a same-origin native POST without asset details", async ({ page, secureAppURL: appURL, api }) => {
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill({ status: 401 }));
    let observed;
    await page.route("**/auth/operator/login", (route) => {
      observed = route.request();
      return route.fulfill({ status: 403, body: "Test-only provider unavailable" });
    });
    await page.goto(appURL + "/#provisioning-adapters");
    await expect.poll(() => observed?.method()).toBe("POST");
    expect(observed.headers().origin).toBe(appURL);
    expect(observed.postData() || "").toBe("");
    expect(observed.url()).toBe(appURL + "/auth/operator/login");
  });

  test("shows verified session scope but stays locked and confirms CSRF logout", async ({ page, secureAppURL: appURL, api }) => {
    let signedIn = true;
    let logoutAttempts = 0;
    let observedCSRF = "";
    const subject = '<img src=x onerror="alert(1)">';
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill(signedIn ? { json: {
      identity: { issuer: "https://identity.example", subject, display_name: "Finance Operator", email: "operator@example.test", organizations: ["production"], permissions: ["read", "create"] }, csrf: "private-csrf-proof",
    } } : { status: 401 }));
    await page.route("**/auth/operator/logout", (route) => {
      logoutAttempts++;
      observedCSRF = route.request().headers()["x-zpr-csrf"];
      if (logoutAttempts === 1) return route.fulfill({ status: 503 });
      signedIn = false;
      return route.fulfill({ status: 204 });
    });
    await page.goto(appURL + "/#provisioning-adapters");
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Finance Operator");
    await expect(page.locator("#operator-scope")).toContainText("operator@example.test");
    await expect(page.locator("#operator-scope")).not.toContainText(subject);
    await expect(page.locator(".provisioning-notice")).toContainText("controls are not connected and remain locked");
    await expect(page.locator(".operator-login img")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toContainText("Sign out was not confirmed");
    await expect(page.getByRole("button", { name: "Sign out", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page.locator("#operator-login-status")).toHaveText("Not signed in");
    expect(observedCSRF).toBe("private-csrf-proof");
    await expect(page.locator("#operator-scope")).toBeHidden();
    expect(await page.evaluate(() => [JSON.stringify(localStorage), JSON.stringify(sessionStorage)].some((value) => value.includes("private-csrf-proof")))).toBe(false);
  });

  test("clears expired sessions on focus and rejects malformed identity responses", async ({ page, secureAppURL: appURL, api }) => {
    let state = "valid";
    await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
    await page.route("**/auth/operator/session", (route) => route.fulfill(state === "expired" ? { status: 401 } : { json: state === "invalid" ?
      { identity: { subject: "unverified" }, csrf: "bad" } :
      { identity: { issuer: "https://identity.example", subject: "admin", display_name: "Admin Operator", organizations: ["production"], permissions: ["read"] }, csrf: "proof" },
    }));
    await page.goto(appURL + "/#provisioning-adapters");
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Admin Operator");
    await page.evaluate(() => {
      window.loginSubmissions = 0;
      document.getElementById("operator-login-form").addEventListener("submit", (event) => {
        event.preventDefault();
        window.loginSubmissions++;
      });
    });
    state = "expired";
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect.poll(() => page.evaluate(() => window.loginSubmissions)).toBe(1);
    await expect(page.locator("#operator-scope")).toBeHidden();
    state = "invalid";
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.locator("#operator-login-status")).toHaveText("Invalid operator session response.");
    await expect(page.getByRole("button", { name: "Sign out", exact: true })).toBeHidden();
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeHidden();
  });
});

test("GUI provisioning worksheet rejects blank values and clears validation on reset", async ({ page, appURL, api }) => {
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ status: 403, json: { error: "Named-user authorization is required." } }));
  await page.goto(appURL + "/#provisioning-adapters");
  const form = page.locator("#provisioning-draft");
  const values = { name: "   ", owner: "Owner", organization: "org", asset_id: "asset", type: "type", profile: "profile", recipient: "owner@example.org" };
  for (const [name, value] of Object.entries(values)) await form.locator(`[name="${name}"]`).fill(value);
  await form.getByRole("button", { name: "Review worksheet" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await form.locator('[name="name"]').evaluate((input) => input.validationMessage)).toBe("Enter a value, not just spaces.");
  await form.getByRole("button", { name: "Clear worksheet" }).click();
  await expect(page.locator("#provisioning-draft-status")).toHaveText("Worksheet cleared. Nothing has been submitted.");
  for (const name of Object.keys(values)) await expect(form.locator(`[name="${name}"]`)).toHaveValue("");
  for (const [name, value] of Object.entries({ ...values, name: "  Laptop 07  " })) await form.locator(`[name="${name}"]`).fill(value);
  await form.getByRole("button", { name: "Review worksheet" }).click();
  await expect(page.getByRole("dialog", { name: "Review invitation worksheet" })).toContainText("Laptop 07");
  await page.getByRole("dialog").getByRole("button", { name: "Close", exact: true }).click();
  await expect(form.locator('[name="name"]')).toHaveValue("Laptop 07");
});

test("GUI provisioning never unlocks on an endpoint response and explains remote enrollment", async ({ page, appURL, api }) => {
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: { organizations: {}, invitation_lifetime_seconds: 3600, approval_lifetime_seconds: 3600, gui_mutations_enabled: false } }));
  await page.goto(appURL + "/#provisioning-adapters");
  await expect(page.locator("#provisioning-status")).toContainText("remain locked");
  await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
  await expect(page.getByRole("region", { name: "Enrollment requests" })).toContainText("not an empty queue");
  await page.getByRole("button", { name: "Help for this page", exact: true }).click();
  const help = page.getByRole("dialog", { name: "Adapter provisioning" });
  await expect(help).toContainText("remote and offline");
  await expect(help).toContainText("separately delivered code");
  await expect(help).toContainText("development package is unsigned");
  await help.getByRole("button", { name: "Close", exact: true }).click();
  await expect(help).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

function provisioningCatalog() {
  return { organizations: { alpha: { types: ["laptop"], profiles: ["standard"] }, beta: { types: ["server"], profiles: ["restricted"] } },
    invitation_lifetime_seconds: 3600, approval_lifetime_seconds: 1800, gui_mutations_enabled: false };
}

function provisioningInvitation(id, organization = "alpha") {
  return { id, asset: { organization, asset_id: `INV-${id}`, name: `Laptop ${id}`, owner: "Owner", type: "laptop", profile: "standard", recipient: "owner@example.org" },
    state: "pending_approval", revision: 2, created_by: 'oidc:["https://id.example","admin"]',
    created_at: "2026-10-07T12:00:00Z", expires_at: "2026-10-07T13:00:00Z",
    claimed_at: "2026-10-07T12:10:00Z", approval_expires_at: "2026-10-07T12:40:00Z", key_fingerprint: "b".repeat(64) };
}

async function mockCancellation(page, api, options = {}) {
  const item = { ...provisioningInvitation("cancel-id"), ...options.item };
  const identity = { issuer: "https://identity.example", subject: "canceller", organizations: ["alpha"], permissions: options.permissions ?? ["read", "cancel"] };
  await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
  await page.route("**/auth/operator/session", (route) => route.fulfill({ json: { identity, csrf: "cancel-csrf" } }));
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: {
    ...provisioningCatalog(), gui_cancel_organizations: options.capability ?? ["alpha"],
  } }));
  api.handlers.set("/api/enrollment/v1/invitations", (route) => route.fulfill({ json: { invitations: [item] } }));
  api.handlers.set("/api/enrollment/v1/invitations/cancel-id", (route) => route.fulfill({ json: item }));
  return item;
}

async function openCancellation(page) {
  await page.getByRole("button", { name: "Details for Laptop cancel-id", exact: true }).click();
  const detail = page.getByRole("dialog", { name: "Enrollment request details" });
  await expect(detail).toContainText("cancel-id");
  return detail;
}

async function confirmCancellation(detail, reason = "Wrong recipient") {
  await detail.getByLabel("Cancellation reason", { exact: true }).fill(reason);
  await detail.getByRole("checkbox").check();
  await detail.getByRole("button", { name: "Confirm cancellation", exact: true }).click();
}

test.describe("GUI provisioning cancellation", () => {
  test.use({ ignoreHTTPSErrors: true });

  for (const [name, options] of Object.entries({
    "missing backend capability": { capability: [] },
    "missing named permission": { permissions: ["read", "create"] },
    "approved record": { item: { state: "approved", revision: 3 } },
    "expired record": { item: { state: "approval_expired" } },
    "malformed backend capability": { capability: ["outside-organization"] },
  })) {
    test(`does not offer cancellation for ${name}`, async ({ page, secureAppURL: appURL, api }) => {
      await mockCancellation(page, api, options);
      await page.goto(appURL + "/#provisioning-adapters");
      if (name === "malformed backend capability") {
        await expect(page.locator("#provisioning-status")).toContainText("Invalid invitation cancellation capabilities");
      } else {
        const detail = await openCancellation(page);
        await expect(detail.getByRole("button", { name: "Confirm cancellation", exact: true })).toHaveCount(0);
      }
    });
  }

  test("requires reason and acknowledgement and sends one revision/key-bound CSRF request", async ({ page, secureAppURL: appURL, api }) => {
    const item = await mockCancellation(page, api);
    const posts = [];
    api.handlers.set("/api/enrollment/v1/invitations/cancel-id/cancel", (route) => {
      posts.push(route.request());
      const body = route.request().postDataJSON();
      Object.assign(item, { state: "cancelled", revision: 3, decision_by: 'oidc:["https://identity.example","canceller"]',
        decision_reason: body.reason, decided_at: new Date().toISOString() });
      return route.fulfill({ json: item });
    });
    await page.goto(appURL + "/#provisioning-adapters");
    const detail = await openCancellation(page);
    await detail.getByRole("button", { name: "Confirm cancellation", exact: true }).click();
    expect(posts).toHaveLength(0);
    await detail.getByLabel("Cancellation reason").fill("é".repeat(129));
    await detail.getByRole("checkbox").check();
    await detail.getByRole("button", { name: "Confirm cancellation", exact: true }).click();
    await expect(detail.getByLabel("Cancellation reason")).toHaveJSProperty("validationMessage", "Enter a reason of 1 to 256 UTF-8 bytes, without line breaks.");
    expect(posts).toHaveLength(0);
    await confirmCancellation(detail, '<img src=x onerror="alert(1)">');
    await expect(page.locator("#provisioning-cancel-status")).toContainText("cancelled at revision 3");
    await expect(page.locator("#provisioning-invitations")).toContainText("cancelled");
    expect(posts).toHaveLength(1);
    expect(posts[0].headers()["x-zpr-csrf"]).toBe("cancel-csrf");
    expect(posts[0].postDataJSON()).toEqual({ organization: "alpha", revision: 2, key_fingerprint: "b".repeat(64), reason: '<img src=x onerror="alert(1)">' });
    const refreshed = await openCancellation(page);
    await expect(refreshed.locator("img")).toHaveCount(0);
    await expect(refreshed).toContainText("canceller");
    await expect(refreshed.getByRole("button", { name: "Confirm cancellation", exact: true })).toHaveCount(0);
  });

  for (const status of [400, 401, 403, 404, 409, 413, 415]) {
    test(`explicit HTTP ${status} rejection requires fresh review without retry`, async ({ page, secureAppURL: appURL, api }) => {
      await mockCancellation(page, api);
      let posts = 0;
      api.handlers.set("/api/enrollment/v1/invitations/cancel-id/cancel", (route) => {
        posts++;
        return route.fulfill({ status, json: { error: "Request rejected" } });
      });
      await page.goto(appURL + "/#provisioning-adapters");
      const detail = await openCancellation(page);
      await confirmCancellation(detail);
      await expect(page.locator("#provisioning-cancel-status")).toContainText(`Cancellation rejected (HTTP ${status})`);
      await expect(page.locator("#provisioning-cancel-status")).not.toContainText("uncertain");
      if (![401, 403].includes(status)) await expect(detail.getByRole("button", { name: "Confirm cancellation" })).toBeDisabled();
      expect(posts).toBe(1);
    });
  }

  for (const failure of ["lost response", "malformed result", "wrong key", "wrong principal", "wrong reason", "HTTP 500", "timeout", "navigation", "session loss", "close"]) {
    test(`${failure} keeps cancellation uncertain across refresh and never retries automatically`, async ({ page, secureAppURL: appURL, api }) => {
      test.setTimeout(45000);
      const item = await mockCancellation(page, api);
      let posts = 0;
      api.handlers.set("/api/enrollment/v1/invitations/cancel-id/cancel", async (route) => {
        posts++;
        if (failure === "lost response") return route.abort("failed");
        if (failure === "HTTP 500") return route.fulfill({ status: 500, json: { error: "Unavailable" } });
        if (["timeout", "navigation", "session loss", "close"].includes(failure)) {
          await new Promise((resolve) => setTimeout(resolve, failure === "timeout" ? 30000 : 700));
          return route.fulfill({ status: 500, json: { error: "Unconfirmed" } });
        }
        const changed = { ...item, state: "cancelled", revision: 3, decision_by: 'oidc:["https://identity.example","canceller"]',
          decision_reason: "Wrong recipient", decided_at: new Date().toISOString() };
        if (failure === "malformed result") changed.id = "wrong-id";
        if (failure === "wrong key") changed.key_fingerprint = "c".repeat(64);
        if (failure === "wrong principal") changed.decision_by = "shared-certificate";
        if (failure === "wrong reason") changed.decision_reason = "wrong";
        return route.fulfill({ json: changed });
      });
      await page.goto(appURL + "/#provisioning-adapters");
      const detail = await openCancellation(page);
      const started = Date.now();
      await confirmCancellation(detail);
      await expect.poll(() => posts).toBe(1);
      if (failure === "navigation") {
        await page.evaluate(() => { location.hash = "#map"; });
        await page.evaluate(() => { location.hash = "#provisioning-adapters"; });
      }
      if (failure === "session loss") await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
      if (failure === "close") await detail.getByRole("button", { name: "Close", exact: true }).click();
      await expect(page.locator("#provisioning-cancel-status")).toContainText("Cancellation outcome uncertain", { timeout: failure === "timeout" ? 24000 : 5000 });
      if (failure === "timeout") {
        expect(Date.now() - started).toBeGreaterThanOrEqual(19500);
        expect(Date.now() - started).toBeLessThan(25000);
      }
      if (await detail.count()) await detail.getByRole("button", { name: "Close", exact: true }).click();
      await page.getByRole("button", { name: "Check again", exact: true }).click();
      const refreshed = await openCancellation(page);
      await expect(refreshed).toContainText("fresh read after an uncertain cancellation");
      await expect(refreshed.getByRole("button", { name: "Confirm cancellation", exact: true })).toHaveCount(0);
      await expect(refreshed.getByRole("button", { name: "Acknowledge refreshed outcome" })).toBeEnabled();
      await refreshed.getByRole("button", { name: "Acknowledge refreshed outcome" }).click();
      await expect(page.locator("#provisioning-cancel-status")).toContainText("No cancellation was sent");
      expect(posts).toBe(1);
      expect(await page.evaluate(() => [JSON.stringify(localStorage), JSON.stringify(sessionStorage)].some((stored) => stored.includes("cancel-id")))).toBe(false);
    });
  }
});

test("GUI provisioning registry shows approved catalogs, paginated records, and fresh key-bound details", async ({ page, appURL, api }) => {
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: { ...provisioningCatalog(), gui_mutations_enabled: true } }));
  const queries = [];
  api.handlers.set("/api/enrollment/v1/invitations", (route) => {
    const query = new URL(route.request().url()).searchParams;
    queries.push(query.toString());
    return route.fulfill({ json: query.get("organization") === "beta" ? { invitations: [] } :
      query.get("after") ? { invitations: [provisioningInvitation("second")] } :
      { invitations: [provisioningInvitation("first")], next_after: "first" } });
  });
  api.handlers.set("/api/enrollment/v1/invitations/second", (route) => route.fulfill({ json: { ...provisioningInvitation("second"), revision: 3, decision_reason: '<img src=x onerror="alert(1)">' } }));
  await page.goto(appURL + "/#provisioning-adapters");
  await expect(page.locator("#provisioning-approved-catalog")).toContainText("standard");
  await expect(page.locator("#provisioning-invitations")).toContainText("Laptop first");
  await page.getByRole("button", { name: "Next page", exact: true }).click();
  await expect(page.locator("#provisioning-invitations")).toContainText("Laptop second");
  await expect(page.locator("#provisioning-invitations")).not.toContainText("Laptop first");
  await page.getByRole("button", { name: "Details for Laptop second" }).click();
  const detail = page.getByRole("dialog", { name: "Enrollment request details" });
  await expect(detail).toContainText("b".repeat(64));
  await expect(detail).toContainText('<img src=x onerror="alert(1)">');
  await expect(detail.locator("img")).toHaveCount(0);
  await expect(detail.locator("dt").filter({ hasText: /^Revision$/ }).locator("+ dd")).toHaveText("3");
  await page.keyboard.press("Escape");
  await expect(detail).toHaveCount(0);
  await page.locator("#provisioning-organization").selectOption("beta");
  await expect(page.locator("#provisioning-registry-status")).toContainText("no invitations on this registry page");
  await expect(page.locator("#provisioning-approved-catalog")).toContainText("restricted");
  expect(queries).toContain("organization=alpha&limit=50&after=first");
  await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
});

test("GUI provisioning registry rejects malformed and cross-organization pages instead of showing empty", async ({ page, appURL, api }) => {
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: provisioningCatalog() }));
  api.handlers.set("/api/enrollment/v1/invitations", (route) => route.fulfill({ json: { invitations: [provisioningInvitation("foreign", "beta")] } }));
  await page.goto(appURL + "/#provisioning-adapters");
  await expect(page.locator("#provisioning-registry-status")).toContainText("response shape is invalid");
  await expect(page.locator("#provisioning-registry-status")).toContainText("not an empty queue");
  await expect(page.locator("#provisioning-invitations")).toBeHidden();
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: { organizations: [] } }));
  await page.getByRole("button", { name: "Check again", exact: true }).click();
  await expect(page.locator("#provisioning-status")).toContainText("Invalid enrollment catalog");
  await expect(page.locator("#provisioning-organization")).toBeDisabled();
});

test("GUI provisioning registry cancels stale organization reads and clears records on navigation and session loss", async ({ page, appURL, api }) => {
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: provisioningCatalog() }));
  let release;
  api.handlers.set("/api/enrollment/v1/invitations", async (route) => {
    if (new URL(route.request().url()).searchParams.get("organization") === "alpha") {
      await new Promise((resolve) => { release = resolve; });
      await route.fulfill({ json: { invitations: [provisioningInvitation("stale")] } }).catch(() => {});
    } else await route.fulfill({ json: { invitations: [provisioningInvitation("beta-current", "beta")] } });
  });
  await page.goto(appURL + "/#provisioning-adapters");
  await expect.poll(() => Boolean(release)).toBe(true);
  await page.locator("#provisioning-organization").selectOption("beta");
  await expect(page.locator("#provisioning-invitations")).toContainText("beta-current");
  release();
  await expect(page.locator("#provisioning-invitations")).not.toContainText("stale");
  await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
  await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
  await expect(page.locator("#provisioning-approved-catalog")).toBeHidden();
  await expect(page.locator("#provisioning-organization")).toBeDisabled();
  await page.goto(appURL + "/#map");
  await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
});

test("GUI provisioning registry rejects duplicate IDs, broken cursors, and invalid pending key details", async ({ page, appURL, api }) => {
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: provisioningCatalog() }));
  let result = { invitations: [provisioningInvitation("same"), provisioningInvitation("same")] };
  api.handlers.set("/api/enrollment/v1/invitations", (route) => route.fulfill({ json: result }));
  await page.goto(appURL + "/#provisioning-adapters");
  await expect(page.locator("#provisioning-registry-status")).toContainText("response shape is invalid");
  await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
  result = { invitations: [provisioningInvitation("same")], next_after: "unrelated" };
  await page.getByRole("button", { name: "Check again", exact: true }).click();
  await expect(page.locator("#provisioning-registry-status")).toContainText("response shape is invalid");
  await expect(page.locator("#provisioning-next")).toBeHidden();
  result = { invitations: [provisioningInvitation("same")] };
  api.handlers.set("/api/enrollment/v1/invitations/same", (route) => route.fulfill({ json: { ...provisioningInvitation("same"), key_fingerprint: undefined } }));
  await page.getByRole("button", { name: "Check again", exact: true }).click();
  await page.getByRole("button", { name: "Details for Laptop same" }).click();
  const detail = page.getByRole("dialog", { name: "Enrollment request details" });
  await expect(detail).toContainText("response shape is invalid");
  await expect(detail.locator("dl")).toHaveCount(0);
  await page.keyboard.press("Escape");
  api.handlers.set("/api/enrollment/v1/invitations/same", (route) => route.fulfill({ json: provisioningInvitation("different") }));
  await page.getByRole("button", { name: "Details for Laptop same" }).click();
  await expect(detail).toContainText("detail identity or response shape is invalid");
  await expect(detail.locator("dl")).toHaveCount(0);
});

test("GUI provisioning registry discards late details after closing or navigation and clears denied access", async ({ page, appURL, api }) => {
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: provisioningCatalog() }));
  api.handlers.set("/api/enrollment/v1/invitations", (route) => route.fulfill({ json: { invitations: [provisioningInvitation("detail")] } }));
  let release;
  api.handlers.set("/api/enrollment/v1/invitations/detail", async (route) => {
    await new Promise((resolve) => { release = resolve; });
    await route.fulfill({ json: provisioningInvitation("detail") }).catch(() => {});
  });
  await page.goto(appURL + "/#provisioning-adapters");
  await page.getByRole("button", { name: "Details for Laptop detail" }).click();
  await expect.poll(() => Boolean(release)).toBe(true);
  await page.keyboard.press("Escape");
  release();
  await expect(page.getByRole("dialog", { name: "Enrollment request details" })).toHaveCount(0);
  await page.evaluate(() => { location.hash = "#map"; });
  await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
  await expect(page.locator("#provisioning-approved-catalog")).toBeHidden();
  api.handlers.set("/api/enrollment/v1/invitations/detail", (route) => route.fulfill({ status: 403, json: { error: "read grant removed" } }));
  await page.goto(appURL + "/#provisioning-adapters");
  await page.getByRole("button", { name: "Details for Laptop detail" }).click();
  await expect(page.locator("#provisioning-status")).toHaveAttribute("data-state", "unavailable");
  await expect(page.locator("#provisioning-registry-status")).toContainText("read grant removed");
  await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
  await expect(page.locator("#provisioning-organization")).toBeDisabled();
  await expect(page.getByRole("dialog", { name: "Enrollment request details" })).toHaveCount(0);
  api.handlers.set("/api/enrollment/v1/invitations", (route) => route.fulfill({ status: 401, json: { error: "session expired" } }));
  await page.getByRole("button", { name: "Check again", exact: true }).click();
  await expect(page.locator("#provisioning-registry-status")).toContainText("session expired");
  await expect(page.locator("#provisioning-approved-catalog")).toBeHidden();
});

async function mockInvitationCreator(page, api, enabled = true) {
  await page.route("**/auth/operator/config", (route) => route.fulfill({ json: { enabled: true } }));
  await page.route("**/auth/operator/session", (route) => route.fulfill({ json: {
    identity: { issuer: "https://identity.example", subject: "creator", organizations: ["alpha", "beta"], permissions: ["read", "create"] },
    csrf: "creation-csrf-proof",
  } }));
  api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: {
    ...provisioningCatalog(), gui_create_organizations: enabled ? ["alpha"] : [],
  } }));
  api.handlers.set("/api/enrollment/v1/invitations", (route) => route.fulfill({ json: { invitations: [] } }));
}

async function fillApprovedInvitation(page) {
  const form = page.locator("#provisioning-draft");
  await form.locator('[name="organization"]').selectOption("alpha");
  await form.locator('[name="type"]').selectOption("laptop");
  await form.locator('[name="profile"]').selectOption("standard");
  for (const [name, value] of Object.entries({ name: "Created laptop", owner: "Operations", asset_id: "INV-CREATE", recipient: "owner@example.org" })) {
    await form.locator(`[name="${name}"]`).fill(value);
  }
}

async function confirmInvitationCreation(page) {
  await page.getByRole("button", { name: "Create invitation", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Confirm invitation creation" });
  await expect(dialog.getByRole("button", { name: "Confirm creation", exact: true })).toBeDisabled();
  await dialog.getByRole("checkbox").check();
  await dialog.getByRole("button", { name: "Confirm creation", exact: true }).click();
}

function createdInvitationResponse(asset) {
  const now = Date.now();
  return { invitation: { id: "created-id", asset, state: "invited", revision: 1,
    created_at: new Date(now).toISOString(), expires_at: new Date(now + 3600000).toISOString(),
    created_by: 'oidc:["https://identity.example","creator"]' }, enrollment_code: "A".repeat(26) };
}

test.describe("GUI provisioning invitation creation", () => {
  test.use({ ignoreHTTPSErrors: true });
  test("requires backend opt-in and both scopes while selecting approved catalog values", async ({ page, secureAppURL: appURL, api }) => {
    await mockInvitationCreator(page, api, false);
    await page.goto(appURL + "/#provisioning-adapters");
    await fillApprovedInvitation(page);
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    api.handlers.set("/api/enrollment/v1/catalog", (route) => route.fulfill({ json: { ...provisioningCatalog(), gui_create_organizations: ["alpha"] } }));
    await page.getByRole("button", { name: "Check again", exact: true }).click();
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeEnabled();
    await page.locator('#provisioning-draft [name="organization"]').selectOption("beta");
    await expect(page.locator('#provisioning-draft [name="type"]')).toHaveValue("");
    await expect(page.locator('#provisioning-draft [name="type"] option')).toHaveText(["Choose approved value", "server"]);
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    await page.route("**/auth/operator/session", (route) => route.fulfill({ json: {
      identity: { issuer: "https://identity.example", subject: "reader", display_name: "Read Only Operator", organizations: ["alpha"], permissions: ["read"] }, csrf: "reader-csrf",
    } }));
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(page.locator("#operator-login-status")).toHaveText("Signed in: Read Only Operator");
    await page.locator('#provisioning-draft [name="organization"]').selectOption("alpha");
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
  });

  test("confirms once with CSRF and clears the one-time code without persistence", async ({ page, secureAppURL: appURL, api }) => {
    await mockInvitationCreator(page, api);
    const posted = [];
    api.handlers.set("/api/enrollment/v1/invitations", (route) => {
      if (route.request().method() === "GET") return route.fulfill({ json: { invitations: [] } });
      posted.push({ headers: route.request().headers(), asset: route.request().postDataJSON() });
      return route.fulfill({ status: 201, json: createdInvitationResponse(posted.at(-1).asset) });
    });
    await page.goto(appURL + "/#provisioning-adapters");
    await fillApprovedInvitation(page);
    await confirmInvitationCreation(page);
    const created = page.getByRole("dialog", { name: "Invitation created — one-time code" });
    await expect(created).toContainText("A".repeat(26));
    await expect(created).toContainText("No email was sent");
    expect(posted).toHaveLength(1);
    expect(posted[0].headers["x-zpr-csrf"]).toBe("creation-csrf-proof");
    expect(posted[0].asset).toEqual({ name: "Created laptop", owner: "Operations", organization: "alpha", asset_id: "INV-CREATE",
      type: "laptop", profile: "standard", recipient: "owner@example.org" });
    expect(await page.evaluate(() => [JSON.stringify(localStorage), JSON.stringify(sessionStorage)].some((value) => value.includes("A".repeat(26)) || value.includes("creation-csrf-proof")))).toBe(false);
    await page.keyboard.press("Escape");
    await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
    await expect(page.locator("#provisioning-create-status")).toContainText("one-time code cleared");
    expect(posted).toHaveLength(1);
  });

  for (const clearing of ["navigation", "session-loss"]) {
    test(`clears a confirmed one-time code on ${clearing}`, async ({ page, secureAppURL: appURL, api }) => {
      await mockInvitationCreator(page, api);
      api.handlers.set("/api/enrollment/v1/invitations", (route) => route.request().method() === "GET" ?
        route.fulfill({ json: { invitations: [] } }) :
        route.fulfill({ status: 201, json: createdInvitationResponse(route.request().postDataJSON()) }));
      await page.goto(appURL + "/#provisioning-adapters");
      await fillApprovedInvitation(page);
      await confirmInvitationCreation(page);
      await expect(page.locator("#provisioning-one-time-code")).toHaveText("A".repeat(26));
      if (clearing === "navigation") await page.evaluate(() => { location.hash = "#map"; });
      else await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
      await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
      await expect(page.locator("#provisioning-create-status")).toContainText("one-time code cleared");
    });
  }

  for (const failure of ["unavailable", "mismatched", "wrong-audit", "wrong-lifetime", "bad-code"]) {
    test(`never retries ${failure} creation responses and requires registry reconciliation`, async ({ page, secureAppURL: appURL, api }) => {
      await mockInvitationCreator(page, api);
      let posts = 0;
      api.handlers.set("/api/enrollment/v1/invitations", (route) => {
        if (route.request().method() === "GET") return route.fulfill({ json: { invitations: [] } });
        posts++;
        const result = createdInvitationResponse(route.request().postDataJSON());
        if (failure === "mismatched") result.invitation.asset.organization = "beta";
        if (failure === "wrong-audit") result.invitation.created_by = 'oidc:["https://identity.example","another-admin"]';
        if (failure === "wrong-lifetime") result.invitation.expires_at = new Date(Date.parse(result.invitation.created_at) + 1800000).toISOString();
        if (failure === "bad-code") result.enrollment_code = "invalid-secret";
        return route.fulfill(failure === "unavailable" ? { status: 503, json: { error: "Response unavailable" } } : { status: 201, json: result });
      });
      await page.goto(appURL + "/#provisioning-adapters");
      await fillApprovedInvitation(page);
      await confirmInvitationCreation(page);
      await expect(page.locator("#provisioning-create-status")).toContainText("Creation outcome uncertain");
      await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
      await page.getByRole("button", { name: "Clear worksheet", exact: true }).click();
      await page.getByRole("button", { name: "Check again", exact: true }).click();
      await fillApprovedInvitation(page);
      await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
      await page.getByRole("button", { name: "Resolve uncertain creation", exact: true }).click();
      const reconcile = page.getByRole("dialog", { name: "Resolve uncertain invitation creation" });
      await expect(reconcile).toContainText("all registry pages");
      await expect(reconcile.getByRole("button", { name: "Clear uncertainty", exact: true })).toBeDisabled();
      await reconcile.getByRole("checkbox").check();
      await reconcile.getByRole("button", { name: "Clear uncertainty", exact: true }).click();
      await expect(page.locator("#provisioning-create-status")).toContainText("No mutation was retried");
      expect(posts).toBe(1);
    });
  }

  test("ignores delayed mutation responses after session loss", async ({ page, secureAppURL: appURL, api }) => {
    await mockInvitationCreator(page, api);
    let release;
    let posts = 0;
    api.handlers.set("/api/enrollment/v1/invitations", async (route) => {
      if (route.request().method() === "GET") return route.fulfill({ json: { invitations: [] } });
      posts++;
      const result = createdInvitationResponse(route.request().postDataJSON());
      await new Promise((resolve) => { release = resolve; });
      await route.fulfill({ status: 201, json: result }).catch(() => {});
    });
    await page.goto(appURL + "/#provisioning-adapters");
    await fillApprovedInvitation(page);
    await confirmInvitationCreation(page);
    await expect.poll(() => Boolean(release)).toBe(true);
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    release();
    await expect(page.locator("#provisioning-create-status")).toContainText("Creation outcome uncertain");
    await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    expect(posts).toBe(1);
  });

  test("times out at twenty seconds without retrying or exposing a late code", async ({ page, secureAppURL: appURL, api }) => {
    await page.clock.install();
    await mockInvitationCreator(page, api);
    let release;
    let posts = 0;
    api.handlers.set("/api/enrollment/v1/invitations", async (route) => {
      if (route.request().method() === "GET") return route.fulfill({ json: { invitations: [] } });
      posts++;
      const result = createdInvitationResponse(route.request().postDataJSON());
      await new Promise((resolve) => { release = resolve; });
      await route.fulfill({ status: 201, json: result }).catch(() => {});
    });
    await page.goto(appURL + "/#provisioning-adapters");
    await fillApprovedInvitation(page);
    await confirmInvitationCreation(page);
    await expect.poll(() => Boolean(release)).toBe(true);
    await page.clock.fastForward(19000);
    await expect(page.locator("#provisioning-create-status")).toContainText("Creating invitation once");
    await page.clock.fastForward(1001);
    await expect(page.locator("#provisioning-create-status")).toContainText("Creation outcome uncertain");
    release();
    await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    expect(posts).toBe(1);
  });

  test("clears denied authorization without mislabeling an explicit rejection as uncertain", async ({ page, secureAppURL: appURL, api }) => {
    await mockInvitationCreator(page, api);
    let posts = 0;
    api.handlers.set("/api/enrollment/v1/invitations", (route) => {
      if (route.request().method() === "GET") return route.fulfill({ json: { invitations: [] } });
      posts++;
      return route.fulfill({ status: 403, json: { error: "create grant removed" } });
    });
    await page.goto(appURL + "/#provisioning-adapters");
    await fillApprovedInvitation(page);
    await confirmInvitationCreation(page);
    await expect(page.locator("#provisioning-create-status")).toContainText("Creation rejected (HTTP 403)");
    await expect(page.locator("#provisioning-create-status")).not.toContainText("uncertain");
    await expect(page.getByRole("button", { name: "Resolve uncertain creation", exact: true })).toBeHidden();
    await expect(page.getByRole("button", { name: "Create invitation", exact: true })).toBeDisabled();
    await expect(page.locator("#provisioning-invitations tbody")).toBeEmpty();
    expect(posts).toBe(1);
  });

  test("reports explicit rejections and enforces the UTF-8 byte limit", async ({ page, secureAppURL: appURL, api }) => {
    await mockInvitationCreator(page, api);
    let posts = 0;
    api.handlers.set("/api/enrollment/v1/invitations", (route) => {
      if (route.request().method() === "GET") return route.fulfill({ json: { invitations: [] } });
      posts++;
      return route.fulfill({ status: 409, json: { error: "active asset" } });
    });
    await page.goto(appURL + "/#provisioning-adapters");
    await fillApprovedInvitation(page);
    await page.locator('#provisioning-draft [name="name"]').fill("é".repeat(129));
    await page.getByRole("button", { name: "Create invitation", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    expect(posts).toBe(0);
    await page.locator('#provisioning-draft [name="name"]').fill("é".repeat(128));
    await confirmInvitationCreation(page);
    await expect(page.locator("#provisioning-create-status")).toContainText("Creation rejected (HTTP 409)");
    await expect(page.locator("#provisioning-one-time-code")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Resolve uncertain creation", exact: true })).toBeHidden();
    expect(posts).toBe(1);
  });
});

test("GUI text editors start unwrapped with a checkbox and keep gutter rows aligned", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  const toggle = page.locator("#page-zpr-config").getByRole("checkbox", { name: "Wrap", exact: true });
  await expect(toggle).not.toBeChecked();
  await source.fill(`short = 1\nlong = "${"x".repeat(600)}"\nlast = 2`);
  await expect(source).toHaveAttribute("wrap", "off");
  await expect(page.locator("#zpr-config-highlight")).toHaveCSS("white-space", "pre");
  const geometry = () => page.evaluate(() => {
    const textarea = document.getElementById("zpr-config-source");
    const rows = [...document.querySelectorAll("#zpr-config-gutter .config-gutter-line")].map((row) => row.getBoundingClientRect().height);
    const highlight = document.getElementById("zpr-config-highlight");
    return { rows, scrollWidth: textarea.scrollWidth, clientWidth: textarea.clientWidth, highlightHeight: highlight.scrollHeight, sourceHeight: textarea.scrollHeight };
  });
  let measured = await geometry();
  expect(measured.rows).toHaveLength(3);
  expect(Math.abs(measured.rows[2] - measured.rows[0])).toBeLessThan(1);
  expect(Math.abs(measured.highlightHeight - measured.sourceHeight)).toBeLessThan(2);

  await toggle.check();
  await expect(source).toHaveAttribute("wrap", "soft");
  await expect(page.locator("#zpr-config-highlight")).toHaveCSS("white-space", "pre-wrap");
  await expect.poll(async () => (await geometry()).rows[1]).toBeGreaterThan(50);
  measured = await geometry();
  expect(measured.rows[1]).toBeGreaterThan(measured.rows[0] * 2);
  expect(measured.scrollWidth).toBeLessThanOrEqual(measured.clientWidth + 1);

  await toggle.uncheck();
  await expect(source).toHaveAttribute("wrap", "off");
  await expect(page.locator("#zpr-config-highlight")).toHaveCSS("white-space", "pre");
  await expect.poll(async () => (await geometry()).rows[1]).toBeLessThan(25);
  measured = await geometry();
  expect(measured.scrollWidth).toBeGreaterThan(measured.clientWidth);
  await page.reload();
  await expect(page.locator("#page-zpr-config").getByRole("checkbox", { name: "Wrap", exact: true })).not.toBeChecked();

  for (const [hash, id] of [["#policy", "policy-source"], ["#gateways", "gateway-source"]]) {
    await page.goto(appURL + "/" + hash);
    await expect(page.locator(`#page-${hash.slice(1)}`).getByRole("checkbox", { name: "Wrap", exact: true })).not.toBeChecked();
    await expect(page.locator(`#${id}`)).toHaveAttribute("wrap", "off");
  }
});

test("GUI Map zero visa badges are white-filled and retain accessible counts", async ({ page, appURL, api }) => {
  api.snapshot.actors = [{ cn: "client", node: false, zpr_addr: "fd00::1" }];
  api.snapshot.services = [{ service_name: "API", actor_cn: "client", zpr_addr: "fd00::1", service_endpoints: "TCP/443" }];
  api.snapshot.active_visas = [];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  for (const selector of ['[data-inspect-actor="client"]', '[data-inspect-service="API"]']) {
    const badge = page.locator(`${selector} .graph-visa-count`);
    await expect(badge).toHaveAttribute("aria-label", "0 active visas");
    await expect(badge.locator("title")).toHaveText("0 active visas");
    await expect(badge.locator("text")).toHaveCount(0);
    await expect(badge.locator("rect")).toHaveCSS("fill", "rgb(255, 255, 255)");
    await expect(badge.locator("rect")).toHaveCSS("stroke", "rgb(23, 77, 61)");
    await expect(badge.locator("rect")).toHaveAttribute("width", "22");
  }
  api.snapshot.active_visas = [{ id: 1, expires: Math.floor(Date.now() / 1000) + 3600, source_addr: "fd00::2", dest_addr: "fd00::1", proto: "TCP", dest_port: 443 }];
  await page.locator("#refresh-now").click();
  for (const selector of ['[data-inspect-actor="client"]', '[data-inspect-service="API"]']) {
    const badge = page.locator(`${selector} .graph-visa-count`);
    await expect(badge.locator("text")).toHaveText("1");
    await expect(badge.locator("rect")).toHaveCSS("fill", "rgb(23, 77, 61)");
  }
  api.snapshot.active_visas = null;
  await page.locator("#refresh-now").click();
  await expect(page.locator('[data-inspect-actor="client"] .graph-visa-count text')).toHaveText("?");
});

for (const reducedMotion of ["no-preference", "reduce"]) {
test(`GUI Map manual pan and zoom disable Auto-fit (${reducedMotion})`, async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion });
  api.snapshot.actors = [{ cn: "client", node: false, zpr_addr: "fd00::1" }];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const autoFit = page.getByRole("checkbox", { name: "Auto-fit", exact: true });
  const svg = page.locator(".topology-graph");
  const viewport = () => page.evaluate(() => ({
    box: document.querySelector(".topology-graph").getAttribute("viewBox"),
    transform: document.querySelector("#graph-world").getAttribute("transform"),
  }));
  await expect(autoFit).toBeChecked();
  await page.getByRole("button", { name: "Fit graph", exact: true }).click();
  await expect(autoFit).toBeChecked();
  const bounds = await svg.boundingBox();
  await page.mouse.click(bounds.x + 8, bounds.y + 8);
  await expect(autoFit).toBeChecked();
  for (const interaction of ["in", "out", "wheel", "pan"]) {
    if (!(await autoFit.isChecked())) await autoFit.check();
    const before = await viewport();
    if (interaction === "in" || interaction === "out") {
      await page.getByRole("button", { name: interaction === "in" ? "Zoom in" : "Zoom out", exact: true }).click();
    } else if (interaction === "wheel") {
      await svg.dispatchEvent("wheel", { deltaY: -100, clientX: bounds.x + 20, clientY: bounds.y + 20 });
    } else {
      await page.mouse.move(bounds.x + 8, bounds.y + 8);
      await page.mouse.down();
      await page.mouse.move(bounds.x + 48, bounds.y + 38, { steps: 4 });
      await expect(autoFit).not.toBeChecked();
      await page.mouse.move(bounds.x + 8, bounds.y + 8, { steps: 4 });
      expect(await viewport()).toEqual(before);
      await page.mouse.move(bounds.x + 48, bounds.y + 38, { steps: 4 });
      await page.mouse.up();
    }
    await expect(autoFit).not.toBeChecked();
    const manual = await viewport();
    expect(manual).not.toEqual(before);
    api.snapshot.actors.push({ cn: `extra-${interaction}`, node: false, zpr_addr: `fd00::${api.snapshot.actors.length + 1}` });
    await page.locator("#refresh-now").click();
    await expect(page.locator(`.graph-vertex[data-inspect-actor="extra-${interaction}"]`)).toHaveCount(1);
    await expect(autoFit).not.toBeChecked();
    expect(await viewport()).toEqual(manual);
    await page.getByRole("button", { name: "Fit graph", exact: true }).click();
    await expect(autoFit).not.toBeChecked();
  }
  await autoFit.check();
  await page.locator("#refresh-now").click();
  await expect(autoFit).toBeChecked();
});
}

test("GUI Map decision feedback uses prominent rings and wires", async ({ page, appURL, api }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  api.snapshot.actors = [
    { cn: "node", node: true, node_details: { adapters: ["client"] } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
  ];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  api.snapshot.recent_visas = [{ id: 123, source_addr: "fd00::1" }];
  await page.locator("#refresh-now").click();
  const ring = page.locator(".graph-decision-ring");
  await expect(ring).toHaveCSS("stroke-width", "6px");
  expect(await ring.evaluate((element) => getComputedStyle(element).filter)).toContain("drop-shadow");
  const feedback = await page.locator('.graph-edge[data-dock-adapter="client"] .graph-link').evaluate((element) => ({
    widths: element.getAnimations()[0].effect.getKeyframes().map((frame) => Number.parseFloat(frame.strokeWidth)),
    transforms: document.querySelector(".graph-decision-ring").getAnimations()[0].effect.getKeyframes().map((frame) => frame.transform).filter(Boolean),
  }));
  expect(Math.max(...feedback.widths)).toBeGreaterThanOrEqual(4.5);
  expect(feedback.transforms).toEqual([]);
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

test("organization base restore requires explicit confirmation", async ({ page, appURL, api }) => {
  const restorePath = "/api/simulator/organizations/beta/restore-base";
  api.handlers.set(restorePath, async (route) => {
    expect(route.request().method()).toBe("POST");
    api.organizations.activation = { state: "completed", operation: "restore-base", organization_id: "beta", progress: "Organization ready" };
    await route.fulfill({ status: 202, json: { activation: api.organizations.activation } });
  });
  await page.goto(appURL + "/organizations.html");
  await page.locator('[data-organization-id="beta"]').click();
  const restore = page.locator('[data-restore-base="beta"]');
  await restore.click();
  const dialog = page.getByRole("dialog", { name: "Restore base state?", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("bundled defaults");
  await expect(dialog).toContainText("backed up locally");
  expect(api.counts.get(restorePath) || 0).toBe(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(api.counts.get(restorePath) || 0).toBe(0);

  await restore.click();
  await dialog.getByRole("button", { name: "Restore base state", exact: true }).click();
  await expect.poll(() => api.counts.get(restorePath)).toBe(1);
  await expect(dialog).not.toBeVisible();
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

test("Organizations shows See Logs only for a failed activation or base restore", async ({ page, appURL, api }) => {
  await page.clock.install();
  await page.goto(appURL + "/organizations.html");
  await expect(page.locator("#organization-title")).toHaveText("Alpha Labs");
  const resetLog = page.locator(".organization-reset-log");
  await expect(resetLog).toHaveText("See Logs");
  await expect(resetLog).toHaveAttribute("href", "/api/simulator/activation-log");
  await expect(resetLog).toHaveAttribute("target", "_blank");
  await expect(resetLog).toHaveAttribute("rel", "noopener noreferrer");
  await expect(resetLog).toBeHidden();
  for (const operation of ["activate", "restore-base"]) {
    api.organizations.activation = { state: "failed", operation, organization_id: "beta", error: "Directory startup failed" };
    await page.clock.runFor(5100);
    await expect(page.getByRole("link", { name: "See Logs", exact: true })).toBeVisible();
    await expect(page.locator("#organization-error")).toHaveText("Directory startup failed");
    for (const state of ["resetting", "completed", "idle"]) {
      api.organizations.activation = { state, operation, organization_id: "beta" };
      await page.clock.runFor(5100);
      await expect(resetLog).toBeHidden();
      await expect(page.locator("#organization-error")).toBeHidden();
    }
  }
});

test("Organizations retains failure log access when the catalog becomes unavailable", async ({ page, appURL, api }) => {
  await page.clock.install();
  api.organizations.activation = { state: "failed", organization_id: "beta", error: "Runtime startup failed" };
  await page.goto(appURL + "/organizations.html");
  const resetLog = page.getByRole("link", { name: "See Logs", exact: true });
  await expect(resetLog).toBeVisible();
  api.handlers.set("/api/simulator/organizations", async (route) => {
    await route.fulfill({ status: 503, json: { error: "Organization service unavailable" } });
  });
  await page.clock.runFor(5100);
  await expect(page.locator("#organization-error")).toHaveText("Organization service unavailable");
  await expect(resetLog).toBeVisible();
  api.handlers.delete("/api/simulator/organizations");
  api.organizations.activation = { state: "completed", organization_id: "beta" };
  await page.clock.runFor(5100);
  await expect(resetLog).toBeHidden();
  await expect(page.locator("#organization-error")).toBeHidden();
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
test("GUI Map buffered denial badge opens node denial details on right-click", async ({ page, appURL, api }) => {
  api.snapshot.actors = [
    { cn: "node", node: true, zpr_addr: "fd00::ff", node_details: { adapters: ["client"], buffered_denials: 4, local_denials: 19 } },
    { cn: "client", node: false, zpr_addr: "fd00::1" },
    { cn: "other", node: false, zpr_addr: "fd00::9" },
  ];
  api.snapshot.recent_denies = [
    { source_addr: "fd00::1", dest_addr: "fd00::2", protocol: 6, dest_port: 443, deny_code: "NoMatchingPolicy", count: 4, last_deny_ms: Date.now() },
    { source_addr: "fd00::9", dest_addr: "fd00::2", protocol: 6, dest_port: 22, deny_code: "OtherNode", count: 1, last_deny_ms: Date.now() },
  ];
  await page.goto(appURL + "/#map");
  await page.getByRole("button", { name: "Pause updates", exact: true }).click();
  const badge = page.locator('.graph-vertex[data-inspect-actor="node"] .graph-denial-count');
  await expect(badge).toHaveAttribute("role", "button");
  await badge.click({ button: "right" });
  const inspector = page.locator("#inspector-body");
  await expect(inspector).toContainText("Node denial telemetry");
  await expect(inspector).toContainText("Buffered denials");
  await expect(inspector).toContainText("NoMatchingPolicy");
  await expect(inspector).not.toContainText("OtherNode");
  await expect(inspector).toContainText("count only");
});

function mockEditorAssistant(api, path, answer, requests, statusPath = path) {
  api.handlers.set(path, async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ json: { ready: true, model: "test-model", models: ["test-model"] } });
      return;
    }
    requests.push(route.request().postDataJSON());
    await route.fulfill({ json: { answer, input_tokens: 40, output_tokens: 9 } });
  });
  if (statusPath !== path) api.handlers.set(statusPath, async (route) => route.fulfill({ json: { ready: true, model: "test-model", models: ["test-model"] } }));
}

test("AI shared shell defaults to collapsed options and safe, undoable insertion including fallback redo", async ({ page, appURL, api }) => {
  await page.goto(appURL + "/#map");
  await page.evaluate(() => {
    const pane = document.createElement("aside");
    pane.id = "test-assistant";
    pane.style.cssText = "position:fixed;inset:20px;z-index:10000;background:white;overflow:auto";
    pane.innerHTML = '<div class="assistant-heading"><span class="assistant-state"></span></div>';
    const source = document.createElement("textarea");
    source.id = "test-ai-source";
    source.value = "original";
    document.body.prepend(pane, source);
    window.testAI = window.ZPRAssistant.mount({
      pane, prefix: "test-ai", getContext: () => ({ source: source.value }), getTarget: () => source,
      request: async (conversation) => {
        window.testAIRequest = conversation;
        return { answer: '<img src=x onerror=alert(1)>\n```\nreplacement\n```', input_tokens: 12, output_tokens: 8 };
      },
      watch: (invalidate) => source.addEventListener("input", invalidate),
    });
    window.testAI.setStatus({ ready: true, model: "test-model", models: ["test-model", "second-model"] });
    pane.append(source);
  });
  const pane = page.locator("#test-assistant");
  await expect(pane.getByRole("checkbox")).toHaveCount(0);
  await expect(pane.getByLabel("Model", { exact: true })).toBeHidden();
  await expect(pane.getByLabel("Max tokens", { exact: true })).toBeHidden();
  await pane.getByText("Model and max tokens", { exact: true }).click();
  await expect(pane.getByLabel("Max tokens", { exact: true })).toBeVisible();
  await pane.getByLabel("Model", { exact: true }).selectOption("second-model");
  await pane.getByLabel("Max tokens", { exact: true }).selectOption("2400");
  await expect(pane.getByRole("textbox", { name: "Message" })).toHaveAttribute("placeholder", "How can I help?");
  await pane.getByRole("textbox", { name: "Message" }).fill("Help");
  await pane.getByRole("button", { name: "Send", exact: true }).click();
  await expect(pane.locator("img")).toHaveCount(0);
  await expect(pane.locator(".assistant-usage")).toContainText("12 input · 8 output");
  expect(await page.evaluate(() => window.testAIRequest)).toEqual({ messages: [{ role: "user", content: "Help" }], model: "second-model", max_tokens: 2400 });
  const source = page.locator("#test-ai-source");
  await source.evaluate((element) => element.setSelectionRange(0, element.value.length));
  await source.evaluate((element) => { element.maxLength = 4; });
  await pane.getByRole("button", { name: "Insert", exact: true }).click();
  await expect(pane.locator(".assistant-error")).toContainText("size limit");
  await expect(source).toHaveValue("original");
  await source.evaluate((element) => { element.removeAttribute("maxlength"); element.readOnly = true; });
  await pane.getByRole("button", { name: "Insert", exact: true }).click();
  await expect(pane.locator(".assistant-error")).toContainText("read-only");
  await source.evaluate((element) => { element.readOnly = false; });
  await page.evaluate(() => { document.execCommand = () => false; });
  await pane.getByRole("button", { name: "Insert", exact: true }).click();
  await expect(source).toHaveValue("replacement");
  await source.press("ControlOrMeta+z");
  await expect(source).toHaveValue("original");
  await source.press("ControlOrMeta+Shift+z");
  await expect(source).toHaveValue("replacement");
  await pane.getByRole("button", { name: "Insert", exact: true }).click();
  await expect(pane.locator(".assistant-error")).toContainText("editor changed after this suggestion");
  await expect(source).toHaveValue("replacement");
  await pane.getByRole("button", { name: "Clear", exact: true }).click();
  await expect(pane.locator(".assistant-usage")).toContainText("0 input · 0 output");
});

test("AI Policy pending conversation cannot leak a response into a selected Assertion record", async ({ page, appURL, api }) => {
  Object.assign(api.policy, { assistant_ready: true, assistant_model: "test-model", assistant_models: ["test-model"] });
  let complete;
  api.handlers.set("/api/policy/assistant", async (route) => {
    await new Promise((resolve) => { complete = resolve; });
    return route.fulfill({ json: { answer: "Obsolete policy response", input_tokens: 40, output_tokens: 20 } });
  });
  await page.goto(appURL + "/#policy");
  await page.locator("#assistant-question").fill("Review policy");
  await page.locator("#assistant-send").click();
  await expect.poll(() => typeof complete).toBe("function");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-assertions"]').click();
  await expect(page.locator("#policy-assertion-editor")).toBeVisible();
  complete();
  await page.route("**/api/policy/assistant", (route) => route.fulfill({ json: { answer: "Fresh assertion response", input_tokens: 2, output_tokens: 3 } }));
  await page.locator("#assistant-question").fill("Review assertion");
  await page.locator("#assistant-send").click();
  await expect(page.locator("#assistant-messages")).toContainText("Fresh assertion response");
  await expect(page.locator("#assistant-messages")).not.toContainText("Obsolete policy response");
  await expect(page.locator("#assistant-usage")).toContainText("2 input · 3 output");
  expect([...api.counts.keys()].filter((path) => path.startsWith("/api/simulator"))).toEqual([]);
});

for (const outcome of ["success", "error"]) {
  test(`AI Config rejects delayed ${outcome} after edit-return and permits a fresh request`, async ({ page, appURL, api }) => {
    Object.assign(api.policy, { assistant_ready: true, assistant_model: "test-model", assistant_models: ["test-model"] });
    let complete;
    let fresh = false;
    api.handlers.set("/api/policy/assistant", async (route) => {
      if (fresh) return route.fulfill({ json: { answer: "Current response", input_tokens: 2, output_tokens: 3 } });
      await new Promise((resolve) => { complete = resolve; });
      return route.fulfill(outcome === "error"
        ? { status: 503, json: { error: "Obsolete error" } }
        : { json: { answer: "Obsolete success", input_tokens: 40, output_tokens: 20 } });
    });
    await page.goto(appURL + "/#zpr-config");
    const source = page.locator("#zpr-config-source");
    await source.fill('[visa_service]\ndock_node = "node"\n');
    const original = await source.inputValue();
    const pane = page.locator("#editor-assistant-zpr-config-pane");
    const question = pane.getByRole("textbox", { name: "Message" });
    await question.fill("Explain");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    await expect(pane.locator(".assistant-state")).toHaveText("Thinking");
    await expect.poll(() => typeof complete).toBe("function");
    await source.fill(original + "# changed\n");
    await source.fill(original);
    complete();
    fresh = true;
    await expect(pane.locator(".assistant-error")).toContainText("editor changed while Claude");
    await question.fill("Explain current");
    await pane.getByRole("button", { name: "Send", exact: true }).click();
    await expect(pane.locator(".assistant-message.assistant")).toContainText("Current response");
    await expect(pane).not.toContainText("Obsolete");
    await expect(pane.locator(".assistant-usage")).toContainText("2 input · 3 output");
    expect([...api.counts.keys()].filter((path) => path.startsWith("/api/simulator"))).toEqual([]);
  });
}

test("AI Scenario Apply supports exact raw Undo and Redo without saving or running", async ({ page, appURL, api }) => {
  let proposals = 0;
  api.handlers.set("/api/simulator/design-assistant", async (route) => {
    const request = route.request().postDataJSON();
    proposals++;
    return route.fulfill({ json: { answer: "Proposal", proposal: { scenario: { ...request.scenario, name: proposals === 1 ? "AI changed name" : "Second AI change" } } } });
  });
  await openRawScenario(page, appURL, api, true);
  const source = page.locator("#scenario-editor-source");
  const original = await source.inputValue();
  await source.fill(JSON.stringify(JSON.parse(original)));
  const exact = await source.inputValue();
  const pane = page.locator("#scenario-assistant-slot");
  await expect(pane.locator("[data-assistant-model]")).toBeHidden();
  await pane.locator("[data-assistant-question]").fill("Review");
  await pane.locator("[data-assistant-submit]").click();
  await pane.locator("[data-assistant-apply]").click();
  await expect(page.locator("#scenario-editor-name")).toHaveValue("AI changed name");
  const applied = await source.inputValue();
  await pane.getByRole("button", { name: "Undo AI change", exact: true }).click();
  await expect(source).toHaveValue(exact);
  await pane.getByRole("button", { name: "Redo AI change", exact: true }).click();
  await expect(source).toHaveValue(applied);
  await pane.locator("[data-assistant-question]").fill("Review again");
  await pane.locator("[data-assistant-submit]").click();
  await pane.locator("[data-assistant-apply]").click();
  await expect(page.locator("#scenario-editor-name")).toHaveValue("Second AI change");
  const second = await source.inputValue();
  await pane.getByRole("button", { name: "Undo AI change", exact: true }).click();
  await expect(source).toHaveValue(applied);
  await pane.getByRole("button", { name: "Undo AI change", exact: true }).click();
  await expect(source).toHaveValue(exact);
  await pane.getByRole("button", { name: "Redo AI change", exact: true }).click();
  await expect(source).toHaveValue(applied);
  await pane.getByRole("button", { name: "Redo AI change", exact: true }).click();
  await expect(source).toHaveValue(second);
  await source.fill(second + "\n");
  await pane.getByRole("button", { name: "Undo AI change", exact: true }).click();
  await expect(pane.locator(".assistant-error")).toContainText("Undo/redo is no longer available");
  await expect(source).toHaveValue(second + "\n");
  expect(api.counts.get("/api/simulator/scenarios/run") || 0).toBe(0);
  expect(api.counts.get("/api/simulator/organizations/alpha/scenarios") || 0).toBe(0);
});

test("AI organization Apply is undoable and never saves or publishes the directory", async ({ page, appURL, api }) => {
  const endpoint = "/api/simulator/organizations/alpha/directory";
  const original = "dn: cn=Operators,dc=alpha,dc=test\ncn: Operators\n";
  const proposed = "dn: cn=Reviewers,dc=alpha,dc=test\ncn: Reviewers\n";
  const mutations = [];
  api.handlers.set(endpoint, (route) => {
    if (route.request().method() !== "GET") mutations.push(route.request().method());
    return route.fulfill({ json: { revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: original } } });
  });
  api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1, summary: "Initial" }] }));
  api.handlers.set("/api/simulator/assistant/status", (route) => route.fulfill({ json: { ready: true, model: "test-model", models: ["test-model"] } }));
  api.handlers.set("/api/simulator/design-assistant", (route) => route.fulfill({ json: { answer: "Proposal", proposal: { directory_ldif: proposed } } }));
  await page.goto(appURL + "/organizations.html");
  await page.locator("#organization-design-assistant").click();
  const pane = page.locator("#organization-assistant-slot");
  await pane.locator("[data-assistant-question]").fill("Review");
  await pane.locator("[data-assistant-submit]").click();
  await pane.locator("[data-assistant-apply]").click();
  await expect(page.locator("#directory-editor-source")).toHaveValue(proposed);
  await pane.getByRole("button", { name: "Undo AI change", exact: true }).click();
  await expect(page.locator("#directory-editor-source")).toHaveValue(original);
  await pane.getByRole("button", { name: "Redo AI change", exact: true }).click();
  await expect(page.locator("#directory-editor-source")).toHaveValue(proposed);
  expect(mutations).toEqual([]);
  expect(api.counts.get(endpoint + "/publish") || 0).toBe(0);
});

test("GUI gateway editor AI Assistant is ready without opt-in, sends the draft, and inserts suggestions undoably", async ({ page, appURL, api }) => {
  api.handlers.set("/api/gateways/contracts", async (route) => route.fulfill({ json: { organization_id: "alpha", contracts: [
    { organization_id: "alpha", instance_id: "public-egress", adapter_cn: "gateway-public-egress", service_name: "public-egress.svc.zpr", external_network: "" },
  ] } }));
  api.handlers.set("/api/gateways/configs", async (route) => route.fulfill({ json: { organization_id: "alpha", configs: [] } }));
  const requests = [];
  mockEditorAssistant(api, "/api/gateways/assistant", 'Add a destination:\n```json\n{"origin": "https://api.example.com"}\n```', requests);
  await page.goto(`${appURL}/#gateways`);
  const source = page.locator("#gateway-source");
  await expect(source).toHaveValue(/"instance_id": "public-egress"/);
  const original = await source.inputValue();
  const pane = page.locator("#editor-assistant-gateway-pane");
  await expect(pane).toBeVisible();
  await expect(pane.locator(".assistant-state")).toHaveText("Ready");
  const question = pane.getByRole("textbox", { name: "Message" });
  await expect(question).toBeEnabled();
  await expect(pane.getByRole("checkbox", { name: "Use assistant" })).toHaveCount(0);
  await question.fill("Add an origin");
  await pane.getByRole("button", { name: "Send" }).click();
  await expect(pane.locator(".assistant-code")).toHaveText('{"origin": "https://api.example.com"}');
  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({ editor: "gateway", source: original, model: "test-model", max_tokens: 1200, messages: [{ role: "user", content: "Add an origin" }] });
  await expect(pane.locator(".assistant-usage")).toContainText("40 input");
  await source.evaluate((element) => element.setSelectionRange(0, element.value.length));
  await pane.getByRole("button", { name: "Insert" }).click();
  await expect(source).toHaveValue('{"origin": "https://api.example.com"}');
  await source.press("ControlOrMeta+z");
  await expect(source).toHaveValue(original);
  const layout = page.locator("#page-gateways .editor-assistant-layout");
  if ((page.viewportSize()?.width || 1000) > 900) {
    const editorBox = await layout.locator(".editor-assistant-main").boundingBox();
    const paneBox = await pane.boundingBox();
    expect(paneBox.x).toBeGreaterThan(editorBox.x + editorBox.width - 2);
  }
  await pane.getByRole("button", { name: "Collapse AI Assistant" }).click();
  await expect(layout).toHaveAttribute("data-assistant-collapsed", "true");
  await expect(pane.locator(".assistant-settings")).toBeHidden();
  await expect(pane.locator(".pane-toggle-label")).toHaveText("AI Assistant");
});

test("GUI ZPR Config AI Assistant uses the policy assistant endpoint with the config editor", async ({ page, appURL, api }) => {
  Object.assign(api.policy, { assistant_ready: true, assistant_model: "test-model", assistant_models: ["test-model"] });
  const requests = [];
  mockEditorAssistant(api, "/api/policy/assistant", "Looks valid.", requests);
  await page.goto(appURL + "/#zpr-config");
  const source = page.locator("#zpr-config-source");
  await source.fill('[visa_service]\ndock_node = "node"\n');
  const pane = page.locator("#editor-assistant-zpr-config-pane");
  await expect(pane.locator(".assistant-state")).toHaveText("Ready");
  await expect(pane.getByRole("checkbox", { name: "Use assistant" })).toHaveCount(0);
  await pane.getByRole("textbox", { name: "Message" }).fill("Explain");
  await pane.getByRole("textbox", { name: "Message" }).press("ControlOrMeta+Enter");
  await expect(pane.locator(".assistant-message.assistant")).toContainText("Looks valid.");
  expect(requests[0]).toMatchObject({ editor: "zpr-config", source: '[visa_service]\ndock_node = "node"\n' });
});

test("GUI assertion editor shows the AI Assistant and sends the assertion source", async ({ page, appURL, api }) => {
  Object.assign(api.policy, { assistant_ready: true, assistant_model: "test-model", assistant_models: ["test-model"] });
  const requests = [];
  mockEditorAssistant(api, "/api/policy/assistant", '```\ngroup "Operators" members >= 2;\n```', requests);
  await page.goto(appURL + "/#policy");
  await expect(page.locator("#policy-record-title")).toHaveText("Test policy");
  await openPolicyPicker(page);
  await page.locator('[data-record-id="test-assertions"]').click();
  await expect(page.locator("#policy-assertion-editor")).toBeVisible();
  const pane = page.locator("#policy-assistant-pane");
  await expect(pane).toBeVisible();
  await expect(page.locator("#assistant-enabled")).toHaveCount(0);
  await expect(page.locator("#assistant-question")).toHaveAttribute("placeholder", "How can I help?");
  await page.locator("#assistant-question").fill("Check operators");
  await page.locator("#assistant-send").click();
  await expect(pane.locator(".assistant-code")).toHaveText('group "Operators" members >= 2;');
  expect(requests[0]).toMatchObject({ editor: "assertion", source: await page.locator("#assertion-source").inputValue() });
  await page.locator("#assertion-source").evaluate((element) => element.setSelectionRange(element.value.length, element.value.length));
  await pane.getByRole("button", { name: "Insert" }).click();
  await expect(page.locator("#assertion-source")).toHaveValue(/group "Operators" members >= 2;$/);
});

test("GUI Simulator directory editor has an AI Assistant using Simulator endpoints", async ({ page, appURL, api }) => {
  const endpoint = "/api/simulator/organizations/alpha/directory";
  api.handlers.set(endpoint, (route) => route.fulfill({ json: {
    revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: cn=Operators,dc=alpha,dc=test\ncn: Operators\n" },
  } }));
  api.handlers.set(endpoint + "/revisions", (route) => route.fulfill({ json: [{ revision: 1, summary: "Initial" }] }));
  const requests = [];
  mockEditorAssistant(api, "/api/simulator/editor-assistant", "```\ncn: Reviewers\n```", requests, "/api/simulator/assistant/status");
  await page.goto(appURL + "/organizations.html");
  await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
  const pane = page.locator("#editor-assistant-directory-ldif-pane");
  await expect(pane).toBeVisible();
  await expect(pane.locator(".assistant-state")).toHaveText("Ready");
  await expect(pane.getByRole("checkbox", { name: "Use assistant" })).toHaveCount(0);
  await pane.getByRole("textbox", { name: "Message" }).fill("Add a group");
  await pane.getByRole("button", { name: "Send" }).click();
  await expect(pane.locator(".assistant-code")).toHaveText("cn: Reviewers");
  expect(requests[0]).toMatchObject({ editor: "directory-ldif", source: "dn: cn=Operators,dc=alpha,dc=test\ncn: Operators\n" });
  expect(api.counts.get("/api/gateways/assistant") || 0).toBe(0);
  expect(api.counts.get("/api/policy/assistant") || 0).toBe(0);
});

test("GUI editor toolbars place Analyze and Format immediately after File", async ({ page, appURL }) => {
  for (const [hash, file] of [["policy", "#policy-files-toggle"], ["gateways", "#gateway-files-toggle"], ["zpr-config", "#zpr-config-files-toggle"]]) {
    await page.goto(`${appURL}/#${hash}`);
    const tools = page.locator(`#page-${hash} .policy-editor-tools`).first();
    await expect(tools).toBeVisible();
    const layout = await tools.evaluate((element, fileSelector) => {
      const box = (target) => target.getBoundingClientRect();
      const file = box(element.querySelector(fileSelector));
      const actions = box(element.querySelector(".policy-attribute-toolbar"));
      const analyze = [...element.querySelectorAll(".policy-attribute-toolbar .button")].map((button) => button.textContent.trim());
      return { gap: actions.left - file.right, sameRow: Math.abs(actions.top - file.top) <= 2, analyze };
    }, file);
    expect(layout.analyze, hash).toEqual(["Analyze", "Format"]);
    expect(layout.sameRow, hash).toBe(true);
    expect(layout.gap, hash).toBeGreaterThanOrEqual(0);
    expect(layout.gap, hash).toBeLessThanOrEqual(10);
  }
});
