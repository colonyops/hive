<script setup lang="ts">
import { computed, ref } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import type { MenuEntry } from '../types/menu'
import AppMenu from './ui/AppMenu.vue'
import BaseButton from './ui/BaseButton.vue'

const props = defineProps<{ modelValue: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()

const options = [
  { value: 'error', label: 'Error' },
  { value: 'warn', label: 'Warning' },
  { value: 'info', label: 'Info' },
  { value: 'debug', label: 'Debug' },
  { value: 'unknown', label: 'Unknown' },
]

const root = ref<HTMLElement | null>(null)
const open = ref(false)
const selected = computed(() => new Set(props.modelValue))
const label = computed(() => {
  const labels = options.filter((option) => selected.value.has(option.value)).map((option) => option.label)
  if (!labels.length) return 'All levels'
  if (labels.length <= 2) return labels.join(' + ')
  return `${labels.length} levels`
})
const entries = computed<MenuEntry[]>(() => [
  {
    kind: 'action',
    id: 'all',
    label: 'All levels',
    checked: props.modelValue.length === 0,
    testid: 'diagnostics-level-option-all',
  },
  { kind: 'separator' },
  ...options.map((option): MenuEntry => ({
    kind: 'action',
    id: option.value,
    label: option.label,
    checked: selected.value.has(option.value),
    testid: `diagnostics-level-option-${option.value}`,
  })),
])

function select(id: string): void {
  if (id === 'all') {
    emit('update:modelValue', [])
    return
  }
  const next = new Set(props.modelValue)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  emit(
    'update:modelValue',
    options.map((option) => option.value).filter((level) => next.has(level)),
  )
}
</script>

<template>
  <div ref="root" class="relative shrink-0">
    <BaseButton
      variant="secondary"
      size="xs"
      aria-haspopup="menu"
      :aria-expanded="open"
      data-testid="diagnostics-level-filter"
      @click="open = !open"
    >
      {{ label }}
      <IconChevronDown class="size-3.5 transition-transform" :class="open ? 'rotate-180' : ''" />
    </BaseButton>
    <AppMenu
      v-if="open"
      :entries="entries"
      :ignore="[root]"
      testid="diagnostics-level-menu"
      @select="select"
      @close="open = false"
    />
  </div>
</template>
