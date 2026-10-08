ZPR Machine Setup - macOS Apple Silicon development enrollment app

DEVELOPMENT ONLY. This app is enrollment-only. It installs no network adapter,
driver, system extension or launch daemon; it does not issue credentials,
configure tunnels/routes/DNS or establish ZPR connectivity.
This local build is ad-hoc signed, NOT Developer ID signed or notarized.
Do not disable Gatekeeper or remove quarantine to distribute it to users.
Managed signing/notarization and actual clean-machine certification are required.

Copy ZPR Machine Setup.app into YOUR ~/Applications folder, not a shared
machine-wide location. Run as the logged-in desktop user without sudo.
Other Mac accounts need separate enrollment invitations/identities.
The initial target is native Apple Silicon; Intel support is not certified.

Administrator preparation:
1. Provision ~/Library/Application Support/ZPR/EnrollmentSetup/setup.json
   from the included setup.example.json.
2. Supply the actual reachable HTTPS device enrollment origin. A remote Mac
   cannot use localhost on the operator's machine.
3. Optionally set ca_file to a trusted PEM CA file beside setup.json (relative
   path) or a trusted absolute path inside the user's home. No CA is imported.
4. Keep key_protection exactly "macos-keychain", omit state_directory, and
   never include an enrollment code or key in configuration.
5. User/root-owned config/CA parents must not be shared-writable or symlinks.
   Use mode 0700 for dedicated user directories and 0600 for configuration.
   Configuration/CA must be inside the trusted real desktop home (including
   its parents as an operator trust assumption), not /tmp or a shared directory.

Open the app. It opens Terminal, starts the loopback browser wizard and opens
your default browser to a private one-hour session URL. Keep Terminal running.
Do not share that URL. Press Ctrl-C when finished. No background startup task,
firewall change, CA trust change or networking setup happens on app installation.
If default-browser opening fails, use the private URL printed in Terminal.

Prepare the key using your organization and invitation ID. Verify its fingerprint
with the administrator through an authenticated channel, then enter the code
provided separately. Check fresh status after uncertain/lost responses.
Approval is a review decision, NOT credential issuance or connectivity.

The RSA-3072 software identity and enrollment metadata are stored as a generic
password item in the desktop user's default macOS Keychain:
  service: com.zpr.enrollment.development
  label: ZPR development enrollment identity
Only a private persistent reference is stored on disk:
  ~/Library/Application Support/ZPR/EnrollmentDevelopment/identity.keychain.json
The key is not written as a plaintext identity file and is not Secure
Enclave-backed or device attestation. Keychain encrypts storage and controls
access; the authorized running wizard still holds the software key in memory.
The user/privileged processes or Keychain recovery can still expose this identity.
Do not grant arbitrary apps access to it.

macOS may prompt to unlock/authorize Keychain, particularly after an ad-hoc build
upgrade. Denying access or a missing/corrupt reference/item fails closed without
creating a replacement key or silently falling back to a plaintext file.
Changing the default Keychain may require deliberate recovery. No automatic
migration from legacy unencrypted development identities is provided.

Replace the app to upgrade; remove the app to uninstall. Both preserve operator
configuration, Keychain item and persistent identity reference. Deleting the app
does not revoke enrollment, and deleting only the reference leaves a Keychain
item behind. Never delete either to retry. Identity retirement/recovery requires
administrator action and revocation, not an uninstall script.

Credential issuance and secure handoff to a future privileged adapter runtime
remain unfinished. Do not pass this user's Keychain identity to a root service
by exporting a plaintext private key.
