#!/bin/sh
# Independent operator installs. Inputs are copied into operator ownership once;
# the running services never open Simulator profiles, catalogs or workspaces.
set -eu
umask 077
fail() { printf 'organization-runtime: %s\n' "$*" >&2; exit 1; }
[ "$#" -eq 2 ] || fail "usage: organization-runtime.sh ORGANIZATION render|start"
organization=$1
action=$2
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
case "$action" in render|start) ;; *) fail "action must be render or start" ;; esac
case "$organization" in
    northstar) prefix=20; base=dc=northstar,dc=test; location=44.98,-93.27 ;;
    great-lakes) prefix=21; base=dc=greatlakes,dc=test; location=43.04,-87.91 ;;
    load-lab) prefix=22; base=dc=loadlab,dc=test; location=41.88,-87.63 ;;
    redwood) prefix=23; base=dc=redwood,dc=test; location=40.80,-124.16 ;;
    velocity) prefix=24; base=dc=velocity,dc=test; location=37.34,-121.89 ;;
    *) fail "unknown operator organization" ;;
esac
command -v sha256sum >/dev/null || fail "sha256sum is required"
(cd "$root" && sha256sum --check --strict SHA256SUMS >/dev/null) ||
    fail "operator binaries or immutable input checksums changed"
inputs=$root/operator-inputs/$organization
set -- -evaluation -state "$root/state/$organization" \
    -organization-id "$organization" -browser-ip 10.0.1.160 \
    -control-room-binary "$root/bin/zpr-control-room" \
    -room-port "${prefix}787" -service-port "${prefix}790" \
    -ldap-port "${prefix}636" -ldap-admin-port "${prefix}789" \
    -logging-port "${prefix}080" -oidc-port "${prefix}556" \
    -vs-admin-port "${prefix}182" -snapshot-delivery-port "${prefix}793" \
    -dns-transfer-port "${prefix}353" \
    -visa-service -data-plane -protected-dns -node-location "$location" \
    -data-plane-image sha256:9f2cb68c4bbf0424888369270f1e503df03bf35e2be69e5a226772e55daa7795 \
    -ldap-image sha256:0de3850635911109f0daf8f5957808ec4f4eb827fbaaa339100b5f1bab925745 \
    -ldap-admin-image sha256:85801624264f5b5a7771c5f12fbbc032edb9dedd6bc4449ef01381950bb7dedc \
    -logging-image sha256:d4a878fac1f6c56003764f7f2a1625668917388f167e222c8c810de3f54c56ba \
    -logging-platform linux/amd64 \
    -oidc-image sha256:5d0656fce7d453c0e3b2706abf40c0d0ce5b371fb0b73b3cf714d05f35fa5f86 \
    -oidc-platform linux/amd64 -directory-base-dn "$base"
case "$organization" in
    northstar)
        for name in demo-company.ldif demo-employees.ldif demo-roles.ldif demo-machines.ldif; do
            set -- "$@" -directory-ldif "$inputs/$name"
        done ;;
    *)
        set -- "$@" -directory-ldif "$inputs/directory.ldif"
        if [ "$organization" = redwood ]; then
            set -- "$@" -directory-ldif "$inputs/directory-people.ldif"
        fi ;;
esac
case "$organization" in
    great-lakes)
        set -- "$@" -data-plane-node shenzhen-office@22.54,114.06 \
            -data-plane-node tijuana-plant@32.51,-117.04 ;;
    redwood)
        set -- "$@" -data-plane-node regional-yard@38.58,-121.49 ;;
esac
if [ "$action" = render ]; then
    set -- "$@" -render-only
elif [ -f "$root/enrollment/$organization/operator-services.json" ]; then
    set -- "$@" -operator-services-file "$root/enrollment/$organization/operator-services.json"
elif [ -f "$root/policy/$organization/operator-services.json" ]; then
    set -- "$@" -operator-services-file "$root/policy/$organization/operator-services.json"
fi
exec "$root/bin/zpr-lab-bootstrap" "$@"
