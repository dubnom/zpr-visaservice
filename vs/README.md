# Visa Service (v2)

ZPR Visa Service binary written in rust.

## Prerequisites

- A running ValKey (or Redis) server. By default `vs` connects to
  `redis://127.0.0.1:6379`.
- A compiled policy file (`.bin2` format).

## To build

- `make`
- `make test` run the unit tests
- `make check` run `cargo fmt --check` and compile with warnings as errors

## To run

The `vs` has sensible defaults. If a `vs.toml` file is present in the working
directory it will be loaded automatically. See `vs.toml` for all available
options and their defaults.

To run with defaults:

```
vs /path/to/policy.bin2
```

- To use a custom configuration file: `-c my-config.toml`.
- For verbose log output: `-v`.

By default, `vs` will look for TLS credentials in `admin-tls-cert.pem` and
`admin-tls-key.pem`. You can generate these:

```bash
openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out admin-tls-cert.pem -keyout admin-tls-key.pem
```

## Optional node geography

Node coordinates are optional display-only properties in the Visa Service
configuration, keyed by the exact node actor CN:

```toml
[nodes."node.example"]
latitude = 43.04
longitude = -87.91
```

Latitude and longitude must both be present or both absent, finite and within
`[-90, 90]` and `[-180, 180]` respectively. Invalid values fail configuration
loading. Zero is valid; omitted coordinates do not acquire a default location.
The authenticated actor Admin API exposes configured values under
`node_details.latitude` and `node_details.longitude`, omitting absent values.
Coordinates do not change identity, routing, policy evaluation or node
connection state. Configuration changes take effect on service restart.
Control Room consumes this production contract independently of Simulator.

## Optional subject snapshot fetcher (not enforced)

When configured, the VS pulls the current signed subject-link snapshot from
the Control-Service delivery listener over pinned mTLS. It verifies the
snapshot against the configured signing pins, installs it into a durable
checkpoint and reports an install receipt. Snapshots are **not evaluated or
enforced**; nothing in the VS reads them yet.

```toml
[subject_snapshot_fetch]
url = "https://control.example:8443"   # https origin only
ca_cert = "/etc/zpr/delivery-ca.pem"   # pinned server root
client_cert = "/etc/zpr/vs-client.pem" # its SPKI must be pinned in recipients
client_key = "/etc/zpr/vs-client.key"
organization_id = "example-org"
vs_id = "vs-1"
epoch = "1"
signing_pins_file = "/etc/zpr/subject-signing-pins.json"
checkpoint_dir = "/var/lib/zpr/subject-snapshot"
poll_seconds = 60                      # 10..=3600
```

The signing pins file uses the same JSON as the Control-Service signing
configuration (`{"version":1,"key_id":…,"pins":[{"key_id":…,"public_key":…}]}`).
Configuration is strict: unknown fields, relative paths, non-https or
path-bearing URLs and out-of-range intervals fail VS startup.

The first install is explicit. `vs --initialize-subject-snapshot` creates the
checkpoint directory (which must not exist) from the current signed snapshot,
reports a receipt and exits. The daemon never creates the checkpoint. Until it
exists the fetcher logs an error on each poll. Once it exists, it installs only
newer verified sequences and refuses rollback or replay, then reports one receipt per
digest. Failed receipts are retried without undoing the install. Fetcher errors
are logged and do not stop the VS.

## vsapikey

This also builds the binary `vsapikey` which is used to generate API keys
for accessing the HTTPS admin api.
