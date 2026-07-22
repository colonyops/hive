import { computed, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { List, Record as RecordEvent } from '../../bindings/github.com/colonyops/hive/desktop/activityservice'
import type { Event as ActivityEvent, RecordInput } from '../../bindings/github.com/colonyops/hive/internal/desktop/activity/models'

// useActivity is a module singleton (like useFlowsSession): the activity log is
// app-global, so the titlebar's unseen indicator and the Activity view share
// one reactive list and one subscription. The backend pushes an
// "activity:appended" wake-up on every append (from any subsystem or a
// frontend Record call); we re-read the latest page on receipt.

const PAGE_SIZE = 200

const events = ref<ActivityEvent[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
// lastSeenId gates the titlebar's unseen dot. It is seeded to the newest id on
// first load so existing history doesn't read as unseen; only events that
// arrive afterwards count until the user opens the Activity view.
const lastSeenId = ref<number | null>(null)

let started = false

const latestId = computed(() => events.value[0]?.id ?? 0)
const unseenCount = computed(() => {
  if (lastSeenId.value === null) return 0
  return events.value.reduce((n, e) => (e.id > lastSeenId.value! ? n + 1 : n), 0)
})

async function load(): Promise<void> {
  loading.value = true
  try {
    const rows = await List(0, PAGE_SIZE)
    events.value = rows ?? []
    if (lastSeenId.value === null) lastSeenId.value = events.value[0]?.id ?? 0
    error.value = null
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

function start(): void {
  if (started) return
  started = true
  // The unsubscribe is intentionally never called: the singleton lives for the
  // app's lifetime, mirroring useFlowsSession's always-on subscriptions.
  Events.On('activity:appended', () => { void load() })
  void load()
}

// markSeen clears the unseen indicator; the Activity view calls it on open.
function markSeen(): void {
  lastSeenId.value = latestId.value
}

// record appends a frontend-originated event. Category defaults to "system"
// and severity to "info" on the backend when omitted. Failures are surfaced to
// the console rather than thrown, so a caller (e.g. a toast handler) is never
// derailed by an audit-log write.
async function record(input: Partial<RecordInput> & { title: string }): Promise<boolean> {
  try {
    await RecordEvent({
      category: input.category ?? '',
      severity: input.severity ?? '',
      title: input.title,
      body: input.body ?? '',
      source: input.source ?? '',
      metadata: input.metadata ?? null,
    })
    return true
  } catch (e) {
    console.error('recording activity event failed', e)
    return false
  }
}

export function useActivity() {
  start()
  return { events, loading, error, unseenCount, latestId, load, markSeen, record }
}
