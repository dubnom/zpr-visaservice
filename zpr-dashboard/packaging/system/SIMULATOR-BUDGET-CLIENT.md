# Simulator budget-authority client inputs

This standalone renderer prepares configuration inputs for a Simulator process
that resolves current budget evidence from an independently operated
Control-Service authority. The direction is Simulator → production authority;
the production system configuration renderer does not consume Simulator
configuration or depend on Simulator availability.

## Operator-provided inputs

Create a JSON configuration based on
[`simulator-budget-client.example.json`](./simulator-budget-client.example.json)
and set:

- `schema_version`: `1`.
- `url`: the authority's exact HTTPS origin, including an optional port but no
  path, query, fragment, or URL credentials.
- `client_certificate_file`: the provisioned PEM client certificate chain.
- `client_key_file`: its matching PEM private key.
- `ca_file`: the PEM trust bundle for the authority's TLS certificate.
- `tls_server_name` (optional): the expected TLS DNS name or IP literal when it
  differs from the URL host.

The certificate, private key, and CA file paths must be distinct canonical
absolute Linux paths. They must be regular protected files readable by the
renderer under the existing operator-file policy; the private key must not be
group- or world-readable. The renderer checks PEM parsing and that the client
certificate matches its private key. It does not contact the authority, verify
that the certificate is currently trusted or authorized there, provision a
grant, or test DNS/network reachability.

Provision valid operator-owned identities, CA material, and any required
authority-side grant out of band. Never place private key contents or grants in
the JSON configuration. Use the example only as a path/schema reference; its
example host and paths are not operational values.

## Build and render

From the `zpr-dashboard` module directory:

```sh
go build -trimpath -buildvcs=false -o /tmp/zpr-simulator-budget-config-render ./cmd/zpr-simulator-budget-config-render
/tmp/zpr-simulator-budget-config-render \
  -config /absolute/path/to/operator-simulator-budget-client.json \
  -output /new/absolute/path/simulator-budget-runtime-inputs.json
```

The output path must not already exist. Rendering first validates the
configuration and protected TLS files; a failure is reported on standard error
and does not publish a partial output. On success the renderer atomically
creates a mode-`0600` JSON file containing:

- `environment`: the `ZPR_BUDGET_AUTHORITY_URL`,
  `ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE`,
  `ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE`, and
  `ZPR_BUDGET_AUTHORITY_CA_FILE` variables, plus
  `ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME` when supplied.
- `file_mounts`: read-only mounts for the three operator-supplied TLS files,
  retaining their absolute paths as both source and destination.

The output contains paths and resolver environment settings, not credential
contents. It is an input artifact only: no service is launched, no environment
is applied, and no container, host, or deployment configuration is changed.
An operator or a future Simulator launcher must explicitly apply these values
and make the read-only files available at the rendered paths. No production
launcher integration is provided here.

Offline package and actual-process CLI checks can be run from the module
directory:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test \
  ./internal/simulatorbudgetconfig \
  ./cmd/zpr-simulator-budget-config-render \
  -count=1
```
