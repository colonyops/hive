import { onScopeDispose, type Ref } from 'vue'
import { useEventListener, useTimeoutFn } from '@vueuse/core'
import { collectFrames, percentile, round, type FrameSummary } from '../lib/frameCollector'

const GESTURE_IDLE_MS = 150

export interface ScrollGestureAttrs extends FrameSummary {
  wheelEvents: number
  scrollEvents: number
  pixels: number
  scrollGapP95: number
  scrollGapWorst: number
}

interface Gesture {
  startedAt: number
  lastAt: number
  wheelEvents: number
  scrollGaps: number[]
  lastScrollAt: number
  lastTop: number
  pixels: number
  frames: () => FrameSummary
}

/**
 * Reports each wheel or trackpad gesture on a scroller. It starts on a wheel event
 * so that a jump from the keyboard or the scrollbar is not counted as one. WebKit
 * scrolls on its own thread, so the gaps between scroll events show when the main
 * thread fell behind even while the frames kept coming.
 */
export function useScrollGestureSpan(
  target: Ref<HTMLElement | null>,
  record: (durationMs: number, attrs: ScrollGestureAttrs) => void,
): void {
  let gesture: Gesture | null = null

  function close(): void {
    if (!gesture) return
    const done = gesture
    gesture = null
    const gaps = [...done.scrollGaps].sort((a, b) => a - b)
    record(done.lastAt - done.startedAt, {
      wheelEvents: done.wheelEvents,
      scrollEvents: gaps.length + (done.lastScrollAt ? 1 : 0),
      pixels: round(done.pixels),
      scrollGapP95: round(percentile(gaps, 95)),
      scrollGapWorst: round(gaps.at(-1) ?? 0),
      ...done.frames(),
    })
  }

  const idle = useTimeoutFn(close, GESTURE_IDLE_MS, { immediate: false })

  useEventListener(
    target,
    'wheel',
    () => {
      const now = performance.now()
      gesture ??= {
        startedAt: now,
        lastAt: now,
        wheelEvents: 0,
        scrollGaps: [],
        lastScrollAt: 0,
        lastTop: target.value?.scrollTop ?? 0,
        pixels: 0,
        frames: collectFrames(),
      }
      gesture.wheelEvents++
      gesture.lastAt = now
      idle.start()
    },
    { passive: true },
  )

  useEventListener(
    target,
    'scroll',
    () => {
      if (!gesture) return
      const now = performance.now()
      if (gesture.lastScrollAt) gesture.scrollGaps.push(now - gesture.lastScrollAt)
      gesture.lastScrollAt = now
      gesture.lastAt = now
      const top = target.value?.scrollTop ?? 0
      gesture.pixels += Math.abs(top - gesture.lastTop)
      gesture.lastTop = top
      idle.start()
    },
    { passive: true },
  )

  onScopeDispose(close)
}
