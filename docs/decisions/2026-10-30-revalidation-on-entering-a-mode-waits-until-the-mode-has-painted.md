# Revalidation on entering a mode waits until the mode has painted

- **Status:** accepted
- **Date:** 2026-10-30

## Context

The Code view and Chats stay mounted and hidden (ADR
terminal-mode-is-hidden-not-unmounted), so entering one is a `v-show` flip. It also
revalidated everything it shows, in the tick of the flip: the session list, tmux
availability, statuses, git status, the editor, window listings, agent profiles.
Against a real stack that held the first frame for about 190 ms in WKWebView.

The page's own thread stayed idle through the stall, and hiding every paint surface
did not shorten it. Delaying the binding calls by 500 ms removed it, and they cost
no visible stall when they landed later. The exact mechanism is inferred, not
proven.

## Decision

**`usePaintedActive` follows `active` and turns true two frames plus 50 ms after a
reveal, and background revalidation watches it.** Attaching and restoring a session
stay on `active`, because they are what the frame shows. The first answer is
immediate, because a first load has nothing on screen to protect.

## Consequences

- A switch's first frames take 16 ms. Revalidation still costs a frame of 56-77 ms
  about 120 ms later, when nothing on screen is moving.
- Data on screen for the first 100 ms of a re-entry is the cached answer. It was
  already, since the old calls took longer than that to return.
- A new refresh on entering a mode should watch this rather than `active`.
- Chats swaps its terminal rather than pooling it, so the swap holds the outgoing
  one on screen until the incoming one draws (`retire` in `useXtermPane`), capped
  at 300 ms for a chat with an empty screen. The empty pane went from about 90 ms
  to none, and text from 122 ms to 68 ms.
