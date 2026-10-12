import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";

const window = {};
runInNewContext(readFileSync(new URL("../cmd/zpr-web-dashboard/static/map-traffic.js", import.meta.url), "utf8"), { window });
const { createTracker, band, describe } = window.ZPRMapTraffic;
const now = Date.parse("2026-10-12T15:00:00Z");
const actor = (time, rx, tx, workers = 1) => ({
  cn: "node-a", node: true, zpr_addr: "fd00::1",
  node_details: {
    counters_updated_at: new Date(time).toISOString(),
    counters: Array.from({ length: workers }, (_, index) => [
      { group: `fastpath.${index}`, name: "inbound_packets_received", value: String(rx) },
      { group: `fastpath.${index}`, name: "outbound_packets_sent", value: String(tx) },
      { group: `fastpath.${index}`, name: "outbound_packets_received", value: "999999" },
      { group: `fastpath.${index}`, name: "inbound_packets_sent", value: "999999" },
      { group: "management", name: "sent", value: "999999" },
    ]).flat(),
  },
});

test("rates sum worker deltas at substrate I/O, retaining uint64 precision", () => {
  const tracker = createTracker();
  const large = 18446744073709500000n;
  tracker.update([actor(now - 5000, large, large, 2)], "org", true, now);
  assert.equal(tracker.get("node-a", now).status, "waiting");
  tracker.update([actor(now, large + 500n, large + 250n, 2)], "org", true, now);
  const entry = tracker.get("node-a", now);
  assert.equal(entry.receive, 200);
  assert.equal(entry.transmit, 100);
  assert.equal(entry.total, 300);
  assert.equal(band(entry.smoothed), 2);
  assert.match(describe(entry, now), /Includes control traffic/);
});

test("bands have fixed exact thresholds and zero is distinct from unavailable", () => {
  for (const [value, expected] of [[0, 0], [0.1, 1], [99.9, 1], [100, 2], [999.9, 2], [1000, 3], [9999.9, 3], [10000, 4]]) {
    assert.equal(band(value), expected);
  }
  const tracker = createTracker();
  tracker.update([actor(now - 5000, 10, 20)], "", true, now);
  tracker.update([actor(now, 10, 20)], "", true, now);
  assert.equal(tracker.get("node-a", now).total, 0);
  assert.equal(tracker.get("node-a", now).status, "ready");
  assert.equal(tracker.get("node-a", now + 10001).status, "stale");
});

test("repeated samples do not invent zero traffic or add history", () => {
  const tracker = createTracker();
  tracker.update([actor(now - 5000, 0, 0)], "", true, now);
  tracker.update([actor(now, 500, 0)], "", true, now);
  tracker.update([actor(now, 500, 0)], "", true, now + 1000);
  assert.equal(tracker.get("node-a", now + 1000).total, 100);
  assert.equal(tracker.get("node-a", now + 1000).history.length, 1);
  tracker.update([actor(now, 501, 0)], "", true, now + 1000);
  assert.equal(tracker.get("node-a", now + 1000).status, "unavailable");
});

test("worker resets, inventory changes, identity changes, and long gaps rebaseline", () => {
  for (const change of ["reset", "inventory", "identity", "gap"]) {
    const tracker = createTracker();
    const start = change === "gap" ? now - 61000 : now - 5000;
    tracker.update([actor(start, 100, 200)], "", true, start);
    const next = actor(now, change === "reset" ? 99 : 500, 300, change === "inventory" ? 2 : 1);
    if (change === "identity") next.zpr_addr = "fd00::2";
    tracker.update([next], "", true, now);
    assert.equal(tracker.get("node-a", now).status, "waiting", change);
    assert.equal(tracker.get("node-a", now).history.length, 0);
  }
  const tracker = createTracker();
  const before = actor(now - 5000, 100, 0, 2), after = actor(now, 150, 0, 2);
  after.node_details.counters[0].value = "99";
  tracker.update([before], "", true, now);
  tracker.update([after], "", true, now);
  assert.equal(tracker.get("node-a", now).status, "waiting");
});

test("invalid telemetry is explicit, never success-shaped zero", () => {
  for (const change of ["missing", "duplicate", "negative", "overflow", "numeric", "future", "stale", "error"]) {
    const tracker = createTracker();
    const sample = actor(now, 0, 0);
    const counters = sample.node_details.counters;
    if (change === "missing") counters.splice(1, 1);
    if (change === "duplicate") counters.push(counters[0]);
    if (change === "negative") counters[0].value = "-1";
    if (change === "overflow") counters[0].value = "18446744073709551616";
    if (change === "numeric") counters[0].value = 0;
    if (change === "future") sample.node_details.counters_updated_at = new Date(now + 1).toISOString();
    if (change === "stale") sample.node_details.counters_updated_at = new Date(now - 10001).toISOString();
    if (change === "error") sample.node_details.counter_stats_error = "Exporter unavailable";
    tracker.update([sample], "", true, now);
    const entry = tracker.get("node-a", now);
    assert.equal(entry.status, change === "stale" ? "stale" : "unavailable", change);
    assert.equal(entry.total, undefined);
    assert.ok(entry.reason);
  }
});

test("outages, removed nodes, and organization changes discard previous sessions", () => {
  for (const change of ["disconnect", "remove", "organization"]) {
    const tracker = createTracker();
    tracker.update([actor(now - 5000, 0, 0)], "one", true, now);
    tracker.update([actor(now, 500, 0)], "one", true, now);
    if (change === "disconnect") tracker.update([actor(now, 500, 0)], "one", false, now);
    if (change === "remove") tracker.update([], "one", true, now);
    tracker.update([actor(now + 5000, 1000, 0)], change === "organization" ? "two" : "one", true, now + 5000);
    assert.equal(tracker.get("node-a", now + 5000).status, "waiting", change);
  }
});

test("backward sample timestamps are rejected", () => {
  const tracker = createTracker();
  tracker.update([actor(now, 100, 100)], "", true, now);
  tracker.update([actor(now - 1000, 90, 90)], "", true, now);
  assert.equal(tracker.get("node-a", now).status, "unavailable");
  assert.match(tracker.get("node-a", now).reason, /backwards/);
});

test("halo smoothing keeps latest numeric values and bounded history", () => {
  const tracker = createTracker();
  tracker.update([actor(now, 0, 0)], "", true, now);
  for (let index = 1; index <= 20; index++) {
    const time = now + index * 1000;
    tracker.update([actor(time, index === 1 ? 0 : (index - 1) * 1000, 0)], "", true, time);
    if (index === 2) {
      const entry = tracker.get("node-a", time);
      assert.equal(entry.total, 1000);
      assert.ok(entry.smoothed > 0 && entry.smoothed < 1000);
    }
  }
  assert.equal(tracker.get("node-a", now + 20000).history.length, 12);
});
