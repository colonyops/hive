import { onBeforeUnmount, shallowRef, toValue, type MaybeRefOrGetter } from 'vue'

const OPEN = '[aria-expanded="true"]'

/**
 * The hover and focus handlers of a tooltip trigger. Bind `triggers` with
 * `v-on` and render a `TooltipBubble` while `anchor` is set. Nothing but a
 * timer lives here until the bubble opens, so a trigger in every row of a
 * long list stays cheap.
 */
export function useTooltip(text: MaybeRefOrGetter<string>, delay: MaybeRefOrGetter<number> = 300) {
  const anchor = shallowRef<HTMLElement | null>(null)
  let timer: ReturnType<typeof setTimeout> | undefined

  function show(target: EventTarget | null): void {
    clearTimeout(timer)
    if (!(target instanceof HTMLElement) || !toValue(text)) return
    // The bubble sits where the trigger's own menu or popover opens.
    if (target.matches(OPEN) || target.querySelector(OPEN)) return
    anchor.value = target
  }

  function hide(): void {
    clearTimeout(timer)
    anchor.value = null
  }

  onBeforeUnmount(hide)

  return {
    anchor,
    hide,
    triggers: {
      pointerenter: (event: PointerEvent) => {
        const target = event.currentTarget
        clearTimeout(timer)
        timer = setTimeout(() => show(target), toValue(delay))
      },
      pointerleave: hide,
      // A click opens menus and popovers right where the bubble sits. `click`
      // also covers Enter and Space on a focused trigger.
      pointerdown: hide,
      click: hide,
      // Keyboard focus skips the dwell: arriving by Tab is already deliberate.
      // Focus that a closing dialog hands back after a click shows nothing.
      focusin: (event: FocusEvent) => {
        if (event.target instanceof Element && event.target.matches(':focus-visible')) show(event.currentTarget)
      },
      focusout: hide,
    },
  }
}
