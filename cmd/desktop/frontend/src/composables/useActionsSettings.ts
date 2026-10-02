import { onMounted, ref } from 'vue'
import {
  CreateAction,
  CreateLauncher,
  DeleteAction,
  DeleteLauncher,
  ListActions,
  ReorderActions,
  UpdateAction,
  UpdateLauncher,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/actionsservice'
import type {
  EditableAction,
  Launcher,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/actions/models'
import { useWailsEvent } from './useWailsEvent'

export type { EditableAction, Launcher }
export type ActionType = EditableAction['type']

function message(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

// Wails mutation notifications and fsnotify can both report the same write.
// One queued read is enough; the generation prevents an older Promise from
// replacing a catalog requested after it began.
//
// Launchers ride the same read and the same wake: they are the other list in
// actions.yml, so a second composable would mean a second reload race over one
// file for no gain.
export function useActionsSettings() {
  const actions = ref<EditableAction[]>([])
  const launchers = ref<Launcher[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  let generation = 0
  let queued = false
  let running = false

  async function reload(): Promise<void> {
    if (running) {
      generation++
      queued = true
      return
    }
    running = true
    const token = ++generation
    loading.value = true
    try {
      const catalog = await ListActions()
      if (token === generation) {
        actions.value = catalog?.actions ?? []
        launchers.value = catalog?.launchers ?? []
        error.value = catalog?.error || null
      }
    } catch (err) {
      if (token === generation) error.value = message(err, 'Could not load actions.')
    } finally {
      if (token === generation) loading.value = false
      running = false
      if (queued) {
        queued = false
        void reload()
      }
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
    try {
      const result = await CreateAction(action)
      await reload()
      return result
    } catch (err) {
      error.value = message(err, 'Could not create action.')
      return null
    }
  }
  async function update(id: string, action: EditableAction): Promise<EditableAction | null> {
    try {
      const result = await UpdateAction(id, action)
      await reload()
      return result
    } catch (err) {
      error.value = message(err, 'Could not update action.')
      return null
    }
  }
  async function remove(id: string): Promise<boolean> {
    try {
      await DeleteAction(id)
      await reload()
      return true
    } catch (err) {
      error.value = message(err, 'Could not delete action.')
      return false
    }
  }
  // The catalog order is the file's order, so a drop shows its result
  // immediately and the write confirms it. A rejected order (the catalog
  // changed underneath the drag) restores what was on screen and re-reads.
  async function reorder(ids: string[]): Promise<boolean> {
    const previous = actions.value
    const byId = new Map(previous.map((action) => [action.id, action]))
    const next = ids.map((id) => byId.get(id)).filter((action): action is EditableAction => !!action)
    if (next.length !== previous.length) {
      await reload()
      return false
    }
    actions.value = next
    try {
      await ReorderActions(ids)
      await reload()
      return true
    } catch (err) {
      // Put the list back, then re-read: a rejected order means the catalog
      // moved underneath the drag. The message is set after the reload because
      // the reload rewrites error with the catalog's own parse state.
      actions.value = previous
      await reload()
      error.value = message(err, 'Could not reorder actions.')
      return false
    }
  }
  async function createLauncher(launcher: Launcher): Promise<Launcher | null> {
    try {
      const result = await CreateLauncher(launcher)
      await reload()
      return result
    } catch (err) {
      error.value = message(err, 'Could not create launcher.')
      return null
    }
  }
  async function updateLauncher(id: string, launcher: Launcher): Promise<Launcher | null> {
    try {
      const result = await UpdateLauncher(id, launcher)
      await reload()
      return result
    } catch (err) {
      error.value = message(err, 'Could not update launcher.')
      return null
    }
  }
  async function removeLauncher(id: string): Promise<boolean> {
    try {
      await DeleteLauncher(id)
      await reload()
      return true
    } catch (err) {
      error.value = message(err, 'Could not delete launcher.')
      return false
    }
  }

  onMounted(() => {
    void reload()
    useWailsEvent('actions:updated', wake)
  })
  return {
    actions,
    launchers,
    loading,
    error,
    reload,
    create,
    update,
    remove,
    reorder,
    createLauncher,
    updateLauncher,
    removeLauncher,
  }
}
