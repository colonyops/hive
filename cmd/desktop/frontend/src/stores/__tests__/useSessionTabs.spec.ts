import { nextTick } from 'vue'
import { beforeEach, describe, expect, it } from 'vitest'
import { openView, paneBoxes, panesOf, resizeSplit, splitBoxes, splitPane } from '../../lib/sessionLayout'
import { resetStores } from '../defineStore'
import { useSessionTabs } from '../useSessionTabs'

const STORAGE_KEY = 'hive.code.session-tabs'

describe('useSessionTabs', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('keeps each session to its own tabs and splits', () => {
    const tabs = useSessionTabs()
    tabs.apply('a', (layout) => splitPane(layout, 'p0', 'row'))
    tabs.apply('b', (layout) => openView(layout, 'docs'))

    expect(panesOf(tabs.layoutOf('a'))).toHaveLength(2)
    expect(panesOf(tabs.layoutOf('b'))).toHaveLength(1)
    expect(panesOf(tabs.layoutOf('b'))[0].activeTab).toBe('docs')
  })

  it('brings tabs, nested splits and sizes back after the app restarts', async () => {
    const tabs = useSessionTabs()
    tabs.apply('a', (layout) => {
      const one = splitPane(openView(layout, 'diff'), 'p0', 'row')
      const two = splitPane(one, panesOf(one)[1].id, 'column')
      return resizeSplit(two, splitBoxes(two)[0].split.id, 0.3)
    })
    const before = tabs.layoutOf('a')
    await nextTick()

    resetStores()

    const after = useSessionTabs().layoutOf('a')
    expect(after).toEqual(before)
    expect([...paneBoxes(after).values()][0].width).toBe(0.3)
  })

  it('forgets a session completely', async () => {
    const tabs = useSessionTabs()
    tabs.apply('a', (layout) => openView(layout, 'diff'))
    tabs.forget('a')
    await nextTick()

    expect(panesOf(tabs.layoutOf('a'))[0].tabs).toEqual([])
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toEqual({})
  })

  it('forgets every session the rule says is gone, and keeps the rest', () => {
    const tabs = useSessionTabs()
    tabs.apply('a', (layout) => openView(layout, 'diff'))
    tabs.apply('b', (layout) => openView(layout, 'docs'))
    tabs.retain((key) => key === 'b')

    expect(panesOf(tabs.layoutOf('a'))[0].tabs).toEqual([])
    expect(panesOf(tabs.layoutOf('b'))[0].tabs).toHaveLength(1)
  })

  it('writes nothing when a rule changes nothing', async () => {
    useSessionTabs().apply('a', (layout) => layout)
    await nextTick()

    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toEqual({})
  })

  it('starts over from a stored layout that breaks the rules', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ a: { panes: [] } }))

    expect(panesOf(useSessionTabs().layoutOf('a'))).toHaveLength(1)
  })
})
