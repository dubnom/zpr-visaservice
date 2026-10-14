# Source and Project Reorganization Plan

Status reconciled 2026-10-14. Most checklist items remain planned; the shared
editor-controller reuse is implemented and deployed. Neutral Gateway forwarding
helpers and naming cleanup are committed but not deployed; they remain inside
the existing Go main package and do not complete production/Simulator package
separation. Reorganize incrementally to
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
  release includes the shared assets; Control Room was not replaced by that
  rollout, so its pending assets are not claimed deployed.
- These repairs and UI changes do not complete source inventory, thin-command
  extraction, feature-module reorganization, Rust subdivision or naming work.
  The latest pending editor and DNS source/documentation changes are not yet
  committed or pushed; use the deployment ledger for runtime scope.

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

Current ownership is obscured by the shared web `package main`: production
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

The inventory is read-only: `go list` succeeded, but no new build/test baseline,
full cross-domain call audit, source relocation, commit, push, or deployment was
performed for it. The checklist below deliberately keeps the broad inventory
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
- [ ] Define allowed dependency directions and avoid a catch-all shared package that recreates the current coupling.
- [ ] Add dependency-boundary checks that fail when production packages import Simulator implementation.
- [ ] Test production startup and operator features with Simulator unavailable and Simulator environment settings unset or invalid.
  - DNS statistics transport was verified with Simulator paused; complete
    startup/feature coverage and package import enforcement remain pending.

## 3. Thin Go command entry points

- [ ] Move domain behavior out of cmd/zpr-web-dashboard into cohesive internal packages.
- [ ] Keep command entry points focused on flags/configuration, dependency assembly, lifecycle and server startup.
- [ ] Extract production operator application/HTTP handlers and clients with explicit injected dependencies.
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

- [ ] Complete inventory, baseline checks and dependency ownership decisions.
- [ ] Enforce production/Simulator separation through package extraction.
- [ ] Thin the web command and extract remaining cohesive Go domains.
- [ ] Split frontend feature controllers and consolidate shared editor behavior.
- [ ] Resolve organization artifact authority before delivering the new organization editor.
- [ ] Address misleading names through explicit compatibility decisions.
- [ ] Subdivide Rust modules where responsibility boundaries justify it.
- [ ] Finish documentation indexing and end-to-end build/package/deployment verification.
