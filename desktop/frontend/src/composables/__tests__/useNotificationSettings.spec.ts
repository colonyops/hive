import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resetNotificationSettingsForTests,
  useNotificationSettings,
} from '../useNotificationSettings'

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

const enabledSettings = {
  notificationsEnabled: true,
  systemNotificationsEnabled: true,
  notificationSound: true,
}

beforeEach(() => {
  vi.clearAllMocks()
  resetNotificationSettingsForTests()
  mocks.NotificationSettings.mockResolvedValue(enabledSettings)
  mocks.SetNotificationSettings.mockResolvedValue(undefined)
  mocks.PermissionStatus.mockResolvedValue('not-requested')
  mocks.RequestNotificationPermission.mockResolvedValue(true)
})

describe('useNotificationSettings', () => {
  it('automatically refreshes persisted settings and live permission on first use', async () => {
    mocks.NotificationSettings.mockResolvedValue({
      notificationsEnabled: false,
      systemNotificationsEnabled: true,
      notificationSound: false,
    })
    mocks.PermissionStatus.mockResolvedValue('granted')

    const settings = useNotificationSettings()

    await vi.waitFor(() => {
      expect(settings.notificationsEnabled.value).toBe(false)
      expect(settings.systemNotificationsEnabled.value).toBe(true)
      expect(settings.notificationSound.value).toBe(false)
      expect(settings.permission.value).toBe('granted')
    })
    expect(mocks.NotificationSettings).toHaveBeenCalledOnce()
    expect(mocks.PermissionStatus).toHaveBeenCalledOnce()
  })

  it('persists an optimistic toggle and rolls it back when saving fails', async () => {
    const settings = useNotificationSettings()
    const failure = new Error('save failed')
    mocks.SetNotificationSettings.mockRejectedValueOnce(failure)

    const pending = settings.setNotificationsEnabled(false)
    expect(settings.notificationsEnabled.value).toBe(false)
    expect(mocks.SetNotificationSettings).toHaveBeenCalledWith({
      notificationsEnabled: false,
      systemNotificationsEnabled: true,
      notificationSound: true,
    })

    await pending
    expect(settings.notificationsEnabled.value).toBe(true)
    expect(settings.error.value).toBe('save failed')
  })

  it('does not let an initial refresh overwrite a newer optimistic toggle', async () => {
    let resolveSettings: (settings: typeof enabledSettings) => void
    mocks.NotificationSettings.mockImplementationOnce(() => new Promise(resolve => {
      resolveSettings = resolve
    }))

    const settings = useNotificationSettings()
    await settings.setNotificationsEnabled(false)
    resolveSettings!(enabledSettings)
    await settings.refresh()

    expect(settings.notificationsEnabled.value).toBe(false)
  })

  it('uses the request result and then refreshes the live permission state', async () => {
    const settings = useNotificationSettings()
    mocks.RequestNotificationPermission.mockResolvedValue(false)
    mocks.PermissionStatus.mockResolvedValue('denied')

    await settings.requestPermission()

    expect(mocks.RequestNotificationPermission).toHaveBeenCalledOnce()
    expect(mocks.PermissionStatus).toHaveBeenCalledTimes(2)
    expect(settings.permission.value).toBe('denied')
  })
})
