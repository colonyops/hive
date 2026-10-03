<script setup lang="ts">
// The one-glob-per-line textarea <-> string[] pattern extracted from
// FeedEditorSheet's filter groups. Globs may contain commas via brace
// expansion ("acme/{a,b}"), so lines are never comma-split.
import { computed } from 'vue'
import FormField from '../../components/ui/FormField.vue'
import TextArea from '../../components/ui/TextArea.vue'

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
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)
}

const text = computed(() => (props.modelValue ?? []).join('\n'))

function onUpdate(value: string) {
  emit('update:modelValue', parseLines(value))
}
</script>

<template>
  <FormField v-slot="{ id }" :label="label" :hint="hint" :error="error" :testid="testid">
    <TextArea
      :id="id"
      :model-value="text"
      :rows="rows ?? 2"
      :placeholder="placeholder"
      monospace
      class="leading-relaxed"
      :data-testid="testid"
      @update:model-value="onUpdate"
    />
  </FormField>
</template>
