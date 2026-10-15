# Features Added Since the Upstream Fork

**Snapshot:** 2026-10-15
**Baseline:** upstream fork point `abb1acd` (2026-09-03; historically recorded
as `origin/main`, not the fork's current moving branch)

This is a capability overview of the work added in this fork after its upstream
baseline. It is intentionally not a commit log. The existing Visa Service,
policy evaluator, compiler, and wire formats remain the foundation; the work
below adds service integrations, operator tools, simulation workflows, and
policy-management capabilities around them. Implementation and security
contracts remain authoritative where this overview is shorter.

## Latest Maintained Additions

- Replaced all six source-editing surfaces with the bundled CodeMirror editor:
  Policy/Assertions, Gateway JSON, ZPR Config TOML, Simulator Directory LDIF
  and Scenario JSON/YAML. Preserve source-owned diagnostics, typography and
  language colors, Find/Replace, Wrap, explicit saves and revision boundaries.
  Format participates in Undo/Redo; Scenario format conversion remains a
  separate history boundary. Loading failures are explicit, not a silent
  return to legacy editing.
- Added guarded structured AI edits, visible-document/source checks, no-op
  rejection and distinct last-request/cumulative token usage. Shared choices
  are 900/1800/2400/3600/7200 with default 2400; legacy open-client values remain
  accepted. Model limits are requested, not billing guarantees. Compact
  settings, hierarchical LDAP namespace completion and same-record Discard
  preserve unsaved-edit and failure behavior.
- Added a Simulator Organizations **Browse...** dialog and **File...** menu
  for New, Edit, Duplicate, Rename, Delete and Restore, with permission/state
  restrictions; Import remains in the creation dialog. Browsing does not
  activate an organization. Fixed matching HTML/JavaScript deployment and
  navigation rebinding; compact summaries give
  the directory graph more screen space. Private drafts remain non-runnable.
- Added private New/Import/Duplicate organization bundles, immutable coherent
  profile/supporting-document revisions, reviewed cross-document AI insertion,
  structured Sites/Directory/Services editors and separate validation/history.
  Full cross-document/compiler certification, section-only AI context and
  reviewed private-draft activation remain unfinished.
- Added explicit production owners for Admin API reads, inventory/enrichment,
  diagnostics acquisition/aggregation, Control-Service routing, operator web
  TLS/OIDC boundaries and Control Room root routing. Production inputs are
  explicit; Simulator may exercise services but is not their configuration or
  availability dependency. Application/configuration/lifecycle separation is
  still in progress; all seven reorganization gates remain open.
- Added production independent assertion seed/test-fixture contracts and an
  opt-in Enrollment-process mTLS administration relay with independent signed
  named-user verification. This is not the full enrollment lifecycle/sole-writer
  API; the standalone BAS registry ownership gap remains.
- Added company-installation and Simulator-companion package contracts plus
  lab bootstrap generation for OpenLDAP/Dex and independent underlay/protected
  DNS selections. Neither complete downloadable bundle nor fresh-Ubuntu
  installation/network/recovery acceptance is available yet. Production does
  not receive demo infrastructure defaults.

Source publication and deployment are separate. The editor migration and
Organizations repair have dated local deployment evidence; Control Room root
routing is locally verified, not deployed by its extraction. Publication does
not synchronize the running components. See the [completed-work record](GUI%20completed%20work.md),
[deployment ledger](GUI%20deployments.md), [reorganization plan](REDIRECTORY.md)
and [system release plan](System%20release%20plan.md) for exact scope, checks,
remaining gaps and rollback evidence.

Current combined-workspace publication checks report 110 passing and 22 failing
selected browser cases (Organization profile/definition source controls/labels);
the CodeMirror and Browse/layout cases pass. Full Go tests retain four known
failure groups. These capabilities are not a full-suite or release-readiness
claim.

## Control Room

- Added a browser-based, read-only operations view alongside the existing CLI
  dashboard. It presents actors, services, network topology, current visas,
  denials, and adapter information with interactive inspectors and maps.
- Added live topology updates that preserve the position of existing nodes,
  distinguish arrivals and removals, show gateway network relationships, and
  use stable service-type colors across maps and tables.
- Added complete per-adapter visa inspection, DNS-aware address labels, and
  refresh behavior that retains the last good data when an upstream read fails.
- Added forward/reverse visa pairing in tables and Map visa lists, animated
  route highlighting, and node management/per-fastpath counters with explicit
  unavailable or stale telemetry states.
- Added a read-only Security Review that groups denial and health evidence,
  highlights changes against a browser-local inventory baseline, supports
  dismissal and restoration, and links IP evidence to DNS names. Findings are
  triage signals, not automatic incident determinations or traffic controls.
- Added an aggregate DNS-probing review finding based on BIND counter deltas:
  at least 30 requests, 20 NXDOMAIN responses, and a 60% NXDOMAIN ratio within
  60 seconds. It is unattributed, review severity only, sampled while Security
  Review is active, and does not raise the high-priority navigation indicator.
- Added a draft-only Gateways view. It lists installed ZPL Gateway services,
  validates destination/path/method/timeout/response-size configuration through
  Control-Service, and saves organization-scoped revisions with concurrency
  checks. It does not provision adapters or activate runtime changes; external
  network classification is policy-owned and not fully exposed by the current
  Admin API.
- Added sortable Activity tables for recent visas and denials, plus counted
  Control Room status tabs; summary metrics remain on Map. Map updates animate
  retained topology components and Fit centers the rendered bounds while
  respecting reduced-motion preferences.
- Added responsive, independently collapsible navigation for Control Room and
  Simulator, plus shared log panels for adapter, controller, application, and
  service output. Log panels retain the last bounded tail on disconnect and
  visibly change to a disconnected state.
- Added direct HTTPS OpenID Connect operator sign-in, named identity and scope
  display, explicit session failures, and CSRF-protected sign-out. The local
  Control Room deployment uses a real Dex provider and dedicated development
  CA. Simulator has independent optional HTTPS/OIDC settings; production does
  not read them. Remote private-management HTTPS configuration and validation
  are implemented, but this is not a certified remote company installation.

## Enrollment And Platform Setup

- Added invitation-backed enrollment primitives: a persistent registry,
  private Control-Service administration APIs, a separate signed device
  challenge/proof HTTPS service, an enrollment client, and a loopback browser
  setup wizard. The device service is opt-in and separate from Control-Service
  administration. Approval does not issue credentials or establish connectivity.
- Added independently authorized named-user delegation from Control Room to
  Control-Service over mutual TLS, exact issuer/subject grants, organization
  scopes, persistent replay protection, and named audit records. The shared
  service certificate alone is not administrator authority.
- Added approved catalog selections, organization-scoped paginated registry
  browsing and fresh request details. Invitation creation requires explicit
  backend opt-in (default off), independent read/create grants, and CSRF
  confirmation. One-time codes are not stored in browser storage, emailed, or
  copied automatically; uncertain responses require reconciliation, not retries.
- Added invited/pending request cancellation by independently authorized named
  administrators, with a reason, fresh revision/key-fingerprint confirmation,
  atomic backend checks and persistent audit. Lost-response recovery requires
  fresh readback and explicit acknowledgement, never automatic resubmission.
  GUI approval/rejection remains unimplemented.
- Added Windows 11 x64 per-user enrollment wizard support and an unsigned NSIS
  setup EXE, without elevation or an adapter driver/service. Identity state uses
  current-user DPAPI and private ACLs; uninstall preserves keys/configuration.
  Cross-build, test compilation and installer payload checks pass; native
  install/launch, DPAPI/ACL, two-user isolation and signed-release certification
  remain outstanding.
- Added an Apple Silicon per-user enrollment app/DMG with a macOS 13.0 floor,
  Terminal/default-browser launch, and required native Keychain storage with
  no plaintext fallback. Identity/metadata stay in the user's default Keychain;
  disk state contains an owner-private reference. Isolated temporary-Keychain
  tests and packaged startup pass; the image is ad-hoc signed, not notarized.
- Built and tested the native Mac adapter, hardened socket/address handling,
  and verified an explicitly administrator-approved isolated utun smoke:
  two fresh interfaces, MTU changes, exact IPv6 /128 aliases, duplicate
  add/remove and descriptor-close teardown. Existing interfaces, default routes
  and DNS were unchanged. This is interface-lifecycle validation, not packet
  forwarding or enrollment-to-adapter certification.
- Installer packages are enrollment-only. Clean-machine desktop certification,
  Mac Developer ID signing/notarization, full privileged traffic lifecycle,
  credential issuance and secure credential-to-runtime handoff remain pending;
  software-key protection is not hardware-backed attestation.

## Policy And Trusted Data

- Added a versioned Policy Repository service with categorized records,
  append-only revisions, optimistic concurrency checks, and private SQLite
  storage. Control Room reaches it through service APIs rather than owning its
  storage or simulator organization logic.
- Added Policy Studio editing support: ZPL completions, LDAP attribute
  discovery, formatting, compiler diagnostics, source-aware test results, and
  a standalone read-only policy/assertion browser.
- Added a Policy-page editor blueprint with shared JavaScript surface, menu,
  identity, History and status helpers for Gateways, ZPR Config, Simulator
  Directory and Scenario. History sits beside the file name; separate version
  labels are removed. Unnamed drafts show **Untitled**, which cannot be saved
  as a name. ZPR Config asks for a name on Save instead of displaying a name box.
- Added shared Word-style **Find & Replace** controls with literal/regular
  expression search, case options, undoable replacements and per-editor Wrap
  controls across Policy/Assertions, Gateways, ZPR Config, scenario JSON/YAML
  and directory LDIF. Changes remain unsaved and never apply runtime settings.
- Source-local errors stay in diagnostic gutters; service/configuration
  failures without valid source locations remain separately visible. Stale
  analysis responses are rejected and diagnostics clear on source/record changes.
  Definitions-only policy analysis reports fixture failures explicitly rather
  than turning red without an explanation.
- Added **Analyze** and automatic policy tests before Save and Save As.
  Failed tests require an explicit confirmation to continue saving. Compiler-
  invalid source may be preserved for repair, but server-side compilation
  prevents it from being staged. Staging creates a candidate; it does not push
  or install a live policy.
- Added ordered bootstrap, platform, and organization policy layers for
  simulation policy composition. Organization workspaces can hold separate
  policy, scenario, and directory data; this workflow is not by itself a claim
  of complete runtime or tenant isolation.
- Added report-only trusted-data assertions stored as revisioned,
  organization-scoped Policy Repository records. Assertions support group and
  people membership/cardinality rules plus allowlisted LDAP attribute checks,
  manual evaluation, and optional scheduled checks. Results do not change ZPL
  permissions, visas, or network access.
- Added optional assistants across Policy/Assertions, Gateways, ZPR Config,
  directory LDIF and scenario form/raw editors. They require server configuration;
  submitted context/chat is sent to Anthropic. Text insertion is undoable and
  structured proposals are checked against their original context; suggestions
  never save, publish, activate or run automatically.

## Services, DNS, And Observability

- Added trusted-service integrations for file, LDAP, and REST-backed providers,
  with status that distinguishes a successful attribute lookup from a failed
  or unverified lookup. Read-only directory browsing is available in the
  operator views.
- Added sortable trusted-source directory tables, LDAP-tree/People/Groups/
  Attributes views, and expandable person attributes. Refresh is explicit;
  revisiting the page or automatic dashboard polling does not reread LDAP.
- Extended service-provider indexing and added publication of service records
  to ZPR DNS. Added an isolated BIND 9 deployment profile that serves on a ZPR
  address and accepts Visa Service updates through TSIG over ZPR.
- Added an OpenObserve deployment profile behind a dedicated ZPR adapter,
  including documented ingestion and reader-access interfaces. A separate
  local Compose mode provides one loopback-only store with an active-
  organization collector that tags telemetry and reads that profile's runtime
  logs. Control Room Diagnostics still requires a separate read-only query
  credential. The legacy ZPR-adapter profile does not automatically provision
  adapters.
- Added a metadata-only OpenLDAP accesslog change feed to the REST trusted
  service. It reports directory change metadata, not attribute values; it does
  not yet invalidate Visa Service attribute caches, reevaluate assertions, or
  revoke visas.

## Simulator

- Added a separate simulation-only operator site with manifest-backed machine
  profiles, workload placement, scenario execution, managed-service logs, and
  machine controls carried over ZPR rather than exposed as an underlay control
  path.
- Merged passive device/runtime inspection and workload log viewing into a
  Workers page; active start/stop/login machine controls were removed from that
  operator surface. Labeled the fleet as Devices, grouped scenarios by folder
  without an extra Unfiled heading, and added a clear action for completed/cancelled run history
  without deleting scenario definitions or logs.
- Added organization-scoped scenario and directory workspaces, organization
  switching and approval flows, and example organizations for exercising
  different policy and directory configurations.
- Added demo integrations for trusted sources, DNS, observability, and policy
  layers. Simulator data is for controlled testing and does not imply that a
  scenario or policy has been installed in a live ZPR network.
- Converted directory and scenario editors from modal dialogs to full editor
  pages. Scenario editing supports JSON/YAML conversion, source-local analysis,
  draft revision history and separate Save/Publish actions.
- Moved Simulator Activity and activation checks to private mutual-TLS
  Control-Service reads instead of a Control Room browser session. Control
  Room does not depend on Simulator APIs, profiles, workloads or availability.
- Added trusted-helper build preflight before stopping an existing organization
  runtime, including a pinned Go container-builder fallback when the Simulator
  has no native compiler. Build failures preserve the existing rig; successful
  helper artifacts publish atomically.
- Added Load Lab, a two-machine stress profile with 200 named service ports,
  200 logical request clients, randomized fresh-connection traffic, and short
  offline/restart intervals. The logical clients share one client adapter
  identity; this tests flow and service fan-out, not hundreds of independent
  adapter identities. Its generated runtime grants still require the normal
  compile/sign/install workflow before running the scenario.

## Security And Quality Boundaries

- Split browser-facing Control Room, Control-Service, and Policy-Service
  responsibilities; service-to-service calls use mutual TLS, while upstream
  Visa Service credentials remain server-side.
- Added loopback and origin checks, bounded API inputs and outputs, private
  repository storage, log redaction, browser CSP protections, and browser
  regression coverage for desktop and mobile workflows.
- Recorded local browser deployments use loopback access. Named-user OIDC,
  explicit enrollment permission grants and direct private-management HTTPS
  configuration are implemented; certified remote installation,
  general-purpose role management, per-viewer directory
  authorization and complete multi-organization runtime isolation are not
  established by these additions. Provisioning read/create/cancel browser
  workflows have real OIDC/mTLS/SQLite coverage with Simulator unavailable.
  Further JavaScript controller/assistant/table/polling consolidation is planned
  in the [GUI checklist](GUI%20work.md), not a completed capability.
  Consult the [security hardening checklist](zpr-dashboard/SECURITY_HARDENING.md)
  and [browser access contract](zpr-dashboard/BROWSER_ACCESS_CONTRACT.md)
  before making deployment or security claims.

## Further Reading

- [Control Room and Services guide](zpr-dashboard/cmd/zpr-web-dashboard/README.md)
- [Gateway work plan](../Gateway%20work.md)
- [Provisioning plan](../Provisioning%20plan.md)
- [Active GUI and JavaScript refactoring backlog](GUI%20work.md)
- [Completed GUI work and verification evidence](GUI%20completed%20work.md)
- [GUI deployment history](GUI%20deployments.md)
- [Trusted-service REST and LDAP change-feed guide](zpr-dashboard/cmd/zpr-trusted-service/README.md)
- [Trusted Data Assertions](zpr-dashboard/ASSERTIONS.md)
- [BIND 9 ZPR DNS guide](dns/bind9/README.md)
- [OpenObserve profile](observability/openobserve/README.md)

Update this page when a new capability becomes part of the maintained fork.
Keep detailed behavior, configuration, and limitations in the owning component
documentation and link to it here rather than duplicating its full reference.