#!/bin/bash

# Write Claude credentials if provided via environment variable
if [ -n "$CLAUDE_CREDENTIALS" ]; then
    mkdir -p "$HOME/.claude"
    echo "$CLAUDE_CREDENTIALS" > "$HOME/.claude/.credentials.json"
    echo "Claude credentials configured."
fi

exec "$@"
