// Toast stack types shared by useToasts (owns the queue) and the
// ToastStack/ToastCard components (render it). See the design spec's "6a
// Toasts" section: four severities, stacked bottom-right, optional inline
// actions, auto-dismiss timer that error toasts opt out of.

export type ToastSeverity = 'info' | 'success' | 'error' | 'auto-action'

export interface ToastActionDef {
  label: string
  onClick: () => void
}

export interface ToastOptions {
  /** Defaults to 'info'. */
  severity?: ToastSeverity
  /** Secondary detail line under the title. */
  body?: string
  /** Up to two inline actions; the first renders as primary (bold, severity-colored), the rest as muted secondary links. */
  actions?: ToastActionDef[]
  /** Auto-dismiss delay in ms. Ignored for severity 'error', which never auto-dismisses. */
  duration?: number
}

export interface ToastInstance {
  id: number
  message: string
  body?: string
  severity: ToastSeverity
  actions: ToastActionDef[]
  /** null means "does not auto-dismiss" (always true for severity 'error'). */
  duration: number | null
}
