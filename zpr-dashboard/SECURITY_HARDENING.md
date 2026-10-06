# Dashboard Security Hardening

**Scope:** Control Room, Simulator, the Policy Service integration, trusted LDAP
readers, organization workspaces, and browser-accessible operational data.

**Status:** working checklist, updated 2026-10-04. Checked items describe
implemented controls, not a claim that the whole dashboard is security-certified.
Keep this plan aligned with the broader ZPR [security model](../../zpr-dev-context/docs/SECURITY_MODEL.md)
and the separate organization-isolation work.

## Current Controls

- [x] Privileged editor/control requests require loopback access or a verified
  client-certificate connection; loopback browser requests validate their Host
  and Origin.
- [x] Control Service and Policy Service service-to-service APIs use mutual TLS.
- [x] Browser credentials and forwarding headers are stripped at the relevant
  service proxies; upstream admin credentials remain server-side.
- [x] Policy and assertion stores use revision checks, bounded request bodies,
  and private filesystem permissions. Assertion source, rule count, LDAP output,
  entry count, and returned violation examples are bounded.
- [x] Trusted-data assertions read from a configured server-side LDAP identity;
  credential-like attributes, URL-backed LDIF values, unresolved memberships,
  and unsupported partial membership data are rejected.
- [x] Assertion settings are stored as revisioned Policy Repository records
  scoped by organization. Evaluations capture the organization and LDAP base DN;
  source reads revalidate organization context, and changing organizations
  clears the prior in-memory result.
- [x] Machine-log responses are bounded and redact recognized credentials.
  Browser tests cover CSP, HTML/ANSI safety, source failures, and polling state.
- [x] A proposed remote browser-access contract is documented in
  [BROWSER_ACCESS_CONTRACT.md](BROWSER_ACCESS_CONTRACT.md). This is a design
  contract only; the mTLS gateway and all application listeners remain
  loopback-only.

## Open Work

### P0: Identity-Bound Directory Access

- [ ] **Enforce viewer-specific graph access on the server.** The Organizations
  graph currently renders the selected profile's directory snapshot in the
  browser; it does not apply LDAP ACLs for a viewer. LDAP can restrict entries
  and attributes by bind identity/group, but client-side hiding is not a security
  boundary. Identify the authenticated viewer, bind as that identity or use an
  explicitly trusted identity-to-authorization mapping, then return only
  authorized people, groups, attributes, and membership edges.
- [ ] Define behavior for unauthorized parents, groups, and edges so hidden
  directory structure cannot be inferred through counts, search results,
  completion catalogs, inspector details, or assertion results.
- [ ] Add adversarial tests with two users in different LDAP groups: verify
  denied entries/attributes and membership edges are absent from API responses,
  search, graph, popup details, exports, and errors. Include forged viewer IDs,
  nested groups, renamed groups, and ACL changes between requests.
- [ ] Until this is implemented, label the profile graph as seed/profile data
  and do not present it as a live, per-user ACL-filtered directory view.

### P0: Browser Authentication and CSRF

- [ ] Implement the proposed remote access contract before any public binding;
  the current client-certificate gateway prototype is not an authenticated
  administrative product.
- [ ] Before any non-loopback deployment, require an authenticated user session
  for browser routes; mTLS between backend services does not identify individual
  browser users. Define roles for read-only monitoring, policy editing/staging,
  directory editing, assertion administration, machine control, and organization
  activation.
- [ ] Bind mutations to the authenticated principal, require CSRF protection and
  origin checks, rotate/expire sessions, and use secure cookie attributes. Test
  unauthenticated access, role escalation, cross-site requests, session replay,
  and logout/revocation.
- [ ] Keep the local-only deployment guard as defense in depth; test IPv4/IPv6
  loopback, proxy Host handling, DNS-rebinding Host values, and malformed Origin
  headers. Do not treat “localhost” as user authentication.

### P0: Organization Isolation and Activation

- [ ] Complete per-organization runtime isolation for LDAP, policy/control
  services, machines, assertion source/results, logs, credentials, and runtime
  files. Per-organization assertion settings are implemented, but the active
  organization selector alone is not runtime or tenant isolation.
- [ ] Replace in-place reset-and-switch with ensure-ready, health-check, then
  switch; on failure retain the old active organization and its usable runtime.
- [ ] Serialize activation with scenario runs, machine-user sessions, assertion
  evaluations, and other mutations. Revalidate the active organization at the
  backend immediately before each write, not only in the confirmation dialog.
- [ ] Test two-organization noninterference across APIs, LDAP, policy revisions,
  assertions, logs, DNS, machine controls, caches, and failure recovery. Verify
  switch-away/switch-back preserves the intended state without leaking data.

### P1: LDAP Reader and Assertion Integrity

- [ ] Provision a dedicated least-privilege LDAP identity per organization.
  Allowlist readable object classes and attributes on the LDAP server as well
  as in the application. Store/broker bind secrets outside process arguments and
  logs; document rotation and revocation.
- [ ] Verify LDAPS certificate hostname and trust-chain behavior, bind-DN/base-DN
  consistency, search completeness, paging/size limits, and filter escaping.
  Fail closed on referrals, partial results, unsupported schema, ACL-hidden
  members, and oversized values; never turn these into a passing assertion.
- [x] Bind assertion settings and evaluation results to organization and
  revision; expose source snapshot timestamps and clear the in-memory result on
  organization changes.
- [ ] Distinguish a current result from one whose trusted-source snapshot has
  changed or whose assertion revision is no longer current. Require an explicit
  rerun before presenting stale results as current.
- [ ] Define authorization for viewing attribute names, coverage counts,
  violating subject identifiers, and assertion history. Do not return attribute
  values or broad LDAP snapshots to the browser by default.
- [ ] Test scheduler shutdown, duplicate workers, timeout/cancellation, LDAP
  outage recovery, clock jumps, concurrent revision writes, and source switching.
  Add an explicit, tested retention policy before making results durable.

### P1: Proxies, Inputs, and Resource Limits

- [ ] Maintain a route-by-route proxy allowlist for destination, method, path,
  headers, redirects, DNS resolution, and response size. Test SSRF, DNS rebinding,
  redirect escape, alternate IP encodings, and proxy-header spoofing.
- [ ] Apply consistent request, response, concurrency, and timeout limits to
  snapshot fan-out, LDAP queries, policy/compiler work, assistant requests, logs,
  directory imports, and assertion evaluation. Ensure cancellation reaches child
  processes and container commands.
- [ ] Add parser fuzzing/property tests for assertion syntax, LDIF handling,
  policy input, and any new filter/query grammar. Test Unicode normalization,
  duplicate identifiers, integer boundaries, and injection payloads.
- [ ] Review information exposure in errors, browser-visible diagnostics, logs,
  source maps, stack traces, and test artifacts. Keep secrets out of URLs,
  command lines, environment dumps, screenshots, and telemetry.

### P1: Supply Chain, Operations, and Verification

- [ ] Add dependency vulnerability/license review, automated update policy,
  reproducible lockfile checks, and an SBOM for release artifacts. Pin CI actions
  and downloaded tools to reviewed immutable versions/digests where practical.
- [ ] Add security headers and CSP regression checks to every served UI mode;
  inventory inline styles/scripts and avoid relaxing CSP to make a feature work.
- [ ] Define operator backup, restore, key rotation, incident response, and
  audit requirements for policy records, assertion history, LDAP credentials,
  and organization workspaces. Audit actor, action, target, result, and time
  without recording secret values or unnecessary personal data.
- [ ] Run Go and browser suites in CI, retain failure traces safely, and add
  authenticated integration tests against a disposable LDAP service with real
  ACLs. Keep fixture-only browser checks distinct from security acceptance.
- [ ] Run periodic threat-model review and penetration testing before claiming
  the dashboard safe for remote or multi-tenant deployment.

## Release Gates

- **Local single-user use:** loopback guards, current tests, protected local
  files, and documented trusted-source behavior.
- **Remote browser access:** P0 browser authentication/CSRF and per-viewer LDAP
  authorization must pass; loopback-only assumptions are insufficient.
- **Multi-organization use:** P0 organization isolation and failure-safe switch
  tests must pass with two concurrently configured organizations.
- **Security claim:** applicable P1 integration, operational, dependency, and
  adversarial tests must be in CI and pass on the release candidate.