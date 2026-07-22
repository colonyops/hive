import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import NotificationSettingsView from '../NotificationSettingsView.vue'
import { resetNotificationSettingsForTests } from '../../composables/useNotificationSettings'

const mocks = vi.hoisted(() => ({
  NotificationSettings: vi.fn(),
  SetNotificationSettings: vi.fn(),
  PermissionStatus: vi.fn(),
  RequestNotificationPermission: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/settingsservice', () => ({
  NotificationSettings: mocks.NotificationSettings,
  SetNotificationSettings: mocks.SetNotificationSettings,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/desktop/notificationservice', () => ({
  PermissionStatus: mocks.PermissionStatus,
  RequestNotificationPermission: mocks.RequestNotificationPermission,
}))

beforeEach(() => {
  vi.clearAllMocks()
  resetNotificationSettingsForTests()
  mocks.NotificationSettings.mockResolvedValue({
    notificationsEnabled: true,
    systemNotificationsEnabled: true,
    notificationSound: true,
  })
  mocks.SetNotificationSettings.mockResolvedValue(undefined)
  mocks.PermissionStatus.mockResolvedValue('not-requested')
  mocks.RequestNotificationPermission.mockResolvedValue(true)
})

describe('NotificationSettingsView', () => {
  it('renders preferences, disables system notifications with master off, and persists the switch', async () => {
    const wrapper = mount(NotificationSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="notification-enable"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="notification-system"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[data-testid="notification-sound"]').exists()).toBe(true)

    await wrapper.find('[data-testid="notification-enable"]').trigger('click')
    await flushPromises()

    expect(mocks.SetNotificationSettings).toHaveBeenCalledWith({
      notificationsEnabled: false,
      systemNotificationsEnabled: true,
      notificationSound: true,
    })
    expect(wrapper.find('[data-testid="notification-system"]').attributes('disabled')).toBeDefined()
  })

  it('requests permission and renders denied guidance from the live state', async () => {
    const wrapper = mount(NotificationSettingsView)
    await flushPromises()
    expect(wrapper.find('[data-testid="notification-permission-status"]').text()).toBe('Not requested')

    mocks.RequestNotificationPermission.mockResolvedValue(false)
    mocks.PermissionStatus.mockResolvedValue('denied')
    await wrapper.find('[data-testid="notification-permission-request"]').trigger('click')
    await flushPromises()

    expect(mocks.RequestNotificationPermission).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="notification-permission-status"]').text()).toBe('Denied')
    expect(wrapper.find('[data-testid="notification-permission-denied-guidance"]').exists()).toBe(true)
  })
})
