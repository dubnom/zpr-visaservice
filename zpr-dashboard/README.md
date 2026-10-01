# ZPR Dashboard TUI Application

This repository contains the Zero-Trust Packet Routing (ZPR) Terminal User Interface application. The dashboard application allows you to manage your ZPR infrastructure quickly through an easy-to-understand and easy-to-use interface.

# Development

To setup your development environment, run `go mod download` to install the local dependencies. Finally, run `make run` to start the TUI.

## Project layout

```
config.toml.example  Endpoint, timeout and credential file paths
cmd/zpr-dashboard/   Program entry point (thin main)
internal/
  app/               Bubble Tea model: Init/Update/View, input handling
  pages/             Top-level tab views (Dashboard, Visas, Actors)
  components/        Reusable panels, tables, charts and their data
  charts/            Braille dot chart and bar chart primitives
  styles/            Color palette and shared lipgloss styles
  config/            Reads config.toml into the environment
  dataplane/         HTTP client for the vs-admin API
```

# Compiling the binary

To compile the final binary, run `make all`. The binaries will be located at bin/zpr-dashboard and bin/zpr-sim.


## Running it

Copy `config.toml.example` to `config.toml` and edit it.


```sh
make run                                # Start admin panel
```

The same lifecycle starts a separate simulation operator site at
`http://127.0.0.1:8788`. It is intentionally simulation-only and provides
manifest-backed agent/service inventory, a fixed fleet of 20 machine profiles,
workload placement, and recent managed-service logs. The fleet has 12 laptops
and 8 desktops; machine type, model, location, owner, and secure posture are
seeded into the local demo LDAP directory when the disposable Linux rig starts.
Its Activity page shows recent visas and denials from Control Room. Control
Room remains the read-only network and policy monitor at
`http://127.0.0.1:8787`.

The Simulator opens on Agents. Shared ZPR actors, links, and service inventory
remain in Control Room; stack lifecycle controls are available from Agents.

The local stack also starts the BIND 9 DNS container from `../dns/bind9` using
The local stack also starts the BIND 9 DNS container from `../dns/bind9` using
the adapter1 ZPR network namespace. It expects the publisher TSIG key and
`svc.zpr` zone file under `.local-runtime/dns-bind/`. BIND's HTTP statistics
channel is loopback-only in the adapter namespace; a host-loopback `socat`
relay feeds the Control Room DNS tab and is removed when the stack stops.
The standard simulator's Exercise action resolves each component's FQDN target
through the bootstrap DNS server in the manifest, then pings the returned ZPR
address. Provider aliases are reconciled into `svc.zpr` by Visa Service using
the configured TSIG publisher key.
Use `scripts/dashboard-stack.sh start-dns` or `stop-dns` to manage DNS without
restarting the simulator's machine fleet.

The simulator defines 20 Docker machine profiles but initially creates only
`zpr-machine-01`, from the `debian:trixie` image. Starting another machine in
the Agents page creates it on demand. Each running container has its own
Linux/ARM64 machine controller and PH adapter with a unique bootstrapped ZPR
identity. Machine adapters use dynamic `fd5a:5052::/32` addresses; the
simulator-control provider uses a reserved `fd5a:5052:adda:1::/64` address so
it is reachable through the PH-advertised overlay route.
Controllers reach the TCP `SimulatorControlService`
(`[fd5a:5052:adda:1:ffff:ffff:ffff:fffe]:8792`) over
ZPR. The service is published by the dedicated `simulator-control` actor and
only the 20 machine identities are authorized to access it. The service
gateway forwards the mutually authenticated TLS protocol to the simulator's
local listener; the ZPR adapter link and per-machine TLS client certificate
are both required.

Machine Login/Logout writes/removes the selected simulated user in that
machine container at `/run/zpr-simulator/user`. It does not start or stop
workloads. User choices come from the machine owners in the demo manifest;
sessions are simulator state and reset when the stack restarts. This is not a
password check or ZPR user authentication: controller mTLS is a separate
machine identity, and the existing BAS auth-code flow authenticates an adapter
key rather than a human.

The Agents page starts with no clients or services assigned. While logged in,
an operator can select any configured workload for a machine and then
individually start/stop it. Selections are simulator control state and a given
workload can be selected on only one logged-in machine at a time. Login/logout
and workload start/stop commands are queued by the simulator and executed by
the machine controller over its ZPR service connection; PH workload adapters
run inside the selected machine container. The selected workload remains
stopped until its individual Start control is used. Docker container Start/Stop
remains a local host operation because an offline container cannot receive a
ZPR command. Stopping a machine stops its workloads and clears its
simulated login session; starting it restores its route but does not log a user
in or restart workloads. Controller connectivity is shown separately from
Docker state.

For local LDAP schema and seed changes, the runtime consumes
`../../.local-runtime/linux-integration/pregen/zpr-machine.schema` and
`demo-machines.ldif`; the 20 profile definitions are in
`../../.local-runtime/simulation-environment.json`.
## Control Room and Policy Service

The browser Control Room lives in `cmd/zpr-web-dashboard` and calls only
same-origin `/api/*` endpoints. The Control-Room process proxies those requests
to Control-Service using mutual TLS and holds no upstream API credentials.
Its topology draws only actors returned by the live admin API; service
registrations remain service badges attached to their provider actor, and a
missing provider does not create a synthetic adapter vertex.
Control-Service aggregates Visa Service Admin data and proxies Policy
Repository requests to a separate Policy-Service process using mutual TLS.
The Policy-Service owns its SQLite journal and ZPLC configuration. It stores
generic categorized records with immutable revisions; ZPL records are
compiler-checked before a version is written, and are not automatically
installed into the running Visa Service. Per-user policy-record authorization
is not implemented yet.

In the ZPL service model, attribute providers, authentication, logging/audit,
and the Policy Repository are distinct trusted-service classes with separate
REST contracts. The trusted-source view associates providers with actors and
reports the latest real lookup outcome when available; it does not browse
provider records.

Foundational services are registered during the extended node boot through the
Visa Service adapter. The boot passes their names in `ZPR_ADAPTER_SERVICES`;
the adapter includes repeated `zpr.services` claims in its real
`authorize_connect` request. Visa Service then persists those services against
the adapter actor and the Control Room reads them from `/admin/services`.

The simulation environment declares these services in
`.local-runtime/simulation-environment.json`. Set `SIMULATION_MANIFEST` to use
a different environment manifest. The manifest is the source for service
registration names, kinds, endpoints, and simulated ownership; boot does not
maintain a second hard-coded service list.

The reusable base contract lives in `.local-runtime/generic-zpr-base.json` and
is validated separately by the installer.

The manifest extends the generic ZPR base and describes the complete bootstrap:
required packages, Rust toolchain, ZPR artifacts, base trusted services, and
boot agents. A scenario adds services and actors on top of that base rather
than replacing the installation contract.

The containerized simulated environment installs the ZPR binaries with
`.local-runtime/build-and-run-linux-node.sh`; it validates and passes the same
manifest into the extended boot. A generic simulation uses the default
manifest, while a scenario-specific simulation sets `SIMULATION_MANIFEST`.

For the local runtime, configure and start the complete stack with
`cmd/zpr-web-dashboard/scripts/dashboard-stack.sh`. It builds one binary,
starts the services in dependency order, records their PIDs and logs, and
supports `start`, `stop`, `restart`, and `status`.

The individual process environment is documented in
[`cmd/zpr-web-dashboard/README.md`](cmd/zpr-web-dashboard/README.md) for
non-local deployments.

Open `http://127.0.0.1:8787` for the Control Room UI.

