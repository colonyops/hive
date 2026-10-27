# The app's faces are picked from installed fonts, not from a bundled set

- **Status:** accepted
- **Date:** 2026-08-06

## Context

The chrome's two faces, `--font-sans` and `--font-mono`, default to the bundled
Inter and JetBrains Mono (ADR bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font).
Users want to choose them. `queryLocalFonts()` does not exist in WKWebView, but
`cmd/desktop/internal/app/fonts` already scans the platform font directories
for the terminal's font picker (ADR terminal-typography-is-configurable).

## Decision

1. **Both faces are chosen by name from what the machine has.** The scan
   returns `Families{All, Monospace}`: the interface picker offers `All`, the
   monospace picker `Monospace`. Nothing more is bundled, and there is no
   free-text box, since enumeration makes it redundant and a misspelled name
   would silently do nothing.
2. **A choice is a family name stacked in front of the bundled one**
   (`appFontStacks` in `lib/appFaces.ts`), so a name the webview cannot resolve,
   or one uninstalled since, degrades to the shipped face. The name is quoted,
   because it comes from a font file or a hand-edited `settings.yaml` and an
   unbalanced quote would invalidate the whole declaration.
3. **The default is persisted as `""` and the System option as its CSS
   keyword** (`system-ui`, `ui-monospace`). Empty tracks whatever the bundled
   face is; writing today's family name would pin it.
4. **The chrome's faces and a terminal's are separate settings.**
   `appearance.font_family` / `mono_font_family` live in Settings > Appearance;
   `appearance.terminal_font_family` stays in Settings > Terminal
   (ADR settings-sections-name-the-surface-they-change). A pane is drawn by an
   atlas renderer against a measured cell and the chrome is not.
5. **The choice is cached in `localStorage` and reconciled against
   `settings.yaml` after mount** (`useAppFont`), so the first paint does not
   flash the bundled face. `settings.yaml` wins outright, including when empty,
   so a stale cache cannot resurrect a font the user cleared.

## Consequences

- `fonts.Lister` returns both lists from one scan and one cache, so the app and
  terminal pickers cannot disagree. A family counts as monospace if any of its
  faces is, since the italic of a fixed-pitch family often is not.
- A font installed while the app runs does not appear until the next launch.
- There are no `font:*` palette entries: the list is open-ended and would swamp
  the palette.
