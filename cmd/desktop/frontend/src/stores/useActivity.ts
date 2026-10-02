import { computed, readonly, ref } from 'vue'
import {
  List,
  Record as RecordEvent,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/activityservice'
import type {
  Event as ActivityEvent,
  RecordInput,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/activity/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

const PAGE_SIZE = 200

export const useActivity = defineStore('activity', () => {
  const page = useResource(async () => (await List(0, PAGE_SIZE)) ?? [], {
    initial: [] as ActivityEvent[],
    errorFallback: 'Could not load activity.',
  })

  // lastSeenId gates the titlebar's unseen dot. It is seeded to the newest id
  // on the first successful load so existing history does not read as unseen;
  // only events that arrive afterwards count until the Activity view opens.
  const lastSeenId = ref<number | null>(null)
  const latestId = computed(() => page.data.value[0]?.id ?? 0)
  const unseenCount = computed(() => {
    const seen = lastSeenId.value
    if (seen === null) return 0
    return page.data.value.reduce((n, event) => (event.id > seen ? n + 1 : n), 0)
  })

  async function reload(): Promise<void> {
    await page.reload()
    if (lastSeenId.value === null && page.error.value === null) lastSeenId.value = latestId.value
  }

  function markSeen(): void {
    lastSeenId.value = latestId.value
  }

  // A frontend-originated event. The backend defaults category to "system" and
  // severity to "info". Failures go to the console rather than the caller, so
  // a toast handler is never derailed by an audit-log write.
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
    } catch (error) {
      console.error('recording activity event failed', error)
      return false
    }
  }

  useWailsEvent('activity:appended', () => {
    void reload()
  })
  void reload()

  return {
    events: readonly(page.data),
    loading: page.loading,
    error: page.error,
    unseenCount,
    reload,
    markSeen,
    record,
  }
})
