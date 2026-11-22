<script setup lang="ts">
// One card in a comments column. Aligned, it sits at `top` beside the text it
// is about (see useAlignedCards); stacked, it follows the card above.
defineProps<{
  active?: boolean
  hovered?: boolean
  aligned?: boolean
  /** Animates a move to a new `top`. */
  sliding?: boolean
  top?: number
}>()
const emit = defineEmits<{ select: []; hover: [on: boolean] }>()
</script>

<template>
  <li
    class="flex cursor-pointer flex-col gap-2 rounded-lg border px-3.5 py-3 duration-150"
    :class="[
      aligned ? 'absolute inset-x-0' : 'relative',
      sliding ? 'transition-[top,border-color,background-color]' : 'transition-[border-color,background-color]',
      active ? 'border-accent/45 bg-raised' : hovered ? 'border-strong bg-raised' : 'border-card bg-raised/60',
    ]"
    :style="aligned ? { top: `${top ?? 0}px` } : undefined"
    @click="emit('select')"
    @mouseenter="emit('hover', true)"
    @mouseleave="emit('hover', false)"
  >
    <span v-if="aligned && active" class="absolute top-5.5 -left-4 h-px w-3 bg-accent/50" aria-hidden="true" />
    <slot />
  </li>
</template>
