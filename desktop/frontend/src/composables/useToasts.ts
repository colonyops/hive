import { ref } from 'vue'
import type { ToastInstance, ToastOptions } from '../types/toast'

// Toasts are app-lifetime UI state. Consumers may mount and unmount without
// dismissing feedback that is still visible in the shared application shell.
const toasts = ref<ToastInstance[]>([])
const toastTimers = new Map<number, ReturnType<typeof setTimeout>>()
let nextToastId = 1
const defaultToastDuration = 4000

function showToast(message: string, options: ToastOptions = {}): number {
  const severity = options.severity ?? 'info'
  const id = nextToastId++
  const duration = severity === 'error' ? null : options.duration ?? defaultToastDuration
  toasts.value = [...toasts.value, { id, message, body: options.body, severity, actions: options.actions ?? [], duration }]
  if (duration !== null) toastTimers.set(id, setTimeout(() => dismissToast(id), duration))
  return id
}

function dismissToast(id: number): void {
  const timer = toastTimers.get(id)
  if (timer !== undefined) {
    clearTimeout(timer)
    toastTimers.delete(id)
  }
  toasts.value = toasts.value.filter((toast) => toast.id !== id)
}

function clearToasts(): void {
  for (const timer of toastTimers.values()) clearTimeout(timer)
  toastTimers.clear()
  toasts.value = []
}

export function useToasts() {
  return { toasts, showToast, dismissToast, clearToasts }
}

export function resetToastsForTests(): void {
  clearToasts()
  nextToastId = 1
}
