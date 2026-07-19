<script setup lang="ts">
import IconMinus from '~icons/lucide/minus'
import IconPlus from '~icons/lucide/plus'
import SettingsField from './SettingsField.vue'

const props = defineProps<{
  label?: string
  modelValue: number
  min?: number
  max?: number
  step?: number
  suffix?: string
  hint?: string
  testid?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: number] }>()

function clamp(n: number): number {
  let v = n
  if (props.min !== undefined) v = Math.max(props.min, v)
  if (props.max !== undefined) v = Math.min(props.max, v)
  return v
}

function decrement() {
  emit('update:modelValue', clamp(props.modelValue - (props.step ?? 1)))
}

function increment() {
  emit('update:modelValue', clamp(props.modelValue + (props.step ?? 1)))
}
</script>

<template>
  <SettingsField :label="label" :hint="hint" :testid="testid">
    <div class="flex h-10 w-fit items-center overflow-hidden rounded-lg border border-strong bg-app font-mono text-[13px] text-text">
      <button
        type="button"
        class="flex h-full w-9 cursor-pointer items-center justify-center text-text-2 hover:bg-chip hover:text-text disabled:cursor-default disabled:opacity-30"
        :aria-label="`Decrease ${label ?? 'value'}`"
        :disabled="min !== undefined && modelValue <= min"
        :data-testid="testid ? `${testid}-decrement` : undefined"
        @click="decrement"
      ><IconMinus class="size-3.5" /></button>
      <span class="min-w-[3.5em] px-2 text-center" :data-testid="testid ? `${testid}-value` : undefined">{{ modelValue }}<template v-if="suffix">{{ suffix }}</template></span>
      <button
        type="button"
        class="flex h-full w-9 cursor-pointer items-center justify-center text-text-2 hover:bg-chip hover:text-text disabled:cursor-default disabled:opacity-30"
        :aria-label="`Increase ${label ?? 'value'}`"
        :disabled="max !== undefined && modelValue >= max"
        :data-testid="testid ? `${testid}-increment` : undefined"
        @click="increment"
      ><IconPlus class="size-3.5" /></button>
    </div>
  </SettingsField>
</template>
