<script setup lang="ts">
// The comment cards. Aligned, as on the Drive page, each card sits on the row
// of the block it comments on and slides down only to clear the card above.
// Stacked, as over the narrow pane, they follow one another. The highlighted
// block is the only marker in the document, so the active card is what ties
// the two together: it draws an outline, a connector to the margin, and its
// actions.
import { useMutationObserver, useResizeObserver } from '@vueuse/core'
import { computed, nextTick, ref, watch } from 'vue'
import BaseButton from '../ui/BaseButton.vue'
import InlineConfirm from '../ui/InlineConfirm.vue'
import InlineError from '../ui/InlineError.vue'
import TextArea from '../ui/TextArea.vue'
import { relativeAge } from '../../lib/age'
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
const review = computed(() => store.review(props.space, props.path))
const comments = computed(() => review.value.data.value.comments ?? [])
const current = computed(() => review.value.data.value.current)

const error = ref<string | null>(null)
const editing = ref<string | null>(null)
const draft = ref('')
const discarding = ref(false)
const clearing = ref(false)

function lines(comment: CommentWire): string {
  return comment.startLine === comment.endLine ? `L${comment.startLine}` : `L${comment.startLine}–${comment.endLine}`
}

function age(comment: CommentWire): string {
  const created = Date.parse(comment.createdAt)
  return Number.isNaN(created) ? '' : relativeAge(created)
}

function block(id: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    `[data-testid="${props.testid}-document"] [data-review-ids~="${CSS.escape(id)}"]`,
  )
}

function pick(comment: CommentWire): void {
  store.setActive(comment.id)
  block(comment.id)?.scrollIntoView({ block: 'center', behavior: 'smooth' })
}

function startEdit(comment: CommentWire): void {
  editing.value = comment.id
  draft.value = comment.text
}

async function saveEdit(id: string): Promise<void> {
  error.value = await store.update(props.space, props.path, id, draft.value)
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

// A card's wanted top is its block's, measured against the cards' own box,
// which scrolls with the document, so scrolling never moves them.
const cards = ref<HTMLElement | null>(null)
const cardElements = new Map<string, HTMLElement>()
const tops = ref<ReadonlyMap<string, number>>(new Map())
const height = ref(0)
const CARD_GAP = 8

function cardRef(id: string, element: unknown): void {
  if (element instanceof HTMLElement) cardElements.set(id, element)
  else cardElements.delete(id)
}

function layout(): void {
  const box = cards.value
  if (!props.alignTo || !box) return
  const base = box.getBoundingClientRect().top
  const next = new Map<string, number>()
  let floor = 0
  for (const comment of comments.value) {
    const target = block(comment.id)
    const wanted = target ? target.getBoundingClientRect().top - base : floor
    const top = Math.max(wanted, floor)
    next.set(comment.id, top)
    floor = top + (cardElements.get(comment.id)?.offsetHeight ?? 0) + CARD_GAP
  }
  tops.value = next
  height.value = floor
}

function relayout(): void {
  void nextTick(layout)
}

watch([comments, editing, store.active, () => props.alignTo], relayout, {
  immediate: true,
  flush: 'post',
})
useResizeObserver(() => props.alignTo ?? null, relayout)
useResizeObserver(cards, relayout)
// The reader marks blocks after it renders, and a highlighter that finishes
// loading renders again.
useMutationObserver(() => props.alignTo ?? null, relayout, {
  subtree: true,
  childList: true,
  attributes: true,
  attributeFilter: ['data-review-ids'],
})

function cardStyle(id: string): Record<string, string> | undefined {
  return props.alignTo ? { top: `${tops.value.get(id) ?? 0}px` } : undefined
}
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
      :style="alignTo ? { minHeight: `${height}px` } : undefined"
    >
      <li
        v-for="(comment, index) in comments"
        :key="comment.id"
        :ref="(element) => cardRef(comment.id, element)"
        class="flex cursor-pointer flex-col gap-2 rounded-lg border px-3.5 py-3"
        :class="[
          alignTo ? 'absolute inset-x-0' : 'relative',
          'transition-[top,border-color,background-color] duration-150',
          store.active.value === comment.id
            ? 'border-accent/45 bg-raised'
            : store.hovered.value === comment.id
              ? 'border-strong bg-raised'
              : 'border-card bg-raised/60',
        ]"
        :style="cardStyle(comment.id)"
        :data-testid="`${testid}-review-card-${index + 1}`"
        @click="pick(comment)"
        @mouseenter="store.setHovered(comment.id)"
        @mouseleave="store.setHovered(null)"
      >
        <span
          v-if="alignTo && store.active.value === comment.id"
          class="absolute top-[22px] -left-4 h-px w-3 bg-accent/50"
          aria-hidden="true"
        />
        <div class="flex items-center gap-2">
          <span
            class="flex size-[18px] shrink-0 items-center justify-center rounded-full bg-accent font-mono text-micro font-semibold text-accent-contrast"
            >{{ index + 1 }}</span
          >
          <span class="text-small font-semibold text-text">You</span>
          <span class="text-caption text-text-4">{{ age(comment) }}</span>
          <span class="ml-auto font-mono text-micro text-text-4" :title="comment.context">{{ lines(comment) }}</span>
        </div>
        <div v-if="editing === comment.id" class="flex flex-col gap-1.5" @click.stop>
          <TextArea v-model="draft" :rows="2" size="sm" :data-testid="`${testid}-review-edit-text-${index + 1}`" />
          <div class="flex justify-end gap-2">
            <BaseButton variant="ghost" size="xs" @click="editing = null">Cancel</BaseButton>
            <BaseButton
              size="xs"
              :disabled="!draft.trim()"
              :data-testid="`${testid}-review-edit-save-${index + 1}`"
              @click="saveEdit(comment.id)"
            >
              Save
            </BaseButton>
          </div>
        </div>
        <p
          v-else
          class="text-small leading-normal break-words whitespace-pre-wrap"
          :class="store.active.value === comment.id ? 'text-text' : 'text-text-2'"
        >
          {{ comment.text }}
        </p>
        <div
          v-if="store.active.value === comment.id && editing !== comment.id"
          class="flex gap-3.5 pt-0.5 text-caption text-text-3"
          @click.stop
        >
          <button
            type="button"
            class="-mx-1 cursor-pointer rounded px-1 hover:bg-chip hover:text-text"
            :data-testid="`${testid}-review-edit-${index + 1}`"
            @click="startEdit(comment)"
          >
            Edit
          </button>
          <button
            type="button"
            class="-mx-1 ml-auto cursor-pointer rounded px-1 hover:bg-severity-error-tint hover:text-severity-error"
            :data-testid="`${testid}-review-delete-${index + 1}`"
            @click="remove(comment.id)"
          >
            Delete
          </button>
        </div>
      </li>
    </ol>
  </div>
</template>
