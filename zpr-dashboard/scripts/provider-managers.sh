#!/bin/sh

provider_manager_urls() {
    manager_file=$1
    manager_overrides=${2:-'{}'}
    manager_registered='{}'
    if [ -f "$manager_file" ]; then
        manager_registered=$(cat "$manager_file") || return 1
    fi
    printf '%s\n%s\n' "$manager_registered" "$manager_overrides" |
        jq -ecs '
            if length == 2 and all(.[]; type == "object" and all(.[]; type == "string"))
            then .[0] * .[1]
            else error("provider manager configuration must contain source-name to URL objects")
            end'
}

register_provider_managers() {
    manager_file=$1
    manager_sources=$2
    manager_url=$3
    manager_registered=$(provider_manager_urls "$manager_file") || return 1
    mkdir -p "$(dirname "$manager_file")" || return 1
    manager_temp=$(mktemp "$manager_file.XXXXXX") || return 1
    if ! printf '%s\n' "$manager_registered" |
        jq --arg sources "$manager_sources" --arg url "$manager_url" \
            'reduce ($sources | split("\n")[] | select(length > 0)) as $source (. ; .[$source] = $url)' > "$manager_temp"; then
        rm -f "$manager_temp"
        return 1
    fi
    chmod 600 "$manager_temp" && mv "$manager_temp" "$manager_file"
}
