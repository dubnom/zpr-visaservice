# Remote Browser Access Contract

**Status:** Proposed production contract; not implemented or approved for rollout.

This contract defines the minimum boundary for browsers outside the ZPR network to reach administrative web interfaces. It does not authorize public exposure of the current gateway or application ports.

## Current State

The optional Browser Access Gateway is a local mTLS prototype. It accepts a browser client certificate, routes the fixed `control.localhost` and `simulator.localhost` hosts to loopback upstreams, and strips client-supplied forwarding and credential headers. It does not establish an individual user identity, roles, or per-user permissions. A client certificate trusted by its CA is transport authentication only.

Control Room and Simulator continue to listen on `127.0.0.1:8787` and `127.0.0.1:8788`. The optional gateway defaults to `127.0.0.1:8443`. Keep all three loopback-only. Production rollout is deferred.

## Required Public Boundary

1. External browsers connect only to a managed HTTPS gateway on an explicitly approved public hostname. No application, Control-Service, Policy-Service, LDAP, logging, machine-control, or admin API listener is directly internet-reachable.
2. The gateway uses TLS 1.3 or later, a managed server certificate valid for the requested hostname, HSTS on the production hostname, and an approved renewal and key-rotation process.
3. The gateway routes only an explicit allowlist of canonical hostnames and paths to fixed private upstreams. Unknown hosts, paths, methods, redirects, and upstreams fail closed. User-controlled `Host`, `Origin`, `Forwarded`, `X-Forwarded-*`, identity, and authorization headers are never trusted.
4. The gateway authenticates named users with the approved organizational identity provider using OIDC Authorization Code with PKCE. Require MFA for administrative roles. mTLS may be an additional device control, but a shared or client-CA certificate alone is not user authentication.
5. Sessions use `Secure`, `HttpOnly`, appropriately scoped `SameSite` cookies, bounded idle and absolute lifetimes, rotation after authentication or privilege change, and server-side logout/revocation. Tokens and credentials must not appear in URLs, browser storage, logs, or referrers.
6. Every static administrative page and every API, event stream, and download route is covered by the same authentication and authorization policy. There is no alternate unauthenticated route or direct-upstream path.
7. The authenticated principal reaches application authorization through a trusted, integrity-protected channel. The gateway strips any client-supplied identity context. Application APIs enforce authorization themselves; hiding controls in the browser is not authorization.
8. Mutating requests require CSRF protection, exact Origin validation, and appropriate content-type and request-size limits. Cross-origin API access is disabled unless separately reviewed and specified.

## Authorization Contract

Access is deny-by-default and assigned to named principals or approved groups. At minimum, the implementation must distinguish:

- **Monitor reader:** read-only topology, actor/service inventory, denials, security findings, and logs that the principal is authorized to see.
- **Policy operator:** policy and assertion operations, with evaluation and staging permissions explicitly separated from viewing.
- **Directory operator:** directory reads and edits authorized by server-side LDAP ACLs; client-side filtering is never sufficient.
- **Simulation operator:** simulated machine, workload, scenario, and organization-activation controls; this role grants no live-network administrative access.
- **Platform administrator:** narrowly scoped gateway and service administration, not implicitly granted by the other roles.

Each API route must map to an explicit permission. Read-only access does not imply access to all personal or sensitive attributes. A request that lacks a supported permission is rejected by the server and audited.

## Audit And Operations

Record authentication success/failure, principal and role changes, authorization denials, mutation attempts/results, session revocation, and gateway/upstream health. Audit entries include timestamp, principal, action, target, result, and correlation ID, but not credentials, tokens, private source values, or unnecessary personal data. Define retention, access, backup, incident response, certificate/key rotation, and emergency revocation before rollout.

Apply per-principal and per-source rate limits, bounded concurrency, upstream timeouts, and response limits. Health checks reveal only service readiness, not administrative data. Production deployment requires monitored alerts for authentication failures, authorization denials, configuration drift, and upstream exposure.

## Release Gates

Public rollout remains blocked until all of the following are complete and reviewed:

- Approved identity provider, MFA policy, role/group mapping, session lifecycle, logout, and emergency revocation are implemented.
- Every route and method is inventoried and tested against unauthenticated, read-only, operator, and administrator roles.
- CSRF, origin, Host/DNS-rebinding, proxy-header spoofing, redirect escape, SSRF, session replay, and rate-limit tests pass.
- Viewer-specific LDAP authorization and organization isolation pass adversarial multi-user tests; shared service credentials do not bypass viewer ACLs.
- Direct application and service listeners are verified private from the deployment network, including IPv4 and IPv6.
- Audit, retention, key rotation, backups, operational ownership, and incident response are in place.
- Security review and release-candidate Go, browser, and authenticated integration suites pass.

Until those gates pass, use the gateway only on loopback for controlled development. Do not set `ZPR_ACCESS_GATEWAY_LISTEN` to a non-loopback address or publish an administrative UI.
