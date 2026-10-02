---
name: desktop-ui-audit
description: Drive and audit the real native Hive Desktop window through the Wails MCP server compiled into every dev build - query the DOM, click, type, read state back, and take pixel screenshots - and author views so an agent can drive them. Use when asked to verify a change in the real app rather than the headless build, smoke-test a workflow end to end, screenshot the native window, check what the UI shows after an event, or review a view's data-testid coverage before a PR.
compatibility: macOS. A dev app from this worktree (`mise run desktop:dev`) whose launch.env carries WAILS_MCP_PORT and whose .mcp.json registers it as hive-desktop-ui (regenerate both with `mise run desktop:dev:prepare`). swiftc and Screen Recording permission for screenshots; Accessibility permission to raise the window.
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
- **`launch.env` has no `WAILS_MCP_PORT`**: it predates the server.
  `mise run desktop:dev:prepare` regenerates a stale or missing file;
  `desktop:dev:fresh` (the file header's advice) also reseeds the data.

Each worktree gets its own port, so this never reaches the installed Hive.app
or another worktree's app. The two look identical otherwise: same name, same
bundle id, and the installed one is usually also running. The port is the
only discriminator, and it also gives the dev app's pid:

```bash
lsof -nP -t -iTCP:$(jq -r '.mcpServers["hive-desktop-ui"].url' .mcp.json | sed -E 's#.*:([0-9]+)/mcp#\1#') -sTCP:LISTEN
```

### Preconditions on arrival

Start with `app_info` for the window list. Its `visible` means "not
occluded" and `focused` means "the key window", so `visible: false` on a
window you expect on screen is the first sign of the covered window below.
Then this page check:

```js
// js_eval
const a = document.activeElement
return {route: location.hash, title: document.title,
        visibility: document.visibilityState, hasFocus: document.hasFocus(),
        testids: document.querySelectorAll('[data-testid]').length,
        focused: a?.dataset.testid ?? a?.tagName.toLowerCase() ?? null}
```

- **`visibility` is `hidden`**: the window is covered, usually by the
  installed Hive.app. A hidden WKWebView never fires `requestAnimationFrame`
  and throttles timers, so every Vue transition stalls: a closed palette stays
  in the DOM at full opacity, an opened one never paints. `window_control
  focus` alone does not bring the window forward. Raise it by the pid above,
  then make it the key window:

  ```bash
  osascript -e "tell application \"System Events\" to set frontmost of (first process whose unix id is $PID) to true"
  ```

  followed by `window_control {action: "focus"}`. The page check then reads
  `visibility: "visible"` and `hasFocus: true`; what proves frames flow is
  this probe:

  ```js
  // js_eval: does a frame arrive?
  return await new Promise(r => {
    const t = setTimeout(() => r({frame: false}), 1500)
    requestAnimationFrame(() => { clearTimeout(t); r({frame: true}) })
  })
  ```

  If the raise is refused (no Accessibility permission), ask the user to
  click the dev window.
- **`route` is `#/feed/...` with hundreds of test ids**: the instance carries
  a copy of the installed app's data and you are on the inbox. **Onboarding is
  not a route**; it shows only while no profile exists, and the way back to it
  is `mise run desktop:dev:reset` plus a restart, which is the user's call.
  Read the state you are in rather than the state you expected.

## 2. Drive: discover, act, read back

Every target is a CSS selector (scrolled into view, centre used) or viewport
coordinates. Discover before acting; the page is what the app shows now, not
what the source suggests, and ids are read off the live DOM, not guessed from
a name.

- `dom_query {selector, limit}` returns tag, text, value, bounds, `visible`,
  and `disabled` per match, and a clean `count: 0` for no match. It is the
  assertion tool.
- `dom_html {selector}` for markup. `screenshot_dom {max_depth}` is geometry
  only (tag, classes, bounds, no ids), so it cannot tell you what to click.
- **`visible` means "has a layout box."** It is true for a hover-only control
  at `opacity: 0`, for a button inside such a strip, and for a row far below
  the fold. When that matters, walk `getComputedStyle(n).opacity` up the
  ancestors and check that the rect intersects the viewport yourself; the
  listing below does both.
- The handles on screen, grouped so a populated inbox (500+ ids) stays under
  the tool-result limit; the raw per-element array only fits a view with
  under about 150 ids:

```js
// js_eval: data-testids grouped by id, with a sample of the text an assertion needs
const inViewport = false   // true: only ids with a box inside the viewport
const faded = e => { for (let n = e; n; n = n.parentElement) if (getComputedStyle(n).opacity === '0') return true; return false }
const groups = new Map()
for (const e of document.querySelectorAll('[data-testid]')) {
  const r = e.getBoundingClientRect()
  const shown = r.width > 0 && r.height > 0 && !faded(e)   // a button inside a faded hover strip is not shown
  const onScreen = r.bottom > 0 && r.right > 0 && r.top < innerHeight && r.left < innerWidth
  if (inViewport && !(shown && onScreen)) continue
  const g = groups.get(e.dataset.testid) ?? {id: e.dataset.testid, tag: e.tagName.toLowerCase(), count: 0, shown: 0, sample: ''}
  g.count++; if (shown) g.shown++
  g.sample ||= (e.innerText || e.getAttribute('aria-label') || e.value || '').trim().replace(/\s+/g, ' ').slice(0, 60)
  groups.set(g.id, g)
}
return [...groups.values()]
```

Act, then read back. **The return value of a mouse or key tool is a hint, not
a result**: it describes whatever the hit test found under the centre point
after the event, often an `svg` or `path` child, sometimes a node the route
change already unmounted (`0x0` bounds), and for a key press the element
focus landed on. Never assert on it; the assertion is a fresh `dom_query`.

- `mouse_click {selector}`; `mouse_move {selector, duration_ms}` to hover;
  `mouse_drag {from_selector, to_selector}` (works for resize handles and
  HTML5 drag sources); `mouse_scroll {selector, delta_y}` scrolls the nearest
  scrollable ancestor and returns its `scrollTop`.
- `keyboard_type {text}` types into the focused element; add `selector` to
  click a field first. The command palette's input already has focus when ⌘K
  opens it, so type with no selector. There is no select-all: clear a field
  with one `keyboard_press {key: "Backspace"}` per character, or in one call:

  ```js
  // js_eval: clear the focused field the way Vue sees it
  const el = document.activeElement; el.value = ''
  el.dispatchEvent(new Event('input', {bubbles: true})); return el.dataset.testid
  ```
- `keyboard_press {key, modifiers}`: `{key: "k", modifiers: ["meta"]}` is ⌘K,
  then `{key: "ArrowDown"}`, `{key: "Enter"}`, `{key: "Escape"}`.
- Calls sent in one batch run serially in the order sent, so an action and
  its read-back can go in one round trip.

**There is no server-side wait** apart from `wait_for_event {name}`. After an
action that goes through Go (a refresh, a flow commit, a route change), wait
inside the page instead of asserting at once. Return the outcome rather than
throwing: a thrown message does not survive the trip (see the guardrails).

```js
// js_eval: a visible match, or {timedOut: true} after 10s
const sel = '[data-testid="detail-pane"]', deadline = Date.now() + 10000
while (Date.now() < deadline) {
  const el = document.querySelector(sel)
  if (el && el.getBoundingClientRect().width > 0) return {visible: true, sel}
  await new Promise(r => setTimeout(r, 200))
}
return {visible: false, timedOut: true, sel}
```

```js
// js_eval: gone, closing (a Vue leave transition in flight), or still there
const sel = '[data-testid="command-palette"]', deadline = Date.now() + 10000
while (Date.now() < deadline) {
  const el = document.querySelector(sel)
  if (!el) return {gone: true, sel}
  if (/-leave-active/.test(el.parentElement?.className ?? '')) return {gone: false, closing: true, sel}
  await new Promise(r => setTimeout(r, 200))
}
return {gone: false, timedOut: true, sel}
```

`closing` with a window that is `hidden` is the stalled transition from the
preconditions; the element lingers with its key handlers attached, and
another Escape toggles it back open. Raise the window first.

A worked sequence, the command palette, **from the feed view** (its commands
are scoped to the current view; elsewhere "unread" matches nothing and the
page shows `No results for "unread"`, which carries no id):

1. `keyboard_press {key: "k", modifiers: ["meta"]}`
2. the visible-wait above, for `[data-testid="command-palette-input"]`
3. `keyboard_type {text: "unread"}`
4. `dom_query {selector: '[data-testid="command-palette-command-title"]'}`
   lists the filtered commands; `ArrowDown` moves the selection
5. `keyboard_press {key: "Enter"}`, then the gone-wait above on
   `[data-testid="command-palette"]`; read `location.hash` back if the
   command navigates

## 3. Assert state, not pixels, where you can

The DOM is the cheap, exact oracle: `dom_query` returns `visible`, `disabled`,
`value`, and text, and the same `data-testid`s the Playwright suite uses. For
what the UI cannot show (did the item route to the feed, what did the flow
emit) read Hive's own MCP server with the **desktop-api** skill.

Pixels are for what the DOM cannot prove: layout, theming, the native
titlebar, a rendered image. `scripts/screenshot.sh` finds the window by the
process on the MCP port and captures it at native resolution, covered or not;
read the file back to look at it. The animated cursor overlay is in the image.

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
- **A thrown error loses its message.** The page reports `error.stack`, and
  WebKit's stack has no message line, so a `js_eval` throw arrives as
  `javascript error: @wails://localhost:<port>/:776:16` and a `mouse_click`
  on a selector with no match as a bare `resolveTarget@...` stack. Return
  values from your own code, and `dom_query` a selector before clicking it so
  "no match" is a readable `count: 0`.
- **A native dialog blocks every evaluation.** An open file panel, a sheet, or
  a notification-permission prompt makes each call time out ("the window may
  be reloading, busy or showing a native dialog"). Avoid paths that open one,
  or ask the user to dismiss it. The panel itself is out of reach, as are the
  tray, the Dock, and notification banners.
- **A hidden window stalls the UI.** See the preconditions: no frames, no
  transitions, throttled timers. Raise it by pid before judging anything that
  animates.
- **Input is synthesized DOM events, not OS events.** `isTrusted` is false.
  Hive's handlers do not check it, so do not add one. Keyboard events go to the
  focused element and bubble to the window listeners that implement chords.
- **The cursor overlay is in the DOM** as `#__wails-mcp-cursor` after the
  first mouse tool runs. It has no `data-testid`, so it only shows up in a
  query by tag (`div`) or in `screenshot_dom`.
- **`app_info` and `windows_list` report occlusion, not existence.**
  `visible: false` means another window covers this one and `focused: false`
  means it is not the key window; both are accurate and both are the early
  signal for the preconditions. Neither says anything about what the page
  shows; the DOM and the screenshot do.
- **One app per worktree.** The port comes from `launch.env` and `.mcp.json`;
  do not hard-code 9099, and do not point a client at a port you did not read
  from one of those files.

## 5. Authoring views an agent can drive

The `data-testid` is the agent API and the Playwright API at once: a flow
proven here ports to `cmd/desktop/e2e/tests/*.spec.ts` with the same
`getByTestId` selectors, and a renamed id breaks both. Before a PR that adds or
changes a view, run the grouped listing above on it and check every step a
smoke test would take has a handle.

- **Every interactive element and every container an assertion reads gets
  one.** Buttons, inputs, rows, the pane that shows the result, the toast, the
  error, the empty state ("No results for ..." included), the backdrop a
  test clicks to dismiss, and the scroll container a test scrolls. The
  Playwright suite asserts on `feed-item`, `detail-pane`, `toast`,
  `feed-search`, `command-palette-input`, `application-settings`; new
  surfaces follow suit. Read the id off the live DOM rather than guessing it
  from a name: `palette-entry` is the flow editor's node palette, and the
  command palette is `command-palette-*`.
- **Name it `<surface>-<object>[-<verb>]`, kebab-case**, as the codebase does:
  `feed-item`, `sidebar-edit-flow`, `onboarding-hive-continue`,
  `settings-category-keybindings`, `flow-node-<id>`. **A repeated row carries
  its identity** (`:data-testid="'item-session-' + session.id"`); thirty rows
  that all say `feed-item` can only be told apart by text. A compound
  component takes a `testid` prop and derives its parts (`${testid}-input`,
  `-confirm`, `-cancel`, `-error`), so one prop names the whole widget.
- **Put state in attributes and text, not only in pixels.** `dom_query`
  returns text, `value`, `disabled`, and bounds; `aria-selected` on the
  selected row, `aria-expanded`, `data-state`, and `disabled` are what an
  agent reads. A selection that lives only in a class name is invisible to
  it. The words an assertion needs belong in the element's text or
  `aria-label`.
- **Icon-only buttons need `aria-label`.** The tool describes an element by
  its text; an icon with none is a nameless button in every listing.
- **Keep ids stable through refactors.** They are not styling hooks; grep
  `cmd/desktop/e2e/tests` before renaming one.
- **No id on decorative nodes.** The listing is read by a model; signal, not
  wallpaper.
- **Give every hover-only affordance a click or key path.** `mouse_move`
  fires hover events, but a workflow that depends on hover is fragile for an
  agent and for a user with a keyboard, and an `opacity: 0` control reads as
  `visible` to every tool.
- **Do not gate behaviour on `event.isTrusted`**, and do not require a native
  dialog where a drop zone or a path field would do; the dialog is the one
  thing nothing here can drive.
