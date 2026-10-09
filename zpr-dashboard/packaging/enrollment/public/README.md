# Public Enrollment Landing Page

This is a static, independently hosted onboarding front door. It does not use
ZPR, Control Room, Simulator, cookies, analytics, or the device enrollment API.
Host the files in this directory on an HTTPS site that new devices can reach
before installation.

For the explicitly selected local simulator phase, serve only on the Mac's
private LAN address. `ZPR_ENROLLMENT_PUBLIC_ROOT` can point at a temporary
runtime copy containing a development download without changing the source
bundle:

```sh
ZPR_ENROLLMENT_PUBLIC_ROOT=/path/to/local/public-copy sh serve-lan.sh 10.0.0.25 8090
```

This development helper uses plain HTTP, binds only to the supplied private
IPv4 address, and serves static files only. It is not suitable for real users
or untrusted networks. HTTP installer links are shown only for RFC1918 or
`.local` hosts and are explicitly labeled unsigned development builds. Use a
trusted HTTPS host and signed/notarized installer for normal deployment.

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

Before hosting, set `macInstallerURL` in `site-config.js` to the approved HTTPS
installer download and `macSetupConfigURL` to a HTTPS setup-config/CA bundle.
The page hides either download while its URL is empty or invalid. HTTP URLs are
accepted only for RFC1918 or `.local` hosts and are visibly labeled development
only. Do not point a production page at an unsigned development package.
Restrict write access to the hosting bucket/repository and use a managed
signing/notarization pipeline for the installer.

The Control Room invitation worksheet accepts the public setup page URL and
offers a copyable email draft after invitation creation. It includes the page
link with organization/invitation IDs and excludes the activation code. The
operator must send the email manually and deliver the code separately.

## Boundary

The page displays invitation identifiers but cannot verify invitation state or
claim it. The native setup app generates and retains the local identity and
performs the signed claim/status protocol. Device challenge/proof endpoints
remain on the dedicated HTTPS enrollment service, not the admin API. Keep admin
review and credential issuance on authenticated backend routes. The current
enrollment implementation does not issue adapter runtime credentials, so this
page must not claim that approval connects a device.

Test locally by opening `index.html#organization=acme&invitation_id=demo-123`.