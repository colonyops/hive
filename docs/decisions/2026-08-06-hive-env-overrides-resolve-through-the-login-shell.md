# Hive env overrides resolve through the login shell

- **Status:** accepted
- **Date:** 2026-08-06

## Context

The New Session form always preselected `claude`, whatever the user's terminal
would have started. Hive resolves that agent from `HIVE_DEFAULT_AGENT` first and
`agents.default` second, and re-reads the variable every time its own form opens
(colonyops/hive#365) — the desktop only ever read `agents.default`, whose
env override `config.Load` folds in once, at startup.

Startup is the wrong moment. The desktop loads hive's config from the
environment a launched `.app` has, which is the launcher's: a variable the user
exports from `.zshrc` or a file it sources is not in it. So the override was
either absent (a Dock launch) or frozen at whatever the terminal that ran
`mise run dev` happened to hold.

`internal/app/execenv` already answers this class of question. It asks the login
shell for its environment once per run and keeps the answer
(ADR [subprocess-environment](2026-07-30-subprocess-environment.md)), which is
how a hook that works in a terminal works from the Dock. That ADR deferred
adopting anything but PATH "until a hook demonstrably needs a non-PATH
variable"; `HIVE_DEFAULT_AGENT` is the first one.

## Decision

1. **`execenv.Resolver.Getenv` answers a variable the way the user's terminal
   would**: this process's value when it has one, the login shell's otherwise.
   A launch that names the variable explicitly is more specific than a startup
   file, so the process wins and no shell is started to second-guess it.

2. **The probe now keeps the whole environment it already collected.** It has
   always run `$SHELL -ilc /usr/bin/env` and thrown away everything but PATH;
   the parse is a map instead of a PATH scan. No new subprocess, same single
   probe, same remembered failure.

3. **`SessionLaunchOptions` applies the override.** The variable replaces the
   preselected agent only when it names a configured profile — hive's own rule —
   and an unknown name is ignored rather than refused: preselecting is all this
   does, and hive validates the agent it is handed. The choices themselves stay
   hive's.

4. **The probe is warmed in the background at startup.** Everything the app
   spawns on the user's behalf waits on it, and now so does opening a form. A
   shell that takes a second to start should not charge that to the first click
   that needs it. Mock and e2e runs skip it: a fixture launch must not start the
   machine's shell.

## Consequences

- Changing the default agent in one place — the variable hive already reads, or
  `agents.default` — moves the desktop's form with it. The form preselects what
  hive would have run, which is the whole point.
- The value is the login shell's as of app startup. A shell profile edited
  mid-run is picked up on the next launch, the same restart the CLI needs.
- Hive's other env overrides (`HIVE_CONTEXT_BASE_DIR`, `HIVE_GIT_PATH`) still
  resolve at config load and remain invisible to a Dock launch. They are read
  once, deep inside vendored code, where a per-call lookup does not fit; nothing
  has asked for them yet, and `Getenv` is where they would go.
- ~~`Environ` is unchanged: children still get this process's environment with
  the resolved PATH, not the shell's variables. Adopting those wholesale would
  let a startup file shadow what the app deliberately sets for a child, so the
  shell's environment is read from, never merged in.~~ **Superseded by
  [ADR a-subprocess-inherits-the-whole-shell-environment-not-just-its-path](2026-08-07-a-subprocess-inherits-the-whole-shell-environment-not-just-its-path.md).**
  It is merged in, but not wholesale: this process wins every name it defines,
  which answers the shadowing objection without withholding `EDITOR`.
- One login shell now starts on every non-mock launch, whether or not the run
  goes on to spawn anything. That is the cost of not paying it mid-click.
