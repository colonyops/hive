<script setup lang="ts">
// A hover tooltip, because `title` takes ~1-2s in WebKit with no knob to turn --
// too slow for a control whose meaning is readable only through its tooltip.
import { useTooltip } from '../../composables/useTooltip'
import TooltipBubble from './TooltipBubble.vue'

const props = withDefaults(
  defineProps<{
    /** Empty renders the trigger alone, with no tooltip at all. */
    text: string
    delay?: number
  }>(),
  { delay: 300 },
)

const { anchor, hide, triggers } = useTooltip(
  () => props.text,
  () => props.delay,
)
</script>

<template>
  <span class="inline-flex" v-on="triggers">
    <slot />
    <TooltipBubble v-if="anchor" :anchor="anchor" :text="text" @dismiss="hide" />
  </span>
</template>
