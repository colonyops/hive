# Real-tmux tests run on the host against private sockets

- **Status:** accepted
- **Date:** 2026-11-01

## Context

Developers build this app from inside tmux. The real-tmux tests skipped whenever a tmux server was reachable, so they only ran in CI. Moving them to Docker would have added an image build and lost macOS coverage.

Isolating through the environment alone is not enough. `$TMUX` outranks `TMUX_TMPDIR`, the terminal code follows it on purpose, and tmux falls back to the default socket in `/tmp` when `TMUX_TMPDIR` names a directory it cannot use. An audit that ran the suite against a refusing `tmux` shim found read-only commands reaching the developer's server.

## Decision

A unit test that needs tmux gets its own server from `tmuxtest.New`, addressed only by `-S`, with no change to the process environment. `tmuxtest.Private` remains for code that takes no Runner. `scripts/gotest.sh` and `tmuxtest.Run` remove the route to a developer's server for any test that uses neither. CLI integration tests stay in Docker, since they spawn the hive binary against one server per run.

## Consequences

Real-tmux tests run locally and can run in parallel. A new test that drives tmux has to go through `tmuxtest`, and code under test has to accept a Runner to stay parallel.
