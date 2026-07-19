<script setup lang="ts">
// Debug/health panel (Phase 6c): a read-only view over the same node_run
// data FlowsCanvas already polls via usePipelineEditor (nodeRuns/
// latestRunByNode — see composables/usePipelineEditor.ts), plus the
// mounting component's usePipelineRuntime summary of its last pump(). Pure
// display — this never mutates its props, so it takes plain values rather
// than a live client of its own.
import { computed } from 'vue'
import { byType } from '../registry'
import type { EditorFlow, NodeRunRecord } from '../lib/wireFlow'
import type { RuntimeSummary } from '../composables/usePipelineRuntime'

const props = defineProps<{
  flow: EditorFlow
  latestRunByNode: Map<string, NodeRunRecord>
  /** Newest-first, as returned by PipelineService.NodeRuns. */
  nodeRuns: NodeRunRecord[]
  runtimeSummary: RuntimeSummary | null
  running: boolean
}>()

/** How many of the newest-first nodeRuns rows the RECENT list shows. */
const RECENT_LIMIT = 20

function nodeLabel(nodeId: string): string {
  const node = props.flow.nodes.find((n) => n.id === nodeId)
  if (!node) return nodeId
  return node.name || byType[node.type]?.label || node.type
}

/** One row per node in the flow's own order (not latestRunByNode's Map insertion order), so the list matches the canvas/palette ordering the user already knows. */
const nodeStatuses = computed(() => props.flow.nodes.map((node) => ({
  node,
  run: props.latestRunByNode.get(node.id) ?? null,
})))

const recent = computed(() => props.nodeRuns.slice(0, RECENT_LIMIT))

/**
 * End-to-end duration for the latest tick: latestRunByNode holds one row
 * per node (its own most recent run), but different nodes' "most recent"
 * can come from different commits (e.g. a downstream node with nothing to
 * do this tick still shows its last real run). The latest tick is the set
 * of latestRunByNode rows that share the single most recent endedAt —
 * i.e. the nodes that actually ran together in the most recent commit —
 * summed, per D2's "sum of per-node durMs for the latest tick".
 */
const endToEnd = computed(() => {
  const runs = [...props.latestRunByNode.values()]
  if (runs.length === 0) return null
  const latestEndedAt = Math.max(...runs.map((r) => r.endedAt))
  const tick = runs.filter((r) => r.endedAt === latestEndedAt)
  return {
    durMs: tick.reduce((sum, r) => sum + r.durMs, 0),
    nodeCount: tick.length,
    endedAt: latestEndedAt,
  }
})

function statusLabel(run: NodeRunRecord | null): string {
  if (!run) return 'idle'
  return run.ok ? 'ok' : 'err'
}

function statusClasses(run: NodeRunRecord | null): string {
  if (!run) return 'text-text-4'
  return run.ok ? 'text-severity-success' : 'text-severity-error'
}

// endedAt is stored as Go's time.UnixNano() — convert to ms for Date.now() comparisons (mirrors FlowsCanvas.vue's ageLabel).
function ageLabel(endedAtNano: number): string {
  const ms = Date.now() - endedAtNano / 1e6
  if (ms < 1000) return 'just now'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  return `${h}h ago`
}
</script>

<template>
  <div class="hive-scroll flex min-h-0 flex-col overflow-y-auto text-[12px]" data-testid="flow-debug-panel">
    <div class="shrink-0 border-b border-row px-3 py-2 text-[11px] font-semibold uppercase tracking-wide text-text-3">
      Debug
    </div>

    <!-- Runtime summary — the mounting component's usePipelineRuntime last-pump result. -->
    <div class="shrink-0 border-b border-row px-3 py-2">
      <div class="flex items-center gap-1.5 text-[10.5px] text-text-3">
        <span class="size-1.5 rounded-full" :class="running ? 'bg-severity-success' : 'bg-text-4'" />
        {{ running ? 'Running' : 'Stopped' }}
      </div>
      <div v-if="runtimeSummary" class="mt-1 text-text-2" data-testid="debug-last-pump">
        Last pump: {{ runtimeSummary.batchSize }} msg{{ runtimeSummary.batchSize === 1 ? '' : 's' }}
        → {{ runtimeSummary.outputCount }} output{{ runtimeSummary.outputCount === 1 ? '' : 's' }}
        / {{ runtimeSummary.discardCount }} discard{{ runtimeSummary.discardCount === 1 ? '' : 's' }}
        <span v-if="runtimeSummary.errorCount > 0" class="text-severity-error">/ {{ runtimeSummary.errorCount }} error{{ runtimeSummary.errorCount === 1 ? '' : 's' }}</span>
      </div>
      <div v-else class="mt-1 text-text-4" data-testid="debug-last-pump-empty">No pump yet.</div>
    </div>

    <!-- End-to-end figure for the latest tick. -->
    <div class="shrink-0 border-b border-row px-3 py-2">
      <div class="text-[10.5px] uppercase tracking-wide text-text-3">End-to-end</div>
      <div v-if="endToEnd" class="mt-1 text-text-2" data-testid="debug-end-to-end">
        {{ Math.round(endToEnd.durMs) }}ms across {{ endToEnd.nodeCount }} node{{ endToEnd.nodeCount === 1 ? '' : 's' }} · {{ ageLabel(endToEnd.endedAt) }}
      </div>
      <div v-else class="mt-1 text-text-4" data-testid="debug-end-to-end-empty">No runs yet.</div>
    </div>

    <!-- Per-node latest status. -->
    <div class="shrink-0 border-b border-row px-3 py-2">
      <div class="mb-1.5 text-[10.5px] uppercase tracking-wide text-text-3">Nodes</div>
      <div v-if="nodeStatuses.length === 0" class="text-text-4">No nodes in this flow.</div>
      <ul v-else class="flex flex-col gap-1">
        <li
          v-for="{ node, run } in nodeStatuses"
          :key="node.id"
          class="flex items-center justify-between gap-2"
          :data-testid="`debug-node-${node.id}`"
        >
          <span class="min-w-0 truncate text-text">{{ nodeLabel(node.id) }}</span>
          <span class="shrink-0 whitespace-nowrap" :class="statusClasses(run)">
            {{ statusLabel(run) }}
            <template v-if="run">
              · {{ run.inCount }}→{{ run.outCount }}→{{ run.dropCount }} · {{ Math.round(run.durMs) }}ms
            </template>
          </span>
        </li>
      </ul>
    </div>

    <!-- RECENT: last N node_run rows, newest first. -->
    <div class="min-h-0 flex-1 px-3 py-2">
      <div class="mb-1.5 text-[10.5px] uppercase tracking-wide text-text-3">Recent</div>
      <div v-if="recent.length === 0" class="text-text-4" data-testid="debug-recent-empty">No activity yet.</div>
      <ul v-else class="flex flex-col gap-1">
        <li
          v-for="(run, i) in recent"
          :key="`${run.nodeId}-${run.endedAt}-${i}`"
          class="flex items-center justify-between gap-2"
          data-testid="debug-recent-row"
        >
          <span class="min-w-0 truncate text-text-2">{{ nodeLabel(run.nodeId) }}</span>
          <span class="shrink-0 whitespace-nowrap" :class="run.ok ? 'text-severity-success' : 'text-severity-error'">
            {{ run.ok ? 'ok' : (run.err || 'err') }} · {{ run.inCount }}→{{ run.outCount }}→{{ run.dropCount }} · {{ ageLabel(run.endedAt) }}
          </span>
        </li>
      </ul>
    </div>
  </div>
</template>
