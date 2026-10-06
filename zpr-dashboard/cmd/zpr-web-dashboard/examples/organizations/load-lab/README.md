# Load Lab

Load Lab is a bounded local stress profile for the Simulator. It activates
200 named TCP services on the existing `echo-service` adapter on machine-05,
and 200 logical HTTP clients on the existing `finance-client` adapter on
machine-03. The logical clients share one ZPR adapter identity and address, but
open a fresh TCP connection for each request; this exercises flow grants and
service fan-out, not 200 separately authenticated adapter identities.

The `load-lab-fanout` scenario randomizes target service and request timing.
Each logical client periodically goes offline for a short randomized pause and
then resumes. The scenario is limited to two machines, 200 clients, 200
services, 120 seconds of traffic, and ports 10000-10199. Cleanup stops the
traffic, service listeners, workloads, sessions, and machines.

Organization activation does not install policy or seed the runtime directory
by itself. Provision the Load Lab LDIF and compile the complete runtime policy
with the normal three-layer policy workflow before running the scenario:

```sh
go run ./cmd/zpr-web-dashboard -mode compose-policy \
  -policy-root cmd/zpr-web-dashboard/examples \
  -policy-organization load-lab \
  -policy-output /tmp/load-lab-runtime.zpl
```

The organization composer appends one `provide` and allow rule per generated
service. Compile/sign and install that output using the existing runtime
workflow. The policy catalog record is illustrative and is not an installable
replacement for the full runtime policy.

The scenario uses new machine-controller stress modes. Rebuild the Linux
machine-controller binary and recreate machine-03 and machine-05 so their
read-only mounted controller binaries are current before running the scenario.
Do this only on the disposable local rig; the scenario itself is not run during
organization activation.

This scenario is intended for a disposable local rig. It generates real ZPR
traffic and Visa requests; it does not run automatically on organization
activation or against a production network.