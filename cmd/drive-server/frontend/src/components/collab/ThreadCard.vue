<script setup lang="ts">
import CommentCard from '@hive/ui/components/review/CommentCard.vue'
import CommentEntry from '@hive/ui/components/review/CommentEntry.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { computed, ref } from 'vue'
import type { DocumentCollab } from '../../lib/collab'
import { relativeTime } from '../../lib/format'
import type { Comment, Thread, Viewer } from '../../types'
import CommentComposer from './CommentComposer.vue'

const props = defineProps<{
  thread: Thread
  viewer: Viewer | null
  collab: Pick<DocumentCollab, 'reply' | 'setResolved' | 'editComment' | 'deleteComment'>
  active?: boolean
  hovered?: boolean
  /** Beside its highlight, so the quote is already in view. */
  aligned?: boolean
  top?: number
  sliding?: boolean
}>()
const emit = defineEmits<{ select: [id: string]; hover: [id: string | null] }>()

const replying = ref(false)
const editing = ref<string | null>(null)
const busy = ref(false)
const error = ref<string | null>(null)

const me = computed(() => props.viewer?.user_id ?? '')
const canComment = computed(() => !!props.viewer?.can_comment)
const canResolve = computed(
  () => canComment.value && (!!props.viewer?.is_owner || (me.value !== '' && props.thread.author.id === me.value)),
)
const resolved = computed(() => !!props.thread.resolved_at)

const mine = (c: Comment) => me.value !== '' && c.author.id === me.value
const canDelete = (c: Comment) => canComment.value && (mine(c) || !!props.viewer?.is_owner)

const lines = computed(() => {
  const a = props.thread.anchor
  if (!a) return undefined
  return a.start_line === a.end_line ? `L${a.start_line}` : `L${a.start_line}–${a.end_line}`
})

function age(c: Comment): string {
  const at = relativeTime(c.created_at)
  return c.edited_at ? `${at} (edited)` : at
}

function initial(name: string): string {
  return (name.trim()[0] ?? '?').toUpperCase()
}

async function saveEdit(id: string, text: string): Promise<void> {
  busy.value = true
  error.value = await props.collab.editComment(id, text)
  busy.value = false
  if (!error.value) editing.value = null
}

async function toggleResolved(): Promise<void> {
  error.value = await props.collab.setResolved(props.thread.id, !resolved.value)
}

async function remove(id: string): Promise<void> {
  error.value = await props.collab.deleteComment(id)
}
</script>

<template>
  <CommentCard
    :active="active"
    :hovered="hovered"
    :aligned="aligned"
    :top="top"
    :sliding="sliding"
    :data-testid="`thread-${thread.id}`"
    @select="emit('select', thread.id)"
    @hover="emit('hover', $event ? thread.id : null)"
  >
    <blockquote
      v-if="thread.anchor && !aligned"
      class="truncate border-l-2 pl-2 text-caption text-text-3"
      :class="thread.outdated ? 'border-text-4 line-through decoration-text-4/60' : 'border-accent/60'"
      :title="thread.anchor.quote"
      data-testid="thread-quote"
    >
      {{ thread.anchor.quote }}
    </blockquote>
    <CommentEntry
      v-for="(c, index) in thread.comments"
      :key="c.id"
      :class="{ 'border-t border-row pt-2': index > 0 }"
      :author="c.author.name"
      :author-title="c.author.email || undefined"
      :age="age(c)"
      :aside="index === 0 ? lines : undefined"
      :aside-title="thread.anchor?.quote"
      :text="c.body"
      :deleted="c.deleted"
      :editing="editing === c.id"
      :can-edit="mine(c)"
      :can-delete="canDelete(c)"
      :active="active"
      :busy="busy"
      :testids="{
        body: 'comment-body',
        deleted: 'comment-deleted',
        edit: 'comment-edit',
        delete: 'comment-delete',
        editText: 'edit-input',
        editSave: 'edit-submit',
      }"
      :data-testid="`comment-${c.id}`"
      @edit="editing = c.id"
      @cancel="editing = null"
      @save="saveEdit(c.id, $event)"
      @delete="remove(c.id)"
    >
      <template #badge>
        <span
          class="flex size-4.5 shrink-0 items-center justify-center rounded-full bg-accent text-micro font-semibold text-accent-contrast"
          aria-hidden="true"
          >{{ initial(c.author.name) }}</span
        >
      </template>
      <template v-if="c.author.agent" #author>
        <BaseBadge tone="accent" class="shrink-0 px-1.5 text-micro" data-testid="agent-badge">
          agent · {{ c.author.agent }}
        </BaseBadge>
      </template>
    </CommentEntry>
    <InlineError v-if="error" variant="line" :message="error" testid="thread-error" />
    <div v-if="replying" @click.stop>
      <CommentComposer
        :submit="(text) => collab.reply(thread.id, text)"
        placeholder="Reply"
        submit-label="Reply"
        autofocus
        cancelable
        testid="reply"
        @cancel="replying = false"
        @done="replying = false"
      />
    </div>
    <div v-else-if="canComment" class="flex gap-3.5 border-t border-row pt-2 text-caption text-text-3" @click.stop>
      <button
        type="button"
        class="-mx-1 cursor-pointer rounded px-1 hover:bg-chip hover:text-text"
        data-testid="thread-reply"
        @click="replying = true"
      >
        Reply
      </button>
      <button
        v-if="canResolve"
        type="button"
        class="-mx-1 ml-auto cursor-pointer rounded px-1 hover:bg-chip hover:text-text"
        data-testid="thread-resolve"
        @click="toggleResolved"
      >
        {{ resolved ? 'Reopen' : 'Resolve' }}
      </button>
    </div>
  </CommentCard>
</template>
