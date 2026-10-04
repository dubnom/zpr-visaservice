(() => {
  if (window.__zprSidebar) return;
  window.__zprSidebar = true;
  let navigationObserver;
  let observedNavigation;

  const storageKey = () => document.querySelector(".sidebar .brand")?.textContent.includes("SIMULATOR")
    ? "zpr.simulator.sidebar-condensed" : "zpr.control-room.sidebar-condensed";

  function condensed() {
    try { return localStorage.getItem(storageKey()) === "true"; }
    catch { return document.body.classList.contains("sidebar-condensed"); }
  }

  function updateNavigationHints() {
    const sidebar = document.querySelector(".sidebar");
    if (!sidebar) return;
    for (const link of sidebar.querySelectorAll(".nav-link")) {
      if (!link.title) link.title = link.textContent.trim();
    }
  }

  function applyState(collapsed) {
    document.body.classList.toggle("sidebar-condensed", collapsed);
    const button = document.querySelector(".sidebar-toggle");
    if (!button) return;
    const name = collapsed ? "Expand side menu" : "Condense side menu";
    button.setAttribute("aria-expanded", String(!collapsed));
    button.setAttribute("aria-label", name);
    button.title = name;
    button.querySelector("span").textContent = collapsed ? "\u203a" : "\u2039";
    updateNavigationHints();
  }

  function initialize() {
    const sidebar = document.querySelector(".sidebar");
    if (!sidebar) return;
    if (!sidebar.querySelector(".sidebar-toggle")) {
      sidebar.id ||= "app-sidebar";
      const header = document.createElement("div");
      header.className = "sidebar-header";
      const button = document.createElement("button");
      button.type = "button";
      button.className = "sidebar-toggle";
      button.setAttribute("aria-controls", sidebar.id);
      const icon = document.createElement("span");
      icon.setAttribute("aria-hidden", "true");
      button.append(icon);
      header.append(button);
      sidebar.prepend(header);
    }
    const brand = sidebar.querySelector('.brand[aria-label="ZPR Control Room map"]');
    const header = sidebar.querySelector(".sidebar-header");
    if (brand) {
      sidebar.classList.add("sidebar-control-room");
      if (header && brand.parentElement !== header) header.prepend(brand);
    }
    const navigation = sidebar.querySelector(".primary-nav");
    if (navigation && observedNavigation !== navigation) {
      navigationObserver?.disconnect();
      observedNavigation = navigation;
      navigationObserver = new MutationObserver(updateNavigationHints);
      navigationObserver.observe(navigation, { childList: true, subtree: true, attributes: true, attributeFilter: ["class", "aria-current"] });
    }
    applyState(condensed());
  }

  document.addEventListener("click", (event) => {
    if (!event.target.closest(".sidebar-toggle")) return;
    const collapsed = !document.body.classList.contains("sidebar-condensed");
    try { localStorage.setItem(storageKey(), String(collapsed)); } catch {}
    applyState(collapsed);
  });
  window.addEventListener("storage", (event) => { if (event.key === storageKey()) initialize(); });
  document.addEventListener("simulator:activate", initialize);
  const bodyObserver = new MutationObserver(initialize);
  bodyObserver.observe(document.body, { childList: true });
  initialize();
})();