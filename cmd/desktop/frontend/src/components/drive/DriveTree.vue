<script setup lang="ts">
// One space as a @pierre/trees tree. The tree applies a rename or a drop to
// itself first; the server's answer then either arrives as a new tree or, on
// a refusal, resync puts the last one back.
import type { FileTreeRowDecoration, FileTreeRowDecorationContext } from '@pierre/trees'
import { computed, ref, watch } from 'vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import PathTree from '../tree/PathTree.vue'
import { directory, usePathTree } from '../tree/usePathTree'
import { useDrive } from '../../stores/useDrive'
import { useDriveActivity } from '../../stores/useDriveActivity'
import type { EntryWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  space: string
  tree: EntryWire
  /** The document on screen, revealed and marked. */
  selected: string | null
  filter: string
  /** Derives `-tree`, `-move-error`, and `-rename-error`. */
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

const stripSlash = (path: string) => path.replace(/\/$/, '')
const parentOf = (path: string) => {
  const slash = path.lastIndexOf('/')
  return slash < 0 ? '' : path.slice(0, slash)
}

// Folders go in as "path/" so an empty one still shows.
const indexed = computed(() => {
  const entries = new Map<string, EntryWire>()
  const paths: string[] = []
  const walk = (entry: EntryWire) => {
    for (const child of entry.children ?? []) {
      entries.set(child.path, child)
      if (child.kind === 'dir') {
        paths.push(`${child.path}/`)
        walk(child)
      } else paths.push(child.path)
    }
  }
  walk(props.tree)
  return { entries, paths }
})
const entryAt = (treePath: string) => indexed.value.entries.get(stripSlash(treePath))

const unseen = computed(() =>
  [...indexed.value.entries.values()].filter(
    (entry) => entry.kind !== 'dir' && activity.isUnopened(props.space, entry.path),
  ),
)

function decorate({ row }: FileTreeRowDecorationContext): FileTreeRowDecoration | null {
  if (row.kind !== 'file' || !activity.isUnopened(props.space, row.path)) return null
  return { text: '●', title: 'Written since you last opened it', parts: [{ text: '●', color: 'var(--color-accent)' }] }
}

const renameError = ref<string | null>(null)
const moveError = ref<string | null>(null)

const tree = usePathTree({
  paths: () => indexed.value.paths,
  storageKey: 'hive.drive.tree.expanded',
  storagePrefix: () => props.space,
  defaultOpen: false,
  fileTreeSearchMode: 'hide-non-matches',
  renderRowDecoration: decorate,
  onSelectionChange(paths) {
    const entry = paths.length === 1 ? entryAt(paths[0]) : undefined
    if (entry && entry.kind !== 'dir') {
      if (entry.path !== props.selected) emit('pick', entry.path)
    } else tree.selectOnly(props.selected)
  },
  renaming: {
    onRename: (event) => void rename(event.sourcePath, event.destinationPath),
    onError: (error) => (renameError.value = error),
  },
  // A document row stands for its own folder, as in a file manager, and only
  // documents drag, which keeps a move inside one space.
  dragAndDrop: {
    canDrag: (paths) => paths.every((path) => entryAt(path)?.kind !== 'dir'),
    onDropComplete: (event) => void drop(event.draggedPaths, event.target.directoryPath),
    onDropError: (error) => (moveError.value = error),
  },
  composition: {
    contextMenu: {
      enabled: true,
      triggerMode: 'both',
      onOpen(item, context) {
        context.close({ restoreFocus: false })
        const entry = entryAt(item.path)
        if (entry) emit('menu', entry, tree.rowElement(item.path) ?? context.anchorElement, context.anchorElement)
      },
    },
  },
})

async function rename(sourcePath: string, destinationPath: string): Promise<void> {
  const from = stripSlash(sourcePath)
  const name = stripSlash(destinationPath).split('/').pop() ?? ''
  renameError.value = null
  const { result, error } = await drive.rename(props.space, from, name)
  renameError.value = error
  if (!result) tree.resync()
  else if (result.path !== from) emit('moved', from, result.path)
}

async function drop(dragged: readonly string[], directoryPath: string | null): Promise<void> {
  moveError.value = null
  const dir = stripSlash(directoryPath ?? '')
  for (const path of dragged) {
    const from = stripSlash(path)
    if (parentOf(from) === dir) continue
    const { result, error } = await drive.move(props.space, from, dir)
    moveError.value = error
    if (!result) {
      tree.resync()
      return
    }
    emit('moved', from, result.path)
  }
}

watch(unseen, () => tree.redecorate())
watch(
  () => props.filter.trim(),
  (query) => tree.model.setSearch(query || null),
  { immediate: true },
)

// The selected document is revealed once per selection, as soon as it is in
// the tree. Revealing on every tree replacement (each drive:updated, refocus,
// or rename) would reopen a folder the user just folded.
let revealed: string | null = null
watch(
  () => [props.selected, indexed.value] as const,
  ([selected], previous) => {
    if (previous && selected !== previous[0]) revealed = null
    if (!selected) tree.selectOnly(null)
    if (!selected || selected === revealed || !indexed.value.entries.has(selected)) return
    revealed = selected
    tree.reveal(selected)
  },
  { immediate: true },
)

function activateFocused(): boolean {
  const path = tree.model.getFocusedPath()
  const entry = path ? entryAt(path) : undefined
  if (!path || !entry) return false
  const dir = directory(tree.model, path)
  if (dir) dir.toggle()
  else emit('pick', entry.path)
  return true
}

/** Walks the tree for a search field that keeps focus; true when it took the key. */
function onKeydown(event: KeyboardEvent): boolean {
  const { model } = tree
  if (event.key === 'ArrowDown') model.focusNextItem()
  else if (event.key === 'ArrowUp') model.focusPreviousItem()
  else if (event.key === 'Home') model.focusFirstItem()
  else if (event.key === 'End') model.focusLastItem()
  else if (event.key === 'Enter') {
    if (!activateFocused()) return false
  } else return false
  event.preventDefault()
  return true
}

function onHostKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && activateFocused()) event.preventDefault()
}

function onHostContextmenu(event: MouseEvent): void {
  const onRow = event
    .composedPath()
    .some((node) => node instanceof HTMLElement && node.getAttribute('role') === 'treeitem')
  if (onRow) return
  event.preventDefault()
  emit('menu', props.tree, event.currentTarget as HTMLElement, null)
}

defineExpose({
  onKeydown,
  startRename: (path: string) => tree.model.startRenaming(entryAt(path)?.kind === 'dir' ? `${path}/` : path),
  reveal: (path: string) => tree.reveal(path),
})
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col"
    :data-testid="`${testid}-tree`"
    @keydown="onHostKeydown"
    @contextmenu="onHostContextmenu"
  >
    <InlineError
      v-if="moveError"
      variant="line"
      class="px-3 py-1 leading-relaxed"
      :message="moveError"
      :testid="`${testid}-move-error`"
    />
    <PathTree :model="tree.model" />
    <InlineError
      v-if="renameError"
      variant="line"
      class="px-3 py-1 leading-relaxed"
      :message="renameError"
      :testid="`${testid}-rename-error`"
    />
  </div>
</template>
