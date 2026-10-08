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

## vsapikey

This also builds the binary `vsapikey` which is used to generate API keys
for accessing the HTTPS admin api.
