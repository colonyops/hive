#!/usr/bin/env bash
set -euo pipefail
# ssh refuses a key the bind mount leaves with the wrong owner or mode.
state=/run/smoke/state
install -d -m 700 -o hive -g hive /home/hive/.ssh
install -m 600 -o hive -g hive "$state/id_ed25519" /home/hive/.ssh/id_ed25519
install -m 644 -o hive -g hive "$state/known_hosts" /home/hive/.ssh/known_hosts
exec sleep infinity
