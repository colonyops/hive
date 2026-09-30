<script setup lang="ts">
import { computed } from 'vue'
import { feedIconComponent } from '../lib/feedIcons'
import type { FeedSummary } from '../types/feed'

const props = defineProps<{ feed: FeedSummary; selected: boolean }>()
const emit = defineEmits<{ select: [] }>()

const icon = computed(() => feedIconComponent(props.feed.icon))
const tooltip = computed(() => props.feed.description || props.feed.name)
</script>

<template>
  <div
    class="sidebar-entry"
    :class="{ 'sidebar-entry-selected': selected }"
    role="button"
    tabindex="0"
    data-testid="sidebar-feed"
    :data-id="feed.id"
    :title="tooltip"
    @click="emit('select')"
    @keydown.enter.prevent="emit('select')"
    @keydown.space.prevent="emit('select')"
  >
    <span class="nav-icon"><component :is="icon" class="size-3" /></span>
    <span class="min-w-0 flex-1 truncate text-left">{{ feed.name }}</span>
    <span class="font-mono text-[11px]" :class="feed.newCount ? 'text-accent' : 'text-text-3'">{{ feed.newCount || feed.count }}</span>
  </div>
</template>

<style scoped>
.sidebar-entry { position: relative; display: flex; align-items: center; gap: 9px; width: 100%; padding: 7px 8px; border-radius: 7px; color: var(--color-text-2); font-size: 13px; cursor: pointer; }
.sidebar-entry:hover { background: var(--color-chip); color: var(--color-text); }
.sidebar-entry:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.sidebar-entry-selected { background: var(--color-hover); color: var(--color-accent); font-weight: 500; }
.sidebar-entry-selected .nav-icon { border-color: var(--color-accent-tint); color: var(--color-accent); }
.nav-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-app); color: var(--color-text-2); }
</style>
