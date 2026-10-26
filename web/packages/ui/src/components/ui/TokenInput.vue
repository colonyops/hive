<script setup lang="ts">
// A typed value that matches a suggestion ignoring case takes the
// suggestion's spelling, so a list never holds "pr" next to "PR".
import { computed, ref } from 'vue'
import IconX from '~icons/lucide/x'
import IconCornerDownLeft from '~icons/lucide/corner-down-left'
import { useListKeyboardNav } from '../../composables/useListKeyboardNav'
import AppTooltip from './AppTooltip.vue'
import IconButton from './IconButton.vue'

const props = withDefaults(
  defineProps<{
    modelValue: string[] | null
    suggestions?: string[]
    placeholder?: string
    monospace?: boolean
    // Returns a non-empty explanation for a token to draw as flagged.
    flag?: (token: string) => string
    id?: string
    ariaLabel?: string
    testid?: string
  }>(),
  {
    suggestions: () => [],
    placeholder: 'Add…',
    monospace: false,
    flag: undefined,
    id: undefined,
    ariaLabel: undefined,
    testid: 'token-input',
  },
)
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()

const input = ref<HTMLInputElement | null>(null)
const draft = ref('')
const open = ref(false)
const active = ref(0)

const tokens = computed(() => props.modelValue ?? [])
const canonical = (value: string): string | undefined =>
  props.suggestions.find((suggestion) => suggestion.toLowerCase() === value.toLowerCase())
const flagged = (token: string): string => props.flag?.(token) ?? ''
const options = computed(() => {
  const selected = new Set(tokens.value.map((token) => token.toLowerCase()))
  const query = draft.value.trim().toLowerCase()
  return props.suggestions.filter(
    (suggestion) => !selected.has(suggestion.toLowerCase()) && (!query || suggestion.toLowerCase().includes(query)),
  )
})
function highlight(label: string): { pre: string; mid: string; post: string } {
  const query = draft.value.trim()
  const index = query ? label.toLowerCase().indexOf(query.toLowerCase()) : -1
  if (index < 0) return { pre: label, mid: '', post: '' }
  return {
    pre: label.slice(0, index),
    mid: label.slice(index, index + query.length),
    post: label.slice(index + query.length),
  }
}

function add(raw?: string): void {
  const value = (raw ?? draft.value).trim().replace(/,+$/, '').trim()
  draft.value = ''
  open.value = false
  if (!value) return
  const normalized = canonical(value) ?? value
  if (!tokens.value.some((token) => token.toLowerCase() === normalized.toLowerCase()))
    emit('update:modelValue', [...tokens.value, normalized])
}
function remove(token: string): void {
  emit(
    'update:modelValue',
    tokens.value.filter((item) => item !== token),
  )
}
function onInput(): void {
  open.value = true
  active.value = 0
}
const nav = useListKeyboardNav({ active, count: () => options.value.length, wrap: false })
function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'ArrowDown') open.value = true
  if (nav.onKeydown(event)) return
  if (event.key === 'Enter' || event.key === ',') {
    event.preventDefault()
    const pick = open.value ? options.value[active.value] : undefined
    add(pick)
  } else if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    event.stopPropagation()
    open.value = false
  } else if (event.key === 'Backspace' && !draft.value && tokens.value.length) {
    remove(tokens.value[tokens.value.length - 1])
  }
}
function onBlur(): void {
  open.value = false
  add()
}

// A form calls flush on submit so a typed but uncommitted token is kept.
defineExpose({ flush: () => add(), focus: () => input.value?.focus() })
</script>

<template>
  <div class="relative">
    <div
      class="flex flex-wrap items-center gap-1.5 rounded border bg-app px-2 py-1.5"
      :class="open ? 'border-accent' : 'border-border'"
      :data-testid="`${testid}-tokens`"
      @click="input?.focus()"
    >
      <span
        v-for="token in tokens"
        :key="token"
        class="inline-flex items-center gap-1 rounded-md border py-0.5 pl-2 pr-1 text-small"
        :class="[
          monospace ? 'font-mono' : '',
          flagged(token)
            ? 'border-dashed border-accent/60 bg-accent-tint text-accent'
            : 'border-strong bg-chip text-text',
        ]"
        :data-testid="`${testid}-token-${token}`"
      >
        <AppTooltip :text="flagged(token)">
          <span>{{ token }}</span>
        </AppTooltip>
        <IconButton :label="`Remove ${token}`" :icon="IconX" size="sm" tone="danger" @click.stop="remove(token)" />
      </span>
      <input
        :id="id"
        ref="input"
        v-model="draft"
        :data-testid="testid"
        :placeholder="placeholder"
        :aria-label="ariaLabel"
        class="min-w-[80px] flex-1 bg-transparent text-small text-text outline-none"
        :class="monospace ? 'font-mono' : ''"
        role="combobox"
        :aria-expanded="open"
        @focus="onInput"
        @input="onInput"
        @keydown="onKeydown"
        @blur="onBlur"
      />
    </div>

    <ul
      v-if="open && options.length"
      class="absolute inset-x-0 top-[calc(100%+6px)] z-20 flex max-h-60 flex-col gap-0.5 overflow-y-auto rounded-lg border border-card bg-raised p-[5px] shadow-popover"
      role="listbox"
      :data-testid="`${testid}-suggestions`"
    >
      <li v-for="(option, index) in options" :key="option" role="option" :aria-selected="index === active">
        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-md px-[9px] py-[7px] text-left"
          :class="index === active ? 'bg-hover' : ''"
          :data-testid="`${testid}-option-${option}`"
          @mousedown.prevent="add(option)"
          @mousemove="active = index"
        >
          <span class="text-small text-text-2" :class="monospace ? 'font-mono' : ''"
            ><span>{{ highlight(option).pre }}</span
            ><span class="text-accent">{{ highlight(option).mid }}</span
            ><span>{{ highlight(option).post }}</span></span
          >
          <span class="flex-1" />
          <IconCornerDownLeft v-if="index === active" class="size-3 text-text-4" />
        </button>
      </li>
    </ul>
  </div>
</template>
