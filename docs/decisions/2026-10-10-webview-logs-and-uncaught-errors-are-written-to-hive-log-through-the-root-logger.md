# Webview logs and uncaught errors are written to hive.log through the root logger

- **Status:** accepted
- **Date:** 2026-10-10

## Context

The frontend had no error capture. An uncaught exception, an unhandled
rejection, or a Vue render error reached only the devtools console, which a
release build never shows. The ~70 `console.warn` calls after failed binding
calls went to the same place. Wails' own system log and the panics it recovers
from bound methods went to stderr in development and were discarded in a
production build. None of it reached `hive.log`, so the Diagnostics window and
the problem report could not show it.

## Decision

1. **One sink.** A webview entry is written through the desktop root logger,
   so it reaches `hive.log`, stderr, the telemetry log bridge, Diagnostics and
   the problem report like any Go log line. There is no second file. Wails gets
   the same logger, at warn and above, through `zerolog.NewSlogHandler`.
   That covers the panics Wails recovers from bound methods. `PanicHandler`
   stays unset so any other panic still exits the process.

2. **Context is carried as fields, not resolved.** A webview line carries
   `cmp=webview` or `cmp=webview.<name>`, plus `window`, `route`, `client_time`,
   the error's `stack` and `kind`, and caller fields. Stacks from a production
   bundle stay minified; source maps are not shipped or applied.

3. **The frontend batches and the backend bounds.** `src/lib/log.ts` buffers
   entries and flushes them through one binding, `ClientLogService.Write`.
   Identical entries in a batch collapse into one with a `count`, and an
   overflowing buffer drops the oldest and reports how many. The backend caps
   message, stack, and field sizes, drops reserved keys, clamps the level to
   at most error, and rate-limits writes, logging one line with the number
   suppressed. A failed flush is never logged, so the logger cannot feed
   itself.

4. **The frontend asks for the level once.** Entries below the root logger's
   level are dropped before they cross the binding.

5. **Code logs through `createLogger`, not `console`.** `no-console` allows no
   method; `log.ts` is the one place that writes to the console. The global
   catcher installs before either Vue app mounts, so the main window and the
   Diagnostics window both report.

## Consequences

- `hive.log` grows faster. It has no rotation or size cap yet; adding one is
  a separate change.
- A webview line from a release build points into hashed bundle names. The
  `cmp`, `route`, and caller fields locate the problem; the stack confirms it.
- The binding works unchanged in the browser (server) build, because it rides
  the same `/wails/runtime` transport.
