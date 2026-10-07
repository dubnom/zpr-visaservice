import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:https";
import { mkdtempSync, readFileSync, writeFileSync, readdirSync, rmSync, chmodSync, mkdirSync, symlinkSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";

const [bundle, keyring, fingerprint, version] = process.argv.slice(2);
const scripts = dirname(fileURLToPath(import.meta.url));
const root = mkdtempSync(join(tmpdir(), "zpr-download-test-"));
chmodSync(root, 0o700);
const cert = join(root, "tls.crt");
const key = join(root, "tls.key");
execFileSync("openssl", ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
  "-subj", "/CN=localhost", "-addext", "subjectAltName=DNS:localhost",
  "-keyout", key, "-out", cert], { stdio: "ignore" });
const packageName = `zpr-enrollment-setup_${version}_amd64.deb`;
let scenario = "valid";
let requests = 0;
let raceDestination;
const server = createServer({ cert: readFileSync(cert), key: readFileSync(key) }, (request, response) => {
  requests++;
  const name = request.url.slice(1);
  if (scenario === "redirect") {
    response.writeHead(302, { Location: "/must-not-follow" });
    response.end();
    return;
  }
  if (scenario === "missing") {
    response.writeHead(404);
    response.end();
    return;
  }
  if (!["release.manifest", "release.manifest.asc", packageName].includes(name)) {
    response.writeHead(404);
    response.end();
    return;
  }
  let data = readFileSync(join(bundle, name));
  if (scenario === "oversize" && name === "release.manifest") data = Buffer.alloc(1025, "x");
  if (scenario === "tampered" && name === packageName) data = Buffer.concat([data, Buffer.from("x")]);
  if (scenario === "large-package" && name === packageName) data = Buffer.alloc(33554433, "x");
  if (scenario === "race" && name === packageName) {
    mkdirSync(raceDestination, { mode: 0o700 });
    writeFileSync(join(raceDestination, "existing"), "do-not-replace");
  }
  if (scenario === "truncated" && name === packageName) {
    response.writeHead(200, { "Content-Length": data.length + 1 });
    response.end(data);
    return;
  }
  // Chunked responses exercise actual body limits, not only Content-Length.
  response.writeHead(200);
  response.write(data);
  response.end();
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const base = `https://localhost:${server.address().port}`;
server.keepAliveTimeout = 100;
let sequence = 0;
async function run(url = base, pin = fingerprint, expected = version, trust = cert, output) {
  const destination = output || join(root, `release-${sequence++}`);
  const child = spawn("sh", [join(scripts, "download-release.sh"), url, keyring, pin, expected, destination, trust], {
    env: { ...process.env, HTTPS_PROXY: "http://127.0.0.1:1", ALL_PROXY: "http://127.0.0.1:1",
      CURL_CA_BUNDLE: "/nonexistent", CURL_HOME: root, HOME: root },
    stdio: ["ignore", "pipe", "pipe"]
  });
  let log = "";
  child.stdout.on("data", (data) => { log += data; });
  child.stderr.on("data", (data) => { log += data; });
  const code = await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", resolve);
  });
  return { code, destination, log };
}
try {
  // A user's curl config must not enable redirects/insecure TLS/proxy overrides.
  writeFileSync(join(root, ".curlrc"), "insecure\nlocation\nproxy = \"http://127.0.0.1:1\"\n");
  const valid = await run();
  assert.equal(valid.code, 0, valid.log);
  assert.deepEqual(readdirSync(valid.destination).sort(), [packageName, "release.manifest", "release.manifest.asc"].sort());
  assert.deepEqual(readFileSync(join(valid.destination, packageName)), readFileSync(join(bundle, packageName)));
  const again = await run(base, fingerprint, version, cert, valid.destination);
  assert.notEqual(again.code, 0);
  for (const test of ["redirect", "missing", "oversize", "large-package", "truncated", "tampered", "untrusted-tls", "wrong-signer", "plaintext"]) {
    scenario = test;
    requests = 0;
    const result = await run(test === "plaintext" ? base.replace("https:", "http:") : base,
      test === "wrong-signer" ? "0".repeat(40) : fingerprint, version,
      test === "untrusted-tls" ? "system" : cert);
    assert.notEqual(result.code, 0, `accepted ${test}`);
    assert.ok(!readdirSync(root).includes(result.destination.split("/").at(-1)), `published ${test}`);
    assert.ok(!readdirSync(root).some((name) => name.startsWith(".zpr-download-")), `staging leaked for ${test}`);
    if (test === "redirect") assert.equal(requests, 1, "followed redirect");
    console.log(`PASS: download rejected ${test}, no bundle published`);
  }
  scenario = "valid";
  const unsafe = join(root, "unsafe");
  mkdirSync(unsafe, { mode: 0o755 });
  chmodSync(unsafe, 0o755);
  assert.notEqual((await run(base, fingerprint, version, cert, join(unsafe, "release"))).code, 0);
  const alias = join(root, "alias");
  symlinkSync(root, alias);
  assert.notEqual((await run(base, fingerprint, version, cert, join(alias, "release"))).code, 0);
  scenario = "race";
  raceDestination = join(root, "concurrent-output");
  const race = await run(base, fingerprint, version, cert, raceDestination);
  assert.notEqual(race.code, 0, race.log);
  assert.deepEqual(readdirSync(raceDestination), ["existing"]);
  assert.equal(readFileSync(join(raceDestination, "existing"), "utf8"), "do-not-replace");
  assert.ok(!readdirSync(root).some((name) => name.startsWith(".zpr-download-")));
  console.log("PASS: unsafe/symlink parents and concurrent output rejected without replacement");
  console.log("HTTPS staging, proxy/curl-config isolation, signature verification, and cleanup checks passed.");
} finally {
  await new Promise((resolve) => server.close(resolve));
  rmSync(root, { recursive: true });
}
