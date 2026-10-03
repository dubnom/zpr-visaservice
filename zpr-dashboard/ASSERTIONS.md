# Trusted Data Assertions

This is a report-only data-validation system, independent of ZPL permissions,
compilation, staging, and network activation. Each organization's Control Room
Assertions page edits that organization's assertion record in its Policy
Repository database. Immutable record revisions preserve rules and periodic
settings with optimistic revision checks. Results remain in memory and do not
form a durable audit archive.

## Syntax

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

Only a server-configured live LDAP reader supplies data. Browsers cannot submit
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

Current limits: one configured local-rig LDAP source per active environment,
direct person memberships, and literal cardinality thresholds. Arbitrary attribute
arithmetic/boolean expressions, nested-group expansion, notifications, and
blocking activation are not implemented. Attribute presence, literal comparisons,
allowed-value lists, and exact multi-value containment are supported.