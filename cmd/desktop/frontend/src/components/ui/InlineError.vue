<script setup lang="ts">
// The one inline error banner. Hand-rolled variants drifted apart on radius,
// border colour, and margin, so a failure looked like a different kind of
// problem depending on which view raised it. Spacing is the parent's job,
// not the banner's.
defineProps<{
  /** Accepts null so callers can pass a nullable error ref straight through. */
  message?: string | null
  testid?: string
}>()
</script>

<template>
  <p
    class="rounded-md border border-severity-error-border bg-severity-error-tint px-3 py-2 text-xs leading-relaxed text-severity-error"
    :data-testid="testid"
  >
    <slot>{{ message }}</slot>
  </p>
</template>
