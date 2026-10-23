const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync, spawnSync } = require("node:child_process");

const [output, context, daemon, base, ...extra] = process.argv.slice(2);
assert.ok(output && context && daemon && base && !extra.length,
  "Usage: node acquire-runtime-inputs.cjs /absolute/new-output context daemon ubuntu@sha256:digest");
assert.ok(path.isAbsolute(output));
assert.match(base, /^ubuntu@sha256:[a-f0-9]{64}$/);
const docker = args => execFileSync("docker", ["--context", context, ...args], {
  encoding: "utf8", maxBuffer: 16 * 1024 * 1024,
}).trim();
assert.equal(docker(["info", "--format", "{{.ID}}"]), daemon, "Wrong Docker daemon");
assert.match(docker(["info", "--format", "{{.Architecture}}"]), /^(aarch64|arm64)$/);
docker(["pull", "--platform", "linux/arm64", base]);
const image = JSON.parse(docker(["image", "inspect", base]))[0];
assert.equal(image.Architecture, "arm64");
assert.equal(image.Os, "linux");
fs.mkdirSync(output, { mode: 0o700 });
const root = fs.realpathSync(output);
const packages = ["ca-certificates", "curl", "iproute2", "jq", "libgcc-s1",
  "libpcap0.8t64", "openssl", "redis-server", "util-linux"];
const id = docker(["create", "--platform", "linux/arm64", "--cap-drop", "ALL",
  "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--cap-add", "FOWNER",
  "--security-opt", "no-new-privileges:true", base, "sh", "-ec",
  `apt-get -o APT::Sandbox::User=root update &&
   apt-get -o APT::Sandbox::User=root --download-only --no-install-recommends -y install ${packages.join(" ")} &&
   for file in /var/cache/apt/archives/*.deb; do
     dpkg-deb -f "$file" Package Version Architecture > "$file.metadata"
   done`]);
const hash = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
try {
  const log = fs.openSync(path.join(root, "acquisition.log"), "wx", 0o600);
  let download;
  try {
    download = spawnSync("docker", ["--context", context, "start", "-a", id], { stdio: ["ignore", log, log] });
  } finally {
    fs.closeSync(log);
  }
  if (download.error) throw download.error;
  assert.equal(download.status, 0, "Dependency download failed; inspect acquisition.log");
  assert.equal(docker(["inspect", "--format", "{{.State.ExitCode}}", id]), "0",
    "Dependency download failed; inspect acquisition.log");
  docker(["cp", `${id}:/var/cache/apt/archives`, path.join(root, "packages")]);
  const files = fs.readdirSync(path.join(root, "packages")).filter(name => name.endsWith(".deb")).sort();
  assert.ok(files.length > 0, "No dependency packages downloaded");
  const inventory = files.map(name => {
    assert.match(name, /^[a-zA-Z0-9_.+%:~-]+\.deb$/);
    const file = path.join(root, "packages", name);
    assert.ok(fs.lstatSync(file).isFile());
    const fields = fs.readFileSync(`${file}.metadata`, "utf8").trim();
    return { file: name, sha256: hash(file), bytes: fs.statSync(file).size, metadata: fields };
  });
  docker(["image", "save", "-o", path.join(root, "base-image.tar"), image.Id]);
  const receipt = {
    schema_version: 1, kind: "compiled-runtime-inputs", target: "linux/arm64",
    acquisition_script_sha256: hash(__filename),
    base_reference: base, base_image_id: image.Id,
    base_archive_sha256: hash(path.join(root, "base-image.tar")),
    acquisition_network: "bridge", acquisition_capabilities: ["CHOWN", "DAC_OVERRIDE", "FOWNER"],
    requested_packages: packages, packages: inventory,
    public_release: false, license_review_complete: false,
  };
  fs.writeFileSync(path.join(root, "inputs.json"), `${JSON.stringify(receipt, null, 2)}\n`, { mode: 0o600 });
  console.log(`Saved ${files.length} dependency packages and immutable base to ${root}`);
} finally {
  docker(["rm", "-f", id]);
}
