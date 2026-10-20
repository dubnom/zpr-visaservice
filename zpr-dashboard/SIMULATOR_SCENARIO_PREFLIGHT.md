# Simulator Scenario Plan Preflight

## Purpose and boundary

The Simulator scenario-plan API builds a read-only description of one explicitly
selected organization and scenario. It reports whether the plan is blocked or
plan-only, the plan's explicit machine references, and the evidence/status
inputs used by the planner. Preflight and approval do not execute scenario
steps. A separate run endpoint consumes a one-use approval to start only the
selected Simulator scenario run.

That approval is not a general run permission, host-provisioning authorization,
deployment authorization, or evidence that provisioning or deployment occurred.
The current UI records approval but does not submit the separate run request.

## Authorization and request rules

All three endpoints are authenticated Simulator API operations and require
named-user operator authorization scoped to the active Simulator organization:

| Endpoint | Permission |
| --- | --- |
| `POST /api/simulator/scenario-plans/preflight` | `scenario.read` |
| `POST /api/simulator/scenario-plans/approve` | `scenario.run` |
| `POST /api/simulator/scenarios/{scenario}/run` | `scenario.run` |

The preflight and approve bodies each include `organization_id`; it must match
the organization authorized by the API middleware. When no authorized
organization is attached to the request context, the handler's active
Simulator organization resolver is used and the requested ID must match it.
The run body has no organization field: middleware authorizes `scenario.run`
for the active Simulator organization, which the run handler reads from the
current manifest. The run's `{scenario}` path must select a published scenario
whose organization and ID match that organization and path. Its one-use
approval must then match the same organization and scenario path.

Authorization failures, organization-scope mismatches, malformed or missing
request fields, and published-scenario/path identity failures are rejected
before approval consumption. A wrong organization, scenario, or digest on an
existing approval also does not consume it. Preflight and approval do not
themselves start a run.

Plan requests must have a JSON body, no query parameters, no unknown JSON
fields, no trailing JSON value, and a body no larger than 8 KiB. IDs must be
valid scenario IDs. The plan endpoints return `400 Bad Request` for invalid request shape/fields,
`403 Forbidden` for an organization outside the authorized scope, `409 Conflict`
for a plan identity/digest conflict or blocked approval, `422 Unprocessable
Entity` for an invalid scenario input, and `503 Service Unavailable` when
required plan inputs or organization context are unavailable. Approval may also
return `500 Internal Server Error` if a digest or approval record cannot be
created.

## Preflight contract

Request:

```http
POST /api/simulator/scenario-plans/preflight
Content-Type: application/json
```

```json
{
  "organization_id": "great-lakes",
  "scenario_id": "great-lakes-runtime-verification"
}
```

On success, the response is `200 OK`. It reports a plan status; it does not
mean the scenario ran:

```json
{
  "status": "blocked",
  "organization_id": "great-lakes",
  "scenario_id": "great-lakes-runtime-verification",
  "manifest_name": "simulation-environment",
  "machine_ids": [],
  "workloads": [],
  "budget_inputs": [],
  "budget_status": "approved",
  "topology_status": "missing",
  "policy_status": "unresolved",
  "northstar_assets_status": "not-required",
  "package_contract_status": "missing",
  "blockers": ["policy resolution is required"],
  "plan_digest": "sha256:<64 lowercase hexadecimal characters>",
  "authorization_scope_digest": "sha256:<64 lowercase hexadecimal characters>"
}
```

The example illustrates the success response after current budget evidence has
been resolved; remaining policy or other blockers still prevent approval. If
authority evidence is absent, stale, revoked, mismatched, or unavailable,
preflight returns `503 Service Unavailable`. When the execution-scope digest is
valid, this error includes a scope-only bootstrap response:

```json
{
  "organization_id": "great-lakes",
  "scenario_id": "great-lakes-runtime-verification",
  "authorization_scope_digest": "sha256:<64 lowercase hexadecimal characters>",
  "blockers": [
    "approved budget evidence is absent",
    "policy resolution is required",
    "current approved budget evidence is unavailable or invalid"
  ]
}
```

This `503` body contains only the requested organization/scenario scope, its
stable authorization digest, and blockers. It contains no full plan,
`plan_digest`, approval, or run authorization. It is not a success-shaped
fallback. The authenticated `scenario.read` preflight request may use this
digest to initiate a separate, explicit human approval in the independent
Budget Authority for the exact organization, scenario, and digest. After that
grant exists, repeat preflight; only a successful plan with no blockers can be
locally approved. Revoked, expired, or unavailable authority evidence again
fails with `503`. If the plan inputs cannot be safely loaded or a valid scope
digest cannot be computed, the handler returns an error without a digest.

The fail-closed bootstrap sequence is: empty authority → authenticated
scope-only preflight response → explicit exact-scope authority grant → fresh
preflight → local plan approval → run-time evidence recheck. If that authority
grant is revoked before run, the run is denied before the one-use local
approval is consumed or run state is allocated.

The success response fields are:

| Field | Meaning |
| --- | --- |
| `status` | `blocked` or `plan-only`; neither value executes anything. |
| `organization_id`, `scenario_id` | The selected plan identity. Both must match the request. |
| `manifest_name` | Name of the simulation manifest used by the planner. |
| `machine_ids` | Sorted, unique machine IDs explicitly referenced by scenario steps or cleanup. |
| `topology` | Optional resolved scenario topology. |
| `workloads` | Fixed workload declarations, each with `machine_id`, `component_id`, and `agent`. |
| `budget_inputs` | Separate requested fanout/service count inputs, if the scenario uses those actions. |
| `budget_status` | `absent`, `unverified`, `approved`, or `unapproved`. |
| `topology_status` | Topology resolution state, such as `missing`, `invalid`, `unresolved`, or `resolved`. |
| `policy_status` | Policy resolution state, such as `unresolved`, `mismatched`, or `resolved`. |
| `northstar_assets_status` | `not-required`, `missing`, `unresolved`, or `resolved`. |
| `package_contract_status` | `missing`, `unsupported`, or `supported`. |
| `blockers` | Reasons the plan is not eligible for approval. |
| `plan_digest` | SHA-256 digest binding approval to the exact full planner result, including budget status and blockers. |
| `authorization_scope_digest` | Versioned, domain-separated SHA-256 digest of the selected organization, scenario, and manifest source inputs. It is the scope sent to the budget authority; it is not the local approval digest. |

Preflight resolves current approved budget evidence for the exact organization,
scenario, and `authorization_scope_digest`. Authority evidence must pass the
server-side currentness check, including a `checked_at` age of at most five
seconds. A successful resolution marks budget status approved and removes only
the planner's known budget-evidence blockers; it does not clear policy,
topology, package, resource, or other blockers. The authorization digest uses
the versioned domain `zpr:simulator:scenario-authorization-scope:v1` followed
by a NUL separator and serialized organization, scenario, and manifest inputs.
It excludes derived evidence, status, and blocker fields. The full
`plan_digest` remains separate
and binds the local one-use approval to the complete preflight snapshot.

Each `budget_inputs` entry has `phase` (`step` or `cleanup`), one-based `step`,
and `action`; it may also contain `fanout_count` and/or `service_count`.
Omitted counts are not evidence of zero cost or approval.

## Machine references and zero-reference scenarios

The planner derives `machine_ids` exclusively from non-empty `machine` fields
in the scenario's `steps` and `cleanup` arrays. It deduplicates and sorts those
references. It does not infer machines from the organization profile, manifest
inventory, topology, load-test profile, or workload count. More than 12
distinct explicit references adds a blocker; an absent or invalid referenced
machine is also blocked.

A scenario with no explicit machine references remains a zero-machine plan;
the planner never falls back to all organization machines. Its actions must
still satisfy the action contract. For example, machine-free traffic requires
a component namespace; actions that require a machine are blocked if the
machine field is absent.

The current UI independently requires a non-empty `machine_ids` list before it
enables approval. Thus the UI will not request approval for a zero-reference
plan. The approval endpoint itself has no separate non-empty-machine check; it
requires a `plan-only` status, approved budget status, and no blockers. This
distinction does not change the no-fallback rule or authorize execution.

## Budget inputs are not approved budgets

For LoadLab, the planner returns fanout and service counts as separate budget
inputs. `stress_traffic` contributes a `fanout_count` (the scenario's
`client_count`, or the organization profile's client count when not overridden)
and a separate profile `service_count`. `start_service_fleet` and
`stop_service_fleet` contribute service-count inputs. Step and cleanup entries
remain distinct. These requested counts are not resource approvals, capacity
limits, or authorization to launch the fanout.

Load-test fanout actions are explicitly blocked from execution by the runtime
plan. There are no approved numeric LoadLab budgets currently. The planner also
requires budget evidence scoped to the exact organization and scenario, with
an approval reference; missing, unverified, unapproved, or mismatched budget
evidence blocks plan approval.

The HTTP plan builder resolves current approved evidence for its stable
authorization scope before returning a successful plan. A successful evidence
check marks the budget approved, but policy resolution and other planner
requirements still apply independently. A count shown in `budget_inputs` does
not satisfy the authority check. No generic `evidence` object is accepted in
either Simulator request; evidence/status fields are planner-owned and are
returned as specific status fields and blockers.

## Digest-bound approval contract

Approval requires a fresh preflight result and a separate explicit operator
confirmation. The client submits:

```http
POST /api/simulator/scenario-plans/approve
Content-Type: application/json
```

```json
{
  "organization_id": "great-lakes",
  "scenario_id": "great-lakes-runtime-verification",
  "plan_digest": "sha256:<64 lowercase hexadecimal characters>",
  "approved": true
}
```

There is no request field for arbitrary evidence. The server rebuilds the plan and re-resolves current authority evidence at
approval time, using the stable `authorization_scope_digest`; it then
recomputes the full digest. The digest is `sha256:` followed by the lowercase
hexadecimal SHA-256 of the JSON-marshaled planner result; it binds the approval
request to the exact plan, including its organization/scenario identity,
machine references, budget inputs/statuses, blockers, and authorization-scope
digest. If the plan changed since preflight, the server returns `409 Conflict`
and requires a new preflight. An authority resolution failure returns
`503 Service Unavailable`. Approval also returns `409 Conflict` unless the rebuilt plan is
`plan-only`, has `budget_status: approved`, and has no blockers.

On accepted approval, the server returns `200 OK`:

```json
{
  "approved": true,
  "approval_id": "<opaque one-use identifier>",
  "organization_id": "great-lakes",
  "scenario_id": "great-lakes-runtime-verification",
  "plan_digest": "sha256:<64 lowercase hexadecimal characters>",
  "authorization_scope_digest": "sha256:<64 lowercase hexadecimal characters>",
  "machine_ids": []
}
```

The approval ID is an opaque, randomly generated, in-memory record scoped to
the organization, scenario, and digest. At most one unconsumed record can be
issued for that exact scope. The internal consume operation checks all three
scope values and removes a successfully consumed record; unknown, mismatched,
or already-consumed IDs are rejected. Approval is consumed by the separate
run endpoint below; the approval response itself does not execute anything.

## Starting an approved Simulator run

The run endpoint is:

```http
POST /api/simulator/scenarios/{scenario}/run
Content-Type: application/json
```

The body accepts exactly the one-use approval ID and its plan digest. It does
not accept an organization ID; the selected organization is read from the
Simulator manifest.

```json
{
  "approval_id": "<opaque one-use identifier>",
  "plan_digest": "sha256:<64 lowercase hexadecimal characters>"
}
```

The endpoint rejects query parameters, unknown fields, trailing JSON values,
and request bodies larger than 8 KiB. Both fields are required. A successful
start returns `202 Accepted` and a snapshot of the run state, for example:

```json
{
  "scenario_id": "great-lakes-runtime-verification",
  "scenario_name": "Great Lakes runtime verification",
  "scenario": {
    "id": "great-lakes-runtime-verification",
    "organization_id": "great-lakes",
    "name": "Great Lakes runtime verification",
    "description": "Approved Simulator scenario.",
    "topology": {
      "nodes": [
        {"id": "node-1", "zpr_address": "10.0.0.1", "substrate": "linux"}
      ],
      "links": []
    },
    "steps": [{"action": "delay", "timeout_seconds": 1}]
  },
  "organization_id": "great-lakes",
  "run_id": "run-0123456789abcdef0123456789abcdef",
  "run_namespace": "great-lakes/great-lakes-runtime-verification/run-0123456789abcdef0123456789abcdef",
  "plan_digest": "sha256:<64 lowercase hexadecimal characters>",
  "machine_ids": [],
  "workspace_revision": 7,
  "state": "running",
  "generation": 3,
  "current_step": 1,
  "active_steps": [1],
  "total_steps": 1,
  "started_at": "2026-10-19T23:00:00Z",
  "steps": []
}
```

This is the `simulatorScenarioRun` snapshot. `scenario` is the selected
published scenario definition; `steps` contains completed step results, while
`active_steps` can identify in-progress work. `finished_at` and `error` are
omitted until present; `active_steps` is omitted when empty. The snapshot can
already reflect a later state if execution progresses before it is serialized.
The run ID is generated only after approval consumption and plan validation;
it has the `run-` prefix followed by 32 lowercase hexadecimal characters. The
namespace is the canonical, un-hashed tuple
`organization_id/scenario_id/run_id`.

### Run authorization and lifecycle

The endpoint loads the selected organization and published scenario revision,
checks that the published scenario's organization and ID match the selected
organization and `{scenario}` path value, and validates the scenario. Under
the run-manager lock it then checks that organization activation is not
resetting and that no run is already `running` or `cleaning`. There is one
active run at a time in the Simulator process. A rejected concurrent start
does not consume its approval because this guard runs first.

These request, identity, and run-slot denials occur before approval
consumption. With an available run slot, the manager first performs a
non-consuming exact-scope approval lookup, verifies the approved plan against
current request inputs, then resolves authoritative budget evidence outside
the manager lock. The current source scope uses the stored
`authorization_scope_digest`, not the full local `plan_digest`. The manager
recomputes that stable digest from current organization, scenario, and manifest
inputs before resolving evidence; changed inputs are rejected before evidence
lookup. Resolver errors, absent or invalid evidence, and request
cancellation fail before the final busy-state check and approval consumption.
The manager re-locks and rechecks the run slot, consumes the exact
`approval_id`, organization, scenario, and full `plan_digest`, and only then
allocates a run ID, namespace, and run state.

The run check validates approved decision, exact authority scope, validity
interval, revocation state, and a maximum five-second `checked_at` age. The
manager also compares the
approved plan's machine IDs, topology, workloads, and budget inputs with the
current request inputs before consuming the local approval. A mismatch or
evidence failure leaves the local approval and run state unchanged. A rejected
concurrent start also preserves its approval. Run state and namespace are not
allocated until all checks pass.

The recorded machine IDs are the plan's explicit references; the run path does
not expand them. A run request with missing fields returns
`400 Bad Request`; an active run, unpublished scenario, or invalid/stale/reused
approval returns `409 Conflict`; an invalid scenario returns `422 Unprocessable
Entity`. Missing manifest or organization inputs can prevent the request with
`503 Service Unavailable`.

This endpoint authorizes only the selected Simulator run. It is not host
provisioning or deployment evidence and does not establish that either
occurred. This document describes the API contract and implementation
lifecycle; it does not claim an end-to-end run was performed.

## Current operational status

Preflight, approval, and run use the independent current-evidence resolver and
the same stable authorization-scope digest. Without a configured, current
approved authority record, preflight and approval fail closed with
`503 Service Unavailable`; with evidence, unrelated planner blockers remain.
The default plan inputs still do not establish policy resolution, and load-test
fanout/service scenarios remain blocked independently. The full `plan_digest`
continues to include status and blockers for local approval integrity. This
implemented API flow is not evidence of deployment or production readiness.

LoadLab fanout/service scenarios have no approved numeric budgets and remain
blocked independently. The current UI records approval but does not call the
separate run endpoint. Neither a preflight response nor a one-use run approval
authorizes host provisioning or deployment.

### Authority-backed operator sequence

The authority source is independent of Simulator. Control-Service does not
call Simulator APIs or read its manifests, profiles, sessions, or workloads.
Configure and operate the two services separately:

1. **Configure Control-Service.** Set persistent
   `ZPR_BUDGET_AUTHORITY_DB_PATH`, `ZPR_CONTROL_SERVICE_CERT_FILE`,
   `ZPR_CONTROL_SERVICE_KEY_FILE`, `ZPR_CONTROL_SERVICE_CLIENT_CA_FILE`, and
   `ZPR_OPERATOR_DELEGATION_TRUST_FILE`. A missing database path disables the
   authority; missing delegation trust disables approval/revocation mutations.
2. **Configure Control Room.** Set `ZPR_CONTROL_SERVICE_URL` to the HTTPS
   service origin; configure `ZPR_CONTROL_CLIENT_CERT_FILE`,
   `ZPR_CONTROL_CLIENT_KEY_FILE`, and `ZPR_CONTROL_SERVICE_CA_FILE` for mTLS;
   `ZPR_CONTROL_SERVICE_TLS_SERVER_NAME` is an optional server-name override.
   Configure `ZPR_OPERATOR_OIDC_CONFIG_FILE`,
   `ZPR_OPERATOR_OIDC_SECRET_FILE`, optional `ZPR_OPERATOR_OIDC_CA_FILE`,
   `ZPR_OPERATOR_DELEGATION_SIGNER_FILE`, and
   `ZPR_OPERATOR_DELEGATION_KEY_FILE`. The signer audience must exactly equal
   `ZPR_CONTROL_SERVICE_URL`; if `ZPR_OPERATOR_DELEGATION_AUDIENCE` is set, it
   must match too. Control-Service's `ZPR_OPERATOR_DELEGATION_TRUST_FILE` must
   recognize the signer and pin the Control Room client certificate.
3. **Configure Simulator's resolver before process start.** Set
   `ZPR_BUDGET_AUTHORITY_URL` to the Control-Service HTTPS origin,
   `ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE`,
   `ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE`, and
   `ZPR_BUDGET_AUTHORITY_CA_FILE`. The optional
   `ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME` sets the expected server name. The
   resolver posts to `/api/budget-authority/current` with the exact
   organization, scenario, and `authorization_scope_digest`; its client
   certificate must be accepted by the Control-Service client CA, and its CA
   file must validate the service certificate. If all resolver settings are
   absent, the default is unavailable; partial or invalid configuration fails
   explicitly. The request timeout is at most three seconds.
4. **Grant source approval before Simulator preflight.** A named operator must
   have `budget.approve` for the exact organization in both the OIDC
   authorization grant and the independent Control-Service delegation grant.
   Through the Control Room route
   `POST /api/budget-authority/approvals`, explicitly approve the exact
   organization/scenario/authorization digest, decision `approved`, and an
   expiry no more than one hour away. Same-origin session and CSRF proof are
   required. There is no seeded grant or automatic approval. The revoke route
   is `POST /api/budget-authority/approvals/{approval_id}/revoke` and requires
   the same organization-scoped permission. In this authority request,
   `scope.plan_digest` carries the Simulator's `authorization_scope_digest`,
   not the full local `plan_digest`:

   ```json
   {
     "scope": {
       "organization_id": "<organization>",
       "scenario_id": "<scenario>",
       "plan_digest": "<authorization_scope_digest>"
     },
     "decision": "approved",
     "expires_at": "<RFC3339 timestamp within one hour>"
   }
   ```
5. **Run Simulator preflight.** Send
   `POST /api/simulator/scenario-plans/preflight` with
   `organization_id` and `scenario_id`, authorized for `scenario.read`.
   Preflight resolves current evidence for `authorization_scope_digest` and
   returns both that digest and the separate full `plan_digest`. Evidence must
   be approved, exactly scope-matched, unrevoked, within its validity interval,
   and checked no more than five seconds before validation. Preflight and the
   later approval each resolve again; neither caches evidence.
6. **Approve and run the local plan.** Only when the preflight result is
   `plan-only`, budget-approved, and blocker-free, send
   `POST /api/simulator/scenario-plans/approve` with its full `plan_digest`,
   exact organization/scenario, and `approved: true`, authorized for
   `scenario.run`. Then send
   `POST /api/simulator/scenarios/{scenario}/run` with the returned one-use
   `approval_id` and same full `plan_digest`, also authorized for
   `scenario.run`. Run start rechecks current inputs and current authority
   evidence before consuming the local token or allocating run state.

**First-approval bootstrap limitation:** the current preflight handler resolves
authority evidence before returning its plan response. With no matching
authority record, it returns `503` and does not return
`authorization_scope_digest`; the public routes documented here do not provide
a budget-independent digest preview. Therefore step 4 requires that an
authorized approver already obtain the exact digest from a separately
approved/trusted plan-input source. Do not guess or hand-edit it. Until a
supported digest-discovery/bootstrap process is provided, the documented
HTTP-only sequence cannot initiate the first approval for a new scope.

Any unavailable source, resolver error, malformed response, scope mismatch,
unapproved/revoked decision, expired/not-yet-valid evidence, or evidence older
than five seconds fails closed. Preflight/approval do not clear policy,
topology, package, or resource blockers. The authority API stores decisions,
not numeric budget values; no amount or capacity is inferred by these steps.

### Operator acceptance specification: Ubuntu two-machine flow

**Status: procedure only; not run by this document change.** Existing
workbench reports in the [multinode demo README](../../zpr-demo/multinode-demo/README.md)
document allowed Alice-to-web0 HTTP, explicit Visa denial, and stop/start
recovery. They do not establish a fresh-host, reboot, rollback, or live
preflight-to-approved-run result. Do not convert those reported results into
new pass marks without a dated, redacted evidence record.

#### Prerequisites and configuration

- Use a version-matched ZPR runtime, Simulator runtime, and scenario-admission
  package; verify each package checksum against its trusted release source.
  The packaged runtime requires an isolated Ubuntu Linux/amd64 disposable
  host, Docker Engine and Compose v2, the locally imported pinned images, and
  root for the TUN-backed workload setup. The clean-state acceptance helper is
  destructive, requires root and an absolute downloads directory, and must be
  invoked only with its explicit `--disposable-host` acknowledgement.
- Install ZPR and Simulator to separate package roots. Give the ZPR runtime a
  separate persistent private state directory; keep ownership/modes managed
  by the installer. Let the initializer generate identities on that host.
  Never copy private identity files or credentials between hosts or include
  their contents in evidence.
- Before any live plan/run acceptance, configure Simulator's manifest,
  organization profiles, scenario directory, and published scenario revision.
  The selected organization must match the manifest and scenario. The flow
  requires operator authorization for `scenario.read` (preflight) and
  `scenario.run` (approval and run); every requested machine must be an
  explicit plan reference and present in the manifest. A two-node network
  readiness result alone does not provision Simulator machine controllers or
  establish their directory users, workload mappings, or service policies.
- Configure the authority independently: Control-Service needs persistent
  `ZPR_BUDGET_AUTHORITY_DB_PATH`, its TLS server certificate/key and trusted
  client CA, and `ZPR_OPERATOR_DELEGATION_TRUST_FILE`. Its current-evidence
  endpoint is `POST /api/budget-authority/current`, HTTPS with verified client
  mTLS. The Simulator process needs `ZPR_BUDGET_AUTHORITY_URL` (HTTPS origin),
  `ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE`,
  `ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE`, and
  `ZPR_BUDGET_AUTHORITY_CA_FILE`; optional
  `ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME` overrides hostname verification.
  The client certificate must chain to the Control-Service client CA, and the
  configured CA must validate the Control-Service server certificate. Set
  these variables before starting the Simulator process; unset configuration
  is default-deny.
- For source-side approval/revocation, configure Control Room's named-user
  OIDC session and delegation signer/key, and Control-Service delegation trust.
  Grant `budget.approve` for the exact organization to both the named-user
  authorization and the independent delegation grant. Mutations also require
  same-origin CSRF proof. Approvals bind exact organization, scenario, and
  authorization digest; expiry is limited to one hour. Current evidence must
  be approved, unrevoked, within its validity interval, and checked within
  five seconds of the consumer check. There is no seeded approval or automatic
  grant; this document does not request or create a real approval.

The current package helper's recognized clean-state order is shown below. It
is a source-backed operator procedure, not a command record from this task.
`installer` must name the verified `zpr-cli` and `downloads` the absolute
directory holding the verified archives. `test-clean-installed-workflow.sh`
also verifies fixed archive hashes and known container/image identities,
retains prior state under timestamped `before-clean` names, and is not a
general reset command.

```sh
bash "$installer" install "$downloads"
bash "$installer" start --disposable-host
bash "$installer" smoke
bash "$installer" operator-start
bash "$installer" simulator-install "$downloads/simulator-scenario-admission-20261019-r6.tar"
bash "$installer" simulator-start --disposable-host
bash "$installer" simulator-test
bash "$installer" simulator-stop
bash "$installer" operator-stop
bash "$installer" stop
bash "$installer" start --disposable-host
bash "$installer" smoke
bash "$installer" operator-start
bash "$installer" simulator-start --disposable-host
bash "$installer" simulator-test
bash "$installer" status
bash "$installer" operator-status
bash "$installer" simulator-status
```

#### Observable acceptance and abort gates

| Checkpoint | Pass evidence | Fail / abort |
|---|---|---|
| Package and identity | Trusted checksums match; only the expected pinned package images/containers are present. Clean start creates host-local identity state. A redacted before/after fingerprint comparison is equal across restart. | Checksum/image/label mismatch, unsafe state path, or identity initialization failure. Stop before starting or replacing state. |
| Ordered two-node start | `start --disposable-host` exits successfully; runtime status is healthy; both named nodes synchronize, both peer links become Active, and the startup gate observes Alice-to-web0 success for six consecutive samples. | Either peer is not Active, node synchronization remains pending, or readiness times out. Retain logs/state; do not clear policy markers or Redis to force progress. |
| Protected HTTP allow/deny | `smoke` succeeds for the allowed Alice-to-web0 request. The scenario acceptance command reports the expected allowed HTTP and explicit Visa policy denial. A denial counts only when the scenario identifies the policy decision; timeout, route failure, or connection refusal is not a policy-deny pass. | Allowed request fails or expected denial is not attributed to Visa policy. Stop and preserve evidence. |
| Preflight and scoped run | **Implementation wired; live host acceptance not run.** With an exact current authority grant and no unrelated planner blockers, preflight should report `plan-only`, approved budget, and no blockers before local approval. After approved run, record the exact organization/scenario, both digests, run ID/namespace, and `machine_ids`; verify the run touches only those explicit machines. | Authority unavailable/stale/revoked/mismatched, unresolved policy or any other blocker, digest/scope mismatch, changed inputs, or run-state allocation on rejection. Do not bypass or manually alter approval state. |
| Cleanup | Scenario declares cleanup actions for only its scoped machines/services. Run results show each cleanup action completed; selected controllers/sessions/workloads return to the documented baseline, and unrelated machines remain unchanged. | Missing/failed cleanup, out-of-scope action, or residual active workload/session. Keep the host isolated and use operator-approved recovery; do not claim cleanup success. |
| Stop/start recovery | Stop in the command order above, restart from the same private state, and repeat `smoke` plus `simulator-test`. Identity/configuration fingerprints and active policy state remain unchanged; allow/deny results recover. | State/fingerprint changes, policy loss, or traffic does not recover. Preserve prior state and logs; do not reinstall over it. |

The packaged `simulator-test` is separate from the dashboard plan/preflight/run
API. The latter still requires current budget evidence, resolved policy, no
other blockers, and an explicitly published scenario with a verified
two-machine runtime configuration. The package smoke/readiness checks do not
prove scenario cleanup.

For any authorized run, record UTC time, host role and disposable-host
acknowledgement, package release/checksum results, command names and exit
statuses, readiness/HTTP outcomes, redacted plan status/blockers, exact scoped
run identifiers, cleanup results, and only whether identity/policy fingerprints
match. Redact machine-owner/user identifiers where not required. Never retain
private keys, certificates, API/delegation tokens, raw identity/configuration
contents, database contents, or unredacted logs. On failure, stop according to
the operator procedure, preserve state and logs for review, and use console or
the separately approved rollback plan. Automatic rollback is not implemented.
Fresh-host installation, host reboot recovery, and backup restoration remain
**UNVERIFIED**; container stop/start is not reboot evidence.

#### Organization coverage and production follow-up

No organization is cleared for all-org live scenario acceptance. The checked-in
dashboard organization profiles currently lack approved budget evidence, and
the default planner leaves policy resolution unestablished. The five dashboard
profiles also lack the v2 `shared-approved-bootstrap`/`OciWeb` package contract;
the separate `demo` profile declares that contract shape, but matching
manifest membership and full package acceptance are unverified. Northstar's
configured LDIF inputs are missing; other profile data provenance and topology
compatibility remain unverified. LoadLab fanout/service inputs remain blocked
without approved budgets and capacity evidence. Do not infer readiness for an
organization from the two-node demo or its HTTP scenario.

Before production qualification, assign an authority owner and organization
approver roster; approve certificate trust, rotation, revocation, and database
backup/retention procedures; establish audit and incident operations; validate
policy and organization-specific provisioning; and separately authorize fresh-host,
reboot, and rollback acceptance. These are follow-ups, not evidence that the
current packages or services are production-ready.

### Browser UI validation (mocked)

On 2026-10-20, the focused [scenario-plan-preflight browser spec](./tests/browser/scenario-plan-preflight.spec.mjs)
passed **22 tests across desktop and tablet**. Validation used task-local
Node.js v24.22.0 from the official macOS ARM64 distribution, verified against
its published SHA-256 checksum, and Playwright CLI 1.63.0. From the
`zpr-dashboard` directory, with an ordinary available `node` executable and
the repository's Playwright dependencies installed, rerun with:

```sh
node ./node_modules/playwright/cli.js test tests/browser/scenario-plan-preflight.spec.mjs
```

This result validates only mocked/browser UI spec behavior. It does not
establish live source or approval evidence, production deployment acceptance,
or host acceptance.
