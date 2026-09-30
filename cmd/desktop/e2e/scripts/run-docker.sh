#!/usr/bin/env bash
set -euo pipefail

# E2E includes server builds, tmux-aware Hive services, and Playwright browser
# binaries. Keep all of that inside the pinned container: there is deliberately
# no host Playwright fallback.
cd "$(dirname "$0")/../../.."

if ! docker info >/dev/null 2>&1; then
  echo "error: Docker is required for the e2e suite; host Playwright is intentionally unsupported" >&2
  exit 1
fi

# Override the image tag when concurrent runs on one host must not clobber
# each other's builds (e.g. two agents verifying different branches).
IMAGE_TAG="${HIVE_E2E_IMAGE_TAG:-hive-desktop-e2e:local}"

# The per-run token is intentionally not a fixed truthy flag: the Docker
# entrypoint, web-server launcher, and action-smoke route all require this
# 256-bit marker. A direct host Playwright invocation therefore cannot start
# the e2e servers.
E2E_HARNESS_MARKER="$(openssl rand -hex 32)"
docker build --file cmd/desktop/e2e/Dockerfile --tag "${IMAGE_TAG}" .

# Mount the results dirs out of the --rm container so failure traces and
# screenshots survive the run (CI uploads test-results as an artifact).
mkdir -p cmd/desktop/e2e/test-results cmd/desktop/e2e/screenshots
# --ipc=host: Chromium can exhaust Docker's default 64MB /dev/shm and crash
# mid-suite (playwright.dev/docs/docker); host IPC is the documented fix.
#
# --env PW_FAIL_ON_FLAKY (no value) forwards the host value only when the
# variable is set on the host; otherwise it stays unset in the container.
# Contract: playwright.config.ts enables failOnFlakyTests when
# PW_FAIL_ON_FLAKY is set, so retried-then-passed tests fail the run —
# use it for a strict flake hunt before landing a branch.
#
# Extra script arguments are appended after the image name; the image
# entrypoint treats a leading flag as Playwright options, so
# `./run-docker.sh --grep foo --repeat-each 5` runs
# `npx playwright test --grep foo --repeat-each 5`. No args = full suite.
docker run --rm --init --network none --ipc=host \
  --env HIVE_DESKTOP_E2E_HARNESS="${E2E_HARNESS_MARKER}" \
  --env PW_FAIL_ON_FLAKY \
  --volume "$(pwd)/cmd/desktop/e2e/test-results:/workspace/cmd/desktop/e2e/test-results" \
  --volume "$(pwd)/cmd/desktop/e2e/screenshots:/workspace/cmd/desktop/e2e/screenshots" \
  "${IMAGE_TAG}" "$@"
