const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const [build, output, context, daemon, ...extra] = process.argv.slice(2);
assert.ok(build && output && context && daemon && (!extra.length || (extra.length === 1 && extra[0] === "--adapter")),
  "Usage: node probe-node-image.cjs /absolute/image-build /absolute/new-evidence context daemon [--adapter]");
const adapterMode = extra[0] === "--adapter";
assert.ok(path.isAbsolute(build) && path.isAbsolute(output));
const hash = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
const docker = args => execFileSync("docker", ["--context", context, ...args], {
  encoding: "utf8", maxBuffer: 8 * 1024 * 1024,
}).trim();
assert.equal(docker(["info", "--format", "{{.ID}}"]), daemon, "Wrong Docker daemon");
assert.match(docker(["info", "--format", "{{.Architecture}}"]), /^(aarch64|arm64)$/);
const image = JSON.parse(fs.readFileSync(path.join(build, "image.json")));
assert.equal(image.kind, "compiled-runtime-image");
assert.equal(image.target, "linux/arm64");
assert.equal(image.public_release, false);
assert.match(image.image_id, /^sha256:[a-f0-9]{64}$/);
const saved = path.join(build, "runtime-image.tar.gz");
assert.equal(hash(saved), image.download_sha256);
assert.equal(fs.readFileSync(`${saved}.sha256`, "utf8").trim(),
  `${image.download_sha256}  runtime-image.tar.gz`);
assert.ok(!fs.existsSync(output), "Existing evidence is never reused");
docker(["image", "load", "-i", saved]);
fs.mkdirSync(output, { mode: 0o700 });
const root = fs.realpathSync(output);
const token = crypto.randomBytes(8).toString("hex");
const containers = [];
const diagnostics = [];
const volumes = [];
let network;
let acceptance;
let adapter;
const exec = (id, script) => docker(["exec", id, "sh", "-ec", script]);
const admin = "vs-admin --svc-url 'https://[::1]:8182' --ca-cert /state/first/authority.pem " +
  "--api-key-file /state/first/administrator.key --format compact";
function volume() {
  const name = docker(["volume", "create", "--label", `zpr.node-probe=${token}`]);
  volumes.push(name);
  return name;
}
function create(name, mounts, net, address, device = false, capabilities = [], user = "501:1000") {
  const id = docker(["create", "--name", `zpr-node-${token}-${name}`,
    "--network", net, ...(address ? ["--ip6", address] : []),
    "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
    "--user", user,
    ...capabilities.flatMap(capability => ["--cap-add", capability]),
    "--pids-limit", "128", "--memory", "512m", "--cpus", "1",
    ...(device ? ["--device", "/dev/net/tun:/dev/net/tun:rw"] : []),
    ...mounts.flatMap(mount => ["--mount", mount]),
    image.image_id, "sleep", "600"]);
  containers.push(id);
  docker(["start", id]);
  assert.equal(docker(["inspect", "--format", "{{.State.Running}}", id]), "true");
  return id;
}
function setup(id, script, tun = false) {
  const helper = docker(["create", "--network", `container:${id}`, "--user", "0:0",
    "--read-only", "--cap-drop", "ALL", "--cap-add", "NET_ADMIN",
    "--security-opt", "no-new-privileges:true", "--pids-limit", "32",
    ...(tun ? ["--device", "/dev/net/tun:/dev/net/tun:rw"] : []),
    image.image_id, "sh", "-ec", `grep -Eq '^CapEff:[[:space:]]+0*1000$' /proc/self/status\n${script}`]);
  containers.push(helper);
  docker(["start", "-a", helper]);
  assert.equal(docker(["inspect", "--format", "{{.State.ExitCode}}", helper]), "0", "Network setup failed");
}
function launch(id, name, command) {
  docker(["exec", "-d", id, "sh", "-ec",
    `umask 077; echo $$ > /state/${name}.pid; exec ${command} > /state/${name}.log 2>&1`]);
  diagnostics.push([id, name]);
}
function stop(id, name) {
  exec(id, `kill "$(cat /state/${name}.pid)"
timeout 10 sh -ec 'while [ -e "/proc/$(cat /state/${name}.pid)" ]; do sleep 0.2; done'`);
}
function copy(id, source, target) {
  const file = path.join(root, target);
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  docker(["cp", `${id}:${source}`, file]);
  fs.chmodSync(file, 0o600);
}
function tunFailure(node, nodeVolume, label, device, reason) {
  const id = docker(["create", "--network", `container:${node}`, "--read-only", "--cap-drop", "ALL",
    "--security-opt", "no-new-privileges:true", "--pids-limit", "128", "--memory", "512m",
    ...(device ? ["--device", "/dev/net/tun:/dev/net/tun:rw"] : []),
    "--mount", `type=volume,src=${nodeVolume},dst=/state`, image.image_id, "sh", "-ec", `umask 077
set +e
timeout 10 ph node -c /state/first/node.toml --logging all=INFO > /state/${label}.log 2>&1
status=$?
set -e
printf 'PH TUN-negative exit: %s\\n' "$status"
grep 'unable to create TUN device' /state/${label}.log > /state/${label}.txt
cat /state/${label}.txt
test "$status" = 134
grep -F '${reason}' /state/${label}.txt`]);
  containers.push(id);
  docker(["start", "-a", id]);
  assert.equal(docker(["inspect", "--format", "{{.State.ExitCode}}", id]), "0", `${label} did not fail at TUN with exit 134`);
  copy(node, `/state/${label}.txt`, `${label}.txt`);
}
function waitSync(visa) {
  exec(visa, `timeout 45 sh -ec 'until ${admin} actors --cn node > /state/actor.json 2>/state/query.log &&
jq -e '\\''.cn == "node" and .node == true and .zpr_addr == "fd5a:5052::2" and .node_details.in_sync == true and .node_details.pending_install == 0'\\'' /state/actor.json >/dev/null; do sleep 0.2; done'
jq '{cn, node, zpr_addr, node_details: {in_sync: .node_details.in_sync, pending_install: .node_details.pending_install, vss_port: .node_details.vss_port}}' /state/actor.json > /state/summary.json`);
}
try {
  const visaVolume = volume();
  const nodeVolume = volume();
  network = docker(["network", "create", "--internal", "--ipv6", "--ipv4=false",
    "--subnet", "fd00:5a::/64", "--label", `zpr.node-probe=${token}`, `zpr-node-${token}`]);
  const net = JSON.parse(docker(["network", "inspect", network]))[0];
  assert.equal(net.Internal, true);
  assert.equal(net.EnableIPv6, true);
  assert.equal(net.EnableIPv4, false);
  const visa = create("visa", [`type=volume,src=${visaVolume},dst=/state`], network, "fd00:5a::3");
  const node = create("node", [`type=volume,src=${nodeVolume},dst=/state`], network, "fd00:5a::2", true);
  exec(visa, `umask 077
sh /opt/zpr/libexec/prepare-visa-service.sh /opt/zpr/bin /state/first --node > /state/prepare.log 2>&1
sh /opt/zpr/libexec/prepare-visa-service.sh /opt/zpr/bin /state/second --node > /state/prepare-second.log 2>&1`);
  if (adapterMode) {
    execFileSync("docker", ["--context", context, "exec", "-i", visa, "sh", "-s"], {
      input: fs.readFileSync(path.join(__dirname, "prepare-adapter-probe.sh")),
      encoding: "utf8", maxBuffer: 8 * 1024 * 1024,
    });
    const adapterVolume = volume();
    adapter = create("adapter", [`type=volume,src=${adapterVolume},dst=/state`], network, "fd00:5a::4", true, ["NET_ADMIN"], "0:0");
    const helper = docker(["create", "--network", "none", "--read-only", "--cap-drop", "ALL",
      "--user", "0:0", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE",
      "--security-opt", "no-new-privileges:true",
      "--mount", `type=volume,src=${visaVolume},dst=/input,readonly`,
      "--mount", `type=volume,src=${adapterVolume},dst=/state`, image.image_id, "sh", "-ec", `umask 077
chown 0:0 /state
mkdir /state/adapter
for file in authority.pem noise.key noise.pem bootstrap.key config.toml; do
  cp "/input/adapter/$file" "/state/adapter/$file"
done
cp /input/second/authority.pem /state/adapter/foreign-authority.pem
cp /input/second/node.key /state/adapter/foreign-bootstrap.key`]);
    containers.push(helper);
    docker(["start", "-a", helper]);
    assert.equal(docker(["inspect", "--format", "{{.State.ExitCode}}", helper]), "0");
    setup(adapter, `ip tuntap add name zprclient0 mode tun multi_queue user 501
ip link set zprclient0 mtu 1400 up
ip -6 route add fd5a:5052::/32 dev zprclient0`, true);
    exec(adapter, `index=$(cat /sys/class/net/eth0/ifindex)
sed "s|\\[fd00:5a::4\\]:5000|[fd00:5a::4%$index]:5000|" /state/adapter/config.toml > /state/adapter/scoped.toml`);
    exec(adapter, `umask 077
sed 's|^ca_file = .*|ca_file = "/state/adapter/foreign-authority.pem"|' /state/adapter/scoped.toml > /state/adapter/foreign-ca.toml
sed 's|^bootstrap_key = .*|bootstrap_key = "/state/adapter/foreign-bootstrap.key"|' /state/adapter/scoped.toml > /state/adapter/foreign-key.toml`);
  }
  const transfer = docker(["create", "--network", "none", "--read-only", "--cap-drop", "ALL",
    "--security-opt", "no-new-privileges:true",
    "--mount", `type=volume,src=${visaVolume},dst=/input,readonly`,
    "--mount", `type=volume,src=${nodeVolume},dst=/state`,
    image.image_id, "sh", "-ec", `umask 077
mkdir /state/first
for file in authority.pem node-noise.key node-noise.pem node.key node.toml; do
  cp "/input/first/$file" "/state/first/$file"
done
cp /input/second/node.key /state/foreign.key
sed 's|^auth_private_key = .*|auth_private_key = "/state/foreign.key"|' /state/first/node.toml > /state/foreign.toml`]);
  containers.push(transfer);
  docker(["start", "-a", transfer]);
  assert.equal(docker(["inspect", "--format", "{{.State.ExitCode}}", transfer]), "0");
  setup(visa, `ip -6 addr add fd5a:5052::1/128 dev lo nodad
ip -6 route add fd5a:5052::2/128 via fd00:5a::2 dev eth0 src fd5a:5052::1`);
  setup(node, `ip tuntap add name zprtest0 mode tun multi_queue user 502
ip link set zprtest0 mtu 1400 up
ip -6 addr add fd5a:5052::2/128 dev zprtest0 nodad
ip -6 route add fd5a:5052::1/128 via fd00:5a::3 dev eth0 src fd5a:5052::2`, true);
  tunFailure(node, nodeVolume, "missing-device", false, "No such file or directory");
  tunFailure(node, nodeVolume, "wrong-tun-owner", true, "Operation not permitted");
  setup(node, `ip link del zprtest0
ip tuntap add name zprtest0 mode tun multi_queue user 501
ip link set zprtest0 mtu 1400 up
ip -6 addr add fd5a:5052::2/128 dev zprtest0 nodad`, true);
  if (adapterMode) setup(node, "ip -6 route add fd5a:5052:adda::/48 dev zprtest0");
  for (const [name, id] of [["visa", visa], ["node", node]]) {
    const config = JSON.parse(docker(["inspect", id]))[0];
    assert.equal(config.Config.User, "501:1000");
    assert.equal(config.HostConfig.Privileged, false);
    assert.equal(config.HostConfig.ReadonlyRootfs, true);
    assert.deepEqual(config.HostConfig.CapDrop, ["ALL"]);
    assert.ok(!config.HostConfig.CapAdd?.length);
    assert.ok(config.HostConfig.SecurityOpt.includes("no-new-privileges:true"));
    assert.ok(!config.HostConfig.PortBindings || !Object.keys(config.HostConfig.PortBindings).length);
    assert.equal(config.Mounts.length, 1);
    assert.equal(config.Mounts[0].Type, "volume");
    assert.equal(config.Mounts[0].Name, name === "visa" ? visaVolume : nodeVolume);
    assert.equal(config.Mounts[0].Destination, "/state");
    assert.equal(config.HostConfig.Devices?.length || 0, name === "node" ? 1 : 0);
    exec(id, `umask 077
test "$(id -u)" = 501
grep -Eq '^CapEff:[[:space:]]+0+$' /proc/self/status
grep -Eq '^NoNewPrivs:[[:space:]]+1$' /proc/self/status
test -z "$(ip -6 route show default)"
test -z "$(ip -4 route show default)"
test ! -e /var/run/docker.sock
ip -6 route show > /state/routes.txt
ip -6 addr show > /state/addresses.txt
sha256sum /opt/zpr/bin/ph /opt/zpr/bin/vs /opt/zpr/bin/vs-admin > /state/binaries.sha256
cat /proc/self/status > /state/launch-status.txt`);
    for (const file of ["routes.txt", "addresses.txt", "binaries.sha256", "launch-status.txt"]) {
      copy(id, `/state/${file}`, `${name}/${file}`);
    }
    exec(node, "test ! -e /state/first/administrator.key; test ! -e /state/first/vs_keys.toml; test ! -e /state/first/database; test ! -e /input");
  }
  launch(visa, "redis", "redis-server --port 0 --unixsocket /state/first/database/redis.sock " +
    "--unixsocketperm 700 --dir /state/first/database --save '' --appendonly yes --appendfsync always");
  exec(visa, `timeout 15 sh -ec 'until [ "$(redis-cli -s /state/first/database/redis.sock --raw PING 2>/dev/null)" = PONG ]; do sleep 0.2; done'`);
  launch(visa, "visa", `vs --config /state/first/vs.toml /state/${adapterMode ? "adapter" : "first"}/bootstrap.bin2`);
  exec(visa, `timeout 30 sh -ec 'until [ "$(curl --silent --noproxy "*" --cacert /state/first/authority.pem -o /dev/null -w "%{http_code}" "https://[::1]:8182/admin/policies")" = 401 ]; do sleep 0.2; done'`);
  launch(node, "foreign", "ph node -c /state/foreign.toml --logging all=INFO");
  exec(visa, `timeout 30 sh -ec 'until grep -q "blob signature verification failed for cn node" /state/visa.log; do sleep 0.2; done'
${admin} actors --nodes > /state/rejected.json
jq -e 'type == "array" and length == 0' /state/rejected.json >/dev/null`);
  stop(node, "foreign");
  launch(node, "node", `ph node -c /state/first/node.toml --logging all=INFO${adapterMode ? " flow_mgmt=DEBUG" : ""}`);
  waitSync(visa);
  if (adapterMode) {
    const config = JSON.parse(docker(["inspect", adapter]))[0];
    assert.equal(config.Config.User, "0:0");
    assert.equal(config.HostConfig.Privileged, false);
    assert.equal(config.HostConfig.ReadonlyRootfs, true);
    assert.deepEqual(config.HostConfig.CapDrop, ["ALL"]);
    assert.deepEqual(config.HostConfig.CapAdd.map(capability => capability.replace(/^CAP_/, "")), ["NET_ADMIN"]);
    assert.ok(config.HostConfig.SecurityOpt.includes("no-new-privileges:true"));
    assert.ok(!config.HostConfig.PortBindings || !Object.keys(config.HostConfig.PortBindings).length);
    assert.equal(config.Mounts.length, 1);
    assert.equal(config.Mounts[0].Type, "volume");
    assert.equal(config.HostConfig.Devices.length, 1);
    exec(adapter, `test -z "$(ip -6 route show default)"
test -z "$(ip -4 route show default)"
test ! -e /var/run/docker.sock
test ! -e /state/first
umask 077
sha256sum /opt/zpr/bin/ph > /state/adapter-binaries.sha256`);
    launch(adapter, "adapter-foreign-ca", "ph adapter -c /state/adapter/foreign-ca.toml --logging all=INFO");
    exec(adapter, `timeout 25 sh -ec 'until grep -q "cert failed signature verification" /state/adapter-foreign-ca.log; do sleep 0.2; done'
grep 'cert failed signature verification' /state/adapter-foreign-ca.log > /state/foreign-ca-rejection.txt`);
    stop(adapter, "adapter-foreign-ca");
    copy(adapter, "/state/foreign-ca-rejection.txt", "adapter-foreign-ca-rejection.txt");
    exec(visa, `${admin} actors > /state/actors-before.json
jq -e 'all(.[]; .cn != "client")' /state/actors-before.json >/dev/null`);
    launch(adapter, "adapter-foreign-key", "ph adapter -c /state/adapter/foreign-key.toml --logging all=INFO");
    exec(visa, `timeout 30 sh -ec 'until grep -q "blob signature verification failed for cn client" /state/visa.log; do sleep 0.2; done'
${admin} actors > /state/actors-rejected.json
jq -e 'all(.[]; .cn != "client")' /state/actors-rejected.json >/dev/null
grep 'blob signature verification failed for cn client' /state/visa.log > /state/client-rejection.txt`);
    stop(adapter, "adapter-foreign-key");
    copy(visa, "/state/client-rejection.txt", "adapter-key-rejection.txt");
    launch(adapter, "adapter", "ph adapter -c /state/adapter/scoped.toml --logging all=INFO");
    exec(visa, `timeout 45 sh -ec 'until ${admin} actors --cn client > /state/client.json 2>/state/client-query.log &&
jq -e '\\''.cn == "client" and .node != true and (.zpr_addr | startswith("fd5a:5052:adda:"))'\\'' /state/client.json >/dev/null; do sleep 0.2; done'
jq '{cn, node: (.node // false), zpr_addr}' /state/client.json > /state/client-summary.json`);
    copy(visa, "/state/client-summary.json", "adapter-summary.json");
    exec(adapter, `timeout 15 sh -ec 'until grep -q "becoming ACTIVE" /state/adapter.log; do sleep 0.2; done'
grep -E 'has verified name CN=node|becoming ACTIVE' /state/adapter.log > /state/adapter-events.txt
grep -q 'has verified name CN=node' /state/adapter-events.txt
grep -Eq '^CapEff:[[:space:]]+0*1000$' "/proc/$(cat /state/adapter.pid)/status"
grep -Eq '^Uid:[[:space:]]+0[[:space:]]+0[[:space:]]+0[[:space:]]+0$' "/proc/$(cat /state/adapter.pid)/status"
grep -Eq '^NoNewPrivs:[[:space:]]+1$' "/proc/$(cat /state/adapter.pid)/status"
ip -j -6 addr show dev zprclient0 > /state/adapter-addresses.json
cat /proc/$(cat /state/adapter.pid)/status > /state/adapter-status.txt`);
    exec(node, "grep 'has verified name CN=client' /state/node.log > /state/client-peer.txt");
    copy(node, "/state/client-peer.txt", "node-adapter-peer.txt");
    copy(adapter, "/state/adapter-events.txt", "adapter-events.txt");
    copy(adapter, "/state/adapter-status.txt", "adapter-status.txt");
    copy(adapter, "/state/adapter-addresses.json", "adapter-addresses.json");
    copy(adapter, "/state/adapter-binaries.sha256", "adapter-binaries.sha256");
    const client = JSON.parse(fs.readFileSync(path.join(root, "adapter-summary.json")));
    const addresses = JSON.parse(fs.readFileSync(path.join(root, "adapter-addresses.json")));
    assert.ok(addresses[0].addr_info.some(address => address.local === client.zpr_addr),
      "Visa-granted dynamic adapter address must actually be assigned on TUN");
    launch(node, "allowed-workload", "redis-server --bind fd5a:5052::2 --port 16379 --protected-mode no --save '' --appendonly no --dir /state");
    launch(node, "denied-workload", "redis-server --bind fd5a:5052::2 --port 16380 --protected-mode no --save '' --appendonly no --dir /state");
    exec(node, `timeout 15 sh -ec 'until [ "$(redis-cli -h fd5a:5052::2 -p 16379 --raw PING 2>/dev/null)" = PONG ] &&
[ "$(redis-cli -h fd5a:5052::2 -p 16380 --raw PING 2>/dev/null)" = PONG ]; do sleep 0.2; done'`);
    exec(adapter, `timeout 30 sh -ec 'until [ "$(timeout 3 redis-cli -h fd5a:5052::2 -p 16379 --raw PING 2>/dev/null)" = PONG ]; do sleep 0.2; done'
timeout 5 redis-cli -h fd5a:5052::2 -p 16379 --raw PING > /state/allowed-traffic.txt
set +e
timeout 5 redis-cli -h fd5a:5052::2 -p 16380 --raw PING > /state/denied-traffic.txt 2>/state/denied-traffic-error.txt
status=$?
set -e
test "$status" = 124
test ! -s /state/denied-traffic.txt
printf 'denied TCP workload timed out with exit 124; no PONG\\n' > /state/denied-result.txt`);
    exec(node, `grep -E 'visa request denied: (NoMatch|Denied)' /state/node.log > /state/traffic-policy-denial.txt
test "$(redis-cli -h fd5a:5052::2 -p 16380 --raw PING)" = PONG`);
    copy(node, "/state/traffic-policy-denial.txt", "traffic-policy-denial.txt");
    copy(adapter, "/state/allowed-traffic.txt", "allowed-traffic.txt");
    copy(adapter, "/state/denied-result.txt", "denied-traffic.txt");
    stop(node, "allowed-workload");
    stop(node, "denied-workload");
    stop(adapter, "adapter");
  }
  exec(visa, `${admin} actors --nodes > /state/accepted.json
jq -e 'length == 1 and .[0].cn == "node"' /state/accepted.json >/dev/null
grep 'blob signature verification failed for cn node' /state/visa.log > /state/rejection.txt`);
  exec(node, `grep -E 'node access granted to visa service|registered VSS' /state/node.log > /state/grant.txt
grep -q 'node access granted to visa service' /state/grant.txt
grep -q 'registered VSS' /state/grant.txt
grep -Eq '^CapEff:[[:space:]]+0+$' "/proc/$(cat /state/node.pid)/status"
grep -Eq '^Uid:[[:space:]]+501[[:space:]]+501[[:space:]]+501[[:space:]]+501$' "/proc/$(cat /state/node.pid)/status"
grep -Eq '^NoNewPrivs:[[:space:]]+1$' "/proc/$(cat /state/node.pid)/status"
cat "/proc/$(cat /state/node.pid)/status" > /state/ph-status.txt`);
  for (const file of ["summary.json", "accepted.json", "rejected.json", "rejection.txt"]) {
    copy(visa, `/state/${file}`, file);
  }
  for (const file of ["grant.txt", "ph-status.txt"]) copy(node, `/state/${file}`, file);
  const summary = JSON.parse(fs.readFileSync(path.join(root, "summary.json")));
  assert.equal(summary.node_details.pending_install, 0);
  assert.equal(summary.node_details.in_sync, true);
  stop(node, "node");
  launch(node, "restarted-node", "ph node -c /state/first/node.toml --logging all=INFO");
  exec(node, `timeout 45 sh -ec 'until grep -q "registered VSS" /state/restarted-node.log; do
kill -0 "$(cat /state/restarted-node.pid)"; sleep 0.2; done'
grep -q 'node access granted to visa service' /state/restarted-node.log
grep -Eq '^CapEff:[[:space:]]+0+$' "/proc/$(cat /state/restarted-node.pid)/status"
grep -E 'node access granted to visa service|registered VSS' /state/restarted-node.log > /state/restarted-grant.txt`);
  waitSync(visa);
  exec(visa, `${admin} actors --nodes > /state/restarted-accepted.json
jq -e 'length == 1 and .[0].cn == "node"' /state/restarted-accepted.json >/dev/null
for process in visa redis; do
  grep -Eq '^CapEff:[[:space:]]+0+$' "/proc/$(cat /state/$process.pid)/status"
  cat "/proc/$(cat /state/$process.pid)/status" > "/state/$process-status.txt"
done`);
  copy(node, "/state/restarted-grant.txt", "restarted-grant.txt");
  for (const [source, target] of [["summary.json", "restarted-summary.json"],
    ["restarted-accepted.json", "restarted-accepted.json"], ["visa-status.txt", "visa/visa-status.txt"],
    ["redis-status.txt", "visa/redis-status.txt"]]) copy(visa, `/state/${source}`, target);
  const expected = JSON.parse(fs.readFileSync(path.join(build, "context/runtime/rust-release.json")));
  for (const role of ["visa", "node"]) {
    for (const line of fs.readFileSync(path.join(root, role, "binaries.sha256"), "utf8").trim().split("\n")) {
      const [digest, file] = line.split(/\s+/);
      assert.equal(digest, expected.binaries.find(binary => binary.name === path.basename(file)).sha256);
    }
  }
  stop(node, "restarted-node");
  stop(visa, "visa");
  stop(visa, "redis");
  acceptance = { schema_version: 1, kind: "compiled-node-container-acceptance",
    image_id: image.image_id, download_sha256: image.download_sha256, harness_sha256: hash(__filename),
    runtime_user: "501:1000", runtime_capabilities: [], read_only_root: true,
    no_new_privileges: true, node_device: "/dev/net/tun:rw", setup_user: "0:0",
    setup_capabilities: ["NET_ADMIN"], internal_ipv6_network: true, default_routes: [],
    published_ports: [], host_mounts: [], simulator_available: false,
    foreign_key_signature_rejection: true, admitted_node: summary,
    missing_device_rejected: true, wrong_tun_owner_rejected: true, node_restart_resynchronized: true,
    node_restart_scope: "PH process only; same container, namespace and state",
    noise_peer_tested: adapterMode, node_to_node_noise_tested: false,
    forwarding_tested: false, complete_installation: false };
  if (adapterMode) {
    acceptance.adapter = JSON.parse(fs.readFileSync(path.join(root, "adapter-summary.json")));
    acceptance.adapter_runtime_user = "0:0";
    acceptance.adapter_runtime_capabilities = ["NET_ADMIN"];
    acceptance.adapter_provisioning_capabilities = ["CHOWN", "DAC_OVERRIDE"];
    acceptance.adapter_foreign_ca_rejected = true;
    acceptance.adapter_foreign_rsa_rejected = true;
    acceptance.adapter_rejected_inventory_count = 0;
    acceptance.adapter_dynamic_address_assigned = true;
    acceptance.noise_adapter_node_mutual_certificates_verified = true;
    acceptance.noise_scope = "Adapter-to-node only";
    acceptance.allowed_tcp_workload = "16379: PONG";
    acceptance.denied_tcp_workload = "16380: timeout, no PONG";
    acceptance.forwarding_tested = true;
    acceptance.forwarding_scope = "One adapter to its node local actor, not inter-node or adapter-to-adapter";
    acceptance.adapter_fixture_sha256 = hash(path.join(__dirname, "prepare-adapter-probe.sh"));
  }
} catch (error) {
  for (const [id, name] of diagnostics) {
    copy(id, `/state/${name}.log`, `private-failure/${name}.log`);
  }
  throw error;
} finally {
  for (const id of containers.reverse()) docker(["rm", "-f", id]);
  for (const name of volumes.reverse()) docker(["volume", "rm", name]);
  if (network) docker(["network", "rm", network]);
}
acceptance.cleanup_verified = true;
fs.writeFileSync(path.join(root, "result.json"), `${JSON.stringify(acceptance, null, 2)}\n`, { mode: 0o600 });
console.log(JSON.stringify(acceptance, null, 2));
