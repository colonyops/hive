import { inject, onScopeDispose, ref, watch, type InjectionKey, type Ref } from 'vue'
import { api, ApiError } from '../api'
import type { SearchHit } from '../types'

const debounceMs = 200

/** The toolbar's search text, shared with the folder listing, which it filters. */
export const searchQueryKey: InjectionKey<Ref<string>> = Symbol('searchQuery')

export function useSearchQuery(): Ref<string> {
  return inject(searchQueryKey, ref(''))
}

/**
 * Debounced full-text search. `unavailable` is set when the server has no
 * search endpoint (404), so the UI can fall back to the listing filter.
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
      else error.value = cause instanceof Error ? cause.message : 'Search failed.'
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
