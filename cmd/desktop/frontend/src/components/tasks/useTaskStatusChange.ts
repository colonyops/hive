import { ref } from 'vue'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { useTasks } from '../../stores/useTasks'
import { errorText } from '../../lib/appError'
import { cascadeCount, statusMeta } from '../../lib/tasksPresentation'

interface StatusTarget {
  id: string
  type: string
}

/**
 * One status change with its confirms, for every surface that moves a task:
 * cancelling always confirms, and an epic moving to done or cancelled with
 * open descendants confirms with the cascade count instead (that copy already
 * implies the cancellation). A change that needs no confirm reports its
 * failure in `error`; a confirmed one keeps the dialog open with it.
 */
export function useTaskStatusChange() {
  const { items, setStatus } = useTasks()
  const confirmation = useConfirmation()
  const error = ref<string | null>(null)

  function request(target: StatusTarget, next: string): void {
    error.value = null
    const onConfirm = () => setStatus(target.id, next)
    const cascade =
      target.type === 'epic' && (next === 'done' || next === 'cancelled') ? cascadeCount(items.value, target.id) : 0
    if (cascade > 0) {
      confirmation.request({
        title: 'Close nested tasks?',
        description: `Marking this epic ${statusMeta(next).label.toLowerCase()} also closes ${cascade} open task${cascade === 1 ? '' : 's'} nested under it.`,
        testid: 'task-cascade-confirm',
        onConfirm,
      })
    } else if (next === 'cancelled') {
      confirmation.request({
        title: 'Cancel this task?',
        description: 'This marks the task cancelled. It stays in the tree but drops out of the open filters.',
        confirmLabel: 'Cancel task',
        testid: 'task-cancel-confirm',
        onConfirm,
      })
    } else {
      setStatus(target.id, next).catch((err: unknown) => {
        error.value = errorText(err, 'Could not update status.')
      })
    }
  }

  function reset(): void {
    error.value = null
    confirmation.cancel()
  }

  return { confirmation, error, request, reset }
}
