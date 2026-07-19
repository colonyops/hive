<script setup lang="ts">
import FieldRow from './FieldRow.vue'

const props = defineProps<{
  label?: string
  modelValue: string
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
  /** font-mono styling for ids/refs/globs, per the design system convention. */
  monospace?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).value)
}
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <input
      type="text"
      :value="modelValue"
      :placeholder="placeholder"
      class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
      :class="{ 'font-mono': monospace }"
      :data-testid="testid"
      @input="onInput"
    >
  </FieldRow>
</template>
