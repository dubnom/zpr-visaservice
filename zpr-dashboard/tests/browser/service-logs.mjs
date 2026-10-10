export function registerServiceLogTests(test, expect) {
  const source = (id, name, logs = [], state = "available") => ({
    id, name, kind: "Required service", identity: `${name}-actor`, state,
    logs: logs.map(body => ({ body })), metrics: [{ name: "requests", value: "18446744073709551615" }],
  });
  const refresh = page => page.evaluate(() => document.dispatchEvent(new CustomEvent("control-room:refreshed")));

  test("Services selector uses the standard log panel with safe raw bodies and independent ANSI records", async ({ page, appURL, api }) => {
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: { state: "available", sources: [
      source("service:a", "Alpha", ["\u001b[32mgreen", 'plain {"id":18446744073709551615} <script>unsafe</script>']),
      source("trusted:b", "Beta", ["Beta only"]),
      { ...source("node:a", "Node", ["Node only"]), kind: "ZPR node" },
    ] } }));
    await page.goto(appURL + "/#diagnostics");
    const selector = page.getByRole("combobox", { name: "Select service" });
    await expect(selector.locator("option")).toHaveCount(2);
    const panel = page.locator("#service-logs .machine-log-panel");
    await expect(panel.locator("h2")).toHaveText("Logs for Alpha");
    const output = panel.locator(".machine-log-output");
    await expect(output).toContainText('plain {"id":18446744073709551615} <script>unsafe</script>');
    await expect(output.locator("pre span")).toHaveCount(1);
    await expect(output.locator("pre span")).toHaveCSS("color", "rgb(0, 187, 0)");
    await expect(output.locator("script, a, time")).toHaveCount(0);
    await expect(output).not.toContainText("Beta only");
    await expect(output).not.toContainText("Node only");
    await expect(output).toHaveCSS("background-color", "rgb(0, 0, 0)");
    await expect(output).toHaveCSS("white-space", "pre");
    await panel.getByLabel("Wrap lines").check();
    await expect(output).toHaveCSS("white-space", "pre-wrap");
    await expect(page.locator(".diagnostics-log-body, .diagnostics-logs")).toHaveCount(0);
    await expect(page.locator(".diagnostics-source-row")).toHaveCount(1);
    await expect(page.locator(".diagnostics-metrics-table")).toContainText("18446744073709551615");
    await expect(page.locator("#diagnostics-aggregate")).toContainText("2 current");
    await selector.selectOption("trusted:b");
    await expect(panel.locator("h2")).toHaveText("Logs for Beta");
    await expect(output.locator("pre")).toHaveText("Beta only");
    await expect(page.locator(".diagnostics-source-row")).toContainText("Beta-actor");
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
    for (const width of [1280, 820, 390]) {
      await page.setViewportSize({ width, height: 844 });
      await expect.poll(() => page.locator("#page-diagnostics").evaluate(element => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth + 1 && element.scrollWidth <= element.clientWidth;
      })).toBe(true);
    }
  });

  test("Services retains selection and per-service scroll with standard maximize controls and navigation restore", async ({ page, appURL, api }) => {
    let sources = [
      source("service:a", "Alpha", Array.from({ length: 100 }, (_, i) => `Alpha ${i}`)),
      source("service:b", "Beta", ["Beta"]),
    ];
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: { sources } }));
    await page.goto(appURL + "/#diagnostics");
    const selector = page.locator("#diagnostics-service-select");
    const panel = page.locator("#service-logs .machine-log-panel");
    const output = panel.locator(".machine-log-output");
    await expect(output).toContainText("Alpha 99");
    await output.evaluate(element => { element.scrollTop = 75; element.dispatchEvent(new Event("scroll")); });
    await selector.selectOption("service:b");
    await expect(output).toContainText("Beta");
    await selector.selectOption("service:a");
    expect(await output.evaluate(element => element.scrollTop)).toBeCloseTo(75, 0);
    sources.reverse();
    await refresh(page);
    await expect(selector).toHaveValue("service:a");
    const maximize = panel.getByRole("button", { name: "Maximize service logs for Alpha" });
    await expect(maximize.locator('svg[data-window-control="maximize"]')).toHaveCount(1);
    await maximize.click();
    await expect(panel).toHaveClass(/maximized/);
    await page.keyboard.press("Escape");
    await expect(panel).not.toHaveClass(/maximized/);
    await expect(maximize).toBeFocused();
    await maximize.click();
    await page.evaluate(() => { location.hash = "#map"; });
    await expect(panel).toHaveCount(0);
    await expect(page.locator("body")).not.toHaveClass(/machine-log-maximized/);
    await page.evaluate(() => { location.hash = "#diagnostics"; });
    await expect(output).toContainText("Alpha 99");
    sources = sources.filter(source => source.id !== "service:a");
    await refresh(page);
    await expect(selector).toHaveValue("service:b");
    await expect(output).toHaveText("Beta");
  });

  test("Services initial Loading, local pause and manual Refresh retain logs through explicit failures", async ({ page, appURL, api }) => {
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    let fail = false;
    api.handlers.set("/api/diagnostics", async route => {
      await gate;
      await route.fulfill(fail ? { status: 503, json: { error: "Provider unavailable" } }
        : { json: { sources: [source("service:a", "Alpha", ["Retained log"])] } });
    });
    await page.goto(appURL + "/#diagnostics");
    await expect(page.locator("#diagnostics-loading")).toBeVisible();
    release();
    const panel = page.locator("#service-logs .machine-log-panel");
    await expect(panel.locator(".machine-log-output")).toContainText("Retained log");
    await expect(page.locator("#diagnostics-loading")).toBeHidden();
    await panel.getByRole("button", { name: "Pause service log updates" }).click();
    const calls = api.counts.get("/api/diagnostics");
    await refresh(page);
    await page.waitForTimeout(100);
    expect(api.counts.get("/api/diagnostics")).toBe(calls);
    fail = true;
    await panel.getByRole("button", { name: "Refresh service logs" }).click();
    await expect(page.locator("#diagnostics-error")).toHaveText("Provider unavailable");
    await expect(panel.locator(".machine-log-output")).toContainText("Retained log");
    await expect(panel.locator(".machine-log-error")).toContainText("last known");
    await expect(page.locator("#diagnostics-loading")).toBeHidden();
    fail = false;
    await panel.getByRole("button", { name: "Resume service log updates" }).click();
    await expect(page.locator("#diagnostics-error")).toBeHidden();
    await expect(panel.locator(".machine-log-error")).toHaveCount(0);
    await page.locator("#diagnostics-filter").fill("absent");
    await expect(panel).toHaveCount(0);
    await expect(page.locator("#diagnostics-service-select")).toBeDisabled();
    await page.locator("#diagnostics-filter").fill("Retained log");
    await expect(panel.locator(".machine-log-output")).toContainText("Retained log");
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    await expect(panel).toHaveCount(0);
    await expect(page.locator("#diagnostics-error")).toContainText("Operator session ended");
    await expect(page.locator("#page-diagnostics")).not.toContainText("Retained log");
  });

  test("Services rejects malformed refresh data without replacing selected logs", async ({ page, appURL, api }) => {
    let malformed = false;
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: {
      sources: malformed ? [{ id: "service:a", logs: [{ body: { unsafe: true } }], metrics: [] }]
        : [source("service:a", "Alpha", ["Last valid sample"])],
    } }));
    await page.goto(appURL + "/#diagnostics");
    await expect(page.locator("#service-logs")).toContainText("Last valid sample");
    malformed = true;
    await refresh(page);
    await expect(page.locator("#diagnostics-error")).toContainText("invalid source or log records");
    await expect(page.locator("#service-logs")).toContainText("Last valid sample");
    await expect(page.locator("#service-logs")).not.toContainText("[object Object]");
  });

  test("Services handles empty inventory, source freshness and invalid records explicitly", async ({ page, appURL, api }) => {
    let sources = [];
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: { sources } }));
    await page.goto(appURL + "/#diagnostics");
    await expect(page.locator("#diagnostics-sources")).toContainText("No service telemetry sources configured.");
    await expect(page.locator("#diagnostics-service-select")).toBeDisabled();
    sources = [source("service:a", "Alpha", ["Last usable log"])];
    await refresh(page);
    const panel = page.locator("#service-logs");
    await expect(panel).toContainText("Last usable log");
    sources = [{ ...source("service:a", "Alpha", [], "unavailable"), error: "Provider offline" }];
    await refresh(page);
    await expect(panel.locator(".machine-log-error")).toContainText("Provider offline");
    await expect(panel).toContainText("Last usable log");
    sources = [source("service:a", "Alpha", ["Stale log"], "stale")];
    await refresh(page);
    await expect(panel.locator(".machine-log-error")).toContainText("stale");
    sources = [source("service:a", "Alpha", [""], "partial")];
    await refresh(page);
    await expect(panel.locator(".machine-log-output pre")).toHaveText("No log body reported.");
    await expect(panel.locator(".machine-log-error")).toContainText("partial");
    for (const invalid of [
      [source("service:a", "Alpha"), source("service:a", "Duplicate")],
      [{ ...source("service:a", "Alpha"), metrics: [{ name: "requests", value: {} }] }],
    ]) {
      sources = invalid;
      await refresh(page);
      await expect(page.locator("#diagnostics-error")).toContainText("invalid source or log records");
      await expect(panel.locator(".machine-log-output pre")).toHaveText("No log body reported.");
    }
  });

  for (const outcome of ["success", "failure"]) {
    test(`Services ignores obsolete ${outcome} after navigation and keeps the latest service selection`, async ({ page, appURL, api }) => {
      const initial = [source("service:a", "Alpha", ["Alpha initial"]), source("service:b", "Beta", ["Beta initial"])];
      api.handlers.set("/api/diagnostics", route => route.fulfill({ json: { sources: initial } }));
      await page.goto(appURL + "/#diagnostics");
      await expect(page.locator("#service-logs")).toContainText("Alpha initial");
      let release;
      const gate = new Promise(resolve => { release = resolve; });
      let started = false;
      api.handlers.set("/api/diagnostics", async route => {
        started = true;
        await gate;
        await route.fulfill(outcome === "success" ? { json: { sources: [source("service:a", "Alpha", ["Obsolete log"])] } }
          : { status: 503, json: { error: "Obsolete failure" } });
      });
      await refresh(page);
      await expect.poll(() => started).toBe(true);
      await page.locator("#diagnostics-service-select").selectOption("service:b");
      await expect(page.locator("#service-logs")).toContainText("Beta initial");
      await page.evaluate(() => { location.hash = "#map"; });
      await expect(page.locator("#service-logs .machine-log-panel")).toHaveCount(0);
      api.handlers.set("/api/diagnostics", route => route.fulfill({ json: { sources: [
        source("service:a", "Alpha", ["Alpha latest"]), source("service:b", "Beta", ["Beta latest"]),
      ] } }));
      release();
      await page.evaluate(() => { location.hash = "#diagnostics"; });
      await expect(page.locator("#service-logs")).toContainText("Beta latest");
      await expect(page.locator("#diagnostics-service-select")).toHaveValue("service:b");
      await expect(page.locator("#service-logs")).not.toContainText("Obsolete log");
      await expect(page.locator("#diagnostics-error")).toBeHidden();
    });
  }
}
