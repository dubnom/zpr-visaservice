(() => {
  const form = document.getElementById("policy-operation-status-form");
  const resultElement = document.getElementById("policy-operation-status-result");
  let generation = 0;
  function clearStatus() {
    generation++;
    resultElement.hidden = true;
    resultElement.textContent = "";
  }
  form.addEventListener("input", clearStatus);
  document.addEventListener("simulator:deactivate", clearStatus);
  const messages = {
    pending: "Operation is pending. This read-only check does not apply it.",
    failed: "The operation is recorded as failed.",
    uncertain: "The operation outcome is uncertain. Reconcile before taking further action.",
    succeeded: "This exact operation succeeded and fresh current state matches its recorded receipt.",
    superseded: "This operation succeeded historically, but a later version is current—even if its digest matches.",
    unknown: "No operation record matches this organization, target, and operation ID.",
    unauthorized: "The configured Policy-Service read identity is not authorized for this organization and target.",
    unavailable: "Policy-Service status is unavailable or could not be verified.",
  };
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!form.reportValidity()) return;
    const fields = new FormData(form);
    const organization = String(fields.get("organization") || "");
    const target = String(fields.get("target") || "");
    const operation = String(fields.get("operation") || "");
    const requestGeneration = ++generation;
    const button = form.querySelector("button[type='submit']");
    button.disabled = true;
    resultElement.hidden = false;
    resultElement.textContent = "Checking read-only operation status...";
    try {
      const path = `/api/simulator/policy-operations/status/${[organization, target, operation].map(encodeURIComponent).join("/")}`;
      const fetchStatus = window.zprOperatorFetch ?? window.fetch.bind(window);
      const response = await fetchStatus(path, { cache: "no-store", headers: { Accept: "application/json" } });
      const result = await response.json();
      if (requestGeneration !== generation) return;
      const message = messages[result?.status] || messages.unavailable;
      if (!response.ok || !["pending", "failed", "uncertain", "succeeded", "superseded"].includes(result?.status)) {
        resultElement.textContent = ["unknown", "unauthorized", "unavailable"].includes(result?.status) ?
          message : messages.unavailable;
        return;
      }
      if (result.organization !== organization || result.operation?.target_id !== target ||
          result.operation?.operation_id !== operation || !result.approver?.issuer || !result.approver?.subject) {
        resultElement.textContent = messages.unavailable;
        return;
      }
      const evidence = [
        `Organization ${result.organization}; target ${result.operation.target_id}; operation ${result.operation.operation_id}.`,
        `Durable record ${result.durable_status}, version ${result.record_version}.`,
        `Named approver ${result.approver.subject} (${result.approver.issuer}).`,
      ];
      if (result.receipt) evidence.push(`Recorded receipt ${result.receipt.status}${result.receipt.applied_digest ? `, digest ${result.receipt.applied_digest}, version ${result.receipt.applied_vinst}` : ""}.`);
      if (result.current_readback) evidence.push(`Fresh current readback digest ${result.current_readback.current_digest}, version ${result.current_readback.current_vinst}.`);
      resultElement.textContent = `${message} ${evidence.join(" ")}`;
    } catch {
      if (requestGeneration === generation) resultElement.textContent = messages.unavailable;
    } finally {
      button.disabled = false;
    }
  });
})();
