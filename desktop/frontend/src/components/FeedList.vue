<script setup lang="ts">
import { computed } from 'vue'
import FeedListItem from './FeedListItem.vue'
import IconCheck from '~icons/lucide/check'
import IconGitBranch from '~icons/lucide/git-branch'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import type { FeedItem } from '../types/feed'

const props = defineProps<{
  title: string
  items: FeedItem[]
  selectedId: string | null
  unreadOnly: boolean
  countLabel: string
  loadError: string | null
}>()
const emit = defineEmits<{ select: [id: string]; 'toggle-unread': []; refresh: [] }>()
const visibleItems = computed(() => props.unreadOnly ? props.items.filter((item) => item.unread) : props.items)
</script>

<template>
  <section class="feed-list flex min-w-0 flex-[1.25] flex-col border-r border-border">
    <header class="flex h-[46px] shrink-0 items-center gap-2.5 border-b border-border bg-pane px-4">
      <span class="source-icon"><IconGitBranch class="size-3" /></span>
      <span class="text-[13px] font-semibold" data-testid="feed-title">{{ title }}</span>
      <span class="font-mono text-[11px] text-text-3">{{ countLabel }}</span>
      <span class="flex-1" />
      <div class="flex gap-1.5">
        <button class="unread-chip" data-testid="unread-chip" :class="{ active: unreadOnly }" @click="emit('toggle-unread')">Unread</button>
        <button class="refresh-chip" aria-label="Refresh" @click="emit('refresh')"><IconRefreshCw class="size-3" /></button>
      </div>
    </header>
    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto">
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
             the list, a plain empty state otherwise. -->
        <div v-if="visibleItems.length === 0" class="state-frame" data-testid="feed-empty">
          <template v-if="unreadOnly">
            <div class="state-icon text-kind-pr"><IconCheck class="size-5" /></div>
            <div class="text-[13.5px] font-semibold">You're all caught up</div>
            <div class="max-w-[240px] text-xs leading-relaxed text-text-3">No unread items in {{ title === 'Unread' ? 'this workspace' : title }}. New PRs and issues will show up here as they arrive.</div>
          </template>
          <template v-else>
            <div class="state-icon text-text-3"><IconGitBranch class="size-5" /></div>
            <div class="text-[13.5px] font-semibold">No items yet</div>
            <div class="max-w-[240px] text-xs leading-relaxed text-text-3">New PRs and issues will show up here as they arrive.</div>
          </template>
          <button class="state-action" @click="emit('refresh')">Refresh now</button>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.feed-list { background: var(--color-list); }
.source-icon { display: inline-flex; align-items: center; justify-content: center; width: 20px; height: 20px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-chip); color: var(--color-text-2); }
.unread-chip, .refresh-chip { cursor: pointer; border: 1px solid var(--color-card); border-radius: 5px; color: var(--color-text-2); font-size: 11px; }
.unread-chip { padding: 3px 9px; }
.unread-chip.active { border-color: var(--color-accent); background: var(--color-accent); color: var(--color-accent-contrast); font-weight: 600; }
.refresh-chip { display: inline-flex; align-items: center; padding: 3px 9px; }
.refresh-chip:hover, .unread-chip:not(.active):hover { border-color: var(--color-strong); color: var(--color-text); }
.state-frame { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; height: 100%; padding: 24px; text-align: center; }
.state-icon { display: flex; align-items: center; justify-content: center; width: 44px; height: 44px; border: 1px solid var(--color-strong); border-radius: 12px; background: var(--color-chip); margin-bottom: 4px; }
.state-action { margin-top: 8px; padding: 6px 14px; border: 1px solid var(--color-strong); border-radius: 7px; color: var(--color-text-2); font-size: 12px; cursor: pointer; }
.state-action:hover { border-color: var(--color-accent); color: var(--color-text); }
</style>
