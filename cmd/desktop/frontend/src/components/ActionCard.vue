<script setup lang="ts">
import { computed } from 'vue'
import AppIcon from './AppIcon.vue'
import { actionTypeMeta } from '../lib/actionPresentation'
import type { ActionView } from '../types/action'
import type { ActionRunView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

const props = defineProps<{ action: ActionView; pending?: boolean; run?: ActionRunView; expanded?: boolean }>()
const view = computed(() => actionTypeMeta(props.action.type))
const active = computed(() => props.run?.status === 'pending' || props.run?.status === 'running')
const runSummary = computed(() => {
  if (props.run?.status === 'done') return 'Action completed'
  if (props.run?.status === 'cancelled') return props.run.error || 'Action cancelled'
  return props.run?.error || 'Action failed'
})
const emit = defineEmits<{ run: [] }>()
</script>

<template>
  <div class="action-row">
    <button
      class="action-row-btn"
      :data-id="action.id"
      data-testid="action-card"
      :disabled="pending || active"
      :title="view.label"
      @click="emit('run')"
    >
      <span class="action-row-icon" :style="{ color: view.color }"><AppIcon :name="view.icon" class="size-3.5" /></span>
      <span class="action-row-label">{{ action.label }}</span>
      <span v-if="pending" class="action-row-pending" data-testid="run-action">Starting…</span>
      <span v-else-if="active" class="action-row-pending" data-testid="run-action">
        {{ run?.status === 'pending' ? 'Queued…' : 'Running…' }}
      </span>
    </button>
    <details
      v-if="run && !active"
      class="action-result"
      :class="{ 'action-result-failed': run.status === 'failed' }"
      :data-testid="run.status === 'failed' ? 'action-failure' : 'action-run-details'"
      :open="expanded"
    >
      <summary class="cursor-pointer">{{ runSummary }}</summary>
      <dl class="mt-2 space-y-1 font-mono text-caption text-text-3">
        <div>
          <dt class="inline">status:</dt>
          <dd class="inline">{{ run.status }}</dd>
        </div>
        <div>
          <dt class="inline">run:</dt>
          <dd class="inline">{{ run.commandId }}</dd>
        </div>
        <div v-if="run.stdout">
          <dt>stdout:</dt>
          <dd class="whitespace-pre-wrap" data-testid="action-stdout">{{ run.stdout }}</dd>
        </div>
        <div v-if="run.stderr">
          <dt>stderr:</dt>
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
.action-result {
  border-top: 1px solid var(--color-border);
  padding: 8px 11px;
  text-align: left;
  font-size: var(--text-small);
  color: var(--color-text-2);
}
.action-result-failed {
  color: var(--color-severity-error);
}
</style>
