# The bundled faces are JetBrains Mono, Inter, and a symbol font

- **Status:** accepted
- **Date:** 2026-08-02

## Context

ADR terminal-typography-is-configurable bundled CaskaydiaMono Nerd Font as five static weights plus italics —
ten WOFF2 faces, 10.0MB — because a *patched* family shipping only 400 and 700
would collapse the terminal's five weight options onto two. It named JetBrains
Mono's patched set as one of the families that fails that test.

Nearly all of that payload is icons: the Nerd Fonts patch adds ~10,400 glyphs to
every face, so the same icon set was carried ten times. Separately, the UI ran on
IBM Plex Sans and IBM Plex Mono, which put three unrelated families on screen at
once — the chrome's sans, the chrome's mono, and the terminal's.

## Decision

1. **The terminal's text face is JetBrains Mono, bundled as its variable roman
   and italic.** Two files, 236KB. Variable is what makes it viable at all: the
   weight setting offers 300/350/400/600/700, JetBrains Mono ships no 350, and
   CSS resolves a requested 350 to the nearest declared face *below* it — so a
   static set would render Semilight as Light and make the default weight a
   no-op. A `wght` axis of 100–800 answers all five exactly, which is what ADR
   0050 needed ten static faces to do.

2. **Icons come from Symbols Nerd Font Mono, stacked behind every text face.**
   It carries the powerline and devicon glyphs and nothing else — no space, no
   Latin — so it answers every weight from one 1.2MB file. Terminal payload goes
   from 10.0MB to 1.4MB.

   This also fixes a hole rather than only shrinking things: previously a user
   who picked a system font got that font's icons or none, because the bundled
   patched face only backed families the webview could not resolve at all. The
   symbol face backs *every* choice, so a TUI's icons render whatever the text
   face is.

   The cost is that a patched face scales its icons to the host font's cell and
   a generic symbol face cannot. Symbols Nerd Font **Mono** is drawn single-cell
   for exactly this use, so the mismatch is bounded, and it is the same deal
   every terminal that supports font fallback offers.

3. **Ligatures are off app-wide, set on `body` and inherited.** JetBrains Mono's
   coding ligatures are `calt` alone, which `font-variant-ligatures: none`
   disables. Panes never form them regardless — an atlas renderer rasterises one
   cell at a time — but xterm's last-resort DOM renderer emits character runs,
   and so does every surface that reaches for `var(--font-mono)`. One inherited
   declaration is what keeps a future component from having to remember.

4. **The UI face is Inter, bundled as one variable file per style.** It replaces
   IBM Plex Sans for legibility at the 11–13.5px the chrome is built at, and
   `--font-mono` now points at the terminal's own JetBrains Mono, which retires
   IBM Plex Mono. The app draws two families, not three.

## Consequences

- **The symbol face has to be warmed explicitly, by name and with a private-use
  sample.** Both halves are load-bearing and neither is obvious. The default
  text `document.fonts.load` loads for is a space, which this face has no glyph
  for; and the call matches faces on the `unicode-range` descriptor rather than
  on real coverage, so — these faces declaring none — every family reads as
  covering everything and a font *stack* always resolves to its first entry.
  Asked for through the stack, the call fetches the text face a second time and
  the symbol face not at all. Canvas never loads a font on demand, so the miss
  is silent: the atlas caches tofu for the pane's life (ADR terminal-atlas-renderer), which is the
  same failure the weight preload exists to prevent.

  It is also partly self-concealing, because JetBrains Mono carries the
  powerline glyphs itself. Only the devicons vanish, so a prompt looks right
  and a tool's icons do not.
- **ADR terminal-typography-is-configurable's point 2 is superseded**; the rest of it — the settings surface,
  the 350 default and why, Go-side enumeration, advance-width monospace
  detection, and stacking a chosen family in front of the bundled one — stands.
  Storing the default as `""` rather than by name is what carries existing users
  onto the new face, which is what that clause was written for.
- **The vendored fonts are format conversions, not modifications.** Both are
  OFL-1.1 without a reserved font name; TTF→WOFF2 via `fonttools`. Provenance
  and version are recorded in `assets/fonts/terminal-fonts.css`.
- **Verification stays visual.** As with ADRs terminal-atlas-renderer and terminal-typography-is-configurable, nothing in CI can
  see that an icon landed a half-cell off or that text rasterises too heavy.
