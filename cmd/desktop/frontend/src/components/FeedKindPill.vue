<script setup lang="ts">
import type { KindStyle } from '../lib/itemPresentation'

// The item-kind label shared by the feed row and the detail pane. `sm` is the
// row's mono tag; `md` is the detail header's pill, which also carries the
// kind's glyph in its slot.
withDefaults(defineProps<{ tone?: KindStyle; size?: 'sm' | 'md' }>(), { tone: 'neutral', size: 'sm' })
</script>

<template>
  <span
    :data-tone="tone"
    :class="[
      'inline-flex shrink-0 items-center whitespace-nowrap font-semibold',
      size === 'sm'
        ? 'rounded-sm px-1.75 py-0.5 font-mono text-micro tracking-[.02em]'
        : 'h-5.5 gap-1.5 rounded-md pl-1.75 pr-2.25 text-caption',
      {
        pr: 'bg-kind-pr-tint text-kind-pr',
        issue: 'bg-kind-issue-tint text-kind-issue',
        neutral: 'bg-chip text-text-2',
      }[tone],
    ]"
    ><slot
  /></span>
</template>
