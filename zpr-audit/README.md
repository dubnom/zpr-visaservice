# ZPR Audit

Audit is included in the minimal ZPR package. It provides history-first sample
workspaces, a separate read-only assertion-settings import, and an authenticated
query of stored audit events. Use day/week/month windows for short-term history
review; Control Room and Security remain the live operational interfaces. The
stored-event query is not available from the standalone unauthenticated preview.
The UI remains independent of Simulator and OpenObserve. An optional development
adapter can read a configured production Policy-Service directly; static hosting
and package inclusion alone do not enable this connection. No real policy
evaluation or broad historical event coverage is implemented.

The local development UI is served from `public/`; see [Run](#run). Imported
assertion settings remain operator-supplied data and are not submitted to a
service by Audit.

## Read current settings from Policy-Service

The dependency-free Node adapter (Node 20+), **not the packaged static UI**, provides
an opt-in read-only connection to the existing production
`GET /api/assertions/settings` contract. Start locally with an authorized service
endpoint and an expected organization ID (the service's organization context,
not a client-selected organization query):

```sh
ZPR_AUDIT_POLICY_SERVICE_URL=https://policy.example:8443 \
ZPR_AUDIT_ORGANIZATION_ID=your-organization-id \
ZPR_AUDIT_TLS_CA=operator-ca.pem \
ZPR_AUDIT_TLS_CERT=audit-client.pem \
ZPR_AUDIT_TLS_KEY=audit-client-key.pem \
npm run start:connected
```

These are illustrative operator-owned paths, not bundled credentials. For an
authorized loopback HTTP service, use `http://127.0.0.1:PORT` and omit TLS files.
Remote HTTP is rejected. HTTPS uses normal certificate verification; custom CA
and client certificate/key support deployments using mutual TLS. Certificate
authorization remains the deployment owner's responsibility. No browser cookies,
bearer tokens, local credential discovery or TLS-verification bypass is provided.
If the deployment cannot authorize this client, retain the authorized file-export
workflow rather than weakening service authentication.

Open `http://127.0.0.1:49282/#inventory`, select **Check connection configuration**,
then **Read current assertion settings**. Configuration checks do not read settings.
`ZPR_AUDIT_PORT` optionally changes the local port. The adapter binds loopback only;
it has no user authentication and must not be exposed as a network service or
reverse-proxied to untrusted users. Local users can read its configured evidence.
Host/origin checks reject cross-origin browser and DNS-rebinding requests. No
cross-origin API access is enabled. `npm start` stays static-only, with imports
available and connected reads explicitly unavailable.

The UI never supplies an upstream URL, organization selector or credentials.
The adapter exposes only `GET /api/audit/source` (non-secret configuration status)
and `GET /api/audit/assertion-settings` (one settings snapshot plus acquisition
metadata). It rejects redirects, non-200 responses, invalid JSON/UTF-8, service
errors, load failures, scope mismatches, unexpected organizations, responses over
256 KiB and reads exceeding 12 seconds. It never writes to the source, invokes an
evaluator, reads runtime/history data, calls Simulator or falls back to fixtures.
Responses use `Cache-Control: no-store`; there is no persistence or background polling.

Successful reads use the existing source review, baseline comparison and report
workflow, keeping fixtures separate. Every new read clears previous evidence and
operator review fields, even on failure. Late reads cannot replace newer reads
or file imports. Reports distinguish service-acquired snapshots from file imports
and retain the configured endpoint, adapter retrieval time and upstream-response
SHA-256 independently from operator-entered claims. The displayed filename/file
size/file SHA-256 describe a **browser-generated settings-only JSON projection**;
the separately labeled upstream-response digest fingerprints original bytes.
Runtime fields are discarded, not reported as evaluation evidence. Neither digest
authenticates origin; acquisition metadata trusts the local adapter and its clock,
and does not prove publication time, activation, freshness or historical validity.
Downloaded HTML may contain sensitive policy text and internal endpoint names.

## Backend building blocks

The dashboard repository now has reusable audit backend primitives:

- `internal/audit` defines a minimal event contract and organization-scoped
  query/page interface without arbitrary event payloads.
- `internal/audit/sqlite` provides private-file SQLite persistence, append-only
  triggers and bounded, cursor-based reads.
- `internal/audit/httpapi` provides a read-only HTTP adapter. Its required
  `ScopeResolver` must obtain the authorized organization from trusted server
  authentication context; caller-supplied organization query parameters are
  rejected.

The read-only endpoint is now mounted in Control-Service and exposed through
Control Room at `GET /api/audit/events`. Control Room requires the distinct
`audit.read` permission and signs a short-lived request-bound delegation;
Control-Service verifies its operator, organization, permission, mTLS peer and
one-use nonce before querying. Add `audit.read` only to explicitly approved
operator grants and the Control-Service delegation trust grants. Configure `ZPR_AUDIT_DB_PATH` to a private absolute path on Control-Service; it
is a required minimum ZPR setting and Control-Service startup now fails if the
store cannot be opened. If delegation trust is missing or invalid, the history
endpoint returns unavailable rather than an empty success, but event producers
can still write to the required store.
The query supports optional RFC 3339 `from`/`until` bounds, exact `source`,
`action` and `outcome` filters, an opaque `cursor`, and `limit` from 1 to 100
(default 50).

The History view keeps fictional sample data separate from live results. Its
**Load live audit feed** control requests the authenticated same-origin
`/api/audit/events` endpoint; the standalone development server has no
authentication and will not provide that endpoint. Unauthorized and unavailable
responses are shown explicitly. An empty page is not evidence that no activity
occurred. History windows default to the last 7 days and offer the last 24 hours,
last 30 days, or a custom range. Bounds are local-time inputs converted to RFC
3339 instants; `from` is inclusive and `until` exclusive. Exact action, source
and outcome matches are optional. Changing filters starts a new query;
pagination continues with the exact query used for its first page. This is a
short-term lookback, not a live operational dashboard or comprehensive
historical archive.

The endpoint reads events explicitly appended to the new store. Producers now
record authorized Audit-history reads as `audit.history.read`, and the embedded
Control-Service enrollment handler records verified named-user requests for
device-enrollment invitation create, cancel, approve, reject, and preflight
recording, plus invitation list/detail and preflight reads. Catalog requests
are not attributed to an organization because they may cover multiple
organizations. This producer does not cover enrollment operations served by an
externally configured enrollment-service proxy. Enrollment records use outcomes
of `requested` and are appended before the handler runs; they indicate an
authorized request was issued, not that business validation or the operation
succeeded.
An append failure blocks the enrollment mutation. Control-Service startup now
requires the audit repository, so the embedded producer cannot silently run
without its event store.
The authoritative Control-Service budget-authority handler also records
delegated approval requests as `budget.approval.create` and revocation requests
as `budget.approval.revoke`. These events are organization-scoped, attribute the
verified operator, and use the same `requested` semantics. They are appended
before the mutation; if event creation or persistence fails, the approval or
revocation is blocked.
No existing Control Room audit tables have been migrated or bridged, and no
underlying policy, visa, identity or service operations are produced. The
external enrollment-service proxy remains uncovered. There are also no
retention/backup operations or centralized historical imports. SQLite
immutability triggers prevent ordinary row updates/deletes; they do not protect
against a database administrator or replace backups and operational controls.
Do not describe this as a complete or tamper-proof audit service.

## Importing real assertion settings

Open **Imported assertions**. Use existing authorized production tooling to
export the response from Policy-Service `GET /api/assertions/settings`, then
choose the JSON file. This contract is defined by
`assertionSettingsResponse` / `assertionSettings` in the existing dashboard's
`policy_assertions.go` and `assertion_runtime.go`. The export must contain:

```json
{
  "organization_id": "your-organization-id",
  "organization_name": "Your organization",
  "settings": {
    "revision": 1,
    "source": "",
    "enabled": false,
    "interval_seconds": 60
  }
}
```

Alternatively, save a successful authorized Control Room `GET /api/assertions`
response. Its organization/settings fields use the same shape. If `scope` is
present it must be `organization`; if `settings_error` is present it must be an
empty string. Runtime fields such as `last_run`, source metadata and watcher
status are deliberately excluded: they are not evidence for this settings-only
contract and may belong to a different revision or organization.

An `error` response (including "Policy Repository assertion settings are
unavailable") is not an export, even when saved as JSON. Audit rejects it.
It also rejects settings-load failures alongside apparently valid settings
rather than presenting cached data as successful evidence. Invalid replacements
clear previous data and cannot generate a report. Retry the read-only export
after the deployment owner confirms readiness; do not restart services or
change production settings to satisfy Audit.

This example documents the shape only; it is not evidence. Audit does not
retrieve it automatically, capture browser sessions, read local credentials,
change production settings or invoke evaluation. Import files are limited to
256 KiB and validated before display. Missing organization names are rejected,
not inferred from trusted-source names or Simulator. Organization values are
asserted by the export and are not independently verified.

Review the source text, organization ID/name, revision, exported enabled/interval
settings, filename and import time. SHA-256 is computed only when Web Crypto is
available (HTTPS or localhost); the private-IP HTTP preview explicitly reports
it unavailable. The digest fingerprints bytes, not authority or authenticity.
Import time is never described as source retrieval time.
Optional operator-entered provenance records the source system (Policy-Service,
Control Room or unspecified), a plain-text reference and retrieval timestamp.
These statements are not imported facts or independently verified authority.
References are not opened or fetched; do not enter credentials. Retrieval
timestamps must use `YYYY-MM-DDTHH:mm:ssZ`, be valid UTC dates and not be later
than the snapshot's import time.

Imported inventory reports require a preparer and purpose. Optional reviewer
notes are escaped and labeled unverified commentary. Reports include their own
unique reference and browser-generated timestamp, separate sections for
imported facts and operator statements, source UTF-8 byte size and explicit
empty-source status. The export itself supplies no retrieval timestamp; the
imported-facts section retains that distinction even when an operator states one.
Replacing or clearing a snapshot clears all operator-entered review fields to
avoid attaching one file's provenance to another. None of these labels establish
authentication, source freshness, evaluation or historical validity.
Imported source review provides display line numbers, a line/byte/revision
summary, literal text search with optional case sensitivity, matching-line
counts and wraparound previous/next navigation. Search counts lines containing
the text, not individual occurrences or assertion results. Jump-to-line rejects
invalid/out-of-range numbers. A selected line can be referenced in reviewer
notes with its organization, settings revision and file, subject to the notes
length limit. Search and line selection reset on replacement/clear.

The display recognizes CRLF, LF and CR line boundaries; line numbers are
review aids, not canonical assertion IDs or compiler locations. It normalizes
display separators only. Report source text remains complete and unfiltered,
including original line endings, regardless of the current search or selection.
No source parsing, execution or evaluator diagnostics are introduced.

### Comparing imported snapshots

After importing a current snapshot, choose a baseline export from the same
organization ID. Both files use the same validation, size limit and optional
SHA-256 computation. Baseline filename, import time, fingerprint and organization
are displayed separately. Different organization IDs are rejected; a name
difference for the same ID is flagged, not treated as a verified rename.

Open **Baseline source provenance** to optionally record its source system,
reference and UTC retrieval timestamp independently of the current snapshot.
The same validation applies to both snapshots: source references are limited to
1000 characters and retrieval must be a real UTC date no later than that file's
own import time. These are operator statements, not exported facts. Invalid
baseline provenance blocks reporting, with an actionable error; correcting or
clearing the offending field restores reporting. Baseline replacement, failed
replacement or clearing resets these fields without changing current provenance.
Reports label each snapshot's operator statements separately and retain
"Source retrieval time: Not supplied" in imported facts.

The comparison summary explicitly identifies same-revision equality, same-revision
conflicts, revision-only settings changes, differing settings across revisions and
reversed revision order. Settings content equality compares exact source text,
enabled and interval, excluding revision; organization-name differences remain a
separate warning. This does not imply semantic equivalence or activation.
Revision difference is current minus baseline, not an edit count, missing-export
count or historical completeness check. Source metrics show both snapshots'
UTF-8 byte counts, displayed-line counts and signed differences. Empty source has
zero displayed lines; trailing blank lines count. Exact bytes include original
line endings. File byte size is reported separately from source size. File
formatting and excluded runtime fields do not affect settings equality.

The comparison shows revision, enabled and interval values plus exact source-text
change detection. Newer baselines and same-revision/different-content conflicts
are explicitly flagged. Revisions alone prove neither chronology nor activation.
Differing source lines are compared **at the same line position**, not aligned
as semantic edits: inserting a line may shift all subsequent positions. CRLF,
LF and CR are normalized only for the line view, so a source may differ exactly
while all displayed line positions match. Missing lines and blank lines remain
distinct. Counts refer to differing positions, not assertions or policy impact.

The review summary counts **changed**, **added**, **removed** and **unchanged**
positions. Added means only the current side has a line at that position;
removed means only baseline does. These are positional classifications, not
aligned edit operations. The line table explicitly labels blank and missing
lines and preserves whitespace in nonempty text.

Filter differences by position type or search literal text on either side,
optionally case-sensitive. Search does not match display labels such as
"Blank line". Review shows 25 matching positions per page. Selecting a position
or using wraparound Previous/Next difference navigates across all matching
pages; Go to position requires a differing position within the active filters.
No matches is a review-filter result, not evidence of no source change.
Exact line-ending-only changes are called out even with no differing displayed
positions. Reset comparison filters restores all differences and first selection.

Add comparison reference to notes records the selected position, classification,
organization and **both** snapshots' revisions, filenames, import times and file
fingerprints (or explicit fingerprint unavailability). References are unverified
operator commentary and respect the 4000-character notes limit. Replacing or
clearing baseline resets filters, selection and pagination, but preserves notes
already attributed to their original snapshot pair; those notes are not evidence
for the new baseline. Replacing current clears all review notes as before.
An in-progress baseline import can be cancelled with Clear baseline, and reports
wait for imports to finish so that pending evidence is not silently omitted.

Reports include both snapshots' provenance, settings comparison, differing line
positions and complete original source texts, regardless of review filters,
selection or pagination. Downloaded and printed reports retain the full comparison,
not just the visible page. Without a valid baseline the report
explicitly covers only the current snapshot. Clearing/replacing the baseline
does not change the current source. Clearing/replacing the current snapshot
clears its baseline; invalid baseline replacements remove the previous comparison.
Files remain in memory only. No source evaluation, live query, history replay
or cross-category evidence merge is introduced.

**Report / print** creates an imported-only HTML/print report with provenance and
limitations. Settings exports do not establish individual assertion identities,
roles, policy references, runtime results or historical evidence; Audit does not
invent them or execute source text. Empty source text is displayed explicitly.
Unknown export fields are not rendered or retained in the normalized record.

Imported data stays in browser memory, does not populate fictional views or
sample report packs, and clears on reload or **Clear imported snapshot**. An
invalid replacement clears the previous record. Handle source files/downloads
as potentially sensitive policy data. No server or dependency changes are needed
for this import-first slice; live authorized connectivity remains future work.

## Run

From this directory:

```sh
python3 -m http.server 49282 --bind 127.0.0.1 --directory public
```

Open <http://localhost:49282>. Ctrl-C stops the development server.
Only public assets are served. No npm install or build is needed to run.
Alternatively open `public/index.html` in a modern browser.

The sample workspaces include access exploration, group membership drilldowns, illustrative
policy changes/history, assertions and tailored report packs. All data is
fictional; the separate Imported assertions tab can display operator-supplied
real snapshots. State resets on reload. Reporting dates are metadata only; they
do not filter the fixtures. Downloads contain user-entered report metadata.
Saved packs, scheduling and live evidence explicitly remain Not Implemented.
Service-first access requires only the service and shows separate Users, Groups
and Machines sections simultaneously. Each has independent counts and pagination;
no cross-category total or combined table is displayed. Result-type filtering is
optional and applied afterward.
Reverse exploration selects a category first, then a user, group or machine
from only that category and lists services;
additional request-context controls apply only to individual users/machines.
Group rows describe candidate policy relationships and remaining constraints,
not grants to every member. Service template reports also include group rows.
Current-view and service-template reports retain separate category sections.
Identity report assembly selects Users or Machines before choosing a subject.
Bulk addition applies only to that selected category; a pack-item shortcut
restores both category and subject. All known fixture identities are available,
including identities with unavailable evidence. The group directory separately
lists user groups and machine groups with category-specific member counts.
The default page size is 25 so all current service candidates appear together.
Candidate search matches displayed names/IDs and is preserved in current-view
report scope. Template reports use their separately selected scopes, not the
Access Explorer search. Packs support up/down ordering and confirmed clearing;
clearing retains shared reporting details. Printing without an active preview
shows an explicit no-report message; prepare a preview before printing.
Browse groups directly from Access Explorer; the group directory counts only
known sample memberships. Reset filters clears search, outcome and membership
filters without changing the selected request/context/revision. Reports offers
a template-content preview before adding it, plus a live pack summary.
Optional reviewer notes are escaped and included as unverified commentary in
pack downloads; they are temporary and must not contain sensitive data.
Access results support name/outcome sorting and local pagination. Changing
request scope, filters, sort or page size resets to page one. Summary counts
remain cohort-wide; current-view reports include all filtered matches in the
selected order, not only the visible page. Empty matches do not prove no access.
This development server is not a production deployment.
The toolbar Help button follows the Control Room interaction pattern: a
current-page modal with workflow steps, recovery guidance and a full-help link.
Escape or Close returns focus to Help without changing workspace selections.
Full help opens separately with topics for access, policy, assertions, history
and reports. Longer general explanations live there; record-specific gaps and
essential sample/period labels stay inline.
Template descriptions are expandable. Downloaded reports retain mandatory
limitations and do not depend on the Help page.
The workspace uses compact screen spacing, tighter table rows and inline
summary counts to leave more room for records without reducing the 14px body
text. Screen-density rules do not apply to printed reports.
Search, sorting and page size share one toolbar on wider windows. Secondary
membership interpretation is expandable; selected request context remains
visible. The four-user result-type filter fits within a 1366 x 768 browser viewport
with the membership explanation collapsed. Narrow screens wrap controls rather
than shrinking text or hiding fields.

### Investigation checkpoint

Three browser regression journeys cover membership → user access → dated
report, policy comparison → affected-service access, and diagnostic assertion →
dated report pack → printable export.

Access context labels, explanation dialogs and current-view reports describe
the selected context consistently; source-unavailable and device-unspecified
selections do not imply managed-device verification. Service cohorts have
per-row contexts, not a single verified session.

Only the r41 → r42 policy comparison is available. Selecting another revision
pair hides the fixed comparison and reports explicitly say no comparison is
available rather than exporting unrelated impact/activation evidence.
Explore affected service opens Finance Reports at r42 regardless of previous
Access Explorer selections. Priya's fixture is permitted under r41's
employee-only condition and context-required under r42's device requirement.

Operator review paths:

- Browse groups → Finance → Priya Shah → change request context → Report / print.
- Policy changes → inspect r41 → r42 and partial activation → Explore affected
  service → switch access revision to compare the sample outcomes.
- Assertions → Diagnostic only → inspect dependencies/limits → Add selected
  assertion to pack → Reports → enter preparer/dates → preview/download/print.

This checkpoint validates navigation and interpretation, not source correctness
or production readiness. Evidence inventory and independent read-only contracts
are the next development stage; broader visual polish remains deferred.

### Assertion workspace

The Assertions tab (`#assertions`) browses the same fictional definitions used
by assertion reports. Search matches names, canonical revision references,
definitions, policy references and sources; the enforcing/diagnostic filter
combines with search. Reset clears both filters. The selected record is retained
while it matches; otherwise the first match is selected. No matches clears
details and disables record actions rather than showing stale evidence.

Each record retains its own revision, plain-language intent, policy reference,
selected-context examples, source dependencies and missing-evidence caveats.
These are not executable assertions or evaluator traces. Diagnostic observations
are not enforcing decisions; assertion satisfaction is not policy permission.
No aggregate pass rate or completeness claim is generated.

Policy/service shortcuts open the fixed r42-related fixture, not live analysis.
The policy shortcut shows the r41-to-r42 comparison; Access resets to the
referenced service's mixed sample candidates at r42, without assuming a group grant.
Add the selected assertion directly to a report pack without changing the
Reports template selection; duplicate template/scope pairs are skipped.
Report / print captures **all filtered assertions**, not just the selected
record, and includes search/role scope and mandatory limitations. Empty filtered
reports explicitly state no matches, not absence of assertions. Reporting dates
remain metadata only. Refresh retains the tab, not selections or filters.

### Assembling larger report packs

Search templates by name or description without changing the selected template
or pack. Catalog entries show how many reports of each type are in the pack.
Add all scopes for a template at once; existing template/scope pairs are skipped
and the feedback reports both added and skipped counts. Removing all reports
for a template requires confirmation and retains other reports and metadata.

Each pack item has its own preview and a shortcut to select that template/scope
again. This shortcut does not edit the existing report. The expandable template
coverage checklist describes assembly only, not evidence completeness,
compliance or mandatory template requirements.

Pack previews and downloaded HTML include linked contents and per-section
return links. Preview links do not change the workspace URL. Returning from a
full preview restores focus to its launch button.

## Reuse boundary

`public/shared/palette.css` is generated from the existing operator/Simulator
palette in `../zpr-dashboard/cmd/zpr-web-dashboard/static/app.css`.
Run `node sync-assets.mjs` after intentional upstream palette updates.
Audit aliases consume those tokens rather than maintaining a separate base
palette. Generated assets are committed so Audit serves independently.
The shared source is not edited by Audit.

Existing tables/dialogs are stylesheet conventions rather than independently
packaged components; the prototype keeps its own layout pending coordinated
extraction. Policy editor/diff surfaces require their editor packaging and
service contracts and are not imported wholesale. The standalone log formatter
is a future reuse candidate when actual event-detail rendering is introduced.
Do not import page controllers that fetch Simulator state, manifests or APIs.
No runtime service dependency is introduced by sharing static assets.

The reviewed prototype remains the workflow baseline. Extracted local CSS and
JavaScript live separately from HTML for maintainability. Larger component
extraction and production contracts are later coordinated work, not implied
live functionality.

`public/fixtures.js` owns the common sample identity, membership, assertion and
bidirectional request model. `public/ui.js` contains local dialog, table,
escaping and selection helpers; `public/app.js` coordinates views and reports.
These files load in that order, without importing service/page controllers.
The URL fragment retains the selected workspace across refresh; filters and
report packs remain temporary and are not encoded in the URL.

## Test

```sh
cd ../zpr-dashboard
go test ./internal/audit/...
cd ../zpr-audit
npm ci
npx playwright install chromium
npm test
```

The test command first runs dependency-free fixture consistency tests, then
browser smoke/navigation tests. Playwright is a development-only dependency. Tests own a temporary server on
49283, refuse to reuse another server, and cover shared palette loading,
groups, reports/downloads, print layout, tablet sizing and absence of external
requests. No production services are contacted.
