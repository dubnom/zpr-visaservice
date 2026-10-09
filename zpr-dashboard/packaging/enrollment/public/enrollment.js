"use strict";

(() => {
  const fragment = new URLSearchParams(window.location.hash.slice(1));
  const query = new URLSearchParams(window.location.search);
  const includesActivationCode = [fragment, query].some((parameters) =>
    ["code", "activation_code", "enrollment_code"].some((key) => parameters.has(key)));
  const organization = fragment.get("organization") || "";
  const invitationID = fragment.get("invitation_id") || "";

  window.history.replaceState(null, "", window.location.pathname);

  const unsafeNotice = document.getElementById("unsafe-link");
  const invalidNotice = document.getElementById("invalid-invitation");
  const invitationSection = document.getElementById("invitation");
  if (includesActivationCode) {
    unsafeNotice.hidden = false;
    invalidNotice.hidden = true;
    return;
  }

  const identifier = /^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$/;
  if (!identifier.test(organization) || !identifier.test(invitationID)) return;

  document.getElementById("organization").textContent = organization;
  document.getElementById("invitation-id").textContent = invitationID;
  invitationSection.hidden = false;
  invalidNotice.hidden = true;

  const installerURL = window.ZPR_ENROLLMENT_PUBLIC_CONFIG?.macInstallerURL;
  if (typeof installerURL !== "string" || !installerURL.trim()) return;
  try {
    const parsed = new URL(installerURL, window.location.href);
    if (parsed.protocol !== "https:" || parsed.username || parsed.password) return;
    const link = document.getElementById("installer-link");
    link.href = parsed.href;
    link.hidden = false;
    document.getElementById("installer-unavailable").hidden = true;
  } catch {
    return;
  }
})();