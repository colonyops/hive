import { ref } from 'vue'

export type ConfirmationOptions = {
  title: string
  description: string
  confirmLabel?: string
  onConfirm: () => Promise<void> | void
}

function message(error: unknown): string {
  return error instanceof Error && error.message ? error.message : 'Could not complete that action.'
}

// Shared state machine for destructive dialogs. Failed confirmation deliberately
// leaves the dialog open with its error so the user can retry or cancel.
export function useConfirmation() {
  const open = ref(false)
  const options = ref<ConfirmationOptions | null>(null)
  const busy = ref(false)
  const error = ref<string | null>(null)

  function request(next: ConfirmationOptions): void {
    options.value = next
    error.value = null
    open.value = true
  }

  function cancel(): void {
    if (busy.value) return
    open.value = false
    options.value = null
    error.value = null
  }

  async function confirm(): Promise<boolean> {
    if (!options.value || busy.value) return false
    busy.value = true
    error.value = null
    try {
      await options.value.onConfirm()
      open.value = false
      options.value = null
      error.value = null
      return true
    } catch (cause) {
      error.value = message(cause)
      return false
    } finally {
      busy.value = false
    }
  }

  return { open, options, busy, error, request, cancel, confirm }
}
