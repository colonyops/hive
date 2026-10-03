<script setup lang="ts">
import FormField from '../../components/ui/FormField.vue'
import TextArea from '../../components/ui/TextArea.vue'

withDefaults(
  defineProps<{
    label?: string
    modelValue: string
    placeholder?: string
    hint?: string
    error?: string
    testid?: string
    rows?: number
    /** font-mono styling for template and structured text fields. */
    monospace?: boolean
  }>(),
  {
    rows: 3,
  },
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <FormField v-slot="{ id }" :label="label" :hint="hint" :error="error" :testid="testid">
    <TextArea
      :id="id"
      :model-value="modelValue"
      :rows="rows"
      :placeholder="placeholder"
      :monospace="monospace"
      :data-testid="testid"
      @update:model-value="emit('update:modelValue', $event)"
    />
  </FormField>
</template>
