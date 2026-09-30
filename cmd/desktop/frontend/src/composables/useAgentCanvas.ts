import { ref, shallowRef, watch } from 'vue'
import type { Ref, ShallowRef } from 'vue'
import type { AgentWorkspacesClient, WorkspaceCanvas, WorkspaceCanvasMeta } from '../lib/agentWorkspacesClient'

function message(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

/**
 * One canvas pane's state: the canvas being shown plus the workspace's canvas
 * listing for the picker. Per-pane, not a module singleton — the pane owns the
 * lifetime. The generation guard is useActionsSettings' shape: a canvas:updated
 * wake and a canvas switch can race, and an older response must never replace
 * a newer request's.
 *
 * The shown canvas is the requested name when the route carries one, else a
 * default from the listing: the open chat's most recent canvas, falling back
 * to the workspace's most recent.
 */
export function useAgentCanvas(client: Ref<AgentWorkspacesClient | null>) {
  const canvas: ShallowRef<WorkspaceCanvas | null> = shallowRef(null)
  const metas: Ref<WorkspaceCanvasMeta[]> = ref([])
  const shown: Ref<string | null> = ref(null)
  const workspace = ref('')
  const requested: Ref<string | null> = ref(null)
  const preferSession: Ref<number | null> = ref(null)
  const loading = ref(false)
  const error = ref('')
  let generation = 0
  let queued = false
  let running = false

  function defaultName(listed: WorkspaceCanvasMeta[]): string | null {
    const own = listed.find((meta) => meta.session === preferSession.value)
    return own?.name ?? listed[0]?.name ?? null
  }

  async function reload(): Promise<void> {
    if (running) { generation++; queued = true; return }
    running = true
    const token = ++generation
    const dir = workspace.value
    loading.value = true
    try {
      const listed = dir && client.value ? await client.value.canvases(dir) : []
      const name = requested.value ?? defaultName(listed)
      const loaded = dir && name && client.value ? await client.value.canvas(dir, name) : null
      if (token === generation) {
        metas.value = listed
        shown.value = name
        canvas.value = loaded
        error.value = ''
      }
    } catch (err) {
      if (token === generation) {
        canvas.value = null
        error.value = message(err, 'Could not read the canvas.')
      }
    } finally {
      if (token === generation) loading.value = false
      running = false
      if (queued) { queued = false; void reload() }
    }
  }

  function wake(): void {
    generation++
    if (running) queued = true
    else void reload()
  }

  /**
   * Points the pane at a workspace's canvases: name pins one, null lets the
   * default win, and session is the open chat the default prefers.
   */
  function show(dir: string, name: string | null, session: number | null): void {
    workspace.value = dir
    requested.value = name
    preferSession.value = session
    wake()
  }

  watch(client, () => wake())

  return { canvas, metas, shown, loading, error, show, wake }
}
