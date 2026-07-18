<script setup lang="ts">
import IconCircleDot from '~icons/lucide/circle-dot'
import IconGitBranch from '~icons/lucide/git-branch'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import type { FeedItem } from '../types/feed'

defineProps<{ item: FeedItem; selected: boolean }>()
const emit = defineEmits<{ select: [] }>()
</script>

<template>
  <button class="feed-item" :class="{ selected }" :data-id="item.id" data-testid="feed-item" @click="emit('select')">
    <div class="flex items-start gap-3">
      <span class="kind-icon mt-px" :class="item.kind === 'PR' ? 'kind-icon-pr' : 'kind-icon-issue'" data-testid="kind-badge" :data-kind="item.kind">
        <IconGitPullRequest v-if="item.kind === 'PR'" class="size-[15px]" />
        <IconCircleDot v-else class="size-[15px]" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="mb-[5px] text-left text-[13.5px] font-medium leading-[1.35] text-text">{{ item.title }}</div>
        <div class="flex items-center gap-[7px] text-text-3">
          <IconGitBranch class="size-[11px] shrink-0" />
          <span class="font-mono text-[11.5px]">{{ item.repo }} #{{ item.num }}</span>
          <span class="text-text-4">·</span>
          <span class="text-[11.5px]">{{ item.author }}</span>
        </div>
      </div>
      <div class="flex shrink-0 flex-col items-end gap-[7px]">
        <div class="flex items-center gap-2">
          <span class="font-mono text-[11px] text-text-4">{{ item.age }}</span>
          <span v-if="item.unread" data-testid="unread-dot" class="size-[7px] shrink-0 rounded-full bg-accent" />
        </div>
        <div class="flex max-w-[150px] flex-wrap items-center justify-end gap-[5px]">
          <span v-for="label in item.labels ?? []" :key="label" class="rounded border border-card bg-chip px-1.5 py-px font-mono text-[10px] text-text-2">{{ label }}</span>
        </div>
      </div>
    </div>
  </button>
</template>

<style scoped>
.feed-item { position: relative; width: 100%; padding: 12px 16px; border-bottom: 1px solid var(--color-row); cursor: pointer; text-align: left; }
.feed-item:hover { background: var(--color-row-hover); }
.feed-item.selected::after { content: ''; position: absolute; inset: 0; background: var(--color-selection); pointer-events: none; }
.kind-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 28px; height: 28px; border-radius: 7px; }
.kind-icon-pr { background: var(--color-kind-pr-tint); color: var(--color-kind-pr); }
.kind-icon-issue { background: var(--color-kind-issue-tint); color: var(--color-kind-issue); }
</style>
