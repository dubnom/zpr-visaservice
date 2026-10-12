(() => {
  "use strict";
  const freshness = 10000;
  const maxInterval = 60000;
  const names = ["inbound_packets_received", "outbound_packets_sent"];
  const uint64Max = (1n << 64n) - 1n;
  const rateFormat = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });
  const formatRate = value => rateFormat.format(value);
  const band = rate => rate === 0 ? 0 : rate < 100 ? 1 : rate < 1000 ? 2 : rate < 10000 ? 3 : 4;

  function readSample(actor, now) {
    const details = actor.node_details || {};
    if (details.counter_stats_error) return { error: details.counter_stats_error };
    const time = Date.parse(details.counters_updated_at);
    if (!Number.isFinite(time) || time > now) return { error: "Packet telemetry unavailable: invalid sample time." };
    if (now - time > freshness) return { error: "Packet telemetry stale.", stale: true };
    const values = new Map();
    for (const counter of details.counters || []) {
      if (!/^fastpath\.\d+$/.test(counter.group) || !names.includes(counter.name)) continue;
      const key = `${counter.group}.${counter.name}`;
      if (values.has(key) || typeof counter.value !== "string" || !/^\d{1,20}$/.test(counter.value)) {
        return { error: "Packet telemetry unavailable: invalid counter inventory." };
      }
      const value = BigInt(counter.value);
      if (value > uint64Max) return { error: "Packet telemetry unavailable: invalid counter value." };
      values.set(key, value);
    }
    const workers = new Set([...values.keys()].map(key => key.split(".")[1]));
    if (!workers.size || [...workers].some(worker => names.some(name => !values.has(`fastpath.${worker}.${name}`)))) {
      return { error: "Packet telemetry unavailable: incomplete RX/TX counters." };
    }
    return { time, values, identity: actor.zpr_addr || "" };
  }

  function createTracker() {
    const entries = new Map();
    let scope;
    function reset() { entries.clear(); }
    function update(actors, organization, connected, now = Date.now()) {
      if (organization !== scope) { reset(); scope = organization; }
      const present = new Set(actors.filter(actor => actor.node).map(actor => actor.cn));
      for (const key of entries.keys()) if (!present.has(key)) entries.delete(key);
      for (const actor of actors.filter(actor => actor.node)) {
        const sample = connected ? readSample(actor, now) : { error: "Packet telemetry unavailable: disconnected." };
        if (sample.error) {
          entries.set(actor.cn, { status: sample.stale ? "stale" : "unavailable", reason: sample.error, history: [] });
          continue;
        }
        const prior = entries.get(actor.cn);
        const previous = prior?.sample;
        if (previous && sample.time < previous.time) {
          entries.set(actor.cn, { status: "unavailable", reason: "Packet telemetry unavailable: sample time moved backwards.", history: [] });
          continue;
        }
        if (previous && sample.identity === previous.identity && sample.time === previous.time) {
          if ([...sample.values].every(([key, value]) => previous.values.get(key) === value) && sample.values.size === previous.values.size) continue;
          entries.set(actor.cn, { status: "unavailable", reason: "Packet telemetry unavailable: counters changed without a new timestamp.", history: [] });
          continue;
        }
        const interval = previous ? sample.time - previous.time : 0;
        const comparable = previous && sample.identity === previous.identity && interval > 0 && interval <= maxInterval &&
          sample.values.size === previous.values.size &&
          [...sample.values].every(([key, value]) => previous.values.has(key) && value >= previous.values.get(key));
        if (!comparable) {
          entries.set(actor.cn, { sample, status: "waiting", reason: "Waiting for two fresh packet samples (new node, reset, or changed inventory).", history: [] });
          continue;
        }
        let rx = 0n, tx = 0n;
        for (const [key, value] of sample.values) {
          const delta = value - previous.values.get(key);
          if (key.endsWith(names[0])) rx += delta;
          else tx += delta;
        }
        const seconds = interval / 1000;
        const receive = Number(rx) / seconds, transmit = Number(tx) / seconds;
        const total = receive + transmit;
        const alpha = 1 - Math.exp(-seconds / 5);
        const smoothed = total === 0 ? 0 : prior.smoothed == null ? total : prior.smoothed + alpha * (total - prior.smoothed);
        const history = [...prior.history, { time: sample.time, total }].slice(-12);
        entries.set(actor.cn, { sample, status: "ready", receive, transmit, total, smoothed, history });
      }
    }
    function get(cn, now = Date.now()) {
      const entry = entries.get(cn);
      if (!entry) return { status: "unavailable", reason: "Packet telemetry unavailable.", history: [] };
      if (entry.sample && now - entry.sample.time > freshness) {
        return { status: "stale", reason: "Packet telemetry stale.", history: [] };
      }
      return entry;
    }
    return { update, get, reset };
  }
  function describe(entry, now = Date.now()) {
    if (entry.status !== "ready") return entry.reason;
    return `Substrate packet I/O: RX ${formatRate(entry.receive)} packets/sec, TX ${formatRate(entry.transmit)} packets/sec; sample ${Math.max(0, Math.floor((now - entry.sample.time) / 1000))}s old. Includes control traffic; not unique application packets.`;
  }
  window.ZPRMapTraffic = { createTracker, describe, formatRate, band };
})();
