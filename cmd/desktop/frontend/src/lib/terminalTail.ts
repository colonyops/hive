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
 * says how to stop.
 *
 * `host` is the element xterm opened into. The user's own scrolling reaches
 * this only as a DOM scroll on `.xterm-viewport`: xterm's viewport reads its
 * own scrollTop, syncs the buffer, and suppresses onScroll so it cannot feed
 * itself, so a wheel, a trackpad or a dragged scrollbar leaves the tail
 * silently. onScroll covers the other half — what output does to the buffer,
 * the auto-pin to the tail and a trim moving it — and onBufferChange covers
 * the alternate screen, which has no scrollback and fires no scroll event on
 * the way in.
 *
 * Call it after `term.open(host)`: `.xterm-viewport` does not exist before,
 * and xterm registers its own listener inside it, so this one runs second and
 * reads a buffer already synced.
 */
export function watchTailPin(
  term: Terminal,
  host: HTMLElement,
  report: (scrolledUp: boolean) => void,
): IDisposable {
  let last: boolean | undefined
  const refresh = (): void => {
    const scrolledUp = scrolledOffTail(term)
    if (scrolledUp === last) return
    last = scrolledUp
    report(scrolledUp)
  }

  const handles = [term.onScroll(refresh), term.buffer.onBufferChange(refresh)]
  const viewport = host.querySelector('.xterm-viewport')
  viewport?.addEventListener('scroll', refresh, { passive: true })

  return {
    dispose: () => {
      for (const handle of handles) handle.dispose()
      viewport?.removeEventListener('scroll', refresh)
    },
  }
}
