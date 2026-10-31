#!/usr/bin/env bash
# Runs the tests that drive a real tmux server, in Docker. tmuxtest.Require
# skips them on any machine with a running tmux server, so `mise run test` on a
# developer's machine passes without them. Set HIVE_TMUX_TEST_ARGS to pass
# arguments to go test.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

packages=$(git grep -lE 'tmuxtest\.(Require|Private)|privateTmux\(' -- '*_test.go' | xargs -n1 dirname | sort -u | sed 's|^|./|')

docker run --rm \
  -v "$PWD:/src:ro" \
  -v hive-tmux-test-gomod:/go/pkg/mod \
  -v hive-tmux-test-gocache:/root/.cache/go-build \
  -e CI=1 \
  -e HIVE_DEFAULT_AGENT= \
  -e HIVE_TMUX_TEST_ARGS="${HIVE_TMUX_TEST_ARGS:-}" \
  -w /src \
  "golang:$(awk '/^go /{print $2}' go.mod)" \
  bash -euc '
    apt-get update -qq >/dev/null
    apt-get install -y -qq --no-install-recommends tmux git >/dev/null
    # From /, because the .git file of a worktree names a path on the host.
    (cd / && git config --global user.email test@example.com && git config --global user.name test)
    tmux -V
    # shellcheck disable=SC2086
    go test -count=1 ${HIVE_TMUX_TEST_ARGS:-} "$@"
  ' _ $packages
