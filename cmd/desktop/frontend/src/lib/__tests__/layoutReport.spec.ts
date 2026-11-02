import { describe, expect, it } from 'vitest'
import { toLayoutReport } from '../layoutReport'
import { newPanelLayout, openTab, splitPanel, type PanelLayout } from '../panelLayout'
import { windowTab } from '../windowTabs'

function layoutWithTwoPanels(): PanelLayout {
  const split = splitPanel(
    openTab(openTab(newPanelLayout(), windowTab('@3')), { id: 'docs', kind: 'docs' }),
    'p0',
    'column',
  )
  return { ...split, root: { ...split.root, ratio: 0.6 } as PanelLayout['root'] }
}

describe('toLayoutReport', () => {
  it('reports a panel with its tabs, its active tab and no split fields', () => {
    const report = toLayoutReport('code', 'sess-1', { 'sess-1': openTab(newPanelLayout(), windowTab('@3')) })

    expect(report).toEqual({
      kind: 'code',
      onScreen: 'sess-1',
      layouts: {
        'sess-1': {
          focusedPanel: 'p0',
          root: {
            kind: 'panel',
            id: 'p0',
            tabs: [{ id: 'window:@3', kind: 'window', windowId: '@3' }],
            activeTab: 'window:@3',
            picking: false,
            axis: '',
            ratio: 0,
            children: [],
          },
        },
      },
    })
  })

  it('reports a split with its axis, ratio and two children', () => {
    const { root } = toLayoutReport('chats', '', { a: layoutWithTwoPanels() }).layouts.a

    expect(root).toMatchObject({
      kind: 'split',
      axis: 'column',
      ratio: 0.6,
      tabs: [],
      activeTab: '',
      picking: false,
    })
    expect(root.children.map((child) => child.id)).toEqual(['p0', 'p1'])
    expect(root.children[0].tabs.map((tab) => tab.windowId)).toEqual(['@3', ''])
    expect(root.children[1]).toMatchObject({ kind: 'panel', picking: true, activeTab: '' })
  })

  it('reports no layouts as an empty record', () => {
    expect(toLayoutReport('code', '', {})).toEqual({ kind: 'code', onScreen: '', layouts: {} })
  })
})
