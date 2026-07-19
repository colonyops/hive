<script setup lang="ts">
// The one-glob-per-line textarea <-> string[] pattern extracted from
// FeedEditorSheet's filter groups. Globs may contain commas via brace
// expansion ("acme/{a,b}"), so lines are never comma-split.
import { computed } from 'vue'
import FieldRow from './FieldRow.vue'

const props = defineProps<{
  label?: string
  modelValue: string[]
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
  rows?: number
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()

function parseLines(text: string): string[] {
  return text.split('\n').map((line) => line.trim()).filter((line) => line.length > 0)
}

const text = computed(() => (props.modelValue ?? []).join('\n'))

function onInput(e: Event) {
  emit('update:modelValue', parseLines((e.target as HTMLTextAreaElement).value))
}
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <textarea
      :value="text"
      :rows="rows ?? 2"
      :placeholder="placeholder"
      class="w-full resize-y rounded-lg border border-strong bg-app px-3 py-2.5 font-mono text-[12.5px] leading-relaxed text-text outline-none placeholder:text-text-4 focus:border-accent"
      :data-testid="testid"
      @input="onInput"
    />
  </FieldRow>
</template>
