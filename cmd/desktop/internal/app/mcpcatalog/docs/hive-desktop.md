# Hive Desktop

The `hive-desktop` MCP server is this install itself. It is how a workspace's
agent observes and operates the running app — reading the inbox, feeds,
profiles and action catalog, forcing a source refresh, and dry-running a flow
— instead of reading `desktop-pipeline.db` or editing config files blind
(ADR mcp-replaces-the-agent-facing-http-api).

It is the only shipped entry that is not a third-party program: there is
nothing to install and nothing to fetch, because the server is already running
inside the app the agent was launched from.

## Launch

Hive reaches it over Streamable HTTP on the loopback server that also hosts
the webhook listener:

```
http://127.0.0.1:<port>/mcp
```

The port is allocated at startup, so this entry carries **no URL in the
registry** — it is resolved when the catalogue is rendered and written into a
workspace's generated `.mcp.json` at that moment (`Descriptor.RuntimeURL`).
That is also why the catalogue reports a problem, rather than a URL, when the
loopback server is disabled: an entry that rendered an address nothing answers
would fail silently inside the agent's own `/mcp` output.

The server requires no token. It sits behind the loopback bind and spawns no
processes — session control stays on the HTTP adapter's token-guarded terminal
prefix (ADR terminal-transport, ADR a-workspace-declares-its-own-authority).

## Stability

`beta`. The tool set is settled enough to build against, but it is new and
still growing toward the rest of the app's surface; tool names may still
change before it is called stable.

## What it needs

`http.enabled` in `settings.yaml`, which is on by default. Nothing else — no
Node, no network access, and no per-workspace configuration.
