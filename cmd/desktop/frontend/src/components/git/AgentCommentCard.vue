<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import TextArea from '@hive/ui/components/ui/TextArea.vue'
import AgentMark from '../AgentMark.vue'
import AgentSeverity from './AgentSeverity.vue'
import { agentIconID } from '../../lib/agentIcon'
import type { AgentComment } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'
import { gitFileName } from '../../lib/gitTree'

const props = defineProps<{
  comment: AgentComment
  /** The rows under it no longer read as the agent quoted them. */
  stale?: boolean
  /** Triage is on this comment. */
  focused?: boolean
  /** Triage asked for the text to be edited before it is adopted. */
  editing?: boolean
  /** Shows the triage keys on the buttons. */
  keys?: boolean
}>()
const emit = defineEmits<{ adopt: [text?: string]; dismiss: []; edit: []; cancel: []; pick: [] }>()

const agent = computed(() => agentIconID(props.comment.agent || props.comment.author))
const label = computed(() =>
  commentLabel({ ...props.comment, kind: props.comment.kind === 'file' ? 'file' : undefined }),
)
const testid = computed(() => `agent-comment-${props.comment.id}`)

const draft = ref('')
watch(
  () => props.editing,
  (editing) => {
    if (editing) draft.value = props.comment.text
  },
  { immediate: true },
)

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    event.stopPropagation()
    if (draft.value.trim()) emit('adopt', draft.value.trim())
  } else if (event.key === 'Escape') {
    event.stopPropagation()
    emit('cancel')
  }
}
</script>

<template>
  <!-- A pointer press here must not reach CodeView, which would start a line selection under the card. -->
  <div
    class="flex max-w-3xl flex-col gap-1 rounded-md border border-l-[3px] border-node-purple/45 border-l-node-purple bg-node-purple-tint px-2.5 py-1.5 font-sans text-caption transition-shadow"
    :class="focused ? 'shadow-[0_0_0_1px_var(--color-node-purple)]' : ''"
    :data-testid="testid"
    :data-focused="focused || undefined"
    @pointerdown.stop
    @click.stop
  >
    <div class="flex h-4.5 min-w-0 items-center gap-1.5">
      <span class="flex shrink-0 items-center gap-1 font-semibold text-node-purple">
        <AgentMark :agent="agent" />
        {{ comment.author }}
      </span>
      <AgentSeverity :severity="comment.severity" />
      <span
        v-if="stale"
        class="flex shrink-0 items-center gap-1 text-severity-warning"
        title="The rows under this comment changed after the agent wrote it"
        :data-testid="`${testid}-stale`"
      >
        <span class="size-1.5 rounded-full bg-severity-warning" aria-hidden="true" />
        lines changed
      </span>
      <button
        type="button"
        class="ml-auto min-w-0 cursor-pointer truncate font-mono text-micro text-text-3 hover:text-text"
        :title="`${comment.path} ${label}`"
        :data-testid="`${testid}-pick`"
        @click="emit('pick')"
      >
        {{ gitFileName(comment.path) }} {{ label }}
      </button>
    </div>
    <div v-if="editing" class="flex flex-col gap-1">
      <TextArea
        v-model="draft"
        :rows="3"
        size="sm"
        :data-testid="`${testid}-edit-text`"
        autofocus
        @keydown="onKeydown"
      />
    </div>
    <p v-else class="leading-snug break-words whitespace-pre-wrap text-text">{{ comment.text }}</p>
    <div class="flex flex-wrap items-center gap-1.5 pt-0.5">
      <template v-if="editing">
        <BaseButton
          size="xs"
          :disabled="!draft.trim()"
          :data-testid="`${testid}-save`"
          @click="emit('adopt', draft.trim())"
        >
          Adopt edited
        </BaseButton>
        <BaseButton variant="ghost" size="xs" @click="emit('cancel')">Cancel</BaseButton>
      </template>
      <template v-else>
        <BaseButton size="xs" :data-testid="`${testid}-adopt`" @click="emit('adopt')">
          Adopt<span v-if="keys" class="ml-1 font-mono opacity-60">a</span>
        </BaseButton>
        <BaseButton variant="ghost" size="xs" :data-testid="`${testid}-edit`" @click="emit('edit')">
          Edit, then adopt<span v-if="keys" class="ml-1 font-mono opacity-60">e</span>
        </BaseButton>
        <BaseButton variant="ghost" size="xs" :data-testid="`${testid}-dismiss`" @click="emit('dismiss')">
          Dismiss<span v-if="keys" class="ml-1 font-mono opacity-60">d</span>
        </BaseButton>
      </template>
    </div>
  </div>
</template>
