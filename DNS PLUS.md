# DNS PLUS: Policy-Aware ZPR Service Discovery

## Goal

Show a ZPR client only the DNS service records that current policy permits it
to discover. This is a service-discovery and information-minimization feature:
it makes the inventory of network services less visible to unauthorized
clients. It is not a replacement for ZPR connection authorization. A client
that already knows a service address must still be denied unless its flow is
allowed by policy.

The DNS service is inside the Visa Service security realm. It can use
Visa-Service-managed identity and policy decisions rather than attempting to
make BIND itself understand ZPR visas. The request must nevertheless be
associated with the *requesting client adapter*, not merely with the DNS
service adapter's own identity.

## Existing baseline

### Product ownership

Protected ZPR service DNS is part of the ZPR product and installation package.
ZPR feeds its records today and is intended to control its policy-aware behavior.
Ship its DNS service, ZPR adapter, publisher integration, configuration and
persistent state with ZPR, independently of Simulator. Company underlay/bootstrap
DNS remains separate so installation and login do not depend on prior ZPR
admission. This ownership decision does not claim that the complete DNS package
or the per-client filtering proposed below is implemented.

- `dns/bind9/zpr-dns.zpl` publishes `dns.svc.zpr` on TCP/53 and currently
  allows authenticated devices to reach it.
- `dns/bind9/README.md` describes a BIND 9 authoritative DNS service behind a
  normal ZPR adapter, with service records published from
  policy-authorized actor registrations.
- Visa Service maintains the live authenticated actor/adapter state and
  evaluates current policy. Its DNS publisher reconciles those records into
  BIND using RFC 2136 and TSIG.
- The current BIND query ACL is subnet-based. It does not implement
  per-request ZPR policy filtering.
- ZPR flow authorization remains authoritative for actual service access.

This work adds per-client visibility filtering to DNS answers; it does not
change the existing service registration or flow-authorization model.

## Proposed request and decision path

1. A client adapter sends a DNS query to the ZPR DNS service.
2. The DNS-side request handler obtains trustworthy context for the requester
   from the inbound ZPR connection. Prefer authenticated peer identity supplied
   by the adapter/runtime. If only a ZPR source address is available, the
   Visa Service must resolve that address to a currently authenticated adapter
   session and reject unknown, expired, ambiguous, or stale mappings. Never
   accept a client-supplied adapter CN as proof of identity.
3. The DNS handler sends a narrow, authenticated request to Visa Service:
   requester identity/session (or verifiable source address), query name,
   query type, and an anti-replay/request context as required by the existing
   internal API conventions.
4. Visa Service evaluates the current policy against the requester and the
   service/resource represented by each candidate DNS record. It returns an
   explicit allow/deny decision and, for allowed discovery, the permitted
   answer data or permitted record identifiers.
5. The DNS handler returns only permitted records. It must not expose denied
   service names through answer, authority, or additional sections, alternate
   record types, DNS error details, or an unauthenticated statistics endpoint.
6. Any later connection to a discovered address is independently checked by
   normal ZPR visa/flow authorization.

The source identity must refer to the actual peer on the query flow. The DNS
adapter's own authenticated identity is not a substitute for the requesting
client's identity. Confirm how the current PH/Visa Service transport exposes
that peer identity before choosing the API shape. If the current DNS handler
cannot reliably obtain the requester's identity, identity propagation is a
prerequisite; do not ship a source-address lookup that can be spoofed or
confused by shared relays.

## Component boundary

Keep policy ownership in Visa Service:

- **Visa Service** is authoritative for authenticated adapter identity,
  current sessions, current policy, and the decision to disclose a record.
- **DNS request handler** obtains verified request context, asks Visa Service
  for a decision, and constructs the DNS response.
- **BIND 9** remains the authoritative DNS engine and record publisher. It may
  continue to store/publish the complete service inventory internally, while
  the request path filters responses before they reach clients.
- **ZPR policy/visa enforcement** continues to authorize actual network
  connections, regardless of DNS outcome.

Prefer a small policy-aware DNS front end in the DNS service adapter (or a
supported BIND integration if its query interface can receive trusted ZPR
peer context and synchronously enforce a decision). Do not assume BIND's
standard ACLs, views, RPZ, query logging, or dynamic-zone update hooks provide
the necessary Visa Service identity and per-query policy hook by themselves.
Select the integration only after a prototype establishes that query identity
survives the PH/BIND boundary.

Do not make Control Room, Simulator, or a DNS management UI a runtime
dependency. The production DNS path must operate with Simulator unavailable.

## Policy semantics

Define discovery over a canonical service resource, not arbitrary DNS text:

- Map a registered service record (for example,
  `service.svc.zpr`) to its canonical ZPR service identity/resource.
- Evaluate whether the requesting adapter is allowed to discover that
  resource under the current policy.
- Apply the same decision to all names and record types that reveal that
  service: A, AAAA, PTR, aliases, SRV/TXT if introduced, and relevant
  authority/additional data.
- Keep the DNS server's bootstrap identity/address available through explicit
  bootstrap configuration; DNS cannot be required to discover the server
  needed to query DNS.
- Treat infrastructure records (zone apex, SOA/NS, DNS service records) as an
  explicit policy category. Do not accidentally leak the full service list
  through zone transfers, AXFR, ANY, or diagnostics.
- Do not return a synthesized address or stale positive answer for a denied
  lookup. Choose a consistent non-disclosing response (normally NXDOMAIN or
  NODATA) and avoid making policy state distinguishable through error text.

The current BIND zone contains registered service records. The policy API
should evaluate the registered actor/service behind each record, rather than
infer authorization from a suffix or trust a caller-provided DNS name.
Adapter identity records and reverse records need explicit semantics: decide
whether the client may discover adapter identities, and ensure PTR answers do
not reveal a service the client could not discover by its forward name.

## Internal API contract

Add a private Visa Service operation (exact transport to follow existing
internal API conventions) with these properties:

- Authenticated and authorized for the DNS service caller; not exposed as a
  public unauthenticated endpoint.
- Binds the decision to verified requester identity/session and current
  policy revision.
- Takes normalized query/resource information, not an arbitrary policy
  expression.
- Returns a typed allow/deny/unavailable result, permitted record data or
  identifiers, and policy revision/decision metadata useful for safe caching
  and audit.
- Rejects stale sessions, malformed identities, unknown resources, ambiguous
  source-address mappings, and stale policy revisions explicitly.
- Supports batching a small candidate RRset if needed to avoid one RPC per
  record, without changing decision semantics.

The current `ph-cli` Unix control socket is an administrative API (counters,
links, logging, and node information); it is not currently this
Visa-Service policy API. Avoid exposing management-socket privileges to
ordinary DNS request processing. The DNS adapter should use the existing
authenticated Visa Service protocol or a narrowly scoped internal extension,
not shell out to `ph-cli`.

## Cache, freshness, and failure behavior

Start with no application-level positive answer cache unless performance
measurements require one. BIND's recursive cache is disabled in the current
authoritative-only profile; preserve that property for policy-filtered
answers.

If caching is later justified:

- Key by verified requester identity/session, canonical query name and type,
  and the policy revision (or a Visa-Service-issued authorization epoch).
- Never share a client-filtered answer across identities.
- Invalidate on policy revision, actor/service registration changes, session
  expiry/revocation, and address reassignment.
- Bound TTL to the smaller of DNS record TTL and policy decision lifetime.
- Do not retain positive answers beyond the session or policy revision that
  authorized them.
- Cache negative results only with equivalent identity/revision scoping.

On Visa Service timeout, disconnect, malformed response, unknown requester,
or stale policy state, fail closed for service records: return no discoverable
service data and emit an operationally useful, rate-limited error. Do not fall
back to the unfiltered BIND zone. Handle authoritative zone metadata according
to the explicitly chosen public/bootstrap policy. Keep the normal ZPR
connection enforcement unchanged.

## Security and privacy requirements

- The requester identity is derived from authenticated ZPR session context;
  never trust DNS payload fields, reverse DNS, source addresses from
  untrusted forwarding headers, or caller-selected CNs without Visa Service
  verification.
- Secure and scope the DNS-to-Visa-Service credential to this operation.
- Ensure the internal decision API cannot be reached by arbitrary adapters.
- Enforce filtering for UDP and TCP DNS and all supported query types.
- Consider DNS-over-TCP framing, multiple questions, truncation/TC fallback,
  EDNS, DNSSEC behavior, and retries; identity and policy must survive retry
  paths.
- Prevent policy-filtered response leakage via BIND statistics, query logs,
  errors, AXFR/IXFR, zone-transfer credentials, and shared caches.
- Rate-limit policy lookups and malformed queries without converting
  infrastructure overload into unfiltered answers.
- Log decision metadata (requester pseudonymous/stable ID as permitted,
  resource, allow/deny, policy revision, latency, outcome), not unnecessary
  full query contents or credentials.
- Keep TSIG publisher and read-only transfer credentials separate from the
  DNS request authorization path.

## Implementation phases

### Phase 0: Confirm identity propagation and policy model

- Trace a real DNS request from adapter to the BIND-facing process and identify
  the source/peer context available at each hop.
- Verify whether PH/Visa Service can correlate that context to one live
  authenticated adapter session, including through the DNS adapter, TUN, and
  any local DNS stub/relay.
- Identify the existing policy evaluator and the canonical service identity
  represented by each `svc.zpr` record.
- Specify visibility semantics for forward service records, adapter records,
  PTR, SOA/NS, AXFR, and unknown names.
- Record the threat model and API authorization boundary before implementation.
- **Exit gate:** a test proves that one authenticated client's query cannot be
  mislabeled as another client's identity, including when requests use the
  same DNS server and when a source address is unknown or reused.

### Phase 1: Design and prototype the decision seam

- Choose between a DNS front-end in the adapter and a BIND-integrated query
  hook only after validating available BIND/PH interfaces.
- Define the narrow internal Visa Service request/response contract, failure
  semantics, and policy-revision behavior.
- Prototype one query name with two clients and two policy outcomes. Verify
  allowed answer, denied non-disclosing response, and fail-closed Visa Service
  outage behavior.
- Confirm how BIND authority data and zone-transfer endpoints avoid bypassing
  filtering.
- Measure added query latency and Visa Service request volume.
- **Exit gate:** end-to-end prototype uses actual authenticated peer identity
  and cannot return the unfiltered answer on any error path.

### Phase 2: Implement Visa Service decision API

- Reuse the current policy evaluation and live actor/session registries; avoid
  maintaining a second policy snapshot in DNS.
- Bind each decision to the live session and current policy revision.
- Return explicit outcomes; do not turn missing actor/service/policy state into
  allow.
- Add unit tests for allow, deny, disconnect, unknown actor, expired session,
  removed service, changed policy, malformed request, unauthorized DNS caller,
  and API outage.
- Add concurrency/race coverage for session revocation and policy replacement
  during a lookup.

### Phase 3: Implement DNS answer filtering

- Obtain immutable verified requester context for each query.
- Canonicalize names and RR types and map records to policy service resources.
- Filter answer, authority, and additional sections consistently.
- Ensure PTR/alias/alternate-type queries obey equivalent discovery decisions.
- Preserve only explicitly allowed bootstrap and zone-control records.
- Fail closed and expose useful service-health telemetry on dependency failure.
- Add no shared response cache initially.

### Phase 4: Test the complete path

- Unit tests for identity binding, policy decisions, RRset filtering, name
  normalization, record types, cache isolation (if applicable), and failure
  paths.
- Integration tests with actual PH adapters, Visa Service, DNS adapter,
  BIND, and disposable service registrations.
- Test two clients with different policy views, then revoke one client's
  permission while sessions remain active.
- Test policy updates and service joins/leaves while DNS queries are in flight.
- Test unknown/reused addresses, forged identity claims, and stale sessions.
- Test UDP/TCP, large/truncated responses, retries, EDNS, reverse lookups,
  AXFR/IXFR denial, and BIND statistics exposure.
- Run with Simulator unavailable for the production Visa Service/DNS
  integration tests; use Simulator only for explicitly isolated end-to-end
  scenarios.
- Measure baseline and filtered query latency, QPS, and Visa Service load.

### Phase 5: Controlled rollout

- Gate the feature behind an explicit DNS configuration flag; keep existing
  broad authenticated-device behavior only during a documented migration
  window.
- Deploy to a test organization with at least two distinct policy profiles.
- Compare decisions with expected policy and inspect only privacy-approved
  decision metadata.
- Verify policy update/revocation propagation, operational alerts, rollback,
  and that no stale filtered answer survives.
- Enable per organization/service only after the identity and fail-closed exit
  gates pass.
- Remove the transitional broad behavior after operators confirm policy
  coverage and bootstrap records.

## Acceptance criteria

1. Two adapters querying the same name receive different answers when policy
   grants them different discovery permissions.
2. The DNS service proves each request's identity from trusted ZPR/Visa Service
   state; a client cannot choose or forge its identity.
3. Unknown, expired, revoked, stale, or ambiguous identities never receive
   filtered service records.
4. Policy changes affect subsequent queries within the documented freshness
   bound, including active sessions.
5. No shared cache, reverse lookup, alternate RR type, AXFR/IXFR, statistics
   endpoint, or error path leaks records hidden from that client.
6. A DNS outage or Visa Service decision failure does not produce an
   unfiltered fallback.
7. Direct connection attempts to hidden services remain governed by ordinary
   ZPR visa/flow authorization; DNS filtering does not weaken or replace it.
8. Production services do not depend on Simulator availability.
9. Query latency and Visa Service load remain within explicitly agreed
   operational budgets.

## Open design decisions

- Which existing ZPR event/transport supplies an unforgeable requester
  identity to the DNS query handler?
- Can the current DNS adapter receive per-request peer identity, or is a
  PH/Visa Service identity-context extension required?
- Should hidden names return NXDOMAIN or NODATA, and what DNSSEC behavior
  preserves correctness without disclosing hidden RRsets?
- Which records are deliberately discoverable to every authenticated client:
  zone apex, SOA/NS, DNS bootstrap address, and DNS service identity?
- Are adapter identity records discoverable under the same policy as their
  services, or a separate policy resource?
- What latency, decision freshness, QPS, rate-limit, and availability budgets
  are acceptable before introducing a revision-scoped cache?
- Which BIND integration is supportable in the deployed BIND version, and can
  it receive trusted requester identity without a custom unmaintained fork?
- Which telemetry and audit retention rules apply to client DNS query names?

## Non-goals

- Replacing Visa Service policy or ZPR flow authorization with DNS.
- Treating hidden DNS names as secret credentials.
- Changing service registration, address assignment, or TSIG update authority
  except where needed to ensure filtered query results cannot be bypassed.
- Adding a Control Room or Simulator runtime dependency.
- Enabling unrestricted BIND recursion or an underlay resolver fallback.
