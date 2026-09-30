// Local webhook listener configuration, shared by the Integrations card (which
// shows the status badge) and the drawer that edits it, so a save updates both
// without either re-fetching independently.
import { ref } from 'vue'
import {
  GeneratePort,
  SetSettings,
  Settings as GetWebhookSettings,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/webhookservice'
import type { WebhookSettings } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type { WebhookSettings }

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

const settings = ref<WebhookSettings | null>(null)
const loading = ref(false)
const error = ref('')
let refreshInFlight: Promise<void> | undefined

// Concurrent callers (card and drawer mount together) share one request.
async function refresh(): Promise<void> {
  if (refreshInFlight) return refreshInFlight
  loading.value = true
  error.value = ''
  refreshInFlight = (async () => {
    try {
      settings.value = await GetWebhookSettings()
    } catch (err) {
      error.value = errText(err)
    } finally {
      loading.value = false
      refreshInFlight = undefined
    }
  })()
  return refreshInFlight
}

// save persists and then re-reads: the backend is what decides whether the new
// configuration leaves a restart pending, so the reply is not assumed here.
async function save(next: { enabled: boolean; port: number }): Promise<boolean> {
  if (!settings.value) return false
  error.value = ''
  try {
    await SetSettings({ ...settings.value, enabled: next.enabled, port: next.port })
  } catch (err) {
    error.value = errText(err)
    return false
  }
  await refresh()
  return true
}

async function generatePort(): Promise<number | undefined> {
  error.value = ''
  try {
    return await GeneratePort()
  } catch (err) {
    error.value = errText(err)
    return undefined
  }
}

export function useWebhookSettings() {
  return { settings, loading, error, refresh, save, generatePort }
}

// Kept only for isolated Vitest modules; production callers share the module
// singleton above for the lifetime of the desktop app.
export function resetWebhookSettingsForTests(): void {
  settings.value = null
  loading.value = false
  error.value = ''
  refreshInFlight = undefined
}
