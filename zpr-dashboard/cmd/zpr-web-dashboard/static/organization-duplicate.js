(() => {
  const request = (path, options) => window.ZPRPageRuntime.requestJSON(window.zprOperatorFetch ?? window.fetch.bind(window), path, options);
  const section = document.createElement("section");
  section.className = "organization-section";
  section.setAttribute("aria-label", "Private organization drafts");
  section.innerHTML = `<h3>Private drafts · not runnable</h3><p id="organization-drafts-status" role="status"></p><div id="organization-drafts-list" class="organization-list"></div><button type="button" class="quiet" id="organization-drafts-refresh">Refresh drafts</button>`;
  document.querySelector("#organization-list").after(section);
  const list = section.querySelector("#organization-drafts-list");
  const catalogStatus = section.querySelector("#organization-drafts-status");
  let catalogGeneration = 0;
  let selectedDraftID = "";
  async function refreshDrafts() {
    const current = ++catalogGeneration;
    try {
      const result = await request("/api/simulator/organization-drafts", { cache: "no-store" });
      if (current !== catalogGeneration) return;
      if (!Array.isArray(result.drafts)) throw new Error("Organization draft catalog is invalid.");
      if (!result.drafts.some(draft => draft.id === selectedDraftID)) selectedDraftID = "";
      list.replaceChildren();
      for (const draft of result.drafts) {
        const button = document.createElement("button");
        button.type = "button";
        const selected = draft.id === selectedOrganizationID;
        button.className = `organization-card${selected ? " selected" : ""}`;
        button.dataset.organizationDraftId = draft.id;
        button.setAttribute("aria-pressed", String(selected));
        const name = document.createElement("strong");
        name.textContent = draft.name;
        const detail = document.createElement("small");
        detail.textContent = `${draft.id} · draft revision ${draft.revision}${draft.source_id ? ` · duplicated from ${draft.source_id}` : " · private definitions"}`;
        button.append(name, detail);
        button.addEventListener("click", () => {
          selectedDraftID = draft.id;
          document.dispatchEvent(new CustomEvent("organization-draft:selected", { detail: draft }));
        });
        list.append(button);
      }
      catalogStatus.textContent = result.drafts.length ? "Open a draft to edit, use AI, validate or review history. Activation is not available." : "No private organization drafts.";
    } catch (error) {
      if (current === catalogGeneration) catalogStatus.textContent = `Draft catalog unavailable: ${error.message}`;
    }
  }
  section.querySelector("#organization-drafts-refresh").addEventListener("click", () => {
    void refreshDrafts();
    if (selectedDraftID) document.dispatchEvent(new CustomEvent("organization-draft:refresh-selected", { detail: { id: selectedDraftID } }));
  });
  document.addEventListener("organization-drafts:changed", () => void refreshDrafts());
  document.addEventListener("organization-drafts:deleted", event => {
    if (selectedDraftID === event.detail.id) selectedDraftID = "";
  });
  document.addEventListener("organization-draft:cleared", () => { selectedDraftID = ""; });
  const dialog = document.createElement("dialog");
  dialog.id = "organization-duplicate-dialog";
  dialog.className = "design-assistant-dialog organization-duplicate-dialog";
  dialog.setAttribute("aria-labelledby", "organization-duplicate-title");
  dialog.innerHTML = `<header class="design-assistant-dialog-head"><h2 id="organization-duplicate-title">Duplicate organization definitions</h2><button type="button" class="quiet" data-duplicate-close aria-label="Close organization dialog" title="Close organization dialog">×</button></header>
    <p id="organization-create-disclosure">Copies bundled fictional definitions only, not unsaved workspace edits. No credentials, enrollment, sessions, run history or live state. Source base DN and desired node/service identifiers and addresses are retained for review. This creates a private draft, never an active organization.</p>
    <fieldset class="organization-create-modes"><legend>Creation type</legend><button type="button" data-create-mode="new">New</button><button type="button" data-create-mode="import">Import</button><button type="button" data-create-mode="duplicate">Duplicate</button></fieldset>
    <div class="assistant-settings"><label data-create-source>Source organization<input id="organization-duplicate-source" readonly></label><label>New organization ID<input id="organization-duplicate-id" maxlength="100"></label><label data-create-overview>Name<input id="organization-duplicate-name" maxlength="200"></label><label data-create-overview>Description<textarea id="organization-duplicate-description" maxlength="4000"></textarea></label><label data-create-new hidden>Directory base DN (optional)<input id="organization-create-base-dn" maxlength="512"></label></div>
    <section data-create-import hidden><label>Import bundle file<input type="file" id="organization-create-file" accept=".json,application/json"></label><label>Portable organization bundle JSON<textarea id="organization-create-import" rows="10" spellcheck="false"></textarea></label><p>Version 1 definition bundle only. Set profile.id and each scenario organization_id to the target ID before review. Unknown fields, credentials, unsafe paths and runtime state are rejected; namespaces are not rewritten.</p></section>
    <button type="button" data-duplicate-preview>Review proposed draft</button>
    <p id="organization-duplicate-status" role="status" aria-live="polite"></p>
    <pre id="organization-duplicate-review" tabindex="0"></pre>
    <label><input id="organization-duplicate-confirm" type="checkbox" disabled>I reviewed these definitions and want to create a draft only.</label>
    <div class="assistant-actions"><button type="button" data-duplicate-create disabled>Create draft</button><button type="button" data-duplicate-reconcile hidden>Check creation result</button></div>`;
  document.getElementById("organization-workspace").append(dialog);
  const source = dialog.querySelector("#organization-duplicate-source");
  const id = dialog.querySelector("#organization-duplicate-id");
  const name = dialog.querySelector("#organization-duplicate-name");
  const description = dialog.querySelector("#organization-duplicate-description");
  const baseDN = dialog.querySelector("#organization-create-base-dn");
  const importSource = dialog.querySelector("#organization-create-import");
  const importFile = dialog.querySelector("#organization-create-file");
  const confirm = dialog.querySelector("#organization-duplicate-confirm");
  const review = dialog.querySelector("#organization-duplicate-review");
  const status = dialog.querySelector("#organization-duplicate-status");
  const previewButton = dialog.querySelector("[data-duplicate-preview]");
  const createButton = dialog.querySelector("[data-duplicate-create]");
  const reconcileButton = dialog.querySelector("[data-duplicate-reconcile]");
  const storageKey = "zpr-organization-duplicate-pending";
  let preview = null;
  let pending = null;
  let busy = false;
  let generation = 0;
  let mode = "duplicate";
  function render() {
    for (const input of [id, name, description, baseDN, importSource, importFile]) input.disabled = busy || Boolean(pending);
    dialog.querySelector("[data-create-source]").hidden = mode !== "duplicate";
    for (const label of dialog.querySelectorAll("[data-create-overview]")) label.hidden = mode === "import";
    dialog.querySelector("[data-create-new]").hidden = mode !== "new";
    dialog.querySelector("[data-create-import]").hidden = mode !== "import";
    for (const button of dialog.querySelectorAll("[data-create-mode]")) {
      button.setAttribute("aria-pressed", String(button.dataset.createMode === mode));
      button.disabled = busy || Boolean(pending);
    }
    dialog.querySelector("#organization-duplicate-title").textContent = mode === "duplicate" ? "Duplicate organization definitions" : mode === "new" ? "New organization draft" : "Import organization definitions";
    dialog.querySelector("#organization-create-disclosure").textContent = mode === "duplicate"
      ? "Copies bundled fictional definitions only, not workspace edits. No credentials, enrollment, sessions, run history or live state. Source base DN and desired node/service identifiers and addresses are retained for review. Draft only, never activated."
      : "Creates private definitions only. Incomplete sections are shown as readiness gaps, not invented configuration. No credentials, live state, publishing or activation.";
    previewButton.disabled = busy || Boolean(pending);
    createButton.disabled = busy || !preview || !confirm.checked;
    confirm.disabled = busy || !preview || Boolean(pending);
    reconcileButton.hidden = !pending;
    reconcileButton.disabled = busy;
  }
  function invalidate() {
    generation++;
    preview = null;
    confirm.checked = false;
    review.textContent = "";
    status.textContent = "Review the changed draft before creating.";
    render();
  }
  for (const input of [id, name, description, baseDN, importSource]) input.addEventListener("input", invalidate);
  importFile.addEventListener("change", async () => {
    invalidate();
    const current = generation;
    try {
      const file = importFile.files[0];
      if (!file) return;
      if (file.size > 8 * 1024 * 1024) throw new Error("Import file exceeds 8 MiB.");
      const text = await file.text();
      if (current !== generation) return;
      importSource.value = text;
      status.textContent = "Import loaded locally. Review validates definitions before draft creation. No AI context is sent by importing.";
    } catch (error) { if (current === generation) status.textContent = error.message; }
  });
  confirm.addEventListener("change", render);
  const duplicatePath = () => mode === "duplicate"
    ? `/api/simulator/organizations/${encodeURIComponent(source.value)}/duplicates/${encodeURIComponent(id.value)}`
    : `/api/simulator/organizations/${encodeURIComponent(id.value)}/create`;
  const body = () => mode === "duplicate" ? { name: name.value, description: description.value }
    : mode === "new" ? { mode, name: name.value, description: description.value, base_dn: baseDN.value }
    : { mode, bundle: JSON.parse(importSource.value) };
  function complete(target) {
    sessionStorage.removeItem(storageKey);
    pending = null;
    dialog.close();
    document.dispatchEvent(new CustomEvent("organization-profile:open", { detail: { id: target } }));
    void refreshDrafts();
  }
  previewButton.addEventListener("click", async () => {
    const current = ++generation;
    busy = true;
    preview = null;
    confirm.checked = false;
    render();
    try {
      const result = await request(duplicatePath() + "/check", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body()),
      });
      if (current !== generation) return;
      const hash = result.source_hash || result.reviewed_hash;
      if (!hash || !result.profile || !Array.isArray(result.documents)) throw new Error("Organization draft preview is invalid.");
      preview = { request: body(), hash };
      review.textContent = `${result.notice}\n\nProfile:\n${JSON.stringify(result.profile, null, 2)}\n\nSupporting documents:\n` +
        result.documents.map(document => `${document.path} (${document.kind})\n${document.content}`).join("\n\n");
      status.textContent = "Review the complete profile and supporting documents, then confirm draft creation.";
    } catch (error) { if (current === generation) status.textContent = error.message; }
    finally { if (current === generation) { busy = false; render(); } }
  });
  createButton.addEventListener("click", async () => {
    if (busy || !preview || !confirm.checked && !pending) return;
    const current = ++generation;
    busy = true;
    try {
      if (!pending) {
        const operation = { source: source.value, target: id.value, mode,
          request: { ...preview.request, operation_id: crypto.randomUUID(),
            [mode === "duplicate" ? "expected_source_hash" : "reviewed_hash"]: preview.hash, confirmed: true } };
        sessionStorage.setItem(storageKey, JSON.stringify(operation));
        pending = operation;
      }
      render();
      const result = await request(duplicatePath(), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(pending.request) });
      if (current !== generation) return;
      if (result.content?.operation_id !== pending.request.operation_id || result.organization_id !== pending.target) throw new Error("Creation receipt does not match this operation.");
      complete(pending.target);
    } catch (error) {
      if (current !== generation) return;
      if (error.details?.review_again === true) {
        sessionStorage.removeItem(storageKey);
        pending = null;
        preview = null;
        confirm.checked = false;
        review.textContent = "";
        status.textContent = `${error.message}. This operation did not create a draft; review again before submitting.`;
      } else status.textContent = `${error.message} Creation is not confirmed. Check creation result or retry the same recorded operation; do not submit a changed request.`;
    } finally { if (current === generation) { busy = false; render(); } }
  });
  reconcileButton.addEventListener("click", async () => {
    if (!pending || busy) return;
    const current = ++generation;
    busy = true;
    render();
    try {
      const result = await request(`/api/simulator/organizations/${encodeURIComponent(pending.target)}/draft`, { cache: "no-store" });
      if (current !== generation) return;
      if (result.content?.operation_id !== pending.request.operation_id) throw new Error("Organization exists but belongs to a different creation operation.");
      complete(pending.target);
    } catch (error) { if (current === generation) status.textContent = `Creation remains unresolved: ${error.message}. Retry uses the same operation and cannot create a second draft.`; }
    finally { if (current === generation) { busy = false; render(); } }
  });
  function close() {
    if (busy) { status.textContent = "Wait for this draft operation to finish before closing."; return; }
    generation++;
    dialog.close();
  }
  dialog.querySelector("[data-duplicate-close]").addEventListener("click", close);
  dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
  const duplicateButton = document.createElement("button");
  duplicateButton.type = "button";
  duplicateButton.className = "button quiet";
  duplicateButton.setAttribute("role", "menuitem");
  duplicateButton.textContent = "New organization";
  function openCreation(nextMode) {
    try {
      const retained = sessionStorage.getItem(storageKey);
      pending = retained ? JSON.parse(retained) : null;
      if (pending) {
        mode = pending.mode || "duplicate";
        source.value = pending.source;
        id.value = pending.target;
        name.value = pending.request.name;
        description.value = pending.request.description;
        baseDN.value = pending.request.base_dn || "";
        importSource.value = pending.request.bundle ? JSON.stringify(pending.request.bundle, null, 2) : "";
        preview = { request: pending.request, hash: pending.request.expected_source_hash || pending.request.reviewed_hash };
        confirm.checked = true;
        review.textContent = "An unresolved creation operation is retained. Check its result or retry the exact recorded request.";
        status.textContent = review.textContent;
      } else {
        const selected = nextMode === "duplicate" ? organizationCatalog.find(organization => organization.id === selectedOrganizationID) : null;
        if (nextMode === "duplicate" && !selected) throw new Error("Select a bundled source organization before duplicating.");
        mode = nextMode;
        source.value = selected?.id || "";
        id.value = "";
        name.value = selected ? `${selected.name} copy` : "";
        description.value = selected?.description || "";
        baseDN.value = importSource.value = importFile.value = "";
        invalidate();
      }
      render();
      if (!dialog.open) dialog.showModal();
    } catch (error) {
      const message = `Cannot open organization creation: ${error.message}`;
      if (dialog.open) status.textContent = message;
      else {
        const error = document.getElementById("organization-error");
        error.textContent = message;
        error.hidden = false;
      }
    }
  }
  duplicateButton.addEventListener("click", () => openCreation("new"));
  document.getElementById("organization-file-menu").prepend(duplicateButton);
  for (const button of dialog.querySelectorAll("[data-create-mode]")) {
    button.addEventListener("click", () => openCreation(button.dataset.createMode));
  }
  document.addEventListener("organization-duplicate:open", () => openCreation("duplicate"));
  window.addEventListener("operator-session-cleared", () => {
    generation++;
    catalogGeneration++;
    pending = preview = null;
    busy = false;
    source.value = id.value = name.value = description.value = baseDN.value = importSource.value = importFile.value = review.textContent = "";
    confirm.checked = false;
    list.replaceChildren();
    catalogStatus.textContent = "Operator session ended. Sign in and refresh drafts to reconcile any pending creation.";
    try { sessionStorage.removeItem(storageKey); }
    catch (error) { catalogStatus.textContent += ` Retained operation metadata could not be cleared: ${error.message}`; }
    if (dialog.open) dialog.close();
    render();
  });
  void refreshDrafts();
})();
