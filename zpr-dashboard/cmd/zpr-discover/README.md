# ZPR migration discovery CLI

`zpr-discover` is the first standalone slice of network discovery for migrating
an existing environment to ZPR. Run it on the operator-selected machine whose
network perspective you want to inventory. It does not remotely select or
deploy a scanner, require a running ZPR network, or depend on Simulator.

The command makes bounded TCP connection attempts, sends one ICMP echo per
target, looks up reverse names, and exports a versioned JSON inventory.
Optional imported adapter data can enrich
that inventory with host-local listeners, process names and service labels.
Live adapter collection, Control Room integration, migration policy generation,
UDP probing and automatic adapter installation are **not implemented**.

## Build and preview

From the dashboard module:

```sh
go build -o bin/zpr-discover ./cmd/zpr-discover

bin/zpr-discover \
  -scanner-id migration-laptop \
  -organization example \
  -targets '192.0.2.0/28' \
  -ports '22,53,80,135,389,443,445,636,1433,2049,3306,3389,5432,6379,8080,8443' \
  -dry-run
```

These documentation addresses must be replaced with an authorized real scope.
Previewing validates the scope and any imported inventory, but opens no network
connections. Scanner/organization labels are operator-supplied provenance,
not authenticated identity or authorization.

Only scan networks you have permission to inventory. An actual scan requires
the explicit `-authorized` flag; it is a confirmation, not a permission system.
There are no default targets or ports, implicit local-subnet
discovery, retries, credential attempts, exploit checks, host configuration
changes, or policy activation.

## Configuration file

Supply `-config path/to/discovery.json` to load options from a JSON object.
The [example config](examples/discovery-config.json) uses documentation-only
targets; replace them with your authorized scope.

```sh
bin/zpr-discover -config cmd/zpr-discover/examples/discovery-config.json -dry-run
# After editing/reviewing the scope:
bin/zpr-discover -config my-discovery.json -authorized -output inventory.json
```

Keys match flag names without the leading dash, including `scanner-id`,
`organization`, `targets`, `ports`, `ping`, `reverse-dns`, `protocol-checks`,
`workers`, `rate`, `timeout`, `duration`, `adapter-inventory`, `output` and
`dry-run`. **`targets` is a nonempty list of strings, one IP, CIDR or dash
range per item** (not a comma-separated string). Port specifications and
durations remain strings; worker/rate values are integers; switches are JSON
booleans. For example:

```json
{
  "targets": [
    "10.0.100.0/22",
    "192.0.2.10-192.0.3.20",
    "198.51.100.10-50",
    "2001:db8::1-2001:db8::ff"
  ]
}
```

This fragment illustrates the target format; scanner/organization labels and
ports are still required in a real config or on the command line. An explicit
IP is also valid as an item. Dash endpoints are inclusive and must ascend
within the same address family. The abbreviated final-octet form is IPv4-only.
Use full endpoints for ranges spanning subnets or for IPv6. Explicit dash
ranges keep their endpoints, including `.0` and `.255` when otherwise valid;
only CIDRs omit IPv4 network/broadcast addresses.

The CLI `-targets` flag still accepts comma-separated entries. Explicit CLI flags override
file values, including `-ping=false` and `-reverse-dns=false`. Defaults apply
to omitted fields. A CLI `-targets` value replaces the whole config list; it
does not append to it. No config file is loaded implicitly. Existing configs
must convert a string `targets` value to an array; the old type is rejected.

`authorized` and nested `config` keys are rejected: actual scans always need
command-line authorization. Unknown/duplicate keys, nulls, wrong types, trailing
data and files over 64 KiB are rejected before network activity. File paths
in the config are relative to the process working directory, not the config
directory. The existing specification-size/port, timeout, duration and concurrency limits
also apply to configuration values.

## Ping and reverse names

**Ping and reverse DNS are enabled by default.** This changes the original
TCP-only default; use `-ping=false -reverse-dns=false` to retain that behavior.
Dry runs perform neither check and do not require a ping executable.

- Ping sends one numeric-address ICMP echo using the system `ping` utility
  (`ping6` for IPv6 on macOS), with no shell, hostname resolution or privilege
  escalation. Supported ping backends are macOS and Linux; Linux requires a
  working ICMP-capable ping installation/permissions. On other systems,
  including Windows, explicitly disable ping; TCP and reverse DNS remain
  available. A missing utility is an explicit error, never a silent skip.
- Ping results distinguish `reply`, `no_reply`, `timeout`, `cancelled`,
  `error` and `not_attempted`. Utility/permission failures are recorded
  separately from ordinary no-reply/timeouts. Utility diagnostics are bounded
  to 4096 bytes. Runtime verification currently covers macOS only.
- Reverse lookup uses the scanner's configured system resolver for each target,
  including hosts with no ping reply or open port. It can send PTR queries
  to that resolver and its upstreams; consider infrastructure-name/address
  disclosure when choosing DNS configuration. Results distinguish `resolved`,
  `no_records`, `timeout`, `cancelled`, `error` and `not_attempted`.
  Returned names are sorted/deduplicated, but remain unverified resolver
  evidence, not authenticated identity or forward-confirmed hostnames.
  The rate limiter bounds lookup invocations, not individual DNS packets;
  system resolver policy may retry queries or contact multiple servers.
- Neither result gates TCP checks, changes the target IP, or enables additional
  targets. No ping reply or PTR record does not mean a host is absent.
- Each host check has the same per-operation timeout as a TCP connection.
  Host checks share the worker pool, start-rate limiter and total scan budget
  with TCP checks. They can make a previously sufficient budget too short.

## Run an authorized scan

After reviewing the dry-run output, replace `-dry-run` with `-authorized`:

```sh
bin/zpr-discover \
  -scanner-id migration-laptop \
  -organization example \
  -targets '192.0.2.10,192.0.2.11' \
  -ports '22,53,80,389,443,445,636,3389,5432,8080' \
  -authorized \
  -rate 20 \
  -workers 8 \
  -timeout 1s \
  -duration 2m \
  -output inventory.json
```

JSON goes to stdout unless `-output` names a new file. Progress and errors go to
stderr. Files are created with mode `0600` and published atomically without
overwriting an existing path; choose a new filename for each scan. If the
filesystem does not support atomic hard-link publication, use stdout and
operator-managed storage instead. Treat inventory as sensitive infrastructure
metadata. There is no inventory-upload service; reverse DNS uses the configured
resolver as described above.

Limits:

- **No host-count cap or CIDR-size cap.** At most **128 unique TCP ports**
  per invocation. Config files and CLI target specifications remain bounded
  to 64 KiB; port specifications to 4 KiB.
- Ranges are sorted and merged without expanding every IP into memory.
  Overlapping ranges are deduplicated, including overlaps between CIDRs,
  individual IPs and dash ranges. Scanning iterates addresses lazily in
  sorted order (IPv4 before IPv6).
  IPv4 network/broadcast addresses are omitted for `/30` and larger networks;
  `/31` and `/32` retain every address. For a CIDR spanning multiple subnets,
  only that CIDR's outer network/broadcast endpoints are omitted, not those
  of every contained subnet. An explicit IP/range may reinclude an omitted
  endpoint.
- Hostnames, IPv4-mapped IPv6, unspecified, multicast, link-local and zoned
  addresses are rejected. A range/CIDR intersecting any unsupported addresses
  is rejected as a whole, even if both endpoints are valid; no silent scope
  trimming occurs. IPv6 loopback and global/ULA unicast are supported.
- Default 8 workers, maximum 32. Default 20 check starts/second, maximum
  100 across all workers, with no retry bursts.
- Connection timeout defaults to 1 second; allowed range 10 milliseconds to
  5 seconds. An optional protocol check has a separate timeout of the same size.
- Overall budget defaults to 2 minutes, allowed range 1 second to 30 minutes.
  A large scope can exceed this budget; the export will be explicitly partial.
  Removing the host cap does not remove the rate, timeout or duration limits.
  Large scopes may need multiple operator-selected, non-overlapping runs;
  there is no automatic continuation/resume.
- Ctrl-C/SIGTERM or budget expiry stops new work, cancels in-flight work, and
  exports a partial report when output remains writable.

Exit codes: `0` for a fully attempted scope without unexpected check
errors, `1` for a partial scan or unexpected endpoint/host-check errors, `2` for invalid
configuration or an input/output failure. Closed ports, timeouts and unreachable
destinations are observations, not command failures. A timeout does not establish
whether a host is down or a port is filtered. A complete report means all
requested attempts finished, **not** that all services were discovered.

## Optional protocol checks

Default TCP checks only connect and close, sending no application data to the
TCP endpoint (ping and reverse DNS are separate host checks). Add
`-protocol-checks` only when the target operators permit these interactions:

| TCP ports | Check | Evidence |
|---|---|---|
| 22 | Read at most 1024 bytes, with bounded banner lines | SSH identification prefix; no authentication |
| 80, 8080 | Send `HEAD /` with a scanner User-Agent | HTTP status line; redirects are never followed |
| 443, 8443, 465, 636, 853, 993, 995 | TLS 1.2+ handshake | TLS only, with system-root certificate verification for the target IP |

No HTTP bodies or raw service banners are exported. HTTP requests and TLS
handshakes can still generate logs/load and must not be assumed impact-free.
TLS probing does not disable certificate verification or send application
requests. Private/self-signed or DNS-name-only certificates may fail validation
when accessed by IP; the endpoint remains `open`, and `probe_error` records the
failure without claiming a verified service. A TLS handshake does not prove
HTTPS, LDAP, SMTP or another application protocol.

Other common ports get a `service_hint` based solely on their port number,
with `hint_basis: "tcp_port_number_only"`. Hints are not protocol detection:
for example, open TCP/389 suggests LDAP but is not proof. Services on
nonstandard ports are still recorded as open, without invented identification.

## Imported adapter inventory

An installed adapter could provide more detail than a remote scan: loopback or
wildcard listeners, ports blocked from this scanner, process ownership and
service labels. This version accepts that data **only through an explicit JSON
file**. It does not claim current adapters already expose such an API.

```sh
bin/zpr-discover \
  -scanner-id migration-laptop \
  -organization example \
  -targets '192.0.2.10' \
  -ports '22,80,443,5432' \
  -adapter-inventory cmd/zpr-discover/examples/adapter-inventory.json \
  -dry-run
```

The [example inventory](examples/adapter-inventory.json) shows the version-1
input format:

- Top-level `schema_version: 1` and `hosts`.
- Each host requires `address`, `adapter_id`, `observed_at` (RFC 3339 timestamp),
  and an explicit `listeners` array, which may be empty.
- Each listener requires `transport` (`tcp` or `udp`), `bind_address`, and
  `port` (1-65535). Optional `process` and `service` are informational labels.
- Maximum input 2 MiB and 4096 listeners per host; no fixed host-count cap.
  Unknown fields, invalid/duplicate hosts and hosts outside the scan scope
  are rejected before scanning. Labels are limited to 256 characters.

The export preserves each imported timestamp and labels the evidence
`imported_unverified`. It does not treat imported process/service labels as
protocol confirmation, imply that imported data is fresh, or promote reported
listeners to remotely reachable ports. UDP listeners may appear in imported
inventory, but no UDP network probes are performed. The host address associates
an import with a requested target; bind addresses are local to that host and
may be loopback or wildcard. IPv6 wildcard semantics do not imply IPv4 exposure.

## Export contract, version 2

The production-independent contract lives in
[`internal/discovery`](../../internal/discovery/). Existing fields must retain
their meanings; incompatible changes require a new schema version.

| Field | Meaning |
|---|---|
| `schema_version` | `2` for both reports and dry-run previews; adapter imports remain version `1` |
| `scanner_id`, `organization` | Operator-assigned labels |
| `started_at`, `finished_at`, `deadline` | UTC scan timing and total deadline |
| `transport`, `ports` | Explicit remote probe scope (`tcp` only) |
| `targets` | Compact sorted/merged ranges: `[{"first":"IP","last":"IP"}, ...]`, inclusive endpoints |
| `host_count` | Exact unique target count as a decimal string |
| `workers`, `attempts_per_second`, `timeout_ms`, `protocol_checks`, `ping`, `reverse_dns` | Effective scan limits/options; limiter covers all check starts |
| `complete`, `error` | Completion flag; interruption explanation when partial |
| `planned` | Exact TCP attempt count as a decimal string (`host_count` times port count) |
| `attempted`, `endpoint_errors` | Numeric TCP attempts started and unexpected connection errors |
| `host_checks_planned` | Exact enabled ping/PTR count as a decimal string |
| `host_checks_attempted`, `host_check_errors` | Numeric host-check attempts and unexpected errors |
| `hosts[].address` | Explicit target IP |
| `hosts[].tcp[]` | One result for every requested port, including attempts not started |
| `hosts[].ping`, `hosts[].reverse_dns` | Present only when enabled; `state`, optional `observed_at` and `error`; PTR results include `names` |
| `hosts[].adapter_inventory` | Optional separate, unverified host-reported evidence |

**This is an incompatible change from version 1.** Consumers must read the
schema version. Planned counts are decimal strings so even IPv6 scopes larger
than 64-bit integers retain exact counts without overflow or JSON-number
precision loss. Dry-run `scope.targets` uses the same compact ranges and adds
`scope.host_count`; it no longer enumerates every target address.

Reports allocate host records only as scanning reaches them, plus any imported
adapter hosts (whose evidence is retained even if scanning never reaches
them). `hosts` is sorted by address. A host may have only `not_attempted`
checks if it was queued just before cancellation. A target in `targets` absent
from `hosts` was **not attempted**, not found absent/down. A cancelled scan
before any work may export an empty `hosts` array. Per-host TCP arrays still
include every requested port. Memory and report size grow with reached/imported
hosts, not the expanded requested scope.

`complete` covers all enabled checks, including host checks, not just TCP. Ordinary no-reply,
missing PTR records and timeouts are observations; utility/resolver failures
are counted in `host_check_errors` and cause a nonzero exit.

An endpoint has `port` and `state`. States are `open`, `closed`, `timeout`,
`unreachable`, `cancelled`, `error`, or `not_attempted`. Attempted endpoints
include `observed_at`; successful connections include the scanner's selected
local socket address. Connection failures have explicit `error` details.
Open endpoints may contain `service_hint`/`hint_basis`,
`detected_protocol`/`protocol_evidence`, and/or `probe_error`.
`not_attempted` must never be interpreted as closed or absent.

Reaching a port describes this scanner's network perspective at this time,
not universal reachability, host ownership, intended access or migration policy.
The JSON export is migration evidence for operator review, not a policy draft.

## Verification

```sh
go test -race ./internal/discovery ./cmd/zpr-discover
go vet ./internal/discovery ./cmd/zpr-discover
```

Tests use loopback servers and in-memory connections, not real network scans.
They cover large compact scopes (including IPv6 counts above 64 bits),
inclusive dash ranges, overlap deduplication, target-list config/CLI precedence,
private/no-overwrite export,
port-state distinctions, cancellation, pacing, bounded protocol checks, TLS
verification failures, adapter import validation and evidence separation.
A dependency test rejects nonstandard runtime dependencies in the engine.
