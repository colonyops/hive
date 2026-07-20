import { computed, onScopeDispose, ref } from 'vue'
import { Clipboard } from '@wailsio/runtime'

export type CopyStatus = 'idle' | 'success' | 'error'

// useClipboard is the single copy-to-clipboard implementation for the desktop
// app. It deliberately uses the native Wails clipboard (Clipboard.SetText) and
// NOT navigator.clipboard / vueuse's useClipboard, which are built on it: in
// the WKWebView that browser API silently no-ops when the document is not
// focused, so copy worked only on every other click. Routing through the Wails
// runtime writes to the OS clipboard regardless of webview focus.
//
// `status` drives success/error affordances; `copied` is the boolean
// convenience for simple buttons. Both auto-reset to idle after resetDelay ms,
// and the pending timer is cleared when the owning scope is disposed.
export function useClipboard(options: { resetDelay?: number } = {}) {
  const resetDelay = options.resetDelay ?? 2000
  const status = ref<CopyStatus>('idle')
  const copied = computed(() => status.value === 'success')
  let timer: ReturnType<typeof setTimeout> | undefined

  async function copy(text: string): Promise<void> {
    try {
      await Clipboard.SetText(text)
      status.value = 'success'
    } catch {
      status.value = 'error'
    }
    if (timer !== undefined) clearTimeout(timer)
    timer = setTimeout(() => {
      status.value = 'idle'
    }, resetDelay)
  }

  onScopeDispose(() => {
    if (timer !== undefined) clearTimeout(timer)
  })

  return { copy, status, copied }
}
