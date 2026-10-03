import { errorText } from '../lib/appError'
import { ref } from 'vue'

/**
 * In-flight state for a provider connected by pasting a URL and a token
 * (Gitea, Grafana, PostHog). The connected-accounts list comes from
 * useIntegrations. GitHub's device flow is useGitHubConnection.
 */
export function useTokenConnection(disconnectAccount: (account: string) => Promise<void>, noun: string) {
  const busy = ref(false)
  const error = ref<string | null>(null)

  /** Runs one backend call. Resolves false, with `error` set, when it fails or another call is in flight. */
  async function run(call: () => Promise<unknown>, fallback: string): Promise<boolean> {
    if (busy.value) return false
    error.value = null
    busy.value = true
    try {
      await call()
      return true
    } catch (err) {
      error.value = errorText(err, fallback)
      return false
    } finally {
      busy.value = false
    }
  }

  async function disconnect(account: string): Promise<void> {
    error.value = null
    try {
      await disconnectAccount(account)
    } catch (err) {
      error.value = errorText(err, `Could not disconnect the ${noun}.`)
    }
  }

  return { busy, error, run, disconnect }
}
