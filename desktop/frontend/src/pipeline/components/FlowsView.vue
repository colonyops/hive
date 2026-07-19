<script setup lang="ts">
// The flows editor view (Phase 6b): composes the flow picker, node palette,
// canvas, and Deploy bar over usePipelineEditor. This is the one place in
// src/pipeline that imports the real Wails bindings and the "flows:updated"
// event — usePipelineEditor's own core takes an injected client and never
// touches @wailsio/runtime or bindings/ directly (see its module docs); this
// component is the adapter FlowsService/PipelineService's generated
// bindings are wired through, mirroring driver.ts's documented injection
// posture (a real PipelineClient adapter would be wired up the same way).
//
// Individual refs/actions are destructured out of usePipelineEditor()
// (rather than kept as one `editor` object) so the template can use them
// directly without a `.value` on every access — the same convention
// useFeedState() + App.vue already use.
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { GetFlow, GetLayout, ListFlows, SaveFlow, SaveLayout } from '../../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { NodeRuns } from '../../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import { usePipelineEditor, type PipelineEditorClient } from '../composables/usePipelineEditor'
import NodePalette from './NodePalette.vue'
import FlowsCanvas from './FlowsCanvas.vue'

const client: PipelineEditorClient = {
  async listFlows() { return await ListFlows() },
  async getFlow(id) { return await GetFlow(id) },
  async saveFlow(flow) { await SaveFlow(flow) },
  async getLayout(id) { return await GetLayout(id) },
  async saveLayout(id, layout) { await SaveLayout(id, layout) },
  async nodeRuns(flowId, limit) { return await NodeRuns(flowId, limit) },
}

const {
  flows, activeFlow, layout, dirty, latestRunByNode, saving, error,
  refreshFlows, refreshNodeRuns, selectFlow, newFlow, addNode, updateNode, deleteNode, moveNode, deploy,
} = usePipelineEditor(client)

let unsubscribe: (() => void) | undefined
onMounted(() => {
  unsubscribe = Events.On('flows:updated', () => {
    void refreshFlows()
    void refreshNodeRuns()
  })
})
onUnmounted(() => unsubscribe?.())

const filePath = computed(() => (activeFlow.value ? `flows/${activeFlow.value.id}.yaml` : ''))

const showNewFlow = ref(false)
const newFlowName = ref('')

function submitNewFlow() {
  const name = newFlowName.value.trim()
  if (!name) return
  newFlow(name)
  newFlowName.value = ''
  showNewFlow.value = false
}

function onPaletteAdd(type: string) {
  if (activeFlow.value) addNode(type)
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-1" data-testid="flows-view">
    <aside class="flex w-[260px] shrink-0 flex-col border-r border-row bg-sidebar">
      <div class="flex shrink-0 items-center justify-between border-b border-row px-3 py-2.5">
        <span class="text-[11px] font-semibold uppercase tracking-wide text-text-3">Flows</span>
        <button class="cursor-pointer text-text-3 hover:text-text" data-testid="flows-new-toggle" @click="showNewFlow = !showNewFlow">+</button>
      </div>

      <div v-if="showNewFlow" class="flex shrink-0 items-center gap-1.5 border-b border-row p-2">
        <input
          v-model="newFlowName"
          type="text"
          placeholder="New flow name…"
          class="w-full min-w-0 rounded-md border border-strong bg-app px-2 py-1.5 text-[12px] text-text outline-none placeholder:text-text-4 focus:border-accent"
          data-testid="new-flow-name"
          @keydown.enter="submitNewFlow"
        >
        <button class="shrink-0 cursor-pointer rounded-md bg-accent px-2 py-1.5 text-[11.5px] font-semibold text-accent-contrast" data-testid="new-flow-submit" @click="submitNewFlow">Add</button>
      </div>

      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto p-1.5" data-testid="flow-picker">
        <button
          v-for="f in flows"
          :key="f.id"
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left hover:bg-hover"
          :class="f.id === activeFlow?.id ? 'bg-selection' : ''"
          :data-testid="`flow-picker-${f.id}`"
          @click="selectFlow(f.id)"
        >
          <span class="size-1.5 shrink-0 rounded-full" :class="!f.valid ? 'bg-severity-error' : f.enabled ? 'bg-severity-success' : 'bg-text-4'" />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[12.5px] text-text">{{ f.name || f.id }}</span>
            <span v-if="!f.valid" class="block truncate text-[10.5px] text-severity-error" data-testid="flow-picker-error">{{ f.error }}</span>
          </span>
        </button>
      </div>

      <div class="min-h-0 shrink-0 border-t border-row" style="height: 45%">
        <NodePalette @add="onPaletteAdd" />
      </div>
    </aside>

    <section class="flex min-w-0 flex-1 flex-col">
      <div class="flex shrink-0 items-center gap-3 border-b border-row bg-pane px-3.5 py-2.5">
        <div class="min-w-0 flex-1">
          <div class="truncate text-[13px] font-semibold text-text" data-testid="flow-title">{{ activeFlow?.name || 'No flow selected' }}</div>
          <div v-if="filePath" class="truncate font-mono text-[10.5px] text-text-3" data-testid="flow-file-path">{{ filePath }}</div>
        </div>
        <span v-if="dirty" class="whitespace-nowrap text-[11.5px] text-accent" data-testid="flow-dirty-indicator">Unsaved changes — Deploy to write</span>
        <span v-if="error" class="max-w-[280px] truncate whitespace-nowrap text-[11.5px] text-severity-error" data-testid="flow-editor-error">{{ error }}</span>
        <button
          class="shrink-0 cursor-pointer rounded-lg bg-accent px-3.5 py-2 text-[12.5px] font-semibold text-accent-contrast disabled:cursor-default disabled:opacity-40"
          :disabled="!dirty || saving || !activeFlow"
          data-testid="deploy-button"
          @click="deploy"
        >{{ saving ? 'Deploying…' : 'Deploy' }}</button>
      </div>

      <FlowsCanvas
        v-if="activeFlow"
        :flow="activeFlow"
        :layout="layout"
        :latest-run-by-node="latestRunByNode"
        @move="moveNode"
        @update-node="updateNode"
        @delete-node="deleteNode"
      />
      <div v-else class="flex flex-1 items-center justify-center px-8 text-center text-[13px] text-text-4" data-testid="flows-view-empty">
        Select a flow, or create a new one, to start editing.
      </div>
    </section>
  </div>
</template>
