# Organization editor plan

[Active backlog](GUI%20work.md) | [Design guidelines](GUI%20design%20guidelines.md)

Status: In progress; the existing-organization profile draft foundation is
implemented and locally verified with targeted checks. The full Go suite has
unrelated failures recorded below. Bundled-definition duplication into a private
draft bundle is also locally verified. New/Import now create reviewed private
incomplete bundles with readiness gaps and portable definition export.
Private multi-document source editing, coherent history and reviewed
cross-document AI are now wired and locally verified. Structured section editors,
full reference/compiler certification and activation remain unfinished. This is the detailed
requirements source for the backlog's organization-editor deliverable, not an
implementation or activation approval. Track commit and deployment separately.
Production Control Room must use independent production contracts and never
require Simulator templates, manifests or availability.

## Approved scope and execution plan

On 2026-10-11 the user selected **Simulator organization design first**:
New/Duplicate/Import, AI-assisted drafts, validation, history and separate
activation. Production organization administration remains an independent
future workflow, not a consumer of Simulator profiles.

### 1. Versioned profile draft foundation — deployed to Simulator

- Import an existing bundled profile into its private organization workspace
  as an unpublished `organization-profile` artifact.
- Provide an organization draft editor using shared source highlighting,
  structured name/description fields and advanced JSON for the same draft.
  Structured edits preserve the other accepted profile fields.
- Separate Save draft, Validate profile and History. Require the current
  revision for every save; reject conflicts rather than overwrite them.
  Load historical content for review and save it as a new immutable revision.
- Validate the existing profile schema, stable organization ID and existing
  structural rules before saving. Reject unknown fields, including credential,
  enrollment/session and arbitrary runtime-state fields. This first slice does
  not certify linked document consistency or readiness, nor accept incomplete
  new-organization profiles.
- Make AI a first-class editor pane: disclose the unsaved fictional profile
  and conversation being sent, show changed fields with before/after values,
  require reviewed insertion, and provide guarded Undo/Redo. Reject proposals
  after edits, organization switches or revision changes. AI does not write,
  save, publish or activate anything.
  Small top-level AI patches are merged into the exact submitted draft and
  validated as a complete profile, avoiding repetition of large organizations
  within the assistant's bounded output budget.
- Require `organization.read`, `organization.analyze` and `organization.edit`
  independently. Bind scoped AI requests to the route's organization.
  Persist a named operator subject as revision author when authenticated.
- Acceptance: fresh-process persistence, immutable history, conflict rejection,
  invalid/cross-organization AI proposals, structured/source preservation,
  cancelled review and unchanged bundled profiles/runtime on draft saves.
- Verification: 18 desktop/tablet profile and existing directory/scenario AI
  cases pass, including 390px editor bounds. Focused Go race tests for profile,
  workspace, AI and authorization contracts pass; embedded dashboard build,
  vet, JavaScript syntax and editor diagnostics pass.
  Required `make test` was run: it fails on the existing Redwood directory
  identity expectation and assistant setup shell tests (`SCRIPT_DIR` unbound).
  Those tests/scripts are not changed by this slice. No full-suite green claim.

### 2. New, Duplicate and Import — deployed to Simulator

#### Implemented duplication slice (2026-10-11; subsequently deployed)

- **Duplicate organization** previews the selected bundled organization's
  profile and supporting LDAP, policy/compiler/catalog and explicitly
  organization-scoped scenario definitions. It does not copy workspace edits,
  policy revisions, run history or observed runtime inventory.
- Require a distinct stable ID, name and description. Show complete sanitized
  definitions and require an explicit **Create draft** confirmation. Remove LDAP
  password/auth-password, user certificate and PKCS12 attributes, including
  folded/base64 values. Reject unsafe paths, symlinks, oversized definitions
  and detected credential-bearing configuration rather than copy them.
- Preserve the source base DN, desired node addresses, directory/device/service
  identifiers and policy namespace for review. Update only typed profile file
  references and scenario organization IDs; do not blindly rewrite LDIF/policy.
  The bundle is **not runnable**. Changing base DN without coherent directory
  changes is rejected; full namespace remapping is still pending.
- Store profile plus supporting documents as one unpublished immutable
  `organization-bundle` revision in private `organization-drafts.db`.
  Profile edits append a whole-bundle revision, preserving document content.
  This is a coherent snapshot foundation, not yet a multi-document editor.
- Require source `organization.read`, source `organization.analyze` for preview,
  and target `organization.create` for creation. Draft reads/AI/edits use the
  target's independent read/analyze/edit grants. Catalog entries are filtered
  by authorized target organization scope.
- Freeze and retain the creation operation and reviewed sanitized source hash
  in the browser tab. Persist operation ID/request hash in the original bundle
  revision. Matching retries return the current draft even after later edits or
  disappearance of source files; changed retries and conflicting IDs fail.
  Operation IDs are scoped to the target organization. Uncertain responses
  support explicit result lookup without creating another draft.
- Show private drafts separately from runnable organizations. Open them in the
  profile editor with AI, Save, Validate and History; never add them to existing
  activation, scenario-run or directory-publish catalogs.
- Profile validation also checks draft file references and LDAP base-DN
  containment. It explicitly does not certify policy compilation, scenario
  reference consistency, runtime readiness or activation safety.
- Verification: 26 desktop/tablet duplication/profile and existing
  directory/scenario AI cases pass, including narrow bounds, source changes,
  lost-response/reload reconciliation, local AI insertion, and session-loss
  cancellation. Focused Go race tests cover concurrent creation, scope grants,
  sanitized source fingerprints, credential removal, symlink/path rejection,
  immutable copied documents, retry reconciliation without source availability,
  and AI for draft-only organizations without a manifest/runtime.
  Embedded build and vet pass. Required `make test` retains the same Redwood
  LDAP and assistant setup `SCRIPT_DIR` failures; no older tests were changed.
  No deployment, activation, commit or push.

#### Implemented New/Import slice (2026-10-11; subsequently deployed)

- **New organization** asks for stable ID, name, description and optional base
  DN. No runtime sites, identities or policy documents are fabricated.
  Empty sections become explicit readiness gaps rather than prevent draft
  saves or AI-assisted profile editing. Initial sites remain part of the
  subsequent topology/section editor, not a required creation prerequisite.
- **Import organization** accepts a local JSON file or pasted version 1
  portable bundle containing `profile` and `documents`. Review shows all
  definitions and gaps before explicit confirmation. Profile and scenario
  organization IDs must already match the target; no namespace rewrite occurs.
- Reject unknown fields, internal receipts, unsupported runtime-state document
  kinds, unsafe/duplicate paths, oversized content and detected credentials.
  Imported LDAP credential/certificate attributes are rejected, not silently
  stripped. Directory containment and supplied local document references are
  mandatory; missing sections remain disclosed gaps.
- Catalog records support known policy/configuration/assertion kinds and matching
  content types. Catalog metadata is restricted to `demo`/`live` flags.
  Import checks do not certify policy compilation or scenario semantics.
  Arbitrary free-form text cannot be guaranteed secret-free; fictional
  definitions only, with operator review before confirmation or AI disclosure.
- Target `organization.create` authorizes preview and creation, with no source
  organization dependency. Independent read/analyze/edit grants authorize later
  draft access, validation/AI and saving. Creation responses remain receipts
  only; exact immutable operation/request hashes reconcile uncertain responses,
  even after subsequent edits or bundled source unavailability.
- **Export saved definitions** downloads the same portable format, without
  operation hashes, provenance receipts or runtime state. Unsaved edits must be
  saved first. Unsupported metadata/content produces an explicit export error.
- Private profile validation now separates incomplete sections from malformed
  supplied configuration. Existing bundled/runnable profile validation remains
  strict. Successful draft checks always return `runnable: false` and a
  structured `readiness_gaps` array, even when no schema gaps remain.
- Fixed workspace SQLite initialization to install its busy timeout before
  WAL/schema operations. Concurrent creation is stress-tested, without adding
  silent retry/default behavior. Scoped dialog bounds/hidden states and readable
  narrow inputs keep creation and AI controls usable.
- Verification: 36 desktop/tablet organization/profile/directory/scenario AI
  cases pass, including a fresh 390px creation/AI/save flow. Focused Go race
  tests, 25 repeated concurrent creation cases, build/vet and syntax/editor
  checks pass. Full `make test` retains only the recorded assistant setup
  `SCRIPT_DIR` and Redwood LDAP expectation failures. No older tests were fixed.
  No deployment, activation, commit or push.

#### Remaining creation UX

- Approved template picker beyond selecting a bundled organization for Duplicate.
- Initial-site structured fields, full namespace remapping and section-specific
  AI context are part of the remaining coherent-workspace work.

### 3. Coherent organization workspace and section-aware AI

#### Implemented coherent-definition slice (2026-10-11; subsequently deployed)

- Open a private profile draft and choose **Edit supporting definitions**.
  Explicitly discard unsaved profile-only edits or save them first; no hidden
  save occurs when entering the full-definition workspace.
- Select the profile or a supporting directory, policy, compiler configuration,
  policy catalog or scenario document. Edit its source with the shared gutter/
  highlighting surface; add/remove documents and change typed paths/kinds.
  Keep incomplete/invalid local edits visible until explicit validation/save.
- **Save all definitions** appends one complete profile/document snapshot with
  required expected revision, immutable original creation receipt/provenance
  and named operator author. Conflicts preserve local edits. Validation uses
  the strict portable-definition boundary, supplied references and LDAP
  containment; checks do not save and always report `runnable: false`.
- Full-definition History loads profile and documents from the same immutable
  revision. Historical selection from the private profile editor opens this
  complete snapshot, not an old profile combined with current documents.
  Saving a reviewed historical snapshot appends to the current revision.
- AI receives the entire exact unsaved fictional profile/supporting bundle,
  explicitly disclosed before Send, without private creation receipts.
  The existing 2 MiB request/context limit is enforced; oversized contexts fail
  explicitly rather than silently omit files. Section-only disclosure/context
  is still pending.
- Model proposals use small top-level profile patches and explicit complete
  document upserts/deletions. Preserve untouched documents/fields, reject
  unknown, missing-delete or contradictory changes, then validate the combined
  result before returning a proposal. Related base-DN/reference changes must
  remain coherent. AI never writes or certifies compiler/runtime readiness.
- Before/after review identifies every changed profile field/document and
  requires **Insert reviewed definitions**. Exact-context guards reject stale
  responses/insertion. Undo/Redo restores all affected local definitions
  together and is invalid after another revision/context change.
- Read/history require target read, save requires target edit, and checks/AI
  require target analyze. Routes are Simulator-only. No production service,
  authorization-grant or runtime changes are introduced.
- Uncertain saves retain local state and instruct explicit Reload to reconcile;
  delayed receipts cannot reopen a logged-out workspace. Logout clears profile,
  documents, history and AI state.
- Verification: 52 desktop/tablet draft/profile/definition and existing
  directory/scenario AI cases pass, including fresh 390px source/review,
  add/remove, conflicts, whole-bundle history, lost saves and delayed logout.
  Focused Go race tests cover atomic persistence, immutable receipts, coherent
  history, unsafe/partial edits, actual denied scoped requests and fake-local
  AI against unsaved documents without source/Simulator runtime availability.
  Build/vet/syntax/editor checks pass; full `make test` retains only the recorded
  assistant setup `SCRIPT_DIR` and Redwood LDAP expectation failures.
  No deployment, activation, commit or push.

#### Remaining section editors and certification

- Add Sites/topology, Directory people/groups, Services/workloads and linked
  Policy, Assertions, ZPR Config and Scenarios using their existing editors.
- Extend the coherent source workspace with those structured editors;
  preserve atomic snapshots and whole-bundle history.
- Check unique IDs, group membership, machine ownership, service/workload
  references, base-DN containment, compiler inputs and scenario targets.
  Distinguish schema errors, incomplete definitions and runtime prerequisites.
- AI gathers requirements iteratively and uses explicitly disclosed selected
  sections rather than requiring whole-bundle context. Preserve reviewed
  cross-document diffs and explicit insertion/Save separation.
- Acceptance: structured/source round trips, unsupported-field rejection,
  concurrent multi-document edits, cancelled AI proposals and coherent rollback.

### 4. Activation review and end-to-end certification

- Add a separate review showing proposed bundle revision, current active
  revision, node/topology/service/directory changes and affected running users.
- Block activation on invalid references, missing package/runtime prerequisites
  or unresolved operations. Require `organization.activate` and confirmation.
- Feed one validated, immutable bundle into the existing Simulator activation
  pipeline. Do not make draft reads/saves provision machines or change services.
- Show operation progress, errors and reconciliation. Keep restoring definitions
  distinct from resetting live runtime state.
- Acceptance: draft-save isolation, denied activation, reviewed activation,
  runtime failure/lost response, recovery and an AI-assisted duplicate-to-run
  end-to-end test on an explicitly approved disposable environment.

### Delivery controls

The user explicitly approved deployment after local verification. The
draft/profile/New/Import/Duplicate/source-document/AI slices are now deployed
to Simulator, verified 2026-10-11 20:18 UTC. Historical "No deployment" notes
under the implementation slices describe their local-verification stage, not
current release status. See [deployment evidence](GUI%20deployments.md) for
exact reconstruction, 52 release browser tests, persistent private-draft
desktop/tablet checks, the scoped LDAP layout correction, approved read-only
supporting definitions, rollback and remaining gaps. No activation/commit/push.

Run focused Go/race and desktop/tablet browser checks after each slice, plus
the required Go suite for backend changes. Keep existing unrelated failures
visible; do not rewrite older tests without approval. Update this plan and the
GUI tracker with verified scope and remaining gaps. No deployment, activation,
commit or push is implied by implementation approval.

#### Organization creation and editing
- [ ] Complete Duplicate organization -> edit draft -> validate -> review activation as the first end-to-end workflow (draft-only steps verified; activation pending).
- [ ] Add New, Duplicate and Import actions to the organization list.
- [ ] Add a short creation wizard with blank/template/existing-organization starting points.
- [ ] Collect display name, stable organization ID, description and directory base DN; suggest IDs and allow review before creation.
- [ ] Collect initial sites/departments and topology without requiring people, services or scenarios to be complete.
- [ ] Show a creation review with the proposed artifacts and missing configuration; finish with Create draft, never automatic activation.
- [ ] Exclude credentials, enrollment identities, sessions and runtime state from duplication and reject them from imports.
- [ ] Use an audited, idempotent creation API with explicit organization.create authorization, confirmation, revision/conflict handling and uncertain-response reconciliation.
- [ ] Add a full-page organization workspace using the shared editor styling, organization name and History.
- [ ] Add full readiness summary distinguishing incomplete drafts, validation failures and runtime not provisioned (name/description profile fields implemented).
- [ ] Add structured Sites & topology tables and a topology preview.
- [ ] Add Directory tree/tables for people and groups alongside the existing LDIF source editor.
- [ ] Add Services & workloads tables with references to available machines and workloads.
- [ ] Open organization-scoped Policy, Assertions and ZPR Config through the existing editors.
- [ ] Add an organization-scoped Scenarios list opening the existing Scenario editor.
- [ ] Offer Advanced source for the organization profile as an alternative view of the same draft, not an independently editable copy; preserve unsupported fields through structured edits.
- [ ] Keep Save draft, Validate and Activate/Apply separate; saving must not change the runtime.
- [ ] Validate individual documents and cross-document references, including group members, service references, duplicate IDs and directory base-DN containment.
- [ ] Save coherent organization revisions across related documents; provide History and rollback without combining incompatible document versions.
- [ ] Require an explicit activation review showing runtime effects, prerequisites and changes before applying anything.
- [ ] Make AI assistance section-aware for overview, directory, policy/configuration and scenarios; disclose exactly which context will be sent.
- [ ] Show AI proposals as reviewable diffs across affected documents; never silently replace, save or activate an organization.
- [ ] Test invalid proposals/imports, stale context, structured/source round trips, cancellation, duplicate IDs, concurrent revisions, lost responses, coherent rollback and runtime isolation on draft saves.
- [ ] Keep production Control Room organization administration separate, using independently authorized production service contracts with no Simulator template, manifest or availability dependencies.
