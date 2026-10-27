# Developer tools are reachable in a shipped build behind a setting

- **Status:** accepted
- **Date:** 2026-08-10

## Context

The developer-tools pane was compiled out of production bundles. A Vite dev
build is not the artifact worth measuring before a release (unminified, HMR,
usually behind the devserver proxy); the signed build is, and there was no way
to open the pane there. pprof is settings-gated rather than build-gated for the
same reason (ADR pprof-debug-endpoint).

## Decision

1. **`development.devtools.enabled` gates the pane, default off.** It takes
   `HIVE_DESKTOP_DEVELOPMENT_DEVTOOLS_ENABLED`, so a signed build can run with
   the pane on for one launch without persisting anything.
2. **The `/dev` route is registered in every build; the pane is the gate.** The
   flag arrives from the backend after the router exists, so a compile-time
   route list cannot express it. `App.vue` renders `DevView` (an async
   component, never fetched while the flag is off) only when allowed, and
   `useAppNavigation` sends the route back to the feed otherwise.
3. **The dev strip stays Vite-only; the command palette is the way in.**
   `Open developer tools` (`dev:open`) appears exactly when the flag is on.

The runtime dashboard and its samplers moved to Settings > Observability
(ADR runtime-observability-belongs-in-settings-and-samples-the-ui-while-open).

## Consequences

- `DevToolsService.Ping` and `Echo` exist only to time the Wails round trip and
  are on the RPC surface of every build. Both are trivial and `Echo` caps its
  payload. The pane times batches, because WebKit clamps `performance.now()` to
  about a millisecond and one round trip costs less than that.
