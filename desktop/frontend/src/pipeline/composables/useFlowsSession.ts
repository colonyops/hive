// App-wide flow editor/runtime session. The editor owns a mutable local draft;
// the runtime owns a separate deployed snapshot selected exclusively by the
// active profile in App.vue. Canvas edits therefore cannot affect a running
// graph until their exact saved snapshot is deployed.
import { computed, shallowRef, watch, type ComputedRef, type Ref } from 'vue'
import { GetFlow, GetLayout, ListFlows, SaveFlow, SaveLayout } from '../../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { Commit, NodeRuns, ReadFrom } from '../../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import { flowFromWire, type EditorFlow, type WireFlow } from '../lib/wireFlow'
import { usePipelineEditor, type PipelineEditorClient } from './usePipelineEditor'
import { usePipelineRuntime, type RuntimeSummary } from './usePipelineRuntime'
import type { PipelineClient } from '../driver'

type PipelineEditor = ReturnType<typeof usePipelineEditor>

export interface FlowsSession extends Omit<PipelineEditor, 'deploy' | 'replaceDraft'> {
  flowsOpen: Ref<boolean>
  flowFocusNodeId: Ref<string | null>
  running: ComputedRef<boolean>
  lastRun: ComputedRef<RuntimeSummary | null>
  runtimeError: ComputedRef<string | null>
  /** The active profile's deployed runtime id, or null while it is loading. */
  runtimeFlowId: ComputedRef<string | null>
  /** Profile selection is the only operation which can select a runtime. */
  bindActiveFlow(id: string | undefined): void
  openFlows(focusNodeId?: string): void
  exitFlows(): void
  discardDraft(): Promise<void>
  /** Saves the editor draft and swaps its saved snapshot if it is the active profile. */
  deploy(): Promise<void>
  /** Reloads the active profile's deployed graph after flows:updated. */
  reloadDeployed(): Promise<void>
  pump(): Promise<void>
  runRuntime(): Promise<void>
  stopRuntime(): void
}

export interface FlowsSessionDeps {
  editorClient?: PipelineEditorClient
  runtimeClient?: PipelineClient
}

function defaultEditorClient(): PipelineEditorClient {
  return {
    async listFlows() { return await ListFlows() },
    async getFlow(id) { return await GetFlow(id) },
    async saveFlow(flow) { await SaveFlow(flow) },
    async getLayout(id) { return await GetLayout(id) },
    async saveLayout(id, layout) { await SaveLayout(id, layout) },
    async nodeRuns(flowId, limit) { return await NodeRuns(flowId, limit) },
  }
}

function defaultRuntimeClient(): PipelineClient {
  return {
    async readFrom(consumer, limit) { return await ReadFrom(consumer, limit) },
    async commit(batch) { await Commit(batch) },
  }
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback
}

function deployedSnapshot(wire: WireFlow): EditorFlow {
  // Wails values are JSON-shaped. Never share nodes/config with the editor's
  // mutable draft, including when both were made from the same GetFlow result.
  return flowFromWire(JSON.parse(JSON.stringify(wire)) as WireFlow)
}

function createFlowsSession(deps: Required<FlowsSessionDeps>): FlowsSession {
  const editor = usePipelineEditor(deps.editorClient)
  const { flows, activeFlow, selectFlow, replaceDraft, deploy: saveDraft, ...editorState } = editor

  const flowsOpen = shallowRef(false)
  const flowFocusNodeId = shallowRef<string | null>(null)
  const desiredFlowId = shallowRef<string | undefined>(undefined)
  const runtime = shallowRef<ReturnType<typeof usePipelineRuntime> | null>(null)
  const deployedFlowId = shallowRef<string | null>(null)
  const runtimeLoadError = shallowRef<string | null>(null)

  // All graph reads, swaps, and runtime drains use one tail. A swap is thus
  // behind every earlier pump, and a pump queued during a reload sees an old
  // whole snapshot or a new whole snapshot, never a mutable editor graph.
  let operationTail: Promise<void> = Promise.resolve()
  function serialize<T>(operation: () => Promise<T>): Promise<T> {
    const result = operationTail.then(operation, operation)
    operationTail = result.then(() => undefined, () => undefined)
    return result
  }

  function openFlows(focusNodeId?: string): void {
    flowsOpen.value = true
    flowFocusNodeId.value = focusNodeId ?? null
  }

  function exitFlows(): void {
    flowsOpen.value = false
  }

  /** Replaces a runtime only after a complete valid candidate exists. */
  async function swapRuntime(id: string, wire: WireFlow): Promise<void> {
    // Profile changes are synchronous while reads are in flight. Do not let an
    // older request install a runtime for the newly selected profile.
    if (desiredFlowId.value !== id || wire.id !== id) return

    let snapshot: EditorFlow
    try {
      snapshot = deployedSnapshot(wire)
    } catch (err) {
      runtimeLoadError.value = errorMessage(err, 'Could not load the deployed flow.')
      return
    }

    // serialize() guarantees earlier pumps have drained. Delaying stop until
    // here preserves the last known-good runtime when fetching/parsing fails.
    runtime.value?.stop()
    runtime.value = usePipelineRuntime(deps.runtimeClient, snapshot)
    deployedFlowId.value = id
    runtimeLoadError.value = null
    await runtime.value.run()
  }

  async function loadAndSwap(id: string, refreshDraft: boolean): Promise<void> {
    let wire: WireFlow
    try {
      wire = await deps.editorClient.getFlow(id)
    } catch (err) {
      if (desiredFlowId.value === id) {
        runtimeLoadError.value = errorMessage(err, 'Could not reload the deployed flow.')
      }
      return // Keep the last-good runtime on an external reload failure.
    }

    if (desiredFlowId.value !== id || wire.id !== id) {
      if (desiredFlowId.value === id && wire.id !== id) {
        runtimeLoadError.value = 'Deployed flow identity did not match the active profile.'
      }
      return
    }

    // A profile runtime reload may update its clean draft, but never clobbers
    // unsaved work or an editor draft the user selected for another flow.
    const shouldRefreshDraft = refreshDraft && !editor.dirty.value
    let wireLayout
    if (shouldRefreshDraft) {
      try {
        wireLayout = await deps.editorClient.getLayout(id)
      } catch (err) {
        // Layout is editor-only; a valid runtime must still remain available.
        console.warn('Unable to reload flow layout', id, err)
      }
    }

    if (desiredFlowId.value !== id) return
    await swapRuntime(id, wire)

    if (wireLayout && desiredFlowId.value === id && !editor.dirty.value) {
      replaceDraft(wire, wireLayout)
    }
  }

  let activationPending: string | undefined
  watch([desiredFlowId, flows], ([id, list]) => {
    if (!id || !list.some((flow) => flow.id === id) || activationPending === id) return
    if (deployedFlowId.value === id) return
    activationPending = id
    void serialize(async () => {
      try {
        if (desiredFlowId.value === id) await loadAndSwap(id, true)
      } finally {
        activationPending = undefined
      }
    })
  }, { immediate: true })

  function bindActiveFlow(id: string | undefined): void {
    if (desiredFlowId.value === id) return
    desiredFlowId.value = id

    // Queue this before the watch queues the candidate load. The old profile
    // therefore drains any already-requested work, then cannot keep consuming
    // under a different selected profile if the new candidate fails to load.
    void serialize(async () => {
      if (desiredFlowId.value !== id || deployedFlowId.value === id) return
      runtime.value?.stop()
      deployedFlowId.value = null
    })
  }

  async function discardDraft(): Promise<void> {
    const id = activeFlow.value?.id
    if (!id) return
    await serialize(async () => {
      if (activeFlow.value?.id !== id) return
      try {
        const [wire, wireLayout] = await Promise.all([deps.editorClient.getFlow(id), deps.editorClient.getLayout(id)])
        if (activeFlow.value?.id === id) replaceDraft(wire, wireLayout)
      } catch (err) {
        editor.error.value = errorMessage(err, 'Could not load the flow.')
      }
    })
  }

  async function deploy(): Promise<void> {
    await serialize(async () => {
      const wire = await saveDraft()
      // saveDraft returns precisely the graph it wrote. A later local edit
      // stays dirty and private, but must not prevent this saved graph running.
      if (!wire || wire.id !== desiredFlowId.value) return
      await swapRuntime(wire.id, wire)
    })
  }

  async function reloadDeployed(): Promise<void> {
    await serialize(async () => {
      await editor.refreshFlows()
      const id = desiredFlowId.value
      if (id) await loadAndSwap(id, true)
    })
  }

  async function pump(): Promise<void> {
    await serialize(async () => { await runtime.value?.pump() })
  }

  async function runRuntime(): Promise<void> {
    await serialize(async () => { await runtime.value?.run() })
  }

  function stopRuntime(): void {
    runtime.value?.stop()
  }

  const running = computed(() => runtime.value?.running.value ?? false)
  const lastRun = computed(() => runtime.value?.lastRun.value ?? null)
  const runtimeError = computed(() => runtimeLoadError.value ?? runtime.value?.error.value ?? null)
  // During a profile switch an old runtime may finish an already requested
  // drain. Never expose it as belonging to the newly active profile.
  const runtimeFlowId = computed(() =>
    deployedFlowId.value === desiredFlowId.value ? deployedFlowId.value : null,
  )

  return {
    ...editorState,
    flows,
    activeFlow,
    selectFlow,
    flowsOpen,
    flowFocusNodeId,
    running,
    lastRun,
    runtimeError,
    runtimeFlowId,
    bindActiveFlow,
    openFlows,
    exitFlows,
    discardDraft,
    deploy,
    reloadDeployed,
    pump,
    runRuntime,
    stopRuntime,
  }
}

let sharedSession: FlowsSession | null = null

export function useFlowsSession(deps: FlowsSessionDeps = {}): FlowsSession {
  if (!sharedSession) {
    sharedSession = createFlowsSession({
      editorClient: deps.editorClient ?? defaultEditorClient(),
      runtimeClient: deps.runtimeClient ?? defaultRuntimeClient(),
    })
  }
  return sharedSession
}

export function resetFlowsSessionForTests(): void {
  sharedSession?.stopRuntime()
  sharedSession = null
}
