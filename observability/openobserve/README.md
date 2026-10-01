# ZPR observability service

**Deployment profile, not an active simulator service.** This profile runs the
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
The example grants access only to publisher adapter CN `vs.zpr` and reader
adapter CN `ops-observer`. Adapt these to real provisioned identities; a
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

`zpr` is the organization in these paths; create it and provision users before
ingestion. OTLP clients must send current timestamps, resource attributes
`service.name`, `service.instance.id` and `zpr.net`, and the signal-specific
fields below. The collection side should batch with bounded memory and retry
transient failures without granting access or blocking visa issuance. Avoid
passwords, private keys, authentication tokens and full packet payloads.

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

No exporter for the existing Visa Service counters/denials or tracing output
is wired by this profile yet: deploying OpenObserve alone creates an empty
store. Collection must run on an authorized ZPR actor and keep the admin API
key separate from the OpenObserve ingestion credential. The service must stay
independent of Visa Service availability; telemetry failure must never relax
policy or stall the authorization path.

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
profile. It waits up to 60 seconds for the named adapter and verifies the
address is actually assigned before starting. If either prerequisite fails,
the container exits; inspect its logs with `docker logs zpr-observability`.
The root credentials are bootstrap-only. No host port mapping is performed.