<script setup lang="ts">
import type { SelectionRail } from '../../composables/useSelectionRail'

defineProps<{ rail: SelectionRail }>()
</script>

<template>
  <span
    class="tree-rail"
    :class="{ 'tree-rail-shown': rail.shown }"
    :style="{ transform: `translateY(${rail.y}px)`, height: `${rail.height}px` }"
    :data-shown="rail.shown"
    aria-hidden="true"
  />
</template>

<style scoped>
/* Out of flow, so moving it costs no layout anywhere else. Square ends: two
   rails on adjacent rows read as one continuous mark instead of pinching at the
   seam. The z-index is load-bearing: rows and panels are positioned boxes too,
   so without it a rail is painted over by whichever comes after it. */
.tree-rail {
  position: absolute;
  left: 0;
  top: 0;
  z-index: 1;
  width: 3px;
  background: var(--color-accent);
  opacity: 0;
  pointer-events: none;
  transition:
    transform 0.2s cubic-bezier(0.2, 0, 0, 1),
    height 0.2s cubic-bezier(0.2, 0, 0, 1),
    opacity 0.12s ease;
}
.tree-rail-shown {
  opacity: 1;
}
@media (prefers-reduced-motion: reduce) {
  .tree-rail {
    transition: none;
  }
}
</style>
