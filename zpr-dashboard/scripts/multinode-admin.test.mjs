import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, delimiter } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const helper = fileURLToPath(new URL("./multinode-admin.sh", import.meta.url));
function run(binding, organization = "great-lakes", exit = "0") {
  const directory = mkdtempSync(join(tmpdir(), "zpr-admin-port-"));
  try {
    writeFileSync(join(directory, "docker"), `#!/bin/sh
[ "$1" = inspect ] && [ "$4" = "$EXPECTED_CONTAINER" ] || exit 9
printf '%s\\n' "$BINDING"
exit "$DOCKER_EXIT"
`, { mode: 0o755 });
    return spawnSync("sh", ["-c", '. "$1"; multinode_admin_port "$2"', "test", helper, organization], {
      encoding: "utf8",
      env: { ...process.env, PATH: directory + delimiter + process.env.PATH, BINDING: JSON.stringify(binding),
        DOCKER_EXIT: exit, EXPECTED_CONTAINER: `${organization}-vs` },
    });
  } finally { rmSync(directory, { recursive: true }); }
}
const binding = (port, host = "127.0.0.1") => ({ running: true, bindings: [{ HostIp: host, HostPort: port }] });

test("discovers actual published port, including nondefault organization mappings", () => {
  for (const [organization, port] of [["load-lab", "8185"], ["great-lakes", "8186"], ["redwood", "24567"]]) {
    const result = run(binding(port), organization);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout.trim(), port);
  }
});

test("rejects stopped, missing, ambiguous, nonlocal, and invalid endpoints explicitly", () => {
  for (const value of [
    { ...binding("8186"), running: false }, { running: true, bindings: null },
    { running: true, bindings: [] }, { running: true, bindings: [binding("8185").bindings[0], binding("8186").bindings[0]] },
    binding("8186", "192.0.2.1"), binding("0"), binding("65536"), binding("not-a-port"), binding(8186),
  ]) {
    const result = run(value);
    assert.notEqual(result.status, 0);
    assert.equal(result.stdout, "");
    assert.match(result.stderr, /Visa admin endpoint/);
  }
});

test("missing container and invalid organization never fall back to 8185", () => {
  for (const [organization, exit] of [["great-lakes", "1"], ["invalid/name", "0"], ["", "0"]]) {
    const result = run(binding("8185"), organization, exit);
    assert.notEqual(result.status, 0);
    assert.equal(result.stdout, "");
    assert.match(result.stderr, /unavailable|invalid/);
  }
});

test("activation, rollback, bootstrap upload, startup and collector wire discovery", () => {
  const activation = readFileSync(new URL("./activate-organization.sh", import.meta.url), "utf8");
  const stack = readFileSync(new URL("./dashboard-stack.sh", import.meta.url), "utf8");
  assert.match(activation, /ZPR_ADMIN_PORT=\$\(multinode_admin_port "\$organization"\)/);
  assert.match(activation, /previous_admin_port=\$\(multinode_admin_port "\$previous_organization"\)/);
  assert.match(activation, /ZPR_ADMIN_URL="https:\/\/127\.0\.0\.1:\$previous_admin_port"/);
  assert.match(activation, /ZPR_ADMIN_URL="https:\/\/127\.0\.0\.1:\$ZPR_ADMIN_PORT"/);
  assert.ok(activation.indexOf('ZPR_ADMIN_PORT=$(multinode_admin_port "$organization")') < activation.indexOf("if ! install_multinode_policy"));
  assert.match(activation, /--svc-url "https:\/\/host\.docker\.internal:\$\{ZPR_ADMIN_PORT:-8185\}"/);
  assert.match(stack, /startup_admin_port=\$\(multinode_admin_port "\$startup_organization"\)/);
  assert.match(stack, /ZPR_ADMIN_URL="https:\/\/127\.0\.0\.1:\$startup_admin_port"/);
  assert.match(stack, /ZPR_VS_ADMIN_URL="https:\/\/vs\.zpr:\$collector_admin_port"/);
  assert.doesNotMatch(activation + stack, /ZPR_ADMIN_URL="?https:\/\/127\.0\.0\.1:8185/);
});
