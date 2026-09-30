import { ref, shallowRef } from 'vue'
import { Settings } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/observabilityservice'
import type { ObservabilitySettings } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export function useObservabilitySettings() {
  const current = shallowRef<ObservabilitySettings | null>(null)
  const loading = ref(false)
  const error = ref('')

  async function refresh(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      current.value = await Settings()
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
    } finally {
      loading.value = false
    }
  }

  return { current, loading, error, refresh }
}
