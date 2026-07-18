#!/usr/bin/env bash
set -euo pipefail

cd ../../
(
  cd desktop/frontend
  npm run build
)
mkdir -p desktop/bin
go build -tags server -o desktop/bin/hive-desktop-server ./desktop
exec env WAILS_SERVER_PORT="${WAILS_SERVER_PORT:-8931}" desktop/bin/hive-desktop-server
