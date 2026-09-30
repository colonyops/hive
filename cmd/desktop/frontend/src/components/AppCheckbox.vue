<script setup lang="ts">
import IconCheck from '~icons/lucide/check'

const props = withDefaults(defineProps<{
  modelValue: boolean
  label?: string
  hint?: string
  disabled?: boolean
  testid?: string
}>(), { label: '', hint: '', disabled: false, testid: undefined })
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

function onChange(event: Event): void {
  emit('update:modelValue', (event.target as HTMLInputElement).checked)
}
</script>

<template>
  <div>
    <label class="flex items-center gap-2 text-[13px]" :class="disabled ? 'cursor-not-allowed text-text-4' : 'cursor-pointer'">
      <span class="relative flex size-4 shrink-0 items-center justify-center rounded-[5px] border-[1.5px] transition-colors focus-within:ring-2 focus-within:ring-accent" :class="modelValue ? 'border-accent bg-accent' : 'border-strong bg-transparent'">
        <input type="checkbox" :checked="modelValue" :disabled="disabled" class="absolute inset-0 size-full cursor-pointer opacity-0 disabled:cursor-not-allowed" :data-testid="testid" @change="onChange">
        <IconCheck v-if="modelValue" class="size-2.5 text-accent-contrast" :stroke-width="3.2" />
      </span>
      <span :class="modelValue ? 'text-text' : 'text-text-2'">{{ label }}<slot /></span>
    </label>
    <p v-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4">{{ hint }}</p>
  </div>
</template>
