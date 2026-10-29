#!/bin/sh
# Simulator-owned lab configuration. Sourced only after production command dispatch.

simulator_require_lab_container() {
    operator_owner=$(docker inspect -f '{{index .Config.Labels "zpr.operator-deployment"}}' "$1" 2>/dev/null || true)
    if [ -n "$operator_owner" ] && [ "$operator_owner" != '<no value>' ]; then
        echo "Simulator refuses to manage an operator-owned container: $1" >&2
        return 1
    fi
}

simulator_start_control_room() {
    simulator_require_lab_container "$CONTROL_ROOM_DOCKER_CONTAINER" || return 1
    if [ "$(docker inspect -f '{{.State.Running}}' "$CONTROL_ROOM_DOCKER_CONTAINER" 2>/dev/null || true)" = true ]; then
        return 0
    fi
    configure_change_review
    ensure_simulator_image "$SIMULATOR_IMAGE" "$SCRIPT_DIR/Dockerfile.simulator" "$DASHBOARD_DIR"
    docker rm -f "$CONTROL_ROOM_DOCKER_CONTAINER" >/dev/null 2>&1 || true
    control_room_proxy_ip=$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')
    set --
    if [ -r "$OPERATOR_DIR/stack.json" ]; then
        sh "$SCRIPT_DIR/local-operator-login.sh" start
        operator_origin=$(jq -er '.origin' "$OPERATOR_DIR/stack.json")
        operator_organization=$(jq -er '.organization' "$OPERATOR_DIR/stack.json")
        set -- -v "$OPERATOR_DIR/room.crt:$OPERATOR_DIR/room.crt:ro" \
            -v "$OPERATOR_DIR/room.key:$OPERATOR_DIR/room.key:ro" \
            -v "$OPERATOR_DIR/oidc.json:$OPERATOR_DIR/oidc.json:ro" \
            -v "$OPERATOR_DIR/client.secret:$OPERATOR_DIR/client.secret:ro" \
            -v "$OPERATOR_DIR/ca.crt:$OPERATOR_DIR/ca.crt:ro" \
            -v "$OPERATOR_DIR/signer.json:$OPERATOR_DIR/signer.json:ro" \
            -v "$OPERATOR_DIR/delegation.key:$OPERATOR_DIR/delegation.key:ro" \
            --add-host zpr-id.localhost:host-gateway \
            -e ZPR_CONTROL_ROOM_ORIGIN="$operator_origin" \
            -e ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP="$control_room_proxy_ip" \
            -e ZPR_CONTROL_ROOM_ORGANIZATION_ID="$operator_organization" \
            -e ZPR_CONTROL_ROOM_CERT_FILE="$OPERATOR_DIR/room.crt" \
            -e ZPR_CONTROL_ROOM_KEY_FILE="$OPERATOR_DIR/room.key" \
            -e ZPR_OPERATOR_OIDC_CONFIG_FILE="$OPERATOR_DIR/oidc.json" \
            -e ZPR_OPERATOR_OIDC_SECRET_FILE="$OPERATOR_DIR/client.secret" \
            -e ZPR_OPERATOR_OIDC_CA_FILE="$OPERATOR_DIR/ca.crt" \
            -e ZPR_OPERATOR_DELEGATION_SIGNER_FILE="$OPERATOR_DIR/signer.json" \
            -e ZPR_OPERATOR_DELEGATION_KEY_FILE="$OPERATOR_DIR/delegation.key"
    fi
    if [ -n "$review_config" ]; then
        set -- "$@" --mount "type=bind,source=$review_config,target=$review_config,readonly" \
            --mount "type=bind,source=$review_directory,target=$review_directory" \
            -e ZPR_CHANGE_REVIEW_CONFIG_FILE="$review_config"
    fi
    docker run -d --name "$CONTROL_ROOM_DOCKER_CONTAINER" \
        --label zpr.control-room=true \
        --restart unless-stopped \
        -v "$SERVICE_CERTS:$SERVICE_CERTS:ro" \
        -p 127.0.0.1:8787:8787 \
        -e ZPR_CONTROL_SERVICE_URL=https://host.docker.internal:8790 \
        -e ZPR_CONTROL_SERVICE_TLS_SERVER_NAME=127.0.0.1 \
        -e ZPR_CONTROL_ROOM_PROXY_IP="$control_room_proxy_ip" \
        -e ZPR_CONTROL_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e ZPR_CONTROL_CLIENT_CERT_FILE="$SERVICE_CERTS/control-room-client.crt" \
        -e ZPR_CONTROL_CLIENT_KEY_FILE="$SERVICE_CERTS/control-room-client.key" \
        "$@" \
        --entrypoint /usr/local/bin/zpr-web-dashboard \
        "$SIMULATOR_IMAGE" -mode control-room -listen 0.0.0.0:8787 >/dev/null
    simulator_wait_for_control_room
}

simulator_wait_for_control_room() {
    if [ -r "$OPERATOR_DIR/stack.json" ]; then
        wait_for_url "$(jq -er '.origin' "$OPERATOR_DIR/stack.json")/auth/operator/config" control-room \
            --cacert "$OPERATOR_DIR/ca.crt"
    else
        wait_for_url http://127.0.0.1:8787/api/snapshot control-room
    fi
}

simulator_restart_control_room() {
    configure_change_review
    ensure_simulator_image "$SIMULATOR_IMAGE" "$SCRIPT_DIR/Dockerfile.simulator" "$DASHBOARD_DIR"
    if [ -r "$OPERATOR_DIR/stack.json" ]; then
        sh "$SCRIPT_DIR/local-operator-login.sh" start
    fi
    stop_control_room
    simulator_start_control_room
}


simulator_start_control_service() {
    simulator_require_lab_container "$CONTROL_CONTAINER" || return 1
    control_platform_services_json=$(control_platform_services) || return 1
    . "$SCRIPT_DIR/provider-managers.sh"
    control_provider_manager_urls=$(provider_manager_urls \
        "${ZPR_PROVIDER_MANAGER_URLS_FILE:-$STATE_DIR/provider-managers.json}" \
        "${ZPR_PROVIDER_MANAGER_URLS:-}") || return 1
    control_diagnostics_username=${ZPR_DIAGNOSTICS_USERNAME:-}
    if [ -z "$control_diagnostics_username" ] && [ -f "$STATE_DIR/diagnostics/query.username" ]; then
        control_diagnostics_username=$(cat "$STATE_DIR/diagnostics/query.username")
    fi
    control_diagnostics_network=${ZPR_DIAGNOSTICS_NETWORK:-$OBSERVABILITY_LOCAL_NETWORK}
    if [ -z "${ZPR_DIAGNOSTICS_NETWORK:-}" ] && [ -f "$STATE_DIR/diagnostics/query.network" ]; then
        control_diagnostics_network=$(cat "$STATE_DIR/diagnostics/query.network")
    fi
    if [ "$control_diagnostics_network" != "$OBSERVABILITY_LOCAL_NETWORK" ]; then
        docker network inspect "$control_diagnostics_network" >/dev/null
    fi
    control_node_names_file=${ZPR_NODE_DISPLAY_NAMES_FILE:-}
    if [ -z "$control_node_names_file" ] && [ -f "$STATE_DIR/node-display-names.json" ]; then
        control_node_names_file="$STATE_DIR/node-display-names.json"
    fi
    control_change_feeds_file=
    control_change_feeds_network=
    if [ "${ZPR_TRUSTED_CHANGE_FEEDS_DISABLED:-}" != 1 ]; then
        control_change_feeds_file=${ZPR_TRUSTED_CHANGE_FEEDS_FILE:-}
        if [ -z "$control_change_feeds_file" ] && [ -f "$STATE_DIR/trusted-change-feeds.json" ]; then
            control_change_feeds_file="$STATE_DIR/trusted-change-feeds.json"
        fi
        control_change_feeds_network=${ZPR_TRUSTED_CHANGE_FEEDS_NETWORK:-}
        if [ -z "$control_change_feeds_network" ] && [ -f "$STATE_DIR/trusted-change-feeds.network" ]; then
            control_change_feeds_network=$(cat "$STATE_DIR/trusted-change-feeds.network")
        fi
    fi
    if [ -n "$control_change_feeds_network" ]; then
        docker network inspect "$control_change_feeds_network" >/dev/null
    fi
    control_ldap_container=${ZPR_ASSERTION_LDAP_CONTAINER:-}
    control_gateway_organization=${ZPR_GATEWAY_ORGANIZATION_ID:-${SIMULATION_ORGANIZATION_ID:-}}
    active_organization_file=${ACTIVE_ORGANIZATION_FILE:-}
    if [ -z "$control_gateway_organization" ] && [ -n "$active_organization_file" ] && [ -r "$active_organization_file" ]; then
        control_gateway_organization=$(tr -d '\r\n' < "$active_organization_file")
    fi
    control_dns_transfer_key_file=${ZPR_DNS_TRANSFER_TSIG_KEY_FILE:-}
    if [ -z "$control_dns_transfer_key_file" ]; then
        control_dns_transfer_key_file=$DNS_VIEWER_KEY_FILE
        control_organization_profile="$ORGANIZATIONS_DIR/$control_gateway_organization.json"
        if [ -n "$control_gateway_organization" ] && [ -r "$control_organization_profile" ] &&
            [ "$(jq -er '.runtime.driver' "$control_organization_profile")" = docker-multinode ]; then
            control_dns_transfer_key_file="$RUNTIME_DIR/multinode/$control_gateway_organization/dns/zpr-dns-viewer.key"
        fi
    fi
    control_dns_stats_url=${ZPR_DNS_STATS_URL:-}
    control_dns_stats_network=
    control_dns_stats_config=${ZPR_DNS_STATS_RELAY_CONFIG_FILE:-$STATE_DIR/dns-stats-relay.json}
    if [ -z "$control_dns_stats_url" ] && [ -r "$control_dns_stats_config" ]; then
        sh "$SCRIPT_DIR/control-dns-stats.sh" start "$control_dns_stats_config"
        control_dns_stats_url=http://zpr-dns-stats-relay:8054
        control_dns_stats_network=zpr-dns-stats-read-only
    fi
    control_dns_stats_url=$(printf '%s' "$control_dns_stats_url" | sed 's#://127\.0\.0\.1:#://host.docker.internal:#')
    stop_control_service
    start_local_observability
    control_admin_url=${ZPR_ADMIN_URL:-https://127.0.0.1:$ADMIN_RELAY_PORT}
    control_admin_url=$(printf '%s' "$control_admin_url" | sed 's#://127\.0\.0\.1:#://host.docker.internal:#')
    set --
    operator_directory=${OPERATOR_DIR:-$RUNTIME_DIR/operator-login}
    if [ -r "$operator_directory/stack.json" ]; then
        set -- -e ZPR_ENROLLMENT_CONFIG_FILE="$operator_directory/enrollment.json" \
            -e ZPR_ENROLLMENT_DATABASE_FILE="$STATE_DIR/enrollment/registry.db" \
            -e ZPR_OPERATOR_DELEGATION_TRUST_FILE="$operator_directory/trust.json"
    fi
    docker run -d --name "$CONTROL_CONTAINER" \
        --label zpr.control-service=true \
        --restart unless-stopped \
        -v "$(docker_socket_path):/var/run/docker.sock" \
        -v "$RUNTIME_DIR:$RUNTIME_DIR" \
        -v "$DASHBOARD_DIR/../../zpr-demo/multinode-demo:$DASHBOARD_DIR/../../zpr-demo/multinode-demo:ro" \
        -p 127.0.0.1:8790:8790 \
        -e ANTHROPIC_API_KEY \
        -e ZPR_CONTROL_SERVICE_LISTEN=0.0.0.0:8790 \
        -e ZPR_GATEWAY_ORGANIZATION_ID="$control_gateway_organization" \
        -e ZPR_AUDIT_DB_PATH="$STATE_DIR/audit/audit.db" \
        -e ZPR_GATEWAY_CONFIG_STORE_DIR="${ZPR_GATEWAY_CONFIG_STORE_DIR:-$STATE_DIR/gateway-configs}" \
        -e ZPR_ANTHROPIC_API_KEY_FILE="${ZPR_ANTHROPIC_API_KEY_FILE:-$STATE_DIR/assistant/api-key}" \
        -e ZPR_CONTROL_SERVICE_CERT_FILE="$SERVICE_CERTS/control-service.crt" \
        -e ZPR_CONTROL_SERVICE_KEY_FILE="$SERVICE_CERTS/control-service.key" \
        -e ZPR_CONTROL_SERVICE_CLIENT_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e ZPR_ASSERTION_STORE_FILE="${ZPR_ASSERTION_STORE_FILE:-$STATE_DIR/assertions/global.json}" \
        -e ZPR_ASSERTION_LDAP_CONTAINER="$control_ldap_container" \
        -e ZPR_ASSERTION_LDAP_BASE_DN="${ZPR_ASSERTION_LDAP_BASE_DN:-}" \
        -e ZPR_ASSERTION_LDAP_BIND_DN="${ZPR_ASSERTION_LDAP_BIND_DN:-}" \
        -e ZPR_ASSERTION_SOURCES_FILE="${ZPR_ASSERTION_SOURCES_FILE:-}" \
        -e ZPR_TRUSTED_CHANGE_FEEDS_FILE="$control_change_feeds_file" \
        -e ZPR_NODE_DISPLAY_NAMES_FILE="$control_node_names_file" \
        -e ZPR_ADAPTER_LOG_CONFIG_FILE="${ZPR_ADAPTER_LOG_CONFIG_FILE:-$STATE_DIR/adapter-logs.json}" \
        -e ZPR_PROVIDER_MANAGER_URLS="$control_provider_manager_urls" \
        -e ZPR_DIAGNOSTICS_CONFIG_FILE="${ZPR_DIAGNOSTICS_CONFIG_FILE:-$STATE_DIR/diagnostics/openobserve.json}" \
        -e ZPR_DIAGNOSTICS_USERNAME="$control_diagnostics_username" \
        -e ZPR_DIAGNOSTICS_TOKEN_FILE="${ZPR_DIAGNOSTICS_TOKEN_FILE:-$STATE_DIR/diagnostics/query.token}" \
        -e ZPR_DIAGNOSTICS_SOURCE_MAP_FILE="${ZPR_DIAGNOSTICS_SOURCE_MAP_FILE:-$STATE_DIR/diagnostics/source-map.json}" \
        -e ZPR_PLATFORM_SERVICES="$control_platform_services_json" \
        -e ZPR_ADMIN_URL="$control_admin_url" \
        -e ZPR_ADMIN_CA_FILE="${ZPR_ADMIN_CA_FILE:-$RUNTIME_DIR/local-admin-cert.pem}" \
        -e ZPR_ADMIN_KEY_FILE="${ZPR_ADMIN_KEY_FILE:-$RUNTIME_DIR/admin-read.key}" \
        -e ZPR_DNS_STATS_URL="$control_dns_stats_url" \
        -e ZPR_DNS_TRANSFER_ADDR="host.docker.internal:$DNS_RECORDS_RELAY_PORT" \
        -e ZPR_DNS_TRANSFER_TSIG_KEY_FILE="$control_dns_transfer_key_file" \
        -e ZPR_POLICY_SERVICE_URL=https://host.docker.internal:8789 \
        -e ZPR_POLICY_SERVICE_TLS_SERVER_NAME=127.0.0.1 \
        -e ZPR_POLICY_SERVICE_CA_FILE="$SERVICE_CERTS/service-ca.crt" \
        -e ZPR_POLICY_CLIENT_CERT_FILE="$SERVICE_CERTS/control-policy-client.crt" \
        -e ZPR_POLICY_CLIENT_KEY_FILE="$SERVICE_CERTS/control-policy-client.key" \
        "$@" \
        --entrypoint /usr/local/bin/zpr-web-dashboard \
        "$SIMULATOR_IMAGE" -mode control-service >/dev/null
    if [ "$control_diagnostics_network" != "$OBSERVABILITY_LOCAL_NETWORK" ] || docker inspect "$OBSERVABILITY_LOCAL_CONTAINER" >/dev/null 2>&1; then
        docker network connect "$control_diagnostics_network" "$CONTROL_CONTAINER"
    fi
    if [ -n "$control_change_feeds_network" ] && [ "$control_change_feeds_network" != "$OBSERVABILITY_LOCAL_NETWORK" ]; then
        docker network connect "$control_change_feeds_network" "$CONTROL_CONTAINER"
    fi
    if [ -n "$control_dns_stats_network" ]; then
        docker network connect "$control_dns_stats_network" "$CONTROL_CONTAINER"
    fi
    wait_for_url https://127.0.0.1:8790/api/snapshot control-service \
        ${ZPR_DASHBOARD_CONTAINER_RUNTIME:+--connect-to 127.0.0.1:8790:host.docker.internal:8790} \
        --cacert "$SERVICE_CERTS/service-ca.crt" \
        --cert "$SERVICE_CERTS/control-room-client.crt" \
        --key "$SERVICE_CERTS/control-room-client.key"
}
