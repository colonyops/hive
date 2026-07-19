// The app-level flows session (hc-8ft4yhm6, "always-on per-profile flow
// runtime"): a MODULE SINGLETON that owns the pipeline editor
// (usePipelineEditor) and the runtime that pumps commits into feed_item
// (usePipelineRuntime) for the active profile's flow, so that feed_item gets
// written continuously — not only while the flows canvas (FlowsView.vue) is
// mounted. App.vue and FlowsView.vue both call useFlowsSession() and get
// back the SAME instance: App.vue drives it from the profile switcher and
// the app-wide "log:appended" subscription (so the sidebar populates with
// the canvas closed); FlowsView.vue reads the same editor/runtime state to
// render the canvas, toolbar, and debug panel.
//
// Scope note: this runs only the ACTIVE profile's flow (one shared runtime
// instance, rebuilt whenever the active flow changes — see the watch
// below). Hayden's stated ultimate goal is to run every ENABLED profile's
// flow concurrently (flow.Enabled already exists on the schema) — that's a
// follow-up, not implemented here. Search for "hc-8ft4yhm6" if extending
// this to multiple concurrent runtimes.
//
// Like usePipelineRuntime.ts and usePipelineEditor.ts, this composable's
// core takes injected clients and never imports bindings/ or
// @wailsio/runtime directly — a default adapter over the real Wails
// bindings is built lazily (buildDefaultDeps) only when no deps are
// supplied, so this file stays importable/unit-testable with a fake client
// (see __tests__/useFlowsSession.spec.ts).
//
// Per usePipelineRuntime's own module docs, the "log:appended" Wails event
// subscription is deliberately NOT owned here — App.vue owns it and calls
// the pump() this composable exposes on every tick, then refreshes
// useFeedState AFTER that pump resolves (commit-then-refresh ordering is
// the whole point of hc-8ft4yhm6; see App.vue).
//
// ── Singleton + test isolation ──────────────────────────────────────────
// The instance is created on the FIRST call to useFlowsSession() and reused
// on every later call, regardless of caller or deps — this is what makes
// App.vue and FlowsView.vue share one editor + one runtime. Because
// usePipelineEditor() registers onMounted/onUnmounted (its node_run poll
// timer) and the runtime watch below registers a `watch()`, that first call
// MUST happen synchronously inside a component's setup() (App.vue's
// top-level <script setup> in production) so those hooks bind to an active
// component instance/effect scope.
//
// Tests that mount fresh App/host components per test must call
// resetFlowsSessionForTests() (e.g. in beforeEach) so each mount creates
// its own instance — otherwise a later test would silently reuse an
// instance whose onMounted/watch already tore down with a previous test's
// unmount.
import { computed, shallowRef, watch, type ComputedRef, type Ref } from 'vue'
import { GetFlow, GetLayout, ListFlows, SaveFlow, SaveLayout } from '../../../bindings/github.com/colonyops/hive/desktop/flowsservice'
import { Commit, NodeRuns, ReadFrom } from '../../../bindings/github.com/colonyops/hive/desktop/pipelineservice'
import { usePipelineEditor, type PipelineEditorClient } from './usePipelineEditor'
import { usePipelineRuntime, type RuntimeSummary } from './usePipelineRuntime'
import type { PipelineClient } from '../driver'

export interface FlowsSessionDeps {
  /** Defaults to a real adapter over FlowsService/PipelineService's Wails bindings. */
  editorClient?: PipelineEditorClient
  /** Defaults to a real adapter over PipelineService.ReadFrom/Commit's Wails bindings. */
  runtimeClient?: PipelineClient
}

export interface FlowsSession extends ReturnType<typeof usePipelineEditor> {
  /** Whether the flows canvas (FlowsView) is the active main-region view. */
  flowsOpen: Ref<boolean>
  /** Node to focus/scroll-to once the canvas opens — set by openFlows(id). Consumed by the 8d nav task; unused today. */
  flowFocusNodeId: Ref<string | null>
  /** True while the shared runtime for the active flow is running. */
  running: ComputedRef<boolean>
  /** The active runtime's most recent pump() outcome, or null before the first pump. */
  lastRun: ComputedRef<RuntimeSummary | null>
  /** The active runtime's last pump failure message, or null. Distinct from `error` (the editor's own load/save error). */
  runtimeError: ComputedRef<string | null>
  /** Sets the flow the session should track (mirrors FlowsView's old `watch([flowId, flows])`) — selects it once it's in the loaded `flows` list. Called by App.vue whenever the active profile changes. */
  bindActiveFlow(id: string | undefined): void
  /** Opens the flows canvas, optionally focusing a node once it's up. */
  openFlows(focusNodeId?: string): void
  /** Returns to the feed view. */
  exitFlows(): void
  /** Delegates to the active flow's runtime pump() — a no-op if no flow is bound or the runtime isn't running. Called by App.vue on every "log:appended" event. */
  pump(): Promise<void>
  /**
   * Manually starts the active flow's runtime (idempotent) — the runtime
   * already auto-starts when a flow becomes active, so this only matters
   * after an explicit stopRuntime(). Not currently wired into FlowsView's
   * UI (its deploy-menu "Refresh now" calls pump() directly instead, which
   * works whether or not the runtime is running); kept on the session API
   * for callers that do need to resume a stopped runtime.
   */
  runRuntime(): Promise<void>
  /**
   * Manually stops the active flow's runtime, pausing feed_item commits
   * for this profile app-wide until runRuntime() is called again. Not
   * currently wired into FlowsView's UI — see runRuntime()'s docs above.
   */
  stopRuntime(): void
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

// Adapts PipelineService.ReadFrom/Commit into driver.ts's injected
// PipelineClient shape, wrapped in async functions (not passed directly) so
// the return type is a plain Promise, not Wails's CancellablePromise —
// same posture as FlowsView.vue's old `pipelineClient` adapter.
function defaultRuntimeClient(): PipelineClient {
  return {
    async readFrom(offset, limit) { return await ReadFrom(offset, limit) },
    async commit(batch) { await Commit(batch) },
  }
}

function createFlowsSession(deps: Required<FlowsSessionDeps>): FlowsSession {
  const editor = usePipelineEditor(deps.editorClient)
  const { flows, activeFlow } = editor

  const flowsOpen = shallowRef(false)
  const flowFocusNodeId = shallowRef<string | null>(null)

  // The flow the session has been told to track (App.vue calls
  // bindActiveFlow() whenever the active profile changes). Kept separate
  // from activeFlow.value?.id because the desired id may arrive before
  // `flows` has finished its first load — the watch below re-checks
  // whenever either changes, exactly like FlowsView's old
  // `watch([() => props.flowId, flows], ...)`.
  const desiredFlowId = shallowRef<string | undefined>(undefined)

  function bindActiveFlow(id: string | undefined): void {
    desiredFlowId.value = id
  }

  watch([desiredFlowId, flows], ([id, list]) => {
    if (!id || activeFlow.value?.id === id) return
    if (list.some((f) => f.id === id)) void editor.selectFlow(id)
  }, { immediate: true })

  function openFlows(focusNodeId?: string): void {
    flowsOpen.value = true
    flowFocusNodeId.value = focusNodeId ?? null
  }

  function exitFlows(): void {
    flowsOpen.value = false
  }

  // ── Runtime (hc-8ft4yhm6) ─────────────────────────────────────────────
  // One usePipelineRuntime instance per active flow — rebuilt whenever the
  // active flow's id changes (a different flow is a different commit
  // consumer and cursor), but NOT on every node/wire edit: addNode/
  // updateNode/etc. mutate the same activeFlow object in place (see
  // usePipelineEditor.ts), so the driver's captured Flow reference already
  // sees those edits on its next pump() without needing a new instance.
  // Unlike FlowsView's old per-canvas instance, this one is started
  // (run()) the moment a flow becomes active, so it pumps immediately and
  // keeps pumping via App.vue's "log:appended" subscription regardless of
  // whether the canvas is ever opened.
  const runtime = shallowRef<ReturnType<typeof usePipelineRuntime> | null>(null)

  watch(() => activeFlow.value?.id, () => {
    runtime.value?.stop()
    runtime.value = activeFlow.value ? usePipelineRuntime(deps.runtimeClient, activeFlow.value) : null
    if (runtime.value) void runtime.value.run()
  }, { immediate: true })

  // Flattened, template-friendly reads of the current runtime's nested
  // refs — a template (or another computed) accessing
  // `runtime.value.running.value` through a shallowRef would not
  // auto-unwrap (Vue only auto-unwraps a ref's own top-level binding), so
  // these computeds do it explicitly instead. Mirrors FlowsView's old
  // runtimeRunning/runtimeLastRun/runtimeError computeds.
  const running = computed(() => runtime.value?.running.value ?? false)
  const lastRun = computed(() => runtime.value?.lastRun.value ?? null)
  const runtimeError = computed(() => runtime.value?.error.value ?? null)

  async function pump(): Promise<void> {
    await runtime.value?.pump()
  }

  async function runRuntime(): Promise<void> {
    await runtime.value?.run()
  }

  function stopRuntime(): void {
    runtime.value?.stop()
  }

  return {
    ...editor,
    flowsOpen,
    flowFocusNodeId,
    running,
    lastRun,
    runtimeError,
    bindActiveFlow,
    openFlows,
    exitFlows,
    pump,
    runRuntime,
    stopRuntime,
  }
}

let sharedSession: FlowsSession | null = null

/**
 * Returns the shared FlowsSession instance, creating it on the first call.
 * Every later call — from App.vue, FlowsView.vue, or anywhere else — gets
 * back the exact same instance; `deps` is only consulted the first time
 * (see the module docs above on why the first call must happen inside a
 * component's setup()).
 */
export function useFlowsSession(deps: FlowsSessionDeps = {}): FlowsSession {
  if (!sharedSession) {
    sharedSession = createFlowsSession({
      editorClient: deps.editorClient ?? defaultEditorClient(),
      runtimeClient: deps.runtimeClient ?? defaultRuntimeClient(),
    })
  }
  return sharedSession
}

/**
 * Test-only: clears the shared instance so the next useFlowsSession() call
 * builds a fresh one (optionally with newly-injected deps). Call this in
 * `beforeEach`/`afterEach` for any spec that mounts a component tree
 * calling useFlowsSession() (App.vue, FlowsView.vue) more than once —
 * without it, later tests would silently reuse a prior test's instance,
 * including its already-torn-down onMounted/watch hooks.
 */
export function resetFlowsSessionForTests(): void {
  sharedSession = null
}
