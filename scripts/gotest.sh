#!/usr/bin/env bash
# go test with no route to a developer's tmux server, for packages that do not
# guard themselves with tmuxtest.Run. The tmux variables are also part of what the
# test cache keys on, so a run inside tmux and one outside share results.
set -euo pipefail

# Fixed rather than per-run for that cache, and created first because tmux falls
# back to the default socket in /tmp when this directory is missing.
export TMUX_TMPDIR=/tmp/hive-test-tmux
mkdir -p "$TMUX_TMPDIR"
unset TMUX TMUX_PANE

exec go test "$@"
