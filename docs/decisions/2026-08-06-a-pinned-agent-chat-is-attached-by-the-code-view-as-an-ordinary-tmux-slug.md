# A pinned agent chat is attached by the Code view as an ordinary tmux slug

- **Status:** accepted
- **Date:** 2026-08-06

## Context

An agent chat and a hive session are often worked on together, but they live in
two areas, so watching one while working in the other meant a mode switch per
glance. A chat is already a tmux session (`agentws-<terminal id>`,
ADR agent-workspace-sessions-are-tmux-sessions) on the same stream and control
plane the Code view uses, and nothing on the attach or sweep path checks a slug
against hive's session list.

## Decision

1. **A pinned chat is a `TerminalSessionRow` in the Code view's own attach
   pool**, the same trick as the scratch terminal: a row of the same shape, so
   the pool, sweep, keyboard walk, and rails carry it unchanged.
2. **The core declares the slug.** `SessionView.Slug` carries
   `agentws-<terminal id>` whether or not the chat is running, so a stopped row
   has a stable key; `TerminalID` still reports liveness. The frontend never
   derives the name. How the terminal id is minted is
   ADR agent-workspace-tmux-sessions-use-persisted-random-ids.
3. **Starting a pinned chat is the Agents area's `ResumeSession`**, not
   `TerminalsService.Start`, because no hive spawn configuration stands behind
   the slug. It stays an offered action (ADR terminal-start-is-an-offered-action).
4. **A pin is view state in `localStorage`**
   (`hive.terminal.sidebar.pinned-chats`), not a column on the chat. The chat
   listing prunes a deleted chat's pin, but only once the listing has loaded.
5. **A chat row is a leaf.** It lists no windows, even with "always show
   windows" on.
6. **`TerminalSessionGroup.kind` is a closed union**: `chats`, `scratch`,
   `repo`.
7. **Both areas may hold the chat open.** Only one is on screen at a time.

## Consequences

- A pinned chat's liveness comes from the window sweep. The Code view reloads
  the cross-workspace chat listing on activation.
- The broker holds one subscriber per session (`broker.subscribe` closes the
  previous one), and both modes stay mounted
  (ADR terminal-mode-is-hidden-not-unmounted), so attaching in one area ends the
  other's socket. The loser offers a reattach; the agent is untouched. If two
  live views are ever needed, fan out in the broker rather than adding an
  exclusivity rule in the frontend.
- Pins are per machine and do not survive a cleared webview store.
- The Agents area owns a chat's lifecycle. The Code view's row menu offers only
  the way back to the chat and unpinning.
