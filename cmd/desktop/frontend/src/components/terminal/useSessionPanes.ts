import { useTimeoutFn } from '@vueuse/core'
import { computed, nextTick, ref, watch } from 'vue'
import {
  focusedWindow,
  placeWindow,
  reconcileWindows,
  revealWindow,
  windowsOnScreen,
  type SessionLayout,
} from '../../lib/sessionLayout'
import { paneMayAutoFocus } from '../../lib/terminalTree'
import { useSessionTabs } from '../../stores/useSessionTabs'
import type { TerminalPool } from './useTerminalPool'
import type { TerminalSessionOps } from './useTerminalSessionOps'
import type { TerminalTree } from './useTerminalTree'

export type SessionPanes = ReturnType<typeof useSessionPanes>

// A new window only has an id once tmux announces it. A request that never lands
// must not claim some later window.
const PENDING_WINDOW_MS = 5000

/**
 * Keeps the visible session's layout and its tmux windows in step. A layout
 * change selects the window it focuses, and a window tmux or a chord activated
 * shows in the layout. Both directions settle, because selecting the focused
 * window changes neither.
 */
export function useSessionPanes(options: { pool: TerminalPool; tree: TerminalTree; ops: TerminalSessionOps }) {
  const { pool, tree, ops } = options
  const sessionTabs = useSessionTabs()

  const row = computed(
    () => tree.attachable.value.find((candidate) => candidate.slug === pool.visibleSlug.value) ?? null,
  )
  const key = computed(() => row.value?.id ?? pool.visibleSlug.value)
  const layout = computed(() => sessionTabs.layoutOf(key.value))
  const onScreen = computed(() => windowsOnScreen(layout.value))

  function apply(rule: (layout: SessionLayout) => SessionLayout): void {
    sessionTabs.apply(key.value, rule)
  }

  const windowIds = computed(() => pool.visible.value?.tabs.value.map((tab) => tab.windowId) ?? [])
  const activeWindow = computed(() => pool.visible.value?.activeWindowId.value ?? '')

  const windowForSecondPane = ref(false)
  const { start: expirePending, stop: keepPending } = useTimeoutFn(
    () => (windowForSecondPane.value = false),
    PENDING_WINDOW_MS,
    { immediate: false },
  )

  watch(
    [key, windowIds, activeWindow],
    ([sessionKey, ids, active], before) => {
      // No windows means an attach in flight or an ended session, and neither
      // says the session's tabs are wrong.
      if (!ids.length) return
      apply((current) => reconcileWindows(current, ids, active))
      if (!before || before[0] !== sessionKey) return
      const known = before[1] ?? []
      const created = ids.find((id) => !known.includes(id))
      if (created && windowForSecondPane.value) {
        windowForSecondPane.value = false
        keepPending()
        apply((current) => placeWindow(current, created, 1))
      }
      // A first active window is an attach landing, which must not pull the
      // restored tabs back to the terminal.
      if (before[2] && active && active !== before[2]) apply((current) => revealWindow(current, active))
    },
    { immediate: true },
  )

  // Immediately after the layout, so select() runs before the DOM flip it waits for.
  watch([() => pool.visible.value, () => focusedWindow(layout.value)], ([session, windowId]) => {
    if (!session || !windowId || !session.tabs.value.some((tab) => tab.windowId === windowId)) return
    if (windowId !== session.activeWindowId.value) {
      void session.select(windowId)
      return
    }
    void nextTick(() => {
      session.renderShown()
      if (paneMayAutoFocus.value) session.focusActive()
    })
  })

  function renderOnScreen(): void {
    const session = pool.visible.value
    if (!session) return
    session.renderShown()
    for (const windowId of onScreen.value.keys()) session.revealWindow(windowId)
  }

  // The active window claims its renderer through select(); the one beside it
  // has to be asked.
  watch([() => pool.visible.value, onScreen], renderOnScreen, { flush: 'post' })

  async function newWindow(pane: number): Promise<void> {
    const target = row.value
    if (!target) return
    if (pane > 0) {
      windowForSecondPane.value = true
      expirePending()
    }
    paneMayAutoFocus.value = true
    await ops.newWindowIn(target)
  }

  async function newAgentWindow(pane: number, agent: string): Promise<void> {
    const target = row.value
    if (!target) return
    if (pane > 0) {
      windowForSecondPane.value = true
      expirePending()
    }
    paneMayAutoFocus.value = true
    await ops.newAgentWindowIn(target, agent)
  }

  return { row, key, layout, onScreen, apply, renderOnScreen, newWindow, newAgentWindow }
}
