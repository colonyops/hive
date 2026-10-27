import { describe, expect, it, vi } from 'vitest'
import type { Terminal } from '@xterm/xterm'
import { TAIL_SLACK_ROWS, scrolledOffTail, watchTailPin } from '../terminalTail'

function pane() {
  const handlers: Record<'scroll' | 'bufferChange', (() => void) | undefined> = {
    scroll: undefined,
    bufferChange: undefined,
  }
  const disposeScroll = vi.fn()
  const disposeBufferChange = vi.fn()
  const term = {
    buffer: {
      active: { baseY: 0, viewportY: 0 },
      onBufferChange: (handler: () => void) => {
        handlers.bufferChange = handler
        return { dispose: disposeBufferChange }
      },
    },
    onScroll: (handler: () => void) => {
      handlers.scroll = handler
      return { dispose: disposeScroll }
    },
  }

  function place(baseY: number, viewportY: number): void {
    term.buffer.active.baseY = baseY
    term.buffer.active.viewportY = viewportY
  }

  return { term: term as unknown as Terminal, handlers, place, disposeScroll, disposeBufferChange }
}

describe('scrolledOffTail', () => {
  it('reads the tail as held until the viewport is past the slack', () => {
    const { term, place } = pane()

    place(100, 100 - TAIL_SLACK_ROWS)
    expect(scrolledOffTail(term)).toBe(false)

    place(100, 100 - TAIL_SLACK_ROWS - 1)
    expect(scrolledOffTail(term)).toBe(true)
  })
})

describe('watchTailPin', () => {
  it('reports every event that moves the viewport, and only on a change', () => {
    const { term, handlers, place } = pane()
    const reported: boolean[] = []
    const watch = watchTailPin(term, (scrolledUp) => reported.push(scrolledUp))

    place(100, 20)
    handlers.scroll?.()
    expect(reported).toEqual([true])

    // The same answer twice is not news — this runs once per rendered line.
    handlers.scroll?.()
    expect(reported).toEqual([true])

    place(100, 100)
    handlers.scroll?.()
    expect(reported).toEqual([true, false])

    place(100, 20)
    handlers.bufferChange?.()
    expect(reported).toEqual([true, false, true])

    watch.dispose()
  })

  it('stops on dispose', () => {
    const { term, disposeScroll, disposeBufferChange } = pane()
    const watch = watchTailPin(term, () => {})

    watch.dispose()
    expect(disposeScroll).toHaveBeenCalled()
    expect(disposeBufferChange).toHaveBeenCalled()
  })
})
