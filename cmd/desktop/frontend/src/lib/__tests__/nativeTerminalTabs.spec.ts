import { describe, expect, it } from 'vitest'
import { nativeAddressOf, nativeTerminalTab, nativeTerminalTabsOf } from '../nativeTerminalTabs'
import { newPanelLayout, openTab } from '../panelLayout'

describe('nativeTerminalTabs', () => {
  it('names a tab by its terminal address, so a session can hold several', () => {
    const a = nativeTerminalTab('t3')
    const b = nativeTerminalTab('box/t4')
    expect(a.id).not.toBe(b.id)
    expect(nativeAddressOf(a)).toBe('t3')
    expect(nativeAddressOf(b)).toBe('box/t4')
    expect(nativeAddressOf({ id: 'docs', kind: 'docs' })).toBeUndefined()
  })

  it('lists every native tab with the panel it sits in', () => {
    let layout = openTab(newPanelLayout(), nativeTerminalTab('t2'))
    layout = openTab(layout, { id: 'docs', kind: 'docs' })
    layout = openTab(layout, nativeTerminalTab('t1'))
    expect(nativeTerminalTabsOf(layout).map(({ address }) => address)).toEqual(['t1', 't2'])
    expect(new Set(nativeTerminalTabsOf(layout).map(({ panel }) => panel)).size).toBe(1)
  })
})
