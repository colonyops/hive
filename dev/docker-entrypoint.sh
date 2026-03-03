#!/bin/bash

# Write Claude credentials if provided via environment variable
if [ -n "$CLAUDE_CREDENTIALS" ]; then
    mkdir -p /root/.claude
    echo "$CLAUDE_CREDENTIALS" > /root/.claude/.credentials.json
    echo "Claude credentials configured."
fi

# Full setup: install hive config pointing at /workspace
if [ "$CONTAINER_SETUP" = "full" ]; then
    mkdir -p /etc/hive
    cp /etc/hive/config.yaml.template /etc/hive/config.yaml
    export HIVE_CONFIG=/etc/hive/config.yaml
fi

exec "$@"
