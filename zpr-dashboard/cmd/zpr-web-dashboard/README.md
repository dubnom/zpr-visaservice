# ZPR Control Room and Services

The browser Control Room is a UI only: it calls same-origin `/api/*` routes
and carries no Visa Service or Policy Repository credentials. The
Control-Room process serves the UI and proxies those routes to Control-Service
over mutual TLS. Control-Service owns the Visa Service Admin client and proxies
Policy Repository requests to Policy-Service over mutual TLS. Policy-Service
owns its REST listener, SQLite journal, and ZPLC configuration. All three
processes use the same Go executable with different `-mode` values and remain
separate processes and trust boundaries.

The assertion editor uses the policy editor's Browse/File toolbar, Analyze/Format
placement, analysis colors, and persistent results gutter. **Analyze** parses,
lints, and evaluates source against configured trusted sources, showing clickable
per-line results without entering a separate test mode. **Format** normalizes
spacing and statement layout, preserving comments and quoted values; it leaves
changes unsaved. Syntax-only clients can use `POST /api/assertions/analyze`;
the editor uses `POST /api/assertions/evaluate` for live analysis and
`POST /api/assertions/format` for formatting.

Policy compiler and evaluator lint warnings appear as clickable gutter markers,
including when analysis cannot obtain test fixtures. An amber-marked result also
opens its warnings alongside the result details. Fixture-generation failures are
reported as **Analysis unavailable**, not as policy compiler errors. The current
ZPT fixture format cannot represent LDAP values containing commas, braces or
control characters, or values over 2048 bytes. The fixtures endpoint omits such
attributes and lists them in `omitted_attributes`. Analyze still runs unless the
policy references an omitted attribute key (for example `user.l`), in which case
it reports **Analysis unavailable** with the attribute name. Whenever Analyze
turns red, a compiler or analysis error with no source line stays visible below
the editor.

## GUI controls

### World Map

Map offers **Topology** (the default) and **World Map**. Both use the same network
renderer, snapshot, component inspectors, visa routes, counts, search, legend,
zoom, pan, Fit and Auto-fit controls. Each view retains its own camera and Auto-fit
setting when switching. World Map anchors nodes to optional `latitude` and
`longitude` from the Visa Service's `node_details` contract; adapters and services
are arranged around their nodes, not assigned invented geographic coordinates.
Components without a located node appear outside the basemap. Selecting a
co-located node in the main graph opens a keyboard-accessible chooser for the
overlapping nodes. Missing or invalid locations have explicit inspection choices;
there is no separate node-location overview, placed-node count, or located-node
list above the graph. It does not fetch Simulator profiles
or geocode names. Switching views renders connectors and active route highlights
immediately, without waiting for the next poll.
Auto-fit and Fit use the network components' bounds, excluding the geographic
basemap; when no nodes or adapters are reported, the map remains available and
Fit falls back to the full basemap extent.
World Map Fit uses the current viewport aspect even after manual zooming, and
Auto-fit also reframes on viewport resize. Dock spacing is based on component
clearance rather than a fixed minimum radius, keeping connections compact while
leaving reported geographic node coordinates unchanged.
The World Map viewport uses the basemap's ocean color without the Topology dot
grid, including outside the basemap when panning or zooming. Topology retains
its separate light/dark backgrounds.
Adapters docked to a node are arranged in a compact arc in the largest gap
between that node's inter-node links, in both map views. The radius grows as
needed to fit the children, service rings and gateway clouds without wrapping
the arc across the link corridors. Nodes without inter-node links retain a
spaced ring.
Gateway components show a distinct connector to a labeled cloud for their
external network; connector endpoints meet the gateway and cloud outlines in
both views.

Configure coordinates as optional node properties in the Visa Service TOML,
keyed by the exact node actor CN, for example:

```toml
[nodes."node.example"]
latitude = 43.04
longitude = -87.91
```

Both properties must be supplied together, finite and within latitude
`[-90, 90]` and longitude `[-180, 180]`. Zero is a valid coordinate. Leaving
both absent preserves existing behavior. Invalid configuration fails to load.
Coordinates are operator-owned display metadata, not identity claims, routing
inputs or policy attributes. They require a Visa Service restart to change;
there is no GUI coordinate mutation endpoint.

The five bundled Simulator organizations explicitly seed approximate coordinates
for all eight runtime nodes. City-named sites use approximate city centers;
fictional sites use deliberately guessed locations recorded in each node's
`coordinate_note`. Northstar's North Campus is placed near Minneapolis; Redwood's
North Hub and Regional Yard near Eureka and Sacramento; Velocity's Performance
Lab near San Jose; Load Lab's bench near Chicago. Great Lakes uses Milwaukee,
Shenzhen and Tijuana. These are simulation placements, not verified company
addresses. Simulator provisioning copies the explicit profile coordinates into
Visa Service configuration under the deployed CNs (`node0.demo`, `node1.demo`,
etc.) using `scripts/render-node-geography.jq`. Control Room still reads only the
production Admin API: it neither loads these profiles nor guesses coordinates.
Existing running services retain their old configuration until redeployed.

Nodes without coordinates are listed as **Location not configured**, not placed
at an invented location. Invalid upstream values are explicitly listed as
**Invalid coordinates**. In the node-location overview, co-located nodes share a count marker that opens a
node chooser; **Nodes with locations** also provides ordinary inspection buttons
for small screens. Enter/Space activate markers and existing node inspection.

The checked-in `geography-land.svg` uses public-domain Natural Earth 1:110m countries,
projected equirectangularly into `1800 x 900` coordinates:
`x = (longitude + 180) * 5`, `y = (90 - latitude) * 5`.
The overview uses this projection; the shared graph scales it by two, with nodes
remaining exactly anchored, and fits the basemap and component bounds together.
The light ocean/land colors and country borders keep the network overlay readable.
The original `worldmap.svg` has CC-BY-4.0 attribution requirements but no
documented projection in the supplied file, so it is retained and not used for
coordinate placement. Regenerate the new asset with
`node scripts/build-geography-basemap.mjs INPUT.geojson OUTPUT.svg`, using
Natural Earth's `ne_110m_admin_0_countries.geojson`; source and license URLs are recorded
in the generated SVG. No external basemap request is required at runtime.

Page-specific Help opens a keyboard-accessible dialog beside uptime and refresh
controls in the upper-right corner. It explains the current view, common
workflows and recovery steps, and provides examples where useful. Policy and
assertion help also describes their syntax and the client, service and
trusted-source security model.

Status tables use automatic secondary column comparisons, so equal primary
values remain deterministic when snapshot ordering changes. Map adapter rings
rotate away from inter-node corridors; inter-node links render above dock links
with a clearance stroke to distinguish unavoidable crossings.
Returning to Map draws the latest cached snapshot immediately, even with updates
paused. Geometry is calculated only while the canvas is visible, so refreshes
on other pages cannot collapse connectors or corrupt the fitted viewport.
Manual zoom/pan and Auto-fit remain selected across navigation.
Before the first snapshot, the canvas identifies loading or a failed request;
an empty connected snapshot reports no nodes or adapters, while an incomplete
API response reports topology unavailable. Failed refreshes retain the last
successfully drawn topology and display the connection error.
Map **Auto-fit** is checked initially and fits the graph on refresh. Unchecking
it preserves the chosen viewport and zoom across refreshes; **Fit** remains a
one-time action without changing the checkbox. Rechecking Auto-fit fits
immediately and resumes automatic fitting. Manual panning, wheel zoom, and the
zoom buttons turn Auto-fit off and preserve the chosen camera on refresh.
Clicking the background without dragging or using Fit does not change the checkbox.
Map **Dark mode** is temporarily hidden; the canvas starts in its light theme.
Right-click an adapter to highlight its current outbound visas: matching service
registrations, endpoint actors, and ordered node routes reported by Visa Service.
Routes draw as thick, glowing blue lines with dashes that move along the path, and
routed components pulse with a blue glow; nothing else is dimmed. Reduced-motion
mode keeps a steady glow without animation.
Reverse visas use the original requester and service-side source port. Expired
visas are excluded; endpoint matches alone do not imply service authorization.
Right-click the same adapter or blank canvas to clear; right-click another adapter
to change focus. Focus follows refreshed snapshots. The status line explicitly
reports unavailable visa inventory or route data; no shortest path is inferred.
For same-node visas without a stored multihop path, matching unambiguous docking
nodes establish the local route.
Click/tap an adapter or service's visa-count badge (or press Enter/Space while it
is focused) to open its complete active visa inventory in the details panel,
including endpoints, direction, pair, expiry, policy and reported route. Inventory follows refreshed snapshots,
excludes expired and duplicate visas, and uses the same matching rules as the
count. Missing inventory is explicitly unavailable, never replaced with recent
decisions or treated as zero. Right-clicking a component behaves exactly like
right-clicking its count badge: adapters and services only toggle route
highlighting (no panel opens, so routes stay unobstructed); nodes open denial
telemetry when they show a denial badge, otherwise their visa counts.
Right-clicking blank canvas clears focus.
Forward and reverse visas are grouped as one connection in the details panel and
on the Visas page. Visa Service reports no pair ID, so a reverse visa pairs with
the forward visa that has the same protocol, swapped addresses, and a destination
port equal to the reverse source port (one-to-one, nearest creation time first).
The Visas **PAIR** column shows the partner ID; unmatched visas are marked
"No reverse visa" or "No forward visa", and visas without a reported direction
are left unpaired. Filtering keeps pairs together.
Legend items are buttons: click one to highlight every component or connection of
that type (nodes, visas, gateways, adapters, services, registrations, trusted
sources, docks or inter-node links); click it again to clear. Legend highlighting
combines with Search. Search, legend and right-click focus only add highlight
styling to matches; other components are never dimmed or faded.
While any search, legend or right-click highlight is active,
a **Clear highlight** button appears at the end of the legend and clears all of
them. On Map, Esc closes an open info panel; otherwise it clears all
highlighting. The info panel and route focus are independent: closing the panel
with × or Esc leaves the route highlight in place.
Connector endpoints intersect the actual SVG glyph boundaries, including rounded
node/service rectangles, circular adapters, visa diamonds, gateway polygons, and
cloud paths. Visible strokes, network clearance, and clickable hit areas use the
same endpoints. Connections track component movement and scale during animation.

Security navigation colors only the narrow side indicator for active
high-priority findings, retaining the normal link background and text, even while another
page is open. Repeated denials mean at least five denied requests or three denial
records from one source. They are high priority when the source is absent from
the live actor inventory or the attempts span at least three distinct
destination/protocol/port/reason combinations. A newly observed actor alone is
informational. Visiting Security acknowledges existing navigation alerts without
dismissing findings. The highlight returns for a new high-priority finding,
changed evidence, a newer reported denial, or a resolved alert that recurs.
Acknowledgements last for the current page session. Dismissing high findings
also clears their navigation highlight; filters do not hide new alerts from
navigation.

For exact finding triggers, generated indicators/evidence, severities, and
hard-coded tuning points, see the [Security Review Checks guide](../../SECURITY_REVIEW_GUIDE.md).

Adapter Logs keeps its controls alongside the section label. Each panel header
shows the configured adapter name in the title font, with a chevron beside it
that opens just the expanded source choice list, without a heading or extra
controls, positioned directly below the adapter title. Select a source to close it, or dismiss it with Escape or an outside
click. The page-wide type selector determines
adapter or controller logs; the selected source identity is also available in
the chevron tooltip. **Running only** excludes stopped, paused, unknown, and
unavailable sources, including disconnected retained history.
**Show all adapters** opens every configured log source for the selected
adapter/controller type; **Hide all adapters** closes those panels. Individual
panels can still be added or removed. The **Wrap** checkbox applies to every
panel, including maximized panels, and starts checked on each page load.
When no sources match the selected log type and running filter, the page states
that no adapters/controllers are available and disables adding/showing panels.
This differs from deliberately closing all panels when sources are available.

ZPR Config uses the editor toolbar, line-number gutter and modification
indicator, a dark syntax-colored TOML surface, and a separated line-number
gutter. **Analyze** checks TOML syntax, colors its success/error state, and places
line-specific parser errors in the gutter. Responses for edited source are
discarded. **Format** normalizes assignment spacing only after syntax
validation; **File** saves a versioned draft, opens a local TOML/ZPLC draft,
starts a new draft, or downloads the current source. Configuration selection and revision history
remain because the repository supports multiple versioned drafts. Validation
and saving never apply configuration to the runtime.

Trusted Sources includes an expandable LDAP tree built from actual distinguished
names and approved attributes, alongside the existing tables. Escaped commas
remain part of an RDN. Providers without entry DNs display an explicit
unavailable-tree message rather than an invented LDAP hierarchy. The browse
contract adds `directory.entries` containing `dn` and `attributes`; excluded
credential attributes remain excluded. Control Room loads trusted records on
the first visit, then only rereads them when the operator clicks the global
Refresh button on Trusted Sources. Automatic snapshot polling and navigation
back to the page preserve the records, filter, and expanded LDAP branches.
Diagnostics still follows global polling; standalone source pages retain their
own refresh action.

The Control Room **Updates (24h)** tab shows trusted-source LDAP change metadata
for the previous 24 hours. It lists the event time, operation, entry DN, rename
target, and attribute names; attribute values are never included. The page reads
the existing `zpr-trusted-service` `/v1/changes` feed through Control-Service,
which owns a separate mTLS client identity. The browser sees neither feed URLs
nor certificates or keys, and the feed must be configured independently of
Simulator.

Configure `ZPR_TRUSTED_CHANGE_FEEDS_FILE` on Control-Service with feed identifiers
matching the names in `ZPR_ASSERTION_SOURCES_FILE`:

```json
{
  "sources": [{
    "name": "great_lakes_ldap",
    "display_name": "Great Lakes LDAP",
    "url": "https://trusted-service.example:8443",
    "server_name": "trusted-service.example",
    "ca_file": "/absolute/path/to/.local-runtime/trusted-service-ca.pem",
    "client_cert_file": "/absolute/path/to/.local-runtime/control-service-trusted-client.crt",
    "client_key_file": "/absolute/path/to/.local-runtime/control-service-trusted-client.key"
  }]
}
```

With `scripts/dashboard-stack.sh`, keep the JSON file and TLS files under the
mounted `.local-runtime` directory and set their absolute container-visible
paths in that JSON plus `ZPR_TRUSTED_CHANGE_FEEDS_FILE` (for example,
`/absolute/path/to/.local-runtime/dashboard-stack/trusted-change-feeds.json`).
Restart Control-Service
with `sh scripts/dashboard-stack.sh restart-control-service` to load changes.
Protect the private key and the runtime directory from untrusted users.

The trusted-service listener must trust that Control-Service client certificate.
Set its `-ldap-changes-retention` and slapd `logpurge` age to 24 hours (neither
may be shorter);
the shipped retention default is 24 hours. The UI uses the feed's server-clock
`since=24h` bootstrap and cursor pagination. If history has expired or the feed
is unavailable, the page reports the error rather than replacing it with an
empty result.

Every Trusted Sources table (People, Groups and Attributes, on both the Control
Room page and the Simulator page) has sortable column headings with the same
treatment as other Control Room tables: click a heading for ascending order,
click again for descending. Empty values always sort last. Groups show `cn` and
`objectClass` as their own columns, ahead of members and the remaining
attributes. People rows are condensed to identity, name, title, unit and mail;
click a row (or press Enter on its identity) to show or hide all of that
person's attributes. Sort order and expanded people persist across filtering
and global polling.

## Operator Boundary

Control Room and Control-Service are production-facing operator components.
They must not call Simulator endpoints, read simulation manifests or profiles,
interpret Simulator sessions/workloads, or require Simulator to run. Simulator
may provision and exercise the normal ZPR services, but operator dependencies
must never point back to Simulator. Test new operator features with Simulator
unavailable; shared UI utilities must keep independent endpoints/data contracts.

Adapter Logs reads `ZPR_ADAPTER_LOG_CONFIG_FILE` on Control-Service. This is an
operator-owned JSON inventory, independent of any test environment:

Container sources report Docker runtime state independently of readable log
history: `docker logs` can succeed for an exited container. State is reported
per source, so a running controller does not make a stopped adapter running.
File-only sources have `unknown` runtime state; file readability cannot prove a
live process. They remain available with **Running only** disabled.

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

This inventory covers configured adapter and controller log sources. Simulator
Workers covers assigned application/service event files, while the
optional OpenObserve collector covers Visa Service counters, denials, and
process logs. The separate Diagnostics view covers ZPR nodes and configured
Visa Service/trusted-service sources without reading Simulator workloads.

For the bundled Compose-backed organizations, `dashboard-stack.sh` refreshes
the Adapter Logs inventory from the active organization's mounted runtime log
directory and runs its collector against that profile's read-only Visa Service
Admin API key. A single loopback-only OpenObserve store is shared, with every
record tagged by `zpr.organization.id`; collectors are switched with the active
organization. Control-Service queries are restricted to the active profile ID
and require a separate read-only OpenObserve query credential.

A source selects an absolute regular-file path, a configured container's log
stream, or a container plus an absolute `path`. Browser requests cannot choose
files, containers, commands, or upstream URLs. Reads are bounded/redacted and
read-only; unavailable sources are reported separately. The endpoint returns
`adapters` with identities and typed sources, never Simulator machine/session
objects. Missing inventory is a log-configuration error, not a Simulator error.

## Unified Diagnostics

The Diagnostics page consumes only Control-Service's provider-neutral
`GET /api/diagnostics` contract; browser requests never select a provider,
query, organization, or credential. Control-Service builds the source catalog
from Visa Service Admin actors, registered services, and trusted-service
status. It queries telemetry by OpenTelemetry `service.name` and
`service.instance.id`, using `zpr.source.type` for node, required-service,
trusted-service, and Visa Service identities. Optional server-owned
`ZPR_DIAGNOSTICS_SOURCE_MAP_FILE` maps catalog IDs to actual OTel identities.

The initial query adapter is OpenObserve. Configure it only on Control-Service
with JSON provider configuration and a private, owner-only query-token file:

```json
{
	"endpoint": "https://openobserve.example.test",
	"organization": "zpr",
	"logs_stream": "zpr_visa_service",
	"metrics_stream": "zpr_visa_service",
	"stale_after_seconds": 300
}
```

Set `ZPR_DIAGNOSTICS_CONFIG_FILE`, `ZPR_DIAGNOSTICS_USERNAME`, and
`ZPR_DIAGNOSTICS_TOKEN_FILE`; keep files under the mounted local runtime for
the bundled Docker deployment. Endpoint URLs must be HTTPS or HTTP loopback,
with no userinfo/path/query. Credentials remain server-side and provider
responses are bounded. The contract returns at most 100 sources, 50 log entries
and 50 metrics/source, 500 log entries and 1,000 metrics overall. Each provider
response is capped at 512 KiB; log bodies at 2 KiB; and attributes at eight
fields of 128 characters each. Search is capped at 200 characters. Sources
report available, stale, partial, or unavailable states and their latest
telemetry timestamp.

The OpenObserve collector accepts an operator-owned
`observability/diagnostic-sources.json` inventory to tail node/trusted-service
log files and numeric metrics snapshots, exporting them as OTLP with source
identity, source type, and active organization resource attributes. Direct
OTLP producers can export the same resource contract. See
`observability/openobserve/diagnostic-sources.example.json`,
`diagnostics/openobserve-provider.example.json`, and
`diagnostics/source-map.example.json`. OpenObserve remains replaceable behind
the Go provider interface; the Control Room contract contains no OpenObserve
query fields or Simulator data.

The Control-Service launcher does not read Simulator manifests or profiles.
Optional gateway annotations come from the operator-owned
`ZPR_PLATFORM_SERVICES` JSON setting and default to an empty list. Set
`ZPR_ASSERTION_LDAP_CONTAINER`, `ZPR_ASSERTION_LDAP_BASE_DN` and
`ZPR_ASSERTION_LDAP_BIND_DN` explicitly to enable Assertion Analyze and LDAP
attribute discovery. Use the directory's read-only service bind identity, not
its admin identity: the reader uses the configured directory credential file.
Preserve these operator settings when recreating Control-Service. The assertion
editor uses the standard white lower-left scrollbar corner on horizontal overflow;
Assertion Analyze shows one tag per source line, with ERR > FAIL > WARN > PASS
precedence. Clicking the tag shows all results and warnings on that line.
the demo LDAP editor URL is not forwarded to Control-Service.
For example, an operator may classify an already reported service for map
display with:

```sh
ZPR_PLATFORM_SERVICES='[{"service_name":"internet-gateway.svc.zpr","actor_cn":"internet-gateway","service_kind":"Gateway","external_network_connection":"public-internet"}]'
```

Include the setting when starting or restarting Control-Service, alongside the
active organization's Admin API connection settings. These annotations enable
gateway/cloud rendering but are operator-provided display metadata, not proof
that the external-network claim was verified from installed policy.

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

Control-Service can provide additional provider manager links with
`ZPR_PROVIDER_MANAGER_URLS`, a JSON object from trusted-service name to URL.
HTTPS is accepted for remote managers; plain HTTP is limited to loopback.
URLs containing credentials or unsupported schemes are ignored. The setting is
server-side and the browser receives only the validated link for each source.

## ZPR Config Drafts

The Control Room ZPR Config page stores versioned `configuration` records under
the Policy Repository's `ZPR Config` category. It validates TOML syntax and
preserves immutable revisions. Drafts are not runtime configuration: saving or
validating never applies, stages, or activates them.

## Operator Login Navigation

Operator sign-in preserves the current application path, query and navigation
fragment in per-tab session storage, restoring it after authentication. Failed
sign-in retries retain the original destination. Only same-origin application
locations are restored, never authentication endpoints or external URLs.
This preserves navigation, not unsaved editor contents across a full-page login.

## Gateway Drafts

The Control Room Gateways page edits one versioned JSON draft per installed
gateway, using the policy editor's layout. **Browse gateways** lists installed
gateway contracts with their adapter and latest draft revision. **File...**
saves, opens, downloads or discards a draft; **Analyze** validates the exact
source through `POST /api/gateways/config/check` (local JSON syntax errors get a
gutter marker); **Format** pretty-prints valid JSON; **History** reloads an earlier
revision; Find & Replace matches the other editors. Save draft (or Ctrl/⌘-S) is
enabled for new or modified, nonempty drafts. Save automatically runs Analyze and
stops on validation errors or superseded source/context; a valid result creates
a new revision via `POST /api/gateways/configs/{id}/revisions`. Gateway identity
fields come from the installed contract. Saving never activates the runtime
gateway. The editor uses Control Room APIs only and does not depend on Simulator.

Gateways opens in the raw JSON editor. **Form editor** switches to a separate
structured view for HTTPS origins, one path prefix per line, GET, HEAD, POST,
PUT, PATCH, DELETE and OPTIONS methods,
timeout and response limits; installed identity fields are read-only.
**Add Destination** stays above the **Destinations Allowed** list. Each unboxed
row contains **Base URL**, **Paths**, and **Remove**. Enter one path prefix per
line in Paths; Remove deletes the whole destination and its prefixes from the draft.
Save stores the edited draft only and does not alter runtime filtering.
New drafts still default to GET and HEAD. TRACE, CONNECT, duplicate methods
and empty method lists are rejected by Analyze/Save. Additional method choices
are draft settings only; Save does not configure either forwarding handler.
The fixed-upstream `gateway-service` forwards all seven methods on `/fetch`
and `/fetch/...`, including request bodies up to 2 MiB (larger requests get
HTTP 413 before contacting the upstream). `/health` remains GET/HEAD-only.
It forwards content/accept, conditional-request and idempotency headers, but
does not forward caller credentials, cookies or proxy/hop-by-hop headers.
Responses remain bounded to 2 MiB; redirects are returned rather than followed.
The separate `web-gateway-service` already forwards methods and bodies over
HTTP and HTTPS CONNECT tunnels, subject to its host/port allowlist. It does not
read saved drafts or enforce their method selections; HTTPS tunnel contents
are opaque to it.

Both Internet-egress modes share the independent forwarding core in
`gateway_forwarding.go`: hostname validation, method validation, redirect
handling, hop-by-hop header removal, and response forwarding. The fixed-upstream
mode deliberately uses selected headers and bounded responses; the web proxy
preserves end-to-end headers and streams responses. CLI mode names and existing
launch configuration remain compatible. The web proxy constructor consumes
explicit host configuration, not Simulator profiles or APIs. Simulator callers
may supply their configuration to it, but the forwarding core has no reverse
dependency. The browser-access gateway remains a separate application proxy.

Still pending: a shared deployed runtime configuration contract, reviewed
activation/rollback, and migration of fixed-upstream callers before retiring
that mode. Draft methods and paths cannot be enforced inside opaque CONNECT
tunnels without a separate HTTPS inspection/termination design.
**Raw JSON editor** returns to the source. Both views edit the same draft:
switching alone preserves exact source formatting; form edits preserve other
JSON fields and require fresh analysis. Unsupported JSON shapes or methods
must be corrected in Raw before opening the form, rather than silently discarded.
Form errors appear inside the form, while Raw uses the ERR gutter. Find/Replace,
Wrap and Format are raw-only; History, File operations and Ctrl/Command-S
work in either view. A new draft
starts with a blank origin: enter your intended HTTPS hostname before Analyze.
Wildcard hosts are not supported; `/` allows every path at that origin.
The same prefix may be allowed at different origins, but duplicates within one
origin are rejected. Editing destinations changes only the draft and requires
fresh analysis before saving; it does not read or change the live gateway's
allowlist.

Analyze preserves the submitted JSON formatting and reports source line numbers
for destination, path-prefix, method, schema and limit validation errors.
Source errors never appear in the top status area. Click the standard **ERR**
gutter marker to select the offending line and open its error details. Editing or switching
drafts clears the marker; live contract/inventory errors have no invented line.

The Policy editor is the blueprint for every source editor. Policy, Assertions,
Gateways, ZPR Config, Simulator Directory (LDIF) and Simulator Scenario
register one controller per source in `editor-page.js` (`window.ZPREditorPage`).
Scenario form and raw modes use the same controller; Assertions share Policy's
File/History chrome without registering duplicate menu listeners. Explicit
load, analyze, save and render adapters preserve each domain's service contracts.
Directory deliberately has no Analyze adapter.
The controller owns command/Save-shortcut bindings, named analysis scopes,
request-owned pending cleanup, identity/status and responsive source sizing.
Domain adapters retain cancellation, diagnostic rendering, warning details,
validation-before-save and publish/stage confirmations. Superseded cleanup
cannot release a newer analysis; disposing removes owned bindings and menus.
All six source viewports use the same bottom-inset/minimum-height calculation,
reschedule after opening, mode switches and resize, and defer to the full-page
Maximize layout while maximized. Small screens retain natural scrolling.
The shared core also
provides the identity row (name, History dropdown and modified dot), the **File…**
menu, the history menu, syntax highlighting, the gutter and the status line.
Editor pages have no kind label and no idle or "Select a…" placeholder text;
status appears only after an action. The Simulator directory and scenario
editors open as full pages rather than modal dialogs; scenario **Save**,
**Publish revision**, YAML/JSON conversion, **Discard**, **Delete scenario** and
**Close** live in its File menu, and the form/raw toggle sits beside Find &
Replace. All editors share the policy editor's gutter geometry (line height,
marker column and padding). Each editor toolbar places **Analyze** and
**Format** immediately after **File…**, with search utilities on the right.
History sits beside the file name in every editor; revision information stays
inside that dropdown rather than in a separate version label. Blank names show
**Untitled**, which is a reserved placeholder and cannot be saved as a name.
Analyze is blue while the current source still needs
analysis, green after success and red after errors; Format uses the policy
editor's green ready style.

Source-line errors appear only in the gutter, not as duplicate messages below
the editor. Hover a marker to read its diagnostic; clicking selects the source
line. Policy markers also open diagnostic details. Service/configuration
failures without a valid source line remain visible below the editor. The
editor frame has no separator or extra top padding above its controls. New
ZPR Config has no name box above Browse; Save asks for a name for an unnamed
draft (cancel submits nothing). Imported filenames supply a draft name, except
the reserved Untitled placeholder. The ZPR Config source area stretches to the
same bottom margin as Policy, and long documents scroll inside the editor.

Every editor with Find and Replace (policy, assertion, gateway, ZPR Config,
raw scenario source and directory LDIF) has a **Wrap** checkbox. Wrapping is
on by default and the choice is remembered per editor in browser storage. Line
gutters size each row to its wrapped height so numbers and markers stay aligned
with their logical lines; uncheck the box to restore horizontal scrolling.

## Activity and Navigation

Simulator Activity reads the current Control Room feed while its page is active.
It polls every four seconds and shows request, approval, denial, and visa totals
alongside sortable recent-visa and denial tables. Manual Refresh is available
beside the live stream status. This is a bounded activity view, not a durable
audit archive. **Workers** merges the former Agents/device inventory and Workload
Logs in a read-only view at `/machine-logs.html`; old `/agents.html` links redirect
there. Expand **Device and workloads** for owner, posture, container/controller
state, authenticated user and selected workload identities/states. Search,
device-type and running-only filters, source selection, the **Wrap** checkbox
(checked by default), pause/follow,
and maximize remain available. Lifecycle, workload assignment and login/logout
controls are no longer on this page; scenarios manage them. Missing telemetry is
shown as unavailable rather than inferred from log presence.

Control Room groups Adapters, Actors, Services, Visas, Denials, and DNS in a
counted Status tab row. Its summary metrics appear on Map only. Map Fit centers
the rendered topology with padding while preserving glyph proportions; retained
components animate when layout bounds move unless reduced motion is enabled.
Count badges overlap the upper-right glyph boundary, with Visa Service badges
centered on the diamond's upper-right edge. Status tables have bounded vertical
scrolling and sticky sortable headings so large inventories remain usable.

Control Room navigation groups Map through Log Manager under **Monitoring**,
the policy, assertion, gateway, directory and ZPR Config editors under
**Configuration**, and **Provisioning → Adapters** after a separator. The
Adapters page provides a memory-only invitation worksheet and local review.
When HTTPS/OIDC and independently verified delegation are configured, it loads
authorized catalogs, organization-scoped registry pages (up to 50 records), and
fresh read-only details with revision, key fingerprint, deadlines, and audit
identity/reason. **Check again** reloads the catalog and first registry page;
**Next page** follows the service cursor. Unavailable/denied reads are not an empty
queue. Navigation and session checks/loss/logout clear records and details, and
late responses are discarded. The form uses approved organization/type/profile
selects when a catalog is available; otherwise it remains an unvalidated worksheet.
Invitation creation defaults off and requires the explicit backend
`gui_invitation_creation_enabled` flag, authorized creator organization capability,
named-user grants at both services, and CSRF confirmation. A confirmed result shows
the audited invitation and its code once; closing/navigation/session checks/loss
erase the code. No code is saved or recovered from registry reads. Uncertain
responses lock further creation until explicit registry reconciliation; no retry
is automatic. It sends no email and enables no approve/reject actions.
Authorized admins can cancel active invited/pending requests from fresh **Details**
without a separate feature flag. Both services must grant `read`/`cancel` for
the organization; the catalog must advertise backend cancel capability.
Confirmation requires a reason and exact revision/fingerprint. Cancellation
records the named principal/reason/time, prevents subsequent claim, and never
revokes issued credentials. Lost responses, timeout, navigation or session loss
during cancellation are uncertain: no automatic retry; reopen fresh **Details**,
inspect/acknowledge the outcome, then reopen before any new decision.
Read access does not unlock mutations or imply live adapter connectivity.
It never calls Simulator.

Direct HTTPS with operator-configured certificates and OIDC is opt-in in
`-mode control-room`, currently restricted to a loopback listener/origin.
Sign in uses the native same-origin POST; callback creates an opaque secure
session cookie. The GUI displays the verified subject and configured enrollment
scope and offers CSRF-protected Sign out. Login is not mounted on default HTTP,
does not confer blanket authority over other operator APIs, and does not unlock enrollment:
verified named-user delegation to Control-Service is separately configurable.
The backend independently checks issuer/subject grants, signing key and verified
client-certificate pin, and a persistent nonce ledger; the shared certificate
alone is not administrator authority. Approval/rejection GUI controls remain
disconnected; creation has its separate default-off capability and cancellation
uses independent named-user/backend cancel grants.
See the [delegation configuration guide](../../README.md#independently-verified-named-user-delegation) and the
[HTTPS/OIDC configuration guide](../../README.md#direct-https-configuration).

The local stack now also supports a real [development operator login](../../README.md#local-development-operator-login)
using Dex on loopback and a dedicated user-trusted CA. The deployed Room URL is
`https://localhost:8787`; the old HTTP URL no longer works in this profile.
`dubnom` logs into the IdP using the protected local `operator-password` file.
The explicit global organization grant covers current/future organizations at
both services; enrollment still uses the independent configured backend catalog.
Editor requests use the session CSRF token in memory. Simulator remains local
HTTP and uses independent mTLS Control-Service access for Activity/activation,
not a browser session. The development gateway is not a substitute for this login.

Simulator navigation is ordered Organizations, Scenarios, Trusted Sources,
Activity, then Workers; this order is retained across page switches. The Simulator
root and brand link open Organizations.

Simulator Scenarios groups cards by folder and places entries without a folder
under **Unfiled**. **Clear** is available only after a run reaches a terminal
state; it clears run history, not the scenario definition or machine logs.
Running and cleanup states use saturated blue; failures use red. When a run
fails, the progress badge and summary retain the original failed run step even
after cleanup advances the step counter. Cleanup-only failures show their own
step instead.

During a running or cleaning scenario, running machines have miniature log
windows above their track headings. The previews show bounded recent text from
the existing Simulator log sources and link to **Workers** for full-size logs.
They refresh every three seconds after the previous request completes, stop
collecting outside active runs or after navigation, and reject obsolete
run/organization responses. Request failures retain the last-good preview with
an explicit error; a mismatched organization clears the previous content.
This is Simulator-only and adds no dependency to Control Room.

Load Lab places both scenario machines on the **Load Test Bench** node.
For Docker multinode profiles with explicit machine-owner mappings, each
machine's first configured owner must have a `location` matching exactly one
runtime node. Keep the owner's LDAP `l` attribute consistent with that profile
location. Single-node profiles without owner mappings (such as Northstar) use
the manifest owner and dock all machines to their sole runtime node, including
machines whose physical location is remote. Multi-node profiles require explicit
owner mappings; scenarios validate placement before any steps run.

Machine startup refreshes the machine-control proxy if Simulator's container
address has changed. Cleanup skips workloads on stopped machines and reports
offline controllers immediately instead of queueing commands that cannot run.
Traffic timeouts and policy denials still fail scenarios; these checks are not
relaxed by lifecycle recovery.

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
Matching destination service badges pulse green using the visa's address,
protocol and service-side port, including reverse visas. Colored outlines grow
25% and contract over 1.2 seconds for adapters and 2 seconds for services,
while glyphs expand 20% and restore. Six-pixel glowing outlines, tinted halos,
and thicker dock-wire feedback make decisions visible against the topology. Pulse
lifetimes survive DNS/search redraws without restarting. Services sharing an
identical endpoint cannot be distinguished by the current visa feed.
Initial history and unchanged snapshots do not replay decisions. Multiple
decisions for an adapter in one snapshot coalesce, with denial taking precedence.
Reduced-motion mode keeps colored feedback without glyph scaling. These are
snapshot observations from the normal operator API, not a complete event stream
or a Simulator dependency.

Component connections and their legend use solid lines; their colors still
distinguish dock, service, and network connections. Small badges beside adapters
and services show active visa counts. Zero counts use an empty outlined badge,
with the count still available to assistive technology and in the tooltip.
Adapter counts include either endpoint,
counting a self-connection once; service counts match the provider address,
protocol, and service-side port, including reverse visas. Services sharing an
endpoint share its count. These use the complete `active_visas` snapshot inventory,
not the ten-entry `recent_visas` history. Control-Service fetches visa details
with at most sixteen concurrent Admin API requests and excludes expired grants.
If the inventory or any detail is unavailable, `active_visas` is null and badges
show `?`, not a misleading zero or partial total. Visas removed between listing
and detail retrieval are skipped. The browser also excludes grants expired since
the snapshot and deduplicates IDs on each render.

Nodes instead show red buffered-denial badges: the number of currently cached
negative decisions reported by that node, not cumulative Visa Service denials.
Zero is an empty red outline; unavailable telemetry is `?`. The node inspector
shows **Local denial occurrences**, the cumulative number of requests suppressed
locally by the denial cache since restart/counter reset, separately from
Visa Service's approved/denied request totals.

Count changes in either direction briefly pulse the badge (including expiry to
zero), without replaying unchanged snapshots or treating unknown readings as
zero. Counts expand and contract twice over 2.4 seconds, using explicit SVG
coordinates and a colored glow, including the outline when a count drops to zero.
Reduced-motion mode fades the badge without scaling. New service grants
also pulse their service connector; adapter decisions pulse the dock connector.

#### Deploying node counters and denial metrics

Update the node packet handler to the version exposing `Buffered Denials` in
its management RPC counters. On the node host, run the operator-owned exporter:

```sh
sh scripts/node-denial-metrics.sh /usr/local/bin/ph-cli /run/zpr/control.sock zpr-core node-01
```

It emits an OTLP JSON resource-metrics payload to stdout. Schedule it at roughly
one-second intervals and send each successful payload to the operator-configured
collector's OTLP `/v1/metrics` receiver using the deployment's normal authenticated
telemetry transport. Never upload failed or partial output. The exporter needs
`jq` and access to the node management socket; it does not discover Simulator
containers or read simulation configuration.

Map the catalog ID `node:<actor CN>` to that exporter `service.name` and
`service.instance.id` in `ZPR_DIAGNOSTICS_SOURCE_MAP_FILE`. Control-Service reads
`zpr.node.denials.buffered` (gauge) and `zpr.node.denials.local` (cumulative sum)
through its existing provider-neutral diagnostics interface. Samples older than
ten seconds (or the configured shorter freshness window), future timestamps,
negative/noninteger values, missing metrics, and provider errors are unavailable.
The badge represents the latest fresh node observation; short-lived entries can
expire between samples. Polling does not reconstruct cache state or local hits
from Visa Service denials. Until runtime rollout and telemetry export are configured,
the inspector states the telemetry error and the Map displays `?`.

For deployments without a metrics collector, Control-Service can consume the
same OTLP payload from operator-owned local files. Set
`ZPR_NODE_DENIAL_METRICS_FILE` to a JSON object mapping node actor CNs to absolute
sample paths:

```json
{"node-01": "/var/lib/zpr/metrics/node-01.json"}
```

When that variable is unset, Control-Service checks for `node-denial-metrics.json`
beside `ZPR_DIAGNOSTICS_CONFIG_FILE`; if no such file exists it uses the
OpenObserve provider. An explicitly configured missing or invalid file is an
error, not a fallback. Configurations and samples are limited to 64 KiB.
Sample resource identities must match the production diagnostics source mapping
(both identifiers default to the actor CN when no mapping is configured).
The same ten-second freshness and integer-count checks apply.

Clicking a node opens packet-processing counters alongside its live node state.
The exporter also emits every management and fastpath-worker counter from the
full `ph-cli counters` output, using cumulative OTLP sums named
`zpr.node.counters.management.<counter_name>` and
`zpr.node.counters.fastpath.<worker_id>.<counter_name>`. Counter names are
lowercase with punctuation/spaces replaced by underscores. The inspector groups
management and individual workers, shows the sample time, and updates while open.
Values retain full unsigned 64-bit precision. These are processing totals since
runtime restart or counter reset, not rates or per-route/per-link statistics.
Missing, incomplete, stale, invalid, or unavailable counter telemetry is shown
explicitly rather than as zero. Legacy samples with denial metrics alone still
drive the badges, but the packet-counter section reports unavailable.

The supervised `scripts/node-denial-exporter.sh` takes an operator-owned JSON
array, reads each explicitly configured node through `docker exec`, and atomically
replaces its sample once per second:

```json
[{
  "container": "production-node-01",
  "cli": "/usr/local/bin/ph-cli",
  "socket": "/run/zpr/control.sock",
  "service_name": "node-01",
  "instance_id": "node-01",
  "output": "/var/lib/zpr/metrics/node-01.json"
}]
```

Run `sh scripts/node-denial-exporter.sh /etc/zpr/node-exporter.json` under the
deployment's service supervisor, or in a dedicated container with a restart
policy, `sh`, `jq`, the Docker CLI/socket, and writable sample-directory mounts.
Create output directories first and expose the sample paths to Control-Service.
Docker-socket access is privileged: restrict this exporter and its configuration
to trusted operators. Export failures are logged, never published as zero;
the last successful sample expires normally. Stop/restart the exporter using
its supervisor and verify fresh samples plus matching live snapshot counts.
These files are independent operator configuration, not Simulator profiles,
manifests, discovery, or APIs; Simulator need not be running.

### Nodes

Control Room's **Monitoring > Nodes** page (`/#node-stats`) uses the same
authenticated `/api/snapshot` feed as the topology. Select a node to view its
address, synchronization, last contact, pending installs/revocations, visa request
counts, attached adapters/links, denials, and counter tables. Docked adapters
shows a clickable count that expands a name-sorted table with adapter names and
reported ZPR addresses (Unavailable when missing). Click table headings to sort
or reverse; the open table and sort survive refreshes for that node.
A narrow management
table sits beside a fastpath comparison table with a Counter column and one
numerically ordered column per worker (number-only headings). Click a heading to
sort; click again to reverse. Worker totals sort with full integer precision,
and sort selection survives snapshot refreshes. Missing worker/counter entries
show Unavailable, not zero. On narrow screens the tables stack and scroll locally.
Changed summary and counter values pulse twice over 1.8 seconds after a snapshot
update, including decreases and resets to zero. First samples, unchanged values,
sorting and node selection do not pulse. Reduced-motion mode uses fading instead
of scaling.
Node selection survives refreshes while that node remains present. Global Refresh,
Pause and polling interval controls apply; the page makes no additional API calls
and works without Simulator. Counter values are rendered as decimal strings,
preserving unsigned 64-bit precision.

The page labels counter samples fresh or stale using the ten-second telemetry
window, including while polling is paused. Provider errors, missing counters,
missing nodes, and snapshot refresh failures are explicit. Last-known values may
remain visible with a stale/error warning; they must not be treated as current.
Buffered denials are a current gauge; local denial occurrences and packet counters
are cumulative. CPU/RAM, byte bandwidth, latency and per-flow rates are not provided.

For setup, copy **both** `service_name` and `instance_id` from each node's entry in
the production `ZPR_DIAGNOSTICS_SOURCE_MAP_FILE` into the exporter configuration.
The actor CN still keys the sample-file map; it is not necessarily either OTLP
resource identifier. For example, a `node:node-01` mapping with
`{"service_name":"zpr-core-node","instance_id":"production-node-01"}` requires those
exact values in the exporter, not `node-01` for both. The exporter rereads its
configuration each polling cycle. Verify fresh sample timestamps and
`node_details.counters` with no `counter_stats_error`/`denial_stats_error` in the
live snapshot after changing it; do not restart production nodes just to repair
an exporter identity mismatch.

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

## Restore organization base state

The Simulator Organizations page provides an explicit **Restore base state**
action. After confirmation, it backs up the selected organization's policy and
workspace database, staged policy candidates, published LDAP seed, and (for a
Docker-multinode organization) LDAP database files under
`.local-runtime/organization-backups/<organization>/<timestamp>-<generation>/`.
It then recreates that organization's workspace from its checked-in policy,
assertion, scenario, and LDIF sources and activates the profile again. Secrets
and certificates are retained. Other organizations' state is not removed.

The operation is serialized with organization activation and is rejected while
a scenario is running or a machine user remains logged in. The backup is kept
for manual recovery; restore does not erase the organization profile or its
checked-in seed files.

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
without echoing it, persists it in a protected runtime file, and reloads the
existing Control-Service container in place while preserving operator settings.
Do not put the key in browser storage, the Policy Repository, shell history,
or this repository. The assistant is ready when the service is configured;
there is no "Use assistant" checkbox and no request is sent until Send is used.
**Model and max tokens** expands the initially collapsed preferences. The
server default model or Haiku 4.5 and per-request limits of 300, 600, 1200, or
2400 tokens are available. The question prompt is **How can I help?**. Displayed
input/output token totals come from successful responses and reset on page
reload; they are not a billing or organization-wide usage limit.

The same AI Assistant pane is available beside the assertion, Gateways
and ZPR Config editors in Control Room, and beside the Simulator's LDAP seed
(LDIF) editor. Each editor sends only its own draft: assertions and ZPR Config
use `POST /api/policy/assistant` with `editor` set to `assertion` or
`zpr-config` (assertions also include the attribute catalog), Gateways uses
`GET`/`POST /api/gateways/assistant` (`gateway.read`/`gateway.analyze`), and
the Simulator uses its own `POST /api/simulator/editor-assistant` with its own
key, so Control Room never calls the Simulator. Suggested replacement text is
shown as a code block with an **Insert** button that places it at the cursor
(replacing any selection) as an ordinary, undoable unsaved edit; nothing is
saved, staged, published or activated by the assistant.

All assistants use `assistant-core.js` for conversations, model/token settings,
usage, pending/error rendering, safe code blocks and pane collapse. Domain
adapters retain their own independent endpoints and readiness checks. Changed
source (including edit-then-restore), record, revision, organization or reset
conversation invalidates obsolete responses and Insert/Apply actions.
Text Insert supports Ctrl/Command+Z and redo, including an explicit fallback
undo stack if native textarea editing is unavailable. Scenario and organization
Apply expose **Undo AI change** and **Redo AI change**; these restore editor
drafts only and refuse to overwrite later edits or another record/revision.

Policy/Assertion assistants attach source (including its group definitions)
and configured trusted-attribute definitions, not a People catalog, individual
directory records, memberships, simulation profiles or runtime sessions.
Their endpoints do not query Simulator or require it to be available. No
additional group/attribute disclosure is added. Directory/Scenario payloads
are unchanged and retain their existing source/context contracts.

### Task-specific assistant skills

Each assistant's system prompt includes a build-embedded Markdown skill:

| Task | Skill |
| --- | --- |
| Policy / ZPL | [policy/SKILL.md](skills/policy/SKILL.md) |
| Trusted-data assertions | [assertion/SKILL.md](skills/assertion/SKILL.md) |
| ZPR Config / ZPLC TOML | [zpr-config/SKILL.md](skills/zpr-config/SKILL.md) |
| Gateway JSON | [gateway/SKILL.md](skills/gateway/SKILL.md) |
| Simulator directory LDIF editor | [directory-ldif/SKILL.md](skills/directory-ldif/SKILL.md) |
| Simulator scenario design | [scenario/SKILL.md](skills/scenario/SKILL.md) |
| Simulator organization design | [organization/SKILL.md](skills/organization/SKILL.md) |

These are explicitly embedded by `assistant_skills.go`, not automatically
discovered Claude Code skills or repository `CLAUDE.md` instructions. Editing
a skill requires rebuilding and restarting the service that owns the assistant:
Control-Service for Policy, Assertions and ZPR Config; Control Room for
Gateways; Simulator for its three tasks. No runtime filesystem or Simulator
lookup is added to production assistants. Missing skill files fail the build.
The text editors retain shared insertion/Analyze guardrails; design assistants
retain their JSON answer/proposal contracts and server-side proposal validation.
Skills provide guidance, not tools, permissions, schema validation, or proof of
execution. Only the relevant task skill and existing request context are sent
in each system prompt.

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

Simulator multi-node activation merges the organization's compiler configuration
with the generated runtime topology and bootstrap key paths. Redwood includes
the shared bootstrap/platform service contracts as well as its illustrative
company contracts; these definitions do not add company access grants.
Policy installation explicitly stops on merge or compile failures, requires a
new nonempty bundle from a temporary compile directory, and never uploads an old
bundle after a failed build. Activation requires all configured nodes to appear
in the production snapshot and be in sync, not merely a connected Admin API.
Rollback refreshes Control-Service credentials after recreating the previous
runtime.

To test the installer failure paths and node-readiness guard:

```sh
go test ./cmd/zpr-web-dashboard -run 'TestOrganizationPolicyInstall|TestOrganizationActivationRequires'
```

To build Redwood's complete composed policy against generated runtime keys:

```sh
ZPR_ZPLC_BIN="$PWD/../../zpr-compiler/target/debug/zplc" \
ZPR_POLICY_RUNTIME_CONFIG_FILE="$PWD/../../.local-runtime/multinode/redwood/admin/multinode-demo.zplc" \
ZPR_POLICY_BOOTSTRAP_DIR="$PWD/../../.local-runtime/multinode/redwood/include" \
go test ./cmd/zpr-web-dashboard -run '^TestRedwoodComposedPolicyBuildsWithRuntimeBootstrap$' -count=1
```

This compiler test requires previously generated Redwood runtime assets.