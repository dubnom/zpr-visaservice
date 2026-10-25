(() => {
  function initializeOperatorLogin() {
    const widget = document.querySelector(".operator-login");
    const status = document.getElementById("operator-login-status");
    const login = document.getElementById("operator-login-form");
    const logout = document.getElementById("operator-logout");
    const scope = document.getElementById("operator-scope");
    const title = widget?.querySelector(".operator-login-title");
    const description = widget?.querySelector(".operator-login-description");
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
    let loginFailure = "";
    const returnLocationKey = "zpr.operator-login.return-location";
    const directSignIn = widget.dataset.operatorApplication !== "simulator";
    const sessionKey = "zpr.operator-login.signed-in";
    const automaticKey = "zpr.operator-login.automatic-attempt";
    const expiryKey = "zpr.operator-login.session-expired";
    let redirecting = false;
    let expiryTimer;
    if (directSignIn && title) title.textContent = "Sign in";
    function localReturnLocation(value) {
      if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//") || /[\\\u0000-\u001f\u007f]/.test(value)) return null;
      const target = new URL(value, location.origin);
      if (target.origin !== location.origin || target.pathname.startsWith("/auth/")) return null;
      target.searchParams.delete("operator_login");
      return `${target.pathname}${target.search}${target.hash}`;
    }
    const retry = document.createElement("button");
    retry.type = "button";
    retry.className = "button button-quiet operator-login-retry";
    retry.textContent = "Retry";
    retry.hidden = true;
    widget.append(retry);

    const setGate = (enabled) => {
      document.body.classList.toggle("operator-auth-gated", enabled);
      if (title) title.hidden = !enabled;
      if (description) description.hidden = !enabled;
      if (enabled) {
        widget.setAttribute("role", "dialog");
        widget.setAttribute("aria-modal", "true");
        widget.setAttribute("aria-labelledby", "operator-login-title");
        widget.setAttribute("aria-describedby", "operator-login-description");
      } else {
        widget.removeAttribute("role");
        widget.removeAttribute("aria-modal");
        widget.removeAttribute("aria-labelledby");
        widget.removeAttribute("aria-describedby");
      }
    };
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

    async function readState({ preserveSession = false } = {}) {
      if (redirecting) return;
      const firstCheck = generation === 0;
      const current = ++generation;
      const url = new URL(location.href);
      const loginResult = url.searchParams.get("operator_login");
      if (loginResult === "failed" || loginResult === "denied" || loginResult === "expired") {
        loginFailure = loginResult;
        url.searchParams.delete("operator_login");
        history.replaceState(history.state, "", `${url.pathname}${url.search}${url.hash}`);
      }
      retry.hidden = true;
      if (firstCheck && !preserveSession && scope.hidden && directSignIn) widget.classList.add("operator-auth-checking");
      if (!directSignIn) status.textContent = "Checking operator login";
      try {
        const hadSession = !scope.hidden || directSignIn && sessionStorage.getItem(sessionKey) === "true";
        if (directSignIn && !loginFailure && sessionStorage.getItem(expiryKey) === "true") loginFailure = "session_expired";
        const configResponse = await fetch("/auth/operator/config", { cache: "no-store", credentials: "same-origin", redirect: "error" });
        if (!configResponse.ok) throw new Error(`Login configuration unavailable (HTTP ${configResponse.status}).`);
        const config = await configResponse.json();
        if (typeof config.enabled !== "boolean") throw new Error("Invalid login configuration response.");
        if (current !== generation) return;
        if (!config.enabled) {
          authConfigured = false;
          canAttemptLogin = false;
          setGate(false);
          status.textContent = widget.dataset.operatorApplication === "simulator"
            ? "User: Not signed in"
            : "Operator login not configured";
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
          if (hadSession && directSignIn) {
            loginFailure = "session_expired";
            if (directSignIn) {
              sessionStorage.removeItem(sessionKey);
              sessionStorage.setItem(expiryKey, "true");
            }
          }
          clearSession();
          login.hidden = false;
          setGate(true);
          status.textContent = loginFailure === "denied"
            ? "This account is not authorized. Contact your operator administrator."
            : loginFailure === "failed" ? "Sign-in failed. Check your credentials and try again."
            : loginFailure === "expired" ? "Your previous sign-in attempt expired or is no longer available. Select Sign in to start a fresh attempt."
            : loginFailure === "session_expired" ? "Session expired. Redirecting to sign in..."
            : "Not signed in";
          if (directSignIn && (!loginFailure || loginFailure === "session_expired") &&
              sessionStorage.getItem(automaticKey) !== "true") {
            redirecting = true;
            sessionStorage.setItem(automaticKey, "true");
            if (title) title.hidden = true;
            if (description) description.hidden = true;
            login.hidden = true;
            if (loginFailure === "session_expired") {
              widget.classList.remove("operator-auth-checking");
              expiryTimer = setTimeout(() => login.requestSubmit(), 1500);
            } else {
              login.requestSubmit();
            }
          } else if (loginFailure === "session_expired") {
            status.textContent = "Session expired. Sign in again to continue.";
          }
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
        login.hidden = true;
        loginFailure = "";
        if (directSignIn) {
          sessionStorage.setItem(sessionKey, "true");
          sessionStorage.removeItem(automaticKey);
          sessionStorage.removeItem(expiryKey);
        }
        const simulator = widget.dataset.operatorApplication === "simulator";
        const appPermissions = simulator
          ? ["simulator.", "organization.", "scenario.", "directory.", "device."]
          : ["monitor.", "policy.", "gateway.", "trusted-source.", "subject-link."];
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
        const returnLocation = localReturnLocation(sessionStorage.getItem(returnLocationKey));
        sessionStorage.removeItem(returnLocationKey);
        if (returnLocation && returnLocation !== `${location.pathname}${location.search}${location.hash}`) {
          const target = new URL(returnLocation, location.origin);
          const sameDocument = target.pathname === location.pathname && target.search === location.search;
          location.replace(returnLocation);
          if (!sameDocument) return;
        }
        csrf = session.csrf;
        logout.hidden = false;
        const humanLabel = identity.display_name || identity.email || (simulator ? identity.subject : "");
        status.textContent = humanLabel ? `User: ${humanLabel}` : "User";
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
        loginFailure = "failed";
      } finally {
        if (!redirecting) widget.classList.remove("operator-auth-checking");
        if (current === generation) window.dispatchEvent(new Event("operator-login-checked"));
      }
    }
    widget.operatorRefresh = readState;
    widget.operatorRefreshAfterAPIRejection = () => readState({ preserveSession: true });
    retry.addEventListener("click", () => void readState());
    login.addEventListener("submit", event => {
      try {
        const previous = loginFailure && localReturnLocation(sessionStorage.getItem(returnLocationKey));
        const target = previous || localReturnLocation(`${location.pathname}${location.search}${location.hash}`);
        if (!target) throw new Error("Unable to preserve the current application location for sign-in.");
        sessionStorage.setItem(returnLocationKey, target);
      } catch (error) {
        event.preventDefault();
        redirecting = false;
        widget.classList.remove("operator-auth-checking");
        login.hidden = false;
        status.textContent = `Sign-in could not start: ${error.message}`;
      }
    });

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
        loginFailure = directSignIn ? "" : "signed_out";
        if (directSignIn) {
          sessionStorage.removeItem(sessionKey);
          sessionStorage.removeItem(automaticKey);
          sessionStorage.removeItem(expiryKey);
        }
        clearSession();
        await readState();
      } catch (error) {
        status.textContent = error.message;
      } finally {
        logout.disabled = false;
        busy = false;
      }
    });
    window.addEventListener("focus", () => { if (widget.isConnected && !busy) void readState({ preserveSession: true }); });
    window.addEventListener("pageshow", (event) => { if (widget.isConnected && event.persisted && !busy) void readState(); });
    window.addEventListener("pagehide", () => { if (widget.isConnected) { clearTimeout(expiryTimer); ++generation; clearSession(); } });
    void readState();
  }

  window.initializeOperatorLogin = initializeOperatorLogin;
  initializeOperatorLogin();
})();
