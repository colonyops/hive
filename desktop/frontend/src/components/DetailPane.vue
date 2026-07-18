<script setup lang="ts">
import ActionCard from './ActionCard.vue'
import type { Action, FeedItem } from '../types/feed'

defineProps<{ item: FeedItem | null; actions: Action[] }>()
const emit = defineEmits<{ 'run-action': [actionId: string]; 'open-browser': [] }>()
</script>

<template>
  <aside class="hive-scroll flex w-[466px] shrink-0 flex-col overflow-y-auto bg-pane" data-testid="detail-pane">
    <template v-if="item">
      <div class="border-b border-border px-5 pb-4 pt-[18px]">
        <div class="mb-2.5 flex items-center gap-2">
          <span class="source-icon">⌘</span>
          <span class="kind-badge" :class="item.kind === 'PR' ? 'text-kind-pr border-kind-pr' : 'text-kind-issue border-kind-issue'">{{ item.kind }}</span>
          <span class="font-mono text-xs text-text-3">{{ item.repo }} #{{ item.num }}</span>
          <span class="flex-1" />
          <button class="open-button" @click="emit('open-browser')">open ↗</button>
        </div>
        <h1 class="text-[17px] font-semibold leading-[1.3] tracking-[-.01em]">{{ item.title }}</h1>
        <p class="mt-[9px] text-xs text-text-3"><span class="text-text-2">{{ item.author }}</span> · {{ item.age }} ago</p>
        <p class="mt-2.5 text-[12.5px] leading-[1.5] text-text-2">{{ item.body }}</p>
      </div>

      <div class="px-5 pb-5 pt-4">
        <div class="mb-[13px] flex items-center gap-2">
          <span class="font-mono text-[10.5px] tracking-[.12em] text-accent">ACTIONS</span>
          <span class="font-mono text-[10.5px] text-text-4">· for {{ item.kind }}</span>
          <span class="flex-1" />
          <button class="edit-button">⚙ Edit</button>
        </div>
        <div class="flex flex-col gap-[9px]">
          <ActionCard v-for="action in actions" :key="action.id" :action="action" @run="emit('run-action', action.id)" />
        </div>
        <div class="mt-3.5 flex items-center gap-2 font-mono text-[11px] text-text-3"><span class="text-accent">ⓘ</span> Runs headless (batch) on <span class="text-text-2">{{ item.branch }}</span></div>
        <div class="mt-1.5 pl-[19px] font-mono text-[11px] text-text-4">Actions defined in .hive/actions.yml · attach via tmux</div>
      </div>
    </template>
    <div v-else class="m-auto font-mono text-xs text-text-4">Select an item to inspect</div>
  </aside>
</template>

<style scoped>
.source-icon { display: inline-flex; align-items: center; justify-content: center; width: 18px; height: 18px; border: 1px solid var(--color-strong); border-radius: 5px; background: var(--color-chip); color: var(--color-text-2); font-family: var(--font-mono); font-size: 8px; font-weight: 700; }
.kind-badge { border-width: 1px; border-radius: 3px; padding: 1px 5px; font-family: var(--font-mono); font-size: 9px; letter-spacing: .06em; }
.open-button, .edit-button { cursor: pointer; border: 1px solid var(--color-card); border-radius: 4px; padding: 2px 7px; color: var(--color-text-2); font-family: var(--font-mono); font-size: 11px; }
.edit-button { border-radius: 5px; padding: 3px 8px; font-family: var(--font-sans); }
.open-button:hover, .edit-button:hover { border-color: var(--color-strong); color: var(--color-text); }
</style>
