<script setup lang="ts">
// Read-only preview of inbox items claimed by one feed node. It uses the same
// claim-aware read path as the sidebar, scoped to the selected feed node.
//
// Mirrors driver.ts/usePipelineEditor.ts's injection posture: takes an
// injected client rather than importing PipelineService/bindings directly,
// so it's mountable in a test with a fake. FlowsView.vue adapts the real
// PipelineService.ListInboxItemsByFeed binding into this shape.
import { ref, watch } from 'vue'
import type { InboxItem } from '../../types/feed'
import { githubPayload } from '../../lib/feedPresentation'

export interface FeedItemsClient {
  feedItems(feedId: string): Promise<Array<InboxItem & { archivedAt?: number | null; archivedActor?: string | null; archivedReason?: string | null; sourceState?: string | null }> | null | undefined>
}

const props = defineProps<{
  /** The feed id a `feed` node's config.feed targets, or null when no feed node is selected. */
  feedId: string | null
  client: FeedItemsClient
}>()

const items = ref<InboxItem[]>([])
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

// item.payload is an opaque source payload, decoded here only through the
// GitHub presentation adapter for title/repository/author display.
function title(item: InboxItem): string { return item.title }
function repo(item: InboxItem): string { return githubPayload(item).repo }
function author(item: InboxItem): string { return githubPayload(item).author }
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
        :key="item.id"
        class="flex items-start gap-2 border-b border-row px-3 py-2"
        :data-testid="`feed-preview-item-${item.id}`"
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
