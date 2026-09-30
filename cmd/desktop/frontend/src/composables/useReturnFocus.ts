import { nextTick, onMounted, onUnmounted } from 'vue'

/**
 * Hands focus back to whatever held it when the overlay opened. Without it an
 * overlay that unmounts drops focus on `<body>` and the next Tab restarts at
 * the top of the app.
 */
export function useReturnFocus(preferred?: () => HTMLElement | null | undefined): void {
  let trigger: HTMLElement | null = null

  onMounted(() => {
    trigger = preferred?.() ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
  })

  onUnmounted(() => {
    // The view behind the overlay re-renders as part of this teardown, so the
    // trigger may not be back in the document until the next tick.
    void nextTick(() => { if (trigger?.isConnected) trigger.focus() })
  })
}
