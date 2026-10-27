# Development GitHub proxy and event simulator

- **Status:** accepted
- **Date:** 2026-07-26

## Context

GitHub's rate limits are scoped to the token, so every concurrent development instance draws from one
budget. Development multiplies the cost: `wails3 dev` restarts the Go process on every backend edit,
discarding the in-memory fetch cache, and each worktree owns an isolated instance
(ADR desktop-configuration) that shares no fetch results with the others. An in-client cache cannot fix
either, because both are about state that cannot be shared between processes.

Exercising lifecycle workflows (review activity, then a merge) also needs a way to make GitHub say
something without waiting for real state to change.

## Decision

A development-only binary, `cmd/desktop/devserver` (`mise run desktop:devserver`), sits between
development instances and GitHub. It is not shipped, not imported by the app, and holds no credentials.

1. **Caching reverse proxy.** Responses are cached in SQLite at `$XDG_CACHE_HOME/hive/devserver/cache.db`,
   shared by every instance and independent of the per-worktree data root, which `dev:fresh` and
   `dev:reset` delete. `singleflight` collapses concurrent identical requests. Only 2xx responses are
   cached: a cached 401 would outlive the bad token, a cached rate-limit 403 would outlive its reset.
2. **The cache key includes a SHA-256 of the bearer token** (never the token itself), since responses are
   account-scoped, and the request body, since every search is a `POST /graphql` to the same URL.
3. **Expired entries revalidate with `If-None-Match`.** A 304 costs no primary quota.
4. **Overlays rewrite responses on egress.** One `Mutations` value renders into every shape the desktop
   reads an item through: a batched GraphQL search, a batched GraphQL state lookup
   (`issueOrPullRequest`), and REST `/notifications` entries. Consistency across shapes is the
   load-bearing property. A simulated merge must drop the item from `is:open` search (`absent`) and make
   the state lookup report it merged, or it produces a state the real API never returns. `absent` is also
   the only way to exercise absence confirmation.
5. **The action vocabulary is GitHub's.** The GitHub classifier reads only `state`, `reason`, `updatedAt`,
   and `labels`, so those are the levers. There is no "approve" action: GitHub has no such notification
   reason and the desktop never fetches review state.
6. **A webhook pusher** delivers JSON to a running instance's webhook listener with `X-Hive-Secret`
   (ADR local-webhook-listener).
7. **A dashboard** at the listen root renders a 2s poll of `/_ctl/state` and drives the control API
   (ADR devserver-agent-control-api).
8. **One checked-in config**, `cmd/desktop/devserver/devserver.yaml`, and no per-user file. It declares no
   overlays, so an instance never sees fabricated data until someone asks for it. It is the source of
   truth for the listen address that every worktree's `launch.env` points at.
9. **One proxy shared by every worktree**, because `singleflight` and the overlay store are per-process.
   A launch that finds the port held by another devserver stands by, retrying the bind, and takes over
   when the live one stops. The bind is the arbiter; the probe only identifies the occupant, so a foreign
   process on the port is fatal.
10. **Every development run is proxied by default.** `cmd/desktop/devtools prepare` writes
    `HIVE_DESKTOP_DEVELOPMENT_GITHUB_API_BASE` into `launch.env`; setting it empty in `overrides.env` opts
    out. There is no preflight; a missing proxy shows as transport errors in the desktop log.
11. **The setting is `development.github.api_base`**, applied with `ghclient.WithAPIBase` on the one client
    that backs both fetching and the connect flow, so traffic cannot split between the proxy and GitHub.
    `Settings.Validate()` rejects anything but an `http`/`https` URL on a loopback host, on both the
    persisted and the effective value.
12. **The OAuth base is never redirected.** The device-flow token exchange does not pass through dev
    tooling and draws no rate-limit budget.

## Consequences

- Restarting devserver does not refetch, since the cache is on disk.
- Overlay state is in-memory and global across worktrees, keyed by `repo#num`. A restart or takeover
  returns to what `devserver.yaml` declares; a debugging session cannot leave permanent fake data behind.
- A proxied instance sees possibly stale, possibly rewritten data. Startup logs a warning when the API base
  is overridden, and every proxied response carries `X-Devserver-Outcome`.
- A second proxy is possible only with an explicit `--listen`. It shares the cache safely but not
  `singleflight` or overlays.
- A devserver launch never returns on its own; when one is already up it blocks in standby.
- devserver knows three route kinds (`POST /graphql`, `GET /notifications`, `GET /user`). Everything else
  passes through uncached and unrewritten.
- The dashboard HTML and JS have no automated tests; the control API they consume does.
