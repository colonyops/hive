# Agent workspace tmux sessions use persisted random ids

- **Status:** accepted
- **Date:** 2026-09-21

## Context

A chat used its SQLite row id in the machine-wide tmux name
`agentws-<id>`. Row ids are unique inside one database, but development
worktrees use isolated databases while sharing the user's tmux server. Two
instances could therefore claim the same name, and tmux correctly refused the
second launch rather than attach it to another instance's agent.

The agent conversation id is not the terminal identity. It can change when a
dead chat relaunches without resume support, while the chat record and its tmux
address stay the same.

## Decision

Each chat record stores an immutable `terminal_id`. New records receive an
eight-character lowercase alphanumeric id from `randid.Generate`; their tmux
name is `agentws-<terminal_id>`. The database enforces uniqueness within an
instance.

The migration backfills existing records with their decimal row ids. Existing
live names such as `agentws-3` therefore remain valid. The row id remains the
HTTP and canvas identity, and `agent_session_id` remains the agent CLI's
conversation identity.

## Consequences

New tmux names stay short and readable while avoiding collisions between
isolated databases. The random space contains 36^8 values. A cross-instance
collision remains statistically possible, and tmux still rejects it without
attaching to or killing the other session.
