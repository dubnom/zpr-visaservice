import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { pathToFileURL } from "node:url";

const busy = run => ["running", "cleaning"].includes(run?.state);
const requireThat = (condition, message) => { if (!condition) throw new Error(message); };
const trafficActions = new Set(["request_test_service", "request_web_gateway", "benchmark_test_service", "stress_traffic", "traffic"]);
const delay = (ms, signal) => new Promise((resolve, reject) => {
  if (signal?.aborted) return reject(new Error("Interrupted"));
  const abort = () => { clearTimeout(timer); reject(new Error("Interrupted")); };
  const timer = setTimeout(() => { signal?.removeEventListener("abort", abort); resolve(); }, ms);
  signal?.addEventListener("abort", abort, { once: true });
});

export function assertClean(status, organization) {
  requireThat(status?.manifest?.organization_id === organization && status?.organization?.id === organization,
    "Runtime organization does not match the selected organization");
  requireThat(status.sessions && status.machine_containers && Array.isArray(status.manifest.machines),
    "Runtime status lacks cleanup evidence");
  for (const machine of status.manifest.machines) {
    const session = status.sessions[machine.id];
    requireThat(session && session.authenticated === false &&
      (session.workloads === null || Array.isArray(session.workloads) && session.workloads.length === 0),
      `Session/workloads remain on ${machine.id}`);
    requireThat(["stopped", "exited", "missing"].includes(status.machine_containers[machine.id]),
      `Machine ${machine.id} is not stopped`);
  }
}

export function classifyRun(run, scenario) {
  requireThat(Array.isArray(run.steps) && run.total_steps === scenario.steps.length + (scenario.cleanup || []).length,
    "Scenario result has invalid step evidence");
  const definitions = [...scenario.steps.map(step => ({ ...step, phase: "run" })),
    ...(scenario.cleanup || []).map(step => ({ ...step, phase: "cleanup" }))];
  const seen = new Set();
  for (const result of run.steps) {
    const expected = definitions[result.number - 1];
    requireThat(Number.isInteger(result.number) && expected && !seen.has(result.number) &&
      expected.action === result.action && expected.phase === result.phase &&
      (expected.machine || "") === (result.machine || "") && (expected.component || "") === (result.component || "") &&
      ["completed", "failed"].includes(result.status), "Scenario result does not match its published steps");
    seen.add(result.number);
  }
  const failures = run.steps.filter(step => step.status === "failed");
  if (run.state === "completed") {
    requireThat(!run.error && !failures.length && seen.size === definitions.length, "Completed run has missing or failed steps");
    return "passed";
  }
  requireThat(["failed", "cancelled"].includes(run.state), "Unexpected terminal scenario state");
  const cleanupCount = run.steps.filter(step => step.phase === "cleanup" && step.status === "completed").length;
  if (run.state === "cancelled" || cleanupCount !== (scenario.cleanup || []).length || !failures.length ||
      failures.some(step => step.phase === "cleanup" || !trafficActions.has(step.action))) return "serious";
  return "traffic-failure";
}

export async function runMatrix({ api, execute = false, exclusiveRuntime = false, organizations,
  pollMs = 2000, activationMs = 20 * 60_000, scenarioMs = 2 * 60 * 60_000,
  cleanupMs = 5 * 60_000, signal, save = () => {}, log = console.log }) {
  requireThat(!execute || exclusiveRuntime, "Execution requires exclusive-runtime approval");
  const report = { version: 1, mode: execute ? "execute" : "plan", started_at: new Date().toISOString(),
    state: "running", original_organization: null, organizations: [], results: [], restoration: "not-needed" };
  let owned;
  let changed = false;
  let original;
  const persist = () => save(report);
  const getCatalog = async () => {
    const catalog = await api("GET", "/api/simulator/scenarios");
    requireThat(catalog.organization?.id && catalog.active_organization_id === catalog.organization.id && catalog.run,
      "Scenario catalog is not for the active organization");
    return catalog;
  };
  const waitFor = async (read, done, timeout, interrupt = signal) => {
    const deadline = Date.now() + timeout;
    while (true) {
      if (interrupt?.aborted) throw new Error("Interrupted");
      const value = await read();
      if (done(value)) return value;
      requireThat(Date.now() < deadline, "Operation timed out");
      await delay(pollMs, interrupt);
    }
  };
  const activate = async (id, interrupt = signal) => {
    const before = await api("GET", "/api/simulator/organizations");
    requireThat(before.activation?.state !== "resetting", "Another organization activation is in progress");
    requireThat(!busy((await getCatalog()).run), "Another scenario is active");
    assertClean(await api("GET", "/api/simulator/status"), before.active_id);
    changed = true;
    persist();
    const accepted = await api("POST", `/api/simulator/organizations/${encodeURIComponent(id)}/activate`);
    requireThat(accepted.activation?.organization_id === id, "Activation acknowledgement changed organization");
    await waitFor(() => api("GET", "/api/simulator/organizations"), catalog => {
      requireThat(catalog.activation?.organization_id === id, "Concurrent organization activation detected");
      requireThat(catalog.activation.state !== "failed", "Organization activation failed");
      return catalog.activation.state === "completed" && catalog.active_id === id;
    }, activationMs, interrupt);
    const catalog = await getCatalog();
    requireThat(catalog.organization.id === id && !busy(catalog.run), "Activated runtime is inconsistent or busy");
    assertClean(await api("GET", "/api/simulator/status"), id);
  };
  try {
    const catalog = await api("GET", "/api/simulator/organizations");
    requireThat(Array.isArray(catalog.organizations) && catalog.organizations.length && catalog.active_id,
      "Organization catalog is empty or invalid");
    original = catalog.active_id;
    report.original_organization = original;
    const available = catalog.organizations.map(org => org.id);
    requireThat(available.every(id => typeof id === "string" && id.length) && new Set(available).size === available.length,
      "Organization catalog has invalid or duplicate IDs");
    const ids = organizations || [...available].sort();
    requireThat(ids.length && new Set(ids).size === ids.length && ids.every(id => available.includes(id)),
      "Requested organizations are missing or duplicated");
    requireThat(catalog.activation?.state !== "resetting" && !busy((await getCatalog()).run),
      "Simulator is already busy; no runtime changes made");
    if (execute) assertClean(await api("GET", "/api/simulator/status"), original);
    for (const id of ids) {
      if (signal?.aborted) throw new Error("Interrupted");
      const planned = await api("GET", `/api/simulator/scenarios?organization_id=${encodeURIComponent(id)}`);
      requireThat(planned.organization?.id === id && Array.isArray(planned.scenarios), "Invalid organization scenario catalog");
      const entries = [...planned.scenarios].sort((a, b) => a.id.localeCompare(b.id));
      requireThat(new Set(entries.map(entry => entry.id)).size === entries.length, "Duplicate scenario IDs");
      for (const entry of entries) {
        requireThat(entry.organization_id === id && entry.id && Number.isInteger(entry.published_revision) &&
          entry.published_revision > 0 && entry.published_scenario?.id === entry.id &&
          entry.published_scenario.organization_id === id && Array.isArray(entry.published_scenario.steps) &&
          entry.published_scenario.steps.length > 0 &&
          (entry.published_scenario.cleanup == null || Array.isArray(entry.published_scenario.cleanup)),
          "Every scenario must have a published, organization-scoped revision");
      }
      report.organizations.push({ id, scenarios: entries.map(entry => ({ id: entry.id, revision: entry.published_revision })) });
      persist();
      log(`${execute ? "Testing" : "Planning"} ${id}: ${entries.length} published scenarios`);
      if (!execute) continue;
      await activate(id);
      for (const entry of entries) {
        const current = await getCatalog();
        requireThat(current.organization.id === id && !busy(current.run), "Concurrent runtime change detected");
        const live = current.scenarios.find(item => item.id === entry.id);
        requireThat(live?.published_revision === entry.published_revision &&
          JSON.stringify(live.published_scenario) === JSON.stringify(entry.published_scenario), "Published scenario changed during matrix");
        assertClean(await api("GET", "/api/simulator/status"), id);
        if (signal?.aborted) throw new Error("Interrupted");
        const result = { organization: id, scenario: entry.id, revision: entry.published_revision, state: "starting" };
        report.results.push(result);
        persist();
        const run = await api("POST", `/api/simulator/scenarios/${encodeURIComponent(entry.id)}/run`);
        requireThat(run.organization_id === id && run.scenario_id === entry.id &&
          run.workspace_revision === entry.published_revision && run.started_at, "Scenario start acknowledgement mismatch");
        owned = { organization: id, scenario: entry.id, started: run.started_at };
        const matches = candidate => candidate.organization_id === owned.organization &&
          candidate.scenario_id === owned.scenario && candidate.started_at === owned.started;
        result.state = "running";
        result.started_at = run.started_at;
        persist();
        const finished = await waitFor(getCatalog, catalog => {
          requireThat(catalog.organization.id === id && matches(catalog.run), "Scenario run replaced by another operator");
          result.current_step = catalog.run.current_step;
          result.runtime_state = catalog.run.state;
          persist();
          return !busy(catalog.run);
        }, scenarioMs);
        owned = undefined;
        requireThat(finished.run.workspace_revision === entry.published_revision && finished.run.finished_at,
          "Terminal run lost its revision or completion timestamp");
        result.state = classifyRun(finished.run, entry.published_scenario);
        result.steps = finished.run.steps.map(step => ({
          number: step.number, phase: step.phase, action: step.action, status: step.status,
        }));
        result.started_at = finished.run.started_at;
        result.finished_at = finished.run.finished_at;
        persist();
        try {
          assertClean(await api("GET", "/api/simulator/status"), id);
        } catch (error) {
          result.state = "serious";
          persist();
          throw error;
        }
        log(`${id}/${entry.id}: ${result.state}`);
        requireThat(result.state !== "serious", "Infrastructure or cleanup scenario failure; matrix stopped");
        requireThat(result.state === "passed", "Traffic scenario failure; matrix stopped on first error");
      }
    }
    requireThat(report.organizations.some(org => org.scenarios.length), "No published scenarios found; nothing tested");
    report.state = !execute ? "planned" : report.results.some(result => result.state !== "passed") ? "failed" : "passed";
  } catch (error) {
    report.state = "failed";
    report.error = error.message;
  } finally {
    if (owned) {
      try {
        const catalog = await getCatalog();
        requireThat(catalog.run.organization_id === owned.organization && catalog.run.scenario_id === owned.scenario &&
          catalog.run.started_at === owned.started, "Cannot cancel a run owned by another operator");
        if (busy(catalog.run)) await api("POST", "/api/simulator/scenarios/cancel");
        await waitFor(getCatalog, next => {
          requireThat(next.run.started_at === owned.started, "Run changed during cancellation");
          return !busy(next.run);
        }, cleanupMs, null);
        report.results.at(-1).state = "interrupted";
      } catch (error) {
        report.recovery_error = error.message;
      }
    }
    if (changed && original) {
      report.restoration = "pending";
      persist();
      try {
        requireThat(!report.recovery_error, "Cleanup unresolved; leave runtime for operator recovery");
        await activate(original, null);
        report.restoration = "restored-selection";
      } catch (error) {
        report.restoration = "failed";
        report.restoration_error = error.message;
        report.state = "failed";
      }
    }
    report.finished_at = new Date().toISOString();
    persist();
  }
  return report;
}

export function parseOptions(args) {
  const options = { execute: false, exclusiveRuntime: false, url: "http://127.0.0.1:8788" };
  for (let index = 0; index < args.length; index++) {
    const argument = args[index];
    if (argument === "--execute") options.execute = true;
    else if (argument === "--exclusive-runtime") options.exclusiveRuntime = true;
    else if (argument === "--help") options.help = true;
    else {
      requireThat(["--url", "--report", "--headers-file", "--organizations", "--scenario-timeout-seconds",
        "--activation-timeout-seconds", "--poll-seconds"].includes(argument), `Unknown option: ${argument}`);
      const value = args[++index];
      requireThat(value && !value.startsWith("--"), `Missing value for ${argument}`);
      if (argument.endsWith("-seconds")) {
        requireThat(/^\d+$/.test(value) && Number(value) > 0 && Number.isSafeInteger(Number(value) * 1000), "Timeout/poll must be positive seconds");
        options[{"--scenario-timeout-seconds": "scenarioMs", "--activation-timeout-seconds": "activationMs", "--poll-seconds": "pollMs"}[argument]] = Number(value) * 1000;
      } else options[{"--url": "url", "--report": "report", "--headers-file": "headersFile", "--organizations": "organizations"}[argument]] =
        argument === "--organizations" ? value.split(",") : value;
    }
  }
  requireThat(!options.execute || options.exclusiveRuntime && options.report, "--execute requires --exclusive-runtime and --report");
  const url = new URL(options.url);
  requireThat(["http:", "https:"].includes(url.protocol) && !url.username && !url.password &&
    url.pathname === "/" && !url.search && !url.hash, "URL must be an HTTP(S) origin without credentials");
  options.url = url.origin;
  return options;
}

async function main() {
  const options = parseOptions(process.argv.slice(2));
  if (options.help) {
    console.log("simulator-matrix.mjs [--url ORIGIN] [--organizations ID,ID] [--headers-file FILE]\n" +
      "Default: plan only. Execute: --execute --exclusive-runtime --report NEW_FILE\n" +
      "Optional: --scenario-timeout-seconds N --activation-timeout-seconds N --poll-seconds N");
    return;
  }
  let lock;
  let lockOwned = false;
  let reportFile;
  const controller = new AbortController();
  const interrupt = () => controller.abort();
  try {
    if (options.report) {
      reportFile = path.resolve(options.report);
      fs.writeFileSync(reportFile, "{}\n", { flag: "wx", mode: 0o600 });
    }
    if (options.execute) {
      lock = path.join(os.tmpdir(), "zpr-simulator-matrix-" + crypto.createHash("sha256").update(options.url).digest("hex") + ".lock");
      const descriptor = fs.openSync(lock, "wx", 0o600);
      lockOwned = true;
      fs.writeFileSync(descriptor, JSON.stringify({ pid: process.pid, origin: options.url, report: reportFile }));
      fs.closeSync(descriptor);
    }
    const headers = options.headersFile ? JSON.parse(fs.readFileSync(options.headersFile, "utf8")) : {};
    requireThat(headers && typeof headers === "object" && !Array.isArray(headers) &&
      Object.values(headers).every(value => typeof value === "string"), "Headers file must contain a string-valued JSON object");
    process.on("SIGINT", interrupt);
    process.on("SIGTERM", interrupt);
    const api = async (method, route) => {
      const response = await fetch(options.url + route, { method, redirect: "error",
        headers: { ...headers, Accept: "application/json", ...(method === "POST" ? { Origin: options.url } : {}) },
        signal: AbortSignal.timeout(30_000) });
      requireThat(response.ok, `Simulator ${method} ${route.split("?")[0]} returned HTTP ${response.status}`);
      return response.json();
    };
    const report = await runMatrix({ ...options, api, signal: controller.signal, save: value => {
      if (reportFile) {
        const temporary = reportFile + "." + crypto.randomUUID() + ".tmp";
        fs.writeFileSync(temporary, JSON.stringify(value, null, 2) + "\n", { mode: 0o600, flag: "wx" });
        fs.renameSync(temporary, reportFile);
      }
    } });
    console.log(`Matrix ${report.state}; ${report.results.length} scenarios; restoration: ${report.restoration}`);
    if (report.error) console.error(report.error);
    if (report.restoration_error) console.error(report.restoration_error);
    process.exitCode = ["passed", "planned"].includes(report.state) ? 0 : 1;
  } finally {
    process.removeListener("SIGINT", interrupt);
    process.removeListener("SIGTERM", interrupt);
    if (lockOwned) fs.unlinkSync(lock);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
