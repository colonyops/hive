// App-wide flow editor session: one mutable local draft, plus the flow
// listing every surface reads.
//
// Execution is not here and cannot be. The Go flow engine owns it, reloads
// itself from the same flows/*.yaml change this session refreshes on, and
// keeps running with this window closed. What is left on this side is the
// editor: which flow is being edited, whether it is dirty, and writing it
// back. See docs/architecture.md ▸ Execution model.
import { computed, shallowRef, watch, type ComputedRef, type Ref } from 'vue'
import { GetFlow, GetLayout, ListFlows, SaveFlow, SaveLayout } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/flowsservice'
import { NodeRuns } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import { errorText } from '../../lib/appError'
import { usePipelineEditor, type PipelineEditorClient } from './usePipelineEditor'

type PipelineEditor = ReturnType<typeof usePipelineEditor>

export interface FlowsSession extends PipelineEditor {
  flowsOpen: Ref<boolean>
  flowFocusNodeId: Ref<string | null>
  /**
   * The selected flow's load error, as reported by the Go listing. A flow
   * whose file does not parse keeps its last deployed version running, so this
   * is what tells the author that what is on disk is not what is executing.
   */
  flowLoadError: ComputedRef<string | null>
  /** Binds profile selection to the editor draft. */
  bindActiveFlow(id: string | undefined): void
  openFlows(focusNodeId?: string): void
  exitFlows(): void
  discardDraft(): Promise<void>
  /** Re-reads the listing and, when the open draft is clean, the draft itself. */
  reloadFlows(): Promise<void>
}

export interface FlowsSessionDeps {
  editorClient?: PipelineEditorClient
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

function createFlowsSession(deps: Required<FlowsSessionDeps>): FlowsSession {
  const editor = usePipelineEditor(deps.editorClient)
  const { flows, activeFlow, selectFlow, replaceDraft } = editor

  const flowsOpen = shallowRef(false)
  const flowFocusNodeId = shallowRef<string | null>(null)
  const selectedProfileId = shallowRef<string | undefined>(undefined)

  // Draft loads and saves share one tail, so a reload can never land a stale
  // draft on top of one the user just selected.
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

  let pendingEditorProfile: string | undefined
  async function selectBoundEditor(id: string): Promise<void> {
    if (pendingEditorProfile !== id || selectedProfileId.value !== id || !flows.value.some((flow) => flow.id === id)) return
    try {
      // Profile navigation has already guarded dirty drafts in App.vue.
      await selectFlow(id)
    } finally {
      if (pendingEditorProfile === id) pendingEditorProfile = undefined
    }
  }

  function bindActiveFlow(id: string | undefined): void {
    if (selectedProfileId.value === id) return
    selectedProfileId.value = id
    pendingEditorProfile = id
    if (!id) return
    // When the target flow is not in the editor's list yet (e.g. a just-created
    // profile whose flows:updated has not landed), the bind is deferred until
    // watch(flows) sees it. Drop the previous profile's draft now: otherwise the
    // canvas keeps showing — and lets the user edit — the wrong flow, and the
    // deferred selectFlow/replaceDraft silently discards those edits (a renamed
    // node reverting, Deploy greying out) the moment it finally runs.
    if (!flows.value.some((flow) => flow.id === id)) editor.clearFlow()
    void serialize(async () => { await selectBoundEditor(id) })
  }

  // The editor's initial ListFlows is asynchronous. Complete a profile/editor
  // binding that happened while the list was still loading.
  watch(flows, () => {
    void serialize(async () => {
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
        editor.error.value = errorText(err, 'Could not load the flow.')
      }
    })
  }

  async function refreshCleanEditorDraft(): Promise<void> {
    const id = activeFlow.value?.id
    if (!id || editor.dirty.value) return
    try {
      const [wire, wireLayout] = await Promise.all([deps.editorClient.getFlow(id), deps.editorClient.getLayout(id)])
      if (activeFlow.value?.id === id && !editor.dirty.value) replaceDraft(wire, wireLayout)
    } catch (err) {
      editor.error.value = errorText(err, 'Could not reload the flow.')
    }
  }

  async function reloadFlows(): Promise<void> {
    await serialize(async () => {
      await editor.refreshFlows()
      await refreshCleanEditorDraft()
    })
  }

  const flowLoadError = computed(() => {
    const id = selectedProfileId.value ?? activeFlow.value?.id
    if (!id) return null
    return flows.value.find((flow) => flow.id === id)?.error || null
  })

  return {
    ...editor,
    flowsOpen,
    flowFocusNodeId,
    flowLoadError,
    bindActiveFlow,
    openFlows,
    exitFlows,
    discardDraft,
    reloadFlows,
  }
}

let sharedSession: FlowsSession | null = null

export function useFlowsSession(deps: FlowsSessionDeps = {}): FlowsSession {
  if (!sharedSession) {
    sharedSession = createFlowsSession({
      editorClient: deps.editorClient ?? defaultEditorClient(),
    })
  }
  return sharedSession
}

export function resetFlowsSessionForTests(): void {
  sharedSession = null
}
