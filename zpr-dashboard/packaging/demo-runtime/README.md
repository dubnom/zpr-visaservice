# Paired demo runtime fixtures

The Northstar directory seed is synthetic and is derived from the Northstar
organization profile plus `simulation-environment.json`. Generate or verify its
LDIF and packaged policy assets from the Visa Service dashboard root:

```sh
python3 packaging/demo-runtime/generate-northstar-directory.py
python3 packaging/demo-runtime/generate-northstar-directory.py --check
```

The generator rejects profile/manifest references to unknown people, preserves
machines whose owner is unspecified, and keeps the dashboard example and
packaged runtime copies in sync. The fixture's `North Campus` location is
simulated, and endpoint security remains false until explicitly provisioned.
