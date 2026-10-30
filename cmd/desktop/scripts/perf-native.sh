#!/usr/bin/env bash
set -euo pipefail

# Runs the native app against a tmux server and data roots of its own, so a
# benchmark cannot reach a real session, the production hive.db, or a real
# credential. The frontend is the production bundle: Vite's dev build costs more
# per frame than the code being measured.
#
#   perf-native.sh <work-dir> [--no-build]
#
# Unsetting TMUX matters as much as TMUX_TMPDIR: an inherited TMUX names the
# socket of whichever server this script runs under, and the app honors it.
cd "$(dirname "$0")/../../.."

work_dir="${1:?usage: perf-native.sh <work-dir> [--no-build]}"
mkdir -p "${work_dir}/data" "${work_dir}/config/workspaces" "${work_dir}/hive"
work_dir="$(cd "${work_dir}" && pwd)"
# A unix socket path is capped at 104 bytes on macOS, which a work dir under a
# long checkout path easily passes.
# HIVE_PERF_TMUX_DIR reuses a server between builds so an A/B run sees the same
# sessions.
tmux_dir="${HIVE_PERF_TMUX_DIR:-$(mktemp -d -t hive-perf-tmux)}"
printf '%s\n' "${tmux_dir}" >"${work_dir}/tmux-dir"

if [[ "${2:-}" != "--no-build" ]]; then
  (cd cmd/desktop/frontend && npm run build)
  go build -tags mcp -o cmd/desktop/bin/hive-desktop-perf ./cmd/desktop 2>/dev/null
fi

test -f launch.env || mise run desktop:dev:prepare
port() { grep "^$1=" launch.env | cut -d'"' -f2; }

exec env -u TMUX -u TMUX_PANE \
  TMUX_TMPDIR="${tmux_dir}" \
  HIVE_DESKTOP_DATA_DIR="${work_dir}/data" \
  HIVE_DESKTOP_CONFIG_DIR="${work_dir}/config" \
  HIVE_DESKTOP_AGENT_WORKSPACES_DIR="${work_dir}/config/workspaces" \
  HIVE_DESKTOP_HIVE_DATA_DIR="${work_dir}/hive" \
  HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE=feed \
  HIVE_DESKTOP_DEVELOPMENT_KEYRING_SERVICE=hive-desktop-perf \
  HIVE_DESKTOP_DEVELOPMENT_PERF_ENABLED=true \
  HIVE_DESKTOP_DEVELOPMENT_PPROF_ENABLED=true \
  HIVE_DESKTOP_HTTP_ENABLED=true \
  HIVE_DESKTOP_HTTP_PORT="$(port HIVE_DESKTOP_HTTP_PORT)" \
  WAILS_MCP=1 \
  WAILS_MCP_HOST=127.0.0.1 \
  WAILS_MCP_PORT="$(port WAILS_MCP_PORT)" \
  "${HIVE_PERF_BINARY:-cmd/desktop/bin/hive-desktop-perf}"
