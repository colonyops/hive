import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import DevView from '../DevView.vue'
import { chooseOption, selectedLabel } from '../../test-utils/select'

// Only the leaf seams are mocked: useNotify itself runs for real, so the
// reported outcome of an "auto" send is the one its own delivery decision
// produced rather than one this spec asserts into existence.
const mocks = vi.hoisted(() => ({
  record: vi.fn(),
  showToast: vi.fn(),
  Notify: vi.fn(),
  focused: { value: true },
  notificationsEnabled: { value: true },
  delivery: { value: 'auto' },
  notificationSound: { value: true },
  permission: { value: 'granted' },
  Ping: vi.fn(),
  Echo: vi.fn(),
}))

vi.mock('../../stores/useActivity', () => ({
  useActivity: () => ({ record: mocks.record }),
}))
vi.mock('../../composables/useToasts', () => ({
  useToasts: () => ({ showToast: mocks.showToast }),
}))
vi.mock('../../stores/useWindowFocus', () => ({
  useWindowFocus: () => ({ focused: mocks.focused }),
}))
vi.mock('../../composables/useNotificationSettings', () => ({
  useNotificationSettings: () => ({
    notificationsEnabled: mocks.notificationsEnabled,
    delivery: mocks.delivery,
    notificationSound: mocks.notificationSound,
    permission: mocks.permission,
  }),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/notificationservice', () => ({
  Notify: mocks.Notify,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/devtoolsservice', () => ({
  Ping: mocks.Ping,
  Echo: mocks.Echo,
}))
const autoBody = 'Dev tools auto test: uses focus and notification settings.'

function deferred<T = void>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

async function chooseChannel(wrapper: VueWrapper<any>, channel: string): Promise<void> {
  await wrapper.get(`[data-testid="dev-notification-channel-${channel}"]`).setValue()
}

async function send(wrapper: VueWrapper<any>): Promise<void> {
  await wrapper.get('[data-testid="dev-notification-send"]').trigger('click')
  await vi.advanceTimersByTimeAsync(0)
}

function result(wrapper: VueWrapper<any>): string {
  return wrapper.get('[data-testid="dev-notification-result"]').text()
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  mocks.record.mockResolvedValue(true)
  mocks.showToast.mockReturnValue(7)
  mocks.Notify.mockResolvedValue(undefined)
  mocks.focused.value = true
  mocks.notificationsEnabled.value = true
  mocks.delivery.value = 'auto'
  mocks.notificationSound.value = true
  mocks.permission.value = 'granted'
  mocks.Ping.mockResolvedValue(1_770_000_000_000)
  mocks.Echo.mockResolvedValue('x')
})

afterEach(() => {
  vi.useRealTimers()
})

describe('DevView notification test card', () => {
  it('sends auto notifications through useNotify and reports the toast it surfaced as', async () => {
    const wrapper = mount(DevView)

    await chooseOption(wrapper, 'dev-notification-severity', 'success')
    await send(wrapper)

    expect(mocks.record).toHaveBeenCalledWith({
      title: 'Test notification',
      body: autoBody,
      severity: 'success',
      category: 'system',
      source: 'dev-view',
      metadata: null,
    })
    expect(mocks.showToast).toHaveBeenCalledWith('Test notification', { body: autoBody, severity: 'success' })
    expect(mocks.Notify).not.toHaveBeenCalled()
    expect(result(wrapper)).toBe('Recorded in Activity and shown as an in-app toast.')

    wrapper.unmount()
  })

  it('reports an auto notification that left the window as a system banner', async () => {
    mocks.focused.value = false
    const wrapper = mount(DevView)

    await send(wrapper)

    expect(mocks.Notify).toHaveBeenCalledWith({
      title: 'Test notification',
      subtitle: '',
      body: autoBody,
      severity: 'info',
      sound: true,
      data: {},
    })
    expect(mocks.showToast).not.toHaveBeenCalled()
    expect(result(wrapper)).toBe('Recorded in Activity and shown as a system banner.')

    wrapper.unmount()
  })

  it('reports an auto notification suppressed to Activity when notifications are off', async () => {
    mocks.notificationsEnabled.value = false
    const wrapper = mount(DevView)

    await send(wrapper)

    expect(mocks.record).toHaveBeenCalledOnce()
    expect(mocks.showToast).not.toHaveBeenCalled()
    expect(mocks.Notify).not.toHaveBeenCalled()
    expect(result(wrapper)).toBe('Recorded in Activity only — notifications are switched off, so nothing surfaced.')

    wrapper.unmount()
  })

  it('reports the banner failure behind an auto notification that fell back to a toast', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    mocks.focused.value = false
    mocks.Notify.mockRejectedValueOnce(new Error('notification permission not granted'))
    const wrapper = mount(DevView)

    await send(wrapper)

    expect(mocks.showToast).toHaveBeenCalledOnce()
    expect(result(wrapper)).toBe(
      'Recorded in Activity. The system banner failed (notification permission not granted), so it fell back to an in-app toast.',
    )

    wrapper.unmount()
  })

  it('forces a toast using the severity mapping and names the toast it raised', async () => {
    mocks.showToast.mockReturnValue(12)
    const wrapper = mount(DevView)

    await chooseOption(wrapper, 'dev-notification-severity', 'warning')
    await chooseChannel(wrapper, 'force-toast')
    await send(wrapper)

    expect(mocks.showToast).toHaveBeenCalledWith('Test notification', {
      body: 'Dev tools forced toast test: bypasses focus and Activity.',
      severity: 'warning',
    })
    expect(mocks.record).not.toHaveBeenCalled()
    expect(mocks.Notify).not.toHaveBeenCalled()
    expect(result(wrapper)).toBe('Toast #12 shown in-app. Nothing was recorded in Activity.')

    wrapper.unmount()
  })

  it('forces a native notification with the cached sound and reports the OS accepting it', async () => {
    mocks.notificationSound.value = false
    const wrapper = mount(DevView)

    await chooseOption(wrapper, 'dev-notification-severity', 'error')
    await chooseChannel(wrapper, 'force-system')
    await send(wrapper)

    expect(mocks.Notify).toHaveBeenCalledWith({
      title: 'Test notification',
      subtitle: 'Hive Dev tools',
      body: 'Dev tools forced system test: bypasses focus and Activity.',
      severity: 'error',
      sound: false,
      data: { source: 'dev-view', channel: 'force-system', severity: 'error' },
    })
    expect(mocks.record).not.toHaveBeenCalled()
    expect(mocks.showToast).not.toHaveBeenCalled()
    expect(result(wrapper)).toBe('The OS accepted a system banner. Nothing was recorded in Activity.')

    wrapper.unmount()
  })

  it('surfaces a forced system failure inline instead of only in the console', async () => {
    mocks.Notify.mockRejectedValueOnce(new Error('native notifications unavailable'))
    const wrapper = mount(DevView)

    await chooseChannel(wrapper, 'force-system')
    await send(wrapper)

    const line = wrapper.get('[data-testid="dev-notification-result"]')
    expect(line.text()).toBe('System banner failed: native notifications unavailable')
    expect(line.classes()).toContain('text-severity-error')

    wrapper.unmount()
  })

  it('counts a delayed test down and dispatches when the countdown reaches zero', async () => {
    const wrapper = mount(DevView)

    await wrapper.get('[data-testid="dev-notification-delay"]').setValue(true)
    await wrapper.get('[data-testid="dev-notification-send"]').trigger('click')

    expect(wrapper.get('[data-testid="dev-notification-pending"]').text()).toContain('Sending in 3s…')
    expect(wrapper.find('[data-testid="dev-notification-result"]').exists()).toBe(false)

    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.get('[data-testid="dev-notification-pending"]').text()).toContain('Sending in 2s…')

    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.get('[data-testid="dev-notification-pending"]').text()).toContain('Sending in 1s…')
    expect(mocks.record).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1000)
    expect(mocks.record).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="dev-notification-pending"]').exists()).toBe(false)
    expect(result(wrapper)).toBe('Recorded in Activity and shown as an in-app toast.')

    wrapper.unmount()
  })

  it('cancels a queued test without dispatching it', async () => {
    const wrapper = mount(DevView)

    await wrapper.get('[data-testid="dev-notification-delay"]').setValue(true)
    await wrapper.get('[data-testid="dev-notification-send"]').trigger('click')
    await wrapper.get('[data-testid="dev-notification-cancel"]').trigger('click')

    expect(result(wrapper)).toBe('Scheduled test cancelled.')

    await vi.advanceTimersByTimeAsync(5000)
    expect(mocks.record).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('restarts the countdown when a second test is queued over a pending one', async () => {
    const wrapper = mount(DevView)

    await wrapper.get('[data-testid="dev-notification-delay"]').setValue(true)
    await wrapper.get('[data-testid="dev-notification-send"]').trigger('click')
    await vi.advanceTimersByTimeAsync(2000)

    await chooseOption(wrapper, 'dev-notification-severity', 'error')
    await wrapper.get('[data-testid="dev-notification-send"]').trigger('click')
    expect(wrapper.get('[data-testid="dev-notification-pending"]').text()).toContain('Sending in 3s…')

    await vi.advanceTimersByTimeAsync(2999)
    expect(mocks.record).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.record).toHaveBeenCalledOnce()
    expect(mocks.record).toHaveBeenCalledWith(expect.objectContaining({ severity: 'error' }))

    wrapper.unmount()
  })

  it('keeps the newest result when a slower send resolves after a later one', async () => {
    const slow = deferred()
    const quick = deferred()
    mocks.Notify.mockReturnValueOnce(slow.promise).mockReturnValueOnce(quick.promise)
    const wrapper = mount(DevView)

    await chooseChannel(wrapper, 'force-system')
    await send(wrapper)
    await send(wrapper)

    quick.resolve()
    await vi.advanceTimersByTimeAsync(0)
    expect(result(wrapper)).toBe('The OS accepted a system banner. Nothing was recorded in Activity.')

    slow.reject(new Error('stale'))
    await vi.advanceTimersByTimeAsync(0)
    expect(result(wrapper)).toBe('The OS accepted a system banner. Nothing was recorded in Activity.')

    wrapper.unmount()
  })

  it('cleans up a pending notification test when closed', async () => {
    const wrapper = mount(DevView)

    await wrapper.get('[data-testid="dev-notification-delay"]').setValue(true)
    await wrapper.get('[data-testid="dev-notification-send"]').trigger('click')
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(3000)

    expect(mocks.record).not.toHaveBeenCalled()
    expect(mocks.showToast).not.toHaveBeenCalled()
    expect(mocks.Notify).not.toHaveBeenCalled()
  })

  it('labels every control and explains each channel in place', async () => {
    const wrapper = mount(DevView)

    for (const testid of [
      'dev-notification-test',
      'dev-notification-severity',
      'dev-notification-channel',
      'dev-notification-delay',
      'dev-notification-send',
    ]) {
      expect(wrapper.find(`[data-testid="${testid}"]`).exists()).toBe(true)
    }

    expect(selectedLabel(wrapper, 'dev-notification-severity')).toBe('Info')

    const channels = wrapper.get('[data-testid="dev-notification-channel"]')
    expect(channels.attributes('role')).toBe('radiogroup')
    expect(channels.text()).toContain('Follow app settings')
    expect(channels.text()).toContain(
      'Records in Activity, then follows your delivery preference, window focus and OS permission.',
    )
    expect(channels.text()).toContain('Force an in-app toast')
    expect(channels.text()).toContain('Force a system banner')

    wrapper.unmount()
  })

  it('stacks the card into one column until its container has room for two', async () => {
    const wrapper = mount(DevView)

    expect(wrapper.get('[data-testid="dev-notification-form"]').classes()).toContain('@container/notify-test')
    expect(wrapper.get('[data-testid="dev-notification-fields"]').classes()).toEqual(
      expect.arrayContaining(['grid', 'grid-cols-1', '@[420px]/notify-test:grid-cols-2']),
    )
    expect(wrapper.get('[data-testid="dev-notification-actions"]').classes()).toEqual(
      expect.arrayContaining(['flex-col', '@[420px]/notify-test:flex-row']),
    )

    wrapper.unmount()
  })
})

describe('DevView Wails latency', () => {
  it('prices an empty call and a payload call separately', async () => {
    const wrapper = mount(DevView)
    await flushPromises()

    await wrapper.get('[data-testid="dev-latency-measure"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await flushPromises()

    // 5 warm-up calls plus 5 batches of 20, per leg.
    expect(mocks.Ping).toHaveBeenCalledTimes(105)
    expect(mocks.Echo).toHaveBeenCalledTimes(105)
    expect(mocks.Echo).toHaveBeenCalledWith(65536)
    expect(wrapper.get('[data-testid="dev-latency-empty"]').text()).toContain('ms per call')
    expect(wrapper.get('[data-testid="dev-latency-payload"]').text()).toContain('64.0 KB payload')
    // The clock's granularity is why the calls are batched, so it is stated.
    expect(wrapper.get('[data-testid="dev-latency-resolution"]').text()).toContain('resolves to')

    wrapper.unmount()
  })
})
