# Tmux launches use owned resource transactions

- **Status:** accepted
- **Date:** 2026-10-08

## Context

Tmux commands can fail after allocating a session, window, or pane. Temporary exit retention can outlive the launch, and a control-client exit does not prove that the session terminated.

## Decision

The tmux exec adapter owns a bounded launch transaction shared by session creation and window addition. It records native IDs before setup, observes command exits with temporary pane-local option leases, and restores those options before commit. Rollback removes only owned resources with a cancellation-independent deadline.

A file lock coordinates launch and existence checks across processes. Pending markers identify interrupted allocations for recovery under that lock. Successful finite commands do not roll back healthy siblings. Surviving the observation interval does not establish readiness.

Clients retain user-visible reports independently of terminal disposal. They confirm session absence through tmux metadata and separate connection loss from termination. Agent detection alone never proves session absence.

## Consequences

Launch errors include available terminal snapshots and incomplete cleanup. Snapshots are bounded terminal output, not complete process logs. Runtime termination without exit metadata has an explicit fallback notice. Desktop reports remain in memory for the current app lifetime; historical failure storage is outside this decision.
