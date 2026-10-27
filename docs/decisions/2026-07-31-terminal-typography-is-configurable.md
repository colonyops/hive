# Terminal typography is configurable, and the bundled face carries five weights

- **Status:** accepted
- **Date:** 2026-07-31

## Context

Terminal text rendered visibly heavier than the rest of the app (#181). Nothing
requested a bold weight. The cause is the rasterizer: panes draw through a
WebGL/Canvas atlas (ADR terminal-atlas-renderer), and
`-webkit-font-smoothing: antialiased` on `body` does not reach Canvas2D
`fillText`. macOS rasterizes canvas glyphs with the heavier default gamma, and
no `Terminal` option or WKWebView control changes that. The weight the terminal
asks for is the only lever.

## Decision

1. **Terminal font family, weight and bold weight are user settings**
   (`terminal_font_family`, `terminal_font_weight`,
   `terminal_font_weight_bold` under `appearance` in `settings.yaml`), applied
   to open panes. They are terminal-scoped; the terminal is the only surface
   where a Nerd Font matters.

2. **The weight setting offers 300/350/400/600/700**, and the bundled face
   must answer each exactly, because CSS never synthesizes a lighter weight.
   The bundled faces are recorded in ADR
   bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font.

3. **The default normal weight is 350 (Semilight), not 400.** It compensates
   for the atlas's rasterization; 400 is what #181 was filed about, and 300
   reads too thin at 13-14px on a dark background. It looks like an
   off-by-default mistake and is not.

4. **Installed families are enumerated in Go, not in the webview.**
   `queryLocalFonts` is Chromium-only and empty on WKWebView.
   `cmd/desktop/internal/app/fonts` walks the OS font directories and parses
   each file with `golang.org/x/image/font/sfnt`, behind a `Lister`.

5. **A family counts as monospace when its Latin advances agree**, not when
   the post table says `isFixedPitch`. A Nerd Font patched face carries
   double-width icons and reports `isFixedPitch=0`. The flag is a fast path
   only.

6. **A chosen family is stacked in front of the bundled one, and the bundled
   one is stored as empty.** A family the webview cannot resolve degrades to
   the shipped face rather than to `monospace`. Storing `""` means a later
   change of bundled face carries existing users with it.

## Consequences

- The weight control is only as honest as the chosen family: a system font
  with two faces collapses five options onto two.
- The font scan is cached for the process. A font installed while the app runs
  appears after a relaunch.
- Both weights are written through one setter (`SetTerminalFontWeights`), so a
  normal weight cannot land above the bold one.
- `loadTerminalFaces` warms both weights and their italics before a pane
  opens, because the atlas caches whatever was resident at open.
- Verification is visual. Tests pin which faces are requested and which options
  reach xterm, not how they rasterize.
