<script setup lang="ts">
// The one way to show an error. Hand-rolled copies drifted apart on colour,
// radius, and border, so a failure looked like a different kind of problem
// depending on which view raised it. Spacing is the parent's job: pass it as
// a class.
withDefaults(
  defineProps<{
    /** Accepts null so callers can pass a nullable error ref straight through. */
    message?: string | null
    testid?: string
    /** `banner` boxes the message; `line` is bare text under a field or in a list. */
    variant?: 'banner' | 'line'
  }>(),
  { message: null, testid: undefined, variant: 'banner' },
)
</script>

<template>
  <div
    role="alert"
    class="text-xs text-severity-error"
    :class="
      variant === 'banner'
        ? 'rounded-md border border-severity-error-border bg-severity-error-tint px-3 py-2 leading-relaxed'
        : ''
    "
    :data-testid="testid"
  >
    <slot>{{ message }}</slot>
  </div>
</template>
