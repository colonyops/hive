import { computed, onScopeDispose, readonly } from 'vue'
import { ListActive } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/jobservice'
import type { Job } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/jobs/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

// The backend owns the terminal-job linger window; this store only keeps
// re-reading while terminal rows remain so they fall out of the authoritative
// result.
const TRAILING_READ_INTERVAL_MS = 500

function isTerminal(job: Job): boolean {
  return job.status === 'done' || job.status === 'failed'
}

export const useJobs = defineStore('jobs', () => {
  const jobs = useResource(async () => (await ListActive()) ?? [], {
    initial: [] as Job[],
    errorFallback: 'Could not load active jobs.',
  })
  const hasActive = computed(() => jobs.data.value.length > 0)

  let trailingTimer: ReturnType<typeof setTimeout> | undefined

  function clearTrailingRead(): void {
    if (trailingTimer === undefined) return
    clearTimeout(trailingTimer)
    trailingTimer = undefined
  }

  function scheduleTrailingRead(): void {
    if (trailingTimer !== undefined) return
    trailingTimer = setTimeout(() => {
      trailingTimer = undefined
      void reload()
    }, TRAILING_READ_INTERVAL_MS)
  }

  async function reload(): Promise<void> {
    await jobs.reload()
    if (jobs.error.value) console.warn('Unable to load active jobs:', jobs.error.value)
    if (jobs.data.value.some(isTerminal)) scheduleTrailingRead()
    else clearTrailingRead()
  }

  useWailsEvent('jobs:updated', () => {
    void reload()
  })
  onScopeDispose(clearTrailingRead)
  void reload()

  return { activeJobs: readonly(jobs.data), hasActive, reload }
})
