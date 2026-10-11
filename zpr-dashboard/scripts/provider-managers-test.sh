#!/bin/sh
set -eu
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
. "$script_dir/provider-managers.sh"
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/provider-managers-test.XXXXXX")
trap 'rm -f "$test_dir/managers.json"; rmdir "$test_dir"' EXIT
registry="$test_dir/managers.json"
export SIMULATOR_URL=http://127.0.0.1:1 SIMULATION_MANIFEST=/nonexistent/simulator

test "$(provider_manager_urls "$registry")" = '{}'
register_provider_managers "$registry" authentication https://auth.example.test/admin
register_provider_managers "$registry" "$(printf 'great_lakes_ldap\nanother_directory')" http://127.0.0.1:12345/
provider_manager_urls "$registry" |
    jq -e 'length == 3 and .authentication == "https://auth.example.test/admin" and
        .great_lakes_ldap == "http://127.0.0.1:12345/" and .another_directory == .great_lakes_ldap' >/dev/null
provider_manager_urls "$registry" '{"great_lakes_ldap":"https://directory.example.test/manage"}' |
    jq -e 'length == 3 and .great_lakes_ldap == "https://directory.example.test/manage" and
        .another_directory == "http://127.0.0.1:12345/"' >/dev/null
register_provider_managers "$registry" great_lakes_ldap http://127.0.0.1:54321/
provider_manager_urls "$registry" |
    jq -e 'length == 3 and .great_lakes_ldap == "http://127.0.0.1:54321/" and
        .another_directory == "http://127.0.0.1:12345/"' >/dev/null
for invalid in '[]' '{"source":123}' 'invalid JSON'; do
    if provider_manager_urls "$registry" "$invalid" >/dev/null 2>&1; then
        echo "Invalid manager override was accepted: $invalid" >&2
        exit 1
    fi
done
printf 'invalid JSON\n' > "$registry"
if register_provider_managers "$registry" new_source https://example.test/ >/dev/null 2>&1; then
    echo "Invalid registry was silently replaced" >&2
    exit 1
fi
test "$(cat "$registry")" = 'invalid JSON'
echo "Provider manager registration tests passed"
