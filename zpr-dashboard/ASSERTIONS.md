# Trusted Data Assertions

This is a report-only data-validation system, independent of ZPL permissions,
compilation, staging, and network activation. Each organization's Control Room
Assertions page edits that organization's assertion record in its Policy
Repository database. Immutable record revisions preserve rules and periodic
settings with optimistic revision checks. Results remain in memory and do not
form a durable audit archive.

## Syntax

### Advisory Lint

Evaluation returns `warnings` separately from errors and PASS/FAIL results.
The editor displays their line numbers and warning codes. Warnings cover
duplicate checks, conflicting literal equalities for the same scope, weak
cardinality checks, unscoped human-profile fields that may include machine or
service accounts, and excessive complexity.

Complexity warns when one assertion exceeds 24 expression-tree nodes, six
tree levels, or 12 groups in a membership list. Split large expressions into
independently understandable checks. These thresholds are advisory and do not
replace parser safety limits. Lint does not change saved rules, trusted data,
evaluation outcomes, or access decisions. Group/role overlap may be intentional;
the linter does not infer separation-of-duty requirements from group names.

Statements end with `;`. Group names are double-quoted, case-sensitive LDAP
`cn` values. `//` starts a comment. Whitespace and multiline statements are
allowed. Counts are integer literals from 0 to 1000000.

```text
// A named group needs at least two distinct people.
group "Operators" members >= 2;

// Apply a cardinality constraint to every returned group.
each group members > 3;

// Every person must belong to exactly one of the listed groups.
people exactly_one ["Employees", "Contractors"];

// Restrict the people being checked to members of another group.
people in "Employees" exactly_one ["Engineering", "Operations"];

// A person may belong to neither group, but never both.
people not_both ["Administrators", "Auditors"];

// Require attributes on people, scoped people, or groups.
people attribute "mail" present;
people in "Operators" attribute "title" == "Engineer";
group "Operators" attribute "description" present;
each group attribute "gidNumber" >= 1000;

// Constrain every value, or require a specific value in a multi-valued field.
people attribute "employeeType" in ["Employee", "Contractor"];
people attribute "title" contains "Reviewer";
people attribute "description" absent;
```

Cardinality operators are `>`, `>=`, `<`, `<=`, `==`, and `!=`. `exactly_one`
checks a nonempty list of distinct groups; `not_both` requires exactly two.
Membership is counted by distinct UID, not duplicate LDAP attribute values.
Unknown groups and empty person/group scopes produce errors, not vacuous passes.
There are at most 200 rules, 64 groups per list, and 64 KiB of assertion source.

### Attribute Semantics

Attribute names are LDAP names and case-insensitive; string values are compared
exactly and case-sensitively. `present` requires at least one nonblank value;
`absent` requires none. Missing/blank values fail every comparison, including
`!=`; use `absent` when absence is intended. `==`, `!=`, and `in` must hold for
every returned value. `contains` checks whether a multi-valued attribute has
one exact listed value, not a substring match.

Comparisons to unquoted signed integer literals use integer semantics for all
values. String literals support `==`, `!=`, `in`, and `contains`; ordered
comparisons require integers. A malformed or overflowing integer in trusted
data is an error, not a normal violation. Thresholds must be representable as
safe JSON integers. Only attributes in the server's approved source allowlist
can be referenced; unknown or excluded names produce errors, even with `absent`.

The Attributes catalog shows names and person/group coverage counts, with
presence-rule insertion. It does not expose attribute values. Results identify
subjects violating the assertion but do not print their observed values.

## Trusted Source

Server-configured live LDAP or authenticated HTTPS JSON readers supply data. Browsers cannot submit
snapshots or raw credentials. Authorized operators may register and edit network
source configuration through the production registration contract below. The initial
connector reads the existing local rig's LDAPS endpoint from its `zpr-vs`
namespace using the CA and password file associated with the running `slapd`.
TLS certificate verification is mandatory. The configured bind identity must
already have complete read access to the person and membership attributes;
silently ACL-filtered data cannot be identified reliably by an LDAP client.
This system does not create identities or grant rights.
Passwords and directory credentials are never returned to the browser.

The default safe attribute allowlist is `uid`, `cn`, `sn`, `givenName`,
`displayName`, `mail`, `title`, `departmentNumber`, `employeeType`,
`employeeNumber`, `uidNumber`, `gidNumber`, `description`, `o`, `ou`, `l`, `st`,
`c`, `postalCode`, `telephoneNumber`, and `objectClass`. Operators may replace
this list with `ZPR_ASSERTION_LDAP_ATTRIBUTES`, a comma-separated list of at
most 64 names. Wildcards, option-qualified/ranged attributes, and names containing
credential-like terms (`password`, `passwd`, `pwd`, `token`, `secret`,
`credential`, `private`, `key`, `auth`, or `certificate`) are rejected.
The client never requests arbitrary/all LDAP attributes. Each retained attribute
has at most 256 distinct values of at most 4096 bytes; blank values are omitted.

The reader searches the configured base DN for `person`, `inetOrgPerson`,
`posixAccount`, `posixGroup`, `groupOfNames`, and `groupOfUniqueNames` entries.
People need unique `uid` identities; groups need unique `cn` names. Membership
supports `memberUid`, `member`, and `uniqueMember` DN references to returned
people. Nested groups, unresolved members, duplicate identities, unsupported
LDIF URL values, incomplete searches, and malformed snapshots produce errors.
No simulator profile/LDIF seed is used as evaluation data.

Container LDAP output is limited to 4 MiB and 20000 entries, with a 10-second read timeout.
Network LDAP uses the stricter limits documented below.
Each run reads a fresh snapshot. Results include revision, draft/saved status,
timestamps, per-rule checked/violation counts, and at most 100 subject examples.
Source failure clears the current run's results and cannot appear as a pass.

## Runtime

Configure the authenticated Control Service to read trusted LDAP. The active
organization's Policy Repository supplies the LDAP base DN, rules, and schedule
over the existing mTLS connection:

```sh
ZPR_ASSERTION_LDAP_CONTAINER=zpr-local-linux-node
```

The dashboard stack supplies the container name and active organization context.
When organizations switch, the Assertions page loads the target organization's
revision, and periodic checks refresh its rules and LDAP base DN. A dirty editor
buffer is retained but cannot be saved or evaluated until explicitly reloaded.
Results from a previous organization are cleared.

### Organization Defaults

Each bundled organization profile supplies `assertions_source`. Defaults cover
UID/CN integrity, populated operator or role groups, and employee email/title
completeness. Department-specific checks are scoped to role groups. Assertion
`people` scopes include human person entries. Hardware entries marked
`zprMachine` and application entries with `sn: Application` remain available as
source metadata but are excluded from employee assertions. Northstar allows
overlapping roles for cross-department users; Redwood and Great Lakes check
mutually exclusive department-group membership within those groups. These
defaults do not prove that every ungrouped human has complete attributes.

Policy-Service fills missing or empty assertion records from the profile using
immutable revisions. Nonempty operator rules and existing periodic settings
are preserved. New records have periodic evaluation disabled and a 60-second
interval. To fill an inactive organization's journal without switching or
starting services, run from the dashboard repository:

```sh
go run ./cmd/zpr-web-dashboard -mode seed-assertions \
	-policy-root cmd/zpr-web-dashboard/examples/organizations \
	-policy-organization redwood \
	-assertions-db ../../.local-runtime/dashboard-stack/policy-private/redwood-policy-only.db
```

`TestBundledOrganizationAssertionsMatchDirectorySeeds` checks the defaults
against the bundled LDIF seeds. Set `SIMULATION_PREGEN_DIR` to the local
`linux-integration/pregen` directory to include Northstar's generated seed.
These fixture checks are not live LDAP evaluations; runtime checks always read
the configured trusted source.

On the first Control Service startup after upgrade, an existing
`ZPR_ASSERTION_STORE_FILE` is imported into the initially active organization's
assertion record and renamed with a `.migrated` suffix. The old rules and
schedule are preserved in the new record, and the backup remains available.

Manual Evaluate can run a draft without saving it. Save validates syntax and
requires the expected organization-record revision. Periodic checks are opt-in, run only the
saved set, and use a configurable 30-3600 second interval. Evaluations are
single-flight. A saved-set change during an evaluation leaves its revision on
the result, so the editor can identify stale results. Checks never alter LDAP,
ZPL, visas, or access decisions.

API routes under the existing authenticated Control Service:

- `GET /api/assertions`: active organization's settings, source metadata, and latest result.
- `PUT /api/assertions`: save source/schedule with the active record's `expected_revision`.
- `GET /api/assertions/source`: fresh group/member-count catalog.
- `POST /api/assertions/evaluate`: evaluate source at `expected_revision`.

## Expressions And Multiple Sources

Existing rules remain compatible and use the configured default source. Select
another named source using `from`:

```text
group "Operators" from "staff" members >= 2;
each group from "hr" attribute "gidNumber" >= 1000;
people from "staff" in "Operators" attribute "mail" present;
```

Use `assert` for a single boolean expression, or `people ... where` to evaluate
an expression separately for each person in the selected source/scope:

```text
assert source("staff").group("Operators").members ==
			 source("hr").group("Operators").members;

people from "staff" in "Operators" where
	(source("hr").attribute("salary") + source("hr").attribute("bonus")) / 12 >= 5000
	and (source("staff").attribute("title") == "Engineer"
			 or source("staff").attribute("title") == "Reviewer");

assert source("hr").person("alice").attribute("salary") >=
			 source("staff").person("alice").attribute("salary") * 1.1;
```

References support `source("name").group("name").members`,
`source("name").group("name").attribute("name")`,
`source("name").person("uid").attribute("name")`, and
`source("name").attribute("name")` for the current person in `people ... where`.
Cross-source people match by exact UID, not display name or email. An absent
person or group, unavailable/excluded attribute, missing/blank value, or more
than one distinct attribute value is an error. Legacy multi-value checks retain
their previous semantics.

`and` and `or` are case-insensitive; general `not` and `!` are unsupported.
Existing `!=`, `absent`, and `not_both` checks remain compatible. Precedence is
unary `+`/`-`, then `*`/`/`/`%`, then `+`/`-`, comparisons, `and`, then `or`.
Parentheses override precedence. Comparisons are `==`, `!=`, `<`, `<=`, `>`, `>=`.
Both boolean operands are evaluated: `or` cannot hide invalid or missing data.
Each statement must produce a boolean result, and all statements must pass.

Arithmetic uses exact rational arithmetic with integer/decimal literals and
numeric attributes. Division does not truncate; `%` requires integer operands.
Ordered comparisons coerce attribute values to numbers; equality between two
attribute/string values compares exact strings, unless one side is a numeric
expression/literal. Quoted strings cannot be arithmetic operands. Division by
zero, invalid numbers, and intermediate numeric-limit violations produce errors.
Absolute values cannot exceed 9007199254740991; numerators/denominators are
limited to 256 bits, numeric tokens to 128 bytes, and exponents to two digits.
Expressions allow at most 64 nested parser levels and 2048 nodes across a file.

### Reader Configuration

Set `ZPR_ASSERTION_SOURCES_FILE` on Control-Service to an operator-owned JSON
file. It accepts up to 16 named readers and a required `default_source` when
nonempty. An empty registry uses `{"default_source":"","sources":[]}`:

```json
{
	"default_source": "staff",
	"sources": [
		{"name": "staff", "kind": "ldap", "container": "directory-container",
		 "attributes": ["uid", "cn", "title", "mail", "salary"]},
		{"name": "hr", "kind": "https-json",
		 "url": "https://hr.example.test/assertions/{organization}",
		 "ca_file": "/etc/zpr/hr-ca.pem", "token_file": "/etc/zpr/hr-token",
		"attributes": ["uid", "salary", "bonus", "gidNumber"]}
	]
}
```

Names are identifiers beginning with a letter, followed by letters, digits,
underscores or hyphens (maximum 64 characters). With the legacy
`ZPR_ASSERTION_LDAP_CONTAINER` set, its reader is available as `ldap`; do not
duplicate that name in the configuration. Without a source file, existing
single-reader LDAP behavior remains unchanged and expressions may reference
`source("ldap")`.

Container LDAP readers inherit the active organization's base/bind DN. A reader may instead
provide an `organizations` map of organization IDs to `base_dn` and `bind_dn`.
If that map is present, organizations not listed cannot use the reader. The LDAP
connector uses the existing container-local LDAPS endpoint, verified CA, and
server-side password file; it does not read simulation data.

#### Operator registration, editing and connection tests (V02/V06)

Control Room **Trusted Sources → Register source / Edit selected source** uses
only production Control-Service contracts. No provider manager or other client
needs to be running. By default the organization comes from authenticated Policy Repository context;
requests repeat it only as a stale-context guard. An operator can explicitly set
`ZPR_TRUSTED_SOURCE_ORGANIZATION_ID` on Control-Service for source registry,
connection-test and selected-source read contracts without Policy-Service.
This is server configuration, never a browser-chosen scope or an availability
fallback. Conflicting already-loaded Policy context fails closed. Policy settings
and assertion evaluation still require Policy-Service and are not enabled by
this setting. Use the same organization in Control Room authorization grants. Existing
operator authentication and same-origin/local/proxy checks apply to all routes.
An ordinary authorized API client can use the same contract.

Provision `ZPR_ASSERTION_SOURCES_FILE` before startup, optionally with the empty
registry above, in a private operator-owned directory. Use mode **0600** for the
file and protect the directory and CA files against untrusted writers. The
service must have replacement/write permission on its directory. There is no
second database, implicit discovery or session-only registration. An unset,
missing, unprotected or unwritable file fails saves explicitly. Existing legacy
container/shared-organization definitions remain readable but file-managed, not
editable through this scoped workflow.

| Request | Contract |
| --- | --- |
| `GET /api/assertions/sources` | Returns current `organization_id`, opaque `revision`, `durable` (a file is configured), and scoped `sources` entries with `source_id`, `editable`, and editable `config`. Only file references, never credential contents, are returned. |
| `POST /api/assertions/sources` | Create a new stable source ID; duplicate IDs conflict. |
| `PUT /api/assertions/sources` | Replace an existing source configuration, keeping source and organization IDs fixed. A missing ID conflicts, never silently creates/renames a source. |
| `DELETE /api/assertions/sources` | Explicitly remove an existing non-default editable source using the same exact `organization_id`, `source_id`, current `revision` and reviewed `config` payload as save/test. Rejects changed config, active operations, default/watched/shared/file-managed sources, stale revision and other organizations. Atomic protected persistence, immutable reader snapshots and cache invalidation are preserved. No default switch, fallback, policy rewrite or cascade; remove references explicitly first. Default-source retirement is not supported. |
| `POST /api/assertions/sources/test` | Validate an unsaved candidate and perform a bounded read-only bind/search or HTTPS snapshot read. Success returns `status: "success"`; validation/connection failures return non-2xx with a source-specific, redacted error. Nothing is saved and no fallback is attempted. |

Mutation/test bodies are strict JSON:

```json
{
  "organization_id": "northstar",
  "source_id": "company_directory",
  "revision": "revision-from-GET",
  "config": {
    "name": "company_directory",
    "display_name": "Company directory",
    "kind": "ldap",
    "url": "ldaps://directory.company.example:636",
    "ca_file": "/etc/zpr/company-ca.pem",
    "attributes": ["cn", "mail"],
    "organizations": {
      "northstar": {
        "base_dn": "dc=company",
        "bind_dn": "cn=reader,dc=company",
        "bind_password_file": "/etc/zpr/company-bind-password"
      }
    }
  }
}
```

For HTTPS, use `kind: "https-json"`, `token_file` and exactly one organization
scope containing `url: "https://provider.company.example/directory"` instead of
LDAP URL/DNs/password reference. Both protocols use verified TLS with system
roots plus optional private CA, never an insecure toggle. Approved attributes
are mandatory. A connection test is not a proof of LDAP ACL completeness or
least-privilege bind-account grants; provision a read-only account externally.

Successful saves return `status: "saved"`, the same organization/source identity
and a new revision. They atomically replace and sync the same protected JSON
file and directory, then publish an immutable in-memory reader snapshot. The
first registration sets the default; later registrations/edits never switch it.
Saving explicitly **does not verify connectivity**. Connection tests and
selected-source browsing accept a successful LDAP search with no people/groups;
assertion evaluation still rejects that empty snapshot and cannot pass vacuously. Configuration edits clear
prior connection evidence in the UI; test failures cannot produce a healthy
badge. Per-source connection observations are browser-session evidence only,
not durable health certification. Recheck after reload or configuration changes.

Stale revisions, organization changes, active evaluations/tests and external
file edits fail closed. The file is a **single-Control-Service-writer** registry,
not a cross-process coordination/HA store; edit externally only while the service
is stopped, then restart. If file/directory sync fails after replacement, the
response reports uncertain durability: restart/read back before retrying. No
credential values, directory snapshots or backend exception details are emitted
by test responses. Protect and rotate file references through the operator's
secret-management process (shortcut X02); local single-writer provisioning is
recorded as X11, not production HA qualification.

#### Production network LDAP and source identity (packet A)

The existing Control-Service assertion runtime owns this read configuration;
`ZPR_ASSERTION_SOURCES_FILE` remains the only assertion source registry. The
operator owns the persistent file and secret references. Policy-Service supplies
the authenticated active organization, not a browser-supplied organization ID.
No Simulator environment, manifest, directory, container or service is consulted.

Use `kind: "ldap"` with an `ldaps://host[:port]` origin **instead of** `container`.
Explicit `attributes` and a nonempty `organizations` map are mandatory. Each map
value requires valid nonempty `base_dn`, `bind_dn` and `bind_password_file`.
A credential reference must resolve to a regular file with no group/other
permissions (0600 or stricter), containing a nonempty password up to 4096 bytes
with at most one trailing line ending. Files are validated at startup and reread
on every connection, so rotation needs no password in configuration or API data.
Protect the configuration, credential directories and CA files from untrusted
writers. Missing/unprotected credentials fail closed. `ca_file` adds a private CA
to system roots; hostname verification and TLS 1.2+ are mandatory. No insecure
mode, URL credentials, URL paths/queries/fragments, StartTLS/plain LDAP downgrade,
organization URL override or HTTPS token configuration is accepted.

**Optional subject-selector declaration:** network LDAPS sources may set
`"subject_selector": "ldap_entry_uuid"` in their source configuration. The
registration editor offers the same optional declaration and preserves it on
ordinary edits. Omit it (or use an empty string) for no declaration. Other
selector names, container LDAP and HTTPS sources reject a nonempty declaration.
Adding/removing it changes source identity and advances the server-owned source
version; display-only edits do not. Existing configuration without it retains
its serialized identity/digest. Registry reads and restart preserve the value.

This is **operator-declared intent only**. Connection tests still test the
existing read path; they do not certify entryUUID support, uniqueness, immutability
or exact-subject lookup. The declaration alone enables no lookup, active link or
VS enforcement. Username/group browsing behavior is unchanged.

An internal exact-subject lookup and review-composition path now uses this
declaration: it searches the configured organization base for exactly one
`entryUUID`, verifies the returned single canonical UUID and rejects incomplete,
duplicate, referral or out-of-scope results. Only explicitly approved/requested
LDAP attributes are returned; no username/email/DN fallback or group expansion
is performed. This path is not called by connection tests or exposed through
review HTTP, and does not change browsing or VS enforcement. See the
[integration scope and dynamic-binding blocker](../MULTISOURCE_POLICY_SUPPORT.md#s05-exact-ldap-subject-lookup-and-source-guarded-review-2026-10-24).

Only simple bind and subtree search are sent. The bind account must be granted
**read-only** directory ACLs by the directory owner; this client never sends LDAP
writes but cannot prove that the account lacks write privileges. No alias
following, referral chasing, paging or partial/size-limited result acceptance is
implemented. The complete response is bounded to 4 MiB of decrypted wire data,
2000 entries and 10 seconds (5-second connect, 8-second server search limit).
Returned entry DNs must remain within the configured base. Existing `uid` person,
`cn` group and direct membership semantics/attribute exclusions are reused; only
approved attribute values are returned. Structural membership attributes are
requested for validation but not exposed unless approved. Directory ACLs must
provide a complete snapshot; silent server filtering remains undetectable.

Stable identity is the pair **(`organization_id`, `source_id`)**, where
`source_id` is the existing case-sensitive `name`, not a new generated ID. Names
remain unique within the source file (16 readers total); a name may have separate
organization scopes. Preserve source-qualified identities and attributes even
when two sources return the same UID/DN. Cross-source correlation is explicit in
assertion expressions, never an automatic identity merge.

| Existing surface | Identity / compatibility rule |
| --- | --- |
| Operator registration/read configuration | `sources[].name` is the stable source ID; optional `display_name` is presentation only. No registration mutation route is added. |
| Assertion expressions and results | Existing `from "name"` / `source("name")` references and source catalogs retain the same name. |
| Browser status | `GET /api/assertions/status` adds `source_id`, `organization_id`, `display_name` alongside existing `name`, `kind` in `sources`. Scoped readers for other organizations are omitted. Endpoints, bind DNs, secret/CA paths and passwords are not source metadata. |
| Browser read | `GET /api/assertions/source?source=name` reads only that source and returns existing `directory`/summary fields plus `source_id`, `source_name`, `kind`, `organization_id`. It retains existing loopback/authenticated Control-Service access controls and rejects unknown/out-of-scope IDs (404), invalid selectors (400), provider failure (502), and changed organization context. Omitting the selector preserves the aggregate/default contract. |
| Visa Service inventory | `/admin/trusted-services[].name` and trusted service `name` remain runtime registration names; `provider` (e.g. `rest/1`) is a kind, `actor_cn` an actor identity. Operators should align the registration name with the source ID when they represent the same provider. This slice neither rewrites registrations nor infers a join from kind, actor, display name or URL. Inventory remains scoped by its existing admin-service organization context. |
| Change feeds | Existing feed `name` and `/api/trusted-sources/change-feeds/{source}/changes` keep their contract. Align a feed name only for the same explicitly configured source in the same organization/deployment; this slice does not add organization scoping to feeds or auto-register a feed for LDAP. Unmatched feed/inventory names remain separate, not guessed aliases. |

Selected-source reads do not contact an unrelated failed reader. An aggregate
read/evaluation fails if any applicable reader fails; defaults are not failover,
and stale snapshots never substitute for required data. LDAP alone provides no
Updates capability; consumers must not synthesize a successful empty feed or
assume every identity reader has a change feed. Revisioned registration, dynamic
disable/removal, feed scoping/capability joins and inventory reconciliation remain
future M1/M2 work, not claimed by this increment.

Consumer fixtures under `cmd/zpr-web-dashboard/testdata/trusted-sources/` include
`sources.json` (two network LDAP registrations with placeholder endpoints and
protected credential references), `source-status.json`,
`source-read-company.json` (stable response subsets; timestamps/additive summary
fields omitted), and both source LDIFs. Local TLS protocol tests execute these
fixtures with Simulator variables unset and invalid, without Docker/live LDAP.

HTTPS readers require a Bearer-token file and verified TLS (system roots plus
an optional `ca_file`). Token references must resolve to protected regular files
(0600 or stricter), checked again on every read. URL credentials, query strings, fragments and redirects
are rejected. The URL template must include `{organization}`; alternatively
provide an `organizations` map whose values contain organization-specific `url`
fields. Unlisted organizations then fail closed. Token files are reread on each
request and are never returned to the browser.

The HTTPS response is a complete JSON snapshot:

```json
{
	"people": ["alice"],
	"groups": {"Operators": ["alice"]},
	"person_attributes": {"alice": {"salary": ["60000"], "bonus": ["5000"]}},
	"group_attributes": {"Operators": {"gidnumber": ["1000"]}}
}
```

Attribute keys must be lowercase approved names. Unknown fields, duplicate
people/members, unresolved memberships, excluded attributes, and oversized
snapshots are rejected. The allowlist, entry/value limits, and 4 MiB snapshot
limit also apply to HTTPS. The provider must return complete authorized data;
the client cannot detect silent server-side filtering.

Every run reads all sources configured for the active organization once under a
shared 10-second deadline. The configured default must be available to that
organization; an inaccessible default is an error, never a reason to substitute
another source.
A reader outage makes the whole run an error with no rule results, even if an
expression would otherwise succeed. Snapshots are fresh but are not a distributed
transaction across providers. Catalogs expose source names and approved attribute
coverage; configuration, credentials, and observed expression values are not
included in evaluation results. The editor's source selector inserts qualified
rules and preserves the selected catalog across refreshes.

Current limits: direct person memberships and scalar expression attributes.
Nested-group expansion, notifications, and blocking activation are not implemented.

## Production LDAP push watcher (B4)

Control-Service can supervise RFC 4533 refresh-and-persist directly from the
same `ZPR_ASSERTION_SOURCES_FILE` used by production source readers. This does
not start or discover a directory, configure LDAP overlays, register a Visa
Service trusted service, or depend on Simulator or Policy-Service availability.
Enable `syncprov` on the operator's LDAP server separately, with an explicitly
scoped read-only account authorized to read the approved attributes and sync
metadata. No LDAP writes are made by the watcher.

Example operator-owned source configuration (paths are server/container-visible):

```json
{
  "default_source": "company_ldap",
  "sources": [{
    "name": "company_ldap",
    "kind": "ldap",
    "url": "ldaps://directory.example:636",
    "ca_file": "/secure/directory-ca.pem",
    "attributes": ["uid", "cn", "mail", "title", "member"],
    "watch_changes": true,
    "organizations": {
      "company": {
        "base_dn": "dc=company,dc=example",
        "bind_dn": "cn=reader,dc=company,dc=example",
        "bind_password_file": "/secure/directory-reader-password",
        "visa_service_id": "company_ldap"
      }
    }
  }]
}
```

Set `ZPR_TRUSTED_WATCH_STATE_DIR` to a persistent directory owned by
Control-Service (0700). Bind-password and source-config files must remain
protected (0600). Mount this directory and the explicit CA/credential files
when running Control-Service in a container. Configure its ordinary `ZPR_ADMIN_*`
production Visa Service connection with a **write-capable** Admin API key.
`visa_service_id` is an explicit mapping to the already registered Visa Service
trusted-service identifier, not an inferred directory/source name. Watchers are
opt-in and network-LDAP-only; missing mappings, state, TLS/credentials or Admin
configuration fail closed. Sources with `watch_changes` are operator-file managed,
not editable through source registration; stop Control-Service before rebinding
or disabling them, edit its source file, then restart.

Each `(organization ID, source ID)` owns one locked checkpoint; cookies and the
upstream/scope/consumer fingerprint are atomically replaced and fsynced after
successful processing. Checkpoint and lock files are 0600 with hashed filenames.
No attribute values, DNs, bind passwords or event journal are persisted. Only the
latest opaque cookie is retained (64 KiB maximum); it is private, never an API
field. A second owner, malformed/public/symlinked checkpoint, or changed source
binding is rejected rather than silently resumed or reset. For a deliberate
rebind, stop the owner, securely retire its checkpoint, then restart for a full
refresh; do not copy cookies between sources. Retired sources' private files
remain until explicitly removed by the operator.

A restart passes the committed cookie on the first LDAP subscription. Transport
loss reconnects with exponential backoff (1–30 seconds). A rejected cookie is
cleared durably before full resync. Entry add/present/modify/delete (including
UUID-only deletion sets) and refresh-complete cause a fresh approved-attribute
source read and `DELETE /admin/services/{visa_service_id}/cache`. The existing
Visa Service implementation reloads file-backed attributes or bumps an HTTP
provider's revision, then queues `TrustedServiceChange` to refresh actors and
reconcile/revoke affected live visas. Refresh-complete also handles deletions
missed while disconnected, without assuming a present set is a deletion set.
Repeated events intentionally repeat the source-wide read/invalidation: the
operation is idempotent, not an exactly-once change log. LDAP cookies are opaque,
not sortable event sequence numbers. Invalid versions, UUIDs, DNs, source scopes
and out-of-order lifecycle transitions stop processing; stale/replayed writes
cannot roll attributes back because each refresh reads current source state.

A failed read, Admin response other than 202, malformed event or checkpoint write
stops that watcher and never acknowledges the failed processing with a new
cookie. No source/default fallback is attempted; restart resumes from the last
safe cookie after the operator fixes the fault. Other sources remain independent.
This is fail-closed consumption, **not** a guarantee that every previously issued
visa is immediately revoked during a provider/control-plane outage.

`GET /api/trusted-sources/change-feeds` adds a `watchers` array containing only
organization/source IDs and health (`starting`, `refreshing`, `resuming`,
`healthy`, `disconnected`, `resync_required`, `failed`, `stopped`), event count,
last successful refresh time and a sanitized failure description. This operator
endpoint works without Policy-Service settings, even when no accesslog feed is
configured. Existing authenticated/mTLS and Control Room `policy.read` guards
apply. `GET /api/assertions` also includes organization-filtered
`change_watchers` when assertion settings are available. No UI behavior changed.
The separate HTTPS accesslog reader remains explicitly configured through
`ZPR_TRUSTED_CHANGE_FEEDS_FILE`; it does not own the syncrepl checkpoint.

Acceptance uses deterministic RFC/control fixtures and a local TLS Admin fixture,
with Simulator unavailable: restart/first-dial resume, organization/source
isolation, refresh/add/modify/delete, duplicate/malformed/out-of-order events,
disconnect/reconnect, rejected-cookie reset and read/Admin/persistence failures.
No live LDAP was changed. **Remaining integration gap:** real LDAP → running
Visa Service revocation was not exercised here. The existing Admin endpoint's
202 is asynchronous acceptance, not a reconciliation completion receipt; it can
also log an enqueue failure while still returning 202. Watcher health therefore
means refresh/invalidation accepted, not proof of visa revocation completion.
Monitor Visa Service reconciliation logs separately; no new completion/retry
protocol or bootstrap registration is claimed.

Focused validation (from `zpr-dashboard`):

```sh
go test -race ./internal/controlservice/changewatcher ./cmd/zpr-trusted-service \
  ./cmd/zpr-web-dashboard -run 'TestCheckpoint|TestWorker|TestConsumer|TestMalformedSync|TestLDAPSync|TestTrustedWatcher|TestTrustedSourceChanges|TestAssertionSource' -count=1
python3 scripts/check-control-room-boundary.py
```

### Evaluation bootstrap as an ordinary client (B2/B3)

The evaluation launcher registers `directory_<install-id>` under
`evaluation-<install-id>` through the documented public API, with a protected
password-file reference, verified LDAPS and an explicit EVALUATION ONLY display
name. No source name is reserved in the production reader/registry. Company
sources coexist with separate identities; first-registration default behavior
is unchanged and bootstrap never switches an existing default or overwrites an
existing entry on restart. See [the B2/B3 runbook and V02/V06 verification
matrix](packaging/evaluation/README.md#b2b3-verification-matrix).
