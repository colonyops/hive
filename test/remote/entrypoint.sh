#!/usr/bin/env bash
set -euo pipefail
if [[ ! -f /etc/ssh/keys/ssh_host_ed25519_key ]]; then
  ssh-keygen -q -t ed25519 -N '' -f /etc/ssh/keys/ssh_host_ed25519_key
fi
install -d -m 700 -o hive -g hive /home/hive/.ssh
install -m 600 -o hive -g hive /run/client.pub /home/hive/.ssh/authorized_keys
exec /usr/sbin/sshd -D -e
