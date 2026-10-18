#!/bin/sh

ensure_simulator_image() {
    image=$1
    dockerfile=$2
    context=$3

    case "${ZPR_SIMULATOR_PREBUILT_IMAGE:-0}" in
        0)
            docker build -f "$dockerfile" -t "$image" "$context"
            ;;
        1)
            if ! platform=$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image"); then
                echo "prebuilt Simulator image is not available locally: $image" >&2
                return 1
            fi
            if [ "$platform" != linux/amd64 ]; then
                echo "prebuilt Simulator image $image is $platform, expected linux/amd64" >&2
                return 1
            fi
            ;;
        *)
            echo "ZPR_SIMULATOR_PREBUILT_IMAGE must be 0 or 1" >&2
            return 2
            ;;
    esac
}
