<script setup lang="ts">
// A workspace's canvases as a filterable list, grouped by how recently each
// was written. The pane opens it over its content and the full-page view keeps
// it in a sidebar.
import { computed, onMounted, ref } from 'vue'
import IconFileText from '~icons/lucide/file-text'
import SearchField from './ui/SearchField.vue'
import { relativeAge } from '../lib/age'
import type { WorkspaceCanvasMeta } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  metas: WorkspaceCanvasMeta[]
  /** The canvas on screen, marked in the list. */
  shown: string | null
  /** Focuses the filter on mount, for a list that opens on request. */
  autofocus?: boolean
  /** Derives `-browse`, `-search` and `-browse-<name>`. */
  testid: string
}>()
const emit = defineEmits<{ pick: [name: string]; escape: [] }>()

const search = ref('')
const searchInput = ref<{ focus: () => void } | null>(null)
onMounted(() => {
  if (props.autofocus) searchInput.value?.focus()
})

const filteredMetas = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return props.metas
  return props.metas.filter(
    (meta) => meta.name.toLowerCase().includes(query) || meta.title.toLowerCase().includes(query),
  )
})

const DAY_MS = 24 * 60 * 60 * 1000

function activityGroup(updatedAt: number, now: number): string {
  const age = now - updatedAt
  if (age < DAY_MS) return 'Today'
  if (age < 7 * DAY_MS) return 'Last week'
  if (age < 30 * DAY_MS) return 'Last 30 days'
  return 'Older'
}

// The listing arrives most-recently-updated first, so one sequential pass
// yields the groups already in display order.
const groupedMetas = computed(() => {
  const now = Date.now()
  const groups: Array<{ label: string; metas: WorkspaceCanvasMeta[] }> = []
  for (const meta of filteredMetas.value) {
    const label = activityGroup(meta.updatedAt, now)
    const last = groups[groups.length - 1]
    if (last?.label === label) last.metas.push(meta)
    else groups.push({ label, metas: [meta] })
  }
  return groups
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" :data-testid="`${testid}-browse`">
    <!-- Flush in the bar, the Code sidebar's filter shape: a boxed field in
         a list this narrow reads as chrome. -->
    <div class="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
      <SearchField
        ref="searchInput"
        v-model="search"
        variant="bar"
        aria-label="Filter canvases"
        :testid="`${testid}-search`"
        @escape="emit('escape')"
      />
    </div>
    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto pb-2 pt-1">
      <template v-for="group in groupedMetas" :key="group.label">
        <p class="px-3 pb-1 pt-2.5 text-micro font-medium uppercase tracking-wide text-text-4">
          {{ group.label }}
        </p>
        <div class="divide-y divide-border">
          <button
            v-for="meta in group.metas"
            :key="meta.name"
            type="button"
            class="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left"
            :class="meta.name === shown ? 'bg-selection' : 'hover:bg-chip'"
            :aria-current="meta.name === shown ? 'true' : undefined"
            :data-testid="`${testid}-browse-${meta.name}`"
            @click="emit('pick', meta.name)"
          >
            <IconFileText
              class="size-3.5 shrink-0"
              :class="meta.name === shown ? 'text-accent' : 'text-text-4'"
              aria-hidden="true"
            />
            <span
              class="min-w-0 flex-1 truncate text-small"
              :class="meta.name === shown ? 'text-text' : 'text-text-2'"
              >{{ meta.title || meta.name }}</span
            >
            <span class="shrink-0 font-mono text-micro text-text-4">{{ relativeAge(meta.updatedAt) }}</span>
          </button>
        </div>
      </template>
      <p v-if="!filteredMetas.length" class="px-3 py-2 text-xs leading-relaxed text-text-4">
        {{ metas.length ? 'No canvases match.' : 'No canvases yet.' }}
      </p>
    </div>
  </div>
</template>
