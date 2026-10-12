import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, writeFileSync, mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawnSync } from "node:child_process";

const helper = fileURLToPath(new URL("./multinode-platform.sh", import.meta.url));
const demo = fileURLToPath(new URL("../../../zpr-demo/multinode-demo/", import.meta.url));
const environment = {
  ...process.env, ZPR_MULTINODE_PLATFORM: "", ZPR_MULTINODE_IMAGE: "",
  ZPR_MULTINODE_DIRECTORY_IMAGE: "", ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE: "",
  ZPR_TRUSTED_SERVICE_BINARY: "",
};
function run(script, extra = {}) {
  return spawnSync("sh", ["-eu", "-c", '. "$1"; ' + script, "test", helper], {
    encoding: "utf8", env: { ...environment, ...extra },
  });
}

test("platform defaults preserve amd64 and explicit ARM64 is exported", () => {
  for (const platform of ["", "linux/amd64", "linux/arm64"]) {
    const arch = platform === "linux/arm64" ? "arm64" : "amd64";
    const result = run('configure_multinode_platform; sh -c \'printf "%s:%s:%s" "$ZPR_MULTINODE_PLATFORM" "$ZPR_MULTINODE_ARCH" "$ZPR_MULTINODE_IMAGE"\'', {
      ZPR_MULTINODE_PLATFORM: platform,
    });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, `linux/${arch}:${arch}:zpr-multinode`);
  }
  const invalid = run('docker() { echo "unexpected image access"; }; configure_multinode_platform', {
    ZPR_MULTINODE_PLATFORM: "linux/386",
  });
  assert.notEqual(invalid.status, 0);
  assert.match(invalid.stderr, /unsupported multi-node platform/);
  assert.equal(invalid.stdout, "");
});

test("wrong-architecture and unavailable images fail the preflight", () => {
  for (const command of ["printf amd64", "return 1"]) {
    const result = run(`docker() { ${command}; }; configure_multinode_platform; verify_multinode_platform`, {
      ZPR_MULTINODE_PLATFORM: "linux/arm64",
    });
    assert.notEqual(result.status, 0);
    if (command.startsWith("printf")) assert.match(result.stderr, /expected arm64/);
  }
});

test("helper verification rejects foreign architectures and non-ELF executables", () => {
  const directory = mkdtempSync(join(tmpdir(), "zpr-native-helper-"));
  const binary = join(directory, "provider");
  try {
    for (const arch of ["amd64", "arm64"]) {
      const header = Buffer.alloc(20);
      Buffer.from([127, 69, 76, 70, 2, 1]).copy(header);
      header.writeUInt16LE(arch === "arm64" ? 183 : 62, 18);
      writeFileSync(binary, header);
      for (const expected of ["amd64", "arm64"]) {
        const result = run('configure_multinode_platform; verify_multinode_binary "$BINARY"', {
          BINARY: binary, ZPR_MULTINODE_PLATFORM: `linux/${expected}`,
        });
        assert.equal(result.status === 0, arch === expected, result.stderr);
      }
    }
    writeFileSync(binary, "#!/bin/sh\nexit 0\n");
    const invalid = run('configure_multinode_platform; verify_multinode_binary "$BINARY"', { BINARY: binary });
    assert.notEqual(invalid.status, 0);
  } finally {
    rmSync(directory, { recursive: true });
  }
});

test("every Compose infrastructure role uses the configured platform and image", () => {
  for (const arch of ["amd64", "arm64"]) {
    const result = spawnSync("docker", ["compose", "-f", join(demo, "docker-compose.yml"),
      "--profile", "legacy", "--profile", "organization", "--profile", "web-gateway",
      "--profile", "directory-manager", "config", "--format", "json"], {
      encoding: "utf8", env: { ...environment,
        ZPR_MULTINODE_PLATFORM: `linux/${arch}`, ZPR_MULTINODE_IMAGE: `test-infra:${arch}`,
        ZPR_MULTINODE_DIRECTORY_IMAGE: `test-directory:${arch}`,
        ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE: `test-manager:${arch}`,
        ZPR_RUNTIME_ROOT: "/test/runtime", ZPR_TRUSTED_SERVICE_BINARY: "/test/provider",
      },
    });
    assert.equal(result.status, 0, result.stderr);
    const services = JSON.parse(result.stdout).services;
    assert.equal(Object.keys(services).length, 9);
    for (const [name, service] of Object.entries(services)) {
      assert.equal(service.platform, `linux/${arch}`, name);
      assert.equal(service.image, name === "directory" ? `test-directory:${arch}` :
        name === "directory-manager" ? `test-manager:${arch}` : `test-infra:${arch}`, name);
    }
  }
});

function shellFunction(source, name) {
  const match = source.match(new RegExp(`^${name}\\(\\) \\{\\n[\\s\\S]*?^\\}`, "m"));
  assert.ok(match, name);
  return match[0];
}

test("missing Visa databases require explicit approval without rotating existing credentials", () => {
  const directory = mkdtempSync(join(tmpdir(), "zpr-visa-recovery-"));
  const stateHelper = join(demo, "local-compute/runtime-state.sh");
  try {
    const fixtures = [
      { credentials: true, mode: "", allowed: false },
      { credentials: true, mode: "0", allowed: false },
      { credentials: true, mode: "1", allowed: true },
      { credentials: true, mode: "0", database: "dump.rdb", allowed: true },
      { credentials: true, mode: "0", database: "appendonlydir/appendonly.aof.manifest", allowed: true },
      { credentials: false, mode: "0", allowed: true },
      { credentials: true, mode: "invalid", allowed: false },
    ];
    fixtures.forEach((fixture, index) => {
      const root = join(directory, String(index));
      const conf = join(root, "conf");
      const credential = join(conf, "vs/vs_keys.toml");
      mkdirSync(join(conf, "vs"), { recursive: true });
      if (fixture.credentials) writeFileSync(credential, "unchanged-test-credential\n");
      if (fixture.database) {
        const database = join(root, "state/redis", fixture.database);
        mkdirSync(join(database, ".."), { recursive: true });
        writeFileSync(database, "persisted-test-database\n");
      }
      const result = spawnSync("bash", ["-eu", "-c", '. "$1"; verify_vs_database_recovery', "test", stateHelper], {
        encoding: "utf8", env: {
          ...environment, CONF_ROOT: conf, RUNTIME_ROOT: root, ZPR_RESTORE_BASE: fixture.mode,
        },
      });
      assert.equal(result.status === 0, fixture.allowed, result.stderr);
      if (!fixture.allowed) assert.match(result.stderr, /refusing policy reset|invalid Visa restore mode/);
      if (fixture.credentials) assert.equal(readFileSync(credential, "utf8"), "unchanged-test-credential\n");
    });
    assert.match(readFileSync(join(demo, "local-compute/deploy-docker.sh"), "utf8"),
      /existing_runtime=yes; fi\n  verify_vs_database_recovery/);
  } finally {
    rmSync(directory, { recursive: true });
  }
});

test("Simulator restart preserves native settings before removing the old container", () => {
  const stack = readFileSync(new URL("./dashboard-stack.sh", import.meta.url), "utf8");
  const saved = JSON.stringify([
    "ZPR_MULTINODE_PLATFORM=linux/arm64", "ZPR_MULTINODE_IMAGE=test-infra:arm64",
    "ZPR_MULTINODE_DIRECTORY_IMAGE=test-directory:arm64",
    "ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE=test-manager:arm64",
    "ZPR_TRUSTED_SERVICE_BINARY=/test/native provider", "UNRELATED_SETTING=ignored",
  ]);
  const result = run(`
    docker() { printf '%s\\n' "$SAVED_ENV"; }
    stop_simulator() { printf 'stop:%s\\n' "$ZPR_MULTINODE_PLATFORM"; }
    start_simulator() { printf 'start:%s:%s:%s:%s\\n' "$ZPR_MULTINODE_PLATFORM" "$ZPR_MULTINODE_IMAGE" "$ZPR_MULTINODE_DIRECTORY_IMAGE" "$ZPR_TRUSTED_SERVICE_BINARY"; }
    ${shellFunction(stack, "restart_simulator")}
    restart_simulator
  `, { SAVED_ENV: saved, SIMULATOR_DOCKER_CONTAINER: "test-simulator",
    STATE_DIR: "/nonexistent/test-state", ZPR_MULTINODE_IMAGE: "explicit-infra:arm64" });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, "stop:linux/arm64\nstart:linux/arm64:explicit-infra:arm64:test-directory:arm64:/test/native provider\n");
  for (const key of ["ZPR_MULTINODE_PLATFORM", "ZPR_MULTINODE_IMAGE", "ZPR_MULTINODE_DIRECTORY_IMAGE",
    "ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE", "ZPR_TRUSTED_SERVICE_BINARY"]) {
    assert.match(shellFunction(stack, "start_simulator"), new RegExp(`-e ${key}=`));
  }
  const uiRestart = shellFunction(stack, "restart_simulator_ui");
  assert.ok(uiRestart.indexOf("inherit_multinode_platform") < uiRestart.indexOf('docker stop "$SIMULATOR_DOCKER_CONTAINER"'));
});

test("saved native configuration survives a complete Simulator stop/start and rejects corrupted files", () => {
  const directory = mkdtempSync(join(tmpdir(), "zpr-native-preferences-"));
  const config = join(directory, "platform.json");
  try {
    writeFileSync(config, JSON.stringify({ ZPR_MULTINODE_PLATFORM: "linux/arm64",
      ZPR_MULTINODE_IMAGE: "saved-infra:arm64" }));
    const result = run('load_multinode_platform_file "$CONFIG"; configure_multinode_platform; printf "%s:%s" "$ZPR_MULTINODE_PLATFORM" "$ZPR_MULTINODE_IMAGE"', { CONFIG: config });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, "linux/arm64:saved-infra:arm64");
    for (const content of ["{", "[]", '{"UNKNOWN":"value"}',
      '{"ZPR_MULTINODE_PLATFORM":7}', '{"ZPR_MULTINODE_PLATFORM":"linux/arm64\\nextra"}']) {
      writeFileSync(config, content);
      const invalid = run('load_multinode_platform_file "$CONFIG"; configure_multinode_platform', { CONFIG: config });
      assert.notEqual(invalid.status, 0);
      assert.equal(invalid.stdout, "");
    }
    const stack = readFileSync(new URL("./dashboard-stack.sh", import.meta.url), "utf8");
    for (const name of ["start_simulator", "restart_simulator", "restart_simulator_ui", "start_stack"]) {
      const body = shellFunction(stack, name);
      assert.match(body, /load_multinode_platform_file "\$STATE_DIR\/multinode-platform.json"/);
      if (name === "start_stack") assert.ok(body.indexOf("load_multinode_platform_file") < body.indexOf("stop_stack"));
    }
  } finally {
    rmSync(directory, { recursive: true });
  }
});

test("native machine-control proxy replaces an old x86 process with ARM64", () => {
  const directory = mkdtempSync(join(tmpdir(), "zpr-native-proxy-"));
  try {
    const stack = readFileSync(new URL("./dashboard-stack.sh", import.meta.url), "utf8");
    const runtime = readFileSync(new URL("./machine-runtime.sh", import.meta.url), "utf8");
    const script = `
      set -eu
      configure_multinode_service_return_route() { :; }
      docker() {
        case "$1" in
          inspect) printf 172.17.0.3; return ;;
          cp) printf 'copy %s\\n' "$2"; return ;;
          exec) ;;
          *) echo "unexpected command: $*" >&2; return 1 ;;
        esac
        if [ "$2" = -d ]; then printf 'launch %s\\n' "$*"; return; fi
        shift 2
        case "$1" in
          mkdir) return ;;
          /app/bin/ph-cli) printf '(Active)\\n' ;;
          ip) printf '12: tun5 inet6 fd5a:5052::1/32 scope global\\n' ;;
          ps) printf '42 /tmp/zpr-machine-control-proxy-linux-amd64 --proxy-upstream 172.17.0.3:8791\\n' ;;
          kill) [ "$2" != -0 ] || return 1; printf 'kill %s\\n' "$2" ;;
          *) echo "unexpected command: $*" >&2; return 1 ;;
        esac
      }
      ${runtime}
      ${shellFunction(stack, "stop_multinode_control_processes")}
      ${shellFunction(stack, "start_zpr_machine_control_service")}
      start_zpr_machine_control_service
    `;
    const result = run(script, {
      ZPR_MULTINODE_PLATFORM: "linux/arm64", RUNTIME_DIR: directory, STATE_DIR: directory,
      SIMULATION_ORGANIZATION_ID: "northstar", SIMULATION_EXPECTED_ORGANIZATION: "",
      ORGANIZATIONS_DIR: fileURLToPath(new URL("../cmd/zpr-web-dashboard/examples/organizations", import.meta.url)),
      ACTIVE_ORGANIZATION_FILE: join(directory, "absent"),
      MACHINE_CONTROL_ADDRESS_FILE: join(directory, "address"),
      SIMULATOR_DOCKER_CONTAINER: "test-simulator",
      MACHINE_CONTROL_PROXY_BIN: "/test/native-proxy",
    });
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /kill 42/);
    assert.match(result.stdout, /copy \/test\/native-proxy/);
    assert.match(result.stdout, /launch .*\/tmp\/zpr-machine-control-proxy-linux-arm64/);
  } finally {
    rmSync(directory, { recursive: true });
  }
});

test("generated nodes, credentials, policy upload and directory management share platform configuration", () => {
  for (const file of ["local-compute/deploy-docker.sh", "local-compute/runtime-state.sh",
    "local-compute/start-directory-manager.sh"]) {
    const source = readFileSync(join(demo, file), "utf8");
    assert.doesNotMatch(source, /--platform linux\/amd64/, file);
    assert.match(source, /\$ZPR_MULTINODE_PLATFORM/, file);
    assert.match(source, /\$ZPR_MULTINODE_IMAGE/, file);
  }
  const deploy = readFileSync(join(demo, "local-compute/deploy-docker.sh"), "utf8");
  assert.match(deploy, /image: \$image, platform: \$platform/);
  const activation = readFileSync(new URL("./activate-organization.sh", import.meta.url), "utf8");
  assert.ok(activation.indexOf("verify_multinode_platform") < activation.indexOf('report_activation_status "Stopping previous runtime"'));
  assert.match(activation, /--platform "\$ZPR_MULTINODE_PLATFORM" "\$ZPR_MULTINODE_IMAGE" \/app\/bin\/vs-admin/);
});

test("generated multi-node overrides preserve node inventory and use native image/platform", () => {
  const directory = mkdtempSync(join(tmpdir(), "zpr-native-nodes-"));
  try {
    const source = readFileSync(join(demo, "local-compute/deploy-docker.sh"), "utf8");
    const profile = fileURLToPath(new URL("../cmd/zpr-web-dashboard/examples/organizations/great-lakes.json", import.meta.url));
    const nodes = readFileSync(profile, "utf8");
    const override = join(directory, "nodes.json");
    const result = spawnSync("bash", ["-eu", "-c", `
      COMPOSE=()
      ${shellFunction(source, "write_node_compose_override")}
      write_node_compose_override
    `], { encoding: "utf8", env: {
      ...environment, ORGANIZATION_PROFILE: profile, NODE_COMPOSE_OVERRIDE: override,
      CONTAINER_PREFIX: "great-lakes-", RUNTIME_ROOT: directory, SCRIPT_DIR: join(demo, "local-compute"),
      ZPR_MULTINODE_PLATFORM: "linux/arm64", ZPR_MULTINODE_IMAGE: "test-infra:arm64",
    } });
    assert.equal(result.status, 0, result.stderr);
    const services = JSON.parse(readFileSync(override, "utf8")).services;
    const configured = JSON.parse(nodes).runtime.nodes;
    assert.equal(Object.keys(services).length, configured.length);
    configured.forEach((node, index) => {
      const service = services[`runtime-node-${index}`];
      assert.equal(service.platform, "linux/arm64");
      assert.equal(service.image, "test-infra:arm64");
      assert.equal(service.container_name, `great-lakes-node${index}`);
      assert.equal(service.networks["zpr-local"].ipv4_address, node.substrate_address);
      assert.equal(service.environment.ZPR_NODE_ZPR_ADDRESS, node.zpr_address);
    });
  } finally {
    rmSync(directory, { recursive: true });
  }
});
