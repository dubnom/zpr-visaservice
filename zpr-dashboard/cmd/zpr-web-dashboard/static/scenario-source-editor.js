(() => {
  const page = window.ZPREditorPage;
  const source = document.getElementById("scenario-editor-source");
  const advanced = document.getElementById("scenario-editor-advanced");
  const form = document.getElementById("scenario-editor-form");
  const modeToggle = document.getElementById("scenario-editor-mode-toggle");
  const analyze = document.getElementById("scenario-source-analyze");
  const formatButton = document.querySelector("#scenario-editor-dialog [data-source-format]");
  const convert = document.getElementById("scenario-source-convert");
  source.maxLength = 1048576;

  let fileVersion = 0;
  let sourceFormat = "json";
  let lastScenario = null;
  let lastError = null;
  const analysisScope = page.createAnalysisScope(() => [
    source.value, sourceFormat, scenarioEditorOrganization,
    scenarioEditorArtifact?.id, scenarioEditorArtifact?.revision,
  ]);

  function sourceFormatName() {
    return sourceFormat === "json" ? "JSON" : "YAML";
  }

  const surface = page.createSourceSurface({
    source,
    highlight: document.getElementById("scenario-source-highlight"),
    gutter: document.getElementById("scenario-source-gutter"),
    language: () => sourceFormat,
    label: sourceFormatName,
  });
  window.scenarioFileMenu = page.createMenu({
    root: document.getElementById("scenario-editor-actions"),
    toggle: document.getElementById("scenario-editor-files-toggle"),
    menu: document.getElementById("scenario-editor-file-menu"),
  });

  function setAnalysisState(state = "") {
    setScenarioAnalyzeState(state);
    analyze.classList.toggle("button-next-evaluate", !state || state === "pending");
  }

  function syncLabels() {
    const label = sourceFormatName();
    source.setAttribute("aria-label", `Raw scenario ${label}`);
    modeToggle.textContent = advanced.open ? "Back to form editor" : `Raw ${label} editor`;
    modeToggle.setAttribute("aria-pressed", String(advanced.open));
    convert.hidden = !advanced.open;
    convert.textContent = `Convert to ${sourceFormat === "json" ? "YAML" : "JSON"}`;
    formatButton.disabled = !advanced.open;
  }

  function render() {
    syncLabels();
    surface.render();
  }

  function clear() {
    fileVersion++;
    analysisScope.invalidate();
    surface.setDiagnostic(null);
    lastScenario = null;
    setAnalysisState(scenarioEditorDirty ? "pending" : "");
    render();
  }
  window.renderScenarioSourceEditor = clear;

  async function check({ quiet = false } = {}) {
    surface.setDiagnostic(null);
    lastError = null;
    setScenarioEditorStatus("");
    fileVersion++;
    const isCurrent = analysisScope.begin();
    const original = source.value;
    const organization = scenarioEditorOrganization;
    try {
      const response = await fetch(`/api/simulator/organizations/${encodeURIComponent(organization)}/scenario-check`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ format: sourceFormat, source: original, scenario_id: scenarioEditorArtifact?.id || "" }),
      });
      const result = await response.json();
      if (!isCurrent()) return null;
      if (!response.ok || result.valid !== true) {
        surface.setDiagnostic(result.line > 0 ? { line: result.line, message: result.error || "Scenario analysis failed." } : null);
        throw new Error(result.error || `Scenario analysis failed (${response.status}).`);
      }
      lastScenario = result.scenario;
      setAnalysisState("success");
      if (!quiet) setScenarioEditorStatus(result.diagnostics);
      render();
      return result;
    } catch (error) {
      if (!isCurrent()) return null;
      setAnalysisState("error");
      error.sourceDiagnostic = Boolean(surface.diagnostic);
      lastError = error;
      setScenarioEditorStatus(surface.diagnostic ? "" : error.message, "error");
      render();
      return null;
    }
  }

  window.readScenarioSourceEditor = async () => {
    if (sourceFormat === "json") {
      try { return JSON.parse(source.value); }
      catch {
        await check({ quiet: true });
        throw lastError || new Error("Scenario JSON must analyze cleanly before saving.");
      }
    }
    const result = await check({ quiet: true });
    if (!result?.scenario) throw lastError || new Error(`Scenario ${sourceFormatName()} must analyze cleanly before saving.`);
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
    const result = await check({ quiet: true });
    if (!result) {
      return;
    }
    sourceFormat = nextFormat;
    source.value = nextFormat === "json" ? result.canonical_json : result.canonical_yaml;
    source.dispatchEvent(new InputEvent("input", { bubbles: true }));
    lastScenario = result.scenario;
    setAnalysisState("success");
    render();
  }

  window.analyzeScenarioSourceEditor = check;
  convert.addEventListener("click", () => convertFormat(sourceFormat === "json" ? "yaml" : "json"));
  formatButton.addEventListener("click", async () => {
    const result = await check({ quiet: true });
    if (!result) return;
    source.value = sourceFormat === "json" ? result.canonical_json : result.canonical_yaml;
    source.dispatchEvent(new InputEvent("input", { bubbles: true }));
    lastScenario = result.scenario;
  });
  const input = document.querySelector("#scenario-editor-dialog [data-source-file]");
  document.querySelector("#scenario-editor-dialog [data-source-open]").addEventListener("click", () => input.click());
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
      advanced.open = true;
      source.value = content;
      source.dispatchEvent(new InputEvent("input", { bubbles: true }));
      setScenarioEditorStatus(`Opened ${file.name}. Analyze before saving.`);
    } catch (error) {
      setScenarioEditorStatus(`Could not open ${file.name}: ${error.message}`, "error");
    }
  });
  document.querySelector("#scenario-editor-dialog [data-source-download]").addEventListener("click", () => {
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
    setScenarioEditorStatus("");
  });
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
    render();
  });
  render();
})();
