<script setup lang="ts">
import type { FeedItem } from '../types/feed'

defineProps<{ item: FeedItem; selected: boolean }>()
const emit = defineEmits<{ select: [] }>()
</script>

<template>
  <button class="feed-item" :class="{ selected }" :data-id="item.id" data-testid="feed-item" @click="emit('select')">
    <div class="relative flex items-start gap-2.5">
      <span class="source-icon mt-px">⌘</span>
      <div class="min-w-0 flex-1">
        <div class="mb-1 flex items-center gap-2">
          <span class="kind-badge" data-testid="kind-badge" :class="item.kind === 'PR' ? 'text-kind-pr border-kind-pr' : 'text-kind-issue border-kind-issue'">{{ item.kind }}</span>
          <span class="font-mono text-[11.5px] text-text-3">{{ item.repo }} #{{ item.num }}</span>
          <span class="flex-1" />
          <span v-if="item.unread" class="size-[7px] shrink-0 rounded-full bg-accent" />
          <span class="font-mono text-[11px] text-text-4">{{ item.age }}</span>
        </div>
        <div class="mb-1.5 text-left text-[13.5px] leading-[1.35] text-zinc-100">{{ item.title }}</div>
        <div class="flex flex-wrap items-center gap-1.5">
          <span class="text-[11.5px] text-text-3">{{ item.author }}</span>
          <span v-for="label in item.labels ?? []" :key="label" class="rounded border border-card bg-chip px-1.5 py-px font-mono text-[10px] text-text-2">{{ label }}</span>
        </div>
      </div>
    </div>
  </button>
</template>

<style scoped>
.feed-item { position: relative; width: 100%; padding: 13px 16px; border-bottom: 1px solid #141417; cursor: pointer; text-align: left; }
.feed-item:hover { background: #0f0f12; }
.feed-item.selected { border-left: 2px solid var(--color-accent); padding-left: 14px; background: rgba(245, 158, 11, .07); }
.source-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 22px; height: 22px; border: 1px solid var(--color-strong); border-radius: 6px; background: var(--color-chip); color: #e4e4e7; font-family: var(--font-mono); font-size: 9px; font-weight: 700; }
.kind-badge { border-width: 1px; border-radius: 3px; padding: 1px 5px; font-family: var(--font-mono); font-size: 9px; letter-spacing: .06em; opacity: .95; }
</style>
