const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync, spawnSync } = require("node:child_process");
const { test } = require("node:test");

const hash = data => crypto.createHash("sha256").update(data).digest("hex");
function fixture(t) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "zpr-runtime-test-")));
  t.after(() => fs.rmSync(root, { recursive: true }));
  const bin = path.join(root, "bin");
  const inputs = path.join(root, "inputs");
  const build = path.join(root, "build");
  const output = path.join(root, "output");
  const log = path.join(root, "docker.log");
  for (const dir of [bin, inputs, build, path.join(inputs, "packages"), path.join(build, "smoke-evidence")]) {
    fs.mkdirSync(dir);
  }
  fs.writeFileSync(path.join(bin, "docker"), `#!${process.execPath}
const fs = require("node:fs");
const args = process.argv.slice(2);
fs.appendFileSync(process.env.DOCKER_TEST_LOG, JSON.stringify(args) + "\\n");
if (args[2] !== "info") process.exit(99);
console.log(args[4] === "{{.ID}}" ? "test-daemon" : "aarch64");
`, { mode: 0o755 });
  fs.writeFileSync(path.join(inputs, "base-image.tar"), "base");
  fs.writeFileSync(path.join(inputs, "packages", "example.deb"), "package");
  const receipt = { schema_version: 1, kind: "compiled-runtime-inputs", target: "linux/arm64",
    base_reference: `ubuntu@sha256:${"a".repeat(64)}`, base_image_id: `sha256:${"b".repeat(64)}`,
    base_archive_sha256: hash("base"),
    packages: [{ file: "example.deb", sha256: hash("package"), bytes: 7, metadata: "Architecture: arm64" }] };
  const save = () => fs.writeFileSync(path.join(inputs, "inputs.json"), JSON.stringify(receipt));
  save();
  const archive = path.join(build, "compiled-recovery-linux-arm64.tar.gz");
  const seed = path.join(root, "seed");
  fs.mkdirSync(seed);
  fs.writeFileSync(path.join(seed, "hello"), "safe");
  execFileSync("tar", ["-czf", archive, "-C", seed, "."]);
  const smoke = { archive_sha256: hash(fs.readFileSync(archive)), target: "linux/arm64",
    rust_binaries_included: true, checks: [
      ...["great-lakes", "load-lab", "northstar", "redwood", "velocity", "lab-one", "lab-two"]
        .map(name => ({ name, exit_code: 0 })),
      { name: "refuse-existing", exit_code: 1 },
    ] };
  const saveArchive = () => {
    smoke.archive_sha256 = hash(fs.readFileSync(archive));
    fs.writeFileSync(`${archive}.sha256`, `${smoke.archive_sha256}  compiled-recovery-linux-arm64.tar.gz\n`);
    fs.writeFileSync(path.join(build, "smoke-evidence/result.json"), JSON.stringify(smoke));
  };
  saveArchive();
  const run = (script = "build-runtime-image.cjs", args = [build, inputs, output, "test-context", "test-daemon"]) =>
    spawnSync(process.execPath, [path.join(__dirname, script), ...args], {
      encoding: "utf8", env: { ...process.env, PATH: `${bin}${path.delimiter}${process.env.PATH}`, DOCKER_TEST_LOG: log },
    });
  return { root, inputs, build, output, log, receipt, save, run, archive, seed, smoke, saveArchive };
}

for (const scenario of [
  ["changed base", f => fs.appendFileSync(path.join(f.inputs, "base-image.tar"), "changed")],
  ["changed package", f => fs.appendFileSync(path.join(f.inputs, "packages/example.deb"), "changed")],
  ["package traversal", f => { f.receipt.packages[0].file = "../example.deb"; f.save(); }],
  ["duplicate package", f => { f.receipt.packages.push(f.receipt.packages[0]); f.save(); }],
  ["wrong package architecture", f => { f.receipt.packages[0].metadata = "Architecture: amd64"; f.save(); }],
  ["package symlink", f => {
    fs.renameSync(path.join(f.inputs, "packages/example.deb"), path.join(f.root, "outside.deb"));
    fs.symlinkSync(path.join(f.root, "outside.deb"), path.join(f.inputs, "packages/example.deb"));
  }],
  ["mutable base", f => { f.receipt.base_reference = "ubuntu:24.04"; f.save(); }],
  ["wrong target", f => { f.receipt.target = "linux/amd64"; f.save(); }],
  ["changed compiled archive", f => fs.appendFileSync(f.archive, "changed")],
  ["missing acceptance", f => fs.unlinkSync(path.join(f.build, "smoke-evidence/result.json"))],
  ["wrong acceptance archive", f => { f.smoke.archive_sha256 = "c".repeat(64);
    fs.writeFileSync(path.join(f.build, "smoke-evidence/result.json"), JSON.stringify(f.smoke)); }],
  ["failed acceptance check", f => { f.smoke.checks[0].exit_code = 1; f.saveArchive(); }],
  ["symlink archive member", f => {
    fs.symlinkSync("/outside", path.join(f.seed, "link"));
    execFileSync("tar", ["-czf", f.archive, "-C", f.seed, "."]);
    f.saveArchive();
  }],
]) {
  test(`runtime assembly refuses ${scenario[0]} before creating output or building`, t => {
    const f = fixture(t);
    scenario[1](f);
    const result = f.run();
    assert.notEqual(result.status, 0);
    assert.equal(fs.existsSync(f.output), false);
    const calls = fs.readFileSync(f.log, "utf8").trim().split("\n").map(line => JSON.parse(line));
    assert.ok(calls.every(args => args[2] === "info"), "Refusal must precede image load/build");
  });
}

test("wrong daemon refuses before acquisition, build or probe", t => {
  const f = fixture(t);
  for (const [script, args] of [
    ["acquire-runtime-inputs.cjs", [f.output, "test-context", "wrong", `ubuntu@sha256:${"a".repeat(64)}`]],
    ["build-runtime-image.cjs", [f.build, f.inputs, f.output, "test-context", "wrong"]],
    ["probe-runtime-image.cjs", [f.build, f.output, "test-context", "wrong"]],
    ["probe-node-image.cjs", [f.build, f.output, "test-context", "wrong"]],
    ["probe-node-image.cjs", [f.build, f.output, "test-context", "wrong", "--peer-node"]],
    ["probe-node-image.cjs", [f.build, f.output, "test-context", "wrong", "--peer-node-early"]],
  ]) {
    const result = f.run(script, args);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Wrong Docker daemon/);
    assert.equal(fs.existsSync(f.output), false);
  }
});

for (const [label, change] of [
  ["changed image bytes", f => fs.appendFileSync(path.join(f.build, "runtime-image.tar.gz"), "changed")],
  ["wrong image target", (f, image) => { image.target = "linux/amd64"; }],
  ["wrong image kind", (f, image) => { image.kind = "unknown"; }],
  ["invalid image identity", (f, image) => { image.image_id = "moving:tag"; }],
  ["wrong checksum sidecar", f => fs.writeFileSync(path.join(f.build, "runtime-image.tar.gz.sha256"), "wrong")],
  ["existing evidence", f => { fs.mkdirSync(f.output); fs.writeFileSync(path.join(f.output, "sentinel"), "unchanged"); }],
]) {
  test(`node container probe refuses ${label} before image load or resource creation`, t => {
    const f = fixture(t);
    const image = { kind: "compiled-runtime-image", target: "linux/arm64", public_release: false,
      image_id: `sha256:${"a".repeat(64)}`, download_sha256: hash("image") };
    fs.writeFileSync(path.join(f.build, "runtime-image.tar.gz"), "image");
    fs.writeFileSync(path.join(f.build, "runtime-image.tar.gz.sha256"), `${image.download_sha256}  runtime-image.tar.gz\n`);
    change(f, image);
    fs.writeFileSync(path.join(f.build, "image.json"), JSON.stringify(image));
    const result = f.run("probe-node-image.cjs", [f.build, f.output, "test-context", "test-daemon"]);
    assert.notEqual(result.status, 0);
    const calls = fs.readFileSync(f.log, "utf8").trim().split("\n").map(line => JSON.parse(line));
    assert.ok(calls.every(args => args[2] === "info"));
    if (label === "existing evidence") assert.equal(fs.readFileSync(path.join(f.output, "sentinel"), "utf8"), "unchanged");
    else assert.equal(fs.existsSync(f.output), false);
  });
}

test("input acquisition refuses mutable tags before invoking Docker", t => {
  const f = fixture(t);
  const result = f.run("acquire-runtime-inputs.cjs", [f.output, "test-context", "test-daemon", "ubuntu:24.04"]);
  assert.notEqual(result.status, 0);
  assert.equal(fs.existsSync(f.log), false);
});

test("node probe rejects unknown or combined modes before Docker", t => {
  const f = fixture(t);
  for (const flags of [["--unknown"], ["--adapter", "--adapter"], ["--peer-node", "--peer-node"],
    ["--adapter", "--peer-node"], ["--peer-node-early", "--peer-node-early"],
    ["--peer-node", "--peer-node-early"]]) {
    const result = f.run("probe-node-image.cjs", [f.build, f.output, "test-context", "test-daemon", ...flags]);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Usage:/);
    assert.equal(fs.existsSync(f.log), false);
  }
});

for (const helper of ["prepare-adapter-probe.sh", "prepare-peer-probe.sh"]) {
  test(`${helper} has valid shell syntax and rejects arguments before touching state`, () => {
    const script = path.join(__dirname, helper);
    assert.equal(spawnSync("sh", ["-n", script]).status, 0);
    const result = spawnSync("sh", [script, "--unknown"], { encoding: "utf8" });
    assert.equal(result.status, 2);
    assert.match(result.stderr, /no arguments expected/);
  });
}

test("node probe records failed phase and cleanup without successful acceptance", t => {
  const f = fixture(t);
  const image = { kind: "compiled-runtime-image", target: "linux/arm64", public_release: false,
    image_id: `sha256:${"a".repeat(64)}`, download_sha256: hash("image") };
  fs.writeFileSync(path.join(f.build, "runtime-image.tar.gz"), "image");
  fs.writeFileSync(path.join(f.build, "runtime-image.tar.gz.sha256"), `${image.download_sha256}  runtime-image.tar.gz\n`);
  fs.writeFileSync(path.join(f.build, "image.json"), JSON.stringify(image));
  fs.writeFileSync(path.join(f.root, "bin/docker"), `#!${process.execPath}
const args = process.argv.slice(2);
if (args[2] === "info") console.log(args[4] === "{{.ID}}" ? "test-daemon" : "aarch64");
else if (args[2] === "image" && args[3] === "load") process.exit(0);
else if (args[2] === "volume" && args[3] === "create") process.exit(47);
else process.exit(99);
`, { mode: 0o755 });
  const result = f.run("probe-node-image.cjs", [f.build, f.output, "test-context", "test-daemon", "--peer-node"]);
  assert.notEqual(result.status, 0);
  const failure = JSON.parse(fs.readFileSync(path.join(f.output, "failure.json")));
  assert.equal(failure.kind, "compiled-node-container-failure");
  assert.equal(failure.accepted, false);
  assert.equal(failure.mode, "--peer-node");
  assert.equal(failure.phase, "preparation");
  assert.equal(failure.exit_status, 47);
  assert.equal(failure.cleanup_verified, true);
  assert.equal(failure.image_id, image.image_id);
  assert.match(failure.harness_sha256, /^[a-f0-9]{64}$/);
  assert.match(failure.peer_fixture_sha256, /^[a-f0-9]{64}$/);
  assert.equal(fs.existsSync(path.join(f.output, "result.json")), false);
});
