// App-wide flow editor/runtime session. The editor owns one mutable local
// draft, while the deployed-runtime manager owns an independent snapshot for
// every enabled flow. Canvas/profile selection therefore never gates work.
import { computed, shallowRef, watch, type ComputedRef, type Ref } from 'vue'
import { GetFlow, GetLayout, ListFlows, SaveFlow, SaveLayout } from '../../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { Commit, NodeRuns, ReadFrom } from '../../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import { flowFromWire, type EditorFlow, type WireFlow } from '../lib/wireFlow'
import { usePipelineEditor, type PipelineEditorClient } from './usePipelineEditor'
import { usePipelineRuntime, type RuntimeSummary } from './usePipelineRuntime'
import type { PipelineClient } from '../driver'

type PipelineEditor = ReturnType<typeof usePipelineEditor>
type PipelineRuntime = ReturnType<typeof usePipelineRuntime>

export interface FlowsSession extends Omit<PipelineEditor, 'deploy' | 'replaceDraft'> {
  flowsOpen: Ref<boolean>
  flowFocusNodeId: Ref<string | null>
  running: ComputedRef<boolean>
  lastRun: ComputedRef<RuntimeSummary | null>
  runtimeError: ComputedRef<string | null>
  /** The selected profile's runtime id, or null when it has no enabled runtime. */
  runtimeFlowId: ComputedRef<string | null>
  /** Binds profile selection to the editor only; it never controls runtimes. */
  bindActiveFlow(id: string | undefined): void
  openFlows(focusNodeId?: string): void
  exitFlows(): void
  discardDraft(): Promise<void>
  /** Saves the editor draft, then updates that flow's runtime if it is enabled. */
  deploy(): Promise<void>
  /** Reconciles every enabled deployed runtime after flows:updated. */
  reloadDeployed(): Promise<void>
  /** Drains every enabled runtime. */
  pump(): Promise<void>
  /** Starts every managed runtime (normally they are started by reconciliation). */
  runRuntime(): Promise<void>
  /** Pauses every managed runtime; runRuntime() can restart them. */
  stopRuntime(): void
  /** Permanently disposes every managed runtime; used on session shutdown. */
  disposeRuntime(): void
}

export interface FlowsSessionDeps {
  editorClient?: PipelineEditorClient
  runtimeClient?: PipelineClient
  /** Test seam for observing runtime lifecycle without constructing browser workers. */
  runtimeFactory?: typeof usePipelineRuntime
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
  // mutable draft, including when both came from the same GetFlow result.
  return flowFromWire(JSON.parse(JSON.stringify(wire)) as WireFlow)
}

function createFlowsSession(deps: Required<FlowsSessionDeps>): FlowsSession {
  const editor = usePipelineEditor(deps.editorClient)
  const { flows, activeFlow, selectFlow, replaceDraft, deploy: saveDraft, ...editorState } = editor

  const flowsOpen = shallowRef(false)
  const flowFocusNodeId = shallowRef<string | null>(null)
  const selectedProfileId = shallowRef<string | undefined>(undefined)
  const runtimes = shallowRef<Map<string, PipelineRuntime>>(new Map())
  const runtimeLoadErrors = shallowRef<Map<string, string>>(new Map())

  // Lifecycle changes and log drains share one tail. A reload can therefore
  // never replace a graph halfway through one of its commits.
  let operationTail: Promise<void> = Promise.resolve()
  function serialize<T>(operation: () => Promise<T>): Promise<T> {
    const result = operationTail.then(operation, operation)
    operationTail = result.then(() => undefined, () => undefined)
    return result
  }

  function setRuntime(id: string, runtime: PipelineRuntime): void {
    const next = new Map(runtimes.value)
    next.set(id, runtime)
    runtimes.value = next
  }

  function removeRuntime(id: string): void {
    const existing = runtimes.value.get(id)
    if (!existing) return
    existing.dispose()
    const next = new Map(runtimes.value)
    next.delete(id)
    runtimes.value = next
  }

  function setRuntimeLoadError(id: string, message: string | null): void {
    const next = new Map(runtimeLoadErrors.value)
    if (message) next.set(id, message)
    else next.delete(id)
    runtimeLoadErrors.value = next
  }

  function isEnabledFlow(id: string): boolean {
    return flows.value.some((flow) => flow.id === id && flow.valid && flow.enabled)
  }

  /** Replaces one runtime only after a complete valid candidate exists. */
  async function loadRuntime(id: string): Promise<void> {
    let wire: WireFlow
    try {
      wire = await deps.editorClient.getFlow(id)
    } catch (err) {
      if (isEnabledFlow(id)) setRuntimeLoadError(id, errorMessage(err, 'Could not reload the deployed flow.'))
      return // An external reload failure keeps the last known-good runtime.
    }

    if (!isEnabledFlow(id) || wire.id !== id) {
      if (isEnabledFlow(id) && wire.id !== id) {
        setRuntimeLoadError(id, 'Deployed flow identity did not match its listing.')
      }
      return
    }

    let snapshot: EditorFlow
    try {
      snapshot = deployedSnapshot(wire)
    } catch (err) {
      setRuntimeLoadError(id, errorMessage(err, 'Could not load the deployed flow.'))
      return
    }

    // The serialized caller has drained all earlier work. Do not stop a
    // usable runtime until the replacement snapshot has been validated.
    removeRuntime(id)
    const runtime = deps.runtimeFactory(deps.runtimeClient, snapshot)
    setRuntime(id, runtime)
    setRuntimeLoadError(id, null)
    await runtime.run()
  }

  /** Starts missing enabled runtimes and stops disabled/deleted ones. */
  async function reconcileRuntimes(reload: boolean): Promise<void> {
    const enabledIDs = new Set(flows.value.filter((flow) => flow.valid && flow.enabled).map((flow) => flow.id))

    for (const id of runtimes.value.keys()) {
      if (!enabledIDs.has(id)) {
        removeRuntime(id)
        setRuntimeLoadError(id, null)
      }
    }

    for (const id of enabledIDs) {
      if (reload || !runtimes.value.has(id)) await loadRuntime(id)
    }
  }

  function openFlows(focusNodeId?: string): void {
    flowsOpen.value = true
    flowFocusNodeId.value = focusNodeId ?? null
  }

  function exitFlows(): void {
    flowsOpen.value = false
  }

  let pendingEditorProfile: string | undefined
  async function selectBoundEditor(id: string): Promise<void> {
    if (pendingEditorProfile !== id || selectedProfileId.value !== id || !flows.value.some((flow) => flow.id === id)) return
    try {
      // Profile navigation has already guarded dirty drafts in App.vue. This
      // selection does not touch runtime ownership, which remains per-flow.
      await selectFlow(id)
    } finally {
      if (pendingEditorProfile === id) pendingEditorProfile = undefined
    }
  }

  function bindActiveFlow(id: string | undefined): void {
    if (selectedProfileId.value === id) return
    selectedProfileId.value = id
    pendingEditorProfile = id
    if (id) void serialize(async () => { await selectBoundEditor(id) })
  }

  // The editor's initial ListFlows is asynchronous. Once it arrives, start
  // all enabled runtimes, and complete a profile/editor binding that happened
  // while the list was still loading. Later list refreshes only add/remove
  // runtimes; they deliberately do not overwrite an independently selected
  // editor draft.
  watch(flows, () => {
    void serialize(async () => {
      await reconcileRuntimes(false)
      if (pendingEditorProfile) await selectBoundEditor(pendingEditorProfile)
    })
  }, { immediate: true })

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
      if (!wire) return
      if (wire.enabled) await loadRuntime(wire.id)
      else removeRuntime(wire.id)
    })
  }

  async function refreshCleanEditorDraft(): Promise<void> {
    const id = activeFlow.value?.id
    if (!id || editor.dirty.value) return
    try {
      const [wire, wireLayout] = await Promise.all([deps.editorClient.getFlow(id), deps.editorClient.getLayout(id)])
      if (activeFlow.value?.id === id && !editor.dirty.value) replaceDraft(wire, wireLayout)
    } catch (err) {
      editor.error.value = errorMessage(err, 'Could not reload the flow.')
    }
  }

  async function reloadDeployed(): Promise<void> {
    await serialize(async () => {
      await editor.refreshFlows()
      await reconcileRuntimes(true)
      await refreshCleanEditorDraft()
    })
  }

  async function pump(): Promise<void> {
    await serialize(async () => {
      await Promise.all([...runtimes.value.values()].map(async (runtime) => { await runtime.pump() }))
    })
  }

  async function runRuntime(): Promise<void> {
    await serialize(async () => {
      await Promise.all([...runtimes.value.values()].map(async (runtime) => { await runtime.run() }))
    })
  }

  function stopRuntime(): void {
    for (const runtime of runtimes.value.values()) runtime.stop()
  }

  function disposeRuntime(): void {
    for (const runtime of runtimes.value.values()) runtime.dispose()
    runtimes.value = new Map()
  }

  const selectedRuntime = computed(() => selectedProfileId.value ? runtimes.value.get(selectedProfileId.value) : undefined)
  const running = computed(() => selectedRuntime.value?.running.value ?? false)
  const lastRun = computed(() => selectedRuntime.value?.lastRun.value ?? null)
  const runtimeError = computed(() => {
    const id = selectedProfileId.value
    return (id ? runtimeLoadErrors.value.get(id) : null) ?? selectedRuntime.value?.error.value ?? null
  })
  const runtimeFlowId = computed(() => selectedRuntime.value ? selectedProfileId.value ?? null : null)

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
    disposeRuntime,
  }
}

let sharedSession: FlowsSession | null = null

export function useFlowsSession(deps: FlowsSessionDeps = {}): FlowsSession {
  if (!sharedSession) {
    sharedSession = createFlowsSession({
      editorClient: deps.editorClient ?? defaultEditorClient(),
      runtimeClient: deps.runtimeClient ?? defaultRuntimeClient(),
      runtimeFactory: deps.runtimeFactory ?? usePipelineRuntime,
    })
  }
  return sharedSession
}

export function resetFlowsSessionForTests(): void {
  sharedSession?.disposeRuntime()
  sharedSession = null
}
