import { Events } from '@wailsio/runtime'
import { onMounted, onUnmounted, ref } from 'vue'
import { CreateAction, DeleteAction, ListActions, UpdateAction } from '../../bindings/github.com/colonyops/hive/desktop/actionsservice'
import type { EditableAction } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/actions/models'

export type { EditableAction }
export type ActionType = EditableAction['type']

function message(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

// Wails mutation notifications and fsnotify can both report the same write.
// One queued read is enough; the generation prevents an older Promise from
// replacing a catalog requested after it began.
export function useActionsSettings() {
  const actions = ref<EditableAction[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  let generation = 0
  let queued = false
  let running = false
  let unsubscribe: (() => void) | undefined

  async function reload(): Promise<void> {
    if (running) { generation++; queued = true; return }
    running = true
    const token = ++generation
    loading.value = true
    try {
      const catalog = await ListActions()
      if (token === generation) {
        actions.value = catalog?.actions ?? []
        error.value = catalog?.error || null
      }
    } catch (err) {
      if (token === generation) error.value = message(err, 'Could not load actions.')
    } finally {
      if (token === generation) loading.value = false
      running = false
      if (queued) { queued = false; void reload() }
    }
  }

  function wake(): void {
    // Invalidate before queueing so an in-flight response cannot win over the
    // newer wake even if it resolves before the queued request begins.
    generation++
    if (running) queued = true
    else void reload()
  }

  async function create(action: EditableAction): Promise<EditableAction | null> {
    try { const result = await CreateAction(action); await reload(); return result } catch (err) { error.value = message(err, 'Could not create action.'); return null }
  }
  async function update(id: string, action: EditableAction): Promise<EditableAction | null> {
    try { const result = await UpdateAction(id, action); await reload(); return result } catch (err) { error.value = message(err, 'Could not update action.'); return null }
  }
  async function remove(id: string): Promise<boolean> {
    try { await DeleteAction(id); await reload(); return true } catch (err) { error.value = message(err, 'Could not delete action.'); return false }
  }
  onMounted(() => { void reload(); unsubscribe = Events.On('actions:updated', wake) })
  onUnmounted(() => unsubscribe?.())
  return { actions, loading, error, reload, create, update, remove }
}
