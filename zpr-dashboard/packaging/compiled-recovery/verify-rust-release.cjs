const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

function verifyRelease(root, inputs) {
  assert.equal(inputs.kind, "offline-release-build-inputs");
  assert.equal(inputs.target, "linux/arm64");
  assert.equal(inputs.profile, "release");
  assert.equal(inputs.locked, true);
  assert.equal(inputs.network, "none");
  assert.match(inputs.builder_image, /^sha256:[a-f0-9]{64}$/);
  const expected = {
    "zpr-core": ["ph", "ph-cli"],
    "zpr-visaservice": ["vs", "vs-admin", "vsapikey", "zpt"],
    "zpr-compiler": ["zplc"],
  };
  const names = Object.values(expected).flat().sort();
  assert.deepEqual(fs.readdirSync(path.join(root, "bin")).sort(), names);
  const hash = content => crypto.createHash("sha256").update(content).digest("hex");
  const hashes = new Map();
  for (const line of fs.readFileSync(path.join(root, "receipts/binaries.sha256"), "utf8").trim().split("\n")) {
    const match = /^([a-f0-9]{64}) {2}bin\/([a-z-]+)$/.exec(line);
    assert.ok(match && !hashes.has(match[2]), "Invalid or duplicate binary digest");
    hashes.set(match[2], match[1]);
  }
  assert.deepEqual([...hashes.keys()].sort(), names);
  const locks = new Map();
  for (const line of fs.readFileSync(path.join(root, "receipts/lockfiles.sha256"), "utf8").trim().split("\n")) {
    const match = /^([a-f0-9]{64}) {2}\/input\/sources\/([a-z-]+)\/Cargo.lock$/.exec(line);
    assert.ok(match && !locks.has(match[2]), "Invalid or duplicate lockfile digest");
    locks.set(match[2], match[1]);
  }
  for (const repository of ["zpr-core", "zpr-common", "zpr-visaservice", "zpr-compiler"]) {
    const source = inputs.repositories.find(item => item.repository === repository);
    assert.ok(source, `Source provenance missing: ${repository}`);
    assert.match(source.revision, /^[a-f0-9]{40}$/);
    assert.match(source.archive_sha256, /^[a-f0-9]{64}$/);
    assert.equal(locks.get(repository), source.lockfile_sha256, `Lockfile mismatch: ${repository}`);
  }
  const verified = [];
  for (const [repository, binaries] of Object.entries(expected)) {
    const messages = fs.readFileSync(path.join(root, `receipts/${repository}-artifacts.jsonl`), "utf8")
      .trim().split("\n").map(line => JSON.parse(line));
    assert.ok(messages.some(message => message.reason === "build-finished" && message.success === true),
      `Build completion receipt missing: ${repository}`);
    for (const name of binaries) {
      const artifacts = messages.filter(message => message.reason === "compiler-artifact" &&
        message.target?.name === name && message.target.kind.includes("bin") && message.executable);
      assert.equal(artifacts.length, 1, `Unique executable build receipt required: ${name}`);
      const artifact = artifacts[0];
      assert.equal(artifact.profile.opt_level, "3", `Not release optimized: ${name}`);
      assert.equal(artifact.profile.debug_assertions, false, `Debug assertions enabled: ${name}`);
      assert.equal(artifact.profile.test, false, `Test executable supplied: ${name}`);
      assert.equal(artifact.executable, `/build/release/target/${repository}/release/${name}`);
      const binaryPath = path.join(root, "bin", name);
      const binary = fs.readFileSync(binaryPath);
      assert.ok(fs.lstatSync(binaryPath).isFile() && !fs.lstatSync(binaryPath).isSymbolicLink());
      assert.ok(binary.subarray(0, 6).equals(Buffer.from([127, 69, 76, 70, 2, 1])));
      assert.equal(binary.readUInt16LE(18), 183, `Not ARM64 ELF: ${name}`);
      assert.equal(hash(binary), hashes.get(name), `Binary digest mismatch: ${name}`);
      assert.ok(fs.statSync(binaryPath).mode & 0o111, `Not executable: ${name}`);
      verified.push({ name, repository, sha256: hashes.get(name), bytes: binary.length });
    }
  }
  return verified;
}

if (require.main === module) {
  const [root, inputFile, ...extra] = process.argv.slice(2);
  assert.ok(root && inputFile && extra.length === 0,
    "Usage: node verify-rust-release.cjs /absolute/release /absolute/release-inputs.json");
  assert.ok(path.isAbsolute(root) && path.isAbsolute(inputFile));
  const verified = verifyRelease(root, JSON.parse(fs.readFileSync(inputFile, "utf8")));
  console.log(JSON.stringify({ kind: "verified-rust-release-build", binaries: verified }, null, 2));
}

module.exports = { verifyRelease };
