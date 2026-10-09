# Public Enrollment Landing Page

This is a static, independently hosted onboarding front door. It does not use
ZPR, Control Room, Simulator, cookies, analytics, or the device enrollment API.
Host the files in this directory on an HTTPS site that new devices can reach
before installation.

## Invitation Link

Email links use a URL fragment so organization and invitation identifiers are
not sent in HTTP requests or normal web-server access logs:

```text
https://enroll.example.com/#organization=acme&invitation_id=opaque-invitation-id
```

The fragment contains identifiers only, never an activation code. The page
removes the fragment from browser history after displaying the identifiers. If
a link includes a `code`, `activation_code`, or `enrollment_code` fragment
parameter, it rejects the link and tells the recipient to request a new one.
Activation codes must be delivered separately and entered only in the trusted
native setup app.

Before hosting, set `macInstallerURL` in `site-config.js` to the organization's
approved HTTPS installer download. The page deliberately hides the download
action while this value is empty or not HTTPS. Do not point it at an unsigned
development package. Restrict write access to the hosting bucket/repository and
use a managed signing/notarization pipeline for the installer.

## Boundary

The page displays invitation identifiers but cannot verify invitation state or
claim it. The native setup app generates and retains the local identity and
performs the signed claim/status protocol. Device challenge/proof endpoints
remain on the dedicated HTTPS enrollment service, not the admin API. Keep admin
review and credential issuance on authenticated backend routes. The current
enrollment implementation does not issue adapter runtime credentials, so this
page must not claim that approval connects a device.

Test locally by opening `index.html#organization=acme&invitation_id=demo-123`.