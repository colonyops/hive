<script setup lang="ts">
import type { Action } from '../types/feed'

defineProps<{ action: Action }>()
const emit = defineEmits<{ run: [] }>()
</script>

<template>
  <button class="action-card" :data-id="action.id" data-testid="action-card" @click="emit('run')">
    <span class="flex size-[30px] shrink-0 items-center justify-center rounded-lg border bg-app text-sm" :style="{ borderColor: action.color, color: action.color }">{{ action.icon }}</span>
    <span class="min-w-0 flex-1 text-left">
      <span class="block text-[13.5px] font-medium text-zinc-100">{{ action.title }}</span>
      <span class="mt-0.5 block text-[11.5px] text-text-3 font-mono">{{ action.sub }}</span>
    </span>
    <span v-if="action.primary" class="shrink-0 rounded-md bg-accent px-2.5 py-[5px] text-xs font-semibold text-app" data-testid="primary-action">Run ↵</span>
    <span v-else class="shrink-0 text-sm text-text-4">▷</span>
  </button>
</template>

<style scoped>
.action-card { display: flex; align-items: center; gap: 12px; width: 100%; cursor: pointer; border: 1px solid var(--color-card); border-radius: 9px; padding: 11px 13px; background: var(--color-sidebar); }
.action-card:hover { border-color: #3a3a40; background: var(--color-chip); }
</style>
