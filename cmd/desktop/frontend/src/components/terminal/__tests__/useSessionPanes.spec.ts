import { computed, effectScope, nextTick, ref, shallowRef, type Ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { focusedWindow, openView, panesOf, placeWindow, selectTab, splitPane } from '../../../lib/sessionLayout'
import { paneMayAutoFocus } from '../../../lib/terminalTree'
import { useSessionTabs } from '../../../stores/useSessionTabs'
import { useSessionPanes } from '../useSessionPanes'

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

function mountPanes(session: FakeSession, recentsLoaded = ref(true)) {
  const visible = shallowRef<FakeSession | null>(session)
  const row = { id: KEY, slug: 'hive-one' }
  const rows: Ref<(typeof row)[]> = ref([row])
  const ops = { newWindowIn: vi.fn().mockResolvedValue(undefined), newAgentWindowIn: vi.fn(), selectRow: vi.fn() }
  const tree = {
    attachable: computed(() => rows.value),
    sessionsLoaded: ref(true),
    serversLoaded: ref(true),
    recentsLoaded,
    sessionsError: ref(null),
    serversError: ref(''),
    offlineServers: ref([]),
    isChat: () => false,
    isHiveSession: () => true,
    isServerSession: () => false,
  }
  const pool = { visible, visibleSlug: ref('hive-one') }
  const scope = effectScope()
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the composable reads only these parts
  const panes = scope.run(() => useSessionPanes({ pool, tree, ops } as any))!
  return { panes, ops, rows, scope, row }
}

describe('useSessionPanes', () => {
  beforeEach(() => {
    localStorage.clear()
    paneMayAutoFocus.value = true
  })

  describe('a window activated by an agent or another client', () => {
    it('does not take the keyboard from the terminal in front of the user', async () => {
      const session = fakeSession(['@1', '@2'])
      mountPanes(session)
      await flushPromises()
      session.focusActive.mockClear()

      session.activeWindowId.value = '@2'
      await flushPromises()

      expect(focusedWindow(useSessionTabs().layoutOf(KEY))).toBe('@2')
      expect(session.focusActive).not.toHaveBeenCalled()
    })

    it('does not replace a view the user is reading', async () => {
      const session = fakeSession(['@1', '@2'])
      mountPanes(session)
      await flushPromises()
      useSessionTabs().apply(KEY, (layout) => openView(layout, 'diff'))
      await flushPromises()

      session.activeWindowId.value = '@2'
      await flushPromises()

      expect(panesOf(useSessionTabs().layoutOf(KEY))[0].activeTab).toBe('diff')
    })
  })

  describe('a window the user asked for', () => {
    it('comes forward even from behind a view, in the pane that asked', async () => {
      const session = fakeSession(['@1'])
      const { panes, row } = mountPanes(session)
      await flushPromises()
      const tabs = useSessionTabs()
      tabs.apply(KEY, (layout) => splitPane(openView(layout, 'diff'), 'p0', 'row'))
      const target = panesOf(tabs.layoutOf(KEY))[1].id

      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      await panes.newWindow(row as any, target)
      session.tabs.value = [...session.tabs.value, { windowId: '@2', name: '@2' }]
      session.activeWindowId.value = '@2'
      await flushPromises()

      expect(panesOf(tabs.layoutOf(KEY))[1].activeTab).toBe('window:@2')
      expect(focusedWindow(tabs.layoutOf(KEY))).toBe('@2')
    })

    it('moves the tmux window to match when the new tab lands mid-order', async () => {
      const session = fakeSession(['@1', '@2'])
      const { panes, row } = mountPanes(session)
      await flushPromises()
      const tabs = useSessionTabs()
      tabs.apply(KEY, (layout) => {
        const split = splitPane(layout, 'p0', 'row')
        return placeWindow(split, '@2', panesOf(split)[1].id)
      })
      await flushPromises()

      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the fake row stands in for a full one
      await panes.newWindow(row as any, 'p0')
      session.tabs.value = [...session.tabs.value, { windowId: '@3', name: '@3' }]
      await flushPromises()

      expect(session.moveWindow).toHaveBeenCalledWith('@3', 1)
    })
  })

  describe('layout to tmux', () => {
    it('selects the window a click focused', async () => {
      const session = fakeSession(['@1', '@2'])
      mountPanes(session)
      await flushPromises()

      useSessionTabs().apply(KEY, (layout) => selectTab(layout, 'p0', 'window:@2'))
      await flushPromises()

      expect(session.select).toHaveBeenCalledWith('@2')
    })

    it('puts the keyboard back when a click returns to the active window from a view', async () => {
      const session = fakeSession(['@1'])
      mountPanes(session)
      await flushPromises()
      const tabs = useSessionTabs()
      tabs.apply(KEY, (layout) => openView(layout, 'docs'))
      await flushPromises()
      session.focusActive.mockClear()

      tabs.apply(KEY, (layout) => selectTab(layout, 'p0', 'window:@1'))
      await flushPromises()
      await nextTick()

      expect(session.focusActive).toHaveBeenCalled()
    })

    it('tells tmux which window a restored layout shows, once a cold attach lands', async () => {
      const tabs = useSessionTabs()
      tabs.apply(KEY, (layout) => {
        const withTabs = { ...layout }
        return {
          ...withTabs,
          root: {
            ...panesOf(layout)[0],
            tabs: [
              { id: 'window:@2', kind: 'window', windowId: '@2' },
              { id: 'window:@1', kind: 'window', windowId: '@1' },
            ],
            activeTab: 'window:@2',
          },
        }
      })
      const session = fakeSession([])
      mountPanes(session)
      await flushPromises()

      session.tabs.value = [
        { windowId: '@1', name: '@1' },
        { windowId: '@2', name: '@2' },
      ]
      session.activeWindowId.value = '@1'
      await flushPromises()

      expect(session.select).toHaveBeenCalledWith('@2')
    })

    it('asks for the renderer of every window on screen, not only the active one', async () => {
      const session = fakeSession(['@1', '@2'])
      mountPanes(session)
      await flushPromises()
      useSessionTabs().apply(KEY, (layout) => {
        const split = splitPane(layout, 'p0', 'row')
        return placeWindow(split, '@2', panesOf(split)[1].id)
      })
      await flushPromises()

      expect(session.revealWindow).toHaveBeenCalledWith('@1')
      expect(session.revealWindow).toHaveBeenCalledWith('@2')
    })
  })

  // A pinned chat's row comes from the recents listing, so until that answers a
  // chat's layout looks like one for a session that no longer exists.
  it('keeps every layout until the recents have answered', async () => {
    useSessionTabs().apply('agentws-7', (layout) => openView(layout, 'docs'))
    const recentsLoaded = ref(false)
    mountPanes(fakeSession(['@1']), recentsLoaded)
    await flushPromises()
    expect(panesOf(useSessionTabs().layoutOf('agentws-7'))[0].tabs.map((tab) => tab.id)).toEqual(['docs'])

    recentsLoaded.value = true
    await flushPromises()
    expect(panesOf(useSessionTabs().layoutOf('agentws-7'))[0].tabs).toEqual([])
  })

  describe('what ends a session', () => {
    const withTabs = async (session: FakeSession) => {
      const mounted = mountPanes(session)
      await flushPromises()
      useSessionTabs().apply(KEY, (layout) => openView(layout, 'diff'))
      await flushPromises()
      return mounted
    }

    it.each(['stopped', 'terminated', 'completed'])('closes every tab when the session is %s', async (reason) => {
      const session = fakeSession(['@1'])
      await withTabs(session)

      session.tabs.value = []
      session.endReason.value = reason
      await flushPromises()

      expect(panesOf(useSessionTabs().layoutOf(KEY))[0].tabs).toEqual([])
    })

    it.each(['attach-failed', 'interrupted', 'disconnected'])(
      'keeps every tab after %s, when tmux may be fine',
      async (reason) => {
        const session = fakeSession(['@1'])
        await withTabs(session)

        session.tabs.value = []
        session.endReason.value = reason
        await flushPromises()

        expect(panesOf(useSessionTabs().layoutOf(KEY))[0].tabs.map((tab) => tab.id)).toEqual(['window:@1', 'diff'])
      },
    )

    it('keeps the views of a session that is only not running', async () => {
      const session = fakeSession(['@1'])
      await withTabs(session)

      session.tabs.value = []
      session.endReason.value = 'not-started'
      await flushPromises()

      expect(panesOf(useSessionTabs().layoutOf(KEY))[0].tabs.map((tab) => tab.id)).toEqual(['diff'])
    })

    it('lets go of a session the list stopped carrying', async () => {
      const session = fakeSession(['@1'])
      useSessionTabs().apply('deleted-elsewhere', (layout) => openView(layout, 'diff'))
      await withTabs(session)

      expect(panesOf(useSessionTabs().layoutOf('deleted-elsewhere'))[0].tabs).toEqual([])
      expect(panesOf(useSessionTabs().layoutOf(KEY))[0].tabs).not.toEqual([])
    })
  })
})
