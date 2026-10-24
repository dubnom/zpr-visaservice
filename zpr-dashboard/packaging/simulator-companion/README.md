# Optional Simulator synthetic-directory companion (B5)

**EVALUATION ONLY — NOT PRODUCTION SUPPORTED.** Install this *after* the
[B1–B3 evaluation bootstrap](../evaluation/README.md) is healthy. It is a
separate foreground ordinary client, not a bootstrap dependency or the legacy
Simulator scenario/endpoint launcher. No policy or endpoint/workload setup is
performed; U01–U06 migration and multi-source policy acceptance remain separate.

The companion discovers the **same evaluation organization** from authenticated
`GET /api/assertions/sources`. It never creates/selects another organization.
Its stable synthetic **source ID** must differ from bundled and company sources.
It owns a separate private directory, HTTPS provider, TLS key and bearer token.
It never writes to bundled LDAP, reads bootstrap manifests/configuration, changes
production environment/configuration, or calls private/Simulator setup APIs.
Control Room/Control-Service receive only an ordinary `https-json` registration;
neither has companion-specific configuration, discovery, navigation or runtime
requirements. Use explicit source references for assertion/selected-source reads;
this is **not** source-qualified permission/visa-policy acceptance. Registering a
source does not prove permission policies ignore unreferenced sources. The
separate multi-source verification found provenance/implicit-merge limitations;
permission-policy opt-in remains gated. No policy, default source, aliasing,
merging or selection semantics are changed here.

## Prerequisites

- An already-running evaluation install, Go (same version as the dashboard), and
  an unused loopback port (default `19446`). No Docker is needed by this command.
- Its explicit production mTLS endpoint, CA, and an authorized ordinary **setup**
  client certificate/key, separate from any workload/endpoint identity. The
  evaluation client pair below has broad local setup authority; this is not a
  least-privilege production credential issuance workflow. Never expose it to
  workloads or use it as a shared endpoint identity.
- The bundled source ID printed/configured by bootstrap (`directory_<install-id>`),
  and an unused stable synthetic source ID, e.g. `synthetic_eval01`.
- Same host/user as Control-Service: source registration contains protected
  absolute CA/token file references readable by that service. No files are
  copied into production automatically. Do not loosen credential permissions.
- Ordinary durable source registry/test/delete contracts. Missing contracts,
  wrong organization, identity/config collision, stale revision, unavailable
  provider and untrusted TLS all fail closed, without reseeding or fallback.

## Add, run, stop and explicitly remove

From `zpr-dashboard`, keep bootstrap running in its original terminal. Substitute
its actual state path, source ID and endpoint/port if different:

```sh
# Explicit credentials, not Simulator environment variables or discovery.
EVAL="$PWD/.zpr-evaluation-local"
STATE="$PWD/.zpr-companion-eval01"
STARTER=directory_REPLACE_WITH_INSTALL_ID

# Authenticated read-only preflight discovers the runtime organization; creates
# only NEW companion state. No source is saved and no bundled data is seeded.
sh packaging/simulator-companion/companion.sh \
  -mode ordinary-client -action init -state "$STATE" \
  -source synthetic_eval01 -starter-source "$STARTER" \
  -endpoint https://localhost:18790 -api-ca "$EVAL/ca.pem" \
  -client-cert "$EVAL/client-chain.pem" -client-key "$EVAL/client-key.pem"

# In a separate terminal, foreground provider + read-only test + registration.
# Use the same explicit paths as above. Restarting reconciles only exact config.
sh packaging/simulator-companion/companion.sh \
  -mode ordinary-client -action serve -state "$STATE" \
  -endpoint https://localhost:18790 -api-ca "$EVAL/ca.pem" \
  -client-cert "$EVAL/client-chain.pem" -client-key "$EVAL/client-key.pem"

# Separate explicit action; may run while the provider is up or after Ctrl-C.
# Does NOT stop bootstrap, unregister company/bundled sources or alter policies.
sh packaging/simulator-companion/companion.sh \
  -mode ordinary-client -action remove -state "$STATE" \
  -confirm-source synthetic_eval01 \
  -endpoint https://localhost:18790 -api-ca "$EVAL/ca.pem" \
  -client-cert "$EVAL/client-chain.pem" -client-key "$EVAL/client-key.pem"
```

`serve` binds only `127.0.0.1`, serves `directory.json` under the saved evaluation
organization, and requires the private bearer token. It then calls the public
read-only test endpoint, creates its own non-default registration if absent, and
verifies exact scoped registration readback. Connection-test success is not
policy acceptance. The self-signed source CA expires after 30 days; this bounded
companion has no automatic renewal/rekey workflow.

Ctrl-C stops only the synthetic provider, **not** its registration. Independent
starter reads, logging and Control Room remain available; an explicit read of
the stopped synthetic source fails without choosing another source. An assertion
run that requires that source may fail closed; no availability or policy-engine
semantics were changed. Remove policy references explicitly before permanent
removal. Do not expect policy references to be rewritten or silently rebound.

`remove` requires an exact confirmation, an editable registration with precisely
this companion's saved configuration, and the current revision. Generic DELETE
also rejects default, watched, shared-organization and operator-file-managed
sources, and active operations. It removes only that non-default source, clears
cached source/run evidence and verifies absence. There is no fallback, default
switch or cascade; default-source retirement/B6 is not implemented. If deletion
or readback fails, inspect/retry the public registry; do not edit production files
or delete credentials to simulate success. An already absent source reports
nothing removed. The foreground server, if still up, must then be stopped with
Ctrl-C. Private state is retained; delete only this companion directory after
confirmed unregister and shutdown. Reinitialization never overwrites state.

## Validation and limits

```sh
mkdir -p .companion-build
TMPDIR="$PWD/.companion-build" go test -race \
  ./cmd/zpr-simulator-companion ./cmd/zpr-web-dashboard \
  -run 'TestCompanion|TestOrdinaryCompanion|TestSourceRegistry' -count=1
python3 scripts/check-control-room-boundary.py
```

The disposable smoke builds/runs the real companion against the real public
registry handlers over verified client-certificate TLS and a real private HTTPS
synthetic directory, with an independent starter LDAP fixture in the same evaluation organization. It exercises
registration, exact retry, explicit reads, provider shutdown, starter continuity,
confirmed own-source removal, durable restart and cleanup. Additional tests cover
same-org discovery, missing contract, collisions, bad TLS/credentials, revisions,
default/watched/shared guards and concurrent readers. Tests also run production
Control Room without Simulator and the strict reverse-dependency audit (zero).
This is isolated test evidence, **not** a deployed Docker/bootstrap/browser smoke,
production qualification or multi-source policy/workload acceptance. No live
install is mutated by validation. Legacy ordinary-client preflight remains
blocked as documented in U01; this companion never invokes that launcher.
