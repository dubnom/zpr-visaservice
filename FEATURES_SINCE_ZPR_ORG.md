# Features Added Since the Upstream Fork

**Snapshot:** 2026-10-04
**Baseline:** `origin/main` at `abb1acd` (2026-09-03)

This is a capability overview of the work added in this fork after its upstream
baseline. It is intentionally not a commit log. The existing Visa Service,
policy evaluator, compiler, and wire formats remain the foundation; the work
below adds service integrations, operator tools, simulation workflows, and
policy-management capabilities around them. Implementation and security
contracts remain authoritative where this overview is shorter.

## Control Room

- Added a browser-based, read-only operations view alongside the existing CLI
  dashboard. It presents actors, services, network topology, current visas,
  denials, and adapter information with interactive inspectors and maps.
- Added live topology updates that preserve the position of existing nodes,
  distinguish arrivals and removals, show gateway network relationships, and
  use stable service-type colors across maps and tables.
- Added complete per-adapter visa inspection, DNS-aware address labels, and
  refresh behavior that retains the last good data when an upstream read fails.
- Added a read-only Security Review that groups denial and health evidence,
  highlights changes against a browser-local inventory baseline, supports
  dismissal and restoration, and links IP evidence to DNS names. Findings are
  triage signals, not automatic incident determinations or traffic controls.
- Added responsive, independently collapsible navigation for Control Room and
  Simulator, plus shared log panels for adapter, controller, application, and
  service output. Log panels retain the last bounded tail on disconnect and
  visibly change to a disconnected state.

## Policy And Trusted Data

- Added a versioned Policy Repository service with categorized records,
  append-only revisions, optimistic concurrency checks, and private SQLite
  storage. Control Room reaches it through service APIs rather than owning its
  storage or simulator organization logic.
- Added Policy Studio editing support: ZPL completions, LDAP attribute
  discovery, formatting, compiler diagnostics, source-aware test results, and
  a standalone read-only policy/assertion browser.
- Added **Evaluate & Test** and automatic policy tests before Save and Save As.
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
- Added an optional policy assistant. It is disabled unless configured;
  submitted policy context is sent to Anthropic when enabled, and suggestions
  are not applied automatically.

## Services, DNS, And Observability

- Added trusted-service integrations for file, LDAP, and REST-backed providers,
  with status that distinguishes a successful attribute lookup from a failed
  or unverified lookup. Read-only directory browsing is available in the
  operator views.
- Extended service-provider indexing and added publication of service records
  to ZPR DNS. Added an isolated BIND 9 deployment profile that serves on a ZPR
  address and accepts Visa Service updates through TSIG over ZPR.
- Added an OpenObserve deployment profile behind a dedicated ZPR adapter,
  including documented ingestion and reader-access interfaces. The profile
  does not automatically provision adapters or export existing metrics.
- Added LDAP change-consumer integration and reliability improvements for
  Control Room data refresh and lifecycle handling.

## Simulator

- Added a separate simulation-only operator site with manifest-backed machine
  profiles, workload placement, scenario execution, managed-service logs, and
  machine controls carried over ZPR rather than exposed as an underlay control
  path.
- Added organization-scoped scenario and directory workspaces, organization
  switching and approval flows, and example organizations for exercising
  different policy and directory configurations.
- Added demo integrations for trusted sources, DNS, observability, and policy
  layers. Simulator data is for controlled testing and does not imply that a
  scenario or policy has been installed in a live ZPR network.
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
- The current browser tools are for local operator workflows. Remote browser
  authentication, user-specific roles, per-viewer directory authorization,
  and complete multi-organization runtime isolation are not established by
  these additions. Consult the [security hardening checklist](zpr-dashboard/SECURITY_HARDENING.md)
  and [browser access contract](zpr-dashboard/BROWSER_ACCESS_CONTRACT.md)
  before making deployment or security claims.

## Further Reading

- [Control Room and Services guide](zpr-dashboard/cmd/zpr-web-dashboard/README.md)
- [Trusted Data Assertions](zpr-dashboard/ASSERTIONS.md)
- [BIND 9 ZPR DNS guide](dns/bind9/README.md)
- [OpenObserve profile](observability/openobserve/README.md)

Update this page when a new capability becomes part of the maintained fork.
Keep detailed behavior, configuration, and limitations in the owning component
documentation and link to it here rather than duplicating its full reference.