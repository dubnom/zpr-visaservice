#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: $0 /path/to/slapd.conf" >&2
    exit 2
fi
config=$1
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fragment="$script_dir/ldap-syncprov.conf"
if [ ! -f "$config" ] || [ -L "$config" ] || [ ! -r "$fragment" ]; then
    echo "LDAP configuration and sync fragment must be readable regular files" >&2
    exit 1
fi
if [ "$(awk '$1 == "database" && $2 != "config" && $2 != "monitor" { count++ } END { print count+0 }' "$config")" -ne 1 ]; then
    echo "Expected exactly one LDAP data database; enable sync per database manually" >&2
    exit 1
fi
if grep -Eq '^[[:space:]]*overlay[[:space:]]+syncprov|^[[:space:]]*include.*ldap-syncprov\.conf' "$config"; then
    slaptest -f "$config" -u >/dev/null
    echo "LDAP sync configuration already enabled; no restart performed"
    exit 0
fi
umask 077
temporary=$(mktemp "${config}.sync.XXXXXX")
trap 'rm -f "$temporary"' EXIT HUP INT TERM
awk '
    /^database[[:space:]]/ && !loaded { print "moduleload syncprov"; loaded=1 }
    { print }
' "$config" > "$temporary"
printf '\ninclude "%s"\n' "$fragment" >> "$temporary"
slaptest -f "$temporary" -u >/dev/null
backup=$(mktemp "${config}.before-sync.XXXXXX")
cp -p "$config" "$backup"
chmod 600 "$backup" "$temporary"
mv "$temporary" "$config"
echo "LDAP sync configuration validated and enabled; restart LDAP without reseeding"