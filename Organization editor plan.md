# Organization editor plan

[Active backlog](GUI%20work.md) | [Design guidelines](GUI%20design%20guidelines.md)

Status: Planned; no organization creation has been wired. This is the detailed
requirements source for the backlog's organization-editor deliverable, not an
implementation or activation approval. Track commit and deployment separately.
Production Control Room must use independent production contracts and never
require Simulator templates, manifests or availability.

  Proposed plan for approval: keep AI proposal-only; gather requirements and selected template/profile context; validate a schema-bound organization draft without writing files; show a diff for identity, directory, policy, services and runtime; require a separate `organization.create` authorization and explicit confirmation; create through an audited, idempotent backend API with revision/conflict handling; test invalid proposals, stale context, cancellation, duplicate IDs and lost responses. No organization creation has been wired.
#### Organization creation and editing
- [ ] Prioritize Duplicate organization -> edit draft -> validate -> review activation as the first end-to-end workflow.
- [ ] Add New, Duplicate and Import actions to the organization list.
- [ ] Add a short creation wizard with blank/template/existing-organization starting points.
- [ ] Collect display name, stable organization ID, description and directory base DN; suggest IDs and allow review before creation.
- [ ] Collect initial sites/departments and topology without requiring people, services or scenarios to be complete.
- [ ] Show a creation review with the proposed artifacts and missing configuration; finish with Create draft, never automatic activation.
- [ ] Exclude credentials, enrollment identities, sessions and runtime state from duplication and reject them from imports.
- [ ] Use an audited, idempotent creation API with explicit organization.create authorization, confirmation, revision/conflict handling and uncertain-response reconciliation.
- [ ] Add a full-page organization workspace using the shared editor styling, organization name and History.
- [ ] Add Overview fields for name/description and a readiness summary distinguishing incomplete drafts, validation failures and runtime not provisioned.
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
