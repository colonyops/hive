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

A merge is only possible when a second event is already waiting, which means
the subscriber is behind. A consumer keeping up sees every event whole, with no
added latency. The batching window is exactly how far behind the consumer is.

Three bounds:

- **Never into the head.** `pump` hands its own copy of `buf[0]` to the
  subscriber and charges the backlog for the size it read, so growing it would
  lose bytes and corrupt the accounting. Merging requires `len(buf) >= 2`.
- **Never across panes.** A frame carries one window and pane id.
- **64 KiB per merged frame** (`maxCoalescedBytes`).

The merge copies into a fresh buffer, because the tail's slice can alias one
the paint gate or decoder still owns. The tail's timestamp is kept, so reported
latency stamps the oldest byte in the frame.

## Consequences

- A subscriber gets a byte stream, not a sequence of frames matching what was
  published. Tests assert bytes, not event counts
  (`TestBrokerCoalescesQueuedOutputWithoutLosingBytes`,
  `TestBrokerNeverCoalescesAcrossWindows`).
- Byte accounting is unchanged, so the overflow bound trips at the same volume.
