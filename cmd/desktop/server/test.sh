#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../../.."
arch="${HIVE_SERVER_ARCH:-$(go env GOARCH)}"
context=$(mktemp -d)
trap 'rm -rf "$context"' EXIT
cp -R "cmd/desktop/bin/server-linux-$arch/." "$context/"
cp cmd/desktop/server/test.py "$context/"
docker build --platform "linux/$arch" -f cmd/desktop/server/Dockerfile.test -t "hive-server-ubuntu-test:$arch" "$context"
docker run --rm --platform "linux/$arch" "hive-server-ubuntu-test:$arch"
