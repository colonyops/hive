<script setup lang="ts">
// One space as a folder tree. Rows are the tree flattened through the fold
// state, so the list stays a plain v-for and keyboard walk; folders come first
// as the Go side sorts them.
import { computed, nextTick, ref, watch, type Component } from 'vue'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconEllipsis from '~icons/lucide/ellipsis'
import IconFolder from '~icons/lucide/folder'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useListKeyboardNav } from '@hive/ui/composables/useListKeyboardNav'
import { useTreeExpansion } from '../../composables/useTreeExpansion'
import { documentIcon } from './documentIcon'
import { ancestorPaths, entryLabel, entryMatches } from '../../lib/drive'
import { useDrive } from '../../stores/useDrive'
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
const emit = defineEmits<{
  pick: [path: string]
  /** toggle is the row's own menu button when it opened the menu, null for a right click. */
  menu: [entry: EntryWire, anchor: HTMLElement, toggle: HTMLElement | null]
  /** A rename or a drop moved from to to. */
  moved: [from: string, to: string]
}>()

const activity = useDriveActivity()
const drive = useDrive()

interface Row {
  entry: EntryWire
  depth: number
}

const query = computed(() => props.filter.trim().toLowerCase())

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
      const own = keep || !query.value || entryMatches(child, query.value)
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
// By path, not a v-for ref array: Vue appends the rows a fold opens to the
// end of that array, so its order stops matching rows.
const rowElements = new Map<string, HTMLElement>()

function setRowElement(path: string, el: unknown): void {
  if (el instanceof HTMLElement) rowElements.set(path, el)
  else rowElements.delete(path)
}

function rowElement(index: number): HTMLElement | undefined {
  const row = rows.value[index]
  return row ? rowElements.get(row.entry.path) : undefined
}

const nav = useListKeyboardNav({
  active,
  count: () => rows.value.length,
  wrap: false,
  homeEnd: () => true,
  row: rowElement,
})

watch(query, () => {
  active.value = 0
})

function inTree(entry: EntryWire, path: string): boolean {
  return entry.path === path || (entry.children ?? []).some((child) => inTree(child, path))
}

// The selected document is revealed once per selection, as soon as it is in
// the tree. Revealing on every tree replacement (each drive:updated, refocus,
// or rename) would reopen a folder the user just folded.
let revealed: string | null = null
watch(
  () => [props.selected, props.tree] as const,
  ([selected], previous) => {
    if (previous && selected !== previous[0]) revealed = null
    if (!selected || selected === revealed || !inTree(props.tree, selected)) return
    revealed = selected
    reveal(selected)
  },
  { immediate: true },
)

/** Opens the folders above path and moves the keyboard highlight onto its row. */
function reveal(path: string): void {
  for (const ancestor of ancestorPaths(path)) {
    const dir = allDirs.value.find((entry) => entry.path === ancestor)
    if (dir) expansion.unfold(dir)
  }
  void nextTick(() => {
    const index = rows.value.findIndex((row) => row.entry.path === path)
    if (index < 0) return
    active.value = index
    rowElement(index)?.scrollIntoView?.({ block: 'nearest' })
  })
}

function kindIcon(entry: EntryWire): Component {
  if (entry.kind === 'dir') return expansion.expanded(entry) ? IconFolderOpen : IconFolder
  return documentIcon(entry.kind)
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
  if (renaming.value !== null) return false
  if (nav.onKeydown(event)) return true
  const row = rows.value[active.value]
  if (!row) return false
  if (event.key === 'Enter') activate(row)
  else if (event.key === 'F2') startRename(row.entry.path)
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
  emit('menu', row.entry, event.currentTarget as HTMLElement, null)
}

function openRowMenu(index: number, event: MouseEvent): void {
  const row = rows.value[index]
  const anchor = rowElement(index)
  if (row && anchor) emit('menu', row.entry, anchor, event.currentTarget as HTMLElement)
}

function openRootMenu(event: MouseEvent): void {
  emit('menu', props.tree, event.currentTarget as HTMLElement, null)
}

const renaming = ref<string | null>(null)
const draft = ref('')
const renameError = ref<string | null>(null)
const renameInput = ref<HTMLInputElement[]>([])
let committing = false

function stemLength(name: string): number {
  if (name.endsWith('.canvas.json')) return name.length - '.canvas.json'.length
  const dot = name.lastIndexOf('.')
  return dot > 0 ? dot : name.length
}

/** Replaces the row's name with a field; Enter or leaving it commits, Escape cancels. */
function startRename(path: string): void {
  const row = rows.value.find((candidate) => candidate.entry.path === path)
  if (!row) return
  renaming.value = path
  draft.value = row.entry.name
  renameError.value = null
  void nextTick(() => {
    const input = renameInput.value[0]
    input?.focus()
    // The stem only, as Finder does: a document keeps its kind's extension.
    input?.setSelectionRange(0, row.entry.kind === 'dir' ? row.entry.name.length : stemLength(row.entry.name))
  })
}

function cancelRename(): void {
  renaming.value = null
  renameError.value = null
}

async function commitRename(): Promise<void> {
  const path = renaming.value
  if (path === null || committing) return
  const name = draft.value.trim()
  const current = path.split('/').pop() ?? path
  if (!name || name === current) {
    cancelRename()
    return
  }
  committing = true
  const { result, error } = await drive.rename(props.space, path, name)
  committing = false
  if (renaming.value !== path) return
  renameError.value = error
  if (!result) return
  renaming.value = null
  if (result.path !== path) emit('moved', path, result.path)
}

function onRenameKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter') {
    event.preventDefault()
    void commitRename()
  } else if (event.key === 'Escape') {
    event.preventDefault()
    cancelRename()
  }
}

// Only a drag that started in this tree is accepted, which keeps a move
// inside one space. A document row stands for its own folder, as in a file
// manager, so the rows at the top level are part of the root's drop area.
const dragging = ref<string | null>(null)
const dropTarget = ref<string | null>(null)
const moveError = ref<string | null>(null)

function parentOf(path: string): string {
  const slash = path.lastIndexOf('/')
  return slash < 0 ? '' : path.slice(0, slash)
}

function onDragStart(row: Row, event: DragEvent): void {
  if (row.entry.kind === 'dir' || renaming.value !== null) {
    event.preventDefault()
    return
  }
  dragging.value = row.entry.path
  moveError.value = null
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', row.entry.path)
  }
}

function onDragOver(event: DragEvent, row: Row | null): void {
  if (dragging.value === null) return
  event.preventDefault()
  event.stopPropagation()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  dropTarget.value = row === null ? '' : row.entry.kind === 'dir' ? row.entry.path : parentOf(row.entry.path)
}

function onDragLeave(event: DragEvent): void {
  const next = event.relatedTarget as Node | null
  if (!next || !(event.currentTarget as HTMLElement).contains(next)) dropTarget.value = null
}

function endDrag(): void {
  dragging.value = null
  dropTarget.value = null
}

async function onDrop(event: DragEvent): Promise<void> {
  const from = dragging.value
  const dir = dropTarget.value
  endDrag()
  if (from === null || dir === null) return
  event.preventDefault()
  if (parentOf(from) === dir) return
  const { result, error } = await drive.move(props.space, from, dir)
  moveError.value = error
  if (result) emit('moved', from, result.path)
}

defineExpose({ onKeydown, startRename, reveal })
</script>

<template>
  <div
    role="tree"
    :aria-label="`Documents in ${space}`"
    tabindex="0"
    class="min-h-full pb-6 outline-none"
    :class="dropTarget === '' ? 'drive-drop-root' : ''"
    :data-testid="`${testid}-tree`"
    :data-drop="dropTarget === '' ? 'true' : undefined"
    @keydown="onKeydown"
    @contextmenu.self.prevent="openRootMenu"
    @dragover="onDragOver($event, null)"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <InlineError
      v-if="moveError"
      variant="line"
      class="px-3 py-1 leading-relaxed"
      :message="moveError"
      :testid="`${testid}-move-error`"
    />
    <div
      v-for="(row, index) in rows"
      :key="row.entry.path"
      :ref="(el) => setRowElement(row.entry.path, el)"
      role="treeitem"
      class="drive-row flex cursor-pointer items-center gap-1.5 pr-1 text-small"
      :class="[
        row.entry.path === selected ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip',
        index === active ? 'drive-row-active' : '',
        row.entry.kind === 'dir' && dropTarget === row.entry.path ? 'drive-row-drop' : '',
      ]"
      :style="{ '--depth': row.depth }"
      :title="row.entry.path"
      :aria-level="row.depth + 1"
      :aria-expanded="row.entry.kind === 'dir' ? expansion.expanded(row.entry) : undefined"
      :aria-selected="row.entry.path === selected"
      :data-kind="row.entry.kind"
      :data-drop="row.entry.kind === 'dir' && dropTarget === row.entry.path ? 'true' : undefined"
      :data-testid="`${testid}-row-${row.entry.path}`"
      :draggable="row.entry.kind !== 'dir' && renaming === null"
      @click="
        () => {
          active = index
          activate(row)
        }
      "
      @contextmenu.prevent="openMenu(row, $event)"
      @dragstart="onDragStart(row, $event)"
      @dragover="onDragOver($event, row)"
      @dragend="endDrag"
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
      <template v-if="renaming === row.entry.path">
        <input
          ref="renameInput"
          v-model="draft"
          type="text"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :aria-label="`Rename ${row.entry.name}`"
          :aria-invalid="renameError ? 'true' : undefined"
          class="drive-rename min-w-0 flex-1 rounded-sm bg-app px-1 text-small text-text outline-none"
          :data-testid="`${testid}-rename-input`"
          @click.stop
          @keydown.stop="onRenameKeydown"
          @blur="commitRename"
        />
      </template>
      <span v-else class="min-w-0 flex-1 truncate" :class="compact ? '' : 'leading-snug'">{{
        entryLabel(row.entry)
      }}</span>
      <span
        v-if="row.entry.kind !== 'dir' && activity.isUnopened(space, row.entry.path)"
        class="size-1.5 shrink-0 rounded-full bg-accent"
        :data-testid="`${testid}-row-${row.entry.path}-unseen`"
        aria-label="Written since you last opened it"
      />
      <IconButton
        v-if="renaming !== row.entry.path"
        class="drive-row-more shrink-0"
        size="sm"
        :label="`Actions for ${row.entry.name}`"
        :icon="IconEllipsis"
        :data-testid="`${testid}-row-${row.entry.path}-menu`"
        @click.stop="openRowMenu(index, $event)"
      />
    </div>
    <InlineError
      v-if="renameError"
      variant="line"
      class="px-3 py-1 leading-relaxed"
      :message="renameError"
      :testid="`${testid}-rename-error`"
    />
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
.drive-row-drop {
  background: var(--color-accent-tint);
  box-shadow: inset 0 0 0 1px var(--color-accent);
}
.drive-drop-root {
  box-shadow: inset 0 0 0 1px var(--color-accent);
}
.drive-rename {
  box-shadow: 0 0 0 1px var(--color-accent);
}
/* The row's menu button shows where the pointer or the keyboard is, so a
   long list does not repeat it on every line. */
.drive-row-more {
  opacity: 0;
}
.drive-row:hover .drive-row-more,
.drive-row-active .drive-row-more,
.drive-row[aria-selected='true'] .drive-row-more,
.drive-row-more:focus-visible {
  opacity: 1;
}
</style>
