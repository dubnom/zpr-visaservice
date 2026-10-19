import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";

const script = new URL("./verify-packaged-identity.sh", import.meta.url);

test("independent identity probe requires correlated admission rejection and preserves ownership", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "zpr-admission-probe-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const bin = join(root, "bin");
  const calls = join(root, "calls");
  await mkdir(bin);
  await writeFile(join(bin, "sleep"), "#!/bin/sh\nexit 0\n", { mode: 0o755 });
  await writeFile(join(bin, "docker"), `#!/bin/sh
set -eu
printf '%s\\n' "$*" >> "$CALLS"
case "$1 $2" in
  'inspect --format')
    case "$3" in
      *State.Running*Image*) service=\${4#zprdemo-fixture-}; service=\${service%-1}
        if [ "$MODE" = mismatch ]; then echo wrong; else echo "true|image|zprdemo-fixture|$service"; fi ;;
      '{{.State.Running}}') echo true ;;
      *) exit 92 ;;
    esac ;;
  'network inspect') echo zprdemo-fixture ;;
  'exec zprdemo-fixture-vs-1')
    case "$3" in
      redis-cli) echo '{"active":"unchanged"}' ;;
      openssl) echo 0123456789abcdef01234567 ;;
      sh)
        case "$5" in
          *wc*) echo 0 ;;
          *tail*)
            case "$MODE" in
              success) echo 'authentication failed: no key found in policy for cn simulator-unapproved-0123456789abcdef01234567' ;;
              unrelated) echo 'authentication failed: no key found in policy for cn other-identity' ;;
              missing) echo 'request timed out' ;;
              *) exit 93 ;;
            esac ;;
        esac ;;
    esac ;;
  'exec zprdemo-fixture-node1-1') echo 'public certificate only' ;;
  'exec zprdemo-fixture-alice-1') exit 0 ;;
  'create --pull') [ "$MODE" != conflict ] || exit 1; echo owned-container-id ;;
  'start owned-container-id'|'rm -f') ;;
  *) exit 94 ;;
esac
`, { mode: 0o755 });
  const run = async (mode, project = "zprdemo-fixture") => {
    await writeFile(calls, "");
    const result = spawnSync("sh", [script.pathname, "verify-packaged-identity"], {
      encoding: "utf8",
      env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, CALLS: calls,
        MODE: mode, SIMULATION_DEMO_PROJECT: project, SIMULATION_DEMO_IMAGE: "image" },
    });
    return { ...result, calls: await readFile(calls, "utf8") };
  };
  const success = await run("success");
  assert.equal(success.status, 0, success.stderr);
  assert.match(success.stdout, /unapproved identity explicitly rejected/);
  assert.match(success.calls, /openssl genrsa -out \/run\/zpr\/bootstrap.key 2048/);
  assert.match(success.calls, /--cap-drop ALL --cap-add NET_ADMIN --device \/dev\/net\/tun/);
  assert.match(success.calls, /--tmpfs \/run\/zpr:rw,noexec,nosuid,size=4m,mode=0700/);
  assert.match(success.calls, /rm -f owned-container-id/);
  assert.doesNotMatch(success.calls, /--privileged|--mount|--bootstrap-key \/conf|--publish/);
  for (const mode of ["unrelated", "missing"]) {
    const rejected = await run(mode);
    assert.notEqual(rejected.status, 0);
    assert.match(rejected.stderr, /no explicit Visa rejection/);
    assert.doesNotMatch(rejected.stdout, /unapproved identity explicitly rejected/);
    assert.match(rejected.calls, /rm -f owned-container-id/);
  }
  const conflict = await run("conflict");
  assert.notEqual(conflict.status, 0);
  assert.match(conflict.stderr, /never adopted or removed/);
  assert.doesNotMatch(conflict.calls, /^rm /m);
  const mismatch = await run("mismatch");
  assert.notEqual(mismatch.status, 0);
  assert.match(mismatch.stderr, /ownership, image or running-state mismatch/);
  assert.doesNotMatch(mismatch.calls, /^(create|start|rm) /m);
  const invalid = await run("success", "zprdemo-fixture;invalid");
  assert.notEqual(invalid.status, 0);
  assert.equal(invalid.calls, "");
});
