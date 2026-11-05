# Chat control is served on a token-guarded MCP server whose token the endpoint file carries

- **Status:** accepted
- **Date:** 2026-11-05

## Context

An applet, an agent, or a script must start an agent workspace chat with an opening prompt and pin it to the Code sidebar (#835). A chat start runs the workspace's command, and that command can bypass the agent's permission prompts. The start route under `/api/terminal/` needs the terminal token, which only the webview holds (ADR terminal-transport). The MCP servers at `/mcp` and `/mcp/drive` have no authentication and exclude session control (ADR mcp-replaces-the-agent-facing-http-api).

The threat is a web page in the user's browser that sends a request to the loopback port. A process that runs as the user can already run any command, so it is trusted.

## Decision

**The chat operations are a `chats` toolset on their own MCP server at `/mcp/chats`, and that server requires a bearer token.**

- The desktop mints the token on every start. It is not the terminal token, so a holder cannot drive the terminals. The terminal token rules do not change.
- The desktop writes the token into the endpoint file (`<data-dir>/desktop/endpoint.json`, mode 0600). A web page can reach the port but cannot read the file.
- `hive mcp serve` and `hive dt` read the token from the file and send it on every call, so a CLI user never handles it. With `--desktop-url`, the CLI sends `HIVE_DESKTOP_TOKEN` instead, and never the file's token.
- A workspace gets the tools through the shipped `hive-chats` catalogue entry. The entry renders only as `hive mcp serve`; without a hive CLI at the app's version it reports a problem, because the HTTP form cannot carry the token.
- Applets call the chat operations through the Wails bridge, after the same check as `hive.exec`. This adds no authority (ADR applets-are-web-pages-with-full-access-to-the-machine).
- A pin is a column on the chat record (`pinned_at`), not browser storage, so every caller can read and change it.

## Consequences

- An HTTP-only agent configuration cannot use `hive-chats`. The open HTTP servers stay for now. Moving every agent to `hive mcp serve` and closing them is later work.
- `schedule_put` on the open `/mcp` can still start a chat in any workspace through a schedule. This decision does not change that.
- A new operation that starts a process belongs on a token-guarded server, not on `/mcp` or `/mcp/drive`.
