#!/usr/bin/env bash
set -euo pipefail
umask 077
prefix="$HOME/.local/share/hive-server"
export PATH="$prefix/bin:$HOME/.local/bin:$PATH"
export HIVE_CONFIG="${HIVE_CONFIG:-$HOME/.config/hive-server/config.yaml}"
export HIVE_DATA_DIR="${HIVE_DATA_DIR:-$prefix/engine}"
export HIVE_DESKTOP_HIVE_DATA_DIR="$HIVE_DATA_DIR"
export HIVE_DESKTOP_DATA_DIR="${HIVE_DESKTOP_DATA_DIR:-$prefix/data}"
export HIVE_DESKTOP_CONFIG_DIR="${HIVE_DESKTOP_CONFIG_DIR:-$HOME/.config/hive-server/desktop}"
export HIVE_DESKTOP_CONNECTION_FILE="${HIVE_DESKTOP_CONNECTION_FILE:-$prefix/connection.json}"
export HIVE_DESKTOP_HTTP_PORT="${HIVE_DESKTOP_HTTP_PORT:-19001}"
export HIVE_DESKTOP_REMOTE_ADDRESS="${HIVE_DESKTOP_REMOTE_ADDRESS:-127.0.0.1:19003}"
export HIVE_DESKTOP_UPDATES_ENABLED=false
export WAILS_SERVER_HOST=127.0.0.1
export WAILS_SERVER_PORT="${WAILS_SERVER_PORT:-19000}"
export TMUX_TMPDIR="${XDG_RUNTIME_DIR:?XDG_RUNTIME_DIR must be set}/hive-server"
mkdir -p "$TMUX_TMPDIR"
case "${1:-serve}" in
  serve) exec "$prefix/bin/hive-server" ;;
  tmux) exec /usr/bin/tmux -D -f /dev/null ;;
  wait-tmux)
    for attempt in $(seq 1 50); do
      if /usr/bin/tmux show-options -g >/dev/null 2>&1; then exit 0; fi
      sleep 0.1
    done
    echo 'tmux did not become ready' >&2; exit 1 ;;
  cli) shift; exec "$prefix/bin/hive" "$@" ;;
  *) echo 'Usage: launch.sh [serve|tmux|wait-tmux|cli ...]' >&2; exit 1 ;;
esac

