# AGENTS.md

## Coding Style
- Never patch a failing test you didn't write unless explicitly told to do so.
- For refactors/renames, search usages across all crates in this workspace.

## Project Note
- This is early release code, so we do not care about migrating existing
  database state.

## Analysis Ownership
- Analyze warnings, errors, and gutter markers must belong to the selected
  policy/assertion record and the exact editor source submitted for that run.
- Do not attach warnings from another record, active runtime policy, provider,
  or compiler configuration to the current file. Configuration diagnostics must
  remain separately identified and must never be assigned invented source lines.
- Clear diagnostics on record/source changes and reject stale analysis responses.
- Preserve valid source-local warnings; never hide them simply to produce a clean
  result. Test definitions-only records, record switching, and delayed responses.

## Control Room Boundary
- Control Room is a production-facing ZPR operator UI, not a Simulator client.
- Control Room and Control-Service must never call Simulator APIs, load
  simulation manifests/profiles, inspect Simulator sessions/workloads, or
  require Simulator to be running. Do not add simulator URL defaults or
  simulator-specific response contracts to their APIs.
- Obtain topology and identity from the Visa Service Admin API, policy and
  organization context from Policy-Service, trusted attributes from configured
  providers, and logs from an operator-configured production log source.
- Simulator may provision or exercise these services through their public
  contracts; dependencies must never point back from Control Room to Simulator.
- Test operator features with Simulator unavailable and simulator environment
  variables unset or deliberately invalid. Shared rendering utilities are
  acceptable only when Control Room has an independent endpoint and data contract.
