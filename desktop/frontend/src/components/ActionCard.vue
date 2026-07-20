<script setup lang="ts">
import { computed } from 'vue'
import AppIcon from './AppIcon.vue'
import IconCornerDownLeft from '~icons/lucide/corner-down-left'
import type { ActionView } from '../types/action'
import type { ActionRunView } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/models'

const presentation = {
  'launch-session': { icon: 'play', color: '#34d399', description: 'Launch session' },
  shell: { icon: 'terminal', color: '#60a5fa', description: 'Run shell command' },
  'publish-message': { icon: 'radio', color: '#a78bfa', description: 'Publish message' },
} as const

const props = defineProps<{ action: ActionView; pending?: boolean; run?: ActionRunView }>()
const view = computed(() => presentation[props.action.type as keyof typeof presentation] ?? { icon: 'play', color: '#94a3b8', description: props.action.type })
const emit = defineEmits<{ run: [] }>()
</script>

<template>
  <button class="action-card" :data-id="action.id" data-testid="action-card" @click="emit('run')" :disabled="pending">
    <span class="action-icon flex size-[30px] shrink-0 items-center justify-center rounded-lg border" :style="{ borderColor: view.color, color: view.color }"><AppIcon :name="view.icon" class="size-3.5" /></span>
    <span class="min-w-0 flex-1 text-left">
      <span class="block text-[13.5px] font-medium text-text">{{ action.label }}</span>
      <span class="mt-0.5 block text-[11.5px] text-text-3 font-mono">{{ view.description }}</span>
    </span>
    <span class="flex shrink-0 items-center gap-1.5 rounded-md bg-accent px-2.5 py-[5px] text-xs font-semibold text-accent-contrast" data-testid="run-action">{{ pending ? 'Running…' : 'Run' }} <IconCornerDownLeft class="size-3" /></span>
  </button>
  <details v-if="run && run.status !== 'done'" class="mt-1 rounded border border-red-900/60 p-2 text-left text-xs text-red-300" data-testid="action-failure">
    <summary class="cursor-pointer">{{ run.error || 'Action failed' }}</summary>
    <dl class="mt-2 space-y-1 font-mono text-[11px] text-text-3">
      <div><dt class="inline text-red-300">status:</dt> <dd class="inline">{{ run.status }}</dd></div>
      <div v-if="run.stdout"><dt class="text-red-300">stdout:</dt><dd class="whitespace-pre-wrap" data-testid="action-stdout">{{ run.stdout }}</dd></div>
      <div v-if="run.stderr"><dt class="text-red-300">stderr:</dt><dd class="whitespace-pre-wrap" data-testid="action-stderr">{{ run.stderr }}</dd></div>
    </dl>
  </details>
</template>

<style scoped>
.action-card { display: flex; align-items: center; gap: 12px; width: 100%; cursor: pointer; border: 1px solid var(--color-card); border-radius: 9px; padding: 11px 13px; background: var(--color-action-card); }
.action-card:hover { border-color: var(--color-action-hover-border); background: var(--color-action-hover); }
.action-icon { background: var(--color-app); }
</style>
