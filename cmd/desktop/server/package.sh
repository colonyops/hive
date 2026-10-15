#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../../.."
arch="${HIVE_SERVER_ARCH:-$(go env GOARCH)}"
case "$arch" in amd64|arm64) ;; *) echo 'HIVE_SERVER_ARCH must be amd64 or arm64' >&2; exit 1 ;; esac
out="$PWD/cmd/desktop/bin/server-linux-$arch"
mkdir -p "$out/bin"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -tags server,production,terminals -trimpath -o "$out/bin/hive-server" ./cmd/desktop
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o "$out/bin/hive" .
cp cmd/desktop/server/{install.sh,launch.sh,hive-server.service,hive-server-tmux.service,environment.example} "$out/"
printf '%s\n' "$arch" > "$out/ARCH"
git rev-parse HEAD > "$out/REVISION"
chmod 755 "$out/"*.sh
archive="${out}.tar.gz"
tar -czf "$archive" -C "$out" .
(cd "$(dirname "$archive")" && shasum -a 256 "$(basename "$archive")" > "$(basename "$archive").sha256")
echo "$archive"

