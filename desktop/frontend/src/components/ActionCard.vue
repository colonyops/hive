<script setup lang="ts">
import AppIcon from './AppIcon.vue'
import IconCornerDownLeft from '~icons/lucide/corner-down-left'
import IconPlay from '~icons/lucide/play'
import type { Action } from '../types/feed'

defineProps<{ action: Action }>()
const emit = defineEmits<{ run: [] }>()
</script>

<template>
  <button class="action-card" :data-id="action.id" data-testid="action-card" @click="emit('run')">
    <span class="action-icon flex size-[30px] shrink-0 items-center justify-center rounded-lg border" :style="{ borderColor: action.color, color: action.color }"><AppIcon :name="action.icon" class="size-3.5" /></span>
    <span class="min-w-0 flex-1 text-left">
      <span class="block text-[13.5px] font-medium text-text">{{ action.title }}</span>
      <span class="mt-0.5 block text-[11.5px] text-text-3 font-mono">{{ action.sub }}</span>
    </span>
    <span v-if="action.primary" class="flex shrink-0 items-center gap-1.5 rounded-md bg-accent px-2.5 py-[5px] text-xs font-semibold text-accent-contrast" data-testid="primary-action">Run <IconCornerDownLeft class="size-3" /></span>
    <IconPlay v-else data-testid="secondary-affordance" class="size-3.5 shrink-0 text-text-4" />
  </button>
</template>

<style scoped>
.action-card { display: flex; align-items: center; gap: 12px; width: 100%; cursor: pointer; border: 1px solid var(--color-card); border-radius: 9px; padding: 11px 13px; background: var(--color-action-card); }
.action-card:hover { border-color: var(--color-action-hover-border); background: var(--color-action-hover); }
.action-icon { background: var(--color-app); }
</style>
