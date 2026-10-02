import { computed, readonly, ref, shallowRef } from 'vue'
import { Available } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice'
import { errorText } from '../lib/appError'
import { createTerminalClient, getTerminalEndpoint, type TerminalClient } from '../lib/terminalClient'
import { defineStore } from './defineStore'

// The tmux availability probe's answer and the control client. Both are
// properties of the backend process rather than of a mounted view: the
// "Checking tmux…" gate is shown once, and every later probe revalidates the
// answer already on screen instead of blanking it. The endpoint (loopback
// address + bearer token) is stable for the life of that process, so a created
// client is never re-minted.
export const useTerminalAvailability = defineStore('terminalAvailability', () => {
  const checking = ref(true)
  const available = ref(false)
  const reason = ref('')
  const client = shallowRef<TerminalClient | null>(null)

  async function probe(): Promise<void> {
    if (!client.value) checking.value = true
    try {
      const availability = await Available()
      available.value = availability.available
      reason.value = availability.reason
      if (availability.available && !client.value) client.value = createTerminalClient(await getTerminalEndpoint())
    } catch (error) {
      available.value = false
      reason.value = errorText(error, 'The terminal is unavailable.')
    } finally {
      checking.value = false
    }
  }

  return {
    checking: readonly(checking),
    available: readonly(available),
    reason: readonly(reason),
    client: computed(() => client.value),
    probe,
  }
})
