<script setup lang="ts">
withDefaults(defineProps<{
  message?: string
  boxed?: boolean
}>(), {
  boxed: false,
})
</script>

<template>
  <div
    :class="[
      'py-8 text-center text-xs',
      boxed ? 'rounded-lg border border-border bg-raised px-4 text-text-3' : 'text-text-4',
    ]"
  >
    <slot>{{ message }}</slot>
  </div>
</template>
