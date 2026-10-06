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

export ANTHROPIC_API_KEY
trap 'unset ANTHROPIC_API_KEY' EXIT
sh "${0:A:h}/dashboard-stack.sh" restart-control-service