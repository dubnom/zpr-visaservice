(() => {
  const root = document.getElementById("provisioning-page");
  if (!root) return;
  const status = root.querySelector("#provisioning-status");
  const error = root.querySelector("#provisioning-error");
  const details = root.querySelector("#provisioning-details");
  const machine = root.querySelector("#provisioning-machine");
  const approval = root.querySelector("#provisioning-approval");
  const verification = root.querySelector("#provisioning-verification");
  const base = () => `/api/simulator/organizations/demo/machines/${encodeURIComponent(machine.value)}/provisioning`;
  let pending = false;
  let snapshot;
  function controls() {
    approval.disabled = pending;
    verification.disabled = pending;
    root.querySelectorAll("[data-provision-action]").forEach(button => {
      const action = button.dataset.provisionAction;
      button.disabled = pending || typeof window.zprOperatorFetch !== "function" ||
        (action !== "refresh" && !snapshot) ||
        (action === "approve-and-provision" && (!approval.checked || !["unprovisioned", "failed"].includes(snapshot?.state))) ||
        (["start", "stop", "verify"].includes(action) && ["unprovisioned", "provisioning"].includes(snapshot?.state)) ||
        (action === "verify" && (!verification.checked || snapshot?.state !== "ready"));
    });
  }
  async function request(action) {
    if (pending || typeof window.zprOperatorFetch !== "function") return;
    if (action === "approve-and-provision" && !approval.checked || action === "verify" && !verification.checked) return;
    pending = true;
    controls();
    error.hidden = true;
    status.textContent = action === "refresh" ? "Checking actual resources and HTTP..." : `Running ${action}; this can take up to three minutes...`;
    try {
      const response = await window.zprOperatorFetch(
        action === "refresh" ? base() : `${base()}/${action}`,
        { method: action === "refresh" ? "GET" : "POST" });
      if (!response.ok) throw new Error(`HTTP ${response.status}: ${(await response.text()).slice(-2500)}`);
      const value = await response.json();
      if (value.version !== 1 || value.organization_id !== "demo" || value.machine_id !== machine.value ||
          typeof value.state !== "string" || typeof value.resources !== "object") {
        throw new Error("Provisioning response does not match this organization and machine.");
      }
      snapshot = value;
      status.textContent = `Machine state: ${value.state}${value.state === "ready" ? " (protected HTTP verified)" : ""}`;
      details.textContent = JSON.stringify(value, null, 2);
      if (value.error) {
        error.textContent = value.error;
        error.hidden = false;
      }
    } catch (failure) {
      snapshot = undefined;
      status.textContent = "Operation could not be confirmed. Refresh to inspect retained state before retrying.";
      error.textContent = failure.message;
      error.hidden = false;
    } finally {
      if (action === "approve-and-provision") approval.checked = false;
      if (action === "verify") verification.checked = false;
      pending = false;
      controls();
    }
  }
  root.addEventListener("click", event => {
    const button = event.target.closest("[data-provision-action]");
    if (button && root.contains(button)) void request(button.dataset.provisionAction);
  });
  approval.addEventListener("change", controls);
  verification.addEventListener("change", controls);
  machine.addEventListener("change", () => { snapshot = undefined; void request("refresh"); });
  window.addEventListener("operator-session-ready", () => { void request("refresh"); });
  document.addEventListener("simulator:activate", event => {
    if (event.detail.path === "/simulator-provisioning.html") void request("refresh");
  });
  controls();
  void request("refresh");
})();
