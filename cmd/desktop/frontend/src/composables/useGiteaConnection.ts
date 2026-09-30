import { ref } from 'vue'
import { Connect, Disconnect } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/giteaservice'
import type { Instance } from '../types/gitea'

/**
 * Like Grafana and unlike GitHub there is no device flow or polled status:
 * Gitea's device flow needs an OAuth application registered on each instance,
 * so an account is connected by pasting a URL and an access token. The
 * connected-accounts list comes from useIntegrations, so this owns only
 * in-flight connect/disconnect state.
 */
export function useGiteaConnection() {
  const busy = ref(false)
  const error = ref<string | null>(null)
  const lastConnected = ref<Instance | null>(null)

  async function connect(url: string, token: string): Promise<boolean> {
    if (busy.value) return false
    error.value = null
    busy.value = true
    try {
      lastConnected.value = await Connect(url, token)
      return true
    } catch (err) {
      error.value = messageOf(err, 'Gitea rejected the connection.')
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
      error.value = messageOf(err, 'Could not disconnect the account.')
    }
  }

  return { busy, error, lastConnected, connect, disconnect }
}

function messageOf(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message
  if (typeof err === 'string' && err) return err
  return fallback
}
