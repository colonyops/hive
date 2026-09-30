# devserver

A development-only proxy between desktop instances and the GitHub API. It does two things:

1. **Caches GitHub responses** across every dev instance and every restart, so N worktrees share one rate-limit budget instead of each burning their own.
2. **Simulates events** — config-driven overlays rewrite what GitHub appears to say, and a webhook pusher injects payloads that never came from GitHub at all. A dashboard drives both.

Not shipped, not imported by the app, loopback-only. Design rationale: [ADR devserver-github-proxy](../../docs/decisions/2026-07-26-devserver-github-proxy.md).

## Quick start

```bash
mise run desktop:devserver     # starts on 127.0.0.1:7777; leave it running
mise run desktop:dev   # already routed through it
```

**Development is proxied by default.** `cmd/desktop/devtools prepare` writes the API base into this worktree's `launch.env`, so there is nothing to export and no flag to remember. Open <http://127.0.0.1:7777> for the dashboard.

**One proxy serves every worktree.** It is a singleton by design: overlay state lives in the process holding the port, so a second one would mean the dashboard you are looking at might not control the instance you are watching.

Launching devserver when one is already running is not an error — the second process **stands by**, retrying the port every `--standby-poll` (2s), and takes over the moment the live one stops. So `mise run desktop:devserver` is safe to run from anywhere without checking first, and closing whichever session happened to start the proxy does not leave the other worktrees without one. A foreign process on the port is still fatal: waiting on it would leave instances pointed at something that is not a proxy.

Nothing preflights the proxy. If it is not running, GitHub calls fail as transport errors in the desktop log, same as any other GitHub failure.

`solo up` runs both tabs from the checked-in `.solo.yml`.

### Running against real GitHub

Put this in the gitignored `overrides.env` beside `launch.env` (mise loads it second, so it wins):

```
HIVE_DESKTOP_DEVELOPMENT_GITHUB_API_BASE=""
```

An empty value means the app talks to api.github.com directly.

### What it does not do on startup

The shipped config declares **no overlays**, so starting devserver never rewrites what a connected instance sees — you get the caching proxy, which is the whole fix for the rate-limit problem, and nothing changes until you click something. A proxy that silently faked data the moment it started would make every subsequent bug suspect.

The app logs a warning at startup when the override is set, and every proxied response carries an `X-Devserver-Outcome` header (`hit`, `miss`, `revalidated`, `stale`, `passthrough`).

## Why it helps

GitHub's limits are **per token**, so concurrent dev instances share one budget. Three things make development worse than the shipped app:

- `LiveProvider`'s cache is in-memory, and `wails3 dev` restarts the Go process on every backend edit
- each worktree owns an isolated data root (ADR desktop-configuration), so two instances share no fetch results
- `ConfirmTerminal` batches every absent item into one aliased GraphQL request per 100, but the client never caches it — it fires fresh every tick

devserver also stores ETags and revalidates with `If-None-Match`. A 304 costs no primary quota, so even an expired entry is usually far cheaper than a refetch. The client itself has none of this ([#62](https://github.com/hay-kot/hive-desktop/issues/62)).

## Config

`mise run desktop:devserver` passes the checked-in `cmd/desktop/devserver/devserver.yaml`. Edit it freely — it is a development default, not a fixture.

This is the only config location. Development configuration is versioned alongside the code whose behaviour it simulates, so there is deliberately no per-user file in a home directory — invisible local state changing what a dev instance sees is the failure mode this avoids. It is also the default when `--config` is absent; an explicit `--config` that does not exist is an error rather than a silent fallback.

`listen` is the address every worktree's `launch.env` is pointed at, so changing it here moves every instance with it.

Every section is optional — the annotated shipped file is the reference.

```yaml
listen: 127.0.0.1:7777
upstream: https://api.github.com
cache:
  ttl: 5m                       # how long before revalidating

# Not set in the shipped config — anything here applies from the first request,
# before you have touched the dashboard. Add entries only to pin a state you
# want back on every restart.
overlays:
  - match: {repo: hay-kot/hive-desktop, num: 58}
    set:
      reason: approval_requested
      labels: [needs-review]

# Multi-step lifecycle sequences are not configured. They are composed and run
# at runtime through POST /_ctl/scenario — see the Control API below.

webhooks:
  # Commented out in the shipped config: the desktop's webhook port is random
  # per install, so no committed URL can be right.
  targets:
    - name: local-desktop
      url: http://127.0.0.1:24681/hooks/devserver   # from Settings ▸ Webhooks
      secret: dev-secret                            # the node's X-Hive-Secret
  payloads:
    pr-opened:
      id: devserver-pr-1
      kind: PR
      repo: acme/widgets
      title: Add retry to fetch
      url: https://github.com/acme/widgets/pull/1
      state: open
```

Overlay state is in-memory. Config seeds it; the dashboard and control API mutate it; a restart returns to exactly what the file says. A debugging session cannot leave permanent fake data behind.

Overlays, actions, and scenarios all target an item by `repo` and `num`, and only an item the proxy has already observed will exist to rewrite. Start devserver, let an instance poll once, and read the repo and number from the dashboard or `GET /_ctl/state` — that is the item list.

### Mutation fields

| Field | Effect |
| --- | --- |
| `state` | `open`, `closed`, or `merged` — drives lifecycle transitions |
| `labels` | Replaces the label set |
| `reason` | Notification reason; becomes the feed's activity summary |
| `title`, `body` | Display text |
| `draft` | Marks a PR draft |
| `absent` | Removes from search results while the batched GraphQL state lookup still answers |
| `updated_at` | Overrides the timestamp; auto-stamped to now when omitted |

`updated_at` is auto-stamped because the classifier ignores any change that does not advance it — an un-stamped mutation would be silently invisible.

## What "simulating an event" actually means

The desktop's `sources.github` reads exactly four things: `state`, `updatedAt`, `labels`, and a notification `reason`. It never fetches review state. So the action vocabulary is GitHub's own:

| Action | What the app sees |
| --- | --- |
| `review-requested` | "Review requested" |
| `approval-requested` | "Approval requested" |
| `comment` | "New comment activity" |
| `ci-activity` | "CI status changed" |
| `mention` | "Mentioned on GitHub" |
| `state-change` | "State changed on GitHub" |
| `draft` / `ready` | Draft flag |
| `merge` / `close` | Terminal transition + drops out of search |
| `reopen` | Back to open, back into search |

**There is no `approve` action.** GitHub has no such notification reason, and the app never reads review state — an approval reaches it as activity on the item. Inventing a richer vocabulary would teach a wrong model of how the app behaves.

`merge` and `close` also set `absent`, because that is how GitHub behaves once an item leaves an `is:open` query, and it is the only way to exercise the desktop's `ConfirmAbsence` path. One overlay is rendered consistently into all three response shapes the app reads — search nodes, notification entries, and the batched GraphQL state lookup — so a simulated merge cannot produce a state the real API could never return.

## Control API

The dashboard is a thin renderer over these, and an agent drives the same surface directly. All under `/_ctl/`.

```bash
# preflight: is this the build under test, and has an app connected yet?
# health reports {devserver, appConnected, requests, itemsObserved, ...};
# version reports the VCS revision + dirty flag Go stamped into the binary.
curl localhost:7777/_ctl/health
curl localhost:7777/_ctl/version

# everything live: observed items, overlays, running scenarios, targets, payloads
curl localhost:7777/_ctl/state

# quick action
curl -XPOST localhost:7777/_ctl/action \
  -d '{"repo":"hay-kot/hive-desktop","num":58,"action":"merge"}'

# arbitrary mutation
curl -XPOST localhost:7777/_ctl/overlay \
  -d '{"repo":"hay-kot/hive-desktop","num":58,"set":{"state":"open","labels":["wip"]}}'

# a multi-step scenario, composed inline and run in the background; each step is
# an action or a raw set, with an optional wait. Steps accumulate onto the item.
# Returns {"scenario":"scenario-1"}; watch it under scenarios in /_ctl/state.
curl -XPOST localhost:7777/_ctl/scenario -d '{"steps":[
  {"repo":"hay-kot/hive-desktop","num":58,"action":"review-requested"},
  {"wait":"60s"},
  {"repo":"hay-kot/hive-desktop","num":58,"set":{"state":"merged","absent":true}}
]}'

curl -XPOST localhost:7777/_ctl/overlay/clear -d '{"repo":"hay-kot/hive-desktop","num":58}'
curl -XPOST localhost:7777/_ctl/overlays/clear
curl -XPOST localhost:7777/_ctl/cache/purge

# push a webhook to a configured target; overrides merge over the named payload
curl -XPOST localhost:7777/_ctl/webhooks/push \
  -d '{"target":"local-desktop","payload":"pr-opened","overrides":{"state":"resolved"}}'

# or to an inline target — the only way to reach a desktop instance, whose
# webhook port is random per install. target may be a name or a {url, secret}.
curl -XPOST localhost:7777/_ctl/webhooks/push \
  -d '{"target":{"url":"http://127.0.0.1:24681/hooks/devserver","secret":"dev-secret"},
       "body":{"id":"x","title":"Inline","state":"open"}}'
```

Unknown JSON fields are rejected, so a typo in a hand-written call fails loudly instead of silently doing nothing. Inline webhook targets must be loopback.

## Webhook pusher

Targets point at a running instance's local webhook listener. Get the base URL from **Settings ▸ Webhooks** (the port is random per install — see ADR local-webhook-listener) and append a `sources.webhook` node's path. The `secret` must match that node's configured secret.

A payload keyed on a stable top-level `id` updates the same inbox item on re-delivery; without one the listener hashes the body, so every push is a new item. Payloads that follow the [canonical item contract](../../docs/decisions/2026-07-24-canonical-item-contract.md) render as first-party feed rows.

## Cache location and lifetime

The cache lives at `$XDG_CACHE_HOME/hive/devserver/cache.db` (`~/.cache/hive/devserver/cache.db` by default), and **survives restarts** — that is why it is SQLite rather than a map. Entries keep their fetch time, so a restart inside the TTL serves straight from disk, and one past it revalidates with `If-None-Match` for a 304 that costs no quota.

The path is deliberately independent of the desktop's data root. That root is the app's state, and ADR desktop-configuration gives each worktree an isolated copy that `dev:reset` exists to delete — deriving the cache from it would give every worktree its own cache and discard it on reset, losing both the sharing and the persistence the cache exists for. Override with `cache.path` in config if you want it elsewhere.

Overlays, observed items, stats, and the activity log are all in-memory and reset on restart. Only responses persist. Delete the file or hit **purge cache** any time; it costs one API call per entry to rebuild.

## Scope

Three endpoints are cached: `POST /graphql`, `GET /notifications`, `GET /user`. `POST /graphql` carries two rewritten document shapes — the batched search and the batched state lookup (`repository.issueOrPullRequest`) that `ConfirmTerminal` sends — and `GET /notifications` is rewritten too; `GET /user` is cached but never rewritten. Everything else passes straight through, so a new client call keeps working without devserver knowing about it.

Only 2xx responses are cached — caching a 401 would outlive the bad token, and caching a rate-limit 403 would extend the outage past its own reset. When upstream fails and a cached copy exists, it is served stale rather than failing the instance.

The cache key includes a hash of the bearer token, so two accounts pointed at one devserver never see each other's data. The token is hashed, never stored.

The OAuth device flow is **not** proxied — `development.github.api_base` only moves the API base. Sign-in still goes to github.com.
