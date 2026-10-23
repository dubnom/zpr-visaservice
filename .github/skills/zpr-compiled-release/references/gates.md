# ZPR release gates and evidence contract

This is a reusable checklist, not a release status report. Consult the current
recovery baseline and exact artifact receipts for completed work. Do not embed
current private paths, daemon IDs, credentials or moving artifact hashes here.

## Gate dependencies and proof

| Gate | Prerequisite | Required proof | Does not establish |
|---|---|---|---|
| Private baseline freeze | Authorized inventory/pause/storage | Consistent backup and verified checksums; preserved Git branches/worktrees; restoration path | Sanitized release or encryption unless separately verified |
| Release inputs | Inventory and provenance | Committed revisions/archives, gitlinks, lockfiles, toolchain/base identities and asset classification | Redistribution permission |
| Rust release | Exact source inputs and offline cache | Seven verified ARM64 binaries; successful optimized, non-test, locked/offline Cargo receipts; output/lock hashes | Native loader compatibility or trust |
| Compiled archive | Clean committed candidate and verified Rust inputs | Eleven allowlisted tools, five organizations/seventeen scenario assets, file/archive inventory, safe modes and secret review | Complete installation |
| Scratch smoke | Exact compiled archive and isolated daemon | Five policy compositions, two independent trust generations, explicit non-overwrite rejection; eight checks | Dynamic Rust execution |
| Native Rust/policy | Exact extracted archive and documented Linux libraries | Seven CLIs, five fresh-key signed bundles, invalid syntax rejected with exit 1/no bundle | Server-side signature enforcement |
| Fresh service | Exact tools/helper and isolated loopback namespace | Independent identities, owner-only state/non-overwrite, trusted HTTPS, anonymous/foreign/read-only rejection, authorized install, Redis AOF/Visa restart | Node/peer/service adapter trust |
| Native node | Fresh service and authorized private TUN/namespaces | Explicit RSA signature rejection plus no node, then expected CN/address, PH grant, VSS registration, synchronized actor and pending installs zero | Noise handshakes, forwarding, node lifecycle or container least privilege |
| Runtime inputs | Reviewed immutable public base and explicit online acquisition | Saved base plus all exact packages with versions/architectures/hashes; script receipt | Same packages on future online acquisition |
| Runtime assembly | Runtime inputs and accepted exact archive | Offline RUN, local base identity check, no source/compiler/private state; image archive/recipe/input hashes | Byte-identical build, license clearance or full install |
| Runtime acceptance | Exact exported/reimported image | UID 501/GID 1000, read-only root, cap-drop ALL, no-new-privileges, no external network/mounts/ports; archive digests, policies, fresh service and durable restart; verified cleanup | PH device/network permissions, full-demo sizing |
| PH node container | Runtime image and authorized isolated internal network/TUN | NET_ADMIN-only namespace setup, UID-owned TUN, nonroot zero-capability PH; missing device/wrong-owner rejection, fresh RSA admission and process restart/re-admission; separate node-only state and verified cleanup | Adapter dynamic addressing, Noise peers, forwarding, revocation or container/VM restart |
| Adapter/peer/lifecycle | Fresh trust and explicit launch contract | Least required device/caps/network configuration; real Noise trust, Visa/service admission, allowed/denied forwarding, restart/revocation and durable state | All demo scenarios |
| Production operator bootstrap | Independent service contracts | Operator configuration from production providers; Control Room/Service pass with Simulator unavailable | Simulator organization provisioning |
| First-run setup | Supported platform/prerequisites and fresh service bootstrap | Loopback one-time token, fresh admin/trust, explicit errors, safe resume/non-overwrite, authenticated/CSRF-protected later settings | Optional AI enabled automatically |
| Optional AI | Secure setup/configuration | Configure/skip, visible send-data caveat, user key/cost notice, generic connectivity-test payload only, server-side secret storage/replace/remove; core works disabled | Permission to send private policies automatically |
| Full demo acceptance | Runtime, adapters, setup and fresh seeds | Five organizations, all seventeen original scenarios, browser acceptance, restart/restore, no source/compiler/external image download after prerequisites | Publication authorization |
| Distribution review | Full component/license inventory | Redistribution rights/notices/SBOM review; static/manual secret review; measured download/rootfs/import/peak install disk against agreed budget | Promotion or upload approval |
| Promotion/publication | All release gates and explicit approval | Reviewed Git target and exact accepted artifact; approved destination/version/checksum; verified published bytes | Permission to publish private backup |

## Evidence rules

Record:

- Input/output SHA-256, source revisions, exact companion gitlinks, lockfiles,
  toolchain/recipe/harness identities, image identity, expected daemon identity,
  architecture, command and exit status.
- Compressed byte counts and independently measured extracted/rootfs payload.
  Label Docker layer accounting, export-tar size, installed filesystem overhead
  and peak install disk separately; do not substitute one for another.
- Which exact checks ran, measurable thresholds, limits, negatives and failure
  reasons. Keep unchanged warnings and incomplete gates visible.
- Permissions/network/capability/mount limits and whether Simulator was absent.
  Do not infer least privilege from a root namespace test.
- Cleanup outcome, test ownership and stopped/running VM/context status.

Public-safe evidence still needs review. Typical allowlist:

- Build/input manifests with approved public fields, checksums, selected package
  inventory, CLI results, reviewed compiler warnings and public certificates.
- Bootstrap inventories without credentials, selected policy-install API shapes,
  reviewed Redis restart/AOF proof and exact negative-test reasons.
- Selected node CN/address and synchronization fields, accepted/rejected CN-only
  inventories, grant/rejection lines and private-test routes.

Never export or publish:

- Private keys, API-key/header files, AI credentials, databases, generated signed
  state or unreviewed raw logs/support archives.
- Raw actor descriptors, identity/JWT/token attributes, directory passwords,
  browser sessions or recovered snapshots.
- Builder sources, caches or receipts containing secret-bearing environments,
  absolute private data paths or command-line credentials.

Keys and raw runtime evidence generated by native probes are private even when
the seed data is fictional. Preserve selected reviewed receipts first, then
delete exact owned private state for every attempt, including failures.
Disposable-container cleanup must remove its private volumes as well.

## Known harness pitfalls

- Wrong source address can cause node rejection before RSA verification. Require
  the exact signature-rejection reason; never count generic failure as proof.
- Preserved PH applies config authentication keys after CLI options. Select a
  foreign negative-test key in explicit config, not a supposedly overriding flag.
- Native service/node preparation is path-bound through private Unix socket and
  config paths. Do not describe generated state as relocatable.
- Insecure legacy admin binaries are not interchangeable with TLS-fixed verified
  outputs. CLI failures must return exit 1 and empty stdout; no insecure flags.
- `vsapikey` exposes `--help`, not necessarily `--version`.
- APT acquisition may need ownership capabilities; that says nothing about runtime
  privilege. Do not add those capabilities to service containers.
- Dockerfile `FROM sha256:...` can resolve as a repository name. Load the saved
  base, verify identity and use the builder's local digest-derived tag.
- A matching daemon ID guards selection, not isolation. Verify target provenance
  and separation from live environments independently.
- Green container acceptance can leave a reimported image intentionally cached;
  no running containers/private state should remain. State retained image/cache
  ownership honestly rather than claiming every byte was removed.

## Resume after failure

Stop at the failed prerequisite. Retain private diagnostics while investigating;
do not silently use an older successful artifact or modify production gates.
Correct the harness/root cause, use a new output path, rerun affected gates and
bind new receipts to the new exact artifact. A failed cleanup is a failure, not a
passed gate with an invisible warning. Do not bypass approval or isolation limits.
