import { useStorage } from '@vueuse/core'
import { isSessionLayout, newSessionLayout, type SessionLayout } from '../lib/sessionLayout'
import { defineStore } from './defineStore'

/**
 * Each Code session's tabs and split, keyed by session id because a rename
 * changes the slug. The rules are in lib/sessionLayout; this owns where the
 * result lives, and it persists so a restart restores them.
 */
export const useSessionTabs = defineStore('sessionTabs', () => {
  const layouts = useStorage<Record<string, SessionLayout>>('hive.code.session-tabs', {}, undefined, { shallow: true })
  const fresh = newSessionLayout()

  function layoutOf(key: string): SessionLayout {
    const stored = layouts.value[key]
    return isSessionLayout(stored) ? stored : fresh
  }

  function apply(key: string, rule: (layout: SessionLayout) => SessionLayout): void {
    const current = layoutOf(key)
    const next = rule(current)
    if (next !== current) layouts.value = { ...layouts.value, [key]: next }
  }

  function forget(key: string): void {
    if (!(key in layouts.value)) return
    const { [key]: _gone, ...rest } = layouts.value
    layouts.value = rest
  }

  /** Lets go of every session `keep` says no longer exists. */
  function retain(keep: (key: string) => boolean): void {
    const kept = Object.entries(layouts.value).filter(([key]) => keep(key))
    if (kept.length !== Object.keys(layouts.value).length) layouts.value = Object.fromEntries(kept)
  }

  return { layoutOf, apply, forget, retain }
})
