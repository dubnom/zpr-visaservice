#!/bin/sh

refresh_multinode_gateway() (
    gateway_organization=$1
    case "$gateway_organization" in ''|*[!a-z0-9-]*) echo "invalid gateway organization id" >&2; return 2 ;; esac
    gateway_profile="$dashboard_dir/cmd/zpr-web-dashboard/examples/organizations/$gateway_organization.json"
    gateway_enabled=$(jq -r '.web_gateway.allowed_hosts | type=="array" and length>0' "$gateway_profile") || return 1
    if [ "$gateway_enabled" != true ]; then
        return 0
    fi
    gateway_container="$gateway_organization-internet-gateway"
    gateway_image=$(docker inspect --format '{{.Config.Image}}' "$gateway_organization-vs") || return 1
    gateway_admin_port=$(multinode_admin_port "$gateway_organization") || return 1
    docker restart "$gateway_container" >/dev/null || return 1
    for gateway_attempt in $(seq 30); do
        if gateway_services=$(docker run --rm \
            -v "$multinode_dir/zpr-conf/include:/runtime/include:ro" \
            -v "$runtime_dir/multinode/$gateway_organization/bob:/runtime/operator:ro" \
            "$gateway_image" /app/bin/vs-admin \
            --svc-url "https://host.docker.internal:$gateway_admin_port" \
            --ca-cert /runtime/include/admin-tls-cert.pem \
            --api-key-file /runtime/operator/web-monitor.key \
            --format compact services) &&
           printf '%s' "$gateway_services" | jq -e 'any(.[]; .id=="internet-gateway.svc.zpr")' >/dev/null; then
            echo "Organization Gateway registered with effective policy: $gateway_organization"
            return 0
        fi
        sleep 1
    done
    echo "Organization Gateway did not register with effective policy: $gateway_organization" >&2
    return 1
)

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
