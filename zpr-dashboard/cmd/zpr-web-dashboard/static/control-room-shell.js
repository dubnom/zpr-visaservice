(() => {
  "use strict";
  // Gives standalone Control Room pages the same sidebar as the main app. The
  // navigation is read from index.html so the two can never drift apart.
  const current = window.location.pathname;
  const addStyle = (href) => {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    document.head.append(link);
  };

  async function sidebar() {
    const response = await fetch("/index.html", { credentials: "same-origin", cache: "no-cache" });
    if (!response.ok) throw new Error("navigation unavailable");
    const parsed = new DOMParser().parseFromString(await response.text(), "text/html");
    const aside = parsed.querySelector("aside.sidebar");
    if (!aside) throw new Error("navigation unavailable");
    for (const link of aside.querySelectorAll("a[href^='#']")) {
      link.setAttribute("href", "/" + link.getAttribute("href"));
      link.classList.remove("active");
      link.removeAttribute("data-page-link");
      link.removeAttribute("data-page-group");
    }
    for (const link of aside.querySelectorAll("a.nav-link")) {
      const target = new URL(link.getAttribute("href"), window.location.origin);
      const here = target.pathname === current && !target.hash;
      link.classList.toggle("active", here);
      if (here) link.setAttribute("aria-current", "page");
    }
    return document.importNode(aside, true);
  }

  async function install() {
    let aside;
    try {
      aside = await sidebar();
    } catch {
      return;
    }
    addStyle("/app.css");
    addStyle("/sidebar.css");
    addStyle("/control-room-typography.css");
    document.body.classList.add("control-room");
    const shell = document.createElement("div");
    shell.className = "app-shell";
    const main = document.createElement("div");
    main.className = "main-content";
    main.append(...document.body.childNodes);
    main.querySelector("header > a[href='/']")?.remove();
    shell.append(aside, main);
    document.body.append(shell);
    if (!window.__zprSidebar) {
      const script = document.createElement("script");
      script.src = "/sidebar.js?v=47";
      document.body.append(script);
    }
  }

  // Wait for DOMContentLoaded so every deferred page script has already run.
  if (document.readyState === "complete") install();
  else document.addEventListener("DOMContentLoaded", install);
})();
