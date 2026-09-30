import { toValue, type MaybeRefOrGetter, type Ref } from 'vue'

const TABBABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/**
 * Confines Tab to `container`. Bind the returned handler to the overlay's own
 * keydown — content teleported out of it (an anchored popover) is deliberately
 * not covered, because the control that owns one hands focus back itself.
 */
export function useFocusTrap(
  container: Ref<HTMLElement | null>,
  options: { enabled?: MaybeRefOrGetter<boolean> } = {},
): { onKeydown: (event: KeyboardEvent) => void } {
  function onKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Tab' || !toValue(options.enabled ?? true)) return
    const stops = Array.from(container.value?.querySelectorAll<HTMLElement>(TABBABLE) ?? [])
    if (!stops.length) return
    const last = stops.length - 1
    const index = stops.indexOf(document.activeElement as HTMLElement)
    // index -1 means focus is outside the overlay — a popover that just closed
    // without reclaiming it. Tab pulls it back to the edge it would wrap to.
    const next = event.shiftKey
      ? (index <= 0 ? last : index - 1)
      : (index === -1 || index === last ? 0 : index + 1)
    event.preventDefault()
    stops[next].focus()
  }

  return { onKeydown }
}
