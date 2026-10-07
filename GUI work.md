# GUI Improvements

## Overall

### Help
- [x] Put help button in the upper-right corner of the page (when it is in the nav it is unclear that it is page specific).
- [x] Show the help text in a pop-up dialog window
- [x] Improve the help text explanation of what things are and what they do.
- [x] Show examples when applicable (mainly the editors and organization or scenario creation).
- [x] Policy and assertion help text needs much more - ZPL guide, assertion guide, explanation of the security/service/client security model, where trusted sources fit in...
- [x] Move the uptime and refresh to be next to the help button.
- [x] Help text is still not following the checked off Improve directive above.

### Page headers
- [x] Get rid of page title since the nav already indicates it.
- [x] Pages shouldn't have their own "REFRESH" buttons when they are part of the global refresh updates.
- [x] Only one line between the topmost status/refresh panel and the content below.

### All text editors
- [x] Add search and replace functionality. Put the buttons on the right side before History. Shared local controls cover policy/assertion, ZPR Config, scenario JSON, and directory LDIF source editors.
- [ ] Add AI Assistant


## Control Room

### Map
- [x] Animate retained adapters when topology bounds move, avoiding sudden jumps.
- [x] Fit centers the rendered topology bounds with padding and preserves glyph proportions.
- [x] Remove the large empty area below the Map canvas.
- [x] Adapter connections to nodes sometimes cover connections between nodes. This may be a generic problem with the map placement algorhythm.
- [x] Auto-fit (fit after/during refresh) should be a checkbox.
- [x] Use solid lines for connecting components.
- [x] Put a little number beside adapters and services showing how many active visas they have.
- [x] Make pulsing even more apparant.
- [x] not all of the connectors are starting at the edges (non circulat shapes show this issue more than others).
- [x] add a dark mode check box.
- [x] Since Nodes don't show visa count - show the count of buffered denies in a similar fashion but use red (and the same outline rules)
- [x] When the number of visas or buffered denies change, do a little pulse.
- [x] Pulse the connector in addition to the adapter.
- [x] Does the map come up blank because it is missing info, or just waiting for a refresh to fill it?
- [x] Possibly related - when I switch to the map from another page, the map is only partially drawn until a refresh.
- [x] Show management and per-fastpath packet-processing counters in node details when a node is clicked, with explicit unavailable/stale telemetry states.
- [x] Add a right mouse click on adapters to highlight current visa allowed services and routes. Right click again on the adapter, or in blank space to remove the highlight.
- [x] Hide the dark mode checkbox for now.
- [x] The visa and buffered denies still don't pulse as far as I can tell. Are we using our expand/contract paradigm.
- [x] Make the right-click highlight stuff work for services as well.
- [x] Right-click on visa count should show the visas.
- [ ] Right-click on Buffer denials should show the details.
- [x] Move the small number indicator to overlap the top right corner of the objects. For the visa service put it overlapping the center of the upper right line. For circles upper right line and overlap it. For future shapes do the same kind of thing.

### Navigation
- [x] Keep Map as the main view and group Adapters, Actors, Services, Visas, Denials, and DNS under horizontal Status tabs with counts.
- [x] Keep summary status metrics on Map only.
- [x] Add Security, Trusted Sources, Logging, and Editor navigation entries with separators.
- [x] Add a ZPR Config menu item for managing versioned ZPLC configuration drafts. Validate and save drafts without applying them to the live runtime.
- [x] Add Trusted Sources links to provider control GUIs using server-configured provider URLs; hide links for providers without a configured manager.
- [x] Rename Logging to "Adapter Logs" and have it show the adapter logs. Remove the arrow. Log Manager remains a separate Tools link.
- [x] When the visa count is zero, show the same circle, but just as an outline with nothing in it.
- [x] When the user interacts through panning and zooming, turn off Auto-fit.

### Service logs and statistics
- [x] Add one Control Room diagnostics view for logs and current stats from every ZPR node and every configured trusted/required service used by Visa Service.
- [x] Standardize ingestion on OpenTelemetry through a ZPR observability trusted service. Put a provider-neutral catalog/query contract behind Control-Service; OpenObserve is the initial replaceable backend, not a UI/API dependency.
- [x] Show source identity/type, last update, stale/unavailable states, bounded searchable logs, and service stats. Keep credentials and provider queries server-side; do not depend on Simulator.
- [x] Test nodes and each configured service class, unavailable/stale sources, redaction/size limits, and Control Room behavior with Simulator unavailable.

### Security review
- [x] Remove the investigation-leads read-only heading and redundant baseline/dismissal explanations.
- [x] Label the browser-local inventory as Baseline and align its count and timestamp with Current.
- [x] Remove the redundant Security Review heading and place Reset Baseline beside the baseline summary.
- [x] Remove the Scan complete message.
- [x] Use explicitly labeled compact controls: Show dismissed controls visibility; Select visible is the bulk-selection checkbox.
- [x] Highlight the nav when a high alert is noticed. This may entail changes to the prioritization of detections (for example - Actor first observed, is not a high alert; multiple requests being blocked could be a high alert - especially if addresses aren't found, varied attempts, etc.)
- [x] Clear the security highlight in the nav once the page is visited (until another security alert)
- [x] Make the security highlight only color the little side indicator, not the main body area.

### Editor
- [x] If there is a horizontal scrollbar, color the forbidden area under the gutter white.
- [x] Keep every source-located Analyze warning in the gutter on its reported line; never render a duplicate warning list above or below the editor. Keep line-less analysis failures in the status area rather than inventing line 1.
- [x] Keep line-specific policy warnings in the gutter; show analysis failures without a source line in the status area instead of attaching them to line 1.
- [x] Rename "Rescan LDAP" to "Refresh Attributes", and make sure it handles all trusted attribute sources.
- [x] Put "Policy" or "Assertion" above the editor to be clear of the mode. Center between the left and right button groups.
- [x] Get rid of the extra line above the name of the policy/assertion being edited.
- [x] Add an indicator to the policy/assertion name line to indicate if the file has been modified. Don't show anything if it isn't.

### Status
- [x] I noticed on the "adapters" page that even when sorted, the records moved around (though nothing changed). I think sorting only sorts on its primary field but if that field has duplicates this occurs. A solution would be to always have a secondary field (or more) that is/are automatically attached. The user has nothing to do with this but the results would be deterministic.
- [x] Status tables need vertical scroll bars when they get big.

### Adapter Logs
- [x] move the buttons to be next to "Adapter Logs".
- [x] Add a new button toggle for showing and hiding all adapters.
- [x] The Adapter picker should be a dropdown next to the adapter title. That should remove its line.
- [x] A page-wide setting for wordwrap toggling should be added next to the page buttons.
- [x] Add a button to toggle seeing only running adapters.
- [x] The adapter picker dropdown should be a pop-up dialog controlled by a place to click.
- [x] I'm confused by the new Adapter titles.
- [x] Running Only checkbox doesn't work - it shows non-running adapters.
- [x] Remove "Adapter logs" and "Controller logs" from the adapter titles.
- [x] Show the adapter name where you used to show "Adapter logs"... in the same font.
- [x] Make a chevron button next to the adapter name which should display the dropdown/dialog.
- [x] Start with Word wrap enabled.
- [x] The adapter picker doesn't need a header or anything else. It just needs the pull down to start in an open position.
- [x] If there aren't any adapters (none available) than show a message that says this/
- [x] Move the picker to be under the adapter title when opened.

### ZPR Config
- [x] Adopt the same look-and-feel as the policy editor. If ZPR config is limited to one file, there is no need for Browse, or some of the File commands. Refresh Attributes is also not relevant.
- [x] Still things to do to get the look and feel better. "Validate syntax" should act like "Analyze". There should be a gutter for errors. A Format button, a file button.
- [x] Where are the colors? Where is the gutter? Why is the "Save Draft" not part of the file pulldown? Make "File..." and Analyze use the same color and sizes of the policy editor.
- [x] Remove "New draft" at the bottom of the editor, and use that space for the editor.
- [x] Remvoe line numbers from the gutter.

### Trusted Sources
- [x] Get rid of "Read Only"
- [x] Add a typical LDAP tree view.
- [x] Don't auto refresh ldap.


## Simulator

### Navigation
- [x] Set order to: Organizations, Scenarios, Trusted Sources, Activity, Workers (merged Agents/Workload logs).

### Agents
- [x] Remove the machine count label.
- [x] Change Simulated Fleet to Devices and make the heading prominent.
- [x] Change the type filter's All Machines option to All Devices.
- [x] Merge Agents and Workload Logs as Workers, retaining passive device/runtime inspection and removing active start/stop/login controls (approved).

### Scenarios
- [x] Remove Available scenarios and the catalog-count label.
- [x] Place New Scenario above the list and label it New Scenario....
- [x] Remove the EXECUTION PLAN label from the scenario editor.
- [x] Remove the Unfiled count/header; unfiled scenarios appear without a separate count.
- [x] Show the scenario name and run state together in the run heading.
- [x] Saturate the running state; show Cancel only while running and Clear only after a terminal run.
- [x] Label the published scenario action "Run".
- [x] Pulse the running state only when scenario progress changes; respect reduced-motion preferences.
- [x] Remove the redundant Organizations list heading and profile-count text.
- [x] Keep the Activate/Active control beside the organization name and place its description below.
- [x] Make the scenario running state more visible with more saturated colors. The
Running" orange should be a non-indicated color like blue.
- [x] When a scenario fails, the step/total steps doesn't show the failure line because of the cleanup.
- [x] We need a raw version of a scenario editor (that gives the same type of experience, colors, columns, etc. as the Control Room editors)
- [x] Add AI Assistants to the dialog and raw scenario editors.

### Activity
- [x] Remove the Decisions and Blocked flows headings.
- [x] Show visas and denials in sortable tables.
- [x] Place Refresh beside the Stream status in the banner.

### Organizations
- [x] Move Activate/Active control next to the organization name in the identity/policy/services pane.
- [x] Show a shortform list of scenarios in the organization identity area; links open scenarios without activating the organization.
- [ ] I want to be able to add new organizations with the help of the AI Assistant. This is a bigger project, so make a plan first and get my approval.

### Workers
- [x] Add the wordwrap button like the Adapter Logs.

### Machine/adapter provisioning

#### What happens today

The local Simulator is a fixed-fleet test fixture, not a general machine
onboarding flow. It defines 20 machine profiles and creates a machine
container on demand when a scenario starts one. Each container runs its own
machine controller and PH adapter, with a machine-specific identity and a
dynamic ZPR address.

At startup, the stack prepares credentials for the fleet. It generates
per-machine mTLS client certificates for the Simulator control channel, then
copies each machine's pre-generated ZPR bootstrap RSA private key into the
workload directory. The controller is configured to use its matching control
certificate and bootstrap key; the shared workload directory is also mounted
into the container. The bootstrap public keys and machine
identities are already represented in local policy/configuration; policy
names each machine by its adapter certificate CN and enumerates the machines
allowed to use the Simulator control service. Starting a container is
therefore not the same as enrolling an unknown device: its identity and
authorization were provisioned ahead of time.

These are two separate credentials: the per-machine mTLS certificate
authenticates the machine controller to the Simulator control service, while
the ZPR bootstrap key authenticates its adapter to the ZPR network. The
pre-generated keys, named fleet, and enumerated rules are appropriate for a
repeatable local test rig, but should not become the production onboarding
pattern. See the stack setup in
[`dashboard-stack.sh`](./zpr-visaservice/zpr-dashboard/scripts/dashboard-stack.sh),
the
[Simulator stack notes](./zpr-visaservice/zpr-dashboard/README.md), and the
[example machine policy](./zpr-visaservice/zpr-dashboard/cmd/zpr-web-dashboard/examples/policy-layers/platform.zpl).

#### What we should do instead

For real machines, use an explicit, auditable enrollment flow instead of
pre-provisioning a permanent bootstrap key and policy entry for every device:

1. Install a generic adapter image. On first enrollment, the device generates
   its own private key locally (preferably non-exportable in a TPM or other
   hardware-backed store); images and fleet manifests must not contain
   per-device private keys.
2. An administrator or device-management system authorizes enrollment with a
   one-time, short-lived credential bound to the intended asset. The adapter
   proves possession of its newly generated key over the authenticated
   enrollment channel. Reject expired, reused, or mismatched enrollment
   attempts.
3. An enrollment/authentication service verifies the device against the
   organization’s inventory and issues or activates a device identity bound
   to that key. It returns trusted, sourced device attributes (such as
   organization, owner, and posture) rather than treating a claimed adapter
   name as authorization.
4. The adapter authenticates with that identity, receives its ZPR address,
   and is admitted by normal policy evaluation. Write policy against
   trustworthy attributes and least-privilege roles, not a hand-maintained
   allow rule for every machine. Identity establishes *which device* joined;
   policy still decides *what it may do*.
5. Support credential renewal, immediate revocation, retirement, and
   replacement-key recovery. Keep an audit trail of who authorized enrollment
   and when; fail closed if enrollment or required attribute sources cannot
   validate the device.

Static bootstrap trust should remain limited to the small set of services
needed to bring up authentication and the network. It should not silently
expand into a long-lived exception for every endpoint. The operator UI should
show enrollment as a lifecycle (pending, active, expired/revoked), with
identity and trusted attributes visible for review; it should not mint or
display device private keys. Simulator can exercise this contract, but its
machine-control mTLS credentials must remain distinct from production adapter
identity and authorization.

#### Detailed GUI and installation workflow

**Status: proposed design and implementation plan, not an existing enrollment
feature.** The recommended first release requires explicit administrator
approval for each new device. A user can run the package, but that does not
give the user authority to approve a device or grant network permissions.
Supported operating systems, package formats, identity provider, credential
lifetimes, and hardware-key requirements need approval before implementation.

##### 1. Prepare the production enrollment service

- Operate an Enrollment Service and device identity registry independently of
  Simulator. Control Room calls Control-Service, which uses authenticated
  production service contracts for enrollment administration.
- Give the new machine a narrowly scoped HTTPS enrollment endpoint reachable
  on its existing network before it has a ZPR identity. This is necessary to
  break the bootstrap dependency; it is not an underlay path to ordinary ZPR
  services. Expose only enrollment operations through an approved gateway,
  not Control-Service, Policy-Service, or the Visa Service Admin API.
- Establish authenticated administrator access, organization-scoped
  permissions, package signing, server trust, audit storage, and service
  ownership. Public access to the current local UI is not part of this work;
  follow the
  [Remote Browser Access Contract](./zpr-visaservice/zpr-dashboard/BROWSER_ACCESS_CONTRACT.md)
  before any external administrative rollout.
- Integrate the device registry with a trusted authentication/attribute
  provider consumed by Visa Service. Issuing a certificate alone is not
  enough: adapter proof, trusted attributes, network admission, and
  revocation must work together. Keep link identity credentials distinct
  from enrollment credentials where the protocols require different keys.
- Preconfigure the minimum infrastructure bootstrap trust and attribute-based
  admission/access policy. Installing a device must not rewrite policy or add
  its key to the static bootstrap list.

##### 2. Administrator creates an enrollment invitation in Control Room

Add an **Enroll device...** action to the adapter/device inventory. It opens a
wizard with the following steps:

1. **Identify the asset:** select the organization and an existing inventory
   asset, or create a pending asset with an asset tag and optional display
   name. Select its intended owner and an approved device profile.
2. **Review requirements:** show supported platform/package, required local
   administrator privileges, hardware-key requirements, and the profile's
   intended access. Profiles map to approved attributes; this is not an
   arbitrary policy editor or an unrestricted role picker.
3. **Create invitation:** the server creates a bounded-lifetime, single-use
   enrollment invitation scoped to this asset and organization. Record who
   created it, its expiry, and the approval requirement.
4. **Deliver installation instructions:** show the signed generic installer
   download and an enrollment code that the user enters in its wizard.
   Distribute the code through an approved secure channel; do not put it in
   a download URL, command line, log, or browser persistent storage. Store
   only a verifier for the invitation secret on the server. If it is lost,
   cancel and replace the invitation rather than retrieving the secret.

The installer is reusable and contains no device private key or enrollment
secret. Trusted deployment configuration identifies the enrollment endpoint
and server trust requirements. Never derive server trust solely from a URL
or certificate supplied by an untrusted invitation.

##### 3. User or administrator runs the package on the new machine

1. The operating system verifies the signed package. The installer explains
   what it will install, what information it will send, and which network
   changes it will make. Elevate only for installation and required adapter
   setup; do not require the user to disable TLS or operating-system checks.
2. Run preflight checks for platform support, existing installation,
   permissions, key-store availability, time, and enrollment connectivity.
   Show actionable failures before changing routes or resolver settings.
3. Install the adapter and its local service. Open a setup wizard that asks
   for the enrollment code, confirms the trusted organization/service, and
   permits editing only non-authoritative information such as a display name.
4. Generate the device's protocol-required private keys locally. Use the
   approved hardware-backed store when required; otherwise use a protected
   OS key store if the selected profile permits it. Report the actual key
   protection level to the administrator. Do not silently downgrade it.
5. Submit the invitation, public key(s), signed server challenge, and bounded
   device evidence over authenticated HTTPS. Evidence can include platform,
   asset information, and verified attestation where supported. Self-reported
   hostname, serial number, or posture is a claim, not a trusted attribute.
6. The server atomically claims the invitation for this enrollment and key,
   verifies possession, and creates a **Pending approval** request. A retry
   for the same request and key resumes it; a different device/key cannot
   reuse the invitation. Browser link previews or merely opening the wizard
   must not consume it.
7. Show **Waiting for administrator approval**, the non-secret request ID,
   and a key-derived fingerprint/verification code in the local wizard.
   Persist enough protected local state to resume after a restart. Waiting
   does not grant an identity usable for ordinary ZPR traffic.

##### 4. Administrator verifies and approves the machine

Control Room shows a **Pending enrollments** count and a review table. Each
request opens details containing:

- Organization, intended asset/owner/profile, invitation creator and expiry,
  submission time, package version, and request ID.
- Device public-key fingerprint, reported platform, actual key protection,
  and separately labeled verified evidence and unverified claims.
- Inventory conflicts, an already-enrolled asset, missing attestation, or
  mismatched requirements. Blocking conflicts cannot be dismissed into a
  successful enrollment.

The administrator compares the verification code with the machine's setup
wizard through a trusted channel, checks the inventory/evidence, and chooses
**Approve** or **Reject**, with an audit reason. Possession of an invitation
alone does not prove that this is the intended physical asset. Approval
requires authoritative inventory or verified attestation, or an explicit
human verification step; the interface must make the remaining uncertainty
visible.

The backend rechecks the administrator's permission, invitation state,
organization, and request revision. Approval binds the asset to the verified
public key and approved profile. Record this decision durably before issuing
credentials; concurrent approvals/rejections must not create duplicate
identities. Rejection, cancellation, and timeout leave the device without
ordinary network access.

##### 5. Package finishes enrollment and makes the first connection

1. The package retrieves the result over a key-authenticated enrollment
   session. After invitation claim, possession of the invitation alone cannot
   retrieve credentials or alter the request.
2. On approval, the service issues a bounded-lifetime device credential and
   a versioned configuration containing the device identity, trust material,
   docking endpoints, and approved DNS/adapter settings. No device private
   key is returned. Configuration is authenticated and bound to this device
   and organization; it does not contain the live network policy.
3. The package verifies the issuer, identity/key binding, configuration
   version, and required fields, then installs the credential/configuration
   atomically. If delivery fails after approval, resume the same enrollment
   and reconcile issuance rather than creating another device or requiring
   reuse of the invitation.
4. Start the adapter, establish its authenticated node link, authenticate
   through the trusted device provider, and let Visa Service evaluate normal
   admission policy. A credential is not a promise of admission or access.
   An address is assigned only after admission succeeds.
5. Apply supported host routes/resolver changes with a saved rollback record.
   Do not strand the enrollment connection or replace unrelated network
   settings. The installer must preserve the user's ordinary network access
   according to the approved platform deployment design.
6. Verify the device's live identity and ZPR address, one explicitly permitted
   connectivity check, and one deliberately forbidden test flow. Report
   routing, authentication, or policy failures separately rather than showing
   **Connected** just because the local service started.
7. The package displays **Enrolled and connected** only after live checks
   succeed. Control Room obtains live adapter state from Visa Service and
   enrollment state from the registry; stale/unavailable sources remain
   explicitly labeled. A successful enrollment with a connection problem
   remains enrolled, with a separate actionable connection status.

##### 6. Manage the device after enrollment

- **Renew:** the adapter proves possession of its current key and obtains a
  replacement credential before expiry, subject to current registry state,
  device requirements, and revocation checks. Keep expiry visible and alert
  on renewal failure; never fall back to static bootstrap authentication.
- **Revoke:** an authorized administrator disables the identity in the
  registry. Propagate the change to the trusted provider and Visa Service,
  terminate affected active admissions/visas, and prevent renewal or
  reconnect. Show propagation pending/failure until enforcement is confirmed;
  changing a GUI label is not revocation.
- **Replace or recover:** a lost key, rebuilt OS, or replacement machine
  requires a fresh invitation and approval. Retire the old key explicitly;
  do not copy it or reactivate it based on the device's name.
- **Retire/uninstall:** retire the server-side identity and record the action.
  The package removes its local credential/service and reverses only the
  network settings it owns. Local uninstall alone is not proof of remote
  revocation.

Keep enrollment lifecycle separate from connectivity. Suggested enrollment
states are **Invited**, **Pending approval**, **Approved / delivery pending**,
**Enrolled**, **Rejected**, **Cancelled**, **Expired**, **Revoked**, and
**Retired**. Connectivity is separately **Connecting**, **Connected**,
**Disconnected**, or **Unknown/stale**. An expired invitation and an expired
device credential are different events and must be labeled accordingly.

##### Implementation sequence and acceptance criteria

- [ ] Approve the first supported OS/package format, administrative identity
  provider, device inventory source, approval rules, validity periods, and
  key-protection requirements. Specify the authenticated pre-ZPR enrollment
  endpoint and its deployment boundary.
- [ ] Define versioned Enrollment Service contracts for invitation creation,
  cancellation, key-bound submission/resume, review, approval/rejection,
  result delivery, renewal, revocation, and retirement. Include
  organization-scoped authorization, challenge/replay protection,
  idempotency, atomic state transitions, rate limits, bounded requests, and
  secret-free audit records.
- [ ] Implement the durable registry, issuer integration, trusted
  authentication/attribute provider, and Visa Service enforcement. Prove that
  a newly approved device can authenticate without a per-device bootstrap
  entry and that revocation stops existing and new traffic.
- [ ] Build and sign the generic package and setup wizard, including local
  key generation, installation, approval waiting/resume, configuration
  verification, network rollback, renewal, and uninstall.
- [ ] Add Control Room's enrollment wizard, pending review, device detail,
  revoke/retire actions, and independent lifecycle/connectivity indicators.
  Do not repurpose Simulator Workers as the production enrollment GUI.
- [ ] Run an end-to-end test from a clean machine: create invitation in the
  GUI, run the package, verify/approve in the GUI, observe the live adapter,
  test allowed/denied traffic, renew, revoke, and confirm traffic stops.
- [ ] Test expired/reused/stolen invitations, wrong organization/key,
  insufficient admin privileges, duplicate assets, concurrent decisions,
  rejected evidence, offline services, package restart, and failures between
  approval, issuance, delivery, and connection. Prove no duplicate identity,
  credential leakage, unauthorized access, or false success.
- [ ] Test route/resolver rollback and uninstall on every supported platform,
  and re-enrollment after key loss without restoring the old identity.
- [ ] Test Control Room, enrollment, and live admission with Simulator stopped
  and Simulator environment variables unset or deliberately invalid.
  Simulator may later test these public contracts, but it must not be a
  dependency of the production process.

The first milestone is complete when an administrator can invite, verify,
approve, observe, and revoke a real new device using the GUI, while its user
only downloads/runs the signed package and follows its setup wizard. No
manual key copying, fleet-manifest edits, or per-device policy compilation
should be required.