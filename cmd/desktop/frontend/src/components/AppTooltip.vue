<script setup lang="ts">
// A hover tooltip, because `title` takes ~1-2s in WebKit with no knob to turn —
// too slow for a row whose glyphs are readable only through their tooltip.
//
// Teleported and fixed: callers sit inside containers that clip (the status
// bar's slot is `overflow-hidden`), which would cut an absolute bubble off.
// Not useAnchoredPopover — it sizes to the anchor's width with a 320px cap.
import { onBeforeUnmount, ref } from 'vue'

const props = withDefaults(defineProps<{
  /** Empty renders the trigger alone, with no tooltip at all. */
  text: string
  delay?: number
}>(), { delay: 300 })

const GAP = 6
const EDGE = 8

const anchor = ref<HTMLElement | null>(null)
const open = ref(false)
const bubble = ref<HTMLElement | null>(null)
const style = ref<Record<string, string>>({})
let timer: ReturnType<typeof setTimeout> | undefined

function place(): void {
  const el = anchor.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  // Measured off the rendered bubble, so show() runs this twice.
  const width = bubble.value?.getBoundingClientRect().width ?? 0
  const height = bubble.value?.getBoundingClientRect().height ?? 0
  const below = window.innerHeight - rect.bottom - GAP - EDGE
  const flip = height > 0 && below < height
  style.value = {
    left: `${Math.max(EDGE, Math.min(rect.left + rect.width / 2 - width / 2, window.innerWidth - EDGE - width))}px`,
    ...(flip ? { bottom: `${window.innerHeight - rect.top + GAP}px` } : { top: `${rect.bottom + GAP}px` }),
  }
}

function show(): void {
  if (!props.text) return
  open.value = true
  place()
  // The first place() had no bubble to measure; this one does.
  requestAnimationFrame(place)
  window.addEventListener('scroll', hide, true)
  window.addEventListener('resize', hide)
}

function schedule(): void {
  clearTimeout(timer)
  timer = setTimeout(show, props.delay)
}

// Focus skips the dwell: arriving by Tab is already deliberate.
function showNow(): void {
  clearTimeout(timer)
  show()
}

function hide(): void {
  clearTimeout(timer)
  open.value = false
  window.removeEventListener('scroll', hide, true)
  window.removeEventListener('resize', hide)
}

onBeforeUnmount(hide)
</script>

<template>
  <span
    ref="anchor"
    class="inline-flex"
    @pointerenter="schedule"
    @pointerleave="hide"
    @focusin="showNow"
    @focusout="hide"
  >
    <slot />
    <Teleport to="body">
      <div
        v-if="open"
        ref="bubble"
        class="app-tooltip"
        :style="style"
        role="tooltip"
        data-testid="app-tooltip"
      >{{ text }}</div>
    </Teleport>
  </span>
</template>

<style scoped>
/* pointer-events: none so the bubble can never sit between the cursor and the
   thing it describes, flickering it on and off. */
.app-tooltip {
  position: fixed;
  z-index: 60;
  max-width: 280px;
  pointer-events: none;
  border: 1px solid var(--color-strong);
  border-radius: 6px;
  background: var(--color-pane);
  padding: 4px 7px;
  color: var(--color-text-2);
  font-size: 11px;
  line-height: 1.35;
  /* pre-line so a caller can put a second line in while long text still wraps
     at max-width. */
  white-space: pre-line;
  box-shadow: 0 10px 28px -10px rgb(0 0 0 / .55);
}
</style>
