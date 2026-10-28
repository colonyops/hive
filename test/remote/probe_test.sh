#!/usr/bin/env bash
# Creates a session on the fixture over ssh, then checks that a local
# `hive servers probe` sees it through `hive daemon dial-stdio`.
set -euo pipefail
cd "$(dirname "$0")/../.."
state="${HIVE_REMOTE_STATE:?run through run.sh}"
client="$state/client"
rm -rf "$client"
mkdir -p "$client/data"

ssh -F "$state/ssh_config" fixture hive new --background --remote /home/hive/repo.git p1

go build -o "$client/hive" .
cat >"$client/config.yaml" <<CONFIG
servers:
  - name: fixture
    ssh: fixture
    ssh_config: $state/ssh_config
CONFIG

out="$(HIVE_CONFIG="$client/config.yaml" HIVE_DATA_DIR="$client/data" "$client/hive" servers probe fixture --json)"
echo "$out"
jq -e '.health.db_ok == true' <<<"$out" >/dev/null || { echo "FAIL: health.db_ok is not true" >&2; exit 1; }
jq -e '.sessions | length == 1' <<<"$out" >/dev/null || { echo "FAIL: want 1 session" >&2; exit 1; }
jq -e '.sessions[0].name == "p1"' <<<"$out" >/dev/null || { echo "FAIL: want session p1" >&2; exit 1; }

HIVE_CONFIG="$client/config.yaml" HIVE_DATA_DIR="$client/data" "$client/hive" servers ls
echo "PASS"
