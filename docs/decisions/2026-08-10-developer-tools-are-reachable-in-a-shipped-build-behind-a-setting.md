# Developer tools are reachable in a shipped build behind a setting

- **Status:** accepted; decisions 4 and 5 superseded by [runtime-observability-belongs-in-settings-and-samples-the-ui-while-open](2026-09-18-runtime-observability-belongs-in-settings-and-samples-the-ui-while-open.md)
- **Date:** 2026-08-10

## Context

The developer-tools pane existed only under `import.meta.env.DEV`: its route
record, its component import, and the strip that links to it were all compiled
out of a production bundle. That was right while the pane only sent test
notifications, and it is wrong now that it reports what the app costs the
machine.

A Vite dev build is not the artifact whose memory and CPU are worth reading.
It serves unminified sources with HMR attached, runs a second server in-process,
and is usually pointed at the devserver proxy — so its numbers answer a
question nobody asked. The build worth eyeballing before a release is the signed
one, and there was no way to open the panel there.

pprof (ADR pprof-debug-endpoint) covers the Go side of the same need and is
already settings-gated rather than build-gated, for the same reason.

## Decision

1. **`development.devtools.enabled` gates the pane, defaulting off.** It sits
   beside `pprof` and `perf` in the development settings and takes
   `HIVE_DESKTOP_DEVELOPMENT_DEVTOOLS_ENABLED`, so a signed build can be
   launched with the panel on for one session without persisting anything.

2. **The `/dev` route is registered in every build; the pane is the gate.**
   The flag arrives from the backend after the router is constructed, so a
   compile-time route list cannot express it. `App.vue` renders the pane only
   when the tools are allowed and pushes the route back to the feed otherwise.
   The component stays a lazy chunk, so a build with the flag off never fetches
   it.

3. **The dev strip stays Vite-only, and the command palette is the way in.**
   The strip is a development build's own chrome — a coloured bar identifying a
   worktree — and has no business in a shipped window. `Open developer tools`
   appears in the palette exactly when the flag is on.

4. **Process metrics come from gopsutil, not from the Go runtime alone.**
   `runtime.MemStats` describes the Go heap, and the Go runtime cannot report
   RSS at all — the number the OS charges for, and the one Activity Monitor
   shows. A panel built on `MemStats` alone would read far below it on a
   cgo-heavy shell and hide the native growth. `internal/app/procstats` reports
   both halves side by side and walks the process tree, capped, so a terminal's
   shell or an agent counts against the app rather than disappearing.

5. **The UI-thread samplers run app-wide, not with the pane.** Frame times come
   from a `requestAnimationFrame` loop and event-loop lag from the overshoot of
   a 250ms probe; both start at boot under the same gate. A sampler that ran
   only while the pane was open would measure the pane, and the pane is never
   what janks. High-frequency samples land in plain arrays and are published to
   Vue once per probe tick, so reactivity is not in the path of the thing being
   measured.

## Consequences

- **The webview is not measured, and the panel says so.** On macOS the WebKit
  processes that render the UI are XPC services parented to launchd; their only
  link back to the app is a responsible-pid the public API does not expose, and
  a `WebContent` process on the machine may belong to another app entirely.
  Attributing them would mean a private API or a guess, so the tree walk
  reports what it can prove and the pane states the exclusion rather than
  quietly under-reporting.
- CPU is a rate differenced against the previous sample, so the sampler is
  stateful and the poll cadence is what the rate is measured over. Pausing and
  resuming makes the next sample cover the whole gap.
- gopsutil is a new direct dependency. It is cgo-free on the platforms shipped
  here, but its darwin process reader uses the cgo path when cgo is enabled —
  which it is, because Wails requires it — and would otherwise shell out to
  `ps`.
- A build with the flag on exposes process names and pids of the tree below the
  app. It is loopback-free — nothing is transmitted — but it is more than a
  shipped build otherwise shows, which is why it is off by default.
- The round-trip measurement calls `Ping` and `Echo`, two service methods that
  exist only to be timed. They are on the RPC surface of every build; both are
  trivial and `Echo` caps its payload. It times batches rather than single
  calls: WebKit clamps `performance.now()` to about a millisecond and a round
  trip costs a fraction of one, so per-call samples were 0 or 1 and their
  percentiles were quantisation.
- WebKit ships neither the Long Tasks nor the Long Animation Frame observer, so
  a long frame is a duration with no attribution — the pane can say a frame took
  184ms but not what held the thread. `performance.memory` is absent too, so
  there is no JS heap figure beside the Go one.
- The frame sampler keeps the page animating, which stops the webview idling
  down. It is gated off by default and pauses while the window is occluded, but
  a build launched with the tools on will use marginally more power than one
  without.
