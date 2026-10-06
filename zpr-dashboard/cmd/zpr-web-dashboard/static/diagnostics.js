(() => {
  const page = document.querySelector("[data-diagnostics]");
  if (!page) return;
  const sourceList = document.getElementById("diagnostics-sources");
  const updated = document.getElementById("diagnostics-updated");
  const error = document.getElementById("diagnostics-error");
  const filter = document.getElementById("diagnostics-filter");
  const count = document.getElementById("diagnostics-count");
  let responseData = null;
  let pending = null;
  let timer = null;
  let active = location.hash === "#diagnostics";

  function appendText(parent, tag, className, text) {
    const child = document.createElement(tag);
    if (className) child.className = className;
    child.textContent = text;
    parent.append(child);
    return child;
  }

  function render() {
    sourceList.replaceChildren();
    const query = filter.value.trim().toLowerCase();
    const sources = (responseData?.sources || []).filter((source) => {
      const content = [source.name, source.kind, source.identity, source.address, ...(source.logs || []).map((log) => log.body), ...(source.metrics || []).map((metric) => `${metric.name} ${metric.value}`)].join(" ").toLowerCase();
      return !query || content.includes(query);
    });
    count.textContent = `${sources.length} / ${responseData?.sources?.length || 0} sources`;
    if (!sources.length) {
      appendText(sourceList, "p", "diagnostics-empty", responseData ? "No sources match this filter." : "No diagnostics data loaded.");
      return;
    }
    const fragment = document.createDocumentFragment();
    for (const source of sources) {
      const article = document.createElement("article");
      article.className = "diagnostics-source";
      article.dataset.state = source.state || "unavailable";
      const heading = document.createElement("header");
      heading.className = "diagnostics-source-heading";
      const identity = document.createElement("div");
      appendText(identity, "h3", "", source.name || source.id);
      appendText(identity, "p", "diagnostics-identity", `${source.kind} · ${source.identity}${source.address ? ` · ${source.address}` : ""}`);
      heading.append(identity);
      appendText(heading, "span", "diagnostics-state", source.state || "unavailable");
      article.append(heading);
      const lastUpdated = source.last_updated ? new Date(source.last_updated).toLocaleString() : "No telemetry received";
      appendText(article, "p", "diagnostics-last-update", `Last update · ${lastUpdated}`);
      if (source.error) appendText(article, "p", "diagnostics-source-error", source.error);
      if ((source.metrics || []).length) {
        const metrics = document.createElement("dl");
        metrics.className = "diagnostics-metrics";
        for (const metric of source.metrics) {
          appendText(metrics, "dt", "", metric.name);
          appendText(metrics, "dd", "", `${metric.value}${metric.unit ? ` ${metric.unit}` : ""}`);
        }
        article.append(metrics);
      } else {
        appendText(article, "p", "diagnostics-no-metrics", "No metrics in the current window.");
      }
      const logs = document.createElement("ol");
      logs.className = "diagnostics-logs";
      for (const log of source.logs || []) {
        const row = document.createElement("li");
        const stamp = log.timestamp ? new Date(log.timestamp).toLocaleTimeString() : "—";
        appendText(row, "time", "diagnostics-log-time", stamp);
        if (log.severity) appendText(row, "span", "diagnostics-log-severity", log.severity);
        appendText(row, "span", "diagnostics-log-body", log.body || "");
        logs.append(row);
      }
      if (!logs.children.length) appendText(logs, "li", "diagnostics-no-logs", "No logs in the current window.");
      article.append(logs);
      fragment.append(article);
    }
    sourceList.append(fragment);
  }

  async function load() {
    if (!active || pending) return;
    pending = new AbortController();
    const request = pending;
    updated.textContent = "Querying telemetry provider…";
    try {
      const response = await fetch("/api/diagnostics", { cache: "no-store", signal: request.signal, headers: { Accept: "application/json" } });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || `Diagnostics unavailable (${response.status})`);
      if (!active || pending !== request) return;
      responseData = result;
      error.hidden = true;
      error.textContent = "";
      updated.textContent = `Updated ${new Date(result.generated_at).toLocaleTimeString()} · ${result.state}`;
      render();
    } catch (failure) {
      if (failure.name !== "AbortError" && active) {
        error.textContent = failure.message || "Diagnostics unavailable.";
        error.hidden = false;
        updated.textContent = "Unavailable";
      }
    } finally {
      if (pending === request) pending = null;
    }
  }

  function setActive() {
    const next = location.hash === "#diagnostics";
    if (active && !next) {
      clearTimeout(timer);
      pending?.abort();
      pending = null;
    }
    active = next;
    if (active) void load();
  }

  filter.addEventListener("input", render);
  document.addEventListener("control-room:refreshed", () => { if (active) void load(); });
  window.addEventListener("hashchange", setActive);
  setActive();
})();
