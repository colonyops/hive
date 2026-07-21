<script setup lang="ts">
import { computed, ref } from 'vue'

const props = withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  size?: 'sm' | 'md'
  busy?: boolean
  disabled?: boolean
  type?: 'button' | 'submit'
}>(), {
  variant: 'primary',
  size: 'md',
  busy: false,
  disabled: false,
  type: 'button',
})

const emit = defineEmits<{ click: [event: MouseEvent] }>()
const buttonRef = ref<HTMLButtonElement | null>(null)

const classes = computed(() => [
  'inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-lg transition disabled:cursor-default disabled:opacity-50',
  props.size === 'md'
    ? 'px-4 py-2.5 text-[13.5px] font-semibold'
    : 'px-3.5 py-2 text-[13px] font-medium',
  {
    primary: 'bg-accent text-accent-contrast hover:brightness-110',
    secondary: 'border border-card text-text-2 hover:text-text',
    danger: 'bg-severity-error text-accent-contrast hover:brightness-110',
    ghost: 'text-text-2 hover:text-text',
  }[props.variant],
])

defineExpose({ focus: () => buttonRef.value?.focus() })
</script>

<template>
  <button ref="buttonRef" :type="type" :disabled="disabled || busy" :class="classes" @click="emit('click', $event)">
    <slot name="icon" />
    <slot />
  </button>
</template>
