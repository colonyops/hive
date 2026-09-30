<script setup lang="ts">
import { computed } from 'vue'
const props = withDefaults(defineProps<{
  modelValue: boolean
  label?: string
  hint?: string
  ariaLabel?: string
  disabled?: boolean
  size?: 'md' | 'sm'
  testid?: string
}>(), { label: '', hint: '', ariaLabel: undefined, disabled: false, size: 'md', testid: undefined })
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const dims = computed(() => props.size === 'sm'
  ? { track: 'h-[14px] w-[24px]', knob: 'top-[2px] size-[10px]', on: 'left-[12px]', off: 'left-[2px]' }
  : { track: 'h-[17px] w-[30px]', knob: 'top-[3px] size-[11px]', on: 'left-[16px]', off: 'left-[3px]' })
</script>

<template>
  <div>
    <button type="button" role="switch" :aria-checked="modelValue" :aria-label="ariaLabel" :disabled="disabled" class="flex items-center gap-2 text-left text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50" :class="disabled ? '' : 'cursor-pointer'" :data-testid="testid" @click="emit('update:modelValue', !modelValue)">
      <span class="relative shrink-0 rounded-full transition-colors" :class="[dims.track, modelValue ? 'bg-severity-success' : 'bg-chip']">
        <span class="absolute rounded-full bg-pane transition-[left]" :class="[dims.knob, modelValue ? dims.on : dims.off]" />
      </span>
      <span v-if="label || $slots.default" :class="modelValue ? 'text-text' : 'text-text-2'">{{ label }}<slot /></span>
    </button>
    <p v-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4">{{ hint }}</p>
  </div>
</template>
