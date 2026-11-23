<script setup lang="ts">
// The comment composer a text selection opens. It does not take focus on
// open, so the selection stays live for a copy. Instead the keyboard reaches
// it while nothing else has focus: a printable key starts the comment with
// that key, unless `claimKey` takes it first.
import { useEventListener } from '@vueuse/core'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import IconGripHorizontal from '~icons/lucide/grip-horizontal'
import { startDrag } from '../../composables/useDragGesture'
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import type { HighlightBox, SourceRange } from '../../lib/reviewSelection'
import BaseButton from '../ui/BaseButton.vue'
import InlineError from '../ui/InlineError.vue'

const props = withDefaults(
  defineProps<{
    /** The selection's box in the viewport. */
    rect: HighlightBox
    range: SourceRange
    /** What the header calls the lines, when `L12–14` alone would not say where. */
    label?: string
    busy: boolean
    error: string | null
    placeholder?: string
    saveLabel?: string
    /** The save shortcut as the platform spells it, shown beside the buttons. */
    saveKey?: string
    /** Takes a printable key before it starts the comment; return true to keep it. */
    claimKey?: (key: string) => boolean
    /** Derives `-comment-popover`, `-comment-grip`, `-comment-text`, `-comment-save`, `-comment-cancel`, `-comment-key`, and `-comment-error`. */
    testid: string
  }>(),
  { label: undefined, placeholder: 'Type to comment…', saveLabel: 'Add Comment', saveKey: '', claimKey: undefined },
)
const emit = defineEmits<{ save: [text: string]; cancel: [] }>()

const model = defineModel<string>('text', { default: '' })
// A local copy, so keys typed faster than the host re-renders all land.
const draft = ref('')
watch(model, (value) => (draft.value = value), { immediate: true })
watch(draft, (value) => (model.value = value))
const popover = ref<HTMLElement | null>(null)
const field = ref<HTMLTextAreaElement | null>(null)
const position = ref({ top: 0, left: 0 })

const GAP = 8
const EDGE = 8
const WIDTH = 360

function clamp(top: number, left: number): { top: number; left: number } {
  const height = popover.value?.offsetHeight ?? 0
  return {
    top: Math.min(Math.max(EDGE, top), window.innerHeight - EDGE - height),
    left: Math.min(Math.max(EDGE, left), window.innerWidth - EDGE - WIDTH),
  }
}

function place(): void {
  const height = popover.value?.offsetHeight ?? 0
  const below = props.rect.top + props.rect.height + GAP
  const top = below + height > window.innerHeight - EDGE ? props.rect.top - GAP - height : below
  position.value = clamp(top, props.rect.left)
}

onMounted(async () => {
  place()
  await nextTick()
  place()
})
watch(() => props.range, place)
useEventListener(window, 'resize', () => {
  position.value = clamp(position.value.top, position.value.left)
})

function startMove(event: PointerEvent): void {
  if (event.button !== 0 || (event.target as Element).closest('button')) return
  event.preventDefault()
  const origin = position.value
  startDrag(event, {
    onMove: (move: PointerEvent) => {
      position.value = clamp(origin.top + move.clientY - event.clientY, origin.left + move.clientX - event.clientX)
    },
  })
}

const lines = computed(
  () =>
    props.label ??
    (props.range.startLine === props.range.endLine
      ? `L${props.range.startLine}`
      : `L${props.range.startLine}–${props.range.endLine}`),
)

useEscapeToClose(() => emit('cancel'))

function save(): void {
  if (draft.value.trim() && !props.busy) emit('save', draft.value)
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    save()
  }
}

function editable(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    (target instanceof HTMLElement && target.isContentEditable)
  )
}

// Captured, so a key meant for the composer does not also reach the app's
// bare-key bindings.
useEventListener(
  document,
  'keydown',
  (event: KeyboardEvent) => {
    if (event.metaKey || event.ctrlKey || event.altKey || editable(event.target)) return
    if (event.key.length !== 1 || event.key === ' ') return
    event.preventDefault()
    event.stopPropagation()
    if (props.claimKey?.(event.key)) return
    draft.value += event.key
    field.value?.focus()
  },
  { capture: true },
)

defineExpose({ contains: (node: Node | null) => !!node && !!popover.value?.contains(node) })
</script>

<template>
  <Teleport to="body">
    <div
      ref="popover"
      class="fixed z-50 flex w-[360px] flex-col gap-2.5 rounded-xl border border-card bg-raised p-3 shadow-popover"
      :style="{ top: `${position.top}px`, left: `${position.left}px` }"
      role="dialog"
      aria-label="Add a comment"
      :data-testid="`${testid}-comment-popover`"
    >
      <div
        class="-mx-3 -mt-3 flex min-w-0 cursor-grab touch-none items-center gap-2 rounded-t-xl px-3 pt-3 pb-0.5 text-caption text-text-4 select-none active:cursor-grabbing"
        title="Drag to move"
        :data-testid="`${testid}-comment-grip`"
        @pointerdown="startMove"
      >
        <IconGripHorizontal class="size-3.5 shrink-0" aria-hidden="true" />
        <span class="shrink-0 font-mono text-micro">{{ lines }}</span>
        <span class="min-w-0 truncate text-text-2">“{{ range.context }}”</span>
        <span class="ml-auto shrink-0">Esc</span>
      </div>
      <textarea
        ref="field"
        v-model="draft"
        rows="3"
        :placeholder="placeholder"
        class="w-full resize-none rounded-lg border border-strong bg-app px-2.75 py-2.25 text-body text-text outline-none placeholder:text-text-4 focus:border-accent/60 focus:ring-3 focus:ring-accent/12"
        :data-testid="`${testid}-comment-text`"
        @keydown="onKeydown"
      />
      <slot name="toolbar" />
      <InlineError v-if="error" variant="line" :message="error" :testid="`${testid}-comment-error`" />
      <div class="flex items-center gap-2">
        <span v-if="saveKey" class="ml-auto font-mono text-micro text-text-4" :data-testid="`${testid}-comment-key`">{{
          saveKey
        }}</span>
        <BaseButton
          variant="ghost"
          size="xs"
          :class="{ 'ml-auto': !saveKey }"
          :data-testid="`${testid}-comment-cancel`"
          @click="emit('cancel')"
        >
          Cancel
        </BaseButton>
        <BaseButton
          size="xs"
          :busy="busy"
          :disabled="!draft.trim()"
          :data-testid="`${testid}-comment-save`"
          @click="save"
        >
          {{ saveLabel }}
        </BaseButton>
      </div>
    </div>
  </Teleport>
</template>
