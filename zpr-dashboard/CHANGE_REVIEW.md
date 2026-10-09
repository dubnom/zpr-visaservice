# Production change review

The source-only first version provides a shared **review queue**, not a generic
workflow engine and not a deployment service. Open `/change-review.html` in
Control Room after enabling it. Existing editors and API save behavior are
unchanged: save an immutable source revision, then submit its ID/revision here.
Sidebar/editor integration is deliberately isolated from the concurrent shared
frontend refactor.

## What works

- Named-user, organization-scoped submission of current saved Policy or Gateway
  revisions. The server reads source/history and validates the revision through
  existing independent production contracts. Browser-supplied source,
  validation, author or deployment instructions are not accepted.
- Immutable before/after source, SHA-256 proposal digest, reason, validation
  summary, impact/boundary and verified issuer/subject author.
- Paginated queue, source diff, complete source views and attributable audit
  events. The diff is a conservative prefix/suffix line comparison, not a
  semantic policy impact analysis.
- `submitted -> approved | rejected | cancelled`. Decisions require the exact
  proposal digest and expected request version. Only its author may withdraw
  a submitted request. Decisions and audit writes commit in one SQLite
  transaction; an audit failure rolls back the decision. Terminal decisions
  cannot be rewritten. Submit a new revision/request for further changes.
- Revalidation on approval: changed source/revision or observed domain context
  rejects approval. A different issuer/subject must approve by default.
- Explicit single-operator self-approval opt-in, never implied by a global grant.
- Unknown mutation outcomes require queue reconciliation; the browser never
  automatically retries submission or review decisions. Session loss clears
  source details. Failed reads retain last-good data with a stale warning.

The **before** revision is the preceding *saved draft*, not a verified live
runtime configuration. No active runtime revision is available from the existing
contracts. Policy validation is compiler validation; Gateway validation binds
the revision to the installed-instance contract. Neither is proof of live
application or an external-network policy.

## Enable explicitly

Provide an operator-owned protected JSON configuration:

```json
{
  "version": 1,
  "database": "/absolute/private/change-review/reviews.sqlite",
  "allow_self_approval": false
}
```

Set `ZPR_CHANGE_REVIEW_CONFIG_FILE` on **Control Room**, not Simulator. The database
parent must be owner-private (`0700`), and the SQLite database must be a private
regular file (`0600`). Keep its private directory persistent and back it up,
including SQLite journal state when applicable; do not commit it to Git.
Run only one configured review service instance unless shared database
deployment/backup/concurrency has been separately reviewed.

Named-user OIDC is mandatory. Add only the required explicit permissions to the
appropriate grants:

| Permission | Operation |
| --- | --- |
| `change.read` | Capabilities, queue and request details |
| `change.submit` | Submit a saved revision, withdraw own pending request |
| `change.review` | Approve/reject; does not grant domain editing or deployment |

Submission additionally requires `policy.edit` or `gateway.edit`. Details and
decisions additionally require the corresponding domain `.read` permission.
Listing the combined queue requires both `policy.read` and `gateway.read`.
Every operation requires an explicit organization within the verified grant;
mutations require the existing same-origin session/CSRF proof. Display names
and email are labels, never identities. No new privileges are seeded automatically.

Set `"allow_self_approval": true` only when deliberately configuring
single-operator operation. The decision still records the same named author/
reviewer and reason. Restart to change configuration/grants; no hot reload.

The stack launcher passes the configuration and narrow persistent database
directory mounts only when `ZPR_CHANGE_REVIEW_CONFIG_FILE` is explicitly set.
Normal manual container deployments must supply these same mounts/environment.
Enabling the source requires a separate deployment approval. This feature has
not been deployed, committed or pushed.

## API

All routes use `/api/change-review/` in Control Room:

| Method | Route | Contract |
| --- | --- | --- |
| GET | `capabilities?organization=...` | Self-approval setting, application disabled |
| GET | `requests?organization=...&offset=0` | Up to 50 records, `next_offset` or -1 |
| GET | `requests/{id}?organization=...` | Exact immutable proposal and audit |
| POST | `requests` | Organization, kind, target, saved revision, reason |
| POST | `requests/{id}/approve` | Organization, digest, expected_version, reason |
| POST | `requests/{id}/reject` | Same decision contract |
| POST | `requests/{id}/cancel` | Same contract, author only |
| POST | `requests/{id}/apply` | Always rejects: application contract unavailable |

Proposal source size is bounded to 256 KiB combined before/after; reasons to
1,024 bytes. API input is strict bounded JSON. This queue has a dedicated private
database in Control Room; production sources remain owned by their domain
services and are read through the existing mTLS Control-Service proxy.
Simulator configuration/APIs/sessions/workloads are never consulted.

## Intentionally not implemented

- Live application, verification, rollback execution, deployment scheduling,
  expiry/escalation, or multi-service orchestration.
- Production revision/CAS APIs needed to apply an approved policy or Gateway
  revision against an unchanged live baseline. The eventual domain service must
  enforce the exact approved digest, fresh authorization, live revision and
  idempotent operation ID; GUI approval alone cannot confer that authority.
- Gateway draft activation, TLS inspection, path/method enforcement inside
  opaque CONNECT tunnels, or changes to the live gateway candidate.
- Enrollment approval replacement, directory/organization/Simulator execution,
  or mandatory review gates on existing editor save/stage routes.
- A claimed `applied`, `verified`, `failed` or `outcome unknown` runtime state
  without an actual production execution contract. Those states belong to the
  next application phase; the GUI's network-uncertainty message is not a
  fabricated persisted runtime result.

## Validation

```sh
go test ./internal/changereview ./internal/operatorauth
go test ./cmd/zpr-web-dashboard -run 'TestChangeReview'
npm run test:browser -- tests/browser/change-review.spec.mjs
```

Tests cover immutable persistence/restart, organization isolation, explicit
self-approval, stale/digest/version rejection, decision races, audit rollback,
domain source capture and authorization, disabled application, desktop/tablet
diff/review, uncertainty reconciliation and last-good read handling.
