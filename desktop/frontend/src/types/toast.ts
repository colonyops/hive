// Toast stack types shared by useToasts (owns the queue) and the
// ToastStack/ToastCard components (render it). Toasts stack bottom-right,
// support optional inline actions, and auto-dismiss after a bounded delay.

export type ToastSeverity = 'info' | 'success' | 'warning' | 'error' | 'auto-action'

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
  /** Auto-dismiss delay in ms. */
  duration?: number
}

export interface ToastInstance {
  id: number
  message: string
  body?: string
  severity: ToastSeverity
  actions: ToastActionDef[]
  duration: number
}
