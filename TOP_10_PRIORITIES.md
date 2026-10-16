# Top 10 ZPR Priorities

Snapshot: 2026-10-15. Listed in suggested priority order, not as an approved
execution schedule. This summary does not replace the detailed acceptance
criteria in the linked work plans.

1. **Reliable network operation and recovery**
   Resolve node/Visa-Service disconnects and DNS adapter reattachment; prove
   recovery through restarts, credential expiry and deliberate failures, not
   just a short healthy run.

2. **End-to-end enrollment and endpoint setup**
   Finish Enrollment/BAS ownership, credential issuance and secure delivery to
   adapters. Prove a new endpoint can install, enroll, join and carry authorized
   traffic.

3. **Complete production installation package**
   Deliver a versioned company-installation bundle with all required services,
   validated configuration, persistent identities, upgrade/rollback and
   fresh-Ubuntu acceptance.

4. **Self-contained Simulator companion package**
   Package Simulator plus independent LDAP, OIDC and lab DNS, exercising the
   same production artifacts. Normal production operation must continue when
   Simulator stops.

5. **Organization creation through reviewed activation**
   Complete the private-draft workflow: linked editors, cross-document/compiler
   validation, activation preview, explicit approval and safe rollback.

6. **Green, reproducible tests and CI**
   Address the recorded 22 Organization browser failures and four Go failure
   groups. Establish clean-checkout CI and real-backend acceptance alongside
   mocked UI tests.

7. **Finish service and directory/code separation**
   Extract Policy-Service routing and remaining application/configuration/
   lifecycle ownership. Close the seven refactoring gates without introducing
   production dependencies on Simulator.

8. **Production policy and Gateway activation**
   Connect validated drafts to authorized compile/sign/install workflows with
   revision checks and rollback. Resolve enforcement limitations, including
   opaque HTTPS tunnels.

9. **Complete business and multi-node acceptance**
   Finish HQ/Shenzhen testing and broader cross-node allowed/denied traffic
   matrices. Extend the Regional matrix success into repeatable, sustained
   network acceptance.

10. **Operational visibility and maintainability**
    Complete node/trusted-service logs and metrics, actionable health reporting,
    audit, retention, backup/restore and certificate rotation so operators can
    diagnose and recover the system.

## Detailed Tracking

- [Implementation progress](../zpr-dev-context/docs/IMPLEMENTATION_PROGRESS.md)
- [GUI backlog](GUI%20work.md)
- [System release plan](System%20release%20plan.md)
- [Source reorganization](REDIRECTORY.md)
- [Organization editor plan](Organization%20editor%20plan.md)

Failure counts and runtime issues above are recorded snapshots, not a fresh
health check. Implementation, source publication, deployment and acceptance
remain separate states.
