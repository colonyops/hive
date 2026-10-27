# Hive env overrides resolve through the login shell

- **Status:** accepted
- **Date:** 2026-08-06

## Context

Hive picks the default agent from `HIVE_DEFAULT_AGENT` first and
`agents.default` second. A launched `.app` has the launcher's environment, so a
variable the user exports from `.zshrc` is invisible to it, and the desktop's
New Session form preselected the wrong agent. `execenv` already asks the login
shell for its environment once per run (ADR subprocess-environment).

## Decision

1. **`execenv.Resolver.Getenv` answers a variable the way the user's terminal
   would**: this process's value when set, the login shell's otherwise. An
   explicit launch value is more specific than a startup file, so it wins.
2. **The probe keeps the whole environment it collects** from
   `$SHELL -ilc /usr/bin/env`. Same single probe, same remembered failure.
3. **The override applies only when it names a configured agent profile**
   (`environmentAgentOverride`), hive's own rule. An unknown name is ignored.
   It is applied when the desktop loads hive's config and in
   `SessionsService.SessionLaunchOptions`.
4. **The probe is warmed in the background at startup**, so the first click
   that needs it does not pay for shell startup. Mock and e2e runs skip it.

## Consequences

- The form preselects what hive would run.
- The value is the login shell's as of app startup. A profile edited mid-run is
  picked up on the next launch.
- Hive's other env overrides (`HIVE_CONTEXT_BASE_DIR`, `HIVE_GIT_PATH`) still
  resolve at config load from the process environment, so a Dock launch does not
  see them. `Getenv` is where they would go.
- Children inherit the merged shell environment
  (ADR a-subprocess-inherits-the-whole-shell-environment-not-just-its-path).
- One login shell starts on every non-mock launch.
