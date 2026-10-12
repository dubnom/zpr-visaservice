#!/bin/sh

multinode_admin_port() (
    case "${1:-}" in
        ''|*[!a-z0-9-]*) echo "invalid multi-node organization id" >&2; return 2 ;;
    esac
    admin_container="$1-vs"
    if ! admin_binding=$(docker inspect --format \
        '{"running":{{.State.Running}},"bindings":{{json (index .NetworkSettings.Ports "8183/tcp")}}}' \
        "$admin_container"); then
        echo "Visa admin endpoint unavailable for $admin_container" >&2
        return 1
    fi
    if ! admin_port=$(printf '%s' "$admin_binding" | jq -er '
        if .running != true or (.bindings | type) != "array" or (.bindings | length) != 1 then
            error("expected one published port on a running Visa Service")
        else .bindings[0] end
        | if (.HostIp == "127.0.0.1" or .HostIp == "0.0.0.0" or .HostIp == "") and
             (.HostPort | type == "string") and (.HostPort | test("^[0-9]{1,5}$")) then
            .HostPort | tonumber | select(. >= 1 and . <= 65535)
          else error("invalid local Visa admin binding") end
    '); then
        echo "Visa admin endpoint has no unambiguous local binding for $admin_container" >&2
        return 1
    fi
    printf '%s\n' "$admin_port"
)
