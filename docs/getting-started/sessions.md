---
icon: lucide/layers
---

# Sessions

A hive session is an isolated git clone paired with a terminal environment for running AI agents. Instead of working directly in your main repository — where multiple agents would step on each other's changes — hive creates separate clones so each agent gets its own workspace. When you create a session, hive clones the repository (or reuses a recycled clone), then spawns a tmux session with windows for your AI tools.

Each project (git remote) can have multiple sessions running in parallel, each working on a different task. Sessions can run multiple agents in separate tmux windows — for example, a primary Claude agent alongside a test runner.

```
colonyops/hive                          <-- project (git remote)
├── fix-auth (26kj0c)                   <-- hive session (isolated clone)
│   └── tmux: fix-auth                  <-- tmux session (spawned by hive)
│       ├── window: claude              <-- primary AI agent
│       ├── window: aider              <-- secondary agent (optional)
│       └── window: shell               <-- regular terminal
├── add-tests (x9m2p)
│   └── tmux: add-tests
│       ├── window: claude
│       └── window: shell
└── refactor-config (m4k8w)
    └── tmux: refactor-config
        ├── window: aider               <-- any AI tool works
        └── window: shell

acme-corp/backend                       <-- another project
├── api-migration (p7n3q)
│   └── tmux: api-migration
│       ├── window: claude
│       └── window: shell
└── fix-ci (r2d5t)
    └── tmux: fix-ci
        ├── window: claude
        └── window: shell
```

## Hive Session

An isolated git clone in a dedicated directory with its own terminal environment. Each session is a self-contained workspace for one or more AI agents working on a specific task or feature.

**Key characteristics**:

- Unique 6-character ID (e.g., `26kj0c`)
- Display name (e.g., `fix-auth-bug`)
- Isolated git clone at a specific path
- One or more agent windows (configured via [agent profiles](../configuration/index.md#agents))
- Lifecycle: `active` → `recycled` → `deleted`

**Not to be confused with**: Tmux session (see relationship below)

## Agent

An AI tool instance (Claude, Aider, Codex) running within a session. Each agent runs in its own tmux window and is independently monitored by the TUI.

Sessions support multiple agents — for example, a primary Claude agent alongside a test runner or code reviewer. Configure agent profiles in your config to define which tools are available. See [agent profiles](../configuration/index.md#agents) for setup.

## Tmux Session

A terminal multiplexer session that hosts a hive session. When you create a hive session, hive spawns a tmux session with the same name as the session slug.

**Relationship**: Each hive session spawns a tmux session with the same name. The tmux session contains agent windows (matched by `tmux.preview_window_matcher` patterns) and a `shell` window. See the architecture diagram above.

## Repository

A git remote URL (e.g., `github.com/colonyops/hive`). Multiple sessions can be created from the same repository.

## Session Lifecycle

Sessions move through a managed lifecycle:

1. **Create** — A new session starts as `active`. Hive clones the repository (or reuses a recycled clone) and spawns a tmux session.
2. **Recycle** — When you're done, recycle the session instead of deleting it. Recycling resets the git repository to a clean state and makes it available for reuse. The next `hive new` for the same repository will reuse a recycled session, avoiding a fresh clone.
3. **Delete** — Permanently removes the session directory and all associated data.
4. **Corrupted** — If hive detects an invalid state (e.g., missing directory, broken git repo), the session is marked corrupted and can only be deleted.

## Status Indicators

The TUI shows real-time agent status:

| Indicator | Color            | Meaning                         |
| --------- | ---------------- | ------------------------------- |
| `[●]`     | Green (animated) | Agent actively working          |
| `[!]`     | Yellow           | Agent needs approval/permission |
| `[>]`     | Cyan             | Agent ready for input           |
| `[?]`     | Dim              | Terminal session not found      |
| `[○]`     | Gray             | Session recycled                |
