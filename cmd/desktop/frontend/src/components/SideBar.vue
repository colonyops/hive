<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconFolder from '~icons/lucide/folder'
import IconFolderPlus from '~icons/lucide/folder-plus'
import IconPencil from '~icons/lucide/pencil'
import IconRss from '~icons/lucide/rss'
import IconSettings from '~icons/lucide/settings'
import IconTrash from '~icons/lucide/trash-2'
import IconWorkflow from '~icons/lucide/workflow'
import IconButton from './ui/IconButton.vue'
import InlineConfirm from './ui/InlineConfirm.vue'
import RenameDialog from './ui/RenameDialog.vue'
import PanelResizeHandle from './ui/PanelResizeHandle.vue'
import SidebarFeedRow from './SidebarFeedRow.vue'
import { useResizablePanel } from '../composables/useResizablePanel'
import { applyMove, SIDEBAR_DRAG_MIME, type DragRef, type DropTarget } from '../lib/feedTree'
import { dropEdge, useDragReorder } from '../composables/useDragReorder'
import type { FeedFolder, FeedSummary, FeedTree, Profile, SidebarSelection } from '../types/feed'
import BaseBadge from './ui/BaseBadge.vue'

const props = defineProps<{ profile: Profile; selection: SidebarSelection; flowsDirty?: boolean }>()
const emit = defineEmits<{
  select: [sel: SidebarSelection]
  'open-flows': []
  'open-settings': []
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
const tree = computed<FeedTree>(() => props.profile.tree ?? props.profile.feeds.map((f) => ({ kind: 'feed', feed: f })))

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
const drag = useDragReorder<DragRef, DropTarget>({
  mime: SIDEBAR_DRAG_MIME,
  payload: (item) => item.id,
  onDrop: (dragged, target) => emit('reorder', applyMove(tree.value, dragged, target)),
})
const { target: dropTarget } = drag

// A feed onto a folder header: top slice drops before the folder, bottom slice
// after it (both top level), the middle drops into the folder.
function onFolderDragOver(e: DragEvent, folder: FeedFolder): void {
  const header = folderDragRef(folder)
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const y = e.clientY - rect.top
  if (drag.dragging.value?.kind === 'folder') drag.over(e, { kind: dropEdge(e), ref: header })
  else if (y < rect.height * 0.25) drag.over(e, { kind: 'before', ref: header })
  else if (y > rect.height * 0.75) drag.over(e, { kind: 'after', ref: header })
  else drag.over(e, { kind: 'into', folderId: folder.id })
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

// ── folder collapse (view state) ────────────────────────────────────────────
// Expand/collapse is transient view state, not configuration — it lives in
// localStorage (keyed by flow id, then folder id), never in the persisted
// sidebar layout. VueUse's useStorage handles serialization + defaults so a
// toggle never touches the backend and never causes a flow reload.
const collapsedByFlow = useStorage<Record<string, string[]>>('hive.sidebar.collapsed', {})

function isCollapsed(folderId: string): boolean {
  return (collapsedByFlow.value[props.profile.id] ?? []).includes(folderId)
}

function onHeaderClick(folder: FeedFolder): void {
  const flowId = props.profile.id
  const current = collapsedByFlow.value[flowId] ?? []
  collapsedByFlow.value = {
    ...collapsedByFlow.value,
    [flowId]: isCollapsed(folder.id) ? current.filter((id) => id !== folder.id) : [...current, folder.id],
  }
}

// ── folder structure (persisted) ────────────────────────────────────────────
// Renaming and deleting both live in the folder's edit dialog, so the tree row
// carries a single hover affordance and no click in it can mutate the layout.
// The dialog reads the folder back out of the tree by id, so a rename reflects
// straight from the persisted layout rather than a local copy.
const editingId = ref<string | null>(null)

const editingFolder = computed<FeedFolder | null>(() => {
  const node = tree.value.find((n) => n.kind === 'folder' && n.folder.id === editingId.value)
  return node?.kind === 'folder' ? node.folder : null
})

// Deleting is the rare exception, so it stays a quiet footer action until it
// is asked for, then expands into a confirm over the dimmed rename.
const confirmingFolderDelete = ref(false)
watch(editingId, () => (confirmingFolderDelete.value = false))

function feedCount(folder: FeedFolder): string {
  const count = folder.feeds.length
  return `${count} ${count === 1 ? 'feed' : 'feeds'}`
}

function folderDeleteConsequence(folder: FeedFolder): string {
  const count = folder.feeds.length
  if (count === 0) return 'The folder is empty, so nothing else changes.'
  return `Its ${feedCount(folder)} ${count === 1 ? 'moves' : 'move'} to the top level — nothing is unsubscribed.`
}

function replaceFolder(folder: FeedFolder, patch: Partial<FeedFolder>): void {
  emit(
    'reorder',
    tree.value.map((n) =>
      n.kind === 'folder' && n.folder.id === folder.id ? { kind: 'folder', folder: { ...n.folder, ...patch } } : n,
    ),
  )
}

function newFolderId(): string {
  const taken = new Set(tree.value.flatMap((n) => (n.kind === 'folder' ? [n.folder.id] : [])))
  let i = 1
  while (taken.has(`folder-${i}`)) i++
  return `folder-${i}`
}

function addFolder(): void {
  const id = newFolderId()
  emit('reorder', [...tree.value, { kind: 'folder', folder: { id, name: 'New folder', feeds: [] } }])
  editingId.value = id
}

function saveFolderName(folder: FeedFolder, name: string): void {
  editingId.value = null
  if (name !== folder.name) replaceFolder(folder, { name })
}

// Deleting a folder ungroups it: its feeds re-enter the top level at the
// folder's slot, so no feed is destroyed — but the folder's name and grouping
// are, with no undo. The edit dialog confirms before it emits, so reaching here
// already means the user said yes.
function deleteFolder(folder: FeedFolder): void {
  const next: FeedTree = []
  for (const n of tree.value) {
    if (n.kind === 'folder' && n.folder.id === folder.id) {
      for (const feed of n.folder.feeds) next.push({ kind: 'feed', feed })
    } else {
      next.push(n)
    }
  }
  editingId.value = null
  emit('reorder', next)
}
</script>

<template>
  <aside
    class="hive-scroll relative flex shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar"
    :style="{ width: size + 'px' }"
  >
    <!-- The height matches the feed list's search bar beside it, so the two
         panes divide on the same line. -->
    <div
      class="flex h-[46px] shrink-0 items-center gap-2 border-b border-border px-4"
      data-testid="sidebar-profile-header"
    >
      <div
        class="min-w-0 flex-1 truncate text-title font-semibold tracking-[-.01em]"
        data-testid="sidebar-profile-name"
      >
        {{ profile.name }}
      </div>
      <IconButton
        label="Profile settings"
        :icon="IconSettings"
        data-testid="sidebar-open-settings"
        @click="emit('open-settings')"
      />
    </div>

    <section class="px-2.5 pb-1.5 pt-3" data-testid="sidebar-feeds">
      <div class="section-label">
        <IconRss class="size-3 text-feeds" /><span>FEEDS</span>
        <IconButton
          label="New folder"
          :icon="IconFolderPlus"
          size="sm"
          class="folder-add ml-auto"
          data-testid="sidebar-new-folder"
          @click="addFolder"
        />
      </div>

      <template v-for="node in tree" :key="node.kind === 'feed' ? node.feed.id : node.folder.id">
        <!-- top-level feed -->
        <div
          v-if="node.kind === 'feed'"
          class="sb-item"
          :class="{ 'drop-before': showBefore(feedRef(node.feed)), 'drop-after': showAfter(feedRef(node.feed)) }"
          draggable="true"
          data-testid="sidebar-item"
          @dragstart="drag.start($event, feedRef(node.feed))"
          @dragover.prevent="drag.over($event, { kind: dropEdge($event), ref: feedRef(node.feed) })"
          @drop.prevent="drag.drop"
          @dragend="drag.end"
        >
          <SidebarFeedRow
            :feed="node.feed"
            :selected="feedSelected(node.feed.id)"
            @select="emit('select', { type: 'feed', feedId: node.feed.id })"
          />
        </div>

        <!-- folder -->
        <div
          v-else
          class="sb-folder"
          data-testid="sidebar-folder"
          :data-id="node.folder.id"
          :class="{
            'drop-before': showBefore(folderDragRef(node.folder)),
            'drop-after': showAfter(folderDragRef(node.folder)),
            'drop-into': showInto(node.folder.id),
          }"
        >
          <div
            class="folder-header"
            draggable="true"
            @dragstart="drag.start($event, folderDragRef(node.folder))"
            @dragover.prevent="onFolderDragOver($event, node.folder)"
            @drop.prevent="drag.drop"
            @dragend="drag.end"
            @click="onHeaderClick(node.folder)"
          >
            <span class="nav-icon">
              <IconFolder class="folder-glyph size-3" />
              <component
                :is="isCollapsed(node.folder.id) ? IconChevronRight : IconChevronDown"
                class="folder-chevron size-3"
              />
            </span>
            <span class="min-w-0 flex-1 truncate text-left font-medium" data-testid="folder-name">{{
              node.folder.name
            }}</span>
            <IconButton
              label="Edit folder"
              :icon="IconPencil"
              size="sm"
              class="folder-action"
              data-testid="folder-edit"
              @click.stop="editingId = node.folder.id"
            />
            <span class="font-mono text-caption" :class="folderNew(node.folder) ? 'text-accent' : 'text-text-3'">{{
              folderNew(node.folder) || folderTotal(node.folder)
            }}</span>
          </div>

          <div v-if="!isCollapsed(node.folder.id)" class="folder-body">
            <div
              v-for="feed in node.folder.feeds"
              :key="feed.id"
              class="sb-item indented"
              :class="{ 'drop-before': showBefore(feedRef(feed)), 'drop-after': showAfter(feedRef(feed)) }"
              draggable="true"
              data-testid="sidebar-item"
              @dragstart="drag.start($event, feedRef(feed))"
              @dragover.prevent="drag.over($event, { kind: dropEdge($event), ref: feedRef(feed) })"
              @drop.prevent="drag.drop"
              @dragend="drag.end"
            >
              <SidebarFeedRow
                :feed="feed"
                :selected="feedSelected(feed.id)"
                @select="emit('select', { type: 'feed', feedId: feed.id })"
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
        @dragover.prevent="drag.over($event, { kind: 'top-end' })"
        @drop.prevent="drag.drop"
        @dragend="drag.end"
      />
    </section>

    <!-- Trash: unrouted + ignored items. Deliberately de-emphasized — no
         count, no unread badge; a place to go, never a queue that calls.
         Styled as a footer utility row alongside Edit flow, not as a feed. -->
    <button
      type="button"
      class="footer-entry mt-auto"
      :class="{ 'footer-entry-selected': selection.type === 'trash' }"
      data-testid="sidebar-trash"
      @click="emit('select', { type: 'trash' })"
    >
      <span class="footer-icon"><IconTrash class="size-3" /></span>
      <span class="min-w-0 flex-1 truncate">Trash</span>
    </button>

    <button
      class="flex items-center gap-2.5 border-t border-border p-2.5 text-left hover:bg-chip"
      data-testid="sidebar-edit-flow"
      @click="emit('open-flows')"
    >
      <span
        class="flex size-[22px] shrink-0 items-center justify-center rounded-md border border-dashed border-card bg-app text-accent"
        ><IconWorkflow class="size-3"
      /></span>
      <span class="min-w-0 flex-1">
        <span class="block text-small font-semibold text-text">Edit flow</span>
        <span class="block truncate font-mono text-caption text-text-3">Open editor</span>
      </span>
      <BaseBadge
        v-if="flowsDirty"
        tone="accent"
        dot
        class="shrink-0 border border-accent/35 px-1.5 py-0.5 text-micro font-semibold"
        data-testid="undeployed-badge"
        >Un-deployed</BaseBadge
      >
      <IconChevronRight class="size-3.5 shrink-0 text-text-4" />
    </button>

    <PanelResizeHandle edge="right" name="sidebar" :start="startResize" :step="step" />

    <RenameDialog
      v-if="editingFolder"
      :key="editingFolder.id"
      title="Edit folder"
      :icon="IconFolder"
      label="Folder name"
      :name="editingFolder.name"
      :hint="editingFolder.feeds.length ? `${feedCount(editingFolder)} inside` : 'No feeds inside'"
      confirm-label="Save"
      :locked="confirmingFolderDelete"
      testid="folder-edit"
      :testids="{ modal: 'folder-edit-modal', input: 'folder-edit-name' }"
      @save="saveFolderName(editingFolder, $event)"
      @close="editingId = null"
    >
      <InlineConfirm
        v-if="confirmingFolderDelete"
        title="Delete this folder?"
        :description="folderDeleteConsequence(editingFolder)"
        confirm-label="Delete"
        cancel-label="Keep"
        testid="folder-delete-confirm"
        @confirm="deleteFolder(editingFolder)"
        @cancel="confirmingFolderDelete = false"
      />
      <template #footer-start>
        <button
          type="button"
          class="cursor-pointer text-small text-text-3 hover:text-severity-error"
          data-testid="folder-edit-delete"
          @click="confirmingFolderDelete = true"
        >
          Delete folder
        </button>
      </template>
    </RenameDialog>
  </aside>
</template>

<style scoped>
.footer-entry {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px;
  border-top: 1px solid var(--color-border);
  color: var(--color-text-2);
  font-size: var(--text-small);
  text-align: left;
  cursor: pointer;
}
.footer-entry:hover {
  background: var(--color-chip);
  color: var(--color-text);
}
.footer-entry-selected {
  color: var(--color-accent);
  font-weight: 500;
}
.footer-entry-selected .footer-icon {
  border-color: var(--color-accent-tint);
  color: var(--color-accent);
}
.footer-icon {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: 1px solid var(--color-card);
  border-radius: var(--radius-md);
  background: var(--color-app);
  color: var(--color-text-3);
}
.nav-icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 1px solid var(--color-strong);
  border-radius: var(--radius-md);
  background: var(--color-app);
  color: var(--color-text-2);
}
.section-label {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 0 6px 8px;
  color: var(--color-text-4);
  font-family: var(--font-mono);
  font-size: var(--text-micro);
  letter-spacing: 0.12em;
}
.folder-add {
  opacity: 0;
}
.section-label:hover .folder-add,
.folder-add:focus-visible {
  opacity: 1;
}

/* An item wrapper carries the drag handle + insertion indicator; the row/header
   inside it stays visually unchanged. */
.sb-item {
  border-radius: var(--radius-lg);
}
.sb-item.indented {
  padding-left: 12px;
}
.drop-before {
  box-shadow: inset 0 2px 0 0 var(--color-accent);
}
.drop-after {
  box-shadow: inset 0 -2px 0 0 var(--color-accent);
}

.folder-header {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 7px 8px;
  border-radius: var(--radius-lg);
  color: var(--color-text-2);
  font-size: var(--text-body);
  cursor: pointer;
}
.folder-header:hover {
  background: var(--color-chip);
  color: var(--color-text);
}
/* The leading icon slot shows the folder glyph by default and swaps to a
   collapse/expand chevron on hover — no permanent chevron column, so the
   folder header aligns with the feed rows above it. */
.folder-glyph {
  display: inline-flex;
}
.folder-chevron {
  display: none;
}
.folder-header:hover .folder-glyph {
  display: none;
}
.folder-header:hover .folder-chevron {
  display: inline-flex;
}
.folder-action {
  opacity: 0;
}
.folder-header:hover .folder-action,
.folder-action:focus-visible {
  opacity: 1;
}
.folder-body {
  margin-top: 1px;
}
.folder-empty {
  padding: 6px 8px 6px 20px;
  font-size: var(--text-caption);
  color: var(--color-text-4);
  font-style: italic;
}
.sb-folder.drop-into {
  background: var(--color-accent-tint);
  border-radius: var(--radius-lg);
}
.sb-folder.drop-before {
  box-shadow: inset 0 2px 0 0 var(--color-accent);
}
.sb-folder.drop-after {
  box-shadow: inset 0 -2px 0 0 var(--color-accent);
}

.sb-end {
  height: 14px;
  border-radius: var(--radius-md);
}
.sb-end.drop-end {
  box-shadow: inset 0 2px 0 0 var(--color-accent);
}
</style>
