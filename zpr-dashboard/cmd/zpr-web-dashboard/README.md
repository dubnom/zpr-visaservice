# ZPR Control Room and Services

The browser Control Room is a UI only: it calls same-origin `/api/*` routes
and carries no Visa Service or Policy Repository credentials. The
Control-Room process serves the UI and proxies those routes to Control-Service
over mutual TLS. Control-Service owns the Visa Service Admin client and proxies
Policy Repository requests to Policy-Service over mutual TLS. Policy-Service
owns its REST listener, SQLite journal, and ZPLC configuration. All three
processes use the same Go executable with different `-mode` values and remain
separate processes and trust boundaries.

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

The Policy Repository exposes its own REST interface in the Policy-Service
process. The Control-Service is its mutually authenticated client; the browser
only sees the Control-Service origin. Live dashboard reads are aggregated by
Control-Service from the Visa Service Admin API; the browser has no credentials
for either upstream API. This separates the service boundary and storage
process, but does not yet provide user-specific record permissions: all local
Control Room operators still share the Control-Service identity. The Policy
Repository is not an attribute source queried during ZPL policy evaluation.

## Policy records

The standalone `/policy-browser.html` page browses policy and assertion records
without editing or evaluation controls. It supports name/category search,
type/category filtering, syntax-highlighted source, saved revisions, and assertion
schedule metadata. Organization changes clear the previous selection; fetch
failures retain the last successfully loaded source and show an error. All viewer
requests are GETs. This is a read-only UI, not a new authorization boundary;
the existing Control-Service authentication still applies.

The page uses a reusable `<zpr-policy-browser>` custom element. Include the
shared styles and component script, then supply the repository API base:

```html
<link rel="stylesheet" href="/app.css">
<link rel="stylesheet" href="/policy-browser.css">
<script src="/policy-browser.js" defer></script>
<zpr-policy-browser api-base="/api/policy"></zpr-policy-browser>
```

Each instance has independent filtering, selection, requests, and refresh timers.
The component refreshes the catalog every ten seconds while the page is visible
and releases its timers and requests when removed. Its API base must expose the
existing catalog, record, and revision GET routes. No editor scripts are needed.

The Policy page organizes named categories and records. Records have a kind,
content type, JSON metadata, and append-only numbered content revisions. The
Policy-Service owns this data. The schema is intentionally generic so later
record types and a production database implementation can replace SQLite
behind the repository interface. SQLite uses WAL mode, full synchronous
commits, foreign keys, and a private database directory. Configure these
variables only for the Policy-Service process, never Control-Service:

```sh
export ZPR_POLICY_DB_FILE='/path/to/private-directory/policy-records.db' # optional
export ZPR_POLICY_CONFIG_FILE='/path/to/policy.zplc'
export ZPR_ZPLC_BIN='/path/to/zplc' # optional when zplc is on PATH
export ZPR_POLICY_SERVICE_CERT_FILE='/path/to/policy-service.crt'
export ZPR_POLICY_SERVICE_KEY_FILE='/path/to/policy-service.key'
export ZPR_POLICY_SERVICE_CLIENT_CA_FILE='/path/to/control-client-ca.crt'
export ZPR_POLICY_DEMO_CATALOG_FILE='/path/to/demo-policy-catalog.json' # optional
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

To load the optional fictional Northstar departments/services/policies,
point `ZPR_POLICY_DEMO_CATALOG_FILE` at
`cmd/zpr-web-dashboard/examples/northstar/demo-policy-catalog.json` and use
its adjacent `policy-demo.zplc` as `ZPR_POLICY_CONFIG_FILE`. The service
catalog entries are examples, not deployed endpoints. The included policies
use only LDAP department/title attributes currently mapped by `demo_ldap`;
they do not assume role-group membership is available to ZPL. See the
[Northstar demo notes](examples/northstar/README.md).

The editor endpoints currently accept loopback requests only. This is a
single-operator local Policy-Service deployment, not production authorization:
the listener requires a trusted Control-Service client certificate, but
records do not yet have per-user permissions. There is no deploy action; the
Visa Service Admin API exposes compiled policy bundles, not editable ZPL
source.

## Policy test API

`POST /api/policy/test` is a reusable, non-deploying evaluator API. Its request
contains candidate `source`, `actors`, and `services`; it has no organization,
directory, or simulator fields. The evaluator compiles the candidate with the
configured policy ZPLC settings, tests every actor/service pair with ZPT, and
returns source-authored compiled rule lines, per-service allow/deny/default-deny summaries,
and matching subject IDs and labels. The response includes `api_version` and
the candidate source hash. Actor `dimensions` (for example `user` and `device`)
allow clients to report unique identity counts without changing evaluator
semantics. Requests are limited to 500 actors, 100 services, and 2,000 pairs.

The editor uses `GET /api/policy/test/fixtures` as a separate demo adapter to
construct the generic actor/service request from its current test data. Other
clients can supply their own fixtures directly to `POST /api/policy/test`.
Candidate bundles are written only to a private temporary directory and are
never installed. Route-constrained rules currently return an explicit error
because ZPT does not yet resolve route-dependent decisions.

Claude is optional and disabled unless the server has an Anthropic key:

```sh
zsh scripts/configure-assistant.sh
```

When Claude is configured, the UI discloses that submitting a question sends
the current policy and chat history to Anthropic. The key stays server-side;
assistant responses are suggestions and are never applied automatically.
Run this from the dashboard module directory; the script prompts for the key
without echoing it and restarts only Control-Service. It does not store the key
for future restarts, so run it again if the stack is restarted. Do not put the
key in browser storage, the Policy
Repository, shell history, or this repository. The Policy page offers a
session-only on/off control, the server default model or Haiku 4.5, and
per-request output limits of 300, 600, 1200, or 2400 tokens. The displayed
input/output token totals come from successful responses and reset on page
reload; they are not a billing or organization-wide usage limit.

## Configure and run

Run all three processes from the `zpr-dashboard` module directory. Use
certificates from private local CAs for this demo; production deployments
should use their managed certificate lifecycle. Each service certificate
must be valid for the hostname in its corresponding service URL. The
Control-Room client certificate must chain to
`ZPR_CONTROL_SERVICE_CLIENT_CA_FILE`; the Control-Service client certificate
must chain to `ZPR_POLICY_SERVICE_CLIENT_CA_FILE`.

```sh
# Policy Repository service: owns SQLite, compiler config, and demo seed.
ZPR_POLICY_SERVICE_LISTEN='127.0.0.1:8789' \
ZPR_POLICY_SERVICE_CERT_FILE='/path/to/policy-service.crt' \
ZPR_POLICY_SERVICE_KEY_FILE='/path/to/policy-service.key' \
ZPR_POLICY_SERVICE_CLIENT_CA_FILE='/path/to/control-client-ca.crt' \
ZPR_POLICY_CONFIG_FILE='/path/to/policy.zplc' \
ZPR_ZPLC_BIN='/path/to/zplc' \
ZPR_POLICY_DB_FILE='/path/to/private-directory/policy-records.db' \
ZPR_POLICY_DEMO_CATALOG_FILE='/path/to/demo-policy-catalog.json' \
go run ./cmd/zpr-web-dashboard -mode policy-service
```

Start Control-Service separately. Its read-only Visa Admin credential and
Policy-Service client certificate stay in this process and are never sent to
the browser:

```sh
export ZPR_CONTROL_SERVICE_LISTEN='127.0.0.1:8790'
export ZPR_CONTROL_SERVICE_CERT_FILE='/path/to/control-service.crt'
export ZPR_CONTROL_SERVICE_KEY_FILE='/path/to/control-service.key'
export ZPR_CONTROL_SERVICE_CLIENT_CA_FILE='/path/to/control-room-client-ca.crt'
export ZPR_ADMIN_URL='https://[fd5a:5052::1]:8182'
export ZPR_ADMIN_CA_FILE='/path/to/admin-ca.pem'
export ZPR_ADMIN_KEY_FILE='/path/to/read-only-api-key'
export ZPR_POLICY_SERVICE_URL='https://127.0.0.1:8789'
export ZPR_POLICY_SERVICE_CA_FILE='/path/to/policy-service-ca.crt'
export ZPR_POLICY_CLIENT_CERT_FILE='/path/to/control-service-client.crt'
export ZPR_POLICY_CLIENT_KEY_FILE='/path/to/control-service-client.key'
go run ./cmd/zpr-web-dashboard -mode control-service
```

Start the browser-facing Control Room last. It has no upstream API
credentials; its client certificate is trusted by Control-Service:

```sh
export ZPR_CONTROL_SERVICE_URL='https://127.0.0.1:8790'
export ZPR_CONTROL_SERVICE_CA_FILE='/path/to/control-service-ca.crt'
export ZPR_CONTROL_CLIENT_CERT_FILE='/path/to/control-room-client.crt'
export ZPR_CONTROL_CLIENT_KEY_FILE='/path/to/control-room-client.key'
go run ./cmd/zpr-web-dashboard -mode control-room
```

Open `http://127.0.0.1:8787`. To choose another local port, pass
`-listen 127.0.0.1:9000`. The server defaults to localhost so the browser UI
and its same-origin API proxy are not exposed to the LAN.

For the local runtime, use `scripts/dashboard-stack.sh` instead of launching
the three modes independently. It builds one binary, starts the services in
dependency order, stores logs and PIDs under `.local-runtime/dashboard-stack`,
and performs protocol-aware readiness checks.
The Map is the default page. A compact row of live visa, node, decision, and
actor/dock metrics remains above the active page; Visa Service health and
uptime appear alongside the refresh controls.

The Visa Admin API URL must be HTTPS, and its certificate chain must validate
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