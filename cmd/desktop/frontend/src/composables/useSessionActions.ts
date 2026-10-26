import { ref, type DeepReadonly } from 'vue'
import {
  DeleteSession,
  PruneSessions,
  EditSession,
  RecycleSession,
  SessionDetail,
  SessionRisk,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  SessionDetail as SessionDetailView,
  SessionSummary,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { SessionRisk as SessionRiskView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { appErrorMessage } from '../lib/appError'
import { useConfirmation, type ConfirmationDetail } from '@hive/ui/composables/useConfirmation'
import { useToasts } from './useToasts'

/** A session as the session list hands it out: deep-readonly. */
export type SessionRow = DeepReadonly<SessionSummary>

/** The field the edit dialog opens on. */
export type EditFocus = 'name' | 'tags'

export interface SessionActionOptions {
  /**
   * Called after a change the session list has to re-read. A rename is one of
   * them, and re-reading is the whole mechanism: the renamed session comes back
   * with a new slug, which is what the host follows to re-attach.
   */
  onChanged?: () => void
}

function message(error: unknown, fallback: string): string {
  return appErrorMessage(error) || (error instanceof Error && error.message ? error.message : fallback)
}

/**
 * Names what a destructive operation does to this session. Hazard 2 of hay-kot/hive-desktop#144: a
 * generic "are you sure" hides the only thing worth confirming, which is what
 * this session in particular is holding; that part is riskDetails.
 */
export function riskDescription(name: string, operation: 'delete' | 'recycle', risk: SessionRiskView): string {
  return operation === 'delete'
    ? `Deleting ${name} removes its directory and kills its terminal session.`
    : risk.recycleDeletes
      ? `Recycling ${name} deletes it — it is a git worktree, so there is no clone of its own to reset.`
      : `Recycling ${name} resets its clone to a clean checkout of the default branch.`
}

/**
 * The git pre-flight as a checklist, one line per check, so unsaved work reads
 * as a red mark rather than a clause buried in a sentence.
 */
export function riskDetails(risk: SessionRiskView): ConfirmationDetail[] {
  return [
    risk.uncommittedChanges
      ? { tone: 'danger', text: 'Uncommitted changes will be lost.' }
      : { tone: 'ok', text: 'No uncommitted changes.' },
    risk.unpushedCommits
      ? { tone: 'danger', text: 'Unpushed commits will be lost.' }
      : { tone: 'ok', text: 'No unpushed commits.' },
  ]
}

/**
 * The session row's operations: read, edit, recycle, delete, and the list-wide
 * prune. Destructive ones are gated on a risk pre-flight and run as background
 * jobs, so what they report is that the job started — `jobs:updated` is what
 * tells the list to re-read.
 */
export function useSessionActions(options: SessionActionOptions = {}) {
  const { showToast } = useToasts()
  const confirmation = useConfirmation()

  const detail = ref<SessionDetailView | null>(null)
  const editing = ref<SessionRow | null>(null)
  const editFocus = ref<EditFocus>('name')
  const editBusy = ref(false)
  const editError = ref<string | null>(null)

  async function openDetail(session: SessionRow): Promise<void> {
    try {
      detail.value = await SessionDetail(session.id)
    } catch (e) {
      showToast(message(e, 'Could not read that session.'), { severity: 'error' })
    }
  }

  function closeDetail(): void {
    detail.value = null
  }

  function requestEdit(session: SessionRow, focus: EditFocus = 'name'): void {
    editing.value = session
    editFocus.value = focus
    editError.value = null
  }

  function cancelEdit(): void {
    if (editBusy.value) return
    editing.value = null
    editError.value = null
  }

  // A refused edit keeps the dialog open with hive's reason: the name or a tag
  // is almost always nearly right, or the slug is taken by another session.
  async function submitEdit(name: string, tags: string[]): Promise<void> {
    const session = editing.value
    if (!session || editBusy.value) return
    editBusy.value = true
    editError.value = null
    try {
      await EditSession(session.id, name.trim(), tags)
      editing.value = null
      options.onChanged?.()
    } catch (e) {
      editError.value = message(e, 'Could not save that session.')
    } finally {
      editBusy.value = false
    }
  }

  async function requestDestructive(session: SessionRow, operation: 'delete' | 'recycle'): Promise<void> {
    let risk: SessionRiskView
    try {
      risk = await SessionRisk(session.id)
    } catch (e) {
      // Without the pre-flight there is nothing specific to confirm against,
      // and confirming against nothing is what hazard 2 forbids.
      showToast(message(e, 'Could not check that session for unsaved work.'), { severity: 'error' })
      return
    }
    confirmation.request({
      title: operation === 'delete' ? 'Delete this session?' : 'Recycle this session?',
      description: riskDescription(session.name, operation, risk),
      details: riskDetails(risk),
      confirmLabel: operation === 'delete' ? 'Delete' : 'Recycle',
      onConfirm: async () => {
        await (operation === 'delete' ? DeleteSession(session.id) : RecycleSession(session.id))
        showToast(operation === 'delete' ? `Deleting ${session.name}…` : `Recycling ${session.name}…`, {
          severity: 'info',
        })
      },
    })
  }

  function requestDelete(session: SessionRow): Promise<void> {
    return requestDestructive(session, 'delete')
  }

  function requestRecycle(session: SessionRow): Promise<void> {
    return requestDestructive(session, 'recycle')
  }

  /** count is the number of prunable (recycled or corrupted) sessions listed. */
  function requestPrune(count: number): void {
    confirmation.request({
      title: 'Prune sessions?',
      description:
        count === 1
          ? 'The one recycled or corrupted session is deleted, along with its directory.'
          : `All ${count} recycled and corrupted sessions are deleted, along with their directories.`,
      confirmLabel: 'Prune',
      onConfirm: async () => {
        await PruneSessions()
        showToast('Pruning sessions…', { severity: 'info' })
      },
    })
  }

  return {
    confirmation,
    detail,
    openDetail,
    closeDetail,
    editing,
    editFocus,
    editBusy,
    editError,
    requestEdit,
    cancelEdit,
    submitEdit,
    requestDelete,
    requestRecycle,
    requestPrune,
  }
}
