#!/bin/sh

inherit_multinode_platform() {
    saved_platform_env=$(docker inspect "$1" --format '{{json .Config.Env}}') || return 1
    saved_platform_settings=$(printf '%s\n' "$saved_platform_env" | jq -er '
        map(select(test("^(ZPR_MULTINODE_(PLATFORM|IMAGE|DIRECTORY_IMAGE|DIRECTORY_MANAGER_IMAGE)|ZPR_TRUSTED_SERVICE_BINARY)=")))
        | if all(.[]; contains("\n") | not) then join("\n")
          else error("multi-node configuration contains a newline") end
    ') || return 1
    apply_multinode_platform_settings "$saved_platform_settings"
}

load_multinode_platform_file() {
    if [ ! -e "$1" ] && [ ! -L "$1" ]; then return 0; fi
    saved_platform_settings=$(jq -er '
        if type != "object" then error("multi-node configuration must be an object") else
          to_entries
          | if all(.[];
              (.key | test("^(ZPR_MULTINODE_(PLATFORM|IMAGE|DIRECTORY_IMAGE|DIRECTORY_MANAGER_IMAGE)|ZPR_TRUSTED_SERVICE_BINARY)$"))
              and (.value | type == "string")
              and (.value | contains("\n") | not))
            then map(.key + "=" + .value) | join("\n")
            else error("invalid multi-node configuration") end
        end
    ' "$1") || return 1
    apply_multinode_platform_settings "$saved_platform_settings"
}

apply_multinode_platform_settings() {
    while IFS= read -r saved_platform_setting; do
        case "$saved_platform_setting" in
            ZPR_MULTINODE_PLATFORM=*) ZPR_MULTINODE_PLATFORM=${ZPR_MULTINODE_PLATFORM:-${saved_platform_setting#*=}}; export ZPR_MULTINODE_PLATFORM ;;
            ZPR_MULTINODE_IMAGE=*) ZPR_MULTINODE_IMAGE=${ZPR_MULTINODE_IMAGE:-${saved_platform_setting#*=}}; export ZPR_MULTINODE_IMAGE ;;
            ZPR_MULTINODE_DIRECTORY_IMAGE=*) ZPR_MULTINODE_DIRECTORY_IMAGE=${ZPR_MULTINODE_DIRECTORY_IMAGE:-${saved_platform_setting#*=}}; export ZPR_MULTINODE_DIRECTORY_IMAGE ;;
            ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE=*) ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE=${ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE:-${saved_platform_setting#*=}}; export ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE ;;
            ZPR_TRUSTED_SERVICE_BINARY=*) ZPR_TRUSTED_SERVICE_BINARY=${ZPR_TRUSTED_SERVICE_BINARY:-${saved_platform_setting#*=}}; export ZPR_TRUSTED_SERVICE_BINARY ;;
        esac
    done <<EOF
$1
EOF
}

configure_multinode_platform() {
    ZPR_MULTINODE_PLATFORM=${ZPR_MULTINODE_PLATFORM:-linux/amd64}
    case "$ZPR_MULTINODE_PLATFORM" in
        linux/amd64) ZPR_MULTINODE_ARCH=amd64 ;;
        linux/arm64) ZPR_MULTINODE_ARCH=arm64 ;;
        *) echo "unsupported multi-node platform: $ZPR_MULTINODE_PLATFORM" >&2; return 1 ;;
    esac
    ZPR_MULTINODE_IMAGE=${ZPR_MULTINODE_IMAGE:-zpr-multinode}
    ZPR_MULTINODE_DIRECTORY_IMAGE=${ZPR_MULTINODE_DIRECTORY_IMAGE:-zpr-multinode-directory}
    ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE=${ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE:-zpr-multinode-directory-manager}
    export ZPR_MULTINODE_PLATFORM ZPR_MULTINODE_ARCH ZPR_MULTINODE_IMAGE
    export ZPR_MULTINODE_DIRECTORY_IMAGE ZPR_MULTINODE_DIRECTORY_MANAGER_IMAGE
}

verify_multinode_platform() {
    infrastructure_arch=$(docker image inspect "$ZPR_MULTINODE_IMAGE" --format '{{.Architecture}}') || return 1
    if [ "$infrastructure_arch" != "$ZPR_MULTINODE_ARCH" ]; then
        echo "multi-node image $ZPR_MULTINODE_IMAGE is $infrastructure_arch, expected $ZPR_MULTINODE_ARCH" >&2
        return 1
    fi
}

verify_multinode_binary() {
    binary_header=$(od -An -tu1 -N6 "$1" | awk '{$1=$1; print}') || return 1
    binary_machine=$(od -An -tu1 -j18 -N2 "$1" | awk '{$1=$1; print}') || return 1
    case "$ZPR_MULTINODE_ARCH" in
        arm64) expected_machine="183 0" ;;
        amd64) expected_machine="62 0" ;;
    esac
    if [ "$binary_header" != "127 69 76 70 2 1" ] || [ "$binary_machine" != "$expected_machine" ]; then
        echo "multi-node helper $1 is not a Linux $ZPR_MULTINODE_ARCH ELF executable" >&2
        return 1
    fi
}
