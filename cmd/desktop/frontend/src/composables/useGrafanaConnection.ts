import { ref } from 'vue'
import { Connect, Disconnect } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/grafanaservice'
import type { Stack } from '../types/grafana'

/**
 * Unlike GitHub there is no device flow or polled status; the connected-accounts
 * list comes from useIntegrations, so this owns only in-flight connect/disconnect
 * state.
 */
export function useGrafanaConnection() {
  const busy = ref(false)
  const error = ref<string | null>(null)
  const lastConnected = ref<Stack | null>(null)

  async function connect(url: string, token: string): Promise<boolean> {
    if (busy.value) return false
    error.value = null
    busy.value = true
    try {
      lastConnected.value = await Connect(url, token)
      return true
    } catch (err) {
      error.value = messageOf(err, 'Grafana rejected the connection.')
      return false
    } finally {
      busy.value = false
    }
  }

  async function disconnect(account: string): Promise<void> {
    error.value = null
    try {
      await Disconnect(account)
    } catch (err) {
      error.value = messageOf(err, 'Could not disconnect the stack.')
    }
  }

  return { busy, error, lastConnected, connect, disconnect }
}

function messageOf(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message
  if (typeof err === 'string' && err) return err
  return fallback
}
