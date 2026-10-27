import type { IDisposable, Terminal } from '@xterm/xterm'

/**
 * How far off the live tail the viewport has to be before the way back is
 * offered. One wheel notch is about three rows, so a nudge — or the row of
 * drift a trackpad leaves behind — does not flash a pill at anyone; a scroll
 * meant as a scroll does.
 */
export const TAIL_SLACK_ROWS = 5

/** Whether `term`'s viewport has left the live tail by more than the slack. */
export function scrolledOffTail(term: Terminal): boolean {
  const buffer = term.buffer.active
  return buffer.baseY - buffer.viewportY > TAIL_SLACK_ROWS
}

/**
 * Reports `term`'s tail pin to `report` on every event that can move it, and
 * says how to stop. onScroll covers the user's own scrolling and what output
 * does to the buffer; onBufferChange covers the alternate screen, which has no
 * scrollback and fires no scroll event on the way in.
 */
export function watchTailPin(term: Terminal, report: (scrolledUp: boolean) => void): IDisposable {
  let last: boolean | undefined
  const refresh = (): void => {
    const scrolledUp = scrolledOffTail(term)
    if (scrolledUp === last) return
    last = scrolledUp
    report(scrolledUp)
  }

  const handles = [term.onScroll(refresh), term.buffer.onBufferChange(refresh)]
  return {
    dispose: () => {
      for (const handle of handles) handle.dispose()
    },
  }
}
