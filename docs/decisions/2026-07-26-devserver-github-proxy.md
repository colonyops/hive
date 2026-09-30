# Development GitHub proxy and event simulator

- **Status:** accepted
- **Date:** 2026-07-26

## Context

GitHub's API rate limits are scoped to the **token**, not the caller. Every concurrent development instance draws from one 5000/hour budget, and development multiplies that in ways the shipped app never sees:

1. `LiveProvider`'s fetch cache is an in-memory map built fresh in `NewLiveProvider` (`internal/app/sources/github/feed/live.go:83`). `wails3 dev` restarts the Go process on every backend edit, so each edit refetches every source immediately — the persistent worktree-local instance ADR desktop-configuration introduced does not help, because the cache never reaches disk.
2. Each worktree owns an isolated data and config root (ADR desktop-configuration). That isolation is the point — two worktrees must not mutate each other's state — but it also means two instances polling the same queries share no fetch results.
3. `ConfirmTerminal` (`live.go:422`) is unbatched, uncached, and outside the singleflight group — one REST call per item that left a source result, per tick. A churning query produces a burst, which is the shape that trips GitHub's *secondary* limits well before the primary quota is exhausted.

The client also has no ETag support anywhere; the only conditional request is `If-Modified-Since` on `/notifications` (`ghclient/client.go:371`). Since ADR owned-github-client the client is **owned code**, so that is now a local fix rather than a cross-repo round trip — ADR owned-github-client names cache integration as one of the things ownership was meant to unblock, and it remains worth doing (#62).

It would not remove the need for this decision. A client-side cache lives in one process, and points 1 and 2 are about *state that cannot be shared between processes*. Even a perfect in-client cache leaves N instances and every `wails3 dev` restart each paying full price. That is the gap devserver closes, and it is the only part of this problem a better client cannot reach.

Separately, exercising lifecycle workflows (a PR getting review activity, then merging) meant waiting for real GitHub state to change, or hand-editing fixtures. The `sources.webhook` connector (ADRs local-webhook-listener, webhook-listener-placement) can already inject arbitrary events, but only into flows built on webhook nodes — not into the `sources.github` nodes where the interesting classification logic lives.

## Decision

A development-only binary, `cmd/devserver`, that sits between development instances and GitHub. It is not shipped, not imported by the app, and holds no credentials of its own.

1. **Caching reverse proxy.** Responses are cached in SQLite (`modernc.org/sqlite`, plain `database/sql` — `sqlc.yaml` stays scoped to `internal/app/store`) at `$XDG_CACHE_HOME/hive/devserver/cache.db`, shared by every instance pointed at it. The location is the cache dir and is deliberately independent of the desktop's data root: ADR desktop-configuration gives each worktree an isolated instance that `dev:fresh` and `dev:reset` exist to delete, so deriving from it would unshare the cache per worktree and discard it on reset — the two properties the cache exists to provide. `golang.org/x/sync/singleflight` collapses the herd that several instances ticking on the same 60s boundary produce. Only 2xx responses are cached: caching a 401 would outlive the bad token, and caching a rate-limit 403 would extend the outage past its own reset.

2. **The cache key includes a hash of the bearer token.** GitHub responses are account-scoped, and two accounts can point at one devserver. Keying only on the request would serve one account's private data to the other. The token is hashed, never stored. The key also includes the request body, because the desktop batches every search into `POST /graphql` where the URL is identical for all of them.

3. **devserver stores ETags and revalidates with `If-None-Match`.** A 304 does not draw down the primary quota, so an expired entry is usually far cheaper than a fresh fetch. This is headroom the client does not have today, and the proxy provides it without an app change — and keeps providing it across processes if the client later gets its own cache.

4. **Config-driven overlays rewrite responses on the way out.** One `Mutations` value is rendered into all three shapes the desktop reads the same logical item through — GraphQL search nodes, REST `/notifications` entries, and single-item REST responses. Rewriting on egress rather than ingress means an overlay can be changed without purging the cache.

   Overlay consistency across shapes is the load-bearing property. GitHub reports a merged PR as `state=closed, merged=true` on the pulls endpoint and drops it from an `is:open` search; simulating a merge therefore has to do **both**, or it produces a state the real API can never return and the resulting bug looks like an app bug. The `absent` field exists for exactly this, and is the only way to exercise the desktop's absence-confirmation path.

5. **The action vocabulary is GitHub's, not an invented one.** The GitHub connector's classifier reads exactly four things — `state`, `reason`, `updatedAt`, and `labels` (`githubPayload` in `internal/app/sources/github/classify.go`) — so those are the only levers that exist. Notably there is **no "approve" action**: GitHub has no such notification reason and the desktop never fetches review state. An approval reaches the app as activity on the item, so it is simulated as the reason it actually produces. Inventing a richer vocabulary would teach a wrong model of the app.

6. **A webhook pusher** delivers configured JSON payloads to a running instance's local webhook listener, with the `X-Hive-Secret` header ADR local-webhook-listener specified. This is the second, independent lever: the proxy makes GitHub say something different, the pusher injects an event that never came from GitHub at all.

7. **A dashboard** at the listen root drives both, over a JSON control API under `/_ctl/`. Assets are embedded and self-contained; the page is a plain 2s poll of one `/_ctl/state` read.

8. **A checked-in development config**, `cmd/devserver/devserver.yaml`, and no other config location. It ships webhook payloads but **declares no overlays**: a config that rewrote responses from the first request would mean a developer who never opened the dashboard could still be looking at fabricated data, and every subsequent bug would be suspect. (It originally shipped named scenarios too; ADR devserver-agent-control-api moved scenarios to a runtime `POST /_ctl/scenario` and removed the config section.)

   There is deliberately no per-user file in a home directory. Development configuration is versioned alongside the code whose behaviour it simulates, is reviewed with it, and is identical for everyone; a `devserver.yaml` under `$XDG_CONFIG_HOME` would be invisible local state silently changing what a dev instance sees. It is also the source of truth for the listen address, which is what lets `cmd/devtools` point every worktree at the same proxy without a second copy of the port.

9. **One proxy, shared by every worktree.** The response cache is already cross-process (WAL, `busy_timeout`), so a second proxy would not corrupt anything — but `singleflight` collapsing and the overlay store are both per-process. Two proxies would mean the dashboard being clicked might not control the instance being watched, which is a confusing failure that costs more than the sharing is worth.

   The singleton is made robust rather than assumed. A launch that finds the port held by one of its own does not exit — it stands by, retrying the bind on an interval, and takes over when the live proxy stops. That makes `mise run devserver` safe to run from any worktree without checking first, and it means the worktree that happened to start the proxy is not the one that has to stay open: whichever session is still standing by inherits it. The bind is the arbiter rather than the probe, so simultaneous launches need no coordination — exactly one wins and the rest go back to waiting. The probe only identifies the occupant, which is what keeps a foreign process on the port fatal rather than something waited on forever.

10. **Every development run is proxied by default.** `cmd/devtools prepare` writes the API base into the worktree's `launch.env`, so opting in costs nothing and the shared budget is the default rather than the thing someone remembers. Opting out is setting the same variable empty in the gitignored `overrides.env`, which mise loads second.

    There is no preflight. `dev` once probed the proxy before starting Wails and failed with both remedies spelled out, but the check earned its keep only in the window where a proxy was missing *and* stayed missing — and standby launches closed most of that window, since any parked devserver takes the port over. A proxy that is not answering now surfaces as transport errors in the desktop log, which is where every other GitHub failure already surfaces.

11. **The setting is `development.github.api_base`** (`HIVE_DESKTOP_DEVELOPMENT_GITHUB_API_BASE`), applied via the client's `WithAPIBase` option in `ghsource.NewProductionClient`. One client template backs both the fetch layer and the connect flow, so a redirected instance cannot split its traffic between the proxy and real GitHub.

    This is a development setting rather than a bare environment variable, which reverses the position an earlier draft of this ADR took. That draft was written before ADR desktop-configuration: there was no `development` namespace, so "not a setting" was the only way to say "not a user-facing knob". ADR desktop-configuration created a better place to say it — `development.*` is where dev-only configuration lives, alongside `mocks.mode`, which is at least as consequential — and being a typed setting is what buys the guarantee below.

12. **Loopback only, enforced by validation.** `Settings.Validate()` rejects any `api_base` that is not an `http`/`https` URL on a loopback host, following the fail-closed posture ADR desktop-configuration set for every dev-only listener. Because validation runs on the persisted value *and* on the effective value after environment overrides, this holds no matter where the value came from.

    This is a stronger guarantee than the env-only design it replaces, which read whatever string it was handed. devserver fronts a GitHub token and can change what an instance sees; neither belongs on a LAN, and now nothing can put them there.

13. **The OAuth base is never redirected.** Only the API base moves. A device-flow token exchange has no business passing through dev tooling, and it draws no rate-limit budget, so redirecting it would be all risk and no benefit. Sign-in reaches github.com even on a fully proxied instance.

## Consequences

- N instances plus their restarts cost one upstream request per unique call per TTL, which was the original problem. Restarting devserver itself does not refetch, since the cache is on disk.
- Overlay state is **in-memory only**: config seeds it, the control API mutates it, and a restart returns to exactly what `devserver.yaml` declares. A debugging session cannot leave permanent fake data behind, at the cost of not being able to save a mutation without editing config.
- An instance pointed at devserver is seeing possibly-stale, possibly-rewritten data by construction. Startup logs a warning naming the base and whether it came from the environment or from `settings.yaml`, and every proxied response carries `X-Devserver-Outcome`.
- The override is reachable through `settings.yaml`, which the env-only design made impossible. Loopback validation bounds the consequence to "this machine", and a stray value is visible in a file and in the startup log rather than in invisible process state. Accepted deliberately; it is the cost of the namespace being typed and validated.
- Development now depends on a running devserver by default. That is the point — it is what makes the shared budget automatic rather than opt-in — but it means one more process to have running, and forgetting it surfaces as failed GitHub calls in the desktop log rather than as a message before launch.
- Overlay state is shared across worktrees along with the proxy. Overlays are keyed by `repo#num`, so a simulation set up while debugging one branch is visible to every other instance. This is a consequence of the singleton, not an accident; clearing from the dashboard is global, and a restart returns to the config's declared state.
- The singleton owns the port, so a second proxy is possible only with an explicit `--listen`. That is available for isolating a risky overlay experiment, and it shares the cache safely, but it is isolation rather than scale: `singleflight` and overlays do not span processes.
- A devserver launch never returns on its own. Standby processes hold no cache handle and cost one bind plus one loopback probe per interval, so leaving them parked is cheap, but `mise run devserver` blocks rather than exiting 0 when one is already up — a script that ran it expecting to return needs its own backgrounding. Takeover is not state transfer: overlays live in the process that held the port, so the inheriting proxy starts from the config's declared state.
- `launch.env` gained a required key, so a worktree prepared before this change reports a missing key and needs `mise run dev:fresh` once.
- devserver knows five route kinds (`POST /graphql`, `/notifications`, `/user`, and single-item issue/pull GETs). Anything else passes through untouched, so a new client call keeps working — uncached and un-rewritten — without devserver needing to know about it.
- The client defects in #62 remain. devserver works around them for development and does not fix them for users; an in-client persisted cache is still worth having, and ADR owned-github-client made it a local change. The two do not conflict — devserver's value is cross-process sharing, which stays useful afterward.
- The dashboard's HTML and JS are not covered by automated tests. The Go control API it consumes is; the page is a thin renderer over it.
