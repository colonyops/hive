<script setup lang="ts">
// The layer over a rendered markdown body that paints comment highlights,
// the text a draft is about, and other readers' selections.
import { ref } from 'vue'
import type { HighlightBox, ReviewHighlight } from '../../lib/reviewSelection'

defineProps<{
  highlights: readonly ReviewHighlight[]
  /** The draft's text, kept painted while its composer has focus. */
  pending?: readonly HighlightBox[]
  /** Another reader's selection: the CSS color of its bar, by highlight id. Painted as a bar, not a comment. */
  tints?: ReadonlyMap<string, string>
  hovered?: string | null
  active?: string | null
  /** Derives `-highlight-<id>` and `-highlight-pending`. */
  testid: string
}>()

const el = ref<HTMLElement | null>(null)
defineExpose({ el })

function boxStyle(box: HighlightBox) {
  return { top: `${box.top}px`, left: `${box.left}px`, width: `${box.width}px`, height: `${box.height}px` }
}
</script>

<template>
  <div ref="el" class="pointer-events-none absolute inset-0 z-10" aria-hidden="true">
    <template v-for="highlight in highlights" :key="highlight.id">
      <template v-if="tints?.has(highlight.id)">
        <div
          v-for="(box, index) in highlight.boxes"
          :key="index"
          class="review-viewer"
          :style="[boxStyle(box), { '--viewer-color': tints.get(highlight.id) }]"
          :data-testid="`${testid}-highlight-${highlight.id}`"
        />
      </template>
      <template v-else>
        <div
          v-for="(box, index) in highlight.boxes"
          :key="index"
          class="review-highlight"
          :class="{ 'is-hovered': hovered === highlight.id, 'is-active': active === highlight.id }"
          :style="boxStyle(box)"
          :data-review-anchor="index === 0 ? highlight.id : undefined"
          :data-testid="`${testid}-highlight-${highlight.id}`"
        />
      </template>
    </template>
    <div
      v-for="(box, index) in pending"
      :key="`pending-${index}`"
      class="review-highlight is-pending"
      :style="boxStyle(box)"
      :data-testid="`${testid}-highlight-pending`"
    />
  </div>
</template>

<style scoped>
/* The highlight sits over the text rather than behind it because inline code
   and other chips paint opaque backgrounds that would cut holes in it, so its
   tint stays light enough to read through. */
.review-highlight {
  position: absolute;
  margin-left: -0.125rem;
  padding: 0 0.125rem;
  box-sizing: content-box;
  border-radius: 2px;
  background: color-mix(in oklab, var(--color-accent) 16%, transparent);
  transition:
    background-color 120ms ease,
    box-shadow 120ms ease;
}
.review-highlight.is-hovered {
  background: color-mix(in oklab, var(--color-accent) 24%, transparent);
}
.review-highlight.is-active {
  background: color-mix(in oklab, var(--color-accent) 28%, transparent);
  box-shadow: 0 0 0 1px color-mix(in oklab, var(--color-accent) 55%, transparent);
}
.review-highlight.is-pending {
  background: color-mix(in oklab, var(--color-accent) 28%, transparent);
}
.review-viewer {
  position: absolute;
  border-left: 3px solid var(--viewer-color);
  background: color-mix(in oklab, var(--viewer-color) 10%, transparent);
}
</style>
