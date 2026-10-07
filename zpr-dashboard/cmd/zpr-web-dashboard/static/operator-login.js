(() => {
  const status = document.getElementById("operator-login-status");
  const login = document.getElementById("operator-login-form");
  const logout = document.getElementById("operator-logout");
  const scope = document.getElementById("operator-scope");
  if (!status || !login || !logout || !scope) return;
  let csrf = "";
  let busy = false;
  let generation = 0;

  function clearSession() {
    const hadSession = !scope.hidden;
    csrf = "";
    logout.hidden = true;
    login.hidden = true;
    scope.hidden = true;
    scope.textContent = "";
    if (hadSession) window.dispatchEvent(new Event("operator-session-cleared"));
  }

  async function readState() {
    const current = ++generation;
    clearSession();
    status.textContent = "Checking operator login";
    try {
      const configResponse = await fetch("/auth/operator/config", { cache: "no-store", credentials: "same-origin", redirect: "error" });
      if (!configResponse.ok) throw new Error(`Login configuration unavailable (HTTP ${configResponse.status}).`);
      const config = await configResponse.json();
      if (typeof config.enabled !== "boolean") throw new Error("Invalid login configuration response.");
      if (current !== generation) return;
      if (!config.enabled) {
        status.textContent = "Operator login not configured";
        return;
      }
      if (location.protocol !== "https:") throw new Error("Operator login requires direct HTTPS.");
      const response = await fetch("/auth/operator/session", { cache: "no-store", credentials: "same-origin", redirect: "error" });
      if (current !== generation) return;
      if (response.status === 401) {
        login.hidden = false;
        status.textContent = "Not signed in";
        return;
      }
      if (!response.ok) throw new Error(`Operator session unavailable (HTTP ${response.status}).`);
      const session = await response.json();
      if (current !== generation) return;
      const identity = session.identity;
      const validList = (list) => Array.isArray(list) && list.length > 0 && list.every((value) => typeof value === "string" && value.length > 0);
      if (!identity || typeof identity.subject !== "string" || !identity.subject ||
          typeof identity.issuer !== "string" || !identity.issuer || !validList(identity.organizations) ||
          !validList(identity.permissions) || typeof session.csrf !== "string" || !session.csrf) {
        throw new Error("Invalid operator session response.");
      }
      csrf = session.csrf;
      logout.hidden = false;
      status.textContent = `Signed in: ${identity.subject}`;
      scope.hidden = false;
      scope.textContent = `Named identity: ${identity.subject} (${identity.issuer}). Configured organizations: ${identity.organizations.join(", ")}. Permissions: ${identity.permissions.join(", ")}. Enrollment actions also require backend delegation.`;
      window.dispatchEvent(new CustomEvent("operator-session-ready", { detail: { identity, csrf } }));
    } catch (error) {
      if (current !== generation) return;
      clearSession();
      status.textContent = error.message;
    }
  }

  logout.addEventListener("click", async () => {
    if (busy || !csrf) return;
    busy = true;
    ++generation;
    logout.disabled = true;
    status.textContent = "Signing out";
    try {
      const response = await fetch("/auth/operator/logout", {
        method: "POST", cache: "no-store", credentials: "same-origin", redirect: "error",
        headers: { "X-ZPR-CSRF": csrf },
      });
      if (response.status !== 204) throw new Error(`Sign out was not confirmed (HTTP ${response.status}). Retry or reload to check the session.`);
      clearSession();
      await readState();
    } catch (error) {
      status.textContent = error.message;
    } finally {
      logout.disabled = false;
      busy = false;
    }
  });
  window.addEventListener("focus", () => { if (!busy) void readState(); });
  window.addEventListener("pageshow", (event) => { if (event.persisted && !busy) void readState(); });
  window.addEventListener("pagehide", () => { ++generation; clearSession(); });
  void readState();
})();
