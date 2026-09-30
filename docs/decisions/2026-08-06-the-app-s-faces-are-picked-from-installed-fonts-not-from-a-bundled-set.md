# The app's faces are picked from installed fonts, not from a bundled set

- **Status:** accepted
- **Date:** 2026-08-06

## Context

The chrome's two faces were fixed at build time by ADR
bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font — Inter for
`--font-sans`, JetBrains Mono for `--font-mono`. Issue #19 asks for both to be
user-selectable, and proposed the shape a web app would take: bundle two or
three more families per role, offer `system-ui` alongside them, and add a
free-text box for anything else, because `queryLocalFonts()` does not exist in
WKWebView so the app cannot know what is installed.

That last premise stopped being true. `internal/app/fonts` already scans the
platform font directories and hands the frontend a list of family names, because
the terminal's font picker needed exactly this (ADR
terminal-typography-is-configurable). It filtered the result to fixed-pitch
families, which is the only reason it could not answer for a UI face.

## Decision

1. **Both faces are chosen from what the machine has, by name.** The scan now
   reports every family it parsed alongside the fixed-pitch subset, so the
   interface picker offers all of them and the monospace picker offers the
   subset. Nothing new is bundled: a curated set of two or three families is a
   guess at taste paid for in binary size on every install, and it is strictly
   worse than the list the user already curated by installing fonts. The
   bundled faces stay exactly as that ADR set them, as the default and as the
   fallback.

   This also retires the free-text escape hatch the issue proposed. Enumeration
   makes it redundant, and a text box that silently does nothing when the name
   is misspelled is the worse of the two controls.

2. **A choice is a family name stacked in front of the bundled one**, so a name
   the scan reported but the webview cannot resolve — or one uninstalled since
   it was picked — degrades to the shipped face rather than to whatever
   `sans-serif` happens to be. The name is quoted on the way into the custom
   property: a family reaches this from a font file's name table or a
   hand-edited `settings.yaml`, and an unbalanced quote there would invalidate
   the whole declaration, taking the fallback down with the bad name.

3. **The default is persisted as `""` and the platform stack as its CSS
   keyword** (`system-ui`, `ui-monospace`). Empty tracks whatever the bundled
   face is, which is what carried users across the IBM Plex → Inter change and
   would carry them across the next one; writing today's family name instead
   would pin it. The System option needs no bundled asset at all, which is why
   it is a keyword rather than a third sentinel.

4. **The chrome's faces and a terminal's are separate settings.** `appearance.
   font_family` / `mono_font_family` are set from Settings ▸ Appearance beside
   the theme, and `appearance.terminal_font_family` stays on Settings ▸ Terminal
   — the surface each value changes is what files it (ADR
   settings-sections-name-the-surface-they-change). A pane's text is drawn by an
   atlas renderer against a measured cell and the chrome's is not, so the two
   have different constraints and no shared control would serve both.

5. **The choice is cached in `localStorage` and reconciled against
   `settings.yaml` after mount**, the ordering `useTheme` already uses and for
   the same reason: the durable record cannot be read synchronously, so
   blocking the first paint on it would cause the flash the setting is supposed
   to prevent. Unlike the theme, `settings.yaml` wins outright — including when
   it is empty. There is no earlier build whose localStorage-only choice has to
   be adopted, and empty is a real selection here rather than "nothing
   persisted", so a stale cache must not resurrect a font the user cleared.

## Consequences

- **The scan is no longer a monospace scan.** `fonts.Lister` returns both lists
  from one pass and one cache, and the frontend reads them through a single
  composable, so the app and terminal pickers cannot disagree about what is
  installed or pay for two scans. A family whose files disagree about pitch
  counts as monospace if any of them is — the italic of a fixed-pitch family
  often is not, and dropping the family over that would hide it from the
  terminal picker, which is the behaviour the previous filter already had.
- **A font installed while the app is running does not appear until the next
  launch.** The scan parses every font file on the machine, so it is cached for
  the process. Unchanged from the terminal picker, and the same deal every
  terminal emulator offers.
- **Verification stays visual.** Nothing in CI can see that a chosen face
  rasterises too heavy at 11–13px, which is where the chrome lives; the pane's
  preview block is drawn at those sizes rather than as a display-size specimen
  for that reason.
- **No `font:*` command-palette entries.** `theme:*` enumerates a closed set of
  fourteen; the font list is however many families the machine has, which would
  swamp the palette. The pane is the surface for this.
