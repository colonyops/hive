import { useStorage, watchDebounced } from '@vueuse/core'
import { computed, ref } from 'vue'
import { Report } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/layoutservice'
import { toLayoutReport } from '../lib/layoutReport'
import { createLogger } from '../lib/log'
import { isPanelLayout, newPanelLayout, type PanelLayout } from '../lib/panelLayout'
import { defineStore } from './defineStore'

const log = createLogger('panel-layouts')

/**
 * Every persisted panel layout of one host, by key. Code keys a session's
 * layout by the session's id, because a rename changes the slug. The rules are
 * in lib/panelLayout; this owns where the result lives, and it persists so a
 * restart restores it.
 */
function definePanelLayouts(name: string, storageKey: string, kind: 'code' | 'chats') {
  return defineStore(name, () => {
    const layouts = useStorage<Record<string, PanelLayout>>(storageKey, {}, undefined, { shallow: true })
    const fresh = newPanelLayout()
    // The key the user is looking at, or '' while this host is off screen.
    const onScreen = ref('')

    // Go keeps a copy for the agents' get_layout, which is the only way an
    // agent learns what it is addressing: the layout lives here, in the browser.
    // The debounce is short because an agent reads right after it writes.
    watchDebounced(
      [layouts, onScreen],
      () => {
        Report(toLayoutReport(kind, onScreen.value, layouts.value)).catch((error: unknown) => {
          log.debug('the layout report did not reach the app', error)
        })
      },
      { debounce: 50, immediate: true },
    )

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

    function markOnScreen(key: string): void {
      onScreen.value = key
    }

    return {
      layoutOf,
      apply,
      forget,
      retain,
      onScreen,
      markOnScreen,
      /** The stored record as it is, for a reader that reports every layout. */
      layouts: computed(() => layouts.value),
    }
  })
}

export const usePanelLayouts = definePanelLayouts('panelLayouts', 'hive.code.panel-layouts', 'code')

/** Chats keeps its own map: a chat pinned to Code shares its id, and Code's sweep drops keys it does not know. */
export const useChatPanelLayouts = definePanelLayouts('chatPanelLayouts', 'hive.chats.panel-layouts', 'chats')
