(() => {
  const source = document.getElementById("scenario-editor-source");
  const advanced = document.getElementById("scenario-editor-advanced");
  const form = document.getElementById("scenario-editor-form");
  const assistant = document.getElementById("scenario-assistant-slot");
  const summary = advanced.querySelector("summary");
  const modeToggle = document.getElementById("scenario-editor-mode-toggle");
  summary.hidden = true;
  source.setAttribute("wrap", "off");
  source.maxLength = 1048576;

  const workbench = document.createElement("div");
  workbench.className = "scenario-source-workbench";
  advanced.before(workbench);
  workbench.append(advanced, assistant);

  const tools = document.createElement("div");
  tools.className = "policy-editor-tools scenario-source-tools";
  tools.innerHTML = `<details class="config-file-menu"><summary class="button button-refresh">File...</summary><div class="config-file-popover" role="menu" aria-label="Scenario file commands"><button type="button" role="menuitem" data-source-save>Save draft</button><button type="button" role="menuitem" data-source-open>Open source...</button><button type="button" role="menuitem" data-source-download>Download source...</button></div></details><button class="button button-next-evaluate" type="button" id="scenario-source-analyze">Analyze</button><button class="button button-save-as-ready" type="button" data-source-format>Format</button><label class="scenario-source-format">Format<select id="scenario-source-format" aria-label="Scenario source format"><option value="yaml">YAML</option><option value="json">JSON</option></select></label><strong id="scenario-source-label">Scenario YAML</strong><span id="scenario-source-modified" hidden>Modified</span><span id="scenario-source-position">Line 1 · Column 1</span><input type="file" accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml,text/x-yaml" hidden data-source-file>`;
  const editor = document.createElement("div");
  editor.className = "config-source-editor scenario-source-editor";
  const gutter = document.createElement("div");
  gutter.className = "config-source-gutter";
  gutter.id = "scenario-source-gutter";
  gutter.setAttribute("aria-label", "Scenario source errors by line");
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
  const formatPicker = tools.querySelector("#scenario-source-format");
  let diagnostic = null;
  let fileVersion = 0;
  let sourceFormat = "json";
  let lastScenario = null;

  function sourceFormatName() {
    return sourceFormat === "json" ? "JSON" : "YAML";
  }

  function setAnalysisState(state = "") {
    setScenarioAnalyzeState(state);
    analyze.classList.toggle("button-next-evaluate", !state || state === "pending");
  }

  function syncLabels() {
    const label = sourceFormatName();
    source.setAttribute("aria-label", `Raw scenario ${label}`);
    tools.querySelector("#scenario-source-label").textContent = `Scenario ${label}`;
    modeToggle.textContent = advanced.open ? "Back to form editor" : `Raw ${label} editor`;
    modeToggle.setAttribute("aria-pressed", String(advanced.open));
  }

  function syncScroll() {
    gutter.scrollTop = highlight.scrollTop = source.scrollTop;
    highlight.scrollLeft = source.scrollLeft;
    editor.dataset.horizontalOverflow = String(source.scrollWidth > source.clientWidth);
  }

  function position() {
    const before = source.value.slice(0, source.selectionStart);
    tools.querySelector("#scenario-source-position").textContent = `Line ${before.split("\n").length} · Column ${source.selectionStart - before.lastIndexOf("\n")}`;
  }

  function renderJSON() {
    return /"(?:\\.|[^"\\])*"|\b(?:true|false|null)\b|-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/g;
  }

  function renderYAML() {
    return /#[^\n]*|"(?:\\.|[^"\\\n])*"|'[^'\n]*'|^[ \t]*[-?]?[ \t]*[A-Za-z0-9_.-]+(?=\s*:)|\b(?:true|false|null)\b|-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/gm;
  }

  function render() {
    syncLabels();
    formatPicker.value = sourceFormat;
    highlight.replaceChildren();
    const tokens = sourceFormat === "json" ? renderJSON() : renderYAML();
    let offset = 0;
    for (const match of source.value.matchAll(tokens)) {
      highlight.append(document.createTextNode(source.value.slice(offset, match.index)));
      const token = document.createElement("span");
      const text = match[0];
      const kind = text.trimStart().startsWith("#") ? "comment"
        : sourceFormat === "json" && text.startsWith('"') ? (/^\s*:/.test(source.value.slice(match.index + text.length)) ? "attribute" : "string")
        : /^["']/.test(text) ? "string"
        : /^[ \t]*[-?]?[ \t]*[A-Za-z0-9_.-]+$/.test(text) ? "attribute"
        : /^(true|false|null)$/.test(text) ? "keyword" : "value";
      token.className = `zpl-${kind}`;
      token.textContent = text;
      highlight.append(token);
      offset = match.index + text.length;
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
        marker.setAttribute("aria-label", `${sourceFormatName()} error on line ${index + 1}: ${diagnostic.message}`);
        marker.addEventListener("click", () => {
          const start = source.value.split("\n").slice(0, index).reduce((length, line) => length + line.length + 1, 0);
          source.focus();
          source.setSelectionRange(start, start + source.value.split("\n")[index].length);
          source.scrollTop = index * Number.parseFloat(getComputedStyle(source).lineHeight);
          syncScroll();
          position();
        });
        row.append(marker);
      } else {
        const spacer = document.createElement("span");
        spacer.className = "config-gutter-marker-space";
        spacer.setAttribute("aria-hidden", "true");
        row.append(spacer);
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
    lastScenario = null;
    setAnalysisState(scenarioEditorDirty ? "pending" : "");
    render();
  }
  window.renderScenarioSourceEditor = clear;

  async function check({ quiet = false } = {}) {
    diagnostic = null;
    const version = ++fileVersion;
    const original = source.value;
    const organization = scenarioEditorOrganization;
    try {
      const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organization)}/scenario-check`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ format: sourceFormat, source: original, scenario_id: scenarioEditorArtifact?.id || "" }),
      });
      const result = await response.json();
      if (version !== fileVersion || organization !== scenarioEditorOrganization) return null;
      if (!response.ok || result.valid !== true) {
        diagnostic = result.line > 0 ? { line: result.line, message: result.error || "Scenario analysis failed." } : null;
        throw new Error(result.error || `Scenario analysis failed (${response.status}).`);
      }
      lastScenario = result.scenario;
      setAnalysisState("success");
      if (!quiet) setScenarioEditorStatus(result.diagnostics);
      render();
      return result;
    } catch (error) {
      if (version !== fileVersion || organization !== scenarioEditorOrganization) return null;
      setAnalysisState("error");
      if (!quiet) setScenarioEditorStatus(error.message, "error");
      render();
      return null;
    }
  }

  window.readScenarioSourceEditor = async () => {
    if (sourceFormat === "json") return JSON.parse(source.value);
    const result = await check({ quiet: true });
    if (!result?.scenario) throw new Error(`Scenario ${sourceFormatName()} must analyze cleanly before saving.`);
    return result.scenario;
  };

  window.peekScenarioSourceEditor = () => {
    if (!scenarioEditorJsonDirty) return JSON.parse(JSON.stringify(scenarioEditorDraft));
    if (sourceFormat === "json") return JSON.parse(source.value);
    if (!lastScenario) throw new Error(`Analyze the ${sourceFormatName()} draft before asking Claude.`);
    return lastScenario;
  };

  function yamlScalar(value) {
    if (value === null) return "null";
    if (typeof value === "number" || typeof value === "boolean") return String(value);
    const text = String(value);
    return /^[A-Za-z0-9_.\/:-]+$/.test(text) ? text : JSON.stringify(text);
  }

  function yamlValue(value, indent = 0) {
    const pad = " ".repeat(indent);
    if (Array.isArray(value)) {
      if (!value.length) return "[]";
      return value.map((entry) => {
        if (entry && typeof entry === "object") {
          const rendered = yamlValue(entry, indent + 2).split("\n");
          return `${pad}- ${rendered[0].trimStart()}${rendered.length > 1 ? `\n${rendered.slice(1).join("\n")}` : ""}`;
        }
        return `${pad}- ${yamlScalar(entry)}`;
      }).join("\n");
    }
    if (value && typeof value === "object") {
      return Object.entries(value).filter(([, entry]) => entry !== undefined).map(([key, entry]) => {
        if (entry && typeof entry === "object" && (!Array.isArray(entry) || entry.length)) return `${pad}${key}:\n${yamlValue(entry, indent + 2)}`;
        return `${pad}${key}: ${yamlValue(entry, indent + 2)}`;
      }).join("\n");
    }
    return yamlScalar(value);
  }

  window.setScenarioSourceFromScenario = (scenario) => {
    if (sourceFormat === "json") source.value = JSON.stringify(scenario, null, 2);
    else source.value = `${yamlValue(scenario)}\n`;
    clear();
    lastScenario = scenario;
  };

  async function convertFormat(nextFormat) {
    if (nextFormat === sourceFormat) return;
    const previous = sourceFormat;
    const result = await check({ quiet: true });
    if (!result) {
      formatPicker.value = previous;
      setScenarioEditorStatus(`Analyze the current ${sourceFormatName()} before switching formats.`, "error");
      return;
    }
    sourceFormat = nextFormat;
    source.value = nextFormat === "json" ? result.canonical_json : result.canonical_yaml;
    source.dispatchEvent(new InputEvent("input", { bubbles: true }));
    lastScenario = result.scenario;
    setAnalysisState("success");
    setScenarioEditorStatus(`Converted raw editor to ${sourceFormatName()}. Changes remain an unsaved draft.`);
    render();
  }

  window.analyzeScenarioSourceEditor = check;
  analyze.addEventListener("click", check);
  formatPicker.addEventListener("change", () => convertFormat(formatPicker.value));
  tools.querySelector("[data-source-format]").addEventListener("click", async () => {
    const result = await check();
    if (!result) return;
    source.value = sourceFormat === "json" ? result.canonical_json : result.canonical_yaml;
    source.dispatchEvent(new InputEvent("input", { bubbles: true }));
    lastScenario = result.scenario;
    setScenarioEditorStatus(`${sourceFormatName()} formatted. Changes remain an unsaved draft.`);
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
    if (file.size > source.maxLength) { setScenarioEditorStatus("Scenario source file exceeds the 1 MiB editor limit.", "error"); return; }
    if (scenarioEditorDirty && !confirm("Replace the unsaved scenario source with this file?")) return;
    const version = fileVersion;
    try {
      const content = await file.text();
      if (version !== fileVersion) { setScenarioEditorStatus("Source changed while the file was opening. File not imported.", "error"); return; }
      sourceFormat = /\.(ya?ml)$/i.test(file.name) ? "yaml" : "json";
      source.value = content;
      source.dispatchEvent(new InputEvent("input", { bubbles: true }));
      setScenarioEditorStatus(`Opened ${file.name} as unsaved ${sourceFormatName()}. Analyze before saving.`);
    } catch (error) {
      setScenarioEditorStatus(`Could not open ${file.name}: ${error.message}`, "error");
    }
  });
  tools.querySelector("[data-source-download]").addEventListener("click", () => {
    tools.querySelector(".config-file-menu").open = false;
    const extension = sourceFormat === "json" ? "json" : "yaml";
    const type = sourceFormat === "json" ? "application/json" : "application/yaml";
    const blob = new Blob([source.value], { type: `${type};charset=utf-8` });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${document.getElementById("scenario-editor-id").value.replace(/[^a-zA-Z0-9_-]/g, "-") || "scenario"}.${extension}`;
    link.click();
    URL.revokeObjectURL(url);
  });
  source.addEventListener("input", () => {
    clear();
    setScenarioEditorStatus(`Unsaved ${sourceFormatName()} draft. Analyze before saving.`);
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
    syncLabels();
    render();
  });
  render();
})();
