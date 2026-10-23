const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync, spawnSync } = require("node:child_process");

const [build, context, expectedDaemon, ...extra] = process.argv.slice(2);
assert.ok(build && context && expectedDaemon && extra.length === 0,
  "Usage: node smoke.cjs /absolute/build explicit-context expected-daemon-id");
assert.ok(path.isAbsolute(build), "Build directory must be absolute");
const root = fs.realpathSync(build);
const archive = path.join(root, "compiled-recovery-linux-arm64.tar.gz");
const digest = content => crypto.createHash("sha256").update(content).digest("hex");
const expectedHash = fs.readFileSync(`${archive}.sha256`, "utf8").trim();
assert.match(expectedHash, /^[a-f0-9]{64} {2}compiled-recovery-linux-arm64\.tar\.gz$/);
assert.equal(digest(fs.readFileSync(archive)), expectedHash.slice(0, 64));

const members = execFileSync("tar", ["-tzf", archive], { encoding: "utf8" }).trim().split("\n");
for (const member of members) {
  assert.ok(member.startsWith("./") && !member.split("/").includes(".."),
    "Archive contains an unsafe path");
}
const listing = execFileSync("tar", ["-tvzf", archive], { encoding: "utf8" }).trim().split("\n");
assert.ok(listing.every(line => line.startsWith("-") || line.startsWith("d")),
  "Archive must contain only directories and regular files");
const extracted = path.join(root, "smoke-extracted");
fs.mkdirSync(extracted, { mode: 0o700 });
execFileSync("tar", ["-xzf", archive, "-C", extracted]);
const receipts = path.join(root, "smoke-evidence");
fs.mkdirSync(receipts, { mode: 0o700 });

const inventory = new Map();
for (const line of fs.readFileSync(path.join(extracted, "SHA256SUMS"), "utf8").trim().split("\n")) {
  const match = /^([a-f0-9]{64}) {2}\.\/([a-zA-Z0-9_./-]+)$/.exec(line);
  assert.ok(match && !match[2].split("/").includes(".."), "Invalid file inventory");
  assert.ok(!inventory.has(match[2]), "Duplicate inventory path");
  inventory.set(match[2], match[1]);
}
function verifyDirectory(directory, relative = "") {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const name = relative + entry.name;
    if (entry.isDirectory()) {
      assert.equal(fs.statSync(path.join(directory, entry.name)).mode & 0o777, 0o755);
      verifyDirectory(path.join(directory, entry.name), `${name}/`);
    } else {
      assert.ok(entry.isFile(), "Extracted tree contains a non-regular file");
      if (name === "SHA256SUMS") continue;
      assert.ok(inventory.has(name), `Unlisted archive file: ${name}`);
      const content = fs.readFileSync(path.join(directory, entry.name));
      assert.equal(digest(content), inventory.get(name), `Digest mismatch: ${name}`);
      assert.ok(name.startsWith("bin/") || name.startsWith("share/examples/") ||
        name.startsWith("licenses/") || ["build.json", "go-build-info.txt", "README.md"].includes(name),
      `Unexpected archive surface: ${name}`);
      if (name.startsWith("bin/")) {
        assert.equal(fs.statSync(path.join(directory, entry.name)).mode & 0o777, 0o755);
        assert.ok(content.subarray(0, 6).equals(Buffer.from([127, 69, 76, 70, 2, 1])));
        assert.equal(content.readUInt16LE(18), 183, "Binary must target Linux ARM64");
      } else {
        assert.equal(fs.statSync(path.join(directory, entry.name)).mode & 0o777, 0o644);
        const text = content.toString("utf8");
        assert.ok(!/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(text), `Private key in ${name}`);
        for (const line of text.split("\n")) {
          if (/^(userPassword|authPassword|userCertificate|userPKCS12)(;[^:]*)?:/i.test(line)) {
            assert.equal(line, "userPassword: DEMO_BIND_PASSWORD_HASH",
              `Unexpected directory credential in ${name}`);
          }
        }
      }
      inventory.delete(name);
    }
  }
}
verifyDirectory(extracted);
assert.equal(inventory.size, 0, "File inventory references missing files");
const binaries = fs.readdirSync(path.join(extracted, "bin")).sort();
assert.deepEqual(binaries, ["zpr-lab-bootstrap", "zpr-system-config", "zpr-trusted-service", "zpr-web-dashboard"]);
const examples = path.join(extracted, "share/examples");
const organizations = fs.readdirSync(path.join(examples, "organizations")).filter(name => name.endsWith(".json")).sort();
assert.deepEqual(organizations, ["great-lakes.json", "load-lab.json", "northstar.json", "redwood.json", "velocity.json"]);
assert.equal(fs.readdirSync(path.join(examples, "scenarios")).filter(name => name.endsWith(".json")).length, 17);
const metadata = JSON.parse(fs.readFileSync(path.join(extracted, "build.json")));
assert.equal(metadata.public_release, false);
assert.equal(metadata.complete_installation, false);
assert.equal(metadata.target, "linux/arm64");

const docker = args => execFileSync("docker", ["--context", context, ...args], {
  encoding: "utf8", maxBuffer: 8 * 1024 * 1024,
}).trim();
assert.equal(docker(["info", "--format", "{{.ID}}"]), expectedDaemon, "Wrong Docker daemon");
assert.match(docker(["info", "--format", "{{.Architecture}}"]), /^(aarch64|arm64)$/);
const token = crypto.randomBytes(8).toString("hex");
const image = `zpr-compiled-smoke:${token}`;
let imageID;
let volume;
const containers = [];
const checks = [];
function run(name, args, success = true) {
  const id = docker(["create", "--name", `zpr-smoke-${token}-${name}`,
    "--network", "none", "--read-only", "--cap-drop", "ALL",
    "--security-opt", "no-new-privileges:true",
    "--mount", `type=volume,src=${volume},dst=/state`,
    image, ...args]);
  containers.push(id);
  const result = spawnSync("docker", ["--context", context, "start", "-a", id], { encoding: "utf8" });
  if (result.error) throw result.error;
  const state = JSON.parse(docker(["inspect", "--format", "{{json .State}}", id]));
  assert.equal(state.Error, "", `${name} must start the compiled executable`);
  const exit = state.ExitCode;
  assert.equal(exit === 0, success, `${name} exit ${exit}: ${result.stderr}${result.stdout}`);
  if (!success) {
    assert.match(result.stderr + result.stdout, /existing directories, files and symlinks are never reused or overwritten/,
      "The negative check must fail because existing output was refused");
  }
  checks.push({ name, exit_code: exit });
  return id;
}
try {
  imageID = docker(["import", "--platform", "linux/arm64", archive, image]);
  volume = docker(["volume", "create", "--label", `zpr.compiled-smoke=${token}`, `zpr-smoke-${token}`]);
  for (const file of organizations) {
    const organization = file.slice(0, -5);
    const id = run(organization, ["/bin/zpr-web-dashboard", "-mode", "compose-policy",
      "-policy-root", "/share/examples", "-policy-organization", organization,
      "-policy-output", `/state/${organization}.zpl`]);
    docker(["cp", `${id}:/state/${organization}.zpl`, path.join(receipts, `${organization}.zpl`)]);
    assert.ok(fs.statSync(path.join(receipts, `${organization}.zpl`)).size > 0);
  }
  for (const name of ["lab-one", "lab-two"]) {
    const id = run(name, ["/bin/zpr-lab-bootstrap", "-domain", "smoke.example.test", "-output", `/state/${name}`]);
    docker(["cp", `${id}:/state/${name}/lab-ca.pem`, path.join(receipts, `${name}-ca.pem`)]);
  }
  assert.notEqual(
    digest(fs.readFileSync(path.join(receipts, "lab-one-ca.pem"))),
    digest(fs.readFileSync(path.join(receipts, "lab-two-ca.pem"))),
    "Fresh lab identities must not reuse the same CA");
  run("refuse-existing", ["/bin/zpr-lab-bootstrap", "-domain", "smoke.example.test", "-output", "/state/lab-one"], false);
} finally {
  const errors = [];
  for (const args of [
    ...containers.reverse().map(id => ["rm", "-f", id]),
    ...(volume ? [["volume", "rm", volume]] : []),
    ...(imageID ? [["image", "rm", image]] : []),
  ]) {
    const result = spawnSync("docker", ["--context", context, ...args], { encoding: "utf8" });
    if (result.error || result.status !== 0) errors.push(`${args[0]}: ${result.error || result.stderr}`);
  }
  assert.equal(errors.length, 0, `Smoke cleanup failed: ${errors.join("; ")}`);
}
const report = {
  schema_version: 1, source_revision: metadata.source_revision, archive_sha256: expectedHash.slice(0, 64),
  smoke_harness_sha256: digest(fs.readFileSync(__filename)),
  docker_context: context, docker_daemon: expectedDaemon, target: "linux/arm64",
  network: "none", source_mounts: false, complete_installation: false, checks,
  archive_bytes: fs.statSync(archive).size,
  extracted_bytes: fs.readdirSync(extracted, { recursive: true, withFileTypes: true })
    .filter(entry => entry.isFile()).reduce((size, entry) => size + fs.statSync(path.join(entry.parentPath, entry.name)).size, 0),
};
fs.writeFileSync(path.join(receipts, "result.json"), `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 });
console.log(`PASS: ${checks.length} compiled offline checks; ${report.archive_bytes} compressed bytes; ${report.extracted_bytes} extracted bytes.`);
console.log("Owned Docker test resources removed. This is not full-install or scenario acceptance.");
