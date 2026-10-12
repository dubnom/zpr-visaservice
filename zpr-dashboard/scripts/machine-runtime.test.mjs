import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, mkdirSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const helper = fileURLToPath(new URL("./machine-runtime.sh", import.meta.url));
test("Simulator controller and workload adapters use the verified nonblocking POSIX engine", () => {
  const controller = readFileSync(new URL("../cmd/zpr-web-dashboard/machine_control.go", import.meta.url), "utf8");
  assert.match(controller, /simulatorPacketIOEngine\s*=\s*"posix_unbatched"/);
  assert.match(controller, /phArgs := \[\]string\{"adapter", "--io-engine", simulatorPacketIOEngine,/);
  assert.match(controller, /exec\.Command\(filepath\.Join\(assets, "ph"\), "adapter", "--io-engine", simulatorPacketIOEngine,/);
});

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "zpr-machine-runtime-"));
  mkdirSync(join(root, "organizations"));
  writeFileSync(join(root, "active"), "northstar\n");
  for (const id of ["northstar", "great-lakes"]) {
    writeFileSync(join(root, "organizations", `${id}.json`), JSON.stringify({ runtime: { driver: "docker-multinode" } }));
  }
  return root;
}
function run(root, script, extra = {}) {
  return spawnSync("sh", ["-eu", "-c", `. "$1"; ${script}`, "test", helper], {
    encoding: "utf8", env: { ...process.env, RUNTIME_DIR: root, STATE_DIR: root,
      ORGANIZATIONS_DIR: join(root, "organizations"), ACTIVE_ORGANIZATION_FILE: join(root, "active"),
      SIMULATION_ORGANIZATION_ID: "", SIMULATION_EXPECTED_ORGANIZATION: "", SIMULATION_RUNTIME_LOCK_TOKEN: "", ...extra },
  });
}
test("activation request wins over the old active selection and scopes assets by generation", () => {
  const root = fixture();
  try {
    writeFileSync(join(root, "organization-request.json"), '{"organization_id":"great-lakes","generation":"generation-2"}');
    const result = run(root, 'resolve_machine_runtime; printf "%s:%s:%s\\n" "$machine_organization" "$machine_generation" "$MACHINE_WORKLOAD_DIR"', { SIMULATION_ORGANIZATION_ID: "northstar" });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout.trim(), `great-lakes:generation-2:${root}/machine-runtimes/great-lakes/generation-2`);
    const conflict = run(root, "resolve_machine_runtime", { SIMULATION_EXPECTED_ORGANIZATION: "northstar" });
    assert.notEqual(conflict.status, 0);
    assert.match(conflict.stderr, /does not match/);
  } finally { rmSync(root, { recursive: true }); }
});
test("invalid generations and corrupted requests cannot reuse old state", () => {
  const root = fixture();
  try {
    for (const request of ['{', '{"organization_id":"great-lakes","generation":"../old"}', '{"organization_id":"great-lakes"}']) {
      writeFileSync(join(root, "organization-request.json"), request);
      const result = run(root, "resolve_machine_runtime");
      assert.notEqual(result.status, 0);
      assert.equal(result.stdout, "");
    }
  } finally { rmSync(root, { recursive: true }); }
});
test("runtime lock excludes independent mutations but permits owned activation children", () => {
  const root = fixture();
  try {
    const acquired = run(root, 'acquire_machine_runtime_lock; printf "%s" "$SIMULATION_RUNTIME_LOCK_TOKEN"');
    assert.equal(acquired.status, 0, acquired.stderr);
    const blocked = run(root, "acquire_machine_runtime_lock");
    assert.notEqual(blocked.status, 0);
    assert.match(blocked.stderr, /mutation is locked/);
    const child = run(root, "acquire_machine_runtime_lock; release_machine_runtime_lock", { SIMULATION_RUNTIME_LOCK_TOKEN: acquired.stdout });
    assert.equal(child.status, 0, child.stderr);
    assert.equal(readFileSync(join(root, "machine-runtime.lock", "token"), "utf8").trim(), acquired.stdout);
    const staleToken = run(root, "acquire_machine_runtime_lock", { SIMULATION_RUNTIME_LOCK_TOKEN: "wrong" });
    assert.notEqual(staleToken.status, 0);
  } finally { rmSync(root, { recursive: true }); }
});
test("activation checks real controllers before publishing context and verifies rollback", () => {
  const activation = readFileSync(new URL("./activate-organization.sh", import.meta.url), "utf8");
  const stack = readFileSync(new URL("./dashboard-stack.sh", import.meta.url), "utf8");
  assert.ok(activation.lastIndexOf('sh "$script_dir/verify-machine-control.sh"') < activation.lastIndexOf('write_runtime_context "$organization"'));
  assert.match(activation, /SIMULATION_ORGANIZATION_ID="\$previous_organization" sh "\$script_dir\/verify-machine-control.sh"/);
  assert.ok(activation.indexOf('"$script_dir/dashboard-stack.sh" stop-runtime-machines', activation.indexOf('report_activation_status "Stopping previous runtime"')) < activation.indexOf('stop_runtime "$previous_organization"'));
  assert.match(stack, /existing_scope.*zpr\.organization\.id/);
  assert.match(stack, /SIMULATION_RUNTIME_GENERATION=\$machine_generation/);
});

test("readiness uses actual launch placement at every node in all five organizations", () => {
  const root = fixture();
  const organizations = fileURLToPath(new URL("../cmd/zpr-web-dashboard/examples/organizations", import.meta.url));
  const manifest = join(root, "manifest.json");
  writeFileSync(manifest, JSON.stringify({ machines: Array.from({ length: 12 }, (_, index) => ({
    id: `machine-${String(index + 1).padStart(2, "0")}`, owner: "default-owner", location: "Default Site",
  })) }));
  try {
    for (const id of ["northstar", "great-lakes", "load-lab", "redwood", "velocity"]) {
      const profile = join(organizations, `${id}.json`);
      const result = run(root, `
        probes=$(select_machine_readiness_probes)
        for machine in $probes; do
          resolve_machine_placement
          printf '%s:%s\\n' "$machine_node_index" "$machine"
        done
      `, { machine_profile: profile, machine_runtime_driver: "docker-multinode", SIMULATION_MANIFEST: manifest });
      assert.equal(result.status, 0, `${id}: ${result.stderr}`);
      const placements = result.stdout.trim().split("\n");
      const nodes = JSON.parse(readFileSync(profile, "utf8")).runtime.nodes;
      assert.deepEqual(placements.map(line => Number(line.split(":")[0])), nodes.map((_, index) => index), id);
      if (id === "northstar") assert.deepEqual(placements, ["0:machine-01"]);
    }
    const activation = readFileSync(new URL("./activate-organization.sh", import.meta.url), "utf8");
    assert.ok(activation.indexOf("select_machine_readiness_probes") <
      activation.indexOf('report_activation_status "Stopping previous runtime"'));
  } finally { rmSync(root, { recursive: true }); }
});

test("readiness rejects uncovered nodes and malformed ownership instead of guessing placement", () => {
  const root = fixture();
  const manifest = join(root, "manifest.json");
  const profile = join(root, "profile.json");
  writeFileSync(manifest, '{"machines":[{"id":"machine-01","owner":"owner","location":"Site A"}]}');
  try {
    const base = { runtime: { topology: "multi-node", nodes: [
      { location: "Site A", substrate_address: "172.30.0.13" },
      { location: "Site B", substrate_address: "172.30.0.14" },
    ] }, directory: { people: [{ uid: "owner", location: "Site A" }] } };
    for (const owners of [{ "machine-01": ["owner"] }, [], false]) {
      writeFileSync(profile, JSON.stringify({ ...base, machine_owners: owners }));
      const result = run(root, "select_machine_readiness_probes", {
        machine_profile: profile, machine_runtime_driver: "docker-multinode", SIMULATION_MANIFEST: manifest,
      });

      assert.notEqual(result.status, 0);
      assert.match(result.stderr, /no readiness machine for configured node 1|invalid organization machine owners/);
    }
  } finally { rmSync(root, { recursive: true }); }
});

test("failed readiness captures receive queues and workers before removing the probe", () => {
  const verification = readFileSync(new URL("./verify-machine-control.sh", import.meta.url), "utf8");
  assert.match(verification, /grep "\^Udp:" \/proc\/net\/snmp/);
  assert.match(verification, /ss -uamn \| head -n 80/);
  assert.match(verification, /wchan:32/);
  assert.match(verification, /\.runtime\.nodes \| keys \| \.\[\]/);
  assert.ok(verification.indexOf('capture_readiness_io "$probe_container"') <
    verification.indexOf('docker stop --time 5 "$probe_container"'));
  assert.match(verification, /exit "\$probe_result"/);
});

test("legacy and container Internet Gateway adapters use a separate tunnel from machine control", () => {
  const source = readFileSync(new URL("../cmd/zpr-web-dashboard/machine_control.go", import.meta.url), "utf8");
  const gateway = source.match(/"internet-gateway":\s*\{[^\n]*tun: "([^"]+)"/);
  const control = source.match(/machineControlTun\s*=\s*"([^"]+)"/);
  assert.ok(gateway && control);
  assert.notEqual(gateway[1], control[1]);
  const legacy = readFileSync(new URL("./simulation-agent.sh", import.meta.url), "utf8");
  assert.match(legacy, new RegExp(`internet-gateway\\)[^\\n]*tun=${gateway[1]};`));
});

test("machine images install HTTPS trust roots for Gateway upstreams", () => {
  const dockerfile = readFileSync(new URL("./Dockerfile.machine", import.meta.url), "utf8");
  const packages = dockerfile.match(/apt-get install[^\n]+/);
  assert.ok(packages);
  assert.match(packages[0], /\bca-certificates\b/);
});

test("activation and rollback recalculate trusted-feed scope for the organization being restored", () => {
  const root = fixture();
  try {
    writeFileSync(join(root, "trusted-change-feeds.network"), "zpr-great-lakes\n");
    const result = run(root, `
      configure_activation_trusted_feeds great-lakes
      printf '%s:' "$trusted_change_feeds_disabled"
      configure_activation_trusted_feeds northstar
      printf '%s:' "$trusted_change_feeds_disabled"
      configure_activation_trusted_feeds great-lakes
      printf '%s' "$trusted_change_feeds_disabled"
    `, { ZPR_TRUSTED_CHANGE_FEEDS_NETWORK: "", ZPR_TRUSTED_CHANGE_FEEDS_DISABLED: "" });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, ":1:");
    const disabled = run(root, 'configure_activation_trusted_feeds great-lakes; printf "%s" "$trusted_change_feeds_disabled"',
      { ZPR_TRUSTED_CHANGE_FEEDS_NETWORK: "", ZPR_TRUSTED_CHANGE_FEEDS_DISABLED: "1" });
    assert.equal(disabled.status, 0, disabled.stderr);
    assert.equal(disabled.stdout, "1");
    const activation = readFileSync(new URL("./activate-organization.sh", import.meta.url), "utf8");
    const rollback = activation.slice(activation.indexOf("rollback()"), activation.indexOf("trap rollback"));
    assert.match(rollback, /configure_activation_trusted_feeds "\$previous_organization"/);
    assert.equal((rollback.match(/ZPR_TRUSTED_CHANGE_FEEDS_DISABLED="\$trusted_change_feeds_disabled"/g) || []).length, 2);
  } finally { rmSync(root, { recursive: true }); }
});
