# Control Room / Control-Service boundary — packet B

2026-10-23; base `4627e4cd99f15e1ee227f788574ab5055ee7a353`.
Scope: B01 inventory and executable C06 regression evidence, not remediation or
production sign-off. The initial packet B evidence is preserved below; the
wave 2 C05 asset remediation and current results are recorded at the end. No
Simulator runtime, Docker stack, deployment or live provider was started.
Source-packet implementation files remain unchanged.

## Inventory and replacement owners

Severity describes architectural impact, not a security vulnerability rating.
**High** blocks independent operator deployment; **Medium** is residual lab/UI
coupling; **Clear** means no reverse dependency found in the inspected seam, not
proof that arbitrary operator-supplied URLs/files cannot point at Simulator.
Paths below are relative to `zpr-dashboard/`.

| Severity / seam | Evidence and current dependency | Owner / replacement |
| --- | --- | --- |
| **High — organization/session selection** | `scripts/dashboard-stack.sh:1040-1044`: `start_control_service` falls back from `ZPR_GATEWAY_ORGANIZATION_ID` to `SIMULATION_ORGANIZATION_ID`, then reads `ACTIVE_ORGANIZATION_FILE`; the global default is Simulator's `active-organization.txt` (line 42). | Operator deployment packet: require an explicit production organization; obtain authoritative context through Policy-Service, never the selected scenario/session. |
| **High — profiles and runtime files/mounts** | Same function, lines 1047-1052, reads `$ORGANIZATIONS_DIR/<organization>.json` and `.runtime.driver` to choose `$RUNTIME_DIR/multinode/<organization>/dns/...`. Lines 1078-1079 mount all shared runtime and the multinode demo tree into Control-Service. | Operator deployment + DNS owners: explicit DNS transfer endpoint/key; mount only named operator secrets/configuration/stores. Remove profile reads and Simulator-owned paths. |
| **High — default launch lifecycle** | `start_stack` (1869 onward) starts Simulator before production services and derives organization, LDAP container/base/bind, Admin URL/key/CA and DNS setup from the manifest/active profile; `start_policy_service` (1814 onward) also selects a simulation profile and generated runtime policy. Not exercised by this packet. | Deployment/Policy-Service packet: independent service startup, operator-owned policy/configuration revisions and production bootstrap contracts. Simulator remains optional setup/workload automation, not the operator launcher. |
| **Medium — artifact/build coupling** | `start_control_room` calls `ensure_simulator_image` (421); both production containers use `$SIMULATOR_IMAGE` (466, 1116). `cmd/zpr-web-dashboard` is one `package main` containing all modes, so its build-time transitive graph includes Simulator even though extracted production packages do not. | Packaging packet: independent production artifacts/entrypoints and explicit configuration. A shared binary alone is not evidence of a runtime API call; it is not an independent production package boundary either. |
| **Medium — synthetic source-name branch** | Production `/api/snapshot` → `application.fetchSnapshot` → `inventory.Fetch(... TrustedEditorURL: trustedEditorURL)` → `inventory.go:70-71` special-cases `demo_ldap`/`rest/1` → `main.go:localLDAPEditorURL` reads `ZPR_DEMO_LDAP_EDITOR_URL`. There is no implicit URL default and the launcher does not forward this setting, but direct production startup can enable the branch. | Source/provider packet (B02/S01/C03): provider-owned registration/manager metadata, no lab-name special case. Preserve ordinary clients' actual display names. |
| **Clear — Simulator assets on production origin (C05 asset seam)** | `runControlRoom` now serves only `fs.Sub(controlRoomFiles, "static")`, an explicit production embed bundle checked against `packaging/control-room/assets.txt`. All 26 Simulator-only static files (including the trusted-source/log/world standalone lab pages and navigation) are absent on the production origin. `-tags production` also excludes them from the executable and excludes the three Simulator assistant skill artifacts. | C05 asset packet resolved this seam: default and production-build HTTP/desktop/tablet tests assert GET/HEAD absence and independent navigation. Simulator default build retains its own static registration, pages and redirects. Shared production rendering keeps independent operator endpoints; full compiled-command extraction and launch configuration remain separate gates. |
| **Clear — production routes** | `internal/controlroom/httpapi/routes.go`: login, change review, enrollment, `/api/` proxy, DNS stylesheet and static. `runControlRoom` also composes budget authority. `internal/controlservice/httpapi/routes.go`: snapshot, diagnostics, actor visas, adapter logs, DNS, policy/assistant, gateways, assertions, trusted change feeds, enrollment and budget authority. No Simulator API registration; actual process test verifies `/api/simulator/scenarios` is 404. | Existing operator/service owners; preserve explicit production route/authorization contracts. The broad proxy is not a Simulator contract. |
| **Clear — extracted imports / direct callers** | Transitive `go list -deps` for `internal/controlroom/...` and `internal/controlservice/...` contains no `internal/simulator*`. New AST guards inspect first-party transitive literals and selected command composition files for Simulator-owned functions/types/imports/configuration. | Control-Service/Control Room owners. New seams must receive ownership coverage; the guard is not a whole-program dynamic call graph. |
| **Clear — authoritative inventory and labels** | `inventory.go` calls the Admin API client, then enriches with explicit `ZPR_PLATFORM_SERVICES`, `ZPR_NODE_DISPLAY_NAMES_FILE` and configured telemetry. No default synthetic topology or `zpr.machine.*` discovery in these production packages. A real actor named `Simulator` is preserved in the regression fixture. | Visa Service owns topology/identity; operators own optional annotations/display names. Do not infer live state from synthetic scenario labels. |
| **Clear — logs / Docker discovery** | `/api/adapter-logs` → `newAdapterLogsHandler` → `adapterlogs.ReadInventory(ZPR_ADAPTER_LOG_CONFIG_FILE)`; reads explicit regular files or executes `docker inspect/logs/exec` for explicitly configured containers only. No manifest/session/workload scan or `docker ps` discovery. Missing configuration is 503, not a machine-log fallback. | Production log/provider owner. Docker access is optional configured acquisition, not proof of Simulator dependency; remove broad shared socket/mount authority in deployment follow-up. |
| **Clear — telemetry** | `diagnostics.go`, `node_denials.go`: explicit `ZPR_DIAGNOSTICS_*`, `ZPR_NODE_DENIAL_METRICS_FILE`, source maps and operator OTLP files. The node-metrics sibling fallback is relative to the diagnostics configuration, not a simulation manifest. | Operator-configured production telemetry/provider contract; report unavailable/stale honestly. |
| **Clear — policy / organization calls** | `policy_service.go` uses `ZPR_POLICY_SERVICE_URL` and explicit mTLS files via `internal/policyclient`; assertions/settings use the Policy-Service contract. The server client is separate from the launcher's profile-derived defaults above. | Policy-Service owns revisions, validation, stage/apply and authoritative readback; replace the lab launcher, not this client contract. |
| **Clear, source limitation — trusted attributes/feeds** | `assertion_runtime.go`/`assertion_sources.go` load explicit `ZPR_ASSERTION_SOURCES_FILE` or `ZPR_ASSERTION_LDAP_*`; `trusted_changes.go` uses `ZPR_TRUSTED_CHANGE_FEEDS_FILE`. Legacy LDAP runs `docker exec` on the configured container; HTTPS sources are independent. No automatic Simulator discovery in these constructors. | Source packet (B02/S02): external LDAP/provider support and stable source identity. A container-only LDAP adapter is a capability constraint, not automatically a reverse Simulator call. Implementation files were read only. |
| **Clear — normal browser requests / shared utilities** | Production `index.html` uses `/api/assertions/source` and `/api/adapter-logs`; `trusted-source.js`/`machine-logs.js` have separate Simulator branches, selected by page context/explicit endpoint. New browser checks load real production assets and navigate eleven operator pages with sources available and unavailable; no Simulator endpoint/asset/external-origin request is accepted. | UI owner keeps independent endpoints/DTOs. Shared rendering is allowed; the co-hosted Simulator entry-page seam is resolved by C05 below. |
| **Configuration gap, not a reverse API call** | `index.html` hardcodes the Log Manager link to `http://127.0.0.1:8800/`. | Operator configuration should own this production log UI destination; no automatic lab URL in production presentation. |
| **Simulator-only, permitted direction** | `simulator*.go`, `scenario*.go`, `directory_workspace.go`, `trusted_source.go`, machine/workload/session handlers and their private manifests/profiles; Simulator navigation can open an independent Control Room origin. `main` mode dispatch is not itself a production runtime call to Simulator. | Simulator packet owns its client/setup automation. Do not import these handlers/contracts into production. |

## Guards and results

- `cmd/zpr-web-dashboard/control_room_boundary_test.go`: executable transitive
  production import/literal guard, direct command caller guard, and real TLS
  Admin snapshot requests with Simulator environment truly unset, unreachable,
  malformed, or pointing to a monitored HTTP tripwire. Invalid JSON and missing
  paths replace Simulator files; a regular file in a directory setting also
  makes traversal impossible. Only the expected Admin reads are accepted;
  missing production log configuration remains unavailable. Actual
  arbitrary-file-access tracing is **not** claimed.
- `control_room_boundary_process_test.go`: starts private child processes using
  the real `runControlService` and `runControlRoom` startup functions, with
  generated mTLS credentials, no configured providers/Simulator, poisoned
  Simulator files/configuration and a monitored Simulator URL. Verifies direct
  login/static startup, real Room → Service mTLS reads, explicit unavailable
  states and no Simulator API registration. Children are killed/waited and all
  private fixtures removed. This is loopback development login, **not** full
  named-user OIDC or real-provider acceptance.
- `tests/browser/control-room-boundary.spec.mjs`: real production static assets,
  production-only mocked API contracts, desktop/tablet, available/unavailable
  sources; inspects **all** browser requests (including failed ones), checks
  navigation/directory/log requests and rejects Simulator/external requests.
  It does not mock away server-side calls: those are tested separately above.
- `scripts/check-control-room-boundary.py`: strict read-only supplementary
  launcher/inventory/shared-asset audit. **Currently exits 1: 11 references** (after C05 assets)
  across the unresolved violation groups above. No skips or expected-failure
  allowlist. C05 replaces the shared-asset check with explicit filesystem/manifest
  checks plus mutation tests. It is an explicit failing acceptance
  command, not a claim that passing focused tests close the boundary. Its
  lexical scope is the named launcher functions, legacy inventory branch and
  shared static mount; it is not a universal shell/data-flow analyzer.

Reproduce from `zpr-dashboard` (Go/Node/Playwright already installed):

```sh
# Keep compiler/test/browser scratch files within this exclusive worktree.
mkdir -p .boundary-validation
TMPDIR="$PWD/.boundary-validation" go test ./internal/controlroom/... ./internal/controlservice/... -count=1
TMPDIR="$PWD/.boundary-validation" go test ./cmd/zpr-web-dashboard -run '^(TestControl|TestProduction|TestDiagnostics|TestNode(DisplayNames|Enrichment)|TestPolicyProxy|TestAssertionMultipleLiveSources)' -count=1
TMPDIR="$PWD/.boundary-validation" npm run test:browser -- control-room-boundary.spec.mjs dashboard.spec.mjs --grep 'Control Room browser|GUI header guideline|Trusted Sources GUI' --workers=1
python3 scripts/check-control-room-boundary.py # deliberately nonzero until violations are removed
rm -rf .boundary-validation
```

Recorded results: all eight extracted production-package suites passed; focused
command tests passed; **18 Playwright tests passed** (four new boundary cases plus
existing source/header regressions). Missing local Node dependencies were restored
with the existing lockfile (`npm ci --ignore-scripts`); no manifest/lockfile change.
Mutation checks confirmed that the production dependency guard rejects an
injected Simulator configuration reference and the strict audit rejects
manifest/demo/shared-page references and a missing audited function.
No shared stack was started. The strict audit fails as documented; **C06 and the
M0/M2 production acceptance gates remain open**.

## Recommended next packets

1. **Operator deployment (C03/C06):** remove organization/profile/session and
   runtime-path fallback, separate production bootstrap/artifacts, explicit
   production DNS/Admin/policy/provider configuration and least-scope mounts.
2. **Production UI boundary (C05 assets resolved):** independent entry bundle and
   forbidden-page HTTP/browser rejection checks are now implemented; retain
   remaining source/configuration work in its owning packets.
3. **Source packet (B02/S01/S02/C03):** remove `demo_ldap` name branching through
   provider metadata; external organization LDAP and coherent source provenance.
4. **Integrated C06/V01/V13:** after remediation, require the strict audit to pass
   and test named-user startup plus real production providers with Simulator
   absent and its directories/mounts inaccessible. Do not infer this from the
   loopback/mocked-provider results here.


## Wave 2 C05 — production asset packet

Base `38c62b3f6cc3af60530d76d578d6b20b0b72851c`, exclusive
`maturing/w2-production-assets`; no shared stack or live mutation.

- Removed the unfiltered static mount reference (strict audit **12 → 11**, no
  allowlist). Remaining findings are nine launcher references and two legacy
  inventory/source-name references; this packet does not edit their owners.
- The 64-file production static manifest admits four production documents and
  their rendering dependencies, including operator login, policy browser,
  change review and budget authority. It excludes **26** Simulator-only files:
  activity, agents, organizations, scenarios, standalone trusted-source/logs,
  world, provisioning, their page-specific scripts/styles, lab directory graph,
  design assistant, redirect and Simulator navigation assets.
- `production` builds embed only the static manifest plus four production
  assistant skills; directory-seed/scenario/organization skills are not embedded.
  The package build copies only the executable and two manifests, not lab trees.
  Simulator mode fails before configuration is loaded in that build. The default
  build retains Simulator pages in its own mode; Control Room uses the explicit
  production bundle in **both** builds. This is not full command/package
  extraction: the shared Go command still has Simulator compile-time imports.
- No trusted-source static UI/browser tests, source implementation, launcher or
  lifecycle configuration, architecture plan, or system release plan changed.
  Neutral shared renderers still choose the operator endpoint/data contracts in
  Control Room; no production navigation points to excluded documents.

### Verified evidence

- All eight extracted production Go package suites and expanded production
  command/HTTP/AST/dependency guards pass. Focused `-tags production` checks,
  actual Control Room/Control-Service child-process startup and forbidden static
  URLs pass. Default Simulator static documents/redirects remain available.
- **10 desktop/tablet Playwright tests pass**: six new real-static-server cases
  across default/production artifacts and Simulator assets, plus four existing
  available/unavailable-source boundary cases. Checks include GET/HEAD absence,
  keyboard navigation, no horizontal overflow, and no reverse browser requests.
  Simulator static fixture does not start Simulator's runtime or any lab stack.
- Five strict audit mutation tests pass (clean bundle, shared filesystem,
  Simulator manifest admission, wildcard/drift, missing audit scope). Strict
  audit itself intentionally remains nonzero with **11** unresolved references.
- `gofmt`, `git diff --check`, packaging shell syntax and a real production
  package build pass. Executing that package with `-mode simulator` exits 1 with
  the production-build-unavailable error before reading lab configuration.
- Required full `make test` was run but does **not** pass in this worktree:
  command tests need absent lab manifests/policy/LDIF files and sibling Rust
  workspace dependencies; an isolated shell harness lacks
  `machine_docker_network_for_org`; observability readiness also fails. Reset
  tests reject the group-writable `/Volumes/Storage` ancestor; macOS enrollment
  IPC exceeds the Unix socket path limit with worktree-local scratch paths.
  None is hidden/skipped or modified by this packet; focused owned checks pass.

Reproduce asset checks with the commands in
[`packaging/control-room/README.md`](zpr-dashboard/packaging/control-room/README.md).
All compiler/browser scratch files were placed inside this exclusive worktree
and cleaned after verification. No push or deploy was performed.

### Shared-file overlap hunks

- `cmd/zpr-web-dashboard/main.go`: remove only the `embed` import and wildcard
  `staticFiles` declaration (moved into build-selected asset files); no flags,
  environment defaults, mode dispatch or startup configuration changed.
- `control_service.go`, `runControlRoom`: change only the first argument of
  `fs.Sub` from `staticFiles` to `controlRoomFiles`; security, proxy construction,
  lifecycle and route handlers are unchanged.
- `simulator.go`: add the build-availability rejection at the start of
  `runSimulator`; extract existing static page/redirect registration unchanged
  into `simulatorStaticHandler`. This is the only startup-adjacent hunk, and it
  does not load or alter configuration.
