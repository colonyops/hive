# Terminal mode ships dark behind an experimental settings opt-in

- **Status:** superseded by [terminal-agents-grafana-and-commands-graduate-out-of-experimental](2026-08-07-terminal-agents-grafana-and-commands-graduate-out-of-experimental.md)
- **Date:** 2026-07-29

> **Superseded (2026-08-07):** the gate is deleted, taking the whole
> `experimental` section with it (ADR terminal-agents-grafana-and-commands-graduate-out-of-experimental) — the graduation this ADR's
> consequences named. Point 3's surviving half is the rule that a rendered mode
> segment is never disabled, because an unavailable terminal explains itself
> inside the mode; `Available` is now the only axis. Point 2 is moot: with no
> flag there is no off state for the routes to be absent in.

## Context

Terminal mode (ADR terminal-transport) is merging ahead of its follow-ups — session sidebar,
appearance controls — and should reach users only deliberately. The app has no
feature-flag mechanism; `settings.yaml` is the one user-facing configuration
surface, and the terminal's transport is already composed per run in
`desktop/main.go`.

## Decision

1. **The gate is `experimental.terminal` in `settings.yaml`**, a new
   `experimental` section for features that ship dark. It defaults to off, has
   an `HIVE_DESKTOP_EXPERIMENTAL_TERMINAL` override like every other setting,
   and is read once at startup — flipping it takes a relaunch, because the
   surfaces it governs are mounted at composition time.

2. **Off means the terminal does not exist, not that it is unavailable.** The
   composition root mints no bearer token, `httpapi` registers no
   `/api/terminal/…` operations — so the route index and OpenAPI document never
   advertise routes that cannot be authorized — and the WebSocket stream is not
   mounted. An empty token in `httpapi.New` is the off signal; the per-request
   token guard stays as defense in depth.

3. **The frontend gate is `wailsui.TerminalService.Enabled`, a separate axis
   from `Available`.** When off, the Hub|Terminal toggle is not rendered at
   all. The rule that the toggle is never disabled is scoped, not revised: it
   governs only the enabled-but-unavailable case, which still explains itself
   inside the mode. `Available` also answers unavailable-with-a-reason when
   off, so a stray entry into the mode degrades the same way as a missing
   tmux. The core stays transport- and flag-free: `app.TerminalsService` does
   not know the setting exists.

## Consequences

- The feature merges and releases without exposure; enabling it is an explicit
  settings edit. The e2e harness opts in (`serve.sh`) so the unavailable-state
  spec keeps exercising the enabled-but-unavailable path.
- The `experimental` section is a place for future ships-dark flags; each is a
  startup read, not a live toggle. Graduating a feature means deleting its
  flag, not defaulting it on.
- Settings ▸ System ▸ Experimental exposes the flag as a switch. It only
  persists the value — the gate stays a startup read — so the UI compares the
  persisted value against what the running process mounted
  (`TerminalService.Enabled`) and shows a restart-pending hint while they
  differ. An earlier revision of this ADR skipped the UI toggle entirely;
  that was revised once the mode had a sidebar and appearance controls worth
  reaching without a YAML edit.
