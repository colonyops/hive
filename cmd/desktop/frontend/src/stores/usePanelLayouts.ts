import { useStorage } from '@vueuse/core'
import { isPanelLayout, newPanelLayout, type PanelLayout } from '../lib/panelLayout'
import { defineStore } from './defineStore'

/**
 * Every persisted panel layout of one host, by key. Code keys a session's
 * layout by the session's id, because a rename changes the slug. The rules are
 * in lib/panelLayout; this owns where the result lives, and it persists so a
 * restart restores it.
 */
function definePanelLayouts(name: string, storageKey: string) {
  return defineStore(name, () => {
    const layouts = useStorage<Record<string, PanelLayout>>(storageKey, {}, undefined, { shallow: true })
    const fresh = newPanelLayout()

    function layoutOf(key: string): PanelLayout {
      const stored = layouts.value[key]
      return isPanelLayout(stored) ? stored : fresh
    }

    function apply(key: string, rule: (layout: PanelLayout) => PanelLayout): void {
      const current = layoutOf(key)
      const next = rule(current)
      if (next !== current) layouts.value = { ...layouts.value, [key]: next }
    }

    function forget(key: string): void {
      if (!(key in layouts.value)) return
      const { [key]: _gone, ...rest } = layouts.value
      layouts.value = rest
    }

    /** Lets go of every layout `keep` says no longer has an owner. */
    function retain(keep: (key: string) => boolean): void {
      const kept = Object.entries(layouts.value).filter(([key]) => keep(key))
      if (kept.length !== Object.keys(layouts.value).length) layouts.value = Object.fromEntries(kept)
    }

    return { layoutOf, apply, forget, retain }
  })
}

export const usePanelLayouts = definePanelLayouts('panelLayouts', 'hive.code.panel-layouts')

/** Chats keeps its own map: a chat pinned to Code shares its id, and Code's sweep drops keys it does not know. */
export const useChatPanelLayouts = definePanelLayouts('chatPanelLayouts', 'hive.chats.panel-layouts')
