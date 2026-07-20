#!/usr/bin/env bash
set -euo pipefail

# E2E includes server builds, tmux-aware Hive services, and Playwright browser
# binaries. Keep all of that inside the pinned container: there is deliberately
# no host Playwright fallback.
cd "$(dirname "$0")/../../.."

if ! docker info >/dev/null 2>&1; then
  echo "error: Docker is required for desktop:e2e; host Playwright is intentionally unsupported" >&2
  exit 1
fi

# The per-run token is intentionally not a fixed truthy flag: the Docker CMD,
# web-server launcher, and action-smoke route all require this 256-bit marker.
# A direct host Playwright invocation therefore cannot start the e2e servers.
E2E_HARNESS_MARKER="$(openssl rand -hex 32)"
docker build --file desktop/e2e/Dockerfile --tag hive-desktop-e2e:local .
docker run --rm --init --network none \
  --env HIVE_DESKTOP_E2E_HARNESS="${E2E_HARNESS_MARKER}" \
  hive-desktop-e2e:local
