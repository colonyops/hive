import type { Ref } from 'vue'
import type { RecordInput } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/activity/models'
import type { NotifyInput } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { Notify as NotifyNative } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/notificationservice'
import { useActivity } from './useActivity'
import { useNotificationSettings, type NotificationDelivery, type NotificationPermission } from './useNotificationSettings'
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
  /**
   * Opaque routing data carried on the OS notification and handed back when
   * the user clicks it. A feed notification puts the inbox item's identity
   * here so Go's notification:activate can name what to open; the toast path
   * ignores it (an in-app toast has nowhere to navigate from).
   */
  data?: Record<string, unknown>
  /**
   * Overrides the node's sound to off. The app's sound preference is the
   * ceiling: this can quiet a notification, never unmute one.
   */
  silent?: boolean
}

export const notifySeverityMapping: Record<NotifySeverity, { activity: string; toast: ToastSeverity }> = {
  info: { activity: 'info', toast: 'info' },
  success: { activity: 'success', toast: 'success' },
  warning: { activity: 'warning', toast: 'warning' },
  error: { activity: 'error', toast: 'error' },
}

export interface NotifySettings {
  notificationsEnabled: Readonly<Ref<boolean>>
  delivery: Readonly<Ref<NotificationDelivery>>
  notificationSound: Readonly<Ref<boolean>>
  permission: Readonly<Ref<NotificationPermission>>
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
      delivery: settings.delivery,
      notificationSound: settings.notificationSound,
      permission: settings.permission,
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

    // Delivery decides where an eligible notification surfaces: "app" never
    // leaves the window, "system" always leaves it, and "auto" — the default —
    // only raises a banner when the user is looking elsewhere.
    const delivery = deps.settings.delivery.value
    if (delivery === 'app' || (delivery === 'auto' && deps.focused.value)) {
      toast()
      return
    }

    // A banner needs a granted OS permission. This path never requests it: the
    // grant is asked once, deliberately, during onboarding (or later in
    // Settings), so a not-requested or denied state falls back to a toast
    // instead of surprising the user with a dialog mid-usage.
    if (deps.settings.permission.value !== 'granted') {
      toast()
      return
    }
    try {
      await deps.osNotify({
        title: event.title,
        subtitle: '',
        body,
        severity,
        sound: deps.settings.notificationSound.value && !event.silent,
        data: event.data ?? {},
      })
    } catch (error) {
      console.warn('[notify] native notification failed; surfacing toast instead', error)
      toast()
    }
  }

  return { notify }
}
