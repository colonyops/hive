<script setup lang="ts">
// The shared comment popover with the review labels: a label's key, while
// nothing has focus, applies that label.
import { ref } from 'vue'
import CommentPopover from '@hive/ui/components/review/CommentPopover.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { HighlightBox, SourceRange } from '@hive/ui/lib/reviewSelection'
import { formatCombo } from '../../composables/useKeybindings'
import { labelText, type ReviewLabel } from '../../lib/reviewLabels'
import { useReviewLabels } from '../../stores/useReviewLabels'

export type SelectionRect = HighlightBox

const props = defineProps<{
  /** The selection's box in the viewport. */
  rect: SelectionRect
  range: SourceRange
  /** What the header calls the lines, when `L12–14` alone would not say where. */
  label?: string
  busy: boolean
  error: string | null
  /** Derives the shared popover's ids, plus `-comment-label-<key>`. */
  testid: string
}>()
const emit = defineEmits<{ save: [text: string]; cancel: [] }>()

const SAVE_KEY = formatCombo('mod+enter')

const text = defineModel<string>('text', { default: '' })
const inner = ref<{ contains: (node: Node | null) => boolean } | null>(null)

const reviewLabels = useReviewLabels()
const labels = reviewLabels.labels
void reviewLabels.reload()

function applyLabel(label: ReviewLabel): void {
  if (!props.busy) emit('save', labelText(label, text.value))
}

function claimKey(key: string): boolean {
  const label = labels.value.find((candidate) => candidate.key === key)
  if (!label) return false
  applyLabel(label)
  return true
}

defineExpose({ contains: (node: Node | null) => !!inner.value?.contains(node) })
</script>

<template>
  <CommentPopover
    ref="inner"
    v-model:text="text"
    :rect="rect"
    :range="range"
    :label="label"
    :busy="busy"
    :error="error"
    placeholder="Type to comment, or pick a label…"
    :save-key="SAVE_KEY"
    :claim-key="claimKey"
    :testid="testid"
    @save="emit('save', $event)"
    @cancel="emit('cancel')"
  >
    <template #toolbar>
      <div class="flex items-center gap-0.5" role="toolbar" aria-label="Labels">
        <IconButton
          v-for="label in labels"
          :key="label.key"
          :label="label.title"
          :tooltip="`${label.title} (${label.key})`"
          :icon="label.icon"
          size="md"
          :disabled="busy"
          :data-testid="`${testid}-comment-label-${label.key}`"
          @click="applyLabel(label)"
        />
      </div>
    </template>
  </CommentPopover>
</template>
