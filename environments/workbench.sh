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
        release=dev-20261029-r7
        simulator_release=dev-20261030-r8
        state=$root/state-dev-20261029-r4
        workspace=$root/simulator-state-dev-20261029-r4
        room=18787 service=18790 ldap=11636 ldap_ui=18789 logging=15080 oidc=15556
        admin=18182 delivery=18793 dns=15353 simulator=18788
        ;;
    stable)
        release=scenario-20261029-r6
        simulator_release=$release
        state=$root/state-scenario-r6
        workspace=$root/simulator-state-scenario-r6
        room=38787 service=38790 ldap=31636 ldap_ui=38789 logging=35080 oidc=35556
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
            -ldap-admin-port "$ldap_ui" \
            -logging-port "$logging" -oidc-port "$oidc" -vs-admin-port "$admin" \
            -snapshot-delivery-port "$delivery" -dns-transfer-port "$dns" \
            -simulator-port "$simulator" -scenario-demo
        ;;
    simulator)
        if [ "$1" = dev ]; then
            set -- --full-catalog
        else
            set --
        fi
        exec "$root/zpr-simulator-$simulator_release/simulator.sh" start \
            "$root/zpr-evaluation-$simulator_release" "$state" "$workspace" "$@"
        ;;
    *)
        echo "unknown role: $2" >&2
        exit 2
        ;;
esac
