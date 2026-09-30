# Terminal typography is configurable, and the bundled face carries five weights

- **Status:** accepted; point 2 superseded by [ADR bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font](2026-08-02-bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font.md)
- **Date:** 2026-07-31

## Context

Terminal panes shipped with their typography fixed: one vendored family, a size
preset, and xterm's default weights. Users reported that all terminal text
renders bold regardless of what the program's SGR attributes ask for (#181).

Nothing in the app was setting a bold weight. The vendored faces declared 400
and 700 correctly, xterm's own `fontWeight`/`fontWeightBold` defaults
(`normal`/`bold`) were in effect, and its atlas builds the canvas font shorthand
from them correctly. What changed was the *rasterizer*: ADR terminal-atlas-renderer moved every
pane off the DOM renderer onto a WebGL/Canvas atlas, and `-webkit-font-smoothing:
antialiased` — set on `body`, and what the rest of the app is drawn with — does
not reach Canvas2D `fillText`. macOS rasterizes canvas glyphs with the heavier
default gamma, so the same face is visibly heavier inside a pane than outside
one. There is no `Terminal` option, and no WKWebView control, that changes this.

So the weight the terminal *asks* for is the only lever, and there was no way to
move it.

## Decision

1. **Terminal font family, weight, and bold weight are user settings**, persisted
   in `settings.yaml` beside the existing size preset and applied to open panes.
   They are terminal-scoped on purpose: this is not the app-wide font picker
   (#19), and the terminal is the only surface where a Nerd Font matters.

2. **The bundled face is CaskaydiaMono Nerd Font, carrying five weights
   (300/350/400/600/700) plus italics, as WOFF2.** Two of these follow from the
   symptom rather than from taste. CSS matches a requested weight to the nearest
   declared face and **never synthesizes a lighter one**, so a family shipping
   only 400 and 700 — which is what Hack, JetBrains Mono's patched set, and most
   patched faces ship — would render "Light" identically to "Regular" and make
   the setting a no-op on the shipped font. WOFF2 is what pays for the extra
   faces: ten of them are smaller than the four TTFs they replaced (10.0MB vs
   10.8MB). Cascadia *Mono* rather than Cascadia *Code* is the ligature-free cut
   of the same design.

3. **The default normal weight is 350 (SemiLight), not 400.** This compensates
   the atlas's rasterization rather than correcting a wrong value — 400 was
   already what was requested and drawn. Recorded because it looks like an
   off-by-default mistake and will be "fixed" back otherwise.

   The usable range turned out to be one step wide, which is why an unusual
   value is the default. 400 is what #181 was filed about. 300 was tried first
   and reads too thin at 13-14px on a dark background: a light stroke on dark
   gets eaten rather than fattened, so it fails in the opposite direction. 350
   is the only rung between them, and it displaced ExtraLight (200) in the
   bundle — 200 sits outside the usable range in the same direction 300 already
   overshoots, so it cost nothing to drop and kept the face count and payload
   flat.

4. **Installed families are enumerated in Go, not in the webview.** The Local
   Font Access API (`queryLocalFonts`) is Chromium-only, so a picker built on it
   is empty on macOS/WKWebView — the platform this ships first. `internal/app/fonts`
   walks the OS font directories and parses each file with
   `golang.org/x/image/font/sfnt`, which was already a dependency. CSS can still
   *resolve* a local family by name; only enumeration was missing.

5. **A family counts as monospace when its Latin advances agree, not when the
   post table says `isFixedPitch`.** A Nerd Font patched face carries
   double-width icon glyphs and therefore reports `isFixedPitch=0` — the flag
   alone hides every face a terminal user is likely to have installed. The flag
   is checked first as a fast path; the measurement is the load-bearing test.

6. **A chosen family is stacked in front of the bundled one, and the bundled one
   is stored as empty.** A family the OS reports but the webview cannot resolve,
   or one uninstalled since it was picked, degrades to the shipped Nerd Font
   rather than to whatever `monospace` resolves to — which is the difference
   between a TUI's icons rendering and rendering as tofu. Storing the default as
   `""` rather than by name means a later change of bundled face carries
   existing users with it.

## Consequences

- **The weight control is honest only to the degree the chosen family is.** A
  system font with two faces collapses the five options onto two. That is the
  font's doing and is not healed here; the hint under the control says so.
- **The scan is cached for the process.** A font installed while the app is
  running appears after a relaunch. Scanning ~700 files measured at ~50ms, so
  the cost is not the reason — a live watch is simply not worth the machinery,
  and it is the same deal every terminal emulator offers.
- **Font enumeration is a new core capability, not just a settings field.** It
  sits in `internal/app/fonts` behind a `Lister`, so a future CLI, MCP tool, or
  HTTP surface can list fonts without going through Wails.
- **Both weights are written through one setter.** Independent setters would let
  a caller land a normal weight above the bold one.
- **The preload got wider.** `loadTerminalFaces` now warms both weights *and*
  the italic of each, because the atlas caches whatever was resident when the
  pane opened (ADR terminal-atlas-renderer) and the weights are no longer fixed at 400/700.
- **Verification stays visual.** As with ADR terminal-atlas-renderer, nothing in CI can see that
  text is too heavy; the tests here pin which faces are requested and which
  options reach xterm, not how they rasterize.
