#!/bin/sh

configure_activation_trusted_feeds() {
    trusted_change_feeds_disabled=${ZPR_TRUSTED_CHANGE_FEEDS_DISABLED:-}
    trusted_change_feeds_network=${ZPR_TRUSTED_CHANGE_FEEDS_NETWORK:-}
    if [ -z "$trusted_change_feeds_network" ] &&
       [ -r "$STATE_DIR/trusted-change-feeds.network" ]; then
        trusted_change_feeds_network=$(tr -d '\r\n' < "$STATE_DIR/trusted-change-feeds.network")
    fi
    case "$trusted_change_feeds_network" in
        zpr-*)
            trusted_change_feeds_organization=${trusted_change_feeds_network#zpr-}
            if [ -r "$ORGANIZATIONS_DIR/$trusted_change_feeds_organization.json" ] &&
               [ "$trusted_change_feeds_organization" != "$1" ]; then
                trusted_change_feeds_disabled=1
                echo "Omitting trusted feed scoped to $trusted_change_feeds_organization for $1" >&2
            fi
            ;;
    esac
}

acquire_machine_runtime_lock() {
    machine_runtime_lock="$STATE_DIR/machine-runtime.lock"
    machine_runtime_lock_owned=no
    if [ -n "${SIMULATION_RUNTIME_LOCK_TOKEN:-}" ] &&
       [ -r "$machine_runtime_lock/token" ] &&
       [ "$(cat "$machine_runtime_lock/token")" = "$SIMULATION_RUNTIME_LOCK_TOKEN" ]; then
        return 0
    fi
    if ! mkdir "$machine_runtime_lock" 2>/dev/null; then
        echo "Simulator runtime mutation is locked; finish the current operation or inspect $machine_runtime_lock before recovery" >&2
        return 1
    fi
    machine_runtime_lock_owned=yes
    SIMULATION_RUNTIME_LOCK_TOKEN=$(openssl rand -hex 16) || { rmdir "$machine_runtime_lock"; return 1; }
    printf '%s\n' "$SIMULATION_RUNTIME_LOCK_TOKEN" > "$machine_runtime_lock/token"
    printf '%s\n' "$$" > "$machine_runtime_lock/pid"
    export SIMULATION_RUNTIME_LOCK_TOKEN
}

release_machine_runtime_lock() {
    if [ "${machine_runtime_lock_owned:-no}" = yes ]; then
        rm -f "$machine_runtime_lock/token" "$machine_runtime_lock/pid"
        rmdir "$machine_runtime_lock"
    fi
}

resolve_machine_runtime() {
    runtime_request="$RUNTIME_DIR/organization-request.json"
    machine_organization=${SIMULATION_ORGANIZATION_ID:-}
    machine_generation=startup
    if [ -r "$runtime_request" ]; then
        request_organization=$(jq -er '.organization_id' "$runtime_request") || return 1
        machine_generation=$(jq -er '.generation' "$runtime_request") || return 1
        if [ -n "${SIMULATION_EXPECTED_ORGANIZATION:-}" ] && [ "$SIMULATION_EXPECTED_ORGANIZATION" != "$request_organization" ]; then
            echo "machine runtime organization does not match activation request" >&2
            return 1
        fi
        machine_organization=$request_organization
    elif [ -z "$machine_organization" ] && [ -r "$ACTIVE_ORGANIZATION_FILE" ]; then
        machine_organization=$(tr -d '\r\n' < "$ACTIVE_ORGANIZATION_FILE")
    fi
    if [ -z "$machine_organization" ]; then
        machine_organization=$(jq -er '.organization_id' "$SIMULATION_MANIFEST") || return 1
    fi
    case "$machine_organization" in ''|*[!a-z0-9-]*) echo "invalid machine runtime organization" >&2; return 1 ;; esac
    case "$machine_generation" in ''|*[!a-z0-9-]*) echo "invalid machine runtime generation" >&2; return 1 ;; esac
    [ "${#machine_generation}" -le 128 ] || { echo "machine runtime generation is too long" >&2; return 1; }
    machine_profile="$ORGANIZATIONS_DIR/$machine_organization.json"
    machine_runtime_driver=$(jq -er '.runtime.driver' "$machine_profile") || return 1
    MACHINE_WORKLOAD_DIR="$STATE_DIR/machine-runtimes/$machine_organization/$machine_generation"
}

machine_docker_network_for_org() {
    machine_network_organization=$1
    machine_network_name="zpr-$machine_network_organization"
    if [ "$machine_network_organization" = northstar ] &&
       [ -n "${SIMULATION_NORTHSTAR_DOCKER_NETWORK:-}" ]; then
        machine_network_name=$SIMULATION_NORTHSTAR_DOCKER_NETWORK
    fi
    case "$machine_network_name" in
        zpr-[a-zA-Z0-9]*) ;;
        *) echo "invalid multi-node Docker network name" >&2; return 1 ;;
    esac
    case "$machine_network_name" in
        *[!a-zA-Z0-9_.-]*) echo "invalid multi-node Docker network name" >&2; return 1 ;;
    esac
    printf '%s\n' "$machine_network_name"
}

resolve_machine_placement() {
    machine_owner=$(jq -r --arg id "$machine" '.machine_owners[$id][0] // empty' "$machine_profile") || return 1
    if [ -n "$machine_owner" ]; then
        machine_location=$(jq -er --arg owner "$machine_owner" '
            [.directory.people[] | select(.uid == $owner) | .location][0] |
            if type == "string" and length > 0 then . else error("machine owner has no organization location") end
        ' "$machine_profile") || return 1
    else
        machine_owner=$(jq -r --arg id "$machine" '.machines[] | select(.id == $id) | .owner // ""' "$SIMULATION_MANIFEST") || return 1
        machine_location=$(jq -er --arg id "$machine" '.machines[] | select(.id == $id) | .location' "$SIMULATION_MANIFEST") || return 1
        if [ "$machine_runtime_driver" = docker-multinode ]; then
            machine_location=$(jq -er '
                if .runtime.topology == "single-node" and (.runtime.nodes | length) == 1 then
                    .runtime.nodes[0].location
                else error("multi-node machine requires a configured organization owner") end
            ' "$machine_profile") || return 1
        fi
    fi
    if [ "$machine_runtime_driver" = docker-multinode ]; then
        machine_node_index=$(jq -er --arg location "$machine_location" '
            [.runtime.nodes | to_entries[] | select(.value.location == $location) | .key] |
            if length == 1 then .[0] else error("machine location must match exactly one configured node") end
        ' "$machine_profile") || return 1
        machine_node_ip=$(jq -er --argjson index "$machine_node_index" '.runtime.nodes[$index].substrate_address' "$machine_profile") || return 1
    fi
}

select_machine_readiness_probes() {
    readiness_candidates=$(jq -er --slurpfile manifest "$SIMULATION_MANIFEST" '
        (if .machine_owners == null then {} else .machine_owners end) as $owners |
        (if ($owners | type) != "object" then error("invalid organization machine owners")
        elif ($owners | length) > 0 then $owners | keys
        else $manifest[0].machines | map(.id) end) |
        if length == 0 then error("no readiness machine candidates")
        elif all(.[]; type == "string" and test("^machine-[0-9]{2}$")) then sort[]
        else error("invalid readiness machine id") end
    ' "$machine_profile") || return 1
    readiness_node_count=$(jq -er '
        .runtime.nodes | if type == "array" and length > 0 then length
        else error("organization has no configured nodes") end
    ' "$machine_profile") || return 1
    readiness_placements=$(
        for machine in $readiness_candidates; do
            resolve_machine_placement || return 1
            printf '%s %s\n' "$machine_node_index" "$machine"
        done
    ) || return 1
    readiness_index=0
    while [ "$readiness_index" -lt "$readiness_node_count" ]; do
        readiness_machine=$(printf '%s\n' "$readiness_placements" |
            awk -v node_index="$readiness_index" '$1 == node_index { print $2; exit }')
        if [ -z "$readiness_machine" ]; then
            echo "no readiness machine for configured node $readiness_index" >&2
            return 1
        fi
        printf '%s\n' "$readiness_machine"
        readiness_index=$((readiness_index + 1))
    done
}

stop_runtime_machines() {
    runtime_machines=$(docker ps --filter label=zpr.machine.id --filter status=running -q) || return 1
    for runtime_machine in $runtime_machines; do
        docker stop --time 5 "$runtime_machine" >/dev/null || return 1
        runtime_running=$(docker inspect -f '{{.State.Running}}' "$runtime_machine") || return 1
        if [ "$runtime_running" != false ]; then
            echo "machine $runtime_machine is still running; refusing organization switch" >&2
            return 1
        fi
    done
}
