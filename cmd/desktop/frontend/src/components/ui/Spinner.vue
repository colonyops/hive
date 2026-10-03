<script setup lang="ts">
import IconLoaderCircle from '~icons/lucide/loader-circle'

withDefaults(
  defineProps<{
    size?: 'sm' | 'md'
    /** Announces the wait to screen readers; without it the spinner is decorative. */
    label?: string
  }>(),
  { size: 'md', label: undefined },
)
</script>

<template>
  <IconLoaderCircle
    :class="['shrink-0 animate-spin', size === 'sm' ? 'size-3' : 'size-3.5']"
    :role="label ? 'status' : undefined"
    :aria-label="label"
    :aria-hidden="label ? undefined : 'true'"
  />
</template>
