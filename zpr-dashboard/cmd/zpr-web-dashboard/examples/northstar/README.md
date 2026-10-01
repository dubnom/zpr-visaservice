# Northstar Policy Studio Demo

This fixture seeds seven ZPL policy examples into the Policy Repository. Their
eight embedded JSON service definitions are illustrative; they are not deployed
endpoints or services in the running ZPR test network.

The policy examples use LDAP `ou` values such as `Accounting`, `Platform`, `HR`,
`Security`, `Field`, and `Legal`. The `demo_ldap` connector maps LDAP `ou` and
`title` to `device.demo.department` and `device.demo.title`. It also resolves
`groupOfNames.member` links and maps each matching group `cn` to the multivalued
`device.demo.role` attribute, so the role-based Security example can test roles such as
`Approver`, `Incident Responder`, and `Security Reviewer`.

The demo ZPLC config declares each fake service class and the same LDAP
attribute mappings. ZPLC validation checks the examples against this config;
it does not deploy the fake services or modify the Visa Service's active
policy.

To populate a local Control Room database, set these environment variables
when starting `zpr-web-dashboard`:

```sh
export ZPR_POLICY_DB_FILE='/path/to/private-directory/policy-records.db'
export ZPR_POLICY_CONFIG_FILE="$PWD/cmd/zpr-web-dashboard/examples/northstar/policy-demo.zplc"
export ZPR_ZPLC_BIN='/path/to/zplc'
export ZPR_POLICY_DEMO_CATALOG_FILE="$PWD/cmd/zpr-web-dashboard/examples/northstar/demo-policy-catalog.json"
```

The catalog import is idempotent: existing categories and records with the
same names are left untouched, so it is safe to restart the demo. To see the
new policy-only tree after a previous import, use a fresh demo database; the
import does not delete existing service records or overwrite existing policies.
The simulator stack uses `.local-runtime/dashboard-stack/policy-private/northstar-policy-only.db`
for this purpose and leaves the earlier default Policy Studio database intact.
`Save As...` can be used to create editable copies of the examples.
