# Typed desktop configuration and worktree-local development instances

- **Status:** accepted
- **Date:** 2026-07-25

## Context

Desktop configuration had grown through unrelated mechanisms: flat keys in
`settings.yaml`, package-level `os.Getenv` calls, `bootstrap.yaml` mutating
legacy environment names, Wails-native server variables, and development
scripts that rebuilt a cache-directory snapshot on every launch. Defaults and
validation lived beside individual consumers, so two callers could resolve the
same setting differently and a settings write could accidentally persist a
process-only override.

Desktop-owned variables also shared names with the external `hive` product
(`HIVE_DATA_DIR`, `HIVE_LOG_LEVEL`) even though the two products have distinct
configuration. Concurrent worktrees needed isolated paths and ports without
mutating the installed app, but cache-backed snapshots were hard to discover,
were discarded on every run, and two launches from one worktree could reset
state out from under each other.

## Decision

**`settings.yaml` is one nested, typed schema.** Its namespaces are `polling`,
`updates`, `notifications`, `appearance`, `webhooks`, `keybindings`, and
`development`. Loading applies, in order: safe compiled defaults, one strict
YAML document with unknown fields rejected, then validation of the persisted
value, then typed `HIVE_DESKTOP_*` environment overrides and validation of the
effective value. An override cannot hide a broken persisted setting.
Environment values are process-local; settings writes mutate the persisted
value and reapply the override rather than materializing it in YAML. The old flat schema and old
desktop-owned variable names have no compatibility aliases.

**Startup resolves paths before constructing the app.** The fixed
`$XDG_CONFIG_HOME/hive/desktop/bootstrap.yaml` pointer still stores only
`data_dir` and `config_dir`, so it remains discoverable after the config root
moves. Explicit `HIVE_DESKTOP_DATA_DIR` and `HIVE_DESKTOP_CONFIG_DIR` values
win over it, then XDG defaults apply. Derived state, config, settings, flows, actions, credentials-index, and log
paths form one immutable `settings.Paths` snapshot injected through the
composition root. Runtime
services do not repeatedly consult process environment.

**Environment ownership is explicit.** Desktop runtime, path, and development
overrides use `HIVE_DESKTOP_<NAMESPACE>_<FIELD>`. `XDG_*` remains the platform
location convention. `WAILS_*` remains framework configuration: the dev
launcher translates `development.vite` and `development.wails` into the
variables Wails and Vite consume before Go starts. Credential/secret variables,
build stamping, release tooling, and vendored Hive variables are not fields in
`settings.yaml` and keep their established names.

**Listeners fail closed.** Webhooks default disabled, bind only a loopback
host, and accept port `0`, allowing the OS to choose the bound port without a
free-port probe. The actual endpoint is reported by the running listener. The
`development.pprof` section has the same disabled, loopback, port-zero-safe
defaults, but the runtime endpoint is deferred until the plugs lifecycle
manager exists; configuration does not justify a bespoke teardown path.

**Each worktree owns a reusable local development instance.** The gitignored
`.hive-desktop/` directory contains isolated data and config roots and a marker
that binds it to that worktree. Setup or `dev:prepare` creates or reuses
it; `dev:fresh` safely deletes and reseeds it; and
`dev:reset` safely deletes it. Destructive operations require the exact
worktree-local path, a regular ownership marker, and no root symlink. An atomic,
stale-recoverable worktree lock serializes these operations, and fresh/reset
refuse while either configured development server is active. Config snapshots materialize symlink
targets instead of preserving links back into installed configuration. The
configured agent-workspace root is snapshotted into the instance separately and
pinned there through `HIVE_DESKTOP_AGENT_WORKSPACES_DIR`, so a configured File
Provider path cannot be reopened by a development process.
`cmd/devtools prepare` atomically writes paths and resolved ports to the
root-level, gitignored `launch.env`. The `dev` mise task loads that
non-secret file followed by optional gitignored `overrides.env`; setup and a
missing-file-only enter hook create `launch.env`, and devtools requires no
special environment-override logic. Wails then starts directly. Application-owned listeners bind port zero directly; devtools
preselects distinct Wails/Vite ports and bridges them to `WAILS_*`. Because
Wails builds the frontend URL with localhost, the Vite host is fixed to the
known-working `127.0.0.1`; the Wails server host remains configurable within
loopback. `cmd/devtools` uses the repository's urfave/cli command convention and
zerolog console output rather than shell orchestration. Framework port
preselection retains a small time-of-check/time-of-use race because Wails and
Vite cannot accept an already-open listener.

## Consequences

- Missing or empty settings are safe: webhooks and pprof are off, listeners are
  loopback-only, development uses live backends, and debug pauses are zero.
- Invalid YAML, environment values, ports, hosts, durations, or closed-set
  values fail startup clearly instead of being silently ignored.
- `bootstrap.yaml` remains a deliberate first-stage pointer rather than part of
  movable `settings.yaml`.
- The settings schema and desktop-owned environment names change without a
  migration. Before alpha, clarity is preferred over retaining the ad hoc
  surface.
  **Update (2026-07-28):** amended in part by ADR yaml-config-migration — config **files**
  (`settings.yaml`, `flows/*.yaml`, `actions.yml`) are now migrated forward in
  place on a breaking schema change instead of hard-failing. The
  no-compatibility-alias stance for **environment variable names** is
  unchanged.
- Wails/Vite port selection still has a small preflight race because those
  frameworks cannot receive an already-open listener; app-owned listeners do
  not have that race.
- Worktree-local data is discoverable and reusable, but it can be large and
  must remain ignored.
- Development data/config/workspaces/ports are isolated; the OS keychain and
  fixed bootstrap pointer are not. Mock mode is the safe default for fully
  isolated development until credential namespaces become instance-aware.
- **Update (2026-07-28):** `hive.db` resolves through a separate `HiveDataDir`
  (defaulting to `DataDir`, so production and e2e are unchanged), and
  `dev` points `HIVE_DESKTOP_HIVE_DATA_DIR` at the installed hive data
  dir. Sessions created in dev therefore land in the real hive database — the
  desktop shares `hive.db` with the CLI in dev as it already does in production
  — while `desktop-pipeline.db` and feed state stay worktree-isolated. The
  accepted cost is that dev runs the vendored hive SHA's migrations against the
  real database; keep the vendored SHA in step with the installed CLI. Setting
  the variable to the worktree data dir in `overrides.env` re-isolates it.
