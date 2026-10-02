import { readonly, ref, shallowReadonly, shallowRef } from 'vue'
import type { AgentSession } from '../lib/agentWorkspacesClient'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'
import { useAgentWorkspaces } from './useAgentWorkspaces'

// The Recents list's data: every session across every workspace, most
// recently opened first. It reuses useAgentWorkspaces' client and probe rather
// than duplicating the probe, which keeps the two lists in step with the same
// availability answer. The store does not load itself: the Agents area and
// the palette reload it when they open.
export const useAgentSessionsAll = defineStore('agentSessionsAll', () => {
  const recents = shallowRef<AgentSession[]>([])
  const recentsLoading = ref(false)
  const recentsLoaded = ref(false)
  const recentsError = ref<string | null>(null)

  async function reload(): Promise<void> {
    const { client, ready } = useAgentWorkspaces()
    await ready()
    if (!client.value) return
    recentsLoading.value = true
    recentsError.value = null
    try {
      recents.value = await client.value.allSessions()
    } catch (e) {
      // Keep the last-good rows, matching useAgentWorkspaces' reload functions.
      recentsError.value = errorText(e, 'Could not list recent sessions.')
    } finally {
      recentsLoading.value = false
      recentsLoaded.value = true
    }
  }

  return {
    recents: shallowReadonly(recents),
    recentsLoading: readonly(recentsLoading),
    recentsLoaded: readonly(recentsLoaded),
    recentsError: readonly(recentsError),
    reload,
  }
})
