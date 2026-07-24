<script setup lang="ts">
import { computed } from 'vue'
import SourceMark from './SourceMark.vue'
import { relativeAge } from '../lib/age'
import { bodySnippet, feedSource, githubPayload, typeLabel } from '../lib/feedPresentation'
import type { InboxItem } from '../types/feed'

const props = defineProps<{ item: InboxItem; archived?: boolean; trash?: boolean; selected: boolean }>()
const emit = defineEmits<{ select: [] }>()
const source = computed(() => feedSource(props.item))
const github = computed(() => githubPayload(props.item))
const type = computed(() => typeLabel(github.value.kind))
const snippet = computed(() => bodySnippet(github.value.body))
const typePillClass = computed(() => github.value.kind === 'PR' ? 'type-pill-pr' : github.value.kind === 'Issue' ? 'type-pill-issue' : 'type-pill-neutral')
</script>

<template>
  <button class="feed-item" :class="{ selected }" :data-id="item.externalId" :data-inbox-id="item.id" data-testid="feed-item" @click="emit('select')">
    <div class="relative flex items-start gap-3">
      <span class="source-badge" :data-source="source.key" data-testid="source-badge"><SourceMark :source="source" class="size-4" /></span>
      <div class="min-w-0 flex-1">
        <div class="flex items-baseline gap-2.5"><div class="min-w-0 flex-1 truncate text-left text-[13.5px] leading-[1.35]" :class="item.unread ? 'font-semibold text-text' : 'font-normal text-text-2'">{{ item.title }}</div><div class="flex shrink-0 items-center gap-2"><span v-if="item.unread" data-testid="unread-dot" class="unread-dot" /><span class="font-mono text-[11px] text-text-4">{{ relativeAge(item.lastEventAt) }}</span></div></div>
        <div class="mt-[5px] flex min-w-0 items-center gap-2"><span v-if="archived && item.archivedReason" class="type-pill type-pill-neutral" data-testid="archive-reason">{{ item.archivedReason }}</span><span v-if="trash && item.ignoredAt != null" class="type-pill type-pill-neutral" data-testid="ignored-pill">ignored</span><span class="type-pill" :class="typePillClass" data-testid="type-pill" :data-kind="github.kind">{{ type }}</span><span class="min-w-0 truncate font-mono text-[11px] text-text-3">{{ source.label }} · {{ github.repo }} #{{ github.num }}</span></div>
        <div v-if="github.author || snippet" class="mt-[5px] truncate text-left text-[12px] leading-[1.4] text-text-3" data-testid="item-snippet"><span v-if="github.author" class="text-text-2">{{ github.author }}</span><template v-if="github.author && snippet"> — </template>{{ snippet }}</div>
      </div>
    </div>
  </button>
</template>

<style scoped>
.feed-item { position: relative; width: 100%; padding: 13px 16px 13px 18px; border-bottom: 1px solid var(--color-row); cursor: pointer; text-align: left; }
.feed-item:hover { background: var(--color-row-hover); }.feed-item.selected::after { content: ''; position: absolute; inset: 0; background: var(--color-selection); pointer-events: none; }.unread-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--color-accent); }.source-badge { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 30px; height: 30px; margin-top: 1px; border-radius: 8px; background: var(--color-chip); border: 1px solid var(--color-strong); color: var(--color-text); }.type-pill { display: inline-flex; flex: none; align-items: center; border-radius: 4px; padding: 2px 7px; font-family: var(--font-mono); font-size: 10px; font-weight: 600; letter-spacing: .02em; }.type-pill-pr { background: var(--color-kind-pr-tint); color: var(--color-kind-pr); }.type-pill-issue { background: var(--color-kind-issue-tint); color: var(--color-kind-issue); }.type-pill-neutral { background: var(--color-chip); color: var(--color-text-2); }
</style>
