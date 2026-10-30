# Code and Chats preload after the hub is up

- **Status:** accepted
- **Date:** 2026-10-30

## Context

The first entry to Code or Chats in a run fetched the mode's chunk, then waited on
an availability probe, with no page on screen. In Code that left about 200 ms
with no sidebar. Keeping xterm out of the initial bundle (ADR
terminal-mode-is-hidden-not-unmounted) is about the cost of startup, not about
making the first click pay.

## Decision

**Half a second after the shell is up, the hub fetches both mode chunks and reads
what they first show:** tmux availability, the session list, the two settings the
tree waits on, the workspaces and the chats. **Then the Code view mounts hidden**
and builds its tree: one status read and one window sweep, then no more polling. A
first entry is a reveal, like every later one. Chats still mounts on first entry,
because its tree needs no second read to take its shape.

- **Preparing attaches nothing.** It opens no tmux client and resumes no chat. The
  attach and its route restore keep following `active`, so a hidden mount cannot
  navigate away from the hub. The tree's reads do spawn tmux, once per run.
- **The tree paints once.** Rows wait for the session list, the window listing and
  the first statuses. Statuses decide which repos open by default, so a tree that
  painted before them grew in visible steps after the first paint.
- **A mount that finds a preloaded client counts as a reveal** (ADR
  revalidation-on-entering-a-mode-waits-until-the-mode-has-painted). It now has
  data on screen to protect, so its revalidation waits for the paint.
- **A click that beats the preload sees a placeholder** with the sidebar's stored
  width, then the toolbar spinner until the tree is ready.

## Consequences

- Every run pays for the chunks, these reads and one hidden mount of the Code view,
  including a run that never leaves the hub.
- This amends "mounted the first time it is entered" in ADR
  terminal-mode-is-hidden-not-unmounted. The chunk is still out of the initial
  bundle.
- The terminal pane is still cold on the first entry, because its attach waits for
  the click.
