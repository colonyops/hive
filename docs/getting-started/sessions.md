# Sessions

## Hive Session

An isolated git clone in a dedicated directory, running an AI agent in a terminal. Each session is a self-contained environment for working on a specific task or feature.

**Key characteristics**:

- Unique 6-character ID (e.g., `26kj0c`)
- Display name (e.g., `fix-auth-bug`)
- Isolated git clone at a specific path
- Lifecycle: `active` → `recycled` → `deleted`

**Not to be confused with**: Tmux session (see relationship below)

## Agent

An AI tool instance (Claude, Aider, Codex) running within a session. The agent is the actual AI assistant that processes your requests.

**Future**: Hive will support multiple agents per session (e.g., primary agent + test runner).

## Tmux Session

A terminal multiplexer session that hosts a hive session. When you create a hive session, hive spawns a tmux session with the same name as the session slug.

**Relationship**:

```
Hive Session: fix-auth-bug (ID: 26kj0c)
   ↓ spawns
Tmux Session: fix-auth-bug
   ├─ Window: claude (runs Claude AI agent)
   └─ Window: shell (regular shell)
```

## Repository

A git remote URL (e.g., `github.com/colonyops/hive`). Multiple sessions can be created from the same repository.

## Session Lifecycle

Sessions move through a managed lifecycle:

```
(new) ──► active ──► recycled ──► (deleted)
              │           │
              └──► corrupted ──► (deleted)
```

When you're done with a session, you can **recycle** it instead of deleting it. Recycling resets the git repository to a clean state and makes it available for reuse. When you create a new session, hive will reuse a recycled session if available, avoiding a fresh clone and saving time.

## Status Indicators

The TUI shows real-time agent status:

| Indicator | Color            | Meaning                         |
| --------- | ---------------- | ------------------------------- |
| `[●]`     | Green (animated) | Agent actively working          |
| `[!]`     | Yellow           | Agent needs approval/permission |
| `[>]`     | Cyan             | Agent ready for input           |
| `[?]`     | Dim              | Terminal session not found      |
| `[○]`     | Gray             | Session recycled                |
