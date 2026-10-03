<script setup lang="ts">
import { ref } from 'vue'
import FormField from '../../components/ui/FormField.vue'
import TextInput from '../../components/ui/TextInput.vue'

defineProps<{
  label?: string
  modelValue?: string
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
  disabled?: boolean
  /** font-mono styling for ids/refs/globs, per the design system convention. */
  monospace?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const inputRef = ref<{ focus: () => void } | null>(null)

defineExpose({ focus: () => inputRef.value?.focus() })
</script>

<template>
  <FormField v-slot="{ id }" :label="label" :hint="hint" :error="error" :testid="testid">
    <!-- `trailing` holds an in-field affordance (e.g. a regenerate button).
         The input gains right padding only when the slot is filled. -->
    <div class="relative">
      <TextInput
        :id="id"
        ref="inputRef"
        :model-value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        :monospace="monospace"
        :class="{ 'pr-10': $slots.trailing }"
        :data-testid="testid"
        @update:model-value="emit('update:modelValue', $event)"
      />
      <div v-if="$slots.trailing" class="absolute inset-y-0 right-0 flex items-center pr-1.5">
        <slot name="trailing" />
      </div>
    </div>
  </FormField>
</template>
