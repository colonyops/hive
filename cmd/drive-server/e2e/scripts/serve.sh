#!/usr/bin/env bash
set -euo pipefail

if [[ ! "${HIVE_DRIVE_E2E_HARNESS:-}" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo 'error: run the suite through scripts/run-docker.sh' >&2
  exit 1
fi

run_dir="${DRIVE_E2E_RUN_DIR:?}"
port="${DRIVE_E2E_PORT:?}"
binary="$(cd "$(dirname "$0")/../../../.." && pwd)/cmd/drive-server/bin/hive-drive-server"

rm -rf "${run_dir}"
mkdir -p "${run_dir}/data" "${run_dir}/config"
cat >"${run_dir}/config/drive-server.yaml" <<YAML
user_policies:
  - name: e2e
    matches:
      - "*@example.com"
    quotas:
      limit: 1GiB
YAML

# --dev-log-mail prints the sign-in code to stdout; the tests read it here.
exec "${binary}" serve --dev-log-mail \
  --listen "127.0.0.1:${port}" \
  --base-url "http://127.0.0.1:${port}" \
  --data-dir "${run_dir}/data" \
  --config-dir "${run_dir}/config" >"${run_dir}/server.log" 2>&1
