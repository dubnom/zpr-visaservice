#!/bin/sh
set -eu

fail() { echo "prepare-visa-service: $*" >&2; exit 1; }
if [ "$#" -ne 2 ] && { [ "$#" -ne 3 ] || [ "$3" != --node ]; }; then
    echo "Usage: sh prepare-visa-service.sh /absolute/compiled-bin /protected/new-state [--node]" >&2
    exit 2
fi
node_mode=${3:-}
for directory in "$1" "$2"; do
    case "$directory" in
        /*) ;;
        *) fail "paths must be absolute" ;;
    esac
    case "$directory" in
        /|*/|*/.|*/..|*/../*|*/./*|*[!a-zA-Z0-9_./-]*)
            fail "use canonical paths containing only letters, digits, slash, dot, underscore and hyphen" ;;
    esac
done
bin=$1
output=$2
parent=$(dirname -- "$output")
[ "$(CDPATH='' cd -- "$parent" && pwd -P)" = "$parent" ] || fail "state parent must exist without symlink components"
[ -z "$(find "$parent" -prune \( -perm -002 -o -perm -020 \) -print)" ] ||
    fail "state parent must not be group- or other-writable"
for tool in openssl; do
    command -v "$tool" >/dev/null 2>&1 || fail "missing prerequisite: $tool"
done
for name in zplc vsapikey vs vs-admin; do
    [ -f "$bin/$name" ] && [ ! -L "$bin/$name" ] && [ -x "$bin/$name" ] ||
        fail "missing regular compiled executable: $name"
done
if [ "$node_mode" = --node ]; then
    [ -f "$bin/ph" ] && [ ! -L "$bin/ph" ] && [ -x "$bin/ph" ] ||
        fail "missing regular compiled executable: ph"
fi
umask 077
mkdir -- "$output" || fail "state already exists or cannot be created; nothing was overwritten"
trap 'status=$?; if [ "$status" -ne 0 ]; then echo "prepare-visa-service: failed; inspect partial private state and logs; no service was launched" >&2; fi' EXIT
trap 'echo "prepare-visa-service: interrupted; inspect partial private state before retrying" >&2; exit 1' HUP INT TERM
mkdir "$output/database"
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 365 \
    -subj '/CN=ZPR isolated engineering authority' \
    -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' \
    -addext 'keyUsage=critical,keyCertSign,cRLSign' \
    -keyout "$output/authority.key" -out "$output/authority.pem" > "$output/key-generation.log" 2>&1
openssl req -new -newkey rsa:2048 -nodes -sha256 \
    -subj '/CN=ZPR isolated Visa administration' \
    -keyout "$output/admin.key" -out "$output/admin.csr" >> "$output/key-generation.log" 2>&1
cat > "$output/admin-extensions.cnf" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=IP:127.0.0.1,IP:::1
EOF
openssl x509 -req -in "$output/admin.csr" -CA "$output/authority.pem" \
    -CAkey "$output/authority.key" -CAcreateserial -days 90 -sha256 \
    -extfile "$output/admin-extensions.cnf" -out "$output/admin.pem" >> "$output/key-generation.log" 2>&1
for name in node visa policy-signing; do
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
        -out "$output/$name.key" >> "$output/key-generation.log" 2>&1
    openssl pkey -in "$output/$name.key" -pubout -out "$output/$name.pub.pem"
done
vs_address=::1
vs_port=8181
node_inventory=
if [ "$node_mode" = --node ]; then
    vs_address=fd5a:5052::1
    vs_port=5002
    openssl genpkey -algorithm X25519 -out "$output/node-noise.key" >> "$output/key-generation.log" 2>&1
    openssl pkey -in "$output/node-noise.key" -pubout -out "$output/node-noise.pub.pem"
    openssl x509 -new -force_pubkey "$output/node-noise.pub.pem" \
        -subj /CN=node -CA "$output/authority.pem" -CAkey "$output/authority.key" \
        -CAserial "$output/authority.srl" -days 90 -sha256 -out "$output/node-noise.pem" \
        >> "$output/key-generation.log" 2>&1
    openssl verify -CAfile "$output/authority.pem" "$output/node-noise.pem" > "$output/node-certificate-verification.txt"
    cat > "$output/node.toml" <<EOF
[global]
name = "node"
ca_file = "$output/authority.pem"
certificate_file = "$output/node-noise.pem"
private_key_file = "$output/node-noise.key"
self_addr = "[fd00:5a::2]:5000"
zpr_addr = ["fd5a:5052::2"]
tun_if = "zprtest0"
control_path = "$output/node-control.sock"
capture_path = "$output/node-capture.sock"

[authentication]
auth_private_key = "$output/node.key"

[node]
advertised_substrate_addr = "[fd00:5a::2]:5000"
local_vs_dock = true
EOF
    node_inventory='  "node_config": "node.toml",'
fi
"$bin/vsapikey" create readwrite engineering-administrator "$output/vs_keys.toml" --init > "$output/administrator.key"
"$bin/vsapikey" create read engineering-observer "$output/vs_keys.toml" > "$output/observer.key"
identity=$(openssl rand -hex 16)
cat > "$output/vs.toml" <<EOF
[core]
vs_addr = "$vs_address"
admin_addr = "::1"
vsapi_port = $vs_port
admin_port = 8182
admin_cert = "admin.pem"
admin_key = "admin.key"
api_keys = "vs_keys.toml"
vk_uri = "redis+unix://$output/database/redis.sock"
identity = "$identity"
file_ts_dir = "."
EOF
cat > "$output/bootstrap.zplc" <<EOF
[nodes.node]
provider = [["device.zpr.adapter.cn", "node"]]
zpr_address = "fd5a:5052::2"

[trusted_services.default]
cert_path = "$output/authority.pem"

[bootstrap]
"vs.zpr" = "$output/visa.pub.pem"
"node" = "$output/node.pub.pem"

[protocols.ping]
l4protocol = "iana.ICMP6"
icmp_type = "request-response"
icmp_codes = [128, 129]

[services.PingableVs]
protocol = "ping"
EOF
cat > "$output/bootstrap.zpl" <<'EOF'
define Node as a device with zpr.adapter.cn:node.
define Vs as a device with zpr.adapter.cn:'vs.zpr'.
define PingableVs as a service with device.zpr.adapter.cn:'vs.zpr'.
service PingableVs as json {"service_class":"PingableVs"}.
  allow Node.
EOF
if ! "$bin/zplc" "$output/bootstrap.zpl" -c "$output/bootstrap.zplc" \
    -k "$output/policy-signing.key" -d "$output" -o bootstrap.bin2 > "$output/compiler.log" 2>&1; then
    fail "bootstrap compilation failed; see private compiler.log; no service was started"
fi
[ -s "$output/bootstrap.bin2" ] || fail "compiler produced no bootstrap policy"
cat > "$output/visa-bootstrap.json" <<EOF
{
  "schema_version": 1,
  "kind": "isolated-visa-engineering-bootstrap",
  "service_identity": "$identity",
  "admin_origin": "https://[::1]:8182",
  "policy": "bootstrap.bin2",
  "ca": "authority.pem",
  "administrator_key": "administrator.key",
  "observer_key": "observer.key",
$node_inventory
  "complete_installation": false
}
EOF
echo "Fresh private Visa engineering state prepared. No service, Simulator, network or system trust was configured."
echo "Isolated test configuration; node transport addresses appear only with --node. Not a production installation; never package this private state."
