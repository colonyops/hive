import { computed, watch } from 'vue'
import { openTab } from '../../lib/panelLayout'
import { usePanelLayouts } from '../../stores/usePanelLayouts'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import { viewTab, viewsFor, type SessionViewKind } from './panelViews'
import { useSessionPanels, type PanelWindowSource } from './useSessionPanels'
import type { TerminalPool } from './useTerminalPool'
import type { TerminalSessionOps } from './useTerminalSessionOps'
import type { TerminalTree } from './useTerminalTree'

export type SessionPanels = ReturnType<typeof useCodePanels>

const SESSION_CLOSED = ['terminated', 'stopped', 'completed']

/** The Code view's panels: the visible session's attach, and the rows and ops around it. */
export function useCodePanels(options: { pool: TerminalPool; tree: TerminalTree; ops: TerminalSessionOps }) {
  const { pool, tree, ops } = options
  const layouts = usePanelLayouts()

  const row = computed(
    () => tree.attachable.value.find((candidate) => candidate.slug === pool.visibleSlug.value) ?? null,
  )
  const key = computed(() => row.value?.id ?? pool.visibleSlug.value)

  const windowIds = computed(() => pool.visible.value?.tabs.value.map((tab) => tab.windowId) ?? [])
  const endReason = computed(() => pool.visible.value?.endReason.value ?? '')
  const source: PanelWindowSource = {
    key,
    windowIds,
    activeWindow: computed(() => pool.visible.value?.activeWindowId.value ?? ''),
    closed: computed(() => SESSION_CLOSED.includes(endReason.value)),
    pending: computed(() => !windowIds.value.length && endReason.value !== 'not-started'),
    select: (windowId) => void pool.visible.value?.select(windowId),
    moveWindow: (windowId, position) => void pool.visible.value?.moveWindow(windowId, position),
    render(onScreen) {
      const session = pool.visible.value
      if (!session) return
      session.renderShown()
      for (const windowId of onScreen) session.revealWindow(windowId)
    },
    focus: () => pool.visible.value?.focusActive(),
  }
  const core = useSessionPanels(source, layouts)

  async function newWindow(target: TerminalSessionRow, panel?: string): Promise<void> {
    core.expectWindowIn(panel)
    await ops.newWindowIn(target)
  }

  async function newAgentWindow(target: TerminalSessionRow, agent: string, panel?: string): Promise<void> {
    core.expectWindowIn(panel)
    await ops.newAgentWindowIn(target, agent)
  }

  function openViewIn(target: TerminalSessionRow, kind: SessionViewKind, panel?: string): void {
    layouts.apply(target.id, (current) => openTab(current, viewTab(kind), panel))
    if (target.slug !== pool.visibleSlug.value) ops.selectRow(target.slug)
  }

  // A session deleted or recycled while not on screen says nothing to the watcher
  // in useSessionPanels. A server that has not answered says nothing about its
  // sessions, so layouts wait until every server has, and a pinned chat's row
  // needs the recents.
  watch(
    [tree.attachable, tree.sessionsLoaded, tree.serversLoaded, tree.recentsLoaded],
    ([rows, loaded, serversLoaded, recentsLoaded]) => {
      if (!loaded || !serversLoaded || !recentsLoaded || tree.sessionsError.value || tree.serversError.value) return
      if (tree.offlineServers.value.length) return
      const known = new Set(rows.flatMap((candidate) => [candidate.id, candidate.slug]))
      layouts.retain((sessionKey) => known.has(sessionKey))
    },
    { immediate: true },
  )

  return {
    row,
    key,
    ...core,
    newWindow,
    newAgentWindow,
    openViewIn,
    viewsFor: (target: TerminalSessionRow): SessionViewKind[] => viewsFor(target, tree),
  }
}
