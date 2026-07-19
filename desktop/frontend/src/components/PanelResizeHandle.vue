<script setup lang="ts">
// A thin draggable divider for useResizablePanel.ts — absolute-positioned on
// whichever edge of its (position: relative) parent panel it's given, so the
// same component works for both left- and right-docked panels. Pointerdown
// hands off to the composable's startResize; Arrow Left/Right nudge the
// width via the composable's step(), for keyboard-only resizing.
const STEP_PX = 12

const props = defineProps<{
  /** Which edge of the parent panel this handle sits on. */
  edge: 'left' | 'right'
  /** Panel name — becomes the data-testid suffix ("resize-handle-<name>") and the aria-label. */
  name: string
  /** The composable's startResize, wired straight to @pointerdown. */
  start: (event: PointerEvent) => void
  /** The composable's step, called with ±STEP_PX on Arrow Left/Right. */
  step: (deltaPx: number) => void
}>()

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'ArrowLeft') {
    e.preventDefault()
    props.step(-STEP_PX)
  } else if (e.key === 'ArrowRight') {
    e.preventDefault()
    props.step(STEP_PX)
  }
}
</script>

<template>
  <div
    class="panel-resize-handle"
    :class="edge === 'left' ? 'panel-resize-handle-left' : 'panel-resize-handle-right'"
    role="separator"
    aria-orientation="vertical"
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
  top: 0;
  bottom: 0;
  z-index: 10;
  width: 6px;
  cursor: col-resize;
  touch-action: none;
  background: transparent;
  transition: background-color 120ms ease;
}

.panel-resize-handle-left { left: -3px; }
.panel-resize-handle-right { right: -3px; }

.panel-resize-handle:hover,
.panel-resize-handle:active {
  background: color-mix(in srgb, var(--color-accent) 45%, transparent);
}

.panel-resize-handle:focus-visible {
  outline: none;
  background: color-mix(in srgb, var(--color-accent) 65%, transparent);
}
</style>
