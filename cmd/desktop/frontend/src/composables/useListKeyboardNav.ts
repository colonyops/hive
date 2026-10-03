import { nextTick, watch, type Ref } from 'vue'

export interface ListKeyboardNavOptions {
  /** The highlighted row's index. */
  active: Ref<number>
  count: () => number
  /** Whether the list is on screen. Closed, nothing is clamped or scrolled. Defaults to always. */
  open?: () => boolean
  /** Run off either end and land on the other. Defaults to true; false stops at the ends. */
  wrap?: boolean
  /** Rows the walk skips. */
  disabled?: (index: number) => boolean
  /** Home and End jump to the ends. Leave it off where the keys move a text caret. */
  homeEnd?: () => boolean
  /** The row element to keep scrolled into view. */
  row?: (index: number) => Element | null | undefined
}

/**
 * The up/down walk of a highlighted row in a list: arrows step (wrapping by
 * default, skipping disabled rows), Home/End jump, the index stays in range as
 * the list shrinks, and the active row stays scrolled into view. Enter and the
 * other keys stay with the host, since what they do differs per list.
 */
export function useListKeyboardNav(options: ListKeyboardNavOptions): {
  step: (delta: number) => void
  jump: (edge: 'start' | 'end') => void
  /** Handles ArrowUp/ArrowDown (and Home/End when enabled); true when it took the key. */
  onKeydown: (event: KeyboardEvent) => boolean
} {
  const { active, count } = options
  const open = options.open ?? (() => true)
  const disabled = options.disabled ?? (() => false)

  watch(
    () => (open() ? count() : -1),
    (n) => {
      if (n >= 0 && active.value >= n) active.value = Math.max(0, n - 1)
    },
  )

  const row = options.row
  if (row) {
    watch(
      () => (open() ? active.value : -1),
      (index) => {
        if (index >= 0) void nextTick(() => row(index)?.scrollIntoView?.({ block: 'nearest' }))
      },
    )
  }

  function step(delta: number): void {
    const n = count()
    if (!n) return
    let index = active.value
    for (let tries = 0; tries < n; tries++) {
      index = options.wrap === false ? Math.min(n - 1, Math.max(0, index + delta)) : (index + delta + n) % n
      if (!disabled(index)) {
        active.value = index
        return
      }
    }
  }

  function jump(edge: 'start' | 'end'): void {
    const n = count()
    const delta = edge === 'start' ? 1 : -1
    for (let index = edge === 'start' ? 0 : n - 1; index >= 0 && index < n; index += delta) {
      if (!disabled(index)) {
        active.value = index
        return
      }
    }
  }

  function onKeydown(event: KeyboardEvent): boolean {
    const move = { ArrowDown: () => step(1), ArrowUp: () => step(-1) }[event.key]
    const edge = { Home: 'start', End: 'end' }[event.key] as 'start' | 'end' | undefined
    if (move) move()
    else if (edge && options.homeEnd?.()) jump(edge)
    else return false
    event.preventDefault()
    return true
  }

  return { step, jump, onKeydown }
}
