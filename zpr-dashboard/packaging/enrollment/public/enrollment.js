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

  const configuredURL = (value) => {
    if (typeof value !== "string" || !value.trim()) return null;
    let parsed;
    try {
      parsed = new URL(value, window.location.href);
    } catch {
      return null;
    }
    if (parsed.username || parsed.password || parsed.search || parsed.hash) return;
    const host = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, "");
    if (parsed.protocol === "http:") {
      const octets = host.split(".").map(Number);
      const privateIPv4 = octets.length === 4 && octets.every((part) => Number.isInteger(part) && part >= 0 && part <= 255) &&
        (octets[0] === 10 || (octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31) ||
          (octets[0] === 192 && octets[1] === 168));
      if (!privateIPv4 && !host.endsWith(".local")) return null;
      return { href: parsed.href, development: true };
    }
    if (parsed.protocol !== "https:") return null;
    return { href: parsed.href, development: false };
  };
  const config = window.ZPR_ENROLLMENT_PUBLIC_CONFIG || {};
  const installer = configuredURL(config.macInstallerURL);
  const setupConfig = configuredURL(config.macSetupConfigURL);
  if (installer) {
    const link = document.getElementById("installer-link");
    link.href = installer.href;
    link.hidden = false;
    document.getElementById("installer-unavailable").hidden = true;
    if (installer.development) document.getElementById("installer-dev-warning").hidden = false;
  }
  if (setupConfig) {
    const link = document.getElementById("setup-config-link");
    link.href = setupConfig.href;
    link.hidden = false;
    if (setupConfig.development) document.getElementById("installer-dev-warning").hidden = false;
  }
})();