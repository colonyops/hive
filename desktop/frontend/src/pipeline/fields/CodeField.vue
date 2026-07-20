<script setup lang="ts">
// The 8b code editor: a transparent <textarea> (caret + editing) layered
// exactly over a tokenized <pre> (lib/highlightCode.ts) so the code reads as
// syntax-colored while staying a plain, fully-editable textarea underneath —
// this IS the documented CodeMirror fallback (min-release-age blocks the
// codemirror/@codemirror/lang-javascript deps), not a stopgap for one. The
// component boundary (props/emits below) is kept clean so CodeMirror could
// replace the internals later without any caller ever needing to change.
//
// A line-number gutter sits to the left. All three layers (gutter/pre/
// textarea) share the same font/size/line-height/padding so the caret lines
// up with the rendered tokens; scrolling the textarea (by wheel or by
// typing past the visible area) re-syncs the other two's scrollTop/
// scrollLeft in onScroll/onInput below.
import { computed, ref } from 'vue'
import { codeLineCount, highlightCode } from '../lib/highlightCode'

const props = defineProps<{
  modelValue: string
  label?: string
  placeholder?: string
  hint?: string
  error?: string
  testid?: string
  /** Visible line count — sizes the code box's fixed height. Defaults to 8. */
  rows?: number
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const LINE_HEIGHT_PX = 20
const PADDING_BLOCK_PX = 24 // 12px top + 12px bottom, matched in the scoped CSS below

const shellHeight = computed(() => (props.rows ?? 8) * LINE_HEIGHT_PX + PADDING_BLOCK_PX)
const lineNumbers = computed(() => Array.from({ length: codeLineCount(props.modelValue) }, (_, i) => i + 1))
const highlightedHtml = computed(() => highlightCode(props.modelValue))

const gutterRef = ref<HTMLElement | null>(null)
const preRef = ref<HTMLElement | null>(null)

function syncScroll(el: HTMLTextAreaElement) {
  if (preRef.value) {
    preRef.value.scrollTop = el.scrollTop
    preRef.value.scrollLeft = el.scrollLeft
  }
  if (gutterRef.value) gutterRef.value.scrollTop = el.scrollTop
}

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  emit('update:modelValue', el.value)
  syncScroll(el)
}

function onScroll(e: Event) {
  syncScroll(e.target as HTMLTextAreaElement)
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
  syncScroll(el)
}
</script>

<template>
  <div>
    <div v-if="label" class="mb-1.5 text-[12.5px] text-text-2">{{ label }}</div>
    <div class="hv-code-shell flex overflow-hidden rounded-[9px] border border-row bg-app" :style="{ height: `${shellHeight}px` }">
      <div ref="gutterRef" class="hv-code-gutter shrink-0 select-none overflow-hidden text-right text-text-4" :data-testid="testid ? `${testid}-gutter` : undefined">
        <div v-for="n in lineNumbers" :key="n" class="hv-code-line">{{ n }}</div>
      </div>
      <div class="relative min-w-0 flex-1">
        <pre ref="preRef" class="hv-code-pre hv-code-layer pointer-events-none m-0 overflow-hidden text-text-2" :data-testid="testid ? `${testid}-pre` : undefined" v-html="highlightedHtml" />
        <textarea
          :value="modelValue"
          :placeholder="placeholder"
          spellcheck="false"
          class="hv-code-textarea hv-code-layer absolute inset-0 resize-none overflow-auto outline-none"
          :data-testid="testid"
          @input="onInput"
          @keydown="onKeydown"
          @scroll="onScroll"
        />
      </div>
    </div>
    <p v-if="error" class="mt-1.5 text-xs leading-relaxed text-kind-issue" :data-testid="testid ? `${testid}-error` : undefined">{{ error }}</p>
    <p v-else-if="hint" class="mt-1.5 text-xs leading-relaxed text-text-4" :data-testid="testid ? `${testid}-hint` : undefined">{{ hint }}</p>
  </div>
</template>

<style scoped>
.hv-code-layer {
  padding: 12px 14px;
  font-family: var(--font-mono);
  font-size: 12.5px;
  line-height: 20px;
  white-space: pre;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  box-sizing: border-box;
}

.hv-code-pre {
  color: var(--color-text-2);
}

.hv-code-pre :deep(.hv-code-key) {
  color: var(--color-code-key);
}

.hv-code-pre :deep(.hv-code-string) {
  color: var(--color-code-string);
}

.hv-code-pre :deep(.hv-code-comment) {
  color: var(--color-code-comment);
}

.hv-code-textarea {
  border: none;
  background: transparent;
  color: transparent;
  caret-color: var(--color-text);
}

.hv-code-textarea::placeholder {
  color: var(--color-text-4);
}

.hv-code-gutter {
  padding: 12px 10px;
  font-family: var(--font-mono);
  font-size: 12.5px;
  line-height: 20px;
  background: var(--color-raised);
  border-right: 1px solid var(--color-row);
}

.hv-code-line {
  height: 20px;
}
</style>
