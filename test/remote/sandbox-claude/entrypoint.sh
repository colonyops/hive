#!/usr/bin/env bash
set -euo pipefail
# A bind mount keeps the host's owner, which the hive user cannot read.
install -d -m 700 -o hive -g hive /home/hive/.config
install -m 600 -o hive -g hive /run/claude-token /home/hive/.config/claude-token
exec /usr/local/bin/entrypoint.sh
