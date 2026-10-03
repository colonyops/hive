import { computed, nextTick, onScopeDispose, ref, shallowReactive, shallowRef, watch, type Ref } from 'vue'
import { useTerminalWindows, type UseTerminalWindows } from '../../composables/useTerminalWindows'
import type { TerminalClient } from '../../lib/terminalClient'
import { paneMayAutoFocus } from '../../lib/terminalTree'

// How long the outgoing session may stand in for one that has not painted:
// long enough to cover a normal attach, short enough that a session with an
// empty screen (which sends no first paint at all) does not read as a dead
// click. Reached only on cold attaches; a pooled session reveals instantly.
export const HOLD_MS = 300

/** Selects `windowId` when it names a tab of `session` that is not already showing. */
export function selectIfWanted(session: UseTerminalWindows, windowId: string): void {
  if (!windowId || windowId === session.activeWindowId.value) return
  if (session.tabs.value.some((tab) => tab.windowId === windowId)) void session.select(windowId)
}

export type TerminalPool = ReturnType<typeof useTerminalPool>

/**
 * The Code view's live attaches. Switching sessions must not blank the pane, so
 * the last `size` attaches stay live (control client, stream and terminals
 * intact, panes hidden) and snapping back to one is a v-show flip. An attach is
 * let go only through `drop`: eviction, explicit close, list removal, and scope
 * disposal. `onEvicted` runs after an eviction, because an evicted session's
 * subtree falls back to a cached listing that predates its attach.
 */
export function useTerminalPool(options: { size: Ref<number>; onEvicted: () => void }) {
  const pool = shallowReactive(new Map<string, UseTerminalWindows>())
  const lastUsed: string[] = []
  const activeSlug = ref('')
  // The attached session, as opposed to the visible one: set on every real
  // attach, left alone by the route-driven detach, and cleared only when `drop`
  // lets that session go. The App-level window projection is keyed off it.
  const lastAttachedSlug = ref('')
  const current = computed(() => (activeSlug.value ? (pool.get(activeSlug.value) ?? null) : null))
  // What the main area shows. It lags the selection during a cold attach: the
  // outgoing session holds the pane until the incoming one has painted, ended,
  // or the hold cap fired, so a switch never shows a blank grid.
  const displayed = shallowRef<UseTerminalWindows | null>(null)
  const displayedSlug = ref('')
  const visible = computed(() => displayed.value ?? current.value)
  // A reorder started during a hold must reach the session on screen, not the selected one.
  const visibleSlug = computed(() => (displayed.value ? displayedSlug.value : activeSlug.value))

  const revealed = new WeakSet<UseTerminalWindows>()
  let holdTimer: ReturnType<typeof setTimeout> | undefined

  watch(
    [current, () => current.value?.painted.value, () => current.value?.status.value],
    () => {
      clearTimeout(holdTimer)
      const incoming = current.value
      if (!incoming || incoming === displayed.value) return
      if (!displayed.value || revealed.has(incoming) || incoming.painted.value || incoming.status.value === 'ended') {
        reveal(incoming)
        return
      }
      holdTimer = setTimeout(() => {
        if (current.value === incoming) reveal(incoming)
      }, HOLD_MS)
    },
    { immediate: true },
  )

  function reveal(incoming: UseTerminalWindows): void {
    revealed.add(incoming)
    displayed.value = incoming
    displayedSlug.value = activeSlug.value
    void nextTick(() => {
      if (displayed.value === incoming && paneMayAutoFocus.value) incoming.focusActive()
    })
  }

  function touch(slug: string): void {
    const at = lastUsed.indexOf(slug)
    if (at !== -1) lastUsed.splice(at, 1)
    lastUsed.push(slug)
    evictOverLimit()
  }

  function evictOverLimit(): void {
    let evicted = false
    for (const victim of [...lastUsed]) {
      if (pool.size <= options.size.value) break
      if (victim !== activeSlug.value && pool.get(victim) !== displayed.value) {
        drop(victim)
        evicted = true
      }
    }
    if (evicted) options.onEvicted()
  }

  watch(options.size, evictOverLimit)

  function drop(slug: string): void {
    const entry = pool.get(slug)
    if (!entry) return
    if (displayed.value === entry) displayed.value = null
    entry.dispose()
    pool.delete(slug)
    const at = lastUsed.indexOf(slug)
    if (at !== -1) lastUsed.splice(at, 1)
    if (lastAttachedSlug.value === slug) lastAttachedSlug.value = ''
  }

  // `wantedWindow` is taken by value: once windows land, the route mirror
  // rewrites ?window to tmux's active one, and the wanted one must survive that.
  function open(slug: string, client: TerminalClient, wantedWindow: string): void {
    activeSlug.value = slug
    const pooled = pool.get(slug)
    if (pooled && pooled.status.value !== 'ended') {
      touch(slug)
      lastAttachedSlug.value = slug
      selectIfWanted(pooled, wantedWindow)
      return
    }
    if (pooled) drop(slug)
    const opened = useTerminalWindows(slug, client)
    pool.set(slug, opened)
    touch(slug)
    lastAttachedSlug.value = slug
    void opened.start().then(() => {
      if (pool.get(slug) === opened) selectIfWanted(opened, wantedWindow)
    })
  }

  // Leaving for the picker keeps the pool warm.
  function detach(): void {
    activeSlug.value = ''
    displayed.value = null
  }

  // A session ending moves nothing else a window sweep watches, so it is a
  // sweep trigger of its own.
  const endedSlugs = computed(() =>
    [...pool.entries()]
      .filter(([, session]) => session.status.value === 'ended')
      .map(([slug]) => slug)
      .join(' '),
  )

  // Every pooled session's panes stay mounted: a Terminal binds to one element
  // for its lifetime, and an incoming session's first paint has to land while
  // its panes are still hidden behind the held one.
  const paneSessions = computed(() =>
    [...pool.entries()].map(([slug, entry]) => ({
      slug,
      entry,
      tabs: entry.tabs.value,
      activeWindowId: entry.activeWindowId.value,
    })),
  )

  onScopeDispose(() => {
    clearTimeout(holdTimer)
    for (const slug of [...pool.keys()]) drop(slug)
  })

  return {
    pool,
    activeSlug,
    lastAttachedSlug,
    current,
    visible,
    visibleSlug,
    endedSlugs,
    paneSessions,
    open,
    detach,
    drop,
  }
}
