<script setup lang="ts">
// A styled monospace <textarea> for authoring node JS. This is the v1
// contract's documented CodeMirror fallback (min-release-age blocks the
// codemirror/@codemirror/lang-javascript deps) — the component boundary
// (props/emits below) is kept clean so CodeMirror can replace the internals
// later without any caller ever needing to change.
const props = defineProps<{
  modelValue: string
  label?: string
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
  rows?: number
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
}

// Tab inserts two spaces instead of moving focus off the field — a nice-to
// have for a plain textarea with no syntax awareness.
function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Tab') return
  e.preventDefault()
  const el = e.target as HTMLTextAreaElement
  const start = el.selectionStart
  const end = el.selectionEnd
  const value = el.value
  const next = `${value.slice(0, start)}  ${value.slice(end)}`
  el.value = next
  el.selectionStart = el.selectionEnd = start + 2
  emit('update:modelValue', next)
}
</script>

<template>
  <div>
    <div v-if="label" class="mb-1.5 text-[12.5px] text-text-2">{{ label }}</div>
    <textarea
      :value="modelValue"
      :rows="rows ?? 8"
      :placeholder="placeholder"
      spellcheck="false"
      class="w-full resize-y rounded-lg border border-strong bg-app px-3 py-2.5 font-mono text-[12.5px] leading-relaxed text-text outline-none placeholder:text-text-4 focus:border-accent"
      :data-testid="testid"
      @input="onInput"
      @keydown="onKeydown"
    />
    <p v-if="error" class="mt-1.5 text-xs leading-relaxed text-kind-issue" :data-testid="testid ? `${testid}-error` : undefined">{{ error }}</p>
    <p v-else-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4" :data-testid="testid ? `${testid}-hint` : undefined">{{ hint }}</p>
  </div>
</template>
