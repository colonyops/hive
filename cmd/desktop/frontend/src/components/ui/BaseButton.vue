<script setup lang="ts">
import { computed, ref } from 'vue'
import Spinner from './Spinner.vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'danger-outline' | 'ghost'
    size?: 'xs' | 'sm' | 'md'
    busy?: boolean
    disabled?: boolean
    type?: 'button' | 'submit'
  }>(),
  {
    variant: 'primary',
    size: 'md',
    busy: false,
    disabled: false,
    type: 'button',
  },
)

const emit = defineEmits<{ click: [event: MouseEvent] }>()
const buttonRef = ref<HTMLButtonElement | null>(null)

const classes = computed(() => [
  'inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-lg transition disabled:cursor-default disabled:opacity-50',
  {
    xs: 'px-3 py-1.5 text-small font-medium',
    sm: 'px-3.5 py-2 text-body font-medium',
    md: 'px-4 py-2.5 text-body font-semibold',
  }[props.size],
  {
    primary: 'bg-accent text-accent-contrast hover:brightness-110',
    secondary: 'border border-card text-text-2 hover:text-text',
    danger: 'bg-severity-error text-accent-contrast hover:brightness-110',
    // Outlined destructive: for confirmations that sit inside another surface,
    // where a filled danger button would out-shout the surface's own primary.
    'danger-outline': 'border border-severity-error/40 text-severity-error hover:bg-severity-error-tint',
    ghost: 'text-text-2 hover:text-text',
  }[props.variant],
])

defineExpose({ focus: () => buttonRef.value?.focus() })
</script>

<template>
  <button
    ref="buttonRef"
    :type="type"
    :disabled="disabled || busy"
    :aria-busy="busy || undefined"
    :class="classes"
    @click="emit('click', $event)"
  >
    <Spinner v-if="busy" />
    <slot v-else name="icon" />
    <slot />
  </button>
</template>
