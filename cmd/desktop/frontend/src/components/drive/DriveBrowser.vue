<script setup lang="ts">
// A space's tree under a filter. The pane opens it over its content and the
// full-page view keeps it in a sidebar, where the field searches every space
// instead of filtering this one.
import { watchDebounced } from '@vueuse/core'
import { computed, onMounted, ref, shallowRef } from 'vue'
import IconFolderPlus from '~icons/lucide/folder-plus'
import DriveEntryMenu from './DriveEntryMenu.vue'
import DriveTree from './DriveTree.vue'
import EmptyState from '../ui/EmptyState.vue'
import IconButton from '../ui/IconButton.vue'
import InlineError from '../ui/InlineError.vue'
import SearchField from '../ui/SearchField.vue'
import { errorText } from '../../lib/appError'
import { entryLabel } from '../../lib/drive'
import { useDrive } from '../../stores/useDrive'
import type {
  EntryWire,
  HitWire,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  space: string
  tree: EntryWire | null
  selected: string | null
  error?: string | null
  compact?: boolean
  /** The field searches every space and lists hits in place of the tree. */
  searchEverySpace?: boolean
  /** Focuses the field on mount, for a browser that opens on request. */
  autofocus?: boolean
  /**
   * Derives `-browse`, `-search`, `-new-folder-open`, `-tree`, `-row-<path>`, `-hit-<space>:<path>`,
   * `-browse-empty`, and the entry menu's ids (DriveEntryMenu).
   */
  testid: string
}>()
const emit = defineEmits<{
  pick: [path: string]
  'open-hit': [space: string, path: string]
  moved: [from: string, to: string]
  deleted: [path: string]
  escape: []
}>()

const { search } = useDrive()

const query = ref('')
const searchInput = ref<{ focus: () => void } | null>(null)
const treeRef = ref<{
  onKeydown: (event: KeyboardEvent) => boolean
  startRename: (path: string) => void
  reveal: (path: string) => void
} | null>(null)
const entryMenu = ref<{
  open: (entry: EntryWire, anchor: HTMLElement, toggle?: HTMLElement | null) => void
  newFolder: (dir: string) => void
} | null>(null)
onMounted(() => {
  if (props.autofocus) searchInput.value?.focus()
})

const searching = computed(() => !!props.searchEverySpace && query.value.trim() !== '')
const filter = computed(() => (props.searchEverySpace ? '' : query.value))

const hits = shallowRef<HitWire[]>([])
const hitsError = ref<string | null>(null)
let generation = 0
watchDebounced(
  () => (searching.value ? query.value.trim() : ''),
  async (q) => {
    const token = ++generation
    if (!q) {
      hits.value = []
      hitsError.value = null
      return
    }
    try {
      const found = await search(q)
      if (token !== generation) return
      hits.value = found
      hitsError.value = null
    } catch (err) {
      if (token !== generation) return
      hitsError.value = errorText(err, 'Could not search the drive.')
    }
  },
  { debounce: 120 },
)

const empty = computed(() => !searching.value && !!props.tree && !props.tree.children?.length)
const noMatches = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q || !props.tree) return false
  const matches = (entry: EntryWire) => entry.name.toLowerCase().includes(q) || entry.title.toLowerCase().includes(q)
  const walk = (entry: EntryWire): boolean => (entry.children ?? []).some((child) => matches(child) || walk(child))
  return !walk(props.tree)
})

// The field keeps focus while the arrows walk the tree under it, the command
// palette's shape.
function forwardKey(event: KeyboardEvent): void {
  if (!searching.value && ['ArrowDown', 'ArrowUp', 'Enter', 'F2'].includes(event.key)) treeRef.value?.onKeydown(event)
}

function openMenu(entry: EntryWire, anchor: HTMLElement, toggle: HTMLElement | null): void {
  entryMenu.value?.open(entry, anchor, toggle)
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" :data-testid="`${testid}-browse`">
    <div class="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
      <SearchField
        ref="searchInput"
        v-model="query"
        variant="bar"
        :placeholder="searchEverySpace ? 'Search every space' : 'Filter'"
        :aria-label="searchEverySpace ? 'Search every space' : 'Filter documents'"
        :testid="`${testid}-search`"
        @keydown="forwardKey"
        @escape="emit('escape')"
      />
      <IconButton
        v-if="tree"
        label="New folder"
        :icon="IconFolderPlus"
        :data-testid="`${testid}-new-folder-open`"
        @click="entryMenu?.newFolder('')"
      />
    </div>
    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto pb-2 pt-1">
      <template v-if="searching">
        <InlineError v-if="hitsError" variant="line" class="px-3 py-2 leading-relaxed" :message="hitsError" />
        <button
          v-for="hit in hits"
          :key="`${hit.space.key}:${hit.entry.path}`"
          type="button"
          class="flex w-full cursor-pointer flex-col items-start gap-0.5 px-3 py-1.5 text-left hover:bg-chip"
          :data-testid="`${testid}-hit-${hit.space.key}:${hit.entry.path}`"
          @click="emit('open-hit', hit.space.key, hit.entry.path)"
        >
          <span class="w-full truncate text-small text-text-2">{{ entryLabel(hit.entry) }}</span>
          <span class="w-full truncate font-mono text-micro text-text-4"
            >{{ hit.space.name }} · {{ hit.entry.path }}</span
          >
        </button>
        <EmptyState
          v-if="!hits.length && !hitsError"
          variant="inline"
          class="px-3 py-2"
          message="No documents match."
          :data-testid="`${testid}-browse-empty`"
        />
      </template>
      <template v-else>
        <InlineError v-if="error" variant="line" class="px-3 py-2 leading-relaxed" :message="error" />
        <DriveTree
          v-if="tree"
          ref="treeRef"
          :space="space"
          :tree="tree"
          :selected="selected"
          :filter="filter"
          :compact="compact"
          :testid="testid"
          @pick="emit('pick', $event)"
          @menu="openMenu"
          @moved="(from, to) => emit('moved', from, to)"
        />
        <EmptyState
          v-if="empty || noMatches"
          variant="inline"
          class="px-3 py-2"
          :message="empty ? 'No documents yet.' : 'No documents match.'"
          :data-testid="`${testid}-browse-empty`"
        />
      </template>
    </div>
    <DriveEntryMenu
      ref="entryMenu"
      :space="space"
      inline-rename
      :testid="testid"
      @rename="treeRef?.startRename($event.path)"
      @created="treeRef?.reveal($event)"
      @moved="(from, to) => emit('moved', from, to)"
      @deleted="emit('deleted', $event)"
    />
  </div>
</template>
