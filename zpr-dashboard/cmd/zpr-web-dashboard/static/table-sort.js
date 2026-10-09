(() => {
  function compareValues(left, right, { numericStrings = false } = {}) {
    if (typeof left === "number" && typeof right === "number") return left - right;
    const a = String(left ?? "");
    const b = String(right ?? "");
    if (numericStrings && a && b) {
      const numericA = Number(a.replaceAll(",", ""));
      const numericB = Number(b.replaceAll(",", ""));
      if (Number.isFinite(numericA) && Number.isFinite(numericB)) return numericA - numericB;
    }
    return a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" });
  }

  function bindSortableHeaders({ table, getSort, onSort }) {
    function update() {
      const sort = getSort();
      for (const header of table.querySelectorAll("th[data-sort-key]")) {
        const button = header.querySelector(".sort-button");
        const active = sort?.key === header.dataset.sortKey;
        const direction = sort?.direction === 1 ? "ascending" : "descending";
        header.setAttribute("aria-sort", active ? direction : "none");
        button.setAttribute("aria-label", active
          ? `Sort by ${button.textContent}, currently ${direction}`
          : `Sort by ${button.textContent}, ascending`);
      }
    }

    for (const header of table.querySelectorAll("th[data-sort-key]")) {
      if (header.querySelector(".sort-button")) continue;
      const button = document.createElement("button");
      button.type = "button";
      button.className = "sort-button";
      button.textContent = header.textContent;
      header.classList.add("sortable-heading");
      header.replaceChildren(button);
      header.addEventListener("click", () => {
        const previous = getSort();
        onSort({
          key: header.dataset.sortKey,
          direction: previous?.key === header.dataset.sortKey ? previous.direction * -1 : 1,
        });
        update();
      });
    }
    update();
    return { update };
  }

  function renderEmptyRow(tbody, columnCount, message) {
    const row = document.createElement("tr");
    const cell = document.createElement("td");
    cell.colSpan = columnCount;
    cell.className = "empty-row";
    cell.textContent = message;
    row.append(cell);
    tbody.replaceChildren(row);
  }

  window.ZPRSortableTable = { compareValues, bindSortableHeaders, renderEmptyRow };
})();
