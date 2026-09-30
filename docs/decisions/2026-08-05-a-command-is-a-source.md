# A command is a source: `sources.exec` runs a CLI on the tick and ingests its stdout as a snapshot

- **Status:** proposed
- **Date:** 2026-08-05
- **Issue:** [#237](https://github.com/hay-kot/hive-desktop/issues/237)

## Context

Every system Hive has no first-party connector for has to be bridged from
outside the app: an external scheduler runs a script, the script POSTs to
`sources.webhook`, and — because a delivery has no presence semantics — the
script keeps its own state file to work out what changed and to synthesize the
terminal `state` that archives an item. That is the plumbing a first-party
source exists to remove, reintroduced one bridge at a time.

Two properties are what make an in-app command node worth having over that
workaround. It runs on the app's tick, so a scheduler that dies is visible
instead of leaving a feed that is silently stale — the worst failure mode an
alerting feed has. And its output is a snapshot, so absence is authoritative
and an entity that stops appearing leaves the feed, which is what
`sources.grafana_alerts` already has and a webhook structurally cannot.

The connector shape itself is settled (ADR source-connector-registry). What was not settled is the
four things a node that runs a command has to answer.

## Decision

### 1. The trust boundary is the config directory, and it does not move

**No gate, no sandbox, no opt-in setting.** The command runs as the user, with
the user's environment.

The boundary this node sits on already exists and already has two doors:
`actions.yml` carries a shell executor whose commands run automatically on
ingested items, and `flows/*.yaml` carries `function` nodes running
author-trusted JavaScript. A gate on the third door while the first two stand
open is not a defense; it is a setting that makes a node quietly do nothing.
Anyone who can write a flow file can already write an action.

What we do instead is make the boundary explicit rather than implicit:

- **The command is a fixed string and is never templated.** `actions.shell`
  renders its command from the message payload; a source has no inbound
  message, so there is nothing to interpolate — and this ADR makes that an
  invariant rather than an accident. Nothing ingested, fetched, or otherwise
  off-machine can influence what runs. This is the property that keeps the node
  as trusted as the file it is written in and no more, and it is why `env`
  values are literal.
- **It says what it is.** The node editor states that it runs a command on this
  machine every poll, and its documentation says to treat a flow file from
  elsewhere the way you would treat a shell script from the same place. A user
  importing a dotfiles repo is the realistic threat, and the answer to it is
  that they can see the command.
- **Experimental stability**, so the config shape can still move.

### 2. Resolved PATH, not a login shell per run

The command runs `sh -c` with `execenv`'s resolved environment (ADR subprocess-environment, ADR
0068), plus optional `cwd`, `env`, and a **required** `timeout`.

The issue proposed matching the pop-up terminal's login shell so aliases and
PATH resolve. Half of that is right and half is not. A poll tick runs forever on
a 60s–5m cadence: paying `$SHELL -ilc` startup on every run for every exec node
charges each one the user's version-manager initialization, and an rc file that
takes a second makes the timeout a coin flip. `execenv` exists precisely for
this — it probes the login shell **once per run** and remembers the PATH — so
the property worth having is bought once instead of per tick.

Aliases are deliberately not supported. An alias is interactive-shell sugar, and
a command in a config file that depends on one is not reproducible on another
machine. `sh -c` is also exactly what `actions.shell` already runs, so the two
places a user writes a command behave the same way.

`timeout` is required and capped at **2m**. Required because a command with no
deadline can wedge the poll loop, and "the source silently stopped producing" is
the failure this node exists to remove. Capped because one tick drains every
pull source in sequence, so a timeout is spent out of every other source's
freshness.

### 3. A broken run is an error, and it is announced once

`Produce` returns an error — emitting nothing — on a non-zero exit, a timeout,
empty stdout, stdout that is not a JSON array, an item with no `id`, duplicate
ids, a non-object entry, or output over 1 MiB. Only a successful exit whose
stdout parses completely is a snapshot; `[]` is the one way to say "empty".

This is the decision that must not be got wrong. `PullSource.Produce` treats a
successful call as authoritative: the producer records the emitted set and
reconciles everything missing from it away. A broken command that returned nil
would therefore archive every item the source owns, and the user would see an
emptied feed rather than a failure.

Three consequences fall out of that and are load-bearing:

- **Parse before emitting.** The whole output is decoded before the first
  `emit`, so a run cannot half-succeed. A partially emitted snapshot would tell
  the producer that the items the command never reached are gone.
- **Oversized stdout fails rather than truncates.** Truncated JSON that happened
  to parse is a *short* snapshot, which is the silent-archive failure wearing a
  different hat. This is also why the format is one JSON array rather than
  NDJSON: a truncated array cannot parse, while truncated NDJSON parses fine as
  fewer items.
- **`null` is rejected explicitly**, because it unmarshals into a slice without
  error — as nothing. `jq` prints it for a missing key, so it is the likeliest
  broken-pipeline output there is.

Failures surface in Activity as `RefreshFailed` with the exit status and a
stderr excerpt. The producer now announces a source's failure **at most once an
hour until it succeeds**, re-arming on the first successful tick. A source that
stays broken is one condition, not one per tick; at the 60s floor, recording
every failure buries a day of real events under 1440 copies of one line, which
is how an audit log stops being read. Suppression is keyed by source rather than
by message, because failure text routinely carries a timestamp or request id
from the tool that produced it and comparing text would defeat itself on exactly
the persistently broken source this exists for.

### 4. Per-instance cadence lives in the producer, not in the connector

`connector.Instance` gains `MinInterval`, and the producer skips an instance
until its floor expires. The exec config's `interval` sets it; every other
connector leaves it zero and is drained every tick exactly as before.

The alternative was for the connector to gate itself inside `Produce` and return
a cached snapshot between runs. That is worse in a way that matters: a cached
snapshot is a *claim about the present* made from stale data, so a re-emitted
cache asserts that items which may have resolved minutes ago are still current —
and the cache dies on restart. Skipping is honest by construction. **A source
that is not drained is not a source drained empty**: `Produce` is not called at
all, no snapshot is appended, and nothing about its tracked set changes.

Three properties of the floor, chosen deliberately:

- **It is quantized to the global tick.** There is one ticker
  (`settings.polling.interval`, floor 60s) and this adds no second scheduler. An
  `interval: 1h` on a 5m tick runs every 12th tick, not on the hour.
- **A failed run still claims its slot.** The floor rate-limits running the
  command, not succeeding at it, so an hourly command that is broken is not
  retried every tick.
- **It is not persisted.** A restart runs every source once, which is what a
  user expects from launching the app, and persisting it would mean a schema
  change to save a rate limit.

A manual refresh (`RefreshSources`) bypasses every floor through the new
`Producer.Refresh`: a user who asked for a refresh gets one.

### 5. One item contract, per-item messages

Stdout is a **JSON array of objects**. Each object needs a top-level `id`
(string or number) which becomes `msg.Key`; `title` and `url` are promoted for
rendering; `kind` is the `applies_to` label; `state` drives lifecycle with
`resolved`/`closed`/`done` terminal. The object is forwarded verbatim as
`msg.Payload`. That is the `sources.webhook` delivery contract exactly, so a
payload written for one works in the other — and the classifier is now literally
the same code, extracted to `sources/canonical`.

Two deliberate differences from webhook, both forced by absence being meaningful
here:

- **`id` is required**, where a webhook falls back to the body's content hash.
  Under a content hash, an item whose payload changes at all becomes a *new*
  item and the old one is archived as absent — an alert with a ticking duration
  field would churn a fresh item every poll. A webhook has no absence semantics
  to get wrong; a snapshot does.
- **The connector declares `CapConfirmAbsence`**, marking every departed item
  `resolved` and terminal, exactly as `sources.grafana_alerts` does for an alert
  that leaves the firing set.

**This deviates from the issue's sketch, intentionally.** The issue emits the
whole stdout as one message and mints per-entity keys in a downstream `function`
node, per the fan-out pattern of ADR function-node-per-entity-feed-items. That pattern carries three costs a
first-party source should not pay: a synthesized item's payload is frozen at
first appearance, it records no `inbox_event` (so it can never notify), and a
departed one is unfiled rather than archived with a reason. Emitting one message
per item from the source gives the fan-out's benefit with none of them, and it
is what `sources.github` and `sources.grafana_alerts` already do. A `function`
node downstream is still free to reshape or split further.

## Consequences

- Any CLI that can print JSON is a source, with correct feed lifecycle and no
  external scheduler, webhook endpoint, or state file. The bridge described in
  #237 and #117 stops being the answer for sources that do not exist yet.
- `MinInterval` is now the answer for any connector whose cost does not suit the
  global tick, not just this one. It is a floor, not a schedule: nothing here
  gives a node its own cron, and a use case that genuinely needs "at 09:00" is
  not served by this and should not be bent into it.
- The canonical `state` vocabulary lives in `sources/canonical` and is shared by
  webhook and exec. A third connector whose payload is user-shaped JSON uses the
  same classifier rather than a fourth interpretation of "resolved".
- `Producer.Tick` and `Producer.Refresh` now mean different things. A caller who
  wants "drain everything now" must use `Refresh`; the scheduled loop must not.
- A command that exits 0 with a truthfully-shaped but *incomplete* list will
  archive what it omitted. That is inherent to a snapshot source and is the
  user's contract to keep — the failure modes we can detect are detected, and
  this one is documented rather than guessed at.
- Activity is quieter and slightly later: a failure that recurs within the hour
  is not re-recorded, so the log shows when a source broke rather than how often
  it has been broken. The debug log still carries every occurrence.
