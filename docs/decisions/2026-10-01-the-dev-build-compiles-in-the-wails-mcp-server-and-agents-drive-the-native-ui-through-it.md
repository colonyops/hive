# The dev build compiles in the Wails MCP server and agents drive the native UI through it

- **Status:** accepted
- **Date:** 2026-10-01

## Context

The headless server build is the agent UI loop: Playwright drives it in Docker
and `desktop:serve` drives it by hand. It exercises the frontend and the Go
core but not the native app: no WKWebView, no window, no shell integration.
Issue #463 asked how to drive the real app and evaluated a vision-guided
CGEvent driver, Appium's mac2 driver, a loopback control API, and GUI
scripting. None of them reaches the page: the WebView is not inspectable, its
RPC transport is in-process, and macOS exposes its content to the
accessibility tree only as an empty scroll area.

Wails v3 (the pinned beta.21) ships a built-in MCP server behind an `mcp`
build tag. It runs inside the app, executes against the real WebView through
`window.ExecJS`, and exposes DOM query, JavaScript evaluation, synthesized
mouse and keyboard input, window control, and event emit/wait over streamable
HTTP on loopback. Without the tag none of it is compiled.

## Decision

1. **Every dev build carries the Wails MCP server.** `devtools prepare`
   writes `WAILS_MCP=1` into `launch.env`, and `wails3 dev` turns that into
   the `mcp` tag. A developer who wants a dev build without it sets
   `WAILS_MCP=` in `overrides.env`. Shipped builds never set the variable, so
   the server is absent from them rather than disabled.

2. **The port is allocated per worktree.** `WAILS_MCP_HOST` and
   `WAILS_MCP_PORT` sit beside the Vite, Wails, and webhook ports in
   `launch.env` and in the `launchKeys` staleness contract. The upstream
   default of 9099 would make two worktrees' apps fight over one listener.
   `fresh` and `reset` treat a bound MCP port as a running app, like the other
   two.

3. **It is the agent surface for the native UI, and the `desktop-ui-audit`
   skill is how agents use it.** The skill pairs it with the app's own MCP
   server: Wails tools drive and read the page, Hive tools read and reload
   state. Pixel screenshots come from `screencapture` against the window id,
   because the Wails server has no pixel tool.

4. **`call_bound_method` is off limits, and so is importing the runtime from
   `js_eval`.** Both load a second copy of `@wailsio/runtime`, whose module
   side effect replaces the page's event dispatcher, after which no frontend
   listener fires (wailsapp/wails#6136). Bound methods are reached through
   Hive's MCP server or through the UI instead.

## Consequences

- The dev app accepts unauthenticated JavaScript evaluation from any local
  process on a loopback port. That matches the posture of the app's own MCP
  server (ADR mcp-replaces-the-agent-facing-http-api) but reaches further: the
  page holds the terminal bearer token, so this surface can do whatever the UI
  can. It exists only in a dev build on a developer's machine.
- The headless loop stays the default for frontend work. The native loop is
  for what the headless build cannot show: the real WebView, the window, and
  shell integration. The "never verify UI with a local GUI build" rule narrows
  to "never by hand".
- Native dialogs, the tray, the Dock, and notification banners remain out of
  reach. An open native panel blocks every evaluation until it closes.
- Input is synthesized DOM events, not OS events. `isTrusted` is false, and a
  handler that requires a trusted event will not fire.
- The helper scripts are macOS-only where they touch the window
  (`screencapture`, `CGWindowListCopyWindowInfo`); the MCP calls are portable.
