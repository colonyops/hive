<script setup lang="ts">
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  tone?: 'neutral' | 'success' | 'accent' | 'muted' | 'danger'
  variant?: 'pill' | 'chip'
  dot?: boolean
}>(), {
  tone: 'neutral',
  variant: 'chip',
  dot: false,
})

const attrs = useAttrs()

const toneClasses = {
  neutral: 'bg-chip text-text-3',
  success: 'bg-severity-success-tint text-severity-success',
  accent: 'bg-accent-tint text-accent',
  muted: 'bg-chip text-text-4',
  danger: 'bg-severity-error-tint text-severity-error',
}

const dotClasses = {
  neutral: 'bg-text-3',
  success: 'bg-severity-success',
  accent: 'bg-accent',
  muted: 'bg-text-4',
  danger: 'bg-severity-error',
}

const classes = computed(() => [
  'inline-flex items-center gap-1.5',
  props.variant === 'pill' ? 'rounded-full' : 'rounded-[5px]',
  toneClasses[props.tone],
])
</script>

<template>
  <span v-bind="attrs" :class="classes">
    <span v-if="props.dot" aria-hidden="true" class="size-1.5 shrink-0 rounded-full" :class="dotClasses[props.tone]" />
    <slot />
  </span>
</template>
