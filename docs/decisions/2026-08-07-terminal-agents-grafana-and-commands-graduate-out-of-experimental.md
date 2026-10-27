# Terminal, Agents, Grafana and Commands graduate out of experimental

- **Status:** accepted
- **Date:** 2026-08-07

## Context

Terminal mode and the Agents area shipped dark behind `experimental.terminal`
and `experimental.agents`. The flags were read
at startup, so every change was a settings edit plus a relaunch, and the route
groups, the terminal bearer token, and the title-bar segments each carried an
off-branch no shipped build took.

## Decision

1. **The `experimental` settings section is deleted, not defaulted on.** That
   removes the off-branches instead of leaving them unreachable.
2. **The surfaces mount unconditionally.** The terminal bearer token, the
   terminal, pop-up terminal, and agent routes, and every title-bar mode
   segment always exist. Availability is the only axis: a build that cannot run
   tmux or a PTY explains itself inside the mode rather than disabling its
   segment. `agents-unavailable.spec.ts` and `terminal-unavailable.spec.ts`
   guard this.
3. **A settings migration drops the retired key** (`dropExperimentalSection`
   in `configmigrate/sets.go`, settings version 2), so a file from a user who
   opted in still loads without an unknown-key warning.
4. **The Grafana connectors and `sources.exec` declare `connector.Stable`.**

## Consequences

- `experimental` is no longer a place for a ships-dark flag. A feature that
  needs one reintroduces a section and its own migration.
