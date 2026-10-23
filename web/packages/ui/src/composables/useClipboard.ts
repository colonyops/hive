import { computed, onScopeDispose, ref } from 'vue'
import { useHiveUiPlatform } from '../platform'

export type CopyStatus = 'idle' | 'success' | 'error'

// useClipboard is the single copy-to-clipboard implementation. It writes
// through the platform clipboard, not navigator.clipboard or vueuse's
// useClipboard, because a host's webview may not honour the browser API.
//
// `status` drives success/error affordances; `copied` is the boolean
// convenience for simple buttons. Both auto-reset to idle after resetDelay ms,
// and the pending timer is cleared when the owning scope is disposed.
export function useClipboard(options: { resetDelay?: number } = {}) {
  const platform = useHiveUiPlatform()
  const resetDelay = options.resetDelay ?? 2000
  const status = ref<CopyStatus>('idle')
  const copied = computed(() => status.value === 'success')
  let timer: ReturnType<typeof setTimeout> | undefined

  // Drives the same auto-resetting affordance from outside a copy attempt —
  // for callers that must fetch the text first (prompts are rendered by the Go
  // service), where a failure before the clipboard write is still a failed copy
  // as far as the user is concerned.
  function setStatus(next: CopyStatus): void {
    status.value = next
    if (timer !== undefined) clearTimeout(timer)
    if (next === 'idle') return
    timer = setTimeout(() => {
      status.value = 'idle'
    }, resetDelay)
  }

  /** Resolves to whether the text reached the clipboard; it never rejects. */
  async function copy(text: string): Promise<boolean> {
    try {
      await platform.clipboard.writeText(text)
      setStatus('success')
      return true
    } catch {
      setStatus('error')
      return false
    }
  }

  onScopeDispose(() => {
    if (timer !== undefined) clearTimeout(timer)
  })

  return { copy, setStatus, status, copied }
}
