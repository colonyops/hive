# The bundled faces are JetBrains Mono, Inter, and a symbol font

- **Status:** accepted
- **Date:** 2026-08-02

## Context

The terminal's weight setting offers 300/350/400/600/700 (ADR
terminal-typography-is-configurable), so the bundled face must answer all five
exactly. A Nerd Fonts patched family repeats about ten thousand icon glyphs in
every face, so shipping five static weights carries the same icon set many
times over. The UI also needs to settle on few families rather than one sans,
one mono, and one terminal face.

## Decision

1. **The terminal's text face is JetBrains Mono, bundled as its variable roman
   and italic** (`cmd/desktop/frontend/src/assets/fonts/`). JetBrains Mono
   ships no static 350, and CSS resolves a requested 350 to the nearest face
   below it. The `wght` axis (100-800) answers all five weights exactly.

2. **Icons come from Symbols Nerd Font Mono, stacked behind every text face.**
   It carries the powerline and devicon glyphs and no Latin, so one file
   answers every weight, and it backs every family a user picks, so a TUI's
   icons render whatever the text face is. Its icons do not scale to the host
   font's cell the way a patched face's do; the Mono cut is drawn single-cell,
   which bounds the mismatch.

3. **Ligatures are off app-wide** (`font-variant-ligatures: none`, inherited).
   Atlas renderers never form them, but xterm's DOM renderer and every surface
   using `var(--font-mono)` emit character runs.

4. **The UI face is Inter** (`@fontsource-variable/inter`), and `--font-mono`
   points at the terminal's JetBrains Mono (`web/packages/ui/src/styles/theme.css`).
   The app draws two families.

## Consequences

- **The symbol face must be warmed explicitly, by name and with a private-use
  sample** (`terminalFaces.ts`). `document.fonts.load` defaults to a space,
  which this face lacks, and it matches on `unicode-range` rather than real
  coverage, so a font stack always resolves to its first entry. Canvas never
  loads a font on demand, so a miss caches tofu for the pane's life (ADR
  terminal-atlas-renderer). The miss is easy to overlook, because JetBrains
  Mono carries the powerline glyphs itself and only devicons vanish.
- The vendored fonts are OFL-1.1 format conversions (TTF to WOFF2), with
  provenance recorded in `assets/fonts/terminal-fonts.css`.
- Verification is visual; nothing in CI sees an icon land a half-cell off.
