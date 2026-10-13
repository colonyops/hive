<script setup lang="ts">
// The comment composer a text selection opens. It floats over the document,
// placed under the selection (above it when there is no room below), and
// drags by its top row out of the way of what the user is reading. It does
// not take focus on open: the selection stays live for a copy, and a click
// into the field starts the comment.
import { useEventListener } from '@vueuse/core'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import IconGripHorizontal from '~icons/lucide/grip-horizontal'
import BaseButton from '../ui/BaseButton.vue'
import InlineError from '../ui/InlineError.vue'
import { startDrag } from '../../composables/useDragGesture'
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import type { SourceRange } from '../../lib/driveReviewSelection'

export type SelectionRect = { top: number; left: number; width: number; height: number }

const props = defineProps<{
  /** The selection's box in the viewport. */
  rect: SelectionRect
  range: SourceRange
  busy: boolean
  error: string | null
  /** Derives `-comment-popover`, `-comment-grip`, `-comment-text`, `-comment-save`, and `-comment-cancel`. */
  testid: string
}>()
const emit = defineEmits<{ save: [text: string]; cancel: [] }>()

const text = defineModel<string>('text', { default: '' })
const popover = ref<HTMLElement | null>(null)
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

const lines = computed(() =>
  props.range.startLine === props.range.endLine
    ? `L${props.range.startLine}`
    : `L${props.range.startLine}–${props.range.endLine}`,
)

useEscapeToClose(() => emit('cancel'))

function save(): void {
  if (text.value.trim() && !props.busy) emit('save', text.value)
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    save()
  }
}

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
        v-model="text"
        rows="3"
        placeholder="Ask or instruct the agent…"
        class="w-full resize-none rounded-lg border border-strong bg-app px-[11px] py-[9px] text-body text-text outline-none placeholder:text-text-4 focus:border-accent/60 focus:ring-3 focus:ring-accent/12"
        :data-testid="`${testid}-comment-text`"
        @keydown="onKeydown"
      />
      <InlineError v-if="error" variant="line" :message="error" :testid="`${testid}-comment-error`" />
      <div class="flex items-center gap-2.5">
        <span class="font-mono text-micro text-text-4">⌘↵ to add</span>
        <BaseButton
          variant="ghost"
          size="xs"
          class="ml-auto"
          :data-testid="`${testid}-comment-cancel`"
          @click="emit('cancel')"
        >
          Cancel
        </BaseButton>
        <BaseButton
          size="xs"
          :busy="busy"
          :disabled="!text.trim()"
          :data-testid="`${testid}-comment-save`"
          @click="save"
        >
          Add Comment
        </BaseButton>
      </div>
    </div>
  </Teleport>
</template>
