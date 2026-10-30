# Pop-out windows host one registered surface in their own Vue root

- **Status:** accepted
- **Date:** 2026-10-30

## Context

People want to read a Drive document, and later other views, beside the window
they type in. `App.vue` composes the whole shell: the feed, flows, onboarding,
the updater, the keymap, and the handlers for links and notifications. Running
it again per window would run those again too, and some must run once.

## Decision

**A pop-out is a native window that hosts one surface.** A surface is a view
registered in `frontend/src/windows/surfaces.ts`, and its kind is declared in
`popOutKinds` in `wailsui/popouts.go`. It takes `params`, emits `update:params`
and `title`, and re-reads Go for its data.

**Each window gets its own Vue root.** `main.ts` picks the root from the URL
query and loads it on demand, as it does for Diagnostics. A surface never reads
the main window's shell. Nothing passes between windows except through Go.

**Go builds the URL.** A page names a kind and its declared params, so it can
only open a declared surface.

**A window is identified by its scope.** Asking for a scope that is open raises
its window. A surface that moves reports its new scope with `Retarget`.

**Closing destroys a pop-out.** Unlike Diagnostics, it holds no hidden webview.

**Focus means any Hive window.** A focus event only causes a re-read of the
windows, because Wails delivers each event on its own goroutine, in no
guaranteed order. Notification gating and polling therefore treat a move between
Hive windows as staying in the app.

**The server build opens no native window.** One would never run and would
never leave the Wails window map. `Open` returns the URL, and the page opens it.

## Consequences

- Handlers that must run once (notifications, link intake, the menu bar) stay in
  the main root. A surface must not import `useFeedState`, which brings
  `useNotify`.
- A view with one viewer on its backend cannot be mirrored. Terminals and chats
  stay out, because a tmux control client has one subscriber per session.
- `localStorage` keys are shared between windows, so a key that must differ per
  window belongs in the surface's params.
- A pop-out has no keymap or palette yet. ⌘W reaches it through the app menu.
- On Linux under GTK4, a closed window leaves its web process running until the
  app quits (wailsapp/wails#6505).
- Window positions are not saved, and pop-outs do not reopen on launch.
