(() => {
  let csrf = "";
  let ready;
  const initialCheck = new Promise((resolve) => { ready = resolve; });
  window.addEventListener("operator-login-checked", () => ready(), { once: true });
  window.addEventListener("operator-session-ready", (event) => { csrf = event.detail?.csrf ?? ""; });
  window.addEventListener("operator-session-cleared", () => { csrf = ""; });
  window.addEventListener("pagehide", () => { csrf = ""; });

  window.zprOperatorFetch = async (path, options = {}) => {
    await initialCheck;
    const url = new URL(path, location.href);
    const method = (options.method ?? "GET").toUpperCase();
    const headers = new Headers(options.headers);
    if (csrf && url.origin === location.origin && url.pathname.startsWith("/api/") &&
        !["GET", "HEAD", "OPTIONS"].includes(method)) {
      headers.set("X-ZPR-CSRF", csrf);
    }
    return fetch(path, { ...options, headers });
  };
})();
