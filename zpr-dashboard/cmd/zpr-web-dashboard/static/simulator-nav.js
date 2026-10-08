(() => {
  if (window.__zprSimulatorNavigation) return;
  window.__zprSimulatorNavigation = true;
  const pageCache = new Map();
  const pageViews = new Map();
  const pageTitles = new Map();
  let currentPath = location.pathname;

  const pageKey = (url) => new URL(url, location.href).pathname;
  const dispatchPageEvent = (name, path) => document.dispatchEvent(new CustomEvent(name, { detail: { path } }));
  const setActiveOrganization = (organization) => {
    const status = document.querySelector(".main-content > .topbar .topbar-status");
    if (!status) return;
    let badge = status.querySelector(".simulator-active-organization");
    if (!badge) {
      badge = document.createElement("span");
      badge.className = "simulator-active-organization";
      badge.setAttribute("aria-label", "Active organization");
      status.append(badge);
    }
    badge.textContent = organization?.name || organization?.id || "Organization unavailable";
    badge.hidden = !organization;
  };
  const refreshActiveOrganization = async () => {
    try {
      const response = await fetch("/api/simulator/organizations", { cache: "no-store" });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      setActiveOrganization((data.organizations || []).find((item) => item.id === data.active_id));
    } catch {
      setActiveOrganization(null);
    }
  };
  let operatorLoginScriptLoading;
  const mountOperatorLogin = () => {
    const topbar = document.querySelector(".main-content > .topbar");
    if (!topbar) return;
    let widget = topbar.querySelector(".simulator-operator-login");
    if (!widget) {
      widget = document.createElement("div");
      widget.className = "operator-login simulator-operator-login";
      widget.dataset.operatorApplication = "simulator";
      widget.setAttribute("aria-label", "Simulator operator login");
      const status = document.createElement("span");
      status.id = "operator-login-status";
      status.setAttribute("role", "status");
      status.setAttribute("aria-live", "polite");
      status.textContent = "Checking operator login";
      const login = document.createElement("form");
      login.id = "operator-login-form";
      login.method = "post";
      login.action = "/auth/operator/login";
      login.hidden = true;
      const signIn = document.createElement("button");
      signIn.className = "button";
      signIn.type = "submit";
      signIn.textContent = "Sign in";
      login.append(signIn);
      const signOut = document.createElement("button");
      signOut.id = "operator-logout";
      signOut.className = "button button-quiet";
      signOut.type = "button";
      signOut.textContent = "Sign out";
      signOut.hidden = true;
      widget.append(status, login, signOut);
      topbar.append(widget);
      const scope = document.createElement("p");
      scope.id = "operator-scope";
      scope.className = "simulator-operator-scope";
      scope.hidden = true;
      topbar.after(scope);
    }
    if (window.initializeOperatorLogin) {
      window.initializeOperatorLogin();
      return;
    }
    if (!operatorLoginScriptLoading) {
      operatorLoginScriptLoading = new Promise((resolve, reject) => {
        const script = document.createElement("script");
        script.src = "/operator-login.js?v=5";
        script.addEventListener("load", resolve, { once: true });
        script.addEventListener("error", reject, { once: true });
        document.head.append(script);
      });
    }
    operatorLoginScriptLoading.then(() => window.initializeOperatorLogin?.()).catch(() => {
      const status = document.getElementById("operator-login-status");
      if (status) status.textContent = "Operator login UI unavailable";
    });
  };
  const syncNavigation = () => {
    const nav = document.querySelector(".primary-nav");
    if (!nav) return;
    let organizationLink = nav.querySelector('a[href="/organizations.html"]');
    if (!organizationLink) {
      organizationLink = document.createElement("a");
      organizationLink.className = "nav-link";
      organizationLink.dataset.simulatorNav = "";
      organizationLink.href = "/organizations.html";
      organizationLink.textContent = "Organizations";
      const scenarioLink = nav.querySelector('a[href="/scenarios.html"]');
      nav.insertBefore(organizationLink, scenarioLink || null);
    }
    let logsLink = nav.querySelector('a[href="/machine-logs.html"]');
    if (!logsLink) {
      logsLink = document.createElement("a");
      logsLink.className = "nav-link";
      logsLink.dataset.simulatorNav = "";
      logsLink.href = "/machine-logs.html";
      logsLink.textContent = "Workers";
      nav.insertBefore(logsLink, nav.querySelector('a[href="/activity.html"]'));
    }
    logsLink.textContent = "Workers";
    nav.querySelector('a[href="/agents.html"]')?.remove();
    const brand = document.querySelector(".brand[data-simulator-nav]");
    if (brand) {
      brand.href = "/organizations.html";
      brand.setAttribute("aria-label", "ZPR Simulator organizations");
    }
    let sourceLink = nav.querySelector('a[href="/trusted-source.html"]');
    if (!sourceLink) {
      sourceLink = document.createElement("a");
      sourceLink.className = "nav-link";
      sourceLink.dataset.simulatorNav = "";
      sourceLink.href = "/trusted-source.html";
      sourceLink.textContent = "Trusted Sources";
      nav.insertBefore(sourceLink, nav.querySelector('a[href="/activity.html"]'));
    }
    sourceLink.textContent = "Trusted Sources";
    for (const path of ["/organizations.html", "/scenarios.html", "/trusted-source.html", "/activity.html", "/machine-logs.html"]) {
      const link = nav.querySelector(`a[href="${path}"]`);
      if (link) nav.append(link);
    }
    nav.querySelectorAll("a[data-simulator-nav]").forEach((link) => {
      link.classList.toggle("active", pageKey(link.href) === currentPath);
    });
    void refreshActiveOrganization();
    mountOperatorLogin();
  };

  window.addEventListener("simulator:organization-context", (event) => setActiveOrganization(event.detail));

  const fetchPage = async (url) => {
    const key = pageKey(url);
    if (!pageCache.has(key)) {
      pageCache.set(key, fetch(url, { headers: { Accept: "text/html" } }).then((response) => {
        if (!response.ok) throw new Error(`page request failed: ${response.status}`);
        return response.text();
      }).catch((error) => {
        pageCache.delete(key);
        throw error;
      }));
    }
    return pageCache.get(key);
  };

  const loadPage = async (url, push) => {
    const nextPath = pageKey(url);
    if (nextPath === currentPath) {
      if (push) history.pushState({}, "", url);
      return;
    }

    let html;
    if (!pageViews.has(nextPath)) html = await fetchPage(url);
    dispatchPageEvent("simulator:deactivate", currentPath);
    const previousView = document.createDocumentFragment();
    while (document.body.firstChild) previousView.append(document.body.firstChild);
    pageViews.set(currentPath, previousView);

    if (pageViews.has(nextPath)) {
      document.body.replaceChildren(pageViews.get(nextPath));
      document.title = pageTitles.get(nextPath) || document.title;
    } else {
      const parsed = new DOMParser().parseFromString(html, "text/html");
      for (const stylesheet of parsed.head.querySelectorAll('link[rel="stylesheet"]')) {
        const href = new URL(stylesheet.getAttribute("href"), location.href).href;
        if (![...document.head.querySelectorAll('link[rel="stylesheet"]')].some((link) => link.href === href)) {
          const link = document.createElement("link");
          link.rel = "stylesheet";
          link.href = href;
          document.head.append(link);
        }
      }
      document.title = parsed.title;
      pageTitles.set(nextPath, parsed.title);
      const scripts = [...parsed.body.querySelectorAll("script[src]")];
      scripts.forEach((script) => script.remove());
      document.body.replaceChildren(...parsed.body.childNodes);
      for (const oldScript of scripts) {
        if (new URL(oldScript.src).pathname === "/simulator-nav.js") continue;
        const script = document.createElement("script");
        script.src = oldScript.src;
        script.async = false;
        document.body.append(script);
        await new Promise((resolve, reject) => { script.addEventListener("load", resolve, { once: true }); script.addEventListener("error", reject, { once: true }); });
      }
    }
    currentPath = nextPath;
    syncNavigation();
    if (push) history.pushState({}, "", url);
    dispatchPageEvent("simulator:activate", currentPath);
  };

  document.addEventListener("click", (event) => {
    const link = event.target.closest("a[data-simulator-nav]");
    if (!link || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    loadPage(link.href, true).catch(() => { window.location.href = link.href; });
  });
  window.addEventListener("popstate", () => loadPage(window.location.href, false));
  window.addEventListener("load", () => {
    syncNavigation();
    pageCache.set(currentPath, Promise.resolve(document.documentElement.outerHTML));
    pageTitles.set(currentPath, document.title);
    document.querySelectorAll("a[data-simulator-nav]").forEach((link) => { fetchPage(link.href).catch(() => {}); });
    dispatchPageEvent("simulator:activate", currentPath);
  }, { once: true });
})();
