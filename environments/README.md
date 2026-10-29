# Local environments

There are two persistent local environments:

- **stable** is the latest full operator/Simulator stack, running in the
  `zpr-original-20261016` Colima profile.
- **dev** is the protected-DNS evaluation acceptance stack in Docker Desktop.

The JSON files in this directory are non-secret inventories of fixed host-port
assignments and state locations. They do not launch, stop, or configure
services. Keep each environment's host ports unchanged; if a port must change,
update its inventory and the owning launcher/configuration together.

Open [the environment dashboard](index.html) for service links and documented
login identities.

## Stable

The stable stack uses the `colima-zpr-original-20261016` Docker context and the
shared runtime state at `../../.local-runtime` relative to the repository root.
Its source checkout is `../../zpr-visaservice`. The `stable.json` inventory
records loopback port mappings, including the Control Room, Simulator,
Policy-Service, Control-Service, operator login, and support relays.

Authenticated readiness checks on 2026-10-27 returned HTTP 200 from Policy-
Service (`28789/api/policy/context`) and Control-Service (`28790/api/snapshot`).
The unauthenticated Control Room and Simulator endpoints responded with HTTP
403, so these checks confirm the listeners are reachable but do not replace an
operator login or full acceptance run.

## Dev

The dev acceptance stack uses the default `desktop-linux` Docker context, the
install registered as `e18d7c386deecf8c06fc5ee4`, and state at
`zpr-dashboard/.zpr-evaluation-acceptance` relative to the repository root.
Its fixed host-port assignments are in `dev.json`. The Control Room and OIDC
ports also bind the selected private host address; directory, logging,
Control-Service, and Visa-Service admin ports remain loopback-only.

Use the [evaluation runbook](../zpr-dashboard/packaging/evaluation/README.md)
for the evaluation launcher's prerequisites and behavior. The inventory is
not a replacement launcher and must not be used to infer production support.

## Temporary environments

Create a temporary environment only for a specific test that cannot safely use
`dev`. Give it a purpose-specific `tmp-<purpose>` name, its own state directory,
and host ports that do not overlap either persistent inventory. Record which
launcher owns its processes and containers before starting it.

After the test, stop the environment with its owning launcher, verify that its
owned processes and containers are stopped, and remove only that temporary
environment's state directory. Do not remove shared runtime state, either
persistent environment, or unrelated containers as part of temporary cleanup.
Do not add another persistent environment without replacing one of these two.

## Retired runtime state

The extra `zpr-fallback-20261016` Colima profile is stopped. Its VM disk and
container state are retained; the profile was not deleted. The older duplicate
Docker Desktop service containers that published ports `8789`, `8790`, `8800`,
and `5556` are also stopped, not removed. Their container state and shared
runtime files remain available for deliberate recovery. They are not part of
the `stable` or `dev` inventories.

The stable Colima stack does not currently run a logger. Use the dev
environment's OpenObserve when logging is needed; do not rely on the retired
Docker Desktop listener on port `8800`.

## Password retrieval

The dashboard links here rather than to secret files. Use the protected local
workflow for the relevant environment:

- Stable operator: follow [Local development operator login](../zpr-dashboard/README.md#local-development-operator-login).
  The password is stored in the protected
  `../../.local-runtime/operator-login/operator-password` file.
- Dev operator and logger: follow the
  [evaluation identity and password instructions](../zpr-dashboard/packaging/evaluation/README.md#identity-health-and-lifecycle).
  The operator and logger passwords are separate protected files in
  `../zpr-dashboard/.zpr-evaluation-acceptance/`; do not share or print them.

Never copy passwords into this repository, the dashboard, browser URLs, shell
history, or logs. The dev evaluation has no named LDAP users; do not treat its
directory reader account as an interactive user.
