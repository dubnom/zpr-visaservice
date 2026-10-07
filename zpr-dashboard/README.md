# ZPR Dashboard TUI Application

This repository contains the Zero-Trust Packet Routing (ZPR) Terminal User Interface application. The dashboard application allows you to manage your ZPR infrastructure quickly through an easy-to-understand and easy-to-use interface.

# Development

To setup your development environment, run `go mod download` to install the local dependencies. Finally, run `make run` to start the TUI.

## Tests

`make test` runs the Go unit and component suites. Browser regressions run in
the pinned Playwright Docker image; Docker is the only additional local
prerequisite. Node/npm and Chromium stay inside the container:

```sh
make test-browser
make test-all
```

When changing browser dependencies, regenerate the lockfile and embedded
browser assets with `make vendor-browser-assets`. This runs npm in the
multi-architecture Node container. CI runs both the Go and browser suites;
failed browser tests retain screenshots and traces as artifacts.

The browser suite runs regressions on desktop and iPad-like tablet Chromium inside
the container, matching the supported web-GUI device scope. It serves the real
static assets with the dashboard's CSP and fixture APIs, without live services,
certificates, or private credentials. Coverage includes
radio log selection, per-source scroll/follow memory, errors and polling lifecycle,
ANSI/HTML safety, policy completions, map/table colors and gateway clouds, visa
refresh/DNS labels, LDAP popups, attributes, memberships, branch collapse, and
explicit organization-switch approval/cancellation and stale-approval handling.
These are deterministic UI regressions, not live network or multi-node acceptance
tests; unfinished protocol, teardown, and recovery work remains separate.

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
Its Activity page is a live view of recent Control Room visa and denial data.
It shows request/approval/denial totals, sortable recent-visa and denial tables,
and a manual Refresh beside the stream status. It is an operational feed, not a
durable audit archive. **Workers** merges passive device/runtime inventory and
workload logs, with device-type filtering. Control Room remains the read-only network and policy monitor at
`http://127.0.0.1:8787`.

Control Room groups Adapters, Actors, Services, Visas, Denials, and DNS under
counted Status tabs. The summary metrics stay on Map rather than repeating on
each status page. Map updates animate retained topology components as bounds
move; Fit centers the rendered topology with padding while preserving glyph
proportions, and reduced-motion preferences suppress movement animation.

Selecting an adapter in the Control Room map shows its current unexpired visas,
including inbound and outbound flows, protocol, expiry, node, and policy ID.
The panel refreshes with polling and queries the complete Visa Service list,
not the recent-ten snapshot. These are current grants involving the adapter's
ZPR address, not confirmations that its PH has installed each grant. Loading
and upstream failures are distinguished from an empty current-visa list.
Refreshes keep the last successful visa list visible, including after a failed
request; switching adapters still loads that adapter's own list. Visa flows in
the inspector and Visas table show matching DNS names alongside the original
addresses, including aliases from the configured `svc.zpr.` zone. The zone
records are cached and refreshed once per minute during normal polling;
unmatched addresses remain numeric, and DNS failures retain cached names.

The policy editor offers compact completions beside the cursor. Inside a
`define` statement's `with` clause, the same menu includes configured attributes
and their sources, including multiline clauses. Suggestions are suppressed
after terminating periods and inside comments, quoted strings, and attribute
values. Control-Space requests suggestions, arrow keys select, Tab accepts,
and Escape dismisses; Enter remains a newline and Shift-Tab moves focus out.

Rescan LDAP, Format, Discard, Evaluate & Test, Save, Save As, and Compile & Stage
share a responsive control strip directly above the source editor. Evaluate &
Test runs ZPLC before the identity simulation. Save and Save As automatically
rerun the simulation; if it fails, the user must confirm Save anyway. Compiler-invalid
source can still be preserved, but cannot be staged. Compile & Stage recompiles
the selected saved revision on the server before creating a candidate.

The Control Room Services page uses distinct type colors for BuiltIn, Regular,
Visa, Gateway, trusted file, trusted REST, and services not in the current
policy. The upper-right **Type colors** dropdown shows the mapping and includes
stable colors for any additional reported types. Labels remain visible alongside
colors; the key stays open during polling and closes on outside click or Escape.
Map service badges use the same background, border, and text colors, including
while hovered or highlighted as arriving or exiting.

The IPv6 simulator policy also defines a ZPR `internet-gateway` service at
`[fd00:1:9::1]:8082`. Its identity uses a dedicated pre-generated bootstrap key,
and the trusted file service assigns its `public-internet` network label. The
Finance workload alone is allowed to use the service. The bundled
`internet-gateway-egress` scenario sends an HTTP health probe through ZPR to a
fixed `https://example.com/` upstream; it is not an arbitrary-host proxy. Rebuild
and restart the disposable Linux rig to load the updated signed policy and
gateway key before running this scenario.

In the Control Room map, each gateway is connected to a dark gray cloud
representing its external network. Hover over the cloud to see the declared
network label.

## Policy Layers

Runtime policy is composed from three ordered source tiers:

1. `examples/policy-layers/bootstrap.zpl`: node, Visa Service, and essential
   bootstrap/administration grants.
2. `examples/policy-layers/platform.zpl`: shared simulator control, DNS,
   observability, and baseline fixture services/grants.
3. `examples/organizations/<id>/runtime-policy.zpl`: company application policy.

The composer always uses the same first two tiers and selects one company tier.
It rejects missing/empty tiers, duplicate sources, path traversal, symlink
escapes, and oversized bundles. This is source composition, not a new precedence
or authorization model: the normal compiler and evaluator still determine
policy semantics. The company runtime layer is separate from its Policy Studio
example catalog; Redwood's illustrative business endpoints are not deployed by
its login-only runtime scenario.

Generate a candidate without changing the running network:

```sh
go run ./cmd/zpr-web-dashboard -mode compose-policy \
  -policy-root cmd/zpr-web-dashboard/examples \
  -policy-organization velocity \
  -policy-output /tmp/velocity-runtime.zpl
```

Compile/sign the generated source with the complete runtime `.zplc` configuration
and the normal protected signing key, not the company's editor-only config.
Northstar, Redwood, and Velocity candidates have been compiler-checked. The
running monolithic rig policy is not replaced by this command. Layer-aware
activation must still coordinate actor/visa reset, company LDAP, Policy-Service
workspace selection, health checks, and DNS-data retention before live rollout.
Control Room consumes generic Policy-Service APIs; it must not call simulator
APIs to select a company or orchestrate reset.

## Simulator Policy Source

Control Room's Policy tree includes **Simulator / Runtime / Simulator runtime
policy** when the local stack configures `ZPR_POLICY_SOURCE_FILE`. This is the
simulator ZPL source, separate from the editable Northstar demonstration catalog
and from acknowledgement of a signed policy installed in Visa Service.

The configured source is imported even when the repository already has records.
Importer-owned records gain a revision when the file changes; identical reads
are idempotent. Manual edits to the imported record and unrelated policies are
never overwritten. `ZPR_POLICY_SEED_CATEGORY` and `ZPR_POLICY_SEED_NAME` control
its location. This view does not install or activate a policy.

## Log Views

Log panels retain each source's last successful bounded tail when an adapter,
controller, application, or service disconnects. The selected source remains
available, its panel turns white, and a disconnect/read-error status appears
without replacing the log text. Reconnection replaces the tail with fresh data
and restores the running style. Retained tails are browser-memory only, scoped
to the active organization, and cleared when organizations change or the page
is reloaded; they are not a durable archive.

Control Room's **Adapter Logs** page shows the manifest fleet with Controller,
Control adapter, and assigned-workload adapter logs. Its read-only API passes
through the authenticated Control Service to the simulator's adapter collector.
The **Adapter logs** and **Controller logs** buttons at the top switch log types
using the same horizontal panels. Each panel remembers its adapter/controller
selection and source scroll/follow state; changing types needs no additional
collection request. The machine/adapter selectors and add/remove panel controls
apply to the selected type.
The Simulator's **Workers** page shows passive device, controller, session and
selected workload status together with assigned application/service
event logs, not Controller or adapter logs. Running machines with no supported
workload logs have an explicit empty state.

Both views poll two seconds after each collection completes, without overlapping
requests. Pause/resume, manual refresh, search, running-only filtering, and
follow-tail controls operate independently of machine lifecycle actions.
Each machine window follows the newest entries by default. Scrolling back pauses
following for that window without affecting the others; scrolling to the bottom
resumes it. Re-enabling **Follow logs** resumes all windows. The Simulator's **Log**
radio controls switch between its workload sources, showing one at a time.
Selection survives polling, and
each source remembers its scroll position and follow state when switching back.
The selector also works in maximized windows; a disappeared source falls back
to the first available source.
Polling is cancelled when navigating away from the page. ANSI foreground and
background colors (standard, bright, 256-color, and RGB), bold, faint, italic,
and underline are rendered using the vendored MIT-licensed `ansi_up` 6.0.6
browser module. Log text is HTML-escaped, terminal hyperlinks stay inert, and
only approved color and text-emphasis styles are applied without relaxing CSP.

Each source returns at most 100 lines and 64 KiB. Collection runs for at most
12 seconds with four concurrent machines; empty and unavailable sources are
shown separately. Adapter Logs reads Docker controller output and PH log files;
Workers reads application/service JSON event files. Neither view generates
traffic or starts log-producing
workloads. Recognized credentials are redacted before sending logs to the
browser. This is a bounded live tail, not a durable audit archive.

## Trusted Data Assertions

Control Room's **Assertions** page edits a per-organization, report-only assertion
record in the Policy Repository, independent of ZPL compilation and access policy.
It evaluates live trusted LDAP membership data with
group cardinality, exact-one membership, mutual exclusion, and approved LDAP
attribute presence/value/integer checks. Manual draft
evaluation and opt-in periodic checks are available; source failures and missing
groups are errors, never successful checks. See [ASSERTIONS.md](ASSERTIONS.md)
for syntax, source configuration, API contracts, and current limits.

## Organization Activation

Selecting a profile does not activate it. **Activate organization** opens a
warning naming the current and target organizations: activation resets the
simulated ZPR environment and can interrupt connections and workloads. Cancel
is focused by default; Cancel or Escape sends no activation request. Only
**Switch organization** approves the reset. If the active organization changes
while the dialog is open, cancel and review the new state before trying again.
The backend still requires scenarios to be finished/cancelled and machine users
to be logged out.

## LDAP Organization Graph

The simulator's Organizations page uses the reusable `<ldap-org-graph>` custom
element from `cmd/zpr-web-dashboard/static/ldap-org-graph.js` and its scoped
stylesheet `ldap-org-graph.css`. Load both assets and assign a directory profile:

```javascript
const graph = document.createElement("ldap-org-graph");
graph.directory = {
  base_dn: "dc=example,dc=test",
  departments: [{ name: "Engineering" }, { name: "Platform", parent: "Engineering" }],
  people: [{ uid: "alex", name: "Alex", department: "Platform", title: "Engineer" }],
  groups: [{ name: "Operators", members: ["alex"] }],
};
container.append(graph);
graph.addEventListener("ldap-node-select", (event) => console.log(event.detail));
```

Each instance owns its camera, search, and selection. Reassign `directory` to
replace the data. Department names are unique parent keys and group members use
person UIDs. Missing parents and cyclic department references attach to the
directory root. The +/- control on a directory, department, or Groups branch
collapses or expands its descendants and compacts the layout. Expand all and
Collapse all operate across branches; Left/Right arrow keys collapse or expand
the focused branch. Selection and search survive branch changes.
Clicking a node opens a component-info popup with its profile attributes,
including structured values. Close it with the close button, Escape, or a
click outside the popup; focus returns to the selected node.
People list their groups, and groups list member names and UIDs. Selecting a
person or group displays dashed membership edges to visible nodes; memberships
remain listed when related branches are collapsed. Node labels and attributes
are text, not interpreted HTML. The graph
shows organizational relationships from profile data, not a live LDAP query or
an LDIF parser, and does not imply that people are physically stored beneath
their department DN. It supports keyboard node selection, pan, zoom, and fit.

## Browser Access Gateway Prototype

The optional gateway is a loopback-only development prototype. It routes
`control.localhost` to `127.0.0.1:8787` and `simulator.localhost` to
`127.0.0.1:8788`; upstreams must remain loopback origins. LDAP and OpenObserve
remain separate local-only tools.

With the default hostnames and port, use `https://control.localhost:8443` for
Control Room and `https://simulator.localhost:8443` for the Simulator.

The prototype requires a server certificate/key and a dedicated browser-client
CA. Do not reuse the machine-control CA. Start it on loopback for controlled
development without restarting the rest of the stack:

```sh
ZPR_ACCESS_GATEWAY_TLS_CERT_FILE=/path/to/gateway.crt \
ZPR_ACCESS_GATEWAY_TLS_KEY_FILE=/path/to/gateway.key \
ZPR_ACCESS_GATEWAY_CLIENT_CA_FILE=/path/to/browser-client-ca.crt \
scripts/dashboard-stack.sh start-browser-gateway
```

The listener defaults to `127.0.0.1:8443`. Browsers must trust the server
certificate and present a client certificate issued by the configured client
CA. This authenticates a client certificate, not a named user; the prototype
has no OIDC, role authorization, user sessions, or per-certificate revocation.
Do not set `ZPR_ACCESS_GATEWAY_LISTEN` to a non-loopback address or publish an
administrative UI. Public access is deferred until the [Remote Browser Access
Contract](BROWSER_ACCESS_CONTRACT.md) is implemented and its release gates pass.
Use `scripts/dashboard-stack.sh stop-browser-gateway` to stop the local
prototype independently.

The Simulator opens on Agents. Shared ZPR actors, links, and service inventory
remain in Control Room; Agents provides per-machine controls without stack-wide
lifecycle buttons.

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
a scenario creates it on demand. Each running container has its own
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

Simulator mode runs in the Linux `zpr-simulator` container, built from source
with a containerized Go toolchain. Its UI is published on `127.0.0.1:8788` and
its mTLS machine-control listener on `8791`; the Docker socket is mounted so it
can manage machine containers. Use
`scripts/dashboard-stack.sh restart-simulator` to rebuild and replace only
Simulator mode without resetting the active ZPR rig or organization.

On each machine start, the Simulator installs a host route for that machine's
assigned ZPR address through the control adapter. This avoids ambiguous return
routing when multiple adapters advertise the same overlay prefix. Startup
also removes stale link-down ZPR routes. The simulator-control adapter uses the
Visa Service's default four-hour adapter authentication lifetime; if its actor
expires, refresh only that adapter with
`scripts/dashboard-stack.sh restart-simulator-control`.

Machine Login/Logout writes/removes the selected simulated user in that
machine container at `/run/zpr-simulator/user`. It does not start or stop
workloads. User choices come from the active Organization's people and
machine-owner assignments; organizations without overrides retain the
manifest-owner behavior. The same rule applies to scenario logins. Sessions
are simulator state and reset when the stack restarts. This is not a
password check or ZPR user authentication: controller mTLS is a separate
machine identity, and the existing BAS auth-code flow authenticates an adapter
key rather than a human.

Workers is read-only: its expandable device details show owner, posture,
container/controller state, authenticated user and selected workload identities.
The former Agents page redirects to Workers at `/machine-logs.html`, and the
navigation has one Workers entry rather than separate Agents and Workload Logs.
Word wrap is initially enabled and can be toggled for all windows, including
maximized logs. Start/stop, login/logout and workload assignment are controlled
by scenarios rather than page buttons. Selections are simulator control state
and a workload can be selected on only one logged-in machine at a time. Login/logout
and workload start/stop commands are queued by the simulator and executed by
the machine controller over its ZPR service connection; PH workload adapters
run inside the selected machine container. The selected workload remains
stopped until a scenario starts it. Docker container Start/Stop
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

The simulation environment declares machine and runtime components in
`.local-runtime/simulation-environment.json`. Organization profiles under
`cmd/zpr-web-dashboard/examples/organizations/` own the LDAP base DN and seed
files, people, groups, machine ownership, policy compiler config/catalog, and
service catalog. Scenarios reference an `organization_id` and may declare an
independent `topology` of nodes, links, and component overrides. The active
profile is selected at stack startup with `SIMULATION_ORGANIZATION_ID` (or the
manifest's `organization_id`); it defaults to `northstar`. The Organizations
page includes an organization-scoped LDAP seed editor. Scenario definitions,
directory LDIF drafts, their immutable revisions, and existing policy records
share the per-organization SQLite file under
`.local-runtime/dashboard-stack/policy-private/`. The scenario editor provides
form-based metadata, repeatable step and cleanup controls, optional parallel
dependencies, advanced JSON for topology-specific fields, and virtual folders
grouped in the catalog; scenarios without a folder appear under **Unfiled**.
The run panel offers **Clear** only for a terminal run, and clearing removes
that run history without deleting its scenario or changing machine logs.
Drafts are validated and versioned; only published
scenarios can run, and each run records the organization and published revision
it used. Deleting a scenario archives it from the catalog while retaining its
immutable revision history. Directory edits are drafts until explicitly
published. Publishing stages a private LDIF at
`.local-runtime/published-directories/<organization>.ldif`; the running
directory is unchanged, and the new seed applies on the next explicit LDAP
reseed or runtime restart. All bundled organization profiles now use isolated
Docker Compose projects; single-node profiles keep one node, while Great Lakes
and Redwood retain their multi-node topologies. Activate another profile
through the Organizations page so its directory, policy, and services are
reseeded together. Set `SIMULATION_MANIFEST` to use a different machine/runtime
manifest.

The Simulator's Claude design assistant can review scenario drafts and
organization profiles. It keeps `ANTHROPIC_API_KEY` server-side and only applies
validated scenario or LDIF proposals to the open draft; saving and publishing
remain explicit operator actions.

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
