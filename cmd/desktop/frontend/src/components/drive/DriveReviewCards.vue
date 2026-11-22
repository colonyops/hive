<script setup lang="ts">
// The comment cards. Aligned, as on the Drive page, each card sits on the row
// of the text it quotes and slides down only to clear the card above.
// Stacked, as over the narrow pane, they follow one another.
import { computed, ref } from 'vue'
import CommentCard from '@hive/ui/components/review/CommentCard.vue'
import CommentEntry from '@hive/ui/components/review/CommentEntry.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import InlineConfirm from '@hive/ui/components/ui/InlineConfirm.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useAlignedCards } from '@hive/ui/composables/useAlignedCards'
import { relativeAge } from '../../lib/age'
import { labelOf } from '../../lib/reviewLabels'
import { useReviewLabels } from '../../stores/useReviewLabels'
import { useDriveReview } from '../../stores/useDriveReview'
import type { CommentWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  space: string
  path: string
  /** The element holding the reader, whose marked blocks the cards line up with; null stacks them. */
  alignTo?: HTMLElement | null
  /**
   * The reader's testid, whose `-document` holds the marked blocks. Derives `-review-card-<n>`,
   * `-review-edit-<n>`, `-review-delete-<n>`, `-review-stale`, `-review-discard`, and `-review-error`.
   */
  testid: string
}>()

const store = useDriveReview()
const { labels } = useReviewLabels()
const review = computed(() => store.review(props.space, props.path))
const comments = computed(() => review.value.data.value.comments ?? [])
const current = computed(() => review.value.data.value.current)

const error = ref<string | null>(null)
const editing = ref<string | null>(null)
const discarding = ref(false)
const clearing = ref(false)

function lines(comment: CommentWire): string {
  return comment.startLine === comment.endLine ? `L${comment.startLine}` : `L${comment.startLine}–${comment.endLine}`
}

function age(comment: CommentWire): string {
  const created = Date.parse(comment.createdAt)
  return Number.isNaN(created) ? '' : relativeAge(created)
}

// The comment's first highlighted line, else its first marked block.
function block(id: string): HTMLElement | null {
  const root = `[data-testid="${props.testid}-document"]`
  const escaped = CSS.escape(id)
  return (
    document.querySelector<HTMLElement>(`${root} [data-review-anchor="${escaped}"]`) ??
    document.querySelector<HTMLElement>(`${root} [data-review-ids~="${escaped}"]`)
  )
}

function pick(comment: CommentWire): void {
  store.setActive(comment.id)
  block(comment.id)?.scrollIntoView({ block: 'center', behavior: 'smooth' })
}

async function saveEdit(id: string, text: string): Promise<void> {
  error.value = await store.update(props.space, props.path, id, text)
  if (!error.value) editing.value = null
}

async function remove(id: string): Promise<void> {
  error.value = await store.remove(props.space, props.path, id)
}

async function discard(): Promise<void> {
  clearing.value = true
  error.value = await store.clear(props.space, props.path)
  clearing.value = false
  if (!error.value) discarding.value = false
}

const cards = ref<HTMLElement | null>(null)
const aligned = useAlignedCards({
  cards,
  ids: () => comments.value.map((c) => c.id),
  anchor: block,
  alignTo: () => props.alignTo ?? null,
  deps: [editing, store.active],
})
</script>

<template>
  <div class="flex flex-col gap-2 px-4 py-2">
    <div
      v-if="!current"
      class="flex flex-col gap-1.5 rounded-lg bg-severity-warning-tint px-3 py-2.5 text-small text-text-2"
      :data-testid="`${testid}-review-stale`"
    >
      These comments are on an earlier version of this document.
      <InlineConfirm
        v-if="discarding"
        class="-mx-3 -mb-2.5 rounded-b-lg"
        title="Discard these comments?"
        description="The comments and the note are deleted."
        confirm-label="Discard"
        :busy="clearing"
        :testid="`${testid}-review-confirm`"
        @confirm="discard"
        @cancel="discarding = false"
      />
      <BaseButton
        v-else
        variant="secondary"
        size="xs"
        class="self-start"
        :data-testid="`${testid}-review-discard`"
        @click="discarding = true"
      >
        Discard
      </BaseButton>
    </div>
    <InlineError v-if="error" variant="line" :message="error" :testid="`${testid}-review-error`" />

    <ol
      ref="cards"
      class="flex flex-col"
      :class="alignTo ? 'relative' : 'gap-2'"
      :style="alignTo ? { minHeight: `${aligned.height.value}px` } : undefined"
    >
      <CommentCard
        v-for="(comment, index) in comments"
        :key="comment.id"
        :ref="(card) => aligned.cardRef(comment.id, card)"
        :aligned="!!alignTo"
        :top="aligned.tops.value.get(comment.id)"
        :sliding="aligned.sliding.value.has(comment.id)"
        :active="store.active.value === comment.id"
        :hovered="store.hovered.value === comment.id"
        :data-testid="`${testid}-review-card-${index + 1}`"
        @select="pick(comment)"
        @hover="store.setHovered($event ? comment.id : null)"
      >
        <CommentEntry
          author="You"
          :age="age(comment)"
          :aside="lines(comment)"
          :aside-title="comment.context"
          :text="comment.text"
          :editing="editing === comment.id"
          can-edit
          can-delete
          :active="store.active.value === comment.id"
          :testids="{
            edit: `${testid}-review-edit-${index + 1}`,
            delete: `${testid}-review-delete-${index + 1}`,
            editText: `${testid}-review-edit-text-${index + 1}`,
            editSave: `${testid}-review-edit-save-${index + 1}`,
          }"
          @edit="editing = comment.id"
          @cancel="editing = null"
          @save="saveEdit(comment.id, $event)"
          @delete="remove(comment.id)"
        >
          <template #badge>
            <span
              class="flex size-4.5 shrink-0 items-center justify-center rounded-full bg-accent font-mono text-micro font-semibold text-accent-contrast"
              >{{ index + 1 }}</span
            >
          </template>
          <template v-if="labelOf(labels, comment.text)" #prefix>
            <component
              :is="labelOf(labels, comment.text)!.icon"
              class="mt-0.75 size-3.5 shrink-0 text-accent"
              aria-hidden="true"
            />
          </template>
        </CommentEntry>
      </CommentCard>
    </ol>
  </div>
</template>
