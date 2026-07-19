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
import { computed, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { GetFlow, GetLayout, ListFlows, SaveFlow, SaveLayout } from '../../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { Commit, FeedItems, NodeRuns, ReadFrom } from '../../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import { usePipelineEditor, type PipelineEditorClient } from '../composables/usePipelineEditor'
import { usePipelineRuntime } from '../composables/usePipelineRuntime'
import type { PipelineClient } from '../driver'
import { buildFlowPrompt } from '../lib/flowPrompt'
import NodePalette from './NodePalette.vue'
import FlowsCanvas from './FlowsCanvas.vue'
import FlowDebugPanel from './FlowDebugPanel.vue'
import FeedItemsPreview, { type FeedItemsClient } from './FeedItemsPreview.vue'

const client: PipelineEditorClient = {
  async listFlows() { return await ListFlows() },
  async getFlow(id) { return await GetFlow(id) },
  async saveFlow(flow) { await SaveFlow(flow) },
  async getLayout(id) { return await GetLayout(id) },
  async saveLayout(id, layout) { await SaveLayout(id, layout) },
  async nodeRuns(flowId, limit) { return await NodeRuns(flowId, limit) },
}

// flowId is the profile's flow (a profile IS a flow): when the flows canvas
// is opened for a profile, select that flow once the list has loaded. If the
// profile hasn't been migrated to a flow yet, the picker stays open.
const props = defineProps<{ flowId?: string }>()

const {
  flows, activeFlow, layout, dirty, nodeRuns, latestRunByNode, saving, error,
  refreshFlows, refreshNodeRuns, selectFlow, newFlow, addNode, updateNode, deleteNode, moveNode, deploy,
} = usePipelineEditor(client)

watch([() => props.flowId, flows], ([id, list]) => {
  if (!id || activeFlow.value?.id === id) return
  if (list.some((f) => f.id === id)) void selectFlow(id)
}, { immediate: true })

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

// ── Runtime hookup (Phase 6c) ─────────────────────────────────────────────
// Adapts PipelineService.ReadFrom/Commit into driver.ts's injected
// PipelineClient shape — the same adapter posture as `client` above.
// wrapped in async functions (not passed directly) so the return type is a
// plain Promise, not Wails's CancellablePromise.
const pipelineClient: PipelineClient = {
  async readFrom(offset, limit) { return await ReadFrom(offset, limit) },
  async commit(batch) { await Commit(batch) },
}

// One usePipelineRuntime instance per selected flow — rebuilt whenever the
// active flow's id changes (a different flow is a different commit
// consumer and cursor), but NOT on every node/wire edit: addNode/
// updateNode/etc. mutate the same activeFlow object in place (see
// usePipelineEditor.ts), so the driver's captured Flow reference already
// sees those edits on its next pump() without needing a new instance.
const runtime = shallowRef<ReturnType<typeof usePipelineRuntime> | null>(null)

watch(() => activeFlow.value?.id, () => {
  runtime.value?.stop()
  runtime.value = activeFlow.value ? usePipelineRuntime(pipelineClient, activeFlow.value) : null
}, { immediate: true })

// Flattened, template-friendly reads of the current runtime's nested refs —
// a template accessing `runtime.running.value` through a shallowRef would
// not auto-unwrap (Vue only auto-unwraps a ref's own top-level binding),
// so these computeds do it explicitly instead.
const runtimeRunning = computed(() => runtime.value?.running.value ?? false)
const runtimeLastRun = computed(() => runtime.value?.lastRun.value ?? null)
const runtimeError = computed(() => runtime.value?.error.value ?? null)

let unsubscribeLog: (() => void) | undefined
onMounted(() => {
  // Drives the mounted runtime's pump() loop on every backend log append —
  // see driver.ts's module docs and usePipelineRuntime's own docs for why
  // this subscription lives here rather than inside the composable.
  unsubscribeLog = Events.On('log:appended', () => {
    void runtime.value?.pump()
  })
})
onUnmounted(() => {
  unsubscribeLog?.()
  runtime.value?.stop()
})

// ── Feed preview (Phase 6c) — a read-only look at one `feed` node's
// persisted items, not the sidebar switchover (Phase 7). Defaults to the
// flow's first feed node and offers a picker when there's more than one. ──
const feedItemsClient: FeedItemsClient = {
  async feedItems(feedId) { return await FeedItems(feedId) },
}

const feedNodes = computed(() => activeFlow.value?.nodes.filter((n) => n.type === 'feed') ?? [])
const selectedFeedNodeId = ref<string | null>(null)

watch(feedNodes, (nodes) => {
  if (!nodes.some((n) => n.id === selectedFeedNodeId.value)) {
    selectedFeedNodeId.value = nodes[0]?.id ?? null
  }
}, { immediate: true })

const previewFeedId = computed(() => {
  const flowId = activeFlow.value?.id
  const node = feedNodes.value.find((n) => n.id === selectedFeedNodeId.value)
  // A feed node's durable feed_item key is its flow-qualified node id.
  return flowId && node ? `${flowId}/${node.id}` : null
})

// ── Copy prompt (Phase 6c) — the flows equivalent of the feed sidebar's
// "Copy feeds config prompt" (see App.vue's copyConfigPrompt command). This
// view has no reachable toast queue (ToastStack is driven by useFeedState,
// mounted as App.vue's sibling — see FlowsView's own module docs above on
// staying out of that path), so success/failure surfaces as a small
// self-clearing inline label instead of a toast. ──────────────────────────
const copyStatus = ref<'idle' | 'success' | 'error'>('idle')
let copyStatusTimer: ReturnType<typeof setTimeout> | undefined

async function onCopyPrompt() {
  try {
    await navigator.clipboard.writeText(buildFlowPrompt())
    copyStatus.value = 'success'
  } catch (err) {
    console.warn('Unable to copy flow prompt', err)
    copyStatus.value = 'error'
  }
  if (copyStatusTimer !== undefined) clearTimeout(copyStatusTimer)
  copyStatusTimer = setTimeout(() => { copyStatus.value = 'idle' }, 2500)
}
onUnmounted(() => {
  if (copyStatusTimer !== undefined) clearTimeout(copyStatusTimer)
})

const showDebug = ref(false)
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
        <span v-if="runtimeError" class="max-w-[220px] truncate whitespace-nowrap text-[11.5px] text-severity-error" data-testid="flow-runtime-error">{{ runtimeError }}</span>
        <span v-if="copyStatus === 'success'" class="whitespace-nowrap text-[11.5px] text-severity-success" data-testid="copy-prompt-status">Prompt copied</span>
        <span v-else-if="copyStatus === 'error'" class="whitespace-nowrap text-[11.5px] text-severity-error" data-testid="copy-prompt-status">Could not copy</span>

        <button
          class="shrink-0 cursor-pointer rounded-lg border border-strong px-3 py-2 text-[12.5px] text-text-2 hover:text-text disabled:cursor-default disabled:opacity-40"
          :disabled="!activeFlow || runtimeRunning"
          data-testid="runtime-run-button"
          @click="runtime?.run()"
        >Run</button>
        <button
          class="shrink-0 cursor-pointer rounded-lg border border-strong px-3 py-2 text-[12.5px] text-text-2 hover:text-text disabled:cursor-default disabled:opacity-40"
          :disabled="!activeFlow || !runtimeRunning"
          data-testid="runtime-stop-button"
          @click="runtime?.stop()"
        >Stop</button>
        <button
          class="shrink-0 cursor-pointer rounded-lg border border-strong px-3 py-2 text-[12.5px] text-text-2 hover:text-text"
          data-testid="copy-prompt-button"
          @click="onCopyPrompt"
        >Copy prompt</button>
        <button
          class="shrink-0 cursor-pointer rounded-lg border border-strong px-3 py-2 text-[12.5px] text-text-2 hover:text-text"
          :class="showDebug ? 'bg-selection' : ''"
          data-testid="debug-toggle-button"
          @click="showDebug = !showDebug"
        >Debug</button>
        <button
          class="shrink-0 cursor-pointer rounded-lg bg-accent px-3.5 py-2 text-[12.5px] font-semibold text-accent-contrast disabled:cursor-default disabled:opacity-40"
          :disabled="!dirty || saving || !activeFlow"
          data-testid="deploy-button"
          @click="deploy"
        >{{ saving ? 'Deploying…' : 'Deploy' }}</button>
      </div>

      <div class="flex min-h-0 flex-1">
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

        <aside
          v-if="showDebug && activeFlow"
          class="flex w-[300px] shrink-0 flex-col border-l border-row bg-sidebar"
          data-testid="flow-debug-aside"
        >
          <div class="min-h-0 flex-1 overflow-hidden" style="height: 60%">
            <FlowDebugPanel
              :flow="activeFlow"
              :latest-run-by-node="latestRunByNode"
              :node-runs="nodeRuns"
              :runtime-summary="runtimeLastRun"
              :running="runtimeRunning"
            />
          </div>
          <div class="flex min-h-0 flex-col border-t border-row" style="height: 40%">
            <div v-if="feedNodes.length > 1" class="shrink-0 border-b border-row px-3 py-2">
              <select
                v-model="selectedFeedNodeId"
                class="w-full rounded-md border border-strong bg-app px-2 py-1 text-[11.5px] text-text"
                data-testid="feed-preview-node-select"
              >
                <option v-for="n in feedNodes" :key="n.id" :value="n.id">{{ n.name || n.id }}</option>
              </select>
            </div>
            <FeedItemsPreview :feed-id="previewFeedId" :client="feedItemsClient" />
          </div>
        </aside>
      </div>
    </section>
  </div>
</template>
