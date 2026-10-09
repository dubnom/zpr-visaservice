(() => {
  const duration = 2400;
  const timers = new WeakMap();

  function pulse(element) {
    let target = element;
    if (element.matches("td, dd, th")) {
      target = element.querySelector(":scope > .poll-value");
      if (!target) {
        target = document.createElement("span");
        target.className = "poll-value";
        target.append(...element.childNodes);
        element.append(target);
      }
    }
    target.classList.add("poll-value");
    target.classList.remove("poll-changed");
    void target.offsetWidth;
    target.classList.add("poll-changed");
    clearTimeout(timers.get(target));
    timers.set(target, setTimeout(() => {
      target.classList.remove("poll-changed");
      timers.delete(target);
    }, duration));
  }

  function createTracker() {
    let previous = new Map();
    return {
      update(fields) {
        for (const [key, field] of fields) {
          if (previous.has(key) && previous.get(key) !== String(field.value)) pulse(field.node);
        }
        previous = new Map([...fields].map(([key, field]) => [key, String(field.value)]));
      },
      reset() { previous.clear(); },
    };
  }

  function markNumericColumns(table) {
    const numeric = [...table.tHead?.rows[0]?.cells || []].map((header) => header.dataset.numeric === "true");
    for (const body of table.tBodies) {
      for (const row of body.rows) {
        if (row.cells.length !== numeric.length) continue;
        [...row.cells].forEach((cell, index) => {
          if (numeric[index]) cell.dataset.numeric = "true";
        });
      }
    }
  }

  window.ZPRPollingDisplay = Object.freeze({ duration, pulse, createTracker, markNumericColumns });
})();
