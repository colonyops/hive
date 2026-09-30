<script setup lang="ts">
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  size?: number
  rounded?: string
}>(), {
  size: 38,
  rounded: 'rounded-[7px]',
})

const attrs = useAttrs()
const classes = computed(() => [
  'flex shrink-0 items-center justify-center',
  props.rounded,
])
</script>

<template>
  <span
    v-bind="attrs"
    :class="classes"
    :style="{ width: `${props.size}px`, height: `${props.size}px` }"
  ><slot /></span>
</template>
