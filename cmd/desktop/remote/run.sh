#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../../.."
repo_root="$PWD"
export HIVE_REMOTE_STATE="${HIVE_REMOTE_STATE:-$repo_root/.hive-remote}"
mkdir -p "$HIVE_REMOTE_STATE"
chmod 700 "$HIVE_REMOTE_STATE"
if [[ ! -f "$HIVE_REMOTE_STATE/id_ed25519" ]]; then
  ssh-keygen -q -t ed25519 -N '' -f "$HIVE_REMOTE_STATE/id_ed25519"
fi
project="hive-remote-$(printf '%s' "$repo_root" | cksum | awk '{print $1}')"
compose=(docker compose -p "$project" -f cmd/desktop/remote/compose.yaml)
ssh_port="${HIVE_REMOTE_SSH_PORT:-19222}"
local_port="${HIVE_REMOTE_LOCAL_PORT:-19001}"
ssh_args=(-i "$HIVE_REMOTE_STATE/id_ed25519" -p "$ssh_port" -o "UserKnownHostsFile=$HIVE_REMOTE_STATE/known_hosts" -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes)
case "${1:-up}" in
  up)
    "${compose[@]}" up --build --detach --wait
    # Obtain the host key through Docker, not an unauthenticated network scan.
    "${compose[@]}" exec -T remote cat /etc/ssh/keys/ssh_host_ed25519_key.pub | awk -v host="[127.0.0.1]:$ssh_port" '{print host, $1, $2}' > "$HIVE_REMOTE_STATE/known_hosts"
    echo 'Remote fixture ready. Configure its SSH host in Desktop Settings → Remote.'
    ;;
  tunnel)
    exec ssh "${ssh_args[@]}" -N -T -o ExitOnForwardFailure=yes -o ServerAliveInterval=15 -o ServerAliveCountMax=3 -L "127.0.0.1:$local_port:127.0.0.1:19003" hive@127.0.0.1
    ;;
  ssh-config)
    cat <<CONFIG
Host $project
  HostName 127.0.0.1
  Port $ssh_port
  User hive
  IdentityFile "$HIVE_REMOTE_STATE/id_ed25519"
  UserKnownHostsFile "$HIVE_REMOTE_STATE/known_hosts"
  IdentitiesOnly yes
CONFIG
    ;;
  connection)
    ssh "${ssh_args[@]}" hive@127.0.0.1 cat /home/hive/connection.json > "$HIVE_REMOTE_STATE/connection.json"
    chmod 600 "$HIVE_REMOTE_STATE/connection.json"
    echo "Connection credential saved to $HIVE_REMOTE_STATE/connection.json"
    echo "Desktop retrieves this credential through SSH automatically. See cmd/desktop/remote/README.md."
    ;;
  test) "${compose[@]}" --profile test run --no-deps --build --rm smoke ;;
  shell) exec ssh "${ssh_args[@]}" -t hive@127.0.0.1 ;;
  down) "${compose[@]}" down ;;
  *) echo 'Usage: run.sh up|ssh-config|tunnel|connection|shell|test|down' >&2; exit 2 ;;
esac
