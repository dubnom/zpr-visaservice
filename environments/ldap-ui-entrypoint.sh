#!/bin/sh
set -eu

case "${ZPR_LDAP_UI_PORT:-}" in
    ''|*[!0-9]*)
        echo "ZPR_LDAP_UI_PORT must be a TCP port" >&2
        exit 2
        ;;
esac

if [ "$ZPR_LDAP_UI_PORT" -lt 1024 ] || [ "$ZPR_LDAP_UI_PORT" -gt 65535 ]; then
    echo "ZPR_LDAP_UI_PORT must be an unprivileged TCP port" >&2
    exit 2
fi

exec php \
    -d session.save_path=/tmp \
    -d session.cookie_httponly=1 \
    -d session.use_strict_mode=1 \
    -S "127.0.0.1:$ZPR_LDAP_UI_PORT" \
    -t /usr/share/phpldapadmin/htdocs
