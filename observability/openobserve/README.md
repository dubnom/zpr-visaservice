# ZPR observability service

**Optional deployment profile.** This profile runs the
AGPL-3.0 open-source edition of [OpenObserve](https://github.com/openobserve/openobserve)
as a single-node logging and metrics store. The Dockerfile pins its upstream
image by digest. It does not vendor or modify OpenObserve. There is no host
port: the process enters a dedicated ZPR adapter's network namespace and
listens only on its assigned ZPR IPv6 address. The launcher requires that
adapter and its identity to be provisioned first; it does not change the
running Visa Service policy or create an adapter.

## Service and network contract

The example [policy](zpr-observability.zpl) declares class `ZprObservability`,
service ID `observability.svc.zpr`, TCP/5080, adapter CN
`zpr-observability`, and address `fd5a:5052:adda:1::54`. Allocate that
address to an authenticated adapter before using the profile. The adapter
must advertise `ZprObservability` in its service claims. Compile the policy
with the network's other ZPL and install the signed result through the normal
policy-update process; do not replace a running policy with this fragment.
The example grants access only to publisher adapter CN `telemetry-publisher`
and reader adapter CN `ops-observer`. The telemetry publisher needs read-only
Visa Service admin access and TCP/5080 ingest access; it does not install policy.
Adapt these to real provisioned identities; a
Control Room process running on the host does not become a ZPR reader by
virtue of its name. DNS publication follows the existing service-directory
rules and does not itself grant access.

OpenObserve binds HTTP to IPv4 loopback only. A `socat` listener binds TCP/5080
to **only** the assigned ZPR IPv6 address and forwards to that loopback socket.
The proxy is required: the upstream IPv6 mode binds all IPv6 interfaces even
when given a specific address. OTLP/gRPC is not published:
`ZO_GRPC_ADDR=127.0.0.1` keeps port 5081 loopback-only in the adapter namespace.
No admin API, UI, OTLP, or gRPC port is published on the host or substrate.
ZPR authorization limits which actors can connect; OpenObserve authentication
is still mandatory for **every** ingest or query request. Use dedicated
non-root credentials when the selected OpenObserve edition supports the
required roles. In editions without enforceable write-only/read-only roles,
place an authenticated path-restricting gateway in front of the shared HTTP
port before granting multiple actors access; the sample ZPL by itself cannot
separate HTTP methods or paths on one TCP port. Do not distribute the root
password to collectors or browsers.

## Wire interfaces (v1)

| Purpose | Transport and path | Caller and authorization |
|---|---|---|
| Logs | OTLP/HTTP `POST /api/zpr/v1/logs` | ZPR-authorized publisher; OpenObserve Basic auth, `stream-name: zpr_visa_service`, `Content-Type: application/json` for OTLP JSON or protobuf media type for OTLP protobuf. |
| Metrics | OTLP/HTTP `POST /api/zpr/v1/metrics` | Same network gate, distinct credential where supported; OpenTelemetry resource attributes identify the emitting component. |
| Optional Prometheus sender | Remote write `POST /api/zpr/prometheus/api/v1/write` | Publisher credential; not a public scrape endpoint. |
| Search and dashboards | OpenObserve UI and authenticated query API on TCP/5080 | ZPR-authorized reader via the dedicated reader adapter; do not expose browser credentials through a public proxy. |

The organization identifier is the path segment in these endpoints; use the
identifier returned by OpenObserve's organization API, not its display name.
OTLP clients must send current timestamps, resource attributes
`service.name`, `service.instance.id` and `zpr.net`, and the signal-specific
fields below. The collection side should batch with bounded memory and retry
transient failures without granting access or blocking visa issuance. Avoid
passwords, private keys, authentication tokens and full packet payloads.

The local collector adds `zpr.organization.id` to the OTLP resource on every
metrics and logs batch. Organization activation passes the newly active ZPR
profile ID before restarting the collector. `OPENOBSERVE_ORG` is a separate
OpenObserve storage-tenant setting; changing the active ZPR profile does not
change that tenant. The shared data volume is retained across switches, and
older records are not retroactively retagged.

* **Visa counters:** Poll the existing authenticated `GET /admin/stats` at a
  fixed interval, export its numeric counters as cumulative OTLP sums with
  stable `zpr_vs_*` names, and export uptime as `zpr_vs_uptime_seconds`.
  Detect process restarts via uptime so a reset never looks like negative
  traffic. Non-numeric values must not become metrics.
* **Visa denials:** Poll `GET /admin/visas/denies?since=<epoch-ms>&limit=500`;
  export timestamp, deny code, protocol, source/destination ZPR addresses,
  destination port and collapsed count as structured logs. Deduplicate
  overlapping polls. This source is an in-memory 500-entry window, **not** a
  durable audit log: bursts and Visa Service restarts can lose events.
* **Process events:** Collect the Visa Service's tracing output as structured
  OTLP logs with timestamp, level, component and message. Redact secrets
  before sending. The current process emits to stdout/stderr; file or socket
  collection requires a separate collector at the source.
* **Policy signals:** The evaluator retains signal messages in visa metadata,
  but does not deliver them to a destination today. Treat signal delivery as
  a separate future producer interface, not as an implemented audit stream.

The profile's `collector.py` polls the authenticated Visa Service admin stats
and denial endpoints, and tails the Visa Service process log in the local
simulator. It exports numeric counters and uptime as OTLP metrics, denials and
redacted process lines as OTLP logs. `collector.sh` runs it inside the dedicated
`telemetry-publisher` adapter namespace and pins only the OpenObserve `/128`
route to that adapter. The collector uses a read-only Visa admin key and an
organization-scoped OpenObserve ingestion token; it never receives the root
password. Polling/export failures are logged locally and do not block Visa
issuance. Denial history remains the Visa Service's bounded in-memory window,
not a durable audit source.

## Deployment

Provision a dedicated adapter and private credentials first. Put
`ZO_ROOT_USER_EMAIL` and `ZO_ROOT_USER_PASSWORD` in an operator-owned
Docker `--env-file` outside the repository, readable only by the operator.
Do not commit, log, or paste the password. The data volume persists across
container restarts and is owned by the non-root OpenObserve process (UID 10001).
OpenObserve requires an 8-128 character password with lower- and uppercase
letters, a digit, and a special character.
The example pins 30-day retention; size, backups, credential rotation, and
restore testing are operator responsibilities. Disable or restrict outbound
connections from the adapter: OpenObserve's optional outbound telemetry is
disabled by default in this image.

```sh
export ZPR_OBSERVABILITY_ENV_FILE=/private/path/openobserve.env
export ZPR_OBSERVABILITY_ADAPTER_NAME=zpr-observability
export ZPR_OBSERVABILITY_ADDR=fd5a:5052:adda:1::54
sh observability/openobserve/run.sh build
sh observability/openobserve/run.sh start
sh observability/openobserve/run.sh status
```

Run these from the `zpr-visaservice` checkout. `run.sh` uses the existing
`zpr-local-linux-node` rig's PID namespace, the same arrangement as the BIND
profile. Provision the policy and public bootstrap keys for both
`zpr-observability` and `telemetry-publisher` first; the latter uses `tun6` and
the dedicated ZPR identity in `zpr-observability.zpl`. Create a private
`collector.env` with the `OPENOBSERVE_ORG` identifier and
`OPENOBSERVE_EMAIL=telemetry-publisher@zpr.local`, and store the one-time
ingestion token in `ingestion.token`, both mode `0600` under the local runtime
observability directory. Set `ZPR_OBSERVABILITY_ENV_FILE` to a private env file
containing `ZO_ROOT_USER_EMAIL` and `ZO_ROOT_USER_PASSWORD`; root credentials are
bootstrap-only. `run.sh start` waits for the service adapter and starts the
collector; `run.sh stop` stops both. No host port mapping is performed.

To make a tenant named `ZPR` the collector's default, create that organization
in OpenObserve, use its exact returned organization ID, and set it in the
private `collector.env` as `OPENOBSERVE_ORG=ZPR`. The ingestion token must also
be scoped to that tenant. If OpenObserve returns a different ID than `ZPR`, use
the returned ID. Restart the collector after changing the tenant or token. This
selects the OpenObserve tenant; `zpr.organization.id` continues to identify the
active ZPR profile that produced each record.

`run.sh status` reports OpenObserve and collector state. To restart only the
collector after changing its code or credentials, run its `stop` and `start`
commands from the checkout. The
collector is best-effort: if the Visa Admin API or OpenObserve is unavailable,
it logs a local warning and retries on the next poll; it never blocks the
Visa Service authorization path. The simulator profile uses the existing
`telemetry-client` identity as a network reader; provision a separate reader
identity and non-root query account before exposing dashboards to other users.

The logger waits up to approximately 60 seconds for both the named adapter
process and its configured ZPR address before starting. Finding a process alone
is not sufficient: address assignment can lag behind adapter startup. New
containers use Docker's `unless-stopped` restart policy, so a failed startup or
OpenObserve process exit is retried unless the operator explicitly stopped it.
This is process recovery, not a guarantee of backend or network health.

If Control Room's Logging server link returns an empty reply, check
`run.sh status` and the container logs before restarting the whole stack. The
host-loopback GUI relay may remain listening while OpenObserve is stopped;
an open relay port does not prove that the logger is available. Startup and
recovery retain the existing data volume and credentials; do not remove the
volume or reseed credentials to fix a readiness failure.