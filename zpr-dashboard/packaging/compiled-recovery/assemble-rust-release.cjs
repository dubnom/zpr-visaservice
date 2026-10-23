const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const { verifyRelease } = require("./verify-rust-release.cjs");

const [release, inputFile, stage, ...extra] = process.argv.slice(2);
assert.ok(release && inputFile && stage && extra.length === 0,
  "Usage: node assemble-rust-release.cjs /absolute/release /absolute/release-inputs.json /absolute/stage");
assert.ok([release, inputFile, stage].every(file => path.isAbsolute(file)));
const inputs = JSON.parse(fs.readFileSync(inputFile, "utf8"));
const binaries = verifyRelease(release, inputs);
assert.ok(!fs.existsSync(path.join(stage, "rust-release.json")), "Rust assembly already exists");
const metadataPath = path.join(stage, "build.json");
const metadata = JSON.parse(fs.readFileSync(metadataPath, "utf8"));
assert.equal(metadata.kind, "compiled-recovery-engineering-slice");
assert.equal(metadata.public_release, false);
assert.equal(metadata.complete_installation, false);
for (const binary of binaries) {
  assert.ok(!fs.existsSync(path.join(stage, "bin", binary.name)), "Refusing executable overwrite");
}
for (const binary of binaries) {
  const destination = path.join(stage, "bin", binary.name);
  fs.copyFileSync(path.join(release, "bin", binary.name), destination, fs.constants.COPYFILE_EXCL);
  assert.equal(crypto.createHash("sha256").update(fs.readFileSync(destination)).digest("hex"),
    binary.sha256, `Staged executable digest mismatch: ${binary.name}`);
  fs.chmodSync(destination, 0o755);
}
const report = {
  schema_version: 1, kind: "verified-rust-release-build", target: "linux/arm64",
  builder_image: inputs.builder_image, profile: inputs.profile, locked: inputs.locked,
  network: inputs.network, build_script_sha256: inputs.build_script_sha256,
  repositories: inputs.repositories.map(({ repository, revision, archive_sha256, lockfile_sha256 }) =>
    ({ repository, revision, archive_sha256, lockfile_sha256 })),
  binaries, runtime_prerequisites: ["Linux ARM64 glibc loader", "OpenSSL 3", "libpcap.so.0.8", "libgcc_s.so.1"],
  complete_installation: false, dependency_license_review: "pending",
};
fs.writeFileSync(path.join(stage, "rust-release.json"), `${JSON.stringify(report, null, 2)}\n`, { flag: "wx" });
metadata.rust_release = { manifest: "rust-release.json", binary_count: binaries.length };
fs.writeFileSync(metadataPath, `${JSON.stringify(metadata, null, 2)}\n`);
console.log("Assembled seven verified release tools; no Cargo sources, caches or private runtime evidence included.");
