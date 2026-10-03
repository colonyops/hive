<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import IconButton from './ui/IconButton.vue'
import AppMenu from './ui/AppMenu.vue'
import SearchField from './ui/SearchField.vue'
import FeedListItem from './FeedListItem.vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
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
import Spinner from './ui/Spinner.vue'
import SegmentedControl, { type SegmentedControlOption } from './ui/SegmentedControl.vue'

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

const trashFilterOptions: SegmentedControlOption<'all' | 'ignored'>[] = [
  { value: 'all', label: 'All' },
  { value: 'ignored', label: 'Ignored' },
]
const unreadFilterOptions: SegmentedControlOption<'all' | 'unread'>[] = [
  { value: 'all', label: 'All' },
  { value: 'unread', label: 'Unread' },
]

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

const viewMenuOpen = ref(false)
const viewMenuWrap = ref<HTMLElement | null>(null)
const authorSubmenu = computed<MenuEntry>(() => ({
  kind: 'submenu',
  id: 'author',
  label: 'Author',
  icon: IconUserRound,
  testid: 'view-author-filter',
  panelTestid: 'view-author-submenu',
  search: { label: 'Search authors', testid: 'view-author-search' },
  entries: [
    { kind: 'action', id: 'author:', label: 'All authors', checked: !props.authorFilter, testid: 'view-author-all' },
    ...props.authors.map<MenuEntry>((author) => ({
      kind: 'action',
      id: `author:${author}`,
      label: author,
      checked: author === props.authorFilter,
      testid: 'view-author-option',
    })),
  ],
}))

const viewMenuEntries = computed<MenuEntry[]>(() => {
  const entries: MenuEntry[] = [
    { kind: 'label', text: 'Sort by' },
    ...sortOptions.map<MenuEntry>((option) => ({
      kind: 'action',
      id: `sort:${option.value}`,
      label: option.label,
      checked: option.value === props.sort,
      testid: `view-sort-${option.value}`,
    })),
  ]
  if (props.authors.length || props.authorFilter) {
    entries.push({ kind: 'separator' }, { kind: 'label', text: 'Filter by' }, authorSubmenu.value)
  }
  entries.push(
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'select-items',
      label: 'Select items',
      icon: IconSquareCheckBig,
      testid: 'view-menu-select-items',
    },
  )
  // Trash has no unread semantics, so it gets no mark-all-read entry at all.
  if (!props.trash) {
    entries.push({
      kind: 'action',
      id: 'mark-read',
      label: 'Mark all as read',
      icon: IconMailCheck,
      testid: 'view-menu-mark-read',
    })
  }
  entries.push({
    kind: 'action',
    id: 'refresh',
    label: props.refreshing ? 'Refreshing…' : 'Refresh',
    icon: IconRefreshCw,
    disabled: props.refreshing,
    testid: 'view-menu-refresh',
  })
  return entries
})
function onViewMenuSelect(id: string): void {
  viewMenuOpen.value = false
  if (id.startsWith('sort:')) emit('set-sort', id.slice('sort:'.length) as FeedSort)
  else if (id.startsWith('author:')) emit('update:author-filter', id.slice('author:'.length))
  else if (id === 'select-items') emit('enter-selection')
  else if (id === 'mark-read') emit('mark-all-read')
  else if (id === 'refresh') emit('refresh')
}

const selectionActionsWrap = ref<HTMLElement | null>(null)
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

// Selected, not just focused: view.focus-search means "start a new search"
// far more often than "edit the old one" — same call TerminalMode's own
// filter field makes on the session-tree half of that command.
const searchInput = ref<{ select: () => void } | null>(null)
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
      <SearchField
        ref="searchInput"
        :model-value="search"
        placeholder="Search items, sources, people…"
        aria-label="Search items"
        testid="feed-search"
        class="min-w-0 flex-1"
        @update:model-value="emit('update:search', $event)"
      />
      <!-- Trash filters by disposition (ignored vs everything); feeds filter
           by unread. Trash carries no unread semantics. -->
      <SegmentedControl
        v-if="trash"
        variant="compact"
        class="shrink-0"
        :model-value="trashFilter"
        :options="trashFilterOptions"
        aria-label="Filter"
        testid="filter-trash"
        @update:model-value="emit('set-trash-filter', $event)"
      />
      <SegmentedControl
        v-else
        variant="compact"
        class="shrink-0"
        :model-value="unreadOnly ? 'unread' : 'all'"
        :options="unreadFilterOptions"
        aria-label="Filter"
        testid="filter"
        @update:model-value="emit('set-unread', $event === 'unread')"
      >
        <template #option="{ option }">
          {{ option.label
          }}<span v-if="option.value === 'unread'" class="font-mono text-micro opacity-85">{{ unreadCount }}</span>
        </template>
      </SegmentedControl>
      <div ref="viewMenuWrap" class="relative shrink-0">
        <IconButton
          label="Feed options"
          :icon="IconEllipsis"
          size="xl"
          variant="outline"
          data-testid="view-menu-toggle"
          aria-haspopup="menu"
          :active="viewMenuOpen"
          :aria-expanded="viewMenuOpen"
          @click="viewMenuOpen = !viewMenuOpen"
        />
        <AppMenu
          v-if="viewMenuOpen"
          :entries="viewMenuEntries"
          width="180px"
          :ignore="[viewMenuWrap]"
          testid="view-menu"
          @close="viewMenuOpen = false"
          @select="onViewMenuSelect"
        />
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
      <IconButton
        label="Copy contents"
        :icon="IconCopy"
        size="lg"
        :disabled="selectedItemIds.length === 0"
        data-testid="selection-copy-contents"
        @click="emit('copy-selection-contents')"
      />
      <div v-if="selectionActions.length" ref="selectionActionsWrap" class="relative">
        <IconButton
          label="Copy with action"
          :icon="IconChevronDown"
          size="lg"
          data-testid="selection-actions-toggle"
          aria-haspopup="menu"
          :active="selectionActionsOpen"
          :aria-expanded="selectionActionsOpen"
          @click="selectionActionsOpen = !selectionActionsOpen"
        />
        <AppMenu
          v-if="selectionActionsOpen"
          :entries="selectionActionEntries"
          :ignore="[selectionActionsWrap]"
          testid="selection-actions-menu"
          @select="chooseSelectionAction"
          @close="selectionActionsOpen = false"
        />
      </div>
      <IconButton
        label="Create session"
        :icon="IconPlus"
        size="lg"
        :disabled="selectedItemIds.length === 0"
        data-testid="selection-create-session"
        @click="emit('create-session-from-selection')"
      />
      <IconButton
        label="Cancel selection"
        :icon="IconX"
        size="lg"
        data-testid="selection-cancel"
        @click="emit('cancel-selection')"
      />
    </div>
    <div class="relative min-h-0 flex-1">
      <div v-if="refreshing" class="refresh-banner" role="status" data-testid="feed-refreshing">
        <Spinner />Refreshing…
      </div>
      <div ref="listContainer" class="hive-scroll h-full overflow-y-auto">
        <!-- Load failure: the "GitHub unreachable" design state. -->
        <div v-if="loadError" class="state-frame" data-testid="feed-error">
          <div class="state-icon text-accent"><IconTriangleAlert class="size-5" /></div>
          <div class="text-body font-semibold">GitHub unreachable</div>
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
              <div class="text-body font-semibold">No matches</div>
              <div v-if="authorFilter" class="max-w-[240px] text-xs leading-relaxed text-text-3">
                No items by {{ authorFilter }} match the current filters.
              </div>
              <div v-else class="max-w-[240px] text-xs leading-relaxed text-text-3">
                Nothing here matches "{{ search.trim() }}". Try a different search.
              </div>
            </template>
            <template v-else-if="unreadOnly">
              <div class="state-icon text-kind-pr"><IconCheck class="size-5" /></div>
              <div class="text-body font-semibold">You're all caught up</div>
              <div class="max-w-[240px] text-xs leading-relaxed text-text-3">
                No unread items in {{ title === 'Unread' ? 'this profile' : title }}. New items will show up here as
                they arrive.
              </div>
            </template>
            <template v-else>
              <div class="state-icon text-text-3"><IconGitBranch class="size-5" /></div>
              <div class="text-body font-semibold">No items yet</div>
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
.selection-bar {
  display: flex;
  flex: none;
  align-items: center;
  gap: 4px;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-pane);
  padding: 6px 14px;
  color: var(--color-text-2);
  font-size: var(--text-small);
}
.selection-count {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-2);
  font-family: var(--font-mono);
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
  font-size: var(--text-small);
  pointer-events: none;
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
  font-size: var(--text-micro);
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
  font-size: var(--text-micro);
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
  border-radius: var(--radius-xl);
  background: var(--color-chip);
  margin-bottom: 4px;
}
.state-action {
  margin-top: 8px;
  padding: 6px 14px;
  border: 1px solid var(--color-strong);
  border-radius: var(--radius-lg);
  color: var(--color-text-2);
  font-size: var(--text-small);
  cursor: pointer;
}
.state-action:hover {
  border-color: var(--color-accent);
  color: var(--color-text);
}
</style>
