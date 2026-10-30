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
tree waits on, the workspaces and the chats. The modes still mount on first entry.

- **Preloading only reads.** It attaches no tmux client and resumes no chat, and it
  does not sweep window listings, which spawn tmux per session. The sidebar's rows
  wait for that sweep on the first entry.
- **A mount that finds a preloaded client counts as a reveal** (ADR
  revalidation-on-entering-a-mode-waits-until-the-mode-has-painted). It now has
  data on screen to protect, so its revalidation waits for the paint.
- **A click that beats the preload sees a placeholder** with the sidebar's stored
  width, so the sidebar column does not move when the mode arrives.

## Consequences

- Every run pays for the chunks and these reads, including one that never leaves
  the hub.
- The first frame of a first entry shows the mode's frame and its status bar.
