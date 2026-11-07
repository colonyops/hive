import { describe, expect, it } from 'vitest'
import { useAppTabs } from '../useAppTabs'

describe('useAppTabs', () => {
  it('adds a tab once and keeps the order it was added in', () => {
    const tabs = useAppTabs()
    tabs.add('drive')
    tabs.add('applet:clock')
    tabs.add('drive')
    expect(tabs.tabs.value).toEqual(['drive', 'applet:clock'])
  })

  it('persists the strip and drops what it cannot render', () => {
    localStorage.setItem('hive.titlebar.tabs', JSON.stringify(['tasks', 'inbox', 'bogus', 'tasks', 'applet:x']))
    expect(useAppTabs().tabs.value).toEqual(['tasks', 'applet:x'])
  })

  it('moves a tab before another, or to the end', () => {
    const tabs = useAppTabs()
    tabs.add('drive')
    tabs.add('tasks')
    tabs.add('settings')
    tabs.move('settings', 'drive')
    expect(tabs.tabs.value).toEqual(['settings', 'drive', 'tasks'])
    tabs.move('settings', null)
    expect(tabs.tabs.value).toEqual(['drive', 'tasks', 'settings'])
  })

  it('returns to the last tab visited that is still in the strip', () => {
    const tabs = useAppTabs()
    tabs.add('drive')
    tabs.add('tasks')
    tabs.visited('code')
    tabs.visited('drive')
    tabs.visited('tasks')
    tabs.remove('drive')
    expect(tabs.previous('tasks')).toBe('code')
    expect(tabs.previous('code')).toBe('tasks')
  })

  it('falls back to Inbox with nothing visited', () => {
    expect(useAppTabs().previous('tasks')).toBe('inbox')
  })
})
