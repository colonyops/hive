#!/usr/bin/env bash
set -euo pipefail
# run.sh owns the host key, so a recreated container keeps the key that
# known_hosts trusts.
install -m 600 -o root -g root /run/host_key /etc/ssh/keys/ssh_host_ed25519_key
install -d -m 700 -o hive -g hive /home/hive/.ssh
install -m 600 -o hive -g hive /run/client.pub /home/hive/.ssh/authorized_keys
exec /usr/sbin/sshd -D -e
