#!/usr/bin/env bash
# The ssh fixture for remote hive servers: two containers, server-a and
# server-b, each running hive behind sshd.
#
#   up          start both servers and write an ssh_config for them
#   test [...]  run the Go tests against a fresh fixture, then remove it
#   run [...]   run the Go tests against the fixture that up started
#   down        remove the fixture
#
# test and run pass their arguments to go test, for example -run TestProbe.
# The tests build the client hive from source on every run; the servers run
# the binary that was current at the last up. To run two fixtures at once,
# give each its own HIVE_REMOTE_STATE, HIVE_REMOTE_SSH_PORT_A, and
# HIVE_REMOTE_SSH_PORT_B.
set -euo pipefail
cd "$(dirname "$0")/../.."
repo_root="$PWD"
export HIVE_REMOTE_STATE="${HIVE_REMOTE_STATE:-$repo_root/.data/remote}"
export HIVE_REMOTE_SSH_PORT_A="${HIVE_REMOTE_SSH_PORT_A:-19222}"
export HIVE_REMOTE_SSH_PORT_B="${HIVE_REMOTE_SSH_PORT_B:-19223}"
export HIVE_REMOTE_PROJECT="hive-remote-$(printf '%s' "$HIVE_REMOTE_STATE" | cksum | awk '{print $1}')"
compose=(docker compose -p "$HIVE_REMOTE_PROJECT" -f test/remote/compose.yaml)

up() {
  mkdir -p "$HIVE_REMOTE_STATE"
  chmod 700 "$HIVE_REMOTE_STATE"
  for key in id_ed25519 server-a_host_ed25519_key server-b_host_ed25519_key; do
    if [[ ! -f "$HIVE_REMOTE_STATE/$key" ]]; then
      ssh-keygen -q -t ed25519 -N '' -f "$HIVE_REMOTE_STATE/$key"
    fi
  done
  "${compose[@]}" up --build --detach --wait
  : >"$HIVE_REMOTE_STATE/known_hosts"
  : >"$HIVE_REMOTE_STATE/ssh_config"
  for server in server-a server-b; do
    port="$HIVE_REMOTE_SSH_PORT_A"
    [[ "$server" == server-b ]] && port="$HIVE_REMOTE_SSH_PORT_B"
    awk -v host="[127.0.0.1]:$port" '{print host, $1, $2}' \
      "$HIVE_REMOTE_STATE/${server}_host_ed25519_key.pub" >>"$HIVE_REMOTE_STATE/known_hosts"
    cat >>"$HIVE_REMOTE_STATE/ssh_config" <<CONFIG
Host $server
  HostName 127.0.0.1
  Port $port
  User hive
  IdentityFile "$HIVE_REMOTE_STATE/id_ed25519"
  UserKnownHostsFile "$HIVE_REMOTE_STATE/known_hosts"
  IdentitiesOnly yes

CONFIG
  done
  echo "ready: ssh -F $HIVE_REMOTE_STATE/ssh_config server-a (or server-b)"
}

down() {
  "${compose[@]}" down -v --remove-orphans
}

gotest() {
  go test -tags remote -count=1 -v ./test/remote/... "$@"
}

cmd="${1:-up}"
shift || true
case "$cmd" in
  up) up ;;
  test)
    down >/dev/null 2>&1 || true
    trap down EXIT
    up
    gotest "$@"
    ;;
  run) gotest "$@" ;;
  down) down ;;
  *)
    echo "usage: run.sh up|test|run|down [go test args]" >&2
    exit 2
    ;;
esac
