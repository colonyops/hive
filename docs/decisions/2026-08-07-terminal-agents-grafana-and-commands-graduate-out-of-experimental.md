# Terminal, Agents, Grafana and Commands graduate out of experimental

- **Status:** accepted
- **Date:** 2026-08-07

## Context

Four features carried an experimental marking, through two unrelated
mechanisms.

Terminal mode and the Agents area shipped dark behind `experimental.terminal`
and `experimental.agents` (ADR terminal-experimental-gate). Both gates have
done their job: the follow-ups they were waiting on — the session sidebar,
appearance controls, the workspace manifest and its autonomy postures — have
landed, and the surfaces behind them are the ones the app is now built around.
What the gates cost is no longer a hypothetical: the flags are read at startup,
so every interaction with them is a settings edit plus a relaunch, and the
route groups, the bearer token, and the title-bar segments each carry an
independent off-branch that nothing in a shipped build ever takes.

The Grafana connectors (`sources.grafana_metrics`, `sources.grafana_alerts`,
`sources.grafana_irm_alerts`) and the command source (`sources.exec`) declared
`connector.Experimental`, which the Integrations screen renders as a badge. The
declaration is about config shape, not about whether a feature is reachable —
and these connectors' config shapes have settled.

## Decision

1. **The `experimental` settings section is deleted, not defaulted on.** That
   is what ADR terminal-experimental-gate said graduation would mean, and it is
   the only outcome that removes the off-branches rather than leaving them
   unreachable. The struct, the two `HIVE_DESKTOP_EXPERIMENTAL_*` overrides,
   the Wails settings methods, the `Enabled` probes on `TerminalService` and
   `AgentsService`, and the `ExperimentalToggle` component all go with it.

2. **The surfaces mount unconditionally.** `desktop/main.go` always mints the
   terminal bearer token and CORS allowlist; `httpapi` always registers the
   terminal, pop-up terminal and agent operations; the title bar always renders
   all three mode segments. `Available` stays the one axis it always was —
   a build that cannot run tmux or a PTY explains itself inside the mode, which
   is the rule the e2e unavailable specs guard.

3. **A settings migration drops the retired key.** The settings decoder is
   strict, so a `settings.yaml` carrying `experimental:` — which is exactly the
   set of files belonging to users who opted in, since the section marshals
   with `omitempty` — would fail startup outright. `SettingsSet` moves to
   version 2 with a step that deletes the key.

4. **The four connectors declare `connector.Stable`.** PostHog stays
   experimental; it is new, and nothing about this decision applies to it.

## Consequences

- The e2e harness loses its per-server `experimental.agents` opt-in and the
  server that existed only to carry it (port 8938). `agents-unavailable.spec.ts`
  now runs in the ordinary chromium and webkit projects, alongside
  `terminal-unavailable.spec.ts`, which it mirrors.
- `agents-disabled.spec.ts` is deleted: it asserted the off state of a gate
  that no longer exists.
- `experimental` is no longer a place to put a ships-dark flag. A feature that
  needs one reintroduces the section and its own migration; the shape to copy
  is in this change's history rather than in an empty struct kept for the
  purpose.
- A settings.yaml written by this build carries `version: 2`, which a build
  older than this one refuses to read (`ErrVersionTooNew`). That is the
  forward-only contract every migrated file type already has.
