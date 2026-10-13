<script setup lang="ts">
// The comment form a text selection opens, anchored under the selection. It
// does not take focus on open: the selection stays live, so the user can
// still copy it, and a click into the field starts the comment.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import BaseButton from '../ui/BaseButton.vue'
import InlineError from '../ui/InlineError.vue'
import { useAnchoredPopover } from '../../composables/useAnchoredPopover'
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import type { SourceRange } from '../../lib/driveReviewSelection'

const props = defineProps<{
  anchor: HTMLElement | null
  range: SourceRange
  busy: boolean
  error: string | null
  /** Derives `-comment-popover`, `-comment-text`, `-comment-save`, and `-comment-cancel`. */
  testid: string
}>()
const emit = defineEmits<{ save: [text: string]; cancel: [] }>()

const text = defineModel<string>('text', { default: '' })
const popover = ref<HTMLElement | null>(null)
const open = ref(true)
const anchorRef = computed(() => props.anchor)
const { style, measure } = useAnchoredPopover(anchorRef, popover, open)

const lines = computed(() =>
  props.range.startLine === props.range.endLine
    ? `L${props.range.startLine}`
    : `L${props.range.startLine}–${props.range.endLine}`,
)

async function place(): Promise<void> {
  measure()
  await nextTick()
  measure()
}
onMounted(place)
watch(() => props.range, place)

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
      :style="style"
      role="dialog"
      aria-label="Add a comment"
      :data-testid="`${testid}-comment-popover`"
    >
      <div class="flex min-w-0 items-center gap-2 text-caption text-text-4">
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
          Add comment
        </BaseButton>
      </div>
    </div>
  </Teleport>
</template>
