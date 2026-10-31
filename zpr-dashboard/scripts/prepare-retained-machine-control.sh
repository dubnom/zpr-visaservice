#!/bin/sh
set -eu
umask 077
[ "$#" -eq 1 ] || { echo "usage: prepare-retained-machine-control.sh PRIVATE_DIRECTORY" >&2; exit 2; }
directory=$1
case "$directory" in /*) ;; *) echo "absolute private directory required" >&2; exit 2 ;; esac
mkdir -p "$directory"
[ "$(CDPATH='' cd -- "$directory" && pwd -P)" = "$directory" ] || {
    echo "canonical private directory required" >&2; exit 1;
}
chmod 700 "$directory"
if [ -e "$directory/control-ca.key" ]; then
    for machine in machine-03 machine-05; do
        openssl verify -purpose sslclient -CAfile "$directory/control-ca.crt" "$directory/$machine/client.crt"
        openssl x509 -in "$directory/$machine/client.crt" -noout -subject -nameopt RFC2253 |
            grep -Fx "subject=CN=$machine" >/dev/null
    done
    openssl verify -purpose sslserver -verify_hostname host.docker.internal \
        -CAfile "$directory/control-ca.crt" "$directory/server.crt"
    exit 0
fi
[ -z "$(find "$directory" -mindepth 1 -maxdepth 1 -print -quit)" ] || {
    echo "refusing to overwrite an incomplete machine-control identity directory" >&2; exit 1;
}
openssl req -x509 -newkey rsa:2048 -nodes -days 30 -sha256 \
    -subj /CN=Velocity-Simulator-Control-CA \
    -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign,cRLSign \
    -keyout "$directory/control-ca.key" -out "$directory/control-ca.crt" 2>"$directory/generation.log"
for machine in machine-03 machine-05 server; do
    identity=$machine
    subject=$machine
    usage=clientAuth
    if [ "$machine" = server ]; then
        identity=.
        subject=host.docker.internal
        usage=serverAuth
    fi
    mkdir -p "$directory/$identity"
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=%s\n' "$usage" > "$directory/$identity/extensions.cnf"
    if [ "$machine" = server ]; then
        printf 'subjectAltName=DNS:host.docker.internal,DNS:localhost,IP:127.0.0.1\n' >> "$directory/$identity/extensions.cnf"
        certificate=server
    else
        certificate=client
    fi
    openssl req -new -newkey rsa:2048 -nodes -sha256 -subj "/CN=$subject" \
        -keyout "$directory/$identity/$certificate.key" -out "$directory/$identity/request.csr" 2>>"$directory/generation.log"
    openssl x509 -req -days 30 -sha256 -in "$directory/$identity/request.csr" \
        -CA "$directory/control-ca.crt" -CAkey "$directory/control-ca.key" \
        -set_serial "$(openssl rand -hex 16 | sed 's/^/0x/')" \
        -extfile "$directory/$identity/extensions.cnf" -out "$directory/$identity/$certificate.crt" 2>>"$directory/generation.log"
    rm "$directory/$identity/request.csr" "$directory/$identity/extensions.cnf"
done
printf 'Prepared Simulator-only mTLS identities; no ZPR enrollment or production trust changed.\n'
