import { nextTick } from 'vue'
import { beforeEach, describe, expect, it } from 'vitest'
import { openTab, panelBoxes, panelsOf, resizeSplit, splitBoxes, splitPanel } from '../../lib/panelLayout'
import { resetStores } from '../defineStore'
import { usePanelLayouts } from '../usePanelLayouts'

const STORAGE_KEY = 'hive.code.panel-layouts'
const docs = { id: 'docs', kind: 'docs' }
const diff = { id: 'diff', kind: 'diff' }

describe('usePanelLayouts', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('keeps each key to its own tabs and splits', () => {
    const layouts = usePanelLayouts()
    layouts.apply('a', (layout) => splitPanel(layout, 'p0', 'row'))
    layouts.apply('b', (layout) => openTab(layout, docs))

    expect(panelsOf(layouts.layoutOf('a'))).toHaveLength(2)
    expect(panelsOf(layouts.layoutOf('b'))).toHaveLength(1)
    expect(panelsOf(layouts.layoutOf('b'))[0].activeTab).toBe('docs')
  })

  it('brings tabs, nested splits and sizes back after the app restarts', async () => {
    const layouts = usePanelLayouts()
    layouts.apply('a', (layout) => {
      const one = splitPanel(openTab(layout, diff), 'p0', 'row')
      const two = splitPanel(one, panelsOf(one)[1].id, 'column')
      return resizeSplit(two, splitBoxes(two)[0].split.id, 0.3)
    })
    const before = layouts.layoutOf('a')
    await nextTick()

    resetStores()

    const after = usePanelLayouts().layoutOf('a')
    expect(after).toEqual(before)
    expect([...panelBoxes(after).values()][0].width).toBe(0.3)
  })

  it('forgets a key completely', async () => {
    const layouts = usePanelLayouts()
    layouts.apply('a', (layout) => openTab(layout, diff))
    layouts.forget('a')
    await nextTick()

    expect(panelsOf(layouts.layoutOf('a'))[0].tabs).toEqual([])
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toEqual({})
  })

  it('forgets every key the rule says is gone, and keeps the rest', () => {
    const layouts = usePanelLayouts()
    layouts.apply('a', (layout) => openTab(layout, diff))
    layouts.apply('b', (layout) => openTab(layout, docs))
    layouts.retain((key) => key === 'b')

    expect(panelsOf(layouts.layoutOf('a'))[0].tabs).toEqual([])
    expect(panelsOf(layouts.layoutOf('b'))[0].tabs).toHaveLength(1)
  })

  it('writes nothing when a rule changes nothing', async () => {
    usePanelLayouts().apply('a', (layout) => layout)
    await nextTick()

    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toEqual({})
  })

  it('starts over from a stored layout that breaks the rules', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ a: { panels: [] } }))

    expect(panelsOf(usePanelLayouts().layoutOf('a'))).toHaveLength(1)
  })
})
