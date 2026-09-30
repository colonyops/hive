import { ref, shallowRef, type Ref, type ShallowRef } from 'vue'
import type { TerminalClient } from '../lib/terminalClient'

// The tmux availability probe's answer and the control client. Module state
// because both are properties of the backend process rather than of a mounted
// view: the "Checking tmux…" gate is shown once, and every later probe
// revalidates the answer already on screen instead of blanking it. The endpoint
// (loopback address + bearer token) is stable for the life of that process, so
// a created client is never re-minted.
const checking = ref(true)
const available = ref(false)
const reason = ref('')
const client: ShallowRef<TerminalClient | null> = shallowRef(null)

export function useTerminalAvailability(): {
  checking: Ref<boolean>
  available: Ref<boolean>
  reason: Ref<string>
  client: ShallowRef<TerminalClient | null>
} {
  return { checking, available, reason, client }
}

export function resetTerminalAvailabilityForTests(): void {
  checking.value = true
  available.value = false
  reason.value = ''
  client.value = null
}
