#!/usr/bin/env bash
# A throwaway hive stack for smoke tests: a client and two servers in Docker,
# with hive on all three. See .agents/skills/cli-smoke/SKILL.md.
#
#   up                        start the stack, or replace it with this branch's binary
#   down                      remove the stack and everything in it
#   exec <service> <cmd...>   run a command as the hive user
#   root <service> <cmd...>   run a command as root
#   tui start [hive args]     start the TUI on the client, in tmux
#   tui keys <tmux keys...>   press keys in the TUI
#   tui show                  print the TUI's screen
#   tui stop                  stop the TUI
#   fault <name> <server> [args]  break a server (names: see fault() below)
#   fault clear <server>      undo every fault on a server
#   test                      check this script against a fresh stack, then remove it
#
# <service> is client, server-a, or server-b. Each worktree gets its own stack.
# To run two in one worktree, give each its own HIVE_SMOKE_STATE.
set -euo pipefail
cd "$(dirname "$0")/../.."
export HIVE_SMOKE_STATE="${HIVE_SMOKE_STATE:-$PWD/.data/smoke}"
export HIVE_SMOKE_PROJECT="hive-smoke-$(printf '%s' "$HIVE_SMOKE_STATE" | cksum | awk '{print $1}')"
compose=(docker compose -p "$HIVE_SMOKE_PROJECT" -f test/smoke/compose.yaml)

up() {
  mkdir -p "$HIVE_SMOKE_STATE"
  chmod 700 "$HIVE_SMOKE_STATE"
  for key in id_ed25519 server-a_host_ed25519_key server-b_host_ed25519_key; do
    if [[ ! -f "$HIVE_SMOKE_STATE/$key" ]]; then
      ssh-keygen -q -t ed25519 -N '' -f "$HIVE_SMOKE_STATE/$key"
    fi
  done
  : >"$HIVE_SMOKE_STATE/known_hosts"
  for server in server-a server-b; do
    awk -v host="$server" '{print host, $1, $2}' \
      "$HIVE_SMOKE_STATE/${server}_host_ed25519_key.pub" >>"$HIVE_SMOKE_STATE/known_hosts"
  done
  "${compose[@]}" up --build --detach --wait --force-recreate --remove-orphans
  echo "ready: test/smoke/run.sh exec client hive session list"
}

down() {
  "${compose[@]}" down -v --remove-orphans
}

on() {
  "${compose[@]}" exec -T -u hive "$@"
}

as_root() {
  "${compose[@]}" exec -T -u root "$@"
}

tui() {
  local cmd="${1:-}"
  shift || true
  case "$cmd" in
    start) on client tmux -u new-session -d -s tui -x 160 -y 50 hive "$@" ;;
    keys) on client tmux send-keys -t tui "$@" ;;
    show) on client tmux capture-pane -p -t tui ;;
    stop) on client tmux kill-session -t tui ;;
    *) echo "usage: run.sh tui start|keys|show|stop" >&2; exit 2 ;;
  esac
}

daemon_pid() {
  as_root "$1" pgrep -f '^/usr/local/bin/hive daemon run'
}

fault() {
  local name="${1:-}" server="${2:-}"
  case "$server" in
    server-a | server-b) ;;
    *) echo "usage: run.sh fault <name> server-a|server-b [args]" >&2; exit 2 ;;
  esac
  shift 2
  case "$name" in
    # The daemon stops answering, but ssh still connects.
    stuck-daemon)
      on "$server" hive session list >/dev/null
      as_root "$server" kill -STOP "$(daemon_pid "$server")"
      ;;
    # The server starts a new daemon at the next connection.
    kill-daemon)
      on "$server" hive session list >/dev/null
      as_root "$server" kill -KILL "$(daemon_pid "$server")"
      ;;
    # Every git command takes this many seconds longer (default 12).
    slow-git)
      as_root "$server" bash -c '
        printf "#!/bin/sh\nsleep %s\nexec /usr/bin/git \"\$@\"\n" "$1" >/usr/local/bin/git
        chmod 755 /usr/local/bin/git' _ "${1:-12}"
      ;;
    # The server's shell prints text before hive starts.
    shell-noise)
      on "$server" bash -c '
        [ -f ~/.bashrc.fault ] || cp ~/.bashrc ~/.bashrc.fault
        { echo "echo Welcome to this server"; cat ~/.bashrc.fault; } >~/.bashrc'
      ;;
    # tc netem arguments, default: delay 300ms loss 5%.
    network)
      if [[ $# -eq 0 ]]; then set -- delay 300ms loss 5%; fi
      as_root "$server" tc qdisc replace dev eth0 root netem "$@"
      ;;
    # The server is off: connections fail fast.
    stop) "${compose[@]}" stop "$server" ;;
    # The server hangs: connections and open streams get no answer and no error.
    freeze) "${compose[@]}" pause "$server" ;;
    clear) clear_faults "$server" ;;
    *) echo "unknown fault: $name" >&2; exit 2 ;;
  esac
}

clear_faults() {
  local server="$1"
  "${compose[@]}" unpause "$server" >/dev/null 2>&1 || true
  "${compose[@]}" up --detach --no-recreate "$server"
  # Not `up --wait`: health checks fail during a freeze, and it gives up on an
  # unhealthy container instead of waiting for the next check to pass.
  local tries=30
  until as_root "$server" bash -c 'exec 3<>/dev/tcp/127.0.0.1/22' 2>/dev/null; do
    ((--tries)) || { echo "$server did not come back" >&2; exit 1; }
    sleep 1
  done
  as_root "$server" bash -c '
    rm -f /usr/local/bin/git
    pkill -CONT -f "^/usr/local/bin/hive daemon run" || true
    tc qdisc del dev eth0 root 2>/dev/null || true'
  on "$server" bash -c '[ ! -f ~/.bashrc.fault ] || mv ~/.bashrc.fault ~/.bashrc'
}

selftest() {
  local id
  id=$(on client hive --target-server server-a session create --json --background \
    --remote /home/hive/repo.git selftest | sed -E 's/.*"id":"([^"]*)".*/\1/')
  [[ -n $id ]]
  on client hive session show --json "server-a:$id" | grep -q '"name":"selftest"'
  on server-a hive session show --json "$id" | grep -q '"name":"selftest"'

  on client hive session create --json --background --remote /home/hive/repo.git local-one >/dev/null
  on client hive session list --json | grep -q '"name":"local-one"'

  tui start
  sleep 2
  tui show | grep -q .
  tui stop

  for name in stuck-daemon shell-noise slow-git; do
    fault "$name" server-b
    fault clear server-b
  done
  fault stop server-b
  fault clear server-b
  fault freeze server-b
  fault clear server-b
  on client hive --target-server server-b session list --json >/dev/null

  # The internal network must have no way out.
  if on client bash -c 'exec 3<>/dev/tcp/1.1.1.1/443' 2>/dev/null; then
    echo "the client reached the internet" >&2
    exit 1
  fi
  echo "smoke stack ok"
}

cmd="${1:-}"
shift || true
case "$cmd" in
  up) up ;;
  down) down ;;
  exec) on "$@" ;;
  root) as_root "$@" ;;
  tui) tui "$@" ;;
  fault) fault "$@" ;;
  test)
    down >/dev/null 2>&1 || true
    trap down EXIT
    up
    selftest
    ;;
  *)
    sed -n '2,19p' "$0" >&2
    exit 2
    ;;
esac
