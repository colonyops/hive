# Keybindings are chord sequences, not a leader key

- **Status:** accepted
- **Date:** 2026-08-29

## Context

The keymap resolved only a single chord, so Zed/Vim-style navigation (`g i`,
`g c`) and multi-step chords (`⌘K ⌘S`) could not be expressed.

## Decision

**Any key becomes a leader by being the first step of some binding.** There is
no leader concept or reserved prefix. A binding is a space-separated string of
canonical combos, stored as one string per command id in `settings.yaml`'s
`keybindings:` map.

**What a keystroke means is a pure transition function**, `stepSequence` in
`composables/useKeybindings.ts`. Given the pending steps and a combo it returns
`run`, `extend`, `swallow`, or `pass` and mutates nothing. `app/useGlobalKeymap.ts`
owns the `pendingSequence` write, the deferred-dispatch timer, and the
suppression checks, so the state machine is testable without a mounted app,
events, or timers.

**Zed's prefix rule.** A step that is both a complete binding and a prefix of a
longer one defers for `SEQUENCE_TIMEOUT_MS` (1000ms). A continuation cancels it;
otherwise it fires through the same context and overlay gate as a chord,
checked at fire time. So **a default binding must never bind a leader alone**:
it would make that key pause a second before every use. `g` binds nothing by
default.

**A keystroke with the primary modifier (`mod`) cancels a pending sequence and
falls through** to normal dispatch (`g` then `⌘K` opens the palette). Only an
unmatched bare key is cleared and swallowed.

**A sequence starts only outside an editable target and outside an overlay.** A
focus change into an editable target clears a pending one. Over a terminal pane,
only a sequence whose first step uses `mod` can start
(ADR a-binding-that-starts-with-the-primary-modifier-works-from-a-terminal-pane).

## Consequences

- A new navigation destination is one `defaultCombos: ['g <letter>']` catalog
  entry.
- Because a sequence cannot start under an overlay, `g t` opens Tasks but cannot
  close it. A command that must open and close an overlay keeps a plain chord
  too (`tasks.toggle` keeps `mod+shift+t`).
- The recorder (`KeybindingSettingsView.vue`) and the `?` Keys scope read the
  same grammar through `canonicalizeBinding` and `keymapRows`, so neither can
  drift from the dispatcher.
