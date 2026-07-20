<script setup lang="ts">
// Token input for the feed-item types an action applies to. Suggests the
// kinds the system actually knows about (live feed-item kinds unioned with
// types already configured elsewhere) and flags anything that won't match a
// known kind. Matching is case-insensitive, so a picked/typed value is
// canonicalised to the known casing (e.g. "pr" -> "PR").
import { computed, ref } from 'vue'
import IconX from '~icons/lucide/x'
import IconCornerDownLeft from '~icons/lucide/corner-down-left'

const props = defineProps<{ modelValue: string[] | null; knownTypes: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()

const input = ref<HTMLInputElement | null>(null)
const draft = ref('')
const open = ref(false)
const active = ref(0)

const tags = computed(() => props.modelValue ?? [])
const canonical = (value: string): string | undefined => props.knownTypes.find((type) => type.toLowerCase() === value.toLowerCase())
const isKnown = (tag: string): boolean => !props.knownTypes.length || canonical(tag) !== undefined
const hasUnknown = computed(() => props.knownTypes.length > 0 && tags.value.some((tag) => !isKnown(tag)))
const suggestions = computed(() => {
  const selected = new Set(tags.value.map((tag) => tag.toLowerCase()))
  const query = draft.value.trim().toLowerCase()
  return props.knownTypes.filter((type) => !selected.has(type.toLowerCase()) && (!query || type.toLowerCase().includes(query)))
})
function highlight(label: string): { pre: string; mid: string; post: string } {
  const query = draft.value.trim()
  const index = query ? label.toLowerCase().indexOf(query.toLowerCase()) : -1
  if (index < 0) return { pre: label, mid: '', post: '' }
  return { pre: label.slice(0, index), mid: label.slice(index, index + query.length), post: label.slice(index + query.length) }
}

function add(raw?: string): void {
  const value = (raw ?? draft.value).trim().replace(/,+$/, '').trim()
  draft.value = ''
  open.value = false
  if (!value) return
  const normalized = canonical(value) ?? value
  if (!tags.value.some((tag) => tag.toLowerCase() === normalized.toLowerCase())) emit('update:modelValue', [...tags.value, normalized])
}
function remove(tag: string): void { emit('update:modelValue', tags.value.filter((item) => item !== tag)) }
function onInput(): void { open.value = true; active.value = 0 }
function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'ArrowDown') { event.preventDefault(); open.value = true; active.value = Math.min(active.value + 1, suggestions.value.length - 1) }
  else if (event.key === 'ArrowUp') { event.preventDefault(); active.value = Math.max(active.value - 1, 0) }
  else if (event.key === 'Enter' || event.key === ',') { event.preventDefault(); const pick = open.value ? suggestions.value[active.value] : undefined; add(pick) }
  else if (event.key === 'Escape' && open.value) { event.preventDefault(); event.stopPropagation(); open.value = false }
  else if (event.key === 'Backspace' && !draft.value && tags.value.length) { remove(tags.value[tags.value.length - 1]) }
}
function onBlur(): void { open.value = false; add() }

// The drawer commits the pending draft when Save is pressed.
defineExpose({ flush: () => add() })
</script>

<template>
  <div>
    <div class="text-xs text-text-2">Applies to <span class="text-text-4">(feed-item types)</span></div>
    <div class="relative mt-1">
      <div
        class="flex flex-wrap items-center gap-1.5 rounded border bg-app px-2 py-1.5"
        :class="open ? 'border-accent' : 'border-border'"
        data-testid="action-applies-to-tokens"
        @click="input?.focus()"
      >
        <span
          v-for="tag in tags"
          :key="tag"
          class="inline-flex items-center gap-1 rounded-md border py-0.5 pl-2 pr-1 font-mono text-[12px]"
          :class="isKnown(tag) ? 'border-strong bg-chip text-text' : 'border-dashed border-[rgba(245,158,11,0.6)] bg-accent-tint text-accent'"
          :title="isKnown(tag) ? undefined : 'Not a known feed-item type — it won\'t match any feed item'"
        >
          <span>{{ tag }}</span>
          <button type="button" class="flex size-4 items-center justify-center rounded text-text-3 hover:text-severity-error" :aria-label="`Remove ${tag}`" @click.stop="remove(tag)"><IconX class="size-2.5" /></button>
        </span>
        <input
          ref="input"
          v-model="draft"
          data-testid="action-applies-to"
          placeholder="Add type…"
          class="min-w-[80px] flex-1 bg-transparent font-mono text-[12.5px] text-text outline-none"
          role="combobox"
          :aria-expanded="open"
          @focus="onInput"
          @input="onInput"
          @keydown="onKeydown"
          @blur="onBlur"
        >
      </div>

      <ul
        v-if="open && suggestions.length"
        class="absolute inset-x-0 top-[calc(100%+6px)] z-20 flex flex-col gap-0.5 rounded-lg border border-card bg-raised p-[5px] shadow-[0_16px_34px_-12px_rgba(0,0,0,.6)]"
        role="listbox"
        data-testid="action-applies-to-suggestions"
      >
        <li v-for="(type, index) in suggestions" :key="type" role="option" :aria-selected="index === active">
          <button
            type="button"
            class="flex w-full items-center gap-2 rounded-md px-[9px] py-[7px] text-left"
            :class="index === active ? 'bg-hover' : ''"
            :data-testid="`action-applies-to-option-${type}`"
            @mousedown.prevent="add(type)"
            @mousemove="active = index"
          >
            <span class="font-mono text-[12.5px] text-text-2"><span>{{ highlight(type).pre }}</span><span class="text-accent">{{ highlight(type).mid }}</span><span>{{ highlight(type).post }}</span></span>
            <span class="flex-1" />
            <IconCornerDownLeft v-if="index === active" class="size-3 text-text-4" />
          </button>
        </li>
      </ul>
    </div>

    <p v-if="hasUnknown" class="mt-1.5 text-[11px] leading-relaxed text-accent">Highlighted types don't match any known feed item yet.</p>
  </div>
</template>
