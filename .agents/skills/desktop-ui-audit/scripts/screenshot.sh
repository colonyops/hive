#!/usr/bin/env bash
# Capture the native dev app's window to a PNG (macOS only).
#
#   screenshot.sh [out.png]     default: cmd/desktop/e2e/screenshots/native-<HHMMSS>.png
#
# The app is found by the process listening on launch.env's WAILS_MCP_PORT, so
# this never captures the installed Hive.app. Needs Screen Recording permission
# for the terminal (or agent) process; swiftc compiles the window lister once.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
env_file=${HIVE_LAUNCH_ENV:-$root/launch.env}

[[ -f $env_file ]] || { echo "no $env_file; run 'mise run desktop:dev:prepare'" >&2; exit 2; }
port=$(sed -nE 's/^WAILS_MCP_PORT="?([0-9]+)"?$/\1/p' "$env_file" | head -1)
[[ -n $port ]] || { echo "$env_file has no WAILS_MCP_PORT; run 'mise run desktop:dev:prepare' to regenerate it" >&2; exit 2; }

pid=$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null | head -1 || true)
[[ -n $pid ]] || { echo "nothing listens on 127.0.0.1:$port; is 'mise run desktop:dev' running?" >&2; exit 1; }

lister=${TMPDIR:-/tmp}/hive-winid-$(shasum "$here/winid.swift" | cut -c1-12)
[[ -x $lister ]] || swiftc -O -o "$lister" "$here/winid.swift"

wid=$("$lister" "$pid" | head -1 | cut -f1 || true)
[[ -n $wid ]] || { echo "pid $pid has no on-screen window (hidden, minimised, or on another Space)" >&2; exit 1; }

out=${1:-$root/cmd/desktop/e2e/screenshots/native-$(date +%H%M%S).png}
mkdir -p "$(dirname "$out")"
screencapture -l "$wid" -x -o "$out"
echo "$out"
