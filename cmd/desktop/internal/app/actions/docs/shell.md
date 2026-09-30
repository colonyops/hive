# Shell

A **shell** action runs an author-trusted command line via `sh -c`. Use it to
reach anything the desktop app has no first-class integration with — a CLI, a
script, a `curl` to an internal service.

## Fields

- `command_template` (required) — the command line to run.
- `cwd` — working directory. On a `session` or `window` target it defaults to
  the session's checkout, so `mise run test` is a complete action; on an `item`
  target it defaults to the desktop process's own. Setting it wins either way.
- `timeout` — a duration string like `"30s"` bounding the run. Must be quoted:
  a bare number is a hard error, not seconds. Omit for no deadline beyond the
  invoking context's.
- `env` — a map of extra environment variables for the command.

## Templates and quoting

`command_template` is a Go `text/template` rendered over the target's data (see
"Template data" above). **Pipe every interpolated value through the `shq`
helper** — it shell-quotes the value so a title containing spaces, quotes, or
`;` cannot break out of its argument:

```
command_template: 'notify-send {{ .Payload.title | shq }} {{ .Payload.url | shq }}'
```

```
targets: [session]
command_template: 'zed {{ .Session.Path | shq }}'
```

## Where a failure shows up

On an `item` target the run is a durable command, and failures keep bounded
stdout/stderr diagnostics on that record, readable from the activity view.

A `session` or `window` run is deliberately not durable — it is a manual
operation against live local state, so it must stay repeatable and must not
replay after a restart. There is no record to hold its streams, so its failure
reason carries the tail of stderr instead, in the jobs list.
