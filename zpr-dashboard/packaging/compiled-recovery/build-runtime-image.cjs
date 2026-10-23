const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const [build, inputs, output, context, daemon, ...extra] = process.argv.slice(2);
assert.ok(build && inputs && output && context && daemon && !extra.length,
  "Usage: node build-runtime-image.cjs /absolute/compiled-build /absolute/inputs /absolute/new-output context daemon");
for (const directory of [build, inputs, output]) assert.ok(path.isAbsolute(directory));
const hash = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
const run = (command, args) => execFileSync(command, args, {
  encoding: "utf8", maxBuffer: 32 * 1024 * 1024,
}).trim();
const docker = args => run("docker", ["--context", context, ...args]);
assert.equal(docker(["info", "--format", "{{.ID}}"]), daemon, "Wrong Docker daemon");
assert.match(docker(["info", "--format", "{{.Architecture}}"]), /^(aarch64|arm64)$/);
const receipt = JSON.parse(fs.readFileSync(path.join(inputs, "inputs.json")));
assert.equal(receipt.schema_version, 1);
assert.equal(receipt.kind, "compiled-runtime-inputs");
assert.equal(receipt.target, "linux/arm64");
assert.match(receipt.base_reference, /^ubuntu@sha256:[a-f0-9]{64}$/);
assert.match(receipt.base_image_id, /^sha256:[a-f0-9]{64}$/);
assert.equal(hash(path.join(inputs, "base-image.tar")), receipt.base_archive_sha256);
assert.ok(Array.isArray(receipt.packages) && receipt.packages.length > 0);
const names = new Set();
for (const entry of receipt.packages) {
  assert.match(entry.file, /^[a-zA-Z0-9_.+%:~-]+\.deb$/);
  assert.match(entry.metadata, /^Architecture: (arm64|all)$/m);
  assert.ok(!names.has(entry.file), "Duplicate package");
  names.add(entry.file);
  const file = path.join(inputs, "packages", entry.file);
  assert.ok(fs.lstatSync(file).isFile(), "Package must be regular");
  assert.equal(fs.statSync(file).size, entry.bytes);
  assert.equal(hash(file), entry.sha256);
}
const archive = path.join(build, "compiled-recovery-linux-arm64.tar.gz");
const archiveHash = hash(archive);
assert.equal(fs.readFileSync(`${archive}.sha256`, "utf8").trim(),
  `${archiveHash}  compiled-recovery-linux-arm64.tar.gz`);
const smoke = JSON.parse(fs.readFileSync(path.join(build, "smoke-evidence/result.json")));
assert.equal(smoke.archive_sha256, archiveHash, "Archive has no matching offline acceptance receipt");
assert.equal(smoke.target, "linux/arm64");
assert.equal(smoke.rust_binaries_included, true);
assert.deepEqual(smoke.checks, [
  ...["great-lakes", "load-lab", "northstar", "redwood", "velocity", "lab-one", "lab-two"]
    .map(name => ({ name, exit_code: 0 })),
  { name: "refuse-existing", exit_code: 1 },
]);
const members = run("tar", ["-tzf", archive]).split("\n");
for (const member of members) {
  assert.ok(member.startsWith("./") && !member.split("/").includes(".."), "Unsafe archive path");
}
assert.ok(run("tar", ["-tvzf", archive]).split("\n").every(line => /^[-d]/.test(line)),
  "Archive must contain regular files/directories only");
fs.mkdirSync(output, { mode: 0o700 });
const root = fs.realpathSync(output);
const stage = path.join(root, "context");
fs.mkdirSync(stage);
const runtime = path.join(stage, "runtime");
fs.mkdirSync(runtime);
run("tar", ["-xzf", archive, "-C", runtime]);
const inventory = new Map();
for (const line of fs.readFileSync(path.join(runtime, "SHA256SUMS"), "utf8").trim().split("\n")) {
  const match = /^([a-f0-9]{64}) {2}\.\/([a-zA-Z0-9_./-]+)$/.exec(line);
  assert.ok(match && !match[2].split("/").includes(".."));
  assert.ok(!inventory.has(match[2]), "Duplicate archive inventory entry");
  inventory.set(match[2], match[1]);
}
function verify(directory, prefix = "") {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const name = prefix + entry.name;
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) verify(file, `${name}/`);
    else {
      assert.ok(entry.isFile(), "Non-regular runtime input");
      if (name === "SHA256SUMS") continue;
      assert.equal(hash(file), inventory.get(name), `Inventory mismatch: ${name}`);
      inventory.delete(name);
    }
  }
}
verify(runtime);
assert.equal(inventory.size, 0, "Missing runtime input");
const metadata = JSON.parse(fs.readFileSync(path.join(runtime, "build.json")));
assert.equal(metadata.source_revision, smoke.source_revision);
assert.equal(metadata.target, "linux/arm64");
assert.equal(metadata.public_release, false);
assert.equal(metadata.complete_installation, false);
assert.equal(metadata.rust_release.binary_count, 7);
fs.mkdirSync(path.join(stage, "packages"));
for (const name of names) fs.copyFileSync(path.join(inputs, "packages", name), path.join(stage, "packages", name));
const baseTag = `zpr-runtime-base:${receipt.base_image_id.slice(7)}`;
fs.writeFileSync(path.join(stage, "Dockerfile"), `FROM ${baseTag}
ENV DEBIAN_FRONTEND=noninteractive
RUN --mount=type=bind,source=packages,target=/packages \\
    printf '#!/bin/sh\\nexit 101\\n' > /usr/sbin/policy-rc.d && chmod 755 /usr/sbin/policy-rc.d && \\
    dpkg -i /packages/*.deb && \\
    mkdir -p /state /opt/zpr && chown 501:1000 /state && chmod 700 /state && \\
    rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/* && \\
    dpkg-query -W > /opt/zpr/runtime-packages.txt
COPY runtime/ /opt/zpr/
ENV PATH=/opt/zpr/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
WORKDIR /state
USER 501:1000
CMD ["/opt/zpr/bin/vs", "--help"]
`);
docker(["image", "load", "-i", path.join(inputs, "base-image.tar")]);
assert.equal(docker(["image", "inspect", "--format", "{{.Id}}", receipt.base_image_id]), receipt.base_image_id);
docker(["image", "tag", receipt.base_image_id, baseTag]);
const tag = `zpr-compiled-runtime:${crypto.randomBytes(8).toString("hex")}`;
const iid = path.join(root, "image-id.txt");
docker(["build", "--network", "none", "--pull=false", "--platform", "linux/arm64",
  "--iidfile", iid, "-t", tag, stage]);
const imageID = fs.readFileSync(iid, "utf8").trim();
const image = JSON.parse(docker(["image", "inspect", imageID]))[0];
assert.equal(image.Architecture, "arm64");
assert.equal(image.Config.User, "501:1000");
const saved = path.join(root, "runtime-image.tar");
docker(["image", "save", "-o", saved, imageID]);
run("gzip", ["-n", saved]);
const result = {
  schema_version: 1, kind: "compiled-runtime-image", target: "linux/arm64",
  archive_sha256: archiveHash, inputs_sha256: hash(path.join(inputs, "inputs.json")),
  source_revision: metadata.source_revision, builder_sha256: hash(__filename),
  dockerfile_sha256: hash(path.join(stage, "Dockerfile")),
  smoke_receipt_sha256: hash(path.join(build, "smoke-evidence/result.json")),
  base_image_id: receipt.base_image_id, image_id: imageID, image_tag: tag,
  assembly_network: "none", engine_reported_image_bytes: image.Size,
  download_bytes: fs.statSync(`${saved}.gz`).size, download_sha256: hash(`${saved}.gz`),
  user: "501:1000", public_release: false, complete_installation: false,
  license_review_complete: false,
};
fs.writeFileSync(path.join(root, "runtime-image.tar.gz.sha256"),
  `${result.download_sha256}  runtime-image.tar.gz\n`, { mode: 0o600 });
fs.writeFileSync(path.join(root, "image.json"), `${JSON.stringify(result, null, 2)}\n`, { mode: 0o600 });
console.log(JSON.stringify(result, null, 2));
