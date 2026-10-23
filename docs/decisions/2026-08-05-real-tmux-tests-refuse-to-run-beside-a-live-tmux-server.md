# Real-tmux tests refuse to run beside a live tmux server

- **Status:** accepted
- **Date:** 2026-08-05

## Context

The terminal tests drive a real tmux server, because a fake would only prove
the fake, and each test ends by killing the server it used. Isolating that
server through the environment (`TMUX_TMPDIR`, scrubbing `$TMUX`) holds only
while every tmux invocation honours it, and one miss kills the developer's own
sessions. The pre-push hook runs these tests.

## Decision

Two layers, in `internal/platform/tmuxtest`:

1. Fixture commands pass the socket explicitly as `tmux -S <path> ...`. `-S`
   wins over `$TMUX` and `TMUX_TMPDIR`, so `kill-server` cannot reach another
   server. `TMUX_TMPDIR` still points the code under test at the same socket.
2. Outside CI (`CI` unset), the tests skip while any tmux server answers.

## Consequences

The real-tmux tests do not run locally for anyone with a tmux server running.
That is intended: the guarantee should not depend on every future test
remembering `-S`. CI and Docker have no server running, so coverage holds there.
Do not relax the check to make them run beside a session; verify in CI or a
container.
