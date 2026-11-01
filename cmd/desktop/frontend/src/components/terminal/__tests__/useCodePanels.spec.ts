import { computed, effectScope, ref, shallowRef, type Ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { openTab, panelsOf, splitPanel } from '../../../lib/panelLayout'
import { paneMayAutoFocus } from '../../../lib/terminalTree'
import { focusedWindow, placeWindow } from '../../../lib/windowTabs'
import { usePanelLayouts } from '../../../stores/usePanelLayouts'
import { viewTab } from '../panelViews'
import { useCodePanels } from '../useCodePanels'

const KEY = 'session-1'

function fakeSession(windowIds: string[], active = windowIds[0] ?? '') {
  const tabs = ref(windowIds.map((windowId) => ({ windowId, name: windowId })))
  const activeWindowId = ref(active)
  return {
    tabs,
    activeWindowId,
    status: ref('live'),
    endReason: ref<string | null>(null),
    // As the real one does: a select makes the window active straight away.
    select: vi.fn((windowId: string) => {
      activeWindowId.value = windowId
      return Promise.resolve()
    }),
    focusActive: vi.fn(),
    renderShown: vi.fn(),
    revealWindow: vi.fn(),
    moveWindow: vi.fn().mockResolvedValue(undefined),
  }
}

type FakeSession = ReturnType<typeof fakeSession>

function mountPanels(session: FakeSession, recentsLoaded = ref(true), tree: { isChat?: boolean } = {}) {
  const visible = shallowRef<FakeSession | null>(session)
  const row = { id: KEY, slug: 'hive-one' }
  const rows: Ref<(typeof row)[]> = ref([row])
  const ops = { newWindowIn: vi.fn().mockResolvedValue(undefined), newAgentWindowIn: vi.fn(), selectRow: vi.fn() }
  const fakeTree = {
    attachable: computed(() => rows.value),
    sessionsLoaded: ref(true),
    serversLoaded: ref(true),
    recentsLoaded,
    sessionsError: ref(null),
    serversError: ref(''),
    offlineServers: ref([]),
    isChat: () => !!tree.isChat,
    isHiveSession: () => !tree.isChat,
    isServerSession: () => false,
  }
  const visibleSlug = ref('hive-one')
  const pool = { visible, visibleSlug }
  const scope = effectScope()
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the composable reads only these parts
  const panels = scope.run(() => useCodePanels({ pool, tree: fakeTree, ops } as any))!
  return { panels, ops, rows, scope, row, visibleSlug }
}

describe('useCodePanels', () => {
  beforeEach(() => {
    localStorage.clear()
    paneMayAutoFocus.value = true
  })

  describe('a window the user asked for', () => {
    it('comes forward even from behind a view, in the panel that asked', async () => {
      const session = fakeSession(['@1'])
      const { panels, row } = mountPanels(session)
      await flushPromises()
      const layouts = usePanelLayouts()
      layouts.apply(KEY, (layout) => splitPanel(openTab(layout, viewTab('diff')), 'p0', 'row'))
      const target = panelsOf(layouts.layoutOf(KEY))[1].id

      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      await panels.newWindow(row as any, target)
      session.tabs.value = [...session.tabs.value, { windowId: '@2', name: '@2' }]
      session.activeWindowId.value = '@2'
      await flushPromises()

      expect(panelsOf(layouts.layoutOf(KEY))[1].activeTab).toBe('window:@2')
      expect(focusedWindow(layouts.layoutOf(KEY))).toBe('@2')
    })

    it('moves the tmux window to match when the new tab lands mid-order', async () => {
      const session = fakeSession(['@1', '@2'])
      const { panels, row } = mountPanels(session)
      await flushPromises()
      usePanelLayouts().apply(KEY, (layout) => {
        const split = splitPanel(layout, 'p0', 'row')
        return placeWindow(split, '@2', panelsOf(split)[1].id)
      })
      await flushPromises()

      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      await panels.newWindow(row as any, 'p0')
      session.tabs.value = [...session.tabs.value, { windowId: '@3', name: '@3' }]
      await flushPromises()

      expect(session.moveWindow).toHaveBeenCalledWith('@3', 1)
    })

    it('asks the session to open the window and the agent window', async () => {
      const { panels, ops, row } = mountPanels(fakeSession(['@1']))
      await flushPromises()

      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      await panels.newWindow(row as any)
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      await panels.newAgentWindow(row as any, 'claude')

      expect(ops.newWindowIn).toHaveBeenCalledWith(row)
      expect(ops.newAgentWindowIn).toHaveBeenCalledWith(row, 'claude')
    })
  })

  describe('views', () => {
    it('offers Drive, Diff and Tasks as the registry says for the row', () => {
      const hive = mountPanels(fakeSession(['@1']))
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      expect(hive.panels.viewsFor(hive.row as any)).toEqual(['diff', 'tasks'])

      const chat = mountPanels(fakeSession(['@1']), ref(true), { isChat: true })
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      expect(chat.panels.viewsFor(chat.row as any)).toEqual(['docs'])
    })

    it('opens a view in the row it names, and selects that row when it is not on screen', async () => {
      const { panels, ops, row, visibleSlug } = mountPanels(fakeSession(['@1']))
      await flushPromises()

      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      panels.openViewIn(row as any, 'diff')
      expect(ops.selectRow).not.toHaveBeenCalled()

      visibleSlug.value = 'other'
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      panels.openViewIn(row as any, 'tasks')
      expect(ops.selectRow).toHaveBeenCalledWith('hive-one')
      expect(panelsOf(usePanelLayouts().layoutOf(KEY))[0].tabs.map((tab) => tab.id)).toContain('tasks')
    })
  })

  // A pinned chat's row comes from the recents listing, so until that answers a
  // chat's layout looks like one for a session that no longer exists.
  it('keeps every layout until the recents have answered', async () => {
    usePanelLayouts().apply('agentws-7', (layout) => openTab(layout, viewTab('docs')))
    const recentsLoaded = ref(false)
    mountPanels(fakeSession(['@1']), recentsLoaded)
    await flushPromises()
    expect(panelsOf(usePanelLayouts().layoutOf('agentws-7'))[0].tabs.map((tab) => tab.id)).toEqual(['docs'])

    recentsLoaded.value = true
    await flushPromises()
    expect(panelsOf(usePanelLayouts().layoutOf('agentws-7'))[0].tabs).toEqual([])
  })

  describe('what ends a session', () => {
    const withTabs = async (session: FakeSession) => {
      const mounted = mountPanels(session)
      await flushPromises()
      usePanelLayouts().apply(KEY, (layout) => openTab(layout, viewTab('diff')))
      await flushPromises()
      return mounted
    }

    it.each(['stopped', 'terminated', 'completed'])('closes every tab when the session is %s', async (reason) => {
      const session = fakeSession(['@1'])
      await withTabs(session)

      session.tabs.value = []
      session.endReason.value = reason
      await flushPromises()

      expect(panelsOf(usePanelLayouts().layoutOf(KEY))[0].tabs).toEqual([])
    })

    it.each(['attach-failed', 'interrupted', 'disconnected'])(
      'keeps every tab after %s, when tmux may be fine',
      async (reason) => {
        const session = fakeSession(['@1'])
        await withTabs(session)

        session.tabs.value = []
        session.endReason.value = reason
        await flushPromises()

        expect(panelsOf(usePanelLayouts().layoutOf(KEY))[0].tabs.map((tab) => tab.id)).toEqual(['window:@1', 'diff'])
      },
    )

    it('keeps the views of a session that is only not running', async () => {
      const session = fakeSession(['@1'])
      await withTabs(session)

      session.tabs.value = []
      session.endReason.value = 'not-started'
      await flushPromises()

      expect(panelsOf(usePanelLayouts().layoutOf(KEY))[0].tabs.map((tab) => tab.id)).toEqual(['diff'])
    })

    it('lets go of a session the list stopped carrying', async () => {
      const session = fakeSession(['@1'])
      usePanelLayouts().apply('deleted-elsewhere', (layout) => openTab(layout, viewTab('diff')))
      await withTabs(session)

      expect(panelsOf(usePanelLayouts().layoutOf('deleted-elsewhere'))[0].tabs).toEqual([])
      expect(panelsOf(usePanelLayouts().layoutOf(KEY))[0].tabs).not.toEqual([])
    })
  })
})
