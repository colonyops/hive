import { onKeyStroke } from '@vueuse/core'
import { toValue, type MaybeRefOrGetter } from 'vue'

export function useEscapeToClose(
  onEscape: () => void,
  options: { enabled?: MaybeRefOrGetter<boolean> } = {},
): void {
  onKeyStroke('Escape', () => {
    if (toValue(options.enabled ?? true)) onEscape()
  })
}
