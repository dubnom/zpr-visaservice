export function registerWindowControlTests(test, expect) {
  const paths = { maximize: "M4 4h16v16H4z", restore: "M8 8V4h12v12h-4M4 8h12v12H4z" };

  async function check(button, state) {
    await expect(button).toHaveText("");
    await expect(button).toHaveAttribute("aria-pressed", String(state === "restore"));
    await expect(button).toHaveAttribute("aria-label", new RegExp(`^${state === "restore" ? "Restore" : "Maximize"} `));
    expect(await button.getAttribute("title")).toBe(await button.getAttribute("aria-label"));
    const icon = button.locator("svg");
    await expect(icon).toHaveAttribute("data-window-control", state);
    await expect(icon).toHaveAttribute("aria-hidden", "true");
    await expect(icon).toHaveAttribute("focusable", "false");
    await expect(icon.locator("path")).toHaveAttribute("d", paths[state]);
    const box = await icon.boundingBox();
    expect(box.width).toBe(16);
    expect(box.height).toBe(16);
    await expect(button).toHaveCSS("color", "rgb(23, 33, 30)");
    await expect(icon).toHaveCSS("stroke", "rgb(23, 33, 30)");
    const buttonBox = await button.boundingBox();
    expect(buttonBox.width).toBe(32);
    expect(buttonBox.height).toBe(32);
  }

  async function toggle(page, button, pane, editor = true) {
    await page.mouse.move(0, 0);
    await check(button, "maximize");
    await expect(button).toHaveCSS("background-color", "rgb(255, 255, 255)");
    await button.hover();
    await expect(button).toHaveCSS("background-color", "rgb(234, 244, 217)");
    await page.mouse.down();
    await expect(button).toHaveCSS("background-color", "rgb(220, 235, 198)");
    await page.mouse.move(0, 0);
    await page.mouse.up();
    await button.evaluate(element => { element.disabled = true; });
    await expect(button).toHaveCSS("background-color", "rgb(255, 255, 255)");
    await expect(button).toHaveCSS("opacity", "0.48");
    await expect(button).toHaveCSS("cursor", "not-allowed");
    await button.evaluate(element => { element.disabled = false; });
    await page.mouse.move(0, 0);
    await page.keyboard.press("Tab");
    await button.focus();
    await expect(button).toHaveCSS("outline-style", "solid");
    await expect(button).toHaveCSS("outline-width", "2px");
    await button.press("Enter");
    await check(button, "restore");
    await expect(pane).toHaveClass(editor ? /editor-page-maximized/ : /maximized/);
    await button.click();
    await check(button, "maximize");
    await expect(pane).not.toHaveClass(editor ? /editor-page-maximized/ : /maximized/);
    await button.click();
    await page.keyboard.press("Escape");
    await check(button, "maximize");
    if (editor) await expect(button).toBeFocused();
  }

  test("Window control icons are shared by all six production and Simulator editors", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/#policy");
    await toggle(page, page.locator("#policy-editor-maximize"), page.locator("#policy-editor-pane"));
    await page.locator("#policy-picker-toggle").click();
    await page.locator('[data-record-id="test-assertions"]').click();
    await expect(page.locator("#policy-assertion-editor")).toBeVisible();
    await toggle(page, page.locator("#policy-editor-maximize"), page.locator("#policy-editor-pane"));
    for (const route of ["zpr-config", "gateways"]) {
      await page.goto(`${appURL}/#${route}`);
      await toggle(page, page.locator(`#editor-maximize-page-${route}`), page.locator(`#page-${route}`));
    }
    await page.goto(appURL + "/scenarios.html");
    await page.locator("#scenario-files-toggle").click();
    await page.locator("#scenario-new").click();
    await toggle(page, page.locator("#editor-maximize-scenario-editor-dialog"), page.locator("#scenario-editor-dialog"));
    await expect(page.locator("#scenario-editor-dialog")).toHaveAttribute("open", "");
    api.handlers.set("/api/simulator/organizations/alpha/directory", route => route.fulfill({ json: {
      revision: 2, published_revision: 1, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" },
    } }));
    api.handlers.set("/api/simulator/organizations/alpha/directory/revisions", route => route.fulfill({ json: [] }));
    await page.goto(appURL + "/organizations.html");
    const graph = page.locator("#organization-detail ldap-org-graph");
    await expect(graph).toBeVisible();
    await toggle(page, graph.locator(".window-control"), graph, false);
    await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
    await toggle(page, page.locator("#editor-maximize-directory-editor-dialog"), page.locator("#directory-editor-dialog"));
    await expect(page.locator("#directory-editor-dialog")).toHaveAttribute("open", "");
  });

  test("Window control icons are shared by adapter, controller and Simulator Worker logs", async ({ page, appURL, api }) => {
    await page.goto(appURL + "/#adapter-logs");
    for (const kind of ["adapter", "controller"]) {
      await page.locator(`[data-adapter-log-type="${kind}"]`).click();
      const panel = page.locator(".adapter-log-column").first();
      await toggle(page, panel.locator('button[aria-pressed]').filter({ has: page.locator("svg[data-window-control]") }), panel, false);
    }
    await page.goto(appURL + "/machine-logs.html");
    const panel = page.locator(".machine-log-panel").first();
    await expect(panel).toBeVisible();
    await toggle(page, panel.locator('button[aria-pressed]').filter({ has: page.locator("svg[data-window-control]") }), panel, false);
  });

  test("Simulator activity refresh pulses changed metrics and right-aligns numeric sortable columns", async ({ page, appURL, api }) => {
    let refresh = 0;
    api.handlers.set("/api/simulator/activity", route => {
      refresh += 1;
      return route.fulfill({ json: {
        stats: { visa_requests: refresh, visa_requests_approved: refresh, visa_requests_denied: 0 },
        visas: [{ source_addr: "10.0.0.1", dest_addr: "10.0.0.2", dest_port: 443, expires: 1781265600 }],
        denies: [{ source_addr: "10.0.0.3", dest_addr: "10.0.0.4", count: 3 }],
        generated_at: "2026-10-13T12:00:00Z",
      } });
    });
    await page.goto(`${appURL}/activity.html`);
    await expect(page.locator("#requests")).toHaveText("1");
    await expect.poll(() => refresh).toBeGreaterThan(1);
    await expect(page.locator("#requests")).toHaveClass(/poll-changed/);
    for (const column of ["port", "count"]) {
      const header = page.locator(`th[data-sort-key="${column}"]`);
      await expect(header).toHaveAttribute("data-numeric", "true");
      await expect(header).toHaveCSS("text-align", "right");
      await expect(page.locator(`td[data-sort-cell="${column}"]`)).toHaveAttribute("data-numeric", "true");
      await expect(page.locator(`td[data-sort-cell="${column}"]`)).toHaveCSS("text-align", "right");
    }
  });
}
