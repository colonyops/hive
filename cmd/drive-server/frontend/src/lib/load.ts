import { errorText } from '@hive/ui/lib/errorText'
import { useAsyncState } from '@vueuse/core'
import { computed } from 'vue'

/**
 * A request a page shows: `data` is null until the first success and keeps
 * the last good value through a failed reload; `error` is the text to show.
 */
export function useLoad<T>(load: () => Promise<T>, fallback: string, options: { immediate?: boolean } = {}) {
  const { state, isLoading, error, execute } = useAsyncState<T | null>(load, null, {
    immediate: options.immediate ?? true,
    resetOnExecute: false,
    // The default hands the error to globalThis.reportError; the page shows it instead.
    onError: () => {},
  })
  return {
    data: state,
    loading: isLoading,
    failure: error,
    error: computed(() => (error.value === undefined ? null : errorText(error.value, fallback))),
    reload: async (): Promise<void> => {
      await execute()
    },
  }
}
