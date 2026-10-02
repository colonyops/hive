---
name: desktop-ui-audit
description: Drive and audit the real native Hive Desktop window through the Wails MCP server compiled into every dev build - query the DOM, click, type, read state back, and take pixel screenshots - and author views so an agent can drive them. Use when asked to verify a change in the real app rather than the headless build, smoke-test a workflow end to end, screenshot the native window, check what the UI shows after an event, or review a view's data-testid coverage before a PR.
compatibility: macOS. A dev app from this worktree (`mise run desktop:dev`) whose launch.env carries WAILS_MCP=1 and WAILS_MCP_PORT (regenerate with `mise run desktop:dev:prepare` if it does not). curl and jq; swiftc and Screen Recording permission for screenshots.
---

# Drive the native app through its Wails MCP server

Every dev build compiles in Wails' own MCP server (ADR
the-dev-build-compiles-in-the-wails-mcp-server-and-agents-drive-the-native-ui-through-it).
It runs inside the app and executes against the real WKWebView: DOM queries,
JavaScript evaluation, synthesized mouse and keyboard input with an animated
on-screen cursor, window control, and event emit/wait. It is the only way to
reach the page in the native app: the WebView is not inspectable, and macOS
exposes its content to the accessibility tree as an empty scroll area.

Pick the loop before starting:

- **A frontend change** is the headless loop: `mise run desktop:serve` and
  browser tooling, with `mise run desktop:e2e` as the gate. It is faster and
  it is what CI knows.
- **The real app** - the native window, the WebView, shell integration, a
  workflow a user would click through - is this skill.
- **State** (what landed in the inbox, what a flow did) is the **desktop-api**
  skill; **events** (make GitHub say something) is **desktop-devserver**. Pair
  them: drive the UI here, assert the state there.

The helper scripts live beside this file. `scripts/mcp.sh` wraps the tool
calls and unwraps results; `scripts/screenshot.sh` captures the window.

## 1. Find the app

The endpoint is `http://127.0.0.1:$WAILS_MCP_PORT/mcp`, with the port in the
worktree's `launch.env`. Each worktree gets its own, so this never reaches the
installed Hive.app or another worktree's app.

```bash
S=.agents/skills/desktop-ui-audit/scripts
$S/mcp.sh status     # {mcp, page: {route, title, testids, viewport, focused}, hive_api}
```

If it does not answer, the app is not running: ask the user to start
`mise run desktop:dev` (it opens a window, so do not start it from a background
shell they cannot see). If `launch.env` has no `WAILS_MCP_PORT`, it predates
the server; `mise run desktop:dev:prepare` regenerates it. For the onboarding
instance, point the scripts at its file: `HIVE_LAUNCH_ENV=launch.onboarding.env`.

## 2. Connect

`prepare` writes the worktree's `.mcp.json` beside `launch.env` with this
server as `hive-desktop-ui`. A Claude Code session started in the worktree
offers it; approve once and the tools are ordinary tools. (The app's own
state tools are a separate registration; see **desktop-api**.) A client
running elsewhere points at the URL in that file:

```bash
jq -r '.mcpServers["hive-desktop-ui"].url' .mcp.json
claude mcp add --transport http hive-desktop-ui "$(jq -r '.mcpServers["hive-desktop-ui"].url' .mcp.json)"
```

The server is stateless and answers plain JSON, so the scripts are plain curl
and work without any client. `$S/mcp.sh tools` prints the table; the full
reference is https://v3.wails.io/guides/mcp-service.

## 3. Drive: discover, act, read back

Every target is a CSS selector (scrolled into view, centre used) or viewport
coordinates. Discover before acting; the page is what the app shows now, not
what the source suggests.

```bash
$S/mcp.sh testids                      # every data-testid in the DOM, visible flag, text
$S/mcp.sh testids feed                 # filter by substring
$S/mcp.sh query '[data-testid="feed-item"]' 5   # tag, text, value, bounds, visible
$S/mcp.sh text '[data-testid="detail-pane"]'    # what the user reads
$S/mcp.sh snapshot 4                   # structural outline of the viewport
```

Act, then read back. A click's own return value describes the element as it
was; the assertion is a fresh query afterwards.

```bash
$S/mcp.sh click '[data-testid="feed-item"]'
$S/mcp.sh wait '[data-testid="detail-pane"]' 5
$S/mcp.sh text '[data-testid="detail-pane"]'

$S/mcp.sh type '[data-testid="feed-search"]' 'retry'   # clicks first, then per-key events
$S/mcp.sh press k meta                                 # a chord: key, then modifiers
$S/mcp.sh type - 'unread'                              # "-": type into whatever has focus
$S/mcp.sh press Enter
$S/mcp.sh press Escape
$S/mcp.sh scroll '[data-testid="feed-list"]' 600
```

Anything the scripts do not cover is one tool call away. `js` runs an async
function body in the page and returns a JSON value, and `call` reaches every
tool with its raw arguments:

```bash
$S/mcp.sh js 'return [...document.querySelectorAll("[data-testid=toast]")].map(t => t.innerText)'
$S/mcp.sh call mouse_drag '{"from_selector":".card","to_selector":".dropzone"}'
$S/mcp.sh call wait_for_event '{"name":"inbox:updated","timeout_ms":15000}'
$S/mcp.sh call window_control '{"action":"set_size","width":1100,"height":700}'
```

**There is no server-side wait**, apart from `wait_for_event`. After an action
that goes through Go (a refresh, a flow commit), poll with `wait` or re-query a
few times rather than asserting immediately.

## 4. Assert state, not pixels, where you can

The DOM is the cheap, exact oracle: `query` returns `visible`, `disabled`,
`value`, and text, and the same `data-testid`s the Playwright suite uses. For
what the UI cannot show (did the item route to the feed, what did the flow
emit) read Hive's own MCP server with the **desktop-api** skill; `status`
prints its `/api/status` beside the page.

Pixels are for what the DOM cannot prove: layout, theming, the native titlebar,
a rendered image. `screenshot.sh` finds the window by the process on the MCP
port and captures it at native resolution; read the file back to look at it.

```bash
$S/screenshot.sh                      # cmd/desktop/e2e/screenshots/native-<time>.png (gitignored)
$S/screenshot.sh /tmp/after-upload.png
```

It needs Screen Recording permission for the terminal process; the first run
compiles `winid.swift` with the system `swiftc`.

## 5. Guardrails

- **Never call `call_bound_method`, and never `import('/wails/runtime.js')`
  from `js_eval`.** Both load a second copy of `@wailsio/runtime` whose module
  side effect replaces `window._wails.dispatchWailsEvent`; every frontend
  `Events.On` listener then stops firing, silently, until reload
  (wailsapp/wails#6136). Reach Go through the UI or through Hive's MCP server.
- **A native dialog blocks every evaluation.** An open file panel, a sheet, or
  a notification-permission prompt makes each call time out ("the window may
  be reloading, busy or showing a native dialog"). Avoid paths that open one,
  or ask the user to dismiss it. The panel itself is out of reach, as are the
  tray, the Dock, and notification banners.
- **Input is synthesized DOM events, not OS events.** `isTrusted` is false.
  Hive's handlers do not check it, so do not add one. Keyboard events go to the
  focused element and bubble to the window listeners that implement chords.
- **The cursor overlay is in the DOM** as `#__wails-mcp-cursor` after the
  first mouse tool runs. It has no `data-testid`; exclude it if a query by tag
  catches it.
- **`app_info` and `windows_list` report `visible:false`** for a window that
  is on screen when the app was launched from a background process. Trust the
  DOM and the screenshot, not that flag.
- **One app per worktree.** The port comes from `launch.env`; do not hard-code
  9099, and do not point the scripts at a port you did not read from a launch
  file.

## 6. Authoring views an agent can drive

The `data-testid` is the agent API and the Playwright API at once: a flow
proven here ports to `cmd/desktop/e2e/tests/*.spec.ts` with the same
`getByTestId` selectors, and a renamed id breaks both. Before a PR that adds or
changes a view, run `$S/mcp.sh testids` on it and check every step a smoke test
would take has a handle.

- **Every interactive element and every container an assertion reads gets
  one.** Buttons, inputs, rows, the pane that shows the result, the toast, the
  error, the empty state. The Playwright suite asserts on `feed-item`,
  `detail-pane`, `toast`, `feed-search`, `command-palette-input`,
  `application-settings`; new surfaces follow suit. Read the id off the live
  DOM with `testids` rather than guessing it from a name: `palette-entry` is
  the flow editor's node palette, and the command palette is
  `command-palette-*`.
- **Name it `<surface>-<object>[-<verb>]`, kebab-case**, as the codebase does:
  `feed-item`, `sidebar-edit-flow`, `onboarding-hive-continue`,
  `action-row-smoke-created`. A list row carries its identity:
  `:data-testid="'item-session-' + session.id"`. A compound component takes a
  `testid` prop and derives its parts (`${testid}-input`, `-confirm`,
  `-cancel`, `-error`), so one prop names the whole widget.
- **Put state in attributes and text, not only in pixels.** `query` returns
  text, `value`, `disabled`, and bounds; `aria-selected`, `aria-expanded`,
  `data-state`, and `disabled` are what an agent reads, a colour is not. The
  words an assertion needs belong in the element's text or `aria-label`.
- **Icon-only buttons need `aria-label`.** The tool describes an element by
  its text; an icon with none is a nameless button in every listing.
- **Keep ids stable through refactors.** They are not styling hooks; grep
  `cmd/desktop/e2e/tests` before renaming one.
- **No id on decorative nodes.** The snapshot has a 2000-node budget and the
  listing is read by a model; signal, not wallpaper.
- **Give every hover-only affordance a click or key path.** `mouse_move`
  fires hover events, but a workflow that depends on hover is fragile for an
  agent and for a user with a keyboard.
- **Do not gate behaviour on `event.isTrusted`**, and do not require a native
  dialog where a drop zone or a path field would do; the dialog is the one
  thing nothing here can drive.
