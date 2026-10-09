export function registerDiagnosticsGuidelineTests(test, expect) {
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
