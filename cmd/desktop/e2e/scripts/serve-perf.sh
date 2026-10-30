#!/usr/bin/env bash
set -euo pipefail

if [[ ! "${HIVE_DESKTOP_E2E_HARNESS:-}" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo "error: the perf lane must run through mise run desktop:perf:bench (Docker harness marker missing)" >&2
  exit 1
fi

cd "$(dirname "$0")/../../../.."
data_root="$(mktemp -d "${TMPDIR:-/tmp}/hive-desktop-perf.XXXXXX")"
trap 'rm -rf "${data_root}"' EXIT
mkdir -p "${data_root}/data" "${data_root}/config" "${data_root}/hive" "${data_root}/workspaces"
cat >"${data_root}/config/settings.yaml" <<YAML
onboarding:
  completed: true
YAML

HIVE_DESKTOP_DATA_DIR="${data_root}/data" \
  HIVE_DESKTOP_CONFIG_DIR="${data_root}/config" \
  HIVE_DESKTOP_HIVE_DATA_DIR="${data_root}/hive" \
  HIVE_DESKTOP_AGENT_WORKSPACES_DIR="${data_root}/workspaces" \
  HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE=feed \
  HIVE_DESKTOP_DEVELOPMENT_PERF_ENABLED=true \
  HIVE_DESKTOP_HTTP_ENABLED=true \
  HIVE_DESKTOP_HTTP_PORT=8942 \
  SHELL=/bin/bash \
  WAILS_SERVER_PORT=8941 \
  cmd/desktop/bin/hive-desktop-server
