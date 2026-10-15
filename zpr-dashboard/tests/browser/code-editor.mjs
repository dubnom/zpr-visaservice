import { createHash } from "node:crypto";

const colorStyles = ["229, 166, 108", "215, 167, 207", "130, 145, 136", "216, 189, 128", "146, 197, 223", "194, 216, 138", "184, 215, 139", "186, 201, 191", "228, 235, 229"];
const colorHashes = colorStyles.map(color => `'sha256-${createHash("sha256").update(`color: rgb(${color});`).digest("base64")}'`).join(" ");
const editorCSP = `default-src 'self'; style-src 'self'; style-src-attr 'unsafe-hashes' ${colorHashes}; script-src 'self'; connect-src 'self'; img-src 'self' data:`;

export function registerCodeEditorTests(test, expect) {
  const host = page => page.locator('.code-editor-host[data-source="policy-source"]');
  const content = page => host(page).locator(".cm-content");
  async function editorPage(page, appURL) {
    await page.route(appURL + "/", async route => {
      const response = await route.fetch();
      await route.fulfill({
        response,
        headers: { ...response.headers(), "content-security-policy": editorCSP },
      });
    });
  }
  async function enable(page, appURL) {
    await editorPage(page, appURL);
    await page.goto(appURL + "/#policy");
    await expect(page.locator("#policy-source")).toBeEnabled();
    await page.locator("#code-editor-trial").check();
    await expect(page.locator("#code-editor-trial-status")).toContainText("CodeMirror trial on");
    await expect(content(page)).toBeVisible();
  }
  async function assertions(page) {
    await page.locator("#policy-picker-toggle").click();
    await page.locator('[data-record-id="test-assertions"]').click();
    await expect(page.locator("#assertion-source")).toBeEnabled();
    return page.locator('.code-editor-host[data-source="assertion-source"] .cm-content');
  }

  function assistantReady(api) {
    Object.assign(api.policy, { assistant_ready: true, assistant_model: "test-model", assistant_models: ["test-model"] });
  }
  async function ask(page, message = "Update the requested lines") {
    await page.locator("#assistant-question").fill(message);
    await page.locator("#assistant-send").click();
  }

  test("CodeMirror trial matches previous policy and assertion font metrics", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    for (const id of ["policy-source", "assertion-source"]) {
      if (id === "assertion-source") await assertions(page);
      const metrics = await page.evaluate(id => {
        const previous = getComputedStyle(document.getElementById(id));
        const current = getComputedStyle(document.querySelector(`.code-editor-host[data-source="${id}"]`).shadowRoot.querySelector(".cm-content"));
        return ["fontSize", "fontFamily", "lineHeight"].map(property => [property, previous[property], current[property]]);
      }, id);
      for (const [property, previous, current] of metrics) expect(current, `${id} ${property}`).toBe(previous);
    }
  });

  test("CodeMirror trial matches legacy policy and assertion token colors and classifications", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    const samples = [
      ["policy-source", 'define Employee as user.\nallow Employee with department: "Finance" to access service.\n# Comment\n'],
      ["assertion-source", 'group "Operators" members >= 2;\n// Comment\nattribute "mail" present;\n'],
    ];
    for (const [id, text] of samples) {
      if (id === "assertion-source") page.once("dialog", dialog => dialog.accept());
      const editor = id === "policy-source" ? content(page) : await assertions(page);
      await editor.fill(text);
      const previousID = id === "policy-source" ? "policy-highlight" : "assertion-highlight";
      const tokens = await page.locator(`#${previousID} span`).evaluateAll(nodes => nodes.map(node => ({ text: node.textContent, color: getComputedStyle(node).color })));
      const highlighted = page.locator(`.code-editor-host[data-source="${id}"] .cm-content span[class*="cm-token-"]`);
      await expect.poll(() => highlighted.evaluateAll(nodes => nodes.map(node => ({ text: node.textContent, color: getComputedStyle(node).color })))).toEqual(tokens);
      await editor.press("ControlOrMeta+End");
      await editor.pressSequentially(id === "policy-source" ? 'define Another as device.\n' : 'group "Extra" members >= 1;\n');
      await expect(page.locator(`#${id}`)).toHaveValue(text + (id === "policy-source" ? 'define Another as device.\n' : 'group "Extra" members >= 1;\n'));
    }
  });

  test("CodeMirror trial applies AI line replacements and insertions as one isolated undoable unsaved edit", async ({ page, appURL, api }) => {
    assistantReady(api);
    const requests = [];
    api.handlers.set("/api/policy/assistant", route => {
      requests.push(route.request().postDataJSON());
      return route.fulfill({ json: { answer: JSON.stringify({
        answer: "Updated the definition and inserted a comment.",
        edits: [
          { before: "define First as user.\n", after: "define First as device.\n" },
          { before: "# Keep this line.\n", after: "# Keep this line.\n# Inserted by AI.\n" },
        ],
      }) } });
    });
    await enable(page, appURL);
    const original = "define First as user.\ndefine Second as user.\n# Keep this line.\n";
    await content(page).fill(original);
    await content(page).press("ControlOrMeta+End");
    await content(page).pressSequentially("# My unsaved change.");
    const beforeAI = await page.locator("#policy-source").inputValue();
    await ask(page);
    const afterAI = beforeAI.replace("define First as user.", "define First as device.").replace("# Keep this line.\n", "# Keep this line.\n# Inserted by AI.\n");
    await expect(page.locator("#policy-source")).toHaveValue(afterAI);
    await expect(page.locator("#assistant-messages")).toContainText("Applied as one unsaved edit");
    await expect(page.locator("#assistant-messages .assistant-insert")).toHaveCount(0);
    expect(requests[0]).toMatchObject({ editor: "policy", direct_edits: true, source: beforeAI });
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue(beforeAI);
    await content(page).press("ControlOrMeta+Shift+Z");
    await expect(page.locator("#policy-source")).toHaveValue(afterAI);
    expect([...api.counts.keys()].some(path => /\/(save|check|test|evaluate|stage)$/.test(path))).toBe(false);
    expect(api.counts.get("/api/policy/records/test-policy/revisions")).toBe(1);
  });

  test("CodeMirror trial rejects malformed ambiguous overlapping and oversized AI edits atomically", async ({ page, appURL, api }) => {
    assistantReady(api);
    let answer;
    api.handlers.set("/api/policy/assistant", route => route.fulfill({ json: { answer } }));
    await enable(page, appURL);
    await content(page).fill("abc\nabc\nunique\n");
    await page.evaluate(() => { document.getElementById("policy-source").maxLength = 30; });
    for (const [edits, message] of [
      [[{ before: "unique", after: "changed" }, { before: "missing", after: "x" }], /missing or ambiguous/],
      [[{ before: "abc", after: "x" }], /ambiguous/],
      [[{ before: "unique", after: "x" }, { before: "nique", after: "y" }], /overlap/],
      [[{ before: "", after: "x" }], /ambiguous/],
      [[{ before: "unique", after: "x".repeat(100) }], /size limit/],
      [[{ before: "unique", after: null }], /before and after/],
    ]) {
      answer = JSON.stringify({ answer: "Attempted change", edits });
      await ask(page);
      await expect(page.locator("#assistant-messages .assistant-error")).toContainText(message);
      await expect(page.locator("#policy-source")).toHaveValue("abc\nabc\nunique\n");
    }
    answer = "```json\ntruncated";
    await ask(page);
    await expect(page.locator("#assistant-messages .assistant-error")).toContainText("invalid edit JSON");
    await expect(page.locator("#policy-source")).toHaveValue("abc\nabc\nunique\n");
  });

  test("CodeMirror trial applies assertion AI edits without saving and rejects responses for another record", async ({ page, appURL, api }) => {
    assistantReady(api);
    const record = api.policy.records.find(record => record.id === "test-assertions");
    record.content_type = "text/vnd.zpr.assertions";
    record.content = 'group "Operators" members >= 1;\n';
    let release;
    let delayed = false;
    api.handlers.set("/api/policy/assistant", async route => {
      expect(route.request().postDataJSON()).toMatchObject({ editor: "assertion", direct_edits: true, source: record.content });
      if (delayed) await new Promise(resolve => { release = resolve; });
      return route.fulfill({ json: { answer: JSON.stringify({ answer: "Raised minimum", edits: [{ before: 'group "Operators" members >= 1;', after: 'group "Operators" members >= 2;' }] }) } });
    });

    await enable(page, appURL);
    const assertion = await assertions(page);
    await ask(page);
    await expect(page.locator("#assertion-source")).toHaveValue('group "Operators" members >= 2;\n');
    await assertion.press("ControlOrMeta+z");
    await expect(page.locator("#assertion-source")).toHaveValue(record.content);
    delayed = true;
    await ask(page);
    await expect.poll(() => Boolean(release)).toBe(true);
    await page.locator("#policy-picker-toggle").click();
    await page.locator('[data-record-id="test-policy"]').click();
    const response = page.waitForResponse("**/api/policy/assistant");
    release();
    await (await response).finished();
    await expect(page.locator("#assistant-messages")).toHaveText("No conversation yet.");
    await expect(page.locator("#policy-source")).toHaveValue("define Employee as user.\n");
    await expect(page.locator("#assertion-source")).toHaveValue(record.content);
    expect(api.counts.get("/api/policy/records/test-assertions/revisions")).toBe(1);
  });

  test("CodeMirror trial keeps drafts intact on structured response errors and allows a successful retry", async ({ page, appURL, api }) => {
    assistantReady(api);
    let fail = true;
    api.handlers.set("/api/policy/assistant", route => fail
      ? route.fulfill({ status: 502, json: { error: "The assistant reached the output token limit before completing its edits. No changes were applied. Increase the output limit or request a smaller change." } })
      : route.fulfill({ json: { answer: JSON.stringify({ answer: "Updated the line.", edits: [{ before: "original", after: "changed" }] }) } }));
    await enable(page, appURL);
    await content(page).fill("original\n");
    await ask(page);
    await expect(page.locator("#assistant-messages .assistant-error")).toContainText("Increase the output limit");
    await expect(page.locator("#policy-source")).toHaveValue("original\n");
    await expect(page.locator("#assistant-question")).toBeEnabled();
    fail = false;
    await ask(page, "Change only the first line");
    await expect(page.locator("#policy-source")).toHaveValue("changed\n");
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue("original\n");
    expect(api.counts.get("/api/policy/records/test-policy/revisions")).toBe(1);
  });

  test("CodeMirror trial rejects delayed AI changes after edits and mode changes and preserves legacy Insert", async ({ page, appURL, api }) => {
    assistantReady(api);
    let release;
    let direct;
    api.handlers.set("/api/policy/assistant", async route => {
      direct = route.request().postDataJSON().direct_edits;
      await new Promise(resolve => { release = resolve; });
      return route.fulfill({ json: { answer: direct ? JSON.stringify({ answer: "Change", edits: [{ before: "original", after: "changed" }] }) : "```zpl\ninserted\n```" } });
    });
    await enable(page, appURL);
    await content(page).fill("original");
    await ask(page);
    await expect.poll(() => Boolean(release)).toBe(true);
    await content(page).press("ControlOrMeta+a");
    await content(page).pressSequentially("my later changes");
    release();
    await expect(page.locator("#assistant-messages")).toContainText("editor changed");
    await expect(page.locator("#policy-source")).toHaveValue("my later changes");
    await content(page).fill("original");
    release = null;
    await ask(page);
    await expect.poll(() => Boolean(release)).toBe(true);
    await page.locator("#code-editor-trial").uncheck();
    release();
    await expect(page.locator("#assistant-messages")).toContainText("mode or permissions changed");
    await expect(page.locator("#policy-source")).toHaveValue("original");
    release = null;
    await ask(page);
    await expect.poll(() => Boolean(release)).toBe(true);
    expect(direct).toBeUndefined();
    release();
    await expect(page.locator("#assistant-messages .assistant-insert")).toBeVisible();
    await expect(page.locator("#policy-source")).toHaveValue("original");
  });

  test("CodeMirror trial preserves text, edits, undo, completion and fallback without writes", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    const initial = await page.locator("#policy-source").inputValue();
    await content(page).click();
    await content(page).press("ControlOrMeta+a");
    await content(page).pressSequentially("def");
    await content(page).press("Control+Space");
    await expect(host(page).getByRole("option", { name: "define", exact: true })).toBeVisible();
    await host(page).getByRole("option", { name: "define", exact: true }).click();
    await expect(page.locator("#policy-source")).toHaveValue("define");
    await content(page).fill('define Test as user.\n');
    await expect(page.locator("#policy-modified-indicator")).toBeVisible();
    await content(page).press("ControlOrMeta+End");
    await content(page).press("Tab");
    await expect(page.locator("#policy-source")).toHaveValue('define Test as user.\n  ');
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue('define Test as user.\n');
    await page.locator("#code-editor-trial").uncheck();
    await expect(page.locator("#policy-source")).toBeVisible();
    await expect(page.locator("#policy-source")).toHaveValue('define Test as user.\n');
    await page.locator("#code-editor-trial").check();
    await expect(content(page)).toHaveText('define Test as user.');
    expect(initial.length).toBeGreaterThan(0);
    expect([...api.counts.keys()].some(path => /\/(save|check|test|evaluate|stage)$/.test(path))).toBe(false);
  });

  test("CodeMirror trial shares Find Replace and Wrap across policy and assertions", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    await content(page).fill("define One as user.\ndefine Two as user.\n");
    await content(page).press("ControlOrMeta+h");
    const tools = page.locator('[data-editor-search-target="policy-source,assertion-source"]');
    await tools.getByRole("textbox", { name: "Find what" }).fill("user");
    await tools.getByRole("textbox", { name: "Replace with" }).fill("service");
    await tools.getByRole("button", { name: "Replace All", exact: true }).click();
    await expect(page.locator("#policy-source")).toHaveValue("define One as service.\ndefine Two as service.\n");
    await tools.getByRole("button", { name: "Close", exact: true }).click();
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue("define One as user.\ndefine Two as user.\n");
    page.once("dialog", dialog => dialog.accept());
    const assertion = await assertions(page);
    await expect(assertion).toBeVisible();
    await assertion.fill('group "Operators" members >= 2;\n');
    await assertion.press("ControlOrMeta+h");
    await tools.getByRole("textbox", { name: "Find what" }).fill("Operators");
    await tools.getByRole("textbox", { name: "Replace with" }).fill("Reviewers");
    await tools.getByRole("button", { name: "Replace All", exact: true }).click();
    await expect(page.locator("#assertion-source")).toHaveValue('group "Reviewers" members >= 2;\n');
    await tools.getByRole("button", { name: "Close", exact: true }).click();
    await page.getByRole("checkbox", { name: "Wrap", exact: true }).first().check();
    await expect(page.locator('.code-editor-host[data-source="assertion-source"] .cm-content')).toHaveClass(/cm-lineWrapping/);
    await page.locator("#code-editor-trial").uncheck();
    await expect(page.locator("#assertion-source")).toHaveValue('group "Reviewers" members >= 2;\n');
    await expect(page.locator("#assertion-source")).toBeVisible();
  });

  test("CodeMirror trial restores read-only states, source-owned diagnostics and result actions", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    await page.evaluate(() => {
      const source = document.getElementById("policy-source");
      source.disabled = true;
    });
    await expect(content(page)).toHaveAttribute("contenteditable", "false");
    await page.evaluate(() => {
      const source = document.getElementById("policy-source");
      source.disabled = false;
      source.value = "define One as user.\n";
      document.getElementById("policy-highlight").innerHTML = '<span class="zpl-error">define</span> One as user.\n';
      const row = document.createElement("div");
      row.dataset.line = "1";
      const button = document.createElement("button");
      button.textContent = "ERR";
      button.addEventListener("click", () => { document.getElementById("policy-file-status").textContent = "Exact diagnostic"; });
      row.append(button);
      document.getElementById("policy-test-gutter-content").replaceChildren(row);
    });
    await expect(host(page).locator(".cm-source-error")).toHaveText("define");
    await host(page).getByRole("button", { name: "ERR", exact: true }).click();
    await expect(page.locator("#policy-file-status")).toHaveText("Exact diagnostic");
    await page.evaluate(() => {
      const original = document.querySelector("#policy-test-gutter-content button");
      original.title = "Updated diagnostic";
      original.disabled = true;
    });
    await expect(host(page).getByRole("button", { name: "ERR", exact: true })).toHaveAttribute("title", "Updated diagnostic");
    await expect(host(page).getByRole("button", { name: "ERR", exact: true })).toBeDisabled();
    await content(page).fill("define Changed as user.\n");
    await expect(host(page).locator(".cm-source-error")).toHaveCount(0);
    await expect(host(page).getByRole("button", { name: "ERR", exact: true })).toHaveCount(0);
    await page.evaluate(() => { document.getElementById("policy-source").value = "define Replacement as user.\n"; });
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue("define Replacement as user.\n");
    await content(page).fill("define Same as user.\n");
    await page.evaluate(() => {
      const source = document.getElementById("policy-source");
      source.value = source.value;
    });
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue("define Same as user.\n");
    await content(page).fill("()");
    await content(page).press("End");
    await content(page).press("ArrowLeft");
    await content(page).pressSequentially("x");
    await expect(page.locator("#policy-source")).toHaveValue("(x)");
    await page.evaluate(() => {
      const source = document.getElementById("policy-source");
      source.value = "abc";
      source.maxLength = 4;
    });
    await content(page).press("ControlOrMeta+End");
    await content(page).pressSequentially("de");
    await expect(page.locator("#policy-source")).toHaveValue("abcd");
    await content(page).press("Backspace");
    await expect(page.locator("#policy-source")).toHaveValue("abc");
  });

  test("CodeMirror trial reports bundle-load failure and keeps current source usable", async ({ page, appURL, api }) => {
    await page.route("**/vendor/code-editor.js?*", route => route.abort());
    await page.goto(appURL + "/#policy");
    await expect(page.locator("#policy-source")).toBeEnabled();
    await page.locator("#code-editor-trial").click();
    await expect(page.locator("#code-editor-trial-status")).toContainText("Could not load CodeMirror");
    await expect(page.locator("#code-editor-trial")).not.toBeChecked();
    await expect(page.locator("#policy-source")).toBeVisible();
  });

  test("CodeMirror trial analyzes exact policy text, rejects stale responses and keeps Save confirmation", async ({ page, appURL, api }) => {
    const submitted = [];
    let complete;
    api.handlers.set("/api/policy/check", async route => {
      submitted.push(route.request().postDataJSON().source);
      if (submitted.length === 1) await new Promise(resolve => { complete = resolve; });
      return route.fulfill({ json: { valid: false, diagnostics: "Error at line 1, column 1: exact policy error" } });
    });
    api.handlers.set("/api/policy/test/fixtures", route => route.fulfill({ json: { actors: [], services: [] } }));
    api.handlers.set("/api/policy/test", route => route.fulfill({ json: { services: [] } }));
    await enable(page, appURL);
    await content(page).fill("define Old as user.\n");
    await page.locator("#policy-check").click();
    await expect.poll(() => Boolean(complete)).toBe(true);
    await content(page).fill("define Current as user.\n");
    complete();
    await expect(page.locator("#policy-check")).toBeEnabled();
    await expect(host(page).locator(".cm-source-error")).toHaveCount(0);
    await expect(host(page).getByRole("button")).toHaveCount(0);
    await page.locator("#policy-check").click();
    await expect(page.locator("#policy-check")).toHaveAttribute("data-analysis-state", "error");
    await expect(host(page).locator(".cm-source-error")).toHaveText("define");
    await expect(host(page).getByRole("button", { name: /Compiler error/ })).toBeVisible();
    await content(page).press("ControlOrMeta+s");
    await expect(page.locator("#version-dialog")).toBeVisible();
    await expect(page.locator("#version-warning")).toContainText(/error/i);
    expect(submitted).toEqual(["define Old as user.\n", "define Current as user.\n"]);
    expect(api.counts.get("/api/policy/records/test-policy/revisions")).toBe(1);
    await page.locator("#version-dialog").getByRole("button", { name: "Cancel", exact: true }).click();
  });

  test("CodeMirror trial saves exact assertion text through the shared shortcut and clears markers on record changes", async ({ page, appURL, api }) => {
    const record = api.policy.records.find(record => record.id === "test-assertions");
    record.content_type = "text/vnd.zpr.assertions";
    record.content = 'group "Operators" members >= 1;\n';
    const writes = [];
    api.handlers.set("/api/policy/records/test-assertions/revisions", route => {
      if (route.request().method() === "GET") return route.fulfill({ json: [] });
      writes.push(route.request().postDataJSON());
      record.content = writes.at(-1).content;
      record.current_revision++;
      return route.fulfill({ json: { revision: record.current_revision } });
    });
    await enable(page, appURL);
    const assertion = await assertions(page);
    const text = 'group "Reviewers" members >= 2;\n';
    await assertion.fill(text);
    await assertion.press("ControlOrMeta+s");
    await expect(page.locator("#assertion-message")).toHaveText("Saved Organization assertions / r2");
    expect(writes).toEqual([{ content: text, expected_revision: 1, summary: "Updated assertion set" }]);
    await page.locator("#policy-picker-toggle").click();
    await page.locator('[data-record-id="test-policy"]').click();
    await expect(content(page)).toBeVisible();
    await expect(host(page).getByRole("button")).toHaveCount(0);
    await page.locator("#code-editor-trial").uncheck();
    await expect(page.locator("#assertion-source")).toHaveValue(text);
  });

  test("CodeMirror color-only CSP blocks arbitrary inline styles and scripts", async ({ browser, appURL }) => {
    const page = await browser.newPage();
    try {
      await page.route(appURL + "/editor-csp-check", route => route.fulfill({
        contentType: "text/html", headers: { "Content-Security-Policy": editorCSP },
        body: '<!doctype html><html><body><p id="sample">Sample</p></body></html>',
      }));
      await page.goto(appURL + "/editor-csp-check");
      const result = await page.evaluate(async () => {
        const blocked = [];
        document.addEventListener("securitypolicyviolation", event => blocked.push(event.violatedDirective));
        const sample = document.getElementById("sample");
        sample.setAttribute("style", "color: rgb(229, 166, 108);");
        const allowed = getComputedStyle(sample).color;
        sample.setAttribute("style", "background-color: red;");
        const background = getComputedStyle(sample).backgroundColor;
        const script = document.createElement("script");
        script.textContent = "window.inlineEditorScriptRan = true";
        document.body.append(script);
        await new Promise(resolve => setTimeout(resolve, 100));
        return { allowed, background, scriptRan: Boolean(window.inlineEditorScriptRan), blocked };
      });
      expect(result.allowed).toBe("rgb(229, 166, 108)");
      expect(result.background).toBe("rgba(0, 0, 0, 0)");
      expect(result.scriptRan).toBe(false);
      expect(result.blocked.sort()).toEqual(["script-src-elem", "style-src-attr"]);
    } finally { await page.close(); }
  });

}
