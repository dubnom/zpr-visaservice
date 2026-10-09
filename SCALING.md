# Scaling

Project-wide tracking for scaling work. All items below are planned, not
implemented. Add future scaling areas here as requirements become concrete.

Reviewed 2026-10-09: editor consolidation, local World Map certification and
Gateway forwarding unit/integration tests do not establish scaling acceptance.
No measured capacity/latency certification or source-generation pipeline has
been completed by that work; the scaling tasks below remain open.

## Trusted sources

### Goal and current constraints

Separate source acquisition from browsing and assertion execution. Move from
whole-source reads and in-memory browser rendering to bounded queries over
indexed, versioned source generations. Raising limits alone is not the solution.

The current LDAP path is bounded to 20,000 entries and 4 MiB with short read
timeouts. It builds complete in-memory identity/attribute/membership maps.
Trusted Sources sends a complete directory to the browser for JavaScript
filtering, sorting and rendering. Nested groups and unresolved memberships
are currently rejected.

### Scale targets and contracts

- [ ] Define representative and maximum identity counts, group counts, membership edge counts, attribute volumes and single-group sizes.
- [ ] Define source freshness requirements, browse latency targets, import/evaluation time targets and concurrent organization/source workloads.
- [ ] Define memory, storage, network and provider-request budgets before changing existing limits.
- [ ] Define completeness and consistency contracts for each provider; paged reads alone must not be described as point-in-time snapshots.
- [ ] Define the states complete/current, complete/stale, import in progress and incomplete/unavailable.
- [ ] Define when stale generations may be browsed or evaluated and when freshness requirements must reject evaluation.

### Production source connectors

- [ ] Support bounded paged LDAP searches and cursor-based HTTPS APIs.
- [ ] Handle provider-specific retrieval of large and multi-valued group memberships without silent truncation.
- [ ] Replace the local Docker/ldapsearch path as the production integration mechanism with independently configured production connectors.
- [ ] Preserve TLS verification, approved-attribute restrictions, protected credentials and explicit organization/source scopes.
- [ ] Detect provider size/time limits, incomplete results, inaccessible required data and unsupported capabilities; never publish partial results as complete.
- [ ] Document connector consistency guarantees and limitations, including changes occurring during paged reads and ACL-filtered visibility.

### Synchronization and recovery

- [ ] Add background initial full imports with bounded resource usage and visible progress.
- [ ] Add supported provider change feeds or incremental synchronization, with full-import fallback for providers without change tracking.
- [ ] Add periodic reconciliation to detect missed changes and drift.
- [ ] Handle deletes, renamed identities, expired cursors, reconnects and interrupted imports.
- [ ] Define safe restart/checkpoint and retry behavior without duplicating entries or publishing incomplete generations.
- [ ] Keep the last complete generation on import failure, explicitly marked stale with the failure and last-success time.

### Indexed, versioned source storage

- [ ] Store organization/source-scoped identities, approved attributes and membership edges in an indexed read model.
- [ ] Use stable provider identity IDs rather than treating display names or DNs as permanent identities.
- [ ] Define indexes and bounded queries for identities, attributes, groups and membership traversal.
- [ ] Publish a new generation atomically only after a complete successful import and required integrity checks.
- [ ] Bind browse responses and assertion results to a source generation and its observation/consistency metadata.
- [ ] Keep credentials separate from source data and never place secret attributes in the read model.
- [ ] Define generation retention, cleanup and references held by running jobs/results so reclamation cannot break active reads.

### Browser API and GUI

- [ ] Return bounded pages instead of complete directory payloads.
- [ ] Add server-side search, filtering and stable sorting with explicit supported fields and bounded query costs.
- [ ] Bind opaque cursors to organization, source, query and generation; reject invalid or expired cursors explicitly.
- [ ] Add individual-entry details and separately paginated group-member endpoints.
- [ ] Use paginated tables and lazy-loaded directory trees; fetch expanded details on demand.
- [ ] Avoid constructing thousands of hidden DOM rows or retaining a full directory merely to render a page.
- [ ] Show source generation, freshness, last successful synchronization, completeness, progress and failures.
- [ ] Cancel obsolete requests and reject stale responses after source, organization, query or generation changes.
- [ ] Keep incomplete/unavailable distinct from an empty source or a valid zero-result search.

### Assertion evaluation

- [ ] Evaluate against a defined source generation using indexed queries and reusable membership sets.
- [ ] Avoid rereading the complete provider for every assertion run.
- [ ] Preserve existing assertion semantics and useful warnings while changing the execution/storage model.
- [ ] Return bounded, paginated violation details with complete counts or explicitly identified count limitations.
- [ ] Distinguish assertion FAIL from provider failure, incomplete data, stale-data rejection and cancelled evaluation.
- [ ] Record the assertion revision/exact submitted draft and each source generation used in an evaluation.
- [ ] Allow operators to inspect the generation that produced a result, subject to retention and authorization, rather than silently showing newer data.
- [ ] Define consistency behavior for expressions involving multiple sources with different generations or freshness.

### Membership semantics

- [ ] Specify direct versus transitive membership explicitly.
- [ ] Add nested-group support with cycle detection, bounded traversal and unresolved-reference handling.
- [ ] Preserve distinct-identity counting and detect duplicate/ambiguous identity mappings.
- [ ] Do not silently change existing assertions from direct to transitive membership; introduce explicit syntax/configuration and behavior tests.
- [ ] Ensure a partially retrieved membership list cannot produce a success-shaped evaluation.

### Background jobs and resource control

- [ ] Make large imports and evaluations asynchronous jobs with bounded progress/status responses.
- [ ] Add cancellation, deadlines, concurrency limits and backpressure.
- [ ] Bound per-organization/source work and prevent one large source from monopolizing shared resources.
- [ ] Keep interactive browsing responsive while imports and evaluations are running.
- [ ] Add operational metrics for source size, import duration, freshness lag, query latency, evaluation duration, queue depth and resource usage.
- [ ] Surface failures explicitly with secret-free diagnostics; do not silently fall back to partial or stale data.

### Security and retention

- [ ] Preserve organization/source isolation at connector, storage, API, job and result boundaries.
- [ ] Define read permissions for summaries, identity details, memberships, historical generations and exports.
- [ ] Protect stored source data and backups and define retention/deletion policies appropriate to identity data.
- [ ] Keep credentials and sensitive attribute values out of operational logs, errors and metrics.
- [ ] Test cross-organization/source denial and prevent cursor/job/result identifiers from bypassing authorization.

### AI assistant context

- [ ] Do not send complete large trusted directories to the model.
- [ ] Prefer the edited assertion/document and approved attribute/schema summaries as assistant context.
- [ ] Support explicitly selected, bounded source context only when needed and authorized.
- [ ] Preview exactly what context will leave the system before submission.
- [ ] Require deliberate selection and appropriate disclosure for real identity values; do not silently include them in summaries.
- [ ] Keep fictional Simulator seed assistance separate from production source access.

### Load and correctness testing

- [ ] Build representative large synthetic sources with configurable identities, groups, membership edges and attribute sizes.
- [ ] Test one enormous group, deep nesting, membership cycles, high fan-out and provider-specific ranged/paged membership retrieval.
- [ ] Test changes during paged reads, deletes/renames, cursor expiration, provider failures and interrupted imports.
- [ ] Verify failed imports never replace a complete generation with partial data.
- [ ] Verify browse/evaluation generation binding, freshness policies and historical result inspection.
- [ ] Measure latency, memory, storage and provider load against the agreed targets; do not use successful completion alone as the scale criterion.
- [ ] Test concurrent organizations/sources, cancellation, queue saturation and interactive browsing during imports/evaluations.
- [ ] Compare indexed evaluations with existing small-source behavior to preserve results and warnings.
- [ ] Test production features with Simulator unavailable and Simulator environment settings unset or invalid.

### Architecture boundary

- [ ] Keep production connectors, configuration, storage and APIs independent of Simulator manifests, profiles, sessions and availability.
- [ ] Allow Simulator to supply large synthetic test sources through explicit test contracts without introducing reverse production dependencies.

### Recommended delivery sequence

- [ ] Establish scale targets and correctness/freshness contracts.
- [ ] Deliver bounded browsing APIs and lazy GUI rendering.
- [ ] Introduce indexed source generations and background full imports.
- [ ] Move assertion execution onto the read model with existing semantics preserved.
- [ ] Add incremental synchronization and explicit nested-membership support.
- [ ] Certify representative load, failure recovery and isolation against measurable targets.

## Visa Service

The items below address code-level scaling risks, not measured capacity claims.
Preserve existing bounded queues, in-memory indexes, policy snapshots,
generation checks and compare-and-swap lifecycle protections while optimizing.
Increasing worker counts or removing the exclusive instance lock is not a
scaling solution by itself.

### Capacity targets and instrumentation

- [ ] Define sustained and burst visa-decision rates, including allowed and denied requests.
- [ ] Define live visa counts, lifetimes, renewal rates and representative path lengths.
- [ ] Define connected node/actor counts and reconnect-storm workloads.
- [ ] Define policy sizes, topology densities and route-dependent rule workloads.
- [ ] Define trusted-provider latency, failure and attribute-change workloads.
- [ ] Set latency targets for decision stages, node-install acknowledgement, revocation convergence and restart/failover recovery.
- [ ] Instrument queue wait, actor lookup, attribute refresh, policy evaluation, route search, persistence and installation acknowledgement.
- [ ] Measure persistence-lock wait, ValKey latency, event backlog, node-sync backlog, cache size, expiry work and memory per visa/path.
- [ ] Benchmark the current 1,024-worker and 1,024-queue bounds before tuning; do not present configured limits as demonstrated capacity.

### Visa persistence and state consistency

- [ ] Measure contention on the global visa backing mutex held across ValKey I/O.
- [ ] Evaluate bounded write batching or carefully partitioned mutation ownership rather than adding workers behind the same serialized writer.
- [ ] Preserve persistence-before-memory ordering, atomic secondary-index updates and correctness under ambiguous commits.
- [ ] Preserve generation checks and compare-and-swap state transitions, including pending-revoke precedence over installation acknowledgements.
- [ ] Bound persistence queues and report overload explicitly without success-shaped issuance responses.
- [ ] Load-test issuance, installation acknowledgements, revocations and expiry concurrently, including ValKey latency and failures.

### Routing and policy evaluation

- [ ] Measure cold/warm route-dependent evaluation on sparse and dense topologies.
- [ ] Implement route-hint/constraint-aware search and pruning instead of ignoring hints and enumerating every simple path.
- [ ] Bound route-search work with explicit indeterminate/error behavior; never treat incomplete search as a conclusive policy denial or select an unauthorized route.
- [ ] Bound route-cache memory and result-copy costs and define safe eviction behavior.
- [ ] Reduce duplicate concurrent cache-miss computation while preserving topology-generation correctness.
- [ ] Test topology changes during search/cache insertion and retain authorized-route selection semantics.
- [ ] Profile policy evaluation CPU cost and runtime responsiveness before selecting execution-pool or evaluator optimizations.

### Reconciliation and event processing

- [ ] Move large policy/attribute reconciliation passes into generation-aware background work so unrelated events do not wait behind complete sweeps.
- [ ] Process visas in bounded batches instead of cloning all metadata and evaluating the full set sequentially.
- [ ] Add progress, cancellation/supersession, resumability and explicit completion/failure status.
- [ ] Prioritize revocation work and define measurable enforcement convergence targets under load.
- [ ] Coalesce redundant refresh/update work without losing actor lifecycle events or required generation ordering.
- [ ] Use affected actor/visa indexes for targeted attribute-change reconciliation where source contracts provide reliable change scope.
- [ ] Preserve refresh-before-evaluation ordering, stale-generation rejection and revocation correctness when a newer policy arrives mid-pass.
- [ ] Keep DNS reconciliation and service-list publication from monopolizing the event loop; bound fan-out and expose failures.

### Node synchronization and fan-out

- [ ] Measure the single current-thread VSS runtime under increasing node counts and pending install/revoke volume.
- [ ] Partition node workers across runtimes/threads while respecting RPC client ownership and per-node ordering requirements.
- [ ] Prefer event-driven pending work where appropriate and stagger heartbeat/ping timers to avoid synchronized load spikes.
- [ ] Bound per-node installation/revocation batches, command queues and cross-node fan-out.
- [ ] Prevent slow or unavailable nodes from delaying unrelated nodes or starving revocation work.
- [ ] Track queued, sent and acknowledged installs/revocations separately; do not equate a returned visa decision with completed installation on every path node.
- [ ] Test disconnect/reconnect, replay, partial acknowledgement and pending-revoke precedence during concurrent pushes.

### Trusted-attribute refresh

- [ ] Coalesce concurrent refreshes for the same actor/source to avoid duplicate provider requests and conflicting writebacks.
- [ ] Bound per-provider concurrency and queueing and implement proactive refresh where freshness contracts permit it.
- [ ] Align provider deadlines with the remaining request budget; resolve the current five-second HTTP provider timeout versus three-second default visa-request budget.
- [ ] Preserve attribute expiry, source revision tracking and persistence-before-revision-commit ordering.
- [ ] Keep indeterminate/stale-source failures fail-closed; do not turn missing or unavailable claims into authoritative absence.
- [ ] Test refresh storms, provider outages, revision changes and concurrent requests against the same actor.

### Request lifecycle and overload

- [ ] Carry request deadlines/cancellation into queued jobs rather than only timing out the response waiter.
- [ ] Discard expired or cancelled work before issuance where safe; avoid consuming capacity for abandoned queued requests.
- [ ] After issuance commits, complete or reconcile required distribution rather than abandoning partially installed state on caller cancellation.
- [ ] Bound background visa-distribution tasks and expose their backlog/failure status.
- [ ] Define explicit overload responses, fair admission and retry guidance to prevent timeout/retry storms.
- [ ] Verify sustained overload remains bounded in memory, task count and recovery time.

### Memory, expiry and restart recovery

- [ ] Measure bytes per live visa, metadata, node reference and actor index at representative path lengths.
- [ ] Replace repeated whole-store expiry scans with an expiry index/timing structure or another bounded approach after measuring the current scan cost.
- [ ] Bound cleanup work and store-lock hold times while preserving expiry and secondary-index consistency.
- [ ] Stream/pipeline state recovery in bounded batches instead of collecting all keys and fetching records sequentially.
- [ ] Report recovery progress and expose readiness only after required state and indexes are coherent.
- [ ] Test restart with large state, expired/malformed records, concurrent key expiration and database interruptions.
- [ ] Certify memory and recovery targets against live count plus renewal/lifecycle churn, not live count alone.

### Admin and monitoring APIs

- [ ] Add cursor pagination and bounded filtering to visa, actor and other large admin listings.
- [ ] Provide summary/count endpoints and bounded individual details so monitoring does not require full-state downloads.
- [ ] Support incremental updates where appropriate with explicit consistency and cursor semantics.
- [ ] Isolate or budget monitoring work so polling/export cannot degrade issuance or revocation.
- [ ] Load-test multiple operator clients while decisions, synchronization and reconciliation run concurrently.

### High availability and horizontal scaling

- [ ] Certify single-authoritative-writer failover before attempting active-active issuance.
- [ ] Preserve and test exclusive ownership/fencing; do not remove the database instance lock merely to run more replicas.
- [ ] Define standby readiness and recovery of visa IDs, policy generations, pending installs and pending revocations.
- [ ] Test lock loss, database outage, ambiguous writes, failover timing and stale-instance behavior.
- [ ] Decide whether independently owned fabric/tenant partitions fit the deployment model before designing active-active operation.
- [ ] Define partition ownership, cross-partition flows, mutation ordering and policy/revocation propagation before distributing issuance.
- [ ] Document that ValKey replication alone does not provide active-active Visa Service correctness.

### Load and correctness certification

- [ ] Benchmark warm/cold attributes, allow/deny bursts, renewal churn and long multihop paths.
- [ ] Test dense topologies and route-dependent rules without relying only on small linear networks.
- [ ] Test reconnect storms and node-sync saturation during ongoing issuance.
- [ ] Test policy/attribute changes under load and measure enforcement convergence at every affected node.
- [ ] Test database/provider latency and outages, caller cancellation, overload and recovery.
- [ ] Verify performance changes preserve policy verdicts, route authorization, freshness checks and visa lifecycle invariants.

### Recommended delivery sequence

- [ ] Instrument the decision pipeline and shared contention points.
- [ ] Establish realistic baseline workloads and measurable capacity/latency targets.
- [ ] Address route enumeration and global persistence contention based on evidence.
- [ ] Make reconciliation and node fan-out bounded, prioritized and observable.
- [ ] Improve expiry, restart recovery and admin-query scaling.
- [ ] Certify single-writer high availability before implementing active-active ownership.
