<script setup lang="ts">
const props = withDefaults(defineProps<{
  modelValue: boolean
  label?: string
  hint?: string
  ariaLabel?: string
  disabled?: boolean
  testid?: string
}>(), { label: '', hint: '', ariaLabel: undefined, disabled: false, testid: undefined })
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <div>
    <button type="button" role="switch" :aria-checked="modelValue" :aria-label="ariaLabel" :disabled="disabled" class="flex items-center gap-2 text-left text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50" :class="disabled ? '' : 'cursor-pointer'" :data-testid="testid" @click="emit('update:modelValue', !modelValue)">
      <span class="relative h-[17px] w-[30px] shrink-0 rounded-full transition-colors" :class="modelValue ? 'bg-severity-success' : 'bg-chip'">
        <span class="absolute top-[3px] size-[11px] rounded-full bg-pane transition-[left]" :class="modelValue ? 'left-[16px]' : 'left-[3px]'" />
      </span>
      <span :class="modelValue ? 'text-text' : 'text-text-2'">{{ label }}<slot /></span>
    </button>
    <p v-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4">{{ hint }}</p>
  </div>
</template>
