---
name: desktop-ui-audit
description: Drive and audit the real native Hive Desktop window through the Wails MCP server compiled into every dev build - query the DOM, click, type, read state back, and take pixel screenshots - and author views so an agent can drive them. Use when asked to verify a change in the real app rather than the headless build, smoke-test a workflow end to end, screenshot the native window, check what the UI shows after an event, or review a view's data-testid coverage before a PR.
compatibility: macOS. A dev app from this worktree (`mise run desktop:dev`) whose launch.env carries WAILS_MCP_PORT and whose .mcp.json registers it as hive-desktop-ui (regenerate both with `mise run desktop:dev:prepare`). swiftc and Screen Recording permission for screenshots.
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

## 1. Find the app

`prepare` writes two files at the worktree root: `launch.env`, whose
`WAILS_MCP_PORT` the app binds, and `.mcp.json`, which registers that endpoint
as the `hive-desktop-ui` server. A Claude Code session started in the worktree
offers its sixteen tools after a one-time approval. Another client reads the
URL from the file:

```bash
jq -r '.mcpServers["hive-desktop-ui"].url' .mcp.json
```

- **The tools are missing**: the session predates the file or the approval was
  declined. Restart in the worktree, or register the URL above with
  `claude mcp add --transport http hive-desktop-ui <url>`.
- **A call cannot connect**: the app is not running. Ask the user to start
  `mise run desktop:dev`; it opens a window, so do not start it from a
  background shell they cannot see.
- **`launch.env` has no `WAILS_MCP_PORT`**: it predates the server, and
  `mise run desktop:dev:prepare` regenerates both files.

Each worktree gets its own port, so this never reaches the installed Hive.app
or another worktree's app. Start with `app_info` for the window list, then a
page check:

```js
// js_eval
return {route: location.hash, title: document.title,
        testids: document.querySelectorAll('[data-testid]').length,
        focused: document.activeElement?.dataset.testid ?? null}
```

## 2. Drive: discover, act, read back

Every target is a CSS selector (scrolled into view, centre used) or viewport
coordinates. Discover before acting; the page is what the app shows now, not
what the source suggests, and ids are read off the live DOM, not guessed from
a name.

- `dom_query {selector, limit}` returns tag, text, value, bounds, `visible`,
  and `disabled` per match. It is the assertion tool.
- `dom_html {selector}` for markup; `screenshot_dom {max_depth}` for an
  outline of the viewport.
- The handles on screen, with the words an assertion needs:

```js
// js_eval: every data-testid in the DOM
return [...document.querySelectorAll('[data-testid]')].map(e => {
  const r = e.getBoundingClientRect()
  return {id: e.dataset.testid, tag: e.tagName.toLowerCase(),
          visible: r.width > 0 && r.height > 0,
          text: (e.innerText || e.getAttribute('aria-label') || e.value || '')
                  .trim().replace(/\s+/g, ' ').slice(0, 60)}
})
```

Act, then read back. A click's own return value describes the element as it
was; the assertion is a fresh `dom_query` afterwards.

- `mouse_click {selector}`; `mouse_move {selector, duration_ms}` to hover;
  `mouse_drag {from_selector, to_selector}`; `mouse_scroll {selector, delta_y}`.
- `keyboard_type {text}` types into the focused element; add `selector` to
  click a field first. The command palette's input already has focus when ⌘K
  opens it, so type with no selector.
- `keyboard_press {key, modifiers}`: `{key: "k", modifiers: ["meta"]}` is ⌘K,
  then `{key: "Enter"}`, `{key: "Escape"}`.

**There is no server-side wait** apart from `wait_for_event {name}`. After an
action that goes through Go (a refresh, a flow commit, a route change), wait
inside the page instead of asserting at once:

```js
// js_eval: a visible match, or a throw after 10s
const sel = '[data-testid="detail-pane"]', deadline = Date.now() + 10000
while (Date.now() < deadline) {
  const el = document.querySelector(sel)
  if (el && el.getBoundingClientRect().width > 0) return {visible: true}
  await new Promise(r => setTimeout(r, 200))
}
throw new Error('timed out waiting for ' + sel)
```

A worked sequence, the command palette:

1. `keyboard_press {key: "k", modifiers: ["meta"]}`
2. the wait above, for `[data-testid="command-palette-input"]`
3. `keyboard_type {text: "unread"}`
4. `dom_query {selector: '[data-testid="command-palette-command-title"]'}`
   lists the filtered commands
5. `keyboard_press {key: "Enter"}`, then `dom_query` on
   `[data-testid="command-palette"]`: a count of 0 means it closed

## 3. Assert state, not pixels, where you can

The DOM is the cheap, exact oracle: `dom_query` returns `visible`, `disabled`,
`value`, and text, and the same `data-testid`s the Playwright suite uses. For
what the UI cannot show (did the item route to the feed, what did the flow
emit) read Hive's own MCP server with the **desktop-api** skill.

Pixels are for what the DOM cannot prove: layout, theming, the native
titlebar, a rendered image. `scripts/screenshot.sh` finds the window by the
process on the MCP port and captures it at native resolution; read the file
back to look at it.

```bash
.agents/skills/desktop-ui-audit/scripts/screenshot.sh                  # cmd/desktop/e2e/screenshots/native-<time>.png (gitignored)
.agents/skills/desktop-ui-audit/scripts/screenshot.sh /tmp/after.png
```

It needs Screen Recording permission for the terminal process; the first run
compiles `winid.swift` with the system `swiftc`.

## 4. Guardrails

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
- **One app per worktree.** The port comes from `launch.env` and `.mcp.json`;
  do not hard-code 9099, and do not point a client at a port you did not read
  from one of those files.

## 5. Authoring views an agent can drive

The `data-testid` is the agent API and the Playwright API at once: a flow
proven here ports to `cmd/desktop/e2e/tests/*.spec.ts` with the same
`getByTestId` selectors, and a renamed id breaks both. Before a PR that adds or
changes a view, run the test-id listing above on it and check every step a
smoke test would take has a handle.

- **Every interactive element and every container an assertion reads gets
  one.** Buttons, inputs, rows, the pane that shows the result, the toast, the
  error, the empty state. The Playwright suite asserts on `feed-item`,
  `detail-pane`, `toast`, `feed-search`, `command-palette-input`,
  `application-settings`; new surfaces follow suit. Read the id off the live
  DOM rather than guessing it from a name: `palette-entry` is the flow
  editor's node palette, and the command palette is `command-palette-*`.
- **Name it `<surface>-<object>[-<verb>]`, kebab-case**, as the codebase does:
  `feed-item`, `sidebar-edit-flow`, `onboarding-hive-continue`,
  `action-row-smoke-created`. A list row carries its identity:
  `:data-testid="'item-session-' + session.id"`. A compound component takes a
  `testid` prop and derives its parts (`${testid}-input`, `-confirm`,
  `-cancel`, `-error`), so one prop names the whole widget.
- **Put state in attributes and text, not only in pixels.** `dom_query`
  returns text, `value`, `disabled`, and bounds; `aria-selected`,
  `aria-expanded`, `data-state`, and `disabled` are what an agent reads, a
  colour is not. The words an assertion needs belong in the element's text or
  `aria-label`.
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
