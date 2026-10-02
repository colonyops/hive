---
kind: added
---

**Routing new items is one trace.** With `telemetry` on, each pass the flow
engine makes over the event log reports as a trace, with one step per flow and
the database commit beneath it, so a feed that updates late resolves to the
flow that spent the time. The Grafana dashboard's Sources tab charts the pass
beside the poll tick.
