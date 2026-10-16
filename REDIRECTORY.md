# Source and Project Reorganization Plan

**Methodology direction (2026-10-16):** Simulator environment setup must migrate
to the real administrator configuration/stage/compile/push workflow through the
same authorized production contracts used by Control Room. Production owns
configuration, compilation/signing, deployment/readback and rollback; Simulator
owns simulated machine/adapter/workload control. This is planned, not implemented.
Do not extract current launcher bypasses unchanged and call that separation.
See the [migration boundary and checklist](System%20release%20plan.md#simulator-methodology-act-as-a-zpr-administrator).

Status reconciled 2026-10-15 UTC (2026-10-14 EDT). Most checklist items remain planned; the shared
editor-controller reuse is implemented and deployed. The first backend
extraction now places neutral Gateway forwarding and the HTTP/CONNECT runtime
in `zpr-dashboard/internal/gateway`, with a standard-library-only dependency
check. It is verified and deployed to Simulator only. The remaining
browser-access gateway host routing and TLS configuration are now extracted to
`internal/browsergateway`, verified and included in Simulator's image only;
the separate browser-gateway process was not updated. Other application
domains still share the Go main package. A third extraction of service TLS
transport primitives is verified and included in Simulator's image only.
A fourth extraction places Control Room's upstream proxy in `internal/controlroom`,
verified and deployed to Control Room only. That current-worktree image also
includes the first three packages; separate gateway and backend-service processes
were not updated. A fifth extraction moves Control Room API permission mapping
and named-user authorization into that same production package. A sixth extraction moves the shared local-request
check into the existing standard-library-only `internal/servicehttp` package,
also verified and deployed to Control Room only. The fifth and sixth increments
are included in the 2026-10-14 15:57 UTC release; other services remain unchanged.
The seventh extraction isolates DNS statistics forwarding in `internal/dnsstats`.
The eighth extracts signed DNS record transfers into `internal/dnsrecords`.
Both are verified and deployed in the Control Room/Control-Service current-worktree
release at 2026-10-14 16:26 UTC, which includes all eight extractions.
Simulator still runs the first three; Policy-Service and separate gateways
were not updated. Earlier release scopes below are historical.
The first seven extractions were committed and pushed as `ed27bca`; historical
uncommitted labels below describe their state at the time of each increment.
The eighth through tenth extractions were subsequently committed and pushed in
`e1b24ab`; the ninth isolates Policy-Service forwarding in `internal/policyclient`,
and the tenth adds its bounded assistant attribute client. The 22:35 UTC Simulator
and 22:43 UTC Control Room dashboard-only releases include that published source;
they did not update Control-Service, Policy-Service or separate gateways.
Earlier uncommitted/not-deployed labels below are historical verification states,
not current publication status. An eleventh extraction isolates neutral
Claude HTTP completion in `internal/assistant`. A twelfth extraction moves
production adapter-log inventory types and validation into
`internal/controlservice/adapterlogs`.
The thirteenth extracts shared log formatting/redaction into `internal/logview`,
removing production adapter-log and diagnostics calls to Simulator-owned
helpers. All three increments are locally verified and included in the
2026-10-15 Simulator worktree release; Control Room/Control-Service rollout
remains open. They are included in the current source publication.
The audited production assistant key fallback is now isolated: production
configuration is in `assistant_config.go`, protected file reading is neutral in
`internal/assistant`, and manifest-sibling discovery is Simulator-only.
This M2 source slice is verified locally and included in the Simulator worktree
release and current publication; the production service processes have not been
updated for this slice. It does not complete the production application boundary.
Great Lakes Restore exposed an assertion-seeding verification failure and
rolled back successfully. The 01:53 UTC Policy-Service repair replaces silent
Simulator-profile discovery with explicit `ZPR_POLICY_ASSERTIONS_SOURCE_FILE`;
the provisioning owner exports the seed and context restarts retain the deployed
service image. Invalid explicit seeds fail; nonempty operator rules are preserved.
Focused race/launcher tests pass and all 28 live LDAP assertions pass. The user's
subsequent Restore completed successfully. Regional Business acceptance later
passed all 106 steps after node-session renewal/runtime and denial-source
repairs; HQ/Shenzhen and sustained authentication-lifetime recovery remain
pending. This does not complete M3 repository/fixture ownership.
The next M3 input slice is now locally verified: production policy fixtures use
explicit `ZPR_POLICY_TEST_FIXTURES_FILE` metadata and live configured LDAP,
with no profile/manifest reads or fictional identity defaults. Simulator
provisioning exports the independent contract. It was deployed to Policy-Service
at 17:59 UTC and published in `990d54a`; this does not close a milestone or expand
the deployment evidence. See the verification record below.
The subsequent M1 nested production contract audit records enrollment,
gateway-draft and change-review routing, authorization and storage semantics.
Four new compatibility tests are locally verified with Simulator unavailable;
no runtime code was moved or deployed. Packaging acceptance and the remaining
cross-domain ownership audit are still open.
The fourteenth extraction now moves gateway draft validation, diagnostics,
installed-identity selection and revision storage into
`internal/controlservice/gatewaydraft`. Its inventory input is independent of
the shared command snapshot; a command adapter preserves existing Admin health
gating. Neutral upstream parsing is in `internal/gateway`, not a Simulator
workload file. The work through the inventory audit was published in `990d54a`;
publication does not expand the dated deployment evidence.
The fifteenth extraction now moves the explicit-config Admin read transport
into `internal/controlservice/adminapi`, with environment/error compatibility
remaining in the command. This new slice is locally verified, not deployed or
published. The sixteenth extraction moves snapshot DTO/acquisition and pure
service/trusted assembly into `internal/controlservice/inventory`. Explicit
callbacks retain command-owned enrichment/configuration and its existing order.
The seventeenth extraction adds explicit display-name and node telemetry
enrichment to that owner. The command supplies file paths, capped catalog IDs
and a query callback; diagnostics/provider discovery and HTTP lifecycle remain
outside. These owners are locally verified, not deployed or published;
the eighteenth extraction moves the diagnostics catalog, wire/provider
contracts and explicit OTLP/OpenObserve acquisition to
`internal/controlservice/diagnostics`. Operator configuration and authorized
HTTP aggregation/composition remain command-owned at that checkpoint. The
subsequent aggregation extraction moves source queries, response states and
budgets into that production package. HTTP validation/deadlines, authorization
composition and operator configuration remain command-owned.
The subsequent production-composition audit records their startup ordering,
configuration timing and exact route/server guards. Its subsequent move gives
all 23 Control-Service registrations one explicit-handler production HTTP owner
in `internal/controlservice/httpapi`; command composition still supplies domain
handlers, configuration, security headers and server lifetime.
The operator web-security loader/request boundary now also has an explicit-input
owner in `internal/operatorweb`, shared by independent Control Room/Simulator
configuration adapters. Control Room HTTP composition, domain/application
ownership and final acceptance remain open.
Full production/Simulator separation is not
complete. Reorganize incrementally to
make ownership and dependency boundaries enforceable without changing behavior.
Keep the existing major project/crate boundaries unless a concrete dependency
problem justifies changing them. This is not a framework migration, microservice
split, or wholesale repository relocation.

Related trackers:
- [Team charter](../AGENT_TEAM_STARTER.md): role boundaries, assignments and handoffs.
- [Component work and status](../COMPONENT_WORK.md): coordination index, owners and status contract.
- [GUI work](GUI%20work.md): active GUI and JavaScript reuse backlog.
- [Completed GUI work](GUI%20completed%20work.md): editor/controller migration checklists and evidence.
- [Organization editor plan](Organization%20editor%20plan.md): detailed creation/editing requirements.
- [Scaling](SCALING.md): performance and capacity changes.
- [Repository guidance](AGENTS.repo.md): production/Simulator boundary.
- [Deployment history](GUI%20deployments.md): dated release scope and verification.

## Remaining delivery milestones (2026-10-14)

**Seven milestones remain open.** These are completion gates, not estimates of
seven edits, seven sessions or seven deployments. The fourteen extractions above
are evidence toward these gates, not fourteen completed milestones. Do not add
another milestone merely because implementation needs another package.

The audited command set comprises 59 non-`_test.go` Go files (58 at the initial
ownership audit, plus `assistant_config.go`); this is a scope
measurement, not a target file count or a classification of every file's role.
Concurrent additions outside this slice are not covered by this measured set.
The older inventory below remains historical. No milestone is complete until
its exit evidence is recorded here.

| ID | Deliverable | Status | Exit criteria |
| --- | --- | --- | --- |
| M1 | Compatibility and ownership baseline | In progress | Map all current command files and affected assets/scripts to owners; inventory production routes, auth/TLS/CSRF, defaults and persistent paths; identify cross-domain calls; record focused baseline commands and unrelated failures. Every remaining move has an owner and compatibility contract. |
| M2 | Production application boundary | In progress | Control Room and Control-Service handlers, inventory, diagnostics, operator configuration and lifecycle are composed from production packages. They cannot import Simulator implementation directly or transitively, read its configuration, or require its availability. Dependency checks and production startup/feature tests with Simulator unavailable enforce this. |
| M3 | Policy and trusted-source ownership | In progress | Repository/versioned documents, configuration forms, trusted connectors and assertion evaluation have cohesive owners and explicit contracts. Separate Simulator seeding/provisioning from production repository defaults; preserve storage ordering, authorization, source-owned diagnostics and task prompt/context boundaries. Reuse existing authentication/enrollment/delegation packages. |
| M4 | Simulator ownership and thin dispatcher | Planned | Organizations/drafts, workspaces, scenarios, machines/sessions/workloads and provisioning live in Simulator-owned packages; test workload runtimes have an explicit owner. Resolve cross-document organization authority before behavior depends on it. The compatibility command retains only flags/configuration, assembly and lifecycle, with no remaining domain implementation hidden in adapters. |
| M5 | Frontend and embedded-resource ownership | In progress | Explicit Control Room/Simulator page adapters and cohesive feature controllers use neutral shared utilities. Preserve public asset URLs, skill selection, embedding/load order, editor behavior and cancellation. Desktop/tablet regressions and production features with Simulator unavailable pass; builds contain all required assets/skills. |
| M6 | Rust and naming disposition | Planned | Review the identified Rust manager/admin responsibilities and misleading names. Record justified splits or explicit retain-as-is decisions; implement only approved changes with invariant tests and compatibility checks. No arbitrary line-count split, repository/module rename or incidental runtime-data migration is required. |
| M7 | Integrated acceptance and handoff | Planned | Review final dependency directions, reconcile architecture/README/index links and artifact locations, and verify native/cross-target builds plus container/installer packaging and required Go/Rust/browser suites. Record all remaining baseline failures as exceptions, not a full pass. Commit/push and deployment remain separately authorized actions; any requested rollout needs persistent-state preservation, rollback and served-asset/service checks. |

### Execution order and stopping rule

1. Finish M1 before choosing the next extraction; use a file-ownership and
   cross-domain call inventory rather than selecting another convenient leaf.
2. Deliver M2 and M3 as cohesive production slices. Each may contain several
   reviewable implementation changes; each change must advance an exit criterion.
3. Finish M4 after production contracts are explicit. M5 can proceed independently
   once its service adapters are defined; existing shared-editor work counts
   toward M5 but does not close it.
4. M6 starts with a review, not an automatic Rust rewrite. Retaining sound modules
   or compatibility names is an acceptable documented outcome.
5. Close M7 only after the implementation gates and acceptance evidence are
   complete. Structural completion does not itself require deploying live systems.

Organization profile-versus-directory authority and any runtime-data/name
migration require explicit decisions rather than assumptions. Record those
decisions under the affected milestone; unresolved decisions block that scope,
not unrelated work. Preserve unrelated worktree edits and never change an
existing failing test without permission.

**Current bounded deliverable:** the M1 source ownership and production-contract
audit below is recorded and its focused baseline passes. The 2026-10-15
compatibility-mode audit now covers all 23 dispatch cases, 40 flags and the
Simulator path/default contract; nested production route/persistence mapping
and packaging acceptance remain open. M1 is not closed.
The audited assistant configuration slice is implemented and locally verified,
as recorded below. Assertion seeding now uses an explicit independent production
input. The policy-fixture contract slice below is also implemented and locally
verified; production runtime rollout and cohesive policy-package ownership remain
open. Report progress as milestones closed/open and the current
milestone's remaining gates, not a running count of tiny extractions.

The detailed checklists below remain the requirement inventory. They refine these
seven gates rather than creating another delivery sequence. A scope increase
must be explicit; unrelated feature, reliability or scaling work does not silently
extend this reorganization.

## M1 audit: command ownership and production contracts (2026-10-14)

This is a source audit of the current worktree, not a runtime health assessment.
The 59 audited command-directory non-`_test.go` Go files are assigned below
(the original 58 plus production `assistant_config.go`). Several are
mixed files: an ownership assignment does not mean that an independent package
already exists or that every function has been audited. Tests remain with their
current implementations until the corresponding extraction.

### Current file ownership map

Paths in this table are relative to
[`cmd/zpr-web-dashboard`](zpr-dashboard/cmd/zpr-web-dashboard/). The proposed
destinations refine the existing tree rather than authorizing new applications.

| Files | Current responsibility and destination |
| --- | --- |
| `main.go`, `control_service.go`, `admin_client.go`, `inventory.go` | Dispatcher, Control Room/Control-Service startup and explicit configuration/composition. `control_service.go` supplies prebuilt handlers to the production `internal/controlservice/httpapi` and `internal/controlroom/httpapi` route owners; authorization wrapper selection remains command-local. `admin_client.go` adapts environment/error compatibility to `internal/controlservice/adminapi`. `inventory.go` aliases production DTOs and delegates acquisition/pure service assembly to `internal/controlservice/inventory`, while selecting enrichment in the original order. Keep flags/assembly/lifecycle and static embedding at the entry point; provider and remaining HTTP/application ownership are still open. |
| `operator_login.go`, `operator_policy.go`, `operator_delegation.go`, `enrollment.go`, `change_review.go` | Production security/enrollment/review composition using existing packages. `operator_login.go` now supplies independent Control Room/Simulator environment inputs and compatibility projections to `internal/operatorweb`, which owns TLS/OIDC loading, request protection, auth-route registration and auth cleanup. Permission/delegation/review domain composition and startup remain command-local. |
| `node_denials.go`, `node_display_names.go` | Command configuration/provider/catalog adapters. Explicit display-name loading/application and pure denial/counter enrichment belong to `internal/controlservice/inventory`. OTLP acquisition and catalog ownership now belong to `internal/controlservice/diagnostics`; provider selection and mapping-file discovery remain command-local. |
| `adapter_logs.go`, `diagnostics.go`, `dns_records.go`, `dns_stats.go`, `trusted_changes.go` | Production operator logs/metrics, DNS and change feeds. Adapter inventory, DNS transport and log formatting already have packages. Diagnostics wire/catalog/provider acquisition and result aggregation now belong to `internal/controlservice/diagnostics`, with compatibility aliases/adapters in the command; configuration, authorized HTTP lifecycle and remaining application composition still need ownership. |
| `gateway_api.go`, `gateway_draft.go`, `gateway_inventory.go` | Production gateway HTTP composition, compatibility aliases and Admin inventory projection. Validation, diagnostics, installed-identity selection and revision storage now belong to `internal/controlservice/gatewaydraft`; the old `gateway_config.go`/`gateway_config_store.go` implementations moved there. Keep separate from Simulator workload execution and actual gateway activation. |
| `browser_gateway.go`, `gateway_service.go` | Browser-access runtime adapter and mixed gateway runtime/workload adapters. Reuse `browsergateway`/`gateway` cores; move fixed-upstream scenario logging to the Simulator/test-workload owner, not the production transport. |
| `policy.go`, `policy_service.go`, `policy_store.go`, `policy_staging.go`, `policy_attributes.go`, `policy_assertions.go`, `policy_test_api.go`, `policy_test_fixtures.go`, `config_form.go` | Policy repository/API/configuration/staging/attributes/assertion settings and policy-test contracts. Assertion seed and fixture inputs are now explicit production contracts, but HTTP utilities, shared command types and composition still need cohesive owners before package movement. |
| `policy_fixture_config.go` (added after the original audit) | Production versioned fixture metadata, strict file/context validation and inventory/directory input types. No Simulator discovery or fictional authority defaults. The fictional export adapter is Simulator-owned `scripts/export-policy-fixtures.jq`. |
| `assistant_config.go` | Production assistant environment/path resolution and shared model/HTTP client composition. No Simulator discovery; protected file reading is in neutral `internal/assistant`. Keep only production/neutral dependencies when moving this composition into a production package. |
| `assertions.go`, `assertion_expressions.go`, `assertion_ldap.go`, `assertion_runtime.go`, `assertion_sources.go`, `lint.go` | Parser/evaluator/lint, provider connectors and Control-Service assertion scheduling. Separate reusable evaluation contracts from production runtime, repository settings and Simulator consumers; preserve source-owned diagnostics. |
| `assistant_skills.go`, `editor_assistant.go`, `design_assistant.go` | Embedded task resources and mixed task dispatch. Neutral completion is extracted; policy/config/assertion/gateway prompts belong to their domains, design/directory/scenario/organization context to Simulator. Keep explicit embedding ownership. |
| `organizations.go`, `organization_activation.go`, `organization_bundle_profile.go`, `organization_create.go`, `organization_definitions.go`, `organization_duplicate.go`, `organization_workspace.go` | Simulator organization profiles, private bundles, portable definitions, creation and activation. Do not treat these as production repository configuration. |
| `directory_workspace.go`, `simulator_workspace.go`, `workspace_store.go`, `trusted_source.go` | Simulator directory/workspace revisions and live directory viewer. `workspace_store.go` is Simulator artifact persistence despite its generic name; `trusted_source.go` loads active Simulator organization/container data despite its generic name. |
| `simulator.go`, `simulator_policy.go`, `scenario.go`, `scenario_check.go`, `scenario_reliability.go` | Simulator routing/security/manifest/component composition and scenario execution/placement. Destinations are Simulator application/scenario owners; production may not import them. |
| `machine_control.go`, `machine_control_scope.go`, `machine_logs.go` | Simulator controller/session/runtime-generation transport, orchestration and logs. Neutral text formatting is extracted; machine/container/workload selection is not neutral. |
| `policy_layers.go`, `test_app.go` | Simulator provisioning policy composition and test/stress/benchmark workloads. The former loads organization profiles; the latter also has workload-log HTTP adapters. Preserve CLI modes but do not place these in production repository/default initialization. |

### Production entry points and compatibility contracts

Authoritative Control-Service registration now belongs to
[`httpapi/routes.go`](zpr-dashboard/internal/controlservice/httpapi/routes.go).
Control Room root registration belongs to
[`controlroom/httpapi/routes.go`](zpr-dashboard/internal/controlroom/httpapi/routes.go).
The command supplies already wrapped handlers; Policy-Service registration
remains in [`policy_service.go`](zpr-dashboard/cmd/zpr-web-dashboard/policy_service.go).
[`assertion_runtime.go`](zpr-dashboard/cmd/zpr-web-dashboard/assertion_runtime.go)
and [`trusted_changes.go`](zpr-dashboard/cmd/zpr-web-dashboard/trusted_changes.go)
supply domain handlers and compatibility registration forwarders.
This route-family inventory includes delegated registrations; it is not a
replacement router specification.

| Surface | Current contract to preserve |
| --- | --- |
| Compatibility CLI | 23 dispatched modes and 40 flags in the current source; default `control-room`, `ZPR_WEB_MODE`/`-mode`, `ZPR_WEB_LISTEN`/`-listen` default `127.0.0.1:8787`. Keep all machine/workload/proxy/policy-composition flags in `main.go`, even when implementations move. See the per-mode audit below; the earlier count of 20 predates the three Business modes. |
| Control Room | Static `/`, `/auth/operator/*`, `/api/change-review/`, signed `/api/enrollment/`, authorized `/api/` proxy and local `GET /bind9.xsl`. Keep direct-TLS/host/approved-peer boundary, OIDC sessions and CSRF; preserve typed-nil/no-auth loopback development behavior and enrollment's distinct authorization. |
| Control-Service | Default listen `127.0.0.1:8790`; server cert/key/client CA required; TLS 1.3 with verified client certificates. Registers snapshot, actor visas, diagnostics, adapter logs, DNS records/stats/stylesheet, gateway contracts/drafts/assistant, policy assistant/proxy, assertion runtime, change feeds and both enrollment prefixes. Preserve route specificity and permission/method behavior. |
| Snapshot/actor visas | `GET /api/snapshot`, `GET /api/actors/{actor}/visas`; preserve JSON tags, empty/null distinctions, partial/disconnected/configuration statuses, sorted output, deduplication, live visa expiry and 404 handling. Snapshot deadline 10s; actor-visas deadline 15s; Admin HTTP timeout 8s and decoder bound 2 MiB. |
| Operator monitoring | `GET /api/diagnostics`, `/api/adapter-logs`, `/api/dns/records`, `/api/dns/stats/` and `/bind9.xsl`; preserve independent operator origins/inventory, stale/error reporting, caps, redaction and no-store headers. File history must not imply a running process. |
| Policy-Service | Default listen `127.0.0.1:8789`; TLS 1.3/verified client cert required. `/api/policy` status/context; categories, records, duplicate/rename/archive/restore, revisions, staging, checks/config-form conversion, tests/fixtures, attributes/rescan; assertion settings GET/PUT. Repository storage/configuration and compiler contracts stay production-owned. |
| Assertions/change feeds | Analyze/format/source, GET/PUT assertion settings view and evaluation; trusted-source feed list and per-source changes. Preserve scheduling/last-update/error behavior and existing source validation and request ownership. |
| Assistants/gateways | Policy and gateway assistant status/completion remain Control-Service-owned; no raw browser credentials or key. Preserve task skills, allowed editors/models, size/token limits and error contracts. Gateway draft validation/revisions are not deployment/activation. |

The permission source of truth is
[`controlroom.APIPermission`](zpr-dashboard/internal/controlroom/authorization.go):
monitoring uses `monitor.read`; policy/assertion routes use
`policy.read`/`policy.analyze`/`policy.edit`; gateways use
`gateway.read`/`gateway.analyze`/`gateway.edit`. Unknown routes fail closed.
Change review and enrollment have their own domain/organization/delegation
checks, not this generic gate.
[`localEditorRequest`](zpr-dashboard/cmd/zpr-web-dashboard/policy.go) retains
the verified-client/peer/host/origin check via `servicehttp`, with explicit
`ZPR_CONTROL_ROOM_PROXY_IP`; it is not equivalent to named-user authorization.
Do not replace one boundary with the other during a move.

### Configuration, state and resource ownership

- Production connections: Control Room uses `ZPR_CONTROL_SERVICE_URL`,
  `ZPR_CONTROL_CLIENT_CERT_FILE`/`KEY_FILE`, service CA/server-name settings;
  Control-Service uses its `ZPR_CONTROL_SERVICE_*` listener credentials and
  `ZPR_ADMIN_*` Visa Admin settings plus `ZPR_POLICY_SERVICE_*`/
  `ZPR_POLICY_CLIENT_*` transport settings. Preserve the existing Admin
  certificate-chain verification and optional configured server-name semantics;
  do not substitute the different service-mTLS transport as a cosmetic cleanup.
- Production monitoring: `ZPR_PLATFORM_SERVICES`, `ZPR_PROVIDER_MANAGER_URLS`,
  `ZPR_DEMO_LDAP_EDITOR_URL`, `ZPR_NODE_DISPLAY_NAMES_FILE`,
  `ZPR_NODE_DENIAL_METRICS_FILE`, `ZPR_DIAGNOSTICS_*`,
  `ZPR_ADAPTER_LOG_CONFIG_FILE`, `ZPR_DNS_STATS_URL` and
  `ZPR_DNS_TRANSFER_*` are operator configuration, not Simulator inventory.
  Node denial config currently derives a sibling filename from diagnostics
  configuration if not explicitly supplied; preserve or explicitly change it,
  never replace it with a simulation default.
- Production persistence: keep `ZPR_POLICY_DB_FILE` (fallback is the user's
  `zpr-control-room/policy-records.db`), policy compiler/config/staging paths,
  assertion sources/legacy settings, `ZPR_ENROLLMENT_DATABASE_FILE`,
  gateway draft store, change-review store and operator signing/session/OIDC
  material outside source moves. Resolve their configuration through owners,
  not by relocating or recreating live state.
- Simulator owns manifest/profile/scenario directories,
  `SIMULATION_WORKSPACE_DB_DIR`, private organization bundles, selected
  organization and machine/session/workload state. Similar SQLite/versioning
  mechanics do not make these production repository inputs.
- `staticFiles` embeds `static/*`; `assistant_skills.go` embeds seven
  task-specific `skills/*/SKILL.md` files. Preserve the compile-time resources,
  URL mappings, browser fixture roots and task boundaries before any relocation.
  `scripts/Dockerfile.simulator` builds the shared command used by multiple
  modes. `dashboard-stack.sh` also builds native/Linux machine binaries and
  references command-local examples; `activate-organization.sh` and
  `verify-machine-control.sh` depend on those examples too. Browser container
  tests compile this command's tests, and vendor tooling writes its static
  directory. The TUI Make build alone is not acceptance for these paths.

### M1 compatibility-mode and packaging audit (2026-10-15)

This source audit follows the independent fixture-input slice. It records all
23 dispatch cases and 40 flag registrations in the current shared command.
It does not start services, inspect private runtime files, certify installers
or prove a full production dependency boundary. Concurrent editor work is
preserved and remains outside this refactoring slice.

#### Dispatch, defaults and failure contracts

The common entry point is
[`main.go`](zpr-dashboard/cmd/zpr-web-dashboard/main.go). All flags are registered
before `flag.Parse`; CLI values override environment-derived defaults.
`envOr` uses nonempty environment text verbatim, including whitespace; do not
silently change it to trimmed-value semantics during relocation.

- `-mode`: `ZPR_WEB_MODE`, otherwise `control-room`.
- `-listen`: `ZPR_WEB_LISTEN`, otherwise `127.0.0.1:8787`. This is also the
  bare Simulator/workload default; selecting a mode does not rewrite it.
  Simulator's usual port 8788 comes from the Docker entrypoint/launcher.
  Workload launchers supply their explicit IPv6 endpoints instead.
- `-gateway-listen`: `ZPR_ACCESS_GATEWAY_LISTEN`, otherwise
  `127.0.0.1:8443`; only browser-gateway dispatch uses this flag.
- `-business-state-dir`: `/var/lib/zpr-business`, with no environment fallback.
- The other 26 string flags default to empty; all ten integer flags default
  to zero. Empty/zero is not a usable inferred workload configuration.
  `-listen` does not configure the separately environment-owned
  Control-Service or Policy-Service listeners.
- Every returned mode error reaches `log.Fatal` (nonzero process exit).
  Unknown modes call `log.Fatalf`; Go flag parsing reports unknown flags and
  invalid integer values. `-help` describes only five modes in its mode
  description today; it is not the authoritative mode inventory.

| Mode | Implementation owner / required inputs | Output, state and failure behavior to preserve |
| --- | --- | --- |
| `control-room` | Production web composition; `-listen`, operator security and Control-Service transport settings | Embedded UI and authorized API proxy; production TLS/auth configuration described above. |
| `control-service` | Production service composition; `ZPR_CONTROL_SERVICE_*`, Admin and policy transports | Default port 8790, verified-client TLS, operator persistence and independent providers; ignores `-listen`. |
| `policy-service` | Production policy composition; `ZPR_POLICY_SERVICE_*`, workspace/compiler/LDAP/seed/fixture settings | Default port 8789, verified-client TLS, repository revisions/staging; ignores `-listen`. |
| `simulator` | Simulator application; `-listen`, manifests/profiles/scenarios, operator security and optional machine-control TLS | HTTP when web security is unset, otherwise configured HTTPS; UI/workspace operations and runtime orchestration. Incomplete machine-control TLS fails startup; a later machine-control listener error is logged by its goroutine, not propagated to the already-running UI. |
| `browser-gateway` | Browser-access composition using `internal/browsergateway`; `-gateway-listen`, all three `ZPR_ACCESS_GATEWAY_TLS_CERT_FILE`/`TLS_KEY_FILE`/`CLIENT_CA_FILE` inputs | Verified-client TLS; host-routed proxy only, no persisted domain state. Missing credentials, invalid host/upstream configuration or listener errors terminate. Separate from workload gateways. |
| `machine-controller` | Simulator machine owner; `-machine-id`, `-control-url`, `-control-ca`, `-client-cert`, `-client-key`, `-zpr-ph`, `-zpr-bootstrap-key`, `-zpr-node-addr`; optional `-zpr-addr` | Starts PH and uses scoped mTLS commands/heartbeat. Certificate CN must equal machine ID. Requires valid organization/generation environment; Linux tunnel/control paths and companion binaries are part of its runtime contract. |
| `machine-control-proxy` | Simulator transport adapter; nonempty `-proxy-listen`, `-proxy-upstream` | TCP forwarding, 3s upstream dial. Listener errors terminate; per-connection dial errors are logged and close that connection. No domain persistence. |
| `test-service` | Simulator test workload; `-listen`, known `-log-workload` | IPv6 HTTP health/benchmark handler; appends workload JSONL. Unknown workload/listen errors fail; handler log errors are logged. |
| `test-client` | Simulator probe; `-listen` destination, IPv6 `-zpr-addr`, nonempty `-client-id`, supported `-log-workload` and `-test-service-name` | Health request validates expected service identity; 3s dial/5s HTTP limit, appends events and prints JSON. Transport/status/identity/log errors fail. |
| `test-log-read` | Simulator log adapter; known `-log-workload` | Reads loopback workload log server `/events`, 3s HTTP limit, copies at most 64 KiB to stdout; unknown workload/non-200/transport errors fail. Not a raw arbitrary-file reader. |
| `benchmark-client` | Simulator measurement; destination `-listen`, IPv6 source `-zpr-addr` | 60s context, one warmup and 100 latency samples, exact 16 MiB transfer, stdout JSON. Wrong status/size or timeout fails; Visa grant timing remains explicitly unmeasured. |
| `stress-service-fleet` | Simulator load workload; IPv6 `-listen`, known `-log-workload`, `-service-count` 100-512 and `-base-port` >=1024 with valid final port | Opens all listeners before serving; closes opened listeners on startup failure. Long-running generated services/events; no persistent fleet catalog. |
| `stress-service-health` | Simulator readiness; IPv6 `-listen`, same count/base-port bounds | Dials each generated endpoint with 250ms timeout; first unavailable service fails. No output database or traffic acceptance claim. |
| `stress-client` | Simulator load workload; IPv6 destination `-listen` and source `-zpr-addr`, supported `-log-workload`, counts/base port, delay/restart flags | Duration 1-180s; clients 50-512. Delay min >=100ms, max <=60000ms; restart interval min >=5s; pause 1-30s with ordered min/max bounds. Writes events and prints bounded-run summary; invalid timing/count/address or failed traffic returns error. |
| `gateway-service` | Simulator fixed-upstream adapter in `gateway_service.go`; IPv6 `-listen`, `-log-workload=internet-gateway`, explicit `-gateway-upstream` | Validated fixed HTTPS origin, safe forwarding and workload events. Not the host-routing browser gateway or a saved gateway-draft deployment. |
| `web-gateway-service` | Shared `internal/gateway` HTTP/CONNECT core, selected by Simulator; IPv6 `-listen`, `-log-workload=internet-gateway`, comma-separated `-gateway-allowed-hosts` | Requires 1-32 valid host patterns, enforces public destination/port restrictions. No manifest discovery or persistent gateway draft state in the core. |
| `web-gateway-client` | Simulator result probe; `-gateway-proxy`, `-gateway-url`, IPv6 `-zpr-addr`, `-client-id`, supported `-log-workload`, optional `-gateway-expected` | Requires IPv6 proxy port 8082; expectation empty/allow/deny. HTTP client limit 25s. Captured response/event JSON survives expectation failure; errors still exit nonzero. Scenario-side target validation is a separate contract, not supplied by CLI dispatch. |
| `business-service` | Simulator `business_simulation.go` plus `internal/simulator/business`; IPv6 `-listen`, known `-log-workload`, `-business-state-dir` | Stateful fictional service under `<state-dir>/<workload>.json`, with workload event logging. Unknown workload, state initialization or listener failure terminates. |
| `business-client` | Simulator business exercise; destination `-listen`, IPv6 source `-zpr-addr`, known `-log-workload` | 20s context, 3s dial/5s HTTP limit, no redirect/proxy discovery; stdout operation record. Actual exercise can mutate fictional business records. |
| `business-health` | Simulator business readiness; destination `-listen`, known `-log-workload` | 10s context, 1s HTTP attempts, validates service identity and simulation marker; readiness timeout/invalid response fails. No business record mutation. |
| `seed-assertions` | Simulator provisioning despite residing in `policy_assertions.go`; `-policy-root` is the organizations directory, `-policy-organization`, `-assertions-db` | Loads fictional profile and validates its assertion source, then seeds the explicitly selected repository through the existing preserving seed helper. Not production default discovery. |
| `compose-policy` | Simulator provisioning in `policy_layers.go`; `-policy-root` is the examples root, `-policy-organization`, `-policy-output` | Reads bootstrap/platform/organization layers and optional Load Lab generation; validates safe paths/size. Writes temporary output beside destination then renames. Parent/path/read/write errors fail. |
| `merge-policy-config` | Provisioning config composition in `policy_layers.go`; `-policy-config-base`, `-policy-config-runtime`, `-policy-config-bootstrap-dir`, `-policy-output` | Merges TOML overlay, resolves `/runtime-include/` keys against the supplied bootstrap directory, checks files; writes mode-0600 temporary output and atomically renames. Missing paths/key/config errors fail. |

The ten integer flags are `service-count`, `base-port`, `client-count`,
`duration-seconds`, `request-delay-min-ms`, `request-delay-max-ms`,
`restart-interval-min-seconds`, `restart-interval-max-seconds`,
`restart-pause-min-seconds`, `restart-pause-max-seconds`. String flags and their
exact dispatch argument wiring are covered by
[`command_compatibility_test.go`](zpr-dashboard/cmd/zpr-web-dashboard/command_compatibility_test.go).
The guard parses source; it does not execute privileged/network workloads or
certify handler behavior merely because the call expression is unchanged.

#### Simulator configuration and persistence

These inputs belong to Simulator/provisioning, not production defaults:

| Input / owner | Default and precedence / failure behavior |
| --- | --- |
| Manifest / `simulator.go` | Trimmed `SIMULATION_MANIFEST`, otherwise CWD-relative `../../.local-runtime/simulation-environment.json`. `SIMULATION_ORGANIZATION_ID` can override the manifest ID; an existing selected-organization file overrides that. Missing selection is allowed, other selection read errors fail; final empty organization falls back to `northstar`. Manifest validation still requires exactly 20 machines. |
| Profile/scenario readers | Trimmed `SIMULATION_ORGANIZATIONS_DIR` / `SIMULATION_SCENARIOS_DIR`, otherwise CWD-relative `examples/organizations` / `examples/scenarios`. Invalid/missing profiles, malformed/oversized scenarios or invalid machine references are errors, not empty catalog defaults. |
| Selection / `organizations.go` | Trimmed `SIMULATION_ACTIVE_ORGANIZATION_FILE`, otherwise `active-organization.txt` beside manifest. Atomic selection-file update follows successful activation. It is not a production organization source. |
| Workspace repository / `workspace_store.go` | Trimmed `SIMULATION_WORKSPACE_DB_DIR`, otherwise `os.UserConfigDir()/zpr-simulator/workspaces`; `<organization>-policy-only.db` remains a Simulator workspace filename. Valid organization required. Protected directory/file, SQLite WAL/full synchronization and immutable scoped revisions remain. The launcher supplies its own state directory instead of relying on the per-user fallback. |
| Directory seeds/publish / `directory_workspace.go` | Profile-relative LDIF files or `SIMULATION_PREGEN_DIR`; pregen fallback is `linux-integration/pregen` beside manifest. `SIMULATION_PUBLISHED_DIRECTORY_DIR` is required to publish; absent setting is HTTP 503, not a new default path. Draft saves do not mutate live LDAP. |
| Stack/reset / `simulator.go`, `organization_activation.go` | Trimmed `SIMULATION_STACK_SCRIPT`, otherwise CWD-relative `scripts/dashboard-stack.sh`. `SIMULATION_ORGANIZATION_RESET_SCRIPT` has no default and must be configured for activation. Reset operation runs under a 15min context with 2min command wait delay; scoped machine runtime state is cleared before/after. |
| Progress/log / `organization_activation.go`, `simulator.go` | Transient `<selection>.progress`; operation status/result remains in memory. Trimmed `SIMULATION_ACTIVATION_LOG_FILE`, otherwise `dashboard-stack/organization-reset.log` beside manifest. Log viewing returns a bounded tail (256 KiB); it does not restore operation outcomes after restart. |
| Controller scope / `machine_control_scope.go`, `machine_control.go` | Controller requires `SIMULATION_ORGANIZATION_ID` and `SIMULATION_RUNTIME_GENERATION`. Listener scope reads manifest-sibling `organization-request.json`; missing request falls back to manifest organization with generation `startup`, malformed existing request fails. Certificate identity, organization and generation remain distinct checks; `/internal/ping` is a TLS startup probe, not scoped readiness proof. |
| Machine-control listener / `simulator.go` | `SIMULATOR_CONTROL_TLS_CERT`, `SIMULATOR_CONTROL_TLS_KEY`, `SIMULATOR_CONTROL_CLIENT_CA` are all-or-none. When configured, trimmed `SIMULATOR_CONTROL_LISTEN` defaults to `127.0.0.1:8791`. TLS 1.3/verified client, scoped readiness/heartbeat/command endpoints; does not share browser operator credentials. |
| Operator web login / `operator_login.go` | `ZPR_SIMULATOR_OPERATOR_CERT_FILE`, `KEY_FILE`, `ORIGIN`, `OIDC_CONFIG_FILE`, `OIDC_SECRET_FILE`, `OIDC_CA_FILE`, `TRUSTED_PEER_IP` use the complete prefix. All unset selects development HTTP; partial TLS/origin/OIDC fails. Exact HTTPS origin, certificate validity and loopback/explicit trusted proxy semantics mirror the neutral web security helper, not production configuration discovery. |
| Operator monitoring consumer / `simulator.go` | Explicit `SIMULATOR_OPERATOR_SERVICE_URL` takes precedence, requires exact HTTPS origin plus `SIMULATOR_OPERATOR_CLIENT_CERT_FILE`, `CLIENT_KEY_FILE`, `SERVICE_CA_FILE`; optional `SERVICE_TLS_SERVER_NAME`. Otherwise `SIMULATOR_CONTROL_ROOM_URL` defaults to `http://127.0.0.1:8787`, with optional `SIMULATOR_CONTROL_ROOM_HOST`. 2s context, no redirects; non-200/decode failures remain errors. This is Simulator calling production, not the reverse. |
| Assistant / `design_assistant.go` | Explicit assistant key/file precedence is preserved. Only Simulator may discover `dashboard-stack/assistant/api-key` beside an explicitly configured manifest. Production assistant isolation is unchanged. |
| Local runtime/workload paths | Controller writes `/run/zpr-workloads/node-address`, control/capture sockets, `/tmp/machine-control-ph.log`; PH CA/CLI are under `/opt/zpr-workloads`. Workload JSONL is `/tmp/zpr-<workload>.jsonl`, not durable run history. Launcher business-state mounts are organization/generation/machine scoped; do not relocate/reuse them as production state. |

The new path regression checks defaults, whitespace trimming for path settings,
manifest-relative selection/log paths, explicit overrides, workspace fallback
and invalid organization rejection. It computes path names only; no private
configuration or running runtime files are opened.

#### Packaging and resource move contract

| Surface | Ownership and relocation obligations |
| --- | --- |
| Shared web executable | `scripts/Dockerfile.simulator` builds `./cmd/zpr-web-dashboard` with Go 1.26/CGO disabled in a module-root context; Alpine runtime copies only the binary. Default entrypoint is Simulator on `0.0.0.0:8788`; launcher overrides the entrypoint/mode for other services. Native mode defaults therefore differ from packaged invocation. The name does not make its production modes Simulator-owned. |
| Launcher assembly | `dashboard-stack.sh` resolves its script/module/project roots, sources `multinode-admin.sh`, `machine-runtime.sh`, `multinode-platform.sh`, and expects sibling repositories and `.local-runtime`. Host paths are mounted explicitly; source examples, shell helpers and exporter are not embedded in the executable. A new package tree must update imports/build contexts, helper paths and mounts together, without adding reverse production dependencies. |
| Machine artifacts | Same command cross-builds into controller/proxy filenames for Linux arm64 and amd64. `Dockerfile.machine` provides Debian networking/runtime tools, not a separately compiled controller. Launcher supplies PH/PH-CLI, CA/bootstrap identities, mounts and architecture selection. Copying one binary does not update all machine artifacts. |
| Gateway runtime image | `Dockerfile.gateway-runtime` overlays a supplied `zpr-web-dashboard` binary at `/app/bin/zpr-web-dashboard` onto an explicitly reviewed base image. It does not compile current source or prove the base's runtime configuration. Independent rollout/protocol validation remains required. |
| Compiler/tester images | `Dockerfile.zpt` uses the workspace-parent context and explicit sibling `zpr-common`/Rust crate copies. `Dockerfile.zplc` uses the sibling compiler repository as context. Moving Go source does not fix or authorize changes to these Rust/compiler build contracts. |
| Embedded UI | `main.go` embeds `static/*`; HTTP serves the `static` sub-FS with current URL/cache/security contracts. Vendor subdirectories are included through Go embedding. Concurrent editor assets remain under this root. File movement needs equivalent embedded paths and browser URL coverage, not only successful Go compilation. |
| Embedded assistant tasks | Seven explicit `skills/*/SKILL.md` embeds in `assistant_skills.go` belong to policy/assertion/config/gateway or Simulator directory/scenario/organization owners. Preserve compile-time inputs and task disclosure/context boundaries when splitting owners. |
| Frontend vendor pipeline | `scripts/vendor-browser-dependencies.sh` uses Node 22, module-root `npm ci --ignore-scripts`, copies pluralize plus license, and now invokes the concurrent editor bundle build. `scripts/build-editor.mjs` maps `browser/code-editor.js` to `static/vendor/code-editor.js` and its licenses. These are observed concurrent work, not verified by this refactoring audit. |
| Browser tests | `tests/browser/dashboard.spec.mjs` resolves assets directly from `../../cmd/zpr-web-dashboard/static/`; Playwright uses desktop/tablet Chromium. Container runner cross-builds Linux setup/enrollment/web-command test artifacts first and mounts module/test binaries separately. Relocation must update source fixture roots and cross-build inputs together. A passing fixture test is not a live deployed service test. |
| Local Make | `make build/run/install` target the TUI `cmd/zpr-dashboard`, not the web executable. `make test` covers `./...`. `test-browser` delegates to its container script. Existing `status` target references undefined `SIM_PATH`; do not treat it as web/package acceptance or silently repair it during this audit. |
| CI | `.github/workflows/zpr-dash.yml` filters dashboard/workflow changes, delegates Go build/test to upstream reusable workflow `v1.5.0`, and separately runs Node 22 `npm ci`, Chromium install, browser tests and failure artifacts. The referenced remote reusable workflow implementation was not fetched; its internal compile/package matrix is not certified here. |
| Enrollment installers | Separate `cmd/zpr-enrollment-setup` and `packaging/enrollment` owner. Mac packaging requires native arm64/CGO Security.framework and explicit macOS 13 deployment target; Windows packaging consumes a PE amd64 binary and NSIS. These are not packaging for the shared web command; signing, notarization, native OS behavior and privileged lifecycle certification stay in the provisioning plan. |

**Acceptance for a future move:** preserve the dispatcher/flags/path checks,
focused machine-scope/workspace/policy-layer/workload/gateway tests and both
Linux command builds; inspect the revised embed/resource and launcher inputs.
Keep runtime output paths stable unless a separately reviewed behavior change
explicitly replaces them. Never infer production independence from the shared
command's name, a TUI-only build, static UI checks or a successful image build.

**Local verification:** all three new compatibility regressions pass, together
with selected machine-scope, workspace revision, policy-layer/merge,
business-state mount and independent-fixture checks under `go test -race`.
`go vet ./...`, CGO-disabled Linux arm64 and amd64 command builds, new-file
formatting, diff whitespace and editor diagnostics pass. Required `make test`
still has exactly the same four baseline failure groups: enrollment readiness,
stale-bundle platform fixture, placement shell extraction and trusted-service
ELF fixture. None of those tests was changed. The editor test runner initially
ran two checks, then reported no discovery after the path check was added;
terminal Go results cover the final three tests. No full browser/image/
installer/remote-CI run, service restart, deployment, commit or push is claimed.
The added code is tests only; runtime behavior is unchanged by this audit.

**Remaining M1 gates at this checkpoint:** nested production enrollment/gateway/
review mapping is recorded in the subsequent audit below. Cross-domain calls
beyond the traced fixture/assistant/monitoring and nested paths, plus actual
installer/CI/resource acceptance, remain open for
the intended move. This audit advances configuration/default/path coverage;
it does not close M1 or any of the seven milestones.

### M1 nested production contracts audit (2026-10-15)

This bounded source audit covers `enrollment.go`, `operator_delegation.go`,
`gateway_api.go`, `gateway_config.go`, `gateway_config_store.go` and
`change_review.go`, following their production composition and existing package
contracts. It does not cover device/BAS enrollment protocols, installer
qualification, or every Admin snapshot enrichment/provider path.

#### Composition and authorization owners

- Control Room registers change review locally, enrollment through its dedicated
  named-user proxy, and other `/api/` requests through
  `internal/controlroom.AuthorizeAPI`. That package maps gateway permissions;
  `internal/operatorauth` checks the active session, organization grant and
  permission. Mutations additionally require the configured exact Origin and
  `X-ZPR-CSRF` proof. A missing organization is 503; unmapped routes and denied
  grants are 403. The command's unconfigured-auth local-development fallback
  remains separate and explicitly blocks both enrollment prefixes.
- The Control Room upstream proxy requires an explicit HTTPS origin and client
  certificate/key/CA. It strips browser credentials, Origin, fetch metadata,
  CSRF and browser-supplied delegation headers. Only a server-signed assertion
  attached through request context can reach the private delegation prefix.
- Control-Service requires TLS 1.3 and verified client certificates. Enrollment
  adds explicit certificate pins or independent signed-user grants; gateway
  handlers use the service's configured organization, not a browser-provided
  organization. The shared local-request check accepts verified mTLS, otherwise
  a loopback/trusted-proxy peer plus loopback Host and matching Origin. This
  check is not a substitute for named-user authorization.

| Endpoint / method | Permission and organization contract | Result / failure contract |
| --- | --- | --- |
| Enrollment `GET /api/enrollment/v1/catalog` | `read`; direct configured certificate principal or named-user catalog session | Only principal-authorized catalogs/capabilities. |
| Enrollment `GET .../preflight`, `GET .../invitations`, `GET .../invitations/{id}` | `read`, explicit query `organization` | Scoped readiness/detail/list; list defaults to 50, supports limit 1-100 and `after` cursor. |
| Enrollment `POST .../preflight`, `POST .../invitations` | `create`, organization in strict JSON body | Saves readiness / creates invitation (201). Creation requires current scoped preflight; named-user creation also requires `gui_invitation_creation_enabled`. |
| Enrollment `POST .../invitations/{id}/{cancel,approve,reject}` | Corresponding permission, explicit body organization | Named-user cancellation and decisions bind revision and key fingerprints; direct certificate cancellation keeps its existing simpler body. Approval rechecks current profile/type catalog. |
| Gateway `GET /api/gateways/contracts` | `gateway.read`, configured Control Room and backend organizations | 200 installed Gateway services joined to actor CNs; 503 for incomplete/unavailable Admin inventory. |
| Gateway `GET /api/gateways/configs`, `GET .../configs/{instance}` | `gateway.read` | Saved scoped drafts can be read without live inventory; no store is 503, absent record 404, storage failure 500. |
| Gateway `POST /api/gateways/config/check` | `gateway.analyze` | Live installed contract required; missing identity 400, no matching contract 409, invalid draft 422, valid draft 200. Does not save/apply. |
| Gateway `POST .../configs/{instance}/revisions` | `gateway.edit` | Required nonnegative `expected_revision`, live contract and valid draft; stale revision 409, invalid draft 422, saved record 200. Does not apply. |
| Gateway `GET /api/gateways/assistant`, `POST` same path | `gateway.read` / `gateway.analyze` | Separate assistant routes take precedence over the generic gateway handler; task prompts/context remain owned by existing assistant composition. |
| Review `GET /api/change-review/capabilities` | `change.read`, explicit query organization | Reports self-approval opt-in and `application_enabled:false`. |
| Review `GET .../requests`, `GET .../requests/{id}` | `change.read` plus both `policy.read` and `gateway.read` for list, or saved record's domain `.read` for detail | Scoped queue, 50 records/page, offset 0-100000; 404 missing record. |
| Review `POST .../requests` | `change.submit` plus selected domain `.edit`, explicit body organization | Captures current saved policy/gateway revision from production services, returns 201; invalid input 400, unavailable/stale source 409. Browser source text is not accepted. |
| Review `POST .../requests/{id}/{approve,reject,apply}` | `change.review` plus saved domain `.read` | Approval recaptures and compares source/validation context. Digest/version/state conflicts and default self-approval are 409. Apply always returns 409: approval is not deployment. |
| Review `POST .../requests/{id}/cancel` | `change.submit` plus saved domain `.read` | Store also requires the original issuer/subject; digest/version/state and authorship conflict is 409. |

Permission mapping accepts HEAD for gateway reads, but the generic gateway
handler explicitly branches on GET and returns 405 for HEAD. Record that
distinction; do not silently turn authorization coverage into a promise of
backend HEAD support during a move. Gateway check diagnostics locate a field
only in the exact submitted JSON; revision-save validation currently omits
`source_line`. Review compiler warnings are captured as context, not remapped
to invented source locations.

#### Configuration, limits and persistence

- Enrollment uses both `ZPR_ENROLLMENT_CONFIG_FILE` and
  `ZPR_ENROLLMENT_DATABASE_FILE`. Neither set yields an explicit 503 handler;
  partial/invalid configuration fails startup before opening the registry.
  Configuration is strict version-1 JSON, at most 65536 bytes. Invitations
  require explicit lifetime (1 second to 30 days); review permissions require
  approval lifetime. Request bodies require JSON media type, one strict
  document, no mutation query and an exact 8192-byte cap.
- `internal/enrollment` owns the registry: paths are resolved absolute, parent
  directories created 0700 and the database chmodded 0600; SQLite uses WAL,
  FULL synchronous, immediate write transactions, one connection and 5s busy
  timeout. Invitations, reviews/key bindings, audit, scoped readiness and
  delegation replay are registry state, not Simulator workspace state.
  Readiness attestations expire after 30 days; missing readiness returns 409.
  Preserve the distinct path/permission semantics rather than replacing them
  with change-review or gateway-store rules.
- Named-user enrollment needs explicit delegation signer/key in Control Room
  and trust configuration in Control-Service. Signing requires HTTPS/OIDC,
  exact backend audience and a protected PKCS#8 Ed25519 key. The private
  `/api/operator-enrollment/v1/` route rejects browser credentials and verifies
  request-bound signatures, independently configured grants and single-use
  replay state. Tokens last 30s; the registry bounds replay state to 8192
  unexpired entries. No trust file yields an explicit unavailable handler;
  configured trust without enrollment catalogs fails startup.
- Gateway drafts use optional `ZPR_GATEWAY_CONFIG_STORE_DIR` and explicit
  `ZPR_GATEWAY_ORGANIZATION_ID`. The root must be absolute and a real directory;
  root/org directories are 0700, records 0600. Writes use a process-local mutex,
  expected revision, a synced temporary file and atomic rename. This is not
  cross-process compare-and-swap. Preserve JSON values and ascending revision
  order, retaining the newest 50 revisions; records are capped at 4 MiB.
  Existing record encoding compacts JSON whitespace on disk/reopen; source-line
  diagnostics still use the exact submitted check request.
  Drafts are at most 64 KiB and strictly bind schema/org/instance/adapter/service.
  The shared JSON envelope decoder has a separate 4096-byte allowance; it is
  not the enrollment decoder. No startup path discovers Simulator files.
- Change review is Control Room-owned via `ZPR_CHANGE_REVIEW_CONFIG_FILE`,
  strict version-1 JSON up to 65536 bytes and named-user auth. Its database path
  must be absolute, with a private nonsymlink parent and private regular file.
  `internal/changereview` owns transactional immutable proposal/digest and
  append-only event records; decisions atomically increment version. Submitted
  before/after source totals at most 256 KiB. `allow_self_approval` defaults
  false. Review requests reject noncanonical paths, unexpected/duplicate query
  keys and mutation queries; their strict JSON decoder uses an 8192+4096 byte
  envelope bound, not enrollment's exact 8192-byte limit.
- Review capture calls the existing production mTLS proxy with a 20s deadline
  per call and a 512 KiB response cap. Gateway capture requires installed
  contract plus current saved revision and its immediate predecessor. Policy
  capture requires configured org/current unarchived policy, predecessor,
  compiler check and a final reread to reject edits during validation.
  Approval recaptures before/after/base/validation/impact. No Simulator endpoint
  or activation contract is involved.

#### Remaining source coupling and verification

At this audit checkpoint, Gateway API still consumed the command's broad `snapshot` through `application`
and `fetchSnapshot`; a cohesive production owner needs an explicit inventory
input instead. `parseGatewayInstanceConfig` also calls `parseGatewayUpstream`,
a pure validator currently located in Simulator workload file
`gateway_service.go`. Move that neutral validation with its callers, not the
workload runtime. The following extraction repairs these gateway source
couplings; the Admin application adapter remains command-owned. Review capture
similarly shares policy/gateway command DTOs
and validators; preserve revision ordering and production validation when
introducing owner interfaces. These are source-ownership couplings, not evidence
that these audited handlers read simulation manifests at runtime.

Four new `TestProductionContract*` regressions pass under race: gateway permission
wiring (including denied requests never reaching upstream), raw/scoped draft
persistence across reopen and failed inventory saves, configured enrollment
readiness/invitation persistence across close/reopen with certificate/scope
checks, and review capture's exact 512 KiB/one-byte-over/status/JSON boundary.
Tests deliberately point Simulator URLs and paths at unavailable locations.
Enrollment supplies a constructed verified-chain request state; it does not
claim a new network TLS/OIDC acceptance run. Existing delegated-browser tests
and the complete six audited internal-package suites pass under race.
Focused command tests, `go vet ./...`, formatting and editor diagnostics pass.
Required `make test` retains the same four baseline failure groups; none was
modified. This slice changes tests/documentation only; no build/image/installer
qualification, restart, deployment, commit or push is claimed.

**Next bounded move at this checkpoint (completed by the following slice):** give gateway draft validation/storage an independent
production owner and explicit installed-inventory contract, preserving this
baseline and the neutral runtime boundary. Continue remaining cross-domain
and packaging acceptance work before closing M1. All seven gates stay open.

### Fourteenth extraction: production gateway draft owner (2026-10-15)

**Verified locally; not deployed, committed or pushed.** This M2 slice implements
the nested audit's next bounded move without changing API routes, permission
mapping, configuration, on-disk schema or activation semantics.

- Moved configuration types/validation, source-owned JSON diagnostics and the
  versioned file store from the command into
  [`internal/controlservice/gatewaydraft`](zpr-dashboard/internal/controlservice/gatewaydraft/).
  The command's `gateway_draft.go` contains type aliases and forwarding adapters
  only, retaining existing assistant/review DTO compatibility. There is no
  second validator or store implementation.
- The production package accepts explicit `Inventory` values (`ActorCNs` and
  minimal service identity metadata) and a context-aware `Reader` contract.
  Installed contracts retain case-insensitive Gateway-kind filtering, actor-CN
  joins, identity validation, external-network metadata, original service order
  and nonnil empty lists. No Admin snapshot, Simulator types, configuration
  discovery, HTTP client or runtime deployment is owned by this package.
- `gateway_api.go` now accepts only that reader, not `snapshot` or `application`.
  `gateway_inventory.go` projects the existing Admin snapshot into independent
  values and rejects disconnected/partial/error-bearing snapshots before
  projection. This deliberately preserves the old broad health gate, including
  unrelated snapshot errors, rather than silently relaxing it. Reading saved
  drafts still does not fetch live inventory; checking/saving still requires it.
  Control-Service startup wires this adapter; missing Admin configuration and
  reader errors remain explicit unavailable responses.
- Moved the pure fixed-upstream parser into neutral `gateway.ParseUpstream`.
  Simulator's fixed gateway and manifest validation retain a forwarding adapter;
  production drafts call the neutral function directly, then enforce their
  additional FQDN/port/path constraints. The neutral package still has only
  standard-library dependencies. No workload logging or runtime moved into
  production.
- Revision numbers, process-local locking, 50-revision retention, 4 MiB record
  cap, 64 KiB configuration cap, private paths/modes and atomic synced-file
  replacement are unchanged. Store encoding still compacts JSON whitespace
  during persistence; this is not a new format or migration. Check diagnostics
  remain tied to the exact submitted source. Approval remains draft review,
  never deployment.

New package tests cover installed-contract filtering/order/metadata, exact
configuration size and identifier bounds, diagnostics (including missing fields,
non-source failures and last duplicate field), retention across reopen, existing
JSON encoding, private file modes, temporary-file cleanup, concurrent stale
writers, organization isolation, list order and corrupt-record refusal.
The low-level diagnostic assertions previously in the command API test moved
with their implementation; its exact HTTP `source_line` assertion remains.
Existing passing store tests now assert their explicit root path rather than
accessing the package's private field. No existing failing test was changed.
Command tests additionally exercise the API with an independent inventory
reader, populated inventory plus errors, canceled context, offline draft reads,
unchanged HEAD handling and health/identity projection with Simulator unavailable.
An allowlist dependency test permits only this package and neutral `gateway`
outside the standard library.

Verification:

```sh
go test -race ./internal/controlservice/gatewaydraft ./internal/gateway ./cmd/zpr-web-dashboard -run '^Test(Gateway|ParseGateway|ParseUpstream|ConfigSize|DiagnosticLine|InstalledContracts|Store|ProductionContract|ChangeReview|InternetGateway)' -count=1 -timeout=180s
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/zpr-web-dashboard-arm64 ./cmd/zpr-web-dashboard
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/zpr-web-dashboard-amd64 ./cmd/zpr-web-dashboard
```

The race selection, vet, both cross-builds, formatting and editor diagnostics
pass. Actual cross-build outputs used a temporary file removed on exit. Required
`make test` still fails only the four recorded baseline groups: enrollment
readiness preflight, stale-bundle platform fixture, placement shell extraction
and trusted-service ELF fixture. Browser/Rust/container/installer/live-service
acceptance was not run for this backend-only slice. Concurrent operator-access,
editor and system-packaging work is preserved and is not part of this extraction.

**Remaining:** HTTP/error/authorization composition, Admin snapshot acquisition
and enrichment, assistant/review DTO composition still share the command.
This package's boundary is enforced, not the entire application's. Next audit
the production inventory acquisition/enrichment contract before moving that
composition; continue the remaining M1 cross-domain and packaging acceptance.
All seven milestone gates remain open. A runtime rollout is separately authorized.

### M1 production inventory acquisition and enrichment audit (2026-10-15)

**Verified locally; tests/documentation only.** This slice traces the existing
Admin client and snapshot assembly in `main.go`, display names in
`node_display_names.go`, telemetry in `node_denials.go` and the relevant
diagnostics provider/catalog contracts. It maps the production consumers before
moving composition. No runtime source, configuration or persistence changed.

#### Admin client and transport contract

- `runControlService` constructs one `adminClient` for the application.
  Configuration is explicit `ZPR_ADMIN_URL`, `ZPR_ADMIN_CA_FILE`, and trimmed
  `ZPR_ADMIN_API_KEY` or `ZPR_ADMIN_KEY_FILE`. A nonempty inline key wins even
  if the file is unavailable; file contents are trimmed. There are no default
  Admin credentials, URLs or Simulator-derived paths. The URL loses trailing
  slashes but is not whitespace-trimmed; current validation requires HTTPS,
  a host and no non-root path. It is not the stricter service-origin validator.
- The existing transport has an 8s HTTP timeout, refuses redirect following,
  and sends `X-API-Key` on GETs. TLS minimum is 1.2. A custom
  `VerifyConnection` validates chain/time/server usage against system roots plus
  the configured CA. Despite `InsecureSkipVerify` being used to replace default
  validation, certificate verification is not disabled. `ZPR_ADMIN_SERVER_NAME`
  supplies the optional hostname check; empty retains historical chain-only
  verification. Do not silently replace this with either no verification or
  mandatory URL-host matching during a source move.
- Non-200 status becomes typed `adminStatusError` after draining at most 4096
  bytes. Success decodes one JSON value through a 2 MiB limited reader; unknown
  fields and trailing values are currently accepted. This is a decoder window,
  not strict whole-response length enforcement. Context cancellation propagates
  through the request. No retry or last-good snapshot cache exists here.
- Prior art was checked in TUI `internal/dataplane`: it has different config
  defaults, configured-CA-only trust, default timeout, connection lifetime and
  response handling. Reusing it unchanged would alter the web contract. The
  next transport owner should accept explicit inputs and preserve this web
  behavior rather than importing TUI environment/default discovery.

#### Acquisition, health and wire-shape contract

`handleSnapshot` returns HTTP 200 JSON with `Cache-Control:no-store`, including
operational failures. No Admin client means `api_status:"not configured"` and
the explicit configuration error. Configured requests get a 10s parent-context
deadline. `fetchSnapshot` itself does not create a deadline; diagnostics uses
its own 18s context, while the gateway reader inherits its caller context.

Seven initial GETs run concurrently. Results arrive in completion order;
actor/service/visa detail expansion is processed as those results are received.
Final error strings are sorted, not ordered by request completion.

| Admin path | Acquisition / retained data | Failure behavior |
| --- | --- | --- |
| `/admin/stats` | `stats` string map | Top-level `stats:` error. |
| `/admin/actors` | Deduplicate listing by CN, concurrently fetch `/admin/actors/{escaped CN}`, sort actors by returned CN | Listing error is top-level; failed detail retains `actor{CN:requested}` plus scoped error. No detail worker cap currently exists. |
| `/admin/network` | Preserve returned network order and metadata | Top-level `network:` error. |
| `/admin/services` | Concurrent `/admin/services/{escaped ID}` details, sort by name | No listing deduplication or worker cap. Failed detail retains a zero-value service plus scoped error. |
| `/admin/trusted-services` | Status list enriched by fetched service metadata; sort by name | Only HTTP 404 is optional (older Admin API), without a top-level error. Other failures add `trusted:` error and use service-kind fallback inventory. |
| `/admin/visas` | Count listing entries, sort IDs descending, fetch all details with a 16-request semaphore | Missing detail 404 is expected expiry/removal and is skipped; other detail errors/cancellation mark the inventory incomplete. Successful details still populate bounded recent history. |
| `/admin/visas/denies?limit=20` | Retain returned denial ordering | Top-level `denies:` error; no local sort or additional truncation. |

Successful top-level endpoints determine reachability: zero successes means
`disconnected`; otherwise any top-level/detail/display-name error means
`partial`; otherwise `connected`. Detail failure still counts its successful
listing endpoint. Optional trusted-status 404 does not count as a success.

`recent_visas` is the newest ten successfully fetched entries by listing-ID
order, not necessarily currently unexpired. `visa_count` is listing length, not
successful-detail or active count. Complete visa inventory yields
`active_visas:[]` or the unexpired grants; incomplete/unavailable inventory yields
`active_visas:null`. That distinction must survive DTO relocation.
Trusted-status success with an empty list emits `trusted_sources:[]`; optional
404 fallback with no trusted services emits `null`. Not-configured HTTP
responses explicitly emit empty arrays/map except `active_visas:null`.
Other successful Admin `null` values are not globally normalized.

`handleActorVisas` is a separate 15s read: fetch actor/address, then all visa
details sequentially, skip detail 404, filter unexpired source/destination by
parsed IP equality, sort ID descending. It does not use the ten-item recent
window. Missing client is 503; invalid actor/address or non-404 fetch failure is
502, never a success-shaped empty list.

#### Enrichment order and explicit operator inputs

The current order is important:

1. Build trusted inventory from fetched status/services.
2. Merge `ZPR_PLATFORM_SERVICES`, then sort services/actors.
3. Apply `ZPR_NODE_DISPLAY_NAMES_FILE`.
4. Query node denial/counter telemetry through configured diagnostics inputs.
5. Compute API health and sort errors.

Platform descriptors may add service entries but never invent actors.
Existing service names keep Admin identity/address/endpoints; configured
Gateway-kind/external-network metadata can mark the matching service as Gateway.
Platform-only trusted descriptors are not retroactively added to the already
built trusted list. Malformed platform JSON is logged and omitted by existing
code; it does not create a snapshot error. A source extraction must not
silently reinterpret these semantics as new authoritative inventory.

Display names read explicit JSON on every refresh (64 KiB maximum), require
unique normalized IPv6 addresses and nonempty trimmed names up to 80 runes,
and apply only to nodes. Invalid/unavailable configuration adds a top-level
error and makes an otherwise connected snapshot partial.

Node telemetry reselects its provider per snapshot. Explicit
`ZPR_NODE_DENIAL_METRICS_FILE` selects a <=64 KiB identity-to-absolute-OTLP-file
map; otherwise a `node-denial-metrics.json` sibling of explicit
`ZPR_DIAGNOSTICS_CONFIG_FILE` is tried. Only an absent implicit sibling falls
through to configured OpenObserve. OTLP files are <=64 KiB and bind exact
service/instance resource identity. No Simulator manifest discovery is involved.
OpenObserve selection uses explicit diagnostics config/credentials, validated
organization/streams and HTTPS or permitted private/loopback origins.

`ZPR_DIAGNOSTICS_SOURCE_MAP_FILE` optionally overrides telemetry service/instance
IDs; existing missing/invalid/over-128-KiB mapping returns no mappings.
The shared diagnostics catalog joins nodes, required services and trusted
services, sorts by kind/name and truncates to 100. Node enrichment consumes that
catalog, requests one provider result per node sequentially, clears old fields,
and accepts metrics at most `min(provider stale-after,10s)` old.
Counters retain decimal uint64 strings, latest-sample selection, management/
fastpath completeness, oldest included timestamp and sorted group/name order.
Missing/stale/invalid/query-failed telemetry populates actor-local error fields,
not snapshot errors; it does not independently change `api_status`.

Provider-manager links are explicit `ZPR_PROVIDER_MANAGER_URLS`. The legacy
`ZPR_DEMO_LDAP_EDITOR_URL` is also explicit and only maps `demo_ldap`/`rest/1`
to an approved loopback HTTP link; it does not query Simulator. Retain the
legacy name as compatibility until a separately approved naming change.

#### Consumers, ownership and next move

Production routes consume the assembled snapshot directly for monitoring,
diagnostics catalog/enrichment and gateway inventory. The gateway adapter
deliberately rejects every snapshot error, including an unrelated display-name
failure; actor-local telemetry errors do not trigger that gate. Simulator
consumers read the operator contract or share DTOs; its Business-denial fallback
also constructs `newAdminClient` for a direct Admin read. Moving the client into
a production package can preserve that allowed Simulator-to-production
dependency, not introduce the reverse.

No Simulator reads were found in the traced acquisition/enrichment functions.
The mixed command file still contains Simulator dispatch and broad shared DTOs;
this source observation is not a full transitive application-boundary proof.
Diagnostics catalog/provider and node enrichment types are intertwined, so do
not copy them or pull the whole diagnostics HTTP layer into an Admin transport
package.

**Next bounded extraction at this audit checkpoint (completed below):** an explicit-config production Admin read client
under `internal/controlservice`, with current typed status/decoder/TLS behavior
and a command-local environment adapter. Then compose snapshot DTO/acquisition
and enrichment through explicit providers while preserving the above ordering,
caps and health shapes. Keep HTTP authorization, Simulator reads, TUI defaults
and operator configuration discovery out of the client. No new package is
needed solely for a line-count split.

Five new `TestInventoryCompatibility*` regressions verify real local TLS/key
precedence, configured-name and untrusted-chain rejection, chain-only
compatibility, redirect refusal, exact 2 MiB decoder boundary and existing
single-decode behavior; health/error/placeholder/null-versus-empty responses;
platform/trusted/display/telemetry order and gateway health gating; and an
observed 16-request visa concurrency maximum with descending result order.
Tests use synthetic Admin servers, explicit temporary files and unavailable
Simulator URLs/paths. No live configuration, credentials or runtime state were
read. Existing snapshot/actor/visa/geography/node telemetry/gateway adapter race
checks pass; the five new tests also pass three consecutive runs under race.
`go vet ./...`, formatting, whitespace and editor diagnostics pass. The editor
test tool found no tests; terminal results are authoritative. Required
`make test` retains exactly the same four known failing groups, unchanged.
No runtime edit, build/installer qualification, service restart, deployment,
commit or push was performed for this audit. All seven milestone gates remain
open, including remaining cross-domain and packaging acceptance.

### Fifteenth extraction: explicit-config production Admin read client (2026-10-15)

**Verified locally; not deployed or published.** The production transport now
belongs to
[`internal/controlservice/adminapi`](zpr-dashboard/internal/controlservice/adminapi/).
Its `Config` accepts the URL, inline key or key-file path, CA-file path and
optional server name. There is no environment/default discovery, Simulator
lookup, TUI configuration import, snapshot DTO or HTTP authorization here.
The dependency regression permits only the standard library.

The command's
[`admin_client.go`](zpr-dashboard/cmd/zpr-web-dashboard/admin_client.go)
reads the same five environment variables and preserves all existing
configuration messages. Its thin read adapter retains the existing private
client fields for command fixtures and translates the package status error
back to the existing command type. There is one TLS/HTTP/decoder implementation,
not a second transport. Control-Service startup, snapshot/actor/visa/gateway
consumers and the allowed Simulator Business-denial caller remain unchanged.
No existing test was rewritten to accommodate the move.

The owner preserves URL validation and trailing-slash behavior, trimmed
inline-key precedence/file fallback, system-plus-configured CA roots, TLS 1.2
minimum, optional name/chain-only verification, 8s timeout, redirect refusal,
GET/API-key headers, caller context, typed status errors, 4096-byte error
drain and one-value 2 MiB JSON decoding. This does not harden URL semantics,
require name checking, reject trailing JSON, add retries/cache or otherwise
change the audited contract.

Five package tests cover explicit inputs despite unavailable environment
values, real local TLS/name rejection, configuration errors/file keys,
status/drain/close/cancellation/no retries, exact decoder limits,
unknown/trailing JSON, redirects and dependency/environment boundaries.
A new command test guards exact configuration-error messages. Existing
inventory and production-contract race tests pass without changes, including
untrusted-CA rejection and snapshot health/wire/enrichment/concurrency shapes.
The new package passes three consecutive race runs; the editor test runner
also reports 22 passing cases for the new owner/adapter tests.
`go vet ./...`, full-module formatting, whitespace/editor diagnostics and
CGO-free Linux arm64/amd64 command builds pass. Required `make test` retains
the same four baseline failure groups: enrollment readiness, stale-bundle
platform fixture, placement shell extraction and trusted-service ELF fixture.
Tests use synthetic endpoints/files; no live credentials or runtime state.

```sh
go test -race ./internal/controlservice/adminapi
go test -race ./cmd/zpr-web-dashboard -run '^Test(AdminClientConfiguration|InventoryCompatibility|FetchSnapshot|ActorInventory|ActorVisas|ActorSnapshot|MergePlatform|MapSnapshot|VisaOrdered|NodePacket|NodeDenial|NodeDisplay|GeographyNode|GatewayInventory|GatewayAPIIndependent|ProductionContract|CommandCompatibility)' -count=1 -timeout=180s
```

**Next bounded move at this checkpoint (completed below):** give production snapshot DTO/acquisition an independent
owner and explicit inputs, then compose enrichment through explicit providers.
Preserve seven-read completion/health behavior, detail limits, optional 404s,
ordering and null-versus-empty shapes. Keep authorization/routes, environment
discovery and Simulator dispatch outside that owner; do not pull the diagnostics
HTTP layer into the transport. All seven milestone gates remain open.

### Sixteenth extraction: production snapshot inventory owner (2026-10-15)

**Verified locally; not deployed or published.** The production
[`internal/controlservice/inventory`](zpr-dashboard/internal/controlservice/inventory/)
package now owns snapshot/actor/node/network/service/trusted/visa/denial wire
types, seven concurrent Admin reads, detail expansion, trusted-source assembly,
explicit platform-service merging and final health/error sorting.
It accepts an explicit `Reader` (`GetJSON`) and `Options`; it performs no
environment, filesystem, Simulator or TUI discovery. Its only non-standard
dependency is the production `adminapi` package for typed HTTP status errors.

The command's
[`inventory.go`](zpr-dashboard/cmd/zpr-web-dashboard/inventory.go)
keeps compatibility type aliases and thin detail/helper adapters. It supplies
an explicit Admin client and trusted-editor-link resolver, then synchronously
composes platform metadata, display names and telemetry through `Enrich`.
The callback runs after trusted assembly and before health is finalized, so
display-name errors still make the snapshot partial and block gateway checks,
while actor-local telemetry failures still do not. Provider/link configuration
is read at the same stage as before, not eagerly cached at startup.

HTTP route authorization, no-client response, response headers and 10s snapshot/
15s actor-visa deadlines stay in the command. The sequential actor-visa route
is unchanged, not replaced by the snapshot's concurrent visa acquisition.
All existing consumers, including diagnostics, gateway projection and Simulator
callers, keep the same JSON contract through aliases. No existing test was
changed. This is a source ownership move, not a claim that the entire mixed
command or transitive provider graph is now separated.

Preserved contracts include:

- Seven top-level reads and completion-order detail processing, without an
  added owner deadline; only successful top-level endpoints establish
  reachability. Optional trusted 404 does not count as a success.
- Actor CN deduplication/escaping and requested-CN failure placeholders;
  service non-deduplication, escaping and existing failure values. Their
  detail fanout remains uncapped.
- Visa listing count, descending ID order, 16-detail concurrency limit,
  expected 404 removal, incomplete-inventory errors, newest-ten successful
  recent details and complete unexpired active inventory.
- Original trusted/provider metadata/health strings, explicit manager-link
  resolution, platform identity-preserving merge and no invented actors.
- Sorted final errors, enrichment-before-health, trusted-before-platform
  ordering, all JSON tags and `null` versus empty shapes, including successful
  Admin null stats/network/denials.

New package tests verify simultaneous top-level launch with a barrier,
unchanged caller context, exact Admin path/query surface, health/reachability/
enrichment order, detail failures, missing versus failed visas, wire shapes,
identity escaping/deduplication, complete active/recent inventories, retained
network/denial ordering, provider joins, cancellation and dependency/discovery
boundaries. Existing command inventory/snapshot/actor/visa/geography/node/
gateway/diagnostics/production/dispatch compatibility race tests pass unchanged.
The package passes three consecutive race runs. Full-module formatting,
`go vet ./...`, whitespace/editor diagnostics and CGO-free Linux arm64/amd64
command builds pass. Required `make test` retains exactly the four known
baseline groups: enrollment readiness, stale-bundle platform fixture, placement
shell extraction and trusted-service ELF fixture. No live credentials,
configuration or runtime state were inspected.

```sh
go test -race ./internal/controlservice/inventory ./internal/controlservice/adminapi -count=3
go test -race ./cmd/zpr-web-dashboard -run '^Test(AdminClientConfiguration|InventoryCompatibility|FetchSnapshot|ActorInventory|ActorVisas|ActorSnapshot|MergePlatform|MapSnapshot|VisaOrdered|NodePacket|NodeDenial|NodeDisplay|Geography|Gateway|DemoLDAPEditorURL|ProviderManager|TrustedSources|ProductionContract|CommandCompatibility|DiagnosticsSources|Simulator.*(Snapshot|Operator))' -count=1 -timeout=180s
```

**Next bounded move at this checkpoint (completed below):** separate explicit display-name/telemetry enrichment
contracts from diagnostics HTTP/provider discovery, then move the production
implementation behind them. Audit the shared catalog, freshness and error
semantics before moving that intertwined graph. Keep operator configuration and
HTTP authorization in composition, preserve refresh-time reads and existing
health gating, and never add reverse Simulator dependencies. All seven
milestone gates remain open.

### Seventeenth extraction: explicit node inventory enrichment (2026-10-15)

**Verified locally; not deployed or published.** Explicit display-name loading/
application and node denial/counter enrichment now live in
[`internal/controlservice/inventory`](zpr-dashboard/internal/controlservice/inventory/).
Display-name application receives a file path, not an environment variable.
`NodeTelemetry` receives the existing catalog source IDs, a context-aware query
callback, provider freshness and provider-error text. Its nil query denotes an
unavailable provider; empty/missing catalog identity is a distinct actor-local
error. The production metric DTO retains the exact diagnostics JSON tags and
is shared through a command alias, without importing diagnostics HTTP types.

The command still reads/normalizes the explicit display-name environment path
on every refresh, selects the OTLP/OpenObserve provider, loads diagnostics
mappings and builds the same sorted/capped catalog. Its query adapter passes
the complete original first-matching diagnostics source and limit 1 to the
provider, preserving source identity, provider-specific node behavior and the
100-source catalog limit. It does not reconstruct uncapped identities or query
all nodes independently of that existing catalog. OTLP file parsing and
OpenObserve transport/configuration remain command-owned for a subsequent
cohesive extraction; no reverse production-to-Simulator dependency was added.

The owner preserves:

- Display JSON's 64 KiB limit, single-value/non-null map contract, IPv6
  normalization and duplicate-address rejection, nonempty trimmed/control-free
  names up to 80 runes, node-only application and identity preservation.
  The explicit file is re-read per refresh. Invalid configuration still adds
  the same top-level error and affects snapshot/gateway health; unconfigured
  paths remain optional.
- Sequential eligible-node queries with the caller context, reset of previous
  counter/denial values and errors before every refresh, nil-provider/missing-
  identity/query-error distinctions and original error text.
- Latest-sample selection (first retained on equal timestamps), decimal uint64
  values without float conversion, 512 distinct counter names, management and
  complete fastpath groups, sorted group/name output and the oldest included
  counter timestamp.
- Provider freshness capped at 10s, acceptance at the exact threshold and
  rejection of older/future/zero/invalid samples. Denial values and counter
  completeness fail independently; telemetry errors stay actor-local rather
  than changing top-level API health.

New owner tests cover exact/over byte, rune, counter-count and freshness bounds,
refresh-time file changes, node/identity scope, latest/tie timestamps, uint64
maximum/overflow, complete counter ordering, stale-data clearing, explicit
catalog/context/query ordering, provider and identity errors, freshness caps
and independent denial/counter failures. A new command contract regression
verifies the exact 100-source capped catalog, complete source DTO forwarding,
mapping, original actor query order and limit 1 with Simulator unavailable.
The dependency guard now covers every runtime file and permits file access
only in the explicit display-name loader; environment discovery remains banned.
No existing tests were modified.

The inventory package passes three consecutive race runs; unchanged node,
diagnostics, inventory, snapshot, gateway, geography and production-contract
consumer races pass. Editor-discovered cases pass; terminal race runs cover
the complete selected suites, including the final rune-bound assertions.
`go vet ./...`, full-module formatting, whitespace/editor
diagnostics and CGO-free Linux arm64/amd64 command builds pass. Required
`make test` retains exactly the same four known baseline failure groups.
No live configuration/credentials/runtime state were read, no service was
restarted, and no commit/push/deployment was performed for this slice.

```sh
go test -race ./internal/controlservice/inventory -count=3
go test -race ./cmd/zpr-web-dashboard -run '^Test(AdminClientConfiguration|InventoryCompatibility|FetchSnapshot|ActorInventory|ActorVisas|ActorSnapshot|MergePlatform|MapSnapshot|VisaOrdered|Node|Diagnostics|Geography|GatewayInventory|GatewayAPIIndependent|DemoLDAPEditorURL|ProviderManager|TrustedSources|ProductionContract|CommandCompatibility)' -count=1 -timeout=180s
```

**Next bounded move at this checkpoint (completed below):** extract the diagnostics catalog/provider contracts and
explicit OTLP/OpenObserve acquisition into a production owner, keeping HTTP
authorization, operator environment selection and refresh-time composition in
the command. Preserve catalog filtering/caps, query bounds, source identity,
freshness and distinct diagnostics-versus-node error behavior before moving
the graph. All seven reorganization milestone gates remain open.

### Eighteenth extraction: production diagnostics acquisition owner (2026-10-15)

**Verified locally; not deployed or published.** The production
[`internal/controlservice/diagnostics`](zpr-dashboard/internal/controlservice/diagnostics/)
package owns diagnostics source/log/response and provider contracts, catalog
assembly, OpenObserve query/response parsing and explicit OTLP file acquisition.
Metric data remains the inventory metric type, preserving JSON tags and avoiding
two independently maintained shapes. The package depends only on production
inventory/Admin and neutral log redaction plus the standard library.

`OpenObserve` receives the already operator-validated endpoint, organization/
streams, credentials and HTTP client. `OTLPFiles` receives the explicitly
configured identity-to-file map. Neither discovers environment variables,
Simulator state, TUI defaults, operator credentials or configuration files.
The command still validates origins/stream identifiers, loads the private token,
creates the 12s HTTP client and selects configured providers. Compatibility
types/methods forward existing fixtures and consumers to one query/parser
implementation; there are no duplicate acquisitions.

Catalog behavior is unchanged: only node actors, nonempty services and trusted
sources; existing identity fallbacks/partial OTel mappings; initial unavailable
state and empty signal arrays; kind/name sorting without new deduplication;
100-source aggregate cap. Explicit selected-source filtering occurs before
that cap. The unused suggested service-name argument was removed from the
pure helper; existing mapping-or-identity fallback remains unchanged, rather
than silently replacing a node's service identity with `zpr-core`.

OpenObserve preserves sequential log/metric searches, the same one-hour window,
POST/query envelope, configured organization/service/instance predicates and
single-quote escaping, Basic credentials and media types. Requested log limit
is caller-selected; metric request size remains 50 or 1024 for `ZPR node`.
These are query limits, not a newly enforced local per-response metric cap.
Response bytes remain bounded at 512 KiB, with existing status/JSON/transport
errors, body closure, no new retry or redirect policy. Parsing preserves field/
timestamp precedence, string metric precision, existing numeric JSON handling,
descending timestamps, log body/redaction and eight safe-attribute bounds.
Partial signals survive either search failing, with errors joined as before.

OTLP reads the supplied file on every query (64 KiB maximum), matches exact
`service.name` and `service.instance.id`, appends gauge then sum points, retains
decimal `asInt` strings and integer nanosecond timestamp precision, and reports
the same missing-map/file/identity/timestamp/JSON errors. Its historical local
file reader does not use the context or per-query result limit; this extraction
does not change that behavior. Node enrichment/freshness remains the separate
inventory owner.

New package tests cover catalog classes/order/mapping/null-empty shapes,
selection before caps, query scope/escaping/envelope/context/path/headers,
sequential requests, exact 512 KiB and 64 KiB limits, response closure and
errors, partial logs and metrics, timestamps, attribute/truncation bounds,
OTLP resource binding/gauge+sum/precision/file replacement, and allowed
dependencies with no environment discovery. A new command regression uses
synthetic credentials/files/endpoints with Simulator unavailable and verifies
12s client/freshness bounds, absent implicit OTLP fallback, invalid implicit
configuration refusal and missing explicit configuration refusal. Existing
diagnostics/node/snapshot/gateway/production compatibility tests pass unchanged.
No existing test was edited.

The new package passes three race runs; focused owner/consumer races and 18
editor test cases pass. `go vet ./...`, full-module formatting, whitespace/editor
diagnostics and CGO-free Linux arm64/amd64 command builds pass. Required
`make test` retains the four known baseline groups: enrollment readiness,
stale-bundle platform fixture, placement shell extraction and trusted-service
ELF fixture. No live state/configuration/credentials were read; no restart,
deployment, commit or push was performed.

```sh
go test -race ./internal/controlservice/diagnostics -count=3
go test -race ./cmd/zpr-web-dashboard -run '^Test(AdminClientConfiguration|InventoryCompatibility|FetchSnapshot|ActorInventory|ActorVisas|ActorSnapshot|MergePlatform|MapSnapshot|VisaOrdered|Node|Diagnostics|Geography|GatewayInventory|GatewayAPIIndependent|DemoLDAPEditorURL|ProviderManager|TrustedSources|ProductionContract|CommandCompatibility)' -count=1 -timeout=180s
```

**Next bounded move at this checkpoint (completed below):** separate diagnostics result aggregation from HTTP:
explicit catalog/provider inputs, four concurrent source queries, stale/partial/
unavailable state, total signal budgets, field truncation and search filtering.
Keep authorization, query validation, 18s request deadline and operator
configuration/refresh timing in command composition. Preserve no-provider/
selected-source behavior and existing headers rather than silently changing
them during the move. All seven reorganization milestone gates remain open.

### Production diagnostics aggregation boundary (2026-10-15)

**Verified locally; not deployed or published. M2 remains in progress; all seven
milestone gates remain open.** Production
[`diagnostics.Aggregate`](zpr-dashboard/internal/controlservice/diagnostics/aggregate.go)
now receives an explicit catalog, provider, provider error, freshness duration,
search text and caller context. The owner takes the catalog/provider result
slices for in-place aggregation; it does not discover configuration or acquire
inventory. The command keeps method/query validation, source-map refresh and
selected-source 404 ordering, the 18s request context, configured provider
selection, authorization composition and JSON writing. A thin adapter preserves
existing command-local provider fixtures without changing their tests.

The move preserves four concurrent source queries and limit-50 provider calls,
original catalog order regardless of completion order, partial query results,
latest log/metric timestamp selection, and source unavailable/partial/stale/
available states. Stale sources alone still leave the overall state available;
any source error makes it partial. Missing-provider errors override the empty-
inventory message as before. Nil provider-returned arrays remain null; catalog
and budget-cleared arrays remain empty arrays. Future timestamps retain the
historical available state.

Budgets remain 500 logs and 1,000 metrics in catalog order, applied before search.
There is no new local per-source cap: 50 is the provider query argument.
Metric fields retain byte-based 128/128/32 truncation plus the existing ellipsis.
Case-insensitive search matches log body or source name/identity, never metrics
or attributes, and does not recompute freshness/state after signals are removed.
The command's shared JSON writer already emits `Cache-Control: no-store` for
both configured and unavailable providers; the redundant handler-only write was
removed without changing headers.

The nonpositive freshness default is now local to each aggregation call instead
of writing a variable captured by the HTTP handler. This removes a concurrent-
request data race while preserving the five-minute default. New tests exercise
20 simultaneous requests to the same zero-default handler, with configured and
missing providers and Simulator unavailable. They also verify HTTP validation
limits, source whitespace/selection semantics, context/deadline forwarding and
unchanged response/header shapes.

Owner tests measure four queries actually in flight, exact ordered 500/1,000
budgets including partial boundary slices, filtering after budgets, 127/128/129
and 31/32/33 metric-field boundaries, source and overall states, timestamp
selection, cancellation and wire null/empty distinctions. Existing owner and
command diagnostics/node/inventory/gateway/production compatibility tests pass
unchanged under three race runs. The final editor test runner reports 26 passing
cases; `go vet ./...`, full formatting, whitespace/editor diagnostics and
CGO-free Linux arm64/amd64 builds pass. Required `make test` retains the same
four baseline groups: enrollment readiness, stale-bundle platform fixture,
placement shell extraction and trusted-service ELF fixture. Concurrent
assistant test edits briefly blocked the first command/full-suite/vet attempts;
after those edits completed, the checks above were rerun. Those files were not
modified by this extraction.

```sh
go test -race ./internal/controlservice/diagnostics ./cmd/zpr-web-dashboard -run '^Test(Aggregate|Diagnostics|AdminClientConfiguration|InventoryCompatibility|FetchSnapshot|ActorInventory|ActorVisas|ActorSnapshot|MergePlatform|MapSnapshot|VisaOrdered|Node|Geography|GatewayInventory|GatewayAPIIndependent|DemoLDAPEditorURL|ProviderManager|TrustedSources|ProductionContract|CommandCompatibility)' -count=3 -timeout=180s
```

**Next bounded deliverable at this checkpoint (completed below):** resume the M1 audit of remaining production
startup/operator configuration and nested route ownership before selecting the
next cohesive M2/M3 move. Inventory and diagnostics cores no longer need a
convenient-leaf extraction; production HTTP/application composition, policy/
trusted-source ownership and integrated packaging acceptance remain open.
No new dependency, frontend change, live configuration/credential read, restart,
deployment, commit or push was performed.

### M1 production startup and HTTP composition audit (2026-10-15)

**Verified locally; no runtime code changed.** This audit covers the remaining
Control Room, Control-Service and Policy-Service startup roots and their nested
route registrations. It complements, rather than replaces, the existing
compatibility-mode, enrollment/review/gateway and inventory contracts.
[`production_composition_contract_test.go`](zpr-dashboard/cmd/zpr-web-dashboard/production_composition_contract_test.go)
adds five tests with 21 editor-runner cases: source-level exact route/delegation
and server-field guards, plus real startup failure-order and request-time
context tests with Simulator unavailable. Source guards prove the current
wiring only; they are not a complete started-service or transitive-dependency
acceptance test.

#### Startup, failure and lifetime contracts

| Root | Current sequence and outcome to preserve |
| --- | --- |
| Control Room | A 20s setup context initializes explicit operator web security, then optional delegation signer, required Control-Service proxy, embedded static root, operator auth routes and change-review store. Missing service transport fails before review configuration or listening. Review and auth cleanup are deferred. Configured TLS uses the preloaded certificate; unconfigured loopback development retains the plain HTTP branch. The server wraps `security.protect` with security headers, reads headers within 5s, idles at 60s and caps headers at 32,768 bytes. This setup context is not the lifetime context for the HTTP listener. |
| Control-Service | Require listener certificate/key/client CA, load the client CA, configure enrollment (remote relay or local registry), then Admin client, trusted feeds, assertion runtime/settings store and scheduler, policy proxy, diagnostics and assistant, optional gateway draft store, routes, and listener. Missing listener credentials/CA, invalid enrollment, feeds or assertion sources abort startup in that order. Admin/policy/diagnostics absence and gateway-store errors remain degraded feature states/logged errors, not new startup failures. The assertion settings refresh/migration has a 12s setup context; the scheduler has a separate canceled-on-return context. Enrollment cleanup is deferred. The listener uses TLS >=1.3 and verified client certificates, header timeout 5s and idle timeout 60s. |
| Policy-Service | Require listener certificate/key/client CA, load client CA, open/seed the explicitly configured production policy workspace, then build routes and listen. Missing CA fails before opening the repository. Compiler configuration may leave the repository usable with compiler-unavailable status; repository/seed failures abort. TLS >=1.3 and verified client certificates plus 5s header timeout remain unchanged. Unlike the other two roots, this server currently has no explicit idle timeout or maximum-header override. Do not silently normalize defaults during a move. |

The new runtime tests exercise exact missing-credential errors, client-CA failure
before enrollment/repository access, enrollment before feed initialization,
feeds before assertions, and assertion configuration before scheduler startup.
They use a synthetic client CA and absent temporary paths, never start a listener,
and assert that failed Policy-Service startup does not create its database.
Control Room is tested with absent production settings and deliberately invalid
Simulator settings; no Simulator configuration enables production TLS/OIDC.
Successful full service startup/shutdown and persistent-state rollout remain
acceptance gates, not claims made by these fail-fast tests.

#### Nested route ownership

| Composition owner today | Exact registrations and delegation |
| --- | --- |
| Control Room command | Five root registrations: review prefix, signed enrollment prefix, generic authorized API prefix, stylesheet GET and static root. Operator security separately mounts auth config/login/callback handling. Enrollment/review must keep precedence over `/api/`; static resources remain independently embedded. |
| Control-Service command | Fifteen direct registrations: both enrollment prefixes, gateway prefix and separate GET/POST gateway assistant, snapshot, actor visas, diagnostics, adapter logs, DNS records/stats/stylesheet, policy assistant and both exact/prefix policy proxy paths. Six assertion registrations and two trusted-feed registrations are delegated to command-local owners. The new source guard pins all 23 registrations and their handler bindings plus those delegation calls. |
| Policy-Service command | Twenty-one method-specific registrations: policy status/context, assertion settings GET/PUT, attribute rescan, policy test/fixtures, stage/check/config check/form, categories/records, duplicate/rename/archive/restore, and revision list/save/read. Assertion settings here are repository operations, not Control-Service's evaluation/settings view; do not merge the two surfaces. |
| Enrollment command adapter | Local configuration mounts the existing `enrollment.APIPrefix` and `operatordelegation.Prefix` handlers. Any remote-enrollment setting selects the relay configuration path; remote settings cannot coexist with local registry/catalog/trust. Only canonical signed named-user delegation routes cross the remote relay; mounting both top-level prefixes does not enable certificate-only remote administration. Existing relay/delegation tests cover that boundary. |

Keep Go method-specific routing, GET/HEAD behavior and path wildcards intact;
handler method checks are not interchangeable with method-specific registration.
Specific gateway assistant and policy assistant routes must continue to win over
their broad gateway/policy prefixes. Control Room named-user permission checks,
local-request checks and backend mTLS are separate boundaries. The nonmanagement
stylesheet proxy uses the local boundary; the management stylesheet path also
requires named-user authorization. Change review and enrollment keep their own
organization/domain/delegation checks, not the generic API permission gate.

#### Configuration read timing and existing owners

- Startup snapshots: listener/TLS/OIDC/signer/review/enrollment inputs and service
  transports, trusted-feed config, assertion source config, main diagnostics
  provider config and gateway store. Control Room organization is supplied when
  mounting its proxies. Preserve production origin validation and distinct Admin
  chain/name verification rather than reusing a differently strict validator.
- Request/refresh inputs remain distinct: snapshot platform/provider/display/
  node-telemetry composition, diagnostics source-map reads, DNS-transfer
  address/key selection and Policy-Service organization context/settings reads.
  The new context test changes organization settings between requests to one
  already constructed Policy-Service handler and verifies the second value.
- Adapter logs capture the configured inventory path when the handler is built
  but reread its file for each request. DNS statistics captures the configured
  origin at handler construction. Main diagnostics captures its configured
  provider at startup, while node enrichment selects its provider per refresh.
  The policy proxy rereads assistant availability when rewriting successful
  policy status. A new application owner must not turn all these into one
  immutable configuration snapshot or add implicit reloads.
- Reuse `controlroom`, `servicehttp`, `operatorauth`, `operatordelegation`,
  `enrollment`, `changereview`, `policyclient`, production inventory/diagnostics/
  gatewaydraft and existing DNS/log owners. Command-local domain handlers,
  workspace/assertion runtime and startup composition still prevent claiming
  complete M2/M3 package separation.

**Evidence:** all new composition tests pass three race runs; unchanged focused
operator/security/transport/assertion/review/enrollment/gateway and CLI contracts
pass under race. Complete `controlroom`, `servicehttp` and `policyclient` package
race suites pass. The editor runner reports 21 passing cases; vet, full-module
formatting, whitespace and changed-file diagnostics pass. Required `make test`
retains the same four known baseline groups: enrollment readiness, stale-bundle
platform fixture, placement shell extraction and trusted-service ELF fixture.
No existing test was edited. No dependency, asset, runtime configuration, live
credential/state, listener, deployment, commit or push changed in this audit.

```sh
go test -race ./cmd/zpr-web-dashboard -run '^TestProductionComposition' -count=3 -timeout=120s
go test -race ./internal/controlroom ./internal/servicehttp ./internal/policyclient -count=1 -timeout=120s
```

**Next cohesive M2 scope at this audit checkpoint (completed below):** give Control-Service HTTP composition a production
owner with explicit already-built handler inputs. Move its fifteen direct and
eight delegated registrations together, retaining specificity, local checks,
error/no-store behavior and independent production contracts. Add served-mux
tests with synthetic handlers and Simulator unavailable, not only source guards.
Keep environment/file resolution, scheduler startup/cancellation, persistence,
TLS server/listener and CLI in the command for this slice. Do not pull policy/
assertion/Simulator implementations into the new owner or disguise callbacks
that discover Simulator as neutral dependencies.

M1 remains open for complete ownership reconciliation and packaging contracts;
M2 for remaining production application/configuration/lifecycle ownership; M3
for cohesive policy/trusted-source packages. M4-M7 remain open as previously
recorded. This audit authorizes no state migration or publication.

### M2 explicit-handler Control-Service HTTP composition (2026-10-15)

**Verified locally; not deployed or published. All seven milestone gates remain
open.** The standard-library-only
[`internal/controlservice/httpapi`](zpr-dashboard/internal/controlservice/httpapi/)
package owns all 23 Control-Service route registrations: fifteen previously
direct, six assertion and two trusted-feed routes. `NewMux` takes explicit
already-built `http.Handler` fields, including grouped `Assertions` and
`TrustedChanges`. No environment, files, service discovery, listener, scheduler
or Simulator implementation belongs to the route owner. Missing handlers retain
the standard mux's explicit construction panic, not an omitted route or
success-shaped fallback; callers supply real unavailable-feature handlers.

The command retains configuration read timing, enrollment/store setup and
cleanup, assertion settings refresh/migration and scheduler cancellation,
provider selection, gateway inventory/store construction, application/domain
handlers, security headers, server/TLS options and listener lifetime. Existing
assertion inline handlers became named methods with unchanged bodies; their
local-request checks, error/status shapes, locking, context/deadlines and
organization/source guards remain. Assertion and trusted-feed `register`
compatibility methods forward to the package registrations, so existing domain
tests use the same route table rather than a duplicate implementation.

Method-specific patterns, GET/HEAD behavior, path wildcards, both enrollment
prefixes, exact/prefix policy routes and assistant precedence are preserved.
Other gateway methods still fall through to the broad gateway handler; non-POST
policy assistant requests still fall through to the policy proxy. Do not replace
these contracts with generic 405 responses during later domain extractions.
The mux forwards request context, query, headers and bodies and preserves
handler responses/errors; it does not invent authentication or no-store headers.
Those stay in the existing handlers/security wrappers, with runtime integration
tests confirming both allowed and denied local requests.

New production mux tests exercise every registration with Simulator unavailable,
specific/broad route precedence, actor/source path values (including an escaped
slash in a single segment), GET/HEAD, 404/405 and `Allow`, canonical path
redirects, header/body/context forwarding, error preservation and required
handler construction. The current Go runtime emits 307 canonical redirects;
the initial new-test expectation of 301 was corrected after observing the
unchanged standard mux behavior, without changing runtime routing.
An import/dependency guard permits only `net/http` in runtime source and rejects
listener startup. New command tests pin every handler binding and exercise real
snapshot, diagnostics, actor-visa, assertion, trusted-feed and policy handlers
through the production mux with synthetic unavailable services.

The prior, passing source-registration audit test was deliberately updated to
follow registrations into the new owner and support receiver-qualified methods.
It still checks all 23 routes and server boundaries; new command-binding tests
retain the lost command-to-route linkage. No pre-existing failing test was
modified and no baseline failure was suppressed.

**Evidence:** the new package passes three race runs; focused unchanged
composition/auth/enrollment/review/assertion/feed/gateway/diagnostics/inventory
and CLI regressions pass under race. The editor runner reports 83 passing cases
across the new mux/integration and updated composition tests. Vet, full-module
formatting, whitespace/editor diagnostics and CGO-free Linux arm64/amd64 command
builds pass. Required `make test` retains the four known baseline groups:
enrollment readiness, stale-bundle platform fixture, placement shell extraction
and trusted-service ELF fixture. No dependency, frontend, live state/credential,
runtime configuration, restart, deployment, commit or push changed in this slice.

```sh
go test -race ./internal/controlservice/httpapi -count=3 -timeout=120s
go test -race ./cmd/zpr-web-dashboard -run '^Test(ControlHTTP|ProductionComposition|ProductionContract|CommandCompatibility|ControlRoom|ControlService|PolicyService|Operator|Management|LocalRequest|LocalEditor|ChangeReview|EnrollmentService|EnrollmentServiceRelay|Assertion|LegacyAssertions|TrustedChanges|TrustedSourceChanges|GatewayAPIIndependent|PolicyAndAssertionAssistantContext|Diagnostics|InventoryCompatibility|FetchSnapshot|ActorInventory|ActorVisas|ActorSnapshot|Node)' -count=1 -timeout=180s
```

**Next cohesive M2 scope at this checkpoint (completed below):** separate the operator web-security loader and
request boundary from command environment discovery. Reuse `operatorauth`,
`servicehttp` and `systemconfig` with explicit web options and preserve Control
Room management/direct-TLS/development behavior, OIDC factory/cleanup, approved
peer/host/origin rules and configuration timing. Simulator keeps its own
environment adapter and may call the shared mechanism; the production owner
must not discover Simulator options. Do not move live state or normalize the
distinct server/transport defaults as part of that move. Policy/trusted-source
ownership and integrated packaging acceptance remain separate open gates.

### M2 explicit-input operator web-security owner (2026-10-15)

**Verified locally; not deployed or published. All seven milestone gates remain
open.** [`internal/operatorweb`](zpr-dashboard/internal/operatorweb/) owns
explicit TLS/OIDC loading, the reusable auth factory, request protection,
operator auth-route registration and authorization-client cleanup. It reuses
`operatorauth`, protected-file reads from `servicehttp`, and management endpoint/
certificate/access validation from `systemconfig`. No dependency was added.
Its dependency guard permits only the existing production/neutral graph and
rejects raw environment/filesystem/process imports in runtime code.

The command retains separate Control Room and Simulator environment adapters,
setup contexts, server/listener selection, security-header wrapping, permission/
delegation/review composition and cleanup timing. The two production loader
entry points now share one Control Room environment projection; Simulator
supplies only its own independently named settings. Existing private config/
security fields and helper methods remain compatibility projections rather
than duplicate implementations, so prior tests and consumers are unchanged.
There is no production read of Simulator settings or dependency on its
availability. Simulator may call the neutral mechanism with its own inputs.

`Load` receives a listen address, explicit config and auth factory. It preserves
the no-config development passthrough, exact HTTPS-origin validation, fixed-port
loopback or explicitly trusted-peer listeners, protected 64 KiB certificate/key/
OIDC files, certificate hostname/time/server-usage checks and TLS >=1.3.
Management remains Control Room only: explicit management interface, production
DNS origin, current server certificate, exact organization/grants and initialized
named-user OIDC are required; it cannot combine with a trusted proxy peer.
Raw management-interface/organization semantics and existing error text were
not normalized as part of this extraction.

OIDC retains strict single-document JSON, callback binding, protected 4 KiB
single-line secret handling (including terminal LF/CRLF removal), explicit
optional IdP CA and caller-context forwarding. Factory errors abort loading.
The optional CA path is trimmed before capturing the reusable factory, rather
than writing the captured path during each invocation. This removes a shared
write under concurrent factory calls without changing when the CA file is read:
the supplied file is reread per invocation and missing/invalid roots fail.
Auth-client cleanup still delegates to `operatorauth.Auth.Close`.

Request protection preserves direct TLS/exact host/approved peer checks and
management foreign-Origin/forwarding-header refusal. The historical direct
loopback boundary is not silently given the stricter management rules.
Management still requires an auth instance at request time. Authenticated GET
login documents use `same-origin` referrers; callbacks/non-document requests
retain the caller's policy. Auth availability exposes only `enabled`; absent
auth mounts the same explicit 503 handler, not an enabled-looking fallback.
The caller supplies existing JSON/error writers, preserving headers and shapes.
Named-user permission checks remain distinct from this connection boundary.

New owner tests exercise explicit inputs despite invalid ambient config,
65,536/65,537-byte OIDC and 4,096/4,097-byte secret boundaries, strict JSON,
secret format/factory errors/context, development/direct-TLS loading, origin
validation, request/referrer/auth-route behavior and concurrent synthetic HTTPS
IdP discovery with explicit CA rereads and cleanup. New command tests verify
that invalid Simulator settings do not affect Control Room and vice versa,
plus real TLS/peer projection through both adapters. Existing management,
OIDC, direct HTTPS, Simulator HTTPS, real login/delegation, authorization and
composition tests pass unchanged. No existing test was edited.

**Evidence:** complete owner, `operatorauth`, `servicehttp` and `systemconfig`
suites pass three race runs; unchanged focused command consumers pass under
race. The editor runner reports 38 passing cases. Vet, full formatting,
whitespace/editor diagnostics and CGO-free Linux arm64/amd64 command builds
pass. An initial formatting assertion overlapped concurrent edits; it was rerun
clean without changing unrelated files. Required `make test` retains the same
four known baseline groups: enrollment readiness, stale-bundle platform fixture,
placement shell extraction and trusted-service ELF fixture. No live credential/
state/configuration was read or changed; no frontend, restart, deployment,
commit or push occurred.

```sh
go test -race ./internal/operatorweb ./internal/operatorauth ./internal/servicehttp ./internal/systemconfig -count=3 -timeout=180s
go test -race ./cmd/zpr-web-dashboard -run '^Test(OperatorWeb|ControlRoom|ControlService|PolicyService|SimulatorHTTPS|Management|Operator|ProductionComposition|ProductionContract|ControlHTTP|ChangeReview|EnrollmentService|LocalRequest|LocalEditor)' -count=1 -timeout=180s
```

**Next cohesive M2 scope:** give Control Room root HTTP composition an
explicit-handler owner alongside its existing production proxy/authorization
package. Preserve auth-config/login route precedence, review and signed
enrollment overrides over the generic API proxy, static/stylesheet routing and
management-versus-development authorization wrappers. Keep security/config
loading, signing/review stores, embedded resources and server lifetime in the
command. Exercise the complete composed mux with Simulator unavailable rather
than claiming package separation from route-source checks alone. Policy/
trusted-source ownership, lifecycle/configuration assembly and integrated
acceptance remain open gates.

### M2 explicit-handler Control Room root HTTP composition (2026-10-15)

**Verified locally; not deployed or published. All seven milestone gates remain
open.** [`internal/controlroom/httpapi`](zpr-dashboard/internal/controlroom/httpapi/)
owns all seven root registrations through explicit required handlers. Runtime
code imports only `net/http`; there is no configuration, discovery, service
startup or Simulator dependency. Missing handlers fail mux construction rather
than silently dropping routes.

Auth configuration retains GET/HEAD precedence over the login prefix; other
methods still reach the login handler. Review and signed enrollment retain
their more-specific overrides over the generic `/api/` proxy. The separate
`/api/operator-enrollment/` path still reaches the generic API wrapper, not the
signed browser enrollment handler. Stylesheet GET/HEAD retains precedence over
static fallback; other methods still reach the file server. Standard mux
canonical redirects and handler-owned errors/headers remain unchanged.

`operatorweb.Security` now exposes its existing configuration/login handlers
individually; its compatibility `Register` method delegates to those same
implementations. There is no duplicate auth implementation or new auth policy.
The command still prepares these handlers before review setup, selects
management-versus-local API/stylesheet wrappers, and owns TLS/OIDC configuration,
signer, review stores/cleanup, static filesystem and revalidation, security
headers, server fields and listener lifetime. Environment read timing and
startup error ordering are preserved.

Served-mux tests exercise every root registration, methods/HEAD, prefix
precedence/fallthrough, request context/body/query/header forwarding, canonical
redirects, required handlers, exact errors and dependency boundaries. Command
integration serves real auth configuration, unconfigured review, signed
enrollment refusal, API/stylesheet boundaries and embedded static resources
with Simulator unavailable in both development and management modes.
Management still requires named authorization; development API remains local,
and its stylesheet deliberately retains the historical local-only wrapper.
Full composition checks retain CSP/referrer/cache semantics and deny foreign
origins/forwarding/remote requests without reaching the backend.

The previously passing route-source guard was deliberately evolved to follow
the moved owner, and command handler-binding coverage now pins the new literal.
No pre-existing failing test was edited. A new static-404 assertion initially
expected `no-cache`; Go's file server removes it on error. The new test was
corrected to pin that existing behavior rather than change runtime caching.

**Evidence:** 91 editor-runner cases pass. Complete Control Room, root router,
operator-web/auth, service-HTTP and system-config suites pass three race runs;
focused unchanged command consumers pass under race. Vet, full formatting,
whitespace/editor diagnostics and CGO-free Linux arm64/amd64 command builds
pass. Required `make test` retains exactly the four known baseline groups:
enrollment readiness, stale-bundle platform fixture, placement shell extraction
and trusted-service ELF fixture. No live credentials/state/configuration,
frontend, restart, deployment, commit or push was part of this slice.

```sh
go test -race ./internal/controlroom/... ./internal/operatorweb ./internal/operatorauth ./internal/servicehttp ./internal/systemconfig -count=3 -timeout=180s
go test -race ./cmd/zpr-web-dashboard -run '^Test(ControlRoom|ControlService|PolicyService|Operator|Management|SimulatorHTTPS|ProductionComposition|ProductionContract|ControlHTTP|ChangeReview|LocalRequest|LocalEditor)' -count=1 -timeout=180s
```

**Next cohesive M2 scope:** explicit-handler Policy-Service root HTTP
composition, preserving per-request organization context and all domain method/
path bindings. Keep repository/configuration/seed/compiler initialization,
security/TLS and service lifetime command-owned. Broader application ownership,
configuration/lifecycle assembly, packaging and integrated acceptance remain
open; the completed root routers do not close those gates.

### Verified cross-domain call findings

1. **Production assistant configuration read Simulator configuration (repaired locally).**
   `runControlService` and `newPolicyServiceProxy` call `newClaudeAssistant`,
   which calls `assistantAPIKey`. Without an explicit key/file,
   `assistantAPIKey` uses `SIMULATION_MANIFEST` to locate a sibling key file.
   The explicit-environment tests and neutral transport dependency tests do
   not reject this fallback. Extracting the transport did not repair it.
   The subsequent M2 slice below removes this fallback from production and
   adds a real protected manifest-sibling key regression.
2. **Production repository initialization loads Simulator organizations.**
   `runPolicyService -> newPolicyWorkspace` calls
   `loadSimulatorOrganization(simulatorOrganizationsDirectory(), ...)` to seed
   assertions. That behavior belongs in explicit provisioning or a separately
   configured production seed contract, not a production-default dependency.
3. **Policy fixtures are Simulator-derived.**
   `policyServiceMux -> handlePolicyTestFixtures -> policyTestFixtures` reaches
   `manifestForOrganization` and Simulator profile/machine/directory data.
   Production fixture generation needs independent operator/provider contracts;
   moving `policy_test_fixtures.go` unchanged would preserve the wrong boundary.
   The subsequent M3 input slice below repairs this observed path. The original
   call graph is retained as audit evidence, not the current implementation.
4. **Main-package shared types hide dependency direction.**
   Snapshot types are consumed by diagnostics, counters, gateways, Simulator
   views and tests; diagnostics providers also enrich snapshots. Move ownership
   and introduce explicit provider/configuration inputs before splitting these
   into mutually importing packages. The command's `application` also combines
   production Admin, local policy repository, remote proxy and assistant roles.
5. **Some apparent sharing is already neutral.**
   Adapter logs/diagnostics now use `logview`; transport/proxy packages have
   narrow dependency checks. Preserve that progress, but these package checks
   cannot prove isolation of the still-shared `main` application.

These findings are observed call paths, not an exhaustive repository audit or
a claim that the running services currently selected a Simulator fallback.
No runtime keys, configuration contents or protected state were read for this audit.

### Baseline and next implementation slice

Passing current-worktree production contract baseline:

```sh
go test -race ./internal/assistant ./internal/controlservice/adapterlogs ./internal/logview ./internal/controlroom ./internal/policyclient ./internal/servicehttp ./cmd/zpr-web-dashboard -run 'Snapshot|ActorInventory|ActorVisas|NodeCounter|NodeDenial|NodeDisplay|ControlRoom|ControlService|Diagnostics|OperatorLogComponents|LogComponents|AdapterLog|FileLogSources|AssistantAPIKey|AssistantTransport|PolicyAndAssertion|DependencyBoundary' -count=1 -timeout=120s
```

This is selected coverage, not a full-suite pass. The previously recorded four
Go failures remain unmodified: enrollment readiness preflight, stale-bundle
platform fixture, scenario placement shell fixture and trusted-service ELF
fixture. Full Rust/browser suites and packaging/rollout were not run for this
documentation-only audit.

**M2 slice completed locally: production assistant configuration isolation.**

- Moved production key resolution and shared client construction out of mixed
  `policy.go` into `assistant_config.go`. Control-Service startup and policy
  status retain `newClaudeAssistant`, now production-only; neither discovers
  credentials through `SIMULATION_MANIFEST`. Preserve environment-key, explicit
  file and operator gateway-store sibling precedence. Explicitly configure
  `ZPR_ANTHROPIC_API_KEY_FILE` instead of the removed production manifest fallback.
- Moved the protected single-link regular-file reader into neutral
  `internal/assistant.ReadAPIKeyFile`, with explicit path input, unchanged
  permission/size/text checks and no environment discovery. Invalid selected
  files now emit a sanitized error-type log and leave the assistant unavailable.
- Added `newSimulatorClaudeAssistant` in the Simulator design owner and wired
  status plus all five design/editor/profile/definition assistant routes to it.
  It preserves explicit settings, manifest-sibling discovery, gateway-store
  fallback when no manifest is selected, and per-request reload.
- Tests use a real protected manifest-sibling key to distinguish ignored
  production discovery from preserved Simulator discovery, verify explicit
  override precedence and missing-file refusal, and exercise policy-status
  readiness through the proxy with dynamic explicit-file changes. Neutral
  reader tests cover valid text/CRLF, exact size limits, invalid text, missing
  files, directories, symlinks, hard links and permission failures.
- Repaired the assistant launch test's extracted shell-function harness to
  include `control_platform_services`, introduced by the immediately preceding
  gateway fix. This was a failure caused by that local change, not one of the
  unrelated baseline failures.
- Focused production/assistant/Simulator race tests, neutral dependency checks,
  `go vet`, native and Linux arm64/amd64 builds, formatting and editor
  diagnostics pass. `make test` was run: all other packages pass, and the web
  command still has exactly the four recorded baseline failures (enrollment
  readiness, stale-bundle platform fixture, placement shell fixture and
  trusted-service ELF fixture). Those tests were not modified.
- No runtime credential/state reads, service restarts, deployments, commit or
  push for this slice. This is not full package isolation or full-suite acceptance.

### M3 input slice: independent production policy fixtures (2026-10-15)

**Verified locally; not deployed, committed or pushed. Seven milestones remain
open.** This is an input-contract repair, not another package extraction.

- The user selected an explicit operator JSON contract with fresh live LDAP,
  rather than a proposal-only change or a snapshot/empty-fixture fallback.
  `ZPR_POLICY_TEST_FIXTURES_FILE` supplies version-1 organization/base-DN context,
  user identity authority, approved directory mappings, explicit device inventory,
  ownership/scope and validated service inputs. Unknown fields, invalid files,
  context mismatches and unsafe configuration fail explicitly. The bounded
  regular file is re-read per request; absent configuration returns HTTP 503,
  without affecting repository use or caller-supplied `POST` fixtures.
- `policy_test_fixtures.go` no longer calls `manifestForOrganization` or uses
  Simulator organization/machine types. Configured LDAP, policy mappings,
  source group roles/tags, sorted users/devices/owner combinations, exact actor
  limits, and unsafe-value omission diagnostics remain. Identity-mapping source
  attributes are included in approved LDAP reads. Production device classification
  uses explicit inventory/scope/ownership or directory object classes, not
  the fictional `machine-` prefix. No default demo authorities, machine models,
  locations, secure flags or manifest owners are fabricated.
- Simulator-owned `scripts/export-policy-fixtures.jq` translates fictional
  profile/manifest metadata, preserving explicit services, fallback ownership,
  service omission warnings and Load Lab's existing first-port behavior.
  `start_policy_context_container` atomically exports a mode-0600 file before
  stopping the previous service and passes only the independent file setting.
  Export failure retains the prior fixture file and running service.
- New regressions exercise the real Policy-Service fixtures handler and LDAP
  reader with Simulator manifest/profile paths nonexistent and its endpoint
  unavailable. Only the LDAP container command is mocked. Checks cover fresh
  directory/config changes, mapped roles, explicit authorities, owner unions,
  service inputs, omission warnings, missing configuration and LDAP failure.
  Additional cases cover strict configuration/context validation, exactly 4 MiB
  versus over-limit files, actor limits, directory-only devices, all bundled
  profile exports, fallback/omission behavior and failed-export preservation.
  An AST guard rejects Simulator symbols/configuration and fictional authority
  defaults in the production fixture reader/configuration files.
- Focused fixture/evaluator/assertion/production-transport race checks pass,
  as do `go vet ./...`, native builds, Linux arm64 command build, changed-file
  formatting, launcher shell syntax and editor diagnostics. Required `make test`
  retains exactly the four documented baseline groups: enrollment readiness,
  stale-bundle platform fixture, placement shell extraction and trusted-service
  ELF fixture. No existing failing tests were modified. The editor test tool
  discovered zero tests; terminal Go test results provide the validation.
- No live credentials/state were read, no service was restarted, and no new
  policy/scenario was activated. A coordinated Policy-Service binary rollout and
  context regeneration remain separate work. Source verification does not
  certify the currently running fixture endpoint.

**Next bounded scope at this fixture checkpoint:** the later nested contract
audit and gateway-owner extraction cover enrollment/gateway/review mapping and
the first cohesive gateway move. Finish remaining cross-domain and packaging
acceptance, then compose the cohesive production policy
owner from these independent inputs. Preserve repository revision/storage
ordering, existing authorization and source-local diagnostics. The input repair
does not authorize a wholesale file move or close M2/M3.

Remaining M1 gates: cross-domain call coverage beyond the paths traced above
and installer/CI/resource
dependency acceptance. The later compatibility audit records all command-mode
inputs and Simulator path/default contracts; it is not runtime certification.
The ownership map covers the original 58-file set, production assistant
configuration and the new fixture contract file; deeper audit gates remain open.

## Progress reconciliation (2026-10-14)

- **Production boundary repair deployed:** Control-Service DNS statistics now
  use explicit operator configuration and an independent relay, rather than
  Simulator's internal relay. Live mutual-TLS status, counter and zone checks
  passed with Simulator paused. The local relay uses Docker administrative
  access; production still needs an authorized ZPR-connected origin.
  This repairs a runtime dependency, not the planned Go package separation
  or a repository-wide dependency-boundary check.
- **Organization draft boundaries implemented:** private definitions save
  complete profile/document snapshots atomically with expected-revision
  protection. Private profile History opens the matching whole bundle.
  Duplicate/New/Import produce private drafts, not runnable organizations;
  saving does not activate, provision or rewrite bundled source files.
  The profile-versus-directory people/groups authority decision and full
  reference/compiler validation remain open.
- **Organization profile UI deployed to Simulator:** structured sections and
  Raw JSON edit the same draft through exclusive Raw/Form modes. File...,
  Browse..., Analyze, Format and shared Wrap/search controls follow the editor
  pattern; History sits beside identity. The 2026-10-14 release passed 48
  selected desktop/tablet organization regressions and verified 74 served
  assets. One existing definitions-history case has a stale Close selector
  and is excluded in both viewports; no full-suite pass is claimed.
- **Shared editor fixes have distinct rollout scopes:** gutter synchronization
  and programmatic/catalog insertion Undo/Redo are locally verified with 90
  focused desktop/tablet cases. The user-approved full-worktree Simulator
  release includes the shared assets. The subsequent 2026-10-14 15:40 UTC
  Control Room-only current-worktree release now includes those shared assets,
  verified against source with 62 selected desktop/tablet fixture regressions.
- These repairs and UI changes do not complete source inventory, thin-command
  extraction, feature-module reorganization, Rust subdivision or naming work.
  The editor and DNS source/documentation changes were subsequently committed
  and pushed as `51f612b`; that publication did not deploy additional targets.
  Use the deployment ledger for runtime scope.

## Thirteenth backend extraction: neutral log formatting (2026-10-14)

**Implemented and locally verified; uncommitted, not deployed.** The eleventh
and twelfth increments remain local too. No runtime configuration or process
was changed.

- Moved shared log-text formatting and the existing credential regex from the
  Simulator machine-log file into
  [`internal/logview`](zpr-dashboard/internal/logview/lines.go). The neutral
  package uses only the standard library, enforced by a transitive dependency
  test, and accepts only caller-supplied text.
- Production adapter logs call `logview.Lines` directly; production diagnostics
  call `logview.Redact` directly. Neither relies on the Simulator machine-log
  helper or regex anymore. Simulator retains a thin `machineLogLines`
  compatibility adapter so existing consumers and tests remain unchanged.
- Preserved byte-tail selection at 65,536 bytes, first-partial-line removal,
  outer whitespace trimming, last-100-line selection, non-nil empty lists and
  the exact redaction regex/replacement. These are existing heuristics, not a
  claim of exhaustive secret detection or a new post-redaction byte cap.
  Diagnostics retain their separate 2,048-byte truncation and attributes policy;
  raw redaction does not trim, split or truncate the supplied text.
- Tests measure exact byte/line boundaries, missing-newline behavior, partial
  lines, blank/CRLF handling, empty JSON arrays, credential formats, unchanged
  ordinary text and unbounded redaction layout. Invalid Simulator/operator
  environment settings do not affect the helper. Existing production log,
  Simulator log and diagnostics regressions pass without modifications.

Passing validation:

```sh
go test -race ./internal/logview ./cmd/zpr-web-dashboard -run 'LogLines|LogRedaction|MachineLog|AdapterLog|FileLogSources|OperatorLogComponents|LogComponents|Diagnostics' -count=1 -timeout=90s
go vet ./internal/logview ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/zpr-web-dashboard
```

Formatting, changed-file editor diagnostics and diff checks pass. This adds no
dependency, route, frontend, resource path, script or mode change. Full Go/browser
suites were not rerun, and unrelated failing tests were not modified.

## Twelfth backend extraction: production adapter-log inventory (2026-10-14)

**Implemented and locally verified; uncommitted, not deployed.** The preceding
assistant transport extraction is still local too. No process, runtime inventory
or deployment configuration was changed by either increment.

- Moved the operator-owned adapter log inventory schema and file parser into
  [`internal/controlservice/adapterlogs`](zpr-dashboard/internal/controlservice/adapterlogs/inventory.go).
  `ReadInventory` accepts only the explicitly supplied file path, reads it afresh
  on every invocation and does not inspect environment or Simulator state.
  A standard-library-only transitive dependency test enforces package isolation.
- The command uses type aliases and a forwarding adapter, preserving every
  existing inventory consumer and JSON field. Environment selection, authenticated
  route registration, HTTP method/status/error handling, file/container log
  collection, runtime-state inspection, timeouts and log redaction remain in
  their current owners. This slice does not extract the complete log service.
- Preserved exact target/source limits (100/8), unique nonempty IDs, required
  names/kinds, absolute-path and mutually exclusive file/container validation,
  safe container/runtime identifiers and error strings/order. Tests measure the
  limits and 128-character container-name boundary, configuration reload and
  independence with Simulator unavailable.
- Decoder compatibility is explicit: unknown fields are rejected at every
  level; null/missing versus empty lists retain their JSON shape; the existing
  65,537-byte limited reader is unchanged. This is still a single JSON decode,
  not a strict file-size or trailing-data check. Tests preserve acceptance of
  trailing data and existing duplicate-field behavior rather than changing
  those semantics during extraction.
- Existing production HTTP, retained-history, runtime-state, redaction and
  Simulator-independence tests remain unchanged. The editor test runner reports
  36 passing cases across the new package and selected runtime-state test files.

Passing validation:

```sh
go test -race ./internal/controlservice/adapterlogs ./cmd/zpr-web-dashboard -run 'AdapterLog|AdapterFile|FileLogSources|ControlService.*Log|OperatorLogComponents|LogComponents' -count=1 -timeout=90s
go vet ./internal/controlservice/adapterlogs ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/zpr-web-dashboard
```

Formatting, editor diagnostics and diff checks pass. No new dependency, frontend,
resource path, mode, script or runtime change is included. Full Go/browser suites
were not rerun; recorded unrelated failing tests were not modified.

## Eleventh backend extraction: neutral assistant HTTP transport (2026-10-14)

**Implemented and locally verified; uncommitted, not deployed.** This increment
does not restart services or change runtime configuration. The first ten
extractions are published in `e1b24ab`; consult the deployment ledger for each
process's release scope rather than the historical labels below.

- Moved Claude HTTP completion and its message/reply JSON types into
  [`internal/assistant`](zpr-dashboard/internal/assistant/client.go). The client
  takes explicit endpoint, key, HTTP client, context, system text, messages,
  model and token limit. It has no environment/filesystem/Simulator access,
  enforced by a standard-library-only transitive dependency test.
- The existing command adapter and type aliases keep policy, editor,
  organization, gateway and Simulator design callers wired to the same transport
  without schema conversion. Key discovery, model/default configuration,
  45-second timeout, redirect policy, task skills, permissions and all
  production-versus-Simulator context selection remain command-owned.
  Legacy command-layer key discovery is deliberately not moved into the
  neutral package; this is not complete production/Simulator separation.
- Preserved request method, endpoint/query, JSON fields, nil-message `[]` shape,
  headers, text-only concatenation including empty text blocks, usage counts,
  body closure, context/transport/read/status/JSON errors and their ordering.
  The existing 64 KiB limited reader still truncates rather than checking for
  an extra byte; the exact-limit and beyond-limit compatibility cases are tested.
- New tests use only local HTTP servers or injected transports. They cover
  explicit configuration with invalid Simulator settings, malformed/type/trailing
  JSON, empty/nontext content, HTTP errors, transport/read failures, body closure,
  cancellation, client timeout, and redirect refusal for 301/302/303/307/308.
  Existing caller security, task-skill, proposal and context tests are unchanged.

Passing validation:

```sh
go test -race ./internal/assistant ./cmd/zpr-web-dashboard -run 'Assistant|Claude|DesignReply|DesignProposal|Complete' -count=1 -timeout=90s
go vet ./internal/assistant ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/zpr-web-dashboard
```

Changed Go files pass formatting/editor diagnostics. No dependency, frontend,
resource path, script, CLI mode, runtime state or deployment change is included.
The full Go/browser suites were not rerun; no full-suite pass is claimed and
the four recorded unrelated Go failures were not modified.

## Tenth backend extraction: Policy-Service assistant attributes (2026-10-14)

**Implemented and locally verified; uncommitted, not deployed.** The live
Control Room/Control-Service release still includes the first eight extractions.
The ninth forwarding increment and this tenth increment remain local source.

- Moved the remote assistant attribute request/decoding into the existing
  [`internal/policyclient`](zpr-dashboard/internal/policyclient/attributes.go).
  `FetchAttributes` takes a context, validated repository origin and configured
  transport. The command retains local-workspace selection and locking, absent
  service/configuration errors, environment/certificate setup and transport cleanup.
- Moved the two-field JSON attribute contract into that production package;
  the existing command name is a type alias, preserving all local consumers
  without conversions or a duplicate schema. The client returns attributes only,
  not people, records or unrelated repository fields.
- Preserved GET `/api/policy`, Accept JSON, a fresh credential-free request,
  12-second client timeout, refusal to follow redirects, context propagation,
  the 4 MiB body limit with a one-byte overrun check, configured-workspace
  requirement and exact status/size/decoding/workspace errors. Read/transport
  failures remain errors rather than empty successful results. Nil versus
  empty attribute arrays and unknown-field handling are unchanged.
- Added explicit-origin tests with invalid Simulator/environment configuration,
  exact-limit acceptance and one-byte-over rejection, malformed/type/trailing
  JSON, HTTP errors, missing/unconfigured status, empty/null/missing attributes,
  redirect refusal for 301/302/303/307/308, canceled contexts, read deadlines and
  truncated responses. Command tests preserve local copy isolation, JSON shape,
  local-workspace precedence and missing/invalid service errors. Existing real
  mTLS assistant-context tests still ensure unrelated people/record data is
  excluded and failed context prevents assistant invocation.
- The existing standard-library-only package dependency test covers the new
  client too. No dependencies, route/permission/CSRF, assistant prompts, repository
  state, frontend resources, scripts or runtime configuration were changed.

Passing validation:

```sh
go test -race ./internal/policyclient ./cmd/zpr-web-dashboard -run 'PolicyAttributes|PolicyAssistantAttributes|PolicyProxy|PolicyService|ControlServiceProxiesPolicy|ControlServiceAssistant|PolicyAndAssertionAssistantContext|PolicyAttribute' -count=1
go vet ./internal/policyclient ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

The editor runner reports 28 selected passing tests. `make test` retains exactly
the same four baseline failures and causes recorded below; all remaining Go
tests pass with those four explicitly excluded. Formatting and editor diagnostics
pass. No image build, deployment, container restart, commit or push was performed.
Full production composition and server/repository extraction remain open.

## Ninth backend extraction: Policy-Service forwarding (2026-10-14)

**Implemented and locally verified; uncommitted, not deployed.** The live
Control Room/Control-Service release still includes the first eight extractions.
This increment does not move Policy-Service's repository or server implementation.

- Moved the Policy-Service reverse-proxy constructor and director into
  [`internal/policyclient`](zpr-dashboard/internal/policyclient/). It takes an
  already validated origin and configured mTLS transport. A transitive
  standard-library-only dependency test prevents application/Simulator coupling.
- Preserved upstream URL scheme/host and Host rewriting, escaped paths and raw
  queries, methods/bodies, removal of Origin/Cookie/Authorization, original-host
  fallback only when `X-Forwarded-Host` is empty, existing forwarded-for behavior
  and 100ms streaming flush interval. This package intentionally does not reuse
  Control Room's different credential/delegation/forwarding-header policy.
- The command retains environment/origin/certificate validation, local-request
  authorization, assistant-context fetching, assistant-status enrichment,
  application JSON errors and error logging. Proxy hooks remain caller-owned.
  No API route, permission, timeout, response shape, TLS or repository behavior
  changed; no dependencies, frontend assets or scripts were added or changed.
- Package tests cover forwarding with invalid Simulator/environment settings,
  escaped paths/query/body, headers, upstream Host, incoming request immutability,
  transport/flush selection, upstream status/body/redirect preservation, request
  context, caller error/enrichment hooks and dependency boundaries. New command
  tests cover enrichment matching by method/path/status, content length, secret
  exclusion and exact JSON 502 errors for transport/malformed-response failures.
  Existing real-mTLS and remote assistant-context regressions remain intact.

Passing validation:

```sh
go test -race ./internal/policyclient ./cmd/zpr-web-dashboard -run 'PolicyProxy|PolicyService|ControlServiceProxiesPolicy|ControlServiceAssistant|PolicyAndAssertionAssistantContext' -count=1
go vet ./internal/policyclient ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

The editor runner reports 28 selected passing tests. `make test` retains exactly
the same four baseline failures and causes recorded below; all remaining tests
pass with those four explicitly excluded. Formatting and editor diagnostics pass.
An initial new test expected the wrong JSON Content-Type; its expectation was
corrected to the existing `application/json; charset=utf-8` contract, without
changing production behavior or pre-existing tests.
No image build, deployment, container restart, commit or push was performed.
Complete production composition and policy repository extraction remain open.

## Eighth backend extraction: DNS record viewer (2026-10-14)

**Implemented, verified and deployed to Control Room/Control-Service; uncommitted.**
The first seven extractions were committed and pushed as `ed27bca`.
The 2026-10-14 16:26 UTC current-worktree release includes all eight; other
services retain their previous deployment scopes.

- Moved the signed AXFR record viewer, BIND TSIG key parser, record formatting,
  deduplication and sorting into
  [`internal/dnsrecords`](zpr-dashboard/internal/dnsrecords/). Its constructor
  accepts an explicit transfer address and key-file path. The command retains
  per-request `ZPR_DNS_TRANSFER_ADDR` and `ZPR_DNS_TRANSFER_TSIG_KEY_FILE` lookup
  and the existing authenticated `GET /api/dns/records` route registration.
- Preserved the fixed `svc.zpr.` zone, HMAC-SHA256 TSIG with 300s fudge, five-second
  dial/read/write timeouts, key-file reload on every request, duplicate removal,
  name/type/value sorting, JSON shape, success cache headers and exact plain-text
  503/502 errors. Failed or incomplete transfers still return an error rather
  than a partial record list. No transfer permissions or key configuration changed.
- Existing parser and record-conversion tests moved with their implementation;
  the command's missing-configuration test remains. Added real local TCP AXFR
  tests verifying request TSIG and signed multi-envelope responses, exact sorted
  JSON and duplicate SOA/record removal. Refused, incomplete and expired-signature
  responses, configuration errors, key reload, explicit configuration with
  invalid Simulator/environment settings, input immutability and per-request
  command configuration are covered.
- A transitive dependency allowlist permits only the standard library, this
  package, the existing `github.com/miekg/dns` dependency and its platform-specific
  `golang.org/x/sys` dependency. No dependencies were added and no Simulator
  package, configuration or runtime is consulted.

Passing validation:

```sh
go test -race ./internal/dnsrecords ./cmd/zpr-web-dashboard -run 'DNSRecord|ParseBINDTSIGKey|DNSStats|ControlServiceDNS|OperatorDNS' -count=1
go vet ./internal/dnsrecords ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

The editor runner reports 12 selected passing tests. `make test` retains exactly
the same four baseline failures and causes recorded below; all remaining Go tests
pass with those four explicitly excluded. Formatting and editor diagnostics pass.
No frontend, relay, BIND, credentials, live configuration or container changes were
made during implementation. The subsequent user-requested deployment replaced
only Control Room and Control-Service, preserving Config/HostConfig and network
membership, addresses and aliases. Live DNS records/statistics, real OIDC and
authenticated reads/denials pass; all 19 other running containers are unchanged.
See the [release ledger](GUI%20deployments.md) for image, scope and rollback
evidence. No commit or push was performed for this increment.
Full Control-Service composition and authorized production DNS transport remain open.

## Seventh backend extraction: DNS statistics proxy (2026-10-14)

**Implemented, verified, committed/pushed as `ed27bca`, and subsequently deployed
to Control Room/Control-Service at 2026-10-14 16:26 UTC.** The implementation
handoff below describes its pre-deployment validation; the
[release ledger](GUI%20deployments.md) records current runtime scope.

- Moved DNS statistics URL validation and HTTP/HTTPS forwarding into
  [`internal/dnsstats`](zpr-dashboard/internal/dnsstats/). Its two constructors
  accept an explicit upstream URL for the statistics prefix and stylesheet.
  The command retains `ZPR_DNS_STATS_URL` lookup and existing route registration.
  There is no Simulator API, configuration, manifest, runtime or availability
  dependency; a standard-library-only transitive dependency test enforces this
  package boundary.
- Preserved HTTP/HTTPS origin validation, whitespace handling, path-prefix
  behavior, root mapping, query/body/method forwarding, browser credential and
  existing forwarding-header suppression, `no-store` upstream responses, default
  Go HTTP transport and flush behavior. HTTPS certificate verification is not
  bypassed. The upstream Host behavior is unchanged.
- Missing/invalid configuration still produces the same plain-text 503 response;
  upstream transport failures still produce the same 502 response. Upstream
  HTTP error statuses and bodies are preserved with `no-store`.
- Existing command-level forwarding/stylesheet tests and independent operator
  relay/script tests remain intact. New package tests cover both constructors,
  validation, path/query/body/headers, input immutability, upstream and transport
  errors, prefix rejection, HTTPS trust verification and explicit operation
  with invalid Simulator and unrelated DNS environment settings. New command
  tests preserve configuration-error messages for both adapter constructors.
- DNS record AXFR/TSIG behavior, relay processes, BIND configuration, production
  transport selection and operator authorization are outside this extraction
  and were not changed.

Passing validation:

```sh
go test -race ./internal/dnsstats ./cmd/zpr-web-dashboard -run 'DNSStats|ControlServiceDNS|OperatorDNS|ControlRoom' -count=1
go vet ./internal/dnsstats ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

The editor runner reports 39 selected passing tests. `make test` retains exactly
the same four baseline failures and causes recorded below; all remaining Go tests
pass with those four explicitly excluded. Formatting, editor diagnostics and diff
whitespace checks pass. No unrelated failing tests, dependencies, static assets,
scripts, credentials, runtime configuration or live state were changed. No image
build, deployment, commit or push was performed. Full Control-Service composition
and an authorized production ZPR DNS transport remain separate work.

## Sixth backend extraction: local request boundary (2026-10-14)

**Implemented, verified and deployed to Control Room only; uncommitted.**
The current-worktree release `zpr-editor-ui:20261014-authorization-local-request`
started at 2026-10-14 15:57 UTC and includes the fifth and sixth increments.
Simulator and backend services were not replaced or restarted. See the
[deployment ledger](GUI%20deployments.md#control-room-authorization-and-local-request-extraction-2026-10-14-1557-utc).

- Extracted the shared editor/service local-request check from `policy.go` into
  [`internal/servicehttp/local_request.go`](zpr-dashboard/internal/servicehttp/local_request.go).
  The existing standard-library-only HTTP/TLS package owns the check; no new
  package or dependency was needed.
- The function accepts the request and explicit trusted proxy IP. The command
  retains `ZPR_CONTROL_ROOM_PROXY_IP` lookup on each request and writes the same
  HTTP 403 JSON errors. All existing policy, assertion, gateway, Control Room
  and other callers still use the unchanged `localEditorRequest` adapter.
- Preserved verified-client-TLS acceptance, loopback or exact trusted-proxy
  peer matching, loopback Host requirement, exact same-scheme/host Origin
  matching, and peer-before-host-before-origin rejection order. Unverified TLS
  does not skip local checks; forwarded headers do not establish peer trust.
  Historical policy-workspace error messages remain exact, even for non-policy
  callers. This is a behavior-preserving extraction, not a trust-policy change.
- Package table tests cover IPv4/IPv6, mapped addresses, proxy configuration,
  malformed peers/hosts/origins, precedence, forwarded-header spoofing, input
  immutability, and verified/unverified TLS. Invalid Simulator settings and an
  unrelated proxy environment value do not influence explicit package inputs.
- A real TLS test accepts a verified client certificate on a production-host
  request and rejects the same host without a client certificate. Command
  tests verify exact JSON status/headers/body, configuration changes between
  requests, and no response writes for accepted requests. Existing application
  boundary and production/delegation regressions remain intact.
- The package's standard-library-only dependency check continues to pass.
  No application configuration, Simulator imports or hidden state were added.

Passing validation:

```sh
go test -race ./internal/servicehttp ./internal/controlroom ./cmd/zpr-web-dashboard -run 'LocalRequest|LocalEditor|PolicyEditorRejects|ServiceHTTP|MutualTLSClient|ControlRoom|ControlService|OperatorAuthorization|OperatorEnrollment|OperatorDelegation|SimulatorAPI|ChangeReview' -count=1
go vet ./internal/servicehttp ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

The editor runner reports five selected passing tests. `make test` retains the
same four baseline failures and causes recorded below; all remaining Go tests
pass with those four explicitly excluded. Formatting, diagnostics and whitespace
checks pass. No unrelated tests, assets, launch scripts, certificates, runtime
configuration or state were changed by the extraction. The subsequent
Control Room-only deployment preserved configuration and the other 20 containers,
matched 77 served assets, and passed real OIDC login, authenticated reads and
unmapped-route/CSRF denial checks. No commit or push was performed.
Full application composition remains a subsequent stage.

## Fifth backend extraction: Control Room API authorization (2026-10-14)

**Implemented, verified and deployed to Control Room only; uncommitted.**
The subsequent 2026-10-14 15:57 UTC current-worktree release includes this
authorization extraction and the sixth local-request increment. Other services
were not restarted. See the
[deployment ledger](GUI%20deployments.md#control-room-authorization-and-local-request-extraction-2026-10-14-1557-utc).

- Moved the API method/path permission map and named-user authorization gate
  from command-local `operator_policy.go` into
  [`internal/controlroom/authorization.go`](zpr-dashboard/internal/controlroom/authorization.go).
  The gate accepts an authorizer, explicit organization, next handler and
  application error formatter. It reads no environment or Simulator state.
- The command retains absent/typed-nil authentication detection and local-only
  development behavior, local peer/host/origin checks, dedicated enrollment
  handling, error formatting, OIDC/session configuration and server startup.
  Existing Simulator and change-review consumers reuse the authorizer interface
  through a command type alias; neither contributes policy to Control Room.
- Method/path matching and permission names are unchanged. Unknown routes are
  rejected before organization lookup or authorization, missing organization
  still returns 503, and failed authorization still returns the same 403 JSON.
  Operator-configured organization is trimmed once and cannot be replaced by a
  request query. Request identity, context, CSRF header, body and next-handler
  ordering are unchanged.
- Moved the original permission-map test with its implementation and expanded
  method, prefix, unsupported-route and gateway coverage. Package gate tests
  verify exact permission/scope, denial ordering, no-store responses and absence
  of downstream calls on denial, with invalid/unavailable Simulator settings.
  Command tests retain nil/typed-nil development fallback and verify exact JSON
  error headers/bodies and local peer/host/origin rejection after authorization.
- The package's existing transitive in-module allowlist still permits only
  Control Room and production authentication/delegation packages. No new
  dependencies, environment settings, routes, modes, assets or scripts were added.

Passing validation:

```sh
go test -race ./internal/controlroom ./cmd/zpr-web-dashboard -run 'ControlRoom|ControlService|OperatorAuthorization|OperatorConfig|OperatorEnrollment|OperatorDelegation|SimulatorAPI|ChangeReview|ConfigurationForm|GatewayAssistantPermissions' -count=1
go vet ./internal/controlroom ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

The editor runner reports 70 selected passing tests. `make test` retains exactly
the same four baseline failures and causes recorded below; all remaining Go
tests pass with those four explicitly excluded. Formatting, editor diagnostics
and diff whitespace checks pass. No unrelated failing tests, certificates,
runtime configuration or live state were changed by the extraction. Its
subsequent Control Room-only deployment passed real login/authenticated reads
and authorization denial checks with configuration preserved. No commit or push
was performed. The shared local-request check was subsequently
extracted in the sixth increment above; complete operator composition remains
pending.

## Fourth backend extraction: Control Room upstream proxy (2026-10-14)

**Implemented, verified and deployed to Control Room only; uncommitted.**
The current-worktree release `zpr-editor-ui:20261014-controlroom-package` started
at 2026-10-14 15:40 UTC. Simulator retains the first three extractions; backend
services and separate gateways were not replaced. See the
[deployment ledger](GUI%20deployments.md#control-room-current-worktree-and-proxy-extraction-2026-10-14-1540-utc).

- Extracted Control Room-to-Control-Service proxy behavior into
  [`internal/controlroom`](zpr-dashboard/internal/controlroom/). It consumes an
  explicitly validated service origin and configured mTLS transport; it reads
  no environment variables, Simulator configuration, profiles or runtime state.
- The command retains endpoint and certificate configuration, origin validation,
  login/authorization, CSRF, delegation signing, JSON error responses/logging,
  API registration, static embedding and server lifecycle.
- Browser credentials, CSRF/fetch metadata and existing forwarding-header
  suppression are unchanged. Only the server-signed request-context assertion
  reaches the operator-enrollment prefix; browser-supplied assertions are removed.
  The private context key moved with the proxy; the existing signer attaches the
  assertion through the package helper only after successful authorization/signing.
- The supplied standard transport is retained; the slow-read clone keeps the
  20s response-header timeout for adapter logs and assertion evaluation/source.
  Other requests retain the configured timeout (10s in the command). Flush
  interval remains 100ms. Paths, escaped paths, queries, bodies, response status
  and JSON error text remain compatible.
- Moved transport-selection tests with their implementation, retained command
  real-mTLS and signed-enrollment integration coverage, and added package tests
  for stripping, delegation scope, transport cloning, unavailable upstreams and
  operation with invalid/unavailable Simulator configuration. Command tests
  preserve missing/invalid configuration messages and the exact 502 JSON body.
- A transitive dependency check permits only this package and the existing
  production `operatorauth`/`operatordelegation` packages within the dashboard
  module. Existing authentication dependencies remain unchanged; unlike the
  neutral leaf packages, this production package is not standard-library-only.
  No new dependencies were added.

Passing validation:

```sh
go test -race ./internal/controlroom ./cmd/zpr-web-dashboard -run 'ControlRoom|ControlService|OperatorEnrollment|OperatorDelegation|AdapterLog|LogComponents' -count=1
go vet ./internal/controlroom ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

The editor test runner also reports 16 passing selected tests. `make test`
retains exactly the four baseline failures recorded below with the same causes;
all remaining Go tests pass with those four explicitly excluded. No unrelated
failing tests, assets, scripts, production certificates or runtime state were
changed by the extraction. Its subsequent Control Room-only deployment preserved
configuration and the other 20 running containers, matched 77 served assets,
and passed real OIDC login plus authenticated production-service reads,
including signed enrollment-catalog forwarding. No commit or push was performed.
Full operator application composition and its complete transitive
Simulator boundary remain subsequent stages.

## Third backend extraction: service TLS transport (2026-10-14)

**Implemented, verified, included in Simulator image only; uncommitted.**
The Simulator release `zpr-simulator:20261014-servicehttp-package` started at
2026-10-14 15:14 UTC and includes all three extractions. That release did not
update other services. The subsequent Control Room current-worktree release
also includes these TLS primitives; Control-Service, Policy-Service and separate
gateways remain unchanged. See the
[deployment ledger](GUI%20deployments.md#simulator-service-tls-package-extraction-2026-10-14-1514-utc).

- Extracted CA loading and mTLS client transport into
  [`internal/servicehttp`](zpr-dashboard/internal/servicehttp/). Its inputs are
  explicit paths, certificate/root objects, and TLS server name; it reads no
  environment variables, profiles, manifests, or runtime state beyond those
  certificate files. A standard-library-only dependency test enforces the leaf
  boundary.
- Control Room's Control-Service proxy, Control-Service's incoming client-CA
  setup, trusted change-feed CA loading, and Simulator's explicitly configured
  operator-service client now use that package. The Policy-Service client
  reuses the identical transport construction while retaining its existing
  service-specific certificate/CA diagnostics.
- TLS 1.3 minimum, CA/hostname verification, client certificates, dial timeout
  (5s), keepalive (30s), handshake timeout (5s), response-header timeout (10s),
  idle timeout (60s), and no implicit environment proxy are unchanged. The
  Control Room slow-read transport still clones and overrides its timeout in
  the application layer.
- Service routes, proxy/header/delegation policies, environment variables,
  defaults, server lifecycle, and listener/client identity ownership remain
  unchanged. Shared transport errors are now Go errors with wrapped causes;
  existing string-based proxy adapters preserve their user-visible messages.
  No fallback to Simulator or system trust was introduced.
- Local certificate tests accept valid mutual TLS and reject wrong hostname,
  untrusted server, missing/untrusted client certificate, missing certificate/key,
  and missing/invalid CA. Tests verify transport defaults and explicit operation
  with invalid Simulator settings. Existing production proxy, trusted-feed,
  authorization, and Simulator client integration tests pass unchanged.

Passing validation:

```sh
go test -race ./internal/servicehttp ./cmd/zpr-web-dashboard -run 'ServiceHTTP|MutualTLSClient|CertificateErrors|PolicyService|ControlService|ControlRoom|Trusted.*Change|Simulator.*(Snapshot|Operator)' -count=1
go vet ./internal/servicehttp ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

`make test` retains exactly the four baseline failures recorded below; all other
Go tests pass with those four explicitly excluded. No baseline failing tests,
dependencies, assets, scripts, production certificates, or runtime configuration
were changed by the extraction. The subsequent Simulator-only deployment
preserved configuration and DNS relays, matched all 76 directly served assets,
and left the other 20 persistent running containers unchanged. Focused service
TLS race regressions pass; production runtime TLS acceptance remains separate.
Application composition and the complete production/Simulator dependency barrier
remain subsequent stages.

## Second backend extraction: browser access gateway (2026-10-14)

**Implemented, verified, included in Simulator image only; uncommitted.**
The subsequent Simulator release `zpr-simulator:20261014-browsergateway-package`
started at 2026-10-14 14:51 UTC. The separate browser-access gateway process
and production services were not replaced. See the
[deployment ledger](GUI%20deployments.md#simulator-browser-access-package-extraction-2026-10-14-1451-utc).

- Moved host routing, loopback-origin validation, proxy credential/header
  stripping, HSTS, error responses and mTLS configuration into
  [`internal/browsergateway`](zpr-dashboard/internal/browsergateway/).
  Only `New` (returning `http.Handler`) and `TLSConfig` are exported; route maps,
  normalization and proxy internals remain private.
- The existing command retains environment/default resolution, security-header
  wrapping, timeouts, logging and listener startup. No flags, default upstreams,
  mode names, resources, deployment scripts or runtime state were changed.
  The optional browser gateway still routes its explicitly configured two
  hosts; that is not a Control Room dependency on Simulator.
- Added package tests for invalid/duplicate host configuration, loopback-only
  upstreams, verified-client requirements, unknown hosts, stripped browser/proxy
  credentials, preserved query/path/status/body, HSTS, and upstream failure.
  Control Room routing passes with the configured Simulator upstream offline
  and invalid Simulator environment settings.
- Kept the existing real certificate/HTTPS integration test at the command
  boundary because it uses the command's certificate fixtures. It now invokes
  the extracted APIs and explicitly checks TLS 1.3 minimum and required verified
  client certificates. A command test preserves missing-configuration/certificate
  error behavior. Package-local unit tests live with the extracted implementation.
- A second `go list -deps` test enforces standard-library-only dependencies
  for browser access routing, separate from Internet-egress forwarding.

Passing validation:

```sh
go test -race ./internal/browsergateway ./internal/gateway ./cmd/zpr-web-dashboard -run 'Browser|Gateway|ValidateWebGateway' -count=1
go vet ./internal/browsergateway ./internal/gateway ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

`make test` retains exactly the same four failures described in the first
extraction baseline below; all other Go tests pass with those four explicitly
excluded. No pre-existing failing test was changed. These implementation
checks did not use live host certificates or perform production forwarding.
The later Simulator-only deployment preserved configuration, restored both
relays, verified assets and left all 21 other running containers unchanged.
It does not establish rollout or acceptance of the separate gateway process.
Control Room/Control-Service composition and full application dependency
enforcement remain open.

## First backend extraction (2026-10-14)

**Implemented, verified, deployed to Simulator only; uncommitted.** This is the
first bounded backend increment, not completion of the proposed dashboard tree.
Simulator release `zpr-simulator:20261014-gateway-package` started at
2026-10-14 14:38 UTC with configuration preserved; production services and
runtime machines were not replaced. See the
[deployment ledger](GUI%20deployments.md#simulator-first-backend-package-extraction-2026-10-14-1438-utc).

- Moved `gateway_forwarding.go` and `web_gateway.go` into
  [`internal/gateway`](zpr-dashboard/internal/gateway/), with their neutral
  unit tests. The command calls explicit package APIs for dispatch, validation,
  forwarding, and host checks; there are no legacy wrapper implementations.
- Kept the fixed-upstream scenario handler, workload-event logging, organization
  profile lookup, and Simulator launch-selection tests command-owned. Header
  allowlists specific to the fixed-upstream adapter remain with that adapter.
- Added a dependency test over `go list -deps`: the extracted package can depend
  only on the standard library. This enforces one neutral leaf, not every
  production/Simulator import direction.
- Preserved all 20 mode names, flag/default definitions, JSON/error behavior,
  forwarding limits, allowlist/tunnel semantics, and public asset paths.
  No static files, examples, skills, scripts, manifests, module identifiers,
  dependencies, or protected runtime state were moved or changed.
- Next: capture a production service/client or Simulator-owned boundary with
  its route, authorization, and configuration contracts before another
  extraction. Full Control Room/Control-Service isolation and thin command
  composition remain open.

### Validation and baseline

Before extraction, `go test ./...` failed in four top-level tests. After
extraction, `make test` fails in exactly the same four, with the same causes:

| Existing failing test | Baseline cause |
|---|---|
| `TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator` | Invitation creation returns HTTP 409 for missing current readiness preflight |
| `TestOrganizationPolicyInstallNeverUploadsStaleBundle` | Extracted shell fixture has unset `ZPR_MULTINODE_PLATFORM` |
| `TestScenarioPlacementPreflightMatchesLauncher` | Shell fixture cannot find `resolve_machine_placement` |
| `TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact` | Mock artifact is not accepted as a Linux amd64 ELF executable on this host |

These tests were not changed or suppressed in the normal test workflow.
All other Go packages/tests pass when those four are explicitly excluded:

```sh
go test ./... -skip '^(TestEnrollmentAdministrationOverVerifiedTLSWithoutSimulator|TestOrganizationPolicyInstallNeverUploadsStaleBundle|TestScenarioPlacementPreflightMatchesLauncher|TestTrustedServiceBuildPublishesAtomicallyAndKeepsFreshArtifact)$' -count=1
```

Additional passing checks on macOS with Go 1.26.1:

```sh
go test -race ./internal/gateway ./cmd/zpr-web-dashboard -run 'Gateway|WebGateway|ValidateWebGateway' -count=1
go vet ./internal/gateway ./cmd/zpr-web-dashboard
go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/zpr-web-dashboard
```

Gateway coverage retains real local HTTP/HTTPS method/body forwarding, CONNECT
tunnels, destination/port rejection, header filtering, size limits, redirect
behavior, and Great Lakes/Northstar launch arguments. New checks cover the
dependency graph, invalid startup configuration, and construction with invalid
Simulator settings. CLI smoke verification confirms `web-gateway-service`
still dispatches to its existing workload validation without starting a server.
These pre-deployment checks did not claim live network or browser certification;
native and both Linux builds compile the existing embedded resources.
The later image build and Simulator-only deployment passed configuration,
asset, relay, and service-isolation checks; 62 selected desktop/tablet fixture
regressions also pass. This is not production gateway rollout or live forwarding
acceptance.

## Inventory and target layout (2026-10-14)

The user accepted this direction for documentation and staged planning.
The following is a source inventory and proposed destination map, not completed
extraction, deployment approval, or a symbol-by-symbol migration plan.

### Measured current layout

| Area | Observed scope |
|---|---|
| Repository | 725 tracked files, excluding untracked/runtime/generated material |
| Dashboard | 540 tracked files; one Go module, 19 command/internal packages resolved with `go list` |
| Web command | 134 tracked Go files; current worktree has 60 implementation files and 75 tests, including one untracked test |
| Executable | 20 dispatched modes; default remains Control Room |
| Browser assets | One mixed static directory; 47 JavaScript, 19 CSS, nine HTML files, one SVG, and two license files in the tracked inventory |
| Browser tests | 26 tracked modules |
| Rust | Five workspace crates retained: `vs`, `libeval`, `zpt`, `vs-admin`, `admin-api-types` |
| Path-sensitive resources | Embedded static files, seven embedded assistant skills, examples, Docker builds, stack scripts, browser fixtures, CI and release commands |

Large-file examples: `static/app.js` has 4,865 lines; `vs/src/visa_mgr.rs`
has 2,833; `vs/src/admin_service.rs` has 2,515; web `scenario.go` has 1,640.
These measurements identify review candidates, not line-count splitting targets.

At inventory time, ownership was obscured by the shared web `package main`: production
services, Simulator, gateway runtimes, machine control, and test workloads can
access each other's unexported functions and globals. For example,
`policy_layers.go` loads Simulator organization definitions; that composition
belongs to Simulator provisioning, not production repository defaults.
Control Room and Control-Service composition also occupy the same source file.
This is evidence of coupling risk, not proof that every production path calls
Simulator.

### Proposed dashboard tree

Names below are proposed package destinations, not existing source paths.
Create only cohesive packages justified by actual dependencies.

```text
zpr-dashboard/
  cmd/
    zpr-web-dashboard/main.go     # existing flags/modes; thin dispatcher
    ...                          # existing dedicated commands retained
  internal/
    controlroom/                 # UI serving, operator session, API proxy
    controlservice/              # production composition
      inventory/                 # Visa Admin snapshot/actors/counters
      diagnostics/               # operator-configured logs/metrics
      trustedsources/            # provider/change-feed integration
      assertions/                # evaluation/runtime coordination
      dns/                       # independent production transport
    policyservice/
      repository/                # SQLite records/categories/revisions
      configform/                # configuration-form conversion
    simulator/
      organizations/             # profile/definition bundles
      workspace/                 # Simulator artifact revisions
      scenarios/                 # validation/execution/cancellation
      machines/                  # sessions/control/workloads
      provisioning/              # activation and seeded policy composition
    gateway/                     # neutral forwarding/runtime contracts
    browsergateway/              # separate browser-access proxy
    testworkloads/               # test/stress/benchmark roles
    assistant/                   # transport; caller owns task/context
    servicehttp/                 # narrow shared HTTP/TLS primitives
    operatorauth/                # existing
    operatordelegation/          # existing
    enrollment/                  # existing
    changereview/                # existing
    app/, pages/, components/    # existing TUI packages retained
    ...                          # other existing cohesive packages retained
    webui/
      embed.go
      static/
        shared/
        controlroom/
        simulator/
  examples/
    simulator/
    policy/
  skills/
    operator/
    simulator/
  scripts/                       # retain launch interfaces
  tests/browser/                 # shared fixtures and feature tests
```

Tests follow their implementation packages; HTTP/composition and browser tests
remain at integration boundaries. Task skills need a package-local embedding
strategy (for example, an owning resource package with its embedded files);
moving them to a top-level directory alone does not preserve Go embedding.
Keep public asset URLs stable initially using explicit serving mappings.

### Allowed dependency direction

- Control Room -> Control-Service -> production APIs, Policy-Service,
  configured providers, and independent operator DNS/log origins.
- Simulator -> public production service contracts, never the reverse.
- Shared rendering/transport leaves -> neither application's runtime state.
- The compatibility dispatcher may assemble both applications; production
  implementation packages must not import Simulator, directly or transitively.

Reuse existing authentication, delegation, enrollment and change-review packages.
Do not introduce a catch-all shared package. Keep one Go module and its existing
module identifier; this plan does not rename the repository or add microservices.
Retain Rust crate roots and independent sibling repositories.

### Compatibility and staged delivery

Before moving code, capture the affected API routes/JSON, named-user permissions,
organization scope, TLS/mTLS, CSRF, flags/environment defaults, persistence paths,
skill/static embedding, and launch/package behavior. The current inventory found:

- `Dockerfile.simulator` builds the shared command, also used for other modes.
- `dashboard-stack.sh` references command-local examples and builds machine
  binaries from that same command.
- Browser fixtures read the current static directory directly.
- Default dashboard Make targets build the terminal UI; that alone does not
  validate the web image.

Sequence: (1) contract/path baselines and ownership, (2) Simulator-only or neutral
leaf extraction, (3) production composition and thin commands, (4) frontend
adapters/controllers and mapped assets, (5) examples/skills/script relocation,
(6) later Rust responsibility subdivision. Validate every extraction and enforce
production independence with Simulator unavailable and its environment settings
unset or invalid. Asset/skill relocation waits for embedding and packaging checks.

Runtime keys, protected configuration, databases, authenticated profiles, and
active organization state are outside source moves. Do not reset or migrate
state incidentally. Unrelated pending changes are not part of an extraction.

The inventory itself was read-only: `go list` succeeded, but no new build/test baseline,
full cross-domain call audit, source relocation, commit, push, or deployment was
performed for it. The later first extraction and its validation are recorded
separately above. The checklist below deliberately keeps the broad inventory
gates open until their remaining contract, call, and baseline evidence exists.
Use the [component index](../COMPONENT_WORK.md#suggested-first-assignments) for
routing and staged assignments, not a second copy of these checklists.

## 1. Inventory and migration safety

- [ ] Inventory application modes, entry points, packages, public contracts, embedded assets and runtime configuration.
  - Initial mode/package/asset/path inventory is recorded above; exhaustive
    contracts and configuration behavior remain to be captured.
- [ ] Map current imports and cross-domain calls; identify production code that can access Simulator implementation through the shared Go main package.
- [ ] Record ownership for Control Room, Control-Service, Policy-Service/repository, trusted sources/assertions, Simulator and shared utilities.
- [ ] Record existing API routes, authorization, TLS/mTLS, CSRF, configuration defaults and startup behavior as compatibility requirements.
- [ ] Establish targeted test/build baselines and document unrelated existing failures without changing their tests merely to make the refactor pass.
- [ ] Inventory scripts, container builds, packaging, CI, examples, tests and documentation that depend on source paths.
- [ ] Preserve unrelated worktree changes and coordinate edits to shared files.
- [ ] Keep runtime keys, protected configuration, databases and authenticated profiles outside source moves; never recreate or overwrite them as part of reorganization.
- [ ] Use small, independently reviewable extractions with validation after each stage; do not combine structural changes with scaling or feature changes.

## 2. Enforce production and Simulator package boundaries

- [ ] Extract Simulator implementation into Simulator-owned packages, including profiles/manifests, sessions, workloads, scenario execution and runtime orchestration.
- [ ] Keep Control Room and Control-Service dependent only on production contracts and operator configuration, never Simulator packages or availability.
- [ ] Allow Simulator to exercise operator services through explicit public contracts without introducing reverse dependencies.
- [ ] Extract neutral API contracts and rendering/helper code only where they contain no Simulator-specific defaults or hidden runtime access.
  - Neutral forwarding and allowlisted HTTP/CONNECT runtime are extracted to
    `internal/gateway`; fixed-upstream workload adapters remain command-owned.
  - Browser-access routing/TLS and service CA/client TLS primitives are also
    extracted to independent `internal/browsergateway` and `internal/servicehttp`
    packages; application composition is still command-owned.
- [ ] Define allowed dependency directions and avoid a catch-all shared package that recreates the current coupling.
- [ ] Add dependency-boundary checks that fail when production packages import Simulator implementation.
  - The first leaf check enforces standard-library-only dependencies for
    `internal/gateway`; application-wide transitive checks remain pending.
  - `internal/controlroom` now enforces a transitive in-module allowlist limited
    to itself and production authentication/delegation packages. The remaining
    shared command application is not covered by this package-level check.
- [ ] Test production startup and operator features with Simulator unavailable and Simulator environment settings unset or invalid.
  - DNS statistics transport was verified with Simulator paused; complete
    startup/feature coverage and package import enforcement remain pending.

## 3. Thin Go command entry points

- [ ] Move domain behavior out of cmd/zpr-web-dashboard into cohesive internal packages.
- [ ] Keep command entry points focused on flags/configuration, dependency assembly, lifecycle and server startup.
- [ ] Extract production operator application/HTTP handlers and clients with explicit injected dependencies.
  - DNS statistics URL validation and statistics/stylesheet forwarding now live
    in standard-library-only `internal/dnsstats`, with an explicit upstream;
    environment lookup and route registration remain command-owned.
  - Signed DNS record transfers and TSIG parsing now live in `internal/dnsrecords`,
    taking an explicit address/key path and using the existing DNS dependency.
    Per-request environment lookup and route authorization remain command-owned.
  - Policy-Service forwarding now lives in standard-library-only
    `internal/policyclient`, taking an explicit validated origin and mTLS
    transport. Assistant enrichment, environment/TLS configuration, authorization
    and application errors remain command-owned.
    The bounded assistant attribute fetch and two-field attribute contract now
    also live in that package; local-workspace selection and configuration stay
    in the command.
  - Control Room's upstream proxy, credential/delegation forwarding and slow-read
    transport selection are now in `internal/controlroom`. API permission mapping
    and named-user authorization are also extracted. The shared local-request
    check now lives in `internal/servicehttp` with explicit proxy configuration.
    Development-mode selection, signing, environment lookup, application
    composition and startup remain command-owned.
- [ ] Extract policy repository and versioned document behavior without exposing storage internals to unrelated domains.
- [ ] Extract trusted-source connectors, assertion evaluation and runtime coordination with clear interfaces.
- [ ] Extract assistant transport and task prompt ownership while preserving endpoint permissions, request bounds and response contracts.
- [ ] Preserve build embedding of task skills and static resources after moving packages; verify missing required assets still fail clearly.
- [ ] Reuse existing authentication, delegation and enrollment packages rather than introducing parallel implementations.
- [ ] Keep unexported details private and export only interfaces/types needed by callers; resolve import cycles through ownership changes rather than broad public APIs.
- [ ] Decide whether separate command entry points improve ownership; retain existing launch compatibility or document explicit changes.
- [ ] Keep one Go module/release process unless an independent lifecycle genuinely requires separation.
- [ ] Move directly related tests with their domains and retain HTTP/integration tests at application composition boundaries.

## 4. Organize frontend by feature and shared behavior

- [ ] Extract map/topology rendering and lifecycle from the large app.js controller.
- [ ] Extract visa/monitoring, policy editing, trusted-source and log feature controllers.
- [ ] Give Control Room and Simulator explicit page entry modules and service adapters.
- [ ] Keep shared editor, table, request, display and lifecycle utilities domain-neutral.
- [x] Complete shared editor-controller reuse using the [completed JavaScript checklist](GUI%20completed%20work.md#javascript-reuse-refactoring) rather than duplicating competing helpers. All six editors use the shared controller; deployed with `zpr-editor-ui:20261009-unified`.
- [ ] Pass load/analyze/save/service behavior through explicit adapters; do not let shared UI utilities reach Simulator state or endpoints.
- [ ] Preserve editor identity/History, dirty state, source-owned diagnostics, keyboard shortcuts, undo, assistant insertion and viewport behavior.
- [ ] Preserve request cancellation, stale-response rejection, polling cleanup and explicit CSRF policies during extraction.
- [ ] Keep the existing JavaScript stack unless a concrete need justifies tooling changes; do not introduce a framework solely to rearrange files.
- [ ] Update asset embedding, script/module load order and cache versions so deployed assets match the new source layout.
- [ ] Validate affected desktop/tablet browser flows and production features with Simulator unavailable.

## 5. Define organization artifact ownership

- [ ] Document the authoritative representation of organization identity, profile, directory, policy, configuration and scenarios.
- [ ] Decide whether profile people/groups are authored data or derived directory summaries; do not retain competing editable authorities.
- [ ] Define structured/source conversion rules, preserve unsupported fields and verify round-trip behavior.
  - Profile Form/Raw synchronization and local-edit preservation are verified;
    full cross-document conversion and compiler/reference checks remain open.
- [ ] Treat checked-in organization examples as seeds/templates, not a second live database.
- [x] Define coherent private organization draft/revision boundaries across profile and supporting documents: atomic whole-bundle saves, optimistic revision checks and matching historical snapshots are implemented. See the [organization editor plan](Organization%20editor%20plan.md).
- [ ] Keep publication, provisioning and activation separate from saving draft state.
- [ ] Align the [organization editor plan](Organization%20editor%20plan.md) with these ownership decisions.
  - Draft save/publication/activation separation is implemented; authoritative
    profile-versus-directory data ownership is still unresolved.

## 6. Correct misleading naming deliberately

- [ ] Review names that no longer match their responsibility, including the dashboard umbrella and Simulator policy-only database filename.
- [ ] Choose descriptive package and application names without forcing an unnecessary repository/module rename.
- [ ] Decide explicitly whether existing database names remain supported, migrate, or require a documented reset; do not casually rename runtime state.
- [ ] Update configuration references, launch scripts, packaging, examples and documentation for approved naming changes.
- [ ] Verify existing runtime data and protected configuration remain usable under the chosen compatibility policy.

## 7. Subdivide large Rust modules by responsibility

- [ ] Review visa_mgr responsibilities and split issuance, actualization/distribution and lifecycle behavior into cohesive internal modules where useful.
- [ ] Group admin HTTP routes by visas, actors, topology and policy instead of one large handler module.
- [ ] Keep persistence ordering, secondary-index consistency, generation checks and revocation transitions together with their invariant tests.
- [ ] Avoid splitting tightly coupled invariant logic merely to reduce line counts.
- [ ] Search usages across the workspace before moving or renaming public symbols.
- [ ] Preserve public crate contracts unless an explicitly reviewed change is necessary.
- [ ] Move unit tests with implementation and retain cross-domain lifecycle/integration coverage.
- [ ] Run cargo test after Rust changes and the relevant formatting/build checks; do not combine this stage with scaling optimizations.

## 8. Documentation and artifact layout

- [ ] Add a documentation index distinguishing architecture/contracts, operator/developer guides, work trackers and historical feature summaries.
  - The workspace component index now links these categories and ownership;
    comprehensive repository guide indexing remains open.
- [ ] Consolidate scattered guidance through links and clear ownership before relocating documents.
- [ ] Update relative links, README instructions and tool guidance after any approved documentation moves.
- [ ] Verify generated builds, test reports, dependencies and local runtime state are excluded from source control as appropriate.
- [ ] Document where source assets, checked-in fixtures, generated outputs and protected runtime state belong.
- [ ] Preserve independent repository boundaries and Git metadata; do not relocate sibling projects just to create a visually uniform tree.

## 9. Completion and deployment verification

- [ ] Verify each extraction preserves API shapes, authorization, TLS/mTLS, CSRF and runtime defaults.
- [ ] Run targeted domain tests followed by required project checks; report baseline failures separately.
- [ ] Confirm build/container/installer packaging includes all moved assets and skills.
- [ ] Review the resulting dependency graph for cycles, accidental Simulator coupling and catch-all utility packages.
- [ ] Update directly affected READMEs, architecture guidance and development commands.
- [ ] If deployment is requested, use supported restart scripts and preserve authenticated profiles, keys, databases and active organization state.
- [ ] Verify served assets and service health after deployment; do not equate a local build with a deployed change.
- [ ] Record completed checklist items only after behavior and persistent source changes are verified.

## Recommended delivery order

Use the [seven remaining milestones](#remaining-delivery-milestones-2026-10-14)
and their execution order as the delivery sequence. Sections 1-9 above supply
the detailed requirements and evidence; they are not nine additional milestones.
