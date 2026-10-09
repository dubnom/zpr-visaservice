export function registerDiagnosticsGuidelineTests(test, expect) {
  for (const firstResult of ["success", "failure"]) {
    test(`Diagnostics shows initial Loading then removes status chatter after ${firstResult}`, async ({ page, appURL, api }) => {
      let release;
      const gate = new Promise(resolve => { release = resolve; });
      const payload = { state: "available", generated_at: "2026-10-09T19:45:00Z",
        sources: [{ id: "node:one", name: "Production node", kind: "ZPR node", metrics: [], logs: [{ body: "Live log" }] }] };
      api.handlers.set("/api/diagnostics", async route => {
        await gate;
        await route.fulfill(firstResult === "success" ? { json: payload } : { status: 503, json: { error: "Provider unavailable" } });
      });
      await page.goto(appURL + "/#diagnostics");
      const loading = page.locator("#diagnostics-loading");
      await expect(loading).toBeVisible();
      await expect(loading).toHaveText("Loading");
      await page.locator("#pause-poll").click();
      release();
      await expect(loading).toBeHidden();
      if (firstResult === "failure") {
        await expect(page.locator("#diagnostics-error")).toHaveText("Provider unavailable");
      }
      api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload }));
      await page.locator("#refresh-now").click();
      await expect(page.locator(".diagnostics-log-body")).toHaveText("Live log");
      await expect(page.locator("#diagnostics-error")).toBeHidden();
      await expect(loading).toBeHidden();
      let finish;
      const refreshGate = new Promise(resolve => { finish = resolve; });
      api.handlers.set("/api/diagnostics", async route => {
        await refreshGate;
        await route.fulfill({ status: 503, json: { error: "Refresh unavailable" } });
      });
      await page.locator("#refresh-now").click();
      await expect(loading).toBeHidden();
      await expect(page.locator(".diagnostics-log-body")).toHaveText("Live log");
      finish();
      await expect(page.locator("#diagnostics-error")).toHaveText("Refresh unavailable");
      await expect(loading).toBeHidden();
      await expect(page.locator("#diagnostics-updated, .diagnostics-header")).toHaveCount(0);
      await expect(page.locator("#page-diagnostics")).not.toContainText(/Querying|Updated/);
    });
  }

  test("Diagnostics uses available page width and uncapped log height without Format JSON", async ({ page, appURL, api }) => {
    const logs = Array.from({ length: 100 }, (_, index) => ({ body: `2026-10-09 INFO entry ${index} ${"details ".repeat(12)}` }));
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: {
      state: "available", generated_at: "2026-10-09T19:40:00Z",
      sources: [{ id: "node:one", name: "Production node", kind: "ZPR node",
        metrics: [{ name: "requests", value: "123" }], logs }],
    } }));
    await page.goto(appURL + "/#diagnostics");
    await expect(page.locator(".diagnostics-log-body")).toHaveCount(100);
    await expect(page.locator("#diagnostics-json")).toHaveCount(0);
    await expect(page.locator("#page-diagnostics").getByText("Format JSON", { exact: true })).toHaveCount(0);
    for (const width of [2400, 1280, 820, 700]) {
      await page.setViewportSize({ width, height: 1000 });
      const geometry = await page.evaluate(() => {
        const main = document.querySelector("#app-main").getBoundingClientRect();
        const table = document.querySelector(".diagnostics-table-scroll");
        const logs = document.querySelector(".diagnostics-logs");
        const metrics = document.querySelector(".diagnostics-metrics-table");
        const details = metrics.parentElement;
        const style = getComputedStyle(details);
        return {
          rightGap: innerWidth - main.right,
          logHeight: logs.getBoundingClientRect().height,
          logCapped: getComputedStyle(logs).maxHeight,
          tableCapped: getComputedStyle(table).maxHeight,
          metricWidth: metrics.getBoundingClientRect().width,
          available: details.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight),
          noPageOverflow: document.documentElement.scrollWidth <= innerWidth,
        };
      });
      expect(geometry.rightGap).toBeLessThanOrEqual(1);
      expect(geometry.logHeight).toBeGreaterThan(220);
      expect(geometry.logCapped).toBe("none");
      expect(geometry.tableCapped).toBe("none");
      expect(geometry.metricWidth).toBeCloseTo(geometry.available, 0);
      expect(geometry.noPageOverflow).toBe(true);
    }
    await page.locator("#diagnostics-filter").fill("entry 99");
    await expect(page.locator(".diagnostics-log-body")).toHaveCount(100);
    await expect(page.locator(".diagnostics-log-body").last()).toContainText("entry 99");
    await page.locator("#diagnostics-filter").fill("absent message");
    await expect(page.locator(".diagnostics-source-row")).toHaveCount(0);
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  test("Diagnostics log entries use full-width body text without duplicate timestamp or severity columns", async ({ page, appURL, api }) => {
    const body = "2026-10-09 19:35:00 INFO \u001b[32mAdapter connected\u001b[0m\nContinuation details";
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: {
      state: "available", generated_at: "2026-10-09T19:35:00Z",
      sources: [{ id: "node:one", name: "Production node", kind: "ZPR node", metrics: [], logs: [
        { timestamp: "2026-10-09T19:35:00Z", severity: "INFO", body },
        { timestamp: "2026-10-09T19:35:01Z", severity: "WARN", body: "" },
      ] }],
    } }));
    await page.goto(appURL + "/#diagnostics");
    const logs = page.locator(".diagnostics-logs");
    const rows = logs.locator("li");
    await expect(rows).toHaveCount(2);
    await expect(logs.locator("time, .diagnostics-log-time, .diagnostics-log-severity")).toHaveCount(0);
    await expect(rows.first()).toHaveText("2026-10-09 19:35:00 INFO Adapter connected\nContinuation details");
    await expect(rows.nth(1)).toHaveText("No log body reported.");
    await expect(rows.first().locator(".diagnostics-log-body span")).toHaveCSS("color", "rgb(0, 187, 0)");
    const fullWidth = async () => {
      const widths = await rows.first().evaluate(row => ({
        row: row.getBoundingClientRect().width,
        body: row.querySelector(".diagnostics-log-body").getBoundingClientRect().width,
        children: row.children.length,
      }));
      expect(widths.children).toBe(1);
      expect(widths.body).toBeCloseTo(widths.row, 0);
    };
    await fullWidth();
    await page.setViewportSize({ width: 700, height: 900 });
    await fullWidth();
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  test("Diagnostics respects ANSI log colors and styles without interpreting provider HTML or links", async ({ page, appURL, api }) => {
    const colored = "\u001b[31mred\u001b[0m plain \u001b[1;32mbold green\u001b[0m \u001b[38;2;12;34;56mtrue color\u001b[0m \u001b[44mblue background\u001b[0m";
    const unsafe = '<img src=x onerror="window.injectedLog=true"><script>window.injectedLog=true</script>';
    const link = "\u001b]8;;https://example.test/\u0007link text\u001b]8;;\u0007";
    let logs = [colored, "plain next record", unsafe, link, '{"id":18446744073709551615,"ok":true}'];
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: {
      state: "available", generated_at: "2026-10-09T19:30:00Z",
      sources: [{ id: "node:one", name: "Production node", kind: "ZPR node", metrics: [], logs: logs.map(body => ({ body, severity: "INFO" })) }],
    } }));
    await page.goto(appURL + "/#diagnostics");
    await page.locator("#pause-poll").click();
    const bodies = page.locator(".diagnostics-log-body");
    await expect(bodies).toHaveCount(5);
    const checkColors = async () => {
      await expect(bodies.nth(0)).toHaveText("red plain bold green true color blue background");
      await expect(bodies.nth(0).locator("span").filter({ hasText: /^red$/ })).toHaveCSS("color", "rgb(187, 0, 0)");
      const green = bodies.nth(0).locator("span").filter({ hasText: /^bold green$/ });
      await expect(green).toHaveCSS("color", "rgb(0, 187, 0)");
      await expect(green).toHaveCSS("font-weight", "700");
      await expect(bodies.nth(0).locator("span").filter({ hasText: /^true color$/ })).toHaveCSS("color", "rgb(12, 34, 56)");
      await expect(bodies.nth(0).locator("span").filter({ hasText: /^blue background$/ })).toHaveCSS("background-color", "rgb(0, 0, 187)");
      await expect(bodies.nth(1)).toHaveText("plain next record");
      await expect(bodies.nth(1).locator("span")).toHaveCount(0);
      await expect(bodies.nth(2)).toHaveText(unsafe);
      await expect(bodies.nth(2).locator("img, script")).toHaveCount(0);
      await expect(bodies.nth(3)).toHaveText("link text");
      await expect(bodies.nth(3).locator("a")).toHaveCount(0);
      expect(await page.evaluate(() => window.injectedLog)).toBeUndefined();
    };
    await checkColors();
    await expect(page.locator("#diagnostics-json")).toHaveCount(0);
    await expect(bodies.nth(4)).toHaveText('{"id":18446744073709551615,"ok":true}');
    logs = ["\u001b[33mnew warning\u001b[0m"];
    await page.locator("#refresh-now").click();
    await expect(bodies).toHaveCount(1);
    await expect(bodies.locator("span")).toHaveCSS("color", "rgb(187, 187, 0)");
    await expect(bodies).toHaveText("new warning");
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  test("Diagnostics shows sortable production sources, expandable details, and change pulses", async ({ page, appURL, api }) => {
    await page.clock.install();
    const simulatorRequests = [];
    page.on("request", request => {
      if (new URL(request.url()).pathname.startsWith("/api/simulator/")) simulatorRequests.push(request.url());
    });
    await page.route("**/api/simulator/**", route => route.fulfill({
      status: 503, json: { error: "Simulator is unavailable" },
    }));

    let payload = {
      generated_at: "2026-10-09T14:00:00Z",
      state: "partial",
      sources: [
        {
          id: "node:z", name: "Zulu node", kind: "ZPR node", identity: "a-identity",
          address: "zpr://zulu", state: "stale", last_updated: "2026-10-09T13:59:00Z",
          metrics: [
            { name: "a-larger", value: "9007199254740993", unit: "events" },
            { name: "z-smaller", value: "9007199254740992", unit: "events" },
          ],
          logs: [{ timestamp: "2026-10-09T13:58:00Z", severity: "INFO", body: '{"started":true}' }],
        },
        {
          id: "node:a", name: "Alpha node", kind: "ZPR node", identity: "z-identity",
          state: "unavailable", error: "Production provider offline", metrics: [], logs: [],
        },
      ],
    };
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload }));

    await page.goto(`${appURL}/#diagnostics`);
    const overview = page.getByRole("table", { name: "Diagnostics source overview" });
    await expect(overview).toBeVisible();
    await expect(overview.locator(":scope > thead th")).toHaveText(["Source", "Identity", "Kind", "State", "Last update"]);
    const sourceRows = page.locator(".diagnostics-source-row");
    await expect(sourceRows).toHaveCount(2);
    await expect(sourceRows.nth(0)).toContainText("Alpha node");
    await expect(sourceRows.nth(1)).toContainText("Zulu node");
    await expect(sourceRows.nth(1)).toHaveAttribute("data-state", "stale");
    await expect(sourceRows.nth(1)).toContainText("stale");
    await expect(sourceRows.nth(0)).toContainText("unavailable");

    await overview.getByRole("button", { name: /Sort by Identity/ }).click();
    await expect(sourceRows.nth(0)).toContainText("Zulu node");
    await expect(sourceRows.nth(1)).toContainText("Alpha node");
    expect(await page.locator(".diagnostics-metric-value").evaluateAll(values =>
      values.every(value => value.getAnimations().length === 0))).toBe(true);

    const zuluDetails = page.locator('.diagnostics-details-row[data-source-key="node:z"]');
    await expect(zuluDetails).toBeVisible();
    await expect(page.getByRole("table", { name: "Metrics for Zulu node" }).getByRole("columnheader"))
      .toHaveText(["Metric", "Value", "Unit"]);
    await expect(zuluDetails.locator(".diagnostics-log-body")).toHaveText('{"started":true}');
    expect(await zuluDetails.locator(".diagnostics-metric-value").evaluateAll(values =>
      values.every(value => value.getAnimations().length === 0))).toBe(true);
    await page.getByRole("button", { name: "Hide details for Zulu node" }).click();
    await expect(zuluDetails).toBeHidden();
    await page.getByRole("button", { name: "Show details for Zulu node" }).click();
    await expect(zuluDetails).toBeVisible();

    await page.getByRole("checkbox", { name: "Format JSON", exact: true }).check();
    await expect(zuluDetails.locator(".diagnostics-log-body")).toHaveText('{\n  "started": true\n}');
    const countBeforeLocalChanges = api.counts.get("/api/diagnostics");
    await page.locator("#diagnostics-filter").fill("a-identity");
    await expect(sourceRows).toHaveCount(1);
    await expect(sourceRows.first()).toContainText("Zulu node");
    await expect(sourceRows.first().getByRole("button", { name: "Hide details for Zulu node" })).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("table", { name: "Metrics for Zulu node" })).toBeVisible();
    expect(api.counts.get("/api/diagnostics")).toBe(countBeforeLocalChanges);

    await page.getByRole("button", { name: /Sort by Value/ }).click();
    const metricRows = () => zuluDetails.locator(".diagnostics-metrics-table tbody tr");
    await expect(metricRows().nth(0)).toContainText("z-smaller");
    await expect(metricRows().nth(1)).toContainText("a-larger");
    expect(await zuluDetails.locator(".diagnostics-metric-value").evaluateAll(values =>
      values.every(value => value.getAnimations().length === 0))).toBe(true);

    const changedValue = "9007199254740994";
    const dispatchRefresh = () => page.evaluate(snapshot => {
      document.dispatchEvent(new CustomEvent("control-room:refreshed", { detail: snapshot }));
    }, api.snapshot);
    payload = {
      ...payload,
      generated_at: "2026-10-09T14:01:00Z",
      sources: payload.sources.map(source => source.id === "node:z"
        ? {
          ...source,
          last_updated: "2026-10-09T14:01:00Z",
          metrics: source.metrics.map(metric => metric.name === "z-smaller" ? { ...metric, value: changedValue } : metric),
        }
        : source),
    };
    await dispatchRefresh();
    await expect.poll(() => api.counts.get("/api/diagnostics")).toBe(countBeforeLocalChanges + 1);
    await expect(zuluDetails.locator(".diagnostics-metric-value").filter({ hasText: changedValue })).toBeVisible();
    await expect.poll(() => zuluDetails.locator(".diagnostics-metric-value").evaluateAll(values =>
      values.some(value => value.getAnimations().length > 0))).toBe(true);
    await expect(metricRows().nth(0)).toContainText("a-larger");
    await expect(metricRows().nth(1)).toContainText("z-smaller");
    await expect(sourceRows.first().getByRole("button", { name: "Hide details for Zulu node" })).toHaveAttribute("aria-expanded", "true");

    const refreshDiagnostics = async () => {
      const previousReads = api.counts.get("/api/diagnostics");
      await dispatchRefresh();
      await expect.poll(() => api.counts.get("/api/diagnostics")).toBe(previousReads + 1);
    };
    payload.sources = payload.sources.map(source => source.id === "node:a"
      ? { ...source, metrics: [{ name: "hidden metric", value: "1", unit: "items" }] }
      : source);
    await refreshDiagnostics();
    payload.sources = payload.sources.map(source => source.id === "node:a"
      ? { ...source, metrics: [{ name: "hidden metric", value: "2", unit: "items" }] }
      : source);
    await refreshDiagnostics();

    await page.locator("#diagnostics-filter").fill("");
    await expect(sourceRows).toHaveCount(2);
    await expect(sourceRows.nth(0)).toContainText("Zulu node");
    await expect(sourceRows.nth(1)).toContainText("Alpha node");
    await expect(sourceRows.nth(1)).toContainText("Not reported");
    const alphaDetails = page.locator('.diagnostics-details-row[data-source-key="node:a"]');
    await expect(alphaDetails).toBeVisible();
    await expect(alphaDetails).toContainText("Production provider offline");
    await expect(alphaDetails.locator(".diagnostics-metric-value")).toHaveText("2");
    await expect(alphaDetails).toContainText("No logs in the current window.");
    expect(await alphaDetails.locator(".diagnostics-metric-value").evaluate(value =>
      value.getAnimations().length === 0)).toBe(true);
    payload.sources = payload.sources.map(source => source.id === "node:a"
      ? { ...source, metrics: [{ name: "hidden metric", value: "3", unit: "items" }] }
      : source);
    await refreshDiagnostics();
    await expect.poll(() => alphaDetails.locator(".diagnostics-metric-value").evaluate(value =>
      value.getAnimations().length > 0)).toBe(true);
    expect(simulatorRequests).toEqual([]);
  });
}
