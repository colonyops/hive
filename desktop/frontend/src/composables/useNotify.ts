import type { Ref } from 'vue'
import type { RecordInput } from '../../bindings/github.com/colonyops/hive/internal/desktop/activity/models'
import type { NotifyInput } from '../../bindings/github.com/colonyops/hive/desktop/models'
import { Notify as NotifyNative } from '../../bindings/github.com/colonyops/hive/desktop/notificationservice'
import { useActivity } from './useActivity'
import { useNotificationSettings, type NotificationPermission } from './useNotificationSettings'
import { useToasts } from './useToasts'
import { useWindowFocus } from './useWindowFocus'
import type { ToastOptions, ToastSeverity } from '../types/toast'

export type NotifySeverity = 'info' | 'success' | 'warning' | 'error'

export interface NotifyEvent {
  title: string
  body?: string
  severity?: NotifySeverity
  category?: string
  source?: string
}

export const notifySeverityMapping: Record<NotifySeverity, { activity: string; toast: ToastSeverity }> = {
  info: { activity: 'info', toast: 'info' },
  success: { activity: 'success', toast: 'success' },
  warning: { activity: 'warning', toast: 'info' },
  error: { activity: 'error', toast: 'error' },
}

export interface NotifySettings {
  notificationsEnabled: Readonly<Ref<boolean>>
  systemNotificationsEnabled: Readonly<Ref<boolean>>
  notificationSound: Readonly<Ref<boolean>>
  permission: Readonly<Ref<NotificationPermission>>
  requestPermission: () => Promise<void>
}

export interface NotifyDeps {
  record: (event: RecordInput) => Promise<boolean>
  showToast: (message: string, options?: ToastOptions) => number
  focused: Readonly<Ref<boolean>>
  settings: NotifySettings
  osNotify: (input: NotifyInput) => Promise<void>
}

function defaults(): NotifyDeps {
  const { record } = useActivity()
  const { showToast } = useToasts()
  const { focused } = useWindowFocus()
  const settings = useNotificationSettings()
  return {
    record,
    showToast,
    focused,
    settings: {
      notificationsEnabled: settings.notificationsEnabled,
      systemNotificationsEnabled: settings.systemNotificationsEnabled,
      notificationSound: settings.notificationSound,
      permission: settings.permission,
      requestPermission: settings.requestPermission,
    },
    osNotify: NotifyNative,
  }
}

export function useNotify(overrides: Partial<NotifyDeps> = {}) {
  // Fully injected instances are hermetic (and avoid starting production
  // singletons); partial injection deliberately fills all missing production
  // dependencies so tests and callers can override just one seam.
  const required: Array<keyof NotifyDeps> = ['record', 'showToast', 'focused', 'settings', 'osNotify']
  const production = required.every((key) => overrides[key] !== undefined) ? undefined : defaults()
  const deps = { ...production, ...overrides } as NotifyDeps

  async function notify(event: NotifyEvent): Promise<void> {
    const severity = event.severity ?? 'info'
    const category = event.category ?? 'system'
    const body = event.body ?? ''
    const source = event.source ?? ''
    const mapping = notifySeverityMapping[severity]
    const recorded = await deps.record({ title: event.title, body, severity: mapping.activity, category, source, metadata: null })
    if (!recorded) console.warn('[notify] activity record failed; surfacing anyway', event)

    if (!deps.settings.notificationsEnabled.value) return
    const toast = () => deps.showToast(event.title, { body, severity: mapping.toast })
    if (deps.focused.value) {
      toast()
      return
    }
    if (!deps.settings.systemNotificationsEnabled.value) return

    if (deps.settings.permission.value === 'not-requested') await deps.settings.requestPermission()
    if (deps.settings.permission.value !== 'granted') {
      toast()
      return
    }
    try {
      await deps.osNotify({ title: event.title, subtitle: '', body, sound: deps.settings.notificationSound.value, data: {} })
    } catch (error) {
      console.warn('[notify] native notification failed; surfacing toast instead', error)
      toast()
    }
  }

  return { notify }
}
