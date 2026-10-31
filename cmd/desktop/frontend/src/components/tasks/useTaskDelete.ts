import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { useTasks } from '../../stores/useTasks'

export function useTaskDelete() {
  const { remove } = useTasks()
  const confirmation = useConfirmation()

  function request(id: string): void {
    confirmation.request({
      title: 'Delete task',
      description: 'Deletes this item and everything nested under it, comments included. This cannot be undone.',
      confirmLabel: 'Delete',
      testid: 'task-delete-confirm',
      onConfirm: () => remove(id),
    })
  }

  return { confirmation, request }
}
