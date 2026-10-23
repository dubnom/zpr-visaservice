---
name: zpr-compiled-release
description: 'Build, reproduce, validate, or resume the ZPR compiled Linux ARM64 demo release. Use for baseline recovery, offline Rust/Go builds, sanitized downloads, runtime images, fresh Visa/node trust, clean-install acceptance, and release evidence. Keeps private backups and live demos separate from distributable artifacts; never treats engineering checks as publication approval.'
argument-hint: 'Release gate or artifact to reproduce; isolated target and protected output directory'
---

# ZPR compiled release

## Purpose and scope

Drive the repeatable build and acceptance workflow, using the project's existing
scripts as the executable implementation. Do not duplicate their logic here or
replace verified checks with agent judgment. An engineering slice is not the
complete demo, a supported installer, or an approved public release.

This skill belongs to the Visa Service repository. Resolve its root relative to
this file (`../../..`), not from an assumed terminal directory. Read repository
instructions before changing code. Commands below run from `zpr-dashboard/`
unless explicitly stated otherwise.

Read [the gate and evidence contract](./references/gates.md) when selecting a gate,
interpreting results, or deciding what can be retained. Read the existing
[packaging README](../../../zpr-dashboard/packaging/compiled-recovery/README.md)
for current script interfaces and prerequisites. Read
[the recovery baseline](../../../RECOVERY-BASELINE.md) for historical acceptance,
not for reusable machine paths, daemon IDs, credentials or artifact hashes.

## Non-negotiable boundaries

- Do not alter running recovery/demo environments or original dirty worktrees.
  Work in the explicitly designated candidate and an isolated test target.
- Preserve experimental work before any destructive Git recovery. Do not reset,
  promote main, force-push, deploy, upload to zpr.net or push images without
  separate explicit authorization. A request to continue testing is not that
  authorization. Never work around an approval denial.
- Private exact-state backups and public release inputs are separate products.
  Never seed a distributable image with recovered databases, credentials, logs,
  sessions or live container layers.
- Control Room/Control-Service must operate without Simulator. They cannot call
  its APIs or read simulation manifests/profiles/sessions/workloads. Simulator
  may exercise independent production operator contracts, not the reverse.
- No automatic AI calls containing private policies, directory data or secrets.
  AI remains optional, with installer-supplied credentials and disclosure.
- Use explicit Docker context and independently confirmed daemon identity on
  every operation. Never change the default context or infer isolation from a
  name/ID alone. Do not pass the host Docker socket into build/test containers.
- Never weaken TLS, source-address checks, policy enforcement or authorization
  to make a harness pass. Prove the intended negative failure reason.
- Clean only explicitly owned paths/resources and exact owned PIDs. Never use
  process-name killing, global prune, broad directory deletion or shared flush.

## 1. Establish the request and resumable state

1. Determine whether the user wants reproduction of an existing accepted
   artifact, a new engineering gate, or completion of the whole release.
   Do not silently reduce whole-release scope to a passing smoke test.
2. Inspect candidate branch/HEAD/status and applicable repository instructions.
   Integrate existing changes; never discard unrelated edits. The archive
   builder requires clean committed source.
3. Identify the current accepted archive and its checksum/evidence. Verify
   receipts rather than trusting previous chat claims or directory names.
4. Confirm the test environment is separate from all live deployments. Record
   protected environments, context, expected daemon identity, architecture,
   default context and VM lifecycle ownership. Ask for missing authorization
   or a target selection before operations that could affect shared resources.
5. Select canonical absolute paths for protected new outputs. Existing outputs
   are refused; after failure inspect evidence and use a new output directory.
6. Track unfinished gates and dependencies. Historical receipts describe only
   the exact hashes they exercised. Never mark a gate passed for changed bytes.

Use operator-selected values for `BUILD`, `RUST_RELEASE`, `RUST_INPUTS`,
`RUNTIME_INPUTS`, `IMAGE_BUILD`, `EVIDENCE`, `CONTEXT`, `DAEMON` and `BASE`.
`BASE` is a reviewed `ubuntu@sha256:<digest>`, never a moving tag.
No example variable implicitly selects a machine or authorizes provisioning.

## 2. Freeze and classify before recovery or publication work

For baseline recovery, inventory revisions, dirty overlays, runtime executables,
images, asset seeds, ports and mutable state before changes. Capture/verify a
private consistent backup only with the necessary pause/storage authorization.
Preserve experimental branches/worktrees and record restoration instructions.

Classify each input as distributable, generated during setup, private-only or
excluded. Unknown provenance or redistribution rights block public publication.
Full backup encryption/storage and release license review are independent gates;
do not infer them from successful compilation.

## 3. Build or reuse verified native binaries

Skip rebuilding only when the exact release binaries and independent source/
lock/profile receipts verify and satisfy the requested gate.

For new Rust builds, prepare exact committed companion-repository archives,
including Common's own policy/VS API gitlinks. Do not substitute a sibling
revision for a recorded gitlink. Stage only authorized sources into an isolated
native ARM64 builder with pinned image identity, network disabled, read-only
sources and separate writable offline Cargo cache/output. No secrets/socket.

Inside that builder:

```sh
sh packaging/compiled-recovery/build-rust-release.sh /input/sources /build/new-release
```

The script path is relative to the dashboard source; use the mounted absolute
path if the builder's working directory differs. Generate the independent
input receipt from the actual archives supplied to the builder, not retrospectively
from output. Copy out only required binaries and receipts, then verify:

```sh
node packaging/compiled-recovery/verify-rust-release.cjs "$RUST_RELEASE" "$RUST_INPUTS"
```

All seven tools require successful locked/offline release-profile evidence,
optimization level 3, non-test/debug-assertion-disabled profiles, exact lock/output
hashes and ARM64 ELF architecture. Stripping alone proves none of this.
Run focused source tests for changes. Record inherited warnings honestly;
do not suppress or fix unrelated warnings just to claim a clean lint result.

## 4. Assemble and test the exact compiled archive

Use existing cached dependencies; do not download implicitly:

```sh
GOPROXY=off GO="$GO" sh packaging/compiled-recovery/build.sh \
  "$BUILD" "$RUST_RELEASE" "$RUST_INPUTS"
node packaging/compiled-recovery/smoke.cjs "$BUILD" "$CONTEXT" "$DAEMON"
```

`GO` is the selected executable path. The builder also supports a Go-only slice,
but that does not satisfy the eleven-tool native/runtime gates. Verify checksum,
membership, modes, source revision, five organization seeds and seventeen scenario
assets. Preserve measured compressed/extracted sizes and smoke receipts.
The scratch gate does not execute dynamic Rust tools.

On the isolated native ARM64 target, transfer and digest-check the exact archive,
extract into a new protected directory and verify `SHA256SUMS`. Use those extracted
tools and assets, never an earlier staging tree:

```sh
sh packaging/compiled-recovery/probe-rust-runtime.sh \
  /absolute/extracted/bin /absolute/extracted/share/examples /protected/new-policy-evidence
```

This creates private signing material. Five signed policy builds are required;
they do not prove Visa enforces signatures.

## 5. Qualify fresh trust independently of Simulator

Follow the packaging README's isolated namespace commands for
`probe-visa-service.sh`, passing the exact archive's preparation helper and tools.
Run the default service-only gate unprivileged. Run optional `--node` separately
only with authorization for root inside disposable network namespaces/TUN.
Do not run either probe in a live host namespace or reuse shared Redis/state.

Require explicit TLS/API permission failures and durable Redis/Visa recovery.
For node mode, require actual RSA signature rejection and empty node inventory,
then exact accepted CN/address, VSS registration, synchronization and zero pending
installs. Preserve source-address checks. The inherited PH config overrides its
CLI authentication key; the negative probe uses an explicit foreign-key config.

Native root-only node success is not a least-privilege container contract or
proof of Noise peer trust/forwarding. Do not turn on privileged Docker as a
shortcut for an unqualified PH/adapter launch.

## 6. Assemble and reimport the offline runtime image

Separate explicit online public-input acquisition from offline assembly:

```sh
node packaging/compiled-recovery/acquire-runtime-inputs.cjs \
  "$RUNTIME_INPUTS" "$CONTEXT" "$DAEMON" "$BASE"
node packaging/compiled-recovery/build-runtime-image.cjs \
  "$BUILD" "$RUNTIME_INPUTS" "$IMAGE_BUILD" "$CONTEXT" "$DAEMON"
node packaging/compiled-recovery/probe-runtime-image.cjs \
  "$IMAGE_BUILD" "$EVIDENCE" "$CONTEXT" "$DAEMON"
```

Retain the saved base and every hash-bound package, not just metadata. Initial
APT acquisition can change over time; exact retained inputs enable offline
assembly, not a claim of byte-identical images. Build network `none` governs
RUN steps; identity-checked local base loading avoids registry resolution.
Never replace it with an unpinned recovery image.

The probe verifies and reloads the saved gzip export, checks all eleven hashes,
runs native policies and the unprivileged service gate, and deletes its containers
and private volumes. If proving a clean reimport, remove only the explicitly owned
built image after saving/checking it, verify absence, then load/probe the export.
Removing its last tag may already remove the image; inspect before another remove.

This all-tools image qualifies loopback Visa/Redis only. PH networking, service
decomposition, supervision, backup/restore and full installer remain separate.

For the additional preconfigured PH node container contract:

```sh
node packaging/compiled-recovery/probe-node-image.cjs \
  "$IMAGE_BUILD" "$EVIDENCE" "$CONTEXT" "$DAEMON"
```

Use new evidence. This requires an isolated Docker target with internal IPv6
network support and existing Linux TUN. Short-lived setup helpers receive
NET_ADMIN only inside owned container namespaces; PH itself runs nonroot with
zero capabilities, a preowned TUN and separate node-only credentials. Require
missing-device/wrong-owner failures, foreign RSA rejection, synchronized admission
and PH process restart/re-admission. Do not count it as adapter dynamic-address
permissions, Noise peer trust, forwarding or container/VM restart acceptance.

Add explicit `--adapter` to that probe for fresh adapter-to-node Noise/RSA trust
and allowed/denied TCP to the node local actor. It needs the test-only
`prepare-adapter-probe.sh` fixture alongside the harness. Require foreign CA/RSA
rejection with no client actor, a Visa-granted dynamic address installed on TUN,
mutually verified peer names, allowed PONG and explicit policy NoMatch plus a
live denied listener. The current adapter runs root with NET_ADMIN only for
dynamic address updates; node/Visa remain nonroot/capability-free. Provisioning
also uses a temporary root CHOWN/DAC_OVERRIDE-only file transfer helper. Record
those privileges honestly. Resolve the actual namespace interface index for
the adapter's scoped IPv6 bind. This is not node-to-node or adapter-to-adapter
forwarding, a nonroot dynamic-adapter solution, or a Visa/service adapter gate.
Failed probes may save raw logs under private-failure; review and remove them,
never publish them as selected acceptance evidence.

## 7. Verify, sanitize, persist and report

For packaging changes run the focused suite from `zpr-dashboard/`:

```sh
node --test packaging/compiled-recovery/runtime-image.test.cjs \
  packaging/compiled-recovery/prepare-visa-service.test.cjs \
  packaging/compiled-recovery/verify-rust-release.test.cjs
```

Add focused tests for new behavior. For Rust/Go changes also follow repository
test requirements. Rerun exact affected artifact gates after implementation
changes; negative input tests do not replace native execution.

Preserve only reviewed evidence described in [the evidence contract](./references/gates.md).
Verify thresholds and hashes programmatically. Remove all owned generated private
state from successful and failed attempts, verify no owned processes/containers/
volumes/test links remain, and stop only a VM started for this validation session
when no further dependent work needs it. Never stop an unrelated or live VM.

Update the packaging README and recovery baseline with current behavior,
artifact/checksum/measurements, failures, remaining limits and acceptance.
Update the external release plan if present; it is not a hidden build dependency.
Commit only intended source/docs on the candidate when authorized, with required
trailers; do not promote main. Preserve receipts outside distributable content.
Report what passed, what remains, cleanup state and reusable artifact links.
If an environment prerequisite or approval blocks progress, state the exact
blocker rather than producing success-shaped output.

## Definition of done

For a requested engineering gate: its exact artifact passes required positive
and negative checks, receipts persist, private state is cleaned, docs agree and
protected environments are unchanged.

For the full downloadable product: **all** product gates in the reference must
pass, including clean installation, setup/AI disclosure, all scenarios/browser
acceptance and licensing. Only explicit approval permits publication or Git
promotion. A skill is workflow guidance, not an enforcement hook or permission.
