<script setup lang="ts">
import { onClickOutside, useEventListener } from '@vueuse/core'
import { computed, nextTick, ref, watch } from 'vue'
import AppMenu from './ui/AppMenu.vue'
import FeedListItem from './FeedListItem.vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCopy from '~icons/lucide/copy'
import IconEllipsis from '~icons/lucide/ellipsis'
import IconGitBranch from '~icons/lucide/git-branch'
import IconMailCheck from '~icons/lucide/mail-check'
import IconPlus from '~icons/lucide/plus'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconSearch from '~icons/lucide/search'
import IconSquareCheckBig from '~icons/lucide/square-check-big'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconUserRound from '~icons/lucide/user-round'
import IconX from '~icons/lucide/x'
import IconArchive from '~icons/lucide/archive'
import { groupItemsByDate } from '../lib/dateGroups'
import type { ActionView } from '../types/action'
import type { FeedSort, InboxItem } from '../types/feed'
import type { MenuEntry } from '../types/menu'

// Presentation-only: the store (useFeedState) owns the search text and the
// filtered `visibleItems`, so keyboard navigation and this list render the
// exact same set. This component just renders and relays intent.
const props = defineProps<{
  title: string
  visibleItems: InboxItem[]
  archivedItems: InboxItem[]
  archivedCount: number
  archivedExpanded: boolean
  trash: boolean
  trashFilter: 'all' | 'ignored'
  selectedId: number | null
  unreadOnly: boolean
  unreadCount: number
  search: string
  authors: string[]
  authorFilter: string
  sort: FeedSort
  loadError: string | null
  refreshing: boolean
  selectionMode: boolean
  selectedItemIds: number[]
  selectionActions: ActionView[]
  sourceIcons?: Record<string, string>
  sourceImages?: Record<string, string>
}>()
const emit = defineEmits<{
  select: [id: number]
  activate: [id: number]
  'set-unread': [value: boolean]
  'toggle-archived': []
  'set-trash-filter': [value: 'all' | 'ignored']
  refresh: []
  'mark-all-read': []
  'enter-selection': []
  'toggle-item-selection': [id: number]
  'cancel-selection': []
  'copy-selection-contents': []
  'run-selection-action': [actionId: string]
  'create-session-from-selection': []
  'update:search': [value: string]
  'update:author-filter': [value: string]
  'set-sort': [value: FeedSort]
  // Row-level intents from a FeedListItem's hover pill / "…" menu, re-emitted
  // with the item so the store can act on rows that are not the selection.
  'item-set-unread': [item: InboxItem, unread: boolean]
  'item-toggle-archive': [item: InboxItem]
  'item-toggle-ignored': [item: InboxItem]
  'item-open-browser': [item: InboxItem]
  'item-copy-link': [item: InboxItem]
  'item-copy-contents': [item: InboxItem]
  'item-create-session': [item: InboxItem, target: 'repository' | 'workspace']
  'item-run-action': [item: InboxItem, actionId: string]
}>()

const sortOptions: { value: FeedSort; label: string }[] = [
  { value: 'newest', label: 'Newest' },
  { value: 'oldest', label: 'Oldest' },
  { value: 'unread', label: 'Unread first' },
]
// Date separators, bucketed on the same timestamp (lastEventAt) the newest and
// oldest sorts order by — so rows never appear out of order inside a group,
// and oldest-first simply yields the buckets in reverse. `unread` sort
// deliberately interleaves dates, so it renders one unlabeled group: real
// separators there would repeat and read as broken ordering.
const itemGroups = computed<{ key: string; label: string | null; items: InboxItem[] }[]>(() => {
  if (props.sort === 'unread') return [{ key: 'all', label: null, items: props.visibleItems }]
  return groupItemsByDate(props.visibleItems).map((group, index) => ({
    ...group,
    // A leading "Today" says nothing the top of the list doesn't — newest-first
    // opens on today by definition. It keeps its label under oldest sort, where
    // it trails and an unlabeled run would read as part of the tier above.
    label: index === 0 && group.key === 'today' ? null : group.label,
  }))
})

const viewMenu = ref<HTMLElement | null>(null)
const viewMenuOpen = ref(false)
const authorMenuOpen = ref(false)
const authorMenuTrigger = ref<HTMLElement | null>(null)
const authorSearchInput = ref<HTMLInputElement | null>(null)
const authorQuery = ref('')
const filteredAuthors = computed(() => {
  const query = authorQuery.value.trim().toLowerCase()
  return query ? props.authors.filter((author) => author.toLowerCase().includes(query)) : props.authors
})
const selectionActionsToggle = ref<HTMLElement | null>(null)
const selectionActionsOpen = ref(false)
const selectedItemIDSet = computed(() => new Set(props.selectedItemIds))
const selectionActionEntries = computed<MenuEntry[]>(() =>
  props.selectionActions.map((action) => ({
    kind: 'action',
    id: action.id,
    label: action.label,
    icon: IconCopy,
    testid: `selection-action-${action.id}`,
  })),
)

function chooseSelectionAction(actionID: string): void {
  selectionActionsOpen.value = false
  emit('run-selection-action', actionID)
}

function closeViewMenu(): void {
  viewMenuOpen.value = false
  authorMenuOpen.value = false
  authorQuery.value = ''
}
function toggleViewMenu(): void {
  if (viewMenuOpen.value) closeViewMenu()
  else viewMenuOpen.value = true
}
function openAuthorMenu(): void {
  authorMenuOpen.value = true
  authorQuery.value = ''
  void nextTick(() => authorSearchInput.value?.focus())
}
function chooseSort(value: FeedSort): void {
  emit('set-sort', value)
  closeViewMenu()
}
function chooseAuthor(value: string): void {
  emit('update:author-filter', value)
  closeViewMenu()
}
function refreshFromMenu(): void {
  emit('refresh')
  closeViewMenu()
}
// Trash has no unread semantics, so it gets no mark-all-read entry at all.
function markAllReadFromMenu(): void {
  emit('mark-all-read')
  closeViewMenu()
}
function enterSelectionFromMenu(): void {
  emit('enter-selection')
  closeViewMenu()
}
function onDocumentKeydown(event: KeyboardEvent): void {
  if (!viewMenuOpen.value || event.key !== 'Escape') return
  if (authorMenuOpen.value) {
    authorMenuOpen.value = false
    authorMenuTrigger.value?.focus()
  } else closeViewMenu()
}

onClickOutside(viewMenu, () => {
  if (viewMenuOpen.value) closeViewMenu()
})
useEventListener(document, 'keydown', onDocumentKeydown)

// Selected, not just focused: view.focus-search means "start a new search"
// far more often than "edit the old one" — same call TerminalMode's own
// filter field makes on the session-tree half of that command.
const searchInput = ref<HTMLInputElement | null>(null)
function focusSearch(): void {
  searchInput.value?.select()
}
defineExpose({ focusSearch })

// Keep the selected row in view when navigation moves the cursor by keyboard
// (mirrors CommandPalette's scrollIntoView on selection change).
const listContainer = ref<HTMLElement | null>(null)
watch(
  () => props.selectedId,
  async (id) => {
    if (!id) return
    await nextTick()
    const rows = listContainer.value?.querySelectorAll('[data-testid="feed-item"]')
    const row = rows && Array.from(rows).find((el) => el.getAttribute('data-inbox-id') === String(id))
    ;(row as HTMLElement | undefined)?.scrollIntoView?.({ block: 'nearest' })
  },
)
</script>

<template>
  <section class="feed-list flex min-w-0 flex-[1.25] flex-col border-r border-border">
    <!-- Top row: search + list-level All/Unread filter. No restated title —
         the sidebar already shows the active source. -->
    <header class="flex h-[46px] shrink-0 items-center gap-2.5 border-b border-border bg-pane px-3.5">
      <label class="search-box">
        <IconSearch class="size-[14px] shrink-0 text-text-3" />
        <input
          ref="searchInput"
          :value="search"
          type="text"
          class="search-input"
          placeholder="Search items, sources, people…"
          data-testid="feed-search"
          @input="emit('update:search', ($event.target as HTMLInputElement).value)"
        />
      </label>
      <!-- Trash filters by disposition (ignored vs everything); feeds filter
           by unread. Trash carries no unread semantics. -->
      <div v-if="trash" class="segmented" role="group" aria-label="Filter">
        <button
          class="seg"
          :class="{ active: trashFilter === 'all' }"
          data-testid="filter-trash-all"
          @click="emit('set-trash-filter', 'all')"
        >
          All
        </button>
        <button
          class="seg"
          :class="{ active: trashFilter === 'ignored' }"
          data-testid="filter-trash-ignored"
          @click="emit('set-trash-filter', 'ignored')"
        >
          Ignored
        </button>
      </div>
      <div v-else class="segmented" role="group" aria-label="Filter">
        <button
          class="seg"
          :class="{ active: !unreadOnly }"
          data-testid="filter-all"
          @click="emit('set-unread', false)"
        >
          All
        </button>
        <button
          class="seg"
          :class="{ active: unreadOnly }"
          data-testid="filter-unread"
          @click="emit('set-unread', true)"
        >
          Unread<span class="seg-count">{{ unreadCount }}</span>
        </button>
      </div>
      <div ref="viewMenu" class="relative shrink-0">
        <button
          type="button"
          class="view-trigger"
          title="Feed options"
          aria-label="Feed options"
          data-testid="view-menu-toggle"
          aria-haspopup="menu"
          :aria-expanded="viewMenuOpen"
          @click="toggleViewMenu"
        >
          <IconEllipsis class="size-4" />
        </button>
        <div v-if="viewMenuOpen" class="view-menu" role="menu" data-testid="view-menu">
          <div class="view-menu-label">Sort by</div>
          <button
            v-for="option in sortOptions"
            :key="option.value"
            type="button"
            class="view-menu-item"
            role="menuitemradio"
            :aria-checked="option.value === sort"
            :data-testid="`view-sort-${option.value}`"
            @click="chooseSort(option.value)"
          >
            <IconCheck
              class="size-3.5"
              :class="option.value === sort ? 'text-accent' : 'opacity-0'"
              :stroke-width="3"
            />
            <span>{{ option.label }}</span>
          </button>
          <template v-if="authors.length || authorFilter">
            <div class="view-menu-divider" />
            <div class="view-menu-label">Filter by</div>
            <div class="relative">
              <button
                ref="authorMenuTrigger"
                type="button"
                class="view-menu-item"
                role="menuitem"
                aria-haspopup="menu"
                :aria-expanded="authorMenuOpen"
                data-testid="view-author-filter"
                @click="openAuthorMenu"
              >
                <IconUserRound class="size-3.5 shrink-0 text-text-3" />
                <span class="flex-1">Author</span>
                <IconChevronRight class="size-3.5 shrink-0 text-text-4" />
              </button>
              <div
                v-if="authorMenuOpen"
                class="author-submenu flex max-h-64 flex-col overflow-hidden"
                role="menu"
                aria-label="Filter by author"
                data-testid="view-author-submenu"
              >
                <label class="flex shrink-0 items-center gap-2 border-b border-row px-2.5 py-2">
                  <IconSearch class="size-3.5 shrink-0 text-text-4" />
                  <input
                    ref="authorSearchInput"
                    v-model="authorQuery"
                    type="text"
                    placeholder="Search authors…"
                    aria-label="Search authors"
                    class="w-0 min-w-0 flex-1 bg-transparent text-[13px] text-text outline-none placeholder:text-text-4"
                    data-testid="view-author-search"
                  />
                </label>
                <div class="hive-scroll min-h-0 overflow-y-auto p-[5px]">
                  <button
                    v-if="!authorQuery.trim()"
                    type="button"
                    class="view-menu-item"
                    role="menuitemradio"
                    :aria-checked="!authorFilter"
                    data-testid="view-author-all"
                    @click="chooseAuthor('')"
                  >
                    <IconCheck
                      class="size-3.5 shrink-0"
                      :class="!authorFilter ? 'text-accent' : 'opacity-0'"
                      :stroke-width="3"
                    />
                    <span>All authors</span>
                  </button>
                  <button
                    v-for="author in filteredAuthors"
                    :key="author"
                    type="button"
                    class="view-menu-item"
                    role="menuitemradio"
                    :aria-checked="author === authorFilter"
                    data-testid="view-author-option"
                    @click="chooseAuthor(author)"
                  >
                    <IconCheck
                      class="size-3.5 shrink-0"
                      :class="author === authorFilter ? 'text-accent' : 'opacity-0'"
                      :stroke-width="3"
                    />
                    <span class="truncate" :title="author">{{ author }}</span>
                  </button>
                  <div
                    v-if="authorQuery.trim() && filteredAuthors.length === 0"
                    class="px-3 py-4 text-center text-[12.5px] text-text-4"
                    data-testid="view-author-empty"
                  >
                    No matches
                  </div>
                </div>
              </div>
            </div>
          </template>
          <div class="view-menu-divider" />
          <button
            type="button"
            class="view-menu-item"
            role="menuitem"
            data-testid="view-menu-select-items"
            @click="enterSelectionFromMenu"
          >
            <IconSquareCheckBig class="size-3.5 text-text-3" />
            <span>Select items</span>
          </button>
          <button
            v-if="!trash"
            type="button"
            class="view-menu-item"
            role="menuitem"
            data-testid="view-menu-mark-read"
            @click="markAllReadFromMenu"
          >
            <IconMailCheck class="size-3.5 text-text-3" />
            <span>Mark all as read</span>
          </button>
          <button
            type="button"
            class="view-menu-item"
            role="menuitem"
            data-testid="view-menu-refresh"
            :disabled="refreshing"
            @click="refreshFromMenu"
          >
            <IconRefreshCw class="size-3.5 text-text-3" :class="{ 'animate-spin': refreshing }" />
            <span>{{ refreshing ? 'Refreshing…' : 'Refresh' }}</span>
          </button>
        </div>
      </div>
    </header>
    <div v-if="authorFilter" class="flex shrink-0 border-b border-border px-3.5 py-2">
      <button
        type="button"
        class="flex min-w-0 items-center gap-2 rounded-md border border-strong px-2 py-1 text-xs text-text-2 hover:text-text"
        data-testid="clear-author-filter"
        :aria-label="`Clear author filter: ${authorFilter}`"
        @click="emit('update:author-filter', '')"
      >
        <span class="truncate">Author: {{ authorFilter }}</span>
        <IconX class="size-3 shrink-0" />
      </button>
    </div>
    <div v-if="selectionMode" class="selection-bar" data-testid="feed-selection-bar">
      <span
        class="selection-count"
        :title="`${selectedItemIds.length} selected`"
        :aria-label="`${selectedItemIds.length} selected`"
      >
        <IconSquareCheckBig class="size-3.5" />
        <span>{{ selectedItemIds.length }}</span>
      </span>
      <span class="flex-1" />
      <button
        type="button"
        class="selection-action"
        title="Copy contents"
        aria-label="Copy contents"
        :disabled="selectedItemIds.length === 0"
        data-testid="selection-copy-contents"
        @click="emit('copy-selection-contents')"
      >
        <IconCopy class="size-3.5" />
      </button>
      <div v-if="selectionActions.length" class="relative">
        <button
          ref="selectionActionsToggle"
          type="button"
          class="selection-action"
          title="Copy with action"
          aria-label="Copy with action"
          data-testid="selection-actions-toggle"
          aria-haspopup="menu"
          :aria-expanded="selectionActionsOpen"
          @click="selectionActionsOpen = !selectionActionsOpen"
        >
          <IconChevronDown class="size-3.5" />
        </button>
        <AppMenu
          v-if="selectionActionsOpen"
          :entries="selectionActionEntries"
          :ignore="[selectionActionsToggle]"
          testid="selection-actions-menu"
          @select="chooseSelectionAction"
          @close="selectionActionsOpen = false"
        />
      </div>
      <button
        type="button"
        class="selection-action"
        title="Create session"
        aria-label="Create session"
        :disabled="selectedItemIds.length === 0"
        data-testid="selection-create-session"
        @click="emit('create-session-from-selection')"
      >
        <IconPlus class="size-3.5" />
      </button>
      <button
        type="button"
        class="selection-action"
        title="Cancel selection"
        aria-label="Cancel selection"
        data-testid="selection-cancel"
        @click="emit('cancel-selection')"
      >
        <IconX class="size-3.5" />
      </button>
    </div>
    <div class="relative min-h-0 flex-1">
      <div v-if="refreshing" class="refresh-banner" role="status" data-testid="feed-refreshing">
        <IconRefreshCw class="size-3.5 animate-spin" />Refreshing…
      </div>
      <div ref="listContainer" class="hive-scroll h-full overflow-y-auto">
        <!-- Load failure: the "GitHub unreachable" design state. -->
        <div v-if="loadError" class="state-frame" data-testid="feed-error">
          <div class="state-icon text-accent"><IconTriangleAlert class="size-5" /></div>
          <div class="text-[13.5px] font-semibold">GitHub unreachable</div>
          <div class="max-w-[240px] text-xs leading-relaxed text-text-3">{{ loadError }}</div>
          <button class="state-action" :disabled="refreshing" @click="emit('refresh')">
            {{ refreshing ? 'Refreshing…' : 'Retry now' }}
          </button>
        </div>
        <template v-else>
          <template v-for="group in itemGroups" :key="group.key">
            <div v-if="group.label" class="date-divider" data-testid="feed-date-divider">
              <span data-testid="feed-date-label">{{ group.label }}</span>
              <span class="date-count" data-testid="feed-date-count">{{ group.items.length }}</span>
            </div>
            <FeedListItem
              v-for="item in group.items"
              :key="item.id"
              :item="item"
              :trash="trash"
              :selected="item.id === selectedId"
              :selection-mode="selectionMode"
              :checked="selectedItemIDSet.has(item.id)"
              :source-icons="sourceIcons"
              :source-images="sourceImages"
              @select="emit('select', item.id)"
              @activate="emit('activate', item.id)"
              @toggle-selection="emit('toggle-item-selection', item.id)"
              @set-unread="(unread) => emit('item-set-unread', item, unread)"
              @toggle-archive="emit('item-toggle-archive', item)"
              @toggle-ignored="emit('item-toggle-ignored', item)"
              @open-browser="emit('item-open-browser', item)"
              @copy-link="emit('item-copy-link', item)"
              @copy-contents="emit('item-copy-contents', item)"
              @create-session="(target) => emit('item-create-session', item, target)"
              @run-action="(actionId) => emit('item-run-action', item, actionId)"
            />
          </template>
          <!-- Archived section: items whose rules still match but whose work is
             done stay in the feed, demoted below the fold. Collapsed by
             default; expanding lazy-loads the rows. -->
          <template v-if="!trash && archivedCount > 0">
            <button
              type="button"
              class="archived-divider"
              data-testid="archived-divider"
              :aria-expanded="archivedExpanded"
              @click="emit('toggle-archived')"
            >
              <IconArchive class="size-3" />
              <span>Archived ({{ archivedCount }})</span>
              <IconChevronDown class="size-3 transition-transform" :class="{ '-rotate-90': !archivedExpanded }" />
            </button>
            <template v-if="archivedExpanded">
              <FeedListItem
                v-for="item in archivedItems"
                :key="item.id"
                :item="item"
                archived
                :selected="item.id === selectedId"
                :selection-mode="selectionMode"
                :checked="selectedItemIDSet.has(item.id)"
                :source-icons="sourceIcons"
                :source-images="sourceImages"
                @select="emit('select', item.id)"
                @activate="emit('activate', item.id)"
                @toggle-selection="emit('toggle-item-selection', item.id)"
                @set-unread="(unread) => emit('item-set-unread', item, unread)"
                @toggle-archive="emit('item-toggle-archive', item)"
                @toggle-ignored="emit('item-toggle-ignored', item)"
                @open-browser="emit('item-open-browser', item)"
                @copy-link="emit('item-copy-link', item)"
                @copy-contents="emit('item-copy-contents', item)"
                @create-session="(target) => emit('item-create-session', item, target)"
                @run-action="(actionId) => emit('item-run-action', item, actionId)"
              />
            </template>
          </template>
          <!-- Empty feed: "You're all caught up" when the unread filter drained
             the list, "No matches" when a search did, a plain empty state otherwise. -->
          <div
            v-if="visibleItems.length === 0 && (trash || archivedCount === 0)"
            class="state-frame"
            data-testid="feed-empty"
          >
            <template v-if="search.trim() || authorFilter">
              <div class="state-icon text-text-3"><IconSearch class="size-5" /></div>
              <div class="text-[13.5px] font-semibold">No matches</div>
              <div v-if="authorFilter" class="max-w-[240px] text-xs leading-relaxed text-text-3">
                No items by {{ authorFilter }} match the current filters.
              </div>
              <div v-else class="max-w-[240px] text-xs leading-relaxed text-text-3">
                Nothing here matches "{{ search.trim() }}". Try a different search.
              </div>
            </template>
            <template v-else-if="unreadOnly">
              <div class="state-icon text-kind-pr"><IconCheck class="size-5" /></div>
              <div class="text-[13.5px] font-semibold">You're all caught up</div>
              <div class="max-w-[240px] text-xs leading-relaxed text-text-3">
                No unread items in {{ title === 'Unread' ? 'this profile' : title }}. New items will show up here as
                they arrive.
              </div>
            </template>
            <template v-else>
              <div class="state-icon text-text-3"><IconGitBranch class="size-5" /></div>
              <div class="text-[13.5px] font-semibold">No items yet</div>
              <div class="max-w-[240px] text-xs leading-relaxed text-text-3">
                New items will show up here as they arrive.
              </div>
            </template>
            <button
              v-if="!search.trim() && !authorFilter"
              class="state-action"
              :disabled="refreshing"
              @click="emit('refresh')"
            >
              {{ refreshing ? 'Refreshing…' : 'Refresh now' }}
            </button>
          </div>
        </template>
      </div>
    </div>
  </section>
</template>

<style scoped>
.feed-list {
  background: var(--color-list);
}
.search-box {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--color-strong);
  border-radius: 8px;
  background: var(--color-app);
  padding: 6px 11px;
}
.search-box:focus-within {
  border-color: var(--color-accent);
}
.search-input {
  min-width: 0;
  flex: 1;
  border: none;
  background: none;
  color: var(--color-text);
  font-family: var(--font-sans);
  font-size: 13px;
  outline: none;
}
.search-input::placeholder {
  color: var(--color-text-4);
}
.segmented {
  display: flex;
  flex: none;
  align-items: center;
  gap: 2px;
  border: 1px solid var(--color-strong);
  border-radius: 8px;
  background: var(--color-app);
  padding: 2px;
}
.seg {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  border-radius: 6px;
  padding: 4px 11px;
  color: var(--color-text-2);
  font-size: 12px;
  font-weight: 500;
}
.seg:hover:not(.active) {
  color: var(--color-text);
}
.seg.active {
  background: var(--color-accent);
  color: var(--color-accent-contrast);
}
.seg-count {
  font-family: var(--font-mono);
  font-size: 10px;
  opacity: 0.85;
}
.view-trigger {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border: 1px solid var(--color-strong);
  border-radius: 8px;
  color: var(--color-text-2);
}
.view-trigger:hover,
.view-trigger[aria-expanded='true'] {
  color: var(--color-text);
}
.view-trigger[aria-expanded='true'] {
  border-color: var(--color-accent);
}
.selection-bar {
  display: flex;
  flex: none;
  align-items: center;
  gap: 4px;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-pane);
  padding: 6px 14px;
  color: var(--color-text-2);
  font-size: 12px;
}
.selection-count {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-2);
  font-family: var(--font-mono);
}
.selection-action {
  display: inline-flex;
  width: 28px;
  height: 28px;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: 6px;
  color: var(--color-text-2);
}
.selection-action:hover:not(:disabled),
.selection-action[aria-expanded='true'] {
  background: var(--color-hover);
  color: var(--color-text);
}
.selection-action:disabled {
  cursor: default;
  color: var(--color-text-4);
}
.refresh-banner {
  position: absolute;
  top: 0;
  right: 0;
  left: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-pane);
  padding: 7px 14px;
  color: var(--color-text-3);
  font-size: 12px;
  pointer-events: none;
}
.view-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 20;
  width: 180px;
  border: 1px solid var(--color-strong);
  border-radius: 8px;
  background: var(--color-pane);
  padding: 5px;
  box-shadow: 0 20px 50px -14px rgb(0 0 0 / 0.5);
}
.author-submenu {
  position: absolute;
  top: -5px;
  right: calc(100% + 7px);
  width: 220px;
  border: 1px solid var(--color-strong);
  border-radius: 8px;
  background: var(--color-pane);
  box-shadow: 0 20px 50px -14px rgb(0 0 0 / 0.5);
}
.view-menu-label {
  padding: 5px 9px 4px;
  color: var(--color-text-3);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.view-menu-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  border-radius: 6px;
  padding: 7px 9px;
  color: var(--color-text-2);
  font-size: 12.5px;
  text-align: left;
}
.view-menu-item:hover {
  background: var(--color-hover);
  color: var(--color-text);
}
.view-menu-divider {
  height: 1px;
  background: var(--color-row);
  margin: 4px;
}
/* A compact full-bleed strip on a raised background: the tier on the left, its
   row count on the right, padded to land on the row's title and age columns.
   Hueless on purpose — the rows already carry color (kind pills, unread dots).
   Only a bottom rule: the line above it is the preceding row's own border. */
.date-divider {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  border-bottom: 1px solid var(--color-row);
  background: var(--color-pane);
  padding: 7px 16px 7px 18px;
  color: var(--color-text-3);
  font-family: var(--font-mono);
  font-size: 10.5px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.date-count {
  color: var(--color-text-4);
}
.archived-divider {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 7px;
  padding: 8px 14px 6px;
  color: var(--color-text-3);
  font-family: var(--font-mono);
  font-size: 10.5px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  cursor: pointer;
  border-top: 1px solid var(--color-row);
  margin-top: 6px;
}
.archived-divider:hover {
  color: var(--color-text);
}
.state-frame {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 100%;
  padding: 24px;
  text-align: center;
}
.state-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border: 1px solid var(--color-strong);
  border-radius: 12px;
  background: var(--color-chip);
  margin-bottom: 4px;
}
.state-action {
  margin-top: 8px;
  padding: 6px 14px;
  border: 1px solid var(--color-strong);
  border-radius: 7px;
  color: var(--color-text-2);
  font-size: 12px;
  cursor: pointer;
}
.state-action:hover {
  border-color: var(--color-accent);
  color: var(--color-text);
}
</style>
