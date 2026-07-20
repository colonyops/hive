<script setup lang="ts">
// A thin draggable divider for useResizablePanel.ts — absolute-positioned on
// whichever edge of its (position: relative) parent panel it's given, so the
// same component works for panels docked on any side. Pointerdown hands off to
// the composable's startResize; the arrow keys nudge the size via the
// composable's step() for keyboard-only resizing — Left/Right for a
// horizontal (left/right) handle, Up/Down for a vertical (top/bottom) one.
import { computed } from 'vue'

const STEP_PX = 12

const props = defineProps<{
  /** Which edge of the parent panel this handle sits on. */
  edge: 'left' | 'right' | 'top' | 'bottom'
  /** Panel name — becomes the data-testid suffix ("resize-handle-<name>") and the aria-label. */
  name: string
  /** The composable's startResize, wired straight to @pointerdown. */
  start: (event: PointerEvent) => void
  /** The composable's step, called with ±STEP_PX on the arrow keys. */
  step: (deltaPx: number) => void
}>()

// A top/bottom handle resizes height, so it's a horizontal bar the user drags
// vertically; a left/right handle is a vertical bar dragged horizontally.
const vertical = computed(() => props.edge === 'top' || props.edge === 'bottom')

function onKeydown(e: KeyboardEvent): void {
  const grow = vertical.value ? 'ArrowDown' : 'ArrowRight'
  const shrink = vertical.value ? 'ArrowUp' : 'ArrowLeft'
  if (e.key === shrink) {
    e.preventDefault()
    props.step(-STEP_PX)
  } else if (e.key === grow) {
    e.preventDefault()
    props.step(STEP_PX)
  }
}
</script>

<template>
  <div
    class="panel-resize-handle"
    :class="`panel-resize-handle-${edge}`"
    role="separator"
    :aria-orientation="vertical ? 'horizontal' : 'vertical'"
    :aria-label="`Resize ${name} panel`"
    tabindex="0"
    :data-testid="`resize-handle-${name}`"
    @pointerdown="start"
    @keydown="onKeydown"
  />
</template>

<style scoped>
.panel-resize-handle {
  position: absolute;
  z-index: 10;
  touch-action: none;
  background: transparent;
  transition: background-color 120ms ease;
}

/* Left/right handles: a full-height vertical bar dragged horizontally. */
.panel-resize-handle-left,
.panel-resize-handle-right {
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
}
.panel-resize-handle-left { left: -3px; }
.panel-resize-handle-right { right: -3px; }

/* Top/bottom handles: a full-width horizontal bar dragged vertically. */
.panel-resize-handle-top,
.panel-resize-handle-bottom {
  left: 0;
  right: 0;
  height: 6px;
  cursor: row-resize;
}
.panel-resize-handle-top { top: -3px; }
.panel-resize-handle-bottom { bottom: -3px; }

.panel-resize-handle:hover,
.panel-resize-handle:active {
  background: color-mix(in srgb, var(--color-accent) 45%, transparent);
}

.panel-resize-handle:focus-visible {
  outline: none;
  background: color-mix(in srgb, var(--color-accent) 65%, transparent);
}
</style>
