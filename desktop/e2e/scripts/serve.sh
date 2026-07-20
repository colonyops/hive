#!/usr/bin/env bash
set -euo pipefail

cd ../../
(
  cd desktop/frontend
  npm run build
)
mkdir -p desktop/bin

# The feed-mode server's flows/*.yaml directory: a checked-in fixture (one
# flow, whose feed node id matches desktop/mockseed.go's seeded feed_item
# rows) rather than desktop.FlowsDir()'s default — keeps the mock sidebar's
# profile/feed list deterministic instead of depending on whatever XDG
# config happens to exist on the machine running the gate.
FEED_FLOWS_DIR="$(pwd)/desktop/e2e/fixtures/flows"
FEED_ACTIONS_PATH="$(pwd)/desktop/e2e/fixtures/actions.yml"
# CGO_ENABLED=0: the server-mode binary is pure Go (matches the Wails server
# Dockerfile). With cgo on, Linux builds pull in Wails' gtk4/webkitgtk bindings,
# which fail on CI runners without the GTK dev packages.
CGO_ENABLED=0 go build -tags server -o desktop/bin/hive-desktop-server ./desktop

# Mock-mode servers keep the gate deterministic and offline. The onboarding
# mock backend is a per-process singleton that stays authenticated once its
# fake device flow grants, so each browser project gets its own instance
# (chromium 8932, webkit 8933). Playwright only waits on the feed server;
# the readiness loop below covers the background ones.

pids=()
cleanup() {
  # Best-effort reap of the onboarding servers we started. On the normal,
  # successful path this body never actually runs: the `exec` at the very
  # end of this script replaces the shell's own process image, so there is
  # no shell left for the EXIT trap to fire from, and Playwright's teardown
  # kills the whole process group (which already includes these background
  # servers) instead. This trap only matters for the early-exit failure
  # paths below (stale port, bind failure, readiness timeout), so those
  # don't leave orphaned servers behind for the next run to trip over.
  if [[ ${#pids[@]} -gt 0 ]]; then
    kill "${pids[@]}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

for port in 8932 8933; do
  # Pre-flight: refuse to reuse a port that's already answering. A stale
  # onboarding server leaked from a previous hard-killed run has usually
  # already granted its fake device-flow auth, so silently attaching to it
  # would make onboarding.spec.ts fail on its very first assertion with no
  # hint that the real cause is a leftover process on this port.
  if curl -fsS -o /dev/null "http://127.0.0.1:${port}/"; then
    echo "error: port ${port} is already in use (stale server from a previous run?)" >&2
    exit 1
  fi
  HIVE_DESKTOP_MOCK=onboarding WAILS_SERVER_PORT="${port}" desktop/bin/hive-desktop-server &
  pids+=("$!")
done

ports=(8932 8933)
for i in "${!ports[@]}"; do
  port="${ports[$i]}"
  pid="${pids[$i]}"
  ready=0
  for _ in $(seq 1 50); do
    if ! kill -0 "${pid}" 2>/dev/null; then
      echo "error: onboarding server on port ${port} exited before becoming ready (bind failure?)" >&2
      exit 1
    fi
    if curl -fsS -o /dev/null "http://127.0.0.1:${port}/"; then
      ready=1
      break
    fi
    sleep 0.2
  done
  if [[ "${ready}" -ne 1 ]]; then
    echo "error: onboarding server on port ${port} did not become ready" >&2
    exit 1
  fi
done

exec env HIVE_DESKTOP_MOCK=feed HIVE_DESKTOP_FLOWS="${FEED_FLOWS_DIR}" HIVE_DESKTOP_ACTIONS="${FEED_ACTIONS_PATH}" WAILS_SERVER_PORT="${WAILS_SERVER_PORT:-8931}" desktop/bin/hive-desktop-server
