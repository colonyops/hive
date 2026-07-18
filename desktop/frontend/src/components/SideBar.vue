<script setup lang="ts">
import type { Profile, SidebarSelection } from '../types/feed'

const props = defineProps<{ profile: Profile; selection: SidebarSelection }>()
const emit = defineEmits<{ select: [sel: SidebarSelection] }>()

function selected(selection: SidebarSelection): boolean {
  if (props.selection.type !== selection.type) return false
  if (selection.type === 'feed') return props.selection.type === 'feed' && props.selection.feedId === selection.feedId
  return true
}
</script>

<template>
  <aside class="hive-scroll flex w-[250px] shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar">
    <div class="border-b border-border px-4 pb-3 pt-4">
      <div class="text-[15px] font-semibold tracking-[-.01em]" data-testid="sidebar-profile-name">{{ profile.name }}</div>
      <div class="mt-1 flex items-center gap-1.5">
        <span class="flex size-[15px] items-center justify-center rounded border border-strong bg-chip font-mono text-[8px] font-bold text-zinc-200">⌘</span>
        <span class="text-xs text-text-3">{{ profile.sourceSummary }}</span>
      </div>
    </div>

    <div class="px-2.5 pb-0.5 pt-3">
      <button class="sidebar-entry" :class="{ 'sidebar-entry-selected': selected({ type: 'all' }) }" @click="emit('select', { type: 'all' })">
        <span class="nav-icon border-[#3a2f18] text-accent">☰</span><span class="flex-1 text-left">All items</span><span class="font-mono text-[11px]">{{ profile.totalCount }}</span>
      </button>
      <button class="sidebar-entry" :class="{ 'sidebar-entry-selected': selected({ type: 'unread' }) }" @click="emit('select', { type: 'unread' })">
        <span class="nav-icon">○</span><span class="flex-1 text-left">Unread</span><span class="size-[7px] rounded-full bg-accent" /><span class="ml-[7px] font-mono text-[11px] text-text-3">{{ profile.unreadCount }}</span>
      </button>
    </div>

    <section class="px-2.5 pb-1.5 pt-2">
      <div class="section-label"><span class="text-kind-pr">◈</span>FEEDS <span class="ml-auto text-sm text-[#3a3a40]">+</span></div>
      <button
        v-for="feed in profile.feeds ?? []"
        :key="feed.id"
        class="sidebar-entry"
        :class="{ 'sidebar-entry-selected': selected({ type: 'feed', feedId: feed.id }) }"
        @click="emit('select', { type: 'feed', feedId: feed.id })"
      >
        <span class="nav-icon font-mono text-[8px] font-bold">⌘</span><span class="flex-1 text-left">{{ feed.name }}</span><span class="font-mono text-[11px]" :class="feed.newCount ? 'text-accent' : 'text-text-3'">{{ feed.newCount || feed.count }}</span>
      </button>
    </section>

    <section class="px-2.5 pb-1.5 pt-2">
      <div class="section-label"><span class="text-tasks">▤</span>TASKS <span class="ml-auto text-sm text-[#3a3a40]">+</span></div>
      <div class="static-entry" data-testid="sidebar-inert"><span class="w-[18px] text-center text-tasks">◆</span><span class="flex-1">MVP v0.1 epic</span><span class="font-mono text-[11px] text-text-3">14</span></div>
      <div class="static-entry" data-testid="sidebar-inert"><span class="w-[18px] text-center text-tasks">◆</span><span class="flex-1">Auth epic</span><span class="font-mono text-[11px] text-text-3">6</span></div>
    </section>

    <section class="px-2.5 pb-1.5 pt-2">
      <div class="section-label"><span class="text-docs">≡</span>DOCS <span class="ml-auto text-sm text-[#3a3a40]">+</span></div>
      <div class="static-entry" data-testid="sidebar-inert"><span class="w-[18px] text-center text-docs">≡</span><span class="flex-1 font-mono text-xs">rollout-plan.md</span></div>
      <div class="static-entry" data-testid="sidebar-inert"><span class="w-[18px] text-center text-docs">≡</span><span class="flex-1 font-mono text-xs">batch-spawn.md</span></div>
    </section>
  </aside>
</template>

<style scoped>
.sidebar-entry, .static-entry { display: flex; align-items: center; gap: 9px; width: 100%; padding: 7px 8px; border-radius: 7px; color: var(--color-text-2); font-size: 13px; }
.sidebar-entry { cursor: pointer; }
.sidebar-entry:hover { background: var(--color-chip); color: var(--color-text); }
.sidebar-entry-selected { background: var(--color-hover); color: var(--color-accent); font-weight: 500; }
.sidebar-entry-selected .nav-icon { border-color: #3a2f18; color: var(--color-accent); }
.nav-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-app); color: var(--color-text-2); }
.section-label { display: flex; align-items: center; gap: 7px; padding: 0 6px 8px; color: var(--color-text-4); font-family: var(--font-mono); font-size: 10.5px; letter-spacing: .12em; }
</style>
