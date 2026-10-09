(() => {
  let csrf = "";
  let ready;
  let sessionRefresh = null;
  const initialCheck = new Promise((resolve) => { ready = resolve; });
  window.addEventListener("operator-login-checked", () => ready(), { once: true });
  window.addEventListener("operator-session-ready", (event) => { csrf = event.detail?.csrf ?? ""; });
  window.addEventListener("operator-session-cleared", () => { csrf = ""; });
  window.addEventListener("pagehide", () => { csrf = ""; });

  function recheckOperatorSession() {
    if (sessionRefresh) return;
    const refresh = document.querySelector(".operator-login")?.operatorRefreshAfterAPIRejection;
    if (!refresh) return;
    sessionRefresh = (async () => {
      try {
        await refresh();
      } finally {
        sessionRefresh = null;
      }
    })();
  }

  window.zprOperatorFetch = async (path, options = {}) => {
    await initialCheck;
    const url = new URL(path, location.href);
    const method = (options.method ?? "GET").toUpperCase();
    const headers = new Headers(options.headers);
    if (csrf && url.origin === location.origin && url.pathname.startsWith("/api/") &&
        !["GET", "HEAD", "OPTIONS"].includes(method)) {
      headers.set("X-ZPR-CSRF", csrf);
    }
    const response = await fetch(path, { ...options, headers });
    if (url.origin === location.origin && url.pathname.startsWith("/api/") &&
        [401, 403].includes(response.status)) {
      recheckOperatorSession();
    }
    return response;
  };
})();
