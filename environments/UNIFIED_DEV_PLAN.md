# Unified dev environment plan (workbench)

Decision (2026-10-28, operator): there are two environments, **dev** and
**stable**, both eventually on `dubnom@workbench.local` (Ubuntu 24.04 x86-64,
Docker 29, Compose v5). Dev comes first; stable is promoted from dev when dev
passes acceptance. Release numbers (r2-r6) are irrelevant: dev is simply the
most current development build. The old stacks are not needed.

Dev must be a **working ZPR network** under the
[common minimum](../System%20release%20plan.md#common-minimum-for-a-working-zpr-network).
The r5 paired demo is a component subset (node + Visa + policy + Simulator);
it meets 2 of the 6 gates and is not an acceptance target.

## Approach

The evaluation launcher (`zpr-dashboard/packaging/evaluation/bootstrap.sh
-visa-service -data-plane -protected-dns -data-plane-endpoints N`) already
builds every required capability and was accepted on macOS/arm64 on
2026-10-27 (see its runbook). The work is to run it on the workbench, then
pair the Simulator with it, rather than extending r5 piece by piece.

| Gate | Provided by | Workbench gap |
| --- | --- | --- |
| Node + Visa connected | `-visa-service -data-plane` | data-plane `build.sh` hard-codes `linux/arm64` |
| Protected DNS | `-protected-dns` | none beyond the above |
| Logging | OpenObserve (B3) | packaged pin is arm64; use the local amd64 `zpr-minimal-openobserve:20261027` via `-logging-image/-logging-platform` |
| Control Room + operator login | production Control Room/Control-Service + Dex | Dex pin is arm64; use the local amd64 Dex via `-oidc-image/-oidc-platform` |
| Identity source | starter OpenLDAP | amd64 pin already present |
| Initial policy, allow/deny | data-plane initial policy | none |

Other gaps: no Go toolchain on the workbench (the launcher is `go run`; install
into the home directory, no root); the repositories are not on the workbench
(copy `maturing-integration`, `zpr-core`, `zpr-common`, `zpr-compiler`);
`docker` group access is available, so no sudo is needed.

## Order

1. **Platform:** make `data-plane/build.sh` take its platform from the host
   (default unchanged on macOS).
2. **Toolchain and sources** on the workbench (home directory only).
3. **Cheapest gates first:** start `bootstrap.sh` with logging, directory and
   operator login (B1-B3); verify the lifecycle-event ingest/query and
   operator login.
4. **Data plane + protected DNS** (`-visa-service -data-plane -protected-dns`);
   verify node admission, DNS resolution over ZPR and the allow/deny probes.
5. **Fixes in dev:** adapters carry the DP-K03 re-auth fix (the data-plane
   image is built from `dp-k03/adapter-reauth`).
6. **Simulator pairing** (separate companion; never a dependency of Control
   Room) and the three scenarios, after the preflight work in
   `SIMULATOR_AS_ZPR_CLIENT_PLAN.md`.
7. **Acceptance run** against the six gates, including restart persistence for
   logging and recovery with the Simulator stopped.
8. **Retire** the Mac `stable`/`dev` stacks and old workbench stacks by
   stopping only (state and volumes kept) once dev has passed; promote dev to
   workbench `stable` by a second install with its own state and ports.

## Rules

- Fixed host ports per environment, recorded in `environments/*.json`; dev must
  not collide with the r5 ports (49300, 49385, 49388) while r5 still runs.
- Nothing is deleted. The old stacks stay stopped, not removed.
- The state directory stays outside synchronized folders and is not committed.
