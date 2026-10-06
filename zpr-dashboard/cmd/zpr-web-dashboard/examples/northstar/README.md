# Northstar Policy Studio Demo

This fixture seeds nine ZPL policy examples into the Policy Repository. Their
ten embedded JSON service definitions are illustrative; they are not deployed
endpoints or services in the running ZPR test network.

The policy examples use LDAP `ou` values such as `Accounting`, `Platform`, `HR`,
`Security`, `Field`, and `Legal`. The `demo_ldap` connector maps LDAP `ou` and
`title` plus machine fields to `device.demo.*` attributes such as `machine_type`,
`location`, and `secure`. The separate `demo_ldap_user` connector maps user
`cn`, `mail`, `ou`, and `title` to `user.*`, and maps matching LDAP group `cn`
values to multivalued `user.role`. It resolves people by `user.sub -> uid`,
which must be supplied by AuthService (`sub -> user.sub`).

Department access examples target authenticated people through `user.department`,
not their devices. Role-group grants are independent of department, so Approvers,
Incident Responders, and Security Reviewers can receive cross-department access
according to their actual LDAP memberships. Candidate-test fixtures expose these
configured group-role mappings as well as membership tags.

The simulator's current Log in control only records a mock machine session; it
does not mint an AuthService user identity. Thus device-posture examples use
device attributes, while user-role examples are compiler-checked demonstrations
that require a real `user.sub` identity to evaluate at runtime.

The demo ZPLC config declares each fake service class and the same LDAP
attribute mappings. ZPLC validation checks the examples against this config;
it does not deploy the fake services or modify the Visa Service's active
policy.

Analyze and candidate tests try the organization's demo configuration first and
the configured runtime/staging configuration when needed. This allows the same
editor to validate both illustrative department policies and shared Network or
Simulator infrastructure policies. Both contexts use the compiler; invalid
syntax does not become valid through a configuration fallback. Infrastructure
rules remain device-oriented and need not match LDAP users.

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
