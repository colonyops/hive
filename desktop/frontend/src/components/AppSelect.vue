<script setup lang="ts">
// Themed single-select. A native <select> renders its open list with the OS
// chrome (a light popup that ignores the app theme); this reimplements the
// listbox so the open state matches the rest of the drawer.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'

const props = defineProps<{ modelValue: string; options: { value: string; label: string }[]; testid?: string; ariaLabel?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const root = ref<HTMLElement | null>(null)
const open = ref(false)
const active = ref(0)
const selectedLabel = computed(() => props.options.find((option) => option.value === props.modelValue)?.label ?? '')

function toggle(): void { open.value ? close() : openList() }
function openList(): void { open.value = true; active.value = Math.max(0, props.options.findIndex((option) => option.value === props.modelValue)) }
function close(): void { open.value = false }
function choose(value: string): void { if (value !== props.modelValue) emit('update:modelValue', value); close() }

function onKeydown(event: KeyboardEvent): void {
  if (!open.value) {
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') { event.preventDefault(); openList() }
    return
  }
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
  else if (event.key === 'ArrowDown') { event.preventDefault(); active.value = (active.value + 1) % props.options.length }
  else if (event.key === 'ArrowUp') { event.preventDefault(); active.value = (active.value - 1 + props.options.length) % props.options.length }
  else if (event.key === 'Home') { event.preventDefault(); active.value = 0 }
  else if (event.key === 'End') { event.preventDefault(); active.value = props.options.length - 1 }
  else if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); choose(props.options[active.value].value) }
}
function onDocumentPointer(event: PointerEvent): void { if (open.value && root.value && !root.value.contains(event.target as Node)) close() }
onMounted(() => document.addEventListener('pointerdown', onDocumentPointer))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocumentPointer))
</script>

<template>
  <div ref="root" class="relative" @keydown="onKeydown">
    <button
      type="button"
      class="flex w-full items-center justify-between gap-2 rounded border bg-app px-2 py-1.5 text-left text-text outline-none"
      :class="open ? 'border-accent' : 'border-border'"
      :data-testid="testid"
      :aria-label="ariaLabel"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="toggle"
    >
      <span class="truncate">{{ selectedLabel }}</span>
      <IconChevronDown class="size-4 shrink-0 text-text-3 transition-transform" :class="open ? 'rotate-180' : ''" />
    </button>
    <ul
      v-if="open"
      class="absolute inset-x-0 top-[calc(100%+6px)] z-20 flex flex-col gap-0.5 rounded-lg border border-card bg-raised p-[5px] shadow-[0_16px_34px_-12px_rgba(0,0,0,.6)]"
      role="listbox"
    >
      <li v-for="(option, index) in options" :key="option.value" role="option" :aria-selected="option.value === modelValue">
        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-md px-[9px] py-[7px] text-left text-[13px]"
          :class="[index === active ? 'bg-hover text-text' : 'text-text-2', option.value === modelValue ? 'text-text' : '']"
          :data-testid="testid ? `${testid}-option-${option.value}` : undefined"
          @click="choose(option.value)"
          @mousemove="active = index"
        >
          <IconCheck class="size-3.5 shrink-0" :class="option.value === modelValue ? 'text-accent' : 'opacity-0'" :stroke-width="3" />
          <span class="truncate">{{ option.label }}</span>
        </button>
      </li>
    </ul>
  </div>
</template>
