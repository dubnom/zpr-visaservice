# ZPR Control Room

A local web dashboard for the Visa Service admin API. It polls the API and
displays Visa Service counters, node/link state, actors, services, active
visas, and recent denials. Policy records live in a separate SQLite journal;
saving a policy appends an immutable version and never installs it into the
running Visa Service.

The Trusted sources page explains each provider in terms of its owning actor,
service endpoint, and most recent real attribute lookup. **Working** means the
latest lookup succeeded; **failed** means it failed, and **unverified** means
no lookup has occurred. This is not a live connection probe. The page does not
browse provider records. Older Visa Service versions without the status
endpoint fall back to policy service descriptors and show **unreported**.
The local `demo_ldap` source optionally links to a separate phpLDAPadmin tab if
`ZPR_DEMO_LDAP_EDITOR_URL` is set to a `http://127.0.0.1:<port>/` URL. No other
source receives an editor link; the browser authenticates directly to the
editor with a separate LDAP admin login. The monitor does not proxy LDAP edits
or store the admin password.

## Trusted-service model

In ZPL, trusted services are service classes/contracts, not a synonym for
attribute providers. Authentication, logging/audit, attribute sources, and
the Policy Repository are peer trusted-service classes; each owns its own REST
interface and record/data model. The Policy Repository is a management
service, not an attribute source that policies query for actor attributes.

For this local implementation, the Policy Repository REST routes are mounted
in the Control Room process under `/api/policy`, and the SQLite repository is
the persistence adapter behind them. This keeps the API contract separate
from the UI while avoiding a second daemon during early development. It is not
yet a separately deployed ZPR trusted service and has no service-to-service or
per-user authorization. A later extraction can host that REST contract behind
its own ZPR service class, authentication, and permissioned record model
without changing the Policy page's category/record concepts.

## Policy records

The Policy page organizes named categories and records. Records have a kind,
content type, JSON metadata, and append-only numbered content revisions. The
schema is intentionally generic so later record types and a separate durable
database implementation can be added behind the same repository interface.
SQLite uses WAL mode, full synchronous commits, foreign keys, and a local-only
database file. Configure its location and the ZPL compiler configuration:

```sh
export ZPR_POLICY_DB_FILE='/path/to/private-directory/policy-records.db' # optional
export ZPR_POLICY_CONFIG_FILE='/path/to/policy.zplc'
export ZPR_ZPLC_BIN='/path/to/zplc' # optional when zplc is on PATH
```

By default, the journal is stored under the user's config directory in a
dedicated mode-`700` subdirectory. An explicit `ZPR_POLICY_DB_FILE` must also
be inside a directory inaccessible to group and other users; the database and
SQLite sidecars are created with owner-only permissions.

`ZPR_POLICY_SOURCE_FILE` is optional and imports one initial policy into a
`Policies` category the first time the database is empty. It is never written
back. Categories can be nested with `/` in `ZPR_POLICY_SEED_CATEGORY`.
Every non-empty policy create/save is checked by the configured ZPLC binary in
parse-only mode. Saves require the current revision number and append a new
revision transactionally; stale writers receive a conflict. A version note and
server-configured `ZPR_POLICY_AUTHOR` (default `local-operator`) are recorded.

The editor endpoints currently accept loopback requests only. This is a
single-operator local service, not production authorization: records do not
yet have per-user permissions. The repository boundary is the intended seam
for a future authenticated trusted-service API and a production database.
There is no deploy action; the Visa Service admin API exposes compiled policy
bundles, not their editable ZPL source.

Claude is optional and disabled unless the server has an Anthropic key:

```sh
export ANTHROPIC_API_KEY='...'
export ANTHROPIC_MODEL='claude-sonnet-4-5-20250929' # optional
```

When Claude is configured, the UI discloses that submitting a question sends
the current policy and chat history to Anthropic. The key stays server-side;
assistant responses are suggestions and are never applied automatically.

## Configure and run

Run from the `zpr-dashboard` module directory. Use a read-only admin API key;
the key and CA file remain on the server and are never sent to the browser.

```sh
export ZPR_ADMIN_URL='https://[fd5a:5052::1]:8182'
export ZPR_ADMIN_CA_FILE='/path/to/admin-ca.pem'
export ZPR_ADMIN_KEY_FILE='/path/to/read-only-api-key'
go run ./cmd/zpr-web-dashboard
```

Open `http://127.0.0.1:8787`. To choose another local port, pass
`-listen 127.0.0.1:9000`. The server defaults to localhost so the browser UI
and its same-origin API proxy are not exposed to the LAN.
The Map is the default page. A compact row of live visa, node, decision, and
actor/dock metrics remains above the active page; Visa Service health and
uptime appear alongside the refresh controls.

The admin API URL must be HTTPS, and its certificate chain must validate
against the configured CA. Existing ZPR demo admin certificates are generated
with a common name but no subject alternative name, matching the current
`vs-admin` client behavior. For certificates with a DNS subject alternative
name, set `ZPR_ADMIN_SERVER_NAME` to enforce that name as well. Use
`ZPR_ADMIN_API_KEY` for an inline key only in controlled local environments;
the key-file option is preferred.

## Development checks

```sh
go test ./cmd/zpr-web-dashboard
go test ./...
```