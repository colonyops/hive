<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import TextArea from '@hive/ui/components/ui/TextArea.vue'
import IconPencil from '~icons/lucide/pencil'
import IconTrash from '~icons/lucide/trash-2'
import IconUserRound from '~icons/lucide/user-round'
import IconX from '~icons/lucide/x'
import AgentMarkdown from './AgentMarkdown.vue'
import DiffThreadMessage from './DiffThreadMessage.vue'
import { agentIconID } from '../../lib/agentIcon'
import type { AgentComment } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'
import { USER_AUTHOR, type ThreadResponse } from '../../stores/useChangesReview'

const props = defineProps<{
  comment: AgentComment
  /** The rows under it no longer read as the agent quoted them. */
  stale?: boolean
  /** The walkthrough is on this comment. */
  focused?: boolean
  /** The thread under the comment, oldest first. It goes out with the comment on Send. */
  responses?: readonly ThreadResponse[]
}>()
const emit = defineEmits<{
  dismiss: []
  pick: []
  respond: [text: string]
  /** Blank text removes the response. */
  'edit-response': [responseId: string, text: string]
}>()

const agent = computed(() => agentIconID(props.comment.agent || props.comment.author))
const label = computed(() =>
  commentLabel({ ...props.comment, kind: props.comment.kind === 'file' ? 'file' : undefined }),
)
const testid = computed(() => `agent-comment-${props.comment.id}`)
const PILLS: Record<string, string> = {
  issue: 'bg-severity-error-tint text-severity-error',
  suggestion: 'bg-severity-info-tint text-severity-info',
  note: 'bg-chip text-text-3',
}
const pill = computed(() => PILLS[props.comment.severity] ?? PILLS.note)

// One editor at a time: the new response at the bottom (null id) or an existing one.
const editing = ref<{ id: string | null } | null>(null)
const draft = ref('')
let field: { focus: () => void } | null = null
function setField(element: unknown): void {
  field = element as { focus: () => void } | null
}

function authorName(response: ThreadResponse): string {
  return response.author === USER_AUTHOR ? 'You' : response.author
}

async function open(id: string | null, text = ''): Promise<void> {
  draft.value = text
  editing.value = { id }
  await nextTick()
  field?.focus()
}

const startReply = () => open(null)

function save(): void {
  const target = editing.value
  if (!target || !draft.value.trim()) return
  if (target.id) emit('edit-response', target.id, draft.value.trim())
  else emit('respond', draft.value.trim())
  editing.value = null
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    event.stopPropagation()
    save()
  } else if (event.key === 'Escape') {
    event.stopPropagation()
    editing.value = null
  }
}

defineExpose({ startReply })
</script>

<template>
  <!-- A pointer press here must not reach CodeView, which would start a line selection under the card. -->
  <div
    class="group flex max-w-3xl flex-col divide-y divide-border rounded-md border bg-raised px-3 font-sans text-body"
    :class="focused ? 'border-accent shadow-[0_0_0_1px_var(--color-accent)]' : 'border-card'"
    :data-testid="testid"
    :data-focused="focused || undefined"
    @pointerdown.stop
    @click.stop
  >
    <DiffThreadMessage :agent="agent">
      <template #header>
        <span class="shrink-0 font-medium text-text">{{ comment.author }}</span>
        <span
          class="shrink-0 rounded-full px-2 text-caption leading-5"
          :class="pill"
          :data-testid="`${testid}-severity`"
          >{{ comment.severity }}</span
        >
        <span
          v-if="stale"
          class="size-1.5 shrink-0 rounded-full bg-severity-warning"
          title="The rows under this comment changed after the agent wrote it"
          aria-label="Lines changed"
          :data-testid="`${testid}-stale`"
        />
        <button
          type="button"
          class="min-w-0 cursor-pointer truncate font-mono text-caption text-text-3 hover:text-text"
          :title="`${comment.path} ${label}`"
          :data-testid="`${testid}-pick`"
          @click="emit('pick')"
        >
          {{ label }}
        </button>
        <IconButton
          class="ml-auto"
          :class="focused ? '' : 'opacity-0 group-focus-within:opacity-100 group-hover:opacity-100'"
          :label="focused ? 'Dismiss (d)' : 'Dismiss'"
          :icon="IconX"
          size="md"
          :data-testid="`${testid}-dismiss`"
          @click="emit('dismiss')"
        />
      </template>
      <AgentMarkdown class="leading-relaxed text-text" :source="comment.text" />
    </DiffThreadMessage>

    <template v-for="response in responses ?? []" :key="response.id">
      <div v-if="editing?.id === response.id" class="py-2.5">
        <TextArea
          :ref="setField"
          v-model="draft"
          :rows="2"
          size="sm"
          :data-testid="`${testid}-response-${response.id}-text`"
          @keydown="onKeydown"
        />
        <div class="mt-1.5 flex justify-end gap-1.5">
          <BaseButton variant="ghost" size="xs" @click="editing = null">Cancel</BaseButton>
          <BaseButton
            size="xs"
            :disabled="!draft.trim()"
            :data-testid="`${testid}-response-${response.id}-save`"
            @click="save"
          >
            Save
          </BaseButton>
        </div>
      </div>
      <DiffThreadMessage
        v-else
        :agent="response.author === USER_AUTHOR ? 'user' : agentIconID(response.author)"
        :data-testid="`${testid}-response-${response.id}`"
      >
        <template #header>
          <span class="shrink-0 font-medium text-text">{{ authorName(response) }}</span>
          <span
            v-if="response.author === USER_AUTHOR"
            class="ml-auto flex shrink-0 items-center gap-0.5 opacity-0 group-focus-within/message:opacity-100 group-hover/message:opacity-100"
          >
            <IconButton
              label="Edit reply"
              :icon="IconPencil"
              size="md"
              :data-testid="`${testid}-response-${response.id}-edit`"
              @click="open(response.id, response.text)"
            />
            <IconButton
              label="Delete reply"
              :icon="IconTrash"
              size="md"
              :data-testid="`${testid}-response-${response.id}-delete`"
              @click="emit('edit-response', response.id, '')"
            />
          </span>
        </template>
        <p class="leading-relaxed break-words whitespace-pre-wrap text-text">{{ response.text }}</p>
      </DiffThreadMessage>
    </template>

    <div v-if="editing && editing.id === null" class="grid grid-cols-[1.5rem_minmax(0,1fr)] gap-x-2.5 py-2.5">
      <span class="mt-1 flex size-6 items-center justify-center rounded-full bg-chip text-text-2" aria-hidden="true">
        <IconUserRound class="size-3.5" />
      </span>
      <div class="flex flex-col gap-1.5">
        <TextArea
          :ref="setField"
          v-model="draft"
          :rows="2"
          size="sm"
          placeholder="Reply with context for the agent"
          :data-testid="`${testid}-reply-text`"
          @keydown="onKeydown"
        />
        <div class="flex justify-end gap-1.5">
          <BaseButton variant="ghost" size="xs" @click="editing = null">Cancel</BaseButton>
          <BaseButton size="xs" :disabled="!draft.trim()" :data-testid="`${testid}-reply-save`" @click="save">
            Reply
          </BaseButton>
        </div>
      </div>
    </div>
    <div v-else class="grid grid-cols-[1.5rem_minmax(0,1fr)] items-center gap-x-2.5 py-2.5">
      <span class="flex size-6 items-center justify-center rounded-full bg-chip text-text-2" aria-hidden="true">
        <IconUserRound class="size-3.5" />
      </span>
      <button
        type="button"
        class="flex h-8 cursor-text items-center rounded-lg border border-border bg-app px-2.75 text-left text-text-4 hover:border-strong"
        :data-testid="`${testid}-reply-start`"
        @click="startReply"
      >
        {{ responses?.length ? 'Add to the thread' : 'Reply with context for the agent'
        }}<span v-if="focused" class="ml-1 font-mono text-caption">(r)</span>
      </button>
    </div>
  </div>
</template>
