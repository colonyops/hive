# Peer agent windows run configured profiles in the existing checkout

- **Status:** accepted
- **Date:** 2026-09-29

## Context

The session header can add a shell, but running two different agents on the
same checkout requires selecting another configured profile. Creating a Hive
session would create a separate checkout; replaying its spawn rules would
also recreate auxiliary windows.

## Decision

Offer configured agent profiles beside the terminal option in the header's
new-window menu. `SessionsService` validates the Hive session and resolves
the selected profile's current command and flags. `tmuxcc` starts that command
in a peer window at the checkout root, using its existing login-shell and
resolved-environment contract. The operation uses the authenticated terminal
HTTP surface and requires the tmux session to be running.

## Consequences

Peers share files, branch state, and the Hive session record. Each window has
its own agent process. The launch does not run checkout setup hooks or replay
the repository's initial window layout. Agent choices and commands follow
the reloaded Hive configuration.
