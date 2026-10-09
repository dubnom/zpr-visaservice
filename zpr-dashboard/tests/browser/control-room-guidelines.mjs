export function registerControlRoomGuidelineTests(test, expect) {
  test("GUI guidelines keep the top banner application-wide and explanations in Help", async ({ page, appURL, api }) => {
    api.snapshot.api_status = "disconnected";
    api.snapshot.errors = ["Visa Service timeout"];
    api.snapshot.stats = { uptime: 120 };
    await page.goto(appURL + "/#node-stats");
    await expect(page.locator("#api-state-text")).toHaveText("Control Room connected");
    await expect(page.locator("#snapshot-source-status")).toHaveText("Visa Service snapshot unavailable");
    await expect(page.locator("#alert-strip")).toContainText("Visa Service timeout");
    await expect(page.locator(".topbar")).not.toContainText("Visa Service");
    await expect(page.locator(".topbar #metric-uptime")).toHaveCount(0);
    await expect(page.locator("#page-node-stats")).not.toContainText("Counters are cumulative");
    await page.locator(".help-trigger").click();
    await expect(page.getByRole("dialog")).toContainText("Buffered denials are a current gauge");
    await page.keyboard.press("Escape");
    await page.goto(appURL + "/#provisioning-adapters");
    await expect(page.locator(".provisioning-steps, .provisioning-notice")).toHaveCount(0);
    await expect(page.locator("#provisioning-recipient-note")).toContainText("Never include an enrollment code in email");
    await page.locator(".help-trigger").click();
    await expect(page.getByRole("dialog")).toContainText("authenticated secure channel");
    await expect(page.getByRole("dialog")).toContainText("Approval does not issue credentials");
  });

  test("GUI guidelines use bold right-aligned numeric headers and stable changed-field pulses", async ({ page, appURL, api }) => {
    await page.emulateMedia({ reducedMotion: "no-preference" });
    api.snapshot.stats = { uptime: 5 };
    api.snapshot.recent_denies = [{ source_addr: "fd00::1", dest_addr: "fd00::2", protocol: 6, dest_port: 443, deny_code: "DENY", count: 1 }];
    await page.goto(appURL + "/#denies");
    await page.locator("#pause-poll").click();
    const header = page.locator('#page-denies th[data-sort-key="count"]');
    await expect(header).toHaveCSS("font-weight", "700");
    await expect(header).toHaveCSS("text-align", "right");
    await expect(header.locator("button")).toHaveCSS("justify-content", "flex-end");
    await expect(page.locator("#deny-list td").last()).toHaveCSS("text-align", "right");
    await expect(page.locator("#deny-list .poll-changed")).toHaveCount(0);
    api.snapshot.recent_denies[0].count = 2;
    api.snapshot.stats.uptime = 8;
    await page.locator("#refresh-now").click();
    await expect(page.locator("#deny-list .poll-changed")).toHaveText("2");
    await expect(page.locator("#status-count-denies")).toHaveClass(/poll-changed/);
    await expect(page.locator("#metric-uptime")).toHaveClass(/poll-changed/);
    await expect(page.locator("#deny-list .poll-changed")).toHaveCSS("animation-name", "poll-value-pulse");
    await expect(page.locator("#deny-list .poll-changed")).toHaveCount(0);
    await header.locator("button").click();
    await expect(page.locator("#deny-list .poll-changed")).toHaveCount(0);
    await page.locator("#refresh-now").click();
    await expect(page.locator("#deny-list .poll-changed")).toHaveCount(0);
  });

  test("GUI guidelines pulse DNS counters after their own asynchronous response", async ({ page, appURL, api }) => {
    let received = 1;
    api.handlers.set("/api/dns/stats/json/v1/status", route => route.fulfill({ json: { "current-time": new Date().toISOString() } }));
    api.handlers.set("/api/dns/stats/json/v1/zones", route => route.fulfill({ json: { views: {} } }));
    api.handlers.set("/api/dns/stats/json/v1/server", route => route.fulfill({ json: { nsstats: { Requestv4: received }, version: "9" } }));
    await page.goto(appURL + "/#dns");
    await page.locator("#pause-poll").click();
    await expect(page.locator("#dns-stat-requests")).toHaveText("1");
    const header = page.locator('table[data-sort-page="dns-counters"] th[data-sort-key="value"]');
    await expect(header).toHaveCSS("text-align", "right");
    received = 2;
    await page.locator("#refresh-now").click();
    await expect(page.locator("#dns-stat-requests")).toHaveText("2");
    await expect(page.locator("#dns-stat-requests")).toHaveClass(/poll-changed/);
    await expect(page.locator('[data-poll-row="Requestv4"] .poll-changed')).toHaveText("2");
    await expect(header).toHaveCSS("font-weight", "700");
  });

  test("GUI guidelines Control Room visible text has an 11px minimum", async ({ page, appURL, api }) => {
    api.snapshot.actors = [{ cn: "node-a", node: true, node_details: {
      in_sync: true, adapters: [], counters_updated_at: new Date().toISOString(),
      counters: [{ group: "management", name: "visa_requested", value: "1" }],
    } }];
    api.snapshot.stats = { uptime: 100 };
    for (const route of ["map", "node-stats", "actors", "denies", "sources", "security-review", "provisioning-adapters", "policy", "gateways"]) {
      await page.goto(`${appURL}/#${route}`);
      await expect(page.locator("#metric-uptime")).toHaveText("1m 40s");
      await expect(page.locator(`#page-${route}`)).toBeVisible();
      const small = await page.evaluate(() => [...document.querySelectorAll("body *")].filter(element =>
        [...element.childNodes].some(node => node.nodeType === Node.TEXT_NODE && node.textContent.trim())
        && !element.closest("#topology-stage")
        && element.getClientRects().length && getComputedStyle(element).visibility !== "hidden"
        && Number.parseFloat(getComputedStyle(element).fontSize) < 11
      ).map(element => ({ element: element.tagName, class: element.className.baseVal ?? element.className,
        id: element.id, text: element.textContent.slice(0, 50), size: getComputedStyle(element).fontSize })));
      expect(small, `${route} text below 11px`).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth), `${route} page overflow`)
        .toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth + 1));
    }
  });

  test("GUI guidelines Security pulses changed scan evidence without pulsing sorting", async ({ page, appURL, api }) => {
    api.snapshot.actors = [{ cn: "adapter-a", node: false, zpr_addr: "fd00::1" }];
    api.snapshot.recent_denies = [{ source_addr: "fd00::bad", dest_addr: "fd00::1", protocol: 6,
      dest_port: 443, count: 5, last_deny_ms: Date.now(), deny_code: "Denied" }];
    await page.goto(appURL + "/#security-review");
    await page.locator("#pause-poll").click();
    const evidence = page.locator('#security-review-findings [data-sort-cell="evidence"]').filter({ hasText: "denied requests" });
    await expect(evidence).toContainText("5 denied requests");
    await expect(evidence.locator(".poll-changed")).toHaveCount(0);
    api.snapshot.recent_denies[0].count = 8;
    await page.locator("#refresh-now").click();
    await expect(evidence.locator(".poll-changed")).toContainText("8 denied requests");
    await expect(evidence.locator(".poll-changed")).toHaveCSS("animation-duration", "2.4s");
    await expect(evidence.locator(".poll-changed")).toHaveCount(0);
    await page.locator('th[data-sort-key="evidence"] button').click();
    await expect(evidence.locator(".poll-changed")).toHaveCount(0);
    await page.locator("#refresh-now").click();
    await expect(evidence.locator(".poll-changed")).toHaveCount(0);
  });

  test("GUI guidelines inspector counter tables sort exactly and pulse live changes", async ({ page, appURL, api }) => {
    api.snapshot.actors = [{ cn: "node-a", node: true, node_details: {
      counters_updated_at: new Date().toISOString(),
      counters: [
        { group: "management", name: "zeta", value: "9007199254740993" },
        { group: "management", name: "alpha", value: "9007199254740992" },
      ],
    } }];
    await page.goto(appURL + "/#actors");
    await page.locator("#pause-poll").click();
    await page.locator('#actor-rows [data-inspect-actor="node-a"]').click();
    const table = page.getByRole("table", { name: "Management counters", exact: true });
    await expect(table.locator("tbody th")).toHaveText(["Alpha", "Zeta"]);
    const sortButton = table.getByRole("button", { name: "Sort by Total", exact: false });
    await sortButton.focus();
    await page.keyboard.press("Enter");
    await expect(table.locator("tbody td")).toHaveText(["9007199254740992", "9007199254740993"]);
    await sortButton.focus();
    await page.keyboard.press("Space");
    await expect(table.locator("tbody th")).toHaveText(["Zeta", "Alpha"]);
    api.snapshot.actors[0].node_details.counters[0].value = "0";
    await page.evaluate(() => document.getElementById("refresh-now").click());
    await expect(table.locator('th[data-sort-key="value"]')).toHaveAttribute("aria-sort", "descending");
    await expect(table.locator('[data-counter-name="zeta"] .poll-changed')).toHaveText("0");
  });
}
