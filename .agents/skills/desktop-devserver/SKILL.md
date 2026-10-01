---
name: desktop-devserver
description: Drive the local devserver at http://127.0.0.1:7777 to simulate GitHub lifecycle events and webhook deliveries against a running Hive Desktop instance. Use when asked to run a scenario, simulate a PR/issue event (review requested, approved, merged, CI, comment, mention), exercise the feed through the dev proxy, or reproduce a lifecycle bug end to end.
compatibility: Requires a running devserver (`mise run desktop:devserver`) and a desktop instance pointed at it (the default in every prepared worktree). curl and jq.
---

# Drive the devserver

`cmd/desktop/devserver` fronts the GitHub API for local development. Beyond caching, it
simulates events two ways, and you drive both over its JSON control API under
`/_ctl/` at `http://127.0.0.1:7777`:

- **Proxy overlays** rewrite what GitHub *appears to say* about an item — its
  state, labels, notification reason, draft flag, or presence in search. This is
  what a `sources.github` node sees.
- **The webhook pusher** injects an event that never came from GitHub at all,
  delivered to a running instance's `sources.webhook` listener.

Pick the lever that matches the flow under test: a bug in GitHub classification
is a proxy overlay; a bug in webhook ingest is a push.

The action vocabulary is deliberately GitHub's own. The desktop reads exactly
four things — `state`, `updatedAt`, `labels`, and a notification `reason` — so
those are the only levers. **There is no `approve` action**: GitHub has no such
notification reason and the app never reads review state, so an approval reaches
it as `approval-requested` activity on the item. Do not invent a richer
vocabulary; it would teach a wrong model of the app.

## 1. Confirm the setup

```bash
# readiness: devserver up, and whether an app has connected + observed items
curl -sf localhost:7777/_ctl/health | jq .   # {devserver, appConnected, requests, itemsObserved, ...}

# is this the build under test? (VCS revision + dirty flag)
curl -s localhost:7777/_ctl/version | jq .    # compare .revision to `git rev-parse HEAD`
```

If `/_ctl/health` fails, devserver is not running — ask the user to start it
with `mise run desktop:devserver` (leave it running). Do not start it yourself in a way
that would block the session; suggest they run `! mise run desktop:devserver` or a
background tab. If `appConnected` is false, no desktop instance has polled yet —
start or wait for one before targeting items. If `/_ctl/version` `.revision`
does not match your working tree, the running proxy is a stale build.

## 2. Find an item to target

Overlays, actions, and scenarios address an item by `repo` and `num`, and only
an item the proxy has **already observed** exists to rewrite. Read the live
state and pick one:

```bash
curl -s localhost:7777/_ctl/state | jq '.items[] | {repo, num, kind, title, state, overlaid}'
```

If the list is empty, the connected instance has not polled yet. Let it tick
once (its floor is 60s) and read again. For webhook pushes the item is invented
by the payload, so no observed item is needed.

## 3. Drive the right lever

### One event

```bash
# a quick action from the vocabulary (see .actions in /_ctl/state for the full list)
curl -XPOST localhost:7777/_ctl/action \
  -d '{"repo":"acme/widgets","num":42,"action":"review-requested"}'

# a raw mutation when an action does not fit
curl -XPOST localhost:7777/_ctl/overlay \
  -d '{"repo":"acme/widgets","num":42,"set":{"labels":["needs-review"],"reason":"comment"}}'
```

Actions and overlays accumulate onto the item; each call merges over the last.
`updatedAt` is auto-stamped, because the classifier ignores a change that does
not advance it.

### A multi-step scenario (compose it for the test)

Build the exact sequence the test needs and POST it. Steps run in the
background; each is an action or a raw `set`, with an optional `wait`. A wait
should exceed the app's 60s poll floor if each step must land as its own event
rather than collapsing into one.

```bash
curl -XPOST localhost:7777/_ctl/scenario -d '{"steps":[
  {"repo":"acme/widgets","num":42,"action":"review-requested"},
  {"wait":"70s"},
  {"repo":"acme/widgets","num":42,"action":"approval-requested"},
  {"wait":"70s"},
  {"repo":"acme/widgets","num":42,"set":{"state":"merged","absent":true}}
]}'
# -> {"scenario":"scenario-1","steps":5}
```

The response returns immediately. Watch it finish in `/_ctl/state`: an in-flight
run appears under `.scenarios` and disappears when done.

```bash
curl -s localhost:7777/_ctl/state | jq '.scenarios'   # [] once the run completes
```

A bad step (unknown action, invalid repo, both `action` and `set`, a step that
does nothing) fails the whole request with a `422` naming the step in `fields`
— read it and fix the step rather than retrying blind.

### A webhook push

The pusher needs a target. The desktop's webhook port is random per install, so
supply it inline as `{url, secret}`. Read the host/port and path prefix from the
app API's `/api/status` (`.webhook.host`, `.webhook.port`, `.webhook.pathPrefix`
— see the **desktop-api** skill) and append a `sources.webhook` node's path;
`secret` must match that node's. Inline targets must be loopback.

```bash
curl -XPOST localhost:7777/_ctl/webhooks/push -d '{
  "target": {"url":"http://127.0.0.1:24681/hooks/devserver","secret":"dev-secret"},
  "body":   {"id":"evt-1","kind":"PR","repo":"acme/widgets","num":42,
             "title":"Add retry","url":"https://github.com/acme/widgets/pull/42","state":"open"}
}'
```

A stable top-level `id` makes a re-push update the same inbox item; drive it to a
terminal `state` (`resolved`/`closed`/`done`) to archive it. Payloads following
the canonical item contract (ADR canonical-item-contract) render as first-party feed rows. A
configured payload can be used instead of an inline body via
`"payload":"pr-opened"` with optional `"overrides":{...}`.

## 4. Verify through the app's MCP tools, then clean up

Confirm the app reacted through its loopback **MCP server** — do not read
`desktop-pipeline.db`. After a proxy overlay/action/scenario, force a re-poll so
you do not wait the 60s floor, then read and retry (the engine commits a moment
after the fetch):

```bash
# refresh_sources reloads now (pull sources), list_inbox reads the result
call refresh_sources
call list_inbox '{"profile":"'"$P"'"}' | jq '.items[] | {title, lifecycle, sourceState, unread}'
```

A webhook push already appends directly, so skip the refresh and just read +
retry. See the **desktop-api** skill for discovering the port, connecting to
`/mcp` (including the `call` helper above), the read tools, and the reload →
read → retry loop in full. Then reset so the next test starts clean:

```bash
curl -XPOST localhost:7777/_ctl/overlay/clear -d '{"repo":"acme/widgets","num":42}'  # one item
curl -XPOST localhost:7777/_ctl/overlays/clear                                       # everything
```

## Guardrails

- **Overlays are global and shared across worktrees.** Clearing is global, and
  every instance pointed at the proxy sees the same overlays. Do not leave a
  simulation in place after a test — clear it.
- **State is in-memory.** A devserver restart drops every overlay and running
  scenario and returns to what `devserver.yaml` declares (no overlays). Nothing
  you set here persists.
- **Loopback only.** Inline webhook targets must be a loopback URL; the endpoint
  rejects anything else.
- **Do not edit `devserver.yaml` to run a scenario.** Scenarios are composed at
  runtime through `/_ctl/scenario`; the config no longer defines them.
