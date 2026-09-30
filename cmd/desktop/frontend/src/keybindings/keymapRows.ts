import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { commands } from './catalog'
import type { CommandContext } from './catalog'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'

// One derivation of "command + effective bindings" shared by the keybindings
// editor and the `?` palette scope, so the two surfaces cannot drift on what a
// command's current bindings are.

export interface KeymapRow {
  id: string
  title: string
  group: string
  context: CommandContext
  /** Canonical bindings, sequences included. */
  combos: string[]
  /** Display form per binding (formatCombo), same order as combos. */
  formatted: string[]
  overridden: boolean
  /** Extra palette match terms, from the catalog. */
  keywords: string[]
}

const kb = useKeybindings()

/** Live rows over the catalog and the effective keymap. */
export const keymapRows: ComputedRef<KeymapRow[]> = computed(() =>
  commands.value.map((command) => {
    const combos = kb.combosFor(command.id)
    return {
      id: command.id,
      title: command.title,
      group: command.group,
      context: command.context,
      combos,
      formatted: combos.map((combo) => formatCombo(combo)),
      overridden: kb.isOverridden(command.id),
      keywords: command.keywords ?? [],
    }
  }),
)

/**
 * One-shot handshake: the `?` scope sets this before routing to Settings ›
 * Keyboard; the editor applies it to its filter on the next mount and clears
 * it, so a stale request never re-applies to an unrelated visit.
 */
export const requestedEditorFilter: Ref<string | null> = ref(null)
