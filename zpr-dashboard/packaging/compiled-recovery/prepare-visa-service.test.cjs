const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { test } = require("node:test");

test("Visa preparation rejects unsafe state paths without overwriting anything", t => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "zpr-visa-prepare-")));
  t.after(() => fs.rmSync(root, { recursive: true }));
  const bin = path.join(root, "bin");
  fs.mkdirSync(bin);
  for (const name of ["vs", "vs-admin", "vsapikey", "zplc"]) {
    fs.writeFileSync(path.join(bin, name), "#!/bin/sh\nexit 99\n", { mode: 0o755 });
  }
  const prepare = (...args) => spawnSync("sh", [path.join(__dirname, "prepare-visa-service.sh"), ...args],
    { encoding: "utf8" });
  assert.equal(prepare().status, 2);
  for (const output of ["relative", `${root}/../escape`, `${root}/bad\npath`, `${root}/trailing/`]) {
    assert.equal(prepare(bin, output).status, 1);
  }
  const existing = path.join(root, "existing");
  fs.mkdirSync(existing);
  fs.writeFileSync(path.join(existing, "sentinel"), "unchanged");
  assert.match(prepare(bin, existing).stderr, /nothing was overwritten/);
  assert.deepEqual(fs.readdirSync(existing), ["sentinel"]);
  assert.equal(fs.readFileSync(path.join(existing, "sentinel"), "utf8"), "unchanged");
  const link = path.join(root, "link");
  fs.symlinkSync(existing, link);
  assert.match(prepare(bin, link).stderr, /nothing was overwritten/);
  assert.match(prepare(bin, path.join(link, "state")).stderr, /without symlink components/);
  const shared = path.join(root, "shared");
  fs.mkdirSync(shared);
  fs.chmodSync(shared, 0o777);
  assert.match(prepare(bin, path.join(shared, "state")).stderr, /not be group- or other-writable/);
  assert.equal(fs.existsSync(path.join(shared, "state")), false);
  fs.unlinkSync(path.join(bin, "vsapikey"));
  const fresh = path.join(root, "fresh");
  assert.match(prepare(bin, fresh).stderr, /missing regular compiled executable/);
  assert.equal(fs.existsSync(fresh), false);
});
