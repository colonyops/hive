<script setup lang="ts">
// The comments on one Markdown document, under its toolbar: each comment with
// its lines and quote, a general note, and the ways out -- copy, send to an
// agent, or clear.
import { computed, ref } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconPencil from '~icons/lucide/pencil'
import IconSend from '~icons/lucide/send'
import IconTrash from '~icons/lucide/trash-2'
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
  /**
   * The reader's testid, whose `-document` holds the marked blocks. Derives `-review`, `-review-row-<n>`,
   * `-review-edit-<n>`, `-review-delete-<n>`, `-review-note`, `-review-copy`, `-review-send`, `-review-clear`,
   * `-review-stale`, `-review-discard`, and `-review-error`.
   */
  testid: string
}>()

const store = useDriveReview()
const review = computed(() => store.review(props.space, props.path))
const comments = computed(() => review.value.data.value.comments ?? [])
const current = computed(() => review.value.data.value.current)
const note = computed({
  get: () => store.note(props.space, props.path),
  set: (text: string) => store.setNote(props.space, props.path, text),
})
const shown = computed(() => comments.value.length > 0 || note.value !== '')

const error = ref<string | null>(null)
const editing = ref<string | null>(null)
const draft = ref('')
const confirming = ref<'clear' | 'discard' | null>(null)
const clearing = ref(false)
const sending = ref(false)

function lines(comment: CommentWire): string {
  return comment.startLine === comment.endLine ? `L${comment.startLine}` : `L${comment.startLine}-${comment.endLine}`
}

function quote(comment: CommentWire): string {
  return comment.context.split('\n')[0]
}

function reveal(comment: CommentWire): void {
  const selector = `[data-testid="${props.testid}-document"] [data-review-ids~="${CSS.escape(comment.id)}"]`
  document.querySelector(selector)?.scrollIntoView({ block: 'center', behavior: 'smooth' })
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
</script>

<template>
  <section
    v-if="shown"
    class="flex max-h-[45%] shrink-0 flex-col gap-2 overflow-y-auto border-b border-border bg-pane px-3 py-2"
    :data-testid="`${testid}-review`"
  >
    <div
      v-if="!current"
      class="flex items-center gap-2 rounded-lg bg-severity-warning-tint px-2.5 py-1.5 text-small text-text-2"
      :data-testid="`${testid}-review-stale`"
    >
      <span class="flex-1">These comments are on an earlier version of this document.</span>
      <BaseButton variant="ghost" size="xs" :data-testid="`${testid}-review-discard`" @click="confirming = 'discard'">
        Discard
      </BaseButton>
    </div>

    <ol class="flex flex-col gap-1">
      <li
        v-for="(comment, index) in comments"
        :key="comment.id"
        class="group flex items-start gap-2 rounded-lg px-1.5 py-1 hover:bg-chip"
        :data-testid="`${testid}-review-row-${index + 1}`"
      >
        <span
          class="mt-0.5 flex h-[18px] min-w-[18px] shrink-0 items-center justify-center rounded-full bg-accent px-1 font-mono text-micro text-accent-contrast"
          >{{ index + 1 }}</span
        >
        <div class="flex min-w-0 flex-1 flex-col gap-0.5">
          <button
            type="button"
            class="flex min-w-0 cursor-pointer items-baseline gap-2 text-left"
            :title="comment.context"
            @click="reveal(comment)"
          >
            <span class="shrink-0 font-mono text-caption text-text-4">{{ lines(comment) }}</span>
            <span class="min-w-0 truncate text-caption text-text-3 italic">{{ quote(comment) }}</span>
          </button>
          <template v-if="editing === comment.id">
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
          </template>
          <p v-else class="text-small break-words whitespace-pre-wrap text-text">{{ comment.text }}</p>
        </div>
        <div
          v-if="editing !== comment.id"
          class="flex shrink-0 opacity-0 group-hover:opacity-100 focus-within:opacity-100"
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
      </li>
    </ol>

    <TextArea v-model="note" :rows="2" size="sm" placeholder="General note" :data-testid="`${testid}-review-note`" />
    <InlineError v-if="error" variant="line" :message="error" :testid="`${testid}-review-error`" />

    <div class="flex items-center gap-2">
      <BaseButton variant="secondary" size="xs" :data-testid="`${testid}-review-copy`" @click="copyFeedback">
        <template #icon>
          <IconCheck v-if="copyStatus === 'success'" class="size-3.5" />
          <IconCopy v-else class="size-3.5" />
        </template>
        {{ copyStatus === 'success' ? 'Copied' : 'Copy feedback' }}
      </BaseButton>
      <BaseButton variant="secondary" size="xs" :data-testid="`${testid}-review-send`" @click="sending = true">
        <template #icon><IconSend class="size-3.5" /></template>
        Send to Agent
      </BaseButton>
      <div class="flex-1" />
      <BaseButton variant="ghost" size="xs" :data-testid="`${testid}-review-clear`" @click="confirming = 'clear'">
        Clear
      </BaseButton>
    </div>

    <InlineConfirm
      v-if="confirming"
      :title="confirming === 'discard' ? 'Discard these comments?' : 'Clear every comment?'"
      description="The comments and the note are deleted."
      :confirm-label="confirming === 'discard' ? 'Discard' : 'Clear'"
      :busy="clearing"
      :testid="`${testid}-review-confirm`"
      @confirm="clear"
      @cancel="confirming = null"
    />

    <SendToAgentDialog
      v-if="sending"
      :space="space"
      :path="path"
      :agent="agent"
      :testid="testid"
      @close="sending = false"
    />
  </section>
</template>
