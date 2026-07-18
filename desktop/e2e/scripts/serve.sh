#!/usr/bin/env bash
set -euo pipefail

cd ../../
(
  cd desktop/frontend
  npm run build
)
mkdir -p desktop/bin
go build -tags server -o desktop/bin/hive-desktop-server ./desktop
exec desktop/bin/hive-desktop-server
