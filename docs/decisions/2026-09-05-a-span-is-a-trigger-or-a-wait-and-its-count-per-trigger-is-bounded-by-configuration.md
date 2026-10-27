# A span is a trigger or a wait, and its count per trigger is bounded by configuration

- **Status:** accepted
- **Date:** 2026-09-05

## Context

Without a rule, what deserves a span becomes taste, and two reviewers give
two answers. The failure modes are orphan root spans (an HTTP request with
no parent to say why it happened) and span counts that scale with data size
(one span per SQL statement), which bury the operation they exist to
explain.

## Decision

**A path gets a span when it is a trigger or a wait. Nothing else does.**

1. **Trigger spans are roots.** A distinct cause entering the app: process
   start, a poll tick, a webhook delivery, an action dispatch, a terminal
   attach, an agent tool call. One span per occurrence. A trigger is the
   unit a person asks a question about ("why is my feed stale").

2. **Wait spans are children.** The app hands work to something it does not
   control and waits: an HTTP round trip, a subprocess, a batch database
   write. They use `observe.StartConditionalSpan`, so a wait with no trigger
   above it emits nothing.

3. **A span must pass three tests.**
   - It is a trigger, or it waits on something outside this process.
   - Its duration varies for a reason the parent's duration cannot show.
   - The number of them per trigger is bounded by **configuration**, not by
     **data size**.

   The third test removes per-item spans. The store spans `CommitBatch`
   (`db.CommitBatch`) and carries the output count as an attribute rather
   than opening a span per statement.

4. **What a rejected candidate becomes instead.**

   | Tempting span | What it should be |
   | --- | --- |
   | Pure computation, no wait | nothing, or a metric if the cost matters |
   | One per item in a collection | a count attribute on the enclosing span |
   | Recording a value | an attribute, or a metric if queried over time |
   | Marking that something happened | a log line |
   | A long-lived stream or connection | a metric |

5. **A span name is a bounded search key that says its layer.** Name the
   operation, never the target: `ingest.source github`, not the source id;
   `http.github GET`, not the URL. Unbounded identity (source id,
   repository, session slug) rides as an attribute.

## Consequences

- A slow individual SQL statement is not visible as its own span. Finding
  one means reading the batch duration, then the local `/metrics` scrape or
  a profile.
- Span volume is a bounded handful per trigger, so there is no sampler. Once
  a trigger fires faster than a few times a second a sampler is needed, and
  its ratio belongs in `telemetry`, not at a call site.
