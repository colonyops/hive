<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import TextArea from '@hive/ui/components/ui/TextArea.vue'
import IconPencil from '~icons/lucide/pencil'
import IconReply from '~icons/lucide/reply'
import IconTrash from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'
import AgentMark from '../AgentMark.vue'
import AgentMarkdown from './AgentMarkdown.vue'
import { agentIconID } from '../../lib/agentIcon'
import type { AgentComment } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'

const props = defineProps<{
  comment: AgentComment
  /** The rows under it no longer read as the agent quoted them. */
  stale?: boolean
  /** The walkthrough is on this comment. */
  focused?: boolean
  /** What the user wrote back, which goes out with the comment on Send. */
  reply?: string
}>()
const emit = defineEmits<{ dismiss: []; pick: []; reply: [text: string] }>()

const agent = computed(() => agentIconID(props.comment.agent || props.comment.author))
const label = computed(() =>
  commentLabel({ ...props.comment, kind: props.comment.kind === 'file' ? 'file' : undefined }),
)
const testid = computed(() => `agent-comment-${props.comment.id}`)
// Inset from the card's edges, so the rounded corners stay whole.
const BARS: Record<string, string> = {
  issue: 'bg-severity-error',
  suggestion: 'bg-severity-info',
  note: 'bg-text-4',
}
const bar = computed(() => BARS[props.comment.severity] ?? BARS.note)

const replying = ref(false)
const draft = ref('')
const field = ref<{ focus: () => void } | null>(null)

async function startReply(): Promise<void> {
  draft.value = props.reply ?? ''
  replying.value = true
  await nextTick()
  field.value?.focus()
}

function saveReply(): void {
  if (!draft.value.trim()) return
  emit('reply', draft.value.trim())
  replying.value = false
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    event.stopPropagation()
    saveReply()
  } else if (event.key === 'Escape') {
    event.stopPropagation()
    replying.value = false
  }
}

defineExpose({ startReply })
</script>

<template>
  <!-- A pointer press here must not reach CodeView, which would start a line selection under the card. -->
  <div
    class="group relative flex max-w-3xl flex-col gap-1.5 rounded-md border bg-raised py-2 pr-3 pl-5 font-sans text-body"
    :class="focused ? 'border-accent shadow-[0_0_0_1px_var(--color-accent)]' : 'border-card'"
    :data-testid="testid"
    :data-focused="focused || undefined"
    @pointerdown.stop
    @click.stop
  >
    <span
      class="absolute top-2 bottom-2 left-2 w-[3px] rounded-full"
      :class="bar"
      :title="comment.severity"
      :aria-label="comment.severity"
      :data-testid="`${testid}-bar`"
    />
    <div class="flex h-6 min-w-0 items-center gap-2 text-small">
      <span class="flex shrink-0 items-center gap-1.5 font-medium text-text">
        <AgentMark :agent="agent" />
        {{ comment.author }}
      </span>
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
    </div>
    <AgentMarkdown class="leading-relaxed text-text" :source="comment.text" />
    <div v-if="replying" class="flex flex-col gap-1.5 border-t border-card pt-2">
      <TextArea
        ref="field"
        v-model="draft"
        :rows="2"
        size="sm"
        placeholder="Reply with context for the agent"
        :data-testid="`${testid}-reply-text`"
        @keydown="onKeydown"
      />
      <div class="flex justify-end gap-1.5">
        <BaseButton variant="ghost" size="xs" @click="replying = false">Cancel</BaseButton>
        <BaseButton size="xs" :disabled="!draft.trim()" :data-testid="`${testid}-reply-save`" @click="saveReply">
          Reply
        </BaseButton>
      </div>
    </div>
    <div
      v-else-if="reply"
      class="group/reply flex flex-col gap-0.5 border-t border-card pt-2"
      :data-testid="`${testid}-reply`"
    >
      <div class="flex h-5 items-center gap-1.5 text-small text-text-3">
        <IconReply class="size-3.5" aria-hidden="true" />
        <span class="font-medium text-text-2">You</span>
        <span
          class="ml-auto flex items-center gap-0.5 opacity-0 group-focus-within/reply:opacity-100 group-hover/reply:opacity-100"
        >
          <IconButton
            label="Edit reply"
            :icon="IconPencil"
            size="sm"
            :data-testid="`${testid}-reply-edit`"
            @click="startReply"
          />
          <IconButton
            label="Delete reply"
            :icon="IconTrash"
            size="sm"
            :data-testid="`${testid}-reply-delete`"
            @click="emit('reply', '')"
          />
        </span>
      </div>
      <p class="leading-relaxed break-words whitespace-pre-wrap text-text">{{ reply }}</p>
    </div>
    <div v-else class="flex">
      <BaseButton variant="ghost" size="xs" :data-testid="`${testid}-reply-start`" @click="startReply">
        <IconReply class="size-3.5" aria-hidden="true" />
        Reply
      </BaseButton>
    </div>
  </div>
</template>
