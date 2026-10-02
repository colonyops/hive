import { computed, onMounted, ref } from 'vue'
import { List } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice'
import type { Integration } from '../types/integrations'
import { useWailsEvent } from './useWailsEvent'

/**
 * The connector registry as the Integrations screen and the source node
 * editor read it: what each connector is, and which accounts are connected.
 *
 * The list is a projection of the Go registry — there is no second list of
 * connectors here to drift out of sync with it.
 */
export function useIntegrations() {
  const integrations = ref<Integration[]>([])
  // Distinct from an empty list: "still loading" must not render as "no
  // integrations", which would flash an empty screen on every open.
  const loaded = ref(false)
  const error = ref<string | null>(null)

  async function reload() {
    try {
      // The generator types every Go slice as nullable. Normalizing here is
      // what lets the rest of the app read `accounts` without a guard.
      integrations.value = ((await List()) ?? []).map((i) => ({ ...i, accounts: i.accounts ?? [] }))
      error.value = null
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
    } finally {
      loaded.value = true
    }
  }

  /** Connected account names for one provider, e.g. 'github'. */
  function accountsFor(provider: string): string[] {
    return integrations.value.find((i) => i.provider === provider)?.accounts ?? []
  }

  /** Credential refs for one provider, in the 'provider/account' form a node's `credential:` field takes. */
  function credentialRefsFor(provider: string): string[] {
    return accountsFor(provider).map((account) => `${provider}/${account}`)
  }

  const connectedCount = computed(() => integrations.value.filter(isConnected).length)

  onMounted(async () => {
    // Any provider's change can add or remove an account here, so unlike
    // useGitHubConnection this listens across providers and does not filter.
    useWailsEvent('connection:updated', () => {
      void reload()
    })
    await reload()
  })

  return { integrations, loaded, error, reload, accountsFor, credentialRefsFor, connectedCount }
}

/**
 * Whether an integration can currently fetch. Mirrors Integration.Connected in
 * Go: an environment override authenticates every fetch while naming no
 * account, so an empty account list is not the same as disconnected.
 */
export function isConnected(integration: Integration): boolean {
  return integration.accounts.length > 0 || integration.envOverride
}

/**
 * Whether this connector has anything to connect at all. A webhook listener is
 * local ingress — it is not "not connected", and it gets no Connect action.
 */
export function takesCredential(integration: Integration): boolean {
  return integration.provider !== ''
}
