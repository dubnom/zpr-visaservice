#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dashboard_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
runtime_dir=$(CDPATH='' cd -- "$dashboard_dir/../../.local-runtime" && pwd)
directory="$runtime_dir/operator-login"
container=zpr-local-operator-idp
image=zpr-local-operator-idp:local
issuer=https://zpr-id.localhost:5556

case "${1:-}" in
    init)
        username=${2:-}
        organization=${3:-}
        case "$username" in ''|*[!a-zA-Z0-9_-]*) echo "Provide a local username using letters, digits, underscore or hyphen" >&2; exit 2 ;; esac
        case "$organization" in ''|*[!a-zA-Z0-9_-]*) echo "Provide an explicit enrollment organization" >&2; exit 2 ;; esac
        [ ! -e "$directory" ] || { echo "Operator configuration already exists; refusing to overwrite identity or secrets" >&2; exit 1; }
        for tool in openssl jq htpasswd; do
            command -v "$tool" >/dev/null || { echo "Required tool missing: $tool" >&2; exit 1; }
        done
        umask 077
        staging="$directory.init.$$"
        mkdir -m 700 "$staging" "$staging/data"
        trap 'echo "Incomplete configuration retained for inspection at $staging" >&2' EXIT
        openssl genrsa -out "$staging/ca.key" 3072 2>/dev/null
        openssl req -new -x509 -sha256 -days 365 -key "$staging/ca.key" -out "$staging/ca.crt" \
            -subj "/CN=ZPR local operator development CA" \
            -addext "basicConstraints=critical,CA:TRUE,pathlen:0" -addext "keyUsage=critical,keyCertSign,cRLSign"
        for name in room idp; do
            case "$name" in room) host=localhost; sans=DNS:localhost,IP:127.0.0.1 ;; idp) host=zpr-id.localhost; sans=DNS:zpr-id.localhost ;; esac
            openssl genrsa -out "$staging/$name.key" 3072 2>/dev/null
            openssl req -new -key "$staging/$name.key" -out "$staging/$name.csr" -subj "/CN=$host"
            printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=%s\n' "$sans" > "$staging/$name.extensions"
            openssl x509 -req -sha256 -days 90 -in "$staging/$name.csr" -CA "$staging/ca.crt" -CAkey "$staging/ca.key" \
                -CAcreateserial -out "$staging/$name.crt" -extfile "$staging/$name.extensions" 2>/dev/null
            rm "$staging/$name.csr" "$staging/$name.extensions"
        done
        openssl rand -hex 24 > "$staging/operator-password"
        openssl rand -hex 32 > "$staging/client.secret"
        hash=$(htpasswd -BinC 10 "$username" < "$staging/operator-password" | cut -d: -f2)
        # Dex encodes its stable user ID and local connector ID as a protobuf subject.
        subject=$(printf '\012\017zpr-local-admin\022\005local' | openssl base64 -A | tr '+/' '-_' | tr -d '=')
        jq -n --arg issuer "$issuer" --arg username "$username" --arg hash "$hash" --rawfile secret "$staging/client.secret" \
            '{issuer:$issuer,storage:{type:"sqlite3",config:{file:"/operator/data/dex.db"}},
            web:{https:"0.0.0.0:5556",tlsCert:"/operator/idp.crt",tlsKey:"/operator/idp.key"},
            frontend:{dir:"/srv/dex/web"},
            oauth2:{skipApprovalScreen:true},enablePasswordDB:true,
            staticClients:[{id:"zpr-control-room",name:"ZPR local Control Room",secret:($secret|rtrimstr("\n")),redirectURIs:["https://localhost:8787/auth/operator/callback"]}],
            staticPasswords:[{email:$username,username:$username,userID:"zpr-local-admin",hash:$hash}],
            expiry:{idTokens:"30m"},logger:{level:"warn",format:"json"}}' > "$staging/dex.json"
        jq -n --arg issuer "$issuer" --arg subject "$subject" \
            '{version:1,issuer:$issuer,client_id:"zpr-control-room",redirect_url:"https://localhost:8787/auth/operator/callback",session_lifetime_seconds:1800,
            grants:[{issuer:$issuer,subject:$subject,organizations:["*"],permissions:["monitor.read","policy.read","policy.analyze","policy.edit","gateway.read","gateway.analyze","gateway.edit","read","create","cancel"]}]}' > "$staging/oidc.json"
        openssl genpkey -algorithm ED25519 -out "$staging/delegation.key"
        public=$(openssl pkey -in "$staging/delegation.key" -pubout -outform DER | tail -c 32 | openssl base64 -A | tr '+/' '-_' | tr -d '=')
        room_pin=$(openssl x509 -in "$runtime_dir/service-certs/control-room-client.crt" -outform DER | openssl dgst -sha256 | awk '{print $NF}')
        reader_pin=$(openssl x509 -in "$runtime_dir/service-certs/control-policy-client.crt" -outform DER | openssl dgst -sha256 | awk '{print $NF}')
        jq -n '{version:1,audience:"https://host.docker.internal:8790",key_id:"local-room"}' > "$staging/signer.json"
        jq -n --arg issuer "$issuer" --arg subject "$subject" --arg public "$public" --arg pin "$room_pin" \
            '{version:1,audience:"https://host.docker.internal:8790",keys:[{key_id:"local-room",public_key:$public,certificate_sha256:$pin}],
            grants:[{issuer:$issuer,subject:$subject,organizations:["*"],permissions:["read","create","cancel"]}]}' > "$staging/trust.json"
        jq -n --arg organization "$organization" --arg pin "$reader_pin" \
            '{version:1,gui_invitation_creation_enabled:false,invitation_lifetime_seconds:86400,approval_lifetime_seconds:86400,
            principals:[{name:"local-maintenance-reader",certificate_sha256:$pin,organizations:[$organization],permissions:["read"]}],
            organizations:{($organization):{types:["laptop","workstation","server"],profiles:["standard"]}}}' > "$staging/enrollment.json"
        jq -n --arg username "$username" '{origin:"https://localhost:8787",organization:"*",username:$username}' > "$staging/stack.json"
        mv "$staging" "$directory"
        trap - EXIT
        printf 'Local operator configuration prepared. Username: %s\nPassword file: %s/operator-password\nCA certificate: %s/ca.crt\nCreation remains disabled until explicitly enabled in enrollment.json.\n' "$username" "$directory" "$directory"
        ;;
    start)
        [ -r "$directory/dex.json" ] || { echo "Run init before starting the local identity provider" >&2; exit 1; }
        if docker inspect "$container" >/dev/null 2>&1; then
            docker start "$container" >/dev/null
        else
            docker build -t "$image" -f "$script_dir/Dockerfile.operator-idp" "$dashboard_dir"
            docker run -d --name "$container" --label zpr.local-operator-idp=true --restart unless-stopped \
                --user 0:0 -p 127.0.0.1:5556:5556 \
                -v "$directory/dex.json:/operator/dex.json:ro" \
                -v "$directory/idp.crt:/operator/idp.crt:ro" -v "$directory/idp.key:/operator/idp.key:ro" \
                -v "$directory/data:/operator/data" "$image" dex serve /operator/dex.json >/dev/null
        fi
        attempts=0
        while :; do
            if discovery=$(curl --fail --silent --show-error --max-time 5 --cacert "$directory/ca.crt" \
                --resolve zpr-id.localhost:5556:127.0.0.1 "$issuer/.well-known/openid-configuration" 2>/dev/null) &&
                printf '%s' "$discovery" | jq -e --arg issuer "$issuer" '.issuer==$issuer' >/dev/null; then
                break
            fi
            attempts=$((attempts + 1))
            if [ "$attempts" -ge 30 ]; then
                echo "Local identity provider did not return trusted discovery; inspect docker logs $container" >&2
                exit 1
            fi
            sleep 1
        done
        echo "Local identity provider ready at $issuer"
        ;;
    stop) docker stop "$container" >/dev/null ;;
    status) docker inspect "$container" --format '{{.State.Status}}' ;;
    trust)
        [ "$(uname -s)" = Darwin ] || { echo "Import ca.crt into your browser trust store on this platform" >&2; exit 1; }
        security add-trusted-cert -r trustRoot -k "$HOME/Library/Keychains/login.keychain-db" "$directory/ca.crt"
        echo "Dedicated development CA installed in the current user's login keychain"
        ;;
    *) echo "usage: $0 {init username enrollment-organization|start|stop|status|trust}" >&2; exit 2 ;;
esac
