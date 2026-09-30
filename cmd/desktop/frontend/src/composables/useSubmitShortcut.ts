import { onKeyStroke } from '@vueuse/core'
import { toValue, type MaybeRefOrGetter } from 'vue'

/**
 * ⌘/Ctrl+Enter submits a dialog from anywhere inside it, including the
 * multi-line fields where plain Enter has to stay a newline.
 */
export function useSubmitShortcut(
  onSubmit: () => void,
  options: { enabled?: MaybeRefOrGetter<boolean> } = {},
): void {
  onKeyStroke('Enter', (event) => {
    if (!event.metaKey && !event.ctrlKey) return
    if (!toValue(options.enabled ?? true)) return
    event.preventDefault()
    onSubmit()
  })
}
