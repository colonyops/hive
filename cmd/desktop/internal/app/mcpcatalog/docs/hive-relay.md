# Hive Relay

The `hive-relay` MCP server is the `/mcp` endpoint of the Hive Relay account
this install is connected to. A workspace's agent uses it to manage the
account's webhook plumbing without the user copying keys around.

## Tools

- `whoami`: the account's email, encryption setting, limits, usage, sign-in
  methods, and the relay version.
- `list_sources`, `create_source`, `update_source`, `delete_source`: webhook
  sources. `create_source` returns the ingress URL a sender posts to; it is
  shown only then.
- `rotate_ingress`: replace a source's ingress URL. The old one stops at once.
- `list_deliveries`: page through a source's stored deliveries after a
  cursor. On an encrypted account it returns metadata only.
- `list_tokens`, `create_token`, `revoke_token`: the account's API keys.

Setting a relay node's dialect and mapping, and reading the contents of an
encrypted account, are the `hive-desktop` server's job: the flow and the key
live in the app, not on the relay.

## Launch

Streamable HTTP at the connected relay's URL plus `/mcp`, with an
`Authorization: Bearer` header. Both are resolved when the workspace is
generated (`Descriptor.AccountURL`), so the registry carries no URL.

## What it needs

A Hive Relay account connected under **Settings ▸ Integrations**. Without one,
the workspace still opens and reports a problem instead of writing an entry
that cannot authenticate. With several, the first one is used.

## The workspace's key

Each workspace gets its own API key, minted on the relay the first time the
workspace is generated and named `workspace:<workspace>`. Hive keeps it in
the keychain and reuses it on every later open. The key is written in plain
text into the workspace's generated MCP config files (`.mcp.json` and
`.codex/config.toml`), so anything that can read the workspace folder can use
it. Revoke it from the relay's token list (`list_tokens`, then
`revoke_token`, or the relay's API) to cut the workspace off.

## Stability

`experimental`. The relay and its tool set are new.
