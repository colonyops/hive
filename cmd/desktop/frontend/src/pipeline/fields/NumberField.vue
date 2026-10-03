<script setup lang="ts">
import FormField from '../../components/ui/FormField.vue'
import TextInput from '../../components/ui/TextInput.vue'

const props = defineProps<{
  label?: string
  modelValue: number
  min?: number
  max?: number
  step?: number
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: number] }>()

function onUpdate(value: string) {
  const n = Number(value)
  emit('update:modelValue', Number.isFinite(n) ? n : 0)
}
</script>

<template>
  <FormField v-slot="{ id }" :label="label" :hint="hint" :error="error" :testid="testid">
    <TextInput
      :id="id"
      type="number"
      :model-value="String(modelValue)"
      :min="min"
      :max="max"
      :step="step"
      :placeholder="placeholder"
      :data-testid="testid"
      @update:model-value="onUpdate"
    />
  </FormField>
</template>
