<script lang="ts">
export interface FenceTreeItem {
  /** Unique across the whole tree. */
  id: string
  children?: readonly FenceTreeItem[]
}
</script>

<script setup lang="ts" generic="T extends FenceTreeItem">
import { computed, reactive, useId } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'

const props = withDefaults(
  defineProps<{
    items: readonly T[]
    /** Rows are selectable and the tree takes focus, with ArrowUp, ArrowDown, ArrowLeft, ArrowRight and Enter. */
    selectable?: boolean
    /** Ids of rows that start folded. */
    collapsed?: readonly string[]
    label: string
    testid: string
  }>(),
  { selectable: false, collapsed: () => [] },
)

const selected = defineModel<string | null>('selected', { default: null })
const emit = defineEmits<{
  /** A click or Enter on a row. */
  activate: [id: string]
}>()

defineSlots<{
  row(props: { item: T; depth: number; expanded: boolean; hasChildren: boolean; selected: boolean }): unknown
}>()

const uid = useId()
// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- only the initial fold state
const folded = reactive(new Set<string>(props.collapsed))

interface Row {
  item: T
  depth: number
  parent: string | null
}

const rows = computed(() => {
  const out: Row[] = []
  const walk = (items: readonly T[], depth: number, parent: string | null): void => {
    for (const item of items) {
      out.push({ item, depth, parent })
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion -- vue-tsc needs it, typed lint reads T loosely
      if (item.children?.length && !folded.has(item.id)) walk(item.children as readonly T[], depth + 1, item.id)
    }
  }
  walk(props.items, 0, null)
  return out
})

const parents = computed(() => {
  const map = new Map<string, string | null>()
  const walk = (items: readonly FenceTreeItem[], parent: string | null): void => {
    for (const item of items) {
      map.set(item.id, parent)
      if (item.children) walk(item.children, item.id)
    }
  }
  walk(props.items, null)
  return map
})

function hasChildren(item: FenceTreeItem): boolean {
  return !!item.children?.length
}

function isDescendant(id: string, ancestor: string): boolean {
  for (let parent = parents.value.get(id); parent; parent = parents.value.get(parent)) {
    if (parent === ancestor) return true
  }
  return false
}

function fold(id: string): void {
  folded.add(id)
  if (selected.value && isDescendant(selected.value, id)) selected.value = id
}

function toggle(id: string): void {
  if (folded.has(id)) folded.delete(id)
  else fold(id)
}

function expandAll(): void {
  folded.clear()
}

function collapseAll(): void {
  const walk = (items: readonly FenceTreeItem[]): void => {
    for (const item of items) {
      if (!item.children?.length) continue
      folded.add(item.id)
      walk(item.children)
    }
  }
  walk(props.items)
  if (selected.value) selected.value = topFoldedAncestor(selected.value) ?? selected.value
}

function topFoldedAncestor(id: string): string | null {
  let top: string | null = null
  for (let parent = parents.value.get(id); parent; parent = parents.value.get(parent)) {
    if (folded.has(parent)) top = parent
  }
  return top
}

function rowId(id: string): string {
  return `${uid}-${id}`
}

function select(id: string): void {
  if (!props.selectable) return
  selected.value = id
}

function onRowClick(id: string): void {
  select(id)
  emit('activate', id)
}

function onKeydown(event: KeyboardEvent): void {
  if (!props.selectable || !rows.value.length) return
  const index = rows.value.findIndex((row) => row.item.id === selected.value)
  const current = index >= 0 ? rows.value[index] : null
  switch (event.key) {
    case 'ArrowDown':
      selected.value = rows.value[Math.min(rows.value.length - 1, index + 1)].item.id
      break
    case 'ArrowUp':
      selected.value = rows.value[Math.max(0, index < 0 ? 0 : index - 1)].item.id
      break
    case 'ArrowRight':
      if (current && hasChildren(current.item) && folded.has(current.item.id)) folded.delete(current.item.id)
      break
    case 'ArrowLeft':
      if (current && hasChildren(current.item) && !folded.has(current.item.id)) fold(current.item.id)
      else if (current?.parent) selected.value = current.parent
      break
    case 'Enter':
      if (current) emit('activate', current.item.id)
      break
    default:
      return
  }
  event.preventDefault()
}

defineExpose({ toggle, expandAll, collapseAll })
</script>

<template>
  <ul
    class="fence-tree"
    role="tree"
    :aria-label="label"
    :tabindex="selectable ? 0 : undefined"
    :aria-activedescendant="selectable && selected ? rowId(selected) : undefined"
    :data-testid="testid"
    @keydown="onKeydown"
  >
    <li
      v-for="row in rows"
      :id="rowId(row.item.id)"
      :key="row.item.id"
      class="fence-tree-row"
      :class="{ 'is-selected': selectable && selected === row.item.id, 'is-selectable': selectable }"
      role="treeitem"
      :aria-level="row.depth + 1"
      :aria-expanded="hasChildren(row.item) ? !folded.has(row.item.id) : undefined"
      :aria-selected="selectable ? selected === row.item.id : undefined"
      :data-testid="`${testid}-row-${row.item.id}`"
      @click="onRowClick(row.item.id)"
    >
      <span v-for="level in row.depth" :key="level" class="fence-tree-guide" aria-hidden="true" />
      <button
        v-if="hasChildren(row.item)"
        type="button"
        class="fence-tree-twisty"
        tabindex="-1"
        aria-label="Toggle"
        :data-testid="`${testid}-twisty-${row.item.id}`"
        @click.stop="toggle(row.item.id)"
      >
        <IconChevronDown class="fence-tree-chevron" :class="{ 'is-folded': folded.has(row.item.id) }" />
      </button>
      <span v-else class="fence-tree-twisty" aria-hidden="true" />
      <slot
        name="row"
        :item="row.item"
        :depth="row.depth"
        :expanded="!folded.has(row.item.id)"
        :has-children="hasChildren(row.item)"
        :selected="selectable && selected === row.item.id"
      />
    </li>
  </ul>
</template>

<style scoped>
.fence-tree {
  margin: 0;
  padding: 4px 0;
  list-style: none;
  outline: none;
}
.fence-tree:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: -2px;
}
.fence-tree-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 3px 12px 3px 8px;
}
.fence-tree-row:hover {
  background: var(--color-row-hover);
}
.fence-tree-row.is-selectable {
  cursor: pointer;
}
.fence-tree-row.is-selected {
  background: var(--color-chip);
  box-shadow: inset 2px 0 0 var(--color-accent);
}
.fence-tree-guide {
  flex: none;
  align-self: stretch;
  width: 18px;
  margin-left: 1px;
  border-left: 1px solid var(--color-border);
}
.fence-tree-twisty {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 14px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-3);
  cursor: pointer;
}
.fence-tree-chevron {
  width: 12px;
  height: 12px;
}
.fence-tree-chevron.is-folded {
  transform: rotate(-90deg);
}
</style>
