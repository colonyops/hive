#!/usr/bin/env bash
set -euo pipefail

# The remote lane: the Desktop e2e tests in tests/workflows/remote, against
# the ssh fixture from test/remote as a real server. It uses a fixture of its
# own (state dir and ports), so it can run beside `mise run cli:remote:test`.
# Arguments go to Playwright after the project filter.
cd "$(dirname "$0")/../../../.."
export HIVE_REMOTE_STATE="${HIVE_REMOTE_STATE:-$PWD/.data/remote-e2e}"
export HIVE_REMOTE_SSH_PORT_A="${HIVE_REMOTE_SSH_PORT_A:-19232}"
export HIVE_REMOTE_SSH_PORT_B="${HIVE_REMOTE_SSH_PORT_B:-19233}"

test/remote/run.sh down >/dev/null 2>&1 || true
trap 'test/remote/run.sh down >/dev/null 2>&1 || true' EXIT
test/remote/run.sh up

HIVE_DESKTOP_E2E_REMOTE_NETWORK="$(test/remote/run.sh network)" \
  HIVE_DESKTOP_E2E_REMOTE_STATE="${HIVE_REMOTE_STATE}" \
  cmd/desktop/e2e/scripts/run-docker.sh --project workflows-remote-chromium "$@"
