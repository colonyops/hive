#!/bin/bash

# Write Claude credentials if provided via environment variable
if [ -n "$CLAUDE_CREDENTIALS" ]; then
    mkdir -p /root/.claude
    echo "$CLAUDE_CREDENTIALS" > /root/.claude/.credentials.json
    echo "Claude credentials configured."
fi

exec "$@"
