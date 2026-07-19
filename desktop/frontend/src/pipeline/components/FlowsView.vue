<script setup lang="ts">
// The flows editor view (8a shell): a 214px node palette, the canvas toolbar
// (flow-selector · node/wire counts · zoom/Fit · Deploy), the canvas itself,
// and a bottom status strip (dirty state + aggregate node status counts).
// This is the one place in src/pipeline that imports the real Wails
// bindings and the "flows:updated" event — usePipelineEditor's own core
// takes an injected client and never touches @wailsio/runtime or bindings/
// directly (see its module docs); this component is the adapter
// FlowsService/PipelineService's generated bindings are wired through,
// mirroring driver.ts's documented injection posture (a real PipelineClient
// adapter would be wired up the same way).
//
// Individual refs/actions are destructured out of usePipelineEditor()
// (rather than kept as one `editor` object) so the template can use them
// directly without a `.value` on every access — the same convention
// useFeedState() + App.vue already use.
import { computed, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconMaximize2 from '~icons/lucide/maximize-2'
import IconMinus from '~icons/lucide/minus'
import IconPlus from '~icons/lucide/plus'
import IconWorkflow from '~icons/lucide/workflow'
import { GetFlow, GetLayout, ListFlows, SaveFlow, SaveLayout } from '../../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { Commit, FeedItems, NodeRuns, ReadFrom } from '../../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import { usePipelineEditor, type PipelineEditorClient } from '../composables/usePipelineEditor'
import { usePipelineRuntime } from '../composables/usePipelineRuntime'
import type { PipelineClient } from '../driver'
import { classify } from '../lib/runStatus'
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
  refreshFlows, refreshNodeRuns, selectFlow, newFlow, addNode, updateNode, deleteNode, addWire, removeWire, moveNode, deploy,
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
const nodeCount = computed(() => activeFlow.value?.nodes.length ?? 0)
const wireCount = computed(() => activeFlow.value?.wires.length ?? 0)

// ── Aggregate node status counts (bottom status strip) — classifies every
// node in the active flow through the same lib/runStatus.ts helper
// FlowsCanvas uses per-card, so the strip's counts can never disagree with
// what the graph shows. Nothing sets a node "running" yet (see
// lib/runStatus.ts's module docs), so that bucket stays 0 until the
// always-on runtime work (hc-8ft4yhm6) gives it a real per-node signal. ────
const statusCounts = computed(() => {
  const counts = { idle: 0, running: 0, ok: 0, error: 0 }
  for (const node of activeFlow.value?.nodes ?? []) {
    counts[classify(latestRunByNode.value.get(node.id), false)]++
  }
  return counts
})

// ── Flow selector (toolbar dropdown — replaces the old flow-list sidebar) ──

const flowMenuOpen = ref(false)
const newFlowName = ref('')

function pickFlow(id: string) {
  void selectFlow(id)
  flowMenuOpen.value = false
}

function submitNewFlow() {
  const name = newFlowName.value.trim()
  if (!name) return
  newFlow(name)
  newFlowName.value = ''
  flowMenuOpen.value = false
}

function onPaletteAdd(type: string) {
  if (activeFlow.value) addNode(type)
}

// ── Canvas zoom/Fit — the buttons live in this toolbar, the zoom/pan state
// and fit() logic stay inside FlowsCanvas (it owns the viewport measurement
// and node geometry); canvasRef reaches through to drive them. ─────────────

const canvasRef = ref<InstanceType<typeof FlowsCanvas> | null>(null)
const zoomPercent = computed(() => Math.round((canvasRef.value?.zoom ?? 1) * 100))

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

// ── Deploy split-button menu — demotes Run/Stop/Copy prompt/Debug behind
// the "▾" so the main Deploy action reads as one clear amber affordance. ───
const deployMenuOpen = ref(false)

function runDeployMenuAction(action: () => void) {
  action()
  deployMenuOpen.value = false
}

const showDebug = ref(false)
</script>

<template>
  <div class="flex h-full min-h-0 flex-1" data-testid="flows-view">
    <aside class="w-[214px] shrink-0 border-r border-row bg-pane" data-testid="flows-palette-rail">
      <NodePalette @add="onPaletteAdd" />
    </aside>

    <section class="flex min-w-0 flex-1 flex-col">
      <div class="flex h-11 shrink-0 items-center gap-2.5 border-b border-row bg-canvas-toolbar px-3.5" data-testid="canvas-toolbar">
        <div class="relative">
          <button
            type="button"
            class="flex h-[30px] cursor-pointer items-center gap-1.5 rounded-lg border border-strong bg-chip px-2.5 text-[12.5px] text-text hover:border-card"
            data-testid="flow-selector-toggle"
            @click="flowMenuOpen = !flowMenuOpen"
          >
            <IconWorkflow class="size-3.5 shrink-0 text-accent" />
            <span class="max-w-[180px] truncate">{{ activeFlow?.name || 'Select a flow' }}</span>
            <IconChevronDown class="size-3.5 shrink-0 text-text-3" />
          </button>

          <div
            v-if="flowMenuOpen"
            class="absolute left-0 top-[calc(100%+6px)] z-20 w-[240px] overflow-hidden rounded-lg border border-strong bg-pane shadow-[0_20px_50px_-14px_rgba(0,0,0,.5)]"
            data-testid="flow-selector-menu"
          >
            <div class="hive-scroll max-h-[240px] overflow-y-auto p-1">
              <button
                v-for="f in flows"
                :key="f.id"
                type="button"
                class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-hover"
                :class="f.id === activeFlow?.id ? 'bg-selection' : ''"
                :data-testid="`flow-selector-option-${f.id}`"
                @click="pickFlow(f.id)"
              >
                <span class="size-1.5 shrink-0 rounded-full" :class="!f.valid ? 'bg-severity-error' : f.enabled ? 'bg-severity-success' : 'bg-text-4'" />
                <span class="min-w-0 flex-1 truncate text-[12.5px] text-text">{{ f.name || f.id }}</span>
              </button>
            </div>
            <div class="flex items-center gap-1.5 border-t border-row p-1.5">
              <input
                v-model="newFlowName"
                type="text"
                placeholder="New flow name…"
                class="w-full min-w-0 rounded-md border border-strong bg-app px-2 py-1.5 text-[12px] text-text outline-none placeholder:text-text-4 focus:border-accent"
                data-testid="flow-selector-new-name"
                @keydown.enter="submitNewFlow"
              >
              <button class="shrink-0 cursor-pointer rounded-md bg-accent px-2 py-1.5 text-[11.5px] font-semibold text-accent-contrast" data-testid="flow-selector-new-submit" @click="submitNewFlow">Add</button>
            </div>
          </div>
        </div>

        <span class="whitespace-nowrap font-mono text-[11px] text-text-3" data-testid="canvas-node-wire-count">{{ nodeCount }} nodes · {{ wireCount }} wires</span>

        <div class="flex-1" />

        <div class="flex h-[30px] items-center overflow-hidden rounded-lg border border-strong bg-chip font-mono text-[12px] text-text-2">
          <button class="flex h-full cursor-pointer items-center px-2.5 hover:bg-hover hover:text-text" data-testid="canvas-zoom-out" @click="canvasRef?.zoomOut()"><IconMinus class="size-3.5" /></button>
          <span class="flex h-full items-center px-1 text-text" data-testid="canvas-zoom-level">{{ zoomPercent }}%</span>
          <button class="flex h-full cursor-pointer items-center px-2.5 hover:bg-hover hover:text-text" data-testid="canvas-zoom-in" @click="canvasRef?.zoomIn()"><IconPlus class="size-3.5" /></button>
        </div>
        <button class="flex h-[30px] cursor-pointer items-center gap-1.5 whitespace-nowrap rounded-lg border border-strong bg-chip px-2.5 text-[12px] text-text-2 hover:border-card hover:text-text" data-testid="canvas-fit" @click="canvasRef?.fit()"><IconMaximize2 class="size-3.5" />Fit</button>

        <div class="mx-0.5 h-5 w-px bg-row" />

        <div class="relative flex h-[30px] items-center">
          <button
            class="flex h-full cursor-pointer items-center rounded-l-lg bg-accent pl-2.5 pr-2 text-[12.5px] font-semibold text-accent-contrast disabled:cursor-default disabled:opacity-40"
            :disabled="!dirty || saving || !activeFlow"
            data-testid="deploy-button"
            @click="deploy"
          >
            <span class="mr-1.5 inline-flex items-center gap-1.5">
              <span v-if="dirty" class="size-[7px] rounded-full bg-accent-contrast/50" data-testid="deploy-dirty-dot" />
              {{ saving ? 'Deploying…' : 'Deploy' }}
            </span>
          </button>
          <button
            class="flex h-full cursor-pointer items-center rounded-r-lg bg-accent pl-1 pr-2.5 text-accent-contrast opacity-70 hover:opacity-100"
            data-testid="deploy-menu-toggle"
            @click="deployMenuOpen = !deployMenuOpen"
          ><IconChevronDown class="size-3.5" /></button>

          <div
            v-if="deployMenuOpen"
            class="absolute right-0 top-[calc(100%+6px)] z-20 w-[170px] overflow-hidden rounded-lg border border-strong bg-pane py-1 shadow-[0_20px_50px_-14px_rgba(0,0,0,.5)]"
            data-testid="deploy-menu"
          >
            <button
              class="flex w-full cursor-pointer items-center px-3 py-1.5 text-left text-[12.5px] text-text-2 hover:bg-hover hover:text-text disabled:cursor-default disabled:opacity-40"
              :disabled="!activeFlow || runtimeRunning"
              data-testid="deploy-menu-run"
              @click="runDeployMenuAction(() => runtime?.run())"
            >Run</button>
            <button
              class="flex w-full cursor-pointer items-center px-3 py-1.5 text-left text-[12.5px] text-text-2 hover:bg-hover hover:text-text disabled:cursor-default disabled:opacity-40"
              :disabled="!activeFlow || !runtimeRunning"
              data-testid="deploy-menu-stop"
              @click="runDeployMenuAction(() => runtime?.stop())"
            >Stop</button>
            <button
              class="flex w-full cursor-pointer items-center px-3 py-1.5 text-left text-[12.5px] text-text-2 hover:bg-hover hover:text-text"
              data-testid="deploy-menu-copy-prompt"
              @click="runDeployMenuAction(onCopyPrompt)"
            >Copy prompt</button>
            <button
              class="flex w-full cursor-pointer items-center px-3 py-1.5 text-left text-[12.5px] text-text-2 hover:bg-hover hover:text-text"
              :class="showDebug ? 'bg-selection' : ''"
              data-testid="deploy-menu-debug-toggle"
              @click="runDeployMenuAction(() => { showDebug = !showDebug })"
            >Debug</button>
          </div>
        </div>
      </div>

      <div class="flex min-h-0 flex-1">
        <FlowsCanvas
          v-if="activeFlow"
          ref="canvasRef"
          :flow="activeFlow"
          :layout="layout"
          :latest-run-by-node="latestRunByNode"
          @move="moveNode"
          @update-node="updateNode"
          @delete-node="deleteNode"
          @add-wire="addWire"
          @remove-wire="removeWire"
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

      <div class="flex h-[30px] shrink-0 items-center gap-3.5 border-t border-row bg-canvas-toolbar px-3.5 font-mono text-[10.5px] text-text-3" data-testid="canvas-status-strip">
        <span v-if="dirty" class="flex items-center gap-1.5" data-testid="flow-dirty-indicator">
          <span class="size-1.5 shrink-0 rounded-full bg-accent" />unsaved changes — deploy to write <span class="text-text-2">{{ filePath }}</span>
        </span>
        <span v-else-if="activeFlow" data-testid="flow-saved-indicator">{{ filePath }}</span>
        <span v-if="error" class="max-w-[240px] truncate text-severity-error" data-testid="flow-editor-error">{{ error }}</span>
        <span v-if="runtimeError" class="max-w-[200px] truncate text-severity-error" data-testid="flow-runtime-error">{{ runtimeError }}</span>
        <span v-if="copyStatus !== 'idle'" :class="copyStatus === 'success' ? 'text-severity-success' : 'text-severity-error'" data-testid="copy-prompt-status">
          {{ copyStatus === 'success' ? 'Prompt copied' : 'Could not copy' }}
        </span>

        <div class="flex-1" />

        <span v-if="activeFlow" class="text-severity-success" data-testid="status-count-ok">● {{ statusCounts.ok }} ok</span>
        <span v-if="activeFlow" class="text-severity-running" data-testid="status-count-running">● {{ statusCounts.running }} running</span>
        <span v-if="activeFlow" class="text-text-4" data-testid="status-count-idle">● {{ statusCounts.idle }} idle</span>
        <span v-if="activeFlow" class="text-severity-error" data-testid="status-count-error">● {{ statusCounts.error }} error</span>
      </div>
    </section>
  </div>
</template>
