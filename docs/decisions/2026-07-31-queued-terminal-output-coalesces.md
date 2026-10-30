# Queued terminal output coalesces in the broker

- **Status:** accepted
- **Date:** 2026-07-31

## Context

tmux caps one `%output` notification at about 2 KB, so a pane printing a large
log produces thousands of events per second. Each became one event, one
WebSocket frame, one webview message, one decode and one `term.write`, and
every fixed per-frame cost was multiplied by that rate. Batching on a timer
would add delay to every frame, keystroke echo included.

## Decision

**The broker folds an incoming `Output` into the last queued event when both
are output for the same pane** (`coalesceLocked` in
`internal/platform/tmux/control/broker.go`), under the lock that already guards
the backlog.

A merge is only possible when a second event is already waiting. A subscriber
on loopback almost never falls behind on its own, so inside a burst the pump
waits up to 3 ms (`burstHold`) for a frame to form. Output that follows a
delivery by 2 ms or more (`burstGap`) goes at once, so a keystroke's echo is never
held, and a queued full frame goes at once, so the hold never caps throughput.

Three bounds:

- **Never into a head a pump holds.** `pump` hands its own copy of `buf[0]` to
  the subscriber and charges the backlog for the size it read, so growing it
  then would lose bytes and corrupt the accounting. The `delivering` flag covers
  that window; a head nobody has taken yet merges like any other event.
- **Never across panes.** A frame carries one window and pane id.
- **64 KiB per merged frame** (`maxCoalescedBytes`).

The first merge copies into a buffer of its own, because the tail's slice can
alias one the paint gate or decoder still owns; later merges append to that
buffer. The tail's timestamp is kept, so reported
latency stamps the oldest byte in the frame.

## Consequences

- A subscriber gets a byte stream, not a sequence of frames matching what was
  published. Tests assert bytes, not event counts
  (`TestBrokerCoalescesQueuedOutputWithoutLosingBytes`,
  `TestBrokerNeverCoalescesAcrossWindows`).
- Byte accounting is unchanged, so the overflow bound trips at the same volume.
- `BenchmarkBrokerFlood` reports frames per MB, which went from about 320 to about
  30 with the hold (ADR
  a-terminal-output-burst-waits-briefly-for-a-frame-instead-of-crossing-the-webview-per-notification).
