#!/bin/sh
set -eu

[ "${1:-}" = verify-packaged-demo ] && [ "$#" = 1 ] || {
    echo "usage: verify-packaged-admission.sh verify-packaged-demo" >&2
    exit 2
}
sh /opt/zpr-simulator/acceptance/verify-packaged-demo.sh verify-packaged-demo
exec sh /opt/zpr-simulator/acceptance/verify-packaged-identity.sh verify-packaged-identity
