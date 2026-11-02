import { describe, expect, it } from 'vitest'
import { appletIdOf, appletTab, appletTabsOf, isAppletTabId } from '../appletTabs'
import { moveTab, newPanelLayout, openTab, splitPanel } from '../panelLayout'

describe('appletTabs', () => {
  it('reads the applet of a tab, and nothing of another tab', () => {
    expect(appletIdOf(appletTab('radio'))).toBe('radio')
    expect(appletIdOf({ id: 'docs', kind: 'docs' })).toBeUndefined()
    expect(appletIdOf({ id: 'applet:x', kind: 'applet' })).toBeUndefined()
    expect(isAppletTabId('applet:x')).toBe(true)
    expect(isAppletTabId('applet:')).toBe(false)
    expect(isAppletTabId('drive')).toBe(false)
  })

  // Moving an iframe in the DOM reloads its page.
  it('keeps the order of its frames when a tab moves between panels', () => {
    const start = openTab(openTab(newPanelLayout(), appletTab('b')), appletTab('a'))
    const split = splitPanel(start, 'p0', 'row')
    const before = appletTabsOf(split).map(({ applet }) => applet)
    const moved = moveTab(split, 'applet:b', split.focusedPanel, 0)
    const after = appletTabsOf(moved)
    expect(after.map(({ applet }) => applet)).toEqual(before)
    expect(after.find(({ applet }) => applet === 'b')?.panel).toBe(split.focusedPanel)
  })
})
