#!/usr/bin/env bash
# Runs the integration tests inside Docker, the only safe place for them: they
# drive tmux sessions that have crashed host tmux. The test binary compiles here,
# so it reuses this machine's Go build cache.
set -euo pipefail
cd "$(dirname "$0")/../.."

CGO_ENABLED=0 GOOS=linux GOARCH="$(docker version --format '{{.Server.Arch}}')" \
  go test -c -tags integration -o dist/integration.test ./test/integration
docker build -f cmd/hive/Dockerfile.test -t hive-integration-test .
docker run --rm hive-integration-test "$@"
