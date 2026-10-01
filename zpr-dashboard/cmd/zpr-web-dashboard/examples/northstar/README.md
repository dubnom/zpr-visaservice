# Northstar Policy Studio Demo

This fixture seeds six department-based ZPL policy examples into the Policy
Repository. Their seven embedded JSON service definitions are illustrative;
they are not deployed endpoints or services in the running ZPR test network.

The six policy examples correspond to LDAP `ou` values such as `Accounting`,
`Platform`, `HR`, `Security`, `Field`, and `Legal`. The current `demo_ldap`
connector maps LDAP `ou` and `title` to `device.demo.department` and
`device.demo.title`. It does **not** map `groupOfNames` memberships, so these
examples intentionally do not claim that LDAP roles such as `Approver` or
`Incident Responder` are available as ZPL attributes.

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
`Save As...` can be used to create editable copies of the examples.
