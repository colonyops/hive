import { computed, readonly, ref } from 'vue'
import {
  GeneratePort,
  SetSettings,
  Settings as GetWebhookSettings,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/webhookservice'
import type { WebhookSettings } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

export type { WebhookSettings }

// Local webhook listener configuration, shared by the Integrations card (which
// shows the status badge) and the drawer that edits it, so a save updates both
// without either re-fetching independently.
export const useWebhookSettings = defineStore('webhookSettings', () => {
  const settings = useResource<WebhookSettings | null>(() => GetWebhookSettings(), {
    initial: null,
    errorFallback: 'Could not load the webhook settings.',
  })
  const actionError = ref<string | null>(null)

  async function reload(): Promise<void> {
    actionError.value = null
    await settings.reload()
  }

  // Persists and then re-reads: the backend decides whether the new
  // configuration leaves a restart pending, so the reply is not assumed here.
  async function save(next: { enabled: boolean; port: number }): Promise<boolean> {
    const current = settings.data.value
    if (!current) return false
    actionError.value = null
    try {
      await SetSettings({ ...current, enabled: next.enabled, port: next.port })
    } catch (error) {
      actionError.value = errorText(error, 'Could not save the webhook settings.')
      return false
    }
    await reload()
    return true
  }

  async function generatePort(): Promise<number | undefined> {
    actionError.value = null
    try {
      return await GeneratePort()
    } catch (error) {
      actionError.value = errorText(error, 'Could not generate a port.')
      return undefined
    }
  }

  void reload()

  return {
    settings: readonly(settings.data),
    loading: settings.loading,
    error: computed(() => actionError.value ?? settings.error.value),
    reload,
    save,
    generatePort,
  }
})
