# Peer agent windows run configured profiles in the existing checkout

- **Status:** accepted
- **Date:** 2026-09-29

## Context

Running a second agent on the same checkout needs another configured
profile. Creating a Hive session would create a separate checkout, and
replaying its spawn rules would recreate auxiliary windows.

## Decision

The session header's new-window menu offers the configured agent profiles
beside the terminal option. `SessionsService.NewAgentWindow` validates the
Hive session and resolves the profile's current command line, and the tmux
control client (`internal/platform/tmux/control`) starts it in a peer window
at the checkout root with its login-shell and resolved-environment contract.
The operation goes through the authenticated terminal HTTP surface and needs
the tmux session to be running.

## Consequences

Peers share files, branch state, and the Hive session record; each window
has its own agent process. The launch runs no checkout setup hooks and does
not replay the initial window layout. Agent choices follow the reloaded Hive
config.
