<script setup lang="ts">
// The comment form a text selection opens in the reader, anchored to the
// Comment button that opened it.
import { computed, nextTick, onMounted, ref } from 'vue'
import BaseButton from '../ui/BaseButton.vue'
import InlineError from '../ui/InlineError.vue'
import TextArea from '../ui/TextArea.vue'
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

const text = ref('')
const popover = ref<HTMLElement | null>(null)
const input = ref<{ focus: () => void } | null>(null)
const open = ref(true)
const anchorRef = computed(() => props.anchor)
const { style, measure } = useAnchoredPopover(anchorRef, popover, open)

const lines = computed(() =>
  props.range.startLine === props.range.endLine
    ? `Line ${props.range.startLine}`
    : `Lines ${props.range.startLine}-${props.range.endLine}`,
)

onMounted(async () => {
  measure()
  await nextTick()
  measure()
  input.value?.focus()
})

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
</script>

<template>
  <Teleport to="body">
    <div
      ref="popover"
      class="fixed z-50 flex w-80 flex-col gap-2 rounded-lg border border-card bg-raised p-3 shadow-popover"
      :style="style"
      role="dialog"
      aria-label="Comment"
      :data-testid="`${testid}-comment-popover`"
    >
      <span class="font-mono text-caption text-text-4">{{ lines }}</span>
      <TextArea
        ref="input"
        v-model="text"
        :rows="3"
        size="sm"
        placeholder="Comment"
        :data-testid="`${testid}-comment-text`"
        @keydown="onKeydown"
      />
      <InlineError v-if="error" variant="line" :message="error" :testid="`${testid}-comment-error`" />
      <div class="flex justify-end gap-2">
        <BaseButton variant="ghost" size="xs" :data-testid="`${testid}-comment-cancel`" @click="emit('cancel')">
          Cancel
        </BaseButton>
        <BaseButton
          size="xs"
          :busy="busy"
          :disabled="!text.trim()"
          :data-testid="`${testid}-comment-save`"
          @click="save"
        >
          Save
        </BaseButton>
      </div>
    </div>
  </Teleport>
</template>
