(() => {
  function initializeOperatorLogin() {
    const widget = document.querySelector(".operator-login");
    const status = document.getElementById("operator-login-status");
    const login = document.getElementById("operator-login-form");
    const logout = document.getElementById("operator-logout");
    const scope = document.getElementById("operator-scope");
    if (!widget || !status || !login || !logout || !scope) return;
    if (widget.dataset.operatorLoginReady === "true") {
      if (widget.isConnected) void widget.operatorRefresh?.();
      return;
    }
    widget.dataset.operatorLoginReady = "true";
    let csrf = "";
    let busy = false;
    let generation = 0;
    let authConfigured = false;
    let canAttemptLogin = false;
    const retry = document.createElement("button");
    retry.type = "button";
    retry.className = "button button-quiet operator-login-retry";
    retry.textContent = "Retry";
    retry.hidden = true;
    widget.append(retry);

    const setGate = (enabled) => document.body.classList.toggle("operator-auth-gated", enabled);
    setGate(true);

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
      const url = new URL(location.href);
      const loginResult = url.searchParams.get("operator_login");
      if (loginResult === "failed" || loginResult === "denied") {
        url.searchParams.delete("operator_login");
        history.replaceState(history.state, "", `${url.pathname}${url.search}${url.hash}`);
      }
      clearSession();
      retry.hidden = true;
      status.textContent = "Checking operator login";
      try {
        const configResponse = await fetch("/auth/operator/config", { cache: "no-store", credentials: "same-origin", redirect: "error" });
        if (!configResponse.ok) throw new Error(`Login configuration unavailable (HTTP ${configResponse.status}).`);
        const config = await configResponse.json();
        if (typeof config.enabled !== "boolean") throw new Error("Invalid login configuration response.");
        if (current !== generation) return;
        if (!config.enabled) {
          authConfigured = false;
          canAttemptLogin = false;
          setGate(false);
          status.textContent = "Operator login not configured";
          return;
        }
        authConfigured = true;
        if (location.protocol !== "https:") {
          canAttemptLogin = false;
          throw new Error("Operator login requires direct HTTPS.");
        }
        canAttemptLogin = true;
        const response = await fetch("/auth/operator/session", { cache: "no-store", credentials: "same-origin", redirect: "error" });
        if (current !== generation) return;
        if (response.status === 401) {
          login.hidden = false;
          setGate(true);
          status.textContent = loginResult === "denied"
            ? "This account is not authorized. Contact your operator administrator."
            : loginResult === "failed" ? "Sign-in failed. Check your credentials and try again." : "Not signed in";
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
        const simulator = widget.dataset.operatorApplication === "simulator";
        const appPermissions = simulator
          ? ["simulator.", "organization.", "scenario.", "directory.", "device."]
          : ["monitor.", "policy.", "gateway."];
        const hasAppPermission = identity.permissions.includes("read") || identity.permissions.some((permission) =>
          appPermissions.some((prefix) => permission.startsWith(prefix))) ||
          (!simulator && identity.permissions.some((permission) => ["create", "cancel", "approve", "reject"].includes(permission)));
        if (!hasAppPermission) {
          csrf = session.csrf;
          logout.hidden = false;
          setGate(true);
          status.textContent = "Signed in, but this identity has no application permissions.";
          return;
        }
        csrf = session.csrf;
        logout.hidden = false;
        const humanLabel = identity.display_name || identity.email || "";
        status.textContent = humanLabel ? `Signed in: ${humanLabel}` : "Signed in";
        setGate(false);
        scope.hidden = false;
        const suffix = widget.dataset.operatorApplication === "simulator"
          ? " Simulator access is limited to the listed scopes."
          : " Enrollment actions also require backend delegation.";
        const label = humanLabel || "Display name unavailable";
        const email = identity.email && identity.email !== humanLabel ? ` (${identity.email})` : "";
        scope.textContent = `${label}${email} · Issuer: ${identity.issuer}. Configured organizations: ${identity.organizations.join(", ")}. Permissions: ${identity.permissions.join(", ")}.${suffix}`;
        window.dispatchEvent(new CustomEvent("operator-session-ready", { detail: { identity, csrf } }));
      } catch (error) {
        if (current !== generation) return;
        clearSession();
        status.textContent = error.message;
        setGate(true);
        login.hidden = !canAttemptLogin;
        retry.hidden = false;
      } finally {
        if (current === generation) window.dispatchEvent(new Event("operator-login-checked"));
      }
    }
    widget.operatorRefresh = readState;
    retry.addEventListener("click", () => void readState());

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
    window.addEventListener("focus", () => { if (widget.isConnected && !busy) void readState(); });
    window.addEventListener("pageshow", (event) => { if (widget.isConnected && event.persisted && !busy) void readState(); });
    window.addEventListener("pagehide", () => { if (widget.isConnected) { ++generation; clearSession(); } });
    void readState();
  }

  window.initializeOperatorLogin = initializeOperatorLogin;
  initializeOperatorLogin();
})();
