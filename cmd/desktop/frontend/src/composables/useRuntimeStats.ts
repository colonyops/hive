import { useIntervalFn } from '@vueuse/core'
import { ref, shallowRef } from 'vue'
import { Stats } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/observabilityservice'
import type { RuntimeStats } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

// Samples the sparklines keep. It matches SparkLine's slot count, so the line
// spans the card once this many have landed — a bit over a minute at the
// default cadence — and scrolls from then on.
const HISTORY = 40

const DEFAULT_INTERVAL_MS = 2000

// CPU is a rate since the backend's previous sample, so the first sample after
// a pause covers the entire gap.
export function useRuntimeStats(intervalMs = DEFAULT_INTERVAL_MS) {
  const stats = shallowRef<RuntimeStats | null>(null)
  const rssHistory = ref<number[]>([])
  const cpuHistory = ref<number[]>([])
  const error = ref('')

  async function refresh(): Promise<void> {
    try {
      const sample = await Stats()
      stats.value = sample
      rssHistory.value = [...rssHistory.value, sample.totalRssBytes].slice(-HISTORY)
      cpuHistory.value = [...cpuHistory.value, sample.totalCpuPercent].slice(-HISTORY)
      error.value = ''
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
    }
  }

  const {
    isActive: polling,
    pause: stop,
    resume,
  } = useIntervalFn(() => void refresh(), intervalMs, { immediate: false, immediateCallback: true })

  function start(): void {
    if (!polling.value) resume()
  }

  return { stats, rssHistory, cpuHistory, polling, error, refresh, start, stop }
}
