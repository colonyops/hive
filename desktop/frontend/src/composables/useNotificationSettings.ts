import { ref } from 'vue'
import {
  NotificationSettings as GetNotificationSettings,
  SetNotificationSettings,
} from '../../bindings/github.com/colonyops/hive/desktop/settingsservice'
import {
  PermissionStatus,
  RequestNotificationPermission,
} from '../../bindings/github.com/colonyops/hive/desktop/notificationservice'
import type { NotificationSettings } from '../../bindings/github.com/colonyops/hive/desktop/models'

export type NotificationPermission = 'granted' | 'denied' | 'not-requested'

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function toPermissionStatus(value: string): NotificationPermission {
  if (value === 'granted' || value === 'denied' || value === 'not-requested') return value
  return 'not-requested'
}

// Notification preferences are application-lifetime state: useNotify will use
// the same values later, so every caller intentionally shares these refs.
const notificationsEnabled = ref(true)
const systemNotificationsEnabled = ref(true)
const notificationSound = ref(true)
const permission = ref<NotificationPermission>('not-requested')
const loading = ref(false)
const requestingPermission = ref(false)
const error = ref('')
let refreshInFlight: Promise<void> | undefined
let initialized = false
// Refresh only applies values if no local action has changed that part of the
// shared state since the refresh began.
let settingsVersion = 0
let permissionVersion = 0

function currentSettings(): NotificationSettings {
  return {
    notificationsEnabled: notificationsEnabled.value,
    systemNotificationsEnabled: systemNotificationsEnabled.value,
    notificationSound: notificationSound.value,
  }
}

// refresh coalesces concurrent callers. Initializing it from the composable
// means useNotify sees persisted preferences even when Settings is never
// opened. A missing native permission provider must not hide persisted
// preferences (such as in the browser/server build), so both calls complete
// before their successful results are applied.
async function refresh(): Promise<void> {
  if (refreshInFlight) return refreshInFlight

  const settingsSnapshot = settingsVersion
  const permissionSnapshot = permissionVersion
  loading.value = true
  error.value = ''
  refreshInFlight = (async () => {
    try {
      const [settingsResult, permissionResult] = await Promise.allSettled([GetNotificationSettings(), PermissionStatus()])
      const errors: string[] = []
      if (settingsResult.status === 'fulfilled') {
        if (settingsVersion === settingsSnapshot) {
          notificationsEnabled.value = settingsResult.value.notificationsEnabled
          systemNotificationsEnabled.value = settingsResult.value.systemNotificationsEnabled
          notificationSound.value = settingsResult.value.notificationSound
        }
      } else {
        errors.push(errText(settingsResult.reason))
      }
      if (permissionResult.status === 'fulfilled') {
        if (permissionVersion === permissionSnapshot) permission.value = toPermissionStatus(permissionResult.value)
      } else {
        errors.push(errText(permissionResult.reason))
      }
      if (errors.length > 0) error.value = errors.join('; ')
    } finally {
      loading.value = false
      refreshInFlight = undefined
    }
  })()
  return refreshInFlight
}

async function persist(value: boolean, setting: typeof notificationsEnabled): Promise<void> {
  const previous = setting.value
  const version = ++settingsVersion
  setting.value = value
  error.value = ''
  try {
    await SetNotificationSettings(currentSettings())
  } catch (err) {
    // Do not undo a newer toggle that happened while this request was pending.
    if (settingsVersion === version && setting.value === value) setting.value = previous
    error.value = errText(err)
  }
}

async function setNotificationsEnabled(value: boolean): Promise<void> {
  await persist(value, notificationsEnabled)
}

async function setSystemNotificationsEnabled(value: boolean): Promise<void> {
  await persist(value, systemNotificationsEnabled)
}

async function setNotificationSound(value: boolean): Promise<void> {
  await persist(value, notificationSound)
}

async function requestPermission(): Promise<void> {
  const version = ++permissionVersion
  requestingPermission.value = true
  error.value = ''
  try {
    const granted = await RequestNotificationPermission()
    if (permissionVersion === version) permission.value = granted ? 'granted' : 'denied'
    try {
      const refreshedPermission = toPermissionStatus(await PermissionStatus())
      // A newer request must win over this request's follow-up OS read.
      if (permissionVersion === version) permission.value = refreshedPermission
    } catch (err) {
      // The request result is still the best available live state if querying
      // the OS immediately afterward is unavailable.
      error.value = errText(err)
    }
  } catch (err) {
    error.value = errText(err)
  } finally {
    requestingPermission.value = false
  }
}

export function useNotificationSettings() {
  if (!initialized) {
    initialized = true
    void refresh()
  }

  return {
    notificationsEnabled,
    systemNotificationsEnabled,
    notificationSound,
    permission,
    loading,
    requestingPermission,
    error,
    refresh,
    setNotificationsEnabled,
    setSystemNotificationsEnabled,
    setNotificationSound,
    requestPermission,
  }
}

// Kept only for isolated Vitest modules; production callers share the module
// singleton above for the lifetime of the desktop app.
export function resetNotificationSettingsForTests(): void {
  notificationsEnabled.value = true
  systemNotificationsEnabled.value = true
  notificationSound.value = true
  permission.value = 'not-requested'
  loading.value = false
  requestingPermission.value = false
  error.value = ''
  refreshInFlight = undefined
  initialized = false
  settingsVersion = 0
  permissionVersion = 0
}
