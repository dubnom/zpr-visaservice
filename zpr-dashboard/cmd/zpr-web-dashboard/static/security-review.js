(() => {
  const root = document.querySelector("[data-security-review]");
  if (!root) return;

  const baselineKey = "zpr.control-room.security-review.baseline.v1";
  const dismissalsKey = "zpr.control-room.security-review.dismissals.v1";
  const state = { baseline: null, findings: [], dismissed: {}, selected: new Set(), scannedAt: null, pending: false, snapshot: null, request: null };
  const byId = (id) => document.getElementById(id);
  const baselineLabel = byId("security-review-baseline");
  const baselineTime = byId("security-review-baseline-time");
  const statusLabel = byId("security-review-status");
  const rows = byId("security-review-findings");
  const countLabel = byId("security-review-count");
  const filter = byId("security-review-filter");
  const resetButton = byId("security-review-reset");
  const dismissSelectedButton = byId("security-review-dismiss-selected");
  const dismissAllButton = byId("security-review-dismiss-all");
  const showDismissedToggle = byId("security-review-show-dismissed");
  const selectVisibleToggle = byId("security-review-select-visible");
  const suspiciousLogLine = /\b(?:authentication failed|failed authentication|auth(?:entication)? failure|login failed|invalid certificate|certificate verify failed|unauthorized|access denied|permission denied|invalid token)\b/i;
  const active = () => location.hash === "#security-review";
  const acknowledgedAlerts = new Set();
  function alertKey(finding) {
    const latestDenial = finding.indicator === "Repeated policy denials"
      ? Math.max(0, ...(state.snapshot?.recent_denies || []).filter((deny) => deny.source_addr === finding.entity).map((deny) => Number(deny.last_deny_ms) || 0))
      : 0;
    return JSON.stringify([findingKey(finding), latestDenial]);
  }
  function acknowledgeAlerts() {
    for (const finding of state.findings) {
      if (finding.severity === "high") acknowledgedAlerts.add(alertKey(finding));
    }
  }

  function readBaseline() {
    try {
      const baseline = JSON.parse(localStorage.getItem(baselineKey) || "null");
      if (baseline && Array.isArray(baseline.actors) && Array.isArray(baseline.services)) return baseline;
    } catch {}
    return null;
  }

  function readDismissals() {
    try {
      const dismissed = JSON.parse(localStorage.getItem(dismissalsKey) || "{}");
      return dismissed && typeof dismissed === "object" && !Array.isArray(dismissed) ? dismissed : {};
    } catch { return {}; }
  }

  function findingKey(finding) {
    return JSON.stringify([finding.indicator, finding.entity, finding.evidence]);
  }

  function persistDismissals() {
    try {
      localStorage.setItem(dismissalsKey, JSON.stringify(state.dismissed));
      statusLabel.textContent = "Dismissal saved in this browser.";
    } catch {
      statusLabel.textContent = "Dismissal could not be saved in browser storage.";
    }
  }

  function serviceIdentity(service) {
    return JSON.stringify([service.service_name, service.actor_cn, service.service_kind, service.service_endpoints]);
  }

  function currentInventory(snapshot) {
    return {
      actors: [...new Set((snapshot.actors || []).map((actor) => actor.cn).filter(Boolean))].sort(),
      services: [...new Set((snapshot.services || []).map(serviceIdentity).filter(Boolean))].sort(),
    };
  }

  function baselineText() {
    const baseline = state.baseline;
    const current = state.snapshot ? currentInventory(state.snapshot) : null;
    const displayService = (identity) => {
      try {
        const [name, actor, kind, endpoint] = JSON.parse(identity);
        return [name, actor, kind, endpoint].filter(Boolean).join(" · ");
      } catch { return identity; }
    };
    const inventoryList = (id, values, saved, service = false) => {
      const cell = byId(id);
      cell.replaceChildren();
      if (!values) { cell.textContent = "—"; return; }
      const list = document.createElement("ul");
      const present = new Set(values);
      const savedValues = new Set(saved || []);
      for (const value of [...new Set([...values, ...savedValues])].sort()) {
        const item = document.createElement("li");
        const change = !present.has(value) ? "removed" : saved && !savedValues.has(value) ? "added" : "";
        item.textContent = `${service ? displayService(value) : value}${change ? ` (${change})` : ""}`;
        if (change) item.dataset.change = change;
        list.append(item);
      }
      if (!list.children.length) cell.textContent = "None";
      else cell.append(list);
    };
    inventoryList("security-review-baseline-actors", baseline?.actors);
    inventoryList("security-review-baseline-services", baseline?.services, null, true);
    inventoryList("security-review-current-actors", current?.actors, baseline?.actors);
    inventoryList("security-review-current-services", current?.services, baseline?.services, true);
    byId("security-review-current").textContent = current ? `${current.actors.length} actors · ${current.services.length} services` : "Waiting";
    byId("security-review-current-time").textContent = state.scannedAt ? new Date(state.scannedAt).toLocaleString() : "No snapshot";
    if (!state.baseline) {
      baselineLabel.textContent = "Not set";
      baselineTime.textContent = "No baseline";
      return;
    }
    const date = new Date(state.baseline.saved_at).toLocaleString();
    baselineLabel.textContent = `${state.baseline.actors.length} actors · ${state.baseline.services.length} services`;
    baselineTime.textContent = date;
  }

  function makeFinding(indicator, entity, evidence, severity = "info", observedAt = Date.now(), entityAddress = "") {
    return { indicator, entity, evidence, severity, observedAt, entityAddress };
  }

  function addressParts(text) {
    const parts = [];
    const addresses = /(?:\d{1,3}\.){3}\d{1,3}|(?:[0-9a-f]{0,4}:){2,}[0-9a-f:.]*/gi;
    let offset = 0;
    for (const match of String(text).matchAll(addresses)) {
      const address = match[0].replace(/\.+$/, "");
      try {
        const host = new URL(`http://${address.includes(":") ? `[${address}]` : address}/`).hostname;
        if (!host.startsWith("[") && !/^\d+\.\d+\.\d+\.\d+$/.test(host)) continue;
      } catch { continue; }
      parts.push({ text: text.slice(offset, match.index) });
      parts.push({ text: dnsAddressLabel(address, address), address });
      offset = match.index + address.length;
    }
    parts.push({ text: text.slice(offset) });
    return parts;
  }

  function entityParts(finding) {
    return finding.entityAddress
      ? [{ text: dnsAddressLabel(finding.entityAddress, finding.entity), address: finding.entityAddress }]
      : addressParts(finding.entity);
  }

  function appendParts(cell, parts) {
    for (const part of parts) {
      if (!part.address) {
        cell.append(document.createTextNode(part.text));
        continue;
      }
      const label = document.createElement("span");
      label.className = "security-address";
      label.textContent = part.text;
      label.title = dnsAddressTitle(part.address);
      cell.append(label);
    }
  }

  function denialFindings(snapshot) {
    const grouped = new Map();
    for (const deny of snapshot.recent_denies || []) {
      const source = deny.source_addr || "Unknown source";
      const group = grouped.get(source) || { hits: 0, records: [] };
      group.hits += Math.max(1, Number(deny.count) || 0);
      group.records.push(deny);
      grouped.set(source, group);
    }
    return [...grouped].map(([source, group]) => {
      const destinations = [...new Set(group.records.map((record) => record.dest_addr).filter(Boolean))];
      const reasons = [...new Set(group.records.map((record) => record.deny_code).filter(Boolean))];
      const latest = Math.max(...group.records.map((record) => Number(record.last_deny_ms) || 0));
      const repeated = group.hits >= 5 || group.records.length >= 3;
      const evidence = `${group.hits} denied requests${destinations.length ? ` · ${destinations.join(", ")}` : ""}${reasons.length ? ` · ${reasons.join(", ")}` : ""}`;
      const known = (snapshot.actors || []).some((actor) => actor.zpr_addr && dnsAddressKey(actor.zpr_addr) === dnsAddressKey(source));
      const varied = new Set(group.records.map((record) => JSON.stringify([record.dest_addr, record.protocol, record.dest_port, record.deny_code]))).size >= 3;
      const high = repeated && (!known || varied);
      return makeFinding(repeated ? "Repeated policy denials" : "Policy denial observed", source, evidence, high ? "high" : repeated ? "review" : "info", latest || Date.now());
    });
  }

  function trustedSourceFindings(snapshot) {
    return (snapshot.trusted_sources || [])
      .filter((source) => source.health === "failed")
      .map((source) => {
        const lastLookup = Number(source.last_lookup_ms) || 0;
        const evidence = [source.health_note || "The most recent attribute lookup failed.", lastLookup ? `Last lookup ${new Date(lastLookup).toLocaleString()}` : "Lookup time unavailable"]
          .filter(Boolean).join(" · ");
        return makeFinding("Trusted-source lookup failed", source.name || "Unknown source", evidence, "review", lastLookup || Date.now());
      });
  }

  function nodeHealthFindings(snapshot) {
    const nowSeconds = Date.now() / 1000;
    return (snapshot.actors || []).flatMap((actor) => {
      const details = actor.node_details;
      if (!actor.node || !details) return [];
      const lastContact = Number(details.last_contact) || 0;
      const contactAge = lastContact ? nowSeconds - lastContact : null;
      const stale = contactAge !== null && contactAge > 300;
      if (details.in_sync !== false && !stale) return [];
      const evidence = [
        details.in_sync === false ? "Node reports out of sync" : "Node contact is stale",
        lastContact ? `Last contact ${new Date(lastContact * 1000).toLocaleString()}` : "No contact time reported",
        details.pending_install > 0 ? `${details.pending_install} pending installs` : "",
        details.pending_revocation > 0 ? `${details.pending_revocation} pending revocations` : "",
      ].filter(Boolean).join(" · ");
      return [makeFinding(details.in_sync === false ? "Node out of sync" : "Node contact stale", actor.cn || "Unknown node", evidence, "review", Date.now(), actor.zpr_addr || "")];
    });
  }

  function inventoryFindings(snapshot) {
    if (!state.baseline) return [];
    const knownActors = new Set(state.baseline.actors);
    const knownServices = new Set(state.baseline.services);
    const findings = [];
    for (const actor of snapshot.actors || []) {
      if (!actor.cn || knownActors.has(actor.cn)) continue;
      knownActors.add(actor.cn);
      findings.push(makeFinding("Actor first observed since baseline", actor.cn, `${actor.node ? "Node" : "Adapter or machine actor"}${actor.zpr_addr ? ` · ${actor.zpr_addr}` : ""}`, "info", Date.now(), actor.zpr_addr || ""));
    }
    for (const service of snapshot.services || []) {
      const key = serviceIdentity(service);
      if (!service.service_name || knownServices.has(key)) continue;
      knownServices.add(key);
      findings.push(makeFinding("Service first observed since baseline", service.service_name, `${service.service_kind || "Unclassified service"}${service.actor_cn ? ` · actor ${service.actor_cn}` : ""}${service.zpr_addr ? ` · ${service.zpr_addr}` : ""}`, "review", Date.now(), service.zpr_addr || ""));
    }
    return findings;
  }

  function logFindings(logs) {
    const findings = [];
    for (const adapter of logs.adapters || []) {
      const adapterID = adapter.id || "Unknown adapter";
      for (const source of adapter.sources || []) {
        for (const line of source.lines || []) {
          if (!suspiciousLogLine.test(line)) continue;
          findings.push(makeFinding("Security-related log message", `${adapterID} · ${source.name}`, line.slice(0, 600), "review"));
        }
      }
    }
    return findings;
  }

  function render() {
    const currentAlerts = new Set(state.findings.filter((finding) => finding.severity === "high").map(alertKey));
    for (const key of acknowledgedAlerts) if (!currentAlerts.has(key)) acknowledgedAlerts.delete(key);
    const highCount = state.findings.filter((finding) => finding.severity === "high" && !state.dismissed[findingKey(finding)] && !acknowledgedAlerts.has(alertKey(finding))).length;
    const nav = document.querySelector('.primary-nav [data-page-link="security-review"]');
    nav.dataset.highAlert = String(highCount > 0);
    if (highCount) nav.setAttribute("aria-label", `Security: ${highCount} high alerts`);
    else nav.removeAttribute("aria-label");
    const query = filter.value.trim().toLowerCase();
    const matching = state.findings.filter((finding) => `${finding.indicator} ${finding.entity} ${finding.evidence} ${finding.entityAddress} ${entityParts(finding).map((part) => part.text).join("")} ${addressParts(finding.evidence).map((part) => part.text).join("")}`.toLowerCase().includes(query));
    const dismissedCount = matching.filter((finding) => state.dismissed[findingKey(finding)]).length;
    const activeFindings = matching.filter((finding) => !state.dismissed[findingKey(finding)]);
    const findings = showDismissedToggle.checked ? matching : activeFindings;
    const allActive = state.findings.filter((finding) => !state.dismissed[findingKey(finding)]);
    const activeKeys = new Set(allActive.map(findingKey));
    state.selected = new Set([...state.selected].filter((key) => activeKeys.has(key)));
    rows.replaceChildren();
    countLabel.textContent = `${activeFindings.length} active · ${dismissedCount} dismissed`;
    dismissSelectedButton.disabled = state.selected.size === 0;
    dismissAllButton.disabled = allActive.length === 0;
    selectVisibleToggle.checked = activeFindings.length > 0 && activeFindings.every((finding) => state.selected.has(findingKey(finding)));
    selectVisibleToggle.indeterminate = activeFindings.some((finding) => state.selected.has(findingKey(finding))) && !selectVisibleToggle.checked;
    selectVisibleToggle.disabled = activeFindings.length === 0;
    if (!findings.length) {
      const row = document.createElement("tr");
      const cell = document.createElement("td");
      cell.colSpan = 6;
      cell.className = "empty-row";
      cell.textContent = showDismissedToggle.checked && dismissedCount ? "No matching findings." : state.scannedAt ? "No active findings match this filter." : "Waiting for live signals.";
      row.append(cell);
      rows.append(row);
      window.sortControlRoomTableRows?.("security-review");
      return;
    }
    for (const finding of findings) {
      const key = findingKey(finding);
      const isDismissed = Boolean(state.dismissed[key]);
      const row = document.createElement("tr");
      row.dataset.severity = finding.severity;
      row.dataset.dismissed = String(isDismissed);
      const selection = document.createElement("td");
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.className = "security-review-select";
      checkbox.setAttribute("aria-label", `Select ${finding.indicator} for ${finding.entity}`);
      checkbox.checked = !isDismissed && state.selected.has(key);
      checkbox.disabled = isDismissed;
      checkbox.dataset.findingKey = key;
      selection.append(checkbox);
      const observed = document.createElement("td");
      observed.className = "security-observed";
      observed.dataset.sortCell = "observed";
      observed.textContent = new Date(finding.observedAt).toLocaleString();
      observed.dataset.sortValue = String(finding.observedAt);
      const indicator = document.createElement("td");
      indicator.className = "security-indicator";
      indicator.dataset.sortCell = "indicator";
      indicator.textContent = finding.indicator;
      const entity = document.createElement("td");
      entity.dataset.sortCell = "entity";
      appendParts(entity, entityParts(finding));
      const evidence = document.createElement("td");
      evidence.className = "security-evidence";
      evidence.dataset.sortCell = "evidence";
      appendParts(evidence, addressParts(finding.evidence));
      const disposition = document.createElement("td");
      disposition.className = "security-disposition";
      const action = document.createElement("button");
      action.type = "button";
      action.className = "button";
      action.textContent = isDismissed ? "Restore" : "Dismiss";
      action.setAttribute("aria-label", `${isDismissed ? "Restore" : "Dismiss"} ${finding.indicator} for ${finding.entity}`);
      action.dataset.securityDisposition = isDismissed ? "restore" : "dismiss";
      action.dataset.findingKey = key;
      disposition.append(action);
      row.append(selection, observed, indicator, entity, evidence, disposition);
      rows.append(row);
    }
    window.sortControlRoomTableRows?.("security-review");
  }

  async function readJSON(url, signal) {
    const response = await fetch(url, { cache: "no-store", signal, headers: { Accept: "application/json" } });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `${url} returned ${response.status}`);
    return data;
  }

  async function scan(snapshot) {
    state.snapshot = snapshot;
    if (state.pending) return;
    if (snapshot.api_status !== "connected") {
      statusLabel.textContent = "Live inventory unavailable; baseline unchanged";
      return;
    }
    state.pending = true;
    const request = new AbortController();
    state.request = request;
    statusLabel.textContent = "";
    try {
      const logsResult = active()
        ? await readJSON("/api/adapter-logs", request.signal).then((logs) => ({ logs }), (error) => ({ error }))
        : {};
      if (request.signal.aborted) return;
      snapshot = state.snapshot;
      if (snapshot.api_status !== "connected") {
        const details = (snapshot.errors || []).join(" · ");
        statusLabel.textContent = `Live inventory unavailable; baseline unchanged${details ? ` · ${details}` : ""}`;
        return;
      }
      state.baseline = readBaseline();
      let baselineCreated = false;
      if (!state.baseline && active()) {
        state.baseline = { ...currentInventory(snapshot), saved_at: new Date().toISOString() };
        try { localStorage.setItem(baselineKey, JSON.stringify(state.baseline)); }
        catch { statusLabel.textContent = "Scanned · browser storage unavailable for baseline."; }
        baselineCreated = true;
      }
      state.scannedAt = new Date();
      baselineText();
      state.findings = [...denialFindings(snapshot), ...trustedSourceFindings(snapshot), ...nodeHealthFindings(snapshot), ...inventoryFindings(snapshot), ...(logsResult.logs ? logFindings(logsResult.logs) : [])]
        .sort((left, right) => ({ high: 0, review: 1, info: 2 }[left.severity] - { high: 0, review: 1, info: 2 }[right.severity]) || right.observedAt - left.observedAt)
        .slice(0, 250);
      if (logsResult.error) statusLabel.textContent = `Scanned · adapter logs unavailable: ${logsResult.error.message}`;
      else if (!(snapshot.actors || []).length && !(snapshot.services || []).length) statusLabel.textContent = "Scanned · live inventory is unavailable.";
      else statusLabel.textContent = "";
      render();
    } catch (error) {
      if (active() && error.name !== "AbortError") statusLabel.textContent = error.message || "Security scan failed.";
    } finally {
      state.pending = false;
      if (state.request === request) state.request = null;
    }
  }

  document.addEventListener("control-room:refreshed", (event) => {
    state.snapshot = event.detail;
    void scan(event.detail);
  });
  window.addEventListener("hashchange", () => {
    if (!active()) state.request?.abort();
    else {
      acknowledgeAlerts();
      render();
    }
  });
  document.addEventListener("control-room:dns-updated", render);
  root.addEventListener("click", (event) => {
    const action = event.target.closest("[data-security-disposition]");
    if (!action) return;
    const key = action.dataset.findingKey;
    if (action.dataset.securityDisposition === "dismiss") state.dismissed[key] = Date.now();
    else delete state.dismissed[key];
    state.selected.delete(key);
    persistDismissals();
    render();
  });
  root.addEventListener("change", (event) => {
    const checkbox = event.target.closest(".security-review-select");
    if (checkbox) {
      if (checkbox.checked) state.selected.add(checkbox.dataset.findingKey);
      else state.selected.delete(checkbox.dataset.findingKey);
      render();
      return;
    }
    if (event.target === selectVisibleToggle) {
      const visible = state.findings.filter((finding) => !state.dismissed[findingKey(finding)] && `${finding.indicator} ${finding.entity} ${finding.evidence}`.toLowerCase().includes(filter.value.trim().toLowerCase()));
      for (const finding of visible) {
        const key = findingKey(finding);
        if (selectVisibleToggle.checked) state.selected.add(key);
        else state.selected.delete(key);
      }
      render();
    }
  });
  dismissSelectedButton.addEventListener("click", () => {
    for (const key of state.selected) state.dismissed[key] = Date.now();
    state.selected.clear();
    persistDismissals();
    render();
  });
  dismissAllButton.addEventListener("click", () => {
    for (const finding of state.findings) state.dismissed[findingKey(finding)] = Date.now();
    state.selected.clear();
    persistDismissals();
    render();
  });
  showDismissedToggle.addEventListener("change", render);
  resetButton.addEventListener("click", () => {
    try { localStorage.removeItem(baselineKey); } catch {}
    state.baseline = null;
    baselineText();
    statusLabel.textContent = "Baseline cleared.";
  });
  filter.addEventListener("input", render);
  state.dismissed = readDismissals();
  baselineText();
  const snapshot = controlRoomSnapshot();
  if (active() && snapshot) void scan(snapshot);
})();
