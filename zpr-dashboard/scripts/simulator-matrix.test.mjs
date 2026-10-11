import test from "node:test";
import assert from "node:assert/strict";
import { runMatrix, parseOptions, classifyRun } from "./simulator-matrix.mjs";
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";

function fixture({ fail = {}, leak = false, busy = false, mutate = false, hold = false, onStart = () => {} } = {}) {
  let active = "original";
  let run = { state: busy ? "running" : "idle" };
  let generation = 0;
  const calls = [];
  const scenarios = id => [1, 2].map(number => {
    const scenario = { id: `scenario-${number}`, organization_id: id,
      steps: [{ action: "start_machine" }, { action: "request_test_service" }],
      cleanup: [{ action: "logout" }, { action: "stop_machine" }] };
    return { ...scenario, published_revision: 1, published_scenario: scenario };
  });
  const catalog = id => ({ active_organization_id: active, organization: { id },
    scenarios: scenarios(id), run: run.organization_id && run.organization_id !== id ? { state: "idle" } : run });
  const api = async (method, route) => {
    calls.push([method, route]);
    if (route === "/api/simulator/organizations") return { active_id: active,
      organizations: ["original", "alpha", "beta"].map(id => ({ id })),
      activation: { organization_id: active, state: "completed" } };
    if (route === "/api/simulator/status") return {
      manifest: { organization_id: active, machines: [{ id: "machine" }] }, organization: { id: active },
      sessions: { machine: { authenticated: leak && run.state === "failed", workloads: [] } },
      machine_containers: { machine: "stopped" },
    };
    if (route.startsWith("/api/simulator/organizations/") && method === "POST") {
      active = route.split("/")[4];
      run = { state: "idle" };
      return { activation: { organization_id: active, state: "resetting" } };
    }
    if (route.startsWith("/api/simulator/scenarios?")) return catalog(new URLSearchParams(route.split("?")[1]).get("organization_id"));
    if (route === "/api/simulator/scenarios" && method === "GET") {
      if (mutate && run.state === "running") return { ...catalog(active), run: { ...run, started_at: "foreign" } };
      return catalog(active);
    }
    if (route === "/api/simulator/scenarios/cancel") {
      run.state = "cancelled";
      return run;
    }
    if (method === "POST" && route.endsWith("/run")) {
      const id = route.split("/")[4];
      const entry = scenarios(active).find(entry => entry.id === id);
      const failingAction = fail[active + "/" + id];
      const definitions = [...entry.steps.map(step => ({ ...step, phase: "run" })),
        ...entry.cleanup.map(step => ({ ...step, phase: "cleanup" }))];
      run = {
        scenario_id: id, organization_id: active, workspace_revision: 1, started_at: "run-" + ++generation,
        finished_at: "finished", total_steps: definitions.length, state: hold || mutate ? "running" : failingAction ? "failed" : "completed",
        steps: definitions.map((step, index) => ({ ...step, number: index + 1,
          status: step.action === failingAction ? "failed" : "completed" })),
      };
      onStart();
      return run;
    }
    throw Error("Unexpected API call: " + method + " " + route);
  };
  return { api, calls, active: () => active };
}

const options = fixture => ({ api: fixture.api, execute: true, exclusiveRuntime: true,
  organizations: ["alpha", "beta"], pollMs: 1, activationMs: 20, scenarioMs: 20, cleanupMs: 20, log: () => {} });
const starts = fixture => fixture.calls.filter(([method, route]) => method === "POST" && route.endsWith("/run"));

test("plan discovers every organization/revision without POSTs", async () => {
  const f = fixture();
  const report = await runMatrix({ api: f.api, log: () => {} });
  assert.equal(report.organizations.length, 3);
  assert.equal(report.organizations.flatMap(org => org.scenarios).length, 6);
  assert.equal(report.results.length, 0);
  assert.equal(report.state, "planned");
  assert.equal(f.calls.some(([method]) => method !== "GET"), false);
});

test("executes every scenario serially and restores original selection", async () => {
  const f = fixture();
  const saved = [];
  const report = await runMatrix({ ...options(f), save: report => saved.push(JSON.stringify(report)) });
  assert.equal(report.state, "passed");
  assert.equal(starts(f).length, 4);
  assert.deepEqual(report.results.map(result => result.organization), ["alpha", "alpha", "beta", "beta"]);
  assert.equal(f.active(), "original");
  assert.equal(report.restoration, "restored-selection");
  assert.ok(saved.length > 8);
  assert.equal(JSON.stringify(report).includes("output"), false);
});

test("first traffic failure stops the matrix after cleanup and restores selection", async () => {
  const f = fixture({ fail: { "alpha/scenario-1": "request_test_service" } });
  const report = await runMatrix(options(f));
  assert.equal(report.state, "failed");
  assert.equal(starts(f).length, 1);
  assert.equal(report.results[0].state, "traffic-failure");
  assert.match(report.error, /Traffic scenario failure.*first error/);
  assert.equal(report.restoration, "restored-selection");
});

test("first traffic failure skips remaining scenarios and organizations", async () => {
  const f = fixture({ fail: { "alpha/scenario-1": "request_test_service", "alpha/scenario-2": "request_test_service" } });
  const report = await runMatrix(options(f));
  assert.equal(starts(f).length, 1);
  assert.equal(report.results.length, 1);
  assert.equal(f.calls.some(([method, route]) => method === "POST" && route === "/api/simulator/organizations/beta/activate"), false);
  assert.match(report.error, /first error/);
});

test("infrastructure failure stops immediately", async () => {
  const f = fixture({ fail: { "alpha/scenario-1": "start_machine" } });
  const report = await runMatrix(options(f));
  assert.equal(starts(f).length, 1);
  assert.match(report.error, /Infrastructure/);
  assert.equal(report.restoration, "restored-selection");
});

test("cleanup failure stops immediately", async () => {
  const f = fixture({ fail: { "alpha/scenario-1": "logout" } });
  const report = await runMatrix(options(f));
  assert.equal(starts(f).length, 1);
  assert.equal(report.results[0].state, "serious");
});

test("leaked sessions stop execution and prevent destructive restoration", async () => {
  const f = fixture({ leak: true, fail: { "alpha/scenario-1": "request_test_service" } });
  const report = await runMatrix(options(f));
  assert.equal(starts(f).length, 1);
  assert.match(report.error, /Session/);
  assert.equal(report.restoration, "failed");
  assert.equal(f.active(), "alpha");
});

test("preexisting scenario is never cancelled or mutated", async () => {
  const f = fixture({ busy: true });
  const report = await runMatrix(options(f));
  assert.equal(report.state, "failed");
  assert.equal(f.calls.some(([method]) => method === "POST"), false);
});

test("foreign run replacement is never cancelled or restored over", async () => {
  const f = fixture({ mutate: true });
  const report = await runMatrix(options(f));
  assert.equal(starts(f).length, 1);
  assert.equal(report.restoration, "failed");
  assert.equal(f.calls.some(([, route]) => route.endsWith("/cancel")), false);
});

test("timeout cancels only its own run then restores selection", async () => {
  const f = fixture({ hold: true });
  const report = await runMatrix(options(f));
  assert.match(report.error, /timed out/);
  assert.equal(f.calls.filter(([, route]) => route.endsWith("/cancel")).length, 1);
  assert.equal(report.restoration, "restored-selection");
});

test("signal interruption still permits bounded cancellation and restoration", async () => {
  const controller = new AbortController();
  const f = fixture({ hold: true, onStart: () => controller.abort() });
  const report = await runMatrix({ ...options(f), signal: controller.signal });
  assert.match(report.error, /Interrupted/);
  assert.equal(f.calls.filter(([, route]) => route.endsWith("/cancel")).length, 1);
  assert.equal(report.restoration, "restored-selection");
});

test("unpublished and changed scenarios stop without running them", async () => {
  const f = fixture();
  const api = async (method, route) => {
    const value = await f.api(method, route);
    if (route.includes("organization_id=alpha")) value.scenarios[0].published_revision = 0;
    return value;
  };
  const report = await runMatrix({ ...options(f), api });
  assert.match(report.error, /published/);
  assert.equal(f.calls.some(([method]) => method === "POST"), false);
});

test("published revision changes between discovery and execution stop before POST", async () => {
  const f = fixture();
  const api = async (method, route) => {
    const value = await f.api(method, route);
    if (route === "/api/simulator/scenarios" && f.active() === "alpha") value.scenarios[0].published_revision = 2;
    return value;
  };
  const report = await runMatrix({ ...options(f), api });
  assert.match(report.error, /Published scenario changed/);
  assert.equal(starts(f).length, 0);
});

test("production empty-session representation permits null workloads", async () => {
  const f = fixture();
  const api = async (method, route) => {
    const value = await f.api(method, route);
    if (route.endsWith("/status")) value.sessions.machine.workloads = null;
    return value;
  };
  assert.equal((await runMatrix({ ...options(f), api })).state, "passed");
});

test("failed activation stops without starting a scenario", async () => {
  const f = fixture();
  let requested = false;
  const api = async (method, route) => {
    if (method === "POST" && route.endsWith("/activate")) requested = true;
    const value = await f.api(method, route);
    if (requested && route.endsWith("/organizations")) value.activation.state = "failed";
    return value;
  };
  const report = await runMatrix({ ...options(f), api });
  assert.match(report.error, /activation failed/);
  assert.equal(starts(f).length, 0);
  assert.equal(report.state, "failed");
});

test("ambiguous failed start acknowledgement never cancels an unowned run", async () => {
  const f = fixture({ hold: true });
  const api = async (method, route) => {
    const value = await f.api(method, route);
    if (method === "POST" && route.endsWith("/run")) throw Error("Connection lost after start");
    return value;
  };
  const report = await runMatrix({ ...options(f), api });
  assert.equal(report.state, "failed");
  assert.equal(report.results[0].state, "starting");
  assert.equal(report.restoration, "failed");
  assert.equal(f.calls.some(([, route]) => route.endsWith("/cancel")), false);
});

test("cleanup timeout cannot reset over a still-running scenario", async () => {
  const f = fixture({ hold: true });
  const api = async (method, route) => route.endsWith("/cancel")
    ? { state: "cleaning" } : f.api(method, route);
  const report = await runMatrix({ ...options(f), api });
  assert.equal(report.state, "failed");
  assert.match(report.recovery_error, /timed out/);
  assert.equal(report.restoration, "failed");
  assert.equal(f.active(), "alpha");
});

test("unknown runtime container state prevents activation", async () => {
  const f = fixture();
  const api = async (method, route) => {
    const value = await f.api(method, route);
    if (route.endsWith("/status")) value.machine_containers.machine = "unknown";
    return value;
  };
  const report = await runMatrix({ ...options(f), api });
  assert.match(report.error, /not stopped/);
  assert.equal(f.calls.some(([method]) => method === "POST"), false);
});

test("completed state without complete step evidence is not success", () => {
  assert.throws(() => classifyRun({ state: "completed", total_steps: 1, steps: [] },
    { steps: [{ action: "request_test_service" }] }), /missing/);
});

test("execution and URL safety gates are explicit", () => {
  assert.throws(() => parseOptions(["--execute"]), /requires/);
  assert.throws(() => parseOptions(["--url", "http://user:password@localhost:8788"]), /credentials/);
  assert.throws(() => parseOptions(["--scenario-timeout-seconds", "0"]), /positive/);
  assert.throws(() => parseOptions(["--unknown"]), /Unknown/);
  assert.equal(parseOptions([]).execute, false);
});

test("CLI plans over real HTTP, persists safe report, and refuses an existing execution lock", async () => {
  const f = fixture();
  const server = createServer(async (request, response) => {
    try {
      response.setHeader("Content-Type", "application/json");
      response.end(JSON.stringify(await f.api(request.method, request.url)));
    } catch (error) {
      response.statusCode = 500;
      response.end(JSON.stringify({ error: error.message }));
    }
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "zpr-matrix-test-"));
  const origin = "http://127.0.0.1:" + server.address().port;
  const lock = path.join(os.tmpdir(), "zpr-simulator-matrix-" + crypto.createHash("sha256").update(origin).digest("hex") + ".lock");
  const script = fileURLToPath(new URL("./simulator-matrix.mjs", import.meta.url));
  const execute = promisify(execFile);
  let lockCreated = false;
  try {
    const report = path.join(directory, "plan.json");
    const result = await execute(process.execPath, [script, "--url", origin, "--report", report]);
    assert.match(result.stdout, /Matrix planned/);
    assert.equal(JSON.parse(await fs.readFile(report)).organizations.length, 3);
    assert.equal((await fs.stat(report)).mode & 0o777, 0o600);
    await fs.writeFile(lock, "existing owner", { flag: "wx" });
    lockCreated = true;
    await assert.rejects(execute(process.execPath, [script, "--url", origin, "--execute",
      "--exclusive-runtime", "--report", path.join(directory, "execution.json")]));
    assert.equal(await fs.readFile(lock, "utf8"), "existing owner");
    assert.equal(f.calls.some(([method]) => method === "POST"), false);
  } finally {
    if (lockCreated) await fs.unlink(lock);
    await fs.rm(directory, { recursive: true });
    await new Promise(resolve => server.close(resolve));
  }
});

test("CLI executes with exact Origin, excludes credentials from report and returns failure exit status", async () => {
  const f = fixture({ fail: { "alpha/scenario-1": "request_test_service" } });
  const origins = [];
  const server = createServer(async (request, response) => {
    origins.push(request.headers.origin);
    assert.equal(request.headers.cookie, "fixture-session");
    assert.equal(request.headers["x-zpr-csrf"], "fixture-proof");
    response.setHeader("Content-Type", "application/json");
    response.end(JSON.stringify(await f.api(request.method, request.url)));
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "zpr-matrix-http-test-"));
  try {
    const origin = "http://127.0.0.1:" + server.address().port;
    const headers = path.join(directory, "headers.json");
    const report = path.join(directory, "report.json");
    await fs.writeFile(headers, JSON.stringify({ Cookie: "fixture-session", "X-ZPR-CSRF": "fixture-proof" }));
    await assert.rejects(promisify(execFile)(process.execPath, [fileURLToPath(new URL("./simulator-matrix.mjs", import.meta.url)),
      "--url", origin, "--execute", "--exclusive-runtime", "--organizations", "alpha,beta",
      "--report", report, "--headers-file", headers]), error => error.code === 1);
    assert.ok(origins.includes(origin));
    const text = await fs.readFile(report, "utf8");
    const result = JSON.parse(text);
    assert.equal(result.state, "failed");
    assert.equal(result.results.length, 1);
    assert.equal(starts(f).length, 1);
    assert.match(result.error, /first error/);
    assert.equal(result.restoration, "restored-selection");
    assert.equal(text.includes("fixture-session") || text.includes("fixture-proof"), false);
  } finally {
    await fs.rm(directory, { recursive: true });
    await new Promise(resolve => server.close(resolve));
  }
});
