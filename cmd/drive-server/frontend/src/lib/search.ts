import { errorText } from '@hive/ui/lib/errorText'
import { onScopeDispose, ref, watch } from 'vue'
import { api, ApiError } from '../api'
import type { SearchHit } from '../types'

const debounceMs = 200

/**
 * Debounced full-text search. `unavailable` is set when the server has no
 * search endpoint (404).
 */
export function useSearch(query: () => string, space: () => string | undefined) {
  const hits = ref<SearchHit[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const unavailable = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  let latest = 0

  async function run(text: string, scope: string | undefined): Promise<void> {
    const id = ++latest
    loading.value = true
    try {
      const results = await api.search(text, scope)
      if (id !== latest) return
      hits.value = results.hits
      error.value = null
      unavailable.value = false
    } catch (cause) {
      if (id !== latest) return
      hits.value = []
      if (cause instanceof ApiError && cause.status === 404) unavailable.value = true
      else error.value = errorText(cause, 'Search failed.')
    } finally {
      if (id === latest) loading.value = false
    }
  }

  watch(
    () => [query().trim(), space()] as const,
    ([text, scope]) => {
      clearTimeout(timer)
      if (!text || unavailable.value) {
        latest++
        hits.value = []
        loading.value = false
        error.value = null
        return
      }
      loading.value = true
      timer = setTimeout(() => void run(text, scope), debounceMs)
    },
  )
  onScopeDispose(() => clearTimeout(timer))

  return { hits, loading, error, unavailable }
}
