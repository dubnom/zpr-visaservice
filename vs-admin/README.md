# vs-admin

Visa Service administration tool which communicates with a running visa service
using its admin API (HTTPS).

The default port for the admin API is 8182. Unless you are on the same host as 
the running visa service you will need policy permission to access the admin
API.

Use an `https://` endpoint and supply its trusted issuing CA with `--ca-cert`.
The client verifies certificate trust, validity dates and the endpoint's DNS/IP
identity for every request. Plain HTTP and invalid certificates are rejected;
there is no insecure-TLS mode. A request failure returns a nonzero exit status.
Prefer `--api-key-file` to avoid putting credentials in shell history or process
arguments. Certificates must include the endpoint name/address in their SAN.

Run the client and local TLS regression tests with:

```sh
cargo test --locked --offline -p vs-admin --bin vs-admin
```
