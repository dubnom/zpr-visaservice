#!/bin/sh
set -eu
umask 077
[ "$#" -ge 1 ] || { echo "usage: prepare-retained-machine-control.sh PRIVATE_DIRECTORY [MACHINE ...]" >&2; exit 2; }
directory=$1
shift
machines=${*:-"machine-03 machine-05"}
case "$directory" in /*) ;; *) echo "absolute private directory required" >&2; exit 2 ;; esac
for machine in $machines; do
    case "$machine" in
        server|.|*/*|*..*|"") echo "machine identifiers must be plain names other than server" >&2; exit 2 ;;
    esac
done
mkdir -p "$directory"
[ "$(CDPATH='' cd -- "$directory" && pwd -P)" = "$directory" ] || {
    echo "canonical private directory required" >&2; exit 1;
}
chmod 700 "$directory"
issue() {
    identity=$1
    subject=$2
    usage=$3
    certificate=$4
    mkdir -p "$directory/$identity"
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=%s\n' "$usage" > "$directory/$identity/extensions.cnf"
    if [ "$usage" = serverAuth ]; then
        printf 'subjectAltName=DNS:host.docker.internal,DNS:localhost,IP:127.0.0.1\n' >> "$directory/$identity/extensions.cnf"
    fi
    openssl req -new -newkey rsa:2048 -nodes -sha256 -subj "/CN=$subject" \
        -keyout "$directory/$identity/$certificate.key" -out "$directory/$identity/request.csr" 2>>"$directory/generation.log"
    openssl x509 -req -days 30 -sha256 -in "$directory/$identity/request.csr" \
        -CA "$directory/control-ca.crt" -CAkey "$directory/control-ca.key" \
        -set_serial "$(openssl rand -hex 16 | sed 's/^/0x/')" \
        -extfile "$directory/$identity/extensions.cnf" -out "$directory/$identity/$certificate.crt" 2>>"$directory/generation.log"
    rm "$directory/$identity/request.csr" "$directory/$identity/extensions.cnf"
}
if [ -e "$directory/control-ca.key" ]; then
    # Existing control CA: verify every requested machine identity and issue
    # only the missing ones from the same CA; the server identity and every
    # existing machine identity are never replaced.
    for machine in $machines; do
        if [ ! -e "$directory/$machine/client.crt" ]; then
            issue "$machine" "$machine" clientAuth client
        fi
        openssl verify -purpose sslclient -CAfile "$directory/control-ca.crt" "$directory/$machine/client.crt"
        openssl x509 -in "$directory/$machine/client.crt" -noout -subject -nameopt RFC2253 |
            grep -Fx "subject=CN=$machine" >/dev/null
    done
    openssl verify -purpose sslserver -verify_hostname host.docker.internal \
        -CAfile "$directory/control-ca.crt" "$directory/server.crt"
    printf 'Prepared Simulator-only mTLS identities; no ZPR enrollment or production trust changed.\n'
    exit 0
fi
[ -z "$(find "$directory" -mindepth 1 -maxdepth 1 -print -quit)" ] || {
    echo "refusing to overwrite an incomplete machine-control identity directory" >&2; exit 1;
}
openssl req -x509 -newkey rsa:2048 -nodes -days 30 -sha256 \
    -subj /CN=Velocity-Simulator-Control-CA \
    -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign,cRLSign \
    -keyout "$directory/control-ca.key" -out "$directory/control-ca.crt" 2>"$directory/generation.log"
for machine in $machines; do
    issue "$machine" "$machine" clientAuth client
done
issue . host.docker.internal serverAuth server
printf 'Prepared Simulator-only mTLS identities; no ZPR enrollment or production trust changed.\n'
