# ZPR Web Monitor

A local, read-only web dashboard for the Visa Service admin API. It polls the
API and displays Visa Service counters, node/link state, actors, services,
active visas, and recent denials. It uses the existing admin API and does not
add write operations.

The Trusted sources page lists active providers from the read-only admin
status API. **Working** means the latest real attribute lookup succeeded;
**failed** means it failed, and **unverified** means no lookup has occurred.
The last-lookup and last-success times are shown, but this is not a live
connection probe. Older Visa Service versions without this endpoint fall back
to policy service descriptors and show **unreported** rather than guessing.
The local `demo_ldap` source optionally links to a separate phpLDAPadmin tab if
`ZPR_DEMO_LDAP_EDITOR_URL` is set to a `http://127.0.0.1:<port>/` URL. No other
source receives an editor link; the browser authenticates directly to the
editor with a separate LDAP admin login. The monitor does not proxy LDAP edits
or store the admin password.

## Configure and run

Run from the `zpr-dashboard` module directory. Use a read-only admin API key;
the key and CA file remain on the server and are never sent to the browser.

```sh
export ZPR_ADMIN_URL='https://[fd5a:5052::1]:8182'
export ZPR_ADMIN_CA_FILE='/path/to/admin-ca.pem'
export ZPR_ADMIN_KEY_FILE='/path/to/read-only-api-key'
go run ./cmd/zpr-web-dashboard
```

Open `http://127.0.0.1:8787`. To choose another local port, pass
`-listen 127.0.0.1:9000`. The server defaults to localhost so the browser UI
and its same-origin API proxy are not exposed to the LAN.
The Map is the default page. A compact row of live visa, node, decision, and
actor/dock metrics remains above the active page; Visa Service health and
uptime appear alongside the refresh controls.

The admin API URL must be HTTPS, and its certificate chain must validate
against the configured CA. Existing ZPR demo admin certificates are generated
with a common name but no subject alternative name, matching the current
`vs-admin` client behavior. For certificates with a DNS subject alternative
name, set `ZPR_ADMIN_SERVER_NAME` to enforce that name as well. Use
`ZPR_ADMIN_API_KEY` for an inline key only in controlled local environments;
the key-file option is preferred.

## Development checks

```sh
go test ./cmd/zpr-web-dashboard
go test ./...
```