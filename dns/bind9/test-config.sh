#!/bin/sh
set -eu

if [ "$#" -ne 0 ]; then
    echo "usage: sh test-config.sh" >&2
    exit 2
fi

command -v docker >/dev/null 2>&1 || {
    echo "Docker is required to validate the BIND image and configurations." >&2
    exit 1
}

if [ -n "${DOCKER_CONTEXT:-}" ]; then
    docker_endpoint=$(docker context inspect "$DOCKER_CONTEXT" --format '{{.Endpoints.docker.Host}}') || {
        echo "Cannot inspect the selected Docker context." >&2
        exit 1
    }
elif [ -n "${DOCKER_HOST:-}" ]; then
    docker_endpoint=$DOCKER_HOST
else
    docker_endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}') || {
        echo "Cannot inspect the current Docker context." >&2
        exit 1
    }
fi
case "$docker_endpoint" in
    unix:///*) ;;
    *)
        echo "DNS config validation requires a local Docker Unix socket." >&2
        exit 1
        ;;
esac

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/zpr-bind-config.XXXXXX")
image_tag="zpr-bind9:config-test-${test_root##*/}"
image_built=0

cleanup() {
    result=$?
    if [ "$image_built" -eq 1 ]; then
        docker image rm "$image_tag" >/dev/null || result=1
    fi
    rm -rf "$test_root" || result=1
    exit "$result"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

umask 077
mkdir -p "$test_root/keys" "$test_root/zones"

docker build --tag "$image_tag" "$script_dir"
image_built=1

docker run --rm --network none --entrypoint tsig-keygen "$image_tag" \
    -a hmac-sha256 zpr-vs-publisher > "$test_root/keys/zpr-vs-publisher.key"
docker run --rm --network none --entrypoint tsig-keygen "$image_tag" \
    -a hmac-sha256 zpr-dns-viewer > "$test_root/keys/zpr-dns-viewer.key"

cat > "$test_root/zones/db.svc.zpr" <<'ZONE'
$ORIGIN svc.zpr.
$TTL 30
@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (1 60 60 86400 30)
  IN NS dns.svc.zpr.
dns IN AAAA fd5a:5052:adda:1::53
dns IN AAAA fd00:1:1::1
ZONE

cat > "$test_root/zones/db.fd00-1.ip6" <<'ZONE'
$ORIGIN 1.0.0.0.0.0.d.f.ip6.arpa.
$TTL 30
@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (1 60 60 86400 30)
  IN NS dns.svc.zpr.
ZONE

cat > "$test_root/zones/db.fd5a-5052.ip6" <<'ZONE'
$ORIGIN 2.5.0.5.a.5.d.f.ip6.arpa.
$TTL 30
@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (1 60 60 86400 30)
  IN NS dns.svc.zpr.
ZONE

cat > "$test_root/zones/db.10.in-addr" <<'ZONE'
$ORIGIN 10.in-addr.arpa.
$TTL 30
@ IN SOA dns.svc.zpr. hostmaster.svc.zpr. (1 60 60 86400 30)
  IN NS dns.svc.zpr.
ZONE

check_conf() {
    config_file=$1
    echo "Checking $config_file and its configured zones" >&2
    docker run --rm --network none --entrypoint named-checkconf \
        --volume "$test_root/keys:/run/named:ro" \
        --volume "$test_root/zones:/var/lib/bind:ro" \
        "$image_tag" -z "$config_file"
}

check_conf /etc/bind/named.conf
check_conf /etc/bind/named.conf.simulator

echo "Protected ZPR DNS configurations and configured zones passed validation."
