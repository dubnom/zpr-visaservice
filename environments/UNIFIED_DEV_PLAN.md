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

## Workbench dev results (2026-10-28, Simulator absent)

Install `a26f4b9e392b516eb0c0df59`, state `~/zpr-dev/state-dev` on the
workbench (Ubuntu 24.04 amd64), started with `-visa-service -data-plane
-protected-dns -data-plane-endpoints 3`, packaged image pins, platform amd64.
Linux fixes: host-arch image build, BIND config staged for the `bind` user, and
state ownership restored on stop.

| Gate | Evidence |
|---|---|
| Node + Visa | Launcher health: node and `vs.zpr` admitted; endpoint-01 and client-01 admitted. |
| Protected DNS | From client-01 over ZPR: `dns.svc.zpr`, `vs.zpr.adapters.svc.zpr`, `endpoint-01.adapters.svc.zpr` resolve. |
| Logging | Launcher: ingest/query and retention across restart. |
| Allow/deny | client-01 to endpoint-01: ping, tcp/7007, udp/7008 reply; tcp/7009, udp/7010 dropped; ping to Visa address denied (`no match`). |
| Control Room | HTTPS UI serves with the install CA. Named-operator login and Visa views not yet exercised in a browser. |

Finding: a ping sent before its target adapter existed kept failing after the
target connected (no Visa request was made) until the sending adapter was
restarted. To be root-caused; probe only after peers are up.

## Update 2026-10-28 (evening): single-operator review and fresh dev install

- Two-person review removed everywhere (commits `b9a0875` and the launcher/docs commit after it). Submit and approve remain separate steps; a proposer's own approval is allowed and flagged `self_review` in the audit, UI and server log (`AUDIT WARNING`). The launcher now grants the administrator `subject-link.approve`; the second login (`reviewer@evaluation.invalid`) is optional.
- Operator grants are written once per state, so the dev install was recreated: the previous state is retained, stopped, at `~/zpr-dev/state-dev-v1`; the active state is `~/zpr-dev/state-dev` (install `9b2c3e8c6374338b207b18c1`). New CA and new passwords.
- Subject-link snapshot delivery runs on Linux (bound to the data-plane network gateway, `host.docker.internal` added).
- Probes on the fresh install: tcp/7007 and udp/7008 reply, tcp/7009 denied. Open issue (ICMP): after a client's first probe, a probe of the other kind to the same destination gets no reply. Ping first then tcp/7007 fails; tcp/udp first then ping fails; restarting client-01 clears it. Not yet root-caused (adapter DP-K03 vs stock, or Visa-side dedup).
- Endpoints 01-03 are Simulator-launched; endpoint-01 was started by hand for probing.

## ICMP/TCP stale-visa finding: root cause and fix (2026-10-28)

- Symptom: after one probe type succeeded, a probe of another type to the same endpoint got no reply until client-01 restarted.
- Cause (zpr-core adapter `ph`): when a re-docked adapter's old link was removed, the node revoked only visas with a stream on that link. Reverse-direction visas embedding the old A2A DH key survived, were matched for new flows, and the far adapter derived a stale shared secret (`dropping packet because MICV Failure` in the client log).
- Fix: zpr-core `dp-k03/adapter-reauth` commit `5530859` adds `VisaTable::revoke_for_a2a_key`, called from `Assembly::drop_peer`, with a unit test.
- Evidence on a fresh install (`a6bdd8e6170fc9046e3162c7`, rebuilt image): two rounds of ping, tcp/7007, udp/7008, ping, tcp/7009 gave replies for the allowed ones and no reply for tcp/7009 every time.
- Not yet done: a soak with repeated client restarts, and the 4 h DP-K03 run.
