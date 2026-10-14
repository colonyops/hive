#!/usr/bin/env bash
set -euo pipefail
mkdir -p /etc/ssh/keys /home/hive/.ssh
if [[ ! -f /etc/ssh/keys/ssh_host_ed25519_key ]]; then
  ssh-keygen -q -t ed25519 -N '' -f /etc/ssh/keys/ssh_host_ed25519_key
fi
install -m 600 -o hive -g hive /run/client.pub /home/hive/.ssh/authorized_keys
chmod 700 /home/hive/.ssh
chown -R hive:hive /home/hive
/usr/sbin/sshd -D -e &
ssh_pid=$!
runuser -u hive -- /usr/local/bin/start-hive.sh &
app_pid=$!
trap 'kill "$app_pid" "$ssh_pid" 2>/dev/null || true; wait || true' EXIT
trap 'exit 0' TERM INT
wait -n "$app_pid" "$ssh_pid"
