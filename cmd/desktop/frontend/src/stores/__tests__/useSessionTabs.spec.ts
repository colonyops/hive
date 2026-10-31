import { nextTick } from 'vue'
import { beforeEach, describe, expect, it } from 'vitest'
import { chooseSplit, openView, resizeSplit, toggleSplit } from '../../lib/sessionLayout'
import { resetStores } from '../defineStore'
import { useSessionTabs } from '../useSessionTabs'

const STORAGE_KEY = 'hive.code.session-tabs'

describe('useSessionTabs', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('keeps each session to its own tabs and split', () => {
    const tabs = useSessionTabs()
    tabs.apply('a', toggleSplit)
    tabs.apply('b', (layout) => openView(layout, 'docs'))

    expect(tabs.layoutOf('a').panes).toHaveLength(2)
    expect(tabs.layoutOf('b').panes).toHaveLength(1)
    expect(tabs.layoutOf('b').panes[0].activeTab).toBe('docs')
  })

  it('restores a session after the app starts again', async () => {
    useSessionTabs().apply('a', (layout) => openView(layout, 'diff'))
    await nextTick()

    resetStores()

    expect(useSessionTabs().layoutOf('a').panes[0].activeTab).toBe('diff')
  })

  it('restores split direction and size per session', async () => {
    const tabs = useSessionTabs()
    tabs.apply('a', (layout) => resizeSplit(chooseSplit(layout, 'column', false), 0.3))
    await nextTick()

    resetStores()

    expect(useSessionTabs().layoutOf('a')).toMatchObject({ splitAxis: 'column', splitRatio: 0.3 })
    expect(useSessionTabs().layoutOf('b')).toMatchObject({ splitAxis: 'auto', splitRatio: 0.5 })
  })

  it('writes nothing when a rule changes nothing', async () => {
    useSessionTabs().apply('a', (layout) => layout)
    await nextTick()

    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toEqual({})
  })

  it('starts over from a stored layout that breaks the rules', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ a: { panes: [] } }))

    expect(useSessionTabs().layoutOf('a').panes).toHaveLength(1)
  })
})
