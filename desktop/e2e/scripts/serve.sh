#!/usr/bin/env bash
set -euo pipefail

cd ../../
(
  cd desktop/frontend
  npm run build
)
mkdir -p desktop/bin
# CGO_ENABLED=0: the server-mode binary is pure Go (matches the Wails server
# Dockerfile). With cgo on, Linux builds pull in Wails' gtk4/webkitgtk bindings,
# which fail on CI runners without the GTK dev packages.
CGO_ENABLED=0 go build -tags server -o desktop/bin/hive-desktop-server ./desktop

# Mock-mode servers keep the gate deterministic and offline. The onboarding
# mock backend is a per-process singleton that stays authenticated once its
# fake device flow grants, so each browser project gets its own instance
# (chromium 8932, webkit 8933). Playwright only waits on the feed server;
# the readiness loop below covers the background ones.
HIVE_DESKTOP_MOCK=onboarding WAILS_SERVER_PORT=8932 desktop/bin/hive-desktop-server &
HIVE_DESKTOP_MOCK=onboarding WAILS_SERVER_PORT=8933 desktop/bin/hive-desktop-server &

for port in 8932 8933; do
  for _ in $(seq 1 50); do
    if curl -fsS -o /dev/null "http://127.0.0.1:${port}/"; then
      break
    fi
    sleep 0.2
  done
done

exec env HIVE_DESKTOP_MOCK=feed WAILS_SERVER_PORT="${WAILS_SERVER_PORT:-8931}" desktop/bin/hive-desktop-server
