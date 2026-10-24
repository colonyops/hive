import { computed, ref } from 'vue'
import {
  InvokeTerminalAction,
  RenderTerminalClipboardAction,
  TerminalActionViews,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { TerminalTarget } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { appErrorMessage } from '../lib/appError'
import { actionTypeMeta } from '../lib/actionPresentation'
import type { ActionView } from '../types/action'
import type { MenuEntry } from '@hive/ui/types/menu'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useToasts } from './useToasts'

/** The terminal surfaces an action can declare in `targets:`. */
export type TerminalSurface = 'session' | 'window' | 'statusbar'

/** Prefix that keeps action entries apart from a menu's own ids. */
export const TERMINAL_ACTION_PREFIX = 'terminal-action:'

function message(error: unknown, fallback: string): string {
  return appErrorMessage(error) || (error instanceof Error && error.message ? error.message : fallback)
}

/**
 * The configured actions the terminal offers on its session and window rows,
 * and the one path that runs them.
 *
 * The two sets are read once per mount and again on `actions:updated`, because
 * what an action targets is catalog configuration rather than anything about a
 * particular session — the same list serves every row.
 *
 * A clipboard action is copied rather than run, mirroring the detail pane: it
 * renders text, so there is no job to watch and no rerun to confirm. Everything
 * else starts a background job, and the jobs UI is where its outcome lands.
 */
export function useTerminalActions() {
  const { showToast } = useToasts()
  const clipboard = useClipboard()

  const views = ref<Record<TerminalSurface, ActionView[]>>({ session: [], window: [], statusbar: [] })
  /** The action awaiting its declared inputs, with the target it will run against. */
  const pendingInputs = ref<{ action: ActionView; target: TerminalTarget } | null>(null)
  const inputsBusy = ref(false)
  const inputsError = ref<string | null>(null)

  async function load(): Promise<void> {
    const surfaces: TerminalSurface[] = ['session', 'window', 'statusbar']
    const loaded = await Promise.all(surfaces.map((surface) => TerminalActionViews(surface).catch(() => null)))
    // A catalog read failing leaves the last-known entries in place: the menus
    // they feed have their own operations to offer either way.
    surfaces.forEach((surface, index) => {
      const list = loaded[index]
      if (list) views.value[surface] = list
    })
  }

  function entriesFor(surface: TerminalSurface): MenuEntry[] {
    return views.value[surface].map((action) => {
      const meta = actionTypeMeta(action.type)
      return {
        kind: 'action',
        id: TERMINAL_ACTION_PREFIX + action.id,
        label: action.label,
        iconName: meta.icon,
        iconColor: meta.color,
        testid: `terminal-action-${action.id}`,
      }
    })
  }

  const sessionEntries = computed(() => entriesFor('session'))
  const windowEntries = computed(() => entriesFor('window'))

  function actionFor(surface: TerminalSurface, id: string): ActionView | undefined {
    return views.value[surface].find((action) => action.id === id)
  }

  async function run(
    action: ActionView,
    target: TerminalTarget,
    inputs: Record<string, string> = {},
  ): Promise<boolean> {
    try {
      if (action.type === 'clipboard') {
        const text = await RenderTerminalClipboardAction(action.id, target, inputs)
        await clipboard.copy(text)
        if (clipboard.status.value === 'error') {
          showToast('Could not copy to the clipboard', { severity: 'error' })
          return false
        }
        showToast(`${action.label} copied`, { severity: 'success' })
        return true
      }
      await InvokeTerminalAction(action.id, target, inputs)
      showToast(`${action.label} started`, { severity: 'info' })
      return true
    } catch (error) {
      const reason = message(error, `Could not run ${action.label}.`)
      if (pendingInputs.value) inputsError.value = reason
      else showToast(reason, { severity: 'error' })
      return false
    }
  }

  /**
   * Runs the entry a row menu emitted. An action that declares inputs opens the
   * form first; everything else runs on the click.
   */
  async function select(surface: TerminalSurface, entryID: string, target: TerminalTarget): Promise<void> {
    if (!entryID.startsWith(TERMINAL_ACTION_PREFIX)) return
    const action = actionFor(surface, entryID.slice(TERMINAL_ACTION_PREFIX.length))
    if (action) await invoke(action, target)
  }

  /** Runs one action, opening the inputs form first when it declares any. */
  async function invoke(action: ActionView, target: TerminalTarget): Promise<void> {
    if (action.inputs?.length) {
      inputsError.value = null
      pendingInputs.value = { action, target }
      return
    }
    await run(action, target)
  }

  function cancelInputs(): void {
    if (inputsBusy.value) return
    pendingInputs.value = null
    inputsError.value = null
  }

  async function submitInputs(values: Record<string, string>): Promise<void> {
    const pending = pendingInputs.value
    if (!pending || inputsBusy.value) return
    inputsBusy.value = true
    inputsError.value = null
    const succeeded = await run(pending.action, pending.target, values)
    inputsBusy.value = false
    if (succeeded) pendingInputs.value = null
  }

  return {
    load,
    sessionEntries,
    windowEntries,
    statusBarActions: computed(() => views.value.statusbar),
    hasWindowActions: computed(() => views.value.window.length > 0),
    select,
    invoke,
    pendingInputs,
    inputsBusy,
    inputsError,
    cancelInputs,
    submitInputs,
  }
}
