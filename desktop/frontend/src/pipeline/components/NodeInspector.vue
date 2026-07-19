<script setup lang="ts">
// The selected-node floating inspector (8a): glyph + title + status line +
// a RECENT list of the node's last few runs, with "⚙ Edit" opening the full
// NodeEditorDrawer — FlowsCanvas.vue mounts this on a single click, keeping
// the drawer itself reserved for an explicit edit intent (8a's "click a node
// -> inspector; Edit -> drawer" flow, replacing the old "click opens the
// drawer directly" behavior).
//
// The mockup's RECENT rows show individual titled items (e.g. "#2841 batch_
// spawn env fix") — that's per-message history no backend data this app has
// records (node_run is one aggregate row per *pump*, not per message; see
// internal/desktop/pipeline). Each row here is one pump's outcome instead
// (counts + age, or the error), the same aggregate shape the status line
// already reports — a real adaptation of the mock onto the real data model,
// not individually-titled items.
import { computed } from 'vue'
import { ageLabel, classify, statusColor, statusLabel, statusPulses } from '../lib/runStatus'
import type { NodeTypeDefinition } from '../nodeType'
import type { FlowNode } from '../types'
import type { NodeRunRecord } from '../lib/wireFlow'

const props = defineProps<{
  node: FlowNode
  def: NodeTypeDefinition
  run: NodeRunRecord | undefined
  running: boolean
  /** Newest-first run history for this node, already limited by the caller. */
  recentRuns: NodeRunRecord[]
}>()

defineEmits<{ edit: [] }>()

const status = computed(() => classify(props.run, props.running))
</script>

<template>
  <div class="absolute overflow-hidden rounded-[10px] border border-strong bg-pane shadow-[0_20px_50px_-14px_rgba(0,0,0,.5)]" data-testid="node-inspector-panel">
    <div class="flex items-center gap-2 border-b border-row px-3 py-2.5" style="background: var(--color-raised)">
      <span class="flex size-[18px] shrink-0 items-center justify-center rounded-md" :style="{ background: def.tint ?? 'var(--color-accent-tint)', color: def.accentToken ?? 'var(--color-accent)' }">
        <component :is="def.glyph" class="size-3" />
      </span>
      <span class="min-w-0 flex-1 truncate text-[12.5px] font-semibold text-text" data-testid="node-inspector-title">
        {{ def.label }}<template v-if="node.name"> · {{ node.name }}</template>
      </span>
      <button class="shrink-0 cursor-pointer whitespace-nowrap text-[11px] text-text-3 hover:text-text" data-testid="node-inspector-edit" @click="$emit('edit')">⚙ Edit</button>
    </div>

    <div class="flex flex-col gap-2.5 px-3 py-2.5">
      <div class="flex items-center gap-1.5">
        <span class="size-[7px] shrink-0 rounded-full" :class="{ 'hive-pulse': statusPulses(status) }" :style="{ background: statusColor(status) }" />
        <span class="truncate font-mono text-[11px] text-text-2" data-testid="node-inspector-status">{{ statusLabel(status, run) }}</span>
      </div>

      <div class="h-px shrink-0 bg-row" />

      <div class="font-mono text-[10px] tracking-[.12em] text-text-4">RECENT</div>
      <div v-if="recentRuns.length === 0" class="font-mono text-[10.5px] text-text-4" data-testid="node-inspector-recent-empty">No runs yet</div>
      <div v-else class="flex flex-col gap-1.5" data-testid="node-inspector-recent">
        <div v-for="(r, i) in recentRuns" :key="i" class="flex items-center gap-2 font-mono text-[10.5px]" data-testid="node-inspector-recent-row">
          <span class="shrink-0" :class="r.ok ? 'text-severity-success' : 'text-severity-error'">{{ r.ok ? '✓' : '✕' }}</span>
          <span class="min-w-0 flex-1 truncate text-text-2">{{ r.ok ? `${r.inCount} → ${r.outCount}` : (r.err || 'error') }}</span>
          <span class="shrink-0 text-text-4">{{ ageLabel(r.endedAt) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.hive-pulse {
  animation: hivePulse 1.6s ease-in-out infinite;
}
</style>
