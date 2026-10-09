(() => {
  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>"']/g, (character) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
    })[character]);
  }

  function protocolName(value) {
    return ({ 1: "ICMP", 6: "TCP", 17: "UDP", 58: "ICMPv6" })[value] || "";
  }

  function formatDateTime(value) {
    return new Date(value).toLocaleString();
  }

  function formatTime(value) {
    return new Date(value).toLocaleTimeString();
  }

  function renderWindowControl(button, maximized, target) {
    const label = `${maximized ? "Restore" : "Maximize"} ${target}`;
    button.classList.add("window-control");
    button.setAttribute("aria-label", label);
    button.setAttribute("aria-pressed", String(maximized));
    button.title = label;
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    for (const [name, value] of Object.entries({
      viewBox: "0 0 24 24", width: "16", height: "16", fill: "none",
      stroke: "currentColor", "stroke-width": "1.5", "aria-hidden": "true", focusable: "false",
    })) svg.setAttribute(name, value);
    svg.dataset.windowControl = maximized ? "restore" : "maximize";
    const path = document.createElementNS(svg.namespaceURI, "path");
    path.setAttribute("d", maximized ? "M8 8V4h12v12h-4M4 8h12v12H4z" : "M4 4h16v16H4z");
    svg.append(path);
    button.replaceChildren(svg);
  }

  window.ZPRSafeDisplay = Object.freeze({ escapeHTML, protocolName, formatDateTime, formatTime, renderWindowControl });
})();
