<script setup lang="ts">
import { onClickOutside } from '@vueuse/core'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import FeedListItem from './FeedListItem.vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconGitBranch from '~icons/lucide/git-branch'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconSearch from '~icons/lucide/search'
import IconSlidersHorizontal from '~icons/lucide/sliders-horizontal'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import type { FeedItem, FeedSort } from '../types/feed'

// Presentation-only: the store (useFeedState) owns the search text and the
// filtered `visibleItems`, so keyboard navigation and this list render the
// exact same set. This component just renders and relays intent.
const props = defineProps<{
  title: string
  visibleItems: FeedItem[]
  selectedId: string | null
  unreadOnly: boolean
  unreadCount: number
  search: string
  sort: FeedSort
  loadError: string | null
}>()
const emit = defineEmits<{
  select: [id: string]
  'set-unread': [value: boolean]
  refresh: []
  'update:search': [value: string]
  'set-sort': [value: FeedSort]
}>()

const sortOptions: { value: FeedSort; label: string }[] = [
  { value: 'newest', label: 'Newest' },
  { value: 'oldest', label: 'Oldest' },
  { value: 'unread', label: 'Unread first' },
]

const viewMenu = ref<HTMLElement | null>(null)
const viewMenuOpen = ref(false)
const activeViewOptionCount = computed(() => props.sort === 'newest' ? 0 : 1)

function closeViewMenu(): void { viewMenuOpen.value = false }
function chooseSort(value: FeedSort): void { emit('set-sort', value); closeViewMenu() }
function refreshFromMenu(): void { emit('refresh'); closeViewMenu() }
function onDocumentKeydown(event: KeyboardEvent): void {
  if (viewMenuOpen.value && event.key === 'Escape') closeViewMenu()
}

onClickOutside(viewMenu, () => { if (viewMenuOpen.value) closeViewMenu() })
onMounted(() => document.addEventListener('keydown', onDocumentKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onDocumentKeydown))

// Keep the selected row in view when navigation moves the cursor by keyboard
// (mirrors CommandPalette's scrollIntoView on selection change).
const listContainer = ref<HTMLElement | null>(null)
watch(() => props.selectedId, async (id) => {
  if (!id) return
  await nextTick()
  const rows = listContainer.value?.querySelectorAll('[data-testid="feed-item"]')
  const row = rows && Array.from(rows).find((el) => el.getAttribute('data-id') === id)
  ;(row as HTMLElement | undefined)?.scrollIntoView?.({ block: 'nearest' })
})
</script>

<template>
  <section class="feed-list flex min-w-0 flex-[1.25] flex-col border-r border-border">
    <!-- Top row: search + list-level All/Unread filter. No restated title —
         the sidebar already shows the active source. -->
    <header class="flex h-[46px] shrink-0 items-center gap-2.5 border-b border-border bg-pane px-3.5">
      <label class="search-box">
        <IconSearch class="size-[14px] shrink-0 text-text-3" />
        <input
          :value="search"
          type="text"
          class="search-input"
          placeholder="Search items, sources, people…"
          data-testid="feed-search"
          @input="emit('update:search', ($event.target as HTMLInputElement).value)"
        >
      </label>
      <div class="segmented" role="group" aria-label="Filter">
        <button class="seg" :class="{ active: !unreadOnly }" data-testid="filter-all" @click="emit('set-unread', false)">All</button>
        <button class="seg" :class="{ active: unreadOnly }" data-testid="filter-unread" @click="emit('set-unread', true)">
          Unread<span class="seg-count">{{ unreadCount }}</span>
        </button>
      </div>
      <div ref="viewMenu" class="relative shrink-0">
        <button
          type="button"
          class="view-trigger"
          data-testid="view-menu-toggle"
          aria-haspopup="menu"
          :aria-expanded="viewMenuOpen"
          @click="viewMenuOpen = !viewMenuOpen"
        >
          <IconSlidersHorizontal class="size-3.5" />
          <span>View</span>
          <span v-if="activeViewOptionCount" class="view-count" data-testid="view-active-count">{{ activeViewOptionCount }}</span>
          <IconChevronDown class="size-3.5 text-text-3 transition-transform" :class="{ 'rotate-180': viewMenuOpen }" />
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
            <IconCheck class="size-3.5" :class="option.value === sort ? 'text-accent' : 'opacity-0'" :stroke-width="3" />
            <span>{{ option.label }}</span>
          </button>
          <div class="view-menu-divider" />
          <button type="button" class="view-menu-item" role="menuitem" data-testid="view-menu-refresh" @click="refreshFromMenu">
            <IconRefreshCw class="size-3.5 text-text-3" />
            <span>Refresh</span>
          </button>
        </div>
      </div>
    </header>
    <div ref="listContainer" class="hive-scroll min-h-0 flex-1 overflow-y-auto">
      <!-- Load failure: the "GitHub unreachable" design state. -->
      <div v-if="loadError" class="state-frame" data-testid="feed-error">
        <div class="state-icon text-accent"><IconTriangleAlert class="size-5" /></div>
        <div class="text-[13.5px] font-semibold">GitHub unreachable</div>
        <div class="max-w-[240px] text-xs leading-relaxed text-text-3">{{ loadError }}</div>
        <button class="state-action" @click="emit('refresh')">Retry now</button>
      </div>
      <template v-else>
        <FeedListItem
          v-for="item in visibleItems"
          :key="item.id"
          :item="item"
          :selected="item.id === selectedId"
          @select="emit('select', item.id)"
        />
        <!-- Empty feed: "You're all caught up" when the unread filter drained
             the list, "No matches" when a search did, a plain empty state otherwise. -->
        <div v-if="visibleItems.length === 0" class="state-frame" data-testid="feed-empty">
          <template v-if="search.trim()">
            <div class="state-icon text-text-3"><IconSearch class="size-5" /></div>
            <div class="text-[13.5px] font-semibold">No matches</div>
            <div class="max-w-[240px] text-xs leading-relaxed text-text-3">Nothing here matches "{{ search.trim() }}". Try a different search.</div>
          </template>
          <template v-else-if="unreadOnly">
            <div class="state-icon text-kind-pr"><IconCheck class="size-5" /></div>
            <div class="text-[13.5px] font-semibold">You're all caught up</div>
            <div class="max-w-[240px] text-xs leading-relaxed text-text-3">No unread items in {{ title === 'Unread' ? 'this workspace' : title }}. New items will show up here as they arrive.</div>
          </template>
          <template v-else>
            <div class="state-icon text-text-3"><IconGitBranch class="size-5" /></div>
            <div class="text-[13.5px] font-semibold">No items yet</div>
            <div class="max-w-[240px] text-xs leading-relaxed text-text-3">New items will show up here as they arrive.</div>
          </template>
          <button v-if="!search.trim()" class="state-action" @click="emit('refresh')">Refresh now</button>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.feed-list { background: var(--color-list); }
.search-box { display: flex; min-width: 0; flex: 1; align-items: center; gap: 8px; border: 1px solid var(--color-strong); border-radius: 8px; background: var(--color-app); padding: 6px 11px; }
.search-box:focus-within { border-color: var(--color-accent); }
.search-input { min-width: 0; flex: 1; border: none; background: none; color: var(--color-text); font-family: var(--font-sans); font-size: 13px; outline: none; }
.search-input::placeholder { color: var(--color-text-4); }
.segmented { display: flex; flex: none; align-items: center; gap: 2px; border: 1px solid var(--color-strong); border-radius: 8px; background: var(--color-app); padding: 2px; }
.seg { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; border-radius: 6px; padding: 4px 11px; color: var(--color-text-2); font-size: 12px; font-weight: 500; }
.seg:hover:not(.active) { color: var(--color-text); }
.seg.active { background: var(--color-accent); color: var(--color-accent-contrast); }
.seg-count { font-family: var(--font-mono); font-size: 10px; opacity: .85; }
.view-trigger { display: inline-flex; height: 32px; align-items: center; gap: 6px; cursor: pointer; border: 1px solid var(--color-strong); border-radius: 8px; padding: 0 9px; color: var(--color-text-2); font-size: 12px; font-weight: 500; }
.view-trigger:hover, .view-trigger[aria-expanded="true"] { color: var(--color-text); }
.view-trigger[aria-expanded="true"] { border-color: var(--color-accent); }
.view-count { display: inline-flex; min-width: 16px; height: 16px; align-items: center; justify-content: center; border-radius: 999px; background: var(--color-accent); padding: 0 4px; color: var(--color-accent-contrast); font-family: var(--font-mono); font-size: 9px; font-weight: 600; }
.view-menu { position: absolute; top: calc(100% + 6px); right: 0; z-index: 20; width: 170px; border: 1px solid var(--color-strong); border-radius: 8px; background: var(--color-pane); padding: 5px; box-shadow: 0 20px 50px -14px rgb(0 0 0 / .5); }
.view-menu-label { padding: 5px 9px 4px; color: var(--color-text-3); font-size: 10px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; }
.view-menu-item { display: flex; width: 100%; align-items: center; gap: 8px; cursor: pointer; border-radius: 6px; padding: 7px 9px; color: var(--color-text-2); font-size: 12.5px; text-align: left; }
.view-menu-item:hover { background: var(--color-hover); color: var(--color-text); }
.view-menu-divider { height: 1px; background: var(--color-row); margin: 4px; }
.state-frame { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; height: 100%; padding: 24px; text-align: center; }
.state-icon { display: flex; align-items: center; justify-content: center; width: 44px; height: 44px; border: 1px solid var(--color-strong); border-radius: 12px; background: var(--color-chip); margin-bottom: 4px; }
.state-action { margin-top: 8px; padding: 6px 14px; border: 1px solid var(--color-strong); border-radius: 7px; color: var(--color-text-2); font-size: 12px; cursor: pointer; }
.state-action:hover { border-color: var(--color-accent); color: var(--color-text); }
</style>
