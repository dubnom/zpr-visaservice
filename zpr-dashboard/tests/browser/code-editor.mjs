import { createHash } from "node:crypto";

const colorStyles = ["229, 166, 108", "215, 167, 207", "130, 145, 136", "216, 189, 128", "146, 197, 223", "194, 216, 138", "184, 215, 139", "186, 201, 191", "228, 235, 229"];
const colorHashes = colorStyles.map(color => `'sha256-${createHash("sha256").update(`color: rgb(${color});`).digest("base64")}'`).join(" ");
const editorCSP = `default-src 'self'; style-src 'self'; style-src-attr 'unsafe-hashes' ${colorHashes}; script-src 'self'; connect-src 'self'; img-src 'self' data:`;

export function registerCodeEditorTests(test, expect) {
  test("CodeMirror formats policy and assertions as isolated undoable edits", async ({ page, appURL, api }) => {
    api.handlers.set("/api/assertions/format", route => route.fulfill({ json: { source: 'group "Operators" members >= 2;\n', warnings: [] } }));
    await enable(page, appURL);
    const samples = [
      ["policy-source", "#policy-format", "define    Employee as user.   \n\n\n"],
      ["assertion-source", "#assertion-format", 'group   "Operators" members >= 2;   \n'],
    ];
    for (const [id, format, original] of samples) {
      if (id === "assertion-source") page.once("dialog", dialog => dialog.accept());
      const editor = id === "policy-source" ? content(page) : await assertions(page);
      await editor.fill(original);
      await editor.press("ControlOrMeta+End");
      await editor.pressSequentially(" ");
      const beforeFormat = original + " ";
      await page.locator(format).click();
      await expect(page.locator(`#${id}`)).not.toHaveValue(beforeFormat);
      const formatted = await page.locator(`#${id}`).inputValue();
      await page.locator(format).press("ControlOrMeta+z");
      await expect(page.locator(`#${id}`)).toHaveValue(beforeFormat);
      await page.locator(format).press("ControlOrMeta+Shift+Z");
      await expect(page.locator(`#${id}`)).toHaveValue(formatted);
      await editor.press("ControlOrMeta+z");
      await expect(page.locator(`#${id}`)).toHaveValue(beforeFormat);
      await editor.press("ControlOrMeta+z");
      await expect(page.locator(`#${id}`)).toHaveValue(original);
    }
    expect(api.counts.get("/api/policy/records/test-policy/revisions")).toBe(1);
    expect(api.counts.get("/api/policy/records/test-assertions/revisions")).toBe(1);
  });

  test("CodeMirror places clicks and the caret correctly after two-axis scrolling", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    const original = Array.from({ length: 450 }, (_, index) => `# Row ${String(index).padStart(3, "0")} ${"abcdefghij".repeat(18)}`).join("\n");
    await content(page).fill(original);
    const scroller = host(page).locator(".cm-scroller");
    for (const [top, left] of [[2300, 450], [4800, 700], [600, 120]]) {
      await scroller.evaluate((element, [top, left]) => { element.scrollTop = top; element.scrollLeft = left; }, [top, left]);
      await expect.poll(async () => host(page).evaluate(element => {
        const root = element.shadowRoot;
        const viewport = root.querySelector(".cm-scroller").getBoundingClientRect();
        const line = [...root.querySelectorAll(".cm-line")].find(line => {
          const box = line.getBoundingClientRect();
          return box.top > viewport.top + 40 && box.bottom < viewport.bottom - 20;
        });
        return Boolean(line);
      })).toBe(true);
      const target = await host(page).evaluate(element => {
        const root = element.shadowRoot;
        const viewport = root.querySelector(".cm-scroller").getBoundingClientRect();
        const line = [...root.querySelectorAll(".cm-line")].find(line => {
          const box = line.getBoundingClientRect();
          return box.top > viewport.top + 40 && box.bottom < viewport.bottom - 20;
        });
        const walker = document.createTreeWalker(line, NodeFilter.SHOW_TEXT);
        let node, offset = 0;
        while ((node = walker.nextNode())) {
          for (let index = 0; index < node.length; index++) {
            const range = document.createRange();
            range.setStart(node, index);
            range.setEnd(node, index + 1);
            const box = range.getBoundingClientRect();
            if (box.left > viewport.left + 150 && box.right < viewport.right - 30) {
              return { x: box.left + 0.2, y: (box.top + box.bottom) / 2, index: document.getElementById("policy-source").value.indexOf(line.textContent) + offset + index, top: box.top, bottom: box.bottom };
            }
          }
          offset += node.length;
        }
        throw new Error("No visible character after scrolling");
      });
      await page.mouse.click(target.x, target.y);
      await expect.poll(() => page.locator("#policy-source").evaluate(source => source.selectionStart)).toBe(target.index);
      const caret = await host(page).evaluate(element => {
        const selection = element.shadowRoot.getSelection?.() || document.getSelection();
        const box = selection.getRangeAt(0).getBoundingClientRect();
        return { top: box.top, bottom: box.bottom, left: box.left };
      });
      expect(Math.abs(caret.top - target.top)).toBeLessThan(2);
      expect(Math.abs(caret.left - target.x)).toBeLessThan(2);
      await expect.poll(() => host(page).locator(".cm-cursor").first().evaluate(cursor => cursor.getBoundingClientRect().top)).toBeGreaterThan(target.top - 2);
      const drawn = await host(page).locator(".cm-cursor").first().boundingBox();
      expect(Math.abs(drawn.y - target.top)).toBeLessThan(3);
      expect(Math.abs(drawn.x - target.x)).toBeLessThan(3);
      await expect(content(page)).toHaveCSS("caret-color", "rgba(0, 0, 0, 0)");
      await scroller.evaluate(element => { element.scrollTop += 18; });
      await expect.poll(async () => {
        const drawn = await host(page).locator(".cm-cursor").first().boundingBox();
        return Math.abs(drawn.y - (target.top - 18));
      }).toBeLessThan(3);
      await page.keyboard.type("X");
      await expect(page.locator("#policy-source")).toHaveValue(original.slice(0, target.index) + "X" + original.slice(target.index));
      await content(page).press("ControlOrMeta+z");
      await expect(page.locator("#policy-source")).toHaveValue(original);
    }
    page.once("dialog", dialog => dialog.accept());
    const assertion = await assertions(page);
    await assertion.fill(Array.from({ length: 250 }, (_, index) => `// Row ${index} ${"word ".repeat(20)}`).join("\n"));
    await assertion.press("ControlOrMeta+End");
    await assertion.press("Home");
    await assertion.press("ArrowRight");
    const assertionHost = page.locator('.code-editor-host[data-source="assertion-source"]');
    const beforeScroll = await assertionHost.locator(".cm-cursor").first().boundingBox();
    await assertionHost.locator(".cm-scroller").evaluate(element => { element.scrollTop -= 18; });
    await expect.poll(async () => {
      const drawn = await assertionHost.locator(".cm-cursor").first().boundingBox();
      return Math.abs(drawn.y - (beforeScroll.y + 18));
    }).toBeLessThan(3);
  });

  test("CodeMirror toolbar Undo and Redo work after Analyze without intercepting assistant text fields", async ({ page, appURL, api }) => {
    assistantReady(api);
    api.handlers.set("/api/policy/check", route => route.fulfill({ json: { valid: false, diagnostics: "Error at line 1, column 1: sample error" } }));
    api.handlers.set("/api/assertions/evaluate", route => route.fulfill({ json: { status: "pass", results: [] } }));
    await enable(page, appURL);
    for (const [id, analyze] of [["policy-source", "#policy-check"], ["assertion-source", "#assertion-analyze"]]) {
      const editor = id === "policy-source" ? content(page) : await assertions(page);
      const before = await page.locator(`#${id}`).inputValue();
      await editor.press("ControlOrMeta+End");
      await editor.pressSequentially("extra");
      await page.locator(analyze).click();
      await expect(page.locator(analyze)).not.toHaveAttribute("data-analysis-state", "pending");
      await page.locator(analyze).press("ControlOrMeta+z");
      await expect(page.locator(`#${id}`)).toHaveValue(before);
      await page.locator(analyze).press("ControlOrMeta+Shift+Z");
      await expect(page.locator(`#${id}`)).toHaveValue(before + "extra");
      await editor.press("ControlOrMeta+z");
      await expect(page.locator(`#${id}`)).toHaveValue(before);
    }
    const sourceBefore = await page.locator("#assertion-source").inputValue();
    await page.locator("#assistant-question").fill("separate question");
    await page.locator("#assistant-question").press("ControlOrMeta+z");
    await expect(page.locator("#assertion-source")).toHaveValue(sourceBefore);
  });

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
    await expect(page.locator("#code-editor-status")).toHaveText("");
    await expect(page.locator("#code-editor-trial")).toHaveCount(0);
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

  test("CodeMirror matches previous policy and assertion font metrics", async ({ page, appURL, api }) => {
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

  test("CodeMirror matches legacy policy and assertion token colors and classifications", async ({ page, appURL, api }) => {
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

  test("CodeMirror applies AI line replacements and insertions as one isolated undoable unsaved edit", async ({ page, appURL, api }) => {
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
    await expect(page.locator("#assistant-messages")).toContainText("Updated the definition and inserted a comment.");
    await expect(page.locator("#assistant-messages")).not.toContainText("Applied as one unsaved");
    await expect(page.locator("#assistant-messages .assistant-insert")).toHaveCount(0);
    expect(requests[0]).toMatchObject({ editor: "policy", direct_edits: true, source: beforeAI });
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue(beforeAI);
    await content(page).press("ControlOrMeta+Shift+Z");
    await expect(page.locator("#policy-source")).toHaveValue(afterAI);
    expect([...api.counts.keys()].some(path => /\/(save|check|test|evaluate|stage)$/.test(path))).toBe(false);
    expect(api.counts.get("/api/policy/records/test-policy/revisions")).toBe(1);
  });

  test("CodeMirror rejects malformed ambiguous overlapping oversized and no-op AI edits atomically", async ({ page, appURL, api }) => {
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
      [[{ before: "unique", after: "unique" }], /do not change/],
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

  test("CodeMirror distinguishes answers without edits and reports per-request and session usage without retrying", async ({ page, appURL, api }) => {
    assistantReady(api);
    let output = 90;
    let fail = false;
    const requests = [];
    api.handlers.set("/api/policy/assistant", route => {
      requests.push(route.request().postDataJSON());
      if (fail) return route.fulfill({ status: 502, json: { error: "Output limit reached. No changes were applied.", input_tokens: 11, output_tokens: 300 } });
      return route.fulfill({ json: { answer: JSON.stringify({ answer: "Here is the explanation.", edits: [] }), input_tokens: 10, output_tokens: output } });
    });
    await enable(page, appURL);
    const original = await page.locator("#policy-source").inputValue();
    await expect(page.locator("#assistant-disclosure")).toHaveText("Submitting sends the current draft and chat history to Anthropic.");
    await expect(page.locator("#assistant-usage")).toBeHidden();
    await expect(page.locator("#policy-assistant-pane .assistant-options summary")).toHaveText("Model and Tokens");
    await page.locator("#policy-assistant-pane .assistant-options summary").click();
    await expect(page.locator("#policy-assistant-pane .assistant-options #assistant-usage")).toBeVisible();
    await expect(page.locator("#assistant-max-tokens")).toHaveValue("2400");
    expect(await page.locator("#assistant-max-tokens option").evaluateAll(options => options.map(option => Number(option.value)))).toEqual([900, 1800, 2400, 3600, 7200]);
    for (const max of [900, 1800, 2400, 3600, 7200]) {
      await page.locator("#assistant-max-tokens").selectOption(String(max));
      await ask(page, "Explain this policy");
      await expect(page.locator("#assistant-messages .assistant-empty")).toHaveText("No editor changes were returned.");
      await expect(page.locator("#assistant-messages .assistant-error")).toHaveCount(0);
      await expect(page.locator("#assistant-usage")).toContainText(`90 output / ${max.toLocaleString()} max`);
      await expect(page.locator("#policy-source")).toHaveValue(original);
    }
    expect(requests.map(request => request.max_tokens)).toEqual([900, 1800, 2400, 3600, 7200]);
    expect(requests.every(request => request.direct_edits === true)).toBe(true);
    await expect(page.locator("#assistant-usage")).toContainText("Session: 50 input · 450 output tokens");
    await page.locator("#assistant-max-tokens").selectOption("900");
    fail = true;
    await ask(page);
    await expect(page.locator("#assistant-messages .assistant-error")).toContainText("Output limit reached");
    await expect(page.locator("#assistant-usage")).toContainText("Session: 61 input · 750 output tokens");
    fail = false;
    output = 950;
    await ask(page);
    await expect(page.locator("#assistant-messages .assistant-error")).toContainText("provider reported 950");
    await expect(page.locator("#assistant-usage")).toContainText("950 output / 900 max");
    await expect(page.locator("#assistant-usage")).toContainText("Session: 71 input · 1,700 output tokens");
    await expect(page.locator("#policy-source")).toHaveValue(original);
    expect(requests).toHaveLength(7);
    await page.locator("#policy-assistant-pane [data-assistant-clear]").click();
    await expect(page.locator("#assistant-usage")).toHaveText("Session: 0 input · 0 output tokens");
  });

  test("CodeMirror applies assertion AI edits without saving and rejects responses for another record", async ({ page, appURL, api }) => {
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

  test("CodeMirror keeps drafts intact on structured response errors and allows a successful retry", async ({ page, appURL, api }) => {
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

  test("CodeMirror rejects delayed AI changes after source and permission changes without legacy Insert", async ({ page, appURL, api }) => {
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
    await page.evaluate(() => { document.getElementById("policy-source").readOnly = true; });
    release();
    await expect(page.locator("#assistant-messages")).toContainText("mode or permissions changed");
    await expect(page.locator("#policy-source")).toHaveValue("original");
    expect(direct).toBe(true);
    await expect(page.locator("#assistant-send")).toBeDisabled();
    await expect(page.locator("#assistant-messages .assistant-insert")).toHaveCount(0);
    await expect(page.locator("#policy-source")).toHaveValue("original");
  });

  test("CodeMirror starts automatically and preserves text, undo and completion without writes", async ({ page, appURL, api }) => {
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
    await expect(page.locator("#policy-source")).not.toBeVisible();
    await expect(page.locator("#policy-source")).toHaveValue('define Test as user.\n');
    await expect(content(page)).toHaveText('define Test as user.');
    await page.reload();
    await expect(content(page)).toBeVisible();
    await expect(page.locator("#policy-source")).not.toBeVisible();
    expect(initial.length).toBeGreaterThan(0);
    expect([...api.counts.keys()].some(path => /\/(save|check|test|evaluate|stage)$/.test(path))).toBe(false);
  });

  test("CodeMirror completes LDAP attributes by short and qualified names with undo", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    for (const prefix of ["dep", "user.dep", "tit", "user."]) {
      const attribute = prefix === "tit" ? "user.title:" : "user.department:";
      const original = `define Employee as user\n  with ${prefix}`;
      await content(page).fill(original);
      await content(page).press("ControlOrMeta+End");
      await content(page).press("Control+Space");
      await expect(host(page).getByRole("option", { name: attribute, exact: true })).toBeVisible();
      await expect(host(page).getByRole("option", { name: "device.secure:", exact: true })).toHaveCount(0);
      await host(page).getByRole("option", { name: attribute, exact: true }).click();
      await expect(page.locator("#policy-source")).toHaveValue(`define Employee as user\n  with ${attribute}`);
      await content(page).press("ControlOrMeta+z");
      await expect(page.locator("#policy-source")).toHaveValue(original);
    }
  });

  test("CodeMirror automatically completes LDAP namespaces after dots for adapter definitions", async ({ page, appURL, api }) => {
    api.policy.attributes.push(
      { attribute: "greatlakes.department", source: "LDAP" },
      { attribute: "greatlakes.location", source: "LDAP" },
      { attribute: "zpr.adapter.cn", source: "LDAP" },
    );
    await enable(page, appURL);
    for (const [prefix, attribute] of [
      ["define great as adapter with greatlakes", "greatlakes.department:"],
      ["define great as an adapter with zpr.adapter", "zpr.adapter.cn:"],
    ]) {
      await content(page).fill(prefix);
      await content(page).press("ControlOrMeta+End");
      await content(page).pressSequentially(".");
      await expect(host(page).getByRole("option", { name: attribute, exact: true })).toBeVisible();
      await host(page).getByRole("option", { name: attribute, exact: true }).click();
      await expect(page.locator("#policy-source")).toHaveValue(prefix.slice(0, prefix.lastIndexOf(" ") + 1) + attribute);
      await content(page).press("ControlOrMeta+z");
      await expect(page.locator("#policy-source")).toHaveValue(prefix + ".");
    }
    for (const text of [
      "define great as adapter.",
      "# define great as adapter with greatlakes.",
      'define great as adapter with greatlakes.department:"unfinished.',
      "define great as adapter with greatlakes.department:Finance.",
    ]) {
      await content(page).fill(text);
      await content(page).press("ControlOrMeta+End");
      await content(page).press("Control+Space");
      await expect(host(page).getByRole("option")).toHaveCount(0);
    }
  });

  test("CodeMirror completes production-shaped LDAP names in a multi-statement policy", async ({ page, appURL, api }) => {
    api.policy.attributes = [
      { attribute: "device.greatlakes.department", source: "ou" },
      { attribute: "device.greatlakes.location", source: "zprMachineLocation" },
    ];
    await enable(page, appURL);
    for (const prefix of ["greatlakes.", "greatlakes.dep", "device.greatlakes."]) {
      const previous = 'define First as adapter.\ndefine Second as device.\n';
      await content(page).fill(previous + "define great as adapter with ");
      await content(page).press("ControlOrMeta+End");
      await content(page).pressSequentially(prefix);
      const attribute = prefix.startsWith("device.") ? "device.greatlakes.department:" : "greatlakes.department:";
      const option = host(page).getByRole("option", { name: attribute, exact: true });
      await expect(option).toBeVisible();
      await option.click();
      await expect(page.locator("#policy-source")).toHaveValue(previous + "define great as adapter with " + attribute);
      await content(page).press("ControlOrMeta+z");
      await expect(page.locator("#policy-source")).toHaveValue(previous + "define great as adapter with " + prefix);
    }
  });

  test("CodeMirror adapter completion offers namespace then attributes without injecting a type prefix or colon", async ({ page, appURL, api }) => {
    api.policy.attributes = [
      { attribute: "device.greatlakes.department", source: "ou" },
      { attribute: "device.greatlakes.location", source: "zprMachineLocation" },
    ];
    await enable(page, appURL);
    const original = "define foo as adapter with grea";
    await content(page).fill("define foo as adapter with ");
    await content(page).press("ControlOrMeta+End");
    await content(page).pressSequentially("grea");
    await expect(host(page).getByRole("option")).toHaveCount(1);
    await expect(host(page).getByRole("option", { name: "greatlakes.", exact: true })).toBeVisible();
    await host(page).getByRole("option", { name: "greatlakes.", exact: true }).click();
    await expect(page.locator("#policy-source")).toHaveValue("define foo as adapter with greatlakes.");
    await expect(host(page).getByRole("option", { name: "greatlakes.location:", exact: true })).toBeVisible();
    await expect(host(page).getByRole("option", { name: "greatlakes.department:", exact: true })).toBeVisible();
    await host(page).getByRole("option", { name: "greatlakes.location:", exact: true }).click();
    await expect(page.locator("#policy-source")).toHaveValue("define foo as adapter with greatlakes.location:");
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue("define foo as adapter with greatlakes.");
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue(original);
  });

  test("CodeMirror Discard cancels safely or reloads the saved policy and resets source history", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    const original = await page.locator("#policy-source").inputValue();
    await content(page).fill("define Changed as adapter.\n");
    await page.locator("#policy-files-toggle").click();
    page.once("dialog", dialog => dialog.dismiss());
    await page.locator("#policy-refresh").click();
    await expect(page.locator("#policy-source")).toHaveValue("define Changed as adapter.\n");
    await page.locator("#policy-files-toggle").click();
    page.once("dialog", dialog => dialog.accept());
    await page.locator("#policy-refresh").click();
    await expect(page.locator("#policy-source")).toHaveValue(original);
    await content(page).press("ControlOrMeta+z");
    await expect(page.locator("#policy-source")).toHaveValue(original);
    await expect(page.locator("#policy-modified-indicator")).not.toBeVisible();
    expect([...api.counts.keys()].some(path => /\/(save|check|test|evaluate|stage)$/.test(path))).toBe(false);
  });

  test("CodeMirror Discard reports a reload failure without losing the draft", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    await content(page).fill("define KeepDraft as adapter.\n");
    api.handlers.set("/api/policy/records/test-policy", route => route.fulfill({ status: 503, json: { error: "Repository unavailable" } }));
    await page.locator("#policy-files-toggle").click();
    page.once("dialog", dialog => dialog.accept());
    await page.locator("#policy-refresh").click();
    await expect(page.locator("#policy-load-status")).toContainText("Repository unavailable");
    await expect(page.locator("#policy-source")).toHaveValue("define KeepDraft as adapter.\n");
  });

  test("CodeMirror assertion Discard reloads saved source and resets history", async ({ page, appURL, api }) => {
    await enable(page, appURL);
    const editor = await assertions(page);
    const original = await page.locator("#assertion-source").inputValue();
    await editor.fill('group "Changed" members >= 2;\n');
    await page.locator("#policy-files-toggle").click();
    page.once("dialog", dialog => dialog.accept());
    await page.locator("#assertion-reload").click();
    await expect(page.locator("#assertion-source")).toHaveValue(original);
    await editor.press("ControlOrMeta+z");
    await expect(page.locator("#assertion-source")).toHaveValue(original);
  });

  test("CodeMirror shares Find Replace and Wrap across policy and assertions", async ({ page, appURL, api }) => {
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
    await expect(page.locator("#assertion-source")).toHaveValue('group "Reviewers" members >= 2;\n');
    await expect(page.locator("#assertion-source")).not.toBeVisible();
  });

  test("CodeMirror restores read-only states, source-owned diagnostics and result actions", async ({ page, appURL, api }) => {
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

  test("CodeMirror disables AI until startup completes and uses current record permissions", async ({ page, appURL, api }) => {
    assistantReady(api);
    let release;
    await page.route("**/vendor/code-editor.js?*", async route => {
      await new Promise(resolve => { release = resolve; });
      await route.continue();
    });
    await page.goto(appURL + "/#policy", { waitUntil: "domcontentloaded" });
    await expect(page.locator("#policy-source")).toBeEnabled();
    await expect(page.locator("#code-editor-status")).toHaveText("Loading editor…");
    await expect(page.locator("#policy-source")).not.toBeVisible();
    await expect(page.locator("#assistant-question")).toBeDisabled();
    await expect.poll(() => Boolean(release)).toBe(true);
    release();
    await expect(content(page)).toBeVisible();
    await expect(content(page)).toHaveAttribute("contenteditable", "true");
    await expect(page.locator("#assistant-question")).toBeEnabled();
    expect(api.counts.get("/api/policy/assistant") || 0).toBe(0);
  });

  test("CodeMirror reports bundle-load failure without exposing a legacy editor or sending AI requests", async ({ page, appURL, api }) => {
    assistantReady(api);
    await page.route("**/vendor/code-editor.js?*", route => route.abort());
    await page.goto(appURL + "/#policy");
    await expect(page.locator("#policy-source")).toBeEnabled();
    await expect(page.locator("#code-editor-status")).toContainText("Could not load the editor");
    await expect(page.locator("#code-editor-trial")).toHaveCount(0);
    await expect(page.locator("#policy-source")).not.toBeVisible();
    await expect(page.locator("#assistant-question")).toBeDisabled();
    await expect(page.locator("#assistant-send")).toBeDisabled();
    expect(api.counts.get("/api/policy/assistant") || 0).toBe(0);
  });

  test("CodeMirror analyzes exact policy text, rejects stale responses and keeps Save confirmation", async ({ page, appURL, api }) => {
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

  test("CodeMirror saves exact assertion text through the shared shortcut and clears markers on record changes", async ({ page, appURL, api }) => {
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
    await expect(page.locator("#assertion-source")).not.toBeVisible();
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
