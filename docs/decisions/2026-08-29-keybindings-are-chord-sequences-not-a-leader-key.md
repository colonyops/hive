# Keybindings are chord sequences, not a leader key

- **Status:** accepted
- **Date:** 2026-08-29

## Context

The keymap could resolve only a single chord (`canonicalizeCombo` split on
`+`, one combo -> one command). `g i`, `g c`, and the rest of Zed/Vim-style
navigation were unrepresentable, and the missing macOS standards (`⌘,`,
`⌘[`/`⌘]`, a shared `/` focus-search) needed the same grammar a sequence does:
a binding that is more than one keystroke, dispatched only once the whole
thing is typed.

## Decision

**Any key becomes a leader simply by being the first step of some binding.**
There is no leader concept, no reserved prefix, and no config surface for
"which key is the leader" -- a binding is just a space-separated string of
canonical combos (`g i`, `⌘K ⌘S`), opaque to everything below the frontend.
settings.yaml's `keybindings:` map already stored opaque combo strings per
command id; a sequence is still one string in that same map, so this needed
no schema change and no migration.

The whole decision of what a keystroke means -- given the steps accepted so
far -- is a **pure transition function**, `stepSequence` in
`useKeybindings.ts`. It reads the live keymap and returns one of `run` /
`extend` / `swallow` / `pass`; it never mutates state itself. `App.vue` owns
only the module-state write (`pendingSequence`), the deferred-dispatch timer,
and the suppression checks below -- everything a component needs DOM access
and lifecycle for, nothing a unit test needs a mounted app for. This split is
what makes the whole state machine (prefix matching, the timeout race, the
mod-cancels rule) testable with no `KeyboardEvent` and no timers.

**Zed's prefix rule**: a step that is both a complete binding and the prefix
of a longer one does not fire immediately -- it defers for
`SEQUENCE_TIMEOUT_MS` (1000ms) so a continuation still gets its chance. A
continuation arriving first cancels the deferred dispatch; nothing arriving
lets it fire on the timeout, through the same context/overlay gate an
ordinary chord uses (checked at fire time, not at arm time -- state can change
in that second). Because of this rule, **a default binding must never bind a
leader alone**: doing so would make every prefix of every sequence pause for a
second before its own single-key meaning could run, which is a tax the user
never asked for. `g` itself, for example, binds nothing by default -- only `g
i` / `g c` / `g a` / `g t` / `g s` do.

**A keystroke carrying the primary modifier (`mod`) cancels a pending
sequence and falls through to normal dispatch**, rather than being swallowed
as a typo: `g` then `⌘K` opens the palette, `g` then `⌘,` opens Settings. Only
an unmatched *bare* key (or a modifier-less variant like `shift+g`) is
cleared-and-swallowed. This is what keeps every existing modifier chord
reachable mid-sequence without an explicit escape.

**Suppression**: a sequence can only *start* outside an editable target and
outside an overlay -- the same doctrine that already governs a bare chord
(architecture.md's pierce/escape section). Continuing one already pending is
unaffected by that same check, because by the time either is open, whatever
got it there already cleared pending. A focused terminal pane owns every key,
so a sequence neither starts nor survives there: a keydown targeting the pane
and a `focusin` into one both clear it outright, with no hint pill ever
showing -- the same `focusin` listener clears pending on a focus change into
an editable target too, since a mouse click there is not a keystroke
`stepSequence` ever sees. `resolve(combo)` stays single-step-only -- it cannot match a
sequence's first step -- so the pierce/escape checks in
`useTerminalWindows.ts` (which resolve through the live keymap to decide what
reaches a focused pane) can never claim a sequence's leader as one of their
own single-chord exceptions.

**Accepted asymmetry**: because a sequence cannot *start* while an overlay
owns the screen, `g t` opens Tasks from anywhere but cannot close it again
once it is open -- pressing `g` while the Tasks overlay is up never becomes a
pending sequence, so `t` afterward is just an unbound keystroke. `tasks.toggle`
keeps its `mod+shift+t` chord specifically so the overlay has a way to close
that is not gated behind "a sequence would have to start under an overlay."
Sequences are for navigation; a toggle that must work both ways keeps a plain
chord.

## Consequences

A new navigation destination is one `defaultCombos: ['g <letter>']` catalog
entry -- no new machinery. A command that must open *and* close an overlay
needs its own chord alongside any sequence, per the accepted asymmetry above;
a sequence-only binding is not a substitute for one. The recorder
(`KeybindingSettingsView.vue`) and the `?` Keys scope both read this same
grammar through `canonicalizeBinding`/`keymapRows`, so neither can drift
from what the dispatcher actually resolves.
