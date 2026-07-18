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
exec env WAILS_SERVER_PORT="${WAILS_SERVER_PORT:-8931}" desktop/bin/hive-desktop-server
