# Source and Project Reorganization Plan

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
Great Lakes Restore exposed an unresolved assertion-seeding verification failure
and rolled back successfully. Keep that repair under independent production
configuration contracts, not a restored Simulator-profile fallback.
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
seven edits, seven sessions or seven deployments. The thirteen extractions above
are evidence toward these gates, not thirteen completed milestones. Do not add
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
| M3 | Policy and trusted-source ownership | Planned | Repository/versioned documents, configuration forms, trusted connectors and assertion evaluation have cohesive owners and explicit contracts. Separate Simulator seeding/provisioning from production repository defaults; preserve storage ordering, authorization, source-owned diagnostics and task prompt/context boundaries. Reuse existing authentication/enrollment/delegation packages. |
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
audit below is recorded and its focused baseline passes. Exhaustive per-mode
configuration/default and packaging acceptance remains open; M1 is not closed.
The audited assistant configuration slice is implemented and locally verified,
as recorded below. Production repository seeding and fixture inputs remain
Simulator-coupled and are the next bounded ownership work. Report progress as milestones closed/open and the current
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
| `main.go`, `control_service.go` | Mixed dispatcher, shared application/models, Visa Admin client/snapshot, Control Room and Control-Service startup. Keep flags/assembly/lifecycle at the entry point; move production inventory and HTTP/application behavior to production owners. Static embedding currently stays here. |
| `operator_login.go`, `operator_policy.go`, `operator_delegation.go`, `enrollment.go`, `change_review.go` | Production security/enrollment/review composition using existing packages. `operator_login.go` also assembles Simulator security: separate caller configuration from neutral security mechanisms rather than importing a Simulator owner into Control Room. |
| `node_denials.go`, `node_display_names.go` | Production inventory enrichment/configuration; candidate owner `controlservice/inventory`. Denial/counter collection currently shares diagnostics types and provider selection. |
| `adapter_logs.go`, `diagnostics.go`, `dns_records.go`, `dns_stats.go`, `trusted_changes.go` | Production operator logs/metrics, DNS and change feeds. Adapter inventory, DNS transport and log formatting already have packages; remaining HTTP/provider/collection composition belongs to Control-Service. |
| `gateway_api.go`, `gateway_config.go`, `gateway_config_store.go` | Production gateway contracts, validation and draft revisions; keep separate from Simulator workload execution and actual gateway activation. |
| `browser_gateway.go`, `gateway_service.go` | Browser-access runtime adapter and mixed gateway runtime/workload adapters. Reuse `browsergateway`/`gateway` cores; move fixed-upstream scenario logging to the Simulator/test-workload owner, not the production transport. |
| `policy.go`, `policy_service.go`, `policy_store.go`, `policy_staging.go`, `policy_attributes.go`, `policy_assertions.go`, `policy_test_api.go`, `policy_test_fixtures.go`, `config_form.go` | Policy repository/API/configuration/staging/attributes/assertion settings and policy-test contracts. Mixed files still contain HTTP utilities and Simulator-derived initialization/fixtures; these must not move wholesale into a production package. |
| `assistant_config.go` | Production assistant environment/path resolution and shared model/HTTP client composition. No Simulator discovery; protected file reading is in neutral `internal/assistant`. Keep only production/neutral dependencies when moving this composition into a production package. |
| `assertions.go`, `assertion_expressions.go`, `assertion_ldap.go`, `assertion_runtime.go`, `assertion_sources.go`, `lint.go` | Parser/evaluator/lint, provider connectors and Control-Service assertion scheduling. Separate reusable evaluation contracts from production runtime, repository settings and Simulator consumers; preserve source-owned diagnostics. |
| `assistant_skills.go`, `editor_assistant.go`, `design_assistant.go` | Embedded task resources and mixed task dispatch. Neutral completion is extracted; policy/config/assertion/gateway prompts belong to their domains, design/directory/scenario/organization context to Simulator. Keep explicit embedding ownership. |
| `organizations.go`, `organization_activation.go`, `organization_bundle_profile.go`, `organization_create.go`, `organization_definitions.go`, `organization_duplicate.go`, `organization_workspace.go` | Simulator organization profiles, private bundles, portable definitions, creation and activation. Do not treat these as production repository configuration. |
| `directory_workspace.go`, `simulator_workspace.go`, `workspace_store.go`, `trusted_source.go` | Simulator directory/workspace revisions and live directory viewer. `workspace_store.go` is Simulator artifact persistence despite its generic name; `trusted_source.go` loads active Simulator organization/container data despite its generic name. |
| `simulator.go`, `simulator_policy.go`, `scenario.go`, `scenario_check.go`, `scenario_reliability.go` | Simulator routing/security/manifest/component composition and scenario execution/placement. Destinations are Simulator application/scenario owners; production may not import them. |
| `machine_control.go`, `machine_control_scope.go`, `machine_logs.go` | Simulator controller/session/runtime-generation transport, orchestration and logs. Neutral text formatting is extracted; machine/container/workload selection is not neutral. |
| `policy_layers.go`, `test_app.go` | Simulator provisioning policy composition and test/stress/benchmark workloads. The former loads organization profiles; the latter also has workload-log HTTP adapters. Preserve CLI modes but do not place these in production repository/default initialization. |

### Production entry points and compatibility contracts

Authoritative registration remains in
[`control_service.go`](zpr-dashboard/cmd/zpr-web-dashboard/control_service.go),
[`policy_service.go`](zpr-dashboard/cmd/zpr-web-dashboard/policy_service.go),
[`assertion_runtime.go`](zpr-dashboard/cmd/zpr-web-dashboard/assertion_runtime.go)
and [`trusted_changes.go`](zpr-dashboard/cmd/zpr-web-dashboard/trusted_changes.go).
This route-family inventory includes delegated registrations; it is not a
replacement router specification.

| Surface | Current contract to preserve |
| --- | --- |
| Compatibility CLI | 20 dispatched modes; default `control-room`, `ZPR_WEB_MODE`/`-mode`, `ZPR_WEB_LISTEN`/`-listen` default `127.0.0.1:8787`. Keep all machine/workload/proxy/policy-composition flags in `main.go`, even when implementations move. |
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

**Next bounded scope:** define independent production assertion seed and policy
fixture input contracts before moving repository initialization/fixture code.
Simulator provisioning may call those contracts, not supply production defaults
through reverse profile/manifest reads. Preserve repository persistence and
provider-authorized fixture semantics; resolve contract/behavior decisions
explicitly rather than replacing them with empty success-shaped fixtures.

Remaining M1 gates: exhaustive environment/default/error and persistence
contracts for all non-production modes, full nested enrollment/gateway/review
contract mapping, cross-domain call coverage beyond the paths traced above and
installer/CI/resource dependency acceptance. The ownership map is complete for
the original 58-file set and the new production configuration file; these deeper
audit gates are not.

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
