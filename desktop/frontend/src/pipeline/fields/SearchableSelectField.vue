<script setup lang="ts">
// A themed, searchable single-select built on the same reimplemented-listbox
// approach as components/AppSelect (a native <select> can't be themed in the
// WebKit webviews Wails uses, and its picker can't show icons or a filter).
// Each option may carry an icon component, rendered both in the trigger's
// selected preview and beside every option row. The popover opens with a
// search box that filters options by label; arrow keys walk the filtered
// list, Enter selects, Esc closes. Wrapped in FieldRow for the shared
// label/hint/error chrome, matching SelectField.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Component } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconSearch from '~icons/lucide/search'
import FieldRow from './FieldRow.vue'

export interface SearchableOption {
  value: string
  label: string
  icon?: Component
}

const props = defineProps<{
  label?: string
  modelValue: string
  options: SearchableOption[]
  placeholder?: string
  searchPlaceholder?: string
  hint?: string
  error?: string
  testid?: string
  ariaLabel?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const root = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)
const open = ref(false)
const query = ref('')
const active = ref(0)

const selected = computed(() => props.options.find((o) => o.value === props.modelValue) ?? null)
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter((o) => o.label.toLowerCase().includes(q))
})

// Keep the active index in range as the filtered list shrinks/grows.
watch(filtered, (list) => { if (active.value >= list.length) active.value = Math.max(0, list.length - 1) })

function openList(): void {
  open.value = true
  query.value = ''
  active.value = Math.max(0, filtered.value.findIndex((o) => o.value === props.modelValue))
  void nextTick(() => searchInput.value?.focus())
}
function close(): void { open.value = false }
function toggle(): void { open.value ? close() : openList() }
function choose(value: string): void {
  if (value !== props.modelValue) emit('update:modelValue', value)
  close()
}

function onTriggerKeydown(event: KeyboardEvent): void {
  if (open.value) return
  if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') { event.preventDefault(); openList() }
}

function onSearchKeydown(event: KeyboardEvent): void {
  const list = filtered.value
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
  else if (event.key === 'ArrowDown') { event.preventDefault(); if (list.length) active.value = (active.value + 1) % list.length }
  else if (event.key === 'ArrowUp') { event.preventDefault(); if (list.length) active.value = (active.value - 1 + list.length) % list.length }
  else if (event.key === 'Home') { event.preventDefault(); active.value = 0 }
  else if (event.key === 'End') { event.preventDefault(); active.value = list.length - 1 }
  else if (event.key === 'Enter') { event.preventDefault(); if (list[active.value]) choose(list[active.value].value) }
}

function onDocumentPointer(event: PointerEvent): void {
  if (open.value && root.value && !root.value.contains(event.target as Node)) close()
}
onMounted(() => document.addEventListener('pointerdown', onDocumentPointer))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocumentPointer))
</script>

<template>
  <FieldRow :label="label" :hint="hint" :error="error" :testid="testid">
    <div ref="root" class="relative">
      <button
        type="button"
        class="flex w-full items-center gap-2 rounded-lg border bg-app px-3 py-2.5 text-left text-[13.5px] text-text outline-none"
        :class="open ? 'border-accent' : 'border-strong'"
        :data-testid="testid"
        :aria-label="ariaLabel"
        aria-haspopup="listbox"
        :aria-expanded="open"
        @click="toggle"
        @keydown="onTriggerKeydown"
      >
        <component :is="selected.icon" v-if="selected?.icon" class="size-4 shrink-0 text-text-2" />
        <span class="min-w-0 flex-1 truncate" :class="selected ? '' : 'text-text-4'">{{ selected?.label ?? placeholder ?? 'Select…' }}</span>
        <IconChevronDown class="size-4 shrink-0 text-text-3 transition-transform" :class="open ? 'rotate-180' : ''" />
      </button>

      <div
        v-if="open"
        class="absolute inset-x-0 top-[calc(100%+6px)] z-20 flex flex-col rounded-lg border border-card bg-raised shadow-[0_16px_34px_-12px_rgba(0,0,0,.6)]"
      >
        <div class="flex items-center gap-2 border-b border-row px-2.5 py-2">
          <IconSearch class="size-3.5 shrink-0 text-text-4" />
          <input
            ref="searchInput"
            v-model="query"
            type="text"
            :placeholder="searchPlaceholder ?? 'Search…'"
            class="min-w-0 flex-1 bg-transparent text-[13px] text-text outline-none placeholder:text-text-4"
            :data-testid="testid ? `${testid}-search` : undefined"
            @keydown="onSearchKeydown"
          >
        </div>
        <ul
          v-if="filtered.length"
          class="hive-scroll flex max-h-[240px] flex-col gap-0.5 overflow-y-auto p-[5px]"
          role="listbox"
        >
          <li v-for="(option, index) in filtered" :key="option.value" role="option" :aria-selected="option.value === modelValue">
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-[9px] py-[7px] text-left text-[13px]"
              :class="[index === active ? 'bg-hover text-text' : 'text-text-2', option.value === modelValue ? 'text-text' : '']"
              :data-testid="testid ? `${testid}-option-${option.value}` : undefined"
              @click="choose(option.value)"
              @mousemove="active = index"
            >
              <IconCheck class="size-3.5 shrink-0" :class="option.value === modelValue ? 'text-accent' : 'opacity-0'" :stroke-width="3" />
              <component :is="option.icon" v-if="option.icon" class="size-4 shrink-0 text-text-2" />
              <span class="min-w-0 flex-1 truncate">{{ option.label }}</span>
            </button>
          </li>
        </ul>
        <div v-else class="px-3 py-4 text-center text-[12.5px] text-text-4" :data-testid="testid ? `${testid}-empty` : undefined">
          No matches
        </div>
      </div>
    </div>
  </FieldRow>
</template>
