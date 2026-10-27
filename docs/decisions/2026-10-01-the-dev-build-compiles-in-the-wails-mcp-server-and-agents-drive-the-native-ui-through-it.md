# The dev build compiles in the Wails MCP server and agents drive the native UI through it

- **Status:** accepted
- **Date:** 2026-10-01

## Context

The headless server build exercises the frontend and the Go core but not the
native app: no WKWebView, no window, no shell integration. Outside drivers
cannot reach the page: the WebView is not inspectable, its RPC transport is
in-process, and macOS exposes its content to the accessibility tree only as an
empty scroll area.

Wails v3 ships an MCP server behind the `mcp` build tag. It runs inside the
app against the real WebView and exposes DOM query, JavaScript evaluation,
synthesized mouse and keyboard input, window control, and event emit/wait over
streamable HTTP on loopback. Without the tag none of it is compiled.

## Decision

1. **Every dev build carries the Wails MCP server.** `devtools prepare` writes
   `WAILS_MCP=1` into `launch.env`, and `wails3 dev` turns that into the `mcp`
   tag. Setting `WAILS_MCP=` in `overrides.env` opts out. Shipped builds never
   set it, so the server is absent from them, not disabled.

2. **The port is allocated per worktree.** `WAILS_MCP_HOST` and
   `WAILS_MCP_PORT` sit in `launch.env` and in the `launchKeys` staleness
   contract, so two worktrees' apps do not fight over one listener. `fresh`
   and `reset` treat a bound MCP port as a running app.

3. **The `desktop-ui-audit` skill is how agents use it.** It pairs the Wails
   server (drive and read the page) with the app's own MCP server (read and
   reload state). `prepare` renders the endpoint into the worktree's
   gitignored `.mcp.json` as `hive-desktop-ui`. Pixel screenshots come from
   `screencapture` against the window id, because the Wails server has no
   pixel tool.

4. **Never use `call_bound_method`, and never import the runtime from
   `js_eval`.** Both load a second copy of `@wailsio/runtime`, whose module
   side effect replaces the page's event dispatcher, after which no frontend
   listener fires (wailsapp/wails#6136). Reach bound methods through Hive's
   MCP server or the UI.

## Consequences

- A dev app accepts unauthenticated JavaScript evaluation from any local
  process. The page holds the terminal bearer token, so this surface can do
  whatever the UI can. It exists only in a dev build.
- The headless loop stays the default for frontend work. The native loop is
  for the real WebView, the window, and shell integration.
- Native dialogs, the tray, the Dock, and notification banners stay out of
  reach, and an open native panel blocks every evaluation until it closes.
- Input is synthesized DOM events: `isTrusted` is false, so a handler that
  requires a trusted event will not fire.
- `.mcp.json` at the worktree root is owned by the devtools, so the repository
  cannot commit one. The blank onboarding instance writes none.
