<script setup lang="ts">
import { computed } from 'vue'
import FeedListItem from './FeedListItem.vue'
import type { FeedItem } from '../types/feed'

const props = defineProps<{
  title: string
  items: FeedItem[]
  selectedId: string | null
  unreadOnly: boolean
  countLabel: string
}>()
const emit = defineEmits<{ select: [id: string]; 'toggle-unread': []; refresh: [] }>()
const visibleItems = computed(() => props.unreadOnly ? props.items.filter((item) => item.unread) : props.items)
</script>

<template>
  <section class="feed-list flex min-w-0 flex-[1.25] flex-col border-r border-border">
    <header class="flex h-[46px] shrink-0 items-center gap-2.5 border-b border-border bg-raised px-4">
      <span class="source-icon">⌘</span>
      <span class="text-[13px] font-semibold">{{ title }}</span>
      <span class="font-mono text-[11px] text-text-3">{{ countLabel }}</span>
      <span class="flex-1" />
      <div class="flex gap-1.5">
        <button class="unread-chip" data-testid="unread-chip" :class="{ active: unreadOnly }" @click="emit('toggle-unread')">Unread</button>
        <button class="refresh-chip" aria-label="Refresh" @click="emit('refresh')">⟳</button>
      </div>
    </header>
    <div class="hive-scroll flex-1 overflow-y-auto">
      <FeedListItem
        v-for="item in visibleItems"
        :key="item.id"
        :item="item"
        :selected="item.id === selectedId"
        @select="emit('select', item.id)"
      />
    </div>
  </section>
</template>

<style scoped>
.feed-list { background: var(--color-list); }
.source-icon { display: inline-flex; align-items: center; justify-content: center; width: 20px; height: 20px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-chip); color: var(--color-text-2); font-family: var(--font-mono); font-size: 9px; font-weight: 700; }
.unread-chip, .refresh-chip { cursor: pointer; border: 1px solid var(--color-card); border-radius: 5px; color: var(--color-text-2); font-size: 11px; }
.unread-chip { padding: 3px 9px; }
.unread-chip.active { border-color: var(--color-accent); background: var(--color-accent); color: var(--color-app); font-weight: 600; }
.refresh-chip { padding: 3px 9px; }
.refresh-chip:hover, .unread-chip:not(.active):hover { border-color: var(--color-strong); color: var(--color-text); }
</style>
