<script setup lang="ts">
import { computed } from 'vue'
import AppIcon from './AppIcon.vue'
import { actionTypeMeta } from '../lib/actionPresentation'
import type { ActionView } from '../types/action'
import type { ActionRunView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

const props = defineProps<{ action: ActionView; pending?: boolean; run?: ActionRunView }>()
const view = computed(() => actionTypeMeta(props.action.type))
const emit = defineEmits<{ run: [] }>()
</script>

<template>
  <div class="action-row">
    <button
      class="action-row-btn"
      :data-id="action.id"
      data-testid="action-card"
      :disabled="pending"
      :title="view.label"
      @click="emit('run')"
    >
      <span class="action-row-icon" :style="{ color: view.color }"><AppIcon :name="view.icon" class="size-3.5" /></span>
      <span class="action-row-label">{{ action.label }}</span>
      <span v-if="pending" class="action-row-pending" data-testid="run-action">Running…</span>
    </button>
    <details v-if="run && run.status !== 'done'" class="action-failure" data-testid="action-failure">
      <summary class="cursor-pointer">{{ run.error || 'Action failed' }}</summary>
      <dl class="mt-2 space-y-1 font-mono text-caption text-text-3">
        <div>
          <dt class="inline text-severity-error">status:</dt>
          <dd class="inline">{{ run.status }}</dd>
        </div>
        <div v-if="run.stdout">
          <dt class="text-severity-error">stdout:</dt>
          <dd class="whitespace-pre-wrap" data-testid="action-stdout">{{ run.stdout }}</dd>
        </div>
        <div v-if="run.stderr">
          <dt class="text-severity-error">stderr:</dt>
          <dd class="whitespace-pre-wrap" data-testid="action-stderr">{{ run.stderr }}</dd>
        </div>
      </dl>
    </details>
  </div>
</template>

<style scoped>
/* One condensed menu row. The divider lives between adjacent rows, so the
   grouped container (DetailPane's .action-list) reads as one object with hairline
   rules rather than a stack of separate cards. */
.action-row + .action-row {
  border-top: 1px solid var(--color-border);
}
.action-row-btn {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 11px;
  padding: 8px 11px;
  text-align: left;
  cursor: pointer;
}
.action-row-btn:hover:not(:disabled) {
  background: var(--color-action-hover);
}
.action-row-btn:disabled {
  cursor: default;
}
.action-row-icon {
  display: inline-flex;
  flex: none;
  width: 14px;
  justify-content: center;
}
.action-row-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: var(--text-small);
  color: var(--color-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.action-row-pending {
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--text-caption);
  color: var(--color-text-3);
}
.action-failure {
  border-top: 1px solid var(--color-border);
  padding: 8px 11px;
  text-align: left;
  font-size: var(--text-small);
  color: var(--color-severity-error);
}
</style>
