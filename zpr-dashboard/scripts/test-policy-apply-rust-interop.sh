#!/bin/bash
set -euo pipefail

expected_revision=2756e9a01d5552965439f94314a6b0550832cf8f
expected_version=0.25.2
tests='^(TestPolicyApplyRustInterop|TestPolicyApplyRustPersistentRecoveryInterop)$'

usage() {
    cat <<EOF
Usage: bash zpr-dashboard/scripts/test-policy-apply-rust-interop.sh

Runs exactly TestPolicyApplyRustInterop and
TestPolicyApplyRustPersistentRecoveryInterop with Go race detection.
Requires cached Go/Cargo dependencies, Go with CGo/race support, Rust,
capnp, a C compiler, and /opt/homebrew/bin/valkey-server.
No dependencies are fetched or installed.

Requires the clean canonical zpr-common checkout beside the main Git worktree:
  revision: $expected_revision
  package zpr version: $expected_version
An absent Cargo sibling zpr-common path is temporarily symlinked to that
checkout; cleanup removes only the unchanged symlink created by this runner.
Existing paths are never replaced.

Command (from zpr-dashboard):
  GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOFLAGS=-mod=readonly \\
  CGO_ENABLED=1 CARGO_NET_OFFLINE=true RUSTUP_AUTO_INSTALL=0 \\
  ZPR_RUST_POLICY_INTEROP=1 ZPR_RUST_POLICY_RECOVERY_INTEROP=1 \\
  go test -race ./cmd/zpr-web-dashboard -run '$tests' -count=1 -v -timeout=15m

Limits: owned loopback Rust receiver, Policy-Service HTTPS and private Valkey
fixtures only. Operator authorization uses a test-only authorizer, not an
authenticated browser/OIDC session. No adapter packet accept/deny is tested.
This is not whole-system, fresh-host, production, or power-loss acceptance.
No live services, credentials, Keychain, publication, or deployment are used.
EOF
}

case "${1-}" in
    -h|--help) usage; exit 0 ;;
    "") ;;
    *) usage >&2; exit 2 ;;
esac
if [ "$#" -ne 0 ]; then
    usage >&2
    exit 2
fi

fail() {
    printf 'ERROR: %s\n' "$*" >&2
    exit 1
}

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
dashboard_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd -P)
root=$(CDPATH='' cd -- "$dashboard_dir/.." && pwd -P)
common_git_dir=$(git -C "$root" rev-parse --path-format=absolute --git-common-dir)
main_root=$(CDPATH='' cd -- "$common_git_dir/.." && pwd -P)
canonical_parent=$(CDPATH='' cd -- "$main_root/.." && pwd -P)
canonical_dependency="$canonical_parent/zpr-common"
dependency_parent=$(CDPATH='' cd -- "$root/.." && pwd -P)
dependency_path="$dependency_parent/zpr-common"

[ -d "$canonical_dependency" ] || fail "canonical local dependency missing: $canonical_dependency"
[ ! -L "$canonical_dependency" ] || fail "canonical dependency must be a checkout, not a symlink"
canonical_dependency=$(CDPATH='' cd -- "$canonical_dependency" && pwd -P)
dependency_root=$(git -C "$canonical_dependency" rev-parse --show-toplevel)
[ "$dependency_root" = "$canonical_dependency" ] || fail "dependency is not its own Git checkout"
revision=$(git -C "$canonical_dependency" rev-parse HEAD)
[ "$revision" = "$expected_revision" ] || fail "zpr-common revision $revision differs from $expected_revision"
[ -z "$(git -C "$canonical_dependency" status --porcelain --untracked-files=all)" ] ||
    fail "canonical zpr-common must be clean"
version=$(awk '/^\[package\]$/ { package_section=1; next }
    /^\[/ { package_section=0 }
    package_section && /^version = / { gsub(/"/, "", $3); print $3; exit }' "$canonical_dependency/Cargo.toml")
[ "$version" = "$expected_version" ] || fail "zpr-common package version is $version, expected $expected_version"

for tool in go cargo capnp cc tee; do
    command -v "$tool" >/dev/null || fail "required local tool missing: $tool"
done
[ -x /opt/homebrew/bin/valkey-server ] || fail "fixture requires /opt/homebrew/bin/valkey-server"
valkey_version=$(/opt/homebrew/bin/valkey-server --version)
case "$valkey_version" in
    *"Valkey server"*) ;;
    *) fail "unexpected Valkey binary: $valkey_version" ;;
esac

created_link_inode=
log=
link_inode() {
    case "$(uname -s)" in
        Darwin) stat -f '%d:%i' "$dependency_path" ;;
        *) stat -c '%d:%i' "$dependency_path" ;;
    esac
}

cleanup() {
    result=$?
    trap - EXIT
    if [ -n "$created_link_inode" ]; then
        if [ -L "$dependency_path" ] &&
            [ "$(readlink "$dependency_path")" = "$canonical_dependency" ] &&
            [ "$(link_inode)" = "$created_link_inode" ]; then
            rm -- "$dependency_path" || result=1
        else
            printf 'ERROR: dependency link changed; refusing to remove it: %s\n' "$dependency_path" >&2
            result=1
        fi
    fi
    if [ -n "$log" ]; then
        rm -f -- "$log" || result=1
    fi
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

if [ -e "$dependency_path" ] || [ -L "$dependency_path" ]; then
    resolved_dependency=$(CDPATH='' cd -- "$dependency_path" && pwd -P) ||
        fail "existing Cargo dependency path is not a directory: $dependency_path"
    [ "$resolved_dependency" = "$canonical_dependency" ] ||
        fail "existing Cargo dependency path is not the canonical checkout: $dependency_path"
else
    ln -s -- "$canonical_dependency" "$dependency_path"
    created_link_inode=$(link_inode)
fi

usage
printf '\nVerified dependency: %s\nRevision: %s\nVersion: %s\n%s\n' \
    "$canonical_dependency" "$revision" "$version" "$valkey_version"
log=$(mktemp "${TMPDIR:-/tmp}/zpr-policy-interop-run.XXXXXX")
cd -- "$dashboard_dir"
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOFLAGS=-mod=readonly \
    CGO_ENABLED=1 CARGO_NET_OFFLINE=true RUSTUP_AUTO_INSTALL=0 \
    ZPR_RUST_POLICY_INTEROP=1 ZPR_RUST_POLICY_RECOVERY_INTEROP=1 \
    go test -race ./cmd/zpr-web-dashboard -run "$tests" -count=1 -v -timeout=15m 2>&1 | tee "$log"
for test_name in TestPolicyApplyRustInterop TestPolicyApplyRustPersistentRecoveryInterop; do
    grep -Eq "^--- PASS: $test_name " "$log" || fail "expected test did not pass: $test_name"
done
if grep -q '^--- SKIP:' "$log"; then
    fail "acceptance tests must execute, not skip"
fi
printf '\nPASS: both owned-fixture race tests executed; operator/browser and packet-path gaps remain as stated above.\n'
