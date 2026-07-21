import { computed, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { ListActive } from '../../bindings/github.com/colonyops/hive/desktop/jobservice'
import type { Job } from '../../bindings/github.com/colonyops/hive/internal/desktop/jobs/models'

// useJobs is a module singleton: the titlebar chip and popover share one live
// list and one app-lifetime jobs:updated subscription. The backend owns the
// terminal-job linger window; this client only keeps re-reading while terminal
// rows remain so they eventually fall out of the authoritative result.

const TRAILING_READ_INTERVAL_MS = 500

const activeJobs = ref<Job[]>([])
const hasActive = computed(() => activeJobs.value.length > 0)

let started = false
let loadSequence = 0
let trailingTimer: ReturnType<typeof setTimeout> | undefined

function isTerminal(job: Job): boolean {
  return job.status === 'done' || job.status === 'failed'
}

function clearTrailingRead(): void {
  if (trailingTimer === undefined) return
  clearTimeout(trailingTimer)
  trailingTimer = undefined
}

function scheduleTrailingRead(): void {
  if (trailingTimer !== undefined) return
  trailingTimer = setTimeout(() => {
    trailingTimer = undefined
    void load()
  }, TRAILING_READ_INTERVAL_MS)
}

async function load(): Promise<void> {
  const sequence = ++loadSequence
  try {
    const rows = (await ListActive()) ?? []
    if (sequence !== loadSequence) return
    activeJobs.value = rows
    if (rows.some(isTerminal)) scheduleTrailingRead()
    else clearTrailingRead()
  } catch (error) {
    if (sequence === loadSequence) {
      console.warn('Unable to load active jobs', error)
      if (activeJobs.value.some(isTerminal)) scheduleTrailingRead()
    }
  }
}

function start(): void {
  if (started) return
  started = true
  // The singleton lives for the app lifetime, so this subscription is
  // intentionally never removed.
  Events.On('jobs:updated', () => { void load() })
  void load()
}

export function useJobs() {
  start()
  return { activeJobs, hasActive }
}
