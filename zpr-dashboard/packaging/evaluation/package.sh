#!/bin/sh
# Build an offline engineering evaluation archive from preloaded Linux images.
set -eu
umask 077

fail() { printf 'evaluation-package: %s\n' "$*" >&2; exit 1; }
[ "$#" -eq 2 ] || fail "usage: sh packaging/evaluation/package.sh RELEASE_ID NEW_ABSOLUTE_OUTPUT_DIRECTORY"
release=$1
output=$2
case "$release" in ""|*[!A-Za-z0-9._-]*) fail "invalid release ID" ;; esac
[ "${#release}" -le 80 ] || fail "release ID exceeds 80 characters"
case "$output" in /*) ;; *) fail "output must be absolute" ;; esac
[ ! -e "$output" ] && [ ! -L "$output" ] || fail "output already exists"
parent=$(dirname "$output")
[ -d "$parent" ] || fail "output parent must exist"
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$root"

command -v go >/dev/null || fail "Go is required on the build host"
command -v docker >/dev/null || fail "Docker is required on the build host"
command -v sha256sum >/dev/null || fail "sha256sum is required on the build host"
command -v git >/dev/null || fail "Git is required for build provenance"
mkdir -m 700 "$output"
package=$output/zpr-evaluation-$release
mkdir -m 700 "$package" "$package/bin" "$package/images"
printf 'INCOMPLETE: do not distribute until package.sh reports success\n' > "$output/INCOMPLETE"

printf 'Building Linux/amd64 bootstrap and production operator services...\n'
GOTOOLCHAIN=local GOPROXY=off GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0 \
    go build -mod=readonly -trimpath -buildvcs=false -o "$package/bin/zpr-lab-bootstrap" ./cmd/zpr-lab-bootstrap
GOTOOLCHAIN=local GOPROXY=off GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0 \
    go build -mod=readonly -trimpath -buildvcs=false -o "$package/bin/zpr-evaluation-supervisor" ./cmd/zpr-evaluation-supervisor
for command in zpr-enrollment-service zpr-enrollment-bas zpr-enrollment-device; do
    GOTOOLCHAIN=local GOPROXY=off GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0 \
        go build -mod=readonly -trimpath -buildvcs=false -o "$package/bin/$command" "./cmd/$command"
done
GOTOOLCHAIN=local GOPROXY=off GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0 \
    go build -mod=readonly -trimpath -buildvcs=false -tags production \
    -o "$package/bin/zpr-control-room" ./cmd/zpr-web-dashboard
companion=$output/zpr-simulator-$release
mkdir -m 700 "$companion" "$companion/bin"
GOTOOLCHAIN=local GOPROXY=off GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0 \
    go build -mod=readonly -trimpath -buildvcs=false \
    -o "$companion/bin/zpr-simulator" ./cmd/zpr-web-dashboard
cp packaging/evaluation/simulator-installed.sh "$companion/simulator.sh"
sh packaging/evaluation/stage-catalog.sh "$companion/catalog"
chmod 700 "$companion/simulator.sh"

save_image() {
    role=$1 reference=$2
    metadata=$(docker image inspect --format '{{.Os}}/{{.Architecture}} {{.Id}}' "$reference") ||
        fail "load the $role image before building; images are never pulled"
    case "$metadata" in "linux/amd64 sha256:"*) ;; *) fail "$role must be Linux/amd64" ;; esac
    id=${metadata#linux/amd64 }
    digest=${id#sha256:}
    case "$digest" in ""|*[!0-9a-f]*) fail "invalid immutable image ID for $role" ;; esac
    [ "${#digest}" -eq 64 ] || fail "invalid immutable image ID for $role"
    printf '%s %s\n' "$role" "$id" >> "$package/images.txt"
    docker save --output "$package/images/$role.tar" "$id"
}
save_image data-plane "${ZPR_EVALUATION_PACKAGE_DATA_PLANE_IMAGE:-zpr-evaluation-data-plane:local}"
save_image ldap "${ZPR_EVALUATION_PACKAGE_LDAP_IMAGE:-osixia/openldap@sha256:0de3850635911109f0daf8f5957808ec4f4eb827fbaaa339100b5f1bab925745}"
save_image ldap-admin-ui "${ZPR_EVALUATION_PACKAGE_LDAP_ADMIN_UI_IMAGE:-zpr-ldap-ui:phpldapadmin-1.2.6.7-4}"
save_image logging "${ZPR_EVALUATION_PACKAGE_LOGGING_IMAGE:-public.ecr.aws/zinclabs/openobserve@sha256:d4a878fac1f6c56003764f7f2a1625668917388f167e222c8c810de3f54c56ba}"
save_image oidc "${ZPR_EVALUATION_PACKAGE_OIDC_IMAGE:-ghcr.io/dexidp/dex@sha256:5d0656fce7d453c0e3b2706abf40c0d0ce5b371fb0b73b3cf714d05f35fa5f86}"
cp packaging/evaluation/installed.sh "$package/demo.sh"
cp packaging/evaluation/data-plane/client.sh "$package/client.sh"
cp packaging/evaluation/README.md "$package/README.md"
chmod 700 "$package/demo.sh" "$package/client.sh"
{
    printf 'kind=zpr-evaluation-offline\nstatus=engineering-unqualified\nrelease=%s\nplatform=linux/amd64\n' "$release"
    printf 'source_revision=%s\n' "$(git rev-parse HEAD)"
    printf 'source_dirty='
    if [ -z "$(git status --porcelain --untracked-files=normal)" ]; then printf 'false\n'; else printf 'true\n'; fi
    go version
} > "$package/BUILD-INFO.txt"
cp "$package/BUILD-INFO.txt" "$companion/BUILD-INFO.txt"
cp packaging/evaluation/README.md "$companion/README.md"
cp "$package/images.txt" "$companion/images.txt"
(
    cd "$companion"
    find catalog -type f -print | LC_ALL=C sort | xargs sha256sum > SHA256SUMS
    sha256sum BUILD-INFO.txt README.md simulator.sh images.txt bin/* >> SHA256SUMS
    sha256sum --check --strict SHA256SUMS
)
(
    cd "$package"
    sha256sum BUILD-INFO.txt README.md demo.sh client.sh images.txt bin/* images/*.tar > SHA256SUMS
    sha256sum --check --strict SHA256SUMS
)
archive=zpr-evaluation-$release-linux-amd64.tar.gz
for name in "zpr-evaluation-$release" "zpr-simulator-$release"; do
    archive=$name-linux-amd64.tar.gz
    case "$(uname -s)" in
        Darwin) COPYFILE_DISABLE=1 tar --no-xattrs -czf "$output/$archive" -C "$output" "$name" ;;
        *) tar -czf "$output/$archive" -C "$output" "$name" ;;
    esac
    (cd "$output" && sha256sum "$archive" > "$archive.sha256")
done
rm "$output/INCOMPLETE"
printf 'Built matching ZPR and Simulator archives in %s\nEngineering endpoint-traffic demonstration; full scenario acceptance remains separate.\n' "$output"
