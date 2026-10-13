<script setup lang="ts">
// The comments on one Markdown document, in a column beside it. Aligned, as
// on the Drive page, each card sits level with the block it comments on and
// slides down only to clear the card above. Stacked, as in the narrow pane,
// the cards follow one another.
import { useMutationObserver, useResizeObserver } from '@vueuse/core'
import { computed, nextTick, ref, watch } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconPencil from '~icons/lucide/pencil'
import IconSend from '~icons/lucide/send'
import IconTrash from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'
import SendToAgentDialog from './SendToAgentDialog.vue'
import BaseButton from '../ui/BaseButton.vue'
import IconButton from '../ui/IconButton.vue'
import InlineConfirm from '../ui/InlineConfirm.vue'
import InlineError from '../ui/InlineError.vue'
import TextArea from '../ui/TextArea.vue'
import { useClipboard } from '../../composables/useClipboard'
import { errorText } from '../../lib/appError'
import { useDriveReview } from '../../stores/useDriveReview'
import type { CommentWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { AgentTargetRef } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{
  space: string
  path: string
  /** The agent Send to Agent offers first. */
  agent?: AgentTargetRef | null
  /** The element holding the reader, whose marked blocks the cards line up with; null stacks them. */
  alignTo?: HTMLElement | null
  /** Shows a close button, for a sidebar that overlays the document. */
  closable?: boolean
  /**
   * The reader's testid, whose `-document` holds the marked blocks. Derives `-review`, `-review-card-<n>`,
   * `-review-edit-<n>`, `-review-delete-<n>`, `-review-note`, `-review-copy`, `-review-send`, `-review-clear`,
   * `-review-stale`, `-review-discard`, `-review-close`, and `-review-error`.
   */
  testid: string
}>()
const emit = defineEmits<{ close: [] }>()

const store = useDriveReview()
const review = computed(() => store.review(props.space, props.path))
const comments = computed(() => review.value.data.value.comments ?? [])
const current = computed(() => review.value.data.value.current)
const note = computed({
  get: () => store.note(props.space, props.path),
  set: (text: string) => store.setNote(props.space, props.path, text),
})

const error = ref<string | null>(null)
const editing = ref<string | null>(null)
const draft = ref('')
const confirming = ref<'clear' | 'discard' | null>(null)
const clearing = ref(false)
const sending = ref(false)

function lines(comment: CommentWire): string {
  return comment.startLine === comment.endLine ? `L${comment.startLine}` : `L${comment.startLine}-${comment.endLine}`
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

const { copy, setStatus, status: copyStatus } = useClipboard()
async function copyFeedback(): Promise<void> {
  try {
    await copy(await store.feedback(props.space, props.path))
    error.value = copyStatus.value === 'error' ? 'Could not copy to the clipboard.' : null
  } catch (err) {
    setStatus('error')
    error.value = errorText(err, 'Could not build the feedback.')
  }
}

async function clear(): Promise<void> {
  clearing.value = true
  error.value = await store.clear(props.space, props.path)
  clearing.value = false
  if (!error.value) confirming.value = null
}

// Alignment: a card's wanted top is its block's, measured against the cards'
// own box, which scrolls with the document, so scrolling never moves them.
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

watch([comments, editing, confirming, () => props.alignTo], relayout, { immediate: true, flush: 'post' })
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
  if (!props.alignTo) return undefined
  return { top: `${tops.value.get(id) ?? 0}px` }
}
</script>

<template>
  <aside class="flex w-72 shrink-0 flex-col" aria-label="Comments" :data-testid="`${testid}-review`">
    <div class="sticky top-0 z-10 flex flex-col gap-2 bg-inherit pt-3 pb-2">
      <div class="flex items-center gap-1">
        <span class="font-mono text-micro tracking-[0.12em] text-text-4 uppercase">
          Comments<template v-if="comments.length"> · {{ comments.length }}</template>
        </span>
        <div class="flex-1" />
        <IconButton
          :label="copyStatus === 'success' ? 'Copied' : 'Copy feedback'"
          :icon="copyStatus === 'success' ? IconCheck : IconCopy"
          :class="{ '!text-severity-error': copyStatus === 'error' }"
          :data-testid="`${testid}-review-copy`"
          @click="copyFeedback"
        />
        <IconButton
          label="Clear comments"
          :icon="IconTrash"
          :data-testid="`${testid}-review-clear`"
          @click="confirming = 'clear'"
        />
        <IconButton
          v-if="closable"
          label="Hide comments"
          :icon="IconX"
          :data-testid="`${testid}-review-close`"
          @click="emit('close')"
        />
      </div>
      <BaseButton size="xs" class="w-full" :data-testid="`${testid}-review-send`" @click="sending = true">
        <template #icon><IconSend class="size-3.5" /></template>
        Send to Agent
      </BaseButton>
      <div
        v-if="!current"
        class="flex flex-col gap-1.5 rounded-lg bg-severity-warning-tint px-2.5 py-2 text-small text-text-2"
        :data-testid="`${testid}-review-stale`"
      >
        These comments are on an earlier version of this document.
        <BaseButton
          variant="secondary"
          size="xs"
          class="self-start"
          :data-testid="`${testid}-review-discard`"
          @click="confirming = 'discard'"
        >
          Discard
        </BaseButton>
      </div>
      <InlineError v-if="error" variant="line" :message="error" :testid="`${testid}-review-error`" />
      <InlineConfirm
        v-if="confirming"
        class="rounded-lg"
        :title="confirming === 'discard' ? 'Discard these comments?' : 'Clear every comment?'"
        description="The comments and the note are deleted."
        :confirm-label="confirming === 'discard' ? 'Discard' : 'Clear'"
        :busy="clearing"
        :testid="`${testid}-review-confirm`"
        @confirm="clear"
        @cancel="confirming = null"
      />
    </div>

    <TextArea
      v-model="note"
      :rows="2"
      size="sm"
      class="mb-2"
      placeholder="General note"
      :data-testid="`${testid}-review-note`"
    />

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
        class="group flex cursor-pointer flex-col gap-1 rounded-lg border bg-raised px-3 py-2.5 shadow-sm transition-[top,border-color] duration-150"
        :class="[
          alignTo ? 'absolute inset-x-0' : '',
          store.active.value === comment.id ? 'border-accent' : 'border-card hover:border-strong',
        ]"
        :style="cardStyle(comment.id)"
        :data-testid="`${testid}-review-card-${index + 1}`"
        @click="pick(comment)"
      >
        <div class="flex items-center gap-2">
          <span
            class="flex h-[18px] min-w-[18px] shrink-0 items-center justify-center rounded-full bg-accent px-1 font-mono text-micro text-accent-contrast"
            >{{ index + 1 }}</span
          >
          <span class="font-mono text-caption text-text-4">{{ lines(comment) }}</span>
          <div class="flex-1" />
          <div
            v-if="editing !== comment.id"
            class="flex opacity-0 group-hover:opacity-100 focus-within:opacity-100"
            @click.stop
          >
            <IconButton
              label="Edit comment"
              :icon="IconPencil"
              size="sm"
              :data-testid="`${testid}-review-edit-${index + 1}`"
              @click="startEdit(comment)"
            />
            <IconButton
              label="Delete comment"
              :icon="IconTrash"
              size="sm"
              tone="danger"
              :data-testid="`${testid}-review-delete-${index + 1}`"
              @click="remove(comment.id)"
            />
          </div>
        </div>
        <p class="line-clamp-2 border-l-2 border-border pl-2 text-caption text-text-3 italic" :title="comment.context">
          {{ comment.context }}
        </p>
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
        <p v-else class="text-small break-words whitespace-pre-wrap text-text">{{ comment.text }}</p>
      </li>
    </ol>

    <SendToAgentDialog
      v-if="sending"
      :space="space"
      :path="path"
      :agent="agent"
      :testid="testid"
      @close="sending = false"
    />
  </aside>
</template>
