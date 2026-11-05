<script setup lang="ts">
import type { FileTreeRowDecoration, FileTreeRowDecorationContext, GitStatus } from '@pierre/trees'
import { computed, shallowRef, watch } from 'vue'
import IconClipboardCopy from '~icons/lucide/clipboard-copy'
import IconExternalLink from '~icons/lucide/external-link'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import type { MenuEntry } from '@hive/ui/types/menu'
import PathTree from '../tree/PathTree.vue'
import { usePathTree } from '../tree/usePathTree'
import { gitOrder, gitTreePath } from '../../lib/gitTree'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  files: ChangedFile[]
  selected: string | null
  /** Files marked viewed, which dim in the list. */
  viewed?: ReadonlySet<string>
  /** How many review comments each file holds. */
  commentCounts?: ReadonlyMap<string, number>
}>()
const emit = defineEmits<{ pick: [path: string]; 'open-editor': [] }>()

const byTreePath = computed(() => new Map(props.files.map((file) => [gitTreePath(file.path), file])))
const treePaths = computed(() => gitOrder(props.files).map((file) => gitTreePath(file.path)))

const GIT_STATUSES = new Set<string>(['added', 'deleted', 'modified', 'renamed', 'untracked'])
const gitStatus = computed(() =>
  props.files.flatMap((file) =>
    GIT_STATUSES.has(file.status) ? [{ path: gitTreePath(file.path), status: file.status as GitStatus }] : [],
  ),
)

function folderComments(dir: string): number {
  let total = 0
  for (const [path, count] of props.commentCounts ?? []) if (gitTreePath(path).startsWith(dir)) total += count
  return total
}

function decorate({ row }: FileTreeRowDecorationContext): FileTreeRowDecoration | null {
  if (row.kind === 'directory') {
    const total = row.isExpanded ? 0 : folderComments(row.path)
    return total
      ? {
          text: String(total),
          title: `${total} review comments`,
          parts: [{ text: `${total}`, color: 'var(--color-accent)' }],
        }
      : null
  }
  const file = byTreePath.value.get(row.path)
  if (!file) return null
  const comments = props.commentCounts?.get(file.path) ?? 0
  const parts = [
    ...(comments ? [{ text: `${comments} `, color: 'var(--color-accent)' }] : []),
    ...(!file.binary && file.additions ? [{ text: `+${file.additions}`, color: 'var(--color-severity-success)' }] : []),
    ...(!file.binary && file.deletions ? [{ text: ` -${file.deletions}`, color: 'var(--color-severity-error)' }] : []),
  ]
  if (!parts.length) return null
  const title = [comments ? `${comments} review comments` : '', file.oldPath ? `${file.oldPath} -> ${file.path}` : '']
    .filter(Boolean)
    .join(' · ')
  return { text: parts.map((part) => part.text).join(''), title: title || undefined, parts }
}

const menu = shallowRef<{ file: ChangedFile; anchor: HTMLElement; toggle: HTMLElement } | null>(null)

const tree = usePathTree({
  paths: treePaths,
  storageKey: 'hive.git.tree.expanded',
  defaultOpen: true,
  gitStatus,
  renderRowDecoration: decorate,
  onSelectionChange(paths) {
    const file = paths.length === 1 ? byTreePath.value.get(paths[0]) : undefined
    if (file && file.path !== props.selected) emit('pick', file.path)
    else if (!file) tree.selectOnly(props.selected && gitTreePath(props.selected))
  },
  composition: {
    contextMenu: {
      enabled: true,
      triggerMode: 'both',
      onOpen(item, context) {
        context.close({ restoreFocus: false })
        const file = byTreePath.value.get(item.path)
        if (!file) return
        menu.value = {
          file,
          anchor: tree.rowElement(item.path) ?? context.anchorElement,
          toggle: context.anchorElement,
        }
      },
    },
  },
})
defineExpose({ anyFolderOpen: tree.anyFolderOpen, setAllFolders: tree.setAllFolders })

watch(
  () => [props.commentCounts, props.viewed] as const,
  () => tree.redecorate(),
)
watch(
  () => props.selected,
  (path) => {
    if (path) tree.reveal(gitTreePath(path))
    else tree.selectOnly(null)
  },
  { immediate: true },
)

// Picking the file on screen again turns off showing it alone, and a click on
// a row that is already selected changes no selection, so the tree reports nothing.
function onClick(event: MouseEvent): void {
  const row = event
    .composedPath()
    .find((node): node is HTMLElement => node instanceof HTMLElement && node.getAttribute('role') === 'treeitem')
  const file = row?.dataset.itemPath ? byTreePath.value.get(row.dataset.itemPath) : undefined
  if (file && file.path === props.selected) emit('pick', file.path)
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Enter') return
  const path = tree.model.getFocusedPath()
  const file = path ? byTreePath.value.get(path) : undefined
  if (!file) return
  event.preventDefault()
  emit('pick', file.path)
}

const viewedCss = computed(() =>
  [...(props.viewed ?? [])]
    .map(
      (path) =>
        `[data-item-path=${JSON.stringify(gitTreePath(path))}] [data-item-section="content"] { color: var(--color-text-4); text-decoration: line-through; }`,
    )
    .join('\n'),
)
// Every folder in a list of changes holds a change, so the tree's
// changed-descendant dot would mark them all.
const rowCss = computed(
  () => `[data-item-type="folder"] [data-item-section="git"] { visibility: hidden; }\n${viewedCss.value}`,
)

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

function onMenuSelect(id: string): void {
  const file = menu.value?.file
  menu.value = null
  if (!file) return
  if (id === 'copy-path') void clipboard.copy(file.path)
  else if (id === 'open-editor') emit('open-editor')
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="changes-tree" @click="onClick" @keydown="onKeydown">
    <PathTree :model="tree.model" :row-css="rowCss" />
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
