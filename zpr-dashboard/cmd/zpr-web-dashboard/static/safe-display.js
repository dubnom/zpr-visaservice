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

  window.ZPRSafeDisplay = Object.freeze({ escapeHTML, protocolName, formatDateTime, formatTime });
})();
