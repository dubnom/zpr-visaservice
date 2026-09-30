(() => {
  if (window.__zprSimulatorNavigation) return;
  window.__zprSimulatorNavigation = true;
  const pageCache = new Map();
  const pageViews = new Map();
  const pageTitles = new Map();
  let currentPath = location.pathname;

  const pageKey = (url) => new URL(url, location.href).pathname;
  const dispatchPageEvent = (name, path) => document.dispatchEvent(new CustomEvent(name, { detail: { path } }));

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
    pageCache.set(currentPath, Promise.resolve(document.documentElement.outerHTML));
    pageTitles.set(currentPath, document.title);
    document.querySelectorAll("a[data-simulator-nav]").forEach((link) => { fetchPage(link.href).catch(() => {}); });
    dispatchPageEvent("simulator:activate", currentPath);
  }, { once: true });
})();
