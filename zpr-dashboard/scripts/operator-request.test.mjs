import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = readFileSync(new URL("../cmd/zpr-web-dashboard/static/operator-request.js", import.meta.url), "utf8");

test("operator requests retain CSRF across repeated Simulator script loads", async () => {
  const events = new EventTarget();
  const calls = [];
  const window = {
    addEventListener: events.addEventListener.bind(events),
  };
  const context = vm.createContext({
    window, URL, Headers,
    location: { origin: "https://localhost:28788", href: "https://localhost:28788/scenarios.html" },
    document: { querySelector: () => null },
    fetch: async (path, options) => {
      calls.push({ path, options });
      return { status: 202 };
    },
  });
  vm.runInContext(source, context);
  const session = new Event("operator-session-ready");
  session.detail = { csrf: "test-csrf" };
  events.dispatchEvent(session);
  events.dispatchEvent(new Event("operator-login-checked"));
  const fetcher = window.zprOperatorFetch;
  vm.runInContext(source, context);
  assert.equal(window.zprOperatorFetch, fetcher);
  await fetcher("/api/simulator/organizations/velocity/activate", { method: "POST" });
  await fetcher("/api/simulator/scenarios/velocity-single-node/run", { method: "POST" });
  await fetcher("/api/simulator/scenarios");
  await fetcher("https://example.test/api/anything", { method: "POST" });
  assert.equal(calls[0].options.headers.get("X-ZPR-CSRF"), "test-csrf");
  assert.equal(calls[1].options.headers.get("X-ZPR-CSRF"), "test-csrf");
  assert.equal(calls[2].options.headers.has("X-ZPR-CSRF"), false);
  assert.equal(calls[3].options.headers.has("X-ZPR-CSRF"), false);
  events.dispatchEvent(new Event("operator-session-cleared"));
  await fetcher("/api/simulator/scenarios/velocity-single-node/run", { method: "POST" });
  assert.equal(calls[4].options.headers.has("X-ZPR-CSRF"), false);
});
