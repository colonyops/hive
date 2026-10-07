<script setup lang="ts">
import { computed } from 'vue'
import AppIcon from './AppIcon.vue'
import { actionTypeMeta } from '../lib/actionPresentation'
import type { ActionView } from '../types/action'
import type { ActionRunView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

const props = defineProps<{ action: ActionView; pending?: boolean; run?: ActionRunView }>()
const view = computed(() => actionTypeMeta(props.action.type))
const active = computed(() => props.run?.status === 'pending' || props.run?.status === 'running')
// The card says how the last run ended; what it printed is the run viewer's.
const outcome = computed(() => {
  switch (props.run?.status) {
    case 'failed':
      return props.run.error || 'Action failed'
    case 'cancelled':
      return props.run.error || 'Action cancelled'
    default:
      return ''
  }
})
const emit = defineEmits<{ run: []; 'open-log': [commandId: number] }>()
</script>

<template>
  <div class="action-row">
    <div class="action-row-main">
      <button
        class="action-row-btn"
        :data-id="action.id"
        data-testid="action-card"
        :disabled="pending || active"
        :title="view.label"
        @click="emit('run')"
      >
        <span class="action-row-icon" :style="{ color: view.color }"
          ><AppIcon :name="view.icon" class="size-3.5"
        /></span>
        <span class="action-row-label">{{ action.label }}</span>
        <span v-if="pending" class="action-row-pending" data-testid="run-action">Starting…</span>
        <span v-else-if="active" class="action-row-pending" data-testid="run-action">
          {{ run?.status === 'pending' ? 'Queued…' : 'Running…' }}
        </span>
      </button>
      <button
        v-if="run"
        type="button"
        class="action-log-link"
        data-testid="action-open-log"
        @click="emit('open-log', run.commandId)"
      >
        {{ active ? 'View live log' : 'View log' }}
      </button>
    </div>
    <div
      v-if="outcome"
      class="action-run-line"
      :class="{ 'action-run-failed': run?.status === 'failed' }"
      :title="outcome"
      :data-testid="run?.status === 'failed' ? 'action-failure' : 'action-cancelled'"
    >
      {{ outcome }}
    </div>
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
.action-row-main {
  display: flex;
  align-items: center;
}
.action-row-main .action-row-btn {
  flex: 1;
  min-width: 0;
}
.action-run-line {
  overflow: hidden;
  padding: 0 11px 8px 36px;
  font-size: var(--text-caption);
  color: var(--color-text-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.action-run-failed {
  color: var(--color-severity-error);
}
.action-log-link {
  flex: none;
  padding: 8px 11px 8px 4px;
  cursor: pointer;
  font-size: var(--text-caption);
  color: var(--color-accent);
}
.action-log-link:hover {
  text-decoration: underline;
}
</style>
