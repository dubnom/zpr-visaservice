import { editorCSP } from "./code-editor.mjs";

const cases = [
  { name: "Gateway", id: "gateway-source", path: "/#gateways", sample: '{"methods":["GET"]}', token: "attribute", format: "#gateway-format", analyze: "#gateway-analyze" },
  { name: "Config", id: "zpr-config-source", path: "/#zpr-config", sample: '[visa_service]\ndock_node="node0"\n', token: "class-definition", format: "#zpr-config-format", analyze: "#zpr-config-validate" },
  { name: "Directory", id: "directory-editor-source", path: "/organizations.html", sample: "dn: dc=alpha,dc=test\ncn: Operators\n", token: "keyword" },
  { name: "Scenario", id: "scenario-editor-source", path: "/scenarios.html", sample: '{"organization_id":"alpha","id":"sample","name":"Sample","description":"Test","steps":[{"action":"delay"}]}', token: "attribute", format: "[data-source-format]", analyze: "#scenario-source-analyze" },
];

const host = (page, id) => page.locator(`.code-editor-host[data-source="${id}"]`);
const content = (page, id) => host(page, id).locator(".cm-content");

async function open(page, appURL, api, item) {
  await page.route(appURL + item.path.split("#")[0], async route => {
    const response = await route.fetch();
    await route.fulfill({ response, headers: { ...response.headers(), "content-security-policy": editorCSP } });
  });
  api.handlers.set("/api/gateways/contracts", route => route.fulfill({ json: { organization_id: "alpha", contracts: [
    { organization_id: "alpha", instance_id: "egress", adapter_cn: "gateway-egress", service_name: "egress.svc.zpr" },
  ] } }));
  api.handlers.set("/api/gateways/configs", route => route.fulfill({ json: { configs: [] } }));
  api.handlers.set("/api/gateways/config/check", route => route.fulfill({ json: { valid: true, diagnostics: "Valid JSON" } }));
  api.handlers.set("/api/policy/config/check", route => route.fulfill({ json: { valid: true, diagnostics: "Valid TOML" } }));
  api.handlers.set("/api/simulator/organizations/alpha/directory", route => route.fulfill({ json: {
    revision: 1, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: "dn: dc=alpha,dc=test\n" },
  } }));
  api.handlers.set("/api/simulator/organizations/alpha/directory/revisions", route => route.fulfill({ json: [] }));
  api.handlers.set("/api/simulator/scenarios", route => route.fulfill({ json: {
    active_organization_id: "alpha", scenarios: [], run: { state: "idle" },
  } }));
  api.handlers.set("/api/simulator/organizations/alpha/scenario-check", route => {
    const request = route.request().postDataJSON();
    try {
      const scenario = JSON.parse(request.source);
      return route.fulfill({ json: { valid: true, scenario, canonical_json: JSON.stringify(scenario, null, 2), diagnostics: "Valid scenario" } });
    } catch {
      return route.fulfill({ status: 422, json: { valid: false, line: 1, error: "Invalid scenario JSON" } });
    }
  });
  await page.goto(appURL + item.path);
  if (item.name === "Directory") await page.getByRole("button", { name: "Edit LDAP seed", exact: true }).click();
  if (item.name === "Scenario") {
    await page.locator("#scenario-files-toggle").click();
    await page.locator("#scenario-new").click();
    await page.locator("#scenario-editor-mode-toggle").click();
  }
  await content(page, item.id).waitFor({ state: "visible" });
  await content(page, item.id).fill(item.sample);
}

export function registerRemainingCodeEditorTests(test, expect) {
  test("CodeMirror Scenario design failure preserves draft and displays provider usage", async ({ page, appURL, api }) => {
    api.handlers.set("/api/simulator/assistant/status", route => route.fulfill({ json: { ready: true, model: "test-model", models: ["test-model"] } }));
    const message = "Claude reached the output token limit before completing the design. No changes were applied. Increase the output limit in Model and Tokens or request a smaller scenario.";
    api.handlers.set("/api/simulator/design-assistant", route => route.fulfill({ status: 502, json: {
      error: message, input_tokens: 321, output_tokens: 2400,
    } }));
    const item = cases.find(item => item.name === "Scenario");
    await open(page, appURL, api, item);
    const pane = page.locator("#scenario-assistant-slot");
    await pane.locator("[data-assistant-question]").fill("Create a fictional delay example");
    await pane.locator("[data-assistant-submit]").click();
    await expect(pane.locator("[data-assistant-thread]")).toContainText(message);
    await expect(page.locator(`#${item.id}`)).toHaveValue(item.sample);
    await expect(pane.locator("[data-assistant-apply]")).toBeHidden();
    await pane.getByText("Model and Tokens", { exact: true }).click();
    await expect(pane.locator("[data-assistant-usage]")).toContainText("2,400 output / 2,400 max");
    await expect(pane.locator("[data-assistant-usage]")).toContainText("321 input");
    expect(api.counts.get("/api/simulator/design-assistant")).toBe(1);
    expect([...api.counts.keys()].some(path => /\/run$|\/publish$/.test(path))).toBe(false);
  });

  for (const item of cases) {
    test(`CodeMirror ${item.name} source preserves typography, colors, wrapping, replacement and history`, async ({ page, appURL, api }) => {
      await open(page, appURL, api, item);
      const source = page.locator(`#${item.id}`);
      const editor = content(page, item.id);
      await expect(source).toBeHidden();
      await expect(host(page, item.id).locator(`.cm-token-${item.token}`).first()).toBeVisible();
      const typography = await source.evaluate(element => {
        const sourceStyle = getComputedStyle(element);
        const editor = element.closest(".config-source-editor").querySelector(".code-editor-host").shadowRoot.querySelector(".cm-scroller");
        const editorStyle = getComputedStyle(editor);
        return [editorStyle.fontSize === sourceStyle.fontSize, editorStyle.fontFamily === sourceStyle.fontFamily, editorStyle.lineHeight === sourceStyle.lineHeight];
      });
      expect(typography).toEqual([true, true, true]);
      await editor.press("ControlOrMeta+End");
      await editor.pressSequentially("extra");
      await expect(source).toHaveValue(item.sample + "extra");
      await editor.press("ControlOrMeta+z");
      await expect(source).toHaveValue(item.sample);
      await editor.press("ControlOrMeta+Shift+Z");
      await expect(source).toHaveValue(item.sample + "extra");
      await source.evaluate(element => window.ZPREditorPage.editText(element, "replacement\nsecond", true));
      await expect(source).toHaveValue("replacement\nsecond");
      await editor.press("ControlOrMeta+z");
      await expect(source).toHaveValue(item.sample + "extra");
      const toolbar = page.locator(`[data-editor-search-target="${item.id}"]`);
      const wrap = toolbar.locator("..").getByRole("checkbox", { name: "Wrap" });
      await wrap.uncheck();
      await expect(editor).not.toHaveClass(/cm-lineWrapping/);
      await wrap.check();
      await expect(editor).toHaveClass(/cm-lineWrapping/);
      await toolbar.getByRole("button", { name: "Find & Replace", exact: true }).click();
      await toolbar.getByRole("textbox", { name: "Find what" }).fill("extra");
      await toolbar.getByRole("textbox", { name: "Replace with" }).fill("changed");
      await toolbar.getByRole("button", { name: "Replace All", exact: true }).click();
      await expect(source).toHaveValue(item.sample + "changed");
      await toolbar.getByRole("button", { name: "Close", exact: true }).click();
      await editor.press("ControlOrMeta+z");
      await expect(source).toHaveValue(item.sample + "extra");
      await source.evaluate(element => { element.value = "loaded record"; element.dispatchEvent(new Event("input", { bubbles: true })); });
      await editor.press("ControlOrMeta+z");
      await expect(source).toHaveValue("loaded record");
      await source.evaluate(element => { element.readOnly = true; });
      await expect(editor).toHaveAttribute("contenteditable", "false");
      expect(await source.evaluate(element => {
        try { window.ZPREditorPage.editText(element, "blocked", true); }
        catch (error) { return error.message; }
      })).toContain("read-only");
      await expect(source).toHaveValue("loaded record");
      await source.evaluate(element => { element.readOnly = false; element.maxLength = 4; element.value = "abc"; });
      await expect(editor).toHaveAttribute("contenteditable", "true");
      await editor.press("ControlOrMeta+End");
      await editor.pressSequentially("de");
      await expect(source).toHaveValue("abcd");
      if (item.path.startsWith("/#")) expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator"))).toBe(false);
    });

    if (!item.format) continue;
    test(`CodeMirror ${item.name} Format and toolbar Undo preserve preceding edits and Analyze`, async ({ page, appURL, api }) => {
      await open(page, appURL, api, item);
      const source = page.locator(`#${item.id}`);
      const editor = content(page, item.id);
      await editor.press("ControlOrMeta+End");
      await editor.pressSequentially(" ");
      const original = item.sample + " ";
      await page.locator(item.format).click();
      await expect(source).not.toHaveValue(original);
      const formatted = await source.inputValue();
      await page.locator(item.format).press("ControlOrMeta+z");
      await expect(source).toHaveValue(original);
      await page.locator(item.format).press("ControlOrMeta+Shift+Z");
      await expect(source).toHaveValue(formatted);
      await page.locator(item.analyze).click();
      await expect(page.locator(item.analyze)).not.toHaveAttribute("data-analysis-state", "pending");
      await page.locator(item.analyze).press("ControlOrMeta+z");
      await expect(source).toHaveValue(original);
      await editor.press("ControlOrMeta+z");
      await expect(source).toHaveValue(item.sample);
    });

    test(`CodeMirror ${item.name} source diagnostics remain clickable and clear on edits`, async ({ page, appURL, api }) => {
      await open(page, appURL, api, item);
      const endpoint = item.name === "Gateway" ? "/api/gateways/config/check" : item.name === "Config" ? "/api/policy/config/check" : "/api/simulator/organizations/alpha/scenario-check";
      api.handlers.set(endpoint, route => route.fulfill({ status: 422, json: item.name === "Gateway"
        ? { valid: false, source_line: 1, diagnostics: "Sample source diagnostic" }
        : { valid: false, line: 1, error: "Sample source diagnostic" } }));
      await page.locator(item.analyze).click();
      const marker = host(page, item.id).locator(".cm-result-gutter button");
      await expect(marker).toHaveCount(1);
      await expect(marker).toHaveAttribute("title", "Sample source diagnostic");
      await marker.click();
      expect(await page.locator(`#${item.id}`).evaluate(source => source.selectionStart)).toBe(0);
      if (item.name === "Gateway") await page.locator("#gateway-error-dialog").getByRole("button", { name: "Close", exact: true }).click();
      await content(page, item.id).press("ControlOrMeta+End");
      await content(page, item.id).pressSequentially(" ");
      await expect(marker).toHaveCount(0);
      await expect(page.locator(item.analyze)).not.toHaveAttribute("data-analysis-state", "error");
    });
  }

  test("CodeMirror Directory save and Discard preserve explicit revision boundaries", async ({ page, appURL, api }) => {
    const item = cases.find(item => item.name === "Directory");
    await open(page, appURL, api, item);
    const source = page.locator(`#${item.id}`);
    const endpoint = "/api/simulator/organizations/alpha/directory";
    const writes = [];
    api.handlers.set(endpoint, route => {
      const request = route.request();
      if (request.method() === "PUT") writes.push(request.postDataJSON());
      return route.fulfill({ json: { revision: 2, published_revision: 0, content: { base_dn: "dc=alpha,dc=test", ldif: item.sample } } });
    });
    await page.locator("#directory-editor-files-toggle").click();
    await page.locator("#directory-editor-save").click();
    await expect(page.locator("#directory-editor-status")).toContainText("Saved version 2");
    expect(writes).toHaveLength(1);
    expect(writes[0].document.ldif).toBe(item.sample);
    await content(page, item.id).press("ControlOrMeta+End");
    await content(page, item.id).pressSequentially("changed");
    await page.locator("#directory-editor-files-toggle").click();
    page.once("dialog", dialog => dialog.accept());
    await page.locator("#directory-editor-discard").click();
    await expect(source).toHaveValue(item.sample);
    await content(page, item.id).press("ControlOrMeta+z");
    await expect(source).toHaveValue(item.sample);
    await page.locator("#directory-editor-form .sim-editor-close").click();
    await expect(page.locator("#directory-editor-dialog")).toBeHidden();
    expect(writes).toHaveLength(1);
    expect([...api.counts.keys()].some(path => /publish|reseed|restore-base|activate/.test(path))).toBe(false);
  });

  test("CodeMirror Scenario YAML conversion, Format Undo and form round trip preserve format context", async ({ page, appURL, api }) => {
    const item = cases.find(item => item.name === "Scenario");
    await open(page, appURL, api, item);
    const yaml = "organization_id: alpha\nid: sample\nname: Sample\ndescription: Test\nsteps:\n  - action: delay\n";
    const formats = [];
    api.handlers.set("/api/simulator/organizations/alpha/scenario-check", route => {
      const request = route.request().postDataJSON();
      formats.push(request.format);
      return route.fulfill({ json: { valid: true, scenario: JSON.parse(item.sample), canonical_json: JSON.stringify(JSON.parse(item.sample), null, 2), canonical_yaml: yaml } });
    });
    await page.locator("#scenario-editor-files-toggle").click();
    await page.locator("#scenario-source-convert").click();
    const source = page.locator(`#${item.id}`);
    await expect(source).toHaveValue(yaml);
    await expect(content(page, item.id)).toHaveAttribute("aria-label", "Raw scenario YAML");
    await expect(host(page, item.id).locator(".cm-token-attribute").first()).toBeVisible();
    const draft = yaml + "\n";
    await content(page, item.id).fill(draft);
    await page.locator(item.format).click();
    await expect(source).toHaveValue(yaml);
    await page.locator(item.format).press("ControlOrMeta+z");
    await expect(source).toHaveValue(draft);
    await page.locator(item.analyze).click();
    await expect(page.locator(item.analyze)).toHaveAttribute("data-analysis-state", "success");
    expect(formats).toEqual(["json", "yaml", "yaml"]);
    await page.locator("#scenario-editor-mode-toggle").click();
    await expect(page.locator("#scenario-editor-name")).toHaveValue("Sample");
    await expect(host(page, item.id)).toBeHidden();
    expect([...api.counts.keys()].some(path => /\/run$|\/publish$/.test(path))).toBe(false);
  });

  test("CodeMirror remaining source editors fail explicitly without exposing a legacy editor", async ({ page, appURL, api }) => {
    await page.route("**/vendor/code-editor.js?*", route => route.abort());
    await page.goto(appURL + "/#zpr-config");
    await expect(page.locator("#page-zpr-config .code-editor-error")).toContainText("Could not load the editor");
    await expect(page.locator("#zpr-config-source")).toBeHidden();
    expect(await page.locator("#zpr-config-source").evaluate(source => {
      try { window.ZPREditorPage.editText(source, "not applied", true); }
      catch (error) { return error.message; }
    })).toContain("unavailable");
    await expect(page.locator("#zpr-config-source")).toHaveValue("");
  });
}
