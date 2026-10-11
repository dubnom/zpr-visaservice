# NATTY: session-scoped address indirection for ZPR

Status: proposed design and implementation plan, not an implemented feature.

## 1. Objective

Allow an adapter to communicate using opaque, attachment-local addresses without
learning the canonical ZPR addresses of remote actors or the network's internal
node topology. Preserve authenticated identity and Visa Service policy enforcement.
Bind translations to authenticated sessions so old mappings cannot be reused
after detach, reconnect, revocation, or expiration.

NATTY is a working name, not a commitment to ordinary NAT semantics. Its core is
address indirection at the adapter/node boundary, not a general-purpose NAT gateway.

The original proposal includes adapter-generated random-ish addresses. This plan
supports evaluating that option but recommends node-issued aliases for the first
implementation. Either approach must use cryptographically secure randomness,
not predictable pseudo-random identifiers.

## 2. Security claims and non-goals

### Intended benefits

- Reduce canonical address disclosure to adapters.
- Limit address correlation across attachment sessions.
- Prevent a compromised adapter from selecting arbitrary canonical source identities.
- Reject reuse of mappings outside their authorized identity, session, and flow.
- Keep routing information at nodes instead of encoding topology in adapter-visible
  addresses.

### Claims this design must not make

- A random address is not authorization and does not itself prevent replay.
- Alias rotation does not replace authenticated transport, visas, or revocation.
- Hiding canonical addresses does not hide traffic timing, volume, service names,
  application payload disclosures, or the substrate endpoint used to reach a node.
- A compromised node can observe mappings and forwarding destinations.
- An authorized adapter can still abuse its authorized operations. Network packet
  replay prevention is not application-level transaction deduplication.
- This design does not guarantee anonymity or conceal topology from privileged
  operators.

### Initial scope exclusions

- No changes to canonical inter-node routing in the first release.
- No adapter-issued authority to create, widen, or extend visas.
- No custom cryptography or new key-exchange algorithm.
- No production dependency on Simulator.
- No public unauthenticated alias-resolution service.
- No initial promise of transparent mobility for existing TCP connections.

## 3. Existing architecture and evidence

The current implementation must be traced in detail before choosing a wire format.
The following repository evidence constrains the design:

- [ZPR core README](../zpr-core/README.md) describes per-Noise-session reset of
  reliable-management retries and duplicate windows. Existing replay/duplicate
  handling must be audited and reused where applicable, not replaced blindly.
- The same README describes visa expiration cleanup and denial caching keyed by
  ingress adapter link, source/destination addresses, protocol, and destination
  port. Alias rotation must not bypass the canonical denial-cache scope.
- [Visa Service README](README.md) describes restart bootstrap, current-session
  liveness, relearning ephemeral A2A keys, and revoking old non-bootstrap visas.
  New mapping lifetimes must follow these guarantees.
- [Milestone 2 startup documentation](../zpr-core/examples/milestone2/README.md)
  describes adapter address registration. It is historical context, not proof
  of the current wire contract.
- [Control Room boundary guidance](AGENTS.repo.md) requires independent operator
  APIs and configuration. Simulator may provision and test services, but operator
  services must not discover NATTY state through Simulator.

Likely implementation areas to investigate:

| Area | Starting point | Investigation |
|---|---|---|
| Adapter/node transport | [PH sources](../zpr-core/adapter/ph/src/) | Packet ingress/egress, registration, bind requests, authenticated link identity |
| Noise integration | [km_noise.rs](../zpr-core/adapter/ph/src/km_noise.rs) | Session establishment, nonce/replay ownership, restart behavior |
| Visa installation | [visa_mgmt.rs](../zpr-core/adapter/ph/src/visa_mgmt.rs) | Bind/install acknowledgments, endpoint matching, cleanup |
| Visa matching | [visa_table.rs](../zpr-core/adapter/ph/src/visa_table.rs) | Canonical flow keys, expiration, translation ordering |
| Shared visa types | [visa.rs](../zpr-common/src/vsapi_types/visa.rs) | Fields, serialization, policy/forwarding dependencies |
| Authority and recovery | [Visa Service sources](vs/src/) | Actor identity, revocation, policy changes, bootstrap |
| Discovery and diagnostics | [Dashboard](zpr-dashboard/) | DNS, enrollment, trusted services, logs and operator visibility |

These are investigation entry points, not assertions that each file needs editing.

## 4. Architectural model

Separate three concepts:

1. **Actor identity:** authenticated certificate identity used for policy.
2. **Canonical address:** internal address used for node routing and visa matching.
3. **Local alias:** opaque address or handle meaningful only within an authenticated
   adapter attachment and its explicitly authorized mapping.

Illustrative forwarding path:

```text
adapter A
  application-facing destination alias
  authenticated attachment session A
        |
        v
ingress node
  validate session and packet freshness
  resolve alias in A's scope
  validate canonical flow against visa
        |
        v
canonical inter-node forwarding
        |
        v
egress node
  validate destination attachment and visa
  rewrite canonical endpoints to B's local view
        |
        v
adapter B
  application-facing local addresses
```

The canonical address is not transmitted in adapter-facing management messages
merely to help the adapter perform translation. Such a design would defeat the
privacy objective.

### Source identity

The ingress node derives canonical source identity from the authenticated
attachment and permitted source bindings. It must not trust an adapter-supplied
canonical source address.

### Destination resolution

The adapter requests a service using a policy-recognized service identifier or a
locally resolved name. The node resolves the canonical endpoint through existing
production service contracts, obtains authorization, and grants a scoped alias.

Names must not create a shortcut around policy. Applications that already receive
canonical literal IP addresses require an explicit migration decision.

### Return traffic

Each endpoint gets its own local address view. Adapter B must not receive A's
canonical address in ordinary packets, error messages, or bind responses.
Reverse mapping is constrained by the visa and endpoint sessions, not simply by
an alias found in a global table.

## 5. Alias allocation options

### Option A: node-issued aliases -- recommended initial implementation

1. Adapter authenticates and negotiates NATTY support.
2. Adapter requests a destination/service binding.
3. Node authorizes the canonical flow through Visa Service.
4. Node allocates an alias from an approved local namespace.
5. Node installs the mapping and sends an authenticated grant.
6. Adapter activates the alias only after a successful grant.

Benefits: centralized collision checks, ownership enforcement, quotas, and cleanup.
The adapter learns only the alias and the grant information it needs.

### Option B: adapter-proposed aliases

1. Adapter generates a candidate using the platform CSPRNG.
2. Candidate travels over the authenticated control channel.
3. Node validates namespace, collision, ownership, and limits.
4. Node authorizes the canonical flow independently.
5. Node accepts the candidate or returns a bounded conflict response.
6. Adapter cannot send traffic through the mapping before confirmation.

An adapter proposal does not become a routable canonical identity. Conflict
responses must not disclose another adapter's bindings. Bound retry rates to
avoid collision probing or resource exhaustion.

### Address versus handle

Choose after the packet-path investigation:

- IPv6 aliases can preserve an IP-oriented application/TUN interface, but require
  carefully reserved local ranges and packet translation.
- Opaque handles may simplify the encrypted adapter/node protocol, but applications
  still need an address view or a proxy API.
- A hybrid can use an application-local IPv6 address and a distinct wire mapping ID.

Specify usable randomness after accounting for fixed prefix/version bits.
Document collision probability using
`p ~= n * (n - 1) / (2 * 2^b)` for `n` allocated aliases and `b` random bits.
Randomness never removes the requirement to check collisions.

## 6. Proposed state and contract

Logical mapping fields; final types belong in the existing shared protocol:

| Field | Meaning |
|---|---|
| Mapping ID | Opaque grant identifier, not an access credential |
| Alias | Adapter-local address/handle |
| Organization/security domain | Prevents cross-tenant resolution |
| Authenticated adapter identity | Owner of the alias |
| Attachment session/epoch | Fresh scope for this authenticated attachment |
| Canonical source/destination | Node-side only |
| Service selector | Authorized service context |
| Flow constraints | Protocol, ports and direction allowed by the visa |
| Visa ID and applicable generation | Links mapping to actual authority |
| Destination attachment generation | Prevents delivery into stale destination sessions |
| Expiration | No later than the applicable visa/session lifetime |
| State | Pending, active, revoked, expired, closed |
| Replay state reference | Existing authenticated transport or flow replay context |

Treat this as a logical schema, not a requirement to duplicate fields already
owned by existing link/visa objects.

Suggested management operations:

- Negotiate capability/version during authenticated attachment.
- Request an alias binding.
- Grant or explicitly deny a binding with bounded, non-sensitive reasons.
- Release a mapping.
- Revoke a mapping or invalidate an attachment generation.

Use the existing authenticated reliable-management framework if it provides the
needed semantics. Requests must be idempotent within their session; a retry must
not accidentally allocate a second mapping. Duplicate grants must not reactivate
revoked state. Bound pending requests and retry lifetimes.

## 7. Replay protection

### Required distinction

Inventory aliasing, reliable-control-message duplicate suppression, encrypted
transport replay rejection, and application request deduplication are different
mechanisms. Document which layer protects each packet class.

### Packet validation requirements

- Authenticate the packet before trusting its alias or sequence information.
- Resolve aliases only in the authenticated ingress session's namespace.
- Bind alias/mapping ID and protocol context to the authenticated envelope or
  authenticated plaintext, as appropriate to the existing protocol.
- Preserve unique nonce use under each encryption key.
- Reject already accepted sequence numbers within a bounded replay window.
- Support only explicitly bounded packet reordering.
- Never advance receive state on authentication failure.
- Never reuse a key with reset sequence/nonce state after a crash.
- Reject sequence wrap; rekey or close before exhaustion.
- Reject old-session packets after reattachment, even if alias bytes are reused.
- Keep control and data sequence domains separate if the current transport requires
  it; do not introduce ambiguous counters across directions or message classes.

Prefer existing proven transport behavior. Add a new replay counter only if
investigation demonstrates a missing protection, and explain its ownership.

### Cross-session and cross-node behavior

New sessions require fresh authenticated security context and fresh mapping scope.
Restarted nodes must not resurrect old aliases using persisted mapping records.
Migration to another node requires reauthorization or a defined transfer protocol;
it must not make an old packet acceptable on two independent replay windows.

### Application caveat

An authorized sender can submit the same application request in a new valid packet.
Protect payments or other non-idempotent operations with application transaction
IDs; NATTY must not claim to solve that problem.

## 8. Lifetimes, recovery, and race handling

| Event | Required behavior |
|---|---|
| First attach | Negotiate support, create fresh scope, install only authorized mappings |
| Soft reconnect | Reuse scope only if transport and replay continuity are proven; otherwise invalidate |
| Adapter restart | New session; old aliases cannot resolve |
| Node restart | No stale mapping restoration; rebuild through authentication and authorization |
| Visa expires | Disable mapping/forwarding promptly using existing expiry machinery |
| Visa revoked | Block forwarding before or atomically with mapping invalidation |
| Policy changes | Follow actual visa revocation/revalidation semantics; do not invent an independent permissive cache |
| Destination detach | Prevent stale delivery; report bounded failure and require a fresh binding |
| Same alias allocated elsewhere | Different owner/session namespace; no cross-resolution |
| Old release/revoke arrives late | Must not delete a new generation's mapping |
| Control/Visa Service unavailable | No new grants; existing authority may continue only within its established validity |
| Node mobility | First release may require reconnection and application flow restart |

Flow cache keys and denial caches must use canonical identity/flow context after
translation. Rotating aliases or source ports must not bypass a cached denial.
Mapping expiration must not extend a visa or reset replay state.

## 9. Packet translation and compatibility

Investigate and specify before implementation:

- Whether source and destination IP headers exist inside authenticated payloads.
- Translation order relative to integrity validation and visa matching.
- IPv6 TCP/UDP pseudo-header checksum adjustment, including fragmented packets.
- ICMPv6 errors with embedded original headers: reverse-translate or safely reject
  without leaking canonical endpoints.
- MTU effects from new envelope fields and path-MTU discovery.
- Extension headers, unsupported protocols, multicast, and broadcast policy.
- Bidirectional flow matching and source-port constraints.
- Canonical literals in DNS responses, enrollment configuration, APIs, logs, and
  application payloads.
- Service discovery TTL versus mapping lifetime and cache invalidation.
- Multiple services on one actor, multiple aliases for one destination, and
  multiple simultaneous attachments.
- ZPR bootstrap/management service addresses and existing well-known endpoints.

Do not change user payload content to hide addresses. Describe residual disclosure
when application protocols carry them. Define unsupported cases explicitly rather
than silently forwarding untranslated packets.

### Adapter-to-adapter encryption gate

Trace A2A encryption separately from hop encryption before selecting the translation
point. A node cannot rewrite an end-to-end authenticated IP header without breaking
integrity, and must not gain payload decryption rights merely to support NATTY.

If canonical endpoint headers are inside A2A ciphertext, either construct those
headers without exposing canonical addresses to adapters through an appropriate
protocol redesign, or keep canonical routing metadata in a node-owned outer
envelope and use an endpoint-local/flow identity inside the A2A-protected payload.
The latter requires a defined peer-visible address view, checksum strategy, and
binding of the encrypted flow to the authorized outer route. Do not assume the
existing protocol already supports either option.

If Phase 0 finds that current A2A setup or visa distribution necessarily exposes
canonical endpoints, record that as a privacy blocker. Resolve it before claiming
full address concealment. Retain authenticated peer identity, fresh A2A keys,
and existing bootstrap/revocation guarantees in any revised contract.

## 10. Resource limits and observability

Bound per-session and node-wide active mappings, pending allocations, request
rates, replay memory, and retired mapping state. Fail new allocations explicitly
at capacity; do not evict active authority in a way that silently misroutes traffic.
Invalidate lookup entries before reusing storage.

Operator metrics should include:

- Active/pending mappings and allocation failures.
- Expired, released, revoked, and session-invalidated mappings.
- Rejected unknown aliases, wrong-session use, and unauthorized flows.
- Replay rejects and authentication failures as distinct categories.
- Collision retries, capacity rejections, translation errors, and lookup latency.

Adapter-visible errors must not disclose canonical addresses or other actors'
existence. Canonical mappings may be available to authorized operators through
independent production APIs, with access control and audit logging.
Do not expose keys or treat aliases as secret credentials. Avoid unrestricted
high-cardinality alias/actor labels in metric streams.

Control Room should distinguish canonical operator identity from local aliases
where needed. Simulator may exercise the operator APIs but cannot become their
source of truth.

## 11. Phased implementation

### Phase 0: trace current behavior and establish a baseline

Deliverables:

- Packet/control flow diagram covering registration, visa request, installation,
  forwarding, reverse delivery, expiry, and reconnect.
- Inventory of all canonical-address disclosures to adapters.
- Replay-protection ownership matrix for management and data paths.
- Baseline tests and measured throughput, latency, memory, and reconnect time.
- Decision on alias allocator, application address representation, and MVP protocols.

Exit condition: verified current contracts and explicit security/privacy limits.
No wire or behavior changes yet.

### Phase 1: define shared contract and reference state machine

- Specify versioned capability negotiation and exact encoding.
- Define mapping ownership, generation, state transitions, and rejection reasons.
- Implement a bounded mapping table with atomic install/remove semantics.
- Add state-machine/property tests, including late messages and session replacement.
- Keep NATTY disabled by default.

Exit condition: no active mapping exists without valid owner, session, and visa.

### Phase 2: one adapter, one node, controlled service

- Implement node-issued aliases first unless Phase 0 justifies adapter proposals.
- Integrate request/grant lifecycle with existing authenticated transport.
- Translate ingress and return traffic while preserving canonical authorization.
- Start with explicitly supported TCP/UDP IPv6 flows.
- Add necessary ICMP/fragment handling before claiming general transparent support.
- Validate that no canonical endpoints appear in the supported adapter-facing
  packet or management paths.

Exit condition: authorized traffic works; unauthorized and replayed traffic fail.

### Phase 3: multi-node routing and recovery

- Keep internal routing canonical; resolve aliases at adapter boundaries.
- Integrate destination reattachment, visa revocation, session invalidation, and
  denial-cache behavior.
- Test node/adapter/VS restarts, asymmetric failure, and delayed messages.
- Define mobility limitations explicitly.

Exit condition: no stale mapping survives a forbidden lifetime transition.

### Phase 4: discovery, operator support, and compatibility

- Integrate production name/service resolution without leaking canonical addresses.
- Address enrollment/bootstrap and well-known service exceptions.
- Add controlled operator visibility and bounded metrics.
- Document unsupported application/address-literal cases.
- Exercise with Simulator unavailable; separately use Simulator for scenario tests.

Exit condition: operators can diagnose failures without exposing private mapping
data to adapters or depending on simulation infrastructure.

### Phase 5: opt-in rollout and hardening

- Negotiate support explicitly; do not silently switch wire semantics.
- Provide independent legacy-compatible, optional, and required-policy modes
  only if all are necessary for rollout.
- Required mode rejects incompatible peers rather than silently exposing canonical
  addresses. Optional fallback must be operator-visible.
- Canary selected adapters/services, compare against baseline, then expand.
- Conduct protocol/security review before enabling production requirements.
- Roll back through session teardown and normal legacy reauthentication, not
  restoration of retired keys or aliases.

Exit condition: correctness, replay, privacy, compatibility, and agreed performance
acceptance criteria pass.

## 12. Verification matrix

| Test | Required result |
|---|---|
| Two services, one adapter | Correct independent bindings and visa constraints |
| Same alias in two sessions | No cross-session lookup |
| Adapter-proposed collision | Safe rejection/retry without foreign mapping disclosure |
| Source spoof | Canonical identity derived from authenticated ingress; packet denied |
| Wrong protocol/port | Mapping cannot widen visa permissions |
| Same captured packet twice | Delivered at most once in the protected packet domain |
| Reordered packet within window | Accepted once, if transport permits reordering |
| Packet older than replay window | Rejected |
| Forged high sequence | Does not advance authenticated replay state |
| Replayed allocation/grant/release | Idempotent or rejected; no stale reactivation/deletion |
| Detach/reattach and restart | Old aliases and ciphertext unusable |
| Alias reused with fresh grant | Cannot inherit old authorization or replay state |
| Visa expiry/revocation | Forwarding blocked within defined enforcement bound |
| Alias rotation after denial | Cannot bypass canonical denial-cache scope |
| Destination restart | Old destination generation cannot receive |
| Node migration | No double acceptance through independent windows |
| Concurrent allocation/revoke | No partially installed or resurrected mapping |
| Capacity/flood | Bounded memory/CPU; explicit failure; valid traffic preserved within limits |
| TCP/UDP checksum | Correct after both translation directions |
| ICMP/fragment/MTU | Defined behavior; no canonical-address leak |
| Adapter-side packet capture | No canonical addresses in supported paths |
| Simulator down | Operator diagnostics and production forwarding remain independent |
| Legacy peer in required mode | Explicit rejection; no privacy downgrade |

Use existing Rust tests, integration harnesses, and packet capture tooling.
Add fuzz/property tests where current tools support them; new dependencies require
a concrete need. Unit tests alone are insufficient for wire privacy and replay.

## 13. Acceptance criteria and measurable gates

- Zero canonical address disclosures in the defined supported adapter-facing
  captures, management responses, DNS responses, and packet error paths.
- Zero successful cross-owner/session alias resolutions in adversarial tests.
- Zero duplicate delivery for captured-packet replay in the protected packet domain.
- Every active mapping references currently valid authority and session scope.
- Duplicate/late control messages cannot widen access or restore retired state.
- Mapping/replay memory remains within configured hard limits under allocation flood.
- All existing authorization, expiry, bootstrap, and reconnect regression suites pass.
- Required mode cannot downgrade to canonical-address exposure.
- Operator diagnostics work with Simulator absent.
- Before Phase 2, record numeric budgets for mapping lookup p95/p99 latency,
  throughput regression, per-mapping memory, revocation enforcement delay, and
  reconnect duration, using baseline measurements. Release is blocked until these
  budgets are approved and the implementation satisfies them.

Do not substitute a successful ping for these criteria.

## 14. Decisions still required

1. Node-issued aliases versus adapter-proposed aliases.
2. IPv6 application addresses, opaque wire handles, or hybrid representation.
3. Per-service versus per-flow mapping granularity.
4. Supported traffic classes and bootstrap exceptions for MVP.
5. Whether existing application sessions survive reattachment; default to explicit
   reconnect until continuity is proven.
6. Address namespace and usable entropy.
7. Existing transport replay coverage versus any demonstrated data-path gap.
8. Operator visibility/access policy for canonical mappings.
9. Privacy-required policy scope and downgrade rules.
10. Numeric resource/performance/enforcement budgets.

Resolve these before implementation changes that lock the wire protocol.

## 15. Difficulty and risk assessment

This is a cross-repository protocol and forwarding change, not a cosmetic address
randomization patch. A single-node prototype is substantially easier than a
production-ready implementation with bidirectional translation, discovery,
revocation, and restart guarantees.

Highest risks:

- Canonical addresses leaking through less obvious discovery or error paths.
- Nonce/replay-state reset errors during reconnect or migration.
- Alias lifecycle diverging from visa authority.
- Translation breaking protocol checksums or endpoint matching.
- Silent legacy fallback undermining an operator's privacy expectation.

Use phase gates rather than a fixed calendar estimate until Phase 0 establishes
the actual packet-path and compatibility work. Preserve existing security
semantics first; address privacy is an additional layer, not a replacement.
