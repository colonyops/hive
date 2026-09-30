# Real-tmux tests refuse to run beside a live tmux server

- **Status:** accepted
- **Date:** 2026-08-05

## Context

The terminal tests in `internal/app` and `internal/adapter/httpapi` drive a real
tmux server, because what the attach path does is a decision about what tmux is
actually holding and a fake would only prove the fake. Each test therefore ends
by destroying the server it used.

Isolation was previously environmental: point `TMUX_TMPDIR` at a scratch
directory, scrub `$TMUX`, and run a bare `tmux kill-server`. That only holds
while *every* tmux invocation in the test — the service's included — honours the
environment. It is one missed scrub away from resolving to the developer's own
socket, and the two fixtures had already drifted (one scrubbed `$TMUX`, the other
did not). The guard that was supposed to catch this checked `$TMUX`, which
detects "this process is inside tmux" — not "this machine has a tmux server",
which is the condition that actually matters when the suite is run from an
editor, an agent, or any shell outside a pane.

The failure is unrecoverable: a developer running `mise run check` loses every
running session, and the pre-push hook runs that gate.

## Decision

Two layers, in `internal/tmuxtest`:

1. Fixture commands pass the socket explicitly as `tmux -S <path> …`. tmux
   resolves `-S` ahead of both `$TMUX` and `TMUX_TMPDIR`, so `kill-server`
   cannot reach another server regardless of what the environment says.
   `TMUX_TMPDIR` still points the code under test at that same socket, since it
   resolves its own; the helper derives the `-S` path to match.
2. Outside CI, the tests skip entirely while any tmux server answers.

## Consequences

The real-tmux tests do not run locally for anyone who uses tmux. That is
intended, and it is why layer 2 exists despite layer 1 being sufficient on
paper: the guarantee should not depend on every future test remembering to pass
`-S`. CI and the Docker e2e environment have no server running, so coverage is
unchanged there.

A local run that reports these tests as skipped is working correctly. Do not
relax the check to make them run beside a session — verify against CI, or in a
container.
