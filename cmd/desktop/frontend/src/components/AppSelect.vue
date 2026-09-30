<script setup lang="ts">
// The app's only single-select control. A native select element renders its
// open list with the OS chrome (a light popup that ignores the app theme), so
// this reimplements the listbox: trigger button + popover, full keyboard
// handling, optional search filter and per-option icons.
import { onClickOutside } from '@vueuse/core'
import { computed, nextTick, ref, useId, watch } from 'vue'
import type { Component } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconSearch from '~icons/lucide/search'
import { useAnchoredPopover } from '../composables/useAnchoredPopover'

export interface AppSelectOption {
  value: string
  label: string
  /** A dim second line under the label, shown in the list only — the trigger stays one line. */
  hint?: string
  icon?: Component
  disabled?: boolean
}

/** `md` matches TextField's metrics (it sits beside one in every form); `sm` is for toolbars and sidebars. */
export type AppSelectSize = 'sm' | 'md'

const props = withDefaults(defineProps<{
  modelValue: string
  options: AppSelectOption[]
  /** Shown when modelValue matches no option. An option with value '' is a real choice, not a placeholder. */
  placeholder?: string
  searchable?: boolean
  searchPlaceholder?: string
  /** Combobox mode: the trigger is a text input, so modelValue may be a value not in options (e.g. a custom git URL). */
  editable?: boolean
  disabled?: boolean
  size?: AppSelectSize
  testid?: string
  ariaLabel?: string
}>(), { size: 'md' })

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const listboxId = useId()

const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLElement | null>(null)
const popover = ref<HTMLElement | null>(null)
const list = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)
const editableInput = ref<HTMLInputElement | null>(null)
const open = ref(false)
const query = ref('')
// Editable mode: `text` is the input's own value (kept in sync with modelValue),
// and `touched` gates filtering to after a keystroke so opening shows the whole
// list — a prefilled value never hides the other options.
const text = ref(props.modelValue)
const touched = ref(false)
const active = ref(0)
watch(() => props.modelValue, (value) => { text.value = value })

function matches(option: AppSelectOption, query: string): boolean {
  return `${option.label} ${option.value} ${option.hint ?? ''}`.toLowerCase().includes(query)
}

const selected = computed(() => props.options.find((option) => option.value === props.modelValue) ?? null)
const visible = computed(() => {
  if (props.editable) {
    const q = touched.value ? text.value.trim().toLowerCase() : ''
    if (!q) return props.options
    return props.options.filter((option) => matches(option, q))
  }
  const q = props.searchable ? query.value.trim().toLowerCase() : ''
  if (!q) return props.options
  return props.options.filter((option) => matches(option, q))
})

const triggerClass = computed(() => ({
  sm: 'gap-1.5 rounded-md px-2 py-1.5 text-[11px]',
  md: 'gap-2 rounded-lg px-3 py-2.5 text-[13.5px]',
}[props.size]))
const optionText = computed(() => (props.size === 'sm' ? 'text-[12px]' : 'text-[13px]'))

// Two independent signals, following AppMenu/CommandPalette: `bg-hover` is
// where the keyboard is, the trailing check is what's selected. The check is
// rendered only on the selected row rather than sitting invisible on every
// other one — a reserved leading gutter indents every label for the sake of
// one, which reads badly in a narrow list.
function optionClass(option: AppSelectOption, index: number): string[] {
  const isActive = index === active.value && !option.disabled
  return [
    optionText.value,
    isActive ? 'bg-hover' : '',
    option.value === props.modelValue || isActive ? 'text-text' : 'text-text-2',
  ]
}

// Keep the active index in range as the filtered list shrinks/grows.
watch(visible, (options) => { if (active.value >= options.length) active.value = Math.max(0, options.length - 1) })

/** The list scrolls past its max height (the icon picker is 30+ rows), so keep the active row on screen. */
function revealActive(): void {
  if (!open.value) return
  void nextTick(() => (list.value?.children[active.value]?.firstElementChild as HTMLElement | undefined)?.scrollIntoView?.({ block: 'nearest' }))
}
watch(active, revealActive)

function firstEnabled(): number {
  const index = visible.value.findIndex((option) => !option.disabled)
  return index === -1 ? 0 : index
}

function openList(): void {
  if (props.disabled) return
  open.value = true
  if (props.editable) touched.value = false
  else query.value = ''
  const current = visible.value.findIndex((option) => option.value === props.modelValue)
  active.value = current === -1 ? firstEnabled() : current
  measure()
  revealActive()
  void nextTick(() => {
    measure() // now that the list has rendered and its natural width is known
    if (props.searchable) searchInput.value?.focus()
    else if (props.editable) { editableInput.value?.focus(); editableInput.value?.select() }
    // A mouse click does not focus a button in WebKit, so without this an
    // open plain listbox can have no focused element inside `root` and
    // onKeydown never fires — arrows and Escape then land in window-level
    // handlers (the tasks tree's navigation, escape-to-close) instead of
    // the list.
    else trigger.value?.focus()
  })
}

// The search box lives in the popover this removes, so closing while it holds
// focus would strand focus on <body> and the next Tab would restart at the top
// of the app instead of moving to the next field. Editable mode never takes
// focus off its own trigger, and refocusing it would reopen the list.
function close(): void {
  const reclaim = !props.editable && (popover.value?.contains(document.activeElement) ?? false)
  open.value = false
  if (reclaim) trigger.value?.focus()
}
function toggle(): void { open.value ? close() : openList() }

function choose(option: AppSelectOption): void {
  if (option.disabled) return
  if (props.editable) { text.value = option.value; touched.value = false }
  if (option.value !== props.modelValue) emit('update:modelValue', option.value)
  close()
}

// Combobox trigger: focusing opens the full list (touched stays false) and
// selects the text so the first keystroke replaces the prefilled value rather
// than appending to it.
function onEditableFocus(): void {
  if (!open.value) openList()
}
function onEditableInput(event: Event): void {
  text.value = (event.target as HTMLInputElement).value
  touched.value = true
  if (!open.value) open.value = true
  active.value = 0
  emit('update:modelValue', text.value)
  measure()
}

/** Walk to the next enabled option, wrapping; a fully disabled list leaves `active` alone. */
function step(delta: number): void {
  const options = visible.value
  if (!options.length) return
  let index = active.value
  for (let n = 0; n < options.length; n++) {
    index = (index + delta + options.length) % options.length
    if (!options[index].disabled) { active.value = index; return }
  }
}

/** Home/End land on the outermost enabled option, not on a disabled edge row. */
function jump(edge: 'start' | 'end'): void {
  const options = visible.value
  const delta = edge === 'start' ? 1 : -1
  for (let index = edge === 'start' ? 0 : options.length - 1; index >= 0 && index < options.length; index += delta) {
    if (!options[index].disabled) { active.value = index; return }
  }
}

/** Shared by the trigger (closed list) and the search box (open list). */
function onKeydown(event: KeyboardEvent): void {
  if (!open.value) {
    if (props.editable) {
      if (event.key === 'ArrowDown') { event.preventDefault(); openList() }
      return // a closed combobox leaves Enter/Space to the form and the input
    }
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') { event.preventDefault(); openList() }
    return
  }
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
  else if (event.key === 'ArrowDown') { event.preventDefault(); step(1) }
  else if (event.key === 'ArrowUp') { event.preventDefault(); step(-1) }
  else if (event.key === 'Home' && !props.editable) { event.preventDefault(); jump('start') }
  else if (event.key === 'End' && !props.editable) { event.preventDefault(); jump('end') }
  else if (event.key === 'Enter') {
    event.preventDefault()
    const option = visible.value[active.value]
    if (option) choose(option)
    else if (props.editable) close() // accept the typed value
  }
  else if (event.key === ' ' && !props.searchable && !props.editable) {
    // Space types a character in the search/combobox input, so only Enter commits there.
    event.preventDefault()
    const option = visible.value[active.value]
    if (option) choose(option)
  }
  // Not prevented: close() puts focus back on the trigger first, so the default
  // Tab carries on from there into the next field.
  else if (event.key === 'Tab') close()
  // An open plain listbox owns the keyboard the way a native select does:
  // the keys it ignores (horizontal arrows, bare letters) are marked handled
  // so a window-level hotkey layer — the tasks tree walks on exactly these —
  // cannot act underneath it. Search/editable mode keeps them: there they
  // move the caret or type into an input, and the focused input is what
  // shields them from hotkey layers instead.
  else if (
    !props.searchable && !props.editable
    && (event.key === 'ArrowLeft' || event.key === 'ArrowRight'
      || (event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey))
  ) {
    event.preventDefault()
  }
}

const { style: popoverStyle, measure } = useAnchoredPopover(root, popover, open)

// The popover lives outside `root` once teleported, so it has to be ignored
// explicitly or clicking an option would count as a click outside.
onClickOutside(root, () => { if (open.value) close() }, { ignore: [popover] })
</script>

<template>
  <div ref="root" class="relative" @keydown="onKeydown">
    <template v-if="editable">
      <input
        ref="editableInput"
        :value="text"
        type="text"
        class="w-full border bg-app pr-9 text-left text-text outline-none placeholder:text-text-4 disabled:cursor-not-allowed disabled:opacity-60"
        :class="[triggerClass, open ? 'border-accent' : 'border-strong']"
        :data-testid="testid"
        :aria-label="ariaLabel"
        :placeholder="placeholder ?? ''"
        :disabled="disabled"
        role="combobox"
        aria-autocomplete="list"
        :aria-expanded="open"
        :aria-controls="open ? listboxId : undefined"
        @focus="onEditableFocus"
        @input="onEditableInput"
        @click="open || openList()"
        @blur="close"
      >
      <button type="button" tabindex="-1" aria-hidden="true" class="absolute inset-y-0 right-0 flex items-center px-2.5 text-text-3" :disabled="disabled" @mousedown.prevent="toggle">
        <IconChevronDown class="size-4 transition-transform" :class="open ? 'rotate-180' : ''" />
      </button>
    </template>
    <button
      v-else
      ref="trigger"
      type="button"
      class="flex w-full items-center justify-between border bg-app text-left text-text outline-none disabled:cursor-not-allowed disabled:opacity-60"
      :class="[triggerClass, open ? 'border-accent' : 'border-strong']"
      :data-testid="testid"
      :aria-label="ariaLabel"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="open ? listboxId : undefined"
      @click="toggle"
    >
      <component :is="selected.icon" v-if="selected?.icon" class="size-4 shrink-0 text-text-2" />
      <span class="min-w-0 flex-1 truncate" :class="selected ? '' : 'text-text-4'">{{ selected ? selected.label : (placeholder ?? '') }}</span>
      <IconChevronDown class="size-4 shrink-0 text-text-3 transition-transform" :class="open ? 'rotate-180' : ''" />
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="popover"
        class="fixed z-50 flex flex-col overflow-hidden rounded-lg border border-card bg-raised shadow-[0_16px_34px_-12px_rgba(0,0,0,.6)]"
        :style="popoverStyle"
        :data-testid="testid ? `${testid}-popover` : undefined"
      >
        <div v-if="searchable" class="flex shrink-0 items-center gap-2 border-b border-row px-2.5 py-2">
          <IconSearch class="size-3.5 shrink-0 text-text-4" />
          <!-- w-0: an input's default intrinsic width would otherwise set the popover's width. -->
          <input
            ref="searchInput"
            v-model="query"
            type="text"
            :placeholder="searchPlaceholder ?? 'Search…'"
            class="w-0 min-w-0 flex-1 bg-transparent text-[13px] text-text outline-none placeholder:text-text-4"
            :data-testid="testid ? `${testid}-search` : undefined"
            @keydown="onKeydown"
          >
        </div>
        <ul
          v-if="visible.length"
          :id="listboxId"
          ref="list"
          class="hive-scroll flex min-h-0 flex-col gap-0.5 overflow-y-auto p-[5px]"
          role="listbox"
          :aria-label="ariaLabel"
        >
          <li v-for="(option, index) in visible" :key="option.value" role="option" :aria-selected="option.value === modelValue">
            <button
              type="button"
              class="flex w-full gap-2 rounded-md px-[9px] py-[7px] text-left disabled:cursor-not-allowed disabled:opacity-40"
              :class="[optionClass(option, index), option.hint ? 'items-start' : 'items-center']"
              :data-testid="testid ? `${testid}-option-${option.value}` : undefined"
              :disabled="option.disabled"
              @mousedown.prevent
              @click="choose(option)"
              @mousemove="active = index"
            >
              <component :is="option.icon" v-if="option.icon" class="size-4 shrink-0 text-text-2" :class="option.hint ? 'mt-px' : ''" />
              <span class="flex min-w-0 flex-1 flex-col gap-0.5">
                <span class="truncate">{{ option.label }}</span>
                <!-- w-0 min-w-full: a percentage min-width contributes nothing to
                     intrinsic sizing, so a long hint truncates instead of stretching
                     the popover to the viewport. -->
                <span v-if="option.hint" class="w-0 min-w-full truncate font-mono text-[10.5px] text-text-4">{{ option.hint }}</span>
              </span>
              <IconCheck v-if="option.value === modelValue" class="size-3.5 shrink-0 text-accent" :class="option.hint ? 'mt-px' : ''" :stroke-width="3" />
            </button>
          </li>
        </ul>
        <div v-else class="px-3 py-4 text-center text-[12.5px] text-text-4" :data-testid="testid ? `${testid}-empty` : undefined">
          {{ searchable && query.trim() ? 'No matches' : 'No options' }}
        </div>
      </div>
    </Teleport>
  </div>
</template>
