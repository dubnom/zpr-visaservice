(() => {
  const form = document.getElementById("provisioning-draft");
  if (!form) return;
  const status = document.getElementById("provisioning-draft-status");
  const fields = [
    ["name", "Name"], ["owner", "Owner"], ["organization", "Requested organization"],
    ["asset_id", "Inventory reference"], ["type", "Requested type"],
    ["profile", "Requested profile"], ["recipient", "Instruction recipient"],
  ];
  let dialog = null;

  function closeDialog() {
    if (!dialog) return;
    const previous = dialog;
    dialog = null;
    previous.close();
    previous.remove();
  }

  function showDialog(title, content) {
    closeDialog();
    dialog = document.createElement("dialog");
    dialog.className = "policy-dialog provisioning-review";
    dialog.setAttribute("aria-labelledby", "provisioning-dialog-title");
    const heading = document.createElement("h2");
    heading.id = "provisioning-dialog-title";
    heading.textContent = title;
    const close = document.createElement("button");
    close.type = "button";
    close.className = "button";
    close.textContent = "Close";
    close.addEventListener("click", closeDialog);
    dialog.append(heading, content, close);
    dialog.addEventListener("cancel", (event) => {
      event.preventDefault();
      closeDialog();
    });
    document.body.append(dialog);
    dialog.showModal();
  }

  form.addEventListener("input", (event) => {
    event.target.setCustomValidity("");
    status.textContent = "Unsaved worksheet. Nothing has been submitted.";
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    for (const [name] of fields) {
      const input = form.elements.namedItem(name);
      input.value = input.value.trim();
      input.setCustomValidity(input.value ? "" : "Enter a value, not just spaces.");
    }
    if (!form.reportValidity()) return;
    const content = document.createElement("div");
    const notice = document.createElement("p");
    notice.textContent = "Unsaved and unvalidated. This review does not create an invitation, reserve an asset, send email, or enroll a machine.";
    const details = document.createElement("dl");
    for (const [name, label] of fields) {
      const term = document.createElement("dt");
      term.textContent = label;
      const value = document.createElement("dd");
      value.textContent = form.elements.namedItem(name).value;
      details.append(term, value);
    }
    content.append(notice, details);
    showDialog("Review invitation worksheet", content);
    status.textContent = "Worksheet reviewed locally. Creation remains locked; no invitation was created.";
  });
  form.addEventListener("reset", () => {
    closeDialog();
    for (const [name] of fields) form.elements.namedItem(name).setCustomValidity("");
    status.textContent = "Worksheet cleared. Nothing has been submitted.";
  });
  window.addEventListener("hashchange", closeDialog);
  window.addEventListener("pagehide", () => {
    form.reset();
    status.textContent = "";
  });
})();
