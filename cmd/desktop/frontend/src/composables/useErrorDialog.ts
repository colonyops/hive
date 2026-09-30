import { ref } from 'vue'

/**
 * A failure the user has to see and act on. `detail` is shown verbatim and is
 * what Copy writes and a diagnostic report carries, so it must never be
 * trimmed to fit the dialog.
 */
export interface ErrorDetails {
  /** Names the operation that failed, e.g. "Deploy failed". */
  title: string
  /** What the failure means for the user's work, in one line. */
  summary?: string
  /** The error text, verbatim. */
  detail: string
  /** Identifiers worth carrying into a report — a flow id, a session name. */
  context?: Record<string, string>
}

// Module-scoped so a surface can raise a failure without knowing where the
// dialog is mounted. App.vue mounts the single ErrorDialog that renders it.
const current = ref<ErrorDetails | null>(null)

export function useErrorDialog() {
  return {
    current,
    showError: (details: ErrorDetails) => { current.value = details },
    dismissError: () => { current.value = null },
  }
}

/** The dialog as one block of text — what Copy writes and a report carries. */
export function errorDetailsText(details: ErrorDetails): string {
  const parts = [details.title]
  if (details.summary) parts.push(details.summary)
  const context = Object.entries(details.context ?? {})
  if (context.length) parts.push(context.map(([key, value]) => `${key}: ${value}`).join('\n'))
  if (details.detail.trim()) parts.push(details.detail.trim())
  return parts.join('\n\n')
}

export function resetErrorDialogForTests(): void {
  current.value = null
}
