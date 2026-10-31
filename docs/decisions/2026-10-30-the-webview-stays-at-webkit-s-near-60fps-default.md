# The webview stays at WebKit's near-60fps default

- **Status:** accepted
- **Date:** 2026-10-30

## Context

ADR the-webview-renders-at-the-display-refresh-rate-through-a-webkit-private-preference
unlocked the webview to 120fps on a ProMotion panel. It shrinks the budget for a
frame from 16.7ms to 8.3ms, and the Changes view does not fit in that. Layout during
a scripted scroll averaged 2.7ms but reached 13ms at p95. A blind comparison with a
real trackpad, one long gesture on each build:

| Changes | 60fps build | 120fps build |
| --- | --- | --- |
| Actual frames per second | 57 | 46 |
| Dropped frames | 13 of 507 | 72 of 244 |
| Frame time p95 | 29ms | 57ms |

The 120fps build delivered fewer frames than the 60fps one and felt rougher. The
preference applies to the whole webview and only before its first load, so it cannot
drop to 60fps during a scroll.

## Decision

The app no longer sets `PreferPageRenderingUpdatesNear60FPSEnabled`, so every webview
renders at WebKit's default of about 60fps.

## Consequences

- Animations and views that do fit in 8.3ms lose 120fps. A terminal pane reached
  118fps on the 120fps build.
- The app uses no private WebKit API for its frame rate.
- Reconsider this once Changes holds 8.3ms at p95. The `scroll:gesture` spans in
  `perf.jsonl` measure it with real input, and the preference still needs setting
  inside the `WKWebView` initializer.
