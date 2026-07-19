<script setup lang="ts">
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCircle from '~icons/lucide/circle'
import IconGitBranch from '~icons/lucide/git-branch'
import IconList from '~icons/lucide/list'
import IconPencil from '~icons/lucide/pencil'
import IconPlus from '~icons/lucide/plus'
import IconRss from '~icons/lucide/rss'
import IconTrash2 from '~icons/lucide/trash-2'
import IconWorkflow from '~icons/lucide/workflow'
import type { Profile, SidebarSelection } from '../types/feed'

const props = defineProps<{ profile: Profile; selection: SidebarSelection; unreadOnly: boolean }>()
const emit = defineEmits<{ select: [sel: SidebarSelection]; 'select-unread': []; 'edit-feeds': []; 'edit-feed': [feedId: string]; 'delete-profile': []; 'open-flows': [] }>()

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
  <aside class="hive-scroll flex w-[250px] shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar">
    <div class="profile-header border-b border-border px-4 pb-3 pt-4" data-testid="sidebar-profile-header">
      <div class="flex items-center gap-2">
        <div class="min-w-0 flex-1 truncate text-[15px] font-semibold tracking-[-.01em]" data-testid="sidebar-profile-name">{{ profile.name }}</div>
        <button
          class="flow-pill flex shrink-0 items-center gap-1.5 rounded-md border border-accent/35 bg-accent-tint px-2 py-1 text-[11.5px] font-semibold text-accent hover:bg-accent/20"
          title="Edit this profile's flow"
          data-testid="sidebar-open-flows"
          @click="emit('open-flows')"
        ><IconWorkflow class="size-3" />Flows</button>
        <button
          class="profile-delete shrink-0 cursor-pointer text-text-3 hover:text-severity-error"
          aria-label="Delete profile"
          data-testid="sidebar-delete-profile"
          @click="emit('delete-profile')"
        ><IconTrash2 class="size-3.5" /></button>
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
        <button class="ml-auto cursor-pointer text-strong hover:text-text-2" aria-label="Edit feeds" data-testid="sidebar-edit-feeds" @click="emit('edit-feeds')"><IconPlus class="size-3.5" /></button>
      </div>
      <!-- A div, not a button: the hover pencil is itself a button and
           interactive elements must not nest. -->
      <div
        v-for="feed in profile.feeds ?? []"
        :key="feed.id"
        class="sidebar-entry"
        role="button"
        tabindex="0"
        :class="{ 'sidebar-entry-selected': feedSelected(feed.id) }"
        data-testid="sidebar-feed"
        :data-id="feed.id"
        @click="emit('select', { type: 'feed', feedId: feed.id })"
        @keydown.enter="emit('select', { type: 'feed', feedId: feed.id })"
      >
        <span class="nav-icon"><IconGitBranch class="size-3" /></span><span class="min-w-0 flex-1 truncate text-left">{{ feed.name }}</span>
        <button
          class="feed-edit cursor-pointer text-text-3 hover:text-text"
          :aria-label="`Edit feed ${feed.name}`"
          :data-testid="`sidebar-feed-edit-${feed.id}`"
          @click.stop="emit('edit-feed', feed.id)"
        ><IconPencil class="size-3" /></button>
        <span class="font-mono text-[11px]" :class="feed.newCount ? 'text-accent' : 'text-text-3'">{{ feed.newCount || feed.count }}</span>
      </div>
    </section>

    <button
      class="mt-auto flex items-center gap-2.5 border-t border-border p-2.5 text-left hover:bg-chip"
      data-testid="sidebar-edit-flow"
      @click="emit('open-flows')"
    >
      <span class="flex size-[22px] shrink-0 items-center justify-center rounded-md border border-dashed border-card bg-app text-accent"><IconWorkflow class="size-3" /></span>
      <span class="min-w-0 flex-1">
        <span class="block text-[12.5px] font-semibold text-text">Edit flow</span>
        <span class="block truncate font-mono text-[11px] text-text-3">sources · processing · outputs</span>
      </span>
      <IconChevronRight class="size-3.5 shrink-0 text-text-4" />
    </button>
  </aside>
</template>

<style scoped>
.sidebar-entry { display: flex; align-items: center; gap: 9px; width: 100%; padding: 7px 8px; border-radius: 7px; color: var(--color-text-2); font-size: 13px; cursor: pointer; }
.sidebar-entry:hover { background: var(--color-chip); color: var(--color-text); }
.sidebar-entry-selected { background: var(--color-hover); color: var(--color-accent); font-weight: 500; }
/* The pencil holds its space (opacity, not display) so hovering never shifts the count badge. */
.feed-edit { display: inline-flex; opacity: 0; }
.sidebar-entry:hover .feed-edit, .feed-edit:focus-visible { opacity: 1; }
/* Same pattern as .feed-edit: holds its space, only reveals on header hover. */
.profile-delete { display: inline-flex; opacity: 0; }
.profile-header:hover .profile-delete, .profile-delete:focus-visible { opacity: 1; }
.sidebar-entry-selected .nav-icon { border-color: var(--color-accent-tint); color: var(--color-accent); }
.nav-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-app); color: var(--color-text-2); }
.section-label { display: flex; align-items: center; gap: 7px; padding: 0 6px 8px; color: var(--color-text-4); font-family: var(--font-mono); font-size: 10.5px; letter-spacing: .12em; }
</style>
