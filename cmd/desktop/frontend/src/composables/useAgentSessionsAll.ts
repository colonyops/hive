import { ref, type Ref } from 'vue'
import { useAgentWorkspaces } from './useAgentWorkspaces'
import type { AgentSession } from '../lib/agentWorkspacesClient'

// The Recents list's data: every session across every workspace, most
// recently opened first. A module singleton mirroring useAgentWorkspaces.ts's
// shape for the same reason — one Agents area is ever mounted, and reusing
// its client/ready() rather than duplicating the probe keeps the two lists in
// step with the same availability answer.

const recents = ref<AgentSession[]>([])
const recentsLoading = ref(false)
const recentsLoaded = ref(false)
const recentsError = ref<string | null>(null)

async function reloadRecents(): Promise<void> {
  const { client, ready } = useAgentWorkspaces()
  await ready()
  if (!client.value) return
  recentsLoading.value = true
  recentsError.value = null
  try {
    recents.value = await client.value.allSessions()
  } catch (e) {
    // Keep the last-good rows, matching useAgentWorkspaces' reload functions.
    recentsError.value = e instanceof Error && e.message ? e.message : 'Could not list recent sessions.'
  } finally {
    recentsLoading.value = false
    recentsLoaded.value = true
  }
}

export function useAgentSessionsAll(): {
  recents: Ref<AgentSession[]>
  recentsLoading: Ref<boolean>
  recentsLoaded: Ref<boolean>
  recentsError: Ref<string | null>
  reloadRecents: () => Promise<void>
} {
  return { recents, recentsLoading, recentsLoaded, recentsError, reloadRecents }
}

export function resetAgentSessionsAllForTests(): void {
  recents.value = []
  recentsLoading.value = false
  recentsLoaded.value = false
  recentsError.value = null
}
