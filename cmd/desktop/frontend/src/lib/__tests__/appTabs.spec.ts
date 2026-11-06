import { describe, expect, it } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'
import { globalTabLocation, isGlobalTabId, tabOfRoute, VIEW_TABS } from '../appTabs'

describe('tabOfRoute', () => {
  it.each([
    ['/feed', 'inbox'],
    ['/flows/p1', 'inbox'],
    ['/profiles/p1/settings', 'inbox'],
    ['/terminal/s1', 'code'],
    ['/workspaces/hive', 'chats'],
    ['/drive', 'drive'],
    ['/tasks', 'tasks'],
    ['/settings/applets', 'settings'],
    ['/applets/pomodoro', 'applet:pomodoro'],
  ])('puts %s under the %s tab', (path, tab) => {
    const route = createAppRouter(createMemoryHistory()).resolve(path)
    expect(tabOfRoute(route.name, route.params)).toBe(tab)
  })

  // Each default location has to come back to the tab it opens, or adding a tab would
  // land the user on a page the strip does not select.
  it('opens every global tab on a route that selects it', () => {
    const router = createAppRouter(createMemoryHistory())
    for (const id of [...VIEW_TABS, 'applet:clock' as const]) {
      const route = router.resolve(globalTabLocation(id))
      expect(tabOfRoute(route.name, route.params)).toBe(id)
    }
  })
})

describe('isGlobalTabId', () => {
  it('accepts the built-in pages and applets, and nothing pinned or empty', () => {
    expect(isGlobalTabId('drive')).toBe(true)
    expect(isGlobalTabId('applet:clock')).toBe(true)
    expect(isGlobalTabId('applet:')).toBe(false)
    expect(isGlobalTabId('inbox')).toBe(false)
    expect(isGlobalTabId(3)).toBe(false)
  })
})
