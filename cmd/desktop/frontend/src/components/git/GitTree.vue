<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconClipboardCopy from '~icons/lucide/clipboard-copy'
import IconEllipsis from '~icons/lucide/ellipsis'
import IconExternalLink from '~icons/lucide/external-link'
import IconMessageSquare from '~icons/lucide/message-square'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useListKeyboardNav } from '@hive/ui/composables/useListKeyboardNav'
import type { MenuEntry } from '@hive/ui/types/menu'
import { documentIcon, folderIcon } from '../drive/driveIcon'
import { useTreeExpansion } from '../../composables/useTreeExpansion'
import { gitFileDir, gitFileName, gitRows, gitStatus, type GitLayout, type GitRow } from '../../lib/gitTree'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  files: ChangedFile[]
  layout: GitLayout
  selected: string | null
  /** Files marked viewed, which dim in the list. */
  viewed?: ReadonlySet<string>
  /** How many review comments each file holds. */
  commentCounts?: ReadonlyMap<string, number>
}>()
const emit = defineEmits<{ pick: [path: string]; 'open-editor': [] }>()

const folders = computed(() =>
  gitRows(props.files, props.layout, () => true).flatMap((row) => (row.kind === 'dir' ? [row.key] : [])),
)
const expansion = useTreeExpansion<string>('hive.git.tree.expanded', {
  key: (dir) => dir,
  defaultOpen: () => true,
  filtering: () => false,
  nodes: () => folders.value,
})
const anyFolderOpen = computed(() => folders.value.some((dir) => expansion.expanded(dir)))
defineExpose({ anyFolderOpen, setAllFolders: expansion.setAll })

const rows = computed(() => gitRows(props.files, props.layout, expansion.expanded))

function folderComments(dir: string): number {
  let total = 0
  for (const [path, count] of props.commentCounts ?? []) if (path.startsWith(`${dir}/`)) total += count
  return total
}

const rowElements = new Map<string, HTMLElement>()

function rowKey(row: GitRow): string {
  return row.kind === 'dir' ? `dir:${row.key}` : `file:${row.file.path}`
}

function setRowElement(row: GitRow, el: unknown): void {
  if (el instanceof HTMLElement) rowElements.set(rowKey(row), el)
  else rowElements.delete(rowKey(row))
}

const active = ref(0)
const nav = useListKeyboardNav({
  active,
  count: () => rows.value.length,
  wrap: false,
  homeEnd: () => true,
  row: (index) => {
    const row = rows.value[index]
    return row ? rowElements.get(rowKey(row)) : undefined
  },
})

watch(
  () => props.selected,
  (path) => {
    if (path) rowElements.get(`file:${path}`)?.scrollIntoView?.({ block: 'nearest' })
  },
)

function activate(row: GitRow): void {
  if (row.kind === 'dir') expansion.toggle(row.key)
  else emit('pick', row.file.path)
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
  if (event.key === 'Enter') activate(row)
  else if (event.key === 'ArrowRight' && row.kind === 'dir' && !expansion.expanded(row.key)) expansion.toggle(row.key)
  else if (event.key === 'ArrowLeft') {
    if (row.kind === 'dir' && expansion.expanded(row.key)) expansion.toggle(row.key)
    else {
      const parent = parentIndex(active.value)
      if (parent < 0) return
      active.value = parent
    }
  } else return
  event.preventDefault()
}

function rowTitle(file: ChangedFile): string {
  return file.oldPath ? `${file.oldPath} -> ${file.path}` : file.path
}

const menu = ref<{ file: ChangedFile; anchor: HTMLElement; toggle: HTMLElement } | null>(null)
const clipboard = useClipboard()

const menuEntries: MenuEntry[] = [
  { kind: 'action', id: 'copy-path', label: 'Copy path', icon: IconClipboardCopy, testid: 'changes-copy-path' },
  {
    kind: 'action',
    id: 'open-editor',
    label: 'Open in editor',
    icon: IconExternalLink,
    testid: 'changes-open-editor',
  },
]

function openMenu(file: ChangedFile, event: MouseEvent): void {
  const toggle = event.currentTarget as HTMLElement
  const anchor = rowElements.get(`file:${file.path}`)
  if (anchor) menu.value = { file, anchor, toggle }
}

function onMenuSelect(id: string): void {
  const file = menu.value?.file
  menu.value = null
  if (!file) return
  if (id === 'copy-path') void clipboard.copy(file.path)
  else if (id === 'open-editor') emit('open-editor')
}
</script>

<template>
  <div
    role="tree"
    aria-label="Changed files"
    tabindex="0"
    class="min-h-full pb-6 outline-none"
    data-testid="changes-tree"
    @keydown="onKeydown"
  >
    <template v-for="(row, index) in rows" :key="rowKey(row)">
      <div
        v-if="row.kind === 'dir'"
        :ref="(el) => setRowElement(row, el)"
        role="treeitem"
        class="git-row flex cursor-pointer items-center gap-1.5 pr-1 text-small text-text-2 hover:bg-chip"
        :class="index === active ? 'git-row-active' : ''"
        :style="{ '--depth': row.depth }"
        :title="row.key"
        :aria-level="row.depth + 1"
        :aria-expanded="expansion.expanded(row.key)"
        :data-testid="`changes-dir-${row.key}`"
        @click="
          () => {
            active = index
            activate(row)
          }
        "
      >
        <IconChevronRight
          class="size-3 shrink-0 text-text-4 transition-transform"
          :class="expansion.expanded(row.key) ? 'rotate-90' : ''"
          aria-hidden="true"
        />
        <component
          :is="folderIcon(gitFileName(row.label), expansion.expanded(row.key)).icon"
          class="size-3.5 shrink-0"
          :class="folderIcon(gitFileName(row.label), expansion.expanded(row.key)).color"
          aria-hidden="true"
        />
        <span class="min-w-0 flex-1 truncate">{{ row.label }}</span>
        <span
          v-if="!expansion.expanded(row.key) && folderComments(row.key)"
          class="flex shrink-0 items-center gap-0.5 pr-1 font-mono text-caption text-accent"
          :title="`${folderComments(row.key)} review comments`"
          :data-testid="`changes-row-${row.key}-comments`"
        >
          <IconMessageSquare class="size-3" aria-hidden="true" />
          {{ folderComments(row.key) }}
        </span>
      </div>
      <div
        v-else
        :ref="(el) => setRowElement(row, el)"
        role="treeitem"
        class="git-row flex cursor-pointer items-center gap-1.5 pr-1 text-small"
        :class="[
          row.file.path === selected ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip',
          index === active ? 'git-row-active' : '',
        ]"
        :style="{ '--depth': row.depth }"
        :title="rowTitle(row.file)"
        :aria-level="row.depth + 1"
        :aria-selected="row.file.path === selected"
        :data-testid="`changes-row-${row.file.path}`"
        @click="
          () => {
            active = index
            activate(row)
          }
        "
      >
        <span class="size-3 shrink-0" aria-hidden="true" />
        <component
          :is="documentIcon(gitFileName(row.file.path), '').icon"
          class="size-3.5 shrink-0"
          :class="documentIcon(gitFileName(row.file.path), '').color"
          aria-hidden="true"
        />
        <span class="min-w-0 truncate" :class="viewed?.has(row.file.path) ? 'text-text-4 line-through' : ''">{{
          gitFileName(row.file.path)
        }}</span>
        <span
          v-if="layout === 'flat' && gitFileDir(row.file.path)"
          class="min-w-0 truncate text-caption text-text-4"
          dir="rtl"
          >{{ gitFileDir(row.file.path) }}</span
        >
        <span class="min-w-0 flex-1" />
        <span
          v-if="commentCounts?.get(row.file.path)"
          class="flex shrink-0 items-center gap-0.5 font-mono text-caption text-accent"
          :title="`${commentCounts.get(row.file.path)} review comments`"
          :data-testid="`changes-row-${row.file.path}-comments`"
        >
          <IconMessageSquare class="size-3" aria-hidden="true" />
          {{ commentCounts.get(row.file.path) }}
        </span>
        <span v-if="!row.file.binary" class="git-row-reveal shrink-0 font-mono text-caption">
          <span v-if="row.file.additions" class="text-severity-success">+{{ row.file.additions }}</span>
          <span v-if="row.file.deletions" class="ml-1 text-severity-error">-{{ row.file.deletions }}</span>
        </span>
        <IconButton
          class="git-row-reveal shrink-0"
          size="sm"
          :label="`Actions for ${gitFileName(row.file.path)}`"
          :icon="IconEllipsis"
          :data-testid="`changes-row-${row.file.path}-menu`"
          @click.stop="openMenu(row.file, $event)"
        />
        <span
          class="w-3 shrink-0 text-center font-mono text-caption font-semibold"
          :class="gitStatus(row.file.status).color"
          :title="gitStatus(row.file.status).label"
          >{{ gitStatus(row.file.status).letter }}</span
        >
      </div>
    </template>
    <AppMenu
      v-if="menu"
      :entries="menuEntries"
      :anchor="menu.anchor"
      :ignore="[menu.toggle]"
      testid="changes-row-menu"
      @select="onMenuSelect"
      @close="menu = null"
    />
  </div>
</template>

<style scoped>
.git-row {
  height: 26px;
  padding-left: calc(8px + var(--depth) * 14px);
}
.git-row-active {
  box-shadow: inset 2px 0 0 var(--color-accent);
}
/* At rest a row is its name and letter, so a long list does not repeat the
   counts and the menu on every line. */
.git-row-reveal {
  opacity: 0;
}
.git-row:hover .git-row-reveal,
.git-row-active .git-row-reveal,
.git-row[aria-selected='true'] .git-row-reveal,
.git-row-reveal:focus-visible {
  opacity: 1;
}
</style>
