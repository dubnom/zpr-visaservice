# GUI work

Active GUI backlog, reconciled on 2026-10-15 UTC after the remaining source
editor migration, Organizations Browse/File repair and Control Room route extraction.
Completed checklists and dated release evidence live in the linked records.

## Document map

- [Design guidelines](GUI%20design%20guidelines.md): durable presentation rules.
- [Completed work](GUI%20completed%20work.md): completed feature checklists and evidence.
- [Deployments](GUI%20deployments.md): dated release, verification and rollback records.
- [Organization editor plan](Organization%20editor%20plan.md): detailed creation/editing workflow.
- [Provisioning plan](../Provisioning%20plan.md): machine/adapter enrollment architecture.
- [System packaging and release plan](System%20release%20plan.md): initial
  production installer milestones and recurring real-release qualification,
  publication, rollout and recovery gates.

## Tracking conventions

Use **Planned -> In progress -> Verified -> Deployed** for each deliverable.
Verified means implementation and required checks are complete, not that it is live.
Record commit/push state separately; deployments can contain uncommitted work.
Only check an implementation task when verified. Move completed requirements and
test evidence to the completed-work archive; put rollout evidence in deployments.
Keep this file focused on open deliverables rather than test logs or release history.
Do not infer current runtime health from a historical verification entry.

## Current status

**Source reconciliation:** `d741cec` published the earlier editor/backend/
bootstrap work. The accompanying publication checkpoint includes the remaining
editor migration, Organizations Browse/File/layout repair, Control Room root
router and this documentation reconciliation. Older "not committed or pushed"
labels below describe their dated verification/deployment stage, not the new
source checkpoint. Publication does not deploy or certify additional services.

- **Organizations Browse/File and layout:** **Deployed to Simulator** on
  2026-10-15. Browse selects bundled organizations/private drafts without
  activation; File reuses New/Edit/Duplicate/Rename/Delete/Restore, with Import
  in the creation dialog and existing restrictions. Matching HTML/JavaScript
  fixes inert controls after navigation; compact headings/collapsible summaries give the graph at least
  600px at the tested 1440x900 viewport. The owning session reports 22 passing
  desktop/tablet checks and real live Browse/File/Edit clicks without JS errors.
  DNS queries/statistics were restored during rollout; the DNS adapter's
  reattachment remains blocked by its node's disconnected Visa Service
  connection. Do not treat UI deployment as network-health acceptance.
  See [completed evidence](GUI%20completed%20work.md#organizations-browsefile-and-layout-2026-10-15-utc)
  and [rollout limits](GUI%20deployments.md#organizations-browsefile-bundle-repair-2026-10-15).
- **Next refactoring slice:** Policy-Service root HTTP composition with
  request-time organization context preserved. Control Room's seven root
  registrations are locally verified; configuration/stores/lifetime stay
  command-owned. All seven gates remain open.
- **Open acceptance:** private organization draft activation and cross-document/
  compiler certification, HQ/Shenzhen Business matrix, sustained recovery
  across authentication expiry/fault injection, production/companion installation
  bundles and fresh-host acceptance. The Regional Business matrix's 106 steps
  passed; that does not close these separate gates.
- **Publication validation follow-up:** the combined editor/Organization browser
  selection reports **110 passed, 22 failed**. CodeMirror and Browse/layout
  checks pass; Organization profile controls are hidden in older source-mode
  checks and definition-source accessible labels match two elements. Keep this
  regression work open; the narrower historical 22/78-case successes below are
  not a current all-Organization-suite pass. No failing tests were patched.
  Required Go tests retain the same four known baseline groups; vet,
  formatting and whitespace checks pass.

- **Remaining source editors:** **Deployed to Control Room and Simulator at
  20:34 UTC; not committed or pushed.**
  Gateway JSON, ZPR Config TOML, Simulator Directory LDIF and Scenario JSON/YAML
  now use the same CodeMirror editor as Policy/Assertions. Preserve original
  typography/colors, source-owned diagnostics, Find/Replace, Wrap, read-only and
  size limits, explicit saves and history boundaries. Format is undoable;
  Scenario conversion still resets history at the format boundary.
  All 78 desktop/tablet CodeMirror checks pass with strict CSP and mocked APIs.
  Isolated release excludes concurrent backend/Organization UI changes.
  Actual served HTML/assets and all four migrated editors verified; backend
  services untouched. Simulator was concurrently recreated by another operation
  and verified as-is with explicit approval. No provider requests.
  See [deployment](GUI%20deployments.md#remaining-source-editors-2026-10-15-2034-utc).
  See [verification](GUI%20completed%20work.md#remaining-source-editor-migration-2026-10-15-utc).
- **Hierarchical adapter LDAP completion:** **Deployed to Control Room at
  20:06 UTC; not committed or pushed.**
  Per clarified requirement, `define foo as adapter with grea` offers only
  `greatlakes.`. Accepting it opens attributes; selecting location inserts
  exactly `greatlakes.location:`, not `device.greatlakes.location:`.
  Multi-part namespaces advance segment by segment, explicit type prefixes
  remain explicit, and each acceptance is undoable. All 52 editor browser
  checks pass with production-shaped catalog data; no Go/provider changes.
  Actual served HTML/46 assets, strict CSP, exact namespace/leaf text and both
  Undo steps verified. Configuration/mounts preserved; all other services
  unchanged. Refresh/sign in again. See
  [rollout/rollback](GUI%20deployments.md#hierarchical-adapter-ldap-completion-2026-10-15-2006-utc).
- **Token limits, production LDAP completion and Discard:** **Deployed to Control
  Room and Control-Service at 19:55 UTC; not committed or pushed.**
  Choices 900/1800/2400/3600/7200, default 2400; browser and both
  backend request validators agree, with old open-client values accepted.
  Production catalog definitions are `device.greatlakes.*`, not the bare
  namespaces used by the previous mocked check. Match short namespace input
  while inserting the full catalog name; fix current-statement boundaries.
  Discard now forces a same-record reload without changing ordinary selection
  or assertion-save behavior. All 54 selected browser checks and focused Go
  race/vet pass; full Go suite retains four known failures. No AI requests.
  Actual served HTML/46 assets and strict-CSP checks verify the increased
  choices/default, production-shaped LDAP completion in multi-statement text,
  insertion/Undo and Discard cancellation/reload/history reset. Exact container
  configuration/mounts/IDs preserved; Policy-Service and Simulator unchanged.
  Refresh/sign in again. See
  [rollout/rollback](GUI%20deployments.md#token-limits-production-ldap-completion-and-discard-2026-10-15-1955-utc).
- **Assistant compact settings and LDAP namespace completion:** **Deployed to
  Control Room at 19:36 UTC; not committed or pushed.** Remove the requested direct-edit/Undo/autosave
  disclosure sentences. Rename the collapsed section **Model and Tokens** and
  move all token usage inside it. Reproduce and repair
  `define great as adapter with greatlakes.`: namespaced attribute dots are no
  longer treated as statement terminators. Short/qualified and multi-part LDAP
  names complete with Undo; comments, strings and completed values stay silent.
  All 44 selected desktop/tablet editor/shared-assistant checks pass; editor
  diagnostics and whitespace checks pass. No Go changes or AI-provider requests.
  Actual served HTML/46 assets, strict CSP, exact disclosure, collapsed stats,
  automatic `greatlakes.` insertion/Undo and login gate verified. Room
  configuration/mounts preserved; backend services and Simulator unchanged.
  Refresh/sign in again. See
  [rollout/rollback](GUI%20deployments.md#assistant-settings-and-ldap-namespace-completion-2026-10-15-1936-utc).
- **Sole policy/assertion editor and explicit AI edit/usage results: Deployed
  to Control Room and Control-Service at 19:12 UTC; this follow-up is not
  committed or pushed.** CodeMirror starts automatically, with
  no legacy switch or editable fallback. Direct AI requests are gated on editor
  readiness; real replacements are checked against the visible document and
  source bridge. No-op lists are rejected, empty edit lists receive a neutral
  notice, and the appended "Applied as one unsaved..." sentence is removed.
  Last-request output/requested maximum and cumulative usage are distinct;
  structured-response failures retain reported usage and overruns are explicit.
  The approved synthetic request used 77/300 output tokens and did not reproduce
  the reported first-request overrun. The application cannot guarantee provider
  billing. All 38 editor browser checks and 12 selected unchanged assistant
  checks pass. Two Scenario checks fail during setup on the old
  `#scenario-organization` selector before reaching assistant code; untouched.
  Focused Go race tests/vet pass; `make test` retains the four known failures.
  Both production containers preserve their exact configuration/mounts/IDs.
  Served HTML and 46 assets match source; actual CSP, sole-editor startup,
  mock AI visible edits/Undo/Redo, usage, Format and sign-in gate pass.
  Policy-Service and Simulator were not updated. Refresh/sign in again.
  See [implementation evidence](GUI%20completed%20work.md#sole-editor-and-ai-editusage-verification-2026-10-15-utc).
  See [rollout/rollback](GUI%20deployments.md#sole-editor-and-explicit-ai-editusage-results-2026-10-15-1912-utc).
- **Policy/Assertions CodeMirror trial:** **Deployed to Control Room**
  (2026-10-15 17:30 UTC).
  An opt-in checkbox enables the locally bundled editor; switching back preserves
  unsaved text. Defaults off each reload. Save/Analyze contracts and source-owned
  diagnostics remain unchanged. An isolated sample-data
  [local preview](http://127.0.0.1:8796/#policy) is running for evaluation;
  it does not connect to live services. Control Room rollout preserves operator
  configuration and defaults the trial off; sign in again, then enable
  **CodeMirror trial** in Policy/Assertions. Simulator was not updated.
  See the [trial checks and limits](GUI%20completed%20work.md#policyassertions-codemirror-trial-2026-10-15-utc)
  and [rollout/rollback evidence](GUI%20deployments.md#control-room-codemirror-trial-2026-10-15-1730-utc).
  **Follow-up deployed at 17:47 UTC:** matching font metrics and requested AI
  line changes applied directly as one undoable unsaved edit. Control Room and
  Control-Service were updated together; existing configuration was preserved.
  Refresh/sign in again to use it. See
  [coordinated rollout](GUI%20deployments.md#control-roomservice-font-parity-and-direct-ai-edits-2026-10-15-1747-utc) and
  [follow-up verification](GUI%20completed%20work.md#codemirror-font-parity-and-direct-ai-edits-2026-10-15-utc).
  **Follow-up deployed at 17:59 UTC:** CodeMirror now uses the previous
  highlighters' token classification/colors. Analyze's missing fixture context
  was repaired by updating Policy-Service and configuring an independent
  operator fixture contract. A non-saving live Analyze evaluated 38 actors
  against 10 services (380 pairs). No Restore, policy save or runtime mutation.
  See [repair rollout](GUI%20deployments.md#editor-color-parity-and-analyze-repair-2026-10-15-1759-utc).
  **AI response repair deployed to Control-Service at 18:13 UTC:** direct edits
  now use a required structured tool payload instead of free-form JSON text.
  Incomplete/token-limited responses explicitly leave the draft unchanged.
  All 28 editor browser checks pass. Control Room was not restarted; no refresh
  is required to retry. See
  [structured-edit rollout](GUI%20deployments.md#structured-ai-editor-responses-2026-10-15-1813-utc).
  **Correction and editor fixes deployed at 18:29 UTC:** the configured model
  rejects forced tool calls; native JSON-schema output is now used and verified
  with approved synthetic no-edit and replacement requests. CodeMirror renders
  its own scroll-aware caret/selection, and Undo/Redo works from Analyze and
  other editor buttons without stealing other text fields' history. All 32
  selected browser checks pass. Refresh/sign in again and enable CodeMirror.
  See [latest rollout](GUI%20deployments.md#scroll-aware-caret-toolbar-history-and-supported-ai-schema-2026-10-15-1829-utc).
  **Follow-up deployed to Control Room at 18:38 UTC:** policy Format now uses
  one isolated undoable edit instead of resetting history; assertion Format
  shares the same isolated replacement path. The assistant proxy now allows
  its bounded context/provider request to finish instead of canceling after
  the ordinary 10-second API budget. All 34 editor checks pass. Refresh/sign
  in again to load the UI. See
  [Format/proxy rollout](GUI%20deployments.md#format-history-and-assistant-proxy-budget-2026-10-15-1838-utc).
  **AI envelope repair deployed to Control-Service at 18:45 UTC:** a synthetic
  refactor reproduced a valid JSON answer preceded by a provider `thinking`
  block, which the prior parser incorrectly rejected. Known reasoning blocks
  are now skipped while the single edit payload remains strictly validated.
  The captured synthetic response passes the actual parser. No refresh needed;
  the operator's real policy was not sent to AI during verification. See
  [envelope repair](GUI%20deployments.md#assistant-reasoning-envelope-repair-2026-10-15-1845-utc).
- **Directory/code refactoring:** independent production policy-fixture inputs
  are **Deployed to Policy-Service at 17:59 UTC** and published in `990d54a`. Policy-Service now uses
  explicit operator JSON plus live LDAP; Simulator provisioning exports that
  contract rather than being read by production. The compatibility audit now
  records all 23 modes, 40 flags, Simulator state/default paths and packaging
  owners, with new dispatcher/path regressions. The nested enrollment,
  gateway-draft and change-review route/auth/persistence audit is also
  **Verified locally**, with four new production-contract tests and the six
  audited internal-package suites passing under race. The subsequent gateway
  draft-owner extraction is now **Verified locally** and published in `990d54a`;
  publication does not expand dated deployment evidence:
  validation/storage/identity selection live in a production package with an
  independent inventory contract; neutral upstream parsing is shared without
  Simulator workload ownership. Focused race tests, vet and both Linux builds
  pass; full Go tests retain the same four baseline failures. Seven milestone
  gates remain open. The production inventory acquisition/enrichment audit is
  now **Verified locally**, with five new compatibility tests covering Admin
  TLS/decoder boundaries, health/wire shapes, enrichment ordering and the
  measured 16-request visa worker limit. No runtime code changed in this audit.
  The subsequent explicit-config Admin read-client extraction is now
  **Verified locally**, not deployed or published: production TLS/credentials/
  JSON GETs have a standard-library-only owner, with environment/error
  compatibility retained in the command. Owner/consumer race tests, vet and
  both Linux builds pass; the same four full-suite baseline failures remain.
  The sixteenth extraction now also gives snapshot DTO/acquisition and pure
  service/trusted assembly an independent production owner, with an explicit
  Admin reader and enrichment callbacks. It is **Verified locally**, not
  deployed or published. Existing consumer tests pass unchanged; new package
  guards verify concurrency, health, wire shapes and dependency boundaries.
  The seventeenth extraction adds explicit display-name and node telemetry
  enrichment to the production inventory owner, with file paths and
  catalog/query callbacks rather than provider discovery. It is
  **Verified locally**, not deployed or published. Boundary/consumer races,
  vet and both Linux builds pass; full tests retain the four baseline groups.
  The eighteenth extraction adds a production diagnostics owner for catalog/
  wire/provider contracts and explicit OTLP/OpenObserve acquisition. Operator
  configuration and authorized HTTP composition stay in the command. It is
  **Verified locally**, not deployed or published: new boundary/bounds/partial-
  signal tests and unchanged consumers pass, along with vet/both Linux builds;
  the full suite retains four baseline failure groups.
  Diagnostics result aggregation is now also **Verified locally**, not deployed
  or published: explicit provider/catalog inputs, four-query concurrency,
  source/overall states and ordered 500-log/1,000-metric budgets live in the
  production diagnostics package. HTTP validation, authorization composition,
  request deadlines and operator config remain command-owned. Per-call
  freshness defaults eliminate a shared-handler write; new exact-bound and
  concurrent-request regressions plus unchanged consumer races pass.
  Vet and both Linux builds pass; the four full-suite baseline groups remain.
  The remaining startup/operator-config and nested-route composition audit is
  now **Verified locally**, with no runtime changes. Five new tests pin root/
  delegated route bindings, server/TLS fields, startup failure ordering and
  request-time policy context; 21 editor-runner cases and focused races pass.
  Explicit-input Control-Service HTTP composition is now **Verified locally**:
  `internal/controlservice/httpapi` owns all fifteen direct and eight delegated
  registrations. Domain handlers/local checks, startup configuration, scheduler/
  store lifetime, security headers and TLS/listener behavior stay command-owned.
  Served-mux/integration and updated ownership guards report 83 passing editor
  cases; package/consumer races, vet and both Linux builds pass. Full tests retain
  the four baseline groups. No publication/deployment in this slice.
  The explicit-input operator web-security loader/request boundary is now
  **Verified locally** in `internal/operatorweb`, with independent Control Room
  and Simulator environment adapters. Existing management/direct-TLS/OIDC/
  development/login/delegation behavior is preserved; protected-file and
  synthetic IdP/factory regressions report 38 passing editor cases. Owner/
  dependency and unchanged consumer races, vet and both Linux builds pass.
  Full tests retain the four baseline groups. No publication/deployment.
  Control Room root HTTP composition is now **Verified locally** in
  `internal/controlroom/httpapi`: seven explicit auth/review/enrollment/API/
  stylesheet/static handlers preserve precedence and management/local wrappers.
  Configuration, stores, embedded files and lifetime remain command-owned.
  Served-mux/real-boundary regressions report 91 passing editor-runner cases;
  owner/consumer races, vet and both Linux builds pass. Full tests retain the
  four baseline groups. No publication/deployment in this slice.
  Next is Policy-Service root HTTP composition with request-time context intact.
  Packaging acceptance and all seven milestone gates remain open. See the
  [Control Room HTTP composition owner](REDIRECTORY.md#m2-explicit-handler-control-room-root-http-composition-2026-10-15),
  [operator web-security owner](REDIRECTORY.md#m2-explicit-input-operator-web-security-owner-2026-10-15),
  [Control-Service HTTP composition owner](REDIRECTORY.md#m2-explicit-handler-control-service-http-composition-2026-10-15),
  [startup and HTTP composition audit](REDIRECTORY.md#m1-production-startup-and-http-composition-audit-2026-10-15),
  [diagnostics aggregation boundary](REDIRECTORY.md#production-diagnostics-aggregation-boundary-2026-10-15),
  [diagnostics-acquisition extraction](REDIRECTORY.md#eighteenth-extraction-production-diagnostics-acquisition-owner-2026-10-15),
  [node-enrichment extraction](REDIRECTORY.md#seventeenth-extraction-explicit-node-inventory-enrichment-2026-10-15),
  [snapshot-owner extraction](REDIRECTORY.md#sixteenth-extraction-production-snapshot-inventory-owner-2026-10-15),
  [Admin read-owner extraction](REDIRECTORY.md#fifteenth-extraction-explicit-config-production-admin-read-client-2026-10-15),
  [inventory contract audit](REDIRECTORY.md#m1-production-inventory-acquisition-and-enrichment-audit-2026-10-15),
  [gateway-owner extraction](REDIRECTORY.md#fourteenth-extraction-production-gateway-draft-owner-2026-10-15),
  [M1 compatibility audit](REDIRECTORY.md#m1-compatibility-mode-and-packaging-audit-2026-10-15) and
  [nested production contract audit](REDIRECTORY.md#m1-nested-production-contracts-audit-2026-10-15), plus the
  [M3 fixture-boundary record](REDIRECTORY.md#m3-input-slice-independent-production-policy-fixtures-2026-10-15).
- **Publication checkpoint:** pending Restore/runtime repairs and Scenario
  browsing source are included in this 2026-10-15 publication. Historical
  uncommitted notes describe their dated releases, not a new deployment.
  See [publication checks and exceptions](GUI%20completed%20work.md#runtime-repairs-and-scenario-browser-publication-2026-10-15-utc).
- **Regional Business repaired and live-verified:** node session pings now
  trigger signed reauthentication before expiry; current Visa runtime includes
  pending-visa pull recovery. Business denial checks use Simulator's configured
  independent operator connection. Regional acceptance passed all 106 steps and
  42 pairs (six allowed, 36 evidenced denials) with complete cleanup.
  Simulator and Visa runtime fixes are deployed; policies/directory/business
  state were preserved without Restore. HQ/Shenzhen acceptance and sustained
  recovery beyond the normal authentication lifetime remain pending.
  See the [deployment evidence](GUI%20deployments.md#regional-business-runtime-recovery-2026-10-15-15041512-utc).
- **Publication:** `e1b24ab` is the previous publication checkpoint. The
  this source checkpoint includes the 2026-10-15 reconciliation and pending work;
  dated "not committed or pushed" notes remain historical evidence. Publication
  does not deploy the discovery CLI, new Results navigation or backend services.
- **Deployed:** Simulator `zpr-simulator:20261015-scenario-browser` at
  2026-10-15 16:24 UTC, following the 15:09 UTC Business denial repair.
  Browse.../File..., one selected scenario, full-width tracks and compact
  running-step details are live. Configuration/mounts/ports were preserved;
  DNS relays were restored. Restart cleared in-memory scenario history.
  Control Room subsequently received the CodeMirror trial at 2026-10-15
  17:30 UTC, followed by the coordinated Control Room/Control-Service font/AI
  update at 17:47 UTC. Policy-Service was not restarted. See
  [deployment scope and verification](GUI%20deployments.md#scenario-browser-deployment-2026-10-15-1624-utc).
- **Restore repair deployed:** Great Lakes Restore failed its assertion-settings
  verification and the previous runtime was restored and verified. A read-only
  probe after rollback found the correct organization/base DN but empty assertion
  source. Independent seed configuration and deployed-image retention are now
  installed in Policy-Service (01:53 UTC); all 28 live LDAP assertions pass.
  A subsequent Restore exposed the separate-stop retention gap; the 02:03 UTC
  follow-up persists the image before removal. Real separate stop/context restart
  now retains the repaired image and all 28 assertions still pass.
  The user's subsequent Restore completed successfully before the 02:07 UTC
  progress-modal deployment. Regional Business acceptance subsequently passed;
  HQ/Shenzhen acceptance remains pending.
- **Completed GUI polish:** shared Help styling, Trusted Sources spacing/counts,
  scenario Browse/File selection, organization viewer File actions and consolidated
  New/Import/Duplicate, Activity cleanup, and Security Review headers.
  Control Room retains its live Updates (24h) feed and independent service
  contracts. See the [completed checklist](GUI%20completed%20work.md#control-room-and-simulator-gui-polish).
- **Finance Gateway repaired and live-verified:** activation reconnects the
  Gateway after effective policy installation and checks service registration
  before DNS publication. DNS base-zone rebuilds discard the previous update
  journal. Two complete live runs pass all 12 steps, including health, Google
  allowed, Apple denied and cleanup. Regional Business acceptance subsequently
  passed; HQ/Shenzhen remains pending;
  see the [deployment evidence](GUI%20deployments.md#finance-gateway-activation-repair-2026-10-15-utc).
- **Verification limits:** Current native core Rust tests and shared socket
  utility tests pass. The full dashboard `make test` still has four known
  failing groups. Focused race checks and 16 selected desktop/tablet browser
  checks pass using the existing container runner; no full browser-suite pass
  is claimed. Control Room's
  post-restart browser check was signed out, not an authenticated live-data
  check. See the [publication verification](GUI%20completed%20work.md#publication-reconciliation-2026-10-15-utc).
  These records do not assert current runtime health.

## In progress

Scenario gateway response capture is **Deployed (Finance live acceptance passed)**:
a Results section
shows HTTP status, request/final URL, elapsed time, bytes read, redacted bounded
headers and inert raw HTML/JSON/text (base64 for non-UTF-8). Observations survive
failed expectations and partial-body errors; empty expectations allow
observation-only fetches within existing destination restrictions. Six
desktop/tablet browser checks, focused Go race tests, vet and the embedded build
pass; the required full Go suite retains four unrelated failures. Results
currently belong to the current run only; durable archives and other result
types remain future work. Simulator and controller binaries were updated from an
isolated Results-only release on 2026-10-15 00:49 UTC; production services were
not restarted. The later business-service executable resolved the observed
manifest-unavailable error. After the Gateway activation/DNS repair, two
complete Finance scenario runs verified Google HTTPS 200 and Apple HTTP 403
with captured results; this does not certify other scenarios or the Business
matrices.
The isolated full suite has the same 13 failures as its clean base, including
external-runtime/seed-dependent failures. The subsequent Runner/Results tabs are
verified locally, not separately deployed. See the
[deployment record](GUI%20deployments.md#scenario-gateway-results-deployment-2026-10-15-0049-utc) and
[verification evidence](GUI%20completed%20work.md#scenario-gateway-response-results).

Migration network discovery is **In progress**; its standalone CLI slice is
**Verified** locally. It has explicit authorized TCP scope, optional conservative
protocol checks, versioned JSON export and imported adapter-listener enrichment.
Ping and reverse names now default on without gating TCP checks; JSON config
files supply options with explicit CLI overrides and command-line-only
authorization. Ping requires the macOS/Linux utility (disable it elsewhere).
Config targets are now a string list of IPs, CIDRs or inclusive dash ranges
(including IPv4 final-octet shorthand). Host and CIDR-size caps are removed:
compact scopes are deduplicated and iterated lazily under the existing
rate/worker/time limits. Version-2 reports/previews store exact planned counts
and full compact scopes, with host records only for reached/imported targets.
Focused race tests, vet, native build, Linux/Windows cross-builds and the
documented no-network preview pass. The follow-up removes the overall deadline
(former `duration` options are rejected) while preserving per-check timeouts and
Ctrl+C partial export, and adds stderr progress with terminal-width-aware
rendering and distinct client/host/open-endpoint counts. Current engine/CLI race
checks pass. This extension was verified against
IPv4/IPv6 loopback only; it does not repeat the earlier authorized subnet scan.
Control Room scanner selection, authenticated live adapter inventory and
reviewed migration drafts remain future work. Discovery must remain independent
of Simulator, and remote observations must not be conflated with host-reported
listeners or port-number service guesses. See the
[CLI contract](zpr-dashboard/cmd/zpr-discover/README.md) and
[verification evidence](GUI%20completed%20work.md#migration-discovery-cli).
The original slice was published in `e1b24ab`; follow-up progress/deadline changes
are included in the current publication. This standalone CLI was not deployed by
the dashboard-only releases.

Great Lakes business simulations are **Implemented, verified and deployed;
live acceptance blocked**. Seven stateful providers are configured for dedicated
`machine-13`, with complete department/site policy grants, directory groups,
provider identities and two 42-pair scenarios (84 total: 13 allow, 71 deny).
Denials require fresh tuple-specific Visa Service evidence, not timeouts.
Writable state is isolated by organization/generation/machine. Source tests,
race checks, runtime plus five standalone catalog compiler checks and native/
Linux builds pass. The software and controller prebuilds are installed, but the
failed Restore did not complete provisioning; neither matrix has run live.
See [completed implementation](GUI%20completed%20work.md#great-lakes-business-simulations-and-restore-feedback).

Simulator organization definition/editing is **In progress**.
The draft/source-editor slices are **Deployed to Simulator** (2026-10-11 20:18 UTC) and add existing-organization profile drafts, name/description and
JSON editing, validation, immutable history and AI before/after review with
guarded local Undo/Redo. Saving does not alter source files, linked documents or
runtime. Bundled-definition **Duplicate -> review -> create private draft ->
AI edit -> save -> validate** is also deployed. Copies are stored as
unpublished coherent bundle snapshots, separately from runnable organizations.
Passwords/certificates and live/session/enrollment/run state are not copied.
Source base DN, desired addresses and identifiers remain explicit review items.
Source-read/target-create scopes, stale previews, concurrent IDs, persistent
creation receipts and lost-response reconciliation are covered.
**New/Import -> review -> create private incomplete draft -> AI edit/save ->
validate gaps**, plus portable saved-definition export, are deployed.
Missing sections do not fabricate runtime configuration or block draft saves;
malformed supplied configuration, unsafe definitions and detected credentials
are rejected. Creation receipts reconcile uncertain responses and concurrent IDs.
Deployed **Edit supporting definitions** provides profile/document selection,
source editing, add/remove and typed paths/kinds, atomic whole-bundle saves,
coherent history and reviewed cross-document AI with guarded Undo/Redo.
Private profile history opens the matching complete bundle. Conflicting or
uncertain saves retain local edits; explicit reload reconciles persisted state.
AI discloses the full unsaved fictional bundle and respects bounded context;
no creation receipts or runtime data are sent.
Structured section editors (Sites/Topology, Directory, Services/Workloads) with
bi-directional Raw/Form synchronization, live topology cards, and live
`<ldap-org-graph>` preview are **Verified** locally.
Full reference/compiler checks, section-only AI context and activation review
are next; production administration remains separate.
The 2026-10-14 22:35 UTC Simulator worktree deployment includes the structured
editors and toolbar/mode alignment; source is committed and pushed. Earlier
targeted browser evidence remains recorded, but no full browser-suite rerun or
runtime activation is claimed for this release. See
[deployment evidence](GUI%20deployments.md) and
[the execution plan](Organization%20editor%20plan.md).

## Next

The open requirements below retain their original scope.
No new priority or implementation approval is implied for backend rollouts.

### System packaging and releases
- **Companion LDAP/OIDC slice (2026-10-15):** added opt-in OpenLDAP/Dex
  configuration generation with explicit matching HTTPS origins, per-install
  lab-user passwords/groups and separate one-time initializer/startup scripts
  that never reseed existing state. Relative-file identity inventory does not
  invent operator subjects or grants. Isolated real-provider test verifies
  LDAPS trust, restricted reader ACLs, PKCE/signed OIDC login, group/subject
  mapping and stable identities across preserved-state LDAP/Dex restart without
  Simulator. Focused race/provider/BIND tests, vet and Linux amd64 build pass;
  required full Go suite retains the same four recorded dashboard failures.
  Existing runtime stacks unchanged; qualified images,
  first-admin grants, trusted attribute mappings and complete network remain
  open. See [identity packaging](zpr-dashboard/packaging/simulator/README.md#ldap-backed-oidc-identity-configuration).
- **Companion independent identity Compose (2026-10-15):** approved same-server,
  separate-stack topology. Added opt-in generated immutable-image LDAP/Dex
  stack, private-IP publication, invoking UID/GID, isolated persistent state
  and explicit networkless initialization. Generated Compose passed real
  authenticated LDAPS readiness, PKCE/OIDC login and container recreation with
  stable subjects, without Simulator, using isolated arm64 engineering images.
  Ubuntu amd64 qualification, real Room grants and complete paired-package
  acceptance remain open. See [startup/recovery instructions](zpr-dashboard/packaging/simulator/README.md#same-host-independent-identity-stack).
- **Existing bootstrap DNS default (2026-10-15):** decoupled underlay record
  requests from protected ZPR DNS generation. Existing network DNS is the default
  for company and lab setup/login; companion bootstrap BIND now requires explicit
  `-host-bootstrap-dns`. Underlay-only, protected-only and combined modes produce
  only their selected files/references. No resolver changes, automatic fallback
  or mDNS/`.local` support. Focused race tests, isolated BIND configuration
  loading, vet and Linux amd64 build pass; full Go suite retains the same four
  recorded dashboard failures. No running service changed. See
  [DNS configuration modes](zpr-dashboard/packaging/simulator/README.md#separate-bootstrap-and-protected-zpr-dns).
- **Combined network / DNS rendering (2026-10-15):** user clarified that the
  two packages together must demonstrate real ZPR admission and allowed/denied
  traffic, not merely infrastructure. Approved OpenLDAP/Dex and protected BIND9
  behind a real ZPR adapter, with separate companion bootstrap DNS. Added
  explicit optional bootstrap/Room/ZPR addresses and client prefix to the
  [lab bootstrap](zpr-dashboard/packaging/simulator/README.md#separate-bootstrap-and-protected-zpr-dns):
  separate BIND configurations/zones, independent publisher/viewer TSIG keys,
  Visa update and least-privilege policy fragments. No applied configuration or
  daemon startup. Focused race/vet/Linux build pass, plus isolated BIND 9.20.29
  arm64 configuration/zone loading; full Go suite retains the same four
  recorded failures. Real adapter/installer/policy wiring and working-network
  acceptance remain open; running validators is not traffic evidence.
- **Companion trust-bootstrap slice (2026-10-15):** added standalone
  [`zpr-lab-bootstrap`](zpr-dashboard/packaging/simulator/README.md) to create
  unique lab CA and Room/Simulator/LDAP/OIDC server identities plus independent
  LDAP/OIDC service-secret files in a new protected directory. Existing output
  is never overwritten; a versioned relative-file inventory is written last.
  Focused race tests, vet and Linux amd64 build pass; required full Go suite
  retains the same four recorded dashboard failure groups. Provider rendering,
  accounts/client registration, DNS, installer handoff, resumable recovery and
  the installable companion bundle remain open. No runtime change/deployment.
- **Simulator companion scope approved (2026-10-15):** provide a separately
  installable Simulator package including lab LDAP, OIDC and DNS. The same real
  ZPR package consumes their explicit standard integration inputs; production
  keeps company-provided services and no dependency on Simulator. Supporting
  services must survive Simulator application shutdown. Define unique lab
  credentials/trust, protected handoff, isolated state/reset/removal and exact
  release compatibility; qualify paired lab operation and dependency outages
  without substituting it for independent company acceptance. No companion
  bundle exists yet. See the
  [companion deliverable and implementation sequence](System%20release%20plan.md#simulator-companion-package).
- **Enrollment ownership slice (2026-10-15):** added an opt-in separate mTLS
  administration listener to the standalone Enrollment Service, sharing its own
  device registry within one process. Control-Service can relay existing signed
  operator routes without opening a registry; mixed remote/local configuration
  fails. Enrollment verifies the original Room assertion plus its independently
  pinned Control-Service transport and grants/replay ledger. This is not the
  full proposed lifecycle API. BAS still opens its configured registry; that
  remaining ownership boundary, bootstrap and endpoint-delivery/package work
  remain open. Full enrollment/delegation race suites, focused relay/audience/
  management regressions, vet and Linux amd64 Enrollment Service/dashboard builds
  pass. Required `make test` retains the same four recorded dashboard failures.
  No running service changed. See
  [configuration and limits](zpr-dashboard/README.md#enrollment-owned-administration-api).
- **Next test deliverable (2026-10-15):** user selected a complete from-scratch
  company installation package, not a Control Room-only image or manual
  source-run test. Package all required ZPR services; company supplies its
  existing OIDC, directory and DNS. Require a versioned loadable bundle,
  install/verify/recovery instructions and fresh-Ubuntu evidence including
  first endpoint admission and allowed/denied traffic without Simulator.
  No such package is available yet: enrollment ownership/API, supervised node,
  trust/bootstrap, image/network/rendering and endpoint-delivery work remain.
  See [package handoff and blockers](System%20release%20plan.md#next-operator-test-deliverable).
- **Management HTTPS slice (2026-10-15):** approved direct HTTPS on a company
  management IP, corporate network/VPN only, not a reverse proxy or public
  endpoint. Added explicit private-IP runtime opt-in with required scoped
  company OIDC and supplied TLS, exact listener/Host/Origin/callback checks and
  forwarded-header rejection. API/DNS asset access retains named permissions;
  existing loopback and Simulator behavior is unchanged. Local verification
  covers real hostname-verified TLS/OIDC/PKCE, CSRF/scopes/logout and independent
  mTLS backend forwarding with Simulator unavailable. A total of 238 selected
  checks, focused race tests, vet and Linux amd64 validator/dashboard builds pass
  in the final shared worktree; full `make test` retains the same four failures.
  An intervening unrelated gateway-refactor compile failure resolved before the
  final run; isolated source-HEAD verification also matches its clean baseline.
  Firewall/routing, actual
  Ubuntu and container-network/installer integration remain open; no deployment.
  See [management HTTPS](zpr-dashboard/README.md#direct-management-https).
- **Offline credential slice (2026-10-15):** added opt-in `-check-files` to the
  management-bootstrap validator, reusing runtime operator-file protection.
  Checks local TLS key/hostname/validity/supplied chain and OIDC secret/CA files
  without network access or writes. A total of 158 selected configuration/file/
  CLI and existing operator regressions pass; focused race tests, vet and Linux
  amd64 validator/dashboard builds pass.
  Full Go suite retains the same four recorded failures. Company/browser trust,
  live IdP, installer integration and mount access remain open. Management
  HTTPS runtime opt-in was subsequently added above.
  No runtime change or deployment. See
  [offline checks and limits](zpr-dashboard/packaging/system/README.md#offline-credential-file-checks).
- **Configuration slice (2026-10-15):** approved company-provided OIDC and
  company-supplied/renewed TLS files. Added a strict, read-only company
  management-bootstrap validator and example, reusing operator OIDC validation.
  Ninety-three selected checks, focused race tests, vet and Linux amd64 build
  pass; full Go suite retains four recorded failures. Offline file checks were
  subsequently added above; trust/liveness checks and rendering remain
  unimplemented. Management HTTPS runtime opt-in was also added above. See the
  [configuration contract](zpr-dashboard/packaging/system/README.md#company-management-bootstrap-configuration).
- **In progress (2026-10-15):** approved Ubuntu 24.04 LTS x86-64 dedicated
  server/VM, Docker Engine/Compose v2 appliance with signed installer, and
  company-hostname remote HTTPS. The
  [initial production inventory and host preflight](zpr-dashboard/packaging/system/README.md)
  identify production image/bootstrap/access gaps. Nineteen isolated preflight
  regressions pass; real Ubuntu acceptance, resource/runtime qualification and
  broader deployment decisions remain open. No installer or release is ready;
  no runtime was changed.
- [ ] Approve the initial deployment/support decisions and owners in the
  [system release plan](System%20release%20plan.md#decisions-required-before-implementation).
- [ ] Complete the production packaging/bootstrap, endpoint admission,
  operations and release-automation milestones. Qualify every real system
  release through the plan's repeatable gates; source publication and local
  component deployments are not whole-system certification.

### Logs and diagnostics
- [x] Repair Great Lakes assertion seeding/verification through independent
  production configuration and retain the deployed image across context
  restarts. Deployed 01:53 UTC; 28 live assertions pass, rollback retained.
- [ ] After the successful Restore, run both Great Lakes Business
  scenarios and verify all 84 expected decisions with actual forwarding and fresh
  denial evidence. Do not mark software deployment as live policy acceptance.
- [ ] Retain organization operation outcomes across Simulator restart; current
  progress/result feedback is in memory and local logs survive independently.
- [ ] Complete the remaining Trusted Sources viewer and metrics/freshness/error detail placement before retiring the aggregate Diagnostics page. The Services selector and shared log viewer are already deployed; see [completed Services work](GUI%20completed%20work.md#services-selector-and-standard-log-display).

### Gateways
- [ ] Deploy the committed forwarding/consolidation changes through a separately reviewed rollout; preserve production settings and verify the actual gateway binary and protocol behavior, not just UI assets.
- [ ] Unify deployed Gateway runtime configuration with the editor contract and migrate fixed-upstream callers before retiring that mode. Define enforcement capabilities for opaque HTTPS tunnels; do not imply draft methods/paths are enforced there.
- [ ] Improve individual path-prefix management and review wildcard requirements against draft validation and runtime matching before adding support. The live web proxy's host patterns do not imply wildcard support in saved gateway drafts.
- [ ] Add verified external-network policy metadata, gateway runtime health, and explicit reviewed activation/rollback after the production provisioning contract is implemented.

### Provisioning
- [ ] **Mac pilot first:** provision one Apple Silicon Mac (macOS 13+) through
  the selected Great Lakes invitation to live ZPR connectivity. The user can
  operate the laptop and authorize administrator prompts. Complete trusted
  credential issuance, enrollment-key-preserving runtime handoff, adapter
  admission, allowed/denied traffic checks, and route/DNS rollback verification.
  Do not consume the invitation until issuance and delivery can complete; no
  enrollment code belongs in this tracker. See the
  [Mac pilot blockers](../Provisioning%20plan.md#first-release-decisions-and-progress).
  The selected handoff uses a signed root helper, one-time local IPC, and
  direct System Keychain import of the separate runtime key after one admin
  authorization. Private enrollment-backed BAS implementation is local only;
  signed runtime-key import approval delivery is implemented locally with
  pinned Ed25519 verification and native-only receipt retention. Adapter
  configuration delivery, helper/runtime integration, and live rollout remain
  open. The BAS client ID/subject derives from inventory asset ID.
- [ ] After the Mac pilot passes, certify native Windows install/browser launch,
  DPAPI/ACL and two-user isolation; sign and deliver releases. No Windows
  adapter/driver/service or credential issuance is included in the enrollment
  installer.
- [ ] Certify Mac Finder/Terminal/browser permissions, Developer ID signing/
  notarization and supported OS versions; complete full privileged traffic/lifecycle
  tests. This remains broader release certification after the single-Mac pilot.
- [ ] **Deferred security issue:** prevent invitation creators from approving
  their own device requests before multi-operator production use. The initial
  single-operator setup currently allows this; named-user authorization,
  revision/key checks and audit remain required. Enforce separation in the
  Enrollment Service transaction, test it, and keep any future single-operator
  exception explicit and audited. See the [security hardening tracker](zpr-dashboard/SECURITY_HARDENING.md#p1-enrollment-approval-separation-of-duties).

## Blocked / needs a decision

- Gateway forwarding rollout requires separate review and actual binary/protocol verification; saved drafts do not imply runtime enforcement.
- Gateway activation depends on the independent production provisioning contract.
- Native platform certification needs the relevant OS environments and release signing; local/ad-hoc builds are not certified releases.
- Self-approval is a deferred risk accepted for the initial single-operator deployment; do not expand to multi-operator production use until the security-hardening item is verified.

## Larger planned work

- [ ] Multi-site BIND 9 DNS resilience for Great Lakes: plan DNS instances in
  Milwaukee, Shenzhen and Tijuana, with one primary and authoritative
  secondaries for internal ZPR zones, authenticated AXFR/IXFR transfers and
  NOTIFY, and site-local recursive resolution with multiple client resolver
  addresses. Define independent production configuration and ZPR policy for
  DNS queries and zone transfers; do not depend on Simulator. Verify zone
  update propagation, actual client failover, primary/site outages, secondary
  zone expiry and recovery. BIND configuration and recursive caches are not
  replicated automatically. Anycast is an optional separate routing/health
  design decision, not assumed in this task. Track only; no deployment yet.
- [ ] Organization creation/editing: Duplicate -> edit draft -> validate -> review activation first. The [detailed plan](Organization%20editor%20plan.md) owns the individual requirements; production administration remains independent of Simulator.

## Scenarios
- [x] Elide detailed text for steps while running; successful output is available under Details, while failures remain visible.
- [x] Replace the folder/card list with Browse... and File..., one selected scenario, and full-width tracks.

Deployed to Simulator on 2026-10-15 at 16:24 UTC. See
[Scenario browsing and compact running steps](GUI%20completed%20work.md#scenario-browsing-and-compact-running-steps-2026-10-15-utc)
for coverage and existing regression-suite limitations, and the
[deployment record](GUI%20deployments.md#scenario-browser-deployment-2026-10-15-1624-utc)
for live checks and rollback.