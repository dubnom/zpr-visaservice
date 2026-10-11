(() => {
  const dialog = document.createElement("dialog");
  dialog.id = "organization-profile-editor";
  dialog.className = "sim-editor-page section-block policy-page editor-page config-editor-panel";
  dialog.setAttribute("aria-labelledby", "organization-profile-title");
  dialog.innerHTML = `<div class="policy-toolbar"><div class="policy-identity"><strong id="organization-profile-title">Organization draft</strong><span id="organization-profile-revision"></span><span id="organization-profile-modified" class="policy-modified-indicator" hidden>Modified</span></div><button class="quiet" type="button" data-profile-close aria-label="Close organization draft">Close</button></div>
    <div class="editor-page-workbench"><div class="editor-page-main">
    <div class="policy-editor-tools"><div class="policy-editor-utilities"><button type="button" data-profile-save>Save draft</button><button type="button" data-profile-validate>Validate profile</button><button type="button" data-profile-reload>Reload draft</button><button type="button" data-profile-definitions hidden>Edit supporting definitions</button><button type="button" data-profile-export hidden>Export saved definitions</button><details class="policy-history-menu"><summary class="button">History</summary><div id="organization-profile-history" class="policy-history"></div></details></div></div>
    <p id="organization-profile-status" role="status" aria-live="polite"></p>
    <p>Draft only. Saving does not change bundled files, linked documents, the active organization or running machines. Profile validation is not runtime readiness.</p>
    <div class="assistant-settings"><label>Name<input id="organization-profile-name"></label><label>Description<input id="organization-profile-description"></label></div>
    <div class="config-source-editor"><div id="organization-profile-gutter" class="config-source-gutter"></div><div class="config-code-surface"><pre id="organization-profile-highlight" aria-hidden="true"></pre><textarea id="organization-profile-source" class="zpr-config-source" aria-label="Organization profile JSON" spellcheck="false" wrap="off"></textarea></div></div>
    </div><div id="organization-profile-assistant"></div></div>`;
  document.body.append(dialog);
  const source = dialog.querySelector("#organization-profile-source");
  const name = dialog.querySelector("#organization-profile-name");
  const description = dialog.querySelector("#organization-profile-description");
  const status = dialog.querySelector("#organization-profile-status");
  const history = dialog.querySelector("#organization-profile-history");
  const surface = window.ZPREditorPage.createSourceSurface({
    source, highlight: dialog.querySelector("#organization-profile-highlight"),
    gutter: dialog.querySelector("#organization-profile-gutter"), language: "json", label: "Organization profile",
  });
  let organizationID = "";
  let revision = 0;
  let saved = "";
  let generation = 0;
  let busy = false;
  let draftBundle = false;
  const dirty = () => source.value !== saved;
  const path = () => `/api/simulator/organizations/${encodeURIComponent(organizationID)}/${draftBundle ? "draft/" : ""}profile`;
  const request = (url, options) => window.ZPRPageRuntime.requestJSON(window.fetch.bind(window), url, options);
  function render() {
    surface.render();
    dialog.querySelector("#organization-profile-revision").textContent = revision ? `Draft revision ${revision}` : "Loading";
    dialog.querySelector("#organization-profile-modified").hidden = !dirty();
    for (const button of dialog.querySelectorAll("[data-profile-save], [data-profile-validate], [data-profile-reload]")) button.disabled = busy || !revision;
    source.disabled = name.disabled = description.disabled = busy || !revision;
    dialog.querySelector("[data-profile-export]").hidden = !draftBundle;
    dialog.querySelector("[data-profile-export]").disabled = busy || !revision || dirty();
    dialog.querySelector("[data-profile-definitions]").hidden = !draftBundle;
    dialog.querySelector("[data-profile-definitions]").disabled = busy || !revision;
    try {
      const profile = JSON.parse(source.value);
      name.value = profile.name ?? "";
      description.value = profile.description ?? "";
    } catch {
      name.disabled = description.disabled = true;
    }
    assistant.render();
  }
  function changed() {
    status.textContent = "Modified draft; validate and review before saving.";
    render();
  }
  dialog.querySelector("[data-profile-definitions]").addEventListener("click", () => {
    if (busy || !draftBundle || !revision || !window.ZPREditorPage.confirmDiscard(dirty(), "Discard unsaved profile edits and open the saved full definition bundle?")) return;
    generation++;
    assistant.reset();
    dialog.close();
    document.dispatchEvent(new CustomEvent("organization-definitions:open", { detail: { id: organizationID } }));
  });
  dialog.querySelector("[data-profile-export]").addEventListener("click", async () => {
    if (busy || !draftBundle || dirty()) return;
    const current = generation;
    busy = true;
    render();
    try {
      const bundle = await request(`/api/simulator/organizations/${encodeURIComponent(organizationID)}/draft/export`, { cache: "no-store" });
      if (current !== generation) return;
      const url = URL.createObjectURL(new Blob([JSON.stringify(bundle, null, 2) + "\n"], { type: "application/json" }));
      const link = document.createElement("a");
      link.href = url;
      link.download = `${organizationID}-draft.json`;
      document.body.append(link);
      link.click();
      link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      status.textContent = "Saved definitions exported without creation receipts or runtime state. Unsaved edits are not exported.";
    } catch (error) { if (current === generation) status.textContent = `Export failed: ${error.message}`; }
    finally { if (current === generation) { busy = false; render(); } }
  });
  source.addEventListener("input", changed);
  for (const [field, input] of [["name", name], ["description", description]]) {
    input.addEventListener("input", () => {
      try {
        const profile = JSON.parse(source.value);
        profile[field] = input.value;
        window.ZPRAssistant.editText(source, JSON.stringify(profile, null, 2), true);
      } catch (error) { status.textContent = error.message; }
    });
  }
  function close() {
    if (!window.ZPREditorPage.confirmDiscard(dirty(), "Discard unsaved organization draft changes?")) return;
    generation++;
    assistant.reset();
    dialog.close();
  }
  dialog.querySelector("[data-profile-close]").addEventListener("click", close);
  dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
  async function load(id, loadedMessage = "Draft loaded. Runtime and bundled configuration are unchanged.", bundle = false) {
    const current = ++generation;
    organizationID = id;
    draftBundle = bundle;
    revision = 0;
    saved = source.value = "";
    history.replaceChildren();
    busy = true;
    assistant.reset();
    status.textContent = "Loading organization draft...";
    dialog.querySelector("#organization-profile-title").textContent = `Organization draft: ${id}`;
    if (!dialog.open) dialog.showModal();
    render();
    try {
      const artifact = await request(path());
      if (current !== generation) return;
      revision = artifact.revision;
      saved = source.value = JSON.stringify(artifact.content, null, 2);
      status.textContent = loadedMessage;
      const revisions = await request(path() + "/revisions");
      if (current !== generation) return;
      for (const entry of revisions) {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = `Revision ${entry.revision}: ${entry.summary}`;
        button.addEventListener("click", async () => {
          if (busy || !window.ZPREditorPage.confirmDiscard(dirty(), "Replace unsaved changes with this historical profile?")) return;
          if (draftBundle) {
            generation++;
            assistant.reset();
            dialog.close();
            document.dispatchEvent(new CustomEvent("organization-definitions:open", { detail: { id: organizationID, revision: entry.revision } }));
            return;
          }
          const selected = ++generation;
          busy = true;
          render();
          assistant.reset();
          try {
            const record = await request(path() + `/revisions/${entry.revision}`);
            if (selected !== generation) return;
            source.value = JSON.stringify(record.content, null, 2);
            status.textContent = `Historical revision ${entry.revision} loaded for review. Save draft creates a new revision; runtime is unchanged.`;
          } catch (error) { if (selected === generation) status.textContent = error.message; }
          finally { if (selected === generation) { busy = false; render(); } }
        });
        history.append(button);
      }
    } catch (error) { if (current === generation) status.textContent = error.message; }
    finally { if (current === generation) { busy = false; render(); } }
  }
  dialog.querySelector("[data-profile-reload]").addEventListener("click", () => {
    if (window.ZPREditorPage.confirmDiscard(dirty(), "Reload the saved draft and discard unsaved changes?")) void load(organizationID, undefined, draftBundle);
  });
  for (const [selector, save] of [["[data-profile-save]", true], ["[data-profile-validate]", false]]) {
    dialog.querySelector(selector).addEventListener("click", async () => {
      const current = ++generation;
      busy = true;
      assistant.reset();
      render();
      try {
        const profile = JSON.parse(source.value);
        const result = await request(path() + (save ? "" : "/check"), {
          method: save ? "PUT" : "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ profile, expected_revision: revision, summary: "Organization profile draft edit" }),
        });
        if (current !== generation) return;
        if (save) {
          saved = source.value;
          busy = false;
          await load(organizationID, "Draft saved as a new revision. Runtime and bundled configuration are unchanged.", draftBundle);
          document.dispatchEvent(new Event("organization-drafts:changed"));
        } else status.textContent = result.notice;
      } catch (error) {
        if (current === generation) status.textContent = `${error.message}${save ? " Save was not confirmed; reload the draft to reconcile before retrying." : ""}`;
      } finally { if (current === generation) { busy = false; render(); } }
    });
  }
  const review = document.createElement("dialog");
  review.setAttribute("aria-label", "Review AI organization changes");
  review.className = "design-assistant-dialog";
  review.innerHTML = `<h2>Review AI organization changes</h2><p>Only the profile draft will change. Nothing is saved or activated.</p><pre></pre><button type="button" data-review-accept>Insert reviewed draft</button><button type="button" data-review-cancel>Cancel</button>`;
  document.body.append(review);
  const assistant = window.mountSimulatorDesignAssistant("organization-profile-assistant", {
    scope: "organization-profile", editorID: dialog.id, applyLabel: "Review profile proposal",
    editable: () => dialog.open && Boolean(revision) && !busy,
    disclosure: "Send sends only this fictional organization profile draft and conversation to Anthropic. Linked files, credentials and live runtime state are not included. Proposals require review and never save or activate.",
    getContext: () => ({ organization_id: organizationID, profile_source: source.value, revision, draft_bundle: draftBundle }),
    onApply: async (proposal, isCurrent) => {
      if (!proposal.profile) throw new Error("No organization profile proposal was returned.");
      const before = source.value;
      const after = JSON.stringify(proposal.profile, null, 2);
      const original = JSON.parse(before);
      const fields = [...new Set([...Object.keys(original), ...Object.keys(proposal.profile)])].sort();
      review.querySelector("pre").textContent = fields.filter(key => JSON.stringify(original[key]) !== JSON.stringify(proposal.profile[key]))
        .map(key => `${key}\nBefore: ${JSON.stringify(original[key], null, 2)}\nAfter: ${JSON.stringify(proposal.profile[key], null, 2)}`).join("\n\n") || "No profile changes.";
      const accepted = await new Promise(resolve => {
        review.querySelector("[data-review-accept]").onclick = () => { review.close(); resolve(true); };
        review.querySelector("[data-review-cancel]").onclick = () => { review.close(); resolve(false); };
        review.oncancel = event => { event.preventDefault(); review.close(); resolve(false); };
        review.showModal();
      });
      if (!accepted) throw new Error("Proposal review cancelled; draft unchanged.");
      if (!isCurrent() || source.value !== before || !dialog.open || busy) throw new Error("Organization draft changed; ask again before applying.");
      const id = organizationID;
      const baseRevision = revision;
      const restore = (expected, value) => {
        if (organizationID !== id || revision !== baseRevision || source.value !== expected || busy || !dialog.open) throw new Error("Draft changed; AI undo/redo is no longer available.");
        window.ZPRAssistant.editText(source, value, true);
      };
      window.ZPRAssistant.editText(source, after, true);
      return { undo: () => restore(after, before), redo: () => restore(before, after) };
    },
  });
  document.addEventListener("click", event => {
    if (!event.target.closest("#organization-edit-profile")) return;
    if (selectedOrganizationID) void load(selectedOrganizationID);
  });
  document.addEventListener("organization-profile:open", event => {
    if (dialog.open && !window.ZPREditorPage.confirmDiscard(dirty(), "Discard unsaved organization draft changes?")) return;
    void load(event.detail.id, "Private draft bundle loaded. Definitions are not runnable; no runtime or linked source files changed.", true);
  });
  window.addEventListener("operator-session-cleared", () => {
    generation++;
    organizationID = "";
    saved = source.value = "";
    name.value = description.value = "";
    revision = 0;
    busy = false;
    history.replaceChildren();
    assistant.reset();
    if (review.open) review.dispatchEvent(new Event("cancel", { cancelable: true }));
    if (dialog.open) dialog.close();
    render();
  });
  const button = document.createElement("button");
  button.id = "organization-edit-profile";
  button.className = "quiet";
  button.type = "button";
  button.textContent = "Edit organization draft";
  document.querySelector(".organization-top-actions").prepend(button);
})();
