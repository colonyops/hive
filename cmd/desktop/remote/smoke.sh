#!/usr/bin/env bash
set -euo pipefail
[[ -f /.dockerenv ]] || { echo 'Remote browser smoke test must run inside Docker' >&2; exit 1; }
install -m 600 /run/connection/id_ed25519 /tmp/remote-key
awk '{$1="remote"; print}' /run/connection/known_hosts > /tmp/known_hosts
ssh_args=(-i /tmp/remote-key -o UserKnownHostsFile=/tmp/known_hosts -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes)
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
cat > "$HOME/.ssh/config" <<'CONFIG'
Host hive-fixture
  HostName remote
  User hive
  IdentityFile /tmp/remote-key
  UserKnownHostsFile /tmp/known_hosts
  IdentitiesOnly yes
CONFIG
chmod 600 "$HOME/.ssh/config"
export HIVE_DESKTOP_DATA_DIR=/tmp/local-hive/data HIVE_DESKTOP_CONFIG_DIR=/tmp/local-hive/config
export HIVE_CONFIG=/tmp/local-hive/hive.yaml HIVE_DATA_DIR=/tmp/local-hive/engine
export HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE=feed HIVE_DESKTOP_HTTP_PORT=19002
export WAILS_SERVER_HOST=127.0.0.1 WAILS_SERVER_PORT=8080
mkdir -p /tmp/local-hive
printf 'version: "0.2.7"\ngit_path: git\n' > "$HIVE_CONFIG"
hive-desktop > /tmp/local-hive.log 2>&1 &
app_pid=$!
trap 'kill "$app_pid" 2>/dev/null || true; wait || true' EXIT
for attempt in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080 > /dev/null; then break; fi
  sleep 1
done
node /smoke/smoke.mjs
