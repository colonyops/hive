import { ref } from 'vue'
import { errorText } from '../lib/appError'

/**
 * One line of the dialog's checklist: `danger` marks work the operation
 * destroys, `ok` marks a check that came back clean.
 */
export type ConfirmationDetail = {
  tone: 'danger' | 'ok'
  text: string
}

export type ConfirmationOptions = {
  title: string
  description: string
  details?: ConfirmationDetail[]
  confirmLabel?: string
  testid?: string
  confirmTestid?: string
  onConfirm: () => Promise<void> | void
}

export type Confirmation = ReturnType<typeof useConfirmation>

// Shared state machine for destructive dialogs, rendered by ConfirmationHost.
// Failed confirmation deliberately leaves the dialog open with its error so the
// user can retry or cancel.
export function useConfirmation() {
  const options = ref<ConfirmationOptions | null>(null)
  const busy = ref(false)
  const error = ref<string | null>(null)

  function request(next: ConfirmationOptions): void {
    if (busy.value) return
    options.value = next
    error.value = null
  }

  function cancel(): void {
    if (busy.value) return
    options.value = null
    error.value = null
  }

  async function confirm(): Promise<boolean> {
    if (!options.value || busy.value) return false
    busy.value = true
    error.value = null
    try {
      await options.value.onConfirm()
      options.value = null
      return true
    } catch (cause) {
      error.value = errorText(cause, 'Could not complete that action.')
      return false
    } finally {
      busy.value = false
    }
  }

  return { options, busy, error, request, cancel, confirm }
}
