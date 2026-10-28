#!/usr/bin/env bash
# The ssh fixture for `hive servers`: up starts it and writes an ssh_config
# for host "fixture", test runs probe_test.sh against a fresh fixture, down
# removes it. To run more than one fixture at a time, give each its own
# HIVE_REMOTE_NAME (the compose project and ssh host alias),
# HIVE_REMOTE_STATE, and HIVE_REMOTE_SSH_PORT.
set -euo pipefail
cd "$(dirname "$0")/../.."
repo_root="$PWD"
export HIVE_REMOTE_STATE="${HIVE_REMOTE_STATE:-$repo_root/.data/remote}"
ssh_port="${HIVE_REMOTE_SSH_PORT:-19222}"
host="${HIVE_REMOTE_NAME:-fixture}"
project="hive-remote-$(printf '%s' "$repo_root" | cksum | awk '{print $1}')"
if [[ -n "${HIVE_REMOTE_NAME:-}" ]]; then
  project="$project-$HIVE_REMOTE_NAME"
fi
compose=(docker compose -p "$project" -f test/remote/compose.yaml)

up() {
  mkdir -p "$HIVE_REMOTE_STATE"
  chmod 700 "$HIVE_REMOTE_STATE"
  if [[ ! -f "$HIVE_REMOTE_STATE/id_ed25519" ]]; then
    ssh-keygen -q -t ed25519 -N '' -f "$HIVE_REMOTE_STATE/id_ed25519"
  fi
  "${compose[@]}" up --build --detach --wait
  # Read the host key through Docker, not from an unauthenticated scan.
  "${compose[@]}" exec -T remote cat /etc/ssh/keys/ssh_host_ed25519_key.pub |
    awk -v host="[127.0.0.1]:$ssh_port" '{print host, $1, $2}' >"$HIVE_REMOTE_STATE/known_hosts"
  cat >"$HIVE_REMOTE_STATE/ssh_config" <<CONFIG
Host $host
  HostName 127.0.0.1
  Port $ssh_port
  User hive
  IdentityFile "$HIVE_REMOTE_STATE/id_ed25519"
  UserKnownHostsFile "$HIVE_REMOTE_STATE/known_hosts"
  IdentitiesOnly yes
CONFIG
  echo "$host ready: ssh -F $HIVE_REMOTE_STATE/ssh_config $host"
}

down() {
  "${compose[@]}" down -v --remove-orphans
}

case "${1:-up}" in
  up) up ;;
  test)
    down >/dev/null 2>&1 || true
    trap down EXIT
    up
    test/remote/probe_test.sh
    ;;
  down) down ;;
  *)
    echo "usage: run.sh up|test|down" >&2
    exit 2
    ;;
esac
