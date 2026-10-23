const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync, spawnSync } = require("node:child_process");

const [build, output, context, daemon, ...extra] = process.argv.slice(2);
assert.ok(build && output && context && daemon && !extra.length,
  "Usage: node probe-runtime-image.cjs /absolute/image-build /absolute/new-evidence context daemon");
assert.ok(path.isAbsolute(build) && path.isAbsolute(output));
const hash = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
const docker = args => execFileSync("docker", ["--context", context, ...args], {
  encoding: "utf8", maxBuffer: 16 * 1024 * 1024,
}).trim();
assert.equal(docker(["info", "--format", "{{.ID}}"]), daemon, "Wrong Docker daemon");
assert.match(docker(["info", "--format", "{{.Architecture}}"]), /^(aarch64|arm64)$/);
const image = JSON.parse(fs.readFileSync(path.join(build, "image.json")));
assert.equal(image.kind, "compiled-runtime-image");
assert.equal(image.target, "linux/arm64");
assert.equal(image.public_release, false);
const saved = path.join(build, "runtime-image.tar.gz");
assert.equal(hash(saved), image.download_sha256);
assert.equal(fs.readFileSync(`${saved}.sha256`, "utf8").trim(),
  `${image.download_sha256}  runtime-image.tar.gz`);
docker(["image", "load", "-i", saved]);
assert.equal(docker(["image", "inspect", "--format", "{{.Id}}", image.image_id]), image.image_id);
fs.mkdirSync(output, { mode: 0o700 });
const root = fs.realpathSync(output);
const token = crypto.randomBytes(8).toString("hex");
const containers = [];
const volumes = [];
const checks = [];
let acceptance;
function run(name, script, args, files) {
  const directory = path.join(root, name);
  fs.mkdirSync(directory, { mode: 0o700 });
  const volume = docker(["volume", "create", "--label", `zpr.runtime-probe=${token}`]);
  volumes.push(volume);
  const id = docker(["create", "-i", "--name", `zpr-runtime-${token}-${name}`,
    "--network", "none", "--read-only", "--cap-drop", "ALL",
    "--security-opt", "no-new-privileges:true", "--pids-limit", "128",
    "--memory", "512m", "--cpus", "1",
    "--mount", `type=volume,src=${volume},dst=/state`,
    image.image_id, "sh", "-s", "--", ...args]);
  containers.push(id);
  const config = JSON.parse(docker(["inspect", id]))[0];
  assert.equal(config.Config.User, "501:1000");
  assert.equal(config.HostConfig.Privileged, false);
  assert.equal(config.HostConfig.ReadonlyRootfs, true);
  assert.equal(config.HostConfig.NetworkMode, "none");
  assert.deepEqual(config.HostConfig.CapDrop, ["ALL"]);
  assert.ok(!config.HostConfig.CapAdd?.length);
  assert.ok(!config.HostConfig.PortBindings || !Object.keys(config.HostConfig.PortBindings).length);
  assert.equal(config.Mounts.length, 1);
  assert.equal(config.Mounts[0].Type, "volume");
  assert.equal(config.Mounts[0].Destination, "/state");
  const result = spawnSync("docker", ["--context", context, "start", "-a", "-i", id], {
    input: script, encoding: "utf8", maxBuffer: 16 * 1024 * 1024, timeout: 240000,
  });
  if (result.error) throw result.error;
  fs.writeFileSync(path.join(directory, "probe.log"), result.stdout + result.stderr, { mode: 0o600 });
  assert.equal(result.status, 0, `${name} failed; inspect private probe.log`);
  const state = JSON.parse(docker(["inspect", "--format", "{{json .State}}", id]));
  assert.equal(state.ExitCode, 0, `${name} container failed; inspect private probe.log`);
  assert.equal(state.OOMKilled, false);
  for (const file of files) {
    const target = path.join(directory, file);
    fs.mkdirSync(path.dirname(target), { recursive: true, mode: 0o700 });
    docker(["cp", `${id}:/state/${name}/${file}`, target]);
  }
  checks.push({ name, exit_code: state.ExitCode, user: config.Config.User,
    read_only_root: true, capabilities: [], network: "none", published_ports: [] });
}
try {
  run("contract", `set -eu
umask 077
mkdir /state/contract
test "$(id -u)" = 501
test "$(id -g)" = 1000
grep -Eq '^CapEff:[[:space:]]+0+$' /proc/self/status
grep -Eq '^NoNewPrivs:[[:space:]]+1$' /proc/self/status
test "$(ip -o link show | wc -l)" -eq 1
test ! -e /var/run/docker.sock
for tool in cargo rustc go gcc cc git node npm; do
  if command -v "$tool" >/dev/null 2>&1; then echo "Unexpected build tool: $tool" >&2; exit 1; fi
done
test ! -e /opt/zpr/src
test ! -e /opt/zpr/node_modules
test ! -e /opt/zpr/target
test -z "$(find /state -mindepth 1 -maxdepth 1 ! -name contract -print)"
cd /opt/zpr
sha256sum -c SHA256SUMS > /state/contract/inventory.txt
sha256sum bin/* > /state/contract/binaries.sha256
cp runtime-packages.txt /state/contract/
ip -o link show > /state/contract/interfaces.txt
cat /proc/self/status > /state/contract/process-status.txt
printf 'PASS: nonroot, zero capabilities, no-new-privileges, loopback only, no toolchain/source/socket; archive inventory verified.\\n' > /state/contract/result.txt
`, [], ["inventory.txt", "binaries.sha256", "runtime-packages.txt", "interfaces.txt", "process-status.txt", "result.txt"]);
  run("policy", fs.readFileSync(path.join(__dirname, "probe-rust-runtime.sh"), "utf8"),
    ["/opt/zpr/bin", "/opt/zpr/share/examples", "/state/policy"],
    ["result.txt", "binaries.sha256", "policies.sha256", "policy-signing-public.pem", "invalid-compiler.log",
      ...["northstar", "great-lakes", "redwood", "load-lab", "velocity"].map(name => `${name}/compiler.log`)]);
  run("service", fs.readFileSync(path.join(__dirname, "probe-visa-service.sh"), "utf8"),
    ["/opt/zpr/bin", "/opt/zpr/libexec/prepare-visa-service.sh", "/state/service"],
    ["result.txt", "binaries.sha256", "first/visa-bootstrap.json", "second/visa-bootstrap.json",
      "network.json", "restarted-network.json", "nonoverwrite.txt", "policy-install.json",
      "verified-tls-install.json", "redis.log",
      ...["foreign-ca", "hostname-mismatch", "plaintext", "foreign-api-key"].map(name => `${name}-output.txt`)]);
  const read = file => JSON.parse(fs.readFileSync(path.join(root, file)));
  assert.deepEqual(read("service/network.json"), read("service/restarted-network.json"));
  assert.notEqual(read("service/first/visa-bootstrap.json").service_identity,
    read("service/second/visa-bootstrap.json").service_identity);
  const redis = fs.readFileSync(path.join(root, "service/redis.log"), "utf8");
  assert.equal((redis.match(/Redis is starting/g) || []).length, 2);
  assert.match(redis, /DB loaded from append only file/);
  for (const name of ["foreign-ca", "hostname-mismatch", "plaintext", "foreign-api-key"]) {
    assert.equal(fs.statSync(path.join(root, "service", `${name}-output.txt`)).size, 0);
  }
  assert.equal(fs.readFileSync(path.join(root, "policy/policies.sha256"), "utf8").trim().split("\n").length, 5);
  const inventory = new Map(fs.readFileSync(path.join(root, "contract/binaries.sha256"), "utf8")
    .trim().split("\n").map(line => line.split(/\s+/).reverse()));
  assert.equal(inventory.size, 11);
  for (const [name, digest] of inventory) {
    assert.equal(hash(path.join(build, "context/runtime", name)), digest);
  }
  acceptance = { schema_version: 1, kind: "compiled-runtime-acceptance",
    image_id: image.image_id, download_sha256: image.download_sha256, checks,
    harness_sha256: hash(__filename),
    service_probe_sha256: hash(path.join(__dirname, "probe-visa-service.sh")),
    policy_probe_sha256: hash(path.join(__dirname, "probe-rust-runtime.sh")),
    archive_sha256: image.archive_sha256, rust_clis: 7, signed_policies: 5,
    durable_database_restart: true, simulator_available: false, ph_node_tested: false,
    public_release: false, complete_installation: false };
} finally {
  for (const id of containers.reverse()) docker(["rm", "-f", id]);
  for (const volume of volumes.reverse()) docker(["volume", "rm", volume]);
}
acceptance.cleanup_verified = true;
fs.writeFileSync(path.join(root, "result.json"), `${JSON.stringify(acceptance, null, 2)}\n`, { mode: 0o600 });
console.log(JSON.stringify(acceptance, null, 2));
