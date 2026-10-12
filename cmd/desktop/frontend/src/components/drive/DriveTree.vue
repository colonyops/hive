<script setup lang="ts">
// One space as a folder tree. Rows are the tree flattened through the fold
// state, so the list stays a plain v-for and keyboard walk; folders come first
// as the Go side sorts them.
import { computed, nextTick, ref, watch, type Component } from 'vue'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconFile from '~icons/lucide/file'
import IconFileCode from '~icons/lucide/file-code'
import IconFileText from '~icons/lucide/file-text'
import IconFolder from '~icons/lucide/folder'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconLayoutDashboard from '~icons/lucide/layout-dashboard'
import { useListKeyboardNav } from '../../composables/useListKeyboardNav'
import { useTreeExpansion } from '../../composables/useTreeExpansion'
import { ancestorPaths, entryLabel } from '../../lib/drive'
import { useDriveActivity } from '../../stores/useDriveActivity'
import type { EntryWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  space: string
  tree: EntryWire
  /** The document on screen, revealed and marked. */
  selected: string | null
  filter: string
  /** Tighter rows for the narrow pane. */
  compact?: boolean
  /** Derives `-tree` and `-row-<path>`. */
  testid: string
}>()
const emit = defineEmits<{ pick: [path: string]; menu: [entry: EntryWire, anchor: HTMLElement] }>()

const activity = useDriveActivity()

interface Row {
  entry: EntryWire
  depth: number
}

const query = computed(() => props.filter.trim().toLowerCase())

function matches(entry: EntryWire): boolean {
  const q = query.value
  return entry.name.toLowerCase().includes(q) || entry.title.toLowerCase().includes(q)
}

const allDirs = computed(() => {
  const dirs: EntryWire[] = []
  const walk = (entry: EntryWire) => {
    for (const child of entry.children ?? []) {
      if (child.kind === 'dir') {
        dirs.push(child)
        walk(child)
      }
    }
  }
  walk(props.tree)
  return dirs
})

const expansion = useTreeExpansion<EntryWire>('hive.drive.tree.expanded', {
  key: (entry) => `${props.space}\n${entry.path}`,
  defaultOpen: () => false,
  filtering: () => query.value !== '',
  nodes: () => allDirs.value,
})

// A folder stays while filtering when it or anything under it matches; a
// matching folder keeps its whole subtree.
const rows = computed(() => {
  const out: Row[] = []
  const walk = (parent: EntryWire, depth: number, keep: boolean) => {
    for (const child of parent.children ?? []) {
      const own = keep || !query.value || matches(child)
      if (child.kind === 'dir') {
        const start = out.length
        out.push({ entry: child, depth })
        if (expansion.expanded(child)) walk(child, depth + 1, own && query.value !== '')
        if (!own && out.length === start + 1) out.pop()
      } else if (own) out.push({ entry: child, depth })
    }
  }
  walk(props.tree, 0, false)
  return out
})

const active = ref(0)
const rowElements = ref<HTMLElement[]>([])
const nav = useListKeyboardNav({
  active,
  count: () => rows.value.length,
  wrap: false,
  homeEnd: () => true,
  row: (index) => rowElements.value[index],
})

watch(query, () => {
  active.value = 0
})

// The selected document is always reachable: its folders open and its row
// scrolls into view, without stealing the keyboard highlight's focus.
watch(
  () => [props.selected, props.tree] as const,
  ([selected]) => {
    if (!selected) return
    for (const path of ancestorPaths(selected)) {
      const dir = allDirs.value.find((entry) => entry.path === path)
      if (dir) expansion.unfold(dir)
    }
    void nextTick(() => {
      const index = rows.value.findIndex((row) => row.entry.path === selected)
      if (index < 0) return
      active.value = index
      rowElements.value[index]?.scrollIntoView?.({ block: 'nearest' })
    })
  },
  { immediate: true },
)

function kindIcon(entry: EntryWire): Component {
  if (entry.kind === 'dir') return expansion.expanded(entry) ? IconFolderOpen : IconFolder
  return (
    ({ markdown: IconFileText, html: IconFileCode, canvas: IconLayoutDashboard } as Record<string, Component>)[
      entry.kind
    ] ?? IconFile
  )
}

function activate(row: Row): void {
  if (row.entry.kind === 'dir') expansion.toggle(row.entry)
  else emit('pick', row.entry.path)
}

function parentIndex(index: number): number {
  const depth = rows.value[index]?.depth ?? 0
  for (let i = index - 1; i >= 0; i--) if (rows.value[i].depth < depth) return i
  return -1
}

/** Handles the walk keys; true when it took the key. A search field forwards its keys here. */
function onKeydown(event: KeyboardEvent): boolean {
  if (nav.onKeydown(event)) return true
  const row = rows.value[active.value]
  if (!row) return false
  if (event.key === 'Enter') activate(row)
  else if (event.key === 'ArrowRight' && row.entry.kind === 'dir' && !expansion.expanded(row.entry))
    expansion.toggle(row.entry)
  else if (event.key === 'ArrowLeft') {
    if (row.entry.kind === 'dir' && expansion.expanded(row.entry) && !query.value) expansion.toggle(row.entry)
    else {
      const parent = parentIndex(active.value)
      if (parent < 0) return false
      active.value = parent
    }
  } else return false
  event.preventDefault()
  return true
}

function openMenu(row: Row, event: MouseEvent): void {
  emit('menu', row.entry, event.currentTarget as HTMLElement)
}

defineExpose({ onKeydown })
</script>

<template>
  <div
    role="tree"
    :aria-label="`Documents in ${space}`"
    tabindex="0"
    class="outline-none"
    :data-testid="`${testid}-tree`"
    @keydown="onKeydown"
  >
    <div
      v-for="(row, index) in rows"
      :key="row.entry.path"
      ref="rowElements"
      role="treeitem"
      class="drive-row flex cursor-pointer items-center gap-1.5 pr-2 text-small"
      :class="[
        row.entry.path === selected ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip',
        index === active ? 'drive-row-active' : '',
      ]"
      :style="{ '--depth': row.depth }"
      :title="row.entry.path"
      :aria-level="row.depth + 1"
      :aria-expanded="row.entry.kind === 'dir' ? expansion.expanded(row.entry) : undefined"
      :aria-selected="row.entry.path === selected"
      :data-kind="row.entry.kind"
      :data-testid="`${testid}-row-${row.entry.path}`"
      @click="
        () => {
          active = index
          activate(row)
        }
      "
      @contextmenu.prevent="openMenu(row, $event)"
    >
      <IconChevronRight
        v-if="row.entry.kind === 'dir'"
        class="size-3 shrink-0 text-text-4 transition-transform"
        :class="expansion.expanded(row.entry) ? 'rotate-90' : ''"
        aria-hidden="true"
      />
      <span v-else class="size-3 shrink-0" aria-hidden="true" />
      <component
        :is="kindIcon(row.entry)"
        class="size-3.5 shrink-0"
        :class="row.entry.path === selected ? 'text-accent' : 'text-text-4'"
        aria-hidden="true"
      />
      <span class="min-w-0 flex-1 truncate" :class="compact ? '' : 'leading-snug'">{{ entryLabel(row.entry) }}</span>
      <span
        v-if="row.entry.kind !== 'dir' && activity.isUnopened(space, row.entry.path)"
        class="size-1.5 shrink-0 rounded-full bg-accent"
        :data-testid="`${testid}-row-${row.entry.path}-unseen`"
        aria-label="Written since you last opened it"
      />
    </div>
  </div>
</template>

<style scoped>
.drive-row {
  height: 26px;
  padding-left: calc(8px + var(--depth) * 14px);
}
.drive-row-active {
  box-shadow: inset 2px 0 0 var(--color-accent);
}
</style>
