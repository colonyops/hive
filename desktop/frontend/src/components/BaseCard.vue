<script setup lang="ts">
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  as?: 'article' | 'button'
  interactive?: boolean
  padded?: boolean
}>(), {
  as: 'article',
  interactive: false,
  padded: true,
})

const attrs = useAttrs()
const classes = computed(() => [
  'flex items-center gap-3',
  props.padded && 'p-4',
  props.interactive && 'transition-colors hover:border-strong hover:bg-chip',
])
</script>

<template>
  <component :is="props.as" v-bind="attrs" :class="classes">
    <slot name="icon" />
    <slot />
    <slot name="actions" />
  </component>
</template>
