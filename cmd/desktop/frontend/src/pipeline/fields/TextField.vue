<script setup lang="ts">
import { ref } from 'vue'
import FieldRow from './FieldRow.vue'

const props = defineProps<{
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
const inputRef = ref<HTMLInputElement | null>(null)

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).value)
}

function focus() {
  inputRef.value?.focus()
}

defineExpose({ focus })
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <!-- `trailing` holds an in-field affordance (e.g. a regenerate button).
         The input gains right padding only when the slot is filled, so
         fields without one keep their exact previous metrics. -->
    <div class="relative">
      <input
        ref="inputRef"
        type="text"
        :value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent disabled:opacity-60"
        :class="[{ 'font-mono': monospace }, $slots.trailing ? 'pr-10' : '']"
        :data-testid="testid"
        @input="onInput"
      >
      <div v-if="$slots.trailing" class="absolute inset-y-0 right-0 flex items-center pr-1.5">
        <slot name="trailing" />
      </div>
    </div>
  </FieldRow>
</template>
