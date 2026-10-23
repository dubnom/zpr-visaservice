#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
activation="$script_dir/activate-organization.sh"
cases=0

check_blocked() {
    mode=$1
    shift
    # No external command (including dirname, Docker, curl or jq) is available.
    # Passing with an empty PATH proves the guard precedes helper sourcing.
    if output=$(PATH= SIMULATION_SETUP_MODE="$mode" /bin/sh "$activation" "$@" 2>&1); then
        status=0
    else
        status=$?
    fi
    [ "$status" -eq 1 ] || { printf 'expected blocked exit 1, got %s: %s\n' "$status" "$output" >&2; exit 1; }
    for blocker in ACTIVATION ASSERTIONS SOURCES POLICY ADMISSION CONFIG RESTORE RECONCILE; do
        case "$output" in
            *"U01-$blocker:"*) ;;
            *) printf 'missing blocker %s: %s\n' "$blocker" "$output" >&2; exit 1 ;;
        esac
    done
    case "$output" in
        *'No setup, source probe, lock, runtime read or mutation was attempted.') ;;
        *) printf 'missing no-side-effects result: %s\n' "$output" >&2; exit 1 ;;
    esac
    case "$output" in
        *'not found'*|*'cannot open'*|*'No such file'*) printf 'guard reached a helper: %s\n' "$output" >&2; exit 1 ;;
    esac
    cases=$((cases + 1))
}

check_invalid() {
    mode=$1
    expected=$2
    shift 2
    if output=$(PATH= SIMULATION_SETUP_MODE="$mode" /bin/sh "$activation" "$@" 2>&1); then
        status=0
    else
        status=$?
    fi
    [ "$status" -eq 2 ] || { printf 'expected usage exit 2, got %s: %s\n' "$status" "$output" >&2; exit 1; }
    case "$output" in
        *"$expected"*) ;;
        *) printf 'unexpected validation result: %s\n' "$output" >&2; exit 1 ;;
    esac
    case "$output" in
        *'not found'*|*'cannot open'*|*'No such file'*) printf 'validation reached a helper: %s\n' "$output" >&2; exit 1 ;;
    esac
    cases=$((cases + 1))
}

for mode in '' legacy-launcher ordinary-client; do
    check_blocked "$mode" preflight-ordinary-client
    check_invalid "$mode" 'usage:' preflight-ordinary-client redwood
    check_invalid "$mode" 'usage:' preflight-ordinary-client redwood extra
done
for action in reset-organization restore-base; do
    check_blocked ordinary-client "$action" redwood
    check_blocked ordinary-client "$action" '../invalid'
done
check_blocked ordinary-client
check_blocked ordinary-client unknown-action
check_invalid typo 'invalid SIMULATION_SETUP_MODE' reset-organization redwood
check_invalid typo 'invalid SIMULATION_SETUP_MODE' preflight-ordinary-client
printf 'ordinary-client preflight: %s cases passed (no runtime dependencies or mutations)\n' "$cases"
