# A terminal output burst waits briefly for a frame instead of crossing the webview per notification

- **Status:** accepted
- **Date:** 2026-10-30

## Context

tmux sends a `%output` line per 1.4 KB. A subscriber on loopback takes each frame
before the next one arrives, so the merge in ADR queued-terminal-output-coalesces
almost never fired. A flood reached the webview as about 15000 WebSocket frames a
second, and a 70 MB wall of text cost 24 CPU-seconds, half of it in WebKit's
processes.

The expected cure was credit-based flow control, as herdr and iTerm2 use. The
measurements do not call for it: tmux's own ceiling is about 45 MB/s through a
control client, the webview parses at that rate, a flood queues at most 1 MB, and
Ctrl+C lands in about 140 ms.

## Decision

**Inside a burst, the broker's pump waits up to 3 ms (`burstHold`) for a frame to
form.** Output within 2 ms (`burstGap`) of the last delivery is a burst. A first
delivery, a full 64 KiB frame, and output from a pane that was quiet all go at
once, so a keystroke's echo never waits and the hold never caps throughput.

## Consequences

- The same wall costs 8 CPU-seconds at 43 MB/s, in about 2200 frames rather than
  102000. Quiet-pane echo stays at p50 9 ms, p95 16 ms.
- A TUI's repaint after the first write in a burst can arrive up to 3 ms later.
- A slower machine could outrun the parser and queue in the webview, which nothing
  bounds. If `maxBacklog` on `output:burst` spans shows that, flow control is the
  next step: acks from xterm's write callback and tmux's per-pane
  `refresh-client -A` pause, not `pause-after` alone, because the Go reader always
  drains tmux.
