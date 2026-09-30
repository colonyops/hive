# A pinned agent chat is attached by the Code view as an ordinary tmux slug

- **Status:** accepted; slug generation amended by [ADR agent-workspace-tmux-sessions-use-persisted-random-ids](2026-09-21-agent-workspace-tmux-sessions-use-persisted-random-ids.md)
- **Date:** 2026-08-06

## Context

An agent chat and a hive session are worked on together — the chat is where the
agent runs, the session is where its output is read — but they lived in two
areas the title bar switches between, so watching one while working in the other
meant a mode flip per glance.

The two are already the same kind of object. ADR agent-workspace-sessions-are-tmux-sessions made a chat a tmux session
named `agentws-<terminal id>` on the same stream and the same control plane
terminal mode uses, and nothing on the attach or sweep path checks a slug
against hive's session list.

## Decision

1. **A pinned chat is a `TerminalSessionRow` and rides the Code view's own
   attach pool.** No second pane, no second transport, no bridge to the Agents
   area's hand-rolled xterm wiring: it is the trick the scratch terminal already
   uses — a row of the same shape, so the pool, the window sweep, the keyboard
   walk and the rails reach it without learning a second kind of row.

2. **The core declares the slug; the frontend never derives it.**
   `SessionView.Slug` carries `agentws-<terminal id>` whether or not the chat is running,
   because a row needs a stable pool and route key while stopped. `TerminalID`
   keeps its existing job of reporting liveness, so the two are not conflated.
   Deriving the name in TypeScript would put the core's tmux naming scheme in a
   second place.

3. **Starting a pinned chat is the Agents area's resume, not hive's spawn.**
   There is no hive session behind an `agentws-*` slug for
   `TerminalsService.Start` to read a spawn configuration from, so the Code view
   calls `ResumeSession` instead. It stays an offered action behind the pane's
   own button (ADR terminal-start-is-an-offered-action) — relaunching runs an agent.

4. **A pin is view state in `localStorage`, not a column on the chat.** It is an
   arrangement of one sidebar, the same class of thing as that sidebar's group
   expansion and running-only filter, and it is read by both areas from one
   module singleton. The listing is what prunes a pin whose chat was deleted —
   only once it has actually loaded, since an empty `recents` is also what a
   gated-off Agents area looks like.

5. **A chat row is a leaf.** A chat is one conversation; its tmux window is how
   that is carried rather than something to navigate between, so the row lists
   no windows even with "always show windows" on.

6. **`TerminalSessionGroup.pinned` becomes a closed `kind` union** — `chats`,
   `scratch`, `repo`. The flag had been standing for two unrelated things,
   "pinned above the repositories and exempt from the filters" and "renders as
   the scratch section", and the chats section is the first case that is the one
   without being the other.

7. **Both areas may hold the chat open.** Only one of them is ever on screen,
   and the pane that is not being looked at is the one that loses the stream.

## Consequences

- **The chat's liveness comes from the window sweep, not hive's status
  projection**, which has no row to answer for it — the same path the scratch
  terminal takes. The Code view therefore reloads the Agents area's cross-
  workspace listing on activation; it is the only reader of it in this mode.
- **A pinned chat and the Agents pane fight over the stream if both attach.**
  `tmuxcc`'s broker holds one subscriber per session (`broker.subscribe` closes
  the previous one), and both modes stay mounted once entered
  (ADR terminal-mode-is-hidden-not-unmounted), so opening the chat in one area ends the other's socket. This is
  accepted rather than fixed: the loser drops to a state that offers a
  reattach, the agent process is untouched, and fanning one subscription out to
  several consumers is a change to the broker that nothing else needs yet. If
  a real use for two live views appears, that fan-out is the fix — not an
  exclusivity rule in the frontend.
- **Pins do not survive a cleared webview store**, and they are per-machine.
  Both follow from point 4 and are the accepted price of not migrating the
  schema for a sidebar arrangement.
- **The Agents area stays the owner of a chat's lifecycle.** The Code view's row
  menu offers only what the pin created — the way back, and unpinning — so
  rename, stop and delete have exactly one home.
