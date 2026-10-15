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
}

.panel-resize-handle::after {
  position: absolute;
  content: '';
  border-radius: 999px;
  background: var(--color-text-4);
  opacity: 0.65;
  transition:
    background-color 120ms ease,
    opacity 120ms ease,
    transform 120ms ease;
}

/* Left/right handles: a full-height vertical bar dragged horizontally. */
.panel-resize-handle-left,
.panel-resize-handle-right {
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
}
.panel-resize-handle-left {
  left: -3px;
}
.panel-resize-handle-right {
  right: -3px;
}
.panel-resize-handle-left::after,
.panel-resize-handle-right::after {
  top: 50%;
  left: 50%;
  width: 3px;
  height: 36px;
  transform: translate(-50%, -50%);
}

/* Top/bottom handles: a full-width horizontal bar dragged vertically. */
.panel-resize-handle-top,
.panel-resize-handle-bottom {
  left: 0;
  right: 0;
  height: 6px;
  cursor: row-resize;
}
.panel-resize-handle-top {
  top: -3px;
}
.panel-resize-handle-bottom {
  bottom: -3px;
}
.panel-resize-handle-top::after,
.panel-resize-handle-bottom::after {
  top: 50%;
  left: 50%;
  width: 36px;
  height: 3px;
  transform: translate(-50%, -50%);
}

.panel-resize-handle:hover::after,
.panel-resize-handle:active::after,
.panel-resize-handle:focus-visible::after {
  background: var(--color-accent);
  opacity: 1;
}

.panel-resize-handle-left:hover::after,
.panel-resize-handle-left:active::after,
.panel-resize-handle-left:focus-visible::after,
.panel-resize-handle-right:hover::after,
.panel-resize-handle-right:active::after,
.panel-resize-handle-right:focus-visible::after {
  transform: translate(-50%, -50%) scaleX(1.35);
}

.panel-resize-handle-top:hover::after,
.panel-resize-handle-top:active::after,
.panel-resize-handle-top:focus-visible::after,
.panel-resize-handle-bottom:hover::after,
.panel-resize-handle-bottom:active::after,
.panel-resize-handle-bottom:focus-visible::after {
  transform: translate(-50%, -50%) scaleY(1.35);
}

.panel-resize-handle:focus-visible {
  outline: none;
}
</style>
