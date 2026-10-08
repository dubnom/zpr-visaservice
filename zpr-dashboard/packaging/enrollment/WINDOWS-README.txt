ZPR Machine Setup - Windows 11 x64 development enrollment wizard

DEVELOPMENT ONLY. This installer DOES NOT install a network adapter, driver,
Windows service, issue credentials or establish ZPR connectivity.
It is currently unsigned. Native Windows installation and DPAPI/ACL behavior
must pass the documented Windows certification suite before distribution.

Install without elevation as the intended desktop user. The files and Start
menu shortcut belong to that user, not every account on the machine. Install
does not launch the wizard, contact an enrollment service, change firewall
rules, register startup tasks, or install a CA into Windows trust.

An administrator must supply the trusted endpoint and optional CA:
1. Copy setup.example.json to setup.json in this installation directory.
2. Replace the invalid example audience with the reachable HTTPS enrollment
   service origin. Do not use a localhost address on a remote machine.
3. If needed, set ca_file to a PEM CA file beside setup.json (relative filename)
   or a trusted absolute local path. No CA is imported automatically.
4. Omit state_directory. Never include an enrollment code or private key.
5. Protect config/CA parents from other users' writes. The wizard validates
   Windows owners/ACLs and rejects reparse points, UNC paths and shared writers.
   Administrator-provided configuration is still required; this is not
   automatic trust discovery.

Open "ZPR Machine Setup (Development)" from the Start menu. The console starts
the loopback-only browser wizard, opens the default browser and prints a private
one-hour URL. Do not share that URL; keep the console running and press Ctrl-C
when finished. Browser launch errors retain the URL for manual opening.

Enter the organization/invitation ID from your administrator. Prepare a key,
verify its fingerprint using an authenticated channel, and enter the enrollment
code delivered separately. Check fresh status when responses are uncertain.
Approval is an administrative decision, NOT credential issuance or connectivity.

State: %LOCALAPPDATA%\ZPR\EnrollmentDevelopment\identity.dpapi
RSA software key, encrypted at rest using current-user Windows DPAPI, with
private user/SYSTEM ACLs. It is not TPM-backed or device attestation. The current
user, privileged administrators or a compromised process under that user can
still impersonate the identity. Windows profile/domain recovery policies may
also permit recovery. Do not copy this file to enroll another user or machine.
Each Windows user has a separate identity and needs a separate asset/invitation.

Upgrade/uninstall preserves enrollment state, setup.json and operator CA files.
Uninstall does not revoke enrollment. Do not delete keys to retry, and do not
reset the Windows account password through an administrator without planning
DPAPI recovery. Loss of the profile/key requires administrator-led replacement
and revocation; there is no plaintext or replacement-key fallback.

No adapter/service-account key handoff is included. A future adapter runtime
needs separately implemented credential issuance, secure handoff and revocation.
