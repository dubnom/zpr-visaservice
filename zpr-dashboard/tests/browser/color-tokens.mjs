import { readFile, readdir } from "node:fs/promises";
import { editorCSP } from "./code-editor.mjs";

const palettes = [
  ["BuiltIn", "294879", "e8edf9", "8da4cb"],
  ["Regular", "285b3a", "e8f3e9", "80ad8a"],
  ["Visa", "873c2b", "ffe5df", "d89480"],
  ["Gateway", "754000", "fff1c8", "cc963d"],
  ["ZPR", "124e78", "e3eff8", "598cb4"],
  ["Policy", "214e68", "e4f0f6", "6395ae"],
  ["Control", "66521e", "f4f0da", "ab963e"],
  ["Auth", "793947", "f7e8eb", "bd7c88"],
  ["Attribute", "31594c", "e6f1ec", "78a591"],
  ["Application", "47525a", "e8ebf0", "909ba3"],
  ["Node", "42613b", "ecf1df", "91a875"],
  ["Logger", "6c3e1f", "f5ece4", "b68657"],
  ['Trusted("file")', "285d64", "e7f1f3", "4e8790"],
  ['Trusted("rest/1")', "754820", "fff0df", "b8753f"],
  ["Not in current policy", "55616b", "edf0f2", "a5aeb6"],
];

const rgb = hex => `rgb(${hex.match(/../g).map(value => parseInt(value, 16)).join(", ")})`;

export function registerColorTokenTests(test, expect) {
  test("CSS color tokens update mounted shadow editors without changing page or terminal surfaces", async ({ page, appURL, api }) => {
    const violations = [];
    page.on("console", message => {
      if (/Content Security Policy|violates.*directive/i.test(message.text())) violations.push(message.text());
    });
    await page.route(appURL + "/", async route => {
      const response = await route.fetch();
      await route.fulfill({ response, headers: { ...response.headers(), "Content-Security-Policy": editorCSP } });
    });
    await page.goto(appURL + "/#policy");
    const host = page.locator('.code-editor-host[data-source="policy-source"]');
    const content = host.locator(".cm-content");
    await expect(content).toBeVisible();
    await page.addStyleTag({ url: appURL + "/machine-logs.css" });
    await page.evaluate(() => {
      const terminal = document.createElement("div");
      terminal.id = "palette-terminal";
      terminal.className = "machine-log-output";
      terminal.textContent = "Log output";
      document.querySelector(".main-content").append(terminal);
    });
    await content.fill('define Employee as user where user.name == "Alice";');
    const keyword = host.locator(".cm-token-keyword").first();
    await expect(keyword).toHaveCSS("color", "rgb(229, 166, 108)");
    await expect(host.locator(".cm-editor")).toHaveCSS("background-color", "rgb(23, 33, 30)");
    await page.evaluate(() => {
      document.documentElement.style.setProperty("--syntax-keyword", "rgb(1, 2, 3)");
      document.documentElement.style.setProperty("--source-surface", "rgb(4, 5, 6)");
      document.documentElement.style.setProperty("--source-text", "rgb(230, 231, 232)");
      document.documentElement.style.setProperty("--source-gutter-surface", "rgb(7, 8, 9)");
      document.documentElement.style.setProperty("--text-on-strong", "rgb(10, 11, 12)");
    });
    await expect(keyword).toHaveCSS("color", "rgb(1, 2, 3)");
    await expect(host.locator(".cm-editor")).toHaveCSS("background-color", "rgb(4, 5, 6)");
    await expect(host.locator(".cm-editor")).toHaveCSS("color", "rgb(230, 231, 232)");
    await expect(host.locator(".cm-gutters")).toHaveCSS("background-color", "rgb(7, 8, 9)");
    await expect(page.locator("body")).toHaveCSS("background-color", "rgb(243, 246, 242)");
    await expect(page.locator("#palette-terminal")).toHaveCSS("background-color", "rgb(0, 0, 0)");
    await expect(page.locator("#palette-terminal")).toHaveCSS("color", "rgb(255, 255, 255)");
    expect(violations).toEqual([]);
  });

  test("CSS semantic component tokens give both sidebars one scoped navigation palette", async ({ page, appURL, api }) => {
    await page.route("**/palette-navigation.html", route => route.fulfill({
      contentType: "text/html",
      body: `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">
        <link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/sidebar.css">
        </head><body><div class="app-shell"><aside class="sidebar">
        <div class="sidebar-header"><a class="brand"><span class="brand-mark"><i></i></span><span><strong>ZPR</strong><small>Control</small></span></a><button class="sidebar-toggle">Toggle</button></div>
        <nav class="primary-nav"><a class="nav-link" href="#normal">Normal</a><a class="nav-link active" href="#active">Active</a></nav>
        <div class="sidebar-foot"><span class="tiny-dot"></span>Connected</div>
        </aside><main class="main-content">Main</main></div></body></html>`,
    }));
    await page.goto(appURL + "/palette-navigation.html");
    for (const simulator of [false, true]) {
      if (simulator) await page.addStyleTag({ url: appURL + "/simulator.css" });
      await expect(page.locator(".sidebar")).toHaveCSS("background-color", "rgb(23, 33, 30)");
      await expect(page.locator(".sidebar")).toHaveCSS("color", "rgb(244, 247, 242)");
      await expect(page.locator(".nav-link.active")).toHaveCSS("background-color", "rgb(181, 227, 79)");
      await page.evaluate(() => {
        document.documentElement.style.setProperty("--sidebar-link-accent", "rgb(1, 2, 3)");
        const sidebar = document.querySelector(".sidebar").style;
        sidebar.setProperty("--sidebar-surface", "rgb(4, 5, 6)");
        sidebar.setProperty("--sidebar-text", "rgb(7, 8, 9)");
        sidebar.setProperty("--sidebar-control-border", "rgb(10, 11, 12)");
        sidebar.setProperty("--sidebar-link-text", "rgb(13, 14, 15)");
        sidebar.setProperty("--sidebar-control-hover", "rgb(16, 17, 18)");
        sidebar.setProperty("--sidebar-hover-text", "rgb(19, 20, 21)");
        sidebar.setProperty("--sidebar-active-edge", "rgb(22, 23, 24)");
      });
      for (const [selector, property, value] of [
        [".sidebar", "background-color", "rgb(4, 5, 6)"],
        [".sidebar", "color", "rgb(7, 8, 9)"],
        [".brand", "color", "rgb(7, 8, 9)"],
        [".sidebar-toggle", "color", "rgb(7, 8, 9)"],
        [".sidebar-toggle", "border-color", "rgb(10, 11, 12)"],
        [".brand-mark", "border-color", "rgb(10, 11, 12)"],
        [".brand-mark i", "background-color", "rgb(1, 2, 3)"],
        [".brand small", "color", "rgb(1, 2, 3)"],
        [".tiny-dot", "background-color", "rgb(1, 2, 3)"],
        [".nav-link:not(.active)", "color", "rgb(13, 14, 15)"],
        [".nav-link.active", "background-color", "rgb(1, 2, 3)"],
        [".nav-link.active", "box-shadow", "rgb(22, 23, 24) 4px 0px 0px 0px inset"],
      ]) await expect(page.locator(selector)).toHaveCSS(property, value);
      await page.locator(".nav-link:not(.active)").hover();
      await expect(page.locator(".nav-link:not(.active)")).toHaveCSS("background-color", "rgb(16, 17, 18)");
      await expect(page.locator(".nav-link:not(.active)")).toHaveCSS("color", "rgb(19, 20, 21)");
      await expect(page.locator("body")).toHaveCSS("background-color", "rgb(243, 246, 242)");
      await page.locator(".main-content").hover();
      await page.evaluate(() => {
        document.documentElement.removeAttribute("style");
        document.querySelector(".sidebar").removeAttribute("style");
      });
    }
  });

  test("CSS semantic component tokens share surfaces and focus across both applications", async ({ page, appURL, api }) => {
    await page.route("**/palette-shared.html", route => route.fulfill({
      contentType: "text/html",
      body: `<!doctype html><html><head>
        ${["app", "diagnostics", "provisioning", "editor-assistant", "organization-profile", "help", "policy-browser", "change-review"].map(name => `<link rel="stylesheet" href="/${name}.css">`).join("")}
        </head><body>
        <input class="topology-search">
        <div class="diagnostics-toolbar"><input type="search"></div>
        <div class="editor-page"><div class="policy-identity"><input></div></div>
        <input class="compact-input">
        <div class="provisioning-fields"><input></div>
        <zpr-policy-browser><input type="search"></zpr-policy-browser>
        <div class="metric-card">Metric</div><div class="topology-node-card">Node</div>
        <div class="zpr-help-dialog">Help</div>
        <div class="provisioning-notice">Notice</div>
        </body></html>`,
    }));
    await page.goto(appURL + "/palette-shared.html");
    const inputs = [".topology-search", ".diagnostics-toolbar input", ".policy-identity input",
      ".compact-input", ".provisioning-fields input", "zpr-policy-browser input"];
    for (const selector of inputs) await expect(page.locator(selector)).toHaveCSS("background-color", "rgb(255, 255, 255)");
    for (const simulator of [false, true]) {
      if (simulator) await page.addStyleTag({ url: appURL + "/simulator.css" });
      await page.evaluate(simulator => {
        const root = document.documentElement.style;
        root.setProperty("--surface-panel", simulator ? "rgb(11, 12, 13)" : "rgb(1, 2, 3)");
        root.setProperty("--surface-subtle", "rgb(4, 5, 6)");
        root.setProperty("--focus-border", "rgb(7, 8, 9)");
        root.setProperty("--focus-ring", "rgba(10, 20, 30, 0.5)");
        root.setProperty("--status-warning-surface", "rgb(31, 32, 33)");
      }, simulator);
      for (const selector of [...inputs, ".topology-node-card"]) {
        await expect(page.locator(selector)).toHaveCSS("background-color", simulator ? "rgb(11, 12, 13)" : "rgb(1, 2, 3)");
      }
      for (const selector of [".metric-card", ".zpr-help-dialog"]) {
        await expect(page.locator(selector)).toHaveCSS("background-color", "rgb(4, 5, 6)");
      }
      await expect(page.locator(".provisioning-notice")).toHaveCSS("background-color", "rgb(31, 32, 33)");
      for (const selector of [".topology-search", ".policy-identity input", ".compact-input"]) {
        await page.locator(selector).focus();
        await expect(page.locator(selector)).toHaveCSS("border-color", "rgb(7, 8, 9)");
        const width = selector === ".compact-input" ? 1 : 2;
        await expect(page.locator(selector)).toHaveCSS("box-shadow", `rgba(10, 20, 30, 0.5) 0px 0px 0px ${width}px`);
      }
    }
    await page.evaluate(() => {
      document.documentElement.style.setProperty("--surface-input", "rgb(41, 42, 43)");
      document.documentElement.style.setProperty("--profile-input-surface", "rgb(51, 52, 53)");
    });
    await expect(page.locator(".topology-search")).toHaveCSS("background-color", "rgb(41, 42, 43)");
    await expect(page.locator(".compact-input")).toHaveCSS("background-color", "rgb(51, 52, 53)");
    await expect(page.locator(".topology-node-card")).toHaveCSS("background-color", "rgb(11, 12, 13)");
    await page.evaluate(() => {
      document.querySelector('link[href="/app.css"]').remove();
      document.querySelectorAll("style").forEach(style => style.remove());
      document.documentElement.removeAttribute("style");
    });
    for (const selector of inputs.slice(1)) await expect(page.locator(selector)).toHaveCSS("background-color", "rgb(255, 255, 255)");
    await expect(page.locator(".zpr-help-dialog")).toHaveCSS("background-color", "rgb(248, 250, 247)");
    await page.locator(".policy-identity input").focus();
    await expect(page.locator(".policy-identity input")).toHaveCSS("border-color", "rgb(98, 133, 111)");
  });

  test("CSS semantic component tokens preserve secondary surfaces and standalone directory graphs", async ({ page, appURL, api }) => {
    await page.route("**/palette-secondary.html", route => route.fulfill({
      contentType: "text/html",
      body: `<!doctype html><html><head>
        ${["app", "assertions", "help", "organization-profile", "policy-browser", "world", "ldap-org-graph"].map(name => `<link rel="stylesheet" href="/${name}.css">`).join("")}
        </head><body>
        <span id="assertion-run-status" data-state="pass">Pass</span>
        <div class="assertion-catalog-tabs"><button aria-selected="true">Selected</button></div>
        <div class="zpr-help-dialog"><p>Help</p><pre>Example</pre></div>
        <input class="compact-input">
        <zpr-policy-browser><pre class="pb-source"><span class="pb-keyword">define</span></pre><p class="pb-error">Error</p></zpr-policy-browser>
        <p class="world-error">Error</p><p class="world-beta">Beta</p>
        <ldap-org-graph><svg><g class="ldap-graph-node person"><rect width="40" height="20"/><text>Person</text></g></svg></ldap-org-graph>
        </body></html>`,
    }));
    await page.goto(appURL + "/palette-secondary.html");
    for (const [selector, property, value] of [
      ["#assertion-run-status", "color", "rgb(40, 91, 58)"],
      [".assertion-catalog-tabs button", "background-color", "rgb(234, 244, 217)"],
      [".zpr-help-dialog", "background-color", "rgb(248, 250, 247)"],
      [".zpr-help-dialog p", "color", "rgb(62, 77, 69)"],
      [".zpr-help-dialog pre", "background-color", "rgb(240, 244, 239)"],
      [".compact-input", "background-color", "rgb(255, 255, 255)"],
      [".pb-source", "background-color", "rgb(23, 33, 30)"],
      [".pb-source", "color", "rgb(228, 235, 229)"],
      [".pb-keyword", "color", "rgb(229, 166, 108)"],
      [".pb-error", "color", "rgb(161, 51, 40)"],
      [".world-error", "background-color", "rgb(255, 241, 238)"],
      [".world-beta", "color", "rgb(89, 67, 29)"],
      ["ldap-org-graph .person rect", "fill", "rgb(232, 242, 247)"],
      ["ldap-org-graph .person rect", "stroke", "rgb(81, 130, 155)"],
    ]) await expect(page.locator(selector)).toHaveCSS(property, value);
    await expect(page.locator("ldap-org-graph > svg")).toHaveCSS("background-image",
      "radial-gradient(rgb(223, 230, 223) 0.8px, rgba(0, 0, 0, 0) 0.8px)");
    await page.evaluate(() => {
      for (const link of document.querySelectorAll('link:not([href="/ldap-org-graph.css"])')) link.remove();
    });
    await expect(page.locator("ldap-org-graph")).toHaveCSS("background-color", "rgb(255, 255, 255)");
    await expect(page.locator("ldap-org-graph")).toHaveCSS("border-color", "rgb(203, 213, 206)");
    await expect(page.locator("ldap-org-graph")).toHaveCSS("color", "rgb(23, 33, 30)");
  });

  test("CSS semantic component tokens preserve source review assistant and terminal states", async ({ page, appURL, api }) => {
    await page.route("**/palette-details.html", route => route.fulfill({
      contentType: "text/html",
      body: `<!doctype html><html><head>
        ${["app", "gateways", "node-stats", "trusted-source", "security-review", "editor-assistant", "machine-logs"].map(name => `<link rel="stylesheet" href="/${name}.css">`).join("")}
        </head><body>
        <div class="gateway-record-item" data-kind="gateway"><span class="policy-kind-icon">Gateway</span></div>
        <p id="gateway-form-error">Error</p><p class="node-stats-error">Error</p>
        <table class="trusted-source-table"><thead><tr><th>Source</th></tr></thead></table>
        <div class="security-review-inventory-details"><span data-change="added">Added</span><span data-change="removed">Removed</span></div>
        <div data-security-review><table class="security-review-table"><tbody>
        <tr data-severity="review"><td class="security-indicator">Review</td></tr>
        <tr data-severity="high"><td class="security-indicator">High</td></tr></tbody></table></div>
        <div class="assistant-message"><pre class="assistant-code">Code</pre></div>
        <div class="editor-page"><div class="policy-identity"><input></div></div>
        <p class="editor-page-status" data-state="error">Error</p>
        <p class="machine-logs-error">Unavailable</p>
        <div class="machine-log-panel running"><span class="machine-log-state">Running</span>
        <div class="machine-log-output" tabindex="0"><p class="machine-log-error">Error</p></div></div>
        <div class="machine-log-type-toggle"><button aria-pressed="true">Selected</button></div>
        <div class="machine-log-source-options"><label><input type="checkbox" checked>Source</label></div>
        </body></html>`,
    }));
    await page.goto(appURL + "/palette-details.html");
    for (const [selector, property, value] of [
      [".gateway-record-item .policy-kind-icon", "color", "rgb(201, 134, 18)"],
      ["#gateway-form-error", "color", "rgb(164, 56, 48)"],
      [".node-stats-error", "color", "rgb(163, 38, 38)"],
      [".trusted-source-table th", "background-color", "rgb(233, 238, 232)"],
      ['[data-change="added"]', "color", "rgb(24, 137, 75)"],
      ['[data-change="removed"]', "color", "rgb(164, 56, 48)"],
      ['[data-severity="review"] .security-indicator', "color", "rgb(156, 75, 38)"],
      ['[data-severity="high"] .security-indicator', "color", "rgb(164, 56, 36)"],
      [".assistant-code", "background-color", "rgb(251, 252, 250)"],
      [".policy-identity input", "border-color", "rgb(170, 185, 173)"],
      [".editor-page-status", "color", "rgb(188, 73, 61)"],
      [".machine-logs-error", "background-color", "rgb(255, 240, 237)"],
      [".machine-log-state", "color", "rgb(33, 132, 91)"],
      [".machine-log-output", "background-color", "rgb(0, 0, 0)"],
      [".machine-log-output", "color", "rgb(255, 255, 255)"],
      [".machine-log-error", "color", "rgb(255, 183, 167)"],
      [".machine-log-type-toggle button", "background-color", "rgb(23, 77, 61)"],
      [".machine-log-source-options label", "background-color", "rgb(234, 244, 217)"],
    ]) await expect(page.locator(selector)).toHaveCSS(property, value);
    await page.locator(".policy-identity input").focus();
    await expect(page.locator(".policy-identity input")).toHaveCSS("border-color", "rgb(98, 133, 111)");
    await expect(page.locator(".policy-identity input")).toHaveCSS("box-shadow", "rgba(181, 227, 79, 0.26) 0px 0px 0px 2px");
    await page.evaluate(() => {
      document.documentElement.style.setProperty("--logs-terminal-surface", "rgb(1, 2, 3)");
      document.documentElement.style.setProperty("--logs-terminal-text", "rgb(240, 241, 242)");
    });
    await expect(page.locator(".machine-log-output")).toHaveCSS("background-color", "rgb(1, 2, 3)");
    await expect(page.locator(".machine-log-output")).toHaveCSS("color", "rgb(240, 241, 242)");
    await expect(page.locator("body")).toHaveCSS("background-color", "rgb(243, 246, 242)");
  });

  test("CSS semantic component tokens preserve state colors and standalone surfaces", async ({ page, appURL, api }) => {
    await page.route("**/palette-components.html", route => route.fulfill({
      contentType: "text/html",
      body: `<!doctype html><html><head>
        ${["app", "sidebar", "diagnostics", "provisioning", "code-editor", "geography"].map(name => `<link rel="stylesheet" href="/${name}.css">`).join("")}
        </head><body>
        <aside class="sidebar"><button class="sidebar-toggle">Toggle</button><span class="nav-group-label">Navigation</span></aside>
        <main class="main-content">
        <div class="diagnostics-toolbar"><input type="search"></div>
        <div class="diagnostics-source" data-state="stale"><span class="diagnostics-state">Stale</span></div>
        <div class="diagnostics-source" data-state="error"><span class="diagnostics-state">Error</span></div>
        <div class="diagnostics-error">Unavailable</div>
        <div class="provisioning-notice">Notice</div>
        <p id="provisioning-preflight-blockers">Blocked</p>
        <div class="cm-result-gutter"><button>Result</button><button data-state="fail">Fail</button><button data-state="warning">Warning</button></div>
        <div class="geography-switch"><button aria-pressed="false">Topology</button><button aria-pressed="true">World</button></div>
        </main></body></html>`,
    }));
    await page.goto(appURL + "/palette-components.html");
    for (const [selector, property, value] of [
      ["body", "color", "rgb(23, 33, 30)"],
      ["body", "background-color", "rgb(243, 246, 242)"],
      [".sidebar-toggle", "border-color", "rgb(102, 128, 118)"],
      [".sidebar-toggle", "color", "rgb(244, 247, 242)"],
      [".nav-group-label", "color", "rgb(132, 147, 138)"],
      [".diagnostics-toolbar input", "background-color", "rgb(255, 255, 255)"],
      ['[data-state="stale"] .diagnostics-state', "color", "rgb(135, 93, 5)"],
      ['[data-state="error"] .diagnostics-state', "color", "rgb(150, 60, 52)"],
      [".diagnostics-error", "background-color", "rgb(255, 241, 238)"],
      [".diagnostics-error", "border-left-color", "rgb(164, 56, 48)"],
      [".provisioning-notice", "background-color", "rgb(255, 240, 229)"],
      [".provisioning-notice", "border-left-color", "rgb(226, 122, 66)"],
      ["#provisioning-preflight-blockers", "color", "rgb(138, 50, 31)"],
      [".cm-result-gutter button:first-child", "background-color", "rgb(35, 54, 43)"],
      [".cm-result-gutter button:first-child", "color", "rgb(210, 231, 209)"],
      ['.cm-result-gutter [data-state="fail"]', "color", "rgb(255, 192, 181)"],
      ['.cm-result-gutter [data-state="warning"]', "color", "rgb(255, 224, 167)"],
      ['.geography-switch [aria-pressed="false"]', "background-color", "rgb(244, 247, 250)"],
      ['.geography-switch [aria-pressed="true"]', "background-color", "rgb(23, 78, 120)"],
    ]) await expect(page.locator(selector)).toHaveCSS(property, value);
    await page.evaluate(() => {
      document.documentElement.style.setProperty("--text-primary", "rgb(1, 2, 3)");
      document.documentElement.style.setProperty("--surface-page", "rgb(4, 5, 6)");
    });
    await expect(page.locator("body")).toHaveCSS("color", "rgb(1, 2, 3)");
    await expect(page.locator("body")).toHaveCSS("background-color", "rgb(4, 5, 6)");
    await page.evaluate(() => {
      const host = document.createElement("div");
      host.id = "palette-shadow-editor";
      document.body.append(host);
      const shadow = host.attachShadow({ mode: "open" });
      shadow.innerHTML = '<link rel="stylesheet" href="/code-editor.css"><div class="cm-result-gutter"><button data-state="fail">Fail</button></div>';
    });
    await expect(page.locator("#palette-shadow-editor button")).toHaveCSS("color", "rgb(255, 192, 181)");
    await expect(page.locator("#palette-shadow-editor button")).toHaveCSS("border-color", "rgb(169, 99, 88)");
    await page.addStyleTag({ url: appURL + "/simulator.css" });
    await expect(page.locator("body")).toHaveCSS("color", "rgb(1, 2, 3)");
    await expect(page.locator("body")).toHaveCSS("background-color", "rgb(4, 5, 6)");
    await page.route("**/palette-review.html", route => route.fulfill({
      contentType: "text/html",
      body: '<!doctype html><link rel="stylesheet" href="/change-review.css"><p id="status" class="error">Error</p><pre>Source</pre>',
    }));
    await page.goto(appURL + "/palette-review.html");
    await expect(page.locator("html")).toHaveCSS("background-color", "rgb(243, 246, 242)");
    await expect(page.locator("#status")).toHaveCSS("color", "rgb(140, 48, 36)");
    await expect(page.locator("pre")).toHaveCSS("background-color", "rgb(0, 0, 0)");
    await expect(page.locator("pre")).toHaveCSS("color", "rgb(255, 255, 255)");
  });

  test("CSS semantic component tokens keep literal palettes out of component declarations", async ({ page, appURL, api }) => {
    const assets = new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url);
    const sources = await Promise.all((await readdir(assets)).filter(name => name.endsWith(".css"))
      .map(async name => ({ name, css: await readFile(new URL(name, assets), "utf8") })));
    await page.goto(appURL + "/#services");
    const literals = await page.evaluate(sources => {
      const found = [];
      function visit(name, rules) {
        for (const rule of rules) {
          if (rule.style) {
            for (const property of rule.style) {
              if (property.startsWith("--")) continue;
              const value = rule.style.getPropertyValue(property);
              if (/#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?)\(|\b(?:white|black)\b/i.test(value)) {
                found.push(`${name}: ${rule.selectorText} ${property}: ${value}`);
              }
            }
          } else if (rule.cssRules) visit(name, rule.cssRules);
        }
      }
      for (const { name, css } of sources) {
        const sheet = new CSSStyleSheet();
        sheet.replaceSync(css);
        visit(name, sheet.cssRules);
      }
      return found;
    }, sources);
    expect(literals).toEqual([]);
  });

  test("CSS color tokens own first-party script colors and preserve the external SVG image", async ({ page, appURL, api }) => {
    const assets = new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url);
    for (const file of (await readdir(assets)).filter(name => /\.(?:js|html)$/.test(name) && name !== "ansi_up.js")) {
      expect(await readFile(new URL(file, assets), "utf8"), file).not.toMatch(/#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?)\(\s*[\d.]/i);
    }
    expect(await readFile(new URL("../../browser/code-editor.js", import.meta.url), "utf8")).not.toMatch(/#[0-9a-f]{3,8}\b/i);
    const svg = await readFile(new URL("geography-land.svg", assets), "utf8");
    const legacy = svg
      .replace(/<style>[\s\S]*?<\/style>\n/, "")
      .replace('<rect class="map-water"', '<rect')
      .replace('<g class="map-land"', '<g fill="#f7f5ed" stroke="#8c9ba5"');
    await page.route("**/palette-*.svg", route => route.fulfill({
      contentType: "image/svg+xml",
      body: route.request().url().endsWith("palette-legacy.svg") ? legacy : svg,
    }));
    await page.goto(appURL + "/#services");
    const images = [];
    for (const version of ["legacy", "css"]) {
      await page.evaluate(version => {
        let image = document.getElementById("palette-test-image");
        if (!image) {
          image = document.createElement("img");
          image.id = "palette-test-image";
          image.width = 600;
          image.height = 300;
          document.body.prepend(image);
        }
        image.src = `/palette-${version}.svg`;
        return image.decode();
      }, version);
      images.push(await page.locator("#palette-test-image").screenshot());
    }
    expect(images[1].equals(images[0])).toBe(true);
  });

  test("CSS color tokens preserve every built-in service palette and browser chrome", async ({ page, appURL, api }) => {
    api.snapshot.services = palettes.map(([kind], index) => ({
      service_name: `palette-${index}`, service_kind: kind, actor_cn: "adapter",
      zpr_addr: "fd00::1", service_endpoints: "tcp:8080",
    }));
    await page.goto(appURL + "/#services");
    await expect(page.locator(".service-type-chip")).toHaveCount(palettes.length);
    for (const [index, [, text, surface, border]] of palettes.entries()) {
      const chip = page.locator(".service-type-chip").nth(index);
      await expect(chip).toHaveCSS("color", rgb(text));
      await expect(chip).toHaveCSS("background-color", rgb(surface));
      await expect(chip).toHaveCSS("border-color", rgb(border));
    }
    await expect(page.locator('meta[name="theme-color"]')).toHaveAttribute("content", "#17211e");
  });

  test("CSS color tokens update service chips swatches and SVG badges without rendering again", async ({ page, appURL, api }) => {
    api.snapshot.actors = [
      { cn: "hub", node: true, zpr_addr: "fd00::ff", node_details: { adapters: ["adapter"], in_sync: true } },
      { cn: "adapter", node: false, zpr_addr: "fd00::1" },
    ];
    api.snapshot.services = [
      { service_name: "regular", service_kind: "Regular", actor_cn: "adapter", zpr_addr: "fd00::1", service_endpoints: "tcp:8080" },
      { service_name: "custom", service_kind: 'Trusted("custom")', actor_cn: "adapter", zpr_addr: "fd00::1", service_endpoints: "tcp:8081" },
    ];
    await page.goto(appURL + "/#services");
    const chip = page.locator('.service-type-chip[data-service-type="Regular"]');
    await expect(chip).toHaveCSS("color", rgb("285b3a"));
    await page.locator("#pause-poll").click();
    await page.evaluate(() => {
      const style = document.documentElement.style;
      style.setProperty("--service-regular-text", "rgb(1, 2, 3)");
      style.setProperty("--service-regular-surface", "rgb(4, 5, 6)");
      style.setProperty("--service-regular-border", "rgb(7, 8, 9)");
      style.setProperty("--service-fallback-text", "0% 10%");
      style.setProperty("--service-fallback-surface", "0% 90%");
      style.setProperty("--service-fallback-border", "0% 50%");
    });
    await expect(chip).toHaveCSS("color", "rgb(1, 2, 3)");
    await expect(chip).toHaveCSS("background-color", "rgb(4, 5, 6)");
    await expect(chip).toHaveCSS("border-color", "rgb(7, 8, 9)");
    await page.getByText("Type colors", { exact: true }).click();
    const swatch = page.locator(".service-type-legend-item").filter({ hasText: /^Regular$/ }).locator(".service-type-swatch");
    await expect(swatch).toHaveCSS("background-color", "rgb(1, 2, 3)");
    await page.getByRole("link", { name: "Map", exact: true }).click();
    const badge = page.locator('.graph-service-badge[data-service-type="Regular"]');
    await expect(badge.locator(":scope > rect")).toHaveCSS("fill", "rgb(4, 5, 6)");
    await expect(badge.locator(":scope > rect")).toHaveCSS("stroke", "rgb(7, 8, 9)");
    await expect(badge.locator(":scope > text")).toHaveCSS("fill", "rgb(1, 2, 3)");
    const custom = page.locator('.graph-service-badge').filter({ has: page.locator("title", { hasText: "custom" }) });
    await expect(custom.locator(":scope > rect")).toHaveCSS("fill", "rgb(230, 230, 230)");
    await page.evaluate(() => {
      document.documentElement.style.setProperty("--service-regular-surface", "rgb(10, 11, 12)");
    });
    await expect(badge.locator(":scope > rect")).toHaveCSS("fill", "rgb(10, 11, 12)");
  });

  test("CSS color tokens retain scenario pulse colors and honor reduced motion", async ({ page, appURL, api }) => {
    await page.emulateMedia({ reducedMotion: "no-preference" });
    await page.goto(appURL + "/scenarios.html");
    await expect.poll(() => page.evaluate(() => typeof renderScenarioRun)).toBe("function");
    await page.evaluate(() => renderScenarioRun({
      scenario_id: "color-test", state: "running", total_steps: 2, current_step: 1, steps: [],
    }));
    const state = page.locator("#scenario-run-state");
    const frames = await state.evaluate(async element => {
      const animation = element.getAnimations().at(-1);
      animation.pause();
      animation.effect.updateTiming({ easing: "linear" });
      const shadows = [];
      for (const offset of [0, 0.45, 1]) {
        animation.currentTime = animation.effect.getTiming().duration * offset;
        await new Promise(requestAnimationFrame);
        shadows.push(getComputedStyle(element).boxShadow);
      }
      return shadows;
    });
    expect(frames).toEqual([
      "rgb(18, 81, 160) 0px 0px 0px 1px inset",
      "rgba(36, 115, 216, 0.4) 0px 0px 0px 3px, rgb(18, 81, 160) 0px 0px 0px 1px inset",
      "rgb(18, 81, 160) 0px 0px 0px 1px inset",
    ]);
    await state.evaluate(element => element.getAnimations().forEach(animation => animation.cancel()));
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.evaluate(() => renderScenarioRun({
      scenario_id: "color-test", state: "running", total_steps: 2, current_step: 2, steps: [],
    }));
    expect(await state.evaluate(element => element.getAnimations().length)).toBe(0);
  });
}
