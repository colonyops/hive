<script setup lang="ts">
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCircle from '~icons/lucide/circle'
import IconGitBranch from '~icons/lucide/git-branch'
import IconList from '~icons/lucide/list'
import IconRss from '~icons/lucide/rss'
import IconSettings from '~icons/lucide/settings'
import IconShare2 from '~icons/lucide/share-2'
import IconWorkflow from '~icons/lucide/workflow'
import PanelResizeHandle from './PanelResizeHandle.vue'
import { useResizablePanel } from '../composables/useResizablePanel'
import type { Profile, SidebarSelection } from '../types/feed'

const props = defineProps<{ profile: Profile; selection: SidebarSelection; unreadOnly: boolean; flowsDirty?: boolean }>()
const emit = defineEmits<{
  select: [sel: SidebarSelection]
  'select-unread': []
  'open-flows': []
  'open-settings': []
  'reveal-in-flow': [feedId: string]
}>()

const { size, startResize, step } = useResizablePanel({
  storageKey: 'hive.panel.sidebar',
  defaultSize: 250,
  min: 190,
  max: 480,
  edge: 'right',
})

// "All items" and "Unread" are both all-scope; the unread filter picks
// which entry lights up. A feed entry highlights regardless of the filter.
function allSelected(): boolean {
  return props.selection.type === 'all' && !props.unreadOnly
}

function unreadSelected(): boolean {
  return props.selection.type === 'all' && props.unreadOnly
}

function feedSelected(feedId: string): boolean {
  return props.selection.type === 'feed' && props.selection.feedId === feedId
}
</script>

<template>
  <aside class="hive-scroll relative flex shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar" :style="{ width: size + 'px' }">
    <div class="profile-header border-b border-border px-4 pb-3 pt-4" data-testid="sidebar-profile-header">
      <div class="flex items-center gap-2">
        <div class="min-w-0 flex-1 truncate text-[15px] font-semibold tracking-[-.01em]" data-testid="sidebar-profile-name">{{ profile.name }}</div>
        <button
          class="settings-button flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md text-text-3 hover:bg-chip hover:text-text"
          title="Profile settings"
          aria-label="Profile settings"
          data-testid="sidebar-open-settings"
          @click="emit('open-settings')"
        ><IconSettings class="size-3.5" /></button>
      </div>
      <div class="mt-1 flex items-center gap-1.5">
        <span class="flex size-[15px] items-center justify-center rounded border border-strong bg-chip text-text-2"><IconGitBranch class="size-2.5" /></span>
        <span class="text-xs text-text-3">{{ profile.sourceSummary }}</span>
      </div>
    </div>

    <div class="px-2.5 pb-0.5 pt-3">
      <button class="sidebar-entry" :class="{ 'sidebar-entry-selected': allSelected() }" @click="emit('select', { type: 'all' })">
        <span class="nav-icon border-accent-tint text-accent"><IconList class="size-3" /></span><span class="flex-1 text-left">All items</span><span class="font-mono text-[11px]">{{ profile.totalCount }}</span>
      </button>
      <button class="sidebar-entry" data-testid="sidebar-unread" :class="{ 'sidebar-entry-selected': unreadSelected() }" @click="emit('select-unread')">
        <span class="nav-icon"><IconCircle class="size-3" /></span><span class="flex-1 text-left">Unread</span><span class="size-[7px] rounded-full bg-accent" /><span class="ml-[7px] font-mono text-[11px] text-text-3">{{ profile.unreadCount }}</span>
      </button>
    </div>

    <section class="px-2.5 pb-1.5 pt-2">
      <div class="section-label">
        <IconRss class="size-3 text-feeds" />FEEDS
      </div>
      <button
        v-for="feed in profile.feeds ?? []"
        :key="feed.id"
        type="button"
        class="sidebar-entry"
        :class="{ 'sidebar-entry-selected': feedSelected(feed.id) }"
        data-testid="sidebar-feed"
        :data-id="feed.id"
        @click="emit('select', { type: 'feed', feedId: feed.id })"
      >
        <span class="nav-icon"><IconGitBranch class="size-3" /></span><span class="min-w-0 flex-1 truncate text-left">{{ feed.name }}</span>
        <span
          class="feed-reveal flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-md text-accent hover:bg-accent/20"
          title="Reveal in flow"
          data-testid="sidebar-reveal-in-flow"
          @click.stop="emit('reveal-in-flow', feed.id)"
        ><IconShare2 class="size-3" /></span>
        <span class="font-mono text-[11px]" :class="feed.newCount ? 'text-accent' : 'text-text-3'">{{ feed.newCount || feed.count }}</span>
      </button>
    </section>

    <button
      class="mt-auto flex items-center gap-2.5 border-t border-border p-2.5 text-left hover:bg-chip"
      data-testid="sidebar-edit-flow"
      @click="emit('open-flows')"
    >
      <span class="flex size-[22px] shrink-0 items-center justify-center rounded-md border border-dashed border-card bg-app text-accent"><IconWorkflow class="size-3" /></span>
      <span class="min-w-0 flex-1">
        <span class="block text-[12.5px] font-semibold text-text">Edit flow</span>
        <span class="block truncate font-mono text-[11px] text-text-3">Open editor</span>
      </span>
      <span
        v-if="flowsDirty"
        class="flex shrink-0 items-center gap-1.5 rounded-md border border-accent/35 bg-accent-tint px-1.5 py-0.5 text-[10.5px] font-semibold text-accent"
        data-testid="undeployed-badge"
      ><span class="size-1.5 shrink-0 rounded-full bg-accent" />Un-deployed</span>
      <IconChevronRight class="size-3.5 shrink-0 text-text-4" />
    </button>

    <PanelResizeHandle edge="right" name="sidebar" :start="startResize" :step="step" />
  </aside>
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
.section-label { display: flex; align-items: center; gap: 7px; padding: 0 6px 8px; color: var(--color-text-4); font-family: var(--font-mono); font-size: 10.5px; letter-spacing: .12em; }
</style>
