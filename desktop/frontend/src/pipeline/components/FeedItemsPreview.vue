<script setup lang="ts">
// Read-only preview of one feed's persisted items (Phase 6c) — proves a
// flow's `feed` node actually persisted something, without being the
// sidebar switchover (that's Phase 7's job, over useFeedState.ts/the
// existing feed_item read path this component deliberately does not touch).
//
// Mirrors driver.ts/usePipelineEditor.ts's injection posture: takes an
// injected client rather than importing PipelineService/bindings directly,
// so it's mountable in a test with a fake. FlowsView.vue is the one place
// that adapts the real PipelineService.FeedItems binding into this shape.
import { ref, watch } from 'vue'
import type { FeedItem } from '../types'

export interface FeedItemsClient {
  feedItems(feedId: string): Promise<FeedItem[] | null | undefined>
}

const props = defineProps<{
  /** The feed id a `feed` node's config.feed targets, or null when no feed node is selected. */
  feedId: string | null
  client: FeedItemsClient
}>()

const items = ref<FeedItem[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

async function load(feedId: string | null): Promise<void> {
  if (!feedId) {
    items.value = []
    error.value = null
    return
  }
  loading.value = true
  error.value = null
  try {
    items.value = (await props.client.feedItems(feedId)) ?? []
  } catch (err) {
    // A preview panel's load failure is not fatal to the rest of the flows
    // view — surface it inline, same posture as usePipelineEditor's
    // refreshNodeRuns (a background/secondary load, not a blocking one).
    console.warn('Unable to load feed items', feedId, err)
    error.value = err instanceof Error && err.message ? err.message : 'Could not load feed items.'
    items.value = []
  } finally {
    loading.value = false
  }
}

watch(() => props.feedId, (id) => { void load(id) }, { immediate: true })

// item.payload is an opaque json.RawMessage (typed `any` — see types.ts),
// deserialized by Wails into the plain feed.Item shape it actually holds
// (kind/repo/title/author/... — internal/desktop/feed/feed.go).
function title(item: FeedItem): string {
  return item.payload?.title || item.itemId
}
function repo(item: FeedItem): string {
  return item.payload?.repo ?? ''
}
function author(item: FeedItem): string {
  return item.payload?.author ?? ''
}
</script>

<template>
  <div class="flex min-h-0 flex-col" data-testid="feed-items-preview">
    <div class="shrink-0 border-b border-row px-3 py-2 text-[11px] font-semibold uppercase tracking-wide text-text-3">
      Feed preview
    </div>

    <div v-if="!feedId" class="px-3 py-4 text-[12px] text-text-4" data-testid="feed-preview-empty">
      Select a feed node to preview its persisted items.
    </div>
    <div v-else-if="loading" class="px-3 py-4 text-[12px] text-text-4" data-testid="feed-preview-loading">Loading…</div>
    <div v-else-if="error" class="px-3 py-4 text-[12px] text-severity-error" data-testid="feed-preview-error">{{ error }}</div>
    <div v-else-if="items.length === 0" class="px-3 py-4 text-[12px] text-text-4" data-testid="feed-preview-no-items">
      No items yet.
    </div>
    <ul v-else class="hive-scroll min-h-0 flex-1 overflow-y-auto" data-testid="feed-preview-list">
      <li
        v-for="item in items"
        :key="item.itemId"
        class="flex items-start gap-2 border-b border-row px-3 py-2"
        :data-testid="`feed-preview-item-${item.itemId}`"
      >
        <span class="mt-1.5 size-1.5 shrink-0 rounded-full" :class="item.unread ? 'bg-accent' : 'bg-transparent'" />
        <div class="min-w-0 flex-1">
          <div class="truncate text-[12.5px] text-text">{{ title(item) }}</div>
          <div class="truncate text-[10.5px] text-text-3">
            {{ repo(item) }}<span v-if="author(item)"> · {{ author(item) }}</span>
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>
