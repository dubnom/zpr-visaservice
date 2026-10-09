# Gateway forwarding rollout preparation

Status: prepared and tested, **not deployed**. This is not draft activation or
certification of external-network policy enforcement.

Prepared candidate: `zpr-gateway-runtime:20261009-reviewed-candidate` (Linux
amd64). Gateway executable SHA-256:
`bf26de64b43baee4aff192553d6b87c6025a4bdccd9edf4ec4ff6f9bfc5251b8`.
The candidate's adapter hash matches the live adapter:
`3e92fb6915e1eafca1d55101cc521b1a2fa80cfe0a8585eb9df853017b508dd6`.
An isolated `--network none` candidate container returned the exact health
JSON and HTTP 403 for an unlisted HTTP proxy destination. The disposable
container/build context were removed; the candidate image is retained locally.
Host-built CLI/socket and all gateway Go tests passed. The production gateway
start time remained unchanged. No deployment, commit or push was performed.
This is a worktree candidate; revalidate the reviewed source/image identity
before deployment, rather than assuming a mutable tag identifies final source.

## Observed production baseline (2026-10-09)

- Container: `great-lakes-internet-gateway`, image `zpr-multinode`.
- Immutable image ID:
  `sha256:84c93367df0bcdc8cd36e320dcdc30cd235d7a243deea7570f1f3d67f5ab128e`.
- Platform: Linux amd64; executable `/app/bin/zpr-web-dashboard`.
- Executable SHA-256:
  `d258078212e4ec047673da8512fef4e63ffae87ec94fb66a17d1dae89dc57226`.
- Started at `2026-10-09T01:23:33.455076423Z`.
- Actual process: `web-gateway-service`, workload `internet-gateway`,
  allowlist `*.google.com`, Visa-assigned IPv6 listener on port 8082.
- Entrypoint: organization-independent `entrypoint-internet-gateway.sh` from
  the demo repository; adapter `/app/bin/ph` uses `tun10` and connects to the
  operator-supplied node address. This observation is not a Control Room
  dependency on Simulator configuration.
- Bind mounts: entrypoint read-only, organization `/conf` read-only, `/logs`
  writable. Network: `zpr-great-lakes`, alias `internet-gateway`; restart policy
  `no`. Keep these settings unchanged unless separately approved.
- No fixed-upstream gateway process was found among running gateway containers.
  That does not establish that all stopped or external callers have migrated.

Replacing Control Room or Control-Service does **not** deploy this gateway
executable. The target is the separate gateway container; its adapter will
restart with the container. Allow for a temporary loss of gateway connectivity.

## Validation

Run from the dashboard directory:

```sh
sh scripts/test-gateway-runtime.sh
```

The script builds the actual dashboard executable, starts its web-gateway CLI
on an isolated IPv6 loopback listener, checks the exact health JSON and verifies
unlisted hosts/ports are denied over actual HTTP and CONNECT sockets. Simulator
is unavailable. The same run includes gateway forwarding/configuration tests:
seven methods and bodies over controlled HTTP/HTTPS, request/response limits,
header filtering, redirects, health mutation rejection, and shared response
modes. These controlled handler tests do not prove public DNS/connectivity or
live Visa routing; the binary smoke test does not claim successful TLS egress.

The [candidate Dockerfile](Dockerfile.gateway-runtime) overlays **only**
`/app/bin/zpr-web-dashboard` on an explicitly selected existing gateway image.
It retains the adapter, tools and entrypoint configuration. Cross-build the
binary for the observed architecture, not the host architecture:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -o /private/temporary-context/zpr-web-dashboard ./cmd/zpr-web-dashboard
docker tag sha256:84c93367df0bcdc8cd36e320dcdc30cd235d7a243deea7570f1f3d67f5ab128e \
  zpr-gateway-runtime-baseline:reviewed
docker build --platform linux/amd64 --build-arg GATEWAY_BASE_IMAGE=zpr-gateway-runtime-baseline:reviewed \
  -f scripts/Dockerfile.gateway-runtime \
  -t zpr-gateway-runtime:reviewed-candidate /private/temporary-context
```

The temporary context must contain only the binary, not operator certificates,
keys, runtime configuration or logs. Reinspect live image/hash/configuration
before a rollout; these values are an observed baseline, not an evergreen
deployment recipe. Keep a private complete container snapshot and record the
candidate executable/image hashes. Do not publish environment dumps.

## Explicit rollout and rollback gates

1. Obtain separate deployment approval and a gateway outage window. No rollout
   is performed by the validation script or image build.
2. Verify current adapter identity/node, host allowlist, IPv6 listener,
   mounts, environment, capabilities/devices, networks/aliases and restart
   policy against the snapshot. Abort on unexpected drift.
3. Stop and retain the old gateway container under an unused rollback name.
   Clone its exact Config/HostConfig/network settings with only the reviewed
   candidate image changed. Do not recreate the organization/node or alter
   operator credentials. Preserve the actual web-gateway mode.
4. Verify adapter registration/active link and the assigned listener, health
   contract, denied host/port cases, and controlled allowed HTTP plus HTTPS
   CONNECT requests through the **actual Visa path**. Use only a separately
   approved non-mutating destination; never send real POST/PUT/PATCH/DELETE
   probes to Google's production endpoints. Seven-method mutation tests use
   controlled fixtures, not public services.
5. Confirm the executable hash inside the running container matches the
   candidate, and compare configuration, identity and unrelated service starts.
   Check Control Room gateway inventory without treating draft-save success
   or static-asset checks as runtime evidence.
6. On failed acceptance, stop/remove only the replacement container, restore
   the retained old container's original name, start it, and reverify adapter
   link/listener and the preserved runtime configuration. Keep the old image
   and rollback container until explicit release acceptance.

## Enforcement and migration boundaries

| Surface | Current enforcement |
| --- | --- |
| Saved gateway draft | Validated/stored configuration, not runtime activation |
| Fixed-upstream `/fetch` | Seven methods, selected headers, 2 MiB body/response limits; separate origin setting |
| Web proxy HTTP | Host/port allowlist, public resolved addresses, header filtering, streaming response |
| HTTPS CONNECT | Allowed destination host and port 443; encrypted methods/paths/bodies are opaque |

Draft paths/methods/limits must not be advertised as enforced by the web proxy.
Its wildcard host patterns are runtime-only; saved draft hosts remain exact.
Do not silently convert fixed-upstream clients to proxy syntax: `/fetch` URL
translation, selected credentials/headers and response limits differ.
Inventory every fixed-upstream caller, choose an explicit production runtime
contract, migrate and test callers, then retire the mode only with approval.
Draft activation/rollback and external-network policy metadata remain blocked
on the independent production provisioning contract. No HTTPS inspection or
termination policy has been selected.
