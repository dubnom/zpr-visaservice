# Trusted Data Assertions

This is a report-only data-validation system, independent of ZPL permissions,
policy records, compilation, staging, and activation. Control Room's Assertions
page edits one global set. Rules and periodic settings are saved in a separate
private JSON store with revision checks. Results are in memory and do not form
a durable audit archive.

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
```

Cardinality operators are `>`, `>=`, `<`, `<=`, `==`, and `!=`. `exactly_one`
checks a nonempty list of distinct groups; `not_both` requires exactly two.
Membership is counted by distinct UID, not duplicate LDAP attribute values.
Unknown groups and empty person/group scopes produce errors, not vacuous passes.
There are at most 200 rules, 64 groups per list, and 64 KiB of assertion source.

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

Configure the authenticated Control Service, not the Policy Service:

```sh
ZPR_ASSERTION_STORE_FILE=/private/assertions/global.json
ZPR_ASSERTION_LDAP_CONTAINER=zpr-local-linux-node
ZPR_ASSERTION_LDAP_BASE_DN=dc=northstar,dc=demo
ZPR_ASSERTION_LDAP_BIND_DN='cn=zpr-reader,ou=Service Accounts,dc=northstar,dc=demo'
```

The normal dashboard stack launcher configures these from the bootstrap
organization unless explicitly overridden. The global rule set is not changed
by switching organizations. Its LDAP base DN remains the configured source;
if that directory disappears or changes, checks report source errors until the
operator reconfigures the reader. Unconfigured readers cannot evaluate or enable
periodic checks. The store directory must be private; saved files use mode 0600
and atomic replacement.

Manual Evaluate can run a draft without saving it. Save validates syntax and
requires the expected global revision. Periodic checks are opt-in, run only the
saved set, and use a configurable 30-3600 second interval. Evaluations are
single-flight. A saved-set change during an evaluation leaves its revision on
the result, so the editor can identify stale results. Checks never alter LDAP,
ZPL, visas, or access decisions.

API routes under the existing authenticated Control Service:

- `GET /api/assertions`: settings, source metadata, and latest result.
- `PUT /api/assertions`: save source/schedule with `expected_revision`.
- `GET /api/assertions/source`: fresh group/member-count catalog.
- `POST /api/assertions/evaluate`: evaluate source at `expected_revision`.

Current limits: one configured local-rig LDAP source, direct person memberships,
literal cardinality thresholds, and one global set. Arbitrary attribute
expressions, nested-group expansion, notifications, and blocking activation are
not implemented in this first version.