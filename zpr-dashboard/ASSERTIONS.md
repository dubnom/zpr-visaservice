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
snapshots, provider URLs, credentials, or source configuration. The initial
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

LDAP output is limited to 4 MiB and 20000 entries, with a 10-second read timeout.
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
UID/CN integrity, populated operator or role groups, and group-scoped email,
title, and department checks. Human-only fields are scoped to named groups
because the LDAP reader also returns machine and service accounts. Northstar
allows overlapping roles for cross-department users; Redwood and Great Lakes
check mutually exclusive department-group membership within those groups.
These defaults do not prove that every ungrouped person has complete attributes.

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
file. It accepts 1-16 named readers and a required `default_source`:

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

LDAP readers inherit the active organization's base/bind DN. A reader may instead
provide an `organizations` map of organization IDs to `base_dn` and `bind_dn`.
If that map is present, organizations not listed cannot use the reader. The LDAP
connector uses the existing container-local LDAPS endpoint, verified CA, and
server-side password file; it does not read simulation data.

HTTPS readers require a Bearer-token file and verified TLS (system roots plus
an optional `ca_file`). URL credentials, query strings, fragments and redirects
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

Every run reads all configured sources once under a shared 10-second deadline.
A reader outage makes the whole run an error with no rule results, even if an
expression would otherwise succeed. Snapshots are fresh but are not a distributed
transaction across providers. Catalogs expose source names and approved attribute
coverage; configuration, credentials, and observed expression values are not
included in evaluation results. The editor's source selector inserts qualified
rules and preserves the selected catalog across refreshes.

Current limits: direct person memberships and scalar expression attributes.
Nested-group expansion, notifications, and blocking activation are not implemented.