#!/usr/bin/env bash
set -euo pipefail

# The suite needs Chromium, Firefox, and WebKit at one pinned version, so it
# runs only in the pinned container; there is no host Playwright fallback.
cd "$(dirname "$0")/../../../.."

if ! docker info >/dev/null 2>&1; then
  echo "error: Docker is required for the drive server e2e suite" >&2
  exit 1
fi

IMAGE_TAG="${HIVE_DRIVE_E2E_IMAGE_TAG:-hive-drive-server-e2e:local}"

E2E_HARNESS_MARKER="$(openssl rand -hex 32)"
docker build --file cmd/drive-server/e2e/Dockerfile --tag "${IMAGE_TAG}" .

mkdir -p cmd/drive-server/e2e/test-results
# --network none: the suite only needs loopback, and a document that escapes
# toward a real host has nowhere to go. The canary listener is what detects an
# escape. --ipc=host stops Chromium exhausting the 64MB /dev/shm.
docker run --rm --init --network none --ipc=host \
  --env HIVE_DRIVE_E2E_HARNESS="${E2E_HARNESS_MARKER}" \
  --volume "$(pwd)/cmd/drive-server/e2e/test-results:/workspace/cmd/drive-server/e2e/test-results" \
  "${IMAGE_TAG}" "$@"
