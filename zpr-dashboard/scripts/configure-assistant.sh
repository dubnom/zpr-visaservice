#!/bin/zsh
set -eu

if [[ ! -t 0 ]]; then
    print -u2 'Enter the Anthropic API key directly in an interactive terminal.'
    exit 1
fi

read -rs 'ANTHROPIC_API_KEY?Anthropic API key: ' || { print; exit 1; }
print
if [[ -z "$ANTHROPIC_API_KEY" ]]; then
    print -u2 'Anthropic API key cannot be empty.'
    exit 1
fi

runtime_dir="${0:A:h:h:h:h}/.local-runtime"
assistant_dir="$runtime_dir/dashboard-stack/assistant"
umask 077
mkdir -m 700 -p "$assistant_dir"
chmod 700 "$assistant_dir"
key_file="$assistant_dir/api-key"
temporary_file=$(mktemp "$assistant_dir/api-key.XXXXXX")
trap 'unset ANTHROPIC_API_KEY; rm -f -- "$temporary_file"' EXIT
printf '%s\n' "$ANTHROPIC_API_KEY" > "$temporary_file"
chmod 600 "$temporary_file"
mv -f -- "$temporary_file" "$key_file"
unset ANTHROPIC_API_KEY
sh "${0:A:h}/dashboard-stack.sh" reload-assistant