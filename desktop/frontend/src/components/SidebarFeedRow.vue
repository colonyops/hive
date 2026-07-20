<script setup lang="ts">
// One feed entry in the sidebar's FEEDS section. Presentational: it renders a
// feed and emits select/reveal, but drag-and-drop and grouping are the parent
// SideBar's concern (the parent wraps this row in a draggable drop zone).
import IconGitBranch from '~icons/lucide/git-branch'
import IconShare2 from '~icons/lucide/share-2'
import type { FeedSummary } from '../types/feed'

defineProps<{ feed: FeedSummary; selected: boolean }>()
const emit = defineEmits<{ select: []; reveal: [] }>()
</script>

<template>
  <button
    type="button"
    class="sidebar-entry"
    :class="{ 'sidebar-entry-selected': selected }"
    data-testid="sidebar-feed"
    :data-id="feed.id"
    @click="emit('select')"
  >
    <span class="nav-icon"><IconGitBranch class="size-3" /></span>
    <span class="min-w-0 flex-1 truncate text-left">{{ feed.name }}</span>
    <span
      class="feed-reveal flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-md text-accent hover:bg-accent/20"
      title="Reveal in flow"
      data-testid="sidebar-reveal-in-flow"
      @click.stop="emit('reveal')"
    ><IconShare2 class="size-3" /></span>
    <span class="font-mono text-[11px]" :class="feed.newCount ? 'text-accent' : 'text-text-3'">{{ feed.newCount || feed.count }}</span>
  </button>
</template>

<style scoped>
.sidebar-entry { display: flex; align-items: center; gap: 9px; width: 100%; padding: 7px 8px; border-radius: 7px; color: var(--color-text-2); font-size: 13px; cursor: pointer; }
.sidebar-entry:hover { background: var(--color-chip); color: var(--color-text); }
.sidebar-entry-selected { background: var(--color-hover); color: var(--color-accent); font-weight: 500; }
/* Reveal-in-flow holds its space (opacity, not display) so hovering never shifts the count badge — same pattern the old feed-edit pencil used. */
.feed-reveal { display: inline-flex; opacity: 0; }
.sidebar-entry:hover .feed-reveal, .feed-reveal:focus-visible { opacity: 1; }
.sidebar-entry-selected .nav-icon { border-color: var(--color-accent-tint); color: var(--color-accent); }
.nav-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-app); color: var(--color-text-2); }
</style>
