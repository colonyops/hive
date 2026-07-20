<script setup lang="ts">
import { computed, ref } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconFolder from '~icons/lucide/folder'
import IconFolderPlus from '~icons/lucide/folder-plus'
import IconGitBranch from '~icons/lucide/git-branch'
import IconList from '~icons/lucide/list'
import IconPencil from '~icons/lucide/pencil'
import IconRss from '~icons/lucide/rss'
import IconSettings from '~icons/lucide/settings'
import IconTrash from '~icons/lucide/trash-2'
import IconWorkflow from '~icons/lucide/workflow'
import PanelResizeHandle from './PanelResizeHandle.vue'
import SidebarFeedRow from './SidebarFeedRow.vue'
import { useResizablePanel } from '../composables/useResizablePanel'
import { applyMove, SIDEBAR_DRAG_MIME, type DragRef, type DropTarget } from '../lib/feedTree'
import type { FeedFolder, FeedSummary, FeedTree, Profile, SidebarSelection } from '../types/feed'

const props = defineProps<{ profile: Profile; selection: SidebarSelection; flowsDirty?: boolean }>()
const emit = defineEmits<{
  select: [sel: SidebarSelection]
  'open-flows': []
  'open-settings': []
  'reveal-in-flow': [feedId: string]
  reorder: [tree: FeedTree]
}>()

const { size, startResize, step } = useResizablePanel({
  storageKey: 'hive.panel.sidebar',
  defaultSize: 250,
  min: 190,
  max: 480,
  edge: 'right',
})

// The rendered tree. A profile with no saved sidebar layout falls back to a
// flat list of its feeds, so nothing changes until the user groups/reorders.
const tree = computed<FeedTree>(() =>
  props.profile.tree ?? props.profile.feeds.map((f) => ({ kind: 'feed', feed: f })),
)

function allSelected(): boolean {
  return props.selection.type === 'all'
}
function feedSelected(feedId: string): boolean {
  return props.selection.type === 'feed' && props.selection.feedId === feedId
}

function feedRef(feed: FeedSummary): DragRef {
  return { kind: 'feed', id: feed.id }
}
function folderDragRef(folder: FeedFolder): DragRef {
  return { kind: 'folder', id: folder.id }
}

function folderNew(folder: FeedFolder): number {
  return folder.feeds.reduce((sum, f) => sum + f.newCount, 0)
}
function folderTotal(folder: FeedFolder): number {
  return folder.feeds.reduce((sum, f) => sum + f.count, 0)
}

// ── drag-and-drop ─────────────────────────────────────────────────────────
// Same native HTML5 DnD approach as the flow palette; the dragged item is
// tracked in a local ref (dataTransfer can't be read during dragover), and a
// drop target drives the insertion indicators.
const dragging = ref<DragRef | null>(null)
const dropTarget = ref<DropTarget | null>(null)

function onDragStart(e: DragEvent, ref: DragRef): void {
  dragging.value = ref
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(SIDEBAR_DRAG_MIME, ref.id)
  }
}

function edge(e: DragEvent): 'before' | 'after' {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  return e.clientY < rect.top + rect.height / 2 ? 'before' : 'after'
}

function allowDrop(e: DragEvent): boolean {
  if (!dragging.value) return false
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  return true
}

function onItemDragOver(e: DragEvent, ref: DragRef): void {
  if (!allowDrop(e)) return
  dropTarget.value = { kind: edge(e), ref }
}

function onFolderDragOver(e: DragEvent, folder: FeedFolder): void {
  if (!allowDrop(e)) return
  if (dragging.value?.kind === 'folder') {
    dropTarget.value = { kind: edge(e), ref: folderDragRef(folder) }
    return
  }
  // A feed onto a folder header: top slice drops before the folder, bottom
  // slice after it (both top level), the middle drops into the folder.
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const y = e.clientY - rect.top
  if (y < rect.height * 0.25) dropTarget.value = { kind: 'before', ref: folderDragRef(folder) }
  else if (y > rect.height * 0.75) dropTarget.value = { kind: 'after', ref: folderDragRef(folder) }
  else dropTarget.value = { kind: 'into', folderId: folder.id }
}

function onEndDragOver(e: DragEvent): void {
  if (!allowDrop(e)) return
  dropTarget.value = { kind: 'top-end' }
}

function onDrop(): void {
  if (dragging.value && dropTarget.value) {
    emit('reorder', applyMove(tree.value, dragging.value, dropTarget.value))
  }
  onDragEnd()
}

function onDragEnd(): void {
  dragging.value = null
  dropTarget.value = null
}

function sameRef(a: DragRef, b: DragRef): boolean {
  return a.kind === b.kind && a.id === b.id
}
function showBefore(ref: DragRef): boolean {
  const t = dropTarget.value
  return !!t && t.kind === 'before' && sameRef(t.ref, ref)
}
function showAfter(ref: DragRef): boolean {
  const t = dropTarget.value
  return !!t && t.kind === 'after' && sameRef(t.ref, ref)
}
function showInto(folderId: string): boolean {
  const t = dropTarget.value
  return !!t && t.kind === 'into' && t.folderId === folderId
}
const showEnd = computed(() => dropTarget.value?.kind === 'top-end')

// ── folder operations ───────────────────────────────────────────────────────
const renamingId = ref<string | null>(null)
const draftName = ref('')

function replaceFolder(folder: FeedFolder, patch: Partial<FeedFolder>): void {
  emit(
    'reorder',
    tree.value.map((n) =>
      n.kind === 'folder' && n.folder.id === folder.id
        ? { kind: 'folder', folder: { ...n.folder, ...patch } }
        : n,
    ),
  )
}

function onHeaderClick(folder: FeedFolder): void {
  if (renamingId.value === folder.id) return
  replaceFolder(folder, { collapsed: !folder.collapsed })
}

function newFolderId(): string {
  const taken = new Set(tree.value.flatMap((n) => (n.kind === 'folder' ? [n.folder.id] : [])))
  let i = 1
  while (taken.has(`folder-${i}`)) i++
  return `folder-${i}`
}

function addFolder(): void {
  const id = newFolderId()
  emit('reorder', [...tree.value, { kind: 'folder', folder: { id, name: 'New folder', collapsed: false, feeds: [] } }])
  renamingId.value = id
  draftName.value = 'New folder'
}

function startRename(folder: FeedFolder): void {
  renamingId.value = folder.id
  draftName.value = folder.name
}

function commitRename(folder: FeedFolder): void {
  const name = draftName.value.trim()
  renamingId.value = null
  if (name && name !== folder.name) replaceFolder(folder, { name })
}

function cancelRename(): void {
  renamingId.value = null
}

function focusInput(vnode: { el?: unknown }): void {
  const el = vnode.el as HTMLInputElement | undefined
  el?.focus()
  el?.select()
}

// Deleting a folder ungroups it: its feeds re-enter the top level at the
// folder's slot (no feed is destroyed), so no confirm is needed.
function deleteFolder(folder: FeedFolder): void {
  const next: FeedTree = []
  for (const n of tree.value) {
    if (n.kind === 'folder' && n.folder.id === folder.id) {
      for (const feed of n.folder.feeds) next.push({ kind: 'feed', feed })
    } else {
      next.push(n)
    }
  }
  emit('reorder', next)
}
</script>

<template>
  <aside class="hive-scroll relative flex shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar" :style="{ width: size + 'px' }">
    <div class="profile-header border-b border-border px-4 pb-3 pt-4" data-testid="sidebar-profile-header">
      <div class="flex items-center gap-2">
        <div class="min-w-0 flex-1 truncate text-[15px] font-semibold tracking-[-.01em]" data-testid="sidebar-profile-name">{{ profile.name }}</div>
        <button
          class="settings-button flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md text-text-3 hover:bg-chip hover:text-text"
          title="Profile settings"
          aria-label="Profile settings"
          data-testid="sidebar-open-settings"
          @click="emit('open-settings')"
        ><IconSettings class="size-3.5" /></button>
      </div>
      <div class="mt-1 flex items-center gap-1.5">
        <span class="flex size-[15px] items-center justify-center rounded border border-strong bg-chip text-text-2"><IconGitBranch class="size-2.5" /></span>
        <span class="text-xs text-text-3">{{ profile.sourceSummary }}</span>
      </div>
    </div>

    <div class="px-2.5 pb-0.5 pt-3">
      <button class="sidebar-entry" :class="{ 'sidebar-entry-selected': allSelected() }" @click="emit('select', { type: 'all' })">
        <span class="nav-icon border-accent-tint text-accent"><IconList class="size-3" /></span><span class="flex-1 text-left">All items</span><span class="font-mono text-[11px]">{{ profile.totalCount }}</span>
      </button>
    </div>

    <section class="px-2.5 pb-1.5 pt-2" data-testid="sidebar-feeds">
      <div class="section-label">
        <IconRss class="size-3 text-feeds" /><span>FEEDS</span>
        <button
          class="folder-add ml-auto flex size-5 items-center justify-center rounded-md text-text-4 hover:bg-chip hover:text-text"
          title="New folder"
          aria-label="New folder"
          data-testid="sidebar-new-folder"
          @click="addFolder"
        ><IconFolderPlus class="size-3" /></button>
      </div>

      <template v-for="node in tree" :key="node.kind === 'feed' ? node.feed.id : node.folder.id">
        <!-- top-level feed -->
        <div
          v-if="node.kind === 'feed'"
          class="sb-item"
          :class="{ 'drop-before': showBefore(feedRef(node.feed)), 'drop-after': showAfter(feedRef(node.feed)) }"
          draggable="true"
          data-testid="sidebar-item"
          @dragstart="onDragStart($event, feedRef(node.feed))"
          @dragover.prevent="onItemDragOver($event, feedRef(node.feed))"
          @drop.prevent="onDrop"
          @dragend="onDragEnd"
        >
          <SidebarFeedRow
            :feed="node.feed"
            :selected="feedSelected(node.feed.id)"
            @select="emit('select', { type: 'feed', feedId: node.feed.id })"
            @reveal="emit('reveal-in-flow', node.feed.id)"
          />
        </div>

        <!-- folder -->
        <div
          v-else
          class="sb-folder"
          data-testid="sidebar-folder"
          :data-id="node.folder.id"
          :class="{ 'drop-before': showBefore(folderDragRef(node.folder)), 'drop-after': showAfter(folderDragRef(node.folder)), 'drop-into': showInto(node.folder.id) }"
        >
          <div
            class="folder-header"
            draggable="true"
            @dragstart="onDragStart($event, folderDragRef(node.folder))"
            @dragover.prevent="onFolderDragOver($event, node.folder)"
            @drop.prevent="onDrop"
            @dragend="onDragEnd"
            @click="onHeaderClick(node.folder)"
          >
            <span class="folder-chevron text-text-4"><component :is="node.folder.collapsed ? IconChevronRight : IconChevronDown" class="size-3" /></span>
            <span class="nav-icon"><IconFolder class="size-3" /></span>
            <input
              v-if="renamingId === node.folder.id"
              v-model="draftName"
              class="folder-rename min-w-0 flex-1"
              data-testid="folder-rename-input"
              @click.stop
              @vue:mounted="focusInput"
              @keydown.enter.prevent="commitRename(node.folder)"
              @keydown.esc.prevent="cancelRename"
              @blur="commitRename(node.folder)"
            >
            <span v-else class="min-w-0 flex-1 truncate text-left font-medium" data-testid="folder-name">{{ node.folder.name }}</span>
            <button
              class="folder-action flex size-5 shrink-0 items-center justify-center rounded-md text-text-4 hover:bg-chip hover:text-text"
              title="Rename folder"
              data-testid="folder-rename"
              @click.stop="startRename(node.folder)"
            ><IconPencil class="size-3" /></button>
            <button
              class="folder-action flex size-5 shrink-0 items-center justify-center rounded-md text-text-4 hover:bg-danger/15 hover:text-danger"
              title="Delete folder"
              data-testid="folder-delete"
              @click.stop="deleteFolder(node.folder)"
            ><IconTrash class="size-3" /></button>
            <span class="font-mono text-[11px]" :class="folderNew(node.folder) ? 'text-accent' : 'text-text-3'">{{ folderNew(node.folder) || folderTotal(node.folder) }}</span>
          </div>

          <div v-if="!node.folder.collapsed" class="folder-body">
            <div
              v-for="feed in node.folder.feeds"
              :key="feed.id"
              class="sb-item indented"
              :class="{ 'drop-before': showBefore(feedRef(feed)), 'drop-after': showAfter(feedRef(feed)) }"
              draggable="true"
              data-testid="sidebar-item"
              @dragstart="onDragStart($event, feedRef(feed))"
              @dragover.prevent="onItemDragOver($event, feedRef(feed))"
              @drop.prevent="onDrop"
              @dragend="onDragEnd"
            >
              <SidebarFeedRow
                :feed="feed"
                :selected="feedSelected(feed.id)"
                @select="emit('select', { type: 'feed', feedId: feed.id })"
                @reveal="emit('reveal-in-flow', feed.id)"
              />
            </div>
            <div v-if="node.folder.feeds.length === 0" class="folder-empty">Drop feeds here</div>
          </div>
        </div>
      </template>

      <!-- Trailing drop zone: drop here to move an item to the end / out of a folder. -->
      <div
        class="sb-end"
        :class="{ 'drop-end': showEnd }"
        data-testid="sidebar-drop-end"
        @dragover.prevent="onEndDragOver"
        @drop.prevent="onDrop"
        @dragend="onDragEnd"
      />
    </section>

    <button
      class="mt-auto flex items-center gap-2.5 border-t border-border p-2.5 text-left hover:bg-chip"
      data-testid="sidebar-edit-flow"
      @click="emit('open-flows')"
    >
      <span class="flex size-[22px] shrink-0 items-center justify-center rounded-md border border-dashed border-card bg-app text-accent"><IconWorkflow class="size-3" /></span>
      <span class="min-w-0 flex-1">
        <span class="block text-[12.5px] font-semibold text-text">Edit flow</span>
        <span class="block truncate font-mono text-[11px] text-text-3">Open editor</span>
      </span>
      <span
        v-if="flowsDirty"
        class="flex shrink-0 items-center gap-1.5 rounded-md border border-accent/35 bg-accent-tint px-1.5 py-0.5 text-[10.5px] font-semibold text-accent"
        data-testid="undeployed-badge"
      ><span class="size-1.5 shrink-0 rounded-full bg-accent" />Un-deployed</span>
      <IconChevronRight class="size-3.5 shrink-0 text-text-4" />
    </button>

    <PanelResizeHandle edge="right" name="sidebar" :start="startResize" :step="step" />
  </aside>
</template>

<style scoped>
.sidebar-entry { display: flex; align-items: center; gap: 9px; width: 100%; padding: 7px 8px; border-radius: 7px; color: var(--color-text-2); font-size: 13px; cursor: pointer; }
.sidebar-entry:hover { background: var(--color-chip); color: var(--color-text); }
.sidebar-entry-selected { background: var(--color-hover); color: var(--color-accent); font-weight: 500; }
.nav-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-app); color: var(--color-text-2); }
.section-label { display: flex; align-items: center; gap: 7px; padding: 0 6px 8px; color: var(--color-text-4); font-family: var(--font-mono); font-size: 10.5px; letter-spacing: .12em; }
.folder-add { opacity: 0; }
.section-label:hover .folder-add, .folder-add:focus-visible { opacity: 1; }

/* An item wrapper carries the drag handle + insertion indicator; the row/header
   inside it stays visually unchanged. */
.sb-item { border-radius: 7px; }
.sb-item.indented { padding-left: 16px; }
.drop-before { box-shadow: inset 0 2px 0 0 var(--color-accent); }
.drop-after { box-shadow: inset 0 -2px 0 0 var(--color-accent); }

.folder-header { display: flex; align-items: center; gap: 7px; width: 100%; padding: 7px 8px; border-radius: 7px; color: var(--color-text-2); font-size: 13px; cursor: pointer; }
.folder-header:hover { background: var(--color-chip); color: var(--color-text); }
.folder-chevron { display: inline-flex; flex: none; }
.folder-action { opacity: 0; }
.folder-header:hover .folder-action, .folder-action:focus-visible { opacity: 1; }
.folder-rename { background: var(--color-app); border: 1px solid var(--color-accent); border-radius: 5px; padding: 1px 5px; font-size: 13px; color: var(--color-text); outline: none; }
.folder-body { margin-top: 1px; }
.folder-empty { padding: 6px 8px 6px 24px; font-size: 11.5px; color: var(--color-text-4); font-style: italic; }
.sb-folder.drop-into { background: var(--color-accent-tint); border-radius: 7px; }
.sb-folder.drop-before { box-shadow: inset 0 2px 0 0 var(--color-accent); }
.sb-folder.drop-after { box-shadow: inset 0 -2px 0 0 var(--color-accent); }

.sb-end { height: 14px; border-radius: 5px; }
.sb-end.drop-end { box-shadow: inset 0 2px 0 0 var(--color-accent); }
</style>
