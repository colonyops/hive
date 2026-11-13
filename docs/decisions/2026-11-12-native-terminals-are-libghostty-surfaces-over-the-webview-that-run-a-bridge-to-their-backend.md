# Native terminals are libghostty surfaces over the webview that run a bridge to their backend

- **Status:** accepted
- **Date:** 2026-11-12

## Context

Code terminals render with xterm.js inside the WKWebView. Full-screen
programs (neovim, agents streaming output) feel slow: every byte crosses the
WebSocket into JavaScript, and every frame is drawn by the page. Herdr (the
herdr-app desktop client of herdr-core) draws terminals natively and feels
fast. Its pieces are libghostty-vt for terminal state, its own GPU renderer
in GPUI, and herdr-core as a terminal runtime that relays raw bytes. The
GPUI renderer cannot be hosted in a Wails window. The libghostty packages
that run inside a webview were not good enough.

Ghostty's embedding library (`ghostty.h`, the library Ghostty's own macOS
app links) draws a complete terminal with Metal into an NSView. Its surface
has only one IO backend: it spawns a command in a PTY it owns. It cannot be
fed bytes from outside.

## Decision

1. **A native terminal is a libghostty surface in an NSView over the
   webview.** `adapter/nativeterm` adds one view per terminal to the Wails
   window's content view, above the WKWebView. The page keeps an empty box
   where the terminal goes and reports its rectangle (CSS pixels) through
   `NativeTerminalService.SetFrame`. The webview never draws these terminals.
   The input path is ported from Ghostty's `SurfaceView_AppKit.swift`.

2. **The surface's command is a bridge, the hive executable in a bridge
   mode, which connects to the real backend.** libghostty draws, and answers
   the queries it sees in the bytes. The program runs elsewhere:
   - `--herdr-attach <socket> <terminal>`: a herdr-core terminal
     (`internal/platform/herdrcore/bridge`), the same protocol `hc attach`
     uses, with no detach key.
   - `--tmux-pane <session> <pane>`: one pane of a hive tmux session
     (`adapter/panebridge`), over the per-pane stream below.

3. **For tmux panes, tmux keeps everything except the renderer.** With
   `appearance.terminal_native_renderer` on, each pane box holds a native
   surface. `GET /api/terminal/pane-stream` (bearer token, no Origin) carries
   one pane. While it is open, `app.PaneTap` diverts that pane's output away
   from the session stream, so xterm.js draws nothing, and the stream starts
   with `tmuxcc.Client.RepaintPane`. Layout, sizes, splits, the tree and
   keybindings are unchanged. Pane boxes are sized from Ghostty's cell (the
   surface reports it) so tmux's grid is the drawn grid. Ghostty's padding is
   zero for the same reason.

4. **The bridge drops Ghostty's query replies on a tmux pane.** tmux already
   answers the program (ADR
   the-embedded-emulator-never-answers-terminal-queries-on-a-tmux-pane).
   Keys and replies share one PTY, so replies are recognized by shape.

5. **⌘ chords leave the surface.** Only copy and paste stay bound in
   Ghostty. Every other ⌘ chord is sent to the page as a synthetic keydown on
   the box, so the keymap works from a native pane (ADR
   a-binding-that-starts-with-the-primary-modifier-works-from-a-terminal-pane).

6. **libghostty is built from a pinned Ghostty commit and linked only with
   the `ghostty` build tag** (`scripts/build/build-ghostty.sh`, macOS, Zig
   and the Metal toolchain). Builds without it report native terminals
   unavailable and keep xterm.js.

## Consequences

- An NSView sits above the page, so anything the page floats over a
  terminal is hidden behind it. Surfaces hide while a modal, the command
  palette, or an overlay is open. Dropdown menus over a terminal still go
  behind it.
- A native pane has one more process (the bridge) and one more hop than the
  in-page path. It skips xterm.js, the page's frame loop and the control
  frames, but tmux still parses all output.
- Bracketed-paste mode is not in a pane's first paint, so a native surface
  opened after a program turned it on pastes without brackets until the
  program sets it again.
- macOS only. Linux and Windows keep xterm.js.
