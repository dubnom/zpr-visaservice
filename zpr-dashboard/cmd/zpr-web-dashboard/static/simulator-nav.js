(() => {
  if (window.__zprSimulatorNavigation) return;
  window.__zprSimulatorNavigation = true;
  const pageCache = new Map();
  const pageViews = new Map();
  const pageTitles = new Map();
  const organizationPages = ["/organizations.html", "/scenarios.html", "/trusted-source.html"];
  const organizationScripts = new Set(["/organizations.js", "/organization-profile.js", "/organization-definitions.js", "/organization-duplicate.js"]);
  let organizationWorkspace;
  let organizationWorkspaceInitialized = false;
  let resolveInitialPage;
  const initialPageReady = new Promise(resolve => { resolveInitialPage = resolve; });
  let currentPath = location.pathname;

  const pageKey = (url) => new URL(url, location.href).pathname;
  const dispatchPageEvent = (name, path) => document.dispatchEvent(new CustomEvent(name, { detail: { path } }));
  function createPagePoller({ path, run, interval, onError }) {
    const paths = Array.isArray(path) ? path : [path];
    const poller = window.ZPRPageRuntime.createPoller({ run, interval, onError });
    document.addEventListener("simulator:activate", (event) => {
      if (paths.includes(event.detail.path)) poller.start();
    });
    document.addEventListener("simulator:deactivate", (event) => {
      if (paths.includes(event.detail.path)) poller.stop();
    });
    return {
      refresh: poller.refresh,
      stop: poller.stop,
    };
  }

  window.ZPRSimulatorNavigation = { createPagePoller };

  let bannerSnapshot;
  let bannerError;
  let bannerOrganizationID;
  const renderBanner = () => {
    const status = document.querySelector(".main-content > .topbar .topbar-status");
    if (!status) return;
    let banner = status.querySelector(".simulator-runtime-banner");
    if (!banner) {
      banner = document.createElement("div");
      banner.className = "simulator-runtime-banner";
      banner.setAttribute("role", "status");
      banner.setAttribute("aria-live", "polite");
      for (const [key, label] of [["backend", "Simulator backend"], ["organization", "Organization"], ["scenario", "Scenario"]]) {
        const item = document.createElement("div");
        item.className = `simulator-banner-${key} connection-state`;
        const content = document.createElement("div");
        const heading = document.createElement("strong");
        heading.textContent = label;
        const value = document.createElement("small");
        value.dataset.bannerValue = key;
        content.append(heading, value);
        if (key === "backend") {
          const lamp = document.createElement("span");
          lamp.className = "state-lamp";
          lamp.setAttribute("aria-hidden", "true");
          item.append(lamp);
        }
        item.append(content);
        banner.append(item);
      }
      for (const child of status.children) child.hidden = true;
      status.prepend(banner);
    }
    const setValue = (key, text) => { banner.querySelector(`[data-banner-value="${key}"]`).textContent = text; };
    banner.dataset.state = bannerError ? "disconnected" : bannerSnapshot ? "connected" : "loading";
    setValue("backend", bannerError ? "Unavailable" : bannerSnapshot ? "Connected" : "Connecting");
    banner.title = bannerError?.message || "";
    setValue("organization", bannerSnapshot?.organization?.name || bannerSnapshot?.active_organization_id || "Unavailable");
    const run = bannerSnapshot?.run;
    const scenario = (bannerSnapshot?.scenarios || []).find(item => item.id === run?.scenario_id);
    setValue("scenario", !bannerSnapshot ? "Unavailable" : run?.scenario_id
      ? `${run.scenario_name || scenario?.name || run.scenario_id} · ${run.state || "unknown"}`
      : "No scenario running · idle");
  };
  const bannerPoller = window.ZPRPageRuntime.createPoller({
    interval: 3000,
    run: async ({ signal, isCurrent }) => {
      const data = await window.ZPRPageRuntime.requestJSON(window.fetch.bind(window), "/api/simulator/scenarios", { signal });
      if (!isCurrent()) return;
      if (typeof data?.active_organization_id !== "string" || !data.active_organization_id ||
          !data.organization || data.organization.id !== data.active_organization_id ||
          !Array.isArray(data.scenarios) || typeof data.run?.state !== "string") {
        throw new Error("Invalid simulator status response.");
      }
      bannerSnapshot = data;
      bannerOrganizationID = data.active_organization_id;
      bannerError = null;
      renderBanner();
    },
    onError: error => {
      bannerSnapshot = null;
      bannerError = error;
      renderBanner();
    },
  });
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
      const title = document.createElement("h2");
      title.className = "operator-login-title";
      title.id = "operator-login-title";
      title.textContent = "ZPR Operator Access";
      title.hidden = true;
      const description = document.createElement("p");
      description.className = "operator-login-description";
      description.id = "operator-login-description";
      description.textContent = "Sign in with an authorized operator account to continue.";
      description.hidden = true;
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
      widget.append(title, description, status, login, signOut);
      const actions = topbar.querySelector(".top-actions") || topbar;
      actions.insertBefore(widget, actions.querySelector(".help-trigger"));
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
      const requestScriptLoading = window.zprOperatorFetch ? Promise.resolve() : new Promise((resolve, reject) => {
        const script = document.createElement("script");
        script.src = "/operator-request.js?v=3";
        script.addEventListener("load", resolve, { once: true });
        script.addEventListener("error", reject, { once: true });
        document.head.append(script);
      });
      operatorLoginScriptLoading = requestScriptLoading.then(() => new Promise((resolve, reject) => {
        const script = document.createElement("script");
        script.src = "/operator-login.js?v=14";
        script.addEventListener("load", resolve, { once: true });
        script.addEventListener("error", reject, { once: true });
        document.head.append(script);
      }));
    }
    operatorLoginScriptLoading.then(() => window.initializeOperatorLogin?.()).catch(() => {
      const status = document.getElementById("operator-login-status");
      if (status) status.textContent = "Operator login UI unavailable";
    });
  };
  const syncNavigation = () => {
    const nav = document.querySelector(".primary-nav");
    if (!nav) return;
    const simulating = currentPath === "/scenarios.html" && location.hash === "#simulate";
    if (currentPath === "/scenarios.html") document.title = simulating ? "ZPR Simulator Simulate" : "ZPR Simulator Scenarios";
    document.body.classList.toggle("simulator-simulating", simulating);
    if (organizationWorkspace) organizationWorkspace.hidden = simulating;
    const runner = document.querySelector(".scenario-run-pane");
    if (runner) runner.hidden = !simulating;
    const catalog = document.querySelector(".scenario-list-pane");
    if (catalog) catalog.hidden = simulating;
    const simulateControls = document.getElementById("scenario-simulate-controls");
    if (simulateControls) simulateControls.hidden = !simulating;
    let simulateLink = nav.querySelector('a[href="/scenarios.html#simulate"]');
    if (!simulateLink) {
      simulateLink = document.createElement("a");
      simulateLink.className = "nav-link";
      simulateLink.dataset.simulatorNav = "";
      simulateLink.href = "/scenarios.html#simulate";
      simulateLink.textContent = "Simulate";
      nav.append(simulateLink);
    }
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
    nav.querySelector('a[href="/scenarios.html"]')?.remove();
    nav.querySelector('a[href="/trusted-source.html"]')?.remove();
    for (const [label, paths] of [
      ["MONITORING", ["/scenarios.html#simulate", "/activity.html", "/machine-logs.html"]],
      ["CONFIGURATION", ["/organizations.html"]],
    ]) {
      let group = nav.querySelector(`[data-simulator-nav-group="${label}"]`);
      if (!group) {
        group = document.createElement("div");
        group.className = "nav-group";
        group.dataset.simulatorNavGroup = label;
        const heading = document.createElement("div");
        heading.className = "nav-group-label";
        heading.textContent = label;
        group.append(heading);
        nav.append(group);
      }
      for (const path of paths) {
        const link = nav.querySelector(`a[href="${path}"]`);
        if (link && link.parentElement !== group) group.append(link);
      }
    }
    const monitoring = nav.querySelector('[data-simulator-nav-group="MONITORING"]');
    const configuration = nav.querySelector('[data-simulator-nav-group="CONFIGURATION"]');
    nav.append(monitoring, configuration);
    if (!nav.querySelector("[data-simulator-nav-separator]")) {
      const separator = document.createElement("div");
      separator.className = "nav-separator";
      separator.dataset.simulatorNavSeparator = "";
      separator.setAttribute("role", "separator");
      nav.insertBefore(separator, configuration);
    }
    nav.insertBefore(nav.querySelector("[data-simulator-nav-separator]"), configuration);
    nav.querySelectorAll("a").forEach((link) => {
      const path = pageKey(link.href);
      const active = link === simulateLink ? simulating
        : path === "/organizations.html" ? !simulating && organizationPages.includes(currentPath)
        : path === currentPath;
      link.classList.toggle("active", active);
      if (active) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
      if (path === currentPath) delete link.dataset.simulatorNav;
      else link.dataset.simulatorNav = "";
    });
    if (organizationPages.includes(currentPath)) {
      let sections = document.querySelector(".organization-navigation");
      if (!sections) {
        sections = document.createElement("nav");
        sections.className = "organization-navigation";
        sections.setAttribute("aria-label", "Organization sections");
        for (const [label, href] of [
          ["Scenarios", "/scenarios.html"],
          ["Identity", "/organizations.html#organization-identity"],
          ["Trusted Sources", "/trusted-source.html"],
          ["Policy", "/organizations.html#organization-policy"],
          ["Services", "/organizations.html#organization-services"],
        ]) {
          const link = document.createElement("a");
          link.textContent = label;
          link.href = href;
          if (new URL(link.href).pathname !== currentPath) link.dataset.simulatorNav = "";
          sections.append(link);
        }
        const detail = document.getElementById("organization-detail");
        if (detail) detail.before(sections);
        else document.querySelector(".main-content > .topbar").after(sections);
      }
      for (const link of sections.querySelectorAll("a")) {
        const url = new URL(link.href);
        if (url.pathname !== currentPath) link.dataset.simulatorNav = "";
        else delete link.dataset.simulatorNav;
        const section = ["#organization-policy", "#organization-services"].includes(location.hash)
          ? location.hash : "#organization-identity";
        const active = url.pathname === currentPath &&
          (currentPath !== "/organizations.html" || url.hash === section);
        if (active) link.setAttribute("aria-current", "page");
        else link.removeAttribute("aria-current");
      }
    }
    renderBanner();
    bannerPoller.start();
    void bannerPoller.refresh();
    mountOperatorLogin();
  };

  window.addEventListener("simulator:organization-context", (event) => {
    if (event.detail?.id === bannerOrganizationID) return;
    bannerOrganizationID = event.detail?.id;
    bannerPoller.stop();
    bannerSnapshot = null;
    bannerError = null;
    renderBanner();
    bannerPoller.start();
  });
  window.addEventListener("operator-session-ready", () => { void bannerPoller.refresh(); });
  window.addEventListener("operator-session-cleared", () => {
    bannerPoller.stop();
    bannerSnapshot = null;
    bannerError = null;
    renderBanner();
    bannerPoller.start();
  });

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

  function mountStyles(parsed) {
    for (const stylesheet of parsed.head.querySelectorAll('link[rel="stylesheet"]')) {
      const href = new URL(stylesheet.getAttribute("href"), location.href).href;
      if (![...document.head.querySelectorAll('link[rel="stylesheet"]')].some(link => link.href === href)) {
        const link = document.createElement("link");
        link.rel = "stylesheet";
        link.href = href;
        document.head.append(link);
      }
    }
  }

  async function loadScript(src) {
    const script = document.createElement("script");
    script.src = src;
    script.async = false;
    const loaded = new Promise((resolve, reject) => {
      script.addEventListener("load", resolve, { once: true });
      script.addEventListener("error", () => reject(new Error(`Could not load ${src}`)), { once: true });
    });
    document.body.append(script);
    await loaded;
  }

  async function mountOrganizationWorkspace(path) {
    if (!organizationPages.includes(path)) return;
    let parsed;
    if (!organizationWorkspace) {
      organizationWorkspace = document.getElementById("organization-workspace");
      if (!organizationWorkspace) {
        parsed = new DOMParser().parseFromString(await fetchPage("/organizations.html"), "text/html");
        organizationWorkspace = parsed.getElementById("organization-workspace");
        if (!organizationWorkspace) throw new Error("Organization workspace is unavailable.");
        mountStyles(parsed);
      }
    }
    const placeholder = document.getElementById("organization-workspace");
    if (placeholder && placeholder !== organizationWorkspace) placeholder.replaceWith(organizationWorkspace);
    else if (!document.body.contains(organizationWorkspace)) document.querySelector(".main-content > .topbar").after(organizationWorkspace);
    if (parsed) {
      const loadedScripts = new Set([...document.querySelectorAll("script[src]")].map(script => new URL(script.src).pathname));
      for (const script of parsed.querySelectorAll("script[src]")) {
        const src = script.getAttribute("src");
        const scriptPath = pageKey(src);
        if (!loadedScripts.has(scriptPath) || ["/editor-assistant.js", "/editor-search.js"].includes(scriptPath)) {
          await loadScript(src);
        }
      }
      organizationWorkspaceInitialized = true;
    }
  }

  const loadPage = async (url, push) => {
    await initialPageReady;
    const nextPath = pageKey(url);
    if (nextPath === currentPath) {
      if (push) history.pushState({}, "", url);
      syncNavigation();
      dispatchPageEvent("simulator:activate", currentPath);
      return;
    }

    let html;
    if (!pageViews.has(nextPath)) html = await fetchPage(url);
    dispatchPageEvent("simulator:deactivate", currentPath);
    organizationWorkspace?.remove();
    const previousView = document.createDocumentFragment();
    while (document.body.firstChild) previousView.append(document.body.firstChild);
    pageViews.set(currentPath, previousView);

    if (pageViews.has(nextPath)) {
      document.body.replaceChildren(pageViews.get(nextPath));
      document.title = pageTitles.get(nextPath) || document.title;
      await mountOrganizationWorkspace(nextPath);
    } else {
      const parsed = new DOMParser().parseFromString(html, "text/html");
      mountStyles(parsed);
      document.title = parsed.title;
      pageTitles.set(nextPath, parsed.title);
      const scripts = [...parsed.body.querySelectorAll("script[src]")];
      scripts.forEach((script) => script.remove());
      document.body.replaceChildren(...parsed.body.childNodes);
      await mountOrganizationWorkspace(nextPath);
      for (const oldScript of scripts) {
        const scriptPath = new URL(oldScript.src).pathname;
        if (scriptPath === "/simulator-nav.js" || organizationWorkspaceInitialized && organizationScripts.has(scriptPath)) continue;
        await loadScript(oldScript.src);
      }
      if (nextPath === "/organizations.html") organizationWorkspaceInitialized = true;
    }
    currentPath = nextPath;
    if (push) history.pushState({}, "", url);
    syncNavigation();
    dispatchPageEvent("simulator:activate", currentPath);
  };

  document.addEventListener("click", (event) => {
    const link = event.target.closest("a[data-simulator-nav]");
    if (!link || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    loadPage(link.href, true).catch(() => { window.location.href = link.href; });
  });
  window.addEventListener("popstate", () => loadPage(window.location.href, false));
  window.addEventListener("hashchange", syncNavigation);
  window.addEventListener("load", async () => {
    try {
      if (currentPath === "/organizations.html") organizationWorkspaceInitialized = true;
      await mountOrganizationWorkspace(currentPath);
      syncNavigation();
      pageCache.set(currentPath, Promise.resolve(document.documentElement.outerHTML));
      pageTitles.set(currentPath, document.title);
      document.querySelectorAll("a[data-simulator-nav]").forEach((link) => { fetchPage(link.href).catch(() => {}); });
      dispatchPageEvent("simulator:activate", currentPath);
    } catch (error) {
      const message = document.createElement("p");
      message.className = "scenario-error";
      message.setAttribute("role", "alert");
      message.textContent = `Organization navigation unavailable: ${error.message}`;
      document.querySelector(".main-content").prepend(message);
    } finally {
      resolveInitialPage();
    }
  }, { once: true });
})();
