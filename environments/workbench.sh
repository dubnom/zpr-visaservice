#!/bin/sh
set -eu
umask 077

if [ "$#" -ne 2 ]; then
    echo "usage: sh workbench.sh dev|stable production|simulator" >&2
    exit 2
fi

root=$HOME/zpr-demo-refresh-20261029-r1
case "$1" in
    dev)
        release=dev-20261029-r1
        state=$root/state-dev-20261029-r1
        workspace=$root/simulator-state-dev-20261029-r1
        room=18787 service=18790 ldap=11636 logging=15080 oidc=15556
        admin=18182 delivery=18793 dns=15353 simulator=18788
        ;;
    stable)
        release=scenario-20261029-r3
        state=$root/state-scenario-r1
        workspace=$root/simulator-state-scenario-r3
        room=38787 service=38790 ldap=31636 logging=35080 oidc=35556
        admin=38182 delivery=38793 dns=35353 simulator=38788
        ;;
    *)
        echo "unknown environment: $1" >&2
        exit 2
        ;;
esac

package=$root/zpr-evaluation-$release
case "$2" in
    production)
        exec "$package/demo.sh" start "$state" \
            -browser-ip 10.0.1.160 \
            -room-port "$room" -service-port "$service" -ldap-port "$ldap" \
            -logging-port "$logging" -oidc-port "$oidc" -vs-admin-port "$admin" \
            -snapshot-delivery-port "$delivery" -dns-transfer-port "$dns" \
            -simulator-port "$simulator" -scenario-demo
        ;;
    simulator)
        exec "$root/zpr-simulator-$release/simulator.sh" start \
            "$package" "$state" "$workspace"
        ;;
    *)
        echo "unknown role: $2" >&2
        exit 2
        ;;
esac
