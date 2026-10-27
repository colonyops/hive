# A command is a source: `sources.exec` runs a CLI on the tick and ingests its stdout as a snapshot

- **Status:** accepted
- **Date:** 2026-08-05

## Context

A system with no first-party connector had to be bridged from outside: an
external scheduler runs a script that POSTs to `sources.webhook` and keeps its
own state file to work out what disappeared. A command node that runs on the
app's tick makes a dead scheduler visible, and its output is a snapshot, so
absence is authoritative. The connector shape is settled
(ADR source-connector-registry); this ADR settles what a node that runs a
command has to answer.

## Decision

### 1. The trust boundary is the config directory

There is no gate, sandbox, or opt-in. The command runs as the user. Anyone who
can write a flow file can already write a shell action in `actions.yml` or a
`function` node.

- **The command is a fixed string and is never templated.** A source has no
  inbound message, and nothing ingested or fetched can influence what runs.
  `env` values are literal for the same reason.
- **It says what it is.** The node editor and docs state that it runs a command
  on this machine every poll.

### 2. Resolved environment, not a login shell per run

The command runs `sh -c` in `execenv`'s resolved environment
(ADR subprocess-environment), with optional `cwd` (absolute or `~/`), `env`,
and a required `timeout`, capped at 2m. `execenv` probes the login shell once
per run, so per-tick shell startup is avoided. Aliases are not supported.
`timeout` is required because a hung command wedges the poll loop, and capped
because one tick drains every pull source in sequence.

### 3. A broken run is an error, announced once

`Produce` returns an error and emits nothing on a non-zero exit, a timeout,
empty stdout, stdout that is not a JSON array (including `null`), a non-object
entry, an item with no `id`, duplicate ids, or more than 1 MiB of stdout. `[]`
is the only way to say "empty".

A successful `Produce` is authoritative: the producer reconciles everything
missing from it away. A broken run that returned nil would archive the whole
feed. So the output is fully decoded before the first emit, and oversized
stdout fails instead of truncating. The format is one JSON array rather than
NDJSON because a truncated array cannot parse.

Failures surface in Activity as `RefreshFailed` with the exit status and a
stderr excerpt. The producer announces a source's failure at most once an hour
until it succeeds (`failureNoticeInterval`), keyed by source, not by message
text.

### 4. Per-instance cadence lives in the producer

`connector.Instance.MinInterval` is a floor the producer enforces: an instance
inside its floor is not drained at all, so nothing about its tracked set
changes. A cached re-emitted snapshot would assert stale data as current.

- The floor is quantized to the global tick (`settings.polling.interval`).
- A failed run still claims its slot.
- It is not persisted; a restart runs every source once.
- `Producer.Refresh` (behind `RefreshSources`) bypasses every floor;
  `Producer.Tick` does not.

### 5. One item per message, the webhook contract

Each array entry needs a top-level `id` (string or number), which becomes the
message key. `title`, `url`, `kind`, and `state` mean what they mean for
`sources.webhook`; the classifier is shared in `sources/canonical`, where
`resolved`, `closed`, and `done` are terminal. Two differences, both because
absence is meaningful here: `id` is required (a content-hash key would churn a
new item whenever a field changes), and the connector declares
`CapConfirmAbsence`, marking a departed item `resolved`.

The source emits one message per item instead of one message fanned out by a
downstream `function` node, so items get live payloads, inbox events, and
archive reasons.

## Consequences

- `MinInterval` is a floor, not a schedule. A use case that needs "at 09:00" is
  not served by it.
- A third connector whose payload is user-shaped JSON should use
  `sources/canonical` too.
- A command that exits 0 with a well-formed but incomplete list archives what
  it omitted. That is the user's contract to keep.
- Activity shows when a source broke, not how often. The debug log carries every
  occurrence.
