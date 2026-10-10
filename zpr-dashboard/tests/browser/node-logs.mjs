export function registerNodeLogTests(test, expect) {
  const nodes = () => [
    { cn: "node-a", display_name: "North Hub", node: true, node_details: {} },
    { cn: "node-b", display_name: "South Hub", node: true, node_details: {} },
  ];
  const source = (id, logs, extra = {}) => ({
    id: `node:${id}`, kind: "ZPR node", state: "available",
    logs: logs.map(body => ({ body })), metrics: [], ...extra,
  });
  const payload = sources => ({ state: "available", sources });
  const refresh = page => page.evaluate(() => document.dispatchEvent(new CustomEvent("control-room:refreshed")));
  const selectNode = (page, id) => page.locator(`#node-stats-nav [data-node-id="${id}"]`).click();
  const selectArea = (page, name) => page.getByRole("tablist", { name: "Node data areas" }).getByRole("tab", { name, exact: true }).click();

  test("Nodes uses a pulldown only when tabs overflow and preserves the active data area", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload([
      source("node-a", ["North logs"]), source("node-b", ["South logs"]),
    ]) }));
    await page.goto(appURL + "/#node-stats");
    await page.locator("#pause-poll").click();
    const container = page.locator("#node-stats-selector");
    const navigation = page.locator("#node-stats-nav");
    const selector = page.getByRole("combobox", { name: "Node information and logs" });
    await expect(navigation).toBeVisible();
    await selectArea(page, "Logs");
    const requiredWidth = await navigation.evaluate(element =>
      [...element.children].reduce((width, tab) => width + tab.getBoundingClientRect().width, 0)
      + parseFloat(getComputedStyle(element).gap) * (element.children.length - 1));
    const setWidth = width => container.evaluate((element, value) => {
      element.style.flex = "none";
      element.style.width = `${value}px`;
    }, width);
    await setWidth(Math.ceil(requiredWidth) + 2);
    await expect(selector).toBeHidden();
    await setWidth(Math.floor(requiredWidth) - 2);
    await expect(selector).toBeVisible();
    await expect(navigation).toBeHidden();
    await selector.selectOption("node-b");
    await expect(page.locator("#node-logs .machine-log-output")).toHaveText("South logs");
    await expect(page.locator("#node-area-logs")).toHaveAttribute("aria-selected", "true");
    await expect(page.locator("#node-stats-summary")).toBeHidden();
    await selector.focus();
    await setWidth(Math.ceil(requiredWidth) + 2);
    await expect(navigation).toBeVisible();
    await expect(navigation.locator('[data-node-id="node-b"]')).toBeFocused();
    await expect(navigation.locator('[data-node-id="node-b"]')).toHaveAttribute("aria-selected", "true");
    await selectNode(page, "node-a");
    await expect(page.locator("#node-area-logs")).toHaveAttribute("aria-selected", "true");
    await container.evaluate(element => element.removeAttribute("style"));
    api.snapshot.actors = Array.from({ length: 16 }, (_, index) => ({
      cn: `node-${index}`, display_name: `Production node ${index}`, node: true, node_details: {},
    }));
    await page.locator("#refresh-now").click();
    await expect(selector).toBeVisible();
    await expect(selector.locator("option")).toHaveCount(16);
    await selectArea(page, "Counters");
    await selector.selectOption("node-15");
    await expect(page.locator("#node-area-counters")).toHaveAttribute("aria-selected", "true");
    await expect(page.locator("#node-stats-groups")).toBeVisible();
    await page.locator("#refresh-now").click();
    await expect(selector).toHaveValue("node-15");
    api.snapshot.actors = [{ cn: "node-15", display_name: "Short", node: true, node_details: {} }];
    await page.locator("#refresh-now").click();
    await expect(navigation).toBeVisible();
    await expect(navigation.getByRole("tab", { name: "Short" })).toHaveAttribute("aria-selected", "true");
    await expect(page.locator("#node-area-counters")).toHaveAttribute("aria-selected", "true");
    api.snapshot.actors[0].display_name = "Long node name ".repeat(20);
    await page.locator("#refresh-now").click();
    await expect(selector).toBeVisible();
    await expect(selector).toHaveValue("node-15");
    api.snapshot.actors = [];
    await page.locator("#refresh-now").click();
    await expect(selector).toBeHidden();
    await expect(navigation).toBeHidden();
  });

  test("Nodes separates data areas with horizontal tabs and retains log position without extra requests", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes().map(node => ({ ...node, node_details: {
      counters_updated_at: new Date().toISOString(),
      counters: [{ group: "management", name: "visa_requested", value: "42" }],
    } }));
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload([
      source("node-a", Array.from({ length: 100 }, (_, index) => `North ${index}`), {
        name: "North Hub", metrics: [{ name: "Packets", value: "42", unit: "packets" }],
      }),
      source("node-b", ["South only"], { name: "South Hub", metrics: [] }),
    ]) }));
    await page.goto(appURL + "/#node-stats");
    await page.locator("#pause-poll").click();
    const tabs = page.getByRole("tablist", { name: "Node data areas" });
    await expect(tabs.getByRole("tab")).toHaveText(["Overview", "Counters", "Health & metrics", "Logs"]);
    await expect(page.locator("#node-stats-summary")).toBeVisible();
    for (const id of ["node-stats-groups", "node-source-info", "node-logs"]) {
      await expect(page.locator(`#${id}`)).toBeHidden();
    }
    await tabs.getByRole("tab", { name: "Overview", exact: true }).focus();
    await page.keyboard.press("ArrowRight");
    await expect(tabs.getByRole("tab", { name: "Counters", exact: true })).toBeFocused();
    await expect(page.locator("#node-stats-summary")).toBeHidden();
    await expect(page.getByRole("table", { name: "Management counters" })).toContainText("42");
    await selectArea(page, "Health & metrics");
    await expect(page.getByRole("region", { name: "Selected node source health and metrics" })).toContainText("Packets");
    await expect(page.locator("#node-stats-groups")).toBeHidden();
    await selectArea(page, "Logs");
    const panel = page.locator("#node-logs .machine-log-panel");
    const output = panel.locator(".machine-log-output");
    await expect(output).toContainText("North 99");
    await panel.getByRole("button", { name: "Pause node log updates" }).click();
    await output.evaluate(element => { element.scrollTop = 80; element.dispatchEvent(new Event("scroll")); });
    const requests = api.counts.get("/api/diagnostics");
    await selectArea(page, "Overview");
    await expect(page.locator("#node-logs")).toBeHidden();
    await selectArea(page, "Logs");
    expect(await output.evaluate(element => element.scrollTop)).toBeCloseTo(80, 0);
    expect(api.counts.get("/api/diagnostics")).toBe(requests);
    await selectNode(page, "node-b");
    await expect(tabs.getByRole("tab", { name: "Logs", exact: true })).toHaveAttribute("aria-selected", "true");
    await panel.getByRole("button", { name: "Refresh node logs" }).click();
    await expect(output).toHaveText("South only");
    await selectNode(page, "node-a");
    await expect(output).toContainText("North 99");
    expect(await output.evaluate(element => element.scrollTop)).toBeCloseTo(80, 0);
    await page.locator("#refresh-now").click();
    await expect(tabs.getByRole("tab", { name: "Logs", exact: true })).toHaveAttribute("aria-selected", "true");
    await tabs.getByRole("tab", { name: "Logs", exact: true }).focus();
    await page.keyboard.press("Home");
    await expect(tabs.getByRole("tab", { name: "Overview", exact: true })).toBeFocused();
    await page.keyboard.press("End");
    await expect(tabs.getByRole("tab", { name: "Logs", exact: true })).toBeFocused();
    for (const width of [1280, 820, 390]) {
      await page.setViewportSize({ width, height: 900 });
      expect(await tabs.evaluate(element => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth;
      })).toBe(true);
      expect(await output.evaluate(element => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth;
      })).toBe(true);
    }
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  test("Nodes owns source health and exact sortable metrics while Services retains only service details and health", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    let value = "18446744073709551615";
    let mode = "available";
    api.handlers.set("/api/diagnostics", route => {
      if (mode === "offline") return route.fulfill({ status: 503, json: { error: "Production provider offline" } });
      const sources = [
        source("node-a", ["North logs"], { name: "North Hub", identity: "node-a", address: "fd00::1",
          state: mode, last_updated: "2026-10-09T20:00:00Z",
          metrics: [{ name: "Huge", value, unit: "packets" }, { name: "Small", value: "9", unit: "packets" }] }),
        source("node-b", ["South logs"], { name: "South Hub", state: mode, metrics: [{ name: "South-only", value: "2", unit: "bytes" }] }),
        source("missing-node", ["Unknown node logs"], { name: "Missing node" }),
        { id: "service:visa", name: "Visa Service", kind: "Required service", state: "available",
          metrics: [{ name: "Service-only", value: "4", unit: "requests" }], logs: [{ body: "Service retained" }] },
      ];
      return route.fulfill({ json: { state: "stale", sources } });
    });
    await page.goto(appURL + "/#diagnostics");
    await expect(page.locator('[data-page-link="diagnostics"]')).toHaveText("Services");
    await expect(page.locator("#page-diagnostics")).toHaveAttribute("aria-label", "Services");
    await expect(page.locator("#diagnostics-aggregate")).toHaveText("Service telemetry: 1 current · 0 stale, partial, or unavailable.");
    await expect(page.locator("#diagnostics-count")).toHaveText("1 / 1 services");
    await expect(page.locator("#diagnostics-sources")).toContainText("Service retained");
    await expect(page.locator("#diagnostics-sources")).toContainText("Service-only");
    await expect(page.locator("#diagnostics-sources")).not.toContainText("North logs");
    await expect(page.locator("#diagnostics-sources")).not.toContainText("Huge");
    await expect(page.locator(".diagnostics-source-row")).toHaveCount(1);
    await expect(page.locator("#diagnostics-sources")).not.toContainText("Missing node");
    await expect(page.locator("#diagnostics-sources")).not.toContainText("Unknown node logs");
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Health & metrics");
    const details = page.getByRole("region", { name: "Selected node source health and metrics" });
    await expect(details).toContainText("fd00::1");
    await expect(details).toContainText(value);
    const table = details.getByRole("table");
    await table.getByRole("button", { name: /^Sort by Value/ }).click();
    await expect(table.locator("tbody tr").first()).toContainText("Small");
    await table.getByRole("button", { name: /^Sort by Value/ }).click();
    await expect(table.locator("tbody tr").first()).toContainText("Huge");
    value = "18446744073709551614";
    await refresh(page);
    await expect(details).toContainText(value);
    await expect(details.locator(".diagnostics-metric-value.poll-changed")).toHaveCount(1);
    await expect(table.locator("tbody tr").first()).toContainText("Huge");
    mode = "offline";
    await refresh(page);
    await expect(details).toContainText("Production provider offline");
    await expect(details).toContainText("Last known; source unavailable");
    await expect(details).toContainText(value);
    await selectNode(page, "node-b");
    await expect(details).not.toContainText(value);
    await expect(details).not.toContainText("South-only");
    mode = "available";
    await selectArea(page, "Logs");
    await page.locator("#node-logs").getByRole("button", { name: "Refresh node logs" }).click();
    await selectArea(page, "Health & metrics");
    await expect(details).toContainText("South-only");
    await expect(details).not.toContainText("Huge");
    mode = "stale";
    await refresh(page);
    await expect(details).toContainText("Last known; source stale");
    await page.evaluate(() => { location.hash = "#diagnostics"; });
    await expect(details).toHaveCount(0);
    await expect(page.locator("#diagnostics-sources")).toContainText("Service retained");
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  test("Nodes rejects malformed metrics and retains only the selected node last-good sample", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    let metrics = [{ name: "Valid", value: "42", unit: "bytes" }];
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload([source("node-a", ["Last good"], { metrics })]) }));
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Health & metrics");
    const details = page.getByRole("region", { name: "Selected node source health and metrics" });
    await expect(details).toContainText("42");
    metrics = [{ name: "Bad", value: { injected: true } }];
    await refresh(page);
    await expect(details).toContainText("invalid metrics");
    await expect(details).toContainText("42");
    await expect(details).not.toContainText("Bad");
    await selectArea(page, "Logs");
    await expect(page.locator("#node-logs .machine-log-output")).toContainText("Last good");
  });

  test("Nodes clears health metrics and logs on session loss and rejects a delayed sample", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    let reads = 0;
    api.handlers.set("/api/diagnostics", async route => {
      if (++reads > 1) await gate;
      await route.fulfill({ json: payload([source("node-a", ["Private node log"], {
        identity: "Private identity", metrics: [{ name: "Private metric", value: "42" }],
      })]) });
    });
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Health & metrics");
    const details = page.getByRole("region", { name: "Selected node source health and metrics" });
    await expect(details).toContainText("Private metric");
    await refresh(page);
    await expect.poll(() => reads).toBe(2);
    await page.evaluate(() => window.dispatchEvent(new Event("operator-session-cleared")));
    release();
    await expect(details).toContainText("Operator session ended");
    await expect(details).not.toContainText("Private");
    await expect(page.locator("#node-logs")).not.toContainText("Private");
    await page.locator('[data-page-link="map"]').click();
    await page.locator('[data-page-link="node-stats"]').click();
    await expect(details).toContainText("Private metric");
  });

  test("Nodes retains healthy metrics independently of partial logs and keeps provider text inert at narrow widths", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    const unsafe = '<img src=x onerror="window.injectedMetric=true">';
    let response = payload([source("node-a", ["Healthy logs"], {
      identity: unsafe, metrics: [{ name: unsafe, value: "42", unit: unsafe }],
    })]);
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: response }));
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Health & metrics");
    const details = page.getByRole("region", { name: "Selected node source health and metrics" });
    await expect(details).toContainText("42");
    await expect(details.locator("img, script, a")).toHaveCount(0);
    for (const width of [1280, 820, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await expect.poll(() => details.evaluate(section => {
        const rect = section.getBoundingClientRect();
        return rect.left >= 0 && rect.right <= innerWidth;
      })).toBe(true);
    }
    response = payload([source("node-a", ["Partial logs"], {
      state: "partial", error: "Metrics provider unavailable", metrics: [],
    })]);
    await refresh(page);
    await expect(details).toContainText("Last known; source unavailable");
    await expect(details).toContainText("42");
    await selectArea(page, "Logs");
    await expect(page.locator("#node-logs .machine-log-output")).toContainText("Partial logs");
    await selectArea(page, "Health & metrics");
    api.handlers.set("/api/diagnostics", route => route.fulfill({ status: 503, json: { error: "Provider offline" } }));
    await refresh(page);
    await expect(details).toContainText("Provider offline");
    await expect(details).toContainText("42");
    await selectArea(page, "Logs");
    await expect(page.locator("#node-logs .machine-log-output")).toContainText("Partial logs");
    expect(await page.evaluate(() => window.injectedMetric)).toBeUndefined();
  });

  test("Nodes uses the shared log panel for the selected production node without Simulator", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload([
      source("node-a", ["2026-10-09 INFO \u001b[32mconnected", '{"id":18446744073709551615}', "<script>unsafe</script>"]),
      source("node-b", ["South only"]),
      { id: "service:visa", kind: "Required service", state: "available", logs: [{ body: "Service only" }] },
    ]) }));
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Logs");
    const panel = page.locator("#node-logs .machine-log-panel");
    await expect(panel.locator("h2")).toHaveText("Logs for North Hub");
    const output = panel.locator(".machine-log-output");
    await expect(output).toContainText("2026-10-09 INFO connected");
    await expect(output).toContainText('{"id":18446744073709551615}');
    await expect(output).toContainText("<script>unsafe</script>");
    await expect(output.locator("script, a, time, .diagnostics-log-severity")).toHaveCount(0);
    await expect(output.locator("pre span")).toHaveCSS("color", "rgb(0, 187, 0)");
    expect(await output.locator("pre").evaluate(element =>
      [...element.childNodes].filter(node => node.nodeType === Node.TEXT_NODE).map(node => node.textContent).join("")
    )).toContain('{"id":18446744073709551615}');
    await expect(output).toHaveCSS("background-color", "rgb(0, 0, 0)");
    await expect(output).not.toContainText("South only");
    await expect(output).not.toContainText("Service only");
    expect(await page.evaluate(() => performance.getEntriesByType("resource")
      .filter(entry => new URL(entry.name).pathname === "/api/diagnostics")
      .map(entry => new URL(entry.name).searchParams.get("source"))
    )).toContain("node:node-a");
    await expect(panel.getByText("Format JSON", { exact: true })).toHaveCount(0);
    await expect(output).toHaveCSS("white-space", "pre");
    await panel.getByLabel("Wrap lines").check();
    await expect(output).toHaveCSS("white-space", "pre-wrap");
    await selectNode(page, "node-b");
    await expect(panel.locator("h2")).toHaveText("Logs for South Hub");
    await expect(output).toHaveText("South only");
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  test("Nodes log panel preserves scroll and follow state, standard icons and navigation restore", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    let count = 100;
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload([
      source("node-a", Array.from({ length: count }, (_, i) => `North entry ${i}`)),
      source("node-b", ["South"]),
    ]) }));
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Logs");
    const panel = page.locator("#node-logs .machine-log-panel");
    const output = panel.locator(".machine-log-output");
    await expect(output).toContainText("North entry 99");
    const bottom = () => output.evaluate(element => element.scrollHeight - element.clientHeight - element.scrollTop);
    await expect.poll(bottom).toBeLessThanOrEqual(8);
    await output.evaluate(element => { element.scrollTop = 80; element.dispatchEvent(new Event("scroll")); });
    count = 110;
    await refresh(page);
    await expect(output).toContainText("North entry 109");
    expect(await output.evaluate(element => element.scrollTop)).toBeCloseTo(80, 0);
    await selectNode(page, "node-b");
    await expect(output).toHaveText("South");
    await selectNode(page, "node-a");
    await expect(output).toContainText("North entry 109");
    expect(await output.evaluate(element => element.scrollTop)).toBeCloseTo(80, 0);
    await output.evaluate(element => { element.scrollTop = element.scrollHeight; element.dispatchEvent(new Event("scroll")); });
    count = 120;
    await refresh(page);
    await expect(output).toContainText("North entry 119");
    await expect.poll(bottom).toBeLessThanOrEqual(8);
    const maximize = panel.getByRole("button", { name: "Maximize node logs for North Hub" });
    await expect(maximize.locator('svg[data-window-control="maximize"]')).toHaveCount(1);
    await maximize.click();
    await expect(panel).toHaveClass(/maximized/);
    await expect(panel.getByRole("button", { name: "Restore node logs for North Hub" })).toHaveAttribute("aria-pressed", "true");
    await page.keyboard.press("Escape");
    await expect(panel).not.toHaveClass(/maximized/);
    await expect(maximize).toBeFocused();
    await maximize.click();
    await page.evaluate(() => { location.hash = "#diagnostics"; });
    await expect(panel).toHaveCount(0);
    await expect(page.locator("body")).not.toHaveClass(/machine-log-maximized/);
    await page.locator('[data-page-link="node-stats"]').click();
    await expect(output).toContainText("North entry 119");
    await page.setViewportSize({ width: 700, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });

  test("Nodes log panel shows initial Loading, retains logs on failure and allows paused manual refresh", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    api.handlers.set("/api/diagnostics", async route => {
      await gate;
      await route.fulfill({ json: payload([source("node-a", ["Last good"])]) });
    });
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Logs");
    const panel = page.locator("#node-logs .machine-log-panel");
    const output = panel.locator(".machine-log-output");
    await expect(panel.locator("header small")).toHaveText("Loading");
    release();
    await expect(output).toHaveText("Last good");
    await expect(panel.locator("header small")).not.toHaveText("Loading");
    await panel.getByRole("button", { name: "Pause node log updates" }).click();
    const before = api.counts.get("/api/diagnostics");
    await refresh(page);
    await page.waitForTimeout(100);
    expect(api.counts.get("/api/diagnostics")).toBe(before);
    api.handlers.set("/api/diagnostics", route => route.fulfill({ status: 503, json: { error: "Provider offline" } }));
    await panel.getByRole("button", { name: "Refresh node logs" }).click();
    await expect(output).toContainText("Provider offline");
    await expect(output).toContainText("Displayed logs are last known.");
    await expect(output).toContainText("Last good");
    await expect(panel.locator("header small")).toHaveText("Paused");
    let finish;
    const nextGate = new Promise(resolve => { finish = resolve; });
    api.handlers.set("/api/diagnostics", async route => {
      await nextGate;
      await route.fulfill({ json: payload([source("node-a", ["Recovered"])]) });
    });
    await panel.getByRole("button", { name: "Refresh node logs" }).click();
    await expect(output).toContainText("Last good");
    await expect(panel.locator("header small")).not.toHaveText("Loading");
    finish();
    await expect(output).toHaveText("Recovered");
    await panel.getByRole("button", { name: "Resume node log updates" }).click();
    await expect(panel.locator("header small")).toHaveText("available");
  });

  for (const outcome of ["success", "failure"]) {
    test(`Nodes rejects obsolete log ${outcome} after selection and navigation`, async ({ page, appURL, api }) => {
      api.snapshot.actors = nodes();
      let release;
      const gate = new Promise(resolve => { release = resolve; });
      let requests = 0;
      api.handlers.set("/api/diagnostics", async route => {
        if (++requests === 1) {
          await gate;
          await route.fulfill(outcome === "success"
            ? { json: payload([source("node-a", ["OBSOLETE"], { identity: "OBSOLETE", metrics: [{ name: "OBSOLETE", value: "999" }] })]) }
            : { status: 503, json: { error: "OBSOLETE failure" } });
        } else await route.fulfill({ json: payload([source("node-b", ["Current South"], { metrics: [{ name: "Current metric", value: "2" }] })]) });
      });
      await page.goto(appURL + "/#node-stats");
      await selectArea(page, "Logs");
      await expect(page.locator("#node-logs header small")).toHaveText("Loading");
      await selectNode(page, "node-b");
      await expect(page.locator("#node-logs .machine-log-output")).toHaveText("Current South");
      await page.locator('[data-page-link="map"]').click();
      release();
      await page.locator('[data-page-link="node-stats"]').click();
      await expect(page.locator("#node-logs .machine-log-output")).toHaveText("Current South");
      await expect(page.locator("#node-logs")).not.toContainText("OBSOLETE");
      await selectArea(page, "Health & metrics");
      await expect(page.getByRole("region", { name: "Selected node source health and metrics" })).toContainText("Current metric");
      await expect(page.getByRole("region", { name: "Selected node source health and metrics" })).not.toContainText("OBSOLETE");
      expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
    });
  }

  test("Nodes log source absence, stale/unavailable and invalid responses stay explicit and node scoped", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    let response = payload([source("node-a", ["Stale North"], { state: "stale" })]);
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: response }));
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Logs");
    const output = page.locator("#node-logs .machine-log-output");
    await expect(output).toContainText("Log sample is stale");
    await expect(output).toContainText("Stale North");
    response = payload([source("node-a", [], { state: "unavailable", error: "No signals" })]);
    await refresh(page);
    await expect(output).toContainText("No signals");
    await expect(output).toContainText("Stale North");
    response = payload([source("node-a", ["Partial North"], { state: "partial", error: "Metrics unavailable" })]);
    await refresh(page);
    await expect(output).toContainText("Metrics unavailable");
    await expect(output).toContainText("Partial North");
    api.handlers.set("/api/diagnostics", route => route.fulfill({ status: 503, json: { error: "Query failed" } }));
    await refresh(page);
    await expect(output).toContainText("Query failed");
    await expect(output).toContainText("Partial North");
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: response }));
    await selectNode(page, "node-b");
    await expect(output).toContainText("Selected node is absent");
    await expect(output).not.toContainText("Stale North");
    response = {};
    await refresh(page);
    await expect(output).toContainText("Node logs response has no source inventory.");
    response = payload([source("node-b", [42])]);
    await refresh(page);
    await expect(output).toContainText("Node logs response contains invalid log records.");
    api.snapshot.actors = [];
    await page.evaluate(snapshot => window.dispatchEvent(new CustomEvent("zpr-snapshot", { detail: snapshot })), api.snapshot);
    await expect(page.locator("#node-logs .machine-log-panel")).toBeHidden();
  });

  test("Shared adapter log panels preserve type selection and per-source scroll using the same viewer", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/#adapter-logs");
    const columns = page.locator(".adapter-log-column");
    await expect(columns).toHaveCount(1);
    await page.locator("#machine-logs-pause").click();
    const choose = async (panel, name, type = "adapter") => {
      await panel.getByRole("button", { name: new RegExp(`Choose ${type} and log source`) }).click();
      await panel.getByRole("dialog", { name: `Choose ${type} log source` }).getByRole("listbox").selectOption({ label: name });
    };
    const first = columns.first();
    await choose(first, "finance-client adapter · machine-first");
    const output = first.locator(".machine-log-output");
    await expect(output).toContainText("finance-client adapter entry 79");
    await output.evaluate(element => { element.scrollTop = 40; element.dispatchEvent(new Event("scroll")); });
    await page.getByRole("button", { name: "Add adapter panel", exact: true }).click();
    await expect(columns).toHaveCount(2);
    await page.getByRole("button", { name: "Controller logs", exact: true }).click();
    await choose(first, "Controller · machine-second", "controller");
    await expect(output).toContainText("Controller entry 79");
    await output.evaluate(element => { element.scrollTop = 70; element.dispatchEvent(new Event("scroll")); });
    await page.getByRole("button", { name: "Adapter logs", exact: true }).click();
    await expect(first.locator("select")).toHaveValue("machine-first\u001ffinance-client adapter");
    expect(await output.evaluate(element => element.scrollTop)).toBeCloseTo(40, 0);
    await page.getByRole("button", { name: "Controller logs", exact: true }).click();
    await expect(first.locator("select")).toHaveValue("machine-second\u001fController");
    expect(await output.evaluate(element => element.scrollTop)).toBeCloseTo(70, 0);
    await columns.nth(1).getByRole("button", { name: "Remove controller panel" }).click();
    await expect(columns).toHaveCount(1);
    expect(api.counts.get("/api/adapter-logs")).toBe(1);
    await expect(page.locator("#node-logs .machine-log-panel")).toHaveCount(0);
  });

  test("Nodes log initial request failure recovers without manufacturing entries", async ({ page, appURL, api }) => {
    api.snapshot.actors = nodes();
    api.handlers.set("/api/diagnostics", route => route.fulfill({ status: 503, json: { error: "Provider not configured" } }));
    await page.goto(appURL + "/#node-stats");
    await selectArea(page, "Logs");
    const panel = page.locator("#node-logs .machine-log-panel");
    await expect(panel.locator(".machine-log-error")).toHaveText("Provider not configured");
    await expect(panel.locator("pre")).toHaveCount(0);
    await expect(panel.locator("header small")).toHaveText("Unavailable");
    api.handlers.set("/api/diagnostics", route => route.fulfill({ json: payload([source("node-a", ["Recovered"])]) }));
    await panel.getByRole("button", { name: "Refresh node logs" }).click();
    await expect(panel.locator("pre")).toHaveText("Recovered");
    await expect(panel.locator(".machine-log-error")).toHaveCount(0);
  });
}
