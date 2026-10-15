#!/bin/sh

business_workload_names() {
    printf '%s\n' support-desk-app engineering-build-farm finance-workspace shipping-portal receiving-portal assembly-mes quality-test-bench
}

prepare_business_keys() {
    business_key_directory=$1
    mkdir -p "$business_key_directory" || return 1
    chmod 700 "$business_key_directory" || return 1
    for business_name in $(business_workload_names); do
        if [ ! -f "$business_key_directory/$business_name.key" ]; then
            business_key_temp=$(mktemp "$business_key_directory/$business_name.XXXXXX") || return 1
            if ! openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$business_key_temp" 2>/dev/null; then
                rm -f "$business_key_temp"
                echo "unable to generate business simulation key: $business_name" >&2
                return 1
            fi
            chmod 600 "$business_key_temp" && mv "$business_key_temp" "$business_key_directory/$business_name.key" || return 1
        fi
        chmod 600 "$business_key_directory/$business_name.key" || return 1
        openssl pkey -in "$business_key_directory/$business_name.key" -pubout \
            -out "$business_key_directory/$business_name.pub.pem" || return 1
    done
}
