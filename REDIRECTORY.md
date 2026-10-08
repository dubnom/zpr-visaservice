# Source and Project Reorganization Plan

All checklist items are planned, not implemented. Reorganize incrementally to
make ownership and dependency boundaries enforceable without changing behavior.
Keep the existing major project/crate boundaries unless a concrete dependency
problem justifies changing them. This is not a framework migration, microservice
split, or wholesale repository relocation.

Related trackers:
- [GUI work](GUI%20work.md): editor and JavaScript reuse work.
- [Scaling](SCALING.md): performance and capacity changes.
- [Repository guidance](AGENTS.repo.md): production/Simulator boundary.

## 1. Inventory and migration safety

- [ ] Inventory application modes, entry points, packages, public contracts, embedded assets and runtime configuration.
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
- [ ] Complete shared editor-controller reuse using the detailed JavaScript checklist in GUI work rather than duplicating competing helpers.
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
- [ ] Treat checked-in organization examples as seeds/templates, not a second live database.
- [ ] Define coherent organization draft/revision boundaries across related documents before implementing the organization creation workspace.
- [ ] Keep publication, provisioning and activation separate from saving draft state.
- [ ] Align the organization editor work in GUI work with these ownership decisions.

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
