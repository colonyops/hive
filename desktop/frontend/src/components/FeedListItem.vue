<script setup lang="ts">
import { computed } from 'vue'
import SourceMark from './SourceMark.vue'
import { bodySnippet, feedSource, typeLabel } from '../lib/feedPresentation'
import type { FeedItem } from '../types/feed'

const props = defineProps<{ item: FeedItem; selected: boolean }>()
const emit = defineEmits<{ select: [] }>()

// The inbox row leads with its source (icon badge) and its type (pill); the
// snippet is a plain-text preview of the item's markdown body.
const source = computed(() => feedSource(props.item))
const type = computed(() => typeLabel(props.item.kind))
const snippet = computed(() => bodySnippet(props.item.body))
const typePillClass = computed(() =>
  props.item.kind === 'PR' ? 'type-pill-pr' : props.item.kind === 'Issue' ? 'type-pill-issue' : 'type-pill-neutral',
)
</script>

<template>
  <button class="feed-item" :class="{ selected }" :data-id="item.id" data-testid="feed-item" @click="emit('select')">
    <div class="relative flex items-start gap-3">
      <span class="source-badge" :data-source="source.key" data-testid="source-badge">
        <SourceMark :source="source" class="size-4" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="flex items-baseline gap-2.5">
          <div
            class="min-w-0 flex-1 truncate text-left text-[13.5px] leading-[1.35]"
            :class="item.unread ? 'font-semibold text-text' : 'font-normal text-text-2'"
          >{{ item.title }}</div>
          <div class="flex shrink-0 items-center gap-2">
            <!-- Unread reads as a small accent dot by the timestamp, email-inbox style. -->
            <span v-if="item.unread" data-testid="unread-dot" class="unread-dot" />
            <span class="font-mono text-[11px] text-text-4">{{ item.age }}</span>
          </div>
        </div>
        <div class="mt-[5px] flex min-w-0 items-center gap-2">
          <span class="type-pill" :class="typePillClass" data-testid="type-pill" :data-kind="item.kind">{{ type }}</span>
          <span class="min-w-0 truncate font-mono text-[11px] text-text-3">{{ source.label }} · {{ item.repo }} #{{ item.num }}</span>
        </div>
        <div v-if="item.author || snippet" class="mt-[5px] truncate text-left text-[12px] leading-[1.4] text-text-3" data-testid="item-snippet">
          <span v-if="item.author" class="text-text-2">{{ item.author }}</span><template v-if="item.author && snippet"> — </template>{{ snippet }}
        </div>
      </div>
    </div>
  </button>
</template>

<style scoped>
.feed-item { position: relative; width: 100%; padding: 13px 16px 13px 18px; border-bottom: 1px solid var(--color-row); cursor: pointer; text-align: left; }
.feed-item:hover { background: var(--color-row-hover); }
.feed-item.selected::after { content: ''; position: absolute; inset: 0; background: var(--color-selection); pointer-events: none; }
.unread-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--color-accent); }
.source-badge { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 30px; height: 30px; margin-top: 1px; border-radius: 8px; background: var(--color-chip); border: 1px solid var(--color-strong); color: var(--color-text); }
.type-pill { display: inline-flex; flex: none; align-items: center; border-radius: 4px; padding: 2px 7px; font-family: var(--font-mono); font-size: 10px; font-weight: 600; letter-spacing: .02em; }
.type-pill-pr { background: var(--color-kind-pr-tint); color: var(--color-kind-pr); }
.type-pill-issue { background: var(--color-kind-issue-tint); color: var(--color-kind-issue); }
.type-pill-neutral { background: var(--color-chip); color: var(--color-text-2); }
</style>
