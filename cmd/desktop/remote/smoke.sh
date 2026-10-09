#!/usr/bin/env bash
set -euo pipefail
[[ -f /.dockerenv ]] || { echo 'Remote browser smoke test must run inside Docker' >&2; exit 1; }
install -m 600 /run/connection/id_ed25519 /tmp/remote-key
awk '{$1="remote"; print}' /run/connection/known_hosts > /tmp/known_hosts
ssh_args=(-i /tmp/remote-key -o UserKnownHostsFile=/tmp/known_hosts -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes)
ssh "${ssh_args[@]}" hive@remote cat /home/hive/connection.json > /tmp/connection.json
chmod 600 /tmp/connection.json
ssh "${ssh_args[@]}" -N -T -o ExitOnForwardFailure=yes -L 127.0.0.1:19001:127.0.0.1:19001 hive@remote &
tunnel_pid=$!
export HIVE_DESKTOP_DATA_DIR=/tmp/local-hive/data HIVE_DESKTOP_CONFIG_DIR=/tmp/local-hive/config
export HIVE_CONFIG=/tmp/local-hive/hive.yaml HIVE_DATA_DIR=/tmp/local-hive/engine
export HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE=feed HIVE_DESKTOP_HTTP_PORT=19002
export WAILS_SERVER_HOST=127.0.0.1 WAILS_SERVER_PORT=8080
mkdir -p /tmp/local-hive
printf 'version: "0.2.7"\ngit_path: git\n' > "$HIVE_CONFIG"
hive-desktop > /tmp/local-hive.log 2>&1 &
app_pid=$!
trap 'kill "$app_pid" "$tunnel_pid" 2>/dev/null || true; wait || true' EXIT
for attempt in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080 > /dev/null && curl -fsS http://127.0.0.1:19001/api/status > /dev/null; then break; fi
  sleep 1
done
export REMOTE_TUNNEL_PID="$tunnel_pid"
node /smoke/smoke.mjs
