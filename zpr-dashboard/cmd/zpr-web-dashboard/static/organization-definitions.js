(() => {
  const dialog = document.createElement("dialog");
  dialog.id = "organization-definitions-editor";
  dialog.className = "sim-editor-page section-block policy-page editor-page config-editor-panel";
  dialog.setAttribute("aria-labelledby", "organization-definitions-title");
  dialog.innerHTML = `<div class="policy-toolbar"><div class="policy-identity"><strong id="organization-definitions-title">Organization definitions</strong><span id="organization-definitions-revision"></span><span data-definitions-modified hidden>Modified</span></div><button type="button" class="quiet" data-definitions-close>Close definitions</button></div>
    <div class="editor-page-workbench"><div class="editor-page-main">
    <div class="policy-editor-tools"><div class="policy-editor-utilities"><button type="button" data-definitions-save>Save all definitions</button><button type="button" data-definitions-check>Validate all definitions</button><button type="button" data-definitions-reload>Reload definitions</button><details class="policy-history-menu"><summary class="button">History</summary><div id="organization-definitions-history" class="policy-history"></div></details></div></div>
    <p id="organization-definitions-status" role="status" aria-live="polite"></p>
    <p>Private draft only. Profile and supporting documents save together as one immutable revision. This never writes bundled files, publishes, activates or changes machines. Validation is not compiler or runtime certification.</p>
    <div class="assistant-settings"><label>Definition file<select id="organization-definition-selector" aria-label="Definition file"></select></label><button type="button" data-definition-add>Add document</button><button type="button" data-definition-remove>Remove document</button></div>
    <div class="assistant-settings" data-definition-fields hidden><label>Document path<input id="organization-definition-path" maxlength="256"></label><label>Document kind<select id="organization-definition-kind" aria-label="Document kind"><option value="directory">Directory (LDIF)</option><option value="policy">Policy (ZPL)</option><option value="policy-config">Compiler configuration (ZPLC)</option><option value="policy-catalog">Policy catalog (JSON)</option><option value="scenario">Scenario (JSON)</option></select></label></div>
    <div class="config-source-editor"><div id="organization-definitions-gutter" class="config-source-gutter"></div><div class="config-code-surface"><pre id="organization-definitions-highlight" aria-hidden="true"></pre><textarea id="organization-definitions-source" class="zpr-config-source" aria-label="Selected organization definition source" spellcheck="false" wrap="off"></textarea></div></div>
    </div><div id="organization-definitions-assistant"></div></div>`;
  document.body.append(dialog);
  const source = dialog.querySelector("#organization-definitions-source");
  const selector = dialog.querySelector("#organization-definition-selector");
  const documentPath = dialog.querySelector("#organization-definition-path");
  const kind = dialog.querySelector("#organization-definition-kind");
  const status = dialog.querySelector("#organization-definitions-status");
  const history = dialog.querySelector("#organization-definitions-history");
  let id = "";
  let revision = 0;
  let profile = "";
  let documents = [];
  let selected = -1;
  let saved = "";
  let busy = false;
  let generation = 0;
  let surface;
  const endpoint = () => `/api/simulator/organizations/${encodeURIComponent(id)}/draft/definitions`;
  const request = (url, options) => window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch ?? window.fetch.bind(window), url, options);
  const snapshot = () => JSON.stringify({ profile, documents });
  const dirty = () => snapshot() !== saved;
  const portable = () => ({ version: 1, profile: JSON.parse(profile), documents });
  function render() {
    dialog.querySelector("#organization-definitions-revision").textContent = revision ? `Draft revision ${revision}` : "Loading";
    dialog.querySelector("[data-definitions-modified]").hidden = !dirty();
    for (const button of dialog.querySelectorAll("[data-definitions-save], [data-definitions-check], [data-definitions-reload], [data-definition-add]")) button.disabled = busy || !revision;
    dialog.querySelector("[data-definition-remove]").disabled = busy || !revision || selected < 0;
    dialog.querySelector("[data-definition-fields]").hidden = selected < 0;
    source.disabled = selector.disabled = documentPath.disabled = kind.disabled = busy || !revision;
    surface?.render();
    assistant.render();
  }
  function updateChoices() {
    selector.replaceChildren();
    for (const [index, label] of [[-1, "Organization profile (JSON)"], ...documents.map((document, index) => [index, `${document.path || "(unnamed)"} (${document.kind})`])]) {
      const option = document.createElement("option");
      option.value = String(index);
      option.textContent = label;
      selector.append(option);
    }
    selector.value = String(selected);
  }
  function select(index) {
    selected = index;
    updateChoices();
    const document = documents[selected];
    source.value = document ? document.content : profile;
    documentPath.value = document?.path || "";
    kind.value = document?.kind || "directory";
    surface?.dispose();
    surface = window.ZPREditorPage.createSourceSurface({
      source, highlight: dialog.querySelector("#organization-definitions-highlight"), gutter: dialog.querySelector("#organization-definitions-gutter"),
      language: selected < 0 || ["scenario", "policy-catalog"].includes(document?.kind) ? "json" : document?.kind === "directory" ? "ldif" : document?.kind === "policy-config" ? "toml" : "zpl",
      label: document?.path || "Organization profile",
    });
    render();
  }
  function modified() {
    status.textContent = "Modified definitions. Validate and review the whole bundle before saving.";
    render();
  }
  source.addEventListener("input", () => {
    if (selected < 0) profile = source.value;
    else documents[selected].content = source.value;
    modified();
  });
  selector.addEventListener("change", () => select(Number(selector.value)));
  documentPath.addEventListener("input", () => {
    documents[selected].path = documentPath.value;
    updateChoices();
    modified();
  });
  kind.addEventListener("change", () => {
    documents[selected].kind = kind.value;
    select(selected);
    modified();
  });
  dialog.querySelector("[data-definition-add]").addEventListener("click", () => {
    if (busy || !revision) return;
    documents.push({ path: "", kind: "directory", content: "" });
    select(documents.length - 1);
    modified();
    documentPath.focus();
  });
  dialog.querySelector("[data-definition-remove]").addEventListener("click", () => {
    if (busy || selected < 0 || !window.confirm(`Remove ${documents[selected].path || "this document"} from this draft? Update profile references before saving.`)) return;
    documents.splice(selected, 1);
    select(-1);
    modified();
  });
  function replace(bundle) {
    if (bundle.version !== 1 || !bundle.profile || !Array.isArray(bundle.documents)) throw new Error("Organization definitions response is invalid.");
    profile = JSON.stringify(bundle.profile, null, 2);
    documents = structuredClone(bundle.documents);
    select(-1);
  }
  async function load(organization, message = "Private definitions loaded. Nothing is activated or published.", historicalRevision = null) {
    const current = ++generation;
    id = organization;
    revision = 0;
    profile = saved = "";
    documents = [];
    history.replaceChildren();
    busy = true;
    assistant.reset();
    dialog.querySelector("#organization-definitions-title").textContent = `Organization definitions: ${id}`;
    status.textContent = "Loading organization definitions...";
    if (!dialog.open) dialog.showModal();
    select(-1);
    try {
      const artifact = await request(endpoint(), { cache: "no-store" });
      if (current !== generation) return;
      replace(artifact.content);
      revision = artifact.revision;
      saved = snapshot();
      status.textContent = message;
      const entries = await request(endpoint() + "/revisions", { cache: "no-store" });
      if (current !== generation) return;
      for (const entry of entries) {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = `Revision ${entry.revision}: ${entry.summary}`;
        button.addEventListener("click", async () => {
          if (busy || !window.ZPREditorPage.confirmDiscard(dirty(), "Replace all unsaved definitions with this historical bundle?")) return;
          const current = ++generation;
          busy = true;
          assistant.reset();
          render();
          try {
            const record = await request(endpoint() + `/revisions/${entry.revision}`, { cache: "no-store" });
            if (current !== generation) return;
            replace(record.content);
            status.textContent = `Historical bundle revision ${entry.revision} loaded for review. Save all definitions appends a new revision; current revision remains ${revision}.`;
          } catch (error) { if (current === generation) status.textContent = error.message; }
          finally { if (current === generation) { busy = false; render(); } }
        });
        history.append(button);
      }
      if (historicalRevision !== null) {
        const record = await request(endpoint() + `/revisions/${historicalRevision}`, { cache: "no-store" });
        if (current !== generation) return;
        replace(record.content);
        status.textContent = `Historical bundle revision ${historicalRevision} loaded for review. Profile and documents are from the same snapshot; Save all definitions appends to current revision ${revision}.`;
      }
    } catch (error) { if (current === generation) status.textContent = `Definitions unavailable: ${error.message}`; }
    finally { if (current === generation) { busy = false; render(); } }
  }
  for (const [action, save] of [["save", true], ["check", false]]) {
    dialog.querySelector(`[data-definitions-${action}]`).addEventListener("click", async () => {
      if (busy || !revision) return;
      const current = ++generation;
      busy = true;
      assistant.reset();
      render();
      try {
        const result = await request(endpoint() + (save ? "" : "/check"), {
          method: save ? "PUT" : "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ bundle: portable(), expected_revision: revision, summary: "Coherent organization definition edit" }),
        });
        if (current !== generation) return;
        if (save) {
          replace(result.content);
          revision = result.revision;
          saved = snapshot();
          busy = false;
          document.dispatchEvent(new Event("organization-drafts:changed"));
          await load(id, "All definitions saved together. Runtime and bundled files are unchanged.");
        } else status.textContent = result.notice;
      } catch (error) {
        if (current === generation) status.textContent = `${error.message}${save ? " Save not confirmed; reload to reconcile before retrying." : ""}`;
      } finally { if (current === generation) { busy = false; render(); } }
    });
  }
  dialog.querySelector("[data-definitions-reload]").addEventListener("click", () => {
    if (!busy && window.ZPREditorPage.confirmDiscard(dirty(), "Reload saved definitions and discard all unsaved changes?")) void load(id);
  });
  const review = document.createElement("dialog");
  review.className = "design-assistant-dialog";
  review.setAttribute("aria-label", "Review AI definition changes");
  review.innerHTML = `<h2>Review AI definition changes</h2><p>Review every changed profile field and document below. Insertion changes only this local draft; nothing is saved or activated.</p><pre></pre><button type="button" data-accept>Insert reviewed definitions</button><button type="button" data-cancel>Cancel</button>`;
  document.body.append(review);
  const assistant = window.mountSimulatorDesignAssistant("organization-definitions-assistant", {
    scope: "organization-definitions", editorID: dialog.id, applyLabel: "Review definition proposal",
    editable: () => dialog.open && Boolean(revision) && !busy,
    disclosure: "Send sends this entire unsaved fictional profile, every supporting document and conversation to Anthropic. Review for credentials first. No creation receipts or live runtime data are sent. Proposals require review and never save, publish or activate.",
    getContext: () => ({ organization_id: id, profile_source: profile, documents_source: JSON.stringify(documents), revision }),
    onApply: async (proposal, isCurrent) => {
      if (!proposal.bundle || proposal.bundle.version !== 1 || !Array.isArray(proposal.bundle.documents)) throw new Error("No valid definition proposal returned.");
      const before = snapshot();
      const previous = portable();
      const changes = [];
      for (const key of [...new Set([...Object.keys(previous.profile), ...Object.keys(proposal.bundle.profile)])].sort()) {
        if (JSON.stringify(previous.profile[key]) !== JSON.stringify(proposal.bundle.profile[key])) changes.push(`Profile: ${key}\nBefore:\n${JSON.stringify(previous.profile[key], null, 2)}\nAfter:\n${JSON.stringify(proposal.bundle.profile[key], null, 2)}`);
      }
      const originals = new Map(previous.documents.map(document => [document.path, document]));
      const replacements = new Map(proposal.bundle.documents.map(document => [document.path, document]));
      for (const path of new Set([...originals.keys(), ...replacements.keys()])) {
        const original = originals.get(path);
        const replacement = replacements.get(path);
        if (JSON.stringify(original) !== JSON.stringify(replacement)) changes.push(`Document: ${path}\nBefore (${original?.kind || "absent"}):\n${original?.content ?? "(absent)"}\nAfter (${replacement?.kind || "removed"}):\n${replacement?.content ?? "(removed)"}`);
      }
      review.querySelector("pre").textContent = changes.join("\n\n") || "No definition changes.";
      const accepted = await new Promise(resolve => {
        review.querySelector("[data-accept]").onclick = () => { review.close(); resolve(true); };
        review.querySelector("[data-cancel]").onclick = () => { review.close(); resolve(false); };
        review.oncancel = event => { event.preventDefault(); review.close(); resolve(false); };
        review.showModal();
      });
      if (!accepted) throw new Error("Definition review cancelled; draft unchanged.");
      if (!isCurrent() || snapshot() !== before || busy || !dialog.open) throw new Error("Definitions changed; ask again before inserting.");
      const organization = id;
      const baseRevision = revision;
      replace(proposal.bundle);
      modified();
      const after = snapshot();
      const restore = (expected, value) => {
        if (id !== organization || revision !== baseRevision || snapshot() !== expected || busy || !dialog.open) throw new Error("Definitions changed; AI undo/redo is no longer available.");
        const state = JSON.parse(value);
        profile = state.profile;
        documents = state.documents;
        select(-1);
        modified();
      };
      return { undo: () => restore(after, before), redo: () => restore(before, after) };
    },
  });
  function close() {
    if (busy) { status.textContent = "Wait for the definitions operation to finish before closing."; return; }
    if (!window.ZPREditorPage.confirmDiscard(dirty(), "Discard all unsaved organization definition changes?")) return;
    generation++;
    assistant.reset();
    dialog.close();
  }
  dialog.querySelector("[data-definitions-close]").addEventListener("click", close);
  dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
  document.addEventListener("organization-definitions:open", event => {
    if (busy) { status.textContent = "Wait for the definitions operation to finish before switching."; return; }
    if (dialog.open && !window.ZPREditorPage.confirmDiscard(dirty(), "Discard all unsaved organization definition changes?")) return;
    void load(event.detail.id, undefined, event.detail.revision ?? null);
  });
  window.addEventListener("operator-session-cleared", () => {
    generation++;
    id = profile = saved = source.value = documentPath.value = "";
    documents = [];
    revision = 0;
    busy = false;
    history.replaceChildren();
    assistant.reset();
    if (review.open) review.dispatchEvent(new Event("cancel", { cancelable: true }));
    if (dialog.open) dialog.close();
    select(-1);
  });
})();
