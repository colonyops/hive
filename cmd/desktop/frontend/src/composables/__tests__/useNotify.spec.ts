import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { notifySeverityMapping, useNotify, type NotifyDeps, type NotifySeverity } from '../useNotify'

function makeSettings(overrides: Partial<NotifyDeps['settings']> = {}): NotifyDeps['settings'] {
  return {
    notificationsEnabled: ref(true),
    delivery: ref('auto'),
    notificationSound: ref(true),
    permission: ref('granted'),
    ...overrides,
  }
}

function makeDeps(overrides: Partial<NotifyDeps> = {}) {
  const deps: NotifyDeps = {
    record: vi.fn().mockResolvedValue(true),
    showToast: vi.fn(),
    focused: ref(true),
    settings: makeSettings(),
    osNotify: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
  return deps
}

describe('useNotify', () => {
  it.each([
    ['info', 'info', 'info'],
    ['success', 'success', 'success'],
    ['warning', 'warning', 'warning'],
    ['error', 'error', 'error'],
  ] as Array<[NotifySeverity, string, string]>)('maps %s to activity %s and toast %s', async (severity, activity, toast) => {
    const deps = makeDeps()
    await useNotify(deps).notify({ title: 'Title', body: 'Body', severity })
    expect(notifySeverityMapping[severity]).toEqual({ activity, toast })
    expect(deps.record).toHaveBeenCalledWith({ title: 'Title', body: 'Body', severity: activity, category: 'system', source: '', metadata: null })
    expect(deps.showToast).toHaveBeenCalledWith('Title', { body: 'Body', severity: toast })
    expect(deps.osNotify).not.toHaveBeenCalled()
  })

  it('uses a focused toast without OS delivery or sound', async () => {
    const deps = makeDeps({ settings: makeSettings({ notificationSound: ref(false) }) })
    await useNotify(deps).notify({ title: 'Focused' })
    expect(deps.showToast).toHaveBeenCalledWith('Focused', { body: '', severity: 'info' })
    expect(deps.osNotify).not.toHaveBeenCalled()
  })

  it('delivers unfocused granted notifications with the configured sound', async () => {
    const deps = makeDeps({ focused: ref(false), settings: makeSettings({ notificationSound: ref(false) }) })
    await useNotify(deps).notify({ title: 'Background', body: 'Body' })
    expect(deps.showToast).not.toHaveBeenCalled()
    expect(deps.osNotify).toHaveBeenCalledWith({ title: 'Background', subtitle: '', body: 'Body', severity: 'info', sound: false, data: {} })
  })

  it('records only when master notifications are disabled', async () => {
    const deps = makeDeps({ settings: makeSettings({ notificationsEnabled: ref(false) }) })
    await useNotify(deps).notify({ title: 'Silent' })
    expect(deps.record).toHaveBeenCalledOnce()
    expect(deps.showToast).not.toHaveBeenCalled()
    expect(deps.osNotify).not.toHaveBeenCalled()
  })

  it('keeps notifications in the app on the "app" delivery mode, focused or not', async () => {
    for (const focused of [true, false]) {
      const deps = makeDeps({ focused: ref(focused), settings: makeSettings({ delivery: ref('app') }) })
      await useNotify(deps).notify({ title: 'In app only' })
      expect(deps.showToast).toHaveBeenCalledWith('In app only', { body: '', severity: 'info' })
      expect(deps.osNotify).not.toHaveBeenCalled()
    }
  })

  it('raises an OS banner on the "system" delivery mode even while focused', async () => {
    const deps = makeDeps({ focused: ref(true), settings: makeSettings({ delivery: ref('system') }) })
    await useNotify(deps).notify({ title: 'Always a banner' })
    expect(deps.showToast).not.toHaveBeenCalled()
    expect(deps.osNotify).toHaveBeenCalledWith({ title: 'Always a banner', subtitle: '', body: '', severity: 'info', sound: true, data: {} })
  })

  it('carries click-routing data and lets an event silence itself', async () => {
    const deps = makeDeps({ focused: ref(false) })
    await useNotify(deps).notify({ title: 'Review requested', data: { profileId: 'work', itemId: 42 }, silent: true })
    expect(deps.osNotify).toHaveBeenCalledWith({
      title: 'Review requested', subtitle: '', body: '', severity: 'info',
      sound: false, data: { profileId: 'work', itemId: 42 },
    })
  })

  it('falls back to a toast when notification permission is denied', async () => {
    const deps = makeDeps({ focused: ref(false), settings: makeSettings({ permission: ref('denied') }) })
    await useNotify(deps).notify({ title: 'Denied' })
    expect(deps.showToast).toHaveBeenCalledWith('Denied', { body: '', severity: 'info' })
    expect(deps.osNotify).not.toHaveBeenCalled()
  })

  it('falls back to a toast without requesting when permission was never requested', async () => {
    const deps = makeDeps({ focused: ref(false), settings: makeSettings({ permission: ref('not-requested') }) })
    await useNotify(deps).notify({ title: 'Ask' })
    expect(deps.showToast).toHaveBeenCalledWith('Ask', { body: '', severity: 'info' })
    expect(deps.osNotify).not.toHaveBeenCalled()
  })

  it('warns but still surfaces transient feedback when durable recording fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const deps = makeDeps({ record: vi.fn().mockResolvedValue(false) })
    const event = { title: 'Record failed' }
    await useNotify(deps).notify(event)
    expect(warn).toHaveBeenCalledWith('[notify] activity record failed; surfacing anyway', event)
    expect(deps.showToast).toHaveBeenCalledOnce()
    warn.mockRestore()
  })

  it('falls back to a toast when native delivery rejects', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const deps = makeDeps({ focused: ref(false), osNotify: vi.fn().mockRejectedValue(new Error('no daemon')) })
    await useNotify(deps).notify({ title: 'Native failure' })
    expect(deps.showToast).toHaveBeenCalledWith('Native failure', { body: '', severity: 'info' })
    expect(warn).toHaveBeenCalledWith('[notify] native notification failed; surfacing toast instead', expect.any(Error))
    warn.mockRestore()
  })
})
