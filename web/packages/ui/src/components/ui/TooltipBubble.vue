<script setup lang="ts">
// Teleported and fixed: triggers sit inside containers that clip (the status
// bar's slot is `overflow-hidden`), which would cut an absolute bubble off.
// Not useAnchoredPopover -- it sizes to the anchor's width with a 320px cap.
import { useEventListener } from '@vueuse/core'
import { onMounted, ref } from 'vue'

const props = defineProps<{ anchor: HTMLElement; text: string }>()
const emit = defineEmits<{ dismiss: [] }>()

const GAP = 6
const EDGE = 8

const bubble = ref<HTMLElement | null>(null)
const style = ref<Record<string, string>>({})

// Below the anchor, centered and kept on screen; above it when there is no
// room below.
function place(): void {
  if (!bubble.value) return
  const rect = props.anchor.getBoundingClientRect()
  const { width, height } = bubble.value.getBoundingClientRect()
  const flip = window.innerHeight - rect.bottom - GAP - EDGE < height
  style.value = {
    left: `${Math.max(EDGE, Math.min(rect.left + rect.width / 2 - width / 2, window.innerWidth - EDGE - width))}px`,
    ...(flip ? { bottom: `${window.innerHeight - rect.top + GAP}px` } : { top: `${rect.bottom + GAP}px` }),
  }
}

onMounted(place)
useEventListener(window, 'scroll', () => emit('dismiss'), { capture: true })
useEventListener(window, 'resize', () => emit('dismiss'))
</script>

<template>
  <Teleport to="body">
    <div ref="bubble" class="app-tooltip" :style="style" role="tooltip" data-testid="app-tooltip">{{ text }}</div>
  </Teleport>
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
  border-radius: var(--radius-md);
  background: var(--color-pane);
  padding: 4px 7px;
  color: var(--color-text-2);
  font-size: var(--text-caption);
  line-height: 1.35;
  /* pre-line so a caller can put a second line in while long text still wraps
     at max-width. */
  white-space: pre-line;
  box-shadow: var(--shadow-popover);
}
</style>
