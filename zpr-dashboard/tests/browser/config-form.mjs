import { spawn } from "node:child_process";
import { createInterface } from "node:readline";

const source = `# Operator-owned source
[resolver]
order = ["hosts", "dns"]
[resolver.hosts]
"example.test" = "192.0.2.1"
[visa_service]
dock_node = "node0" # keep this comment
[nodes.node0]
zpr_address = "fd00::1"
provider = [["device.zpr.adapter.cn", "node0"]]
future_node_setting = 9223372036854775807
[nodes.node0.substrate_addrs]
lan = "192.0.2.1:1200"
[nodes.node1]
zpr_address = "fd00::2"
provider = [["device.zpr.adapter.cn", "node1"]]
[links.backbone]
peers = [{ node = "node0", interface = "lan", future = true }, { node = "node1" }]
attributes = [["zpr.cost", "5"]]
[trusted_services.directory]
api = "validation/2"
service = "lookup"
client = "login"
cert_path = "public.pem"
expiration_seconds = 3600
returns_attributes = ["ou -> device.demo.department", "role -> user.role{}"]
identity_attributes = ["device.zpr.adapter.cn"]
provider = [["device.zpr.adapter.cn", "directory"]]
[protocols.https]
l4protocol = "iana.TCP"
port = 443
[protocols.ping]
l4protocol = "iana.ICMP6"
icmp_type = "request-response"
icmp_codes = [128, 129]
[services.web]
protocol = "https"
provider = [["device.demo.app", "web"]]
[bootstrap]
"device.example" = "bootstrap-public.pem"
[unfamiliar]
keep = "literal # unchanged"
`;

export function registerConfigFormTests(base, expect) {
  const test = base.extend({
    configEndpoint: [async ({}, use) => {
      const child = spawn(process.env.ZPR_OPERATOR_TEST_BINARY, ["-test.run=^TestConfigurationFormBrowserFixture$", "-test.timeout=120s"], {
        env: { ...process.env, ZPR_CONFIG_FORM_BROWSER_FIXTURE: "1" }, stdio: ["pipe", "pipe", "pipe"],
      });
      let output = "";
      child.stderr.on("data", chunk => { output += chunk; });
      const lines = createInterface({ input: child.stdout });
      const exit = new Promise((resolve, reject) => { child.once("exit", resolve); child.once("error", reject); });
      try {
        const url = await new Promise((resolve, reject) => {
          const timer = setTimeout(() => reject(new Error(`Config fixture timed out: ${output}`)), 15000);
          lines.on("line", line => {
            output += line + "\n";
            if (line.startsWith("ZPR-CONFIG-FORM-FIXTURE ")) { clearTimeout(timer); resolve(line.slice("ZPR-CONFIG-FORM-FIXTURE ".length)); }
          });
          child.once("exit", code => { clearTimeout(timer); reject(new Error(`Config fixture exited ${code}: ${output}`)); });
          child.once("error", error => { clearTimeout(timer); reject(error); });
        });
        await use(url);
      } finally {
        lines.close();
        child.stdin.end("stop\n");
        const timer = setTimeout(() => child.kill("SIGTERM"), 10000);
        const code = await exit;
        clearTimeout(timer);
        expect(code, output).toBe(0);
      }
    }, { scope: "worker" }],
  });
  test.describe("Config form", () => {
    test.skip(!process.env.ZPR_OPERATOR_TEST_BINARY, "Requires the operator test binary with the real TOML endpoint.");
    async function open(page, appURL, api, endpoint, content = source) {
      api.handlers.set("/api/policy/config/form", async route => {
        const response = await fetch(endpoint + "/api/policy/config/form", {
          method: "POST", headers: { "Content-Type": "application/json" }, body: route.request().postData(),
        });
        await route.fulfill({ status: response.status, contentType: "application/json", body: await response.text() });
      });
      await page.goto(appURL + "/#zpr-config");
      await expect.poll(() => api.counts.get("/api/policy") || 0).toBeGreaterThan(0);
      await expect(page.locator("#zpr-config-mode-toggle")).toBeEnabled();
      await page.locator("#zpr-config-source").fill(content);
      await page.locator("#zpr-config-mode-toggle").click();
      await expect(page.locator("#zpr-config-form-fields")).toBeEnabled();
      await expect(page.locator("#zpr-config-form-error")).toBeHidden();
    }
    function input(page, path) {
      return page.locator(`[data-config-path='${JSON.stringify(path)}']`);
    }
    async function edit(page, path, value) {
      const before = await page.locator("#zpr-config-source").inputValue();
      const control = input(page, path);
      await control.fill(value);
      await control.press("Tab");
      await expect.poll(() => page.locator("#zpr-config-source").inputValue()).not.toBe(before);
      await expect(page.locator("#zpr-config-form-fields")).toBeEnabled();
      await expect(page.locator("#zpr-config-form-error")).toBeHidden();
      await expect(input(page, path)).toHaveValue(value);
    }

    test("covers compiler sections, patches exact source, and never calls Simulator", async ({ page, appURL, api, configEndpoint }) => {
      await open(page, appURL, api, configEndpoint);
      await expect(page.locator("#zpr-config-source")).toBeHidden();
      await expect(page.locator("#zpr-config-format")).toBeDisabled();
      for (const title of ["Resolver", "Visa Service", "Nodes", "Links", "Trusted services", "Protocols", "Services"]) {
        await expect(page.locator("#zpr-config-form").getByRole("heading", { name: title, exact: true })).toBeVisible();
      }
      await expect(page.locator("#zpr-config-form")).toContainText("future_node_setting");
      await expect(page.locator("#zpr-config-form")).toContainText("unfamiliar");
      await edit(page, ["visa_service", "dock_node"], "node1");
      const draft = await page.locator("#zpr-config-source").inputValue();
      expect(draft).toBe(source.replace('dock_node = "node0"', 'dock_node = "node1"'));
      await expect(page.locator("#zpr-config-modified")).toBeVisible();
      await edit(page, ["trusted_services", "directory", "expiration_seconds"], "7200");
      await edit(page, ["protocols", "https", "port"], "443,8000-9000");
      await edit(page, ["protocols", "ping", "icmp_codes"], "130, 131");
      await edit(page, ["resolver", "hosts", "example.test"], "192.0.2.2");
      await edit(page, ["nodes", "node0", "substrate_addrs", "lan"], "192.0.2.2:1200");
      await edit(page, ["bootstrap", "device.example"], "replacement-public.pem");
      const link = page.locator('[data-config-table=\'["links","backbone"]\']');
      const beforePeers = await page.locator("#zpr-config-source").inputValue();
      await link.getByLabel("Node", { exact: true }).first().fill("node1");
      await link.getByLabel("Node", { exact: true }).first().press("Tab");
      await expect.poll(() => page.locator("#zpr-config-source").inputValue()).not.toBe(beforePeers);
      await expect(page.locator("#zpr-config-form-fields")).toBeEnabled();
      expect(await page.locator("#zpr-config-source").inputValue()).toContain('"future" = true');
      api.handlers.set("/api/policy/config/check", async route => {
        expect(route.request().postDataJSON().source).toBe(await page.locator("#zpr-config-source").inputValue());
        await route.fulfill({ json: { valid: true, diagnostics: "TOML syntax valid; runtime configuration is unchanged." } });
      });
      await page.locator("#zpr-config-validate").click();
      await expect(page.locator("#zpr-config-status")).toContainText("runtime configuration is unchanged");
      await page.locator("#zpr-config-mode-toggle").click();
      await expect(page.locator("#zpr-config-source")).toBeVisible();
      expect([...api.counts.keys()].some(path => path.startsWith("/api/simulator"))).toBe(false);
      const width = await page.locator("#page-zpr-config").evaluate(node => ({ scroll: node.scrollWidth, client: node.clientWidth }));
      expect(width.scroll).toBeLessThanOrEqual(width.client + 1);
    });

    test("adds and removes entries, preserves comments, and creates a draft revision", async ({ page, appURL, api, configEndpoint }) => {
      await open(page, appURL, api, configEndpoint);
      page.once("dialog", dialog => dialog.accept("new.web"));
      await page.getByRole("button", { name: "Add service", exact: true }).click();
      await expect(input(page, ["services", "new.web", "protocol"])).toBeVisible();
      await expect(page.locator("#zpr-config-validate")).toBeDisabled();
      await edit(page, ["services", "new.web", "protocol"], "https");
      const card = page.locator('[data-config-table=\'["services","new.web"]\']');
      page.once("dialog", dialog => dialog.accept());
      await card.getByRole("button", { name: "Remove entry", exact: true }).click();
      await expect(card).toHaveCount(0);
      const payloads = [];
      let record;
      api.handlers.set("/api/policy/records", async route => {
        const payload = route.request().postDataJSON();
        payloads.push(payload);
        record = { ...payload, id: "form-config", current_revision: 1 };
        api.policy.records.push(record);
        await route.fulfill({ json: record });
      });
      api.handlers.set("/api/policy/categories", route => route.fulfill({ json: { id: "config-category", path: "ZPR Config" } }));
      api.handlers.set("/api/policy/records/form-config", route => route.fulfill({ json: record }));
      api.handlers.set("/api/policy/records/form-config/revisions", route => route.fulfill({ json: [] }));
      await page.locator("#zpr-config-files-toggle").click();
      page.once("dialog", dialog => dialog.accept("Form draft"));
      await page.locator("#zpr-config-save").click();
      await expect(page.locator("#zpr-config-status")).toHaveText("Saved version 1; runtime unchanged.");
      expect(payloads).toHaveLength(1);
      expect(payloads[0].kind).toBe("configuration");
      expect(payloads[0].content).toContain("# keep this comment");
      expect(payloads[0].content).toContain("9223372036854775807");
      expect([...api.counts.keys()].some(path => /stage|install|activate|simulator/.test(path))).toBe(false);
    });

    test("invalid TOML and unsupported layouts remain recoverable in raw mode", async ({ page, appURL, api, configEndpoint }) => {
      await page.goto(appURL + "/#zpr-config");
      await expect(page.locator("#zpr-config-mode-toggle")).toBeEnabled();
      api.handlers.set("/api/policy/config/form", async route => {
        const response = await fetch(configEndpoint + "/api/policy/config/form", { method: "POST", body: route.request().postData() });
        await route.fulfill({ status: response.status, json: await response.json() });
      });
      await page.locator("#zpr-config-source").fill("broken = [");
      await page.locator("#zpr-config-mode-toggle").click();
      await expect(page.locator("#zpr-config-form-error")).toContainText("Invalid TOML");
      await expect(page.locator("#zpr-config-validate")).toBeDisabled();
      await page.locator("#zpr-config-mode-toggle").click();
      await expect(page.locator("#zpr-config-source")).toHaveValue("broken = [");
      const inline = 'visa_service = { dock_node = "node0", future = true }\n';
      await page.locator("#zpr-config-source").fill(inline);
      await page.locator("#zpr-config-mode-toggle").click();
      const dock = input(page, ["visa_service", "dock_node"]);
      await dock.fill("node1");
      await dock.press("Tab");
      await expect(page.locator("#zpr-config-form-error")).toContainText("raw editor");
      expect(await page.locator("#zpr-config-source").inputValue()).toBe(inline);
      page.once("dialog", dialog => dialog.accept());
      await page.locator("#zpr-config-mode-toggle").click();
      await expect(page.locator("#zpr-config-source")).toBeVisible();
    });

    test("invalid form numbers block save and discard refreshes the form", async ({ page, appURL, api, configEndpoint }) => {
      await open(page, appURL, api, configEndpoint);
      const number = input(page, ["trusted_services", "directory", "expiration_seconds"]);
      await number.fill("-1");
      await number.press("Tab");
      await expect(page.locator("#zpr-config-form-error")).toContainText("integer from 0 to 4294967295");
      await expect(page.locator("#zpr-config-validate")).toBeDisabled();
      await page.locator("#zpr-config-files-toggle").click();
      await expect(page.locator("#zpr-config-save")).toBeDisabled();
      await expect(page.locator("#zpr-config-discard")).toBeEnabled();
      page.once("dialog", dialog => dialog.accept());
      await page.locator("#zpr-config-discard").click();
      await expect(page.locator("#zpr-config-form-error")).toBeHidden();
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("");
      await expect(page.locator("#zpr-config-modified")).toBeHidden();
    });

    test("history, discard and optimistic revision saves use the selected source", async ({ page, appURL, api, configEndpoint }) => {
      let record = { id: "versioned-config", category_id: "test", kind: "configuration", name: "Versioned config", current_revision: 2,
        content: '[visa_service]\ndock_node = "current"\n' };
      api.policy.records.push(record);
      api.handlers.set("/api/policy/records/versioned-config", route => route.fulfill({ json: record }));
      api.handlers.set("/api/policy/records/versioned-config/revisions", async route => {
        if (route.request().method() === "POST") {
          const body = route.request().postDataJSON();
          expect(body.expected_revision).toBe(2);
          expect(body.content).toContain('dock_node = "updated"');
          record = { ...record, current_revision: 3, content: body.content };
          await route.fulfill({ json: record });
        } else await route.fulfill({ json: [{ number: 1 }, { number: record.current_revision }] });
      });
      api.handlers.set("/api/policy/records/versioned-config/revisions/1", route => route.fulfill({ json: {
        number: 1, content: '[visa_service]\ndock_node = "original"\n',
      } }));
      await open(page, appURL, api, configEndpoint);
      await page.locator("#zpr-config-picker-toggle").click();
      page.once("dialog", dialog => dialog.accept());
      await page.locator('[data-record-id="versioned-config"]').click();
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("current");
      await page.locator("#zpr-config-history-menu summary").click();
      await page.locator("#zpr-config-history").getByRole("button", { name: /Version 1/ }).click();
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("original");
      await expect(page.locator('#zpr-config-history [data-revision="1"]')).toHaveAttribute("aria-current", "true");
      await page.locator("#zpr-config-files-toggle").click();
      page.once("dialog", dialog => dialog.accept());
      await page.locator("#zpr-config-discard").click();
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("current");
      await edit(page, ["visa_service", "dock_node"], "updated");
      await input(page, ["visa_service", "dock_node"]).focus();
      await page.keyboard.press(process.platform === "darwin" ? "Meta+s" : "Control+s");
      await expect(page.locator("#zpr-config-status")).toHaveText("Saved version 3; runtime unchanged.");
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("updated");
      await expect(page.locator("#zpr-config-modified")).toBeHidden();
    });

    test("imports and New refresh the form without changing runtime", async ({ page, appURL, api, configEndpoint }) => {
      await open(page, appURL, api, configEndpoint);
      page.once("dialog", dialog => dialog.accept());
      await page.locator("#zpr-config-file-input").setInputFiles({
        name: "Imported.zplc", mimeType: "text/plain", buffer: Buffer.from('[visa_service]\ndock_node = "imported"\n'),
      });
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("imported");
      await expect(page.locator("#zpr-config-title")).toHaveText("Imported");
      await page.locator("#zpr-config-files-toggle").click();
      page.once("dialog", dialog => dialog.accept());
      await page.locator("#zpr-config-new").click();
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("");
      await expect(page.locator("#zpr-config-title")).toHaveText("Untitled");
      expect(await page.locator("#zpr-config-source").inputValue()).toBe("");
    });

    test("removes optional sections and preserves analysis ownership after form edits", async ({ page, appURL, api, configEndpoint }) => {
      await open(page, appURL, api, configEndpoint);
      api.handlers.set("/api/policy/config/check", route => route.fulfill({ json: { valid: true, diagnostics: "Source-local analysis" } }));
      await page.locator("#zpr-config-validate").click();
      await expect(page.locator("#zpr-config-status")).toHaveText("Source-local analysis");
      await edit(page, ["visa_service", "dock_node"], "node1");
      await expect(page.locator("#zpr-config-status")).toBeHidden();
      await expect(page.locator("#zpr-config-validate")).not.toHaveAttribute("data-analysis-state", "success");
      page.once("dialog", dialog => dialog.accept());
      await page.locator('[data-config-table=\'["visa_service"]\']').getByRole("button", { name: "Remove section" }).click();
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("");
      expect(await page.locator("#zpr-config-source").inputValue()).not.toContain("[visa_service]");
      expect(await page.locator("#zpr-config-source").inputValue()).toContain("# keep this comment");
      await expect(page.locator("#zpr-config-validate")).toBeEnabled();
    });

    test("rejects delayed mutations after an edit-return cycle in the same source", async ({ page, appURL, api, configEndpoint }) => {
      await open(page, appURL, api, configEndpoint);
      let release;
      let started = false;
      const gate = new Promise(resolve => { release = resolve; });
      api.handlers.set("/api/policy/config/form", async route => {
        const body = route.request().postDataJSON();
        const response = await fetch(configEndpoint + "/api/policy/config/form", { method: "POST", body: JSON.stringify(body) });
        if (body.operation) { started = true; await gate; }
        await route.fulfill({ status: response.status, json: await response.json() });
      });
      const dock = input(page, ["visa_service", "dock_node"]);
      await dock.fill("stale");
      await dock.press("Tab");
      await expect.poll(() => started).toBe(true);
      await page.evaluate(() => {
        const source = document.getElementById("zpr-config-source");
        const original = source.value;
        source.value += "\n# assistant edit\n";
        source.dispatchEvent(new Event("input", { bubbles: true }));
        source.value = original;
        source.dispatchEvent(new Event("input", { bubbles: true }));
      });
      release();
      await expect(dock).toHaveValue("node0");
      expect(await page.locator("#zpr-config-source").inputValue()).toBe(source);
      await expect(page.locator("#zpr-config-form-error")).toBeHidden();
    });

    test("ignores delayed form responses after the raw source changes", async ({ page, appURL, api, configEndpoint }) => {
      await open(page, appURL, api, configEndpoint);
      let release;
      let started = false;
      const gate = new Promise(resolve => { release = resolve; });
      api.handlers.set("/api/policy/config/form", async route => {
        const body = route.request().postDataJSON();
        const response = await fetch(configEndpoint + "/api/policy/config/form", { method: "POST", body: JSON.stringify(body) });
        if (body.operation) { started = true; await gate; }
        await route.fulfill({ status: response.status, json: await response.json() });
      });
      const dock = input(page, ["visa_service", "dock_node"]);
      await dock.fill("stale");
      await dock.press("Tab");
      await expect.poll(() => started).toBe(true);
      page.once("dialog", dialog => dialog.accept());
      await page.locator("#zpr-config-mode-toggle").click();
      const fresh = '[visa_service]\ndock_node = "fresh"\n';
      await page.locator("#zpr-config-source").fill(fresh);
      release();
      await page.locator("#zpr-config-mode-toggle").click();
      await expect(input(page, ["visa_service", "dock_node"])).toHaveValue("fresh");
      expect(await page.locator("#zpr-config-source").inputValue()).toBe(fresh);
    });
  });
}
