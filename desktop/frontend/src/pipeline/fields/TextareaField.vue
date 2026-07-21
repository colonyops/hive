<script setup lang="ts">
import FieldRow from './FieldRow.vue'

const props = withDefaults(defineProps<{
  label?: string
  modelValue: string
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
  rows?: number
  /** font-mono styling for template and structured text fields. */
  monospace?: boolean
}>(), {
  rows: 3,
})

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
}
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <textarea
      :value="modelValue"
      :rows="rows"
      :placeholder="placeholder"
      class="w-full resize-y rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
      :class="{ 'font-mono': monospace }"
      :data-testid="testid"
      @input="onInput"
    />
  </FieldRow>
</template>
