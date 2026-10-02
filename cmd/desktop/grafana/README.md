# Grafana dashboard

`hive-desktop-overview.json` is the reference dashboard for Hive Desktop's own
telemetry: metrics, logs, and traces over OTLP, plus continuous profiles. It is
a Grafana v2 dashboard resource that [gcx](https://github.com/grafana/gcx)
pushes and pulls. `folder.json` is the "Hive Desktop" folder it lives in.

Use it to watch your own install, or as the worked example of how each signal
the app emits is queried.

## Push it to a stack

You need a gcx context for the stack (`gcx login`) and Hive Desktop exporting
to that stack. [Telemetry
settings](https://hivedesktop.com/configuration/settings/#telemetry) covers the
export side.

```sh
gcx resources push -p grafana --context my-stack
```

This creates or updates the folder and the dashboard (`halj6lf`). On Grafana
Cloud the datasources resolve on their own. On any other Grafana, open the
controls menu (the `+4` beside the variables) and pick the Metrics, Logs,
Traces, and Profiles datasources.

## Change it

Edit the JSON and push it, or edit in Grafana and bring the change back with
`gcx resources pull dashboards/halj6lf -o json`. Drop `metadata.namespace` from
the pulled file before committing it, so it stays portable between stacks.

When a change adds, renames, or removes an instrument, update the panel that
reads it in the same change.

## What each tab reads

Channel is `deployment.environment.name` and Build is `service.version`. Every
panel filters on both.

| Tab      | Question                                          | Signals                                                                                                             |
| -------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Runtime  | How heavy is the app and what it spawned?         | `process.*` gauges (`cmd/desktop/internal/app/procstats`), Go runtime metrics, `target_info`                                    |
| Terminal | Is terminal output keeping up?                    | `tmux.stream.*` (`cmd/desktop/internal/app/tmuxcc`), `terminal.attach` spans                                                    |
| Sources  | Why is my feed stale, and am I near a rate limit? | `ingest.tick` and `ingest.source <kind>` spans, `runtime.drain` and `runtime.flow` spans (`cmd/desktop/internal/app/runtime`), `source.*` and otelhttp metrics (`cmd/desktop/internal/app/sources/...`) |
| Logs     | What went wrong?                                  | The zerolog bridge, as `service_name="hive-desktop"`                                                                |
| Traces   | What triggered work, and how long did it take?    | Root spans, error spans, and HTTP spans with no trigger above them                                                  |
| Profiles | Where do CPU and memory go in the code?           | Pyroscope CPU, heap-in-use, and allocation profiles                                                                 |

## From instrument to query

An OTel instrument name becomes a Prometheus name with dots as underscores,
the unit as a suffix, and `_total` on counters:

| Instrument (unit)                  | Series                                         | Labels                              |
| ---------------------------------- | ---------------------------------------------- | ----------------------------------- |
| `process.cpu.usage` (`%`)          | `process_cpu_usage_percent`                    | `process_scope`: `self`, `descendants` |
| `process.memory.rss` (`By`)        | `process_memory_rss_bytes`                     | `process_scope`                     |
| `process.descendants.count`        | `process_descendants_count`                    |                                     |
| `process.descendants.truncated`    | `process_descendants_truncated`                |                                     |
| `tmux.stream.bytes` (`By`)         | `tmux_stream_bytes_total`                      |                                     |
| `tmux.stream.buffer.depth` (`By`)  | `tmux_stream_buffer_depth_bytes_bucket`        | `le`                                |
| `tmux.stream.frame.latency` (`s`)  | `tmux_stream_frame_latency_seconds_bucket`     | `le`                                |
| `tmux.stream.lifecycle`            | `tmux_stream_lifecycle_total`                  | `state`: `paused`, `resumed`        |
| `source.ratelimit.remaining`       | `source_ratelimit_remaining`                   | `source`                            |
| `source.rss.fetch`                 | `source_rss_fetch_total`                       | `result`                            |
| `http.client.request.duration`     | `http_client_request_duration_seconds_bucket`  | `source`, `http_response_status_code`, `error_type` |

Every series also carries `job="hive-desktop"`, `service_version`,
`deployment_environment_name`, and `instance` (the per-launch
`service.instance.id`). `host.id` is not promoted to a label. It is on
`target_info`, a resource attribute in Tempo, structured metadata in Loki, and
the `host_id` label on profiles.

Some behavior is easy to mistake for a broken panel:

- A counter has no series until it first increments. Backlog pauses and RSS
  fetches stay empty until a pause or an RSS poll happens.
- TraceQL metrics cover at most 25 hours on Grafana Cloud. Trace-derived panels
  error on a longer range.
- Grafana Cloud renames HTTP client spans such as `http.github GET` to the bare
  method. The original name is in `span.grafana.original_span_name`.
- A heap-in-use flame graph merges every snapshot in the range, so its total is
  a sum of snapshots. Read its proportions, not its sizes.
