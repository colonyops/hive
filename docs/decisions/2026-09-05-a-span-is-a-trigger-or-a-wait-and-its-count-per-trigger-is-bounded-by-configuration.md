# A span is a trigger or a wait, and its count per trigger is bounded by configuration

- **Status:** accepted
- **Date:** 2026-09-05

## Context

Traces were adopted with one span, `app.startup`, so nothing had to decide what
else deserved one. The first two additions made the gap obvious. `otelhttp`
produced a root span per source request, with no parent to explain why the
request happened, and the `store` decorator opened a span per SQL statement,
which is a count that scales with how much data arrived rather than with
anything configured.

Without a rule this becomes taste, and taste does not survive code review.
"Instrument the important paths" gives two reviewers two answers.

## Decision

**A path gets a span when it is a trigger or a wait. Nothing else does.**

1. **Trigger spans are roots.** A distinct cause entering the app: process
   start, a poll tick, a webhook delivery, an agent tool call, a command the
   user invoked. One span per occurrence. A trigger span is the unit a person
   asks a question about, which is why the list reads like questions:
   *why is the app slow to open*, *why is my feed stale*, *why did that action
   fire*.

2. **Wait spans are children.** The app hands work to something it does not
   control and waits: an HTTP round trip, a subprocess, a batch database write.
   These use `observe.StartConditionalSpan`, so a wait with no trigger above it
   emits nothing. A background wait nobody asked for is not a trace.

3. **Three tests, all of which must pass.**
   - Is it a trigger, or does it wait on something outside this process?
   - Does its duration vary for a reason the parent's own duration cannot show?
   - Is the number of them per trigger bounded by **configuration** rather than
     by **data size**?

   The third test is the one that decides the cases people get wrong, and it is
   what removed the per-statement SQL span: `CommitBatch` issues several
   statements for each output in its batch and `IngestObservation` runs once per
   ingested item, so a single tick produced spans in the hundreds and buried the
   tick it existed to explain. The count became an attribute on the enclosing
   span instead.

4. **What a rejected candidate becomes instead.**

   | Tempting span | What it should be |
   | --- | --- |
   | Pure computation, no wait | nothing, or a metric if the cost matters |
   | One per item in a collection | a count attribute on the enclosing span |
   | Recording a value | an attribute, or a metric if it is queried over time |
   | Marking that something happened | a log line |
   | A long-lived stream or connection | a metric — a span that lasts a session is not a span |

5. **A span name is a search key, so it is bounded and it says its layer.**
   Name the operation, never the target: `ingest.source github`, not the source
   id a user chose; `http.github GET`, not the URL, because a source path
   carries org and repository names. The layer prefix is what makes a name
   legible on its own — a bare `github GET` in a trace list does not say what
   produced it. Unbounded identity (source id, repository, session slug) rides
   as an attribute, where it costs nothing per series.

## Consequences

- `ingest.tick` is the first trigger span after `app.startup`, and it is what
  makes the rest readable: the `otelhttp` spans stop being orphan roots and the
  conditional `store` spans start firing, both without touching either.
- `store` no longer decorates `DBTX`. It spans `CommitBatch` and carries the
  output count as an attribute. The sqlc query-name extraction went with it,
  which is a real loss: a slow individual statement is no longer visible as its
  own span, and finding one means reading the batch duration and then reaching
  for the local `/metrics` scrape or a profile.
- The remaining trigger spans are not built yet: webhook delivery, action
  dispatch, terminal attach, MCP tool call. Each is additive and none blocks
  another. The frontend's UI spans stay in `perf.jsonl` until the separate
  decision in ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape is taken.
- Span volume is now roughly one root per minute per install plus a bounded
  handful of children, which needs no sampler. A sampler becomes a decision the
  first time a trigger fires faster than a few times a second; the ratio belongs
  in `telemetry`, not at a call site.
