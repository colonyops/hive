<script setup lang="ts">
import { useListKeyboardNav } from '@hive/ui/composables/useListKeyboardNav'
import { computed, nextTick, ref, watch, type Component } from 'vue'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconFileCode from '~icons/lucide/file-code'
import IconFileText from '~icons/lucide/file-text'
import IconFolder from '~icons/lucide/folder'
import IconFolderOpen from '~icons/lucide/folder-open'
import { ancestorDirs, flattenTree, type TreeNode, type TreeRow } from '../lib/tree'
import { useTreeExpansion } from '../lib/treeExpansion'

const props = defineProps<{
  tree: TreeNode
  spaceKey: string
  /** The document on screen, revealed and marked. */
  selected: string | null
  /** While filtering, every folder is open so no match hides in a folded one. */
  filtering: boolean
}>()
const emit = defineEmits<{ open: [path: string]; 'open-folder': [path: string] }>()

const expansion = useTreeExpansion(() => props.spaceKey)
const isOpen = (dirPath: string) => props.filtering || expansion.isOpen(dirPath)
const rows = computed(() => flattenTree(props.tree, isOpen))

const active = ref(0)
const treeElement = ref<HTMLElement | null>(null)

const nav = useListKeyboardNav({
  active,
  count: () => rows.value.length,
  wrap: false,
  homeEnd: () => true,
  row: (index) => treeElement.value?.querySelector(`[data-row-index="${index}"]`),
})

watch(
  () => props.filtering,
  () => {
    active.value = 0
  },
)

// Revealed once per selection: revealing on every tree replacement would reopen a folder the user just folded.
watch(
  () => props.selected,
  (path) => {
    if (!path) return
    // The selection is a folder in the navigation tree, so it opens too.
    ;[...ancestorDirs(path), path].forEach(expansion.open)
    void nextTick(() => {
      const index = rows.value.findIndex((row) => row.node.path === path)
      if (index >= 0) active.value = index
    })
  },
  { immediate: true },
)

function icon(node: TreeNode): Component {
  if (node.kind === 'dir') return isOpen(node.path) ? IconFolderOpen : IconFolder
  return node.doc?.kind === 'html' ? IconFileCode : IconFileText
}

function activate(row: TreeRow): void {
  if (row.node.kind === 'dir') {
    expansion.open(row.node.path)
    emit('open-folder', row.node.path)
  } else emit('open', row.node.path)
}

function select(index: number, row: TreeRow): void {
  active.value = index
  activate(row)
}

function toggleRow(index: number, row: TreeRow): void {
  active.value = index
  expansion.toggle(row.node.path)
}

function parentIndex(index: number): number {
  const depth = rows.value[index]?.depth ?? 0
  for (let i = index - 1; i >= 0; i--) if (rows.value[i].depth < depth) return i
  return -1
}

function onKeydown(event: KeyboardEvent): void {
  if (nav.onKeydown(event)) return
  const row = rows.value[active.value]
  if (!row) return
  const folder = row.node.kind === 'dir'
  const open = folder && isOpen(row.node.path)
  if (event.key === 'Enter') activate(row)
  else if (event.key === 'ArrowRight' && folder && !open) expansion.toggle(row.node.path)
  else if (event.key === 'ArrowLeft') {
    if (open && !props.filtering) expansion.toggle(row.node.path)
    else if (parentIndex(active.value) >= 0) active.value = parentIndex(active.value)
    else return
  } else return
  event.preventDefault()
}
</script>

<template>
  <div
    v-if="rows.length"
    ref="treeElement"
    role="tree"
    tabindex="0"
    class="doc-tree pb-2 outline-none"
    aria-label="Documents"
    data-testid="doc-tree"
    @keydown="onKeydown"
  >
    <div
      v-for="(row, index) in rows"
      :key="row.node.path"
      role="treeitem"
      class="doc-row flex cursor-pointer items-center gap-1.5 pr-2 text-small"
      :class="[
        row.node.path === selected ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip',
        index === active ? 'doc-row-active' : '',
      ]"
      :style="{ '--depth': row.depth }"
      :title="row.node.kind === 'doc' ? row.node.path : row.node.path + '/'"
      :aria-level="row.depth + 1"
      :aria-expanded="row.node.kind === 'dir' ? isOpen(row.node.path) : undefined"
      :aria-selected="row.node.path === selected"
      :data-row-index="index"
      :data-kind="row.node.kind"
      :data-testid="`tree-row-${row.node.path}`"
      @click="select(index, row)"
    >
      <IconChevronRight
        v-if="row.node.kind === 'dir'"
        class="size-3 shrink-0 text-text-4 transition-transform"
        :class="isOpen(row.node.path) ? 'rotate-90' : ''"
        aria-hidden="true"
        @click.stop="toggleRow(index, row)"
      />
      <span v-else class="size-3 shrink-0" aria-hidden="true" />
      <component
        :is="icon(row.node)"
        class="size-3.5 shrink-0"
        :class="row.node.path === selected ? 'text-accent' : 'text-text-4'"
        aria-hidden="true"
      />
      <span class="min-w-0 flex-1 truncate">{{ row.node.label }}</span>
      <span
        v-if="row.node.doc?.shared"
        class="size-1.5 shrink-0 rounded-full bg-accent"
        title="Shared"
        aria-label="Shared"
      />
    </div>
  </div>
</template>

<style scoped>
.doc-row {
  height: 26px;
  padding-left: calc(8px + var(--depth) * 14px);
}
.doc-tree:focus-visible .doc-row-active {
  box-shadow: inset 2px 0 0 var(--color-accent);
}
</style>
