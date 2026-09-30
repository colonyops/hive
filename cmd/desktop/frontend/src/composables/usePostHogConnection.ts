import { ref } from 'vue'
import { Connect, Disconnect, Projects } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/posthogservice'
import type { Project } from '../types/posthog'

/**
 * Connecting PostHog is two steps because a personal API key spans projects:
 * `loadProjects` validates the key and lists what it can reach, then `connect`
 * binds the one the user picked. The connected-accounts list comes from
 * useIntegrations, so this owns only the in-flight and picker state.
 */
export function usePostHogConnection() {
  const busy = ref(false)
  const error = ref<string | null>(null)
  const projects = ref<Project[]>([])
  const lastConnected = ref<Project | null>(null)

  async function loadProjects(url: string, token: string): Promise<boolean> {
    if (busy.value) return false
    error.value = null
    busy.value = true
    try {
      projects.value = (await Projects(url, token)) ?? []
      return projects.value.length > 0
    } catch (err) {
      projects.value = []
      error.value = messageOf(err, 'PostHog rejected the API key.')
      return false
    } finally {
      busy.value = false
    }
  }

  async function connect(url: string, token: string, projectID: number): Promise<boolean> {
    if (busy.value) return false
    error.value = null
    busy.value = true
    try {
      lastConnected.value = await Connect(url, token, projectID)
      return true
    } catch (err) {
      error.value = messageOf(err, 'PostHog rejected the connection.')
      return false
    } finally {
      busy.value = false
    }
  }

  /** Drops the picker so the drawer returns to asking for a host and key. */
  function reset(): void {
    projects.value = []
    error.value = null
  }

  async function disconnect(account: string): Promise<void> {
    error.value = null
    try {
      await Disconnect(account)
    } catch (err) {
      error.value = messageOf(err, 'Could not disconnect the project.')
    }
  }

  return { busy, error, projects, lastConnected, loadProjects, connect, reset, disconnect }
}

function messageOf(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message
  if (typeof err === 'string' && err) return err
  return fallback
}
