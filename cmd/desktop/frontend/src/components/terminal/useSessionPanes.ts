import { useTimeoutFn } from '@vueuse/core'
import { computed, nextTick, ref, watch } from 'vue'
import {
  focusedWindow,
  moveTab,
  openView,
  reconcileWindows,
  revealWindow,
  windowOrder,
  windowsOnScreen,
  type SessionLayout,
  type SessionViewKind,
} from '../../lib/sessionLayout'
import { paneMayAutoFocus } from '../../lib/terminalTree'
import { useSessionTabs } from '../../stores/useSessionTabs'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
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

  // A pane a "+" named for the window it is making.
  const newWindowPane = ref<number | null>(null)
  const { start: expirePending, stop: keepPending } = useTimeoutFn(
    () => (newWindowPane.value = null),
    PENDING_WINDOW_MS,
    { immediate: false },
  )

  watch(
    [key, windowIds, activeWindow, () => pool.visible.value?.status.value],
    ([sessionKey, ids, active, status], before) => {
      // No windows while attaching says nothing. After an end, the windows are
      // gone and their tabs go with them.
      if (!ids.length && status !== 'ended') return
      const target = newWindowPane.value ?? undefined
      apply((current) => reconcileWindows(current, ids, { active, target }))
      if (!before || before[0] !== sessionKey) return
      const known = before[1] ?? []
      if (ids.some((id) => !known.includes(id))) {
        newWindowPane.value = null
        keepPending()
      }
      // A first active window is an attach landing, which must not pull the
      // restored tabs away from what they show.
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

  function expectWindowIn(pane: number | undefined): void {
    newWindowPane.value = pane ?? null
    if (pane !== undefined) expirePending()
    paneMayAutoFocus.value = true
  }

  async function newWindow(target: TerminalSessionRow, pane?: number): Promise<void> {
    expectWindowIn(pane)
    await ops.newWindowIn(target)
  }

  async function newAgentWindow(target: TerminalSessionRow, agent: string, pane?: number): Promise<void> {
    expectWindowIn(pane)
    await ops.newAgentWindowIn(target, agent)
  }

  function openViewIn(target: TerminalSessionRow, kind: SessionViewKind, pane?: number): void {
    sessionTabs.apply(target.id, (current) => openView(current, kind, pane))
    if (target.slug !== pool.visibleSlug.value) ops.selectRow(target.slug)
  }

  // Which views make sense for a session. A pinned chat and the scratch terminal
  // have no session checkout, and a server's session has none on this machine.
  function viewsFor(target: TerminalSessionRow): SessionViewKind[] {
    const views: SessionViewKind[] = []
    if (target.canvasOwner || tree.isChat(target)) views.push('docs')
    if (tree.isHiveSession(target) && !tree.isServerSession(target)) views.push('diff', 'tasks')
    return views
  }

  // tmux's order drives the window chords, so a tab that moves takes its window along.
  function syncWindowOrder(windowId: string): void {
    const position = windowOrder(layout.value).indexOf(windowId)
    if (position >= 0) void pool.visible.value?.moveWindow(windowId, position)
  }

  function moveTabTo(tabId: string, pane: number, gap: number): void {
    apply((current) => moveTab(current, tabId, pane, gap))
    if (tabId.startsWith('window:')) syncWindowOrder(tabId.slice('window:'.length))
  }

  return {
    row,
    key,
    layout,
    onScreen,
    apply,
    renderOnScreen,
    newWindow,
    newAgentWindow,
    openViewIn,
    viewsFor,
    moveTabTo,
    syncWindowOrder,
  }
}
