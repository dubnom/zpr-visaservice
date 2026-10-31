#!/bin/sh
set -eu
umask 077
fail() { printf 'zpr-evaluation: %s\n' "$*" >&2; exit 1; }
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
action=${1:-}
[ "$#" -gt 0 ] || fail "usage: ./demo.sh verify|load|start STATE [evaluation flags...]"
shift
command -v sha256sum >/dev/null || fail "sha256sum is required"
case "$action" in
    verify|load|start) ;;
    *) fail "usage: ./demo.sh verify|load|start STATE [evaluation flags...]" ;;
esac
case "$action" in
    verify|load) [ "$#" -eq 0 ] || fail "$action takes no arguments" ;;
    start) [ "$#" -ge 1 ] || fail "start requires a private absolute state directory" ;;
esac
(cd "$root" && sha256sum --check --strict SHA256SUMS) || fail "package verification failed; nothing started"
[ "$action" != verify ] || exit 0
[ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ] ||
    fail "this engineering package requires Linux x86-64"
command -v docker >/dev/null || fail "Docker Engine with Compose v2 is required"
docker compose version >/dev/null || fail "Docker Compose v2 is required"
data_plane='' ldap='' ldap_admin_ui='' logging='' oidc=''
while read -r role image extra; do
    [ -z "$extra" ] || fail "invalid image inventory"
    case "$image" in sha256:*) ;; *) fail "immutable image ID required" ;; esac
    digest=${image#sha256:}
    case "$digest" in ""|*[!0-9a-f]*) fail "invalid image digest" ;; esac
    [ "${#digest}" -eq 64 ] || fail "invalid image digest"
    case "$role" in
        data-plane) [ -z "$data_plane" ] || fail "duplicate data-plane image"; data_plane=$image ;;
        ldap) [ -z "$ldap" ] || fail "duplicate LDAP image"; ldap=$image ;;
        ldap-admin-ui) [ -z "$ldap_admin_ui" ] || fail "duplicate LDAP GUI image"; ldap_admin_ui=$image ;;
        logging) [ -z "$logging" ] || fail "duplicate logging image"; logging=$image ;;
        oidc) [ -z "$oidc" ] || fail "duplicate OIDC image"; oidc=$image ;;
        *) fail "unexpected image role" ;;
    esac
done < "$root/images.txt"
[ -n "$data_plane" ] && [ -n "$ldap" ] && [ -n "$ldap_admin_ui" ] && [ -n "$logging" ] && [ -n "$oidc" ] ||
    fail "incomplete image inventory"
if [ "$action" = load ]; then
    for role in data-plane ldap ldap-admin-ui logging oidc; do
        docker load --input "$root/images/$role.tar"
    done
fi
for image in "$data_plane" "$ldap" "$ldap_admin_ui" "$logging" "$oidc"; do
    [ "$(docker image inspect --format '{{.Os}}/{{.Architecture}} {{.Id}}' "$image")" = "linux/amd64 $image" ] ||
        fail "required exact image unavailable; run ./demo.sh load first"
done
[ "$action" != load ] || exit 0
state=$1
shift
case "$state" in /*) ;; *) fail "state directory must be absolute" ;; esac
printf 'Engineering evaluation: privileged data plane; use only on a trusted isolated host.\n'
printf 'Foreground startup; Ctrl-C stops only this installation and retains its state.\n'
exec "$root/bin/zpr-lab-bootstrap" -evaluation -state "$state" \
    -control-room-binary "$root/bin/zpr-control-room" \
    -visa-service -data-plane -protected-dns -data-plane-endpoints 3 \
    -data-plane-image "$data_plane" -ldap-image "$ldap" -ldap-admin-image "$ldap_admin_ui" \
    -logging-image "$logging" -logging-platform linux/amd64 \
    -oidc-image "$oidc" -oidc-platform linux/amd64 "$@"
