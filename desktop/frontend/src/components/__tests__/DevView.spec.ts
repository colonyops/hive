import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DevView from '../DevView.vue'

const mocks = vi.hoisted(() => ({
  notify: vi.fn(),
  showToast: vi.fn(),
  Notify: vi.fn(),
  notificationSound: { value: true },
}))

vi.mock('../../composables/useNotify', () => ({
  useNotify: () => ({ notify: mocks.notify }),
  notifySeverityMapping: {
    info: { activity: 'info', toast: 'info' },
    success: { activity: 'success', toast: 'success' },
    warning: { activity: 'warning', toast: 'info' },
    error: { activity: 'error', toast: 'error' },
  },
}))
vi.mock('../../composables/useToasts', () => ({
  useToasts: () => ({ showToast: mocks.showToast }),
}))
vi.mock('../../composables/useNotificationSettings', () => ({
  useNotificationSettings: () => ({ notificationSound: mocks.notificationSound }),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/desktop/notificationservice', () => ({
  Notify: mocks.Notify,
}))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.notify.mockResolvedValue(undefined)
  mocks.Notify.mockResolvedValue(undefined)
  mocks.notificationSound.value = true
})

afterEach(() => {
  vi.useRealTimers()
})

describe('DevView notification test controls', () => {
  it('sends auto notifications through useNotify so Activity and focus settings apply', async () => {
    const wrapper = mount(DevView)

    await wrapper.find('[data-testid="dev-notification-severity"]').setValue('success')
    await wrapper.find('[data-testid="dev-notification-send"]').trigger('click')

    expect(mocks.notify).toHaveBeenCalledWith({
      title: 'Test notification',
      body: 'Dev tools auto test: uses focus and notification settings.',
      severity: 'success',
      category: 'system',
      source: 'dev-view',
    })
    expect(mocks.showToast).not.toHaveBeenCalled()
    expect(mocks.Notify).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('forces a toast using the notification severity mapping without recording Activity', async () => {
    const wrapper = mount(DevView)

    await wrapper.find('[data-testid="dev-notification-severity"]').setValue('warning')
    await wrapper.find('[data-testid="dev-notification-channel"]').setValue('force-toast')
    await wrapper.find('[data-testid="dev-notification-send"]').trigger('click')

    expect(mocks.showToast).toHaveBeenCalledWith('Test notification', {
      body: 'Dev tools forced toast test: bypasses focus and Activity.',
      severity: 'info',
    })
    expect(mocks.notify).not.toHaveBeenCalled()
    expect(mocks.Notify).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('forces a native notification with cached sound without recording Activity', async () => {
    const wrapper = mount(DevView)
    mocks.notificationSound.value = false

    await wrapper.find('[data-testid="dev-notification-severity"]').setValue('error')
    await wrapper.find('[data-testid="dev-notification-channel"]').setValue('force-system')
    await wrapper.find('[data-testid="dev-notification-send"]').trigger('click')

    expect(mocks.Notify).toHaveBeenCalledWith({
      title: 'Test notification',
      subtitle: 'Hive Dev tools',
      body: 'Dev tools forced system test: bypasses focus and Activity.',
      sound: false,
      data: { source: 'dev-view', channel: 'force-system', severity: 'error' },
    })
    expect(mocks.notify).not.toHaveBeenCalled()
    expect(mocks.showToast).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('delays dispatch by three seconds and replaces a pending scheduled test', async () => {
    vi.useFakeTimers()
    const wrapper = mount(DevView)

    await wrapper.find('[data-testid="dev-notification-delay"]').setValue(true)
    await wrapper.find('[data-testid="dev-notification-send"]').trigger('click')
    await wrapper.find('[data-testid="dev-notification-severity"]').setValue('error')
    await wrapper.find('[data-testid="dev-notification-send"]').trigger('click')

    vi.advanceTimersByTime(2999)
    expect(mocks.notify).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)
    expect(mocks.notify).toHaveBeenCalledOnce()
    expect(mocks.notify).toHaveBeenCalledWith(expect.objectContaining({ severity: 'error' }))

    wrapper.unmount()
  })

  it('cleans up a pending notification test when closed', async () => {
    vi.useFakeTimers()
    const wrapper = mount(DevView)

    await wrapper.find('[data-testid="dev-notification-delay"]').setValue(true)
    await wrapper.find('[data-testid="dev-notification-send"]').trigger('click')
    wrapper.unmount()
    vi.advanceTimersByTime(3000)

    expect(mocks.notify).not.toHaveBeenCalled()
  })

  it('logs notification delivery failures', async () => {
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {})
    mocks.notify.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mount(DevView)

    await wrapper.find('[data-testid="dev-notification-send"]').trigger('click')
    await Promise.resolve()

    expect(warning).toHaveBeenCalledWith('[dev-view] auto notification test failed', expect.any(Error))
    warning.mockRestore()
    wrapper.unmount()
  })
})
