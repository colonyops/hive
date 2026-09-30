import { describe, expect, it } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import {
  applicationSettingsSections,
  createAppRouter,
  isApplicationSettingsSection,
  isProfileSettingsSection,
  profileSettingsSections,
} from '../router'

describe('createAppRouter', () => {
  it('registers the developer tools route in dev mode', () => {
    const router = createAppRouter(createMemoryHistory())

    expect(router.resolve('/dev').name).toBe('dev')
  })

  // A section that routes but does not resolve to itself lands the user on the
  // default pane with no error — the URL changes and nothing else does. The
  // route matcher and the section resolver are built from one list so that
  // cannot happen; this asserts it for every section, including new ones.
  // The Agents area is a third mode beside the hub and terminal, so its route
  // must resolve the same way — a bare /workspaces entering the area and a
  // named one opening a specific workspace.
  it('resolves the agents route with and without a workspace', () => {
    const router = createAppRouter(createMemoryHistory())

    expect(router.resolve('/workspaces').name).toBe('agents')
    const resolved = router.resolve('/workspaces/hive')
    expect(resolved.name).toBe('agents')
    expect(resolved.params.workspace).toBe('hive')
  })

  // Activity stopped being a route when it became an overlay (#441). A stale
  // #/activity — a hand-typed link, or a hash left in a window that was never
  // reloaded — has to land somewhere sensible rather than on a blank shell.
  it('degrades an old /activity link to the feed', async () => {
    const router = createAppRouter(createMemoryHistory())

    await router.push('/activity')
    expect(router.currentRoute.value.name).toBe('feed')
  })

  it('routes every application settings section to itself', () => {
    const router = createAppRouter(createMemoryHistory())

    for (const section of applicationSettingsSections) {
      const resolved = router.resolve(`/settings/${section}`)
      expect(resolved.name, `/settings/${section} is not routable`).toBe('application-settings')
      expect(resolved.params.section, `/settings/${section} resolved to another section`).toBe(section)
      expect(isApplicationSettingsSection(resolved.params.section)).toBe(true)
    }
  })

  it('routes every profile settings section to itself', () => {
    const router = createAppRouter(createMemoryHistory())

    for (const section of profileSettingsSections) {
      const resolved = router.resolve(`/profiles/p1/settings/${section}`)
      expect(resolved.name, `${section} is not routable`).toBe('profile-settings')
      expect(resolved.params.section).toBe(section)
      expect(isProfileSettingsSection(resolved.params.section)).toBe(true)
    }
  })

  it('sends an unknown settings section to the feed rather than a blank shell', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/settings/not-a-section')

    expect(router.currentRoute.value.name).toBe('feed')
  })

  it('rejects values outside the section lists', () => {
    expect(isApplicationSettingsSection('agents')).toBe(true)
    expect(isApplicationSettingsSection('nope')).toBe(false)
    expect(isApplicationSettingsSection(undefined)).toBe(false)
    expect(isApplicationSettingsSection(['agents'])).toBe(false)
    expect(isProfileSettingsSection('danger')).toBe(true)
    expect(isProfileSettingsSection('nope')).toBe(false)
  })
})
