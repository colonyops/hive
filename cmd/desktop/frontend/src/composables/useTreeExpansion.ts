import { useStorage } from '@vueuse/core'

export interface TreeExpansion<N> {
  expanded: (node: N) => boolean
  toggle: (node: N) => void
  /** Opens a folded node. One the default already opens keeps no stored entry. */
  unfold: (node: N) => void
  setAll: (open: boolean) => void
}

/**
 * Fold state of a sidebar tree's groups, kept in localStorage under
 * `storageKey`: it is view state, not configuration. A group with no stored
 * entry takes `defaultOpen`, so a tree of dormant groups stays out of the way
 * of the live ones, and an explicit toggle always wins over the default.
 * While `filtering()` holds every group is open: it is only listed because
 * something in it matched, and a folded one would hide the match.
 */
export function useTreeExpansion<N>(
  storageKey: string,
  options: {
    key: (node: N) => string
    defaultOpen: (node: N) => boolean
    filtering: () => boolean
    /** Every group, not only the drawn ones, so a bulk fold also covers the filtered-away ones. */
    nodes: () => readonly N[]
  },
): TreeExpansion<N> {
  const stored = useStorage<Record<string, boolean>>(storageKey, {})

  function expanded(node: N): boolean {
    if (options.filtering()) return true
    return stored.value[options.key(node)] ?? options.defaultOpen(node)
  }

  return {
    expanded,
    toggle: (node) => {
      stored.value[options.key(node)] = !expanded(node)
    },
    unfold: (node) => {
      if (!expanded(node)) stored.value[options.key(node)] = true
    },
    setAll: (open) => {
      for (const node of options.nodes()) stored.value[options.key(node)] = open
    },
  }
}
