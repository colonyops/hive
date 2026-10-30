import { ref, watch, type Ref } from 'vue'

// Past the frames of the reveal, and short enough that a list refreshed behind
// it still reads as fresh.
const SETTLE_MS = 50
// A covered webview fires no animation frames, and a refresh should not wait for
// the window to come back.
const FRAME_FALLBACK_MS = 200

/**
 * Follows a hidden-not-unmounted mode's `active`, but turns true only once the
 * mode has painted, and is what background revalidation should watch. Binding
 * calls started in the tick of a reveal held its first frame for over 100ms in
 * WKWebView while the page's own thread sat idle, and the same calls cost nothing
 * visible once the frame is up. The first answer is immediate, because a first
 * load has nothing on screen to protect, unless `preloaded` says this mount
 * already has data to show, which makes it a reveal like any other.
 */
export function usePaintedActive(active: () => boolean, options: { preloaded?: boolean } = {}): Ref<boolean> {
  const painted = ref(active() && !options.preloaded)
  let run = 0
  watch(
    active,
    (shown) => {
      const current = ++run
      painted.value = false
      if (!shown) return
      const settle = (): void => {
        setTimeout(() => {
          if (current === run) painted.value = true
        }, SETTLE_MS)
      }
      let frames = 0
      const fallback = setTimeout(settle, FRAME_FALLBACK_MS)
      const onFrame = (): void => {
        if (current !== run) {
          clearTimeout(fallback)
          return
        }
        if (++frames < 2) {
          requestAnimationFrame(onFrame)
          return
        }
        clearTimeout(fallback)
        settle()
      }
      requestAnimationFrame(onFrame)
    },
    { immediate: !!options.preloaded && active() },
  )
  return painted
}
