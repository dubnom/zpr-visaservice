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
   never include an enrollment code, private key or BAS HMAC secret in configuration.
   For the runtime-import pilot, independently provision runtime_import_public_key
   as the canonical base64 Ed25519 public key of the approved receipt issuer.
   Do not obtain this trust pin from the invitation or a device response.
   Without this pin the app can inspect claim/status but cannot verify an
   import authorization. The pin is public, not a runtime credential.
5. User/root-owned config/CA parents must not be shared-writable or symlinks.
   Use mode 0700 for dedicated user directories and 0600 for configuration.
   Configuration/CA must be inside the trusted real desktop home (including
   its parents as an operator trust assumption), not /tmp or a shared directory.

Open the app. It opens Terminal, starts the loopback browser wizard and opens
your default browser to a private one-hour session URL. Keep Terminal running.
Do not share that URL. Press Ctrl-C when finished. No background startup task,
firewall change, CA trust change or networking setup happens on app installation.
If default-browser opening fails, use the private URL printed in Terminal.

Do not enter the selected pilot code until the administrator has confirmed a
reachable device service, current Authentication/BAS policy authorization, and
an approved runtime installation/handoff path. This package does not supply
those services or the privileged helper. An invitation should not be consumed
merely to test the local wizard.

Prepare the two keys using your organization and invitation ID. Verify BOTH
the enrollment-proof and runtime-key fingerprints with the administrator
through an authenticated channel, then enter the separately supplied code.
Check fresh status after uncertain/lost responses. Approval is a review
decision, NOT credential issuance or connectivity. With an independent issuer
pin, fresh dual-key proofs can obtain a short-lived signed runtime-import
authorization. It remains in native memory, not the browser or a receipt file.
The app explicitly reports that runtime installation is not implemented.

Separate RSA-3072 enrollment-proof/runtime keys and enrollment metadata are stored as a generic
password item in the desktop user's default macOS Keychain:
  service: com.zpr.enrollment.development
  label: ZPR development enrollment identity
Only a private persistent reference is stored on disk:
  ~/Library/Application Support/ZPR/EnrollmentDevelopment/identity.keychain.json
Neither key is written as a plaintext identity file and neither is Secure
Enclave-backed or device attestation. Keychain encrypts storage and controls
access; the authorized running wizard still holds the software keys in memory.
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

Secure handoff and adapter installation remain unfinished. The selected design
requires a signed, authenticated one-time IPC helper, durable duplicate-import
protection and direct runtime-key import into the System Keychain. The enrollment-
proof key must never cross into root. Neither private key may be staged in a
plaintext file. The BAS HMAC signing secret must never be supplied to the Mac.
Privileged helper installation, tunnel/route/DNS operations and live invitation
claim require separate explicit approval; this development app performs none.
