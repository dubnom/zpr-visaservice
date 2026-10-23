const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { test } = require("node:test");
const { spawnSync } = require("node:child_process");
const { verifyRelease } = require("./verify-rust-release.cjs");

test("release verifier requires optimized receipts, exact hashes and ARM64 binaries", t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "zpr-release-verifier-"));
  t.after(() => fs.rmSync(root, { recursive: true }));
  fs.mkdirSync(path.join(root, "bin"));
  fs.mkdirSync(path.join(root, "receipts"));
  const inputs = {
    kind: "offline-release-build-inputs", target: "linux/arm64", profile: "release",
    locked: true, network: "none", builder_image: `sha256:${"a".repeat(64)}`,
    repositories: ["zpr-core", "zpr-common", "zpr-visaservice", "zpr-compiler"].map(repository => ({
      repository, revision: "a".repeat(40), archive_sha256: "b".repeat(64), lockfile_sha256: "c".repeat(64),
    })),
  };
  const binary = Buffer.alloc(64);
  Buffer.from([127, 69, 76, 70, 2, 1]).copy(binary);
  binary.writeUInt16LE(183, 18);
  const hash = crypto.createHash("sha256").update(binary).digest("hex");
  const checksums = [];
  for (const [repository, names] of Object.entries({
    "zpr-core": ["ph", "ph-cli"], "zpr-visaservice": ["vs", "vs-admin", "vsapikey", "zpt"], "zpr-compiler": ["zplc"],
  })) {
    const messages = names.map(name => ({
      reason: "compiler-artifact", target: { name, kind: ["bin"] },
      executable: `/build/release/target/${repository}/release/${name}`,
      profile: { opt_level: "3", debug_assertions: false, test: false },
    }));
    messages.push({ reason: "build-finished", success: true });
    fs.writeFileSync(path.join(root, `receipts/${repository}-artifacts.jsonl`), messages.map(x => JSON.stringify(x)).join("\n"));
    for (const name of names) {
      fs.writeFileSync(path.join(root, "bin", name), binary, { mode: 0o755 });
      checksums.push(`${hash}  bin/${name}`);
    }
  }
  fs.writeFileSync(path.join(root, "receipts/binaries.sha256"), checksums.join("\n"));
  fs.writeFileSync(path.join(root, "receipts/lockfiles.sha256"), inputs.repositories.map(x =>
    `${x.lockfile_sha256}  /input/sources/${x.repository}/Cargo.lock`).join("\n"));
  assert.equal(verifyRelease(root, inputs).length, 7);
  const stage = path.join(root, "stage");
  fs.mkdirSync(stage);
  fs.mkdirSync(path.join(stage, "bin"));
  const metadata = { kind: "compiled-recovery-engineering-slice", public_release: false, complete_installation: false };
  fs.writeFileSync(path.join(stage, "build.json"), JSON.stringify(metadata));
  const inputFile = path.join(root, "inputs.json");
  inputs.private_runtime_file = "/private/do-not-copy";
  fs.writeFileSync(inputFile, JSON.stringify(inputs));
  const assemble = () => spawnSync(process.execPath,
    [path.join(__dirname, "assemble-rust-release.cjs"), root, inputFile, stage], { encoding: "utf8" });
  const assembly = assemble();
  assert.equal(assembly.status, 0, assembly.stderr);
  assert.deepEqual(fs.readdirSync(path.join(stage, "bin")).sort(),
    ["ph", "ph-cli", "vs", "vs-admin", "vsapikey", "zplc", "zpt"]);
  const report = JSON.parse(fs.readFileSync(path.join(stage, "rust-release.json")));
  assert.equal(report.binaries.length, 7);
  assert.equal(report.private_runtime_file, undefined);
  assert.ok(!JSON.stringify(report).includes("/private/do-not-copy"));
  assert.equal(report.complete_installation, false);
  assert.equal(assemble().status, 1, "Existing assembly must be refused");

  const receiptPath = path.join(root, "receipts/zpr-core-artifacts.jsonl");
  const original = fs.readFileSync(receiptPath, "utf8");
  for (const [field, value] of [["opt_level", "0"], ["debug_assertions", true], ["test", true]]) {
    const messages = original.split("\n").map(x => JSON.parse(x));
    messages[0].profile[field] = value;
    fs.writeFileSync(receiptPath, messages.map(x => JSON.stringify(x)).join("\n"));
    assert.throws(() => verifyRelease(root, inputs));
    assert.equal(assemble().status, 1, `Assembly must reject ${field} without copying`);
  }
  fs.writeFileSync(receiptPath, original);
  const executable = path.join(root, "bin/ph");
  fs.appendFileSync(executable, "tampered");
  assert.throws(() => verifyRelease(root, inputs), /digest mismatch/);
  assert.match(assemble().stderr, /digest mismatch/);
  fs.writeFileSync(executable, binary);
  const wrongArch = Buffer.from(binary);
  wrongArch.writeUInt16LE(62, 18);
  fs.writeFileSync(executable, wrongArch);
  assert.throws(() => verifyRelease(root, inputs), /Not ARM64/);
  fs.writeFileSync(executable, binary);
  inputs.repositories[0].lockfile_sha256 = "d".repeat(64);
  assert.throws(() => verifyRelease(root, inputs), /Lockfile mismatch/);
  fs.writeFileSync(inputFile, JSON.stringify(inputs));
  assert.match(assemble().stderr, /Lockfile mismatch/);
  inputs.repositories[0].lockfile_sha256 = "c".repeat(64);
  fs.writeFileSync(inputFile, JSON.stringify(inputs));
  fs.unlinkSync(receiptPath);
  assert.match(assemble().stderr, /ENOENT/);
});
