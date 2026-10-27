# Agent workspace tmux sessions use persisted random ids

- **Status:** accepted
- **Date:** 2026-09-21

## Context

Chats live on the user's one tmux server, but development worktrees use
isolated databases. A tmux name derived from a database id can therefore
collide between instances, and tmux refuses the second launch. The agent
conversation id is not a terminal identity either: it changes when a chat
relaunches without resume support.

## Decision

Each chat record stores an immutable `terminal_id`, unique within an
instance. A new record gets an eight-character lowercase alphanumeric id from
`randid.Generate`, and its tmux session is `agentws-<terminal_id>`. Records
that predate the column kept their old numeric row id as `terminal_id`, so
live names such as `agentws-3` stay valid.

The chat's record id stays the HTTP and canvas identity, and
`agent_session_id` stays the agent CLI's conversation identity.

## Consequences

A cross-instance collision is still possible in a 36^8 space. tmux rejects
it without attaching to or killing the other session.
