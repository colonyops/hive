<script setup lang="ts">
import { computed } from 'vue'
const props = withDefaults(
  defineProps<{
    modelValue: boolean
    label?: string
    hint?: string
    ariaLabel?: string
    disabled?: boolean
    size?: 'md' | 'sm'
    testid?: string
  }>(),
  { label: '', hint: '', ariaLabel: undefined, disabled: false, size: 'md', testid: undefined },
)
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const dims = computed(() =>
  props.size === 'sm'
    ? { track: 'h-3.5 w-6', knob: 'top-0.5 size-2.5', on: 'left-3', off: 'left-0.5' }
    : { track: 'h-4.25 w-7.5', knob: 'top-0.75 size-2.75', on: 'left-4', off: 'left-0.75' },
)
</script>

<template>
  <div>
    <button
      type="button"
      role="switch"
      :aria-checked="modelValue"
      :aria-label="ariaLabel"
      :disabled="disabled"
      class="flex items-center gap-2 text-left text-body outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50"
      :class="disabled ? '' : 'cursor-pointer'"
      :data-testid="testid"
      @click="emit('update:modelValue', !modelValue)"
    >
      <span
        class="relative shrink-0 rounded-full transition-colors"
        :class="[dims.track, modelValue ? 'bg-severity-success' : 'bg-chip']"
      >
        <span
          class="absolute rounded-full bg-pane transition-[left]"
          :class="[dims.knob, modelValue ? dims.on : dims.off]"
        />
      </span>
      <span v-if="label || $slots.default" :class="modelValue ? 'text-text' : 'text-text-2'">{{ label }}<slot /></span>
    </button>
    <p v-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4">{{ hint }}</p>
  </div>
</template>
