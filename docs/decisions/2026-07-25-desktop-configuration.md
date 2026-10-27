# Typed desktop configuration and worktree-local development instances

- **Status:** accepted
- **Date:** 2026-07-25

## Context

Desktop configuration needs one place where defaults, validation, and environment overrides resolve, so two
callers cannot read the same setting differently and a settings write cannot persist a process-only
override. Desktop-owned variables must not collide with the CLI's. Concurrent worktrees need isolated
paths and ports without touching the installed app.

## Decision

**`settings.yaml` is one nested, typed schema** (`cmd/desktop/internal/app/settings`). Loading applies, in
order: compiled defaults, the migrated YAML document (ADR yaml-config-migration), validation of the
persisted value, then typed `HIVE_DESKTOP_*` environment overrides and validation of the effective value.
An override cannot hide a broken persisted setting. Overrides are process-local: a settings write mutates
the persisted value and reapplies the override instead of writing it to YAML. Unknown keys are ignored
(ADR a-settings-yaml-key-the-build-does-not-declare-is-ignored-not-rejected).

**Startup resolves paths before constructing the app.** The fixed
`$XDG_CONFIG_HOME/hive/desktop/bootstrap.yaml` pointer stores only `data_dir` and `config_dir`, so it stays
discoverable after the config root moves. `HIVE_DESKTOP_DATA_DIR` and `HIVE_DESKTOP_CONFIG_DIR` win over
it, then XDG defaults apply. The derived paths form one immutable `settings.Paths` snapshot injected through
the composition root; runtime services do not read the process environment. `hive.db` resolves separately:
`HIVE_DESKTOP_HIVE_DATA_DIR`, then the CLI's `HIVE_DATA_DIR` from the login shell, then the desktop data
dir.

**Environment ownership is explicit.** Desktop settings use `HIVE_DESKTOP_<NAMESPACE>_<FIELD>`. `XDG_*` is
the platform location convention. `WAILS_*` is framework configuration: the dev launcher translates
`development.vite` and `development.wails` into the variables Wails and Vite read. Credential variables,
build stamping, and release tooling keep their own names.

**Listeners bind loopback only and accept port `0`**, so the OS picks the port without a free-port probe;
the running listener reports the bound endpoint. The shared `http` server is on by default
(ADR local-webhook-listener); `development.pprof` is off by default and mounts on that server.

**Each worktree owns a reusable development instance** in the gitignored `.hive-desktop/` directory:
isolated data and config roots plus a marker binding it to the worktree. `desktop:dev:prepare` creates or
reuses it, `desktop:dev:fresh` deletes and reseeds it, `desktop:dev:reset` deletes it. Destructive
operations require the exact worktree-local path, the ownership marker, and no root symlink; a worktree
lock serializes them, and fresh/reset refuse while a dev server is running. Config snapshots materialize
symlink targets. The agent-workspace root is copied into the instance and pinned with
`HIVE_DESKTOP_AGENT_WORKSPACES_DIR`, so a dev process cannot reopen a File Provider path.
`cmd/desktop/devtools prepare` writes the non-secret `launch.env` (paths and preselected Wails/Vite ports);
`desktop:dev` loads it, then the optional developer-authored `overrides.env`. The Vite host is fixed to
`127.0.0.1` because Wails builds the frontend URL with localhost.

## Consequences

- Missing settings are safe: listeners are loopback-only and pprof is off.
- Invalid YAML values, environment values, ports, hosts, durations, and closed-set values fail startup
  clearly.
- Desktop-owned environment names have no compatibility aliases.
- Wails/Vite port preselection keeps a small time-of-check/time-of-use race because neither framework
  accepts an open listener. App-owned listeners do not have it.
- Dev points `HIVE_DESKTOP_HIVE_DATA_DIR` at the installed hive data dir, so dev sessions land in the real
  `hive.db` and dev runs this checkout's hive migrations against it. `desktop-pipeline.db` and feed state
  stay worktree-isolated. Setting the variable in `overrides.env` re-isolates it.
- The OS keychain and `bootstrap.yaml` are shared with the installed app. Use a mock mode when credential
  isolation matters.
