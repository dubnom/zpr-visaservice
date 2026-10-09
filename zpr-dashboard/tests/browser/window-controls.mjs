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
    await page.locator("#scenario-new").click();
    await toggle(page, page.locator("#editor-maximize-scenario-editor-dialog"), page.locator("#scenario-editor-dialog"));
    await expect(page.locator("#scenario-editor-dialog")).toHaveAttribute("open", "");
    api.handlers.set("/api/simulator/organizations/alpha/directory", route => route.fulfill({ json: {
      revision: 2, published_revision: 1, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" },
    } }));
    api.handlers.set("/api/simulator/organizations/alpha/directory/revisions", route => route.fulfill({ json: [] }));
    await page.goto(appURL + "/organizations.html");
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
}
