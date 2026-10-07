(() => {
  const source = document.getElementById("scenario-editor-source");
  const advanced = document.getElementById("scenario-editor-advanced");
  const form = document.getElementById("scenario-editor-form");
  const assistant = document.getElementById("scenario-assistant-slot");
  const summary = advanced.querySelector("summary");
  summary.textContent = "Raw JSON editor";
  summary.className = "button button-refresh";
  source.setAttribute("aria-label", "Raw scenario JSON");
  source.setAttribute("wrap", "off");
  source.maxLength = 1048576;
  const workbench = document.createElement("div");
  workbench.className = "scenario-source-workbench";
  advanced.before(workbench);
  workbench.append(advanced, assistant);
  const tools = document.createElement("div");
  tools.className = "policy-editor-tools scenario-source-tools";
  tools.innerHTML = `<details class="config-file-menu"><summary class="button button-refresh">File...</summary><div class="config-file-popover" role="menu" aria-label="Scenario file commands"><button type="button" role="menuitem" data-source-save>Save draft</button><button type="button" role="menuitem" data-source-open>Open JSON...</button><button type="button" role="menuitem" data-source-download>Download JSON...</button></div></details><button class="button" type="button" id="scenario-source-analyze">Analyze</button><button class="button button-save-as-ready" type="button" data-source-format>Format</button><strong>Scenario JSON</strong><span id="scenario-source-modified" hidden>Modified</span><span id="scenario-source-position">Line 1 · Column 1</span><input type="file" accept=".json,application/json" hidden data-source-file>`;
  const editor = document.createElement("div");
  editor.className = "config-source-editor scenario-json-editor";
  const gutter = document.createElement("div");
  gutter.className = "config-source-gutter";
  gutter.id = "scenario-source-gutter";
  gutter.setAttribute("aria-label", "Scenario JSON errors by source line");
  const surface = document.createElement("div");
  surface.className = "config-code-surface";
  const highlight = document.createElement("pre");
  highlight.id = "scenario-source-highlight";
  highlight.setAttribute("aria-hidden", "true");
  source.classList.add("zpr-config-source");
  advanced.append(tools, editor);
  editor.append(gutter, surface);
  surface.append(highlight, source);
  const analyze = tools.querySelector("#scenario-source-analyze");
  let diagnostic = null;
  let fileVersion = 0;

  function syncScroll() {
    gutter.scrollTop = highlight.scrollTop = source.scrollTop;
    highlight.scrollLeft = source.scrollLeft;
    editor.dataset.horizontalOverflow = String(source.scrollWidth > source.clientWidth);
  }

  function position() {
    const before = source.value.slice(0, source.selectionStart);
    tools.querySelector("#scenario-source-position").textContent = `Line ${before.split("\n").length} · Column ${source.selectionStart - before.lastIndexOf("\n")}`;
  }

  function render() {
    highlight.replaceChildren();
    const tokens = /"(?:\\.|[^"\\])*"|\b(?:true|false|null)\b|-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/g;
    let offset = 0;
    for (const match of source.value.matchAll(tokens)) {
      highlight.append(document.createTextNode(source.value.slice(offset, match.index)));
      const token = document.createElement("span");
      const kind = match[0].startsWith('"') ? /^\s*:/.test(source.value.slice(match.index + match[0].length)) ? "attribute" : "string"
        : /^(true|false|null)$/.test(match[0]) ? "keyword" : "value";
      token.className = `zpl-${kind}`;
      token.textContent = match[0];
      highlight.append(token);
      offset = match.index + match[0].length;
    }
    highlight.append(document.createTextNode(source.value.slice(offset) + "\n"));
    gutter.replaceChildren(...source.value.split("\n").map((_, index) => {
      const row = document.createElement("div");
      row.className = "config-gutter-line";
      row.dataset.line = String(index + 1);
      if (diagnostic?.line === index + 1) {
        const marker = document.createElement("button");
        marker.type = "button";
        marker.className = "config-error-marker";
        marker.textContent = "!";
        marker.title = diagnostic.message;
        marker.setAttribute("aria-label", `JSON error on line ${index + 1}: ${diagnostic.message}`);
        marker.addEventListener("click", () => {
          const start = source.value.split("\n").slice(0, index).reduce((length, line) => length + line.length + 1, 0);
          source.focus();
          source.setSelectionRange(start, start + source.value.split("\n")[index].length);
          source.scrollTop = index * Number.parseFloat(getComputedStyle(source).lineHeight);
          syncScroll();
          position();
        });
        row.append(marker);
      }
      return row;
    }));
    tools.querySelector("#scenario-source-modified").hidden = !scenarioEditorDirty;
    position();
    syncScroll();
  }

  function clear() {
    fileVersion++;
    diagnostic = null;
    delete analyze.dataset.analysisState;
    render();
  }
  window.renderScenarioSourceEditor = clear;

  async function check() {
    diagnostic = null;
    const version = ++fileVersion;
    const original = source.value;
    const organization = scenarioEditorOrganization;
    try {
      const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organization)}/scenario-check`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ source: original, scenario_id: scenarioEditorArtifact?.id || "" }),
      });
      const result = await response.json();
      if (version !== fileVersion || organization !== scenarioEditorOrganization) return null;
      if (!response.ok || result.valid !== true) {
        diagnostic = result.line > 0 ? { line: result.line, message: result.error || "Scenario analysis failed." } : null;
        throw new Error(result.error || `Scenario analysis failed (${response.status}).`);
      }
      const scenario = readScenarioEditorSource();
      analyze.dataset.analysisState = "success";
      setScenarioEditorStatus(result.diagnostics);
      render();
      return scenario;
    } catch (error) {
      if (version !== fileVersion || organization !== scenarioEditorOrganization) return null;
      analyze.dataset.analysisState = "error";
      setScenarioEditorStatus(error.message, "error");
      render();
      return null;
    }
  }

  analyze.addEventListener("click", check);
  tools.querySelector("[data-source-format]").addEventListener("click", async () => {
    const scenario = await check();
    if (!scenario) return;
    source.value = JSON.stringify(scenario, null, 2);
    source.dispatchEvent(new InputEvent("input", { bubbles: true }));
    setScenarioEditorStatus("JSON formatted. Changes remain an unsaved draft.");
  });
  tools.querySelector("[data-source-save]").addEventListener("click", () => {
    tools.querySelector(".config-file-menu").open = false;
    document.getElementById("scenario-editor-save").click();
  });
  const input = tools.querySelector("[data-source-file]");
  tools.querySelector("[data-source-open]").addEventListener("click", () => {
    tools.querySelector(".config-file-menu").open = false;
    input.click();
  });
  input.addEventListener("change", async () => {
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    if (file.size > source.maxLength) { setScenarioEditorStatus("JSON file exceeds the 1 MiB editor limit.", "error"); return; }
    if (scenarioEditorDirty && !confirm("Replace the unsaved scenario source with this file?")) return;
    const version = fileVersion;
    try {
      const content = await file.text();
      if (version !== fileVersion) { setScenarioEditorStatus("Source changed while the file was opening. File not imported.", "error"); return; }
      source.value = content;
      source.dispatchEvent(new InputEvent("input", { bubbles: true }));
      setScenarioEditorStatus(`Opened ${file.name} as unsaved JSON. Analyze before saving.`);
    } catch (error) {
      setScenarioEditorStatus(`Could not open ${file.name}: ${error.message}`, "error");
    }
  });
  tools.querySelector("[data-source-download]").addEventListener("click", () => {
    tools.querySelector(".config-file-menu").open = false;
    const blob = new Blob([source.value], { type: "application/json;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${document.getElementById("scenario-editor-id").value.replace(/[^a-zA-Z0-9_-]/g, "-") || "scenario"}.json`;
    link.click();
    URL.revokeObjectURL(url);
  });
  source.addEventListener("input", () => {
    clear();
    setScenarioEditorStatus("Unsaved JSON draft. Analyze before saving.");
  });
  source.addEventListener("scroll", syncScroll);
  source.addEventListener("click", position);
  source.addEventListener("keyup", position);
  source.addEventListener("select", position);
  source.addEventListener("keydown", (event) => {
    if (event.key !== "Tab" || event.shiftKey || event.ctrlKey || event.metaKey || event.altKey) return;
    event.preventDefault();
    if (source.value.length - (source.selectionEnd - source.selectionStart) + 2 > source.maxLength) {
      setScenarioEditorStatus("Indentation exceeds the editor character limit.", "error");
      return;
    }
    source.setRangeText("  ", source.selectionStart, source.selectionEnd, "end");
    source.dispatchEvent(new InputEvent("input", { bubbles: true }));
  });
  advanced.addEventListener("toggle", () => {
    form.classList.toggle("scenario-raw-mode", advanced.open);
    summary.textContent = advanced.open ? "Back to form editor" : "Raw JSON editor";
    render();
  });
  render();
})();
