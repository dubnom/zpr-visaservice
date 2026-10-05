(() => {
  const icons = {
    policy: {
      name: "traffic-light",
      path: "M12,9A2,2 0 0,1 10,7C10,5.89 10.9,5 12,5C13.11,5 14,5.89 14,7A2,2 0 0,1 12,9M12,14A2,2 0 0,1 10,12C10,10.89 10.9,10 12,10C13.11,10 14,10.89 14,12A2,2 0 0,1 12,14M12,19A2,2 0 0,1 10,17C10,15.89 10.9,15 12,15C13.11,15 14,15.89 14,17A2,2 0 0,1 12,19M20,10H17V8.86C18.72,8.41 20,6.86 20,5H17V4A1,1 0 0,0 16,3H8A1,1 0 0,0 7,4V5H4C4,6.86 5.28,8.41 7,8.86V10H4C4,11.86 5.28,13.41 7,13.86V15H4C4,16.86 5.28,18.41 7,18.86V20A1,1 0 0,0 8,21H16A1,1 0 0,0 17,20V18.86C18.72,18.41 20,16.86 20,15H17V13.86C18.72,13.41 20,11.86 20,10Z",
    },
    assertions: {
      name: "database-check",
      path: "M12 3C16.42 3 20 4.79 20 7S16.42 11 12 11 4 9.21 4 7 7.58 3 12 3M4 9C4 11.21 7.58 13 12 13S20 11.21 20 9V12.08L19 12C16.41 12 14.2 13.64 13.36 15.94L12 16C7.58 16 4 14.21 4 12V9M4 14C4 16.21 7.58 18 12 18H13C13 19.05 13.27 20.04 13.75 20.9L12 21C7.58 21 4 19.21 4 17V14M18 21.08L15.25 18.08L16.41 16.92L18 18.5L21.59 14.92L22.75 16.33L18 21.08",
    },
  };

  window.zprPolicyRecordIcon = (kind) => {
    const definition = icons[kind];
    if (!definition) return null;
    const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    icon.classList.add("policy-kind-icon");
    icon.dataset.icon = definition.name;
    icon.setAttribute("viewBox", "0 0 24 24");
    icon.setAttribute("aria-hidden", "true");
    icon.setAttribute("focusable", "false");
    const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    path.setAttribute("d", definition.path);
    icon.append(path);
    return icon;
  };
})();
