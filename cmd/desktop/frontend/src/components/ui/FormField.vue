<script setup lang="ts">
import { useId } from 'vue'
import InlineError from './InlineError.vue'

defineProps<{
  label?: string
  hint?: string
  /** Replaces the hint while set. Accepts null so callers can pass an error ref straight through. */
  error?: string | null
  testid?: string
}>()

const id = useId()
</script>

<template>
  <div>
    <label v-if="label || $slots.label" :for="id" class="mb-1.5 block text-[12.5px] text-text-2">
      <slot name="label">{{ label }}</slot>
    </label>
    <slot :id="id" />
    <InlineError
      v-if="error"
      :testid="testid ? `${testid}-error` : undefined"
      variant="line"
      class="mt-1.5 leading-relaxed"
      :message="error"
    />
    <p
      v-else-if="hint"
      class="mt-1.5 text-xs leading-relaxed text-text-4"
      :data-testid="testid ? `${testid}-hint` : undefined"
    >
      {{ hint }}
    </p>
  </div>
</template>
