# Trusted-service REST provider

This command exposes one read-only, mutually authenticated HTTPS endpoint:

`POST /v1/attributes` with `Content-Type: application/json`:

```json
{"identities":[{"key":"device.zpr.adapter.cn","value":"alice"}]}
```

Response: `{"attributes":{"color":["red"]}}`. Names in `attributes` are **source-side**
names; Visa Service applies the policy's `returns_attributes` mappings. Unknown identities
return an empty object. All matching identities contribute attributes; conflicting values
fail the lookup rather than silently choosing one. Unavailable sources return non-2xx;
Visa Service must treat these as indeterminate. Neither request nor response is cached by
the provider API. The provider accepts at most 64 identities and a 64 KiB request.

Changes to the file or LDAP directory are visible at the provider's next lookup.
Visa Service refreshes an actor when its trusted attributes expire, or after an
admin cache flush changes the source revision. This version has no push
notification or background polling: a change is not immediately reflected in
an unexpired actor record. On a failed refresh, expired attributes cannot
authorize access; a revision-stale lookup is treated as indeterminate.

Run a file-backed provider using the same identity-keyed JSON format as the built-in
Visa Service file source:

```sh
go run ./cmd/zpr-trusted-service -listen 127.0.0.1:8443 \
  -cert /secure/provider.crt -key /secure/provider.key -client-ca /secure/visa-ca.crt \
  -file /secure/actors.json
```

For example, `actors.json` can contain:

```json
{"device.zpr.adapter.cn":{"alice":{"color":["red"]}}}
```

For LDAPS, replace `-file` with `-ldap-uri ldaps://directory.example:636`,
`-ldap-ca /secure/ldap-ca.crt`, `-ldap-base 'dc=example,dc=org'`,
`-ldap-bind 'cn=reader,dc=example,dc=org'`, `-ldap-password-file /secure/bind-password`,
`-ldap-identities '{"user.sub":"uid"}'`, and `-ldap-attributes 'department,role'`.
Only listed person attributes are returned. To resolve `groupOfNames` membership, also set
`-ldap-groups-base 'ou=Roles,dc=example,dc=org'`. The provider searches that subtree for
groups whose `member` matches the person's DN and returns each matching group's `cn` in the
multivalued source-side `role` attribute. Map it with `role -> device.demo.role` in the
Visa Service policy. Omitting the flag disables group lookups. LDAP identity values are
escaped in equality filters; attribute names are restricted to LDAP-safe names. The bind
account should have read-only access, and its password file must be readable only by the
service account.
LDAP connectivity requires a reachable TLS-enabled directory; the automated
tests cover the filter and provider contract but do not use a live directory.

Visa Service policy declares `api = "rest/1"`, mapped `returns_attributes`, and a TTL
over 60 seconds. The Visa Service config separately supplies the URL and TLS material:

```toml
[trusted_service_http.example]
url = "https://127.0.0.1:8443"
ca_cert = "/secure/provider-ca.crt"
client_cert = "/secure/visa-client.crt"
client_key = "/secure/visa-client.key"
```

Server verification is pinned to `ca_cert`; a valid Visa Service client certificate is
required. Do not expose this endpoint publicly or store private keys in the repository.