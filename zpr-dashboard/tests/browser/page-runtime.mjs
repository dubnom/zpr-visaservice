export function registerPageRuntimeTests(test, expect) {
  test("Shared page runtime retains Trusted Sources directory data on a failed manual read", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/#sources");
    const browser = page.locator("trusted-source-browser");
    await expect(browser.locator(".trusted-source-table")).toContainText("alice");
    await page.locator("#pause-poll").click();
    api.handlers.set("/api/assertions/source", route => route.fulfill({ status: 503, json: { error: "Directory provider unavailable" } }));
    await page.locator("#refresh-now").click();
    await expect(browser.locator("[data-source-message]")).toContainText("Directory provider unavailable");
    await expect(browser.locator("[data-source-message]")).toContainText("last known");
    await expect(browser.locator(".trusted-source-table")).toContainText("alice");
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  for (const completion of ["success", "failure"]) {
    test(`Shared page runtime rejects obsolete Trusted Sources ${completion} after navigation and manual return`, async ({ page, appURL, api }) => {
      await page.goto(appURL + "/#sources");
      const browser = page.locator("trusted-source-browser");
      await expect(browser.locator(".trusted-source-table")).toContainText("alice");
      await page.locator("#pause-poll").click();
      await page.evaluate(async () => {
        const original = window.zprOperatorFetch;
        const data = await (await original("/api/assertions/source")).json();
        window.delayedDirectory = [];
        window.zprOperatorFetch = (url, options) => {
          if (url !== "/api/assertions/source") return original(url, options);
          return new Promise((resolve, reject) => window.delayedDirectory.push({
            signal: options.signal,
            finish(name) { resolve(new Response(JSON.stringify({ ...data, default_source: name, source_name: name }), { status: 200 })); },
            fail() { reject(new Error("Obsolete directory failure")); },
          }));
        };
      });
      await page.locator("#refresh-now").click();
      await expect.poll(() => page.evaluate(() => window.delayedDirectory.length)).toBe(1);
      await page.evaluate(() => { location.hash = "#map"; });
      await expect.poll(() => page.evaluate(() => window.delayedDirectory[0].signal.aborted)).toBe(true);
      await page.evaluate(() => { location.hash = "#sources"; });
      await expect(browser.locator(".trusted-source-table")).toContainText("alice");
      expect(await page.evaluate(() => window.delayedDirectory.length)).toBe(1);
      await page.locator("#refresh-now").click();
      await expect.poll(() => page.evaluate(() => window.delayedDirectory.length)).toBe(2);
      await page.evaluate(() => window.delayedDirectory[1].finish("Current directory"));
      await expect(browser.locator("[data-source-title]")).toHaveText("Current directory");
      await page.evaluate(result => {
        if (result === "success") window.delayedDirectory[0].finish("Obsolete directory");
        else window.delayedDirectory[0].fail();
      }, completion);
      await expect(browser.locator("[data-source-title]")).toHaveText("Current directory");
      await expect(browser.locator("[data-source-message]")).toBeHidden();
    });
  }

  test("Shared page runtime retains DNS last-good rows on malformed production responses", async ({ page, appURL, api }) => {
    api.dnsRecords.records = [{ name: "worker.svc.zpr.", type: "AAAA", value: "fd00::1", ttl: 30 }];
    api.handlers.set("/api/dns/stats/json/v1/status", route => route.fulfill({ json: { boot_time: "2026-10-09T12:00:00Z" } }));
    api.handlers.set("/api/dns/stats/json/v1/server", route => route.fulfill({ json: { nsstats: { Requestv4: 123, QrySuccess: 122 } } }));
    api.handlers.set("/api/dns/stats/json/v1/zones", route => route.fulfill({ json: { views: {} } }));
    await page.goto(appURL + "/#dns");
    await expect(page.locator("#dns-record-rows")).toContainText("worker.svc.zpr.");
    await expect(page.locator("#dns-counter-rows")).toContainText("123");
    await page.locator("#pause-poll").click();
    const before = await page.locator("#dns-counter-rows").textContent();
    api.handlers.set("/api/dns/records", route => route.fulfill({ status: 200, contentType: "application/json", body: "malformed" }));
    api.handlers.set("/api/dns/stats/json/v1/server", route => route.fulfill({ status: 200, contentType: "application/json", body: "malformed" }));
    await page.locator("#refresh-now").click();
    await expect(page.locator("#dns-stats-status")).toContainText("invalid JSON response");
    await expect(page.locator("#dns-stats-status")).toContainText("last known");
    await expect(page.locator("#dns-record-status")).toContainText("invalid JSON response");
    await expect(page.locator("#dns-record-status")).toContainText("last known");
    await expect(page.locator("#dns-counter-rows")).toHaveText(before);
    await expect(page.locator("#dns-record-rows")).toContainText("worker.svc.zpr.");
    expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
  });

  for (const completion of ["success", "failure"]) {
    test(`Shared page runtime rejects obsolete Diagnostics ${completion} after navigation and return`, async ({ page, appURL, api }) => {
      api.handlers.set("/api/diagnostics", route => route.fulfill({ json: {
        state: "available", generated_at: "2026-10-09T12:00:00Z",
        sources: [{ id: "node:one", name: "Initial source", kind: "ZPR node", metrics: [], logs: [] }],
      } }));
      await page.goto(appURL + "/#diagnostics");
      await expect(page.locator(".diagnostics-source-row")).toContainText("Initial source");
      await page.locator("#pause-poll").click();
      await page.evaluate(() => {
        const original = window.zprOperatorFetch;
        window.delayedTelemetry = [];
        window.zprOperatorFetch = (url, options) => {
          if (url !== "/api/diagnostics") return original(url, options);
          return new Promise((resolve, reject) => window.delayedTelemetry.push({
            signal: options.signal,
            finish(name) { resolve(new Response(JSON.stringify({
              state: "available", generated_at: "2026-10-09T12:01:00Z",
              sources: [{ id: "node:one", name, kind: "ZPR node", metrics: [], logs: [] }],
            }), { status: 200 })); },
            fail() { reject(new Error("Obsolete provider failure")); },
          }));
        };
      });
      await page.locator("#refresh-now").click();
      await expect.poll(() => page.evaluate(() => window.delayedTelemetry.length)).toBe(1);
      await page.evaluate(() => { location.hash = "#map"; });
      await expect.poll(() => page.evaluate(() => window.delayedTelemetry[0].signal.aborted)).toBe(true);
      await page.evaluate(() => { location.hash = "#diagnostics"; });
      await expect.poll(() => page.evaluate(() => window.delayedTelemetry.length)).toBe(2);
      await page.evaluate(() => window.delayedTelemetry[1].finish("Current source"));
      await expect(page.locator(".diagnostics-source-row")).toContainText("Current source");
      await page.evaluate(result => {
        if (result === "success") window.delayedTelemetry[0].finish("Obsolete source");
        else window.delayedTelemetry[0].fail();
      }, completion);
      await expect(page.locator(".diagnostics-source-row")).toContainText("Current source");
      await expect(page.locator("#diagnostics-error")).toBeHidden();
      expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator/"))).toBe(false);
    });
  }

  test("Shared page runtime serializes refresh, aborts navigation and rejects stale completion", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/#map");
    const result = await page.evaluate(async () => {
      const requests = [];
      const pending = [];
      const errors = [];
      const rendered = [];
      const poller = window.ZPRPageRuntime.createPoller({
        onPending: value => pending.push(value),
        onError: error => errors.push(error.message),
        run({ signal, isCurrent }) {
          return new Promise((resolve, reject) => requests.push({
            signal,
            finish(value) { if (isCurrent()) rendered.push(value); resolve(); },
            fail() { reject(new Error("obsolete")); },
          }));
        },
      });
      poller.start();
      await Promise.resolve();
      const first = poller.refresh();
      const second = poller.refresh();
      const serialized = first === second && requests.length === 1;
      poller.stop();
      poller.start();
      await Promise.resolve();
      requests[0].finish("old");
      await first;
      requests[1].finish("new");
      await poller.refresh();
      poller.stop();
      poller.start();
      await Promise.resolve();
      const failure = poller.refresh();
      poller.stop();
      requests[2].fail();
      await failure;
      poller.dispose();
      poller.start();
      return { serialized, aborted: requests[0].signal.aborted, rendered, errors, pending, requests: requests.length };
    });
    expect(result).toEqual({ serialized: true, aborted: true, rendered: ["new"], errors: [], pending: [true, false, true, false, true, false], requests: 3 });
  });

  test("Shared page runtime pauses timers, permits manual refresh and disposes listeners", async ({ page, appURL, api }) => {
    await page.clock.install();
    await page.goto(appURL + "/#map");
    await page.evaluate(() => {
      window.runtimeCalls = 0;
      window.runtimePoller = window.ZPRPageRuntime.createPoller({
        interval: 1000,
        run: async () => { window.runtimeCalls++; },
        onError: error => { throw error; },
      });
      window.runtimePoller.start();
    });
    await expect.poll(() => page.evaluate(() => window.runtimeCalls)).toBe(1);
    await page.clock.runFor(1001);
    await expect.poll(() => page.evaluate(() => window.runtimeCalls)).toBe(2);
    await page.evaluate(() => window.runtimePoller.setPaused(true));
    await page.clock.runFor(3000);
    expect(await page.evaluate(() => window.runtimeCalls)).toBe(2);
    await page.evaluate(() => window.runtimePoller.refresh());
    expect(await page.evaluate(() => window.runtimeCalls)).toBe(3);
    await page.evaluate(() => { window.runtimePoller.setPaused(false); window.runtimePoller.dispose(); });
    await page.clock.runFor(3000);
    expect(await page.evaluate(() => window.runtimeCalls)).toBe(3);
  });

  test("Shared page runtime JSON preserves errors, abort identity and single-attempt mutations", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/#map");
    const result = await page.evaluate(async () => {
      const runtime = window.ZPRPageRuntime;
      const options = { method: "POST", signal: new AbortController().signal, headers: { "X-ZPR-CSRF": "proof" }, body: "{}" };
      let calls = 0;
      let forwarded;
      let malformed;
      try {
        await runtime.requestJSON(async (url, value) => {
          calls++;
          forwarded = value;
          return new Response("not JSON", { status: 502 });
        }, "/api/test", options);
      } catch (error) { malformed = error.message; }
      let structured;
      try {
        await runtime.requestJSON(async () => new Response(JSON.stringify({ error: "conflict", line: 4 }), { status: 409 }), "/api/test");
      } catch (error) { structured = [error.message, error.line, error.details.error, error.status]; }
      const abort = new DOMException("cancelled", "AbortError");
      let sameAbort;
      try { await runtime.requestJSON(async () => { throw abort; }, "/api/test"); } catch (error) { sameAbort = error === abort; }
      const accepted = await runtime.requestJSON(async () => new Response('{"diagnostics":"warning"}', { status: 422 }), "/api/check", {}, { acceptError: value => Boolean(value.diagnostics) });
      return { calls, malformed, forwarded: [forwarded.method, forwarded.signal === options.signal, forwarded.headers["X-ZPR-CSRF"], forwarded.body], structured, sameAbort, accepted };
    });
    expect(result).toEqual({ calls: 1, malformed: "HTTP 502: invalid JSON response", forwarded: ["POST", true, "proof", "{}"], structured: ["conflict", 4, "conflict", 409], sameAbort: true, accepted: { diagnostics: "warning" } });
  });
}
