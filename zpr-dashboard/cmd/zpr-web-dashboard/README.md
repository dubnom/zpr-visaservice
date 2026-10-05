# ZPR Control Room and Services

The browser Control Room is a UI only: it calls same-origin `/api/*` routes
and carries no Visa Service or Policy Repository credentials. The
Control-Room process serves the UI and proxies those routes to Control-Service
over mutual TLS. Control-Service owns the Visa Service Admin client and proxies
Policy Repository requests to Policy-Service over mutual TLS. Policy-Service
owns its REST listener, SQLite journal, and ZPLC configuration. All three
processes use the same Go executable with different `-mode` values and remain
separate processes and trust boundaries.

## Operator Boundary

Control Room and Control-Service are production-facing operator components.
They must not call Simulator endpoints, read simulation manifests or profiles,
interpret Simulator sessions/workloads, or require Simulator to run. Simulator
may provision and exercise the normal ZPR services, but operator dependencies
must never point back to Simulator. Test new operator features with Simulator
unavailable; shared UI utilities must keep independent endpoints/data contracts.

Adapter Logs reads `ZPR_ADAPTER_LOG_CONFIG_FILE` on Control-Service. This is an
operator-owned JSON inventory, independent of any test environment:

```json
{
	"adapters": [
		{
			"id": "office-adapter",
			"name": "Office adapter",
			"sources": [
				{"name": "Link", "kind": "adapter", "file": "/var/log/zpr/office-adapter.log"},
				{"name": "Controller", "kind": "controller", "container": "office-controller"}
			]
		}
	]
}
```

A source selects an absolute regular-file path, a configured container's log
stream, or a container plus an absolute `path`. Browser requests cannot choose
files, containers, commands, or upstream URLs. Reads are bounded/redacted and
read-only; unavailable sources are reported separately. The endpoint returns
`adapters` with identities and typed sources, never Simulator machine/session
objects. Missing inventory is a log-configuration error, not a Simulator error.

The ZPR browser-based GUIs, including Control Room and Simulator, support desktop
and iPad-like tablet devices when the available viewport resolution is sufficient
to use the interface comfortably. Phone-sized mobile devices are not supported.
This support boundary applies only to ZPR web GUIs; it does not change support
expectations for terminal interfaces, APIs, command-line tools, or other clients.

Control Room and Simulator side menus can be condensed with the arrow button
and reopened with the same control. The active page name stays visible. Desktop
keeps a narrow menu with readable tab labels; iPad-like tablet layouts collapse
to a current-page bar when needed. Each app saves its own menu preference in
browser storage, including across Simulator page transitions and browser
Back/Forward navigation.

The Trusted sources page explains each provider in terms of its owning actor,
service endpoint, and most recent real attribute lookup. **Working** means the
latest lookup succeeded; **failed** means it failed, and **unverified** means
no lookup has occurred. This is not a live connection probe. The page also
offers a read-only browser for the configured LDAP directory, showing people,
groups, and approved attribute values. The Simulator exposes the same browser
for its active organization's directory. Neither view edits or publishes source
data. Older Visa Service versions without the status endpoint fall back to
policy service descriptors and show **unreported**.
The local `demo_ldap` source optionally links to a separate phpLDAPadmin tab if
`ZPR_DEMO_LDAP_EDITOR_URL` is set to a `http://127.0.0.1:<port>/` URL. No other
source receives an editor link; the browser authenticates directly to the
editor with a separate LDAP admin login. The monitor does not proxy LDAP edits
or store the admin password.

## Activity and Navigation

Simulator Activity reads the current Control Room feed while its page is active.
It polls every four seconds and shows request, approval, denial, and visa totals
alongside sortable recent-visa and denial tables. Manual Refresh is available
beside the live stream status. This is a bounded activity view, not a durable
audit archive. The Agents page calls the fleet **Devices** and filters by device
type.

Control Room groups Adapters, Actors, Services, Visas, Denials, and DNS in a
counted Status tab row. Its summary metrics appear on Map only. Map Fit centers
the rendered topology with padding while preserving glyph proportions; retained
components animate when layout bounds move unless reduced motion is enabled.

Simulator Scenarios groups cards by folder and places entries without a folder
under **Unfiled**. **Clear** is available only after a run reaches a terminal
state; it clears run history, not the scenario definition or machine logs.

## Security Review

Policy Analyze displays advisory warnings separately from compiler errors and
test decisions. The compiler's parsed-policy lint flags individual accessor
identities, duplicate/redundant or unrestricted grants, and empty service
groups. Group/role predicates are preferred; intentional infrastructure pins
remain valid. Candidate tests also warn about rules receiving no hits in the
evaluated population, which may reflect shadowing or incomplete fixture coverage
rather than a universally ineffective rule. No lint path depends on Simulator.

On the Map, newly observed visa grants pulse the requesting adapter with a green
ring and briefly expand/restore its glyph; new denials use red. Its dock wire
pulses the same color at the same time, then restores its original appearance.
Matching destination service badges pulse blue using the visa's address,
protocol and service-side port, including reverse visas. Colored outlines grow
25% and contract over 1.2 seconds, while glyphs expand 12% and restore. Pulse
lifetimes survive DNS/search redraws without restarting. Services sharing an
identical endpoint cannot be distinguished by the current visa feed.
Initial history and unchanged snapshots do not replay decisions. Multiple
decisions for an adapter in one snapshot coalesce, with denial taking precedence.
Reduced-motion mode keeps colored feedback without glyph scaling. These are
snapshot observations from the normal operator API, not a complete event stream
or a Simulator dependency.

The Control Room Security Review is a read-only triage view using the normal
Control Room Refresh, Pause, and refresh-interval controls. It consumes the
shared inventory snapshot and collects adapter logs only while its page is
active, without overlapping log reads or a separate polling timer. It groups
recent policy denials by source, flags counts of five or more, compares actors
and service identities/endpoints with a baseline stored in the current browser,
reports failed trusted-source lookups and nodes that are out of sync or have
not contacted the service in five minutes, and looks for explicit
authentication, certificate, and authorization failure phrases in adapter
logs. New identities and matching log lines are leads for operator review, not
proof of intrusion. The view does not block traffic or change policy, actors,
services, or logs; it only sees events exposed by the current snapshot and
available adapter logs.

The inventory comparison stacks the saved Baseline above Current in aligned
actor/service columns, with capture times and explicit added/removed identities.
The compact Show dismissed checkbox controls visibility of dismissed findings;
Select visible is the independent bulk-selection checkbox, not a scan toggle.

Dismissals are stored separately in the current browser. Operators can dismiss
one finding, selected findings, or all active findings, then show and restore
dismissed items. Changed evidence creates a new finding identity, so updated
activity can reappear. The inventory baseline is also browser-local, but
resetting it does not clear dismissals, and dismissing findings does not change
the baseline.

Entity and evidence IPs use the shared DNS record cache for display names, with
raw addresses on hover. Filtering matches both names and original addresses.
DNS-cache updates relabel existing findings without another log collection;
unresolved addresses remain visible rather than inventing a name.

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

The editor's picker uses a context menu for category/record creation, copy,
paste, duplicate, delete, and archive/restore actions. Right-click a category
or record, or press Shift+F10 or the Menu key on a focused item. The picker
tree itself supports the same shortcut when no row is selected. Every row supports the menu, including
its label and revision metadata. Category rows offer creation and paste;
policy/assertion rows offer record actions with protected/archive restrictions.
Arrow keys navigate the menu; Escape closes it
and restores focus. Existing unsaved-edit and protected-record checks still apply.

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

The local `scripts/dashboard-stack.sh` launcher selects the newer executable
between the compiler's release and debug builds unless `ZPR_ZPLC_BIN` is set.
Rebuild the selected compiler after grammar changes; passing source tests alone
does not update the compiler used by a running Policy-Service. Verify the live
`/api/policy/check` response before claiming the editor uses the new grammar.

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

To validate every organization catalog and composed runtime policy with the
current compiler, run from the dashboard repository:

```sh
ZPR_ZPLC_BIN="$PWD/../../zpr-compiler/target/release/zplc" \
ZPR_POLICY_RUNTIME_CONFIG_FILE="$PWD/../../.local-runtime/linux-integration/pregen/v6-1node-3actor-ping.zplc" \
go test ./cmd/zpr-web-dashboard -run '^TestAllOrganizationPoliciesPassConfiguredZPLC$' -count=1 -v
```

Catalogs use each organization's demo configuration. Shared one-node runtime
layers use `ZPR_POLICY_RUNTIME_CONFIG_FILE`; Great Lakes uses its multi-node
demo configuration. These are compiler checks, not proof of live forwarding or
policy deployment. Catalog imports do not overwrite existing saved revisions.