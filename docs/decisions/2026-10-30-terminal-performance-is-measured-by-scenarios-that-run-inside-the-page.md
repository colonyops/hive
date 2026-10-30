# Terminal performance is measured by scenarios that run inside the page

- **Status:** accepted
- **Date:** 2026-10-30

## Context

ADR ui-performance-spans-are-recorded-to-jsonl gives an agent a way to time a path
it suspects. It does not say how the terminal behaves as a whole, or whether a
change moved it. A headless benchmark cannot answer that: the cost is split across
tmux, Go, the socket, and the webview's own processes, and the webview that matters
is WKWebView, which Playwright cannot drive.

## Decision

1. **Probes are always on when recording is.** Each pane records `input:echo`,
   `output:burst`, and `scroll:gesture`, and the page records `window`, long tasks,
   route changes, and requests, at one span per human action or per 10 s. A probe
   reads the gate at creation, so a shipped build installs nothing.

2. **Scenarios run inside the page against real tmux**
   (`lib/perf/terminalBench.ts`, `window.__hiveTerminalBench`), so one script
   measures both lanes: `mise run desktop:perf:native` for the WKWebView on a real
   display, and `mise run desktop:perf:bench` for Chromium in Docker.

3. **The native lane isolates by socket and data root.** `perf-native.sh` sets
   `TMUX_TMPDIR` and unsets `TMUX`, because the app honors an inherited socket.
   The server build gains tmux only with the `serverterminal` tag, so the plain
   e2e lane still asserts the unavailable UI.

4. **Aggregation stays out of the app.** `cmd/tools/perfreport` reads the file
   afterwards and compares against a baseline.

## Consequences

- Native numbers need a visible window, because a covered WKWebView fires no
  animation frames. Runs flag this as `hiddenDuringRun`.
- Software GL makes the Docker lane's frame times useless as absolutes. It asserts
  only counts of wrong frames, which GL speed cannot excuse.
- `perf-native.sh` builds the production frontend: Vite's dev build costs more per
  frame than the code being measured.
